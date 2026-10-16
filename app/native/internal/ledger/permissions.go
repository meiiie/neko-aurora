package ledger

import (
	"encoding/json"
	"nekobot.local/native/internal/executors"
	"nekobot.local/native/internal/protocol"
	"reflect"
	"sort"
	"strings"
)

type SourceAccess struct {
	Revision uint64 `json:"revision"`
	Granted  bool   `json:"granted"`
	Nonce    string `json:"nonce"`
}
type ContextAttempt struct {
	Core      *CoreBinding `json:"core,omitempty"`
	PinSHA256 string       `json:"pinSha256"`
	Reply     string       `json:"reply,omitempty"`
	Epoch     uint64       `json:"epoch"`
	Reason    string       `json:"reason"`
}
type AccessInput struct {
	RequestID        string `json:"requestId"`
	ExpectedRevision uint64 `json:"expectedRevision"`
	Granted          bool   `json:"granted"`
	ClientNonce      string `json:"clientNonce"`
}
type RefreshInput struct {
	RequestID     string `json:"requestId"`
	ExpectedEpoch uint64 `json:"expectedEpoch"`
}
type StoppedReceipt struct {
	NativeProcessGeneration string                  `json:"nativeProcessGeneration,omitempty"`
	Kind                    string                  `json:"kind,omitempty"`
	ExecutorProof           *executors.RuntimeProof `json:"executorProof,omitempty"`
	Version                 int                     `json:"version"`
	NativeProcessID         uint32                  `json:"nativeProcessId"`
	SessionID               string                  `json:"sessionId"`
}

