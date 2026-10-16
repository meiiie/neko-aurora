package ledger

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"nekobot.local/native/internal/protocol"
)

func profileGroup(t *testing.T) (*Store, *profileServiceFixture, Collaboration) {
	t.Helper()
	s, f, _, _, _, input, intent := groupProfileSetup(t)
	c := groupProfileApprove(t, s, input)
	g, _, err := s.CreateCollaboration(groupProfileSelect(t, intent, c))
	if err != nil {
		t.Fatal(err)
	}
	return s, f, g
}

func TestGroupCoreProfileThreeStagesKeepOriginalSourceAndDeadline(t *testing.T) {
	s, _, g := profileGroup(t)
	deadline := g.CoreProfileGroup.Plan.DeadlineAtUnixMS
	original := g.CoreProfileGroup.OriginalPin
	for stage, weight := range []int{1, 1, 2} {
		current, w := advanceGroup(t, s, g.ID)
		if len(current.RequestIDs) != stage+1 || w.CoreProfileGroupBinding == nil || w.CoreProfileGroupBinding.Stage != stage+1 || w.CoreProfileGroupBinding.CollaborationID != g.ID || w.CoreProfilePin == nil {
			t.Fatal("stage mapping")
		}
		p := w.CoreProfilePin
		if p.SourceID != original.SourceID || p.SourceRevision != original.SourceRevision || p.ProfileRevision != original.ProfileRevision || p.ConsentID != original.ConsentID || p.ConsentRevision != original.ConsentRevision || p.CredentialSnapshotID != original.CredentialSnapshotID || p.ExpiresAtUnixMS != deadline || p.Limits.MaxRequests != weight || p.Limits.MaxSteps != weight || p.Limits.MaxOutputTokens != weight*682 {
			t.Fatal("stage repinned or reset shared budget")
		}
		w, b := admittedRecoveryStage(t, s, w)
		if err := s.Finish(w.RequestID, w.Epoch, "completed", ""); err != nil {
			t.Fatal(err)
		}
		serviceStopped(t, s, w, b)
	}
	if err := s.AdvanceCollaborations(); err != nil {
		t.Fatal(err)
	}
	final := groupState(t, s, g.ID)
	if final.Status != "completed" || len(final.RequestIDs) != 3 || final.DeadlineAt != g.DeadlineAt || final.CoreProfileGroup.OriginalPin != original || final.CoreProfileGroup.Budget.RecoveryReserved != (GroupCoreProfileQuota{}) {
		t.Fatal("final group changed authority or fabricated measured usage")
	}
	dir := s.dir
	s.Close()
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal("completed group replay", err)
	}
	defer reopened.Close()
	if !reflect.DeepEqual(groupState(t, reopened, g.ID), final) {
		t.Fatal("completed group replay changed pins")
	}
}

func TestGroupCoreProfileDeepCopyAndOrphanBinding(t *testing.T) {
	s, _, _, _, _, input, intent := groupProfileSetup(t)
	c := groupProfileApprove(t, s, input)
	selection := groupProfileSelect(t, intent, c)
	g, _, err := s.CreateCollaboration(selection)
	if err != nil {
		t.Fatal(err)
	}
	c.GroupIntent.Participants[0] = "caller-mutated-bot"
	c.GroupIntent.Goal = "caller-mutated-goal"
	g.CoreProfileGroup.Plan.StageAllocations[0].Limits.MaxRequests = 8
	g.CoreProfileSelection.ExpectedConsentRevision = 99
	stored := groupState(t, s, g.ID)
	if stored.CoreProfileSelection.ExpectedConsentRevision != 1 || stored.CoreProfileGroup.Plan.StageAllocations[0].Limits.MaxRequests != 1 {
		t.Fatal("detached group aliased store")
	}
	_, w := advanceGroup(t, s, g.ID)
	w.CoreProfileGroupBinding.Stage = 3
	got, _ := s.Get(w.RequestID)
	if got.CoreProfileGroupBinding.Stage != 1 {
		t.Fatal("detached Work binding aliased store")
	}
	legacy := intent
	legacy.ClientNonce = "group-profile-legacy-orphan"
	old, _, err := s.CreateCollaboration(legacy)
	if err != nil {
		t.Fatal(err)
	}
	probe := copyWork(got)
	probe.Collaboration = &CollaborationBinding{ID: old.ID, Stage: 1}
	probe.CoreProfileGroupBinding.CollaborationID = old.ID
	s.mu.Lock()
	accepted := s.validCoreProfileIntake(probe) || s.groupWorkProfileCurrent(probe)
	s.mu.Unlock()
	if accepted {
		t.Fatal("orphan profile binding adopted legacy group")
	}
	probe = copyWork(got)
	probe.CoreProfileGroupBinding = nil
	s.mu.Lock()
	accepted = s.validCoreProfileIntake(probe)
	s.mu.Unlock()
	if accepted {
		t.Fatal("missing group binding accepted")
	}
}

