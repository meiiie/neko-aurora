// Package vault is the server-side secret store. Values are sealed with
// AES-256-GCM (AAD = secret name) under a random key kept in vault.key (0600)
// next to vapid.pem in the state dir. Only names and metadata ever leave the
// package through List; plaintext is reachable only via Get for in-process
// use (e.g. SMTP auth) and is never journaled or logged.
package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrName    = errors.New("invalid_secret_name")
	ErrValue   = errors.New("invalid_secret_value")
	ErrMissing = errors.New("secret_not_found")
	ErrCorrupt = errors.New("vault_corrupt")
)

var nameRe = regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,63}$`)

func ValidName(n string) bool { return nameRe.MatchString(n) }

// Meta is the public description of a secret. Hint is an optional
// non-secret display string (e.g. the Gmail address) set by the caller.
type Meta struct {
	Name    string `json:"name"`
	Label   string `json:"label,omitempty"`
	Kind    string `json:"kind,omitempty"`
	Hint    string `json:"hint,omitempty"`
	Updated string `json:"updated"`
}

type entry struct {
	Meta
	V     int    `json:"v,omitempty"`
	Nonce []byte `json:"nonce"`
	CT    []byte `json:"ct"`
}

type Vault struct {
	mu    sync.RWMutex
	path  string
	aead  cipher.AEAD
	items map[string]entry
	cache map[string]string // decrypted, in memory only (for redaction)
}

// aad binds a ciphertext to its name under a versioned, domain-separated
// label (envelope versioning after CopilotKit/openmuse vault.ts, MIT).
// v0 entries (first release) used the bare name.
func aad(v int, name string) []byte {
	if v == 0 {
		return []byte(name)
	}
	return []byte("neko-aurora:secret:v1:" + name)
}

func Open(dir string) (*Vault, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	kp := filepath.Join(dir, "vault.key")
	key, err := os.ReadFile(kp)
	if os.IsNotExist(err) {
		key = make([]byte, 32)
		if _, err = io.ReadFull(rand.Reader, key); err != nil {
			return nil, err
		}
		f, err := os.OpenFile(kp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return nil, err
		}
		_, err = f.Write(key)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	if len(key) != 32 {
		return nil, ErrCorrupt
	}
	_ = os.Chmod(kp, 0o600)
	blk, _ := aes.NewCipher(key)
	aead, _ := cipher.NewGCM(blk)
	v := &Vault{path: filepath.Join(dir, "vault.json"), aead: aead, items: map[string]entry{}, cache: map[string]string{}}
	b, err := os.ReadFile(v.path)
	if err == nil {
		var list []entry
		if json.Unmarshal(b, &list) != nil {
			return nil, ErrCorrupt
		}
		for _, e := range list {
			pt, err := aead.Open(nil, e.Nonce, e.CT, aad(e.V, e.Name))
			if err != nil || !ValidName(e.Name) || (e.V != 0 && e.V != 1) || len(e.Nonce) != aead.NonceSize() {
				return nil, ErrCorrupt
			}
			v.items[e.Name], v.cache[e.Name] = e, string(pt)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return v, nil
}

func (v *Vault) save() error {
	list := make([]entry, 0, len(v.items))
	for _, e := range v.items {
		list = append(list, e)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	b, _ := json.MarshalIndent(list, "", " ")
	tmp := v.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, v.path)
}

func (v *Vault) Set(m Meta, value string) (Meta, error) {
	if v == nil {
		return Meta{}, ErrMissing
	}
	if !ValidName(m.Name) {
		return Meta{}, ErrName
	}
	if value == "" || len(value) > 8192 || strings.ContainsAny(value, "\x00") {
		return Meta{}, ErrValue
	}
	nonce := make([]byte, v.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return Meta{}, err
	}
	m.Updated = time.Now().UTC().Format(time.RFC3339)
	v.mu.Lock()
	defer v.mu.Unlock()
	old, had := v.items[m.Name]
	oldv := v.cache[m.Name]
	v.items[m.Name] = entry{Meta: m, V: 1, Nonce: nonce, CT: v.aead.Seal(nil, nonce, []byte(value), aad(1, m.Name))}
	v.cache[m.Name] = value
	if err := v.save(); err != nil {
		if had {
			v.items[m.Name], v.cache[m.Name] = old, oldv
		} else {
			delete(v.items, m.Name)
			delete(v.cache, m.Name)
		}
		return Meta{}, err
	}
	return m, nil
}

func (v *Vault) Delete(name string) error {
	if v == nil {
		return ErrMissing
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if _, ok := v.items[name]; !ok {
		return ErrMissing
	}
	delete(v.items, name)
	delete(v.cache, name)
	return v.save()
}

// Get returns a plaintext value for in-process use only.
func (v *Vault) Get(name string) (string, bool) {
	if v == nil {
		return "", false
	}
	v.mu.RLock()
	defer v.mu.RUnlock()
	s, ok := v.cache[name]
	return s, ok
}

func (v *Vault) Has(name string) bool { _, ok := v.Get(name); return ok }

func (v *Vault) Meta(name string) (Meta, bool) {
	if v == nil {
		return Meta{}, false
	}
	v.mu.RLock()
	defer v.mu.RUnlock()
	e, ok := v.items[name]
	return e.Meta, ok
}

func (v *Vault) List() []Meta {
	out := []Meta{}
	if v == nil {
		return out
	}
	v.mu.RLock()
	defer v.mu.RUnlock()
	for _, e := range v.items {
		out = append(out, e.Meta)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Redact replaces every stored secret value (4+ bytes) in s with «NAME».
func (v *Vault) Redact(s string) string {
	if v == nil || s == "" {
		return s
	}
	v.mu.RLock()
	defer v.mu.RUnlock()
	for n, val := range v.cache {
		if len(val) >= 4 && v.items[n].Kind != "plain" {
			s = strings.ReplaceAll(s, val, "«"+n+"»")
			if c := strings.ReplaceAll(val, " ", ""); c != val && len(c) >= 4 {
				s = strings.ReplaceAll(s, c, "«"+n+"»")
			}
		}
	}
	return s
}

// Writer wraps w so every write is redacted (used for the process log).
func (v *Vault) Writer(w io.Writer) io.Writer { return redactW{v, w} }

type redactW struct {
	v *Vault
	w io.Writer
}

func (r redactW) Write(p []byte) (int, error) {
	if _, err := r.w.Write([]byte(r.v.Redact(string(p)))); err != nil {
		return 0, err
	}
	return len(p), nil
}