func accessRevision(w Work) uint64 {
	if w.ContextAccess == nil {
		return 0
	}
	return w.ContextAccess.Revision
}
func accessGranted(w Work) bool { return w.ContextAccess == nil || w.ContextAccess.Granted }
func operationsComplete(w Work) bool {
	for _, op := range w.Operations {
		if op.Status != "completed" {
			return false
		}
	}
	return true
}
func validAccessNonce(nonce string) bool {
	if len(nonce) < 8 || len(nonce) > 128 {
		return false
	}
	return strings.IndexFunc(nonce, func(c rune) bool {
		return !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_')
	}) == -1
}
func (s *Store) contextAllowed(w Work) bool { return s.contextAllowedUnderGroup(w, "") }
func (s *Store) contextAllowedForGroupRecovery(w Work, id string) bool {
	return s.contextAllowedUnderGroup(w, id)
}
func (s *Store) contextAllowedUnderGroup(w Work, recoveryID string) bool {
	checked := map[string]bool{}
	visiting := map[string]bool{}
	var allowed func(Work) bool
	allowed = func(work Work) bool {
		if result, found := checked[work.RequestID]; found {
			return result
		}
		groupAllowed := s.collaborationAllowed(work)
		if recoveryID != "" && work.Collaboration != nil && work.Collaboration.ID == recoveryID {
			g, found := s.collaborations[recoveryID]
			stage := work.Collaboration.Stage
			groupAllowed = found && stage > 0 && stage <= len(g.RequestIDs) && g.RequestIDs[stage-1] == work.RequestID
		}
		if visiting[work.RequestID] || !accessGranted(work) || !s.publicationAllowed(work) || !s.resourceAllowed(work) || !groupAllowed || !s.pluginsAllowed(work) {
			return false
		}
		visiting[work.RequestID] = true
		result := true
		if work.Context != nil {
			if work.Context.Version < 5 && len(work.Context.PublishedReports) > 0 {
				result = false
			}
			for _, pin := range work.Context.PublishedReports {
				if !s.publishedReportAllowed(work, pin, work.Context.AtSequence) {
					result = false
					break
				}
			}
			for _, pin := range work.Context.Sources {
				source, found := s.work[pin.RequestID]
				sameBot := source.BotID == work.BotID || (work.Context.Version >= 3 && validChannelID(work.ConversationID) && pin.BotID == source.BotID)
				if !found || source.RequestID == work.RequestID || source.SourceMessageID != pin.MessageID || source.CreatedSequence != pin.Sequence || source.CreatedSequence > work.Context.AtSequence || source.OwnerUserID != work.OwnerUserID || source.SpaceID != work.SpaceID || !sameBot || source.ConversationID != work.ConversationID || accessRevision(source) != pin.AccessRevision || !allowed(source) {
					result = false
					break
				}
			}
		}
		delete(visiting, work.RequestID)
		checked[work.RequestID] = result
		return result
	}
	return allowed(w)
}
func (s *Store) RuntimeAllowed(id string, epoch uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.work[id]
	return ok && w.Epoch == epoch && w.DesiredState == "running" && s.activeExecutionAllowed(w) && (w.Core != nil || boundedRuntimePrompt(w)) && s.collaborationBudgetAllows(w) && s.contextAllowed(w) && s.accountAllowed(w) && s.executorAllowed(w)
}
func (s *Store) SetSourceAccess(input AccessInput) (Work, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.work[input.RequestID]
	if !ok {
		return Work{}, false, ErrNotFound
	}
	if !validAccessNonce(input.ClientNonce) {
		return Work{}, false, ErrInvalid
	}
	if w.ContextAccess != nil && w.ContextAccess.Nonce == input.ClientNonce {
		if w.ContextAccess.Granted != input.Granted || input.ExpectedRevision+1 != w.ContextAccess.Revision {
			return Work{}, false, ErrConflict
		}
		if err := s.suspendInvalidContext(); err != nil {
			return Work{}, false, err
		}
		return copyWork(s.work[w.RequestID]), true, nil
	}
	if accessRevision(w) != input.ExpectedRevision || accessRevision(w) == ^uint64(0) {
		return Work{}, false, ErrConflict
	}
	w = copyWork(w)
	w.ContextAccess = &SourceAccess{Revision: input.ExpectedRevision + 1, Granted: input.Granted, Nonce: input.ClientNonce}
	if err := s.commit(Event{Type: "work.source_access", Work: w}); err != nil {
		return Work{}, false, err
	}
	if err := s.suspendInvalidContext(); err != nil {
		return Work{}, false, err
	}
	return copyWork(s.work[w.RequestID]), false, nil
}
func suspendible(w Work) bool {
	return w.Status == "waiting_host" || w.Status == "starting" || w.Status == "running" || w.Status == "recovery_required"
}
func (s *Store) suspendInvalidContext() error {
	for _, current := range s.sortedWorkLocked() {
		if !suspendible(current) || s.contextAllowed(current) {
			continue
		}
		w := copyWork(current)
		w.Status = "suspending"
		if w.RuntimeStopped || (current.Status == "waiting_host" && w.AttemptID == "") {
			w.Status = "suspended"
		}
		w.DesiredState = "suspended"
		w.Epoch++
		w.Failure = "context_permission_changed"
		if err := s.commit(Event{Type: "work." + w.Status, Work: w}); err != nil {
			return err
		}
	}
	return nil
}
func (s *Store) sortedWorkLocked() []Work {
	work := make([]Work, 0, len(s.work))
	for _, w := range s.work {
		work = append(work, w)
	}
	sort.Slice(work, func(i, j int) bool { return work[i].CreatedSequence < work[j].CreatedSequence })
	return work
}
func validSuspension(s *Store, old, w Work, typ string) bool {
	if old.Status == "suspending" && typ == "work.suspended" {
		return old.RuntimeStopped && w.Status == "suspended" && w.Epoch == old.Epoch && w.DesiredState == "suspended"
	}
	if !suspendible(old) || s.contextAllowed(old) || w.Epoch != old.Epoch+1 || w.DesiredState != "suspended" || w.Failure != "context_permission_changed" {
		return false
	}
	wanted := "suspending"
	if old.RuntimeStopped || (old.Status == "waiting_host" && old.AttemptID == "") {
		wanted = "suspended"
	}
	return typ == "work."+wanted && w.Status == wanted
}
func (s *Store) MarkRuntimeStopped(id, attempt string, receipt StoppedReceipt) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, found := s.work[id]
	if !found {
		return ErrNotFound
	}
	if attempt == "" || w.AttemptID != attempt || !validStoppedReceipt(w, receipt) {
		return ErrConflict
	}
	if w.RuntimeStopped {
		return nil
	}
	w = copyWork(w)
	w.RuntimeStopped = true
	w.RuntimeStopReceipt = &receipt
	typ := "work.runtime_stopped"
	if receipt.Version == 2 {
		typ = "work.runtime_never_launched"
	}
	return s.commit(Event{Type: typ, Work: w})
}
func validStoppedReceipt(w Work, receipt StoppedReceipt) bool {
	if receipt.NativeProcessGeneration != "" && !ValidProcessGeneration(receipt.NativeProcessGeneration) {
		return false
	}
	if !executorProofMatches(w, receipt.ExecutorProof) {
		return false
	}
	if receipt.Version == 2 {
		return receipt.Kind == "never_launched" && receipt.NativeProcessID == 0 && receipt.NativeProcessGeneration == "" && receipt.SessionID == "" && w.ExecutorPolicyVersion == 1 && w.Core == nil && w.RuntimeAttemptBinding == nil && len(w.Operations) == 0
	}
	if receipt.Kind != "" {
		return false
	}
	if receipt.Version != 1 || receipt.NativeProcessID == 0 || (receipt.SessionID != "" && !lowerHex(receipt.SessionID, 32)) {
		return false
	}
	binding := w.RuntimeAttemptBinding
	if binding == nil {
		binding = w.Core
		if w.ExecutorPolicyVersion == 1 && binding != nil && !executorProofMatches(w, binding.ExecutorProof) {
			binding = nil
		}
	}
	return binding == nil || (binding.NativeProcessID == receipt.NativeProcessID && binding.SessionID == receipt.SessionID && binding.NativeProcessGeneration == receipt.NativeProcessGeneration)
}

