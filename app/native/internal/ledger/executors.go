package ledger

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"nekobot.local/native/internal/executors"
	"nekobot.local/native/internal/protocol"
	"reflect"
	"strings"
)

// ConfigureExecutor selects the trusted-manager authority; nil fails closed.
// Historical replay deliberately never calls this live registry.
func (s *Store) ConfigureExecutor(r *executors.Registry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.executorRequired = true
	s.executor = r
}

func (s *Store) pinNewExecutor(w *Work) error {
	if !s.executorRequired {
		return nil
	}
	if s.executor == nil {
		return ErrUnavailable
	}
	snap := s.executor.Snapshot()
	if !executors.ValidInstallPin(snap.Pin) {
		return ErrUnavailable
	}
	w.ExecutorPolicyVersion = 1
	pin := snap.Pin
	w.ExecutorPin = &pin
	return nil
}

func ExecutorAuthorityHash(w Work) string {
	// Only immutable authority. Epoch and attempt have their own lease fields;
	// live status, reply, control revisions and old Core provenance are excluded.
	payload := map[string]any{"domain": "nekobot-task-authority-v1", "requestId": w.RequestID, "sourceMessageId": w.SourceMessageID, "owner": w.OwnerUserID, "space": w.SpaceID, "bot": w.BotID, "botConfigRevision": w.BotConfigRevision, "audience": w.ConversationID, "text": w.Text, "lane": w.Lane, "publication": w.Publication, "account": w.Account, "resources": w.Resources, "adapter": w.Adapter, "context": w.Context, "toolPolicyVersion": w.ToolPolicyVersion, "pluginPins": w.PluginPins, "collaboration": w.Collaboration, "taskLink": w.TaskLink, "executorPin": w.ExecutorPin}
	if w.CoreProfilePin != nil {
		payload["coreProfilePin"] = w.CoreProfilePin
	}
	if w.CoreProfileGroupBinding != nil {
		payload["coreProfileGroupBinding"] = w.CoreProfileGroupBinding
	}
	if w.ContextProtocolVersion != 0 {
		payload["contextProtocolVersion"] = w.ContextProtocolVersion
		payload["runtimeContextSHA256"] = w.RuntimeContextSHA256
	}
	raw, e := json.Marshal(payload)
	if e != nil {
		return ""
	}
	canonical, e := protocol.Canonical(raw)
	if e != nil {
		return ""
	}
	h := sha256.Sum256(canonical)
	return hex.EncodeToString(h[:])
}
func validExecutorWork(w Work) bool {
	if w.ExecutorPolicyVersion == 0 {
		return w.ExecutorPin == nil && w.CurrentAttemptLease == nil
	}
	if w.ExecutorPolicyVersion != 1 || w.ExecutorPin == nil || !executors.ValidInstallPin(*w.ExecutorPin) {
		return false
	}
	if w.CurrentAttemptLease == nil {
		return w.AttemptID == ""
	}
	l := w.CurrentAttemptLease
	return executors.ValidLease(*l) && l.RequestID == w.RequestID && l.AttemptID == w.AttemptID && l.ExecutorID == w.ExecutorPin.ExecutorID && l.Generation == w.ExecutorPin.Generation
}
func validExecutorClaim(old, w Work, version int) bool {
	if version == 0 {
		return old.ExecutorPolicyVersion == 0 && w.ExecutorPolicyVersion == 0 && reflect.DeepEqual(old.ExecutorPin, w.ExecutorPin) && w.CurrentAttemptLease == nil
	}
	if version != 1 || !validExecutorWork(w) || w.CurrentAttemptLease == nil || w.CurrentAttemptLease.WorkEpoch != w.Epoch || w.CurrentAttemptLease.AuthoritySHA256 != ExecutorAuthorityHash(w) {
		return false
	}
	if old.ExecutorPolicyVersion == 0 {
		if !neverLaunched(old) || len(old.Operations) != 0 {
			return false
		}
	} else if old.ExecutorPolicyVersion != 1 || !reflect.DeepEqual(old.ExecutorPin, w.ExecutorPin) {
		return false
	}
	if old.Core != nil && (!old.RuntimeStopped || old.RuntimeStopReceipt == nil || !validStoppedReceipt(old, *old.RuntimeStopReceipt) || !operationsComplete(old)) {
		return false
	}
	return old.CurrentAttemptLease == nil || (old.CurrentAttemptLease.ID != w.CurrentAttemptLease.ID && old.CurrentAttemptLease.AttemptID != w.CurrentAttemptLease.AttemptID && old.CurrentAttemptLease.Fence < w.CurrentAttemptLease.Fence)
}
func (s *Store) claimExecutor(old Work, w *Work) error {
	if !s.executorRequired {
		if w.ExecutorPolicyVersion != 0 {
			return ErrConflict
		}
		return nil
	}
	if s.executor == nil {
		return ErrConflict
	}
	if w.ExecutorPolicyVersion == 0 {
		if !neverLaunched(old) || len(old.Operations) != 0 {
			return ErrConflict
		}
		if e := s.pinNewExecutor(w); e != nil {
			return e
		}
	}
	if w.ExecutorPolicyVersion != 1 || w.ExecutorPin == nil {
		return ErrConflict
	}
	if old.Core != nil && (!old.RuntimeStopped || old.RuntimeStopReceipt == nil || !validStoppedReceipt(old, *old.RuntimeStopReceipt) || !operationsComplete(old)) {
		return ErrConflict
	}
	reserved := copyWork(*w)
	reserved.CurrentAttemptLease = nil
	if _, e := MarshalRuntimeLaunch(reserved, nil); e != nil {
		return e
	}
	l, _, e := s.executor.Issue(*w.ExecutorPin, executors.TaskScope{RequestID: w.RequestID, AttemptID: w.AttemptID, WorkEpoch: w.Epoch, AuthoritySHA256: ExecutorAuthorityHash(*w)})
	if e != nil {
		return ErrConflict
	}
	w.CurrentAttemptLease = &l
	if _, e = MarshalRuntimeLaunch(*w, nil); e != nil {
		return e
	}
	return nil
}
func (s *Store) withExecutor(w Work, fn func() error) error {
	if !s.executorRequired && w.ExecutorPolicyVersion == 0 {
		return fn()
	}
	if s.executor == nil || !validExecutorWork(w) || w.CurrentAttemptLease == nil || w.CurrentAttemptLease.WorkEpoch != w.Epoch || w.CurrentAttemptLease.AuthoritySHA256 != ExecutorAuthorityHash(w) {
		return ErrConflict
	}
	var callbackErr error
	e := s.executor.WithLease(*w.ExecutorPin, *w.CurrentAttemptLease, func() error { callbackErr = fn(); return callbackErr })
	if callbackErr != nil {
		return callbackErr
	}
	if e != nil {
		return ErrConflict
	}
	return nil
}
func (s *Store) executorAllowed(w Work) bool {
	return s.withExecutor(w, func() error { return nil }) == nil
}
func executorProofMatches(w Work, p *executors.RuntimeProof) bool {
	if w.ExecutorPolicyVersion == 0 {
		return p == nil
	}
	return w.ExecutorPolicyVersion == 1 && w.CurrentAttemptLease != nil && p != nil && executors.MatchRuntimeProof(*w.CurrentAttemptLease, *p)
}
func (s *Store) ReconcileExecutors() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, old := range s.work {
		if old.Status == "suspending" && old.Failure == "executor_lease_unavailable" && old.RuntimeStopped {
			w := copyWork(old)
			w.Status = "suspended"
			if e := s.commit(Event{Type: "work.executor_suspended", Work: w}); e != nil {
				return e
			}
			continue
		}
		if (old.Status != "starting" && old.Status != "running" && old.Status != "recovery_required") || old.DesiredState != "running" || s.executorAllowed(old) {
			continue
		}
		w := copyWork(old)
		w.Epoch++
		w.DesiredState = "suspended"
		w.Status = "suspending"
		if old.RuntimeStopped {
			w.Status = "suspended"
		}
		w.Failure = "executor_lease_unavailable"
		if e := s.commit(Event{Type: "work.executor_suspended", Work: w}); e != nil {
			return e
		}
	}
	return nil
}
func validExecutorSuspension(old, w Work) bool {
	expected := copyWork(old)
	if old.Status == "suspending" && old.Failure == "executor_lease_unavailable" && old.RuntimeStopped {
		expected.Status = "suspended"
		return reflect.DeepEqual(expected, w)
	}
	if (old.Status != "starting" && old.Status != "running" && old.Status != "recovery_required") || old.DesiredState != "running" || old.Epoch >= executors.SafeInteger {
		return false
	}
	expected.Epoch++
	expected.DesiredState = "suspended"
	expected.Status = "suspending"
	if old.RuntimeStopped {
		expected.Status = "suspended"
	}
	expected.Failure = "executor_lease_unavailable"
	return reflect.DeepEqual(expected, w)
}

