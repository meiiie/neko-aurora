package ledger

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func collaborationFixture(t *testing.T) (*Store, Collaboration, CollaborationInput, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	c := sharedChannel(t, s, "Collaboration", "group-fixture-channel")
	in := CollaborationInput{ChannelID: c.ID, ExpectedChannelRevision: c.Revision, Participants: []string{"bot-meiiie", "bot-davin"}, CoordinatorBotID: "bot-meiiie", Goal: "Compare plans and return a file", MaxStages: 3, ClientNonce: "group-fixture-create"}
	g, dup, err := s.CreateCollaboration(in)
	if err != nil || dup {
		t.Fatal(err)
	}
	return s, g, in, dir
}
func groupState(t *testing.T, s *Store, id string) Collaboration {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if g, ok := s.collaborations[id]; ok {
		return copyCollaboration(g)
	}
	t.Fatal("group missing")
	return Collaboration{}
}
func advanceGroup(t *testing.T, s *Store, id string) (Collaboration, Work) {
	t.Helper()
	if err := s.AdvanceCollaborations(); err != nil {
		t.Fatal(err)
	}
	g := groupState(t, s, id)
	if len(g.RequestIDs) == 0 {
		t.Fatal("stage missing", g)
	}
	w, err := s.Get(g.RequestIDs[len(g.RequestIDs)-1])
	if err != nil {
		t.Fatal(err)
	}
	return g, w
}
func admitGroup(t *testing.T, s *Store, w Work) Work {
	t.Helper()
	w, err := s.Claim(w.RequestID, w.Epoch)
	if err != nil {
		t.Fatal(err)
	}
	b := CoreBinding{NativeProcessID: 401, SessionID: strings.Repeat("a", 32), Task: json.RawMessage(`{"id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","root":"/fixture","activationEpoch":1,"activationId":"cccccccccccccccccccccccccccccccc"}`), Execution: json.RawMessage(`{"version":1}`), CoreSHA256: "ac643e3440ccf8a77e4451688f585643a646d4ba798b128a2c5c89a929c0dab5"}
	if err = s.Admit(w.RequestID, w.Epoch, b); err != nil {
		t.Fatal(err)
	}
	w, _ = s.Get(w.RequestID)
	return w
}

func TestCollaborationThreeStagesSingleWriterAndSharedContext(t *testing.T) {
	s, g, _, dir := collaborationFixture(t)
	private, _, err := s.Accept(Input{BotID: "bot-davin", ClientNonce: "group-private-canary", Text: "private-davin-canary", Lane: "foreground"})
	if err != nil {
		t.Fatal(err)
	}
	completeChannelWork(t, s, private, "private-reply-canary")
	var ids []string
	for stage, bot := range []string{"bot-meiiie", "bot-davin", "bot-meiiie"} {
		current, w := advanceGroup(t, s, g.ID)
		if len(current.RequestIDs) != stage+1 || w.BotID != bot || w.Collaboration.Stage != stage+1 {
			t.Fatal("stage budget/identity", current, w)
		}
		if strings.Contains(w.RuntimePrompt(), "private-davin-canary") {
			t.Fatal("private history imported")
		}
		for _, id := range ids {
			found := false
			for _, src := range w.Context.Sources {
				found = found || src.RequestID == id
			}
			if !found {
				t.Fatal("missing colleague source", id)
			}
		}
		w = admitGroup(t, s, w)
		result, err := s.ExecuteTool(w.RequestID, w.Epoch, invocation("final file"))
		if err != nil {
			t.Fatal(err)
		}
		if stage < 2 {
			if !strings.Contains(string(result), "isError") || strings.Contains(string(actionCapabilities(w)), "artifact.write_text") {
				t.Fatal("non-final writer granted", stage, string(result))
			}
		} else if strings.Contains(string(result), "isError") {
			t.Fatal("final writer denied", string(result))
		}
		if err = s.AppendText(w.RequestID, w.Epoch, bot+" result"); err != nil {
			t.Fatal(err)
		}
		if err = s.Finish(w.RequestID, w.Epoch, "completed", ""); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, w.RequestID)
	}
	if err = s.AdvanceCollaborations(); err != nil {
		t.Fatal(err)
	}
	final := groupState(t, s, g.ID)
	if final.Status != "completed" || !reflect.DeepEqual(final.RequestIDs, ids) {
		t.Fatal(final)
	}
	seq := s.Snapshot().Sequence
	for i := 0; i < 5; i++ {
		if err = s.AdvanceCollaborations(); err != nil {
			t.Fatal(err)
		}
	}
	if s.Snapshot().Sequence != seq {
		t.Fatal("budget extended")
	}
	s.Close()
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal("replay", err)
	}
	defer reopened.Close()
	if !reflect.DeepEqual(groupState(t, reopened, g.ID), final) {
		t.Fatal("mapping changed")
	}
}

