package connections

import (
	"encoding/json"
	"errors"
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
