package ledger

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func stopRecord(w Work, b CoreBinding) NativeStopRecord {
	return NativeStopRecord{Version: 1, RequestID: w.RequestID, AttemptID: w.AttemptID, InstallPin: *w.ExecutorPin, AttemptLease: *w.CurrentAttemptLease, Binding: b, Receipt: StoppedReceipt{Version: 1, NativeProcessID: b.NativeProcessID, NativeProcessGeneration: b.NativeProcessGeneration, SessionID: b.SessionID, ExecutorProof: b.ExecutorProof}}
}
func TestNativeStopStrictParser(t *testing.T) {
	s, _, w, b := serviceRecoveryFixture(t)
	raw, _ := json.Marshal(stopRecord(w, b))
	if _, e := ParseNativeStop(raw); e != nil {
		t.Fatal(e)
	}
	for name, bad := range map[string][]byte{
		"duplicate": append([]byte(`{"version":1,`), raw[1:]...),
		"unknown":   append([]byte(`{"unknown":false,`), raw[1:]...),
		"trailing":  append(append([]byte{}, raw...), []byte(` {}`)...),
		"partial":   raw[:len(raw)-1],
		"missing":   []byte(`{"version":1}`),
		"null":      []byte(`null`),
		"oversized": []byte(strings.Repeat(" ", NativeStopMaxBytes+1)),
	} {
		t.Run(name, func(t *testing.T) {
			if _, e := ParseNativeStop(bad); e == nil {
				t.Fatal("accepted malformed outbox")
			}
		})
	}
	for _, gen := range []string{"", "0", "01", "-1", "1e3", "18446744073709551616"} {
		if ValidProcessGeneration(gen) {
			t.Fatal("invalid generation", gen)
		}
	}
	if !ValidProcessGeneration("18446744073709551615") {
		t.Fatal("u64 FILETIME bound")
	}
	if s.Snapshot().Sequence == 0 {
		t.Fatal("empty fixture")
	}
}
func TestNativeStopExactCurrentAttemptAndReplay(t *testing.T) {
	s, _, w, b := serviceRecoveryFixture(t)
	record := stopRecord(w, b)
	for _, kind := range []string{"request", "attempt", "pin", "lease", "pid", "generation", "session", "context", "receipt", "version"} {
		t.Run(kind, func(t *testing.T) {
			raw, _ := json.Marshal(record)
			var bad NativeStopRecord
			json.Unmarshal(raw, &bad)
			switch kind {
			case "request":
				bad.RequestID = "req-" + strings.Repeat("e", 32)
			case "attempt":
				bad.AttemptID = "attempt-" + strings.Repeat("e", 32)
			case "pin":
				bad.InstallPin.Generation++
			case "lease":
				bad.AttemptLease.WorkEpoch++
			case "pid":
				bad.Binding.NativeProcessID++
			case "generation":
				bad.Binding.NativeProcessGeneration = "134362178434013774"
			case "session":
				bad.Binding.SessionID = strings.Repeat("e", 32)
			case "context":
				bad.Binding.ContextAdmission.ManifestSHA256 = strings.Repeat("e", 64)
			case "receipt":
				bad.Receipt.NativeProcessGeneration = "134362178434013774"
			case "version":
				bad.Version = 2
			}
			before := s.Snapshot().Sequence
			if s.ReconcileNativeStop(bad) == nil || s.Snapshot().Sequence != before {
				t.Fatal("mismatched proof committed")
			}
		})
	}
	before, _ := s.Get(w.RequestID)
	if e := s.ReconcileNativeStop(record); e != nil {
		t.Fatal(e)
	}
	seq := s.Snapshot().Sequence
	if e := s.ReconcileNativeStop(record); e != nil || s.Snapshot().Sequence != seq {
		t.Fatal("non-idempotent historical receipt", e)
	}
	after, _ := s.Get(w.RequestID)
	if !after.RuntimeStopped || after.Status != before.Status || after.Epoch != before.Epoch || !reflect.DeepEqual(after.CurrentAttemptLease, before.CurrentAttemptLease) {
		t.Fatal("stop revived authority")
	}
	dir := s.dir
	s.Close()
	for i := 0; i < 3; i++ {
		re, e := Open(dir)
		if e != nil {
			t.Fatal(e)
		}
		got, _ := re.Get(w.RequestID)
		if !reflect.DeepEqual(got, after) {
			t.Fatal("receipt replay drift")
		}
		re.Close()
	}
}
func TestNativeStopAfterCancelCannotRecover(t *testing.T) {
	s, _, w, b := serviceRecoveryFixture(t)
	if _, e := s.Cancel(w.RequestID); e != nil {
		t.Fatal(e)
	}
	if e := s.ReconcileNativeStop(stopRecord(w, b)); e != nil {
		t.Fatal(e)
	}
	if e := s.ReconcileExecutors(); e != nil {
		t.Fatal(e)
	}
	got, _ := s.Get(w.RequestID)
	if !got.RuntimeStopped || got.DesiredState != "cancelled" || serviceRecoveryReady(got) {
		t.Fatal("cancelled receipt resumed task")
	}
}
func TestNativeStopUnacknowledgedOrPriorActivationDeny(t *testing.T) {
	for _, kind := range []string{"missing-admission", "prior-activation"} {
		t.Run(kind, func(t *testing.T) {
			s, _, w, b := serviceRecoveryFixture(t)
			s.mu.Lock()
			old := s.work[w.RequestID]
			if kind == "missing-admission" {
				old.Core = nil
			} else {
				core := *old.Core
				core.Task = json.RawMessage(`{"id":"prior-activation"}`)
				old.Core = &core
			}
			s.work[w.RequestID] = old
			s.mu.Unlock()
			before := s.Snapshot().Sequence
			if s.ReconcileNativeStop(stopRecord(w, b)) == nil || s.Snapshot().Sequence != before {
				t.Fatal("unacknowledged binding adopted")
			}
		})
	}
}
