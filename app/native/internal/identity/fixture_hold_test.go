package identity

import (
	"errors"
	"testing"
)

func TestHoldAllRequiresEveryPrincipalAndReleasesAuthority(t *testing.T) {
	a, err := NewFixture(Config{Version: 1, FixtureOnly: true, AuthorityID: "hold-many-fixture", SpaceID: "local-space", SessionTTLSeconds: 30, Actors: []ActorDevice{{ActorID: "local-owner", DeviceID: "owner-device", DeviceRevision: 1}, {ActorID: "fixture-bob", DeviceID: "bob-device", DeviceRevision: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	pins := []VerifiedPrincipal{}
	for _, v := range []struct{ actor, device string }{{"local-owner", "owner-device"}, {"fixture-bob", "bob-device"}} {
		ticket, e := a.BootstrapTicket(v.actor, v.device)
		if e != nil {
			t.Fatal(e)
		}
		_, p, e := a.Exchange(ticket)
		if e != nil {
			t.Fatal(e)
		}
		pins = append(pins, p)
	}
	release, err := a.HoldAll(pins)
	if err != nil {
		t.Fatal(err)
	}
	release()
	if err = a.RevokeSession(pins[1].SessionID); err != nil {
		t.Fatal("release did not allow mutation", err)
	}
	if release, err = a.HoldAll(pins); !errors.Is(err, ErrUnauthenticated) {
		if release != nil {
			release()
		}
		t.Fatal("revoked member admitted", err)
	}
	if !a.Valid(pins[0]) {
		t.Fatal("failure retained lock or revoked unrelated actor")
	}
	if release, err = a.HoldAll(nil); !errors.Is(err, ErrUnauthenticated) {
		if release != nil {
			release()
		}
		t.Fatal("empty authority proof admitted", err)
	}
}
