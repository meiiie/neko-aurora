package providerexec

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"nekobot.local/native/internal/protocol"
)

type WitnessBody struct {
	Version           int    `json:"version"`
	Purpose           string `json:"purpose"`
	BindingSHA256     string `json:"bindingSHA256"`
	ServiceGeneration string `json:"serviceGeneration"`
	AuthoritySHA256   string `json:"authoritySHA256"`
	Revision          int64  `json:"revision"`
	State             string `json:"state"`
	IssuedAtUnixMS    int64  `json:"issuedAtUnixMS"`
	ValidUntilUnixMS  int64  `json:"validUntilUnixMS"`
}
type WitnessRecord struct {
	WitnessBody
	MAC string `json:"mac"`
}
type WitnessConfig struct {
	Path              string
	Binding           SavedTaskBinding
	ServiceGeneration string
	AuthoritySHA256   string
	RevisionKey       string
	CurrentTask       func(SavedTaskBinding) bool
}
type WitnessController struct {
	mu                   sync.Mutex
	path                 string
	binding              SavedTaskBinding
	bindingHash          string
	boot, authority, key string
	currentTask          func(SavedTaskBinding) bool
	last                 WitnessRecord
	lastBytes            []byte
	terminal             bool
	observed             int64
	openedWall           int64
	openedMono           time.Time
	proofDeadline        time.Time
	now                  func() time.Time
}

