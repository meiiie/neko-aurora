package ledger

import (
	"encoding/json"
	"errors"
	"nekoaurora.local/native/internal/executors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func serviceRecoveryFixture(t *testing.T) (*Store, *executors.Registry, Work, CoreBinding) {
	t.Helper()
	s, e := Open(filepath.Join(t.TempDir(), "journal"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	r, e := executors.Open(filepath.Join(t.TempDir(), "executors"), executors.Descriptor{HostSHA256: strings.Repeat("1", 64), CoreSHA256: "97ca4c8acff2e0bb6dc0b9577d856b3601d4c6c914579a9c9a1fe3cbb67c47fb", RootSHA256: strings.Repeat("3", 64), CapabilitiesSHA256: strings.Repeat("4", 64)})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { r.Close() })
	snap := r.Snapshot()
	if e = r.MarkReady(snap.Pin, snap.BootID); e != nil {
		t.Fatal(e)
	}
	s.ConfigureExecutor(r)
	s.ConfigureContextAdmission(true)
	w, _, e := s.Accept(Input{"bot-meiiie", "service-crash-intake", "Keep the original context", "background"})
	if e != nil {
		t.Fatal(e)
	}
	w, e = s.Claim(w.RequestID, w.Epoch)
	if e != nil {
		t.Fatal(e)
	}
	b := executorBinding(w, 401)
	b.ContextAdmission = &ContextAdmission{Version: 1, SessionID: b.SessionID, ContextPinSHA256: contextHash(w), ManifestSHA256: w.RuntimeContextSHA256, SourceIDs: []string{strings.Repeat("d", 64)}, Task: b.Task, Execution: b.Execution}
	raw, _ := json.Marshal(b)
	var fields map[string]any
	json.Unmarshal(raw, &fields)
	fields["nativeProcessGeneration"] = "134362178434013773"
	raw, _ = json.Marshal(fields)
	json.Unmarshal(raw, &b)
	if e = s.BindRuntimeAttempt(w.RequestID, w.AttemptID, b); e != nil {
		t.Fatal(e)
	}
	if e = s.Admit(w.RequestID, w.Epoch, b); e != nil {
		t.Fatal(e)
	}
	if e = s.AppendText(w.RequestID, w.Epoch, "Saved output"); e != nil {
		t.Fatal(e)
	}
	if e = r.MarkUnavailable(snap.Pin, snap.BootID, "host_disconnected"); e != nil {
		t.Fatal(e)
	}
	if e = s.ReconcileExecutors(); e != nil {
		t.Fatal(e)
	}
	return s, r, w, b
}
func serviceStopped(t *testing.T, s *Store, w Work, b CoreBinding) Work {
	t.Helper()
	raw, _ := json.Marshal(StoppedReceipt{Version: 1, NativeProcessID: b.NativeProcessID, SessionID: b.SessionID, ExecutorProof: b.ExecutorProof})
	var fields map[string]any
	json.Unmarshal(raw, &fields)
	fields["nativeProcessGeneration"] = "134362178434013773"
	raw, _ = json.Marshal(fields)
	var receipt StoppedReceipt
	json.Unmarshal(raw, &receipt)
	if e := s.MarkRuntimeStopped(w.RequestID, w.AttemptID, receipt); e != nil {
		t.Fatal(e)
	}
	if e := s.ReconcileExecutors(); e != nil {
		t.Fatal(e)
	}
	got, _ := s.Get(w.RequestID)
	return got
}
func TestServiceRecoveryExplicitVersionedIntentPreservesContextAndGetsNewLease(t *testing.T) {
	s, r, w, b := serviceRecoveryFixture(t)
	old := serviceStopped(t, s, w, b)
	p := r.Snapshot()
	if e := r.MarkReady(p.Pin, p.BootID); e != nil {
		t.Fatal(e)
	}
	in := TaskControlInput{Version: 2, Action: "recover", RequestID: w.RequestID, ExpectedEpoch: old.Epoch, ExpectedRevision: old.TaskRevision, ClientNonce: "service-recover-owner-001"}
	result, dup, e := s.ControlTask(in)
	if e != nil || dup {
		t.Fatal("explicit service recovery denied", e)
	}
	if result.Work.Status != "waiting_host" || result.Work.DesiredState != "running" || !reflect.DeepEqual(result.Work.Core, old.Core) || !reflect.DeepEqual(result.Work.Context, old.Context) || result.Work.Reply != old.Reply {
		t.Fatal("recovery rewrote prior authority")
	}
	again, dup, e := s.ControlTask(in)
	if e != nil || !dup || !reflect.DeepEqual(again, result) {
		t.Fatal("lost ACK retry", e)
	}
	claimed, e := s.Claim(w.RequestID, result.Work.Epoch)
	if e != nil {
		t.Fatal(e)
	}
	if claimed.AttemptID == old.AttemptID || claimed.CurrentAttemptLease.ID == old.CurrentAttemptLease.ID || claimed.CurrentAttemptLease.Fence <= old.CurrentAttemptLease.Fence || !reflect.DeepEqual(claimed.ExecutorPin, old.ExecutorPin) || claimed.RuntimeContextSHA256 != old.RuntimeContextSHA256 {
		t.Fatal("reused lease or changed install/context")
	}
	dir := s.dir
	s.Close()
	for i := 0; i < 3; i++ {
		re, e := Open(dir)
		if e != nil {
			t.Fatal(e)
		}
		got, _ := re.Get(w.RequestID)
		if !reflect.DeepEqual(got, claimed) {
			t.Fatal("replay drift")
		}
		re.Close()
	}
}
func TestServiceRecoveryMissingStopCancelAndStaleIntentDeny(t *testing.T) {
	for _, kind := range []string{"missing", "cancel", "epoch", "revision", "generation", "source-revoke", "install-revoke", "unknown-effect", "legacy-context"} {
		t.Run(kind, func(t *testing.T) {
			s, r, w, b := serviceRecoveryFixture(t)
			old, _ := s.Get(w.RequestID)
			if kind != "missing" {
				old = serviceStopped(t, s, w, b)
			}
			switch kind {
			case "cancel":
				s.Cancel(w.RequestID)
				old, _ = s.Get(w.RequestID)
			case "generation":
				s.mu.Lock()
				x := s.work[w.RequestID]
				raw, _ := json.Marshal(x.RuntimeAttemptBinding)
				var data map[string]any
				json.Unmarshal(raw, &data)
				delete(data, "nativeProcessGeneration")
				raw, _ = json.Marshal(data)
				var clean CoreBinding
				json.Unmarshal(raw, &clean)
				x.RuntimeAttemptBinding = &clean
				s.work[w.RequestID] = x
				s.mu.Unlock()
			case "source-revoke":
				_, _, e := s.SetSourceAccess(AccessInput{RequestID: w.RequestID, ExpectedRevision: 0, Granted: false, ClientNonce: "service-source-revoke"})
				if e != nil {
					t.Fatal(e)
				}
				old, _ = s.Get(w.RequestID)
			case "install-revoke":
				p := r.Snapshot()
				r.Revoke(p.Pin, "service-install-revoke")
			case "unknown-effect":
				s.mu.Lock()
				x := s.work[w.RequestID]
				x.Operations = append(x.Operations, Operation{Key: "fixture-effect", Status: "unknown"})
				s.work[w.RequestID] = x
				s.mu.Unlock()
			case "legacy-context":
				s.mu.Lock()
				x := s.work[w.RequestID]
				x.ContextProtocolVersion = 0
				s.work[w.RequestID] = x
				s.mu.Unlock()
			}
			in := TaskControlInput{Version: 2, Action: "recover", RequestID: w.RequestID, ExpectedEpoch: old.Epoch, ExpectedRevision: old.TaskRevision, ClientNonce: "service-negative-" + kind}
			if kind == "epoch" {
				in.ExpectedEpoch--
			}
			if kind == "revision" {
				in.ExpectedRevision++
			}
			before := s.Snapshot().Sequence
			if _, _, e := s.ControlTask(in); !errors.Is(e, ErrConflict) || s.Snapshot().Sequence != before {
				t.Fatal("unsafe recovery committed", e)
			}
		})
	}
}