func TestGroupCoreProfileZeroOutputUnitAndIntentChangeBeforeCommit(t *testing.T) {
	for _, change := range []string{"zero_unit", "participants", "coordinator", "nonce", "purpose"} {
		t.Run(change, func(t *testing.T) {
			s, _, _, _, _, in, _ := groupProfileSetup(t)
			switch change {
			case "zero_unit":
				in.MaxOutputTokens = 1
			case "participants":
				in.GroupIntent.Participants[0] = "bot-sasha"
			case "coordinator":
				in.GroupIntent.CoordinatorBotID = "bot-sasha"
			case "nonce":
				in.GroupIntent.ClientNonce = in.ClientNonce
			case "purpose":
				in.Purpose = "single_bot_v1"
			}
			before := s.Snapshot().Sequence
			if _, _, err := s.ApproveCoreProfile(in); !errors.Is(err, ErrInvalid) || s.Snapshot().Sequence != before {
				t.Fatal("invalid finite consent committed", err)
			}
		})
	}
}

func TestGroupCoreProfileCancelDuringVerifyFencesDispatch(t *testing.T) {
	s, f, g := profileGroup(t)
	current, w := advanceGroup(t, s, g.ID)
	w, err := s.Claim(w.RequestID, w.Epoch)
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	f.mu.Lock()
	f.verify = func(CoreProfilePin) error { once.Do(func() { close(entered) }); <-release; return nil }
	f.mu.Unlock()
	done := make(chan error, 1)
	go func() { done <- s.VerifyWorkCoreProfile(w.RequestID, w.Epoch) }()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("off-lock verify did not start")
	}
	_, _, err = s.CancelCollaboration(g.ID, CollaborationCancelInput{ExpectedRevision: current.Revision, ClientNonce: "group-profile-cancel-during-verify"})
	close(release)
	if err != nil {
		t.Fatal("cancel blocked by credential verification", err)
	}
	if err = <-done; err == nil {
		t.Fatal("group-only cancellation missed after off-lock verify")
	}
	f.mu.Lock()
	f.verify = nil
	f.mu.Unlock()
	launched := false
	if err = s.Dispatch(w.RequestID, w.Epoch, func() error { launched = true; return nil }); err == nil || launched {
		t.Fatal("cancelled group dispatched native work", err)
	}
}

func TestGroupCoreProfileAcceptedStageCommitGapReplaysWithoutLaunch(t *testing.T) {
	s, f, r, _, _, input, intent := groupProfileSetup(t)
	c := groupProfileApprove(t, s, input)
	g, _, err := s.CreateCollaboration(groupProfileSelect(t, intent, c))
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	w, err := s.stageWorkLocked(g)
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	before := s.Snapshot().Sequence
	if _, err = s.Claim(w.RequestID, w.Epoch); err == nil || s.Snapshot().Sequence != before {
		t.Fatal("unmapped accepted stage claimed")
	}
	dir := s.dir
	s.Close()
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal("accepted gap replay", err)
	}
	defer reopened.Close()
	reopened.ConfigureCoreProfiles(f)
	reopened.ConfigureExecutor(r)
	reopened.ConfigureContextAdmission(true)
	if err = reopened.ReconcileCoreProfiles(); err != nil {
		t.Fatal(err)
	}
	if err = reopened.AdvanceCollaborations(); err != nil {
		t.Fatal(err)
	}
	current := groupState(t, reopened, g.ID)
	got, _ := reopened.Get(w.RequestID)
	if len(current.RequestIDs) != 1 || current.RequestIDs[0] != w.RequestID || got.Status != "waiting_host" || got.CoreProfilePin == nil || !reflect.DeepEqual(got.CoreProfilePin, w.CoreProfilePin) || current.CoreProfileGroup.Budget.RecoveryReserved != (GroupCoreProfileQuota{}) {
		t.Fatal("commit gap suspended/duplicated/reset the original accepted stage", current.Status, got.Status)
	}
	if _, err = reopened.Claim(w.RequestID, got.Epoch); err != nil {
		t.Fatal("mapped original stage did not claim", err)
	}
}

