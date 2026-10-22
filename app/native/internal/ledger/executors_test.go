package ledger

import (
	"encoding/json"
	"errors"
	"nekoaurora.local/native/internal/executors"
	"path/filepath"
	"strings"
	"testing"
)

func executorFixture(t *testing.T) (*Store, *executors.Registry) {
	t.Helper()
	s, e := Open(filepath.Join(t.TempDir(), "journal"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	d := executors.Descriptor{HostSHA256: strings.Repeat("1", 64), CoreSHA256: "ac643e3440ccf8a77e4451688f585643a646d4ba798b128a2c5c89a929c0dab5", RootSHA256: strings.Repeat("3", 64), CapabilitiesSHA256: strings.Repeat("4", 64)}
	r, e := executors.Open(filepath.Join(t.TempDir(), "executor"), d)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { r.Close() })
	p := r.Snapshot()
	if e = r.MarkReady(p.Pin, p.BootID); e != nil {
		t.Fatal(e)
	}
	s.ConfigureExecutor(r)
	return s, r
}

func executorBinding(w Work, pid uint32) CoreBinding {
	proof, _ := executors.ProofFor(*w.CurrentAttemptLease)
	return CoreBinding{NativeProcessID: pid, SessionID: strings.Repeat("a", 32), Task: json.RawMessage(`{"id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","root":"/fixture","activationEpoch":1,"activationId":"cccccccccccccccccccccccccccccccc"}`), Execution: json.RawMessage(`{"version":1,"authorityId":"fixture-authority"}`), CoreSHA256: w.ExecutorPin.CoreSHA256, ExecutorProof: &proof}
}
func TestExecutorCurrentStopBindingAcceptedAfterRevokeButAdmissionDenied(t *testing.T) {
	s, r := executorFixture(t)
	w, _, _ := s.Accept(Input{"bot-meiiie", "exec-late-proof-001", "hello", "foreground"})
	w, e := s.Claim(w.RequestID, w.Epoch)
	if e != nil {
		t.Fatal(e)
	}
	b := executorBinding(w, 401)
	p := r.Snapshot()
	r.Revoke(p.Pin, "late-proof-revoke")
	if e = s.ReconcileExecutors(); e != nil {
		t.Fatal(e)
	}
	if e = s.BindRuntimeAttempt(w.RequestID, w.AttemptID, b); e != nil {
		t.Fatal("current stop provenance lost", e)
	}
	if e = s.Admit(w.RequestID, w.Epoch, b); !errors.Is(e, ErrConflict) {
		t.Fatal("revoked admission", e)
	}
	bad := StoppedReceipt{Version: 1, NativeProcessID: 402, SessionID: b.SessionID, ExecutorProof: b.ExecutorProof}
	if e = s.MarkRuntimeStopped(w.RequestID, w.AttemptID, bad); !errors.Is(e, ErrConflict) {
		t.Fatal("wrong PID", e)
	}
	bad.NativeProcessID = 401
	if e = s.MarkRuntimeStopped(w.RequestID, w.AttemptID, bad); e != nil {
		t.Fatal(e)
	}
	if e = s.ReconcileExecutors(); e != nil {
		t.Fatal(e)
	}
	dir := s.dir
	s.Close()
	re, e := Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	defer re.Close()
	got, _ := re.Get(w.RequestID)
	if got.Status != "suspended" || got.Core != nil || got.RuntimeAttemptBinding.NativeProcessID != 401 {
		t.Fatal("receipt revived task")
	}
}
func TestExecutorOutputEffectCompletionAndResumeAreFenced(t *testing.T) {
	s, r := executorFixture(t)
	w, _, _ := s.Accept(Input{"bot-meiiie", "exec-output-001", "hello", "foreground"})
	w, e := s.Claim(w.RequestID, w.Epoch)
	if e != nil {
		t.Fatal(e)
	}
	b := executorBinding(w, 401)
	if e = s.BindRuntimeAttempt(w.RequestID, w.AttemptID, b); e != nil {
		t.Fatal(e)
	}
	if e = s.Admit(w.RequestID, w.Epoch, b); e != nil {
		t.Fatal(e)
	}
	if e = s.AppendText(w.RequestID, w.Epoch, "durable result"); e != nil {
		t.Fatal(e)
	}
	p := r.Snapshot()
	r.Revoke(p.Pin, "output-revoke")
	for _, action := range []func() error{func() error { return s.AppendText(w.RequestID, w.Epoch, "late") }, func() error { _, e := s.ExecuteTool(w.RequestID, w.Epoch, invocation("late artifact")); return e }, func() error { return s.Finish(w.RequestID, w.Epoch, "completed", "") }} {
		if e = action(); !errors.Is(e, ErrConflict) {
			t.Fatal("late mutation", e)
		}
	}
	if e = s.MarkRuntimeStopped(w.RequestID, w.AttemptID, StoppedReceipt{Version: 1, NativeProcessID: 401, SessionID: b.SessionID, ExecutorProof: b.ExecutorProof}); e != nil {
		t.Fatal(e)
	}
	if e = s.Finish(w.RequestID, w.Epoch, "recovery_required", "fixture interruption"); e != nil {
		t.Fatal(e)
	}
	before, _ := s.Get(w.RequestID)
	queued, e := s.Requeue(w.RequestID)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Claim(queued.RequestID, queued.Epoch); !errors.Is(e, ErrConflict) {
		t.Fatal("install rebound", e)
	}
	after, _ := s.Get(w.RequestID)
	if after.Core.CoreSHA256 != before.Core.CoreSHA256 || after.Reply != "durable result" || *after.ExecutorPin != *before.ExecutorPin {
		t.Fatal("historical provenance rewritten")
	}
}
func TestExecutorClaimPinsAndRevocationFences(t *testing.T) {
	s, r := executorFixture(t)
	w, _, e := s.Accept(Input{"bot-meiiie", "exec-test-001", "hello", "foreground"})
	if e != nil {
		t.Fatal(e)
	}
	w, e = s.Claim(w.RequestID, w.Epoch)
	if e != nil {
		t.Fatal(e)
	}
	if w.ExecutorPolicyVersion != 1 || w.ExecutorPin == nil || w.CurrentAttemptLease == nil {
		t.Fatal("no immutable install / attempt lease")
	}
	if w.CurrentAttemptLease.AuthoritySHA256 != ExecutorAuthorityHash(w) {
		t.Fatal("authority mismatch")
	}
	called := false
	if e = s.Dispatch(w.RequestID, w.Epoch, func() error { called = true; return nil }); e != nil || !called {
		t.Fatal(e)
	}
	p := r.Snapshot()
	if _, _, e = r.Revoke(p.Pin, "revoke-test-001"); e != nil {
		t.Fatal(e)
	}
	called = false
	if e = s.Dispatch(w.RequestID, w.Epoch, func() error { called = true; return nil }); !errors.Is(e, ErrConflict) || called {
		t.Fatalf("revoked dispatch: %v", e)
	}
	if s.RuntimeAllowed(w.RequestID, w.Epoch) {
		t.Fatal("revoked live lease")
	}
	if e = s.ReconcileExecutors(); e != nil {
		t.Fatal(e)
	}
	latest, _ := s.Get(w.RequestID)
	if latest.Status != "suspending" || latest.CurrentAttemptLease == nil || latest.CurrentAttemptLease.WorkEpoch != w.Epoch {
		t.Fatalf("bad suspension: %+v", latest)
	}
	proof, _ := executors.ProofFor(*w.CurrentAttemptLease)
	if e = s.MarkRuntimeStopped(w.RequestID, w.AttemptID, StoppedReceipt{Version: 1, NativeProcessID: 401, ExecutorProof: &proof}); e != nil {
		t.Fatal(e)
	}
	if e = s.ReconcileExecutors(); e != nil {
		t.Fatal(e)
	}
	latest, _ = s.Get(w.RequestID)
	if latest.Status != "suspended" {
		t.Fatal(latest.Status)
	}
}
func TestExecutorNoProofOrWrongAttemptNeverStops(t *testing.T) {
	s, _ := executorFixture(t)
	w, _, _ := s.Accept(Input{"bot-meiiie", "exec-test-002", "hello", "foreground"})
	w, e := s.Claim(w.RequestID, w.Epoch)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.MarkRuntimeStopped(w.RequestID, w.AttemptID, StoppedReceipt{Version: 1, NativeProcessID: 401}); !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
	proof, _ := executors.ProofFor(*w.CurrentAttemptLease)
	proof.AttemptID = "attempt-" + strings.Repeat("f", 32)
	if e = s.MarkRuntimeStopped(w.RequestID, w.AttemptID, StoppedReceipt{Version: 1, NativeProcessID: 401, ExecutorProof: &proof}); !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
}
func TestExecutorUnavailablePreservesQueueNoFallback(t *testing.T) {
	s, r := executorFixture(t)
	w, _, _ := s.Accept(Input{"bot-meiiie", "exec-test-003", "hello", "foreground"})
	p := r.Snapshot()
	r.MarkUnavailable(p.Pin, p.BootID, "host_disconnected")
	if _, e := s.Claim(w.RequestID, w.Epoch); !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
	latest, _ := s.Get(w.RequestID)
	if latest.Status != "waiting_host" || latest.AttemptID != "" {
		t.Fatal("queue obligation lost")
	}
}
func TestExecutorReplayDoesNotConsultCurrentRegistry(t *testing.T) {
	s, r := executorFixture(t)
	dir := s.dir
	w, _, _ := s.Accept(Input{"bot-meiiie", "exec-test-004", "hello", "foreground"})
	w, e := s.Claim(w.RequestID, w.Epoch)
	if e != nil {
		t.Fatal(e)
	}
	p := r.Snapshot()
	r.Revoke(p.Pin, "revoke-test-004")
	s.Close()
	reopened, e := Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	reopened.ConfigureExecutor(r)
	got, e := reopened.Get(w.RequestID)
	if e != nil || got.CurrentAttemptLease == nil || *got.CurrentAttemptLease != *w.CurrentAttemptLease {
		t.Fatal(e)
	}
	if e = reopened.RecoverInterrupted(); e != nil {
		t.Fatal(e)
	}
	if reopened.RuntimeAllowed(w.RequestID, w.Epoch) {
		t.Fatal("replay revived revoked lease")
	}
}
func TestExecutorFullLaunchEnvelopeBound(t *testing.T) {
	s, _ := executorFixture(t)
	w, _, e := s.Accept(Input{"bot-meiiie", "exec-test-005", strings.Repeat("\\", 30000), "foreground"})
	if e != nil {
		t.Fatal(e)
	}
	w, e = s.Claim(w.RequestID, w.Epoch)
	if e != nil {
		t.Fatal(e)
	}
	frame, e := MarshalRuntimeLaunch(w, nil)
	if e != nil || len(frame) > 64000 {
		t.Fatal(e)
	}
	if _, e = MarshalRuntimeLaunch(w, map[string]any{"contextManifest": strings.Repeat("x", 3000)}); !errors.Is(e, ErrInvalid) {
		t.Fatal("actual frame metadata not bounded", e)
	}
}

func TestExecutorOversizedCompanionRejectedBeforeIntakeAndLegacyTaskLocal(t *testing.T) {
	s, r := executorFixture(t)
	before := s.Snapshot().Sequence
	text := strings.Repeat("\\", 31000)
	if _, _, e := s.Accept(Input{"bot-meiiie", "executor-bound-negative", text, "foreground"}); !errors.Is(e, ErrInvalid) {
		t.Fatal("companion bound bypass", e)
	}
	if s.Snapshot().Sequence != before {
		t.Fatal("oversized intake committed")
	}
	s.executorRequired = false
	legacy, _, e := s.Accept(Input{"bot-meiiie", "executor-bound-legacy", text, "foreground"})
	if e != nil {
		t.Fatal(e)
	}
	s.ConfigureExecutor(r)
	before = s.Snapshot().Sequence
	if _, e = s.Claim(legacy.RequestID, legacy.Epoch); !errors.Is(e, ErrInvalid) {
		t.Fatal("legacy oversized claim", e)
	}
	if s.Snapshot().Sequence != before {
		t.Fatal("oversized claim committed")
	}
	if e = s.BlockRuntimeEnvelope(legacy.RequestID, legacy.Epoch); e != nil {
		t.Fatal(e)
	}
	dir := s.dir
	s.Close()
	for i := 0; i < 3; i++ {
		re, e := Open(dir)
		if e != nil {
			t.Fatal(e)
		}
		re.ConfigureExecutor(r)
		if e = re.RecoverInterrupted(); e != nil {
			t.Fatal(e)
		}
		got, _ := re.Get(legacy.RequestID)
		if got.Status != "suspended" || got.Failure != "runtime_frame_oversized" || got.ExecutorPin != nil {
			t.Fatal("global corruption or legacy rebound")
		}
		re.Close()
	}
}

func TestExecutorLegacyAdoptionAndLaunchedLegacyBlocked(t *testing.T) {
	s, r := executorFixture(t)
	s.executorRequired = false
	w, _, e := s.Accept(Input{"bot-meiiie", "exec-legacy-001", "hello", "foreground"})
	if e != nil {
		t.Fatal(e)
	}
	s.ConfigureExecutor(r)
	w, e = s.Claim(w.RequestID, w.Epoch)
	if e != nil || w.ExecutorPolicyVersion != 1 {
		t.Fatal(e)
	}
	// A historical admitted work can be replayed, but is never adopted silently.
	legacy, old := running(t, t.TempDir())
	defer legacy.Close()
	legacy.ConfigureExecutor(r)
	if _, e = legacy.Claim(old.RequestID, old.Epoch); !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
	if e = legacy.RecoverInterrupted(); e != nil {
		t.Fatal(e)
	}
	got, _ := legacy.Get(old.RequestID)
	if got.ExecutorPin != nil || got.Core.CoreSHA256 != old.Core.CoreSHA256 || legacy.RuntimeAllowed(old.RequestID, old.Epoch) {
		t.Fatal("launched legacy silently adopted")
	}
}
func TestExecutorNeverLaunchedReceiptDoesNotReviveAndRejectsAmbiguity(t *testing.T) {
	s, r := executorFixture(t)
	w, _, _ := s.Accept(Input{"bot-meiiie", "exec-nolaunch-001", "hello", "foreground"})
	w, e := s.Claim(w.RequestID, w.Epoch)
	if e != nil {
		t.Fatal(e)
	}
	proof, _ := executors.ProofFor(*w.CurrentAttemptLease)
	receipt := StoppedReceipt{Version: 2, Kind: "never_launched", ExecutorProof: &proof}
	for _, bad := range []StoppedReceipt{{Version: 2, Kind: "never_launched", NativeProcessID: 401, ExecutorProof: &proof}, {Version: 2, Kind: "other", ExecutorProof: &proof}, {Version: 2, Kind: "never_launched"}} {
		if validStoppedReceipt(w, bad) {
			t.Fatal("ambiguous receipt accepted")
		}
	}
	ambiguous := copyWork(w)
	ambiguous.Core = &CoreBinding{}
	if validStoppedReceipt(ambiguous, receipt) {
		t.Fatal("Core history accepts no-launch")
	}
	ambiguous = copyWork(w)
	ambiguous.Operations = []Operation{{Status: "started"}}
	if validStoppedReceipt(ambiguous, receipt) {
		t.Fatal("effect accepts no-launch")
	}
	p := r.Snapshot()
	r.Revoke(p.Pin, "exec-nolaunch-revoke")
	if e = s.ReconcileExecutors(); e != nil {
		t.Fatal(e)
	}
	if e = s.MarkRuntimeStopped(w.RequestID, w.AttemptID, receipt); e != nil {
		t.Fatal(e)
	}
	if s.RuntimeAllowed(w.RequestID, w.Epoch) {
		t.Fatal("no-launch revived")
	}
	dir := s.dir
	s.Close()
	for i := 0; i < 3; i++ {
		re, e := Open(dir)
		if e != nil {
			t.Fatal(e)
		}
		re.ConfigureExecutor(r)
		if e = re.RecoverInterrupted(); e != nil {
			t.Fatal(e)
		}
		got, _ := re.Get(w.RequestID)
		if got.Status != "suspended" || got.RuntimeStopReceipt.Version != 2 {
			t.Fatal(got.Status)
		}
		re.Close()
	}
}
func TestExecutorReplayRejectsPinLeaseMutationAndMixedVersion(t *testing.T) {
	s, _ := executorFixture(t)
	w, _, _ := s.Accept(Input{"bot-meiiie", "exec-replay-001", "hello", "foreground"})
	old := copyWork(w)
	w, e := s.Claim(w.RequestID, w.Epoch)
	if e != nil {
		t.Fatal(e)
	}
	delete(s.work, w.RequestID)
	s.work[w.RequestID] = old
	good := Event{Type: "work.claimed", Work: w, ExecutorClaimVersion: 1}
	if !validReplay(s, good) {
		t.Fatal("valid claim")
	}
	bad := good
	bad.Work = copyWork(w)
	bad.Work.CurrentAttemptLease.AuthoritySHA256 = strings.Repeat("0", 64)
	if validReplay(s, bad) {
		t.Fatal("wrong scope accepted")
	}
	bad = good
	bad.ExecutorClaimVersion = 0
	if validReplay(s, bad) {
		t.Fatal("version downgrade")
	}
	bad = good
	bad.Type = "work.text"
	if validReplay(s, bad) {
		t.Fatal("mixed version")
	}
}

func TestExecutorGroupPinFrozenAndRevocationPreventsStageTwo(t *testing.T) {
	s, r := executorFixture(t)
	c := sharedChannel(t, s, "Executor group", "executor-group-channel")
	in := CollaborationInput{ChannelID: c.ID, ExpectedChannelRevision: c.Revision, Participants: []string{"bot-meiiie", "bot-davin"}, CoordinatorBotID: "bot-meiiie", Goal: "Review the finite goal", MaxStages: 3, ClientNonce: "executor-group-create"}
	g, _, e := s.CreateCollaboration(in)
	if e != nil {
		t.Fatal(e)
	}
	_, w := advanceGroup(t, s, g.ID)
	if g.ExecutorPin == nil || w.ExecutorPin == nil || *g.ExecutorPin != *w.ExecutorPin {
		t.Fatal("stage rebind")
	}
	w, e = s.Claim(w.RequestID, w.Epoch)
	if e != nil {
		t.Fatal(e)
	}
	b := executorBinding(w, 401)
	if e = s.BindRuntimeAttempt(w.RequestID, w.AttemptID, b); e != nil {
		t.Fatal(e)
	}
	if e = s.Admit(w.RequestID, w.Epoch, b); e != nil {
		t.Fatal(e)
	}
	s.AppendText(w.RequestID, w.Epoch, "Retained first stage result")
	if e = s.Finish(w.RequestID, w.Epoch, "completed", ""); e != nil {
		t.Fatal(e)
	}
	p := r.Snapshot()
	r.Revoke(p.Pin, "executor-group-revoke")
	if e = s.AdvanceCollaborations(); e != nil {
		t.Fatal(e)
	}
	got := groupState(t, s, g.ID)
	if got.Status != "suspended" || len(got.RequestIDs) != 1 || *got.ExecutorPin != *g.ExecutorPin {
		t.Fatal("group rebound", got)
	}
	dir := s.dir
	s.Close()
	re, e := Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	defer re.Close()
	re.ConfigureExecutor(r)
	if e = re.RecoverInterrupted(); e != nil {
		t.Fatal(e)
	}
	if e = re.AdvanceCollaborations(); e != nil {
		t.Fatal(e)
	}
	got = groupState(t, re, g.ID)
	if got.Status != "suspended" || len(got.RequestIDs) != 1 {
		t.Fatal("restart created stage")
	}
}

func TestExecutorStopProofValidationIsHistoricalAfterExpiry(t *testing.T) {
	s, _ := executorFixture(t)
	w, _, _ := s.Accept(Input{"bot-meiiie", "executor-expired-static", "hello", "foreground"})
	w, e := s.Claim(w.RequestID, w.Epoch)
	if e != nil {
		t.Fatal(e)
	}
	// Typed historical wire fixture, not a live lease issued by the registry.
	w.CurrentAttemptLease.IssuedAtMs = 1
	w.CurrentAttemptLease.ExpiresAtMs = 2
	b := executorBinding(w, 401)
	w.Status = "suspending"
	w.DesiredState = "suspended"
	w.Epoch++
	if !validAttemptBinding(w, b) {
		t.Fatal("expired proof prevented recording current runtime binding")
	}
	w.RuntimeAttemptBinding = &b
	receipt := StoppedReceipt{Version: 1, NativeProcessID: 401, SessionID: b.SessionID, ExecutorProof: b.ExecutorProof}
	if !validStoppedReceipt(w, receipt) {
		t.Fatal("expired receipt lost original attempt")
	}
	wrong := *b.ExecutorProof
	wrong.WorkEpoch = w.Epoch
	receipt.ExecutorProof = &wrong
	if validStoppedReceipt(w, receipt) {
		t.Fatal("future epoch substituted into old proof")
	}
}

func TestExecutorSupplementAndReplaceDoNotRebindOrCopyAttemptLease(t *testing.T) {
	for _, action := range []string{"supplement", "replace"} {
		t.Run(action, func(t *testing.T) {
			s, r := executorFixture(t)
			w, _, e := s.Accept(Input{"bot-meiiie", "executor-control-" + action, "initial obligation", "foreground"})
			if e != nil {
				t.Fatal(e)
			}
			p := r.Snapshot()
			r.Revoke(p.Pin, "executor-control-revoke")
			result, _, e := s.ControlTask(TaskControlInput{Version: 1, RequestID: w.RequestID, ExpectedEpoch: w.Epoch, ExpectedRevision: 0, Action: action, Text: "linked obligation", ClientNonce: "executor-control-command"})
			if e != nil {
				t.Fatal(e)
			}
			if result.Related == nil || result.Related.ExecutorPolicyVersion != 1 || result.Related.ExecutorPin == nil || *result.Related.ExecutorPin != *w.ExecutorPin || result.Related.CurrentAttemptLease != nil {
				t.Fatal("control rebound authority")
			}
			if action == "replace" {
				s.ReconcileTaskControls()
			}
			related, _ := s.Get(result.Related.RequestID)
			if _, e = s.Claim(related.RequestID, related.Epoch); !errors.Is(e, ErrConflict) {
				t.Fatal("revoked control dispatched", e)
			}
		})
	}
}