func validAttemptBinding(w Work, b CoreBinding) bool {
	if w.CoreProfilePin != nil && b.CoreSHA256 != w.CoreProfilePin.RuntimeSHA256 {
		return false
	}
	if b.NativeProcessGeneration != "" && !ValidProcessGeneration(b.NativeProcessGeneration) {
		return false
	}
	if !validContextBinding(w, b) {
		return false
	}
	if !executorProofMatches(w, b.ExecutorProof) || (w.ExecutorPin != nil && b.CoreSHA256 != w.ExecutorPin.CoreSHA256) {
		return false
	}
	if (w.Status != "starting" && w.Status != "suspending" && w.Status != "cancelling") || b.NativeProcessID == 0 || !lowerHex(b.SessionID, 32) || !protocol.CoreRuntimeAllowed(b.CoreSHA256) {
		return false
	}
	var task struct {
		ID              string `json:"id"`
		Root            string `json:"root"`
		ActivationEpoch uint64 `json:"activationEpoch"`
		ActivationID    string `json:"activationId"`
	}
	if json.Unmarshal(b.Task, &task) != nil || !lowerHex(task.ID, 32) || task.Root == "" || task.ActivationEpoch == 0 || task.ActivationEpoch > 9007199254740991 || !lowerHex(task.ActivationID, 32) || !json.Valid(b.Execution) {
		return false
	}
	var currentExecution, savedExecution struct {
		AuthorityID string `json:"authorityId"`
	}
	if json.Unmarshal(b.Execution, &currentExecution) != nil || currentExecution.AuthorityID == "" {
		return false
	}
	if w.Core == nil {
		return b.TaskRecovery == nil
	}
	var savedTask struct {
		ID              string `json:"id"`
		Root            string `json:"root"`
		ActivationEpoch uint64 `json:"activationEpoch"`
		ActivationID    string `json:"activationId"`
	}
	step := uint64(1)
	if b.TaskRecovery != nil {
		if b.TaskRecovery.Version != 1 || b.TaskRecovery.Kind != "prior_receipt_retry" || !protocol.CorePriorReceiptRetry(b.CoreSHA256) {
			return false
		}
		step = 2
	}
	return json.Unmarshal(w.Core.Task, &savedTask) == nil && json.Unmarshal(w.Core.Execution, &savedExecution) == nil && b.CoreSHA256 == w.Core.CoreSHA256 && b.SessionID == w.Core.SessionID && task.ID == savedTask.ID && task.Root == savedTask.Root && savedTask.ActivationEpoch <= 9007199254740991-step && task.ActivationEpoch == savedTask.ActivationEpoch+step && task.ActivationID != savedTask.ActivationID && currentExecution.AuthorityID == savedExecution.AuthorityID
}

