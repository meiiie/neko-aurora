package ledger

import (
	"bytes"
	"encoding/json"
	"nekobot.local/native/internal/executors"
	"nekobot.local/native/internal/protocol"
	"reflect"
	"strconv"
)

const NativeStopMaxBytes = 16384

type NativeStopRecord struct {
	Version      int                    `json:"version"`
	RequestID    string                 `json:"requestId"`
	AttemptID    string                 `json:"attemptId"`
	InstallPin   executors.InstallPin   `json:"installPin"`
	AttemptLease executors.AttemptLease `json:"attemptLease"`
	Binding      CoreBinding            `json:"binding"`
	Receipt      StoppedReceipt         `json:"receipt"`
}

func ValidProcessGeneration(s string) bool {
	n, e := strconv.ParseUint(s, 10, 64)
	return e == nil && n > 0 && strconv.FormatUint(n, 10) == s
}
func equalNativeBinding(a, b CoreBinding) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	x, e := protocol.Canonical(x)
	if e != nil {
		return false
	}
	y, e = protocol.Canonical(y)
	return e == nil && bytes.Equal(x, y)
}
func ParseNativeStop(raw []byte) (NativeStopRecord, error) {
	var out NativeStopRecord
	if len(raw) > NativeStopMaxBytes {
		return out, ErrInvalid
	}
	canonical, e := protocol.Canonical(raw)
	if e != nil {
		return out, ErrInvalid
	}
	dec := json.NewDecoder(bytes.NewReader(canonical))
	dec.DisallowUnknownFields()
	if dec.Decode(&out) != nil {
		return out, ErrInvalid
	}
	rendered, _ := json.Marshal(out)
	exact, e := protocol.Canonical(rendered)
	if e != nil || !bytes.Equal(canonical, exact) || out.Version != 1 {
		return out, ErrInvalid
	}
	return out, nil
}
func (s *Store) ReconcileNativeStop(record NativeStopRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.poisoned {
		return ErrUnavailable
	}
	w, ok := s.work[record.RequestID]
	if !ok {
		return ErrNotFound
	}
	if record.Version != 1 || w.ExecutorPolicyVersion != 1 || w.ExecutorPin == nil || w.CurrentAttemptLease == nil || w.RuntimeAttemptBinding == nil || w.Core == nil || w.ContextProtocolVersion != 1 || record.AttemptID != w.AttemptID || !reflect.DeepEqual(record.InstallPin, *w.ExecutorPin) || !reflect.DeepEqual(record.AttemptLease, *w.CurrentAttemptLease) || !equalNativeBinding(record.Binding, *w.RuntimeAttemptBinding) || !equalNativeBinding(record.Binding, *w.Core) || !ValidProcessGeneration(record.Binding.NativeProcessGeneration) || record.Receipt.Version != 1 || record.Receipt.NativeProcessGeneration != record.Binding.NativeProcessGeneration || !validStoppedReceipt(w, record.Receipt) {
		return ErrConflict
	}
	if w.RuntimeStopped {
		if w.RuntimeStopReceipt != nil && reflect.DeepEqual(*w.RuntimeStopReceipt, record.Receipt) {
			return nil
		}
		return ErrConflict
	}
	w = copyWork(w)
	w.RuntimeStopped = true
	receipt := record.Receipt
	w.RuntimeStopReceipt = &receipt
	return s.commit(Event{Type: "work.runtime_stopped", Work: w})
}
func serviceRecoveryReady(w Work) bool {
	return w.Status == "suspended" && w.DesiredState == "suspended" && w.Failure == "executor_lease_unavailable" && w.ExecutorPolicyVersion == 1 && w.ExecutorPin != nil && w.CurrentAttemptLease != nil && w.Core != nil && w.RuntimeAttemptBinding != nil && w.ContextProtocolVersion == 1 && w.Core.ContextAdmission != nil && ValidProcessGeneration(w.RuntimeAttemptBinding.NativeProcessGeneration) && stoppedTask(w) && operationsComplete(w)
}
