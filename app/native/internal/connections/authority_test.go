package connections

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestRemovedAccountNonceCannotCreateReplacementAfterRestart(t *testing.T) {
	dir := t.TempDir()
	vault := &fixtureVault{secrets: map[string][32]byte{}}
	s, err := Open(dir, vault)
	if err != nil {
		t.Fatal(err)
	}
	in := fixtureInput()
	a, _, err := s.SaveAccount(in)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RemoveAccount(a.ID, a.Revision); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir, vault)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.SaveAccount(in); !errors.Is(err, ErrConflict) {
		t.Fatal("removed create nonce resurrected an account", err)
	}
	if len(s.Snapshot().Accounts) != 0 {
		t.Fatal("replacement published")
	}
}

func TestCredentialGenerationSurvivesMetadataEditAndChangesOnRotation(t *testing.T) {
	s, err := Open(t.TempDir(), &fixtureVault{secrets: map[string][32]byte{}})
	if err != nil {
		t.Fatal(err)
	}
	in := fixtureInput()
	a, _, err := s.SaveAccount(in)
	if err != nil {
		t.Fatal(err)
	}
	generation := func(a Account) uint64 {
		b, _ := json.Marshal(a)
		var fields map[string]any
		json.Unmarshal(b, &fields)
		n, _ := fields["credentialGeneration"].(float64)
		return uint64(n)
	}
	if generation(a) != 1 {
		t.Fatal("credential generation missing")
	}
	in.ID, in.ExpectedRevision, in.ClientNonce, in.Secret = a.ID, a.Revision, "metadata-update-fixture", ""
	a, _, err = s.SaveAccount(in)
	if err != nil || generation(a) != 1 {
		t.Fatal("metadata edit rotated credential", err)
	}
	in.ExpectedRevision, in.ClientNonce, in.Secret = a.Revision, "credential-rotation-fixture", "fixture-only-next-key"
	a, _, err = s.SaveAccount(in)
	if err != nil || generation(a) != 2 {
		t.Fatal("rotation did not advance generation", err)
	}
}
