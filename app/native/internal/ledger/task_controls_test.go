package ledger

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func controlInput(w Work, action, nonce string) TaskControlInput {
	return TaskControlInput{Version: 1, RequestID: w.RequestID, ExpectedEpoch: w.Epoch, ExpectedRevision: w.TaskRevision, Action: action, ClientNonce: nonce}
}

func TestTaskControlsQueuedPauseResumeCASAndReplay(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	w, _, err := s.Accept(input("control-queued-intake", "background"))
	if err != nil {
		t.Fatal(err)
	}
	pause := controlInput(w, "pause", "control-pause-command")
	result, dup, err := s.ControlTask(pause)
	if err != nil || dup {
		t.Fatal(err)
	}
	paused := result.Work
	if paused.Status != "paused" || paused.DesiredState != "paused" || paused.Epoch != w.Epoch+1 || !reflect.DeepEqual(paused.Context, w.Context) {
		t.Fatal("pause changed pin or lost obligation")
	}
	if _, err = s.Claim(paused.RequestID, paused.Epoch); !errors.Is(err, ErrConflict) {
		t.Fatal("paused dispatch", err)
	}
	stale := controlInput(w, "resume", "control-resume-stale")
	before := s.Snapshot().Sequence
	if _, _, err = s.ControlTask(stale); !errors.Is(err, ErrConflict) || s.Snapshot().Sequence != before {
		t.Fatal("stale resume committed", err)
	}
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	result, dup, err = s.ControlTask(pause)
	if err != nil || !dup || result.Work.RequestID != w.RequestID {
		t.Fatal("lost ACK replay", err)
	}
	resume := controlInput(result.Work, "resume", "control-resume-command")
	result, _, err = s.ControlTask(resume)
	if err != nil {
		t.Fatal(err)
	}
	if result.Work.Status != "waiting_host" || result.Work.RequestID != w.RequestID || !reflect.DeepEqual(result.Work.Context, w.Context) || result.Work.TaskRevision != 2 {
		t.Fatal("resume did not preserve obligation")
	}
	if _, _, err = s.ControlTask(pause); err != nil {
		t.Fatal("old committed receipt", err)
	}
	changed := pause
	changed.Action = "cancel"
	if _, _, err = s.ControlTask(changed); !errors.Is(err, ErrConflict) {
		t.Fatal("nonce payload reuse", err)
	}
}

func TestTaskControlsUnsupportedLaunchedPauseDoesNotCancel(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	w, _, _ := s.Accept(input("control-launched-intake", "background"))
	w = startPermissionWork(t, s, w)
	before := s.Snapshot().Sequence
	if _, _, err = s.ControlTask(controlInput(w, "pause", "control-launched-pause")); !errors.Is(err, ErrUnsupported) {
		t.Fatal("unnegotiated pause admitted", err)
	}
	after, _ := s.Get(w.RequestID)
	if !reflect.DeepEqual(after, w) || s.Snapshot().Sequence != before {
		t.Fatal("unsupported pause cancelled or changed task")
	}
}

func TestTaskControlsOfflineSupplementAfterCancelIsRejected(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	w, _, _ := s.Accept(input("control-cancel-intake", "background"))
	stale := controlInput(w, "supplement", "control-offline-supplement")
	stale.Text = "Also include the appendix"
	result, _, err := s.ControlTask(controlInput(w, "cancel", "control-cancel-command"))
	if err != nil {
		t.Fatal(err)
	}
	before := s.Snapshot().Sequence
	if _, _, err = s.ControlTask(stale); !errors.Is(err, ErrConflict) || s.Snapshot().Sequence != before {
		t.Fatal("offline command revived task", err)
	}
	stale.ExpectedEpoch = result.Work.Epoch
	if _, _, err = s.ControlTask(stale); !errors.Is(err, ErrConflict) {
		t.Fatal("cancelled task accepted fresh supplement", err)
	}
}

func TestTaskControlsSupplementKeepsActiveRunAndLinkedIdentity(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	w, _, _ := s.Accept(input("control-supplement-intake", "background"))
	w = startPermissionWork(t, s, w)
	in := controlInput(w, "supplement", "control-supplement-command")
	in.Text = "Include compatible source notes"
	result, _, err := s.ControlTask(in)
	if err != nil {
		t.Fatal(err)
	}
	if result.Related == nil || result.Related.RequestID == w.RequestID || result.Work.Status != "running" || result.Work.Epoch != w.Epoch || result.Work.Text != w.Text || !reflect.DeepEqual(result.Work.Core, w.Core) || !reflect.DeepEqual(result.Work.Context, w.Context) {
		t.Fatal("supplement mutated active run")
	}
	link := result.Related.TaskLink
	if link == nil || link.ParentRequestID != w.RequestID || link.ParentMessageID != w.SourceMessageID || link.Kind != "supplement" || link.ParentPinSHA256 != w.Context.SHA256 {
		t.Fatal("durable provenance missing")
	}
	if err = s.AppendText(w.RequestID, w.Epoch, "old work continues"); err != nil {
		t.Fatal(err)
	}
	relatedID := result.Related.RequestID
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	result, dup, err := s.ControlTask(in)
	if err != nil || !dup || result.Related.RequestID != relatedID || result.Work.Reply != "old work continues" {
		t.Fatal("supplement duplicated on replay", err)
	}
}

