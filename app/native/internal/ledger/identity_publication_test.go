package ledger

import (
	"encoding/json"
	"nekoaurora.local/native/internal/identity"
	"strings"
	"testing"
	"time"
)

func TestFixturePublishedReportOutlivesPublisherSessionWithoutPrivateSource(t *testing.T) {
	s, a, p, now := fixtureLedger(t)
	c := fixtureChannel(t, s, p["owner"])
	source, _, e := s.AcceptFor(p["owner"], Input{BotID: "bot-meiiie", ClientNonce: "fixture-report-private", Text: "Make report. FIXTURE_REPORT_PRIVATE_INPUT_CANARY", Lane: "foreground"}, SubmissionScope{})
	if e != nil {
		t.Fatal(e)
	}
	source = fixtureAdmit(t, s, source)
	if _, e = s.ExecuteTool(source.RequestID, source.Epoch, fixtureInvocation(source, "PUBLISHED_REPORT_RESULT_BYTES_FIXTURE")); e != nil {
		t.Fatal(e)
	}
	if e = s.AppendText(source.RequestID, source.Epoch, "Completed report. PUBLISHED_RESULT_ONLY_FIXTURE"); e != nil {
		t.Fatal(e)
	}
	if e = s.Finish(source.RequestID, source.Epoch, "completed", ""); e != nil {
		t.Fatal(e)
	}
	source, _ = s.Get(source.RequestID)
	d, _, e := s.DeliverReportFor(p["owner"], ReportDeliveryInput{BotID: "bot-meiiie", DestinationChannelID: c.ID, ExpectedDestinationRevision: c.Revision, ClientNonce: "fixture-report-publish", Text: "@meiiie send the report from the private conversation to this channel"})
	if e != nil || d.Status != "delivered" || len(d.Artifacts) != 1 {
		t.Fatal("fixture owner publication", e)
	}
	*now = now.Add(31 * time.Second)
	if a.Valid(p["owner"]) {
		t.Fatal("publisher session stayed live")
	}
	ticket, e := a.BootstrapTicket("fixture-reader", "reader-device")
	if e != nil {
		t.Fatal(e)
	}
	_, fresh, e := a.Exchange(ticket)
	if e != nil {
		t.Fatal(e)
	}
	snapshot, e := s.ScopedSnapshot(fresh)
	if e != nil || len(snapshot.ReportDeliveries) != 1 || !snapshot.ReportDeliveries[0].Active || snapshot.ReportDeliveries[0].Reply != source.Reply || snapshot.ReportDeliveries[0].Source != nil {
		t.Fatal("publisher session expiry invalidated publication", e)
	}
	raw, _ := json.Marshal(snapshot)
	if strings.Contains(string(raw), "FIXTURE_REPORT_PRIVATE_INPUT_CANARY") || strings.Contains(string(raw), source.RequestID) {
		t.Fatal("private publication source leaked")
	}
	artifact, bytes, e := s.ScopedReadReportArtifact(fresh, d.ID, d.Artifacts[0].ID)
	if e != nil || string(bytes) != "PUBLISHED_REPORT_RESULT_BYTES_FIXTURE" || artifact != source.Artifacts[0] {
		t.Fatal("selected published raw artifact denied after publisher expiry", e)
	}
	if _, _, e = s.ScopedReadArtifact(fresh, c.ID, source.Artifacts[0].ID); e == nil {
		t.Fatal("private source artifact channel fallback")
	}
	if e = a.RevokeSession(fresh.SessionID); e != nil {
		t.Fatal(e)
	}
	if _, _, e = s.ScopedReadReportArtifact(fresh, d.ID, d.Artifacts[0].ID); e == nil {
		t.Fatal("revoked reader downloaded publication")
	}
}
func TestFixtureScopeReadLinearizesBeforeDeviceRevokeAndIntakeCAS(t *testing.T) {
	s, a, p, _ := fixtureLedger(t)
	c := fixtureChannel(t, s, p["owner"])
	entered := make(chan struct{})
	finish := make(chan struct{})
	readDone := make(chan error, 1)
	go func() {
		readDone <- s.WriteScopedSnapshot(p["bob"], func(out Snapshot) error { close(entered); <-finish; return nil })
	}()
	<-entered
	started := make(chan struct{})
	revoked := make(chan error, 1)
	go func() { close(started); _, e := a.ControlDevice("bob-device", "revoke", 1); revoked <- e }()
	<-started
	select {
	case <-revoked:
		close(finish)
		<-readDone
		t.Fatal("revoke passed a held publication read")
	case <-time.After(30 * time.Millisecond):
	}
	close(finish)
	if e := <-readDone; e != nil {
		t.Fatal(e)
	}
	if e := <-revoked; e != nil {
		t.Fatal(e)
	}
	if _, e := s.ScopedSnapshot(p["bob"]); e == nil {
		t.Fatal("post-revoke read accepted")
	}
	before := s.Snapshot().Sequence
	next, _, e := s.SaveChannelFor(p["owner"], ChannelInput{ID: c.ID, Name: c.Name, BotIDs: c.BotIDs, ExpectedRevision: c.Revision, ClientNonce: "fixture-cas-race", HumanACL: &HumanACL{Version: 1, AuthorityID: a.AuthorityID(), Members: []HumanMember{{ActorID: "fixture-bob", Role: "writer"}, {ActorID: "fixture-reader", Role: "reader"}}}})
	if e != nil || next.Revision != c.Revision+1 {
		t.Fatal(e)
	}
	after := s.Snapshot().Sequence
	if after <= before {
		t.Fatal("audience revision did not advance")
	}
	if _, _, e = s.AcceptFor(p["bob2"], Input{BotID: "bot-meiiie", ClientNonce: "fixture-cas-stale", Text: "Intent captured before audience changed", Lane: "foreground"}, SubmissionScope{ChannelID: c.ID, ExpectedChannelRevision: &c.Revision}); e == nil || s.Snapshot().Sequence != after {
		t.Fatal("stale audience intake CAS accepted")
	}
	forged := p["bob2"]
	forged.ActorID = "local-owner"
	if _, _, e = s.AcceptFor(forged, Input{BotID: "bot-meiiie", ClientNonce: "fixture-forged-owner", Text: "forged actor", Lane: "foreground"}, SubmissionScope{ChannelID: c.ID, ExpectedChannelRevision: &next.Revision}); e == nil || s.Snapshot().Sequence != after {
		t.Fatal("caller-spoofed VerifiedPrincipal accepted")
	}
	if !identity.ValidPrincipal(p["bob2"]) {
		t.Fatal("control fixture malformed")
	}
}
