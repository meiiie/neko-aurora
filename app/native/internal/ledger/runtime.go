package ledger

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"nekobot.local/native/internal/executors"
	"nekobot.local/native/internal/protocol"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"
)

type CoreBinding struct {
	NativeProcessGeneration string                  `json:"nativeProcessGeneration,omitempty"`
	ContextAdmission        *ContextAdmission       `json:"contextAdmission,omitempty"`
	ExecutorProof           *executors.RuntimeProof `json:"executorProof,omitempty"`
	NativeProcessID         uint32                  `json:"nativeProcessId,omitempty"`
	SessionID               string                  `json:"sessionId"`
	Task                    json.RawMessage         `json:"task"`
	Execution               json.RawMessage         `json:"execution"`
	CoreSHA256              string                  `json:"coreSha256"`
	TaskRecovery            *TaskRecovery           `json:"taskRecovery,omitempty"`
}
type TaskRecovery struct {
	Version int    `json:"version"`
	Kind    string `json:"kind"`
}
type Artifact struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
	SHA256   string `json:"sha256"`
	Bytes    int    `json:"bytes"`
}
type Operation struct {
	Key      string          `json:"key"`
	Digest   string          `json:"digest"`
	Status   string          `json:"status"`
	Text     string          `json:"text"`
	Artifact Artifact        `json:"artifact"`
	Result   json.RawMessage `json:"result,omitempty"`
}