func TestTaskControlsReplacementWaitsNativeStopAndFencesLateOutput(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	w, _, _ := s.Accept(input("control-replace-intake", "background"))
	w = startPermissionWork(t, s, w)
	if err = s.AppendText(w.RequestID, w.Epoch, "retained draft"); err != nil {
		t.Fatal(err)
	}
	w, _ = s.Get(w.RequestID)
	in := controlInput(w, "replace", "control-replace-command")
	in.Text = "Make a document instead of slides"
	result, _, err := s.ControlTask(in)
	if err != nil {
		t.Fatal(err)
	}
	old := result.Work
	successor := *result.Related
	if old.DesiredState != "cancelled" || old.Status != "cancelling" || successor.Status != "waiting_control_stop" || successor.TaskLink.Kind != "replace" {
		t.Fatal("replacement not fenced")
	}
	if err = s.AppendText(w.RequestID, w.Epoch, "late old final"); !errors.Is(err, ErrConflict) {
		t.Fatal("late output accepted", err)
	}
	if _, err = s.Claim(successor.RequestID, successor.Epoch); !errors.Is(err, ErrConflict) {
		t.Fatal("successor ran before native stop", err)
	}
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RecoverInterrupted(); err != nil {
		t.Fatal(err)
	}
	if err = s.ReconcileTaskControls(); err != nil {
		t.Fatal(err)
	}
	current, _ := s.Get(successor.RequestID)
	if current.Status != "waiting_control_stop" {
		t.Fatal("restart assumed native child stopped")
	}
	if err = s.MarkRuntimeStopped(w.RequestID, w.AttemptID, StoppedReceipt{Version: 1, NativeProcessID: 402, SessionID: w.Core.SessionID}); !errors.Is(err, ErrConflict) {
		t.Fatal("wrong PID accepted", err)
	}
	if err = s.MarkRuntimeStopped(w.RequestID, w.AttemptID, StoppedReceipt{Version: 1, NativeProcessID: 401, SessionID: w.Core.SessionID}); err != nil {
		t.Fatal(err)
	}
	if err = s.ReconcileTaskControls(); err != nil {
		t.Fatal(err)
	}
	current, _ = s.Get(successor.RequestID)
	if current.Status != "waiting_host" {
		t.Fatal("stopped successor not released")
	}
	parent, _ := s.Get(w.RequestID)
	if parent.Reply != "retained draft" || current.Reply != "" || current.Core != nil {
		t.Fatal("old final attached to replacement")
	}
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	current, _ = s.Get(successor.RequestID)
	if current.Status != "waiting_host" {
		t.Fatal("release replay lost")
	}
}