func reservedLaunchBound(w Work) bool {
	if w.ExecutorPin == nil {
		w.ExecutorPolicyVersion = 1
		w.ExecutorPin = &executors.InstallPin{Version: 1, ExecutorID: "executor-" + strings.Repeat("0", 32), Generation: executors.SafeInteger, Descriptor: executors.Descriptor{HostSHA256: strings.Repeat("0", 64), CoreSHA256: strings.Repeat("0", 64), RootSHA256: strings.Repeat("0", 64), CapabilitiesSHA256: strings.Repeat("0", 64)}}
	}
	w.CurrentAttemptLease = nil
	_, e := MarshalRuntimeLaunch(w, nil)
	return e == nil
}
func validEnvelopeBlock(old, w Work) bool {
	if old.Status != "waiting_host" || old.DesiredState != "running" || (!neverLaunched(old) && (!stoppedTask(old) || !operationsComplete(old))) || old.Epoch >= executors.SafeInteger || reservedLaunchBound(old) {
		return false
	}
	expected := copyWork(old)
	expected.Status = "suspended"
	expected.DesiredState = "suspended"
	expected.Epoch++
	expected.Failure = "runtime_frame_oversized"
	return reflect.DeepEqual(expected, w)
}
func (s *Store) BlockRuntimeEnvelope(id string, epoch uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.work[id]
	if !ok {
		return ErrNotFound
	}
	if old.Epoch != epoch {
		return ErrConflict
	}
	w := copyWork(old)
	w.Status = "suspended"
	w.DesiredState = "suspended"
	w.Epoch++
	w.Failure = "runtime_frame_oversized"
	if !validEnvelopeBlock(old, w) {
		return ErrConflict
	}
	return s.commit(Event{Type: "work.runtime_frame_blocked", Work: w})
}

