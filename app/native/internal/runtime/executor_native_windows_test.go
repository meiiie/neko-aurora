//go:build windows && neko_native_fixture

package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"nekobot.local/native/internal/executors"
	"nekobot.local/native/internal/ledger"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

type nativeFixture struct {
	calls  atomic.Int32
	resume bool
}

func (f *nativeFixture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer fixture-only" {
		http.Error(w, "fixture boundary", 403)
		return
	}
	var body map[string]any
	if json.NewDecoder(io.LimitReader(r.Body, 2_000_001)).Decode(&body) != nil {
		http.Error(w, "invalid fixture", 400)
		return
	}
	n := f.calls.Add(1)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Connection", "close")
	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}
	chunk := func(delta map[string]any, finish any) {
		raw, _ := json.Marshal(map[string]any{"id": "executor-native-fixture", "object": "chat.completion.chunk", "created": 1, "model": "neko-controlled-fixture", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}})
		fmt.Fprintf(w, "data: %s\n\n", raw)
		flusher.Flush()
	}
	chunk(map[string]any{"role": "assistant", "content": "Native lease fixture response. "}, nil)
	if f.resume && n > 1 {
		chunk(map[string]any{"content": "Resumed with a fresh lease."}, nil)
		chunk(map[string]any{}, "stop")
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
		return
	}
	<-r.Context().Done()
}

type nativeHandle struct{ h syscall.Handle }

