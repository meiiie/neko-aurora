package connections

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"nekobot.local/native/internal/protocol"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"
)

var ErrInvalid = errors.New("invalid_connection")
var ErrConflict = errors.New("connection_revision_conflict")
var ErrUnavailable = errors.New("connection_unavailable")

type Account struct {
	ID                   string `json:"id"`
	Name                 string `json:"name"`
	Provider             string `json:"provider"`
	Endpoint             string `json:"endpoint"`
	Model                string `json:"model"`
	Revision             uint64 `json:"revision"`
	CredentialGeneration uint64 `json:"credentialGeneration"`
	HasSecret            bool   `json:"hasSecret"`
	Status               string `json:"status"`
}
type storedAccount struct {
	Account
	SecretRef   string `json:"secretRef,omitempty"`
	Nonce       string `json:"nonce"`
	Fingerprint string `json:"fingerprint"`
}
type AccountInput struct {
	ID               string `json:"id"`
	ExpectedRevision uint64 `json:"expectedRevision"`
	ClientNonce      string `json:"clientNonce"`
	Name             string `json:"name"`
	Provider         string `json:"provider"`
	Endpoint         string `json:"endpoint"`
	Model            string `json:"model"`
	Secret           string `json:"secret"`
}
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}
type Plugin struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Version  string `json:"version"`
	License  string `json:"license"`
	Tools    []Tool `json:"tools"`
	Status   string `json:"status"`
	Revision uint64 `json:"revision"`
}
type PluginInput struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
	License string `json:"license"`
	Tools   []Tool `json:"tools"`
}
type Snapshot struct {
	Revision          uint64    `json:"revision"`
	Accounts          []Account `json:"accounts"`
	Plugins           []Plugin  `json:"plugins"`
	ProviderExecution string    `json:"providerExecution"`
	Capabilities      []string  `json:"capabilities"`
}
type disk struct {
	Version    int             `json:"version"`
	Revision   uint64          `json:"revision"`
	Accounts   []storedAccount `json:"accounts"`
	Plugins    []Plugin        `json:"plugins"`
	UsedNonces []string        `json:"usedNonces,omitempty"`
}
type Sealer interface {
	Seal(accountID, reference string, secret []byte) error
	Match(accountID, reference string, secret []byte) (bool, error)
}
type Store struct {
	mu     sync.Mutex
	dir    string
	data   disk
	sealer Sealer
}

