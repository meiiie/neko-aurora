package connections

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
)

type fixtureVault struct {
	mu      sync.Mutex
	secrets map[string][32]byte
	writes  int
	failed  bool
}

func (v *fixtureVault) Seal(id, ref string, b []byte) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.failed {
		return ErrUnavailable
	}
	v.secrets[id+":"+ref] = sha256.Sum256(b)
	v.writes++
	return nil
}
func (v *fixtureVault) Match(id, ref string, b []byte) (bool, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.secrets[id+":"+ref] == sha256.Sum256(b), nil
}
func fixtureInput() AccountInput {
	return AccountInput{ClientNonce: "fixture-account-1", Name: "Fixture One", Provider: "openai-compatible", Endpoint: "https://example.invalid/v1", Model: "fixture-model", Secret: "fixture-only-not-a-key"}
}
func TestAccountNonceSecretConflictAndPublicProjection(t *testing.T) {
	v := &fixtureVault{secrets: map[string][32]byte{}}
	s, err := Open(t.TempDir(), v)
	if err != nil {
		t.Fatal(err)
	}
	in := fixtureInput()
	a, duplicate, err := s.SaveAccount(in)
	if err != nil || duplicate || !a.HasSecret || a.Status != "stored_unverified" {
		t.Fatal(err)
	}
	again, duplicate, err := s.SaveAccount(in)
	if err != nil || !duplicate || a.ID != again.ID || v.writes != 1 {
		t.Fatal("duplicate created another secret/account")
	}
	in.Secret = "fixture-only-changed"
	if _, _, err = s.SaveAccount(in); !errors.Is(err, ErrConflict) {
		t.Fatal("nonce accepted changed secret")
	}
	b, _ := json.Marshal(s.Snapshot())
	if strings.Contains(string(b), in.Secret) || strings.Contains(string(b), "secretRef") || strings.Contains(string(b), "fingerprint") {
		t.Fatal("private metadata exposed")
	}
}
func TestAccountRevisionRaceAndRestart(t *testing.T) {
	dir := t.TempDir()
	v := &fixtureVault{secrets: map[string][32]byte{}}
	s, _ := Open(dir, v)
	in := fixtureInput()
	a, _, err := s.SaveAccount(in)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	success := 0
	var mu sync.Mutex
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			update := in
			update.ID = a.ID
			update.ExpectedRevision = a.Revision
			update.ClientNonce = string(rune('a'+i)) + "-fixture-update"
			update.Name = update.ClientNonce
			update.Secret = ""
			if _, _, err := s.SaveAccount(update); err == nil {
				mu.Lock()
				success++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	if success != 1 {
		t.Fatal("revision race admitted multiple writes")
	}
	restored, err := Open(dir, v)
	if err != nil || restored.Snapshot().Accounts[0].Revision != 2 || !restored.Snapshot().Accounts[0].HasSecret {
		t.Fatal("restart lost identity/revision")
	}
	if err = restored.RemoveAccount(a.ID, 1); !errors.Is(err, ErrConflict) {
		t.Fatal("stale delete accepted")
	}
}
func TestVaultFailureCannotReportSealedAccount(t *testing.T) {
	s, _ := Open(t.TempDir(), &fixtureVault{secrets: map[string][32]byte{}, failed: true})
	if _, _, err := s.SaveAccount(fixtureInput()); !errors.Is(err, ErrUnavailable) {
		t.Fatal("vault failure hidden")
	}
	if len(s.Snapshot().Accounts) != 0 {
		t.Fatal("unsealed account published")
	}
}
func TestPluginManifestCannotGrantExecutionOrRemoteSchema(t *testing.T) {
	s, _ := Open(t.TempDir(), nil)
	in := PluginInput{ID: "fixture-plugin", Name: "Fixture Plugin", Version: "1.0.0", License: "MIT", Tools: []Tool{{Name: "report", Description: "Read a bounded report", InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"],"additionalProperties":false}`)}}}
	p, err := s.AddPlugin(in)
	if err != nil || p.Status != "manifest_only_disabled" {
		t.Fatal("manifest granted execution", err)
	}
	in.ID = "remote-plugin"
	in.Tools[0].InputSchema = json.RawMessage(`{"type":"object","$ref":"https://example.invalid/schema"}`)
	if _, err = s.AddPlugin(in); !errors.Is(err, ErrInvalid) {
		t.Fatal("remote/unbounded schema accepted")
	}
	if s.Snapshot().Plugins[0].ID != "neko-artifacts" {
		t.Fatal("builtin contract replaced")
	}
}
