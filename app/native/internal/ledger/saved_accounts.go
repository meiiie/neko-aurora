package ledger

import "nekobot.local/native/internal/providerexec"

const savedAccountLeaseScope = "saved-account-task-v1"

// SavedAccountPolicy is supplied only by trusted server code, never browser input.
// Version 1 binds the original Send intent: consent ID is requestId and revision
// is 1. Changes to that intent require a new Work, not mutation of this policy.
type SavedAccountPolicy struct {
	Version int                          `json:"version"`
	Limits  providerexec.SavedTaskLimits `json:"limits"`
}

func validSavedAccountPolicy(p *SavedAccountPolicy) bool {
	if p == nil || p.Version != 1 {
		return false
	}
	l := p.Limits
	return l.MaxInputBytes >= 1 && l.MaxInputBytes <= 64000 && l.MaxOutputTokens >= 1 && l.MaxOutputTokens <= 4096 && l.MaxRequests >= 1 && l.MaxRequests <= 8 && l.MaxSteps >= 1 && l.MaxSteps <= 6 && l.MaxSeconds >= 1 && l.MaxSeconds <= 600
}

// ClaimSavedAccount is deliberately not called by the runtime manager yet.
// It cannot promote a previously claimed fixture or resume an older attempt.
func (s *Store) ClaimSavedAccount(id string, epoch uint64, policy SavedAccountPolicy) (Work, error) {
	if !validSavedAccountPolicy(&policy) {
		return Work{}, ErrInvalid
	}
	return s.claim(id, epoch, &policy)
}
func savedAccountBinding(w Work) (providerexec.SavedTaskBinding, error) {
	if !validSavedAccountPolicy(w.SavedAccountPolicy) || w.Account == nil || w.AccountLease == nil || w.AccountLease.Scope != savedAccountLeaseScope || w.ContextProtocolVersion != 1 || w.RuntimeContext == nil || w.ExecutorPolicyVersion != 1 || w.CurrentAttemptLease == nil || w.CoreProfilePin != nil || w.OwnerUserID != "local-owner" || w.SpaceID != "local-space" || w.RequesterPin != nil || w.Adapter != nil || w.Collaboration != nil || w.TaskLink != nil {
		return providerexec.SavedTaskBinding{}, ErrConflict
	}
	a, l := w.Account, w.AccountLease
	b := providerexec.SavedTaskBinding{AccountID: a.AccountID, AccountRevision: int64(a.Revision), CredentialGeneration: int64(a.CredentialGeneration), Provider: a.Provider, Endpoint: a.Endpoint, Model: a.Model, RequestID: w.RequestID, AttemptID: w.AttemptID, Epoch: int64(w.Epoch), LeaseFence: int64(l.Fence), ContextSHA256: w.RuntimeContextSHA256, ConsentID: w.RequestID, ConsentRevision: 1, ExpiresAtUnixMS: mustExpiry(l).UnixMilli(), Limits: w.SavedAccountPolicy.Limits}
	if _, e := providerexec.SavedTaskBindingHash(b); e != nil {
		return providerexec.SavedTaskBinding{}, ErrInvalid
	}
	return b, nil
}
func (s *Store) savedAccountTaskBindingLocked(id, attempt string, epoch uint64) (providerexec.SavedTaskBinding, error) {
	w, ok := s.work[id]
	if !ok {
		return providerexec.SavedTaskBinding{}, ErrNotFound
	}
	if s.closed || s.poisoned || s.readOnly || w.AttemptID != attempt || w.Epoch != epoch || (w.Status != "starting" && w.Status != "running") || w.DesiredState != "running" || w.RuntimeStopped || !s.validNewRuntimeContext(w) || !s.contextAllowed(w) {
		return providerexec.SavedTaskBinding{}, ErrConflict
	}
	b, e := savedAccountBinding(w)
	if e != nil {
		return b, e
	}
	release, e := s.executionPrincipalGuard(w)
	if e != nil {
		return providerexec.SavedTaskBinding{}, e
	}
	defer release()
	releaseAccount, e := s.accountGuard(w, true)
	if e != nil {
		return providerexec.SavedTaskBinding{}, e
	}
	defer releaseAccount()
	if !s.executorAllowed(w) {
		return providerexec.SavedTaskBinding{}, ErrConflict
	}
	return b, nil
}
func (s *Store) SavedAccountTaskBinding(id, attempt string, epoch uint64) (providerexec.SavedTaskBinding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.savedAccountTaskBindingLocked(id, attempt, epoch)
}

// CurrentSavedAccountTask is a live witness check, not a durable authorization.
func (s *Store) CurrentSavedAccountTask(binding providerexec.SavedTaskBinding) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if binding.Epoch < 1 {
		return false
	}
	current, e := s.savedAccountTaskBindingLocked(binding.RequestID, binding.AttemptID, uint64(binding.Epoch))
	return e == nil && current == binding
}
