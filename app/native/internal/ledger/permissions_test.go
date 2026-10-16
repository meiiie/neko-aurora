package ledger

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func startPermissionWork(t *testing.T, s *Store, w Work) Work {
	t.Helper()
	w, err := s.Claim(w.RequestID, w.Epoch)
	if err != nil {
		t.Fatal(err)
	}
	b := CoreBinding{NativeProcessID: 401, SessionID: strings.Repeat("a", 32), Task: json.RawMessage(`{"id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","root":"/fixture","activationEpoch":1,"activationId":"cccccccccccccccccccccccccccccccc"}`), Execution: json.RawMessage(`{"version":1,"authorityId":"fixture-authority"}`), CoreSHA256: "4b3fb6202551912f86b5dd5faffccf75d854174c2ddd6b9fb331470a2b2e0b36"}
	if err = s.Admit(w.RequestID, w.Epoch, b); err != nil {
		t.Fatal(err)
	}
	w, _ = s.Get(w.RequestID)
	return w
}

func TestResumeRevokedBeforeAdmissionKeepsCurrentAttemptStopProof(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	source := completedPermissionSource(t, s, "resume-race-source", "fixture source")
	w, _, err := s.Accept(input("resume-race-consumer", "background"))
	if err != nil {
		t.Fatal(err)
	}
	w = startPermissionWork(t, s, w)
	if err = s.Finish(w.RequestID, w.Epoch, "recovery_required", "transport_interrupted"); err != nil {
		t.Fatal(err)
	}
	w, err = s.Requeue(w.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	w, err = s.Claim(w.RequestID, w.Epoch)
	if err != nil {
		t.Fatal(err)
	}
	b := *w.Core
	b.NativeProcessID = 402
	b.Task = json.RawMessage(`{"id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","root":"/fixture","activationEpoch":2,"activationId":"dddddddddddddddddddddddddddddddd"}`)
	if _, _, err = s.SetSourceAccess(AccessInput{source.RequestID, 0, false, "resume-race-revoke"}); err != nil {
		t.Fatal(err)
	}
	if err = s.MarkRuntimeStopped(w.RequestID, w.AttemptID, StoppedReceipt{1, b.NativeProcessID, b.SessionID}); !errors.Is(err, ErrConflict) {
		t.Fatal("unbound child must not prove saved Core stopped", err)
	}
	wrong := b
	wrong.SessionID = strings.Repeat("e", 32)
	if err = s.BindRuntimeAttempt(w.RequestID, w.AttemptID, wrong); !errors.Is(err, ErrConflict) {
		t.Fatal("unverified resume session", err)
	}
	wrong = b
	wrong.Execution = json.RawMessage(`{"authorityId":"different-fixture-authority"}`)
	if err = s.BindRuntimeAttempt(w.RequestID, w.AttemptID, wrong); !errors.Is(err, ErrConflict) {
		t.Fatal("unverified resume authority", err)
	}
	if err = s.BindRuntimeAttempt(w.RequestID, w.AttemptID, b); err != nil {
		t.Fatal(err)
	}
	if err = s.Admit(w.RequestID, w.Epoch, b); !errors.Is(err, ErrConflict) {
		t.Fatal("revoked admission", err)
	}
	if err = s.MarkRuntimeStopped(w.RequestID, w.AttemptID, StoppedReceipt{1, 401, b.SessionID}); !errors.Is(err, ErrConflict) {
		t.Fatal("old child stop receipt", err)
	}
	if err = s.MarkRuntimeStopped(w.RequestID, w.AttemptID, StoppedReceipt{1, 402, b.SessionID}); err != nil {
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
		current, _ := s.Get(w.RequestID)
		if current.Status != "suspended" || !current.RuntimeStopped || current.Core.NativeProcessID != 401 || current.RuntimeAttemptBinding.NativeProcessID != 402 {
			t.Fatal("attempt proof changed saved Core/lifecycle")
		}
		current.RuntimeAttemptBinding.Task[0] = 'x'
		current, _ = s.Get(w.RequestID)
		if !json.Valid(current.RuntimeAttemptBinding.Task) {
			t.Fatal("binding alias escaped")
		}
		if i == 2 {
			fresh, err := s.RefreshContext(RefreshInput{w.RequestID, current.Epoch})
			if err != nil || fresh.Core != nil || len(fresh.ContextAttempts) != 1 {
				t.Fatal("explicit refresh remains stuck after valid native proof", err)
			}
		}
		s.Close()
	}
}

func TestRefreshRejectsOversizeEncodedPromptWithoutChangingObligation(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	in := input("refresh-large-current", "foreground")
	in.Text = strings.Repeat("\x01", 9800) + "fixture request"
	w, _, err := s.Accept(in)
	if err != nil {
		t.Fatal(err)
	}
	completedPermissionSource(t, s, "refresh-large-source", strings.Repeat("q", 7200))
	if _, _, err = s.SetSourceAccess(AccessInput{w.RequestID, 0, false, "refresh-large-revoke"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.SetSourceAccess(AccessInput{w.RequestID, 1, true, "refresh-large-regrant"}); err != nil {
		t.Fatal(err)
	}
	before, _ := s.Get(w.RequestID)
	sequence := s.Snapshot().Sequence
	if _, err = s.RefreshContext(RefreshInput{w.RequestID, before.Epoch}); !errors.Is(err, ErrInvalid) {
		t.Fatal("oversize refresh accepted", err)
	}
	after, _ := s.Get(w.RequestID)
	oldBytes, _ := json.Marshal(before)
	newBytes, _ := json.Marshal(after)
	if !bytes.Equal(oldBytes, newBytes) || s.Snapshot().Sequence != sequence {
		t.Fatal("rejected refresh changed obligation/journal")
	}
}

func TestLegacyOversizeRefreshReplaysAndBlocksOnlyItsObligation(t *testing.T) {
	for _, mode := range []string{"unlaunched", "native-stopped"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			s, err := Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			in := input("legacy-refresh-large-current", "foreground")
			in.Text = strings.Repeat("\x01", 9800) + "fixture request"
			w, _, err := s.Accept(in)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "native-stopped" {
				w = startPermissionWork(t, s, w)
			}
			completedPermissionSource(t, s, "legacy-refresh-large-source", strings.Repeat("q", 7200))
			if _, _, err = s.SetSourceAccess(AccessInput{w.RequestID, 0, false, "legacy-refresh-large-revoke"}); err != nil {
				t.Fatal(err)
			}
			if mode == "native-stopped" {
				current, _ := s.Get(w.RequestID)
				if err = s.MarkRuntimeStopped(w.RequestID, w.AttemptID, StoppedReceipt{1, 401, w.Core.SessionID}); err != nil {
					t.Fatal(err)
				}
				if err = s.Finish(w.RequestID, current.Epoch, "suspended", "context_permission_changed"); err != nil {
					t.Fatal(err)
				}
			}
			if _, _, err = s.SetSourceAccess(AccessInput{w.RequestID, 1, true, "legacy-refresh-large-regrant"}); err != nil {
				t.Fatal(err)
			}
			old, _ := s.Get(w.RequestID)
			legacy := copyWork(old)
			legacy.ContextAttempts = append(legacy.ContextAttempts, contextArchive(old))
			legacy.Context = s.freshContext(old)
			legacy.ContextRevision++
			legacy.Epoch++
			legacy.Status = "waiting_host"
			legacy.DesiredState = "running"
			legacy.Core = nil
			legacy.Reply = ""
			legacy.Failure = ""
			if boundedRuntimePrompt(legacy) {
				t.Fatal("fixture is not oversized")
			}
			e := Event{Type: "work.context_refreshed", Work: legacy}
			if !validRuntimeReplay(s, e) {
				t.Fatal("historical unversioned refresh rejected")
			}
			e.ContextRefreshVersion = 1
			if validRuntimeReplay(s, e) {
				t.Fatal("new version accepted oversized refresh")
			}
			e.ContextRefreshVersion = 0
			if err = s.commit(e); err != nil {
				t.Fatal(err)
			}
			if s.RuntimeAllowed(w.RequestID, legacy.Epoch) {
				t.Fatal("legacy oversized prompt allowed to launch")
			}
			if _, err = s.Claim(w.RequestID, legacy.Epoch); !errors.Is(err, ErrInvalid) {
				t.Fatal("legacy oversized claim accepted", err)
			}
			s.Close()
			for i := 0; i < 3; i++ {
				s, err = Open(dir)
				if err != nil {
					t.Fatal("legacy journal became corrupt", err)
				}
				if err = s.RecoverInterrupted(); err != nil {
					t.Fatal(err)
				}
				current, _ := s.Get(w.RequestID)
				if current.Status != "suspended" || current.Failure != "context_prompt_oversized" || current.Epoch != legacy.Epoch+1 || current.AttemptID != legacy.AttemptID || current.RuntimeStopped != legacy.RuntimeStopped || current.Context.SHA256 != legacy.Context.SHA256 || len(current.ContextAttempts) != 1 {
					t.Fatal("legacy task-local fence changed accepted context")
				}
				if i == 0 {
					in := input("legacy-refresh-independent", "foreground")
					in.BotID = "bot-davin"
					if _, _, err = s.Accept(in); err != nil {
						t.Fatal("unrelated intake blocked", err)
					}
				}
				s.Close()
			}
		})
	}
}
func completedPermissionSource(t *testing.T, s *Store, nonce, reply string) Work {
	t.Helper()
	w, _, err := s.Accept(input(nonce, "foreground"))
	if err != nil {
		t.Fatal(err)
	}
	w = startPermissionWork(t, s, w)
	if err = s.AppendText(w.RequestID, w.Epoch, reply); err != nil {
		t.Fatal(err)
	}
	if err = s.Finish(w.RequestID, w.Epoch, "completed", ""); err != nil {
		t.Fatal(err)
	}
	w, _ = s.Get(w.RequestID)
	return w
}
func TestRevokedQueuedPinRequiresExplicitRefreshEvenAfterRegrant(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	source := completedPermissionSource(t, s, "permission-source-nonce", "fixture private source")
	queued, _, err := s.Accept(input("permission-queued-nonce", "background"))
	if err != nil || len(queued.Context.Sources) != 1 {
		t.Fatal(err)
	}
	pin, _ := json.Marshal(queued.Context)
	change := AccessInput{source.RequestID, 0, false, "permission-revoke-nonce"}
	if _, duplicate, err := s.SetSourceAccess(change); err != nil || duplicate {
		t.Fatal(err)
	}
	current, _ := s.Get(queued.RequestID)
	if current.Status != "suspended" || current.Epoch != queued.Epoch+1 || current.DesiredState != "suspended" {
		t.Fatal("queued obligation not suspended")
	}
	if _, err := s.Claim(current.RequestID, current.Epoch); !errors.Is(err, ErrConflict) {
		t.Fatal("revoked dispatch admitted", err)
	}
	if _, duplicate, err := s.SetSourceAccess(change); err != nil || !duplicate {
		t.Fatal("revoke not idempotent", err)
	}
	if _, _, err := s.SetSourceAccess(AccessInput{source.RequestID, 0, true, "permission-stale-nonce"}); !errors.Is(err, ErrConflict) {
		t.Fatal("stale revision accepted")
	}
	if _, _, err := s.SetSourceAccess(AccessInput{source.RequestID, 1, true, "permission-regrant-nonce"}); err != nil {
		t.Fatal(err)
	}
	if s.RuntimeAllowed(queued.RequestID, current.Epoch) {
		t.Fatal("regrant silently revived old pin")
	}
	after, _ := json.Marshal(current.Context)
	if !bytes.Equal(pin, after) {
		t.Fatal("revoke rewrote immutable accepted context")
	}
	fresh, err := s.RefreshContext(RefreshInput{queued.RequestID, current.Epoch})
	if err != nil || fresh.ContextRevision != 1 || fresh.Epoch != current.Epoch+1 || fresh.Status != "waiting_host" || len(fresh.Context.Sources) != 1 || fresh.Context.Sources[0].AccessRevision != 2 {
		t.Fatal("explicit refresh failed", err)
	}
	if fresh.ContextAttempts[0].PinSHA256 != queued.Context.SHA256 {
		t.Fatal("old pin provenance lost")
	}
	if _, err = s.Claim(fresh.RequestID, fresh.Epoch); err != nil {
		t.Fatal(err)
	}
}
func TestTransitiveRevokeFencesEffectsUntilNativeQuiescenceReceipt(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	source := completedPermissionSource(t, s, "permission-root-nonce", "fixture root private source")
	derived := completedPermissionSource(t, s, "permission-derived-nonce", "fixture derived private answer")
	active, _, err := s.Accept(input("permission-active-nonce", "background"))
	if err != nil {
		t.Fatal(err)
	}
	active = startPermissionWork(t, s, active)
	if err = s.AppendText(active.RequestID, active.Epoch, "fixture partial reply"); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.SetSourceAccess(AccessInput{source.RequestID, 0, false, "permission-root-revoke"}); err != nil {
		t.Fatal(err)
	}
	current, _ := s.Get(active.RequestID)
	if current.Status != "suspending" || current.Epoch != active.Epoch+1 || current.Reply != "fixture partial reply" || current.Core.SessionID != active.Core.SessionID {
		t.Fatal("active state or obligation lost")
	}
	if err = s.AppendText(active.RequestID, active.Epoch, "late old output"); !errors.Is(err, ErrConflict) {
		t.Fatal("late output admitted")
	}
	if err = s.AppendText(active.RequestID, current.Epoch, "late fenced output"); !errors.Is(err, ErrConflict) {
		t.Fatal("current epoch bypassed permission")
	}
	if _, err = s.ExecuteTool(active.RequestID, current.Epoch, invocation("forbidden effect")); !errors.Is(err, ErrConflict) {
		t.Fatal("late effect admitted")
	}
	if _, err = s.RefreshContext(RefreshInput{active.RequestID, current.Epoch}); !errors.Is(err, ErrConflict) {
		t.Fatal("refresh without stopped proof")
	}
	if err = s.MarkRuntimeStopped(active.RequestID, active.AttemptID, StoppedReceipt{1, 402, active.Core.SessionID}); !errors.Is(err, ErrConflict) {
		t.Fatal("other child receipt accepted")
	}
	if err = s.MarkRuntimeStopped(active.RequestID, active.AttemptID, StoppedReceipt{1, 401, active.Core.SessionID}); err != nil {
		t.Fatal(err)
	}
	if err = s.Finish(current.RequestID, current.Epoch, "suspended", "context_permission_changed"); err != nil {
		t.Fatal(err)
	}
	fresh, err := s.RefreshContext(RefreshInput{current.RequestID, current.Epoch})
	if err != nil || fresh.Core != nil || fresh.Reply != "" || len(fresh.Context.Sources) != 0 {
		t.Fatal("revoked source or derived answer imported on refresh", err)
	}
	if len(fresh.ContextAttempts) != 1 || fresh.ContextAttempts[0].Reply != "fixture partial reply" || fresh.ContextAttempts[0].Core.SessionID != active.Core.SessionID {
		t.Fatal("archive lost old obligation evidence")
	}
	if strings.Contains(fresh.RuntimePrompt(), "fixture partial reply") || strings.Contains(fresh.RuntimePrompt(), derived.Reply) {
		t.Fatal("archive leaked into fresh model context")
	}
	fresh.ContextAttempts[0].Core.Task[0] = 'x'
	stored, _ := s.Get(fresh.RequestID)
	if stored.ContextAttempts[0].Core.Task[0] != '{' {
		t.Fatal("caller mutated archive")
	}
	claimed, err := s.Claim(stored.RequestID, stored.Epoch)
	if err != nil || claimed.RuntimeStopped || claimed.RuntimeStopReceipt != nil {
		t.Fatal(err)
	}
	if err = s.MarkRuntimeStopped(stored.RequestID, active.AttemptID, StoppedReceipt{1, 401, active.Core.SessionID}); !errors.Is(err, ErrConflict) {
		t.Fatal("old attempt stop receipt accepted")
	}
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal("permission events failed replay", err)
	}
	defer s.Close()
	if err = s.RecoverInterrupted(); err != nil {
		t.Fatal(err)
	}
}
func TestCrashAfterAccessCommitStillFencesDispatchAndRecoversSuspension(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	source := completedPermissionSource(t, s, "permission-crash-source", "fixture crash source")
	queued, _, _ := s.Accept(input("permission-crash-queued", "background"))
	source.ContextAccess = &SourceAccess{Revision: 1, Granted: false, Nonce: "permission-crash-revoke"}
	if err := s.commit(Event{Type: "work.source_access", Work: source}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Claim(queued.RequestID, queued.Epoch); !errors.Is(err, ErrConflict) {
		t.Fatal("commit/fence crash window dispatched")
	}
	s.Close()
	for i := 0; i < 3; i++ {
		var err error
		s, err = Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		if err = s.RecoverInterrupted(); err != nil {
			t.Fatal(err)
		}
		current, _ := s.Get(queued.RequestID)
		if current.Status != "suspended" || current.Epoch != queued.Epoch+1 || current.Context.SHA256 != queued.Context.SHA256 {
			t.Fatal("restart changed fence or pin")
		}
		s.Close()
	}
}
func TestSuspendedUnresolvedEffectReconcilesWithoutAllowingRefresh(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	source := completedPermissionSource(t, s, "permission-effect-source", "fixture effect source")
	active, _, _ := s.Accept(input("permission-effect-active", "background"))
	active = startPermissionWork(t, s, active)
	hash := sha256.Sum256([]byte(active.RequestID + ":" + strings.Repeat("a", 32) + ":" + strings.Repeat("d", 32)))
	path := filepath.Join(dir, "artifacts", hex.EncodeToString(hash[:]))
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ExecuteTool(active.RequestID, active.Epoch, invocation("fixture unresolved effect")); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, _, err := s.SetSourceAccess(AccessInput{source.RequestID, 0, false, "permission-effect-revoke"}); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkRuntimeStopped(active.RequestID, active.AttemptID, StoppedReceipt{1, 401, active.Core.SessionID}); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.RecoverInterrupted(); err != nil {
		t.Fatal("task-local effect blocked permission recovery", err)
	}
	current, _ := s.Get(active.RequestID)
	if current.Status != "suspended" || current.Operations[0].Status != "unknown" || !current.RuntimeStopped {
		t.Fatal("wrong receipt/fence")
	}
	if _, err = s.RefreshContext(RefreshInput{active.RequestID, current.Epoch}); !errors.Is(err, ErrConflict) {
		t.Fatal("unresolved effect restarted in new Core")
	}
}

func TestLegacyV1PinFromUnresolvedCompletedSourceRemainsReadable(t *testing.T) {
	dir := t.TempDir()
	s, source := running(t, dir)
	hash := sha256.Sum256([]byte(source.RequestID + ":" + strings.Repeat("a", 32) + ":" + strings.Repeat("d", 32)))
	if err := os.MkdirAll(filepath.Join(dir, "artifacts", hex.EncodeToString(hash[:])), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ExecuteTool(source.RequestID, source.Epoch, invocation("fixture legacy source")); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	source, _ = s.Get(source.RequestID)
	source.Status = "completed"
	if err := s.commit(Event{Type: "work.completed", Work: source}); err != nil {
		t.Fatal(err)
	}
	legacy := Work{RequestID: "req-legacy-fixture", SourceMessageID: "msg-legacy-fixture", OwnerUserID: "local-owner", SpaceID: "local-space", BotID: "bot-meiiie", ConversationID: "conversation-bot-meiiie", ClientNonce: "legacy-context-v1-fixture", Text: "fixture request", Lane: "foreground", Status: "waiting_host", DesiredState: "running", Epoch: 1, BotConfigRevision: 1, CreatedAt: source.CreatedAt, CreatedSequence: uint64(len(s.events) + 1)}
	legacy.Context = s.captureContextVersion(legacy, 1)
	if len(legacy.Context.Sources) != 1 {
		t.Fatal("legacy selector changed")
	}
	if err := s.commit(Event{Type: "work.accepted", Work: legacy}); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err := Open(dir)
	if err != nil {
		t.Fatal("legacy pin became journal corruption", err)
	}
	defer s.Close()
	if err = s.RecoverInterrupted(); err != nil {
		t.Fatal(err)
	}
	if current, _ := s.Get(legacy.RequestID); current.Context.SHA256 != legacy.Context.SHA256 {
		t.Fatal("legacy accepted pin rewritten")
	}
	newer, _, err := s.Accept(input("fresh-context-v2-fixture", "foreground"))
	if err != nil || newer.Context.Version != 2 || len(newer.Context.Sources) != 0 {
		t.Fatal("new pin reused unresolved legacy source", err)
	}
}
