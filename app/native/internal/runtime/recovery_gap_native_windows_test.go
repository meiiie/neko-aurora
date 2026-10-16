//go:build windows && neko_native_fixture

package runtime

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"nekobot.local/native/internal/executors"
	"nekobot.local/native/internal/ledger"
)

type gapFixture struct {
	store   *ledger.Store
	manager *Manager
	model   *nativeFixture
	scratch string
}

func gapStart(t *testing.T) *gapFixture {
	t.Helper()
	scratch, e := os.MkdirTemp(os.Getenv("TEMP"), "core-ack-gap-")
	if e != nil {
		t.Fatal(e)
	}
	s, e := ledger.Open(filepath.Join(scratch, "state"))
	if e != nil {
		t.Fatal(e)
	}
	model := &nativeFixture{resume: true}
	listener, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	server := &http.Server{Handler: model}
	go server.Serve(listener)
	config := Config{HostExecutable: os.Getenv("NEKO_NATIVE_FIXTURE_HOST"), CoreExecutable: os.Getenv("NEKO_NATIVE_FIXTURE_CORE"), Supervisor: os.Getenv("NEKO_NATIVE_FIXTURE_SUPERVISOR"), FixtureEndpoint: "http://" + listener.Addr().String() + "/v1"}
	for _, p := range []string{config.HostExecutable, config.CoreExecutable, config.Supervisor} {
		if !filepath.IsAbs(p) {
			t.Fatal("absolute genuine fixture paths required")
		}
	}
	m, e := New(s, config, filepath.Join(scratch, "state"))
	if e != nil {
		t.Fatal(e)
	}
	if m.executor == nil || !m.executor.Snapshot().Ready {
		t.Fatal("real host readiness missing")
	}
	t.Cleanup(func() { server.Close(); m.executor.Close(); s.Close() })
	t.Logf("owned fixture scratch: %s", scratch)
	return &gapFixture{s, m, model, scratch}
}

func gapAccept(t *testing.T, g *gapFixture, nonce string) ledger.Work {
	t.Helper()
	w, _, e := g.store.Accept(ledger.Input{BotID: "bot-meiiie", ClientNonce: nonce, Text: "Owned exact native admission pipe probe " + nonce, Lane: "background"})
	if e != nil {
		t.Fatal(e)
	}
	return w
}

type gapPhase struct {
	claimed    ledger.Work
	binding    ledger.CoreBinding
	stopped    ledger.StoppedReceipt
	handle     nativeHandle
	generation string
	trace      []json.RawMessage
}

func gapBindingNegatives(t *testing.T, g *gapFixture, w ledger.Work, actual ledger.CoreBinding, previous *ledger.CoreBinding) {
	t.Helper()
	for _, kind := range []string{"manifest-hash", "task-root", "previous-binding"} {
		bad := actual
		if kind == "manifest-hash" {
			c := *bad.ContextAdmission
			c.ManifestSHA256 = strings.Repeat("0", 64)
			bad.ContextAdmission = &c
		}
		if kind == "task-root" {
			var task map[string]any
			json.Unmarshal(bad.Task, &task)
			task["root"] = "E:/outside-owned-task"
			bad.Task, _ = json.Marshal(task)
		}
		if kind == "previous-binding" {
			if previous == nil {
				continue
			}
			bad = *previous
		}
		before := g.store.Snapshot().Sequence
		if g.store.BindRuntimeAttempt(w.RequestID, w.AttemptID, bad) == nil || g.store.Snapshot().Sequence != before {
			t.Fatalf("%s binding admitted or mutated journal", kind)
		}
	}
}