func (s *Store) BindRuntimeAttempt(id, attempt string, binding CoreBinding) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, found := s.work[id]
	if !found {
		return ErrNotFound
	}
	if w.CoreProfilePin != nil && !protocol.CoreProfileBridgeAllowed(binding.CoreSHA256) {
		return ErrForbidden
	}
	principalRelease, principalErr := s.executionPrincipalGuard(w)
	if principalErr != nil {
		return principalErr
	}
	defer principalRelease()
	if attempt == "" || w.AttemptID != attempt || w.RuntimeStopped || !validAttemptBinding(w, binding) {
		return ErrConflict
	}
	if w.RuntimeAttemptBinding != nil {
		if reflect.DeepEqual(*w.RuntimeAttemptBinding, binding) {
			return nil
		}
		return ErrConflict
	}
	w = copyWork(w)
	w.RuntimeAttemptBinding = &binding
	return s.commit(Event{Type: "work.runtime_bound", Work: w})
}

func boundedRuntimePrompt(w Work) bool {
	frame, err := MarshalRuntimeLaunch(w, nil)
	return err == nil && len(frame) <= 64000
}
func (s *Store) freshContext(w Work) *ContextPin {
	w.CreatedSequence = uint64(len(s.events) + 1)
	return s.captureContext(w)
}
func refreshable(w Work) bool {
	return w.Status == "suspended" && w.DesiredState == "suspended" && accessGranted(w) && len(w.Operations) == 0 && len(w.ContextAttempts) < 4 && (w.RuntimeStopped || (w.Core == nil && w.AttemptID == ""))
}
func contextArchive(w Work) ContextAttempt {
	hash := ""
	if w.Context != nil {
		hash = w.Context.SHA256
	}
	return ContextAttempt{Core: w.Core, PinSHA256: hash, Reply: w.Reply, Epoch: w.Epoch, Reason: "explicit_context_refresh"}
}
func (s *Store) validContextRefresh(old, w Work, version int) bool {
	if old.CoreProfileGroupBinding != nil {
		return false
	}
	if !s.validNewRuntimeContext(w) || (version != 3 && old.ContextProtocolVersion != w.ContextProtocolVersion) || (version == 3 && w.ContextProtocolVersion != 1) {
		return false
	}
	source := old
	source.CreatedSequence = uint64(len(s.events) + 1)
	pinVersion := 3
	if version < 2 {
		pinVersion = 2
	}
	if version == 3 {
		pinVersion = s.captureContext(source).Version
	}
	if version == 4 {
		pinVersion = 4
		if old.BotProfile == nil {
			return false
		}
	}
	if version == 5 {
		pinVersion = 5
	}
	return refreshable(old) && s.pluginsAllowed(old) && (version == 0 || boundedRuntimePrompt(w)) && w.Status == "waiting_host" && w.DesiredState == "running" && w.Epoch == old.Epoch+1 && w.ContextRevision == old.ContextRevision+1 && reflect.DeepEqual(w.Context, s.captureContextVersion(source, pinVersion)) && reflect.DeepEqual(w.ContextAttempts, append(append([]ContextAttempt(nil), old.ContextAttempts...), contextArchive(old)))
}
func (s *Store) RefreshContext(input RefreshInput) (Work, error) {
	current, err := s.Get(input.RequestID)
	if err != nil {
		return Work{}, err
	}
	if current.CoreProfileGroupBinding != nil {
		return Work{}, ErrConflict
	}
	return s.update(input.RequestID, input.ExpectedEpoch, "work.context_refreshed", func(w *Work) error {
		if w.CoreProfileGroupBinding != nil {
			return ErrConflict
		}
		if !refreshable(*w) || !s.publicationAllowed(*w) || !s.pluginsAllowed(*w) || !s.continuityAllowed(*w) {
			return ErrConflict
		}
		w.ContextAttempts = append(w.ContextAttempts, contextArchive(*w))
		w.Context = s.freshContext(*w)
		w.ContextRevision++
		w.Epoch++
		w.Status = "waiting_host"
		w.DesiredState = "running"
		w.Core = nil
		if err := s.attachRuntimeContext(w); err != nil {
			return err
		}
		w.Reply = ""
		w.Failure = ""
		if !boundedRuntimePrompt(*w) {
			return ErrInvalid
		}
		return nil
	})
}
