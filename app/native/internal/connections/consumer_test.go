package connections

import (
	"encoding/json"
	"errors"
	"nekobot.local/native/internal/providerexec"
	"path/filepath"
	"strings"
	"testing"
)

func TestConsumerSelectionIsExactOpaqueAndRevokedByRotation(t *testing.T) {
	vault := &fixtureVault{secrets: map[string][32]byte{}}
	s, e := Open(t.TempDir(), vault)
	if e != nil {
		t.Fatal(e)
	}
	in := fixtureInput()
	a, _, e := s.SaveAccount(in)
	if e != nil {
		t.Fatal(e)
	}
	pin := pinAccount(a)
	selection, e := s.SelectConsumer(pin)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(selection)
	if strings.Contains(string(raw), "secret-") || strings.Contains(string(raw), in.Secret) {
		t.Fatal("private selection exposed")
	}
	frame, e := selection.PrivateFrame("fixture-consumer-op", 1, strings.Repeat("b", 64))
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(string(frame), `"secretRef":"secret-`) || strings.Contains(string(frame), in.Secret) {
		t.Fatal("private descriptor is not opaque")
	}
	if !s.ConsumerCurrent(selection) {
		t.Fatal("selected pin not current")
	}
	changed := in
	changed.ID = a.ID
	changed.ExpectedRevision = a.Revision
	changed.ClientNonce = "fixture-account-rotation"
	changed.Secret = "fixture-only-rotated-key"
	if _, _, e = s.SaveAccount(changed); e != nil {
		t.Fatal(e)
	}
	if s.ConsumerCurrent(selection) {
		t.Fatal("old credential selection survived rotation")
	}
	if _, e = s.SelectConsumer(pin); !errors.Is(e, ErrConflict) {
		t.Fatal("old pin rebound")
	}
}
func TestConsumerMetadataChangeRemoveAndEmptySecretDeny(t *testing.T) {
	s, e := Open(t.TempDir(), nil)
	if e != nil {
		t.Fatal(e)
	}
	in := fixtureInput()
	in.Secret = ""
	a, _, e := s.SaveAccount(in)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.SelectConsumer(Pin{a.ID, a.Revision, 1, a.Provider, a.Endpoint, a.Model}); !errors.Is(e, ErrConflict) {
		t.Fatal("empty key granted consumer")
	}
	v := &fixtureVault{secrets: map[string][32]byte{}}
	s, e = Open(t.TempDir(), v)
	if e != nil {
		t.Fatal(e)
	}
	in = fixtureInput()
	a, _, e = s.SaveAccount(in)
	if e != nil {
		t.Fatal(e)
	}
	selection, e := s.SelectConsumer(pinAccount(a))
	if e != nil {
		t.Fatal(e)
	}
	if e = s.RemoveAccount(a.ID, a.Revision); e != nil {
		t.Fatal(e)
	}
	if s.ConsumerCurrent(selection) {
		t.Fatal("removed account current")
	}
	if _, e = selection.PrivateFrame("invalid", 0, "not-hash"); !errors.Is(e, ErrInvalid) {
		t.Fatal("private frame binding unchecked")
	}
}

func TestPrivateSavedTaskFrameExactSelectionAndPrivacy(t *testing.T) {
	vault := &fixtureVault{secrets: map[string][32]byte{}}
	root := t.TempDir()
	s, e := Open(root, vault)
	if e != nil {
		t.Fatal(e)
	}
	in := fixtureInput()
	a, _, e := s.SaveAccount(in)
	if e != nil {
		t.Fatal(e)
	}
	selection, e := s.SelectConsumer(pinAccount(a))
	if e != nil {
		t.Fatal(e)
	}
	b := providerexec.SavedTaskBinding{AccountID: a.ID, AccountRevision: int64(a.Revision), CredentialGeneration: int64(a.CredentialGeneration), Provider: a.Provider, Endpoint: a.Endpoint, Model: a.Model, RequestID: "req-synthetic", AttemptID: "attempt-synthetic", Epoch: 1, LeaseFence: 1, ContextSHA256: strings.Repeat("b", 64), ConsentID: "req-synthetic", ConsentRevision: 1, ExpiresAtUnixMS: 2000000000000, Limits: providerexec.SavedTaskLimits{MaxInputBytes: 64000, MaxOutputTokens: 4096, MaxRequests: 8, MaxSteps: 6, MaxSeconds: 600}}
	capsule := filepath.Join(t.TempDir(), "capsule.json")
	home := t.TempDir()
	key := strings.Repeat("6", 64)
	frame, e := s.PrivateSavedTaskFrame(selection, b, root, capsule, home, key)
	if e != nil {
		t.Fatal(e)
	}
	defer clear(frame)
	var got map[string]json.RawMessage
	if e = json.Unmarshal(frame, &got); e != nil || len(got) != 8 {
		t.Fatal("wrong request fields")
	}
	if len(frame) > 16384 || !strings.Contains(string(frame), `"secretRef":"secret-`) || strings.Contains(string(frame), in.Secret) {
		t.Fatal("private frame exposure or bound")
	}
	public, _ := json.Marshal(selection)
	if strings.Contains(string(public), "secretRef") || strings.Contains(string(public), selection.reference) || strings.Contains(string(public), key) {
		t.Fatal("selection leaks private fields")
	}
	for _, field := range []string{"account", "revision", "generation", "provider", "endpoint", "model", "fixture"} {
		t.Run(field, func(t *testing.T) {
			bad := b
			switch field {
			case "account":
				bad.AccountID = "account-" + strings.Repeat("f", 32)
			case "revision":
				bad.AccountRevision++
			case "generation":
				bad.CredentialGeneration++
			case "provider":
				bad.Provider = "anthropic"
			case "endpoint":
				bad.Endpoint = "https://other.invalid/v1"
			case "model":
				bad.Model = "other"
			case "fixture":
				bad.FixtureOnly = true
			}
			if _, e := s.PrivateSavedTaskFrame(selection, bad, root, capsule, home, key); e == nil {
				t.Fatal("mismatched pin encoded")
			}
		})
	}
	if _, e := s.PrivateSavedTaskFrame(selection, b, t.TempDir(), capsule, home, key); e == nil {
		t.Fatal("foreign catalog encoded")
	}
	if _, e := s.PrivateSavedTaskFrame(selection, b, root, capsule, home, "bad"); e == nil {
		t.Fatal("invalid key encoded")
	}
	if _, e := s.PrivateSavedTaskFrame(selection, b, root, filepath.Join(home, strings.Repeat("\"", 4000)), filepath.Join(home, strings.Repeat("\"", 4000)), key); e == nil {
		t.Fatal("oversized escaped request encoded")
	}
	if e = s.RemoveAccount(a.ID, a.Revision); e != nil {
		t.Fatal(e)
	}
	if _, e := s.PrivateSavedTaskFrame(selection, b, root, capsule, home, key); !errors.Is(e, ErrConflict) {
		t.Fatal("stale selection encoded")
	}
}