func gapLaunch(t *testing.T, g *gapFixture, w ledger.Work, mode string, previous *ledger.CoreBinding) gapPhase {
	t.Helper()
	if e := g.manager.probeExecutor(); e != nil {
		t.Fatal(e)
	}
	claimed, e := g.store.Claim(w.RequestID, w.Epoch)
	if e != nil {
		t.Fatal(e)
	}
	spec, e := ledger.MarshalRuntimeLaunch(claimed, nil)
	if e != nil {
		t.Fatal(e)
	}
	cmd := exec.Command(g.manager.config.HostExecutable, "--runtime", g.manager.config.CoreExecutable, "--supervisor", g.manager.config.Supervisor, "--state-root", g.manager.tasks, "--fixture-endpoint", g.manager.config.FixtureEndpoint, "--install-state-root", g.manager.state)
	hideWindow(cmd)
	cmd.Env = hostEnvironment()
	input, e := cmd.StdinPipe()
	if e != nil {
		t.Fatal(e)
	}
	output, e := cmd.StdoutPipe()
	if e != nil {
		t.Fatal(e)
	}
	var diagnostic bytes.Buffer
	cmd.Stderr = &diagnostic
	if e = g.store.Dispatch(claimed.RequestID, claimed.Epoch, func() error {
		raw, err := os.ReadFile(g.manager.config.HostExecutable)
		if err != nil || hashBytes(raw) != claimed.ExecutorPin.HostSHA256 {
			return executors.ErrUnavailable
		}
		return cmd.Start()
	}); e != nil {
		t.Fatal(e)
	}
	encoder := json.NewEncoder(hostFrameWriter{input})
	if e = encoder.Encode(json.RawMessage(spec)); e != nil {
		t.Fatal(e)
	}
	phase := gapPhase{claimed: claimed}
	waited := false
	defer func() {
		input.Close()
		if !waited {
			cmd.Process.Kill()
			cmd.Wait()
			if phase.handle.h != 0 {
				syscall.TerminateProcess(phase.handle.h, 3)
				syscall.WaitForSingleObject(phase.handle.h, 5000)
			}
		}
	}()
	watchdog := time.AfterFunc(30*time.Second, func() { cmd.Process.Kill() })
	defer watchdog.Stop()
	scanner := bufio.NewScanner(output)
	scanner.Buffer(make([]byte, 4096), 65536)
	var sequence uint64
	for scanner.Scan() {
		raw := append([]byte(nil), scanner.Bytes()...)
		phase.trace = append(phase.trace, json.RawMessage(raw))
		var event struct {
			Sequence uint64          `json:"sequence"`
			Kind     string          `json:"kind"`
			Data     json.RawMessage `json:"data"`
		}
		if json.Unmarshal(raw, &event) != nil || event.Sequence != sequence+1 {
			t.Fatal("invalid real host sequence")
		}
		sequence = event.Sequence
		var result json.RawMessage = json.RawMessage(`{}`)
		switch event.Kind {
		case "launch_ready", "prompt_ready":
			proof, err := executors.ParseRuntimeProof(event.Data)
			if err != nil || !executors.MatchRuntimeProof(*claimed.CurrentAttemptLease, proof) || !g.store.RuntimeAllowed(claimed.RequestID, claimed.Epoch) {
				t.Fatal("real lease boundary mismatch", err)
			}
		case "admitted":
			if decodeStrictProofFrame(event.Data, &phase.binding) != nil {
				t.Fatal("real admission decode failed")
			}
			phase.handle = openNativeCore(t, phase.binding.NativeProcessID, g.manager.config.CoreExecutable)
			var c, exit, kernel, user syscall.Filetime
			if err := syscall.GetProcessTimes(phase.handle.h, &c, &exit, &kernel, &user); err != nil {
				t.Fatal(err)
			}
			phase.generation = strconv.FormatUint(uint64(c.HighDateTime)<<32|uint64(c.LowDateTime), 10)
			if phase.generation != phase.binding.NativeProcessGeneration {
				t.Fatal("real Child generation mismatch")
			}
			gapBindingNegatives(t, g, claimed, phase.binding, previous)
			if mode != "withhold-before-durable" {
				if e = g.store.BindRuntimeAttempt(claimed.RequestID, claimed.AttemptID, phase.binding); e != nil {
					t.Fatal(e)
				}
				if e = g.store.Admit(claimed.RequestID, claimed.Epoch, phase.binding); e != nil {
					t.Fatal(e)
				}
				if previous != nil && mode == "normal" {
					current, _ := g.store.Get(claimed.RequestID)
					mixed := *previous
					mixed.ContextAdmission = phase.binding.ContextAdmission
					nativeLateEffect(t, g.store, ledger.Work{RequestID: claimed.RequestID, Core: &mixed, ContextProtocolVersion: 1}, current)
					var oldTask map[string]any
					json.Unmarshal(previous.Task, &oldTask)
					before := g.store.Snapshot().Sequence
					if err := g.store.AppendText(claimed.RequestID, uint64(oldTask["activationEpoch"].(float64)), "old activation output before current prompt"); !errors.Is(err, ledger.ErrConflict) || g.store.Snapshot().Sequence != before {
						t.Fatal("previous output fence failed while current activation running", err)
					}
				}
			}
			if mode == "withhold-before-durable" || mode == "withhold-after-durable" {
				input.Close()
				continue
			}
		case "capabilities", "tool":
			result, e = g.store.ExecuteTool(claimed.RequestID, claimed.Epoch, event.Data)
			if e != nil {
				t.Fatal(e)
			}
		case "text":
			var v struct {
				Text string `json:"text"`
			}
			if json.Unmarshal(event.Data, &v) != nil {
				t.Fatal("real text invalid")
			}
			if e = g.store.AppendText(claimed.RequestID, claimed.Epoch, v.Text); e != nil {
				t.Fatal(e)
			}
			if mode == "stop-after-text" {
				input.Close()
				continue
			}
		case "completed":
			if e = g.store.Finish(claimed.RequestID, claimed.Epoch, "completed", ""); e != nil {
				t.Fatal(e)
			}
		case "runtime_stopped":
			if decodeStrictProofFrame(event.Data, &phase.stopped) != nil {
				t.Fatal("real stop decode failed")
			}
			phase.handle.assertStopped(t)
			if phase.stopped.NativeProcessGeneration != phase.generation || phase.stopped.NativeProcessID != phase.binding.NativeProcessID || phase.stopped.SessionID != phase.binding.SessionID || !executors.MatchRuntimeProof(*claimed.CurrentAttemptLease, *phase.stopped.ExecutorProof) {
				t.Fatal("real actualwait stop scope mismatch")
			}
			continue
		default:
			t.Fatal("unexpected real host event", event.Kind)
		}
		if e = encoder.Encode(map[string]any{"sequence": sequence, "ok": true, "result": result}); e != nil {
			t.Fatal(e)
		}
	}
	if scanner.Err() != nil {
		t.Fatal(scanner.Err())
	}
	waitErr := cmd.Wait()
	waited = true
	if phase.stopped.Version != 1 || phase.handle.h == 0 {
		t.Fatalf("missing genuine stop: %v %s", waitErr, diagnostic.String())
	}
	if strings.HasPrefix(mode, "withhold-") {
		if waitErr == nil || !strings.Contains(diagnostic.String(), "host_ack_missing") {
			t.Fatalf("ACK withholding not induced: %v %s", waitErr, diagnostic.String())
		}
		record, err := readNativeStop(g.manager.state, claimed.RequestID, claimed.AttemptID)
		if err != nil || record.RequestID != claimed.RequestID || record.AttemptID != claimed.AttemptID || !reflect.DeepEqual(record.Binding, phase.binding) || !reflect.DeepEqual(record.Receipt, phase.stopped) || !reflect.DeepEqual(record.InstallPin, *claimed.ExecutorPin) || !reflect.DeepEqual(record.AttemptLease, *claimed.CurrentAttemptLease) {
			t.Fatal("unacknowledged authentic stop fact lost or scope changed", err)
		}
	}
	raw, _ := json.MarshalIndent(map[string]any{"mode": mode, "claim": claimed, "binding": phase.binding, "actualStop": phase.stopped, "handleGeneration": phase.generation, "events": phase.trace, "hostDiagnostic": diagnostic.String(), "waitError": fmtError(waitErr)}, "", "  ")
	if e = os.WriteFile(filepath.Join(g.scratch, claimed.AttemptID+"-trace.json"), raw, 0600); e != nil {
		t.Fatal(e)
	}
	return phase
}