func copyWork(w Work) Work {
	if w.CoreProfileGroupBinding != nil {
		p := *w.CoreProfileGroupBinding
		w.CoreProfileGroupBinding = &p
	}
	if w.CoreProfilePin != nil {
		p := *w.CoreProfilePin
		w.CoreProfilePin = &p
	}
	if w.CoreProfileSelection != nil {
		p := *w.CoreProfileSelection
		w.CoreProfileSelection = &p
	}
	w.RequesterPin = copyRequester(w.RequesterPin)
	w.BotState = copyBotState(w.BotState)
	if w.ExpectedBotStateRevision != nil {
		v := *w.ExpectedBotStateRevision
		w.ExpectedBotStateRevision = &v
	}
	w.BotProfile = copyBotProfile(w.BotProfile)
	if w.ExpectedBotProfileRevision != nil {
		revision := *w.ExpectedBotProfileRevision
		w.ExpectedBotProfileRevision = &revision
	}
	w.RuntimeContext = copyRuntimeContext(w.RuntimeContext)
	if w.ExecutorPin != nil {
		p := *w.ExecutorPin
		w.ExecutorPin = &p
	}
	if w.CurrentAttemptLease != nil {
		l := *w.CurrentAttemptLease
		w.CurrentAttemptLease = &l
	}
	w.PluginSelection = copySelection(w.PluginSelection)
	w.PluginPins = append([]PluginPin(nil), w.PluginPins...)
	if w.Collaboration != nil {
		b := *w.Collaboration
		w.Collaboration = &b
	}
	if w.TaskLink != nil {
		link := *w.TaskLink
		w.TaskLink = &link
	}
	if w.Adapter != nil {
		pin := *w.Adapter
		w.Adapter = &pin
	}
	w.Resources = copyResources(w.Resources)
	if w.Publication != nil {
		p := *w.Publication
		w.Publication = &p
	}
	if w.Account != nil {
		pin := *w.Account
		w.Account = &pin
	}
	if w.AccountLease != nil {
		lease := *w.AccountLease
		w.AccountLease = &lease
	}
	if w.RuntimeAttemptBinding != nil {
		binding := *w.RuntimeAttemptBinding
		binding.ContextAdmission = copyContextAdmission(binding.ContextAdmission)
		if binding.ExecutorProof != nil {
			p := *binding.ExecutorProof
			binding.ExecutorProof = &p
		}
		if binding.TaskRecovery != nil {
			proof := *binding.TaskRecovery
			binding.TaskRecovery = &proof
		}
		binding.Task = append(json.RawMessage(nil), binding.Task...)
		binding.Execution = append(json.RawMessage(nil), binding.Execution...)
		w.RuntimeAttemptBinding = &binding
	}
	if w.RuntimeStopReceipt != nil {
		receipt := *w.RuntimeStopReceipt
		if receipt.ExecutorProof != nil {
			p := *receipt.ExecutorProof
			receipt.ExecutorProof = &p
		}
		w.RuntimeStopReceipt = &receipt
	}
	if w.ContextAccess != nil {
		access := *w.ContextAccess
		w.ContextAccess = &access
	}
	w.ContextAttempts = append([]ContextAttempt(nil), w.ContextAttempts...)
	for i := range w.ContextAttempts {
		if w.ContextAttempts[i].Core != nil {
			core := *w.ContextAttempts[i].Core
			core.ContextAdmission = copyContextAdmission(core.ContextAdmission)
			if core.ExecutorProof != nil {
				p := *core.ExecutorProof
				core.ExecutorProof = &p
			}
			if core.TaskRecovery != nil {
				proof := *core.TaskRecovery
				core.TaskRecovery = &proof
			}
			core.Task = append(json.RawMessage(nil), core.Task...)
			core.Execution = append(json.RawMessage(nil), core.Execution...)
			w.ContextAttempts[i].Core = &core
		}
	}
	if w.Context != nil {
		pin := *w.Context
		pin.Profile = copyBotProfile(pin.Profile)
		pin.PluginPins = append([]PluginPin(nil), pin.PluginPins...)
		pin.Resources = copyResources(pin.Resources)
		pin.Sources = append([]ContextSource{}, pin.Sources...)
		pin.PublishedReports = copyPublishedReports(pin.PublishedReports)
		for i := range pin.Sources {
			pin.Sources[i].Artifacts = append([]Artifact(nil), pin.Sources[i].Artifacts...)
		}
		w.Context = &pin
	}
	if w.Core != nil {
		core := *w.Core
		core.ContextAdmission = copyContextAdmission(core.ContextAdmission)
		if core.ExecutorProof != nil {
			p := *core.ExecutorProof
			core.ExecutorProof = &p
		}
		if core.TaskRecovery != nil {
			proof := *core.TaskRecovery
			core.TaskRecovery = &proof
		}
		core.Task = append(json.RawMessage(nil), core.Task...)
		core.Execution = append(json.RawMessage(nil), core.Execution...)
		w.Core = &core
	}
	w.Artifacts = append([]Artifact(nil), w.Artifacts...)
	w.Operations = append([]Operation(nil), w.Operations...)
	for i := range w.Operations {
		w.Operations[i].Result = append(json.RawMessage(nil), w.Operations[i].Result...)
	}
	return w
}
func (s *Store) Get(id string) (Work, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.work[id]
	if !ok {
		return Work{}, ErrNotFound
	}
	return copyWork(w), nil
}
func (s *Store) Runtime(label string) { s.mu.Lock(); defer s.mu.Unlock(); s.runtime = label }
func (s *Store) update(id string, epoch uint64, typ string, change func(*Work) error) (Work, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.work[id]
	if !ok {
		return Work{}, ErrNotFound
	}
	if w.Epoch != epoch {
		return Work{}, ErrConflict
	}
	if typ == "work.claimed" || typ == "work.admitted" || typ == "work.text" || typ == "work.completed" || typ == "work.requeued" || typ == "work.context_refreshed" {
		principalRelease, principalErr := s.executionPrincipalGuard(w)
		if principalErr != nil {
			return Work{}, principalErr
		}
		defer principalRelease()
		release, err := s.accountGuard(w, typ == "work.admitted" || typ == "work.text" || typ == "work.completed")
		if err != nil {
			return Work{}, err
		}
		defer release()
	}
	if (typ == "work.claimed" || typ == "work.admitted" || typ == "work.text" || typ == "work.completed") && (!s.contextAllowed(w) || !s.collaborationBudgetAllows(w)) {
		return Work{}, ErrConflict
	}
	apply := func() error {
		w = copyWork(w)
		if err := change(&w); err != nil {
			return err
		}
		e := Event{Type: typ, Work: w}
		if typ == "work.claimed" && w.ExecutorPolicyVersion == 1 {
			e.ExecutorClaimVersion = 1
		}
		if typ == "work.claimed" && s.work[id].ContextProtocolVersion == 0 && w.ContextProtocolVersion == 1 {
			e.ContextClaimVersion = 1
		}
		if typ == "work.completed" {
			e.CompletionVersion = 1
		}
		if typ == "work.context_refreshed" {
			e.ContextRefreshVersion = 1
			if w.Context != nil && w.Context.Version == 3 {
				e.ContextRefreshVersion = 2
			}
			if w.ContextProtocolVersion == 1 {
				e.ContextRefreshVersion = 3
			}
			if w.Context != nil && w.Context.Version == 4 && w.ContextProtocolVersion == 0 {
				e.ContextRefreshVersion = 4
			}
			if w.Context != nil && w.Context.Version == 5 {
				e.ContextRefreshVersion = 5
			}
		}
		if !validRuntimeReplay(s, e) {
			return ErrConflict
		}
		if err := s.commit(e); err != nil {
			return err
		}
		return nil
	}
	var err error
	if typ == "work.admitted" || typ == "work.text" || typ == "work.completed" {
		err = s.withExecutor(w, apply)
	} else {
		err = apply()
	}
	return w, err
}
func (s *Store) Claim(id string, epoch uint64) (Work, error) {
	attempt, err := newID("attempt-")
	if err != nil {
		return Work{}, err
	}
	return s.update(id, epoch, "work.claimed", func(w *Work) error {
		if w.Status != "waiting_host" || w.DesiredState != "running" {
			return ErrConflict
		}
		if w.Core == nil && !boundedRuntimePrompt(*w) {
			return ErrInvalid
		}
		old := copyWork(*w)
		if s.contextAdmissionRequired && w.ContextProtocolVersion == 0 {
			if !neverLaunched(old) || len(old.Operations) != 0 {
				return ErrConflict
			}
			if err := s.attachRuntimeContext(w); err != nil {
				return err
			}
		}
		w.Status = "starting"
		w.AttemptID = attempt
		if w.Account != nil {
			id, err := newID("lease-")
			if err != nil {
				return err
			}
			fence := uint64(1)
			if w.AccountLease != nil {
				fence = w.AccountLease.Fence + 1
			}
			if fence == 0 {
				return ErrConflict
			}
			now := s.clock()
			w.AccountLease = &AccountLease{Version: 1, ID: id, RequestID: w.RequestID, BotID: w.BotID, OwnerUserID: w.OwnerUserID, SpaceID: w.SpaceID, Audience: w.ConversationID, AttemptID: attempt, Epoch: w.Epoch, Fence: fence, IssuedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(10 * time.Minute).Format(time.RFC3339Nano), Scope: "local-controlled-fixture"}
			w.AccountLease.AccountID = w.Account.AccountID
			w.AccountLease.AccountRevision = w.Account.Revision
			w.AccountLease.CredentialGeneration = w.Account.CredentialGeneration
			w.AccountLease.ContextSHA256 = contextHash(*w)
		}
		w.RuntimeStopped = false
		w.RuntimeStopReceipt = nil
		w.RuntimeAttemptBinding = nil
		w.Failure = ""
		return s.claimExecutor(old, w)
	})
}
func (s *Store) Admit(id string, epoch uint64, b CoreBinding) error {
	_, err := s.update(id, epoch, "work.admitted", func(w *Work) error {
		if w.CoreProfilePin != nil && (b.CoreSHA256 != w.CoreProfilePin.RuntimeSHA256 || !protocol.CoreProfileBridgeAllowed(b.CoreSHA256)) {
			return ErrInvalid
		}
		if !validContextBinding(*w, b) {
			return ErrInvalid
		}
		if !executorProofMatches(*w, b.ExecutorProof) || (w.ExecutorPolicyVersion == 1 && w.RuntimeAttemptBinding == nil) || w.Status != "starting" || len(b.SessionID) != 32 || !protocol.CoreRuntimeAllowed(b.CoreSHA256) || len(b.Task) == 0 || len(b.Execution) == 0 || (b.TaskRecovery != nil && w.RuntimeAttemptBinding == nil) || (w.RuntimeAttemptBinding != nil && !reflect.DeepEqual(*w.RuntimeAttemptBinding, b)) {
			return ErrInvalid
		}
		w.Status = "running"
		w.Core = &b
		return nil
	})
	return err
}
func (s *Store) AppendText(id string, epoch uint64, text string) error {
	_, err := s.update(id, epoch, "work.text", func(w *Work) error {
		if w.Status != "running" || !utf8.ValidString(text) || len(w.Reply)+len(text) > 65536 {
			return ErrInvalid
		}
		w.Reply += text
		return nil
	})
	return err
}
func (s *Store) Finish(id string, epoch uint64, status, failure string) error {
	typ := "work." + status
	_, err := s.update(id, epoch, typ, func(w *Work) error {
		if status == "completed" {
			for _, op := range w.Operations {
				if op.Status != "completed" {
					return ErrConflict
				}
			}
		}
		w.Status = status
		w.Failure = failure
		return nil
	})
	return err
}
func (s *Store) RecoverInterrupted() error {
	if s.readOnly {
		return ErrReadOnly
	}
	if err := s.ReconcileExecutors(); err != nil {
		return err
	}
	s.mu.Lock()
	publicationErr := s.suspendInvalidPublication()
	s.mu.Unlock()
	if publicationErr != nil {
		return publicationErr
	}
	if err := s.ReconcileAccounts(); err != nil {
		return err
	}
	if err := s.ReconcileCoreProfiles(); err != nil {
		return err
	}
	if err := s.reconcileArtifacts(); err != nil {
		return err
	}
	s.mu.Lock()
	err := s.suspendInvalidContext()
	s.mu.Unlock()
	if err != nil {
		return err
	}
	for _, w := range s.Snapshot().Work {
		if w.Status == "waiting_host" && w.Core == nil && (w.AttemptID == "" || w.RuntimeStopped) && !boundedRuntimePrompt(w) {
			if _, err := s.update(w.RequestID, w.Epoch, "work.prompt_blocked", func(current *Work) error {
				current.Status = "suspended"
				current.DesiredState = "suspended"
				current.Epoch++
				current.Failure = "context_prompt_oversized"
				return nil
			}); err != nil {
				return err
			}
		}
		if w.Status == "starting" || w.Status == "running" {
			if err := s.Finish(w.RequestID, w.Epoch, "recovery_required", "transport_interrupted"); err != nil {
				return err
			}
		}
		if w.Status == "cancelling" {
			if err := s.Finish(w.RequestID, w.Epoch, "cancelled", "outcome_not_rollback"); err != nil {
				return err
			}
		}
		if w.Status == "suspending" && w.RuntimeStopped {
			reason := w.Failure
			if reason == "" {
				reason = "context_permission_changed"
			}
			if err := s.Finish(w.RequestID, w.Epoch, "suspended", reason); err != nil {
				return err
			}
		}
	}
	return s.ReconcileTaskControls()
}
func (s *Store) Requeue(id string) (Work, error) {
	w, err := s.Get(id)
	if err != nil {
		return Work{}, err
	}
	if w.CoreProfileGroupBinding != nil {
		return Work{}, ErrConflict
	}
	return s.update(id, w.Epoch, "work.requeued", func(w *Work) error {
		if w.CoreProfileGroupBinding != nil {
			return ErrConflict
		}
		if w.ExecutorPolicyVersion == 1 && (!w.RuntimeStopped || w.RuntimeStopReceipt == nil || !validStoppedReceipt(*w, *w.RuntimeStopReceipt) || !operationsComplete(*w)) {
			return ErrConflict
		}
		if w.Status != "recovery_required" || !s.contextAllowed(*w) {
			return ErrConflict
		}
		w.Status = "waiting_host"
		w.Epoch++
		w.Failure = ""
		return nil
	})
}
func validRuntimeReplay(s *Store, e Event) bool {
	old, ok := s.work[e.Work.RequestID]
	if !ok {
		return false
	}
	w := e.Work
	if (e.Type == "work.claimed" || e.Type == "work.admitted" || e.Type == "work.text" || e.Type == "work.operation_started" || e.Type == "work.completed" || e.Type == "work.requeued") && !s.groupProfileRecordAuthority(old) {
		return false
	}
	if e.Type == "work.requeued" && old.CoreProfileGroupBinding != nil {
		return false
	}
	if (e.Type == "work.claimed" || e.Type == "work.admitted" || e.Type == "work.text" || e.Type == "work.operation_started" || e.Type == "work.completed" || e.Type == "work.requeued") && (!s.contextAllowed(old) || (old.Collaboration != nil && (s.collaborations[old.Collaboration.ID].BudgetClosedAt != "" || s.collaborations[old.Collaboration.ID].BudgetClosedReason != ""))) {
		return false
	}
	if w.OwnerUserID != old.OwnerUserID || w.SpaceID != old.SpaceID || w.BotID != old.BotID || w.Text != old.Text || w.ClientNonce != old.ClientNonce || w.SourceMessageID != old.SourceMessageID || w.ConversationID != old.ConversationID || w.CreatedSequence != old.CreatedSequence || w.CreatedAt != old.CreatedAt || w.BotConfigRevision != old.BotConfigRevision {
		return false
	}
	expected := copyWork(old)
	if !reflect.DeepEqual(old.Publication, w.Publication) || !sameAccount(old, w) {
		return false
	}
	switch e.Type {
	case "work.claimed":
		if e.ContextClaimVersion == 1 {
			if old.ContextProtocolVersion != 0 || !neverLaunched(old) || len(old.Operations) != 0 || w.ContextProtocolVersion != 1 || !s.validNewRuntimeContext(w) {
				return false
			}
			expected.ContextProtocolVersion = w.ContextProtocolVersion
			expected.RuntimeContext = w.RuntimeContext
			expected.RuntimeContextSHA256 = w.RuntimeContextSHA256
		}
		if !validExecutorClaim(old, w, e.ExecutorClaimVersion) {
			return false
		}
		expected.ExecutorPolicyVersion = w.ExecutorPolicyVersion
		expected.ExecutorPin = w.ExecutorPin
		expected.CurrentAttemptLease = w.CurrentAttemptLease
		if old.Status != "waiting_host" || old.DesiredState != "running" || w.Status != "starting" || !strings.HasPrefix(w.AttemptID, "attempt-") {
			return false
		}
		expected.Status = w.Status
		expected.AttemptID = w.AttemptID
		if old.Account != nil {
			if !validAccountLease(w) || (old.AccountLease != nil && (w.AccountLease.Fence != old.AccountLease.Fence+1 || w.AccountLease.ID == old.AccountLease.ID)) || (old.AccountLease == nil && w.AccountLease.Fence != 1) {
				return false
			}
			expected.AccountLease = w.AccountLease
		}
		expected.RuntimeStopped = false
		expected.RuntimeStopReceipt = nil
		expected.RuntimeAttemptBinding = nil
		expected.Failure = ""
	case "work.admitted":
		if w.Core == nil || !validContextBinding(old, *w.Core) {
			return false
		}
		if w.Core == nil || !executorProofMatches(old, w.Core.ExecutorProof) || (old.ExecutorPolicyVersion == 1 && old.RuntimeAttemptBinding == nil) {
			return false
		}
		if old.Status != "starting" || w.Status != "running" || w.Core == nil || (old.Core != nil && old.Core.CoreSHA256 != w.Core.CoreSHA256) || (w.Core.TaskRecovery != nil && old.RuntimeAttemptBinding == nil) || (old.RuntimeAttemptBinding != nil && !reflect.DeepEqual(*old.RuntimeAttemptBinding, *w.Core)) {
			return false
		}
		expected.Status = w.Status
		expected.Core = w.Core
	case "work.text":
		if old.Status != "running" || w.Status != "running" || !strings.HasPrefix(w.Reply, old.Reply) || len(w.Reply) > 65536 {
			return false
		}
		expected.Reply = w.Reply
	case "work.operation_started":
		if !stageWriteAllowed(old) || !pinnedTool(old, "artifact.write_text") {
			return false
		}
		if old.Status != "running" || len(w.Operations) != len(old.Operations)+1 || len(w.Operations) > 8 || !reflect.DeepEqual(append([]Operation(nil), w.Operations[:len(old.Operations)]...), old.Operations) {
			return false
		}
		added := w.Operations[len(w.Operations)-1]
		if added.Status != "started" || len(added.Key) != 65 || len(added.Digest) != 64 || added.Result != nil || !artifactMatches(added.Artifact, []byte(added.Text)) {
			return false
		}
		expected.Operations = w.Operations
	case "work.artifact":
		if !reconcilable(old.Status) || len(w.Artifacts) != len(old.Artifacts)+1 || len(w.Operations) != len(old.Operations) || !reflect.DeepEqual(append([]Artifact(nil), w.Artifacts[:len(old.Artifacts)]...), old.Artifacts) {
			return false
		}
		changes := 0
		for i, op := range w.Operations {
			previous := old.Operations[i]
			if reflect.DeepEqual(previous, op) {
				continue
			}
			if previous.Status != "started" || op.Status != "completed" || op.Artifact != w.Artifacts[len(w.Artifacts)-1] || len(op.Result) == 0 {
				return false
			}
			previous.Status = op.Status
			previous.Result = op.Result
			if !reflect.DeepEqual(previous, op) {
				return false
			}
			changes++
		}
		if changes != 1 {
			return false
		}
		expected.Artifacts = w.Artifacts
		expected.Operations = w.Operations
	case "work.operation_unknown":
		if !reconcilable(old.Status) || len(w.Operations) != len(old.Operations) {
			return false
		}
		changes := 0
		for i, op := range w.Operations {
			previous := old.Operations[i]
			if reflect.DeepEqual(previous, op) {
				continue
			}
			if previous.Status != "started" || op.Status != "unknown" {
				return false
			}
			previous.Status = op.Status
			if !reflect.DeepEqual(previous, op) {
				return false
			}
			changes++
		}
		if changes != 1 {
			return false
		}
		expected.Operations = w.Operations
	case "work.completed":
		if old.Collaboration != nil && e.CompletionVersion != 1 {
			return false
		}
		if old.Status != "running" || w.Status != "completed" {
			return false
		}
		if e.CompletionVersion == 1 {
			for _, op := range old.Operations {
				if op.Status != "completed" {
					return false
				}
			}
		}
		expected.Status = w.Status
		expected.Failure = w.Failure
	case "work.recovery_required":
		if (old.Status != "running" && old.Status != "starting") || w.Status != "recovery_required" {
			return false
		}
		expected.Status = w.Status
		expected.Failure = w.Failure
	case "work.requeued":
		if old.Status != "recovery_required" || w.Status != "waiting_host" || w.Epoch != old.Epoch+1 {
			return false
		}
		expected.Status = w.Status
		expected.Epoch = w.Epoch
		expected.Failure = ""
	case "work.cancelling":
		if (old.Status != "running" && old.Status != "starting" && old.Status != "suspending") || w.Status != "cancelling" || w.DesiredState != "cancelled" || w.Epoch != old.Epoch+1 {
			return false
		}
		expected.Status = w.Status
		expected.DesiredState = w.DesiredState
		expected.Epoch = w.Epoch
	case "work.cancelled":
		if (old.Status != "cancelling" && old.Status != "recovery_required" && old.Status != "suspended" && old.Status != "suspending" && old.Status != "paused" && old.Status != "waiting_control_stop") || w.Status != "cancelled" {
			return false
		}
		expected.Status = w.Status
		expected.DesiredState = w.DesiredState
		expected.Epoch = w.Epoch
		expected.Failure = w.Failure
	case "work.source_access":
		if w.ContextAccess == nil || w.ContextAccess.Revision != accessRevision(old)+1 || !validAccessNonce(w.ContextAccess.Nonce) || accessRevision(old) == ^uint64(0) {
			return false
		}
		expected.ContextAccess = w.ContextAccess
	case "work.publication_suspended":
		if !validPublicationSuspension(s, old, w) {
			return false
		}
		expected.Status = w.Status
		expected.DesiredState = w.DesiredState
		expected.Epoch = w.Epoch
		expected.Failure = w.Failure
	case "work.principal_suspended":
		if !validPrincipalSuspension(old, w) {
			return false
		}
		expected.Status = w.Status
		expected.DesiredState = w.DesiredState
		expected.Epoch = w.Epoch
		expected.Failure = w.Failure
	case "work.core_profile_suspended":
		probe := copyWork(w)
		probe.Failure = "requester_authority_changed"
		if old.CoreProfilePin == nil || w.Failure != "core_profile_authority_changed" || !validPrincipalSuspension(old, probe) {
			return false
		}
		expected.Status = w.Status
		expected.DesiredState = w.DesiredState
		expected.Epoch = w.Epoch
		expected.Failure = w.Failure
	case "work.runtime_frame_blocked":
		return validEnvelopeBlock(old, w)
	case "work.executor_suspended":
		return validExecutorSuspension(old, w)
	case "work.account_suspended":
		if !validAccountSuspension(old, w) {
			return false
		}
		expected.Status = w.Status
		expected.DesiredState = w.DesiredState
		expected.Epoch = w.Epoch
		expected.Failure = w.Failure
	case "work.account_expired":
		return validAccountExpiration(old, w)
	case "work.suspending", "work.suspended":
		if !validSuspension(s, old, w, e.Type) {
			return false
		}
		expected.Status = w.Status
		expected.DesiredState = w.DesiredState
		expected.Epoch = w.Epoch
		expected.Failure = w.Failure
	case "work.runtime_bound":
		if old.AttemptID == "" || old.RuntimeStopped || old.RuntimeAttemptBinding != nil || w.RuntimeAttemptBinding == nil || !validAttemptBinding(old, *w.RuntimeAttemptBinding) {
			return false
		}
		expected.RuntimeAttemptBinding = w.RuntimeAttemptBinding
	case "work.runtime_stopped", "work.runtime_never_launched":
		if w.RuntimeStopReceipt == nil || (e.Type == "work.runtime_never_launched") != (w.RuntimeStopReceipt.Version == 2) {
			return false
		}
		if w.RuntimeStopped != true || old.RuntimeStopped || old.AttemptID == "" || w.RuntimeStopReceipt == nil || !validStoppedReceipt(old, *w.RuntimeStopReceipt) {
			return false
		}
		expected.RuntimeStopped = true
		expected.RuntimeStopReceipt = w.RuntimeStopReceipt
	case "work.prompt_blocked":
		if old.Status != "waiting_host" || old.Core != nil || (old.AttemptID != "" && !old.RuntimeStopped) || boundedRuntimePrompt(old) || w.Status != "suspended" || w.DesiredState != "suspended" || w.Epoch != old.Epoch+1 || w.Failure != "context_prompt_oversized" {
			return false
		}
		expected.Status = w.Status
		expected.DesiredState = w.DesiredState
		expected.Epoch = w.Epoch
		expected.Failure = w.Failure
	case "work.context_admission_blocked":
		if !s.validContextAdmissionBlock(old, w) {
			return false
		}
		expected.Status, expected.DesiredState, expected.Failure, expected.Epoch = w.Status, w.DesiredState, w.Failure, w.Epoch
	case "work.context_refreshed":
		if !s.validContextRefresh(old, w, e.ContextRefreshVersion) {
			return false
		}
		expected.Status = w.Status
		expected.DesiredState = w.DesiredState
		expected.Epoch = w.Epoch
		expected.Context = w.Context
		expected.ContextProtocolVersion = w.ContextProtocolVersion
		expected.RuntimeContext = w.RuntimeContext
		expected.RuntimeContextSHA256 = w.RuntimeContextSHA256
		expected.Core = nil
		expected.Reply = ""
		expected.ContextRevision = w.ContextRevision
		expected.ContextAttempts = w.ContextAttempts
		expected.Failure = ""
	default:
		return false
	}
	return reflect.DeepEqual(expected, w)
}
func toolError(label string) json.RawMessage {
	b, _ := json.Marshal(map[string]any{"isError": true, "content": []any{map[string]string{"type": "text", "text": label}}})
	return b
}
func (s *Store) ExecuteTool(id string, epoch uint64, frame json.RawMessage) (json.RawMessage, error) {
	if s.readOnly {
		return nil, ErrReadOnly
	}
	canonical, err := protocol.Canonical(frame)
	if err != nil {
		return toolError("Invalid tool request"), nil
	}
	var call struct {
		Name      string `json:"name"`
		Arguments struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		} `json:"arguments"`
		Meta map[string]json.RawMessage `json:"_meta"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(canonical)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&call) != nil || (call.Name != "execute_tool" && call.Name != "capabilities") {
		return toolError("Action denied by host"), nil
	}
	var report struct {
		Filename string `json:"filename"`
		Text     string `json:"text"`
	}
	var read struct {
		FileID string `json:"fileId"`
		Offset int    `json:"offset"`
		Bytes  int    `json:"bytes"`
	}
	var publishedRead struct {
		DeliveryID string `json:"deliveryId"`
		ArtifactID string `json:"artifactId"`
		Offset     int    `json:"offset"`
		Bytes      int    `json:"bytes"`
	}
	switch {
	case call.Name == "capabilities":
		var envelope map[string]json.RawMessage
		_ = json.Unmarshal(canonical, &envelope)
		if string(envelope["arguments"]) != "{}" {
			return toolError("Invalid discovery arguments"), nil
		}
	case call.Arguments.Name == "artifact.write_text":
		d := json.NewDecoder(bytes.NewReader(call.Arguments.Arguments))
		d.DisallowUnknownFields()
		if d.Decode(&report) != nil {
			return toolError("Invalid report arguments"), nil
		}
		if len(report.Filename) == 0 || len(report.Filename) > 80 || filepath.Base(report.Filename) != report.Filename || strings.ContainsAny(report.Filename, "/\\:\x00\r\n") || (!strings.HasSuffix(report.Filename, ".md") && !strings.HasSuffix(report.Filename, ".txt")) || len(report.Text) == 0 || len(report.Text) > 24000 || !utf8.ValidString(report.Text) || hasSecret([]byte(report.Text)) {
			return toolError("Invalid report filename or content"), nil
		}
	case call.Arguments.Name == "project.read_file":
		d := json.NewDecoder(bytes.NewReader(call.Arguments.Arguments))
		d.DisallowUnknownFields()
		if d.Decode(&read) != nil || !resourceID(read.FileID, "file-") || read.Offset < 0 || read.Bytes < 1 || read.Bytes > 4000 {
			return toolError("Invalid file read range"), nil
		}
	case call.Arguments.Name == "report.read_artifact":
		d := json.NewDecoder(bytes.NewReader(call.Arguments.Arguments))
		d.DisallowUnknownFields()
		if d.Decode(&publishedRead) != nil || !resourceID(publishedRead.DeliveryID, "report-delivery-") || !lowerHex(publishedRead.ArtifactID, 64) || publishedRead.Offset < 0 || publishedRead.Bytes < 1 || publishedRead.Bytes > 4000 {
			return toolError("Invalid published report read range"), nil
		}
	default:
		return toolError("Action denied by host"), nil
	}
	var op struct {
		Version         int    `json:"version"`
		OperationID     string `json:"operationId"`
		SessionID       string `json:"sessionId"`
		TaskID          string `json:"taskId"`
		Root            string `json:"root"`
		ActivationEpoch uint64 `json:"activationEpoch"`
		ActivationID    string `json:"activationId"`
	}
	opDecoder := json.NewDecoder(strings.NewReader(string(call.Meta["neko.operation"])))
	opDecoder.DisallowUnknownFields()
	if opDecoder.Decode(&op) != nil || op.Version != 1 || !lowerHex(op.OperationID, 32) {
		return toolError("Missing canonical operation"), nil
	}
	var preparedSource UploadedFile
	var preparedText []byte
	var preparedError error
	if call.Name == "execute_tool" && call.Arguments.Name == "project.read_file" {
		preparedSource, preparedText, preparedError = s.preparePinnedFileText(id, epoch, read.FileID)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.work[id]
	if ok && !contextMetaMatches(w, call.Meta) {
		return toolError("Missing canonical context receipt"), nil
	}
	if !ok || w.Epoch != epoch || w.Status != "running" || !s.contextAllowed(w) || !s.collaborationBudgetAllows(w) || w.Core == nil || w.Core.SessionID != op.SessionID {
		return nil, ErrConflict
	}
	principalRelease, principalErr := s.executionPrincipalGuard(w)
	if principalErr != nil {
		return nil, principalErr
	}
	defer principalRelease()
	release, guardErr := s.accountGuard(w, true)
	if guardErr != nil {
		return nil, guardErr
	}
	defer release()
	var result json.RawMessage
	err = s.withExecutor(w, func() error {
		var inner error
		result, inner = func() (json.RawMessage, error) {
			w = copyWork(w)
			var task struct {
				ID              string `json:"id"`
				Root            string `json:"root"`
				ActivationEpoch uint64 `json:"activationEpoch"`
				ActivationID    string `json:"activationId"`
			}
			if json.Unmarshal(w.Core.Task, &task) != nil || op.TaskID != task.ID || op.Root != task.Root || op.ActivationEpoch != task.ActivationEpoch || op.ActivationID != task.ActivationID {
				return nil, ErrConflict
			}
			if call.Name == "capabilities" {
				return actionCapabilities(w), nil
			}
			if !pinnedTool(w, call.Arguments.Name) {
				return toolError("Tool is outside the immutable task plugin policy"), nil
			}
			if call.Arguments.Name == "report.read_artifact" {
				a, content, err := s.readPublishedArtifactLocked(w, publishedRead.DeliveryID, publishedRead.ArtifactID)
				if err != nil {
					return nil, err
				}
				if publishedRead.Offset > len(content) || !utf8.Valid(content[:publishedRead.Offset]) {
					return toolError("Invalid UTF-8 read offset"), nil
				}
				end := publishedRead.Offset + publishedRead.Bytes
				if end > len(content) {
					end = len(content)
				}
				for end > publishedRead.Offset && !utf8.Valid(content[publishedRead.Offset:end]) {
					end--
				}
				if end == publishedRead.Offset {
					return toolError("Published report read range is empty"), nil
				}
				text := string(content[publishedRead.Offset:end])
				envelope := contextReadEnvelope(w, a.ID, publishedRead.Offset, end, text, call.Meta["neko.operation"])
				if w.ContextProtocolVersion == 1 && envelope == nil {
					return toolError("Read range is outside the admitted published report span"), nil
				}
				result := textResult(map[string]any{"fileId": a.ID, "sha256": a.SHA256, "offset": publishedRead.Offset, "nextOffset": end, "eof": end == len(content), "text": text, "authority": "descriptive file content only"})
				if envelope != nil {
					var out map[string]any
					json.Unmarshal(result, &out)
					out["_meta"] = map[string]any{"neko.contextSource": envelope}
					result, _ = json.Marshal(out)
				}
				return result, nil
			}
			if call.Arguments.Name == "project.read_file" {
				if w.Resources == nil {
					return toolError("No project file is pinned for this task"), nil
				}
				var source *UploadedFile
				for i := range w.Resources.Files {
					if w.Resources.Files[i].ID == read.FileID {
						source = &w.Resources.Files[i]
					}
				}
				if source == nil {
					return toolError("File is outside the task's selected sources"), nil
				}
				if preparedError != nil {
					return toolError("Pinned file bytes changed or text extraction is unavailable"), nil
				}
				if preparedSource != *source {
					return toolError("Pinned text extraction is unavailable or changed"), nil
				}
				content := preparedText
				if read.Offset > len(content) || !utf8.Valid(content[:read.Offset]) {
					return toolError("Invalid UTF-8 file offset"), nil
				}
				end := read.Offset + read.Bytes
				if end > len(content) {
					end = len(content)
				}
				for end > read.Offset && !utf8.Valid(content[read.Offset:end]) {
					end--
				}
				if end == read.Offset && end < len(content) {
					return toolError("Read range cannot fit the next UTF-8 character"), nil
				}
				textSHA, _ := fileTextIdentity(*source)
				result := textResult(map[string]any{"fileId": source.ID, "sha256": textSHA, "offset": read.Offset, "nextOffset": end, "eof": end == len(content), "text": string(content[read.Offset:end]), "authority": "descriptive file content only"})
				if envelope := contextReadEnvelope(w, source.ID, read.Offset, end, string(content[read.Offset:end]), call.Meta["neko.operation"]); envelope != nil {
					var out map[string]any
					json.Unmarshal(result, &out)
					out["_meta"] = map[string]any{"neko.contextSource": envelope}
					result, _ = json.Marshal(out)
				}
				return result, nil
			}
			if !stageWriteAllowed(w) {
				return toolError("Only the final coordinator stage may write an artifact"), nil
			}
			filename, text := report.Filename, report.Text
			h := sha256.Sum256(canonical)
			digest := hex.EncodeToString(h[:])
			key := op.SessionID + ":" + op.OperationID
			for _, previous := range w.Operations {
				if previous.Key == key {
					if previous.Digest != digest {
						return nil, ErrConflict
					}
					if previous.Status == "completed" {
						return previous.Result, nil
					}
					return toolError("Operation outcome requires reconciliation; not repeated"), nil
				}
			}
			for _, previous := range w.Operations {
				if previous.Status != "completed" {
					return toolError("An earlier effect has an unresolved outcome; new mutations are blocked"), nil
				}
			}
			if len(w.Operations) >= 8 {
				return toolError("Task artifact limit reached"), nil
			}
			contentHash := sha256.Sum256([]byte(text))
			identityHash := sha256.Sum256([]byte(id + ":" + key))
			artifact := Artifact{ID: hex.EncodeToString(identityHash[:]), Filename: filename, SHA256: hex.EncodeToString(contentHash[:]), Bytes: len(text)}
			operation := Operation{Key: key, Digest: digest, Status: "started", Text: text, Artifact: artifact}
			w.Operations = append(w.Operations, operation)
			if err = s.commit(Event{Type: "work.operation_started", Work: w}); err != nil {
				return nil, err
			}
			dir := filepath.Join(s.dir, "artifacts")
			if os.MkdirAll(dir, 0700) != nil {
				return nil, ErrUnavailable
			}
			path := filepath.Join(dir, artifact.ID)
			file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err != nil {
				return nil, ErrUnavailable
			}
			_, err = file.WriteString(text)
			if err == nil {
				err = file.Sync()
			}
			closeErr := file.Close()
			if err != nil || closeErr != nil {
				return nil, ErrUnavailable
			}
			result := artifactResult(artifact)
			w = copyWork(w)
			w.Operations[len(w.Operations)-1].Status = "completed"
			w.Operations[len(w.Operations)-1].Result = result
			w.Artifacts = append(w.Artifacts, artifact)
			if err = s.commit(Event{Type: "work.artifact", Work: w}); err != nil {
				return nil, err
			}
			return result, nil
		}()
		return inner
	})
	return result, err
}
func (s *Store) ReadArtifact(id string) (Artifact, []byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readArtifactLocked(id, "")
}
func (s *Store) ReadChannelArtifact(channel, id string) (Artifact, []byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.channel(channel); !ok {
		return Artifact{}, nil, ErrNotFound
	}
	return s.readArtifactLocked(id, channel)
}
func (s *Store) readArtifactLocked(id, channel string) (Artifact, []byte, error) {
	for _, w := range s.work {
		if !s.continuityAllowed(w) || s.reportDerived(w) && !s.contextAllowed(w) {
			continue
		}
		if channel != "" && (w.ConversationID != channel || !s.publicationAllowed(w) || !s.contextAllowed(w)) {
			continue
		}
		for _, a := range w.Artifacts {
			if a.ID == id {
				b, err := os.ReadFile(filepath.Join(s.dir, "artifacts", a.ID))
				if err != nil {
					return Artifact{}, nil, ErrUnavailable
				}
				if !artifactMatches(a, b) {
					return Artifact{}, nil, ErrCorrupt
				}
				return a, b, nil
			}
		}
	}
	return Artifact{}, nil, ErrNotFound
}
func lowerHex(value string, size int) bool {
	if len(value) != size {
		return false
	}
	for _, c := range value {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func artifactMatches(a Artifact, b []byte) bool {
	h := sha256.Sum256(b)
	return lowerHex(a.ID, 64) && len(b) == a.Bytes && hex.EncodeToString(h[:]) == a.SHA256
}
func artifactResult(a Artifact) json.RawMessage {
	b, _ := json.Marshal(map[string]any{"content": []any{map[string]string{"type": "text", "text": fmt.Sprintf("Saved artifact %s (%d bytes), SHA256 %s", a.Filename, a.Bytes, a.SHA256)}}})
	return b
}
func (s *Store) reconcileArtifacts() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, original := range s.work {
		w := copyWork(original)
		for i, operation := range w.Operations {
			if operation.Status != "started" {
				continue
			}
			if !reconcilable(w.Status) {
				return ErrCorrupt
			}
			b, err := os.ReadFile(filepath.Join(s.dir, "artifacts", operation.Artifact.ID))
			typ := "work.operation_unknown"
			if err == nil && artifactMatches(operation.Artifact, b) {
				w.Operations[i].Status = "completed"
				w.Operations[i].Result = artifactResult(operation.Artifact)
				w.Artifacts = append(w.Artifacts, operation.Artifact)
				typ = "work.artifact"
			} else {
				w.Operations[i].Status = "unknown"
			}
			if err = s.commit(Event{Type: typ, Work: w}); err != nil {
				return err
			}
			w = copyWork(s.work[id])
		}
	}
	return nil
}

func reconcilable(status string) bool {
	return status == "waiting_host" || status == "starting" || status == "running" || status == "cancelling" || status == "cancelled" || status == "recovery_required" || status == "completed" || status == "suspending" || status == "suspended"
}