var identifier = regexp.MustCompile(`^[a-z][a-z0-9_-]{2,63}$`)
var version = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[a-zA-Z0-9.-]+)?$`)
var accountSecretPattern = regexp.MustCompile(`-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----|\b(?:sk-(?:proj-)?[A-Za-z0-9_-]{24,}|gh[pousr]_[A-Za-z0-9]{30,}|AKIA[A-Z0-9]{16}|xox[baprs]-[A-Za-z0-9-]{20,})\b|(?i:(?:api[_-]?key|access[_-]?token|password|client[_-]?secret)\s*[:=]\s*["'][^"'\r\n]{12,}["'])`)

func Open(dir string, sealer Sealer) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, sealer: sealer, data: disk{Version: 1, Revision: 1, Accounts: []storedAccount{}, Plugins: []Plugin{}}}
	b, err := os.ReadFile(filepath.Join(dir, "connections.json"))
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	canonical, err := protocol.Canonical(b)
	if err != nil {
		return nil, ErrUnavailable
	}
	d := json.NewDecoder(strings.NewReader(string(canonical)))
	d.DisallowUnknownFields()
	if d.Decode(&s.data) != nil || s.data.Version != 1 || s.data.Revision == 0 || len(s.data.Accounts) > 32 || len(s.data.Plugins) > 16 {
		return nil, ErrUnavailable
	}
	if len(s.data.UsedNonces) > 512 {
		return nil, ErrUnavailable
	}
	for i := range s.data.Accounts {
		a := &s.data.Accounts[i]
		if a.HasSecret && a.CredentialGeneration == 0 {
			a.CredentialGeneration = 1
		}
		if !containsNonce(s.data.UsedNonces, a.Nonce) {
			s.data.UsedNonces = append(s.data.UsedNonces, a.Nonce)
		}
	}
	if len(s.data.UsedNonces) > 512 {
		return nil, ErrUnavailable
	}
	return s, nil
}
func containsNonce(nonces []string, nonce string) bool {
	for _, value := range nonces {
		if value == nonce {
			return true
		}
	}
	return false
}
func randomID(prefix string) (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(b[:]), nil
}
func (s *Store) save(next disk) error {
	next.Revision++
	b, err := json.Marshal(next)
	if err != nil || len(b) > 65536 {
		return ErrUnavailable
	}
	temporary, err := randomID("connections-")
	if err != nil {
		return ErrUnavailable
	}
	path := filepath.Join(s.dir, temporary+".tmp")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return ErrUnavailable
	}
	defer os.Remove(path)
	n, err := file.Write(b)
	if err == nil && n != len(b) {
		err = io.ErrShortWrite
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		return ErrUnavailable
	}
	if os.Rename(path, filepath.Join(s.dir, "connections.json")) != nil {
		return ErrUnavailable
	}
	s.data = next
	return nil
}
func (s *Store) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := Snapshot{Revision: s.data.Revision, Accounts: []Account{}, Plugins: []Plugin{builtinPlugin()}, ProviderExecution: "deferred", Capabilities: []string{"account_save_receipts_v1"}}
	for _, a := range s.data.Accounts {
		out.Accounts = append(out.Accounts, a.Account)
	}
	out.Plugins = append(out.Plugins, s.data.Plugins...)
	return out
}
func text(value string, limit int) bool {
	return utf8.ValidString(value) && strings.TrimSpace(value) != "" && len(value) <= limit && !strings.ContainsAny(value, "\x00\r\n")
}
func (s *Store) SaveAccount(in AccountInput) (Account, bool, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Model = strings.TrimSpace(in.Model)
	if sensitiveAccountMetadata(Account{Name: in.Name, Model: in.Model, Endpoint: in.Endpoint}) || accountSecretPattern.MatchString(in.ClientNonce) {
		return Account{}, false, ErrInvalid
	}
	endpoint, err := url.Parse(in.Endpoint)
	if err != nil || endpoint.Scheme != "https" || endpoint.Host == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" || len(in.Endpoint) > 512 || !text(in.Name, 80) || !text(in.Model, 120) || (in.Provider != "openai-compatible" && in.Provider != "anthropic") || len(in.Secret) > 4096 || !utf8.ValidString(in.Secret) || !text(in.ClientNonce, 128) {
		return Account{}, false, ErrInvalid
	}
	canonical, _ := accountInputMetadata(in)
	hash := sha256.Sum256(canonical)
	fingerprint := hex.EncodeToString(hash[:])
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range s.data.Accounts {
		if a.Nonce == in.ClientNonce {
			if a.Fingerprint != fingerprint {
				return Account{}, false, ErrConflict
			}
			if in.Secret != "" {
				if s.sealer == nil {
					return Account{}, false, ErrUnavailable
				}
				secret := []byte(in.Secret)
				matches, e := s.sealer.Match(a.ID, a.SecretRef, secret)
				for i := range secret {
					secret[i] = 0
				}
				if e != nil {
					return Account{}, false, ErrUnavailable
				}
				if !matches {
					return Account{}, false, ErrConflict
				}
			}
			return a.Account, true, nil
		}
	}
	if containsNonce(s.data.UsedNonces, in.ClientNonce) {
		return Account{}, false, ErrConflict
	}
	if len(s.data.UsedNonces) >= 512 {
		return Account{}, false, ErrUnavailable
	}
	next := s.data
	next.UsedNonces = append(append([]string{}, next.UsedNonces...), in.ClientNonce)
	next.Accounts = append([]storedAccount{}, next.Accounts...)
	index := -1
	for i, a := range next.Accounts {
		if a.ID == in.ID {
			index = i
		}
		if strings.EqualFold(a.Name, in.Name) && a.Provider == in.Provider && a.ID != in.ID {
			return Account{}, false, ErrConflict
		}
	}
	var a storedAccount
	if in.ID == "" {
		if in.ExpectedRevision != 0 || len(next.Accounts) >= 32 {
			return Account{}, false, ErrConflict
		}
		id, err := randomID("account-")
		if err != nil {
			return Account{}, false, ErrUnavailable
		}
		a.Account = Account{ID: id, Revision: 1}
	} else {
		if index < 0 || next.Accounts[index].Revision != in.ExpectedRevision {
			return Account{}, false, ErrConflict
		}
		a = next.Accounts[index]
		if a.Revision == ^uint64(0) {
			return Account{}, false, ErrUnavailable
		}
		if a.Provider != in.Provider || a.Endpoint != in.Endpoint {
			return Account{}, false, ErrConflict
		}
		a.Revision++
	}
	a.Name = in.Name
	a.Provider = in.Provider
	a.Endpoint = in.Endpoint
	a.Model = in.Model
	a.Status = "stored_unverified"
	a.Nonce = in.ClientNonce
	a.Fingerprint = fingerprint
	if in.Secret != "" {
		if s.sealer == nil {
			return Account{}, false, ErrUnavailable
		}
		reference, err := randomID("secret-")
		if err != nil {
			return Account{}, false, ErrUnavailable
		}
		secret := []byte(in.Secret)
		err = s.sealer.Seal(a.ID, reference, secret)
		for i := range secret {
			secret[i] = 0
		}
		in.Secret = ""
		if err != nil {
			return Account{}, false, ErrUnavailable
		}
		a.HasSecret = true
		a.SecretRef = reference
		if a.CredentialGeneration == ^uint64(0) {
			return Account{}, false, ErrUnavailable
		}
		a.CredentialGeneration++
	}
	if index < 0 {
		next.Accounts = append(next.Accounts, a)
	} else {
		next.Accounts[index] = a
	}
	if err = s.save(next); err != nil {
		return Account{}, false, err
	}
	return a.Account, false, nil
}
func (s *Store) RemoveAccount(id string, revision uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.data
	next.Accounts = append([]storedAccount{}, next.Accounts...)
	for i, a := range next.Accounts {
		if a.ID == id {
			if a.Revision != revision {
				return ErrConflict
			}
			next.Accounts = append(next.Accounts[:i], next.Accounts[i+1:]...)
			return s.save(next)
		}
	}
	return ErrConflict
}
func validSchema(raw json.RawMessage) bool {
	canonical, err := protocol.Canonical(raw)
	if err != nil || len(canonical) > 4096 {
		return false
	}
	var schema map[string]any
	if json.Unmarshal(canonical, &schema) != nil || schema["type"] != "object" || schema["additionalProperties"] != false {
		return false
	}
	for key := range schema {
		if key != "type" && key != "properties" && key != "required" && key != "additionalProperties" {
			return false
		}
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok || len(properties) > 16 {
		return false
	}
	for name, value := range properties {
		if !identifier.MatchString(name) {
			return false
		}
		p, ok := value.(map[string]any)
		if !ok || len(p) > 3 {
			return false
		}
		for key := range p {
			if key != "type" && key != "description" && key != "maxLength" {
				return false
			}
		}
		if p["type"] != "string" && p["type"] != "boolean" && p["type"] != "integer" {
			return false
		}
	}
	required, ok := schema["required"].([]any)
	if !ok {
		return false
	}
	seen := map[string]bool{}
	for _, value := range required {
		name, ok := value.(string)
		if !ok || properties[name] == nil || seen[name] {
			return false
		}
		seen[name] = true
	}
	return true
}
func (s *Store) AddPlugin(in PluginInput) (Plugin, error) {
	if !identifier.MatchString(in.ID) || in.ID == "neko-artifacts" || !text(in.Name, 80) || !version.MatchString(in.Version) || len(in.Version) > 40 || !text(in.License, 80) || len(in.Tools) == 0 || len(in.Tools) > 8 {
		return Plugin{}, ErrInvalid
	}
	seen := map[string]bool{}
	for _, tool := range in.Tools {
		if !identifier.MatchString(tool.Name) || seen[tool.Name] || !text(tool.Description, 400) || !validSchema(tool.InputSchema) {
			return Plugin{}, ErrInvalid
		}
		seen[tool.Name] = true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.data.Plugins {
		if p.ID == in.ID {
			return Plugin{}, ErrConflict
		}
	}
	if len(s.data.Plugins) >= 16 {
		return Plugin{}, ErrConflict
	}
	p := Plugin{ID: in.ID, Name: in.Name, Version: in.Version, License: in.License, Tools: in.Tools, Status: "manifest_only_disabled", Revision: 1}
	next := s.data
	next.Plugins = append(append([]Plugin{}, next.Plugins...), p)
	if err := s.save(next); err != nil {
		return Plugin{}, err
	}
	return p, nil
}
func builtinPlugin() Plugin {
	return Plugin{ID: "neko-artifacts", Name: "Tệp của Neko", Version: "1.0.0", License: "original-host-implementation", Tools: []Tool{{Name: "artifact.write_text", Description: "Ghi báo cáo UTF-8 vào tác vụ hiện tại.", InputSchema: json.RawMessage(`{"type":"object","properties":{"filename":{"type":"string"},"text":{"type":"string"}},"required":["filename","text"],"additionalProperties":false}`)}}, Status: "available_for_requested_task_artifacts", Revision: 1}
}