func signedWitness(body WitnessBody, key string) ([]byte, error) {
	if !savedHex.MatchString(key) {
		return nil, ErrSavedAuthority
	}
	material, _ := hex.DecodeString(key)
	derive := hmac.New(sha256.New, material)
	derive.Write([]byte("neko-saved-key-v1/authority-witness"))
	derived := derive.Sum(nil)
	clear(material)
	raw, err := savedCanonical(body)
	if err != nil {
		clear(derived)
		return nil, err
	}
	mac := hmac.New(sha256.New, derived)
	mac.Write(raw)
	clear(derived)
	return savedCanonical(WitnessRecord{body, hex.EncodeToString(mac.Sum(nil))})
}
func (c *WitnessController) parse(raw []byte) (WitnessRecord, error) {
	var w WitnessRecord
	canonical, err := protocol.Canonical(raw)
	if err != nil || len(raw) > 4096 || !bytes.Equal(raw, canonical) || !exactSavedKeys(raw, []string{"version", "purpose", "bindingSHA256", "serviceGeneration", "authoritySHA256", "revision", "state", "issuedAtUnixMS", "validUntilUnixMS", "mac"}) || json.Unmarshal(raw, &w) != nil {
		return w, ErrSavedAuthority
	}
	if w.Version != 1 || w.Purpose != "saved_account_authority_v1" || w.BindingSHA256 != c.bindingHash || w.ServiceGeneration != c.boot || w.AuthoritySHA256 != c.authority || !savedPositive(w.Revision, savedSafeInt) || w.IssuedAtUnixMS < 0 || w.IssuedAtUnixMS > savedSafeInt || w.ValidUntilUnixMS < 0 || w.ValidUntilUnixMS > c.binding.ExpiresAtUnixMS || !savedHex.MatchString(w.MAC) {
		return w, ErrSavedAuthority
	}
	if w.State == "active" {
		if w.ValidUntilUnixMS <= w.IssuedAtUnixMS || w.ValidUntilUnixMS-w.IssuedAtUnixMS > 5000 {
			return w, ErrSavedAuthority
		}
	} else if w.State != "retired" || w.ValidUntilUnixMS > w.IssuedAtUnixMS {
		return w, ErrSavedAuthority
	}
	expected, err := signedWitness(w.WitnessBody, c.key)
	if err != nil || !hmac.Equal(expected, raw) {
		return w, ErrSavedAuthority
	}
	return w, nil
}
func OpenWitness(cfg WitnessConfig) (*WitnessController, error) {
	hash, err := SavedTaskBindingHash(cfg.Binding)
	if err != nil || !savedBoot.MatchString(cfg.ServiceGeneration) || !savedHex.MatchString(cfg.AuthoritySHA256) || !savedHex.MatchString(cfg.RevisionKey) {
		return nil, ErrSavedAuthority
	}
	path, err := privateWitnessPath(cfg.Path)
	if err != nil {
		return nil, ErrSavedAuthority
	}
	now := time.Now()
	if cfg.Binding.ExpiresAtUnixMS <= now.UnixMilli() || cfg.Binding.ExpiresAtUnixMS-now.UnixMilli() > cfg.Binding.Limits.MaxSeconds*1000 {
		return nil, ErrSavedAuthority
	}
	c := &WitnessController{path: path, binding: cfg.Binding, bindingHash: hash, boot: cfg.ServiceGeneration, authority: cfg.AuthoritySHA256, key: cfg.RevisionKey, currentTask: cfg.CurrentTask, observed: now.UnixMilli(), openedWall: now.UnixMilli(), openedMono: now, now: time.Now}
	raw, err := readPrivateWitness(path)
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return nil, ErrSavedAuthority
	}
	w, err := c.parse(raw)
	if err != nil {
		return nil, err
	}
	c.last = w
	c.lastBytes = raw
	c.proofDeadline = now.Add(time.Duration(w.ValidUntilUnixMS-c.observed) * time.Millisecond)
	c.terminal = w.State == "retired" || c.observed < w.IssuedAtUnixMS || c.observed >= w.ValidUntilUnixMS
	return c, nil
}
func (c *WitnessController) current() (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	return c.currentTask != nil && c.currentTask(c.binding)
}
func (c *WitnessController) clock() (int64, error) {
	now := c.now().UnixMilli()
	if now < c.observed || now >= c.binding.ExpiresAtUnixMS || time.Since(c.openedMono).Milliseconds() >= c.binding.ExpiresAtUnixMS-c.openedWall {
		return now, ErrSavedAuthority
	}
	c.observed = now
	return now, nil
}
func (c *WitnessController) reconcile() error {
	if c.last.State == "active" && !time.Now().Before(c.proofDeadline) {
		return ErrSavedAuthority
	}
	raw, err := readPrivateWitness(c.path)
	if os.IsNotExist(err) && len(c.lastBytes) == 0 {
		return nil
	}
	if err != nil {
		return ErrSavedAuthority
	}
	w, err := c.parse(raw)
	if err != nil {
		return err
	}
	if len(c.lastBytes) > 0 && (w.Revision < c.last.Revision || w.Revision == c.last.Revision && !bytes.Equal(raw, c.lastBytes)) {
		return ErrSavedAuthority
	}
	if w.Revision != c.last.Revision {
		c.proofDeadline = time.Now().Add(time.Duration(w.ValidUntilUnixMS-c.observed) * time.Millisecond)
	}
	c.last = w
	c.lastBytes = raw
	if w.State == "retired" || c.observed < w.IssuedAtUnixMS || c.observed >= w.ValidUntilUnixMS {
		return ErrSavedAuthority
	}
	return nil
}
func (c *WitnessController) write(state string, now int64) (WitnessRecord, error) {
	revision := c.last.Revision + 1
	if !savedPositive(revision, savedSafeInt) {
		return WitnessRecord{}, ErrSavedAuthority
	}
	if now < c.last.IssuedAtUnixMS {
		now = c.last.IssuedAtUnixMS
	}
	until := now
	if state == "active" {
		until = now + 5000
	}
	if until > c.binding.ExpiresAtUnixMS {
		until = c.binding.ExpiresAtUnixMS
	}
	body := WitnessBody{1, "saved_account_authority_v1", c.bindingHash, c.boot, c.authority, revision, state, now, until}
	raw, err := signedWitness(body, c.key)
	if err != nil {
		return WitnessRecord{}, err
	}
	if err = replacePrivateWitness(c.path, raw, c.lastBytes); err != nil {
		return WitnessRecord{}, err
	}
	w, err := c.parse(raw)
	if err != nil {
		return WitnessRecord{}, err
	}
	c.last = w
	c.lastBytes = raw
	return w, nil
}
func (c *WitnessController) retireLocked() error {
	c.terminal = true
	_, err := c.write("retired", c.now().UnixMilli())
	return err
}
func (c *WitnessController) Retire() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.last.State == "retired" && c.terminal {
		return nil
	}
	return c.retireLocked()
}
func (c *WitnessController) Issue() (WitnessRecord, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.terminal {
		return WitnessRecord{}, ErrSavedAuthority
	}
	now, err := c.clock()
	if err != nil || c.reconcile() != nil {
		c.retireLocked()
		return WitnessRecord{}, ErrSavedAuthority
	}
	if !c.current() {
		c.retireLocked()
		return WitnessRecord{}, ErrSavedAuthority
	}
	if !c.current() {
		c.retireLocked()
		return WitnessRecord{}, ErrSavedAuthority
	}
	if now, err = c.clock(); err != nil || c.reconcile() != nil {
		c.retireLocked()
		return WitnessRecord{}, ErrSavedAuthority
	}
	issuedMono := time.Now()
	w, err := c.write("active", now)
	if err != nil {
		c.terminal = true
		return WitnessRecord{}, ErrSavedAuthority
	}
	c.proofDeadline = issuedMono.Add(time.Duration(w.ValidUntilUnixMS-w.IssuedAtUnixMS) * time.Millisecond)
	if !c.current() {
		c.retireLocked()
		return WitnessRecord{}, ErrSavedAuthority
	}
	if now, err = c.clock(); err != nil || now >= w.ValidUntilUnixMS || !time.Now().Before(c.proofDeadline) {
		c.retireLocked()
		return WitnessRecord{}, ErrSavedAuthority
	}
	return w, nil
}
func replacePrivateWitness(path string, raw, prior []byte) error {
	if len(raw) > 4096 {
		return ErrSavedAuthority
	}
	if _, err := privateWitnessPath(path); err != nil {
		return err
	}
	current, err := readPrivateWitness(path)
	if os.IsNotExist(err) {
		if len(prior) != 0 {
			return ErrSavedAuthority
		}
	} else if err != nil || !bytes.Equal(current, prior) {
		return ErrSavedAuthority
	}
	suffix := make([]byte, 16)
	if _, err = rand.Read(suffix); err != nil {
		return ErrSavedAuthority
	}
	temporary := path + ".tmp-" + hex.EncodeToString(suffix)
	f, err := os.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return ErrSavedAuthority
	}
	defer os.Remove(temporary)
	if _, err = f.Write(raw); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		return ErrSavedAuthority
	}
	if _, err = privateWitnessPath(path); err != nil {
		return err
	}
	current, err = readPrivateWitness(path)
	if os.IsNotExist(err) {
		if len(prior) != 0 {
			return ErrSavedAuthority
		}
	} else if err != nil || !bytes.Equal(current, prior) {
		return ErrSavedAuthority
	}
	if err = os.Rename(temporary, path); err != nil {
		return ErrSavedAuthority
	}
	written, err := readPrivateWitness(path)
	if err != nil || !bytes.Equal(written, raw) {
		return ErrSavedAuthority
	}
	return nil
}
func privateWitnessPath(path string) (string, error) {
	if !filepath.IsAbs(path) || !savedText(path, 4096) || filepath.Clean(path) != path {
		return "", ErrSavedAuthority
	}
	for cursor := filepath.Dir(path); ; cursor = filepath.Dir(cursor) {
		st, err := os.Lstat(cursor)
		if err != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 || !privateDirectoryIdentity(cursor) {
			return "", ErrSavedAuthority
		}
		if filepath.Dir(cursor) == cursor {
			break
		}
	}
	return path, nil
}
