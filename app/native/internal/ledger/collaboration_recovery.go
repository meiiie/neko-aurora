package ledger

import (
	"nekobot.local/native/internal/executors"
	"reflect"
	"strings"
	"time"
)

type CollaborationRecoveryInput struct {
	Version              int    `json:"version"`
	RequestID            string `json:"requestId"`
	ExpectedRevision     uint64 `json:"expectedRevision"`
	ExpectedEpoch        uint64 `json:"expectedEpoch"`
	ExpectedTaskRevision uint64 `json:"expectedTaskRevision"`
	ClientNonce          string `json:"clientNonce"`
}
type CollaborationRecoveryEvent struct {
	CoreProfileReservationVersion int                        `json:"coreProfileReservationVersion,omitempty"`
	NonceDomainVersion            int                        `json:"nonceDomainVersion,omitempty"`
	Input                         CollaborationRecoveryInput `json:"input"`
	IssuedAt                      string                     `json:"issuedAt"`
}
type CollaborationRecoveryResult struct {
	Collaboration Collaboration `json:"collaboration"`
	Work          Work          `json:"work"`
}

func groupBudgetAt(g Collaboration, at time.Time) bool {
	created, e1 := time.Parse(time.RFC3339Nano, g.CreatedAt)
	deadline, e2 := time.Parse(time.RFC3339Nano, g.DeadlineAt)
	return g.BudgetClosedAt == "" && g.BudgetClosedReason == "" && e1 == nil && e2 == nil && !at.Before(created) && at.Before(deadline)
}
func collaborationBudgetClosureReason(g Collaboration, at time.Time) string {
	created, e1 := time.Parse(time.RFC3339Nano, g.CreatedAt)
	deadline, e2 := time.Parse(time.RFC3339Nano, g.DeadlineAt)
	if e1 != nil || e2 != nil {
		return ""
	}
	if at.Before(created) {
		return "clock_before_creation"
	}
	if !at.Before(deadline) {
		return "deadline_observed"
	}
	return ""
}
func (s *Store) observeCollaborationBudgetLocked(g Collaboration, at time.Time) (Collaboration, error) {
	if g.BudgetClosedAt != "" || g.Status == "completed" || g.Status == "cancelled" {
		return g, nil
	}
	reason := collaborationBudgetClosureReason(g, at)
	if reason == "" {
		return g, nil
	}
	if g.Revision >= executors.SafeInteger {
		return g, ErrInvalid
	}
	g = copyCollaboration(g)
	g.Revision++
	g.BudgetClosedAt, g.BudgetClosedReason = at.UTC().Format(time.RFC3339Nano), reason
	if err := s.commit(Event{Type: "collaboration.budget_closed", Collaboration: &g}); err != nil {
		return g, err
	}
	return g, nil
}
func (s *Store) collaborationBudgetAllows(w Work) bool {
	if w.Collaboration == nil {
		return true
	}
	g, ok := s.collaborations[w.Collaboration.ID]
	if !ok || g.Status != "running" || w.Collaboration.Stage < 1 || w.Collaboration.Stage > len(g.RequestIDs) || g.RequestIDs[w.Collaboration.Stage-1] != w.RequestID {
		return false
	}
	now := s.clock()
	g, err := s.observeCollaborationBudgetLocked(g, now)
	return err == nil && groupBudgetAt(g, now)
}
func collaborationRecoveryAccountAt(w Work, at time.Time) bool {
	if w.AccountExpiredAt != "" {
		return false
	}
	l := w.AccountLease
	if l == nil {
		return w.Account == nil
	}
	if w.Account == nil {
		return false
	}
	issued, e1 := time.Parse(time.RFC3339Nano, l.IssuedAt)
	expiry, e2 := time.Parse(time.RFC3339Nano, l.ExpiresAt)
	return e1 == nil && e2 == nil && expiry.After(issued) && !at.Before(issued) && at.Before(expiry)
}
func (s *Store) collaborationRecoveryReady(g Collaboration, w Work, at time.Time) bool {
	if !s.validGroupProfileWork(w, g) || g.Status != "suspended" || g.Reason != "task_suspended" || !groupBudgetAt(g, at) || !collaborationRecoveryAccountAt(w, at) || len(g.RequestIDs) < 1 || len(g.RequestIDs) > g.MaxStages || w.Collaboration == nil || w.Collaboration.ID != g.ID || w.Collaboration.Stage != len(g.RequestIDs) || g.RequestIDs[len(g.RequestIDs)-1] != w.RequestID || !serviceRecoveryReady(w) || !equalNativeBinding(*w.Core, *w.RuntimeAttemptBinding) || w.Epoch >= executors.SafeInteger || w.TaskRevision >= executors.SafeInteger || g.Revision >= executors.SafeInteger || !reflect.DeepEqual(g.ExecutorPin, w.ExecutorPin) || !reflect.DeepEqual(g.Account, w.Account) || !reflect.DeepEqual(g.Resources, w.Resources) || g.ExecutorPolicyVersion != w.ExecutorPolicyVersion || g.ToolPolicyVersion != w.ToolPolicyVersion || !reflect.DeepEqual(g.PluginPins, w.PluginPins) || !reflect.DeepEqual(g.PluginSelection, w.PluginSelection) || w.BotID != stageBot(g, len(g.RequestIDs)) || w.ClientNonce != stageNonce(g, len(g.RequestIDs)) || w.Text != stageText(g, len(g.RequestIDs)) || w.ConversationID != g.ChannelID || w.OwnerUserID != g.OwnerUserID || w.SpaceID != g.SpaceID || !s.contextAllowedForGroupRecovery(w, g.ID) {
		return false
	}
	for i, id := range g.RequestIDs[:len(g.RequestIDs)-1] {
		prior, ok := s.work[id]
		if !ok || prior.Collaboration == nil || prior.Collaboration.ID != g.ID || prior.Collaboration.Stage != i+1 || prior.Status != "completed" || strings.TrimSpace(prior.Reply) == "" || !operationsComplete(prior) || !stoppedTask(prior) || !s.contextAllowedForGroupRecovery(prior, g.ID) {
			return false
		}
	}
	return true
}
func validCollaborationRecoveryInput(in CollaborationRecoveryInput) bool {
	return in.Version == 2 && resourceID(in.RequestID, "req-") && in.ExpectedRevision > 0 && in.ExpectedEpoch > 0 && validAccessNonce(in.ClientNonce)
}
func (s *Store) recoveryResult(e Event) CollaborationRecoveryResult {
	return CollaborationRecoveryResult{Collaboration: copyCollaboration(s.collaborations[e.Collaboration.ID]), Work: copyWork(s.work[e.CollaborationRecovery.Input.RequestID])}
}
func (s *Store) RecoverCollaboration(id string, in CollaborationRecoveryInput) (CollaborationRecoveryResult, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.poisoned {
		return CollaborationRecoveryResult{}, false, ErrUnavailable
	}
	if !resourceID(id, "collab-") || !validCollaborationRecoveryInput(in) {
		return CollaborationRecoveryResult{}, false, ErrInvalid
	}
	if e, ok := s.collaborationActions[in.ClientNonce]; ok {
		if e.Type != "collaboration.recovered" || e.Collaboration.ID != id || e.CollaborationRecovery == nil || !reflect.DeepEqual(e.CollaborationRecovery.Input, in) {
			return CollaborationRecoveryResult{}, false, ErrConflict
		}
		return s.recoveryResult(e), true, nil
	}
	if s.lifecycleNonceUsed(in.ClientNonce) {
		return CollaborationRecoveryResult{}, false, ErrConflict
	}
	if _, ok := s.taskControlNonces[in.ClientNonce]; ok {
		return CollaborationRecoveryResult{}, false, ErrConflict
	}
	if _, ok := s.collaborationNonces[in.ClientNonce]; ok {
		return CollaborationRecoveryResult{}, false, ErrConflict
	}
	if len(s.collaborationActions) >= 2048 {
		return CollaborationRecoveryResult{}, false, ErrInvalid
	}
	g, ok := s.collaborations[id]
	if !ok {
		return CollaborationRecoveryResult{}, false, ErrNotFound
	}
	old, ok := s.work[in.RequestID]
	now := s.clock()
	if ok && g.Revision == in.ExpectedRevision && old.Epoch == in.ExpectedEpoch && old.TaskRevision == in.ExpectedTaskRevision && len(g.RequestIDs) > 0 && g.RequestIDs[len(g.RequestIDs)-1] == old.RequestID && old.Collaboration != nil && old.Collaboration.ID == g.ID && old.Collaboration.Stage == len(g.RequestIDs) {
		var err error
		g, err = s.observeCollaborationBudgetLocked(g, now)
		if err != nil {
			return CollaborationRecoveryResult{}, false, err
		}
	}
	if !ok || g.Revision != in.ExpectedRevision || old.Epoch != in.ExpectedEpoch || old.TaskRevision != in.ExpectedTaskRevision || !s.collaborationRecoveryReady(g, old, now) || !s.collaborationAuthority(g) {
		return CollaborationRecoveryResult{}, false, ErrConflict
	}
	if g.CoreProfileGroup != nil {
		if err := s.groupProfileGuard(g); err != nil {
			return CollaborationRecoveryResult{}, false, ErrConflict
		}
		current, exists := s.work[old.RequestID]
		if !exists || !reflect.DeepEqual(current, old) || !s.collaborationRecoveryReady(g, current, s.clock()) || !s.collaborationAuthority(g) {
			return CollaborationRecoveryResult{}, false, ErrConflict
		}
		now = s.clock()
	}
	release, err := s.controlAccountGuard(old)
	if err != nil {
		return CollaborationRecoveryResult{}, false, err
	}
	defer release()
	g = copyCollaboration(g)
	if !reserveGroupProfileRecovery(&g, old, in, now) {
		return CollaborationRecoveryResult{}, false, ErrConflict
	}
	g.Status, g.Reason = "running", ""
	g.Revision++
	w := copyWork(old)
	w.Status, w.DesiredState, w.Failure = "waiting_host", "running", ""
	w.Epoch++
	w.TaskRevision++
	if !reservedLaunchBound(w) {
		return CollaborationRecoveryResult{}, false, ErrInvalid
	}
	e := Event{Type: "collaboration.recovered", Collaboration: &g, Work: w, Nonce: in.ClientNonce, CollaborationRecovery: &CollaborationRecoveryEvent{NonceDomainVersion: 1, Input: in, IssuedAt: now.UTC().Format(time.RFC3339Nano)}}
	if g.CoreProfileGroup != nil {
		e.CollaborationRecovery.CoreProfileReservationVersion = 1
	}
	if !s.validCollaborationRecoveryEvent(e) {
		return CollaborationRecoveryResult{}, false, ErrConflict
	}
	err = s.withInstallPin(old.ExecutorPolicyVersion, old.ExecutorPin, func() error { return s.commit(e) })
	if err != nil {
		return CollaborationRecoveryResult{}, false, err
	}
	return s.recoveryResult(e), false, nil
}
func (s *Store) validCollaborationRecoveryEvent(e Event) bool {
	if s.lifecycleNonceUsed(e.Nonce) {
		return false
	}
	c := e.CollaborationRecovery
	if e.Type != "collaboration.recovered" || c == nil || c.NonceDomainVersion < 0 || c.NonceDomainVersion > 1 || e.Collaboration == nil || e.Nonce != c.Input.ClientNonce || !validCollaborationRecoveryInput(c.Input) || e.TaskControl != nil || e.Plugin != nil || e.ProjectContinuity != nil || e.Bot != nil || e.Channel != nil || e.Project != nil || e.Upload != nil || e.AdapterSession != nil || e.ContextClaimVersion != 0 || e.ExecutorClaimVersion != 0 || e.CompletionVersion != 0 || e.ContextRefreshVersion != 0 || e.ResourceRevision != 0 {
		return false
	}
	if len(s.collaborationActions) >= 2048 {
		return false
	}
	if _, ok := s.collaborationActions[e.Nonce]; ok {
		return false
	}
	if _, ok := s.taskControlNonces[e.Nonce]; ok {
		return false
	}
	if c.NonceDomainVersion == 1 {
		if _, ok := s.collaborationNonces[e.Nonce]; ok {
			return false
		}
	}
	g, ok := s.collaborations[e.Collaboration.ID]
	if !ok {
		return false
	}
	if g.CoreProfileGroup == nil && c.CoreProfileReservationVersion != 0 || g.CoreProfileGroup != nil && c.CoreProfileReservationVersion != 1 {
		return false
	}
	if g.CoreProfileGroup != nil {
		p := g.CoreProfileGroup.OriginalPin
		consent, exists := s.coreProfileConsents[p.ConsentID]
		if !exists || consent.Status != "approved" || consent.Revision != p.ConsentRevision || s.coreProfileRetired[consent.ID] {
			return false
		}
	}
	old, ok := s.work[c.Input.RequestID]
	if !ok {
		return false
	}
	at, err := time.Parse(time.RFC3339Nano, c.IssuedAt)
	if err != nil || at.UTC().Format(time.RFC3339Nano) != c.IssuedAt || g.Revision != c.Input.ExpectedRevision || old.Epoch != c.Input.ExpectedEpoch || old.TaskRevision != c.Input.ExpectedTaskRevision || !s.collaborationRecoveryReady(g, old, at) {
		return false
	}
	g = copyCollaboration(g)
	if !reserveGroupProfileRecovery(&g, old, c.Input, at) {
		return false
	}
	g.Status, g.Reason = "running", ""
	g.Revision++
	w := copyWork(old)
	w.Status, w.DesiredState, w.Failure = "waiting_host", "running", ""
	w.Epoch++
	w.TaskRevision++
	return reflect.DeepEqual(g, *e.Collaboration) && reflect.DeepEqual(w, e.Work)
}

func groupServiceStopPending(g Collaboration, w Work) bool {
	return g.Status == "suspended" && g.Reason == "task_suspended" && len(g.RequestIDs) > 0 && g.RequestIDs[len(g.RequestIDs)-1] == w.RequestID && w.Collaboration != nil && w.Collaboration.ID == g.ID && w.Collaboration.Stage == len(g.RequestIDs) && w.Status == "suspending" && w.DesiredState == "suspended" && w.Failure == "executor_lease_unavailable" && w.ExecutorPolicyVersion == 1 && w.ExecutorPin != nil && w.CurrentAttemptLease != nil && w.Core != nil && w.RuntimeAttemptBinding != nil && w.ContextProtocolVersion == 1 && w.Core.ContextAdmission != nil && ValidProcessGeneration(w.RuntimeAttemptBinding.NativeProcessGeneration) && equalNativeBinding(*w.Core, *w.RuntimeAttemptBinding)
}