func fmtError(e error) string {
	if e == nil {
		return ""
	}
	return e.Error()
}

func TestNativeRecoveryGapAdmissionACKLoss(t *testing.T) {
	for _, mode := range []string{"withhold-before-durable", "withhold-after-durable"} {
		t.Run(mode, func(t *testing.T) {
			g := gapStart(t)
			w := gapAccept(t, g, "native-ack-loss-"+mode)
			phase := gapLaunch(t, g, w, mode, nil)
			before, _ := g.store.Get(w.RequestID)
			if (before.Core != nil) != (mode == "withhold-after-durable") || before.RuntimeStopped || g.model.calls.Load() != 0 {
				t.Fatal("ACK boundary state mismatch")
			}
			g.manager.executor.Close()
			g.store.Close()
			reopened, e := ledger.Open(g.manager.state)
			if e != nil {
				t.Fatal(e)
			}
			defer reopened.Close()
			next, e := New(reopened, g.manager.config, g.manager.state)
			if e != nil {
				t.Fatal(e)
			}
			defer next.executor.Close()
			after, e := reopened.Get(w.RequestID)
			if e != nil {
				t.Fatal(e)
			}
			proofSaved := mode == "withhold-after-durable"
			if after.RuntimeStopped != proofSaved || !reflect.DeepEqual(before.Core, after.Core) || !reflect.DeepEqual(before.Context, after.Context) || !reflect.DeepEqual(before.RuntimeContext, after.RuntimeContext) || before.RuntimeContextSHA256 != after.RuntimeContextSHA256 || before.SourceMessageID != after.SourceMessageID || before.ClientNonce != after.ClientNonce || !reflect.DeepEqual(before.RuntimeAttemptBinding, after.RuntimeAttemptBinding) || !reflect.DeepEqual(before.ExecutorPin, after.ExecutorPin) {
				t.Fatal("restart changed unacknowledged proof/context/intake")
			}
			seq := reopened.Snapshot().Sequence
			intent := ledger.TaskControlInput{Version: 2, Action: "recover", RequestID: w.RequestID, ExpectedEpoch: after.Epoch, ExpectedRevision: after.TaskRevision, ClientNonce: "explicit-native-recovery-" + mode}
			_, _, e = reopened.ControlTask(intent)
			if !proofSaved {
				if !errors.Is(e, ledger.ErrConflict) || reopened.Snapshot().Sequence != seq || after.Core != nil || after.RuntimeAttemptBinding != nil {
					t.Fatal("pre-bind stop fact promoted admission or recovery", e)
				}
			} else {
				if e != nil || after.Status != "suspended" || reopened.Snapshot().Sequence <= seq {
					t.Fatal("saved admission plus authentic stop failed explicit recovery", e)
				}
				if _, duplicate, err := reopened.ControlTask(intent); err != nil || !duplicate {
					t.Fatal("explicit recovery replay changed intent", err)
				}
			}
			if g.model.calls.Load() != 0 {
				t.Fatal("unacknowledged prompt executed automatically")
			}
			if phase.binding.ContextAdmission == nil || phase.generation == "" {
				t.Fatal("genuine context/process provenance omitted")
			}
			nativeReceipt(t, g.scratch, next, map[string]any{"mode": mode, "status": "PASS", "admittedAcknowledgement": "withheld on owned native pipe", "actualStopObservedByFixture": phase.stopped, "stopFactReconciledOnlyWithSavedAdmission": proofSaved, "before": before, "after": after, "automaticModelCalls": g.model.calls.Load(), "recoveryError": fmtError(e)})
		})
	}
}

