package ledger

import (
	"encoding/json"
	"errors"
	"nekobot.local/native/internal/identity"
	"nekobot.local/native/internal/syncstream"
	"reflect"
	"strings"
	"testing"
	"time"
)

func fixtureLedger(t *testing.T) (*Store, *identity.Authority, map[string]identity.VerifiedPrincipal, *time.Time) {
	t.Helper()
	now := time.Unix(1800000000, 0)
	s, e := Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	a, e := identity.NewFixture(identity.Config{Version: 1, FixtureOnly: true, AuthorityID: "fixture-ledger", SpaceID: "local-space", SessionTTLSeconds: 30, Actors: []identity.ActorDevice{{ActorID: "local-owner", DeviceID: "owner-device", DeviceRevision: 1}, {ActorID: "fixture-bob", DeviceID: "bob-device", DeviceRevision: 1}, {ActorID: "fixture-bob", DeviceID: "bob-device-two", DeviceRevision: 1}, {ActorID: "fixture-reader", DeviceID: "reader-device", DeviceRevision: 1}, {ActorID: "fixture-eve", DeviceID: "eve-device", DeviceRevision: 1}}, Now: func() time.Time { return now }})
	if e != nil {
		t.Fatal(e)
	}
	pins := map[string]identity.VerifiedPrincipal{}
	for _, v := range []struct{ actor, device, key string }{{"local-owner", "owner-device", "owner"}, {"fixture-bob", "bob-device", "bob"}, {"fixture-bob", "bob-device-two", "bob2"}, {"fixture-reader", "reader-device", "reader"}, {"fixture-eve", "eve-device", "eve"}} {
		ticket, e := a.BootstrapTicket(v.actor, v.device)
		if e != nil {
			t.Fatal(e)
		}
		_, p, e := a.Exchange(ticket)
		if e != nil {
			t.Fatal(e)
		}
		pins[v.key] = p
	}
	s.ConfigureIdentity(a)
	return s, a, pins, &now
}
func fixtureChannel(t *testing.T, s *Store, p identity.VerifiedPrincipal) Channel {
	t.Helper()
	c, _, e := s.SaveChannelFor(p, ChannelInput{Name: "Fixture shared", BotIDs: []string{"bot-meiiie", "bot-davin"}, ClientNonce: "fixture-channel-create", HumanACL: &HumanACL{Version: 1, AuthorityID: p.AuthorityID, Members: []HumanMember{{ActorID: "fixture-reader", Role: "reader"}, {ActorID: "fixture-bob", Role: "writer"}}}})
	if e != nil {
		t.Fatal(e)
	}
	return c
}
func TestFixtureRequesterNonceACLManifestAndReplay(t *testing.T) {
	s, a, p, now := fixtureLedger(t)
	c := fixtureChannel(t, s, p["owner"])
	in := Input{BotID: "bot-meiiie", ClientNonce: "fixture-same-actor-nonce", Text: "Shared original human request", Lane: "foreground"}
	scope := SubmissionScope{ChannelID: c.ID, ExpectedChannelRevision: &c.Revision}
	w, dup, e := s.AcceptFor(p["bob"], in, scope)
	if e != nil || dup {
		t.Fatal("writer intake", e)
	}
	if w.RequesterPin == nil || *w.RequesterPin != p["bob"] || w.Publication.ActorID != "fixture-bob" || w.RuntimeContext == nil || w.RuntimeContext.Version != 2 || w.RuntimeContext.Requester == nil || *w.RuntimeContext.Requester != p["bob"] || w.RuntimeContext.PrimaryMessage.SenderID != "fixture-bob" || w.BotProfile != nil || w.Account != nil {
		t.Fatal("requester attribution or scope lost")
	}
	same, dup, e := s.AcceptFor(p["bob2"], in, scope)
	if e != nil || !dup || same.RequestID != w.RequestID || !reflect.DeepEqual(same.RequesterPin, w.RequesterPin) {
		t.Fatal("device rotation duplicated or repinned", e)
	}
	changed := in
	changed.BotID = "bot-davin"
	if _, _, e = s.AcceptFor(p["bob2"], changed, scope); !errors.Is(e, ErrConflict) {
		t.Fatal("global nonce wrong bot accepted", e)
	}
	if _, _, e = s.AcceptFor(p["reader"], in, scope); !errors.Is(e, ErrForbidden) {
		t.Fatal("reader sent", e)
	}
	if _, _, e = s.AcceptFor(p["eve"], in, scope); !errors.Is(e, ErrForbidden) {
		t.Fatal("outsider sent", e)
	}
	if _, _, e = s.Accept(in); !errors.Is(e, ErrForbidden) {
		t.Fatal("legacy executable intake in fixturemode", e)
	}
	other, dup, e := s.AcceptFor(p["owner"], in, scope)
	if e != nil || dup || other.RequestID == w.RequestID {
		t.Fatal("cross actor nonce collision", e)
	}
	before := s.Snapshot().Sequence
	if _, e = a.ControlDevice("bob-device", "revoke", 1); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Claim(w.RequestID, w.Epoch); e == nil {
		t.Fatal("revoked requester claimed")
	}
	if e = s.ReconcilePrincipals(); e != nil {
		t.Fatal(e)
	}
	got, _ := s.Get(w.RequestID)
	if got.Status != "suspended" || got.Epoch != w.Epoch+1 || got.Failure != "requester_authority_changed" || s.Snapshot().Sequence <= before {
		t.Fatal("revoke not durable")
	}
	*now = now.Add(time.Minute)
	dir := s.dir
	s.Close()
	reopened, e := Open(dir)
	if e != nil {
		t.Fatal("requester journal replay", e)
	}
	defer reopened.Close()
	restored, _ := reopened.Get(w.RequestID)
	if !reflect.DeepEqual(restored.RequesterPin, w.RequesterPin) || reopened.RuntimeAllowed(other.RequestID, other.Epoch) {
		t.Fatal("replay inferred live fixture grant")
	}
}
func TestFixtureScopedProjectionCursorAndACLInvalidation(t *testing.T) {
	s, a, p, _ := fixtureLedger(t)
	c := fixtureChannel(t, s, p["owner"])
	if _, _, e := s.SetBotProfile("bot-meiiie", BotProfileInput{Instructions: "PRIVATE_PROFILE_CANARY_FIXTURE", IncludeHistory: true, ExpectedRevision: 0, ClientNonce: "fixture-private-profile"}); e != nil {
		t.Fatal(e)
	}
	private, _, e := s.AcceptFor(p["owner"], Input{BotID: "bot-meiiie", ClientNonce: "fixture-private-canary", Text: "PRIVATE_SOURCE_CANARY_FIXTURE", Lane: "foreground"}, SubmissionScope{})
	if e != nil {
		t.Fatal(e)
	}
	w, _, e := s.AcceptFor(p["bob"], Input{BotID: "bot-meiiie", ClientNonce: "fixture-public-request", Text: "PUBLIC_HUMAN_FIXTURE", Lane: "foreground"}, SubmissionScope{ChannelID: c.ID, ExpectedChannelRevision: &c.Revision})
	if e != nil {
		t.Fatal(e)
	}
	snapshot, e := s.ScopedSnapshot(p["reader"])
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(snapshot)
	for _, forbidden := range []string{private.RequestID, "PRIVATE_SOURCE_CANARY_FIXTURE", "PRIVATE_PROFILE_CANARY_FIXTURE", "runtimeContext", "contextPin", "botProfilePin", "coreSha256", "runtimeAttemptBinding", "accountPin"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("scoped snapshot leaked %s", forbidden)
		}
	}
	if len(snapshot.Channels) != 1 || snapshot.Channels[0].SelfRole != "reader" || len(snapshot.Work) != 1 || snapshot.Work[0].RequestID != w.RequestID {
		t.Fatal("scoped visibility/selfRole missing")
	}
	scope := LocalSyncScope{BotID: "bot-meiiie", ChannelID: c.ID}
	sync, e := s.ScopedLocalSync(p["bob"], scope, "")
	if e != nil {
		t.Fatal(e)
	}
	token, _ := (syncstream.Cursor{Version: 1, InstanceID: sync.InstanceID, ScopeID: sync.ScopeID, Sequence: sync.To}).Encode()
	if _, e = s.ScopedLocalSync(p["bob2"], scope, token); !errors.Is(e, syncstream.ErrScope) {
		t.Fatal("cross session cursor accepted", e)
	}
	before := s.Snapshot().Sequence
	next, _, e := s.SaveChannelFor(p["owner"], ChannelInput{ID: c.ID, Name: c.Name, BotIDs: c.BotIDs, ExpectedRevision: c.Revision, ClientNonce: "fixture-acl-remove", HumanACL: &HumanACL{Version: 1, AuthorityID: a.AuthorityID(), Members: []HumanMember{{ActorID: "fixture-reader", Role: "reader"}}}})
	if e != nil || next.Revision != c.Revision+1 {
		t.Fatal(e)
	}
	events, e := s.ScopedEvents(p["bob"], before)
	if e != nil {
		t.Fatal(e)
	}
	invalidated := false
	for _, event := range events {
		if event.Event["type"] == "scope_invalidated" {
			invalidated = true
		}
		encoded, _ := json.Marshal(event)
		if strings.Contains(string(encoded), c.ID) || strings.Contains(string(encoded), w.Text) {
			t.Fatal("removed audience SSE retained data")
		}
	}
	if !invalidated {
		t.Fatal("audience removal emitted no invalidation")
	}
	empty, e := s.ScopedSnapshot(p["bob"])
	if e != nil || len(empty.Channels) != 0 || len(empty.Work) != 0 {
		t.Fatal("removed audience retained snapshot")
	}
	if _, e = s.ScopedLocalSync(p["bob"], scope, token); !errors.Is(e, ErrForbidden) {
		t.Fatal("ACL removed cursor allowed", e)
	}
}