func TestGroupCoreProfileRecordedRevocationBlocksForgedNewEvents(t *testing.T) {
	s, _, g := profileGroup(t)
	_, w := advanceGroup(t, s, g.ID)
	consent := g.CoreProfileGroup.OriginalPin
	if _, _, err := s.ControlCoreProfile(CoreProfileConsentControlInput{ConsentID: consent.ConsentID, ExpectedRevision: consent.ConsentRevision, Action: "revoke", ClientNonce: "group-profile-revoke-recorded"}); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"create", "accepted", "claimed"} {
		t.Run(kind, func(t *testing.T) {
			s.mu.Lock()
			defer s.mu.Unlock()
			var e Event
			switch kind {
			case "create":
				copy := copyCollaboration(g)
				copy.ID = "collab-" + strings.Repeat("e", 32)
				copy.ClientNonce = "group-profile-forged-new-group"
				copy.CoreProfileGroup.IntentSHA256 = groupProfileIntentHash(groupProfileIntent(copy.CollaborationInput))
				e = Event{Sequence: uint64(len(s.events) + 1), Type: "collaboration.created", Collaboration: &copy}
			case "accepted":
				copy := copyWork(w)
				copy.RequestID = "req-" + strings.Repeat("f", 32)
				copy.SourceMessageID = "msg-" + strings.Repeat("e", 32)
				copy.CreatedSequence = uint64(len(s.events) + 1)
				e = Event{Sequence: copy.CreatedSequence, Type: "work.accepted", Work: copy}
			case "claimed":
				copy := copyWork(w)
				copy.Status = "starting"
				copy.AttemptID = "attempt-" + strings.Repeat("e", 32)
				e = Event{Sequence: uint64(len(s.events) + 1), Type: "work.claimed", Work: copy}
			}
			if validReplay(s, e) {
				t.Fatal("recorded revoked consent authorized forged", kind)
			}
		})
	}
	if _, _, err := s.CancelCollaboration(g.ID, CollaborationCancelInput{ExpectedRevision: groupState(t, s, g.ID).Revision, ClientNonce: fmt.Sprintf("group-profile-cancel-revoked-%d", w.Epoch)}); err != nil {
		t.Fatal("revoked group could not cancel", err)
	}
	dir := s.dir
	s.Close()
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal("revocation/cancel replay used live service", err)
	}
	defer reopened.Close()
}