func TestNativeRecoveryGapPriorReceiptRetryPlusTwo(t *testing.T) {
	g := gapStart(t)
	w := gapAccept(t, g, "native-prior-receipt-plus-two")
	first := gapLaunch(t, g, w, "stop-after-text", nil)
	if e := g.store.MarkRuntimeStopped(w.RequestID, first.claimed.AttemptID, first.stopped); e != nil {
		t.Fatal(e)
	}
	g.manager.failed(first.claimed, "fixture_transport_interrupted")
	original, _ := g.store.Get(w.RequestID)
	if original.Core == nil || original.RuntimeContext == nil {
		t.Fatal("original durable binding absent")
	}
	queued, e := g.store.Requeue(w.RequestID)
	if e != nil {
		t.Fatal(e)
	}
	second := gapLaunch(t, g, queued, "withhold-before-durable", original.Core)
	if e = g.store.MarkRuntimeStopped(w.RequestID, second.claimed.AttemptID, second.stopped); e != nil {
		t.Fatal(e)
	}
	g.manager.failed(second.claimed, "fixture_admitted_ack_withheld")
	missing, _ := g.store.Get(w.RequestID)
	if !reflect.DeepEqual(missing.Core, original.Core) || missing.RuntimeAttemptBinding != nil {
		t.Fatal("lost receipt replaced saved prior Core binding")
	}
	queued, e = g.store.Requeue(w.RequestID)
	if e != nil {
		t.Fatal(e)
	}
	third := gapLaunch(t, g, queued, "normal", original.Core)
	if e = g.store.MarkRuntimeStopped(w.RequestID, third.claimed.AttemptID, third.stopped); e != nil {
		t.Fatal(e)
	}
	final, e := g.store.Get(w.RequestID)
	if e != nil {
		t.Fatal(e)
	}
	var a, b, c map[string]any
	json.Unmarshal(first.binding.Task, &a)
	json.Unmarshal(second.binding.Task, &b)
	json.Unmarshal(third.binding.Task, &c)
	if a["activationEpoch"] != float64(1) || b["activationEpoch"] != float64(2) || c["activationEpoch"] != float64(3) || a["id"] != b["id"] || a["id"] != c["id"] || a["root"] != c["root"] || a["activationId"] == c["activationId"] {
		t.Fatal("actual activation +2 task proof missing")
	}
	if third.binding.TaskRecovery == nil || third.binding.TaskRecovery.Version != 1 || third.binding.TaskRecovery.Kind != "prior_receipt_retry" || third.binding.SessionID != first.binding.SessionID || third.binding.CoreSHA256 != first.binding.CoreSHA256 {
		t.Fatal("negotiated real +2/session proof missing")
	}
	if final.Status != "completed" || !reflect.DeepEqual(final.Context, original.Context) || !reflect.DeepEqual(final.RuntimeContext, original.RuntimeContext) || final.RuntimeContextSHA256 != original.RuntimeContextSHA256 || final.SourceMessageID != original.SourceMessageID || final.ClientNonce != original.ClientNonce || !reflect.DeepEqual(final.ExecutorPin, original.ExecutorPin) || !reflect.DeepEqual(third.binding.ContextAdmission.SourceIDs, first.binding.ContextAdmission.SourceIDs) || third.binding.ContextAdmission.ManifestSHA256 != first.binding.ContextAdmission.ManifestSHA256 || third.binding.ContextAdmission.ContextPinSHA256 != first.binding.ContextAdmission.ContextPinSHA256 {
		t.Fatal("actual +2 changed pinned context/session/install/intake")
	}
	if third.claimed.CurrentAttemptLease.ID == first.claimed.CurrentAttemptLease.ID || third.claimed.CurrentAttemptLease.Fence <= second.claimed.CurrentAttemptLease.Fence || len(final.Operations) != 0 || len(final.Artifacts) != 0 {
		t.Fatal("fresh attempt/effect fence missing")
	}
	before := g.store.Snapshot().Sequence
	if e = g.store.AppendText(w.RequestID, first.claimed.Epoch, "late previous activation output"); !errors.Is(e, ledger.ErrConflict) || g.store.Snapshot().Sequence != before {
		t.Fatal("old output not fenced", e)
	}
	nativeReceipt(t, g.scratch, g.manager, map[string]any{"mode": "actual-native-prior-receipt-retry-plus-two", "status": "PASS", "original": original, "unacknowledged": missing, "final": final, "activationEpochs": []any{a["activationEpoch"], b["activationEpoch"], c["activationEpoch"]}, "actualStopReceipts": []ledger.StoppedReceipt{first.stopped, second.stopped, third.stopped}, "nativeTaskRecovery": third.binding.TaskRecovery, "localModelCalls": g.model.calls.Load(), "freshMismatchedPreviousBindingNegative": true, "previousOperationAndOutputDeniedWhileCurrentActivationRunning": true})
}
