package ledger

import (
	"encoding/json"
	"errors"
	"nekoaurora.local/native/internal/identity"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func fixtureAdmit(t *testing.T, s *Store, w Work) Work {
	t.Helper()
	w, e := s.Claim(w.RequestID, w.Epoch)
	if e != nil {
		t.Fatal(e)
	}
	b := CoreBinding{CoreSHA256: "df5c0feac1073d8471e567afca2e62a4e06f19ebb7d55f4d6537502c6dea0319", NativeProcessID: 41, SessionID: strings.Repeat("a", 32), Task: json.RawMessage(`{"id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","root":"/fixture","activationEpoch":1,"activationId":"cccccccccccccccccccccccccccccccc"}`), Execution: json.RawMessage(`{"authorityId":"fixture"}`)}
	b.ContextAdmission = &ContextAdmission{Version: 1, SessionID: b.SessionID, ContextPinSHA256: contextHash(w), ManifestSHA256: w.RuntimeContextSHA256, SourceIDs: []string{strings.Repeat("f", 64)}, Task: b.Task, Execution: b.Execution}
	old := b
	old.CoreSHA256 = "97ca4c8acff2e0bb6dc0b9577d856b3601d4c6c914579a9c9a1fe3cbb67c47fb"
	if validContextBinding(w, old) {
		t.Fatal("historical Core acquired requester v2 authority")
	}
	if e = s.BindRuntimeAttempt(w.RequestID, w.AttemptID, b); e != nil {
		t.Fatal(e)
	}
	if e = s.Admit(w.RequestID, w.Epoch, b); e != nil {
		t.Fatal(e)
	}
	w, _ = s.Get(w.RequestID)
	return w
}
func fixtureInvocation(w Work, text string) json.RawMessage {
	var f map[string]any
	json.Unmarshal(invocation(text), &f)
	f["_meta"].(map[string]any)["neko.contextAdmission"] = w.Core.ContextAdmission
	raw, _ := json.Marshal(f)
	return raw
}
func TestFixtureLiveRequesterFencesToolOutputAndHistoricalStop(t *testing.T) {
	s, a, p, _ := fixtureLedger(t)
	c := fixtureChannel(t, s, p["owner"])
	w, _, e := s.AcceptFor(p["bob"], Input{BotID: "bot-meiiie", ClientNonce: "fixture-runtime-live", Text: "Write a report file", Lane: "foreground"}, SubmissionScope{ChannelID: c.ID, ExpectedChannelRevision: &c.Revision})
	if e != nil {
		t.Fatal(e)
	}
	w = fixtureAdmit(t, s, w)
	before := s.Snapshot().Sequence
	if _, e = a.ControlDevice("bob-device", "revoke", 1); e != nil {
		t.Fatal(e)
	}
	if s.RuntimeAllowed(w.RequestID, w.Epoch) {
		t.Fatal("revoked live execution allowed")
	}
	if _, e = s.ExecuteTool(w.RequestID, w.Epoch, fixtureInvocation(w, "LATE_REVOKED_FILE_CANARY")); e == nil {
		t.Fatal("revoked artifact write accepted")
	}
	if e = s.AppendText(w.RequestID, w.Epoch, "LATE_REVOKED_REPLY_CANARY"); e == nil {
		t.Fatal("late revoked text accepted")
	}
	if e = s.Finish(w.RequestID, w.Epoch, "completed", ""); e == nil {
		t.Fatal("late revoked completion accepted")
	}
	if s.Snapshot().Sequence != before {
		t.Fatal("revoked output changed journal")
	}
	files, _ := os.ReadDir(filepath.Join(s.dir, "artifacts"))
	if len(files) != 0 {
		t.Fatal("revoked tool wrote physical output")
	}
	if e = s.ReconcilePrincipals(); e != nil {
		t.Fatal(e)
	}
	current, _ := s.Get(w.RequestID)
	if current.Status != "suspending" || current.Epoch != w.Epoch+1 {
		t.Fatal("active requester revoke failed")
	}
	bad := StoppedReceipt{Version: 1, NativeProcessID: 42, SessionID: w.Core.SessionID}
	if e = s.MarkRuntimeStopped(w.RequestID, w.AttemptID, bad); !errors.Is(e, ErrConflict) {
		t.Fatal("forged stop accepted", e)
	}
	receipt := StoppedReceipt{Version: 1, NativeProcessID: 41, SessionID: w.Core.SessionID}
	if e = s.MarkRuntimeStopped(w.RequestID, w.AttemptID, receipt); e != nil {
		t.Fatal("authentic stop denied after revoke", e)
	}
	dir := s.dir
	s.Close()
	reopened, e := Open(dir)
	if e != nil {
		t.Fatal("principal stop replay", e)
	}
	defer reopened.Close()
	restored, _ := reopened.Get(w.RequestID)
	if !restored.RuntimeStopped || !reflect.DeepEqual(restored.RuntimeStopReceipt, &receipt) || reopened.RuntimeAllowed(w.RequestID, restored.Epoch) {
		t.Fatal("stop proof/permission replay changed")
	}
}
func TestFixtureCompletedSharedHistorySurvivesSessionExpiry(t *testing.T) {
	s, a, p, now := fixtureLedger(t)
	c := fixtureChannel(t, s, p["owner"])
	w, _, e := s.AcceptFor(p["bob"], Input{BotID: "bot-meiiie", ClientNonce: "fixture-completed-history", Text: "Original shared human request", Lane: "foreground"}, SubmissionScope{ChannelID: c.ID, ExpectedChannelRevision: &c.Revision})
	if e != nil {
		t.Fatal(e)
	}
	w = fixtureAdmit(t, s, w)
	if e = s.AppendText(w.RequestID, w.Epoch, "COMPLETED_SHARED_RESULT_FIXTURE"); e != nil {
		t.Fatal(e)
	}
	if e = s.Finish(w.RequestID, w.Epoch, "completed", ""); e != nil {
		t.Fatal(e)
	}
	*now = now.Add(31 * time.Second)
	if a.Valid(p["bob"]) {
		t.Fatal("expired pin live")
	}
	if e = s.ReconcilePrincipals(); e != nil {
		t.Fatal(e)
	}
	ticket, e := a.BootstrapTicket("fixture-bob", "bob-device")
	if e != nil {
		t.Fatal(e)
	}
	_, fresh, e := a.Exchange(ticket)
	if e != nil {
		t.Fatal(e)
	}
	view, e := s.ScopedSnapshot(fresh)
	if e != nil || len(view.Work) != 1 || view.Work[0].Reply != "COMPLETED_SHARED_RESULT_FIXTURE" {
		t.Fatal("session expiry revoked historical shared data", e)
	}
	next, _, e := s.AcceptFor(fresh, Input{BotID: "bot-davin", ClientNonce: "fixture-completed-followup", Text: "Summarize the existing shared result", Lane: "foreground"}, SubmissionScope{ChannelID: c.ID, ExpectedChannelRevision: &c.Revision})
	if e != nil || len(next.Context.Sources) != 1 || next.Context.Sources[0].Reply != "COMPLETED_SHARED_RESULT_FIXTURE" {
		t.Fatal("completed history depends on expired live principal", e)
	}
	if !identity.ValidPrincipal(*next.RequesterPin) || next.RuntimeContext.Requester.SessionID == w.RequesterPin.SessionID {
		t.Fatal("new intake did not pin fresh principal")
	}
}