// MarshalRuntimeLaunch is shared with trusted launch metadata producers.
// Added fields may not replace existing authority, and the actual encoded
// envelope, including resume/install/lease and escaping, remains <=64000.
func MarshalRuntimeLaunch(w Work, additive map[string]any) ([]byte, error) {
	text := w.RuntimePrompt()
	if w.Core != nil {
		text = "Continue the existing task using its receipt and preserved context; do not repeat completed effects."
	}
	frame := map[string]any{"requestId": w.RequestID, "botId": w.BotID, "text": text}
	if w.CoreProfilePin != nil {
		frame["coreProfilePin"] = w.CoreProfilePin
	}
	if w.ContextProtocolVersion == 1 {
		if w.RuntimeContext == nil || !lowerHex(w.RuntimeContextSHA256, 64) {
			return nil, ErrInvalid
		}
		frame["contextProtocol"] = 1
		if w.Core == nil {
			frame["text"] = w.RuntimeContext.PrimaryMessage.Text
			frame["contextManifest"] = w.RuntimeContext
			frame["contextManifestSHA256"] = w.RuntimeContextSHA256
			frame["contextPinSHA256"] = contextHash(w)
		} else {
			if !validContextBinding(w, *w.Core) {
				return nil, ErrInvalid
			}
			frame["contextExpected"] = w.Core.ContextAdmission
		}
	} else if w.ContextProtocolVersion != 0 {
		return nil, ErrInvalid
	}
	if w.Core != nil {
		frame["resume"] = w.Core
	}
	if w.ExecutorPolicyVersion == 1 {
		frame["executorPin"] = w.ExecutorPin
		lease := w.CurrentAttemptLease
		if lease == nil {
			// Reserve the largest valid companion width at intake, before any
			// registry write. This placeholder is never a stored/live lease.
			reserved := executors.AttemptLease{Version: 1, ID: "executor-lease-" + strings.Repeat("0", 32), ExecutorID: "executor-" + strings.Repeat("0", 32), Generation: executors.SafeInteger, BootID: "executor-boot-" + strings.Repeat("0", 32), TaskScope: executors.TaskScope{RequestID: w.RequestID, AttemptID: "attempt-" + strings.Repeat("0", 32), WorkEpoch: executors.SafeInteger, AuthoritySHA256: strings.Repeat("0", 64)}, Fence: executors.SafeInteger, IssuedAtMs: executors.SafeInteger - 600000, ExpiresAtMs: executors.SafeInteger}
			lease = &reserved
		}
		frame["attemptLease"] = lease
	}
	for k, v := range additive {
		if k == "coreProfilePin" {
			return nil, ErrInvalid
		}
		if k == "requestId" || k == "botId" || k == "text" || k == "resume" || k == "executorPin" || k == "attemptLease" || k == "contextProtocolVersion" || k == "contextProtocol" || k == "contextManifestSHA256" || k == "contextPinSHA256" || k == "contextExpected" || k == "contextManifest" {
			return nil, ErrInvalid
		}
		if _, exists := frame[k]; exists {
			return nil, ErrInvalid
		}
		frame[k] = v
	}
	raw, e := json.Marshal(frame)
	if e != nil || len(raw) > 64000 {
		return nil, ErrInvalid
	}
	return raw, nil
}

func (s *Store) withInstallPin(version int, pin *executors.InstallPin, commit func() error) error {
	if !s.executorRequired && version == 0 && pin == nil {
		return commit()
	}
	if s.executor == nil || version != 1 || pin == nil {
		return ErrConflict
	}
	var callbackErr error
	e := s.executor.WithInstall(*pin, func() error { callbackErr = commit(); return callbackErr })
	if callbackErr != nil {
		return callbackErr
	}
	if e != nil {
		return ErrConflict
	}
	return nil
}