func TestGroupCoreProfileRecoveryReservesOriginalCapsAtomically(t *testing.T) {
	s, _, r, _, _, input, intent := groupProfileSetup(t)
	c := groupProfileApprove(t, s, input)
	g, _, err := s.CreateCollaboration(groupProfileSelect(t, intent, c))
	if err != nil {
		t.Fatal(err)
	}
	var w Work
	var b CoreBinding
	for stage := 1; stage <= 3; stage++ {
		g, w = advanceGroup(t, s, g.ID)
		w, b = admittedRecoveryStage(t, s, w)
		if stage < 3 {
			if err = s.Finish(w.RequestID, w.Epoch, "completed", ""); err != nil {
				t.Fatal(err)
			}
			serviceStopped(t, s, w, b)
		}
	}
	originalPin := *w.CoreProfilePin
	originalBinding := *w.CoreProfileGroupBinding
	stop := func() {
		state := r.Snapshot()
		if err = r.MarkUnavailable(state.Pin, state.BootID, "host_disconnected"); err != nil {
			t.Fatal(err)
		}
		if err = s.ReconcileExecutors(); err != nil {
			t.Fatal(err)
		}
		w = serviceStopped(t, s, w, b)
		if err = s.AdvanceCollaborations(); err != nil {
			t.Fatal(err)
		}
		g = groupState(t, s, g.ID)
		state = r.Snapshot()
		if err = r.MarkReady(state.Pin, state.BootID); err != nil {
			t.Fatal(err)
		}
	}
	stop()
	in := groupRecoverInput(g, w, "group-profile-original-recovery")
	result, duplicate, err := s.RecoverCollaboration(g.ID, in)
	if err != nil || duplicate {
		t.Fatal("profile group recovery", err)
	}
	budget := result.Collaboration.CoreProfileGroup.Budget
	if budget.RecoveryReserved != (GroupCoreProfileQuota{2, 2, 1364}) || len(budget.RecoveryReservations) != 1 || budget.RecoveryReservations[0].ClientNonce != in.ClientNonce || *result.Work.CoreProfilePin != originalPin || *result.Work.CoreProfileGroupBinding != originalBinding || result.Work.RequestID != w.RequestID || result.Collaboration.DeadlineAt != g.DeadlineAt {
		t.Fatal("recovery reset grant, stage pin or reservation")
	}
	before := s.Snapshot().Sequence
	_, duplicate, err = s.RecoverCollaboration(g.ID, in)
	if err != nil || !duplicate || s.Snapshot().Sequence != before {
		t.Fatal("duplicate recovery reserved twice", err)
	}
	w, err = s.Claim(result.Work.RequestID, result.Work.Epoch)
	if err != nil {
		t.Fatal("resumed claim", err)
	}
	b = executorBinding(w, 503)
	b.NativeProcessGeneration = "134362178434013773"
	var task map[string]any
	if json.Unmarshal(w.Core.Task, &task) != nil {
		t.Fatal("saved task fixture")
	}
	task["activationEpoch"] = task["activationEpoch"].(float64) + 1
	task["activationId"] = strings.Repeat("e", 32)
	b.Task, _ = json.Marshal(task)
	b.Task, _ = protocol.Canonical(b.Task)
	b.Execution, _ = protocol.Canonical(b.Execution)
	admission := *w.Core.ContextAdmission
	admission.SourceIDs = append([]string(nil), admission.SourceIDs...)
	admission.Task, admission.Execution = b.Task, b.Execution
	b.ContextAdmission = &admission
	if err = s.BindRuntimeAttempt(w.RequestID, w.AttemptID, b); err != nil {
		t.Fatal("resumed binding", err)
	}
	if err = s.Admit(w.RequestID, w.Epoch, b); err != nil {
		t.Fatal("resumed admission", err)
	}
	w, _ = s.Get(w.RequestID)
	stop()
	before = s.Snapshot().Sequence
	if _, _, err = s.RecoverCollaboration(g.ID, groupRecoverInput(g, w, "group-profile-no-refund-recovery")); !errors.Is(err, ErrConflict) || s.Snapshot().Sequence != before {
		t.Fatal("crash refunded or widened exhausted reservation", err)
	}
	if _, err = s.Requeue(w.RequestID); !errors.Is(err, ErrConflict) {
		t.Fatal("generic recovery bypass", err)
	}
	if _, err = s.RefreshContext(RefreshInput{RequestID: w.RequestID, ExpectedEpoch: w.Epoch}); !errors.Is(err, ErrConflict) {
		t.Fatal("context refresh bypass", err)
	}
	dir := s.dir
	s.Close()
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal("recovery reservation replay", err)
	}
	defer reopened.Close()
	got := groupState(t, reopened, g.ID)
	if !reflect.DeepEqual(got.CoreProfileGroup.Budget, budget) || got.DeadlineAt != g.DeadlineAt {
		t.Fatal("replay refunded recovery pool")
	}
}

func TestGroupCoreProfileDeadlineObservationSurvivesRewindAndReplay(t *testing.T) {
	s, _, g := profileGroup(t)
	_, w := advanceGroup(t, s, g.ID)
	created, _ := time.Parse(time.RFC3339Nano, g.CreatedAt)
	now := time.UnixMilli(g.CoreProfileGroup.Plan.DeadlineAtUnixMS + 1)
	s.now = func() time.Time { return now }
	if err := s.VerifyWorkCoreProfile(w.RequestID, w.Epoch); err == nil {
		t.Fatal("expired group remained authorized")
	}
	closed := groupState(t, s, g.ID)
	if closed.BudgetClosedAt == "" || closed.BudgetClosedReason != "deadline_observed" {
		t.Fatal("deadline observation was not durable")
	}
	now = created.Add(time.Second)
	if err := s.VerifyWorkCoreProfile(w.RequestID, w.Epoch); err == nil {
		t.Fatal("clock rewind reopened the group")
	}
	dir := s.dir
	s.Close()
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal("expired group replay required live profile service", err)
	}
	defer reopened.Close()
	got := groupState(t, reopened, g.ID)
	if got.BudgetClosedAt != closed.BudgetClosedAt || got.CoreProfileGroup.Plan.DeadlineAtUnixMS != closed.CoreProfileGroup.Plan.DeadlineAtUnixMS {
		t.Fatal("restart reset original deadline")
	}
}
