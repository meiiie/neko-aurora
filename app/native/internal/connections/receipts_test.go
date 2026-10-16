package connections

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestAccountSaveReceiptRotatePreserveSecretAndReopen(t *testing.T) {
	vault := &fixtureVault{secrets: map[string][32]byte{}}
	dir := t.TempDir()
	s, err := Open(dir, vault)
	if err != nil {
		t.Fatal(err)
	}
	in := fixtureInput()
	a, _, err := s.SaveAccount(in)
	if err != nil {
		t.Fatal(err)
	}
	in.ID, in.ExpectedRevision, in.ClientNonce, in.Model, in.Secret = a.ID, a.Revision, "fixture-model-change", "other-model", ""
	b, _, err := s.SaveAccount(in)
	if err != nil || b.CredentialGeneration != a.CredentialGeneration || !b.HasSecret || vault.writes != 1 {
		t.Fatal("blank edit changed secret", err)
	}
	in.ExpectedRevision, in.ClientNonce, in.Secret = b.Revision, "fixture-secret-rotate", "fixture-only-rotated-value"
	c, _, err := s.SaveAccount(in)
	if err != nil || c.Revision != b.Revision+1 || c.CredentialGeneration != b.CredentialGeneration+1 || vault.writes != 2 {
		t.Fatal("rotation generation", err)
	}
	for round := 0; round < 3; round++ {
		s, err = Open(dir, vault)
		if err != nil {
			t.Fatal(err)
		}
		r, e := s.AccountReceipt(in.ClientNonce)
		if e != nil || !r.Found || !r.Used || r.Account.ID != c.ID || r.Account.CredentialGeneration != c.CredentialGeneration {
			t.Fatal("durable account receipt", e)
		}
		out, _ := json.Marshal(r)
		if strings.Contains(string(out), in.Secret) || strings.Contains(string(out), "secretRef") || strings.Contains(string(out), "hasSubmittedSecret") {
			t.Fatal("receipt contains private input")
		}
		old, e := s.AccountReceipt("fixture-account-1")
		if e != nil || old.Found || !old.Used || old.Account != nil || old.InputFingerprint != "" {
			t.Fatal("old nonce rebound", e)
		}
		*r.Account = Account{}
		fresh, _ := s.AccountReceipt(in.ClientNonce)
		if fresh.Account.ID != c.ID {
			t.Fatal("receipt mutated catalog")
		}
	}
	original := in
	in.Secret = "fixture-only-wrong-value"
	if _, _, err = s.SaveAccount(in); !errors.Is(err, ErrConflict) {
		t.Fatal("same nonce changed secret", err)
	}
	if _, dup, err := s.SaveAccount(original); err != nil || !dup || vault.writes != 2 {
		t.Fatal("exact retry duplicated vault", err)
	}
}

func TestAccountSaveRejectsRecognizedSensitivePublicFields(t *testing.T) {
	s, err := Open(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"name", "model", "endpoint", "nonce"} {
		in := fixtureInput()
		in.Secret = ""
		value := strings.Join([]string{"sk", strings.Repeat("A", 40)}, "-")
		switch field {
		case "name":
			in.Name = value
		case "model":
			in.Model = value
		case "endpoint":
			in.Endpoint = "https://" + value + ".example.invalid/v1"
		case "nonce":
			in.ClientNonce = value
		}
		before := s.Snapshot().Revision
		if _, _, err = s.SaveAccount(in); !errors.Is(err, ErrInvalid) || s.Snapshot().Revision != before {
			t.Fatal("sensitive metadata persisted", field, err)
		}
	}
}