func TestCollaborationTaskCommitBeforeGroupMapRestartIsIdempotent(t *testing.T) {
	s, g, in, dir := collaborationFixture(t)
	mapped, w := advanceGroup(t, s, g.ID)
	s.Close()
	path := filepath.Join(dir, "transactions.jsonl")
	b, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	if err := os.WriteFile(path, []byte(strings.Join(lines[:len(lines)-1], "\n")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		var err error
		s, err = Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		if err = s.AdvanceCollaborations(); err != nil {
			t.Fatal(err)
		}
		again := groupState(t, s, g.ID)
		if !reflect.DeepEqual(again, mapped) || len(s.Snapshot().Work) != 1 {
			t.Fatal("orphan stage duplicated", again)
		}
		retry, dup, err := s.CreateCollaboration(in)
		if err != nil || !dup || retry.RequestIDs[0] != w.RequestID {
			t.Fatal("lost group ACK", err)
		}
		s.Close()
	}
}

func TestCollaborationCancelFenceAndNoUnrelatedCancellation(t *testing.T) {
	s, g, _, dir := collaborationFixture(t)
	current, w := advanceGroup(t, s, g.ID)
	w = admitGroup(t, s, w)
	unrelated, _, err := s.Accept(Input{BotID: "bot-davin", ClientNonce: "group-unrelated-task", Text: "Independent owner task", Lane: "foreground"})
	if err != nil {
		t.Fatal(err)
	}
	cancel := CollaborationCancelInput{ExpectedRevision: current.Revision, ClientNonce: "group-owner-cancel"}
	cancelled, dup, err := s.CancelCollaboration(g.ID, cancel)
	if err != nil || dup || cancelled.Status != "cancelling" {
		t.Fatal(err)
	}
	if err = s.AppendText(w.RequestID, w.Epoch, "late reply"); !errors.Is(err, ErrConflict) {
		t.Fatal("late group reply", err)
	}
	if _, err = s.ExecuteTool(w.RequestID, w.Epoch, invocation("late file")); !errors.Is(err, ErrConflict) {
		t.Fatal("late effect", err)
	}
	other, _ := s.Get(unrelated.RequestID)
	if other.Status != "waiting_host" {
		t.Fatal("cancel escaped group")
	}
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.RecoverInterrupted(); err != nil {
		t.Fatal(err)
	}
	if err = s.AdvanceCollaborations(); err != nil {
		t.Fatal(err)
	}
	if groupState(t, s, g.ID).Status != "cancelling" {
		t.Fatal("cancel crash gap fabricated runtime stop")
	}
	if err = s.MarkRuntimeStopped(w.RequestID, w.AttemptID, StoppedReceipt{Version: 1, NativeProcessID: 401, SessionID: strings.Repeat("a", 32)}); err != nil {
		t.Fatal(err)
	}
	if err = s.AdvanceCollaborations(); err != nil {
		t.Fatal(err)
	}
	if groupState(t, s, g.ID).Status != "cancelled" {
		t.Fatal("stop receipt not reconciled")
	}
	if _, dup, err = s.CancelCollaboration(g.ID, cancel); err != nil || !dup {
		t.Fatal("lost cancel ACK", err)
	}
}

func TestCollaborationResourcePinDoesNotSilentlySelectNewUploads(t *testing.T) {
	s, c, p := productFixture(t)
	f, _, err := s.UploadFile(UploadInput{ProjectID: p.ID, ProjectRevision: p.Revision, ChannelID: c.ID, ClientNonce: "group-file-original", Filename: "quote.txt", Content: []byte("Vendor quote original120")})
	if err != nil {
		t.Fatal(err)
	}
	g, _, err := s.CreateCollaboration(CollaborationInput{ChannelID: c.ID, ExpectedChannelRevision: c.Revision, Participants: []string{"bot-meiiie", "bot-davin"}, CoordinatorBotID: "bot-meiiie", Goal: "Compare vendor quote", MaxStages: 3, ClientNonce: "group-resource-create", ProjectID: p.ID, ProjectRevision: p.Revision})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = s.UploadFile(UploadInput{ProjectID: p.ID, ProjectRevision: p.Revision, ChannelID: c.ID, ClientNonce: "group-file-newer", Filename: "quote-new.txt", Content: []byte("Vendor quote newer180")})
	if err != nil {
		t.Fatal(err)
	}
	_, w := advanceGroup(t, s, g.ID)
	if len(w.Resources.Files) != 1 || w.Resources.Files[0].ID != f.ID {
		t.Fatal("newer sources selected")
	}
	completeChannelWork(t, s, w, "first proposal")
	_, next := advanceGroup(t, s, g.ID)
	if !reflect.DeepEqual(next.Resources, g.Resources) {
		t.Fatal("stage resources changed")
	}
	if err = s.SetFileAccess(FileAccessInput{FileID: f.ID, ExpectedRevision: 0, Granted: false, ClientNonce: "group-revoke-file"}); err != nil {
		t.Fatal(err)
	}
	if err = s.AdvanceCollaborations(); err != nil {
		t.Fatal(err)
	}
	if groupState(t, s, g.ID).Status != "suspended" {
		t.Fatal("resource revocation ignored")
	}
}

func TestCollaborationStopsWhenStageOutcomeIsUnresolved(t *testing.T) {
	s, g, _, _ := collaborationFixture(t)
	_, w := advanceGroup(t, s, g.ID)
	w = admitGroup(t, s, w)
	if err := s.Finish(w.RequestID, w.Epoch, "recovery_required", "transport_interrupted"); err != nil {
		t.Fatal(err)
	}
	if err := s.AdvanceCollaborations(); err != nil {
		t.Fatal(err)
	}
	current := groupState(t, s, g.ID)
	if current.Status != "needs_owner" || len(current.RequestIDs) != 1 {
		t.Fatal("unbounded retry", current)
	}
	if _, err := s.Requeue(w.RequestID); !errors.Is(err, ErrConflict) {
		t.Fatal("group resumed implicitly", err)
	}
}

func TestCollaborationAudienceAndAccountPinsStopBeforeNextStage(t *testing.T) {
	for _, kind := range []string{"channel", "account"} {
		t.Run(kind, func(t *testing.T) {
			s, c, a, _, _ := accountFixture(t)
			channel := sharedChannel(t, s, "Authority", "group-authority-channel")
			in := CollaborationInput{ChannelID: channel.ID, ExpectedChannelRevision: channel.Revision, Participants: []string{"bot-meiiie", "bot-davin"}, CoordinatorBotID: "bot-meiiie", Goal: "Finite decision", MaxStages: 3, ClientNonce: "group-authority-create", AccountID: a.ID, AccountRevision: a.Revision}
			g, _, err := s.CreateCollaboration(in)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "channel" {
				_, _, err = s.SaveChannel(ChannelInput{ID: channel.ID, ExpectedRevision: 1, Name: channel.Name, BotIDs: channel.BotIDs, ClientNonce: "group-revise-channel"})
			} else {
				err = c.RemoveAccount(a.ID, a.Revision)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err = s.AdvanceCollaborations(); err != nil {
				t.Fatal(err)
			}
			current := groupState(t, s, g.ID)
			if current.Status != "suspended" || len(current.RequestIDs) != 0 {
				t.Fatal("authority silently rebound", current)
			}
		})
	}
}

func TestCollaborationTimeBudgetStopsWithoutAnotherStage(t *testing.T) {
	s, g, _, _ := collaborationFixture(t)
	created, err := time.Parse(time.RFC3339Nano, g.CreatedAt)
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return created.Add(6 * time.Minute) }
	if err = s.AdvanceCollaborations(); err != nil {
		t.Fatal(err)
	}
	current := groupState(t, s, g.ID)
	if current.Status != "needs_owner" || current.Reason != "stage_budget_expired" || len(current.RequestIDs) != 0 {
		t.Fatal("time budget extended", current)
	}
}

func TestCollaborationRetainsCompletedResultsAfterOwnerCancel(t *testing.T) {
	s, g, _, _ := collaborationFixture(t)
	current, w := advanceGroup(t, s, g.ID)
	completeChannelWork(t, s, w, "completed coordinator proposal")
	if _, _, err := s.CancelCollaboration(g.ID, CollaborationCancelInput{ExpectedRevision: current.Revision, ClientNonce: "cancel-before-review"}); err != nil {
		t.Fatal(err)
	}
	if err := s.AdvanceCollaborations(); err != nil {
		t.Fatal(err)
	}
	follow, _, err := s.AcceptScoped(Input{BotID: "bot-davin", ClientNonce: "owner-after-cancel", Text: "Summarize already shared proposal", Lane: "foreground"}, g.ChannelID, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, src := range follow.Context.Sources {
		found = found || src.RequestID == w.RequestID
	}
	if !found {
		t.Fatal("completed result lost from authorized channel context")
	}
}

func TestCollaborationCancelledDoesNotAssertStoppedWithoutReceipt(t *testing.T) {
	s, g, _, dir := collaborationFixture(t)
	current, w := advanceGroup(t, s, g.ID)
	w, err := s.Claim(w.RequestID, w.Epoch)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.CancelCollaboration(g.ID, CollaborationCancelInput{ExpectedRevision: current.Revision, ClientNonce: "cancel-before-admission"}); err != nil {
		t.Fatal(err)
	}
	if err = s.RecoverInterrupted(); err != nil {
		t.Fatal(err)
	}
	if err = s.AdvanceCollaborations(); err != nil {
		t.Fatal(err)
	}
	if groupState(t, s, g.ID).Status != "cancelling" {
		t.Fatal("group claimed actual runtime stopped without receipt")
	}
	if err = s.MarkRuntimeStopped(w.RequestID, w.AttemptID, StoppedReceipt{Version: 1, NativeProcessID: 401}); err != nil {
		t.Fatal(err)
	}
	if err = s.AdvanceCollaborations(); err != nil {
		t.Fatal(err)
	}
	final := groupState(t, s, g.ID)
	if final.Status != "cancelled" {
		t.Fatal("stop receipt not settled")
	}
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if !reflect.DeepEqual(groupState(t, s, g.ID), final) {
		t.Fatal("stop proof replay")
	}
}

func TestCollaborationCancelIncludesAcceptedUnmappedStageAfterCrash(t *testing.T) {
	s, g, _, dir := collaborationFixture(t)
	_, w := advanceGroup(t, s, g.ID)
	s.Close()
	path := filepath.Join(dir, "transactions.jsonl")
	b, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	if err := os.WriteFile(path, []byte(strings.Join(lines[:len(lines)-1], "\n")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.Claim(w.RequestID, w.Epoch); !errors.Is(err, ErrConflict) {
		t.Fatal("unmapped orphan dispatched", err)
	}
	if _, _, err = s.CancelCollaboration(g.ID, CollaborationCancelInput{ExpectedRevision: 1, ClientNonce: "cancel-unmapped-orphan"}); err != nil {
		t.Fatal(err)
	}
	if err = s.AdvanceCollaborations(); err != nil {
		t.Fatal(err)
	}
	current, _ := s.Get(w.RequestID)
	if current.Status != "cancelled" || len(groupState(t, s, g.ID).RequestIDs) != 0 || groupState(t, s, g.ID).Status != "cancelled" {
		t.Fatal("orphan cancellation escaped mapping", current)
	}
}

func TestCollaborationReplayCannotForgeStageWriterOrGoal(t *testing.T) {
	s, g, _, _ := collaborationFixture(t)
	_, w := advanceGroup(t, s, g.ID)
	e := Event{Type: "work.accepted", Sequence: w.CreatedSequence, Work: copyWork(w)}
	delete(s.work, w.RequestID)
	delete(s.nonces, workNonceKey(w))
	original := s.collaborations[g.ID]
	original.RequestIDs = []string{}
	original.Revision = 1
	s.collaborations[g.ID] = original
	s.events = s.events[:2]
	if !validReplay(s, e) {
		t.Fatal("unmodified accepted-event control failed")
	}
	for _, kind := range []string{"stage", "goal", "bot", "audience"} {
		t.Run(kind, func(t *testing.T) {
			forged := e
			forged.Work = copyWork(e.Work)
			switch kind {
			case "stage":
				forged.Work.Collaboration.Stage = 3
			case "goal":
				forged.Work.Text = "Changed owner goal"
			case "bot":
				forged.Work.BotID = "bot-davin"
			case "audience":
				forged.Work.Publication.AudienceRevision++
			}
			if validReplay(s, forged) {
				t.Fatal("forged collaboration replay accepted", kind)
			}
		})
	}
}

func TestCollaborationNewCompletionRequiresResolvedVersionedReceipt(t *testing.T) {
	s, g, _, _ := collaborationFixture(t)
	_, w := advanceGroup(t, s, g.ID)
	w = admitGroup(t, s, w)
	completed := copyWork(w)
	completed.Status = "completed"
	if validRuntimeReplay(s, Event{Type: "work.completed", Work: completed}) {
		t.Fatal("new group task accepted legacy unversioned completion")
	}
	if !validRuntimeReplay(s, Event{Type: "work.completed", Work: completed, CompletionVersion: 1}) {
		t.Fatal("versioned completed control rejected")
	}
}

func TestCollaborationConcurrentAdvanceAndCancelRemainBounded(t *testing.T) {
	for i := 0; i < 20; i++ {
		s, g, _, _ := collaborationFixture(t)
		var wait sync.WaitGroup
		wait.Add(2)
		start := make(chan struct{})
		var advanceErr, cancelErr error
		go func() { defer wait.Done(); <-start; advanceErr = s.AdvanceCollaborations() }()
		go func() {
			defer wait.Done()
			<-start
			_, _, cancelErr = s.CancelCollaboration(g.ID, CollaborationCancelInput{ExpectedRevision: 1, ClientNonce: "concurrent-group-cancel"})
		}()
		close(start)
		wait.Wait()
		if advanceErr != nil {
			t.Fatal(advanceErr)
		}
		if errors.Is(cancelErr, ErrConflict) {
			current := groupState(t, s, g.ID)
			_, _, cancelErr = s.CancelCollaboration(g.ID, CollaborationCancelInput{ExpectedRevision: current.Revision, ClientNonce: "concurrent-group-cancel"})
		}
		if cancelErr != nil {
			t.Fatal(cancelErr)
		}
		if err := s.AdvanceCollaborations(); err != nil {
			t.Fatal(err)
		}
		current := groupState(t, s, g.ID)
		if current.Status != "cancelled" || len(current.RequestIDs) > 1 {
			t.Fatal("concurrent scope/budget", current)
		}
		for _, w := range s.Snapshot().Work {
			if w.Status != "cancelled" || w.Collaboration.ID != g.ID {
				t.Fatal("concurrent orphan survived", w)
			}
		}
		s.Close()
	}
}

func TestCollaborationStageAcceptanceRechecksRemovedAccount(t *testing.T) {
	s, c, a, _, _ := accountFixture(t)
	channel := sharedChannel(t, s, "Guard", "group-pin-guard-channel")
	g, _, err := s.CreateCollaboration(CollaborationInput{ChannelID: channel.ID, ExpectedChannelRevision: 1, Participants: []string{"bot-meiiie", "bot-davin"}, CoordinatorBotID: "bot-meiiie", Goal: "Bounded comparison", MaxStages: 3, ClientNonce: "group-pin-guard-create", AccountID: a.ID, AccountRevision: a.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if err = c.RemoveAccount(a.ID, a.Revision); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	before := len(s.events)
	if _, err = s.stageWorkLocked(g); !errors.Is(err, ErrConflict) || len(s.events) != before {
		t.Fatal("stage commit admitted stale account", err)
	}
}
