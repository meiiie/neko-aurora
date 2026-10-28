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

	"nekoaurora.local/native/internal/privatefs"
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

func Open(dir string) (*Vault, error) { return open(dir, nil) }

// OpenWithKey uses a caller-held 32-byte key (hosted mode derives one per
// user from AURORA_SECRET) and never writes a key file, so a copy of the
// state volume alone cannot decrypt the vault.
func OpenWithKey(dir string, key []byte) (*Vault, error) {
	if len(key) != 32 {
		return nil, ErrCorrupt
	}
	return open(dir, key)
}

func open(dir string, fixed []byte) (*Vault, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	kp := filepath.Join(dir, "vault.key")
	var key []byte
	var err error
	if fixed != nil {
		key = fixed
	} else {
		if err = privatefs.Protect(kp); err == nil {
			key, err = os.ReadFile(kp)
		}
	}
	if fixed == nil && os.IsNotExist(err) {
		key = make([]byte, 32)
		if _, err = io.ReadFull(rand.Reader, key); err != nil {
			return nil, err
		}
		f, err := privatefs.CreateNew(kp)
		if err != nil {
			return nil, err
		}
		_, err = f.Write(key)
		if err == nil {
			err = f.Sync()
		}
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
	blk, _ := aes.NewCipher(key)
	aead, _ := cipher.NewGCM(blk)
	v := &Vault{path: filepath.Join(dir, "vault.json"), aead: aead, items: map[string]entry{}, cache: map[string]string{}}
	if err = privatefs.Protect(v.path); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	defer register(v)
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
	return privatefs.WriteFile(v.path, b)
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
	old, ok := v.items[name]
	if !ok {
		return ErrMissing
	}
	value := v.cache[name]
	delete(v.items, name)
	delete(v.cache, name)
	if err := v.save(); err != nil {
		v.items[name], v.cache[name] = old, value
		return err
	}
	return nil
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
		if !isItemKind(e.Kind) {
			out = append(out, e.Meta)
		}
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
		if isItemKind(v.items[n].Kind) {
			s = redactItem(s, n, val)
			continue
		}
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

// redactItem masks each secret field of a typed item: «VI_ID.field». A card
// number is also caught with spaces/dashes every 4 digits.
func redactItem(s, n, val string) string {
	var f map[string]string
	if json.Unmarshal([]byte(val), &f) != nil {
		return s
	}
	keys := make([]string, 0, len(f))
	for k := range f {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return len(f[keys[i]]) > len(f[keys[j]]) }) // longest first
	for _, k := range keys {
		x := f[k]
		if plainField[k] || len(x) < 3 || (len(x) < 4 && k != "cvc") {
			continue
		}
		tag := "«" + n + "." + k + "»"
		if k == "cvc" {
			// 3-4 digits are too common to replace everywhere: only next to a CVC label
			s = regexp.MustCompile(`(?i)((?:cvc|cvv|csc|security code|mã bảo mật)\W{0,6})`+regexp.QuoteMeta(x)+`\b`).ReplaceAllString(s, "${1}"+tag)
			continue
		}
		s = strings.ReplaceAll(s, x, tag)
		if k == "number" && len(x) >= 12 {
			for _, sep := range []string{" ", "-"} {
				var g []string
				for i := 0; i < len(x); i += 4 {
					g = append(g, x[i:min(i+4, len(x))])
				}
				s = strings.ReplaceAll(s, strings.Join(g, sep), tag)
			}
		}
	}
	return s
}

// Every open vault, so the shared process log is redacted with all of them
// (hosted mode opens one vault per user; one log.SetOutput serves all).
var (
	regMu sync.RWMutex
	reg   = map[string]*Vault{}
)

func register(v *Vault) {
	regMu.Lock()
	reg[v.path] = v // reopening the same dir replaces the old handle
	regMu.Unlock()
}

// Close drops v from the log redaction set (account deleted).
func (v *Vault) Close() {
	regMu.Lock()
	if reg[v.path] == v {
		delete(reg, v.path)
	}
	regMu.Unlock()
}

// LogWriter redacts every write with every open vault.
func LogWriter(w io.Writer) io.Writer { return allW{w} }

type allW struct{ w io.Writer }

func (a allW) Write(p []byte) (int, error) {
	s := string(p)
	regMu.RLock()
	for _, v := range reg {
		s = v.Redact(s)
	}
	regMu.RUnlock()
	if _, err := a.w.Write([]byte(s)); err != nil {
		return 0, err
	}
	return len(p), nil
}
