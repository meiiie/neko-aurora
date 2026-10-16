package identity

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func testAuthority(t *testing.T, now *time.Time) *Authority {
	t.Helper()
	a, e := NewFixture(Config{Version: 1, FixtureOnly: true, AuthorityID: "fixture-test", SpaceID: "local-space", SessionTTLSeconds: 30, Actors: []ActorDevice{{"local-owner", "owner-device", 1}, {"fixture-bob", "bob-device", 1}}, Now: func() time.Time { return *now }})
	if e != nil {
		t.Fatal(e)
	}
	return a
}
func testSession(t *testing.T, a *Authority, actor, device string) (string, VerifiedPrincipal) {
	t.Helper()
	ticket, e := a.BootstrapTicket(actor, device)
	if e != nil {
		t.Fatal(e)
	}
	token, p, e := a.Exchange(ticket)
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e = a.Exchange(ticket); e == nil {
		t.Fatal("bootstrap ticket reused")
	}
	return token, p
}
func TestFixtureIdentityRotationSpoofExpiryAndBackwardClock(t *testing.T) {
	now := time.Unix(1800000000, 0)
	a := testAuthority(t, &now)
	token, p := testSession(t, a, "fixture-bob", "bob-device")
	if _, e := a.Resolve(strings.Repeat("0", 64)); e == nil {
		t.Fatal("guessed session accepted")
	}
	for _, field := range []string{"actor", "device", "epoch", "expiry"} {
		q := p
		switch field {
		case "actor":
			q.ActorID = "local-owner"
		case "device":
			q.DeviceID = "owner-device"
		case "epoch":
			q.IssuerEpoch++
		case "expiry":
			q.ExpiresAtUnixMs++
		}
		if a.Valid(q) {
			t.Fatalf("spoof accepted: %s", field)
		}
	}
	now = now.Add(-time.Millisecond)
	if a.Valid(p) {
		t.Fatal("future session accepted after clock reversal")
	}
	now = now.Add(time.Millisecond)
	if !a.Valid(p) {
		t.Fatal("original session invalid")
	}
	staleTicket, e := a.BootstrapTicket("fixture-bob", "bob-device")
	if e != nil {
		t.Fatal(e)
	}
	out, e := a.ControlDevice("bob-device", "rotate", 1)
	if e != nil {
		t.Fatal(e)
	}
	if a.Valid(p) {
		t.Fatal("rotated old pin valid")
	}
	if _, e = a.Resolve(token); e == nil {
		t.Fatal("rotated bearer valid")
	}
	if _, _, e = a.Exchange(staleTicket); e == nil {
		t.Fatal("rotated bootstrap valid")
	}
	_, next, e := a.Exchange(out.Ticket)
	if e != nil {
		t.Fatal(e)
	}
	if ActorNamespace(p) != ActorNamespace(next) || Binding(p) == Binding(next) {
		t.Fatal("rotation confused intent namespace and session binding")
	}
	now = now.Add(30 * time.Second)
	if a.Valid(next) {
		t.Fatal("expired session valid")
	}
}
func TestFixtureStatusNoBearerAndOwnerControl(t *testing.T) {
	now := time.Unix(1800000000, 0)
	a := testAuthority(t, &now)
	token, bob := testSession(t, a, "fixture-bob", "bob-device")
	raw, _ := json.Marshal(Status(bob))
	if strings.Contains(string(raw), token) || strings.Contains(string(raw), bob.SessionID) {
		t.Fatal("status disclosed bearer/session identifier")
	}
	if _, e := a.ControlDeviceFor(bob, "owner-device", "revoke", 1); e == nil {
		t.Fatal("member controlled owner")
	}
	_, owner := testSession(t, a, "local-owner", "owner-device")
	if _, e := a.ControlDeviceFor(owner, "bob-device", "revoke", 1); e != nil {
		t.Fatal(e)
	}
	if a.Valid(bob) {
		t.Fatal("revoked device valid")
	}
	if _, e := a.ControlDeviceFor(owner, "bob-device", "rotate", 1); e == nil {
		t.Fatal("stale revision accepted")
	}
}