func openNativeCore(t *testing.T, pid uint32, path string) nativeHandle {
	t.Helper()
	h, e := syscall.OpenProcess(0x101001, false, pid)
	if e != nil {
		t.Fatal(e)
	}
	name := make([]uint16, 32768)
	size := uint32(len(name))
	proc := syscall.NewLazyDLL("kernel32.dll").NewProc("QueryFullProcessImageNameW")
	ok, _, err := proc.Call(uintptr(h), 0, uintptr(unsafe.Pointer(&name[0])), uintptr(unsafe.Pointer(&size)))
	if ok == 0 {
		syscall.CloseHandle(h)
		t.Fatal(err)
	}
	actual := strings.TrimPrefix(syscall.UTF16ToString(name[:size]), `\\?\`)
	expected, e := filepath.EvalSymlinks(path)
	if e != nil || !strings.EqualFold(filepath.Clean(actual), filepath.Clean(expected)) {
		syscall.CloseHandle(h)
		t.Fatalf("owned Core image mismatch: %s", actual)
	}
	event, e := syscall.WaitForSingleObject(h, 0)
	if e != nil || event != 258 {
		syscall.CloseHandle(h)
		t.Fatalf("Core was not live: %d %v", event, e)
	}
	t.Cleanup(func() { syscall.CloseHandle(h) })
	return nativeHandle{h}
}
func (h nativeHandle) assertStopped(t *testing.T) {
	t.Helper()
	event, e := syscall.WaitForSingleObject(h.h, 0)
	if e != nil || event != 0 {
		t.Fatalf("receipt preceded OS process exit: %d %v", event, e)
	}
}
func nativeWait(t *testing.T, s *ledger.Store, id string, ready func(ledger.Work) bool) ledger.Work {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		w, e := s.Get(id)
		if e != nil {
			t.Fatal(e)
		}
		if ready(w) {
			return w
		}
		time.Sleep(20 * time.Millisecond)
	}
	w, _ := s.Get(id)
	t.Fatalf("native work timeout: status=%s failure=%s core=%v stopped=%v", w.Status, w.Failure, w.Core != nil, w.RuntimeStopped)
	return w
}
func nativeStart(t *testing.T, resume bool) (*ledger.Store, *Manager, *nativeFixture, func(), string) {
	t.Helper()
	host, core, supervisor := os.Getenv("NEKO_NATIVE_FIXTURE_HOST"), os.Getenv("NEKO_NATIVE_FIXTURE_CORE"), os.Getenv("NEKO_NATIVE_FIXTURE_SUPERVISOR")
	if host == "" || core == "" || supervisor == "" {
		t.Skip("explicit owned native fixture paths required")
	}
	for _, p := range []string{host, core, supervisor} {
		if !filepath.IsAbs(p) {
			t.Fatal("absolute fixture paths required")
		}
	}
	scratch, e := os.MkdirTemp(os.Getenv("TEMP"), "native-executor-s3-")
	if e != nil {
		t.Fatal(e)
	}
	t.Logf("retained native scratch: %s", scratch)
	s, e := ledger.Open(filepath.Join(scratch, "state"))
	if e != nil {
		t.Fatal(e)
	}
	fixture := &nativeFixture{resume: resume}
	listener, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		s.Close()
		t.Fatal(e)
	}
	server := &http.Server{Handler: fixture}
	go server.Serve(listener)
	m, e := New(s, Config{HostExecutable: host, CoreExecutable: core, Supervisor: supervisor, FixtureEndpoint: "http://" + listener.Addr().String() + "/v1"}, filepath.Join(scratch, "state"))
	if e != nil {
		s.Close()
		server.Close()
		t.Fatal(e)
	}
	if m.executor == nil || !m.executor.Snapshot().Ready {
		s.Close()
		server.Close()
		if m.executor != nil {
			m.executor.Close()
		}
		t.Fatal("actual release host did not prove readiness")
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { m.Run(ctx); close(done) }()
	var stopped atomic.Bool
	stop := func() {
		if stopped.Swap(true) {
			return
		}
		cancel()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("native manager stop timeout")
		}
		server.Close()
	}
	t.Cleanup(func() { stop(); s.Close() })
	return s, m, fixture, stop, scratch
}
func nativeReplay(t *testing.T, s *ledger.Store, stop func(), scratch string, w ledger.Work) {
	t.Helper()
	stop()
	if e := s.Close(); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 3; i++ {
		reopened, e := ledger.Open(filepath.Join(scratch, "state"))
		if e != nil {
			t.Fatal(e)
		}
		if e = reopened.RecoverInterrupted(); e != nil {
			reopened.Close()
			t.Fatal(e)
		}
		got, e := reopened.Get(w.RequestID)
		reopened.Close()
		if e != nil || !reflect.DeepEqual(got, w) {
			t.Fatalf("typed replay changed stopped native obligation on restart%d: %v", i, e)
		}
	}
}

// These disk reads produce post-stop fixture evidence only; live authority
// always remains in the registry guards, never an unlocked file read.
func nativeRetirement(t *testing.T, scratch string, lease *executors.AttemptLease) string {
	t.Helper()
	raw, e := os.ReadFile(filepath.Join(scratch, "state", "executors", "executor-v1.json"))
	if e != nil {
		t.Fatal(e)
	}
	var state struct {
		Leases map[string]struct {
			Lease   executors.AttemptLease `json:"lease"`
			Retired string                 `json:"retired"`
		} `json:"leases"`
	}
	if json.Unmarshal(raw, &state) != nil {
		t.Fatal("invalid post-stop registry evidence")
	}
	saved, found := state.Leases[lease.RequestID+":"+lease.AttemptID]
	if !found || saved.Lease != *lease {
		t.Fatal("post-stop lease evidence missing")
	}
	return saved.Retired
}
func nativeReceipt(t *testing.T, scratch string, m *Manager, record map[string]any) {
	t.Helper()
	record["codeCommit"] = os.Getenv("NEKO_NATIVE_FIXTURE_CODE_COMMIT")
	record["hostSourceCommit"] = os.Getenv("NEKO_NATIVE_FIXTURE_HOST_SOURCE_COMMIT")
	record["scope"] = "Actual local Go/Rust/native Core with owned loopback model fixture; controlled registry clock only when declared, no GUI/external provider/pairing/cloud."
	for name, path := range map[string]string{"hostSha256": m.config.HostExecutable, "coreSha256": m.config.CoreExecutable, "supervisorSha256": m.config.Supervisor} {
		b, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		h := sha256.Sum256(b)
		record[name] = hex.EncodeToString(h[:])
	}
	b, e := json.MarshalIndent(record, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(scratch, "native-receipt.json")
	if e = os.WriteFile(path, b, 0600); e != nil {
		t.Fatal(e)
	}
	t.Logf("native receipt: %s", path)
}
func nativeLateEffect(t *testing.T, s *ledger.Store, live, settled ledger.Work) {
	t.Helper()
	var task struct {
		ID              string `json:"id"`
		Root            string `json:"root"`
		ActivationEpoch uint64 `json:"activationEpoch"`
		ActivationID    string `json:"activationId"`
	}
	if json.Unmarshal(live.Core.Task, &task) != nil {
		t.Fatal("missing actual Core task metadata")
	}
	meta := map[string]any{"neko.operation": map[string]any{"version": 1, "operationId": strings.Repeat("f", 32), "sessionId": live.Core.SessionID, "taskId": task.ID, "root": task.Root, "activationEpoch": task.ActivationEpoch, "activationId": task.ActivationID}}
	if live.ContextProtocolVersion == 1 {
		meta["neko.contextAdmission"] = live.Core.ContextAdmission
	}
	frame, e := json.Marshal(map[string]any{"name": "execute_tool", "arguments": map[string]any{"name": "artifact.write_text", "arguments": map[string]any{"filename": "late-effect.md", "text": "Owned fixture must remain unwritten."}}, "_meta": meta})
	if e != nil {
		t.Fatal(e)
	}
	before := s.Snapshot().Sequence
	if _, e = s.ExecuteTool(live.RequestID, settled.Epoch, frame); !errors.Is(e, ledger.ErrConflict) {
		t.Fatal("late actual-task effect not fenced", e)
	}
	got, e := s.Get(live.RequestID)
	if e != nil || len(got.Operations) != 0 || len(got.Artifacts) != 0 || got.Reply != settled.Reply || s.Snapshot().Sequence != before {
		t.Fatal("denied late effect changed journal/output")
	}
}
func TestNativeExecutorLeaseLossStopsActualCore(t *testing.T) {
	for _, mode := range []string{"expiry", "revoke"} {
		t.Run(mode, func(t *testing.T) {
			s, m, fixture, stop, scratch := nativeStart(t, false)
			var offset atomic.Int64
			heartbeats := 0
			if mode == "expiry" {
				if e := executors.FixtureSetClock(m.executor, func() time.Time { return time.Now().Add(time.Duration(offset.Load())) }); e != nil {
					t.Fatal(e)
				}
			}
			accepted, _, e := s.Accept(ledger.Input{BotID: "bot-meiiie", ClientNonce: "native-executor-" + mode, Text: "Keep this owned local fixture alive.", Lane: "background"})
			if e != nil {
				t.Fatal(e)
			}
			live := nativeWait(t, s, accepted.RequestID, func(w ledger.Work) bool { return w.Status == "running" && w.Core != nil && w.Reply != "" })
			if live.ContextProtocolVersion == 1 && (live.Core.ContextAdmission == nil || live.Core.ContextAdmission.ManifestSHA256 != live.RuntimeContextSHA256 || live.Core.ContextAdmission.ContextPinSHA256 != live.Context.SHA256 || len(live.Core.ContextAdmission.SourceIDs) != 1) {
				t.Fatal("actual Core context receipt missing")
			}
			if live.CurrentAttemptLease == nil || live.Core.ExecutorProof == nil || live.Core.CoreSHA256 != live.ExecutorPin.CoreSHA256 {
				t.Fatal("actual Core companion not admitted")
			}
			handle := openNativeCore(t, live.Core.NativeProcessID, m.config.CoreExecutable)
			if _, e = s.Requeue(live.RequestID); !errors.Is(e, ledger.ErrConflict) {
				t.Fatal("unproved active resume accepted", e)
			}
			if mode == "expiry" {
				// Keep the 15s heartbeat live while accelerated clock time passes
				// the fixed 10min lease; these heartbeats never renew that lease.
				for elapsed := 5 * time.Second; elapsed <= 605*time.Second; elapsed += 5 * time.Second {
					offset.Store(int64(elapsed))
					if e = m.executor.Heartbeat(*live.ExecutorPin, live.CurrentAttemptLease.BootID); e != nil {
						t.Fatal(e)
					}
					heartbeats++
				}
				if e = m.executor.WithLease(*live.ExecutorPin, *live.CurrentAttemptLease, func() error { return nil }); !errors.Is(e, executors.ErrExpired) {
					t.Fatal("controlled expiry did not retire current lease", e)
				}
			} else {
				if _, _, e = m.executor.Revoke(*live.ExecutorPin, "native-executor-revoke"); e != nil {
					t.Fatal(e)
				}
			}
			settled := nativeWait(t, s, live.RequestID, func(w ledger.Work) bool { return w.Status == "suspended" && w.RuntimeStopped })
			handle.assertStopped(t)
			if settled.Epoch != live.Epoch+1 || settled.DesiredState != "suspended" || settled.Failure != "executor_lease_unavailable" || settled.RuntimeStopReceipt == nil || settled.RuntimeStopReceipt.Version != 1 || settled.RuntimeStopReceipt.NativeProcessID != live.Core.NativeProcessID || settled.RuntimeStopReceipt.SessionID != live.Core.SessionID {
				t.Fatal("native lease stop receipt lost current attempt")
			}
			if !reflect.DeepEqual(settled.Core, live.Core) || !reflect.DeepEqual(settled.Context, live.Context) || !reflect.DeepEqual(settled.CurrentAttemptLease, live.CurrentAttemptLease) || !reflect.DeepEqual(settled.ExecutorPin, live.ExecutorPin) || len(settled.Operations) != 0 {
				t.Fatal("native suspension changed immutable authority")
			}
			proof := *settled.RuntimeStopReceipt.ExecutorProof
			if !executors.MatchRuntimeProof(*live.CurrentAttemptLease, proof) || proof.WorkEpoch != live.Epoch || proof.WorkEpoch == settled.Epoch {
				t.Fatal("stop did not preserve original saved lease epoch")
			}
			bad := *settled.RuntimeStopReceipt
			bad.NativeProcessID++
			if e = s.MarkRuntimeStopped(live.RequestID, live.AttemptID, bad); !errors.Is(e, ledger.ErrConflict) {
				t.Fatal("wrong PID receipt admitted", e)
			}
			bad = *settled.RuntimeStopReceipt
			badProof := *bad.ExecutorProof
			badProof.WorkEpoch = settled.Epoch
			bad.ExecutorProof = &badProof
			if e = s.MarkRuntimeStopped(live.RequestID, live.AttemptID, bad); !errors.Is(e, ledger.ErrConflict) {
				t.Fatal("future work epoch substituted into receipt", e)
			}
			if e = s.MarkRuntimeStopped(live.RequestID, "attempt-"+strings.Repeat("0", 32), *settled.RuntimeStopReceipt); !errors.Is(e, ledger.ErrConflict) {
				t.Fatal("old attempt receipt admitted", e)
			}
			if e = s.AppendText(live.RequestID, live.Epoch, "late original output"); !errors.Is(e, ledger.ErrConflict) {
				t.Fatal("late original output accepted", e)
			}
			if e = s.AppendText(live.RequestID, settled.Epoch, "late current epoch output"); !errors.Is(e, ledger.ErrConflict) {
				t.Fatal("lease fence bypassed by current epoch", e)
			}
			if e = s.Finish(live.RequestID, settled.Epoch, "completed", ""); !errors.Is(e, ledger.ErrConflict) {
				t.Fatal("expired completion accepted", e)
			}
			if _, e = s.Requeue(live.RequestID); !errors.Is(e, ledger.ErrConflict) {
				t.Fatal("suspended authority silently resumed", e)
			}
			if fixture.calls.Load() != 1 {
				t.Fatalf("unexpected extra local model prompt after authority loss: %d", fixture.calls.Load())
			}
			nativeLateEffect(t, s, live, settled)
			retirement := nativeRetirement(t, scratch, live.CurrentAttemptLease)
			if (mode == "expiry" && retirement != "expired") || (mode == "revoke" && retirement != "revoked") {
				t.Fatal("unexpected actual lease retirement", retirement)
			}
			t.Logf("actual %s native PID=%d originalEpoch=%d fencedEpoch=%d stop receipt matches; controlled clock=%v", mode, live.Core.NativeProcessID, live.Epoch, settled.Epoch, mode == "expiry")
			nativeReplay(t, s, stop, scratch, settled)
			nativeReceipt(t, scratch, m, map[string]any{"status": "PASS", "case": mode, "controlledRegistryClock": mode == "expiry", "clockOffsetNs": offset.Load(), "heartbeatSteps": heartbeats, "retirement": retirement, "initial": live, "final": settled, "actualOSExitBeforeReceipt": true, "negativePIDAttemptEpochReceipts": "PASS", "lateOutputCompletionEffect": "PASS", "restartCount": 3, "localModelCalls": fixture.calls.Load()})
		})
	}
}
func TestNativeExecutorExplicitRecoveryUsesFreshLeaseSameCore(t *testing.T) {
	s, m, fixture, stop, scratch := nativeStart(t, true)
	accepted, _, e := s.Accept(ledger.Input{BotID: "bot-meiiie", ClientNonce: "native-executor-recovery", Text: "Recover the same owned local fixture task after forced process exit.", Lane: "background"})
	if e != nil {
		t.Fatal(e)
	}
	live := nativeWait(t, s, accepted.RequestID, func(w ledger.Work) bool { return w.Status == "running" && w.Core != nil && w.Reply != "" })
	handle := openNativeCore(t, live.Core.NativeProcessID, m.config.CoreExecutable)
	if _, e = s.Requeue(live.RequestID); !errors.Is(e, ledger.ErrConflict) {
		t.Fatal("resume without process stop accepted", e)
	}
	if e = syscall.TerminateProcess(handle.h, 71); e != nil {
		t.Fatal(e)
	}
	prior := nativeWait(t, s, live.RequestID, func(w ledger.Work) bool { return w.Status == "recovery_required" && w.RuntimeStopped })
	handle.assertStopped(t)
	if prior.RuntimeStopReceipt == nil || !executors.MatchRuntimeProof(*live.CurrentAttemptLease, *prior.RuntimeStopReceipt.ExecutorProof) {
		t.Fatal("actual interrupted stop proof absent")
	}
	fresh, e := s.Requeue(live.RequestID)
	if e != nil {
		t.Fatal(e)
	}
	if fresh.Status != "waiting_host" || !reflect.DeepEqual(fresh.Context, live.Context) || !reflect.DeepEqual(fresh.Core, live.Core) || !reflect.DeepEqual(fresh.ExecutorPin, live.ExecutorPin) {
		t.Fatal("requeue silently rebound task/context/install")
	}
	done := nativeWait(t, s, live.RequestID, func(w ledger.Work) bool { return w.Status == "completed" && w.RuntimeStopped })
	if done.CurrentAttemptLease.ID == live.CurrentAttemptLease.ID || done.AttemptID == live.AttemptID || done.CurrentAttemptLease.Fence <= live.CurrentAttemptLease.Fence || done.CurrentAttemptLease.WorkEpoch != done.Epoch || done.Core.NativeProcessID == live.Core.NativeProcessID || done.Core.SessionID != live.Core.SessionID || !reflect.DeepEqual(done.ExecutorPin, live.ExecutorPin) || !reflect.DeepEqual(done.Context, live.Context) || done.Core.CoreSHA256 != live.Core.CoreSHA256 {
		t.Fatal("native recovery did not keep authority while renewing attempt")
	}
	var oldTask, newTask struct {
		ID              string `json:"id"`
		ActivationEpoch uint64 `json:"activationEpoch"`
	}
	json.Unmarshal(live.Core.Task, &oldTask)
	json.Unmarshal(done.Core.Task, &newTask)
	if oldTask.ID != newTask.ID || newTask.ActivationEpoch != oldTask.ActivationEpoch+1 {
		t.Fatal("native Core semantic task continuity not proved")
	}
	if live.ContextProtocolVersion == 1 && (live.Core.ContextAdmission == nil || done.Core.ContextAdmission == nil || !reflect.DeepEqual(live.Core.ContextAdmission.SourceIDs, done.Core.ContextAdmission.SourceIDs) || live.Core.ContextAdmission.ManifestSHA256 != done.Core.ContextAdmission.ManifestSHA256 || !reflect.DeepEqual(live.RuntimeContext, done.RuntimeContext)) {
		t.Fatal("native context provenance changed on resume")
	}
	if e = s.MarkRuntimeStopped(done.RequestID, live.AttemptID, *prior.RuntimeStopReceipt); !errors.Is(e, ledger.ErrConflict) {
		t.Fatal("old receipt settled new activation", e)
	}
	if e = s.AppendText(done.RequestID, live.Epoch, "old late output"); !errors.Is(e, ledger.ErrConflict) {
		t.Fatal("old attempt output accepted", e)
	}
	if fixture.calls.Load() != 2 {
		t.Fatalf("unexpected local fixture calls: %d", fixture.calls.Load())
	}
	t.Logf("actual recovery native oldPID=%d newPID=%d oldActivation=%d newActivation=%d newFence=%d", live.Core.NativeProcessID, done.Core.NativeProcessID, oldTask.ActivationEpoch, newTask.ActivationEpoch, done.CurrentAttemptLease.Fence)
	nativeReplay(t, s, stop, scratch, done)
	nativeReceipt(t, scratch, m, map[string]any{"status": "PASS", "case": "normal-clock-explicit-recovery", "controlledRegistryClock": false, "initial": live, "interrupted": prior, "final": done, "actualOSExitBeforeReceipt": true, "oldReceiptAndLateOutputDenied": "PASS", "sameCoreTaskAndSession": true, "oldActivation": oldTask.ActivationEpoch, "newActivation": newTask.ActivationEpoch, "restartCount": 3, "localModelCalls": fixture.calls.Load()})
}