func TestTaskControlsReplacementUnknownEffectBlocksDispatch(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	w, _, _ := s.Accept(input("control-effect-intake", "background"))
	w = startPermissionWork(t, s, w)
	text := "control unresolved physical write"
	hash := sha256.Sum256([]byte(w.RequestID + ":" + strings.Repeat("a", 32) + ":" + strings.Repeat("d", 32)))
	if err = os.MkdirAll(filepath.Join(dir, "artifacts", hex.EncodeToString(hash[:])), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ExecuteTool(w.RequestID, w.Epoch, invocation(text)); err == nil {
		t.Fatal("effect failure fixture did not fail")
	}
	in := controlInput(w, "replace", "control-effect-replace")
	in.Text = "New document goal"
	result, _, err := s.ControlTask(in)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.MarkRuntimeStopped(w.RequestID, w.AttemptID, StoppedReceipt{Version: 1, NativeProcessID: 401, SessionID: w.Core.SessionID}); err != nil {
		t.Fatal(err)
	}
	s.Close()
	for i := 0; i < 3; i++ {
		s, err = Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		if err = s.RecoverInterrupted(); err != nil {
			t.Fatal(err)
		}
		next, _ := s.Get(result.Related.RequestID)
		parent, _ := s.Get(w.RequestID)
		if next.Status != "waiting_control_stop" || parent.Operations[0].Status != "unknown" {
			t.Fatal("unknown effect released replacement")
		}
		if _, _, err = s.Accept(input("control-unrelated-intake", "foreground")); err != nil {
			t.Fatal("task-local unknown blocked service", err)
		}
		s.Close()
	}
}

func TestTaskControlsResumeRevalidatesWithdrawnSource(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	source := completedPermissionSource(t, s, "control-source-intake", "source notes")
	w, _, _ := s.Accept(input("control-consumer-intake", "background"))
	result, _, err := s.ControlTask(controlInput(w, "pause", "control-source-pause"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.SetSourceAccess(AccessInput{source.RequestID, 0, false, "control-source-revoke"}); err != nil {
		t.Fatal(err)
	}
	before := s.Snapshot().Sequence
	if _, _, err = s.ControlTask(controlInput(result.Work, "resume", "control-source-resume")); !errors.Is(err, ErrConflict) || s.Snapshot().Sequence != before {
		t.Fatal("stale pin resumed", err)
	}
}

func TestTaskControlsSupplementRevisionCASWithoutStoppingRun(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	w, _, _ := s.Accept(input("control-version-intake", "background"))
	w = startPermissionWork(t, s, w)
	in := controlInput(w, "supplement", "control-version-first")
	in.Text = "Add compatible analysis"
	first, _, err := s.ControlTask(in)
	if err != nil {
		t.Fatal(err)
	}
	stale := in
	stale.ClientNonce = "control-version-stale"
	stale.Text = "Offline old revision"
	before := s.Snapshot().Sequence
	if _, _, err = s.ControlTask(stale); !errors.Is(err, ErrConflict) || s.Snapshot().Sequence != before {
		t.Fatal("same runtime epoch bypassed task revision", err)
	}
	stale.ExpectedRevision = first.Work.TaskRevision
	if _, _, err = s.ControlTask(stale); err != nil {
		t.Fatal("current compatible supplement denied", err)
	}
	if err = s.AppendText(w.RequestID, w.Epoch, "active run unchanged"); err != nil {
		t.Fatal(err)
	}
}

func TestTaskControlsRevalidateAccountAudienceAndProject(t *testing.T) {
	t.Run("account", func(t *testing.T) {
		s, c, a, _, _ := accountFixture(t)
		w := acceptBound(t, s, a, "control-account-intake")
		result, _, err := s.ControlTask(controlInput(w, "pause", "control-account-pause"))
		if err != nil {
			t.Fatal(err)
		}
		if err = c.RemoveAccount(a.ID, a.Revision); err != nil {
			t.Fatal(err)
		}
		before := s.Snapshot().Sequence
		if _, _, err = s.ControlTask(controlInput(result.Work, "resume", "control-account-resume")); !errors.Is(err, ErrConflict) || s.Snapshot().Sequence != before {
			t.Fatal("removed account resumed", err)
		}
	})
	t.Run("audience", func(t *testing.T) {
		s, err := Open(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		channel, _, err := s.SaveChannel(ChannelInput{Name: "Task control", BotIDs: []string{"bot-meiiie", "bot-davin"}, ClientNonce: "control-channel-create"})
		if err != nil {
			t.Fatal(err)
		}
		w, _, err := s.AcceptProduct(input("control-channel-intake", "background"), SubmissionScope{ChannelID: channel.ID})
		if err != nil {
			t.Fatal(err)
		}
		result, _, err := s.ControlTask(controlInput(w, "pause", "control-channel-pause"))
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err = s.SaveChannel(ChannelInput{ID: channel.ID, Name: channel.Name, BotIDs: []string{"bot-meiiie"}, ExpectedRevision: channel.Revision, ClientNonce: "control-channel-change"}); err != nil {
			t.Fatal(err)
		}
		if _, _, err = s.ControlTask(controlInput(result.Work, "resume", "control-channel-resume")); !errors.Is(err, ErrConflict) {
			t.Fatal("changed audience resumed", err)
		}
	})
	t.Run("project", func(t *testing.T) {
		s, channel, project := productFixture(t)
		defer s.Close()
		w, _, err := s.AcceptProduct(input("control-project-intake", "background"), SubmissionScope{ChannelID: channel.ID, ProjectID: project.ID, ProjectRevision: project.Revision})
		if err != nil {
			t.Fatal(err)
		}
		result, _, err := s.ControlTask(controlInput(w, "pause", "control-project-pause"))
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err = s.SaveProject(ProjectInput{ID: project.ID, Name: project.Name, Goal: "New goal", ChannelIDs: project.ChannelIDs, ExpectedRevision: project.Revision, ClientNonce: "control-project-change"}); err != nil {
			t.Fatal(err)
		}
		if _, _, err = s.ControlTask(controlInput(result.Work, "resume", "control-project-resume")); !errors.Is(err, ErrConflict) {
			t.Fatal("changed project resumed", err)
		}
	})
}

func TestTaskControlsReplacementCanBeCancelledBeforeNativeStop(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	w, _, _ := s.Accept(input("control-replacement-cancel-intake", "background"))
	w = startPermissionWork(t, s, w)
	in := controlInput(w, "replace", "control-replacement-new")
	in.Text = "Replacement goal"
	result, _, err := s.ControlTask(in)
	if err != nil {
		t.Fatal(err)
	}
	cancelled, _, err := s.ControlTask(controlInput(*result.Related, "cancel", "control-replacement-cancel"))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.MarkRuntimeStopped(w.RequestID, w.AttemptID, StoppedReceipt{Version: 1, NativeProcessID: 401, SessionID: w.Core.SessionID}); err != nil {
		t.Fatal(err)
	}
	if err = s.Finish(w.RequestID, result.Work.Epoch, "cancelled", "outcome_not_rollback"); err != nil {
		t.Fatal(err)
	}
	if err = s.ReconcileTaskControls(); err != nil {
		t.Fatal(err)
	}
	current, _ := s.Get(cancelled.Work.RequestID)
	if current.Status != "cancelled" {
		t.Fatal("reconciler revived cancelled successor")
	}
}
