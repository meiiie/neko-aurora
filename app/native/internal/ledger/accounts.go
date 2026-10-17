package ledger

import (
	"errors"
	"nekobot.local/native/internal/connections"
	"reflect"
	"strings"
	"time"
)

type AccountLease struct {
	Version              int    `json:"version"`
	ID                   string `json:"id"`
	RequestID            string `json:"requestId"`
	BotID                string `json:"botId"`
	OwnerUserID          string `json:"ownerUserId"`
	SpaceID              string `json:"spaceId"`
	Audience             string `json:"audience"`
	AttemptID            string `json:"attemptId"`
	Epoch                uint64 `json:"epoch"`
	Fence                uint64 `json:"fence"`
	IssuedAt             string `json:"issuedAt"`
	ExpiresAt            string `json:"expiresAt"`
	Scope                string `json:"scope"`
	AccountID            string `json:"accountId"`
	AccountRevision      uint64 `json:"accountRevision"`
	CredentialGeneration uint64 `json:"credentialGeneration"`
	ContextSHA256        string `json:"contextSha256"`
}

func (s *Store) EnableAccounts(accounts *connections.Store) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accounts = accounts
}

func (s *Store) AcceptAccount(in Input, accountID string, revision uint64) (Work, bool, error) {
	return s.acceptAccountScoped(in, "", accountID, revision)
}
func (s *Store) acceptAccountScoped(in Input, channelID, accountID string, revision uint64) (Work, bool, error) {
	if in.Lane == "" {
		in.Lane = "foreground"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.accounts == nil {
		return Work{}, false, ErrConflict
	}
	// The nonce receipt is checked first so an ACK lost before removal still returns
	// the original work, without recreating authority or consulting today's card.
	if id, ok := s.nonces[inputNonceKey(in, channelID)]; ok {
		old := s.work[id]
		if old.PluginSelection != nil || old.Resources != nil || old.Adapter != nil || old.Account == nil || old.Account.AccountID != accountID || old.Account.Revision != revision || old.Text != in.Text || old.Lane != in.Lane || (channelID != "" && old.ConversationID != channelID) {
			return Work{}, false, ErrConflict
		}
		return copyWork(old), true, nil
	}
	var w Work
	var duplicate bool
	err := s.accounts.WithSelection(accountID, revision, func(pin connections.Pin) error {
		var err error
		w, duplicate, err = s.acceptLocked(in, &pin, channelID)
		return err
	})
	if err != nil {
		if errors.Is(err, connections.ErrConflict) {
			return Work{}, false, ErrConflict
		}
		return Work{}, false, err
	}
	return w, duplicate, nil
}

func validAccountLease(w Work) bool {
	l := w.AccountLease
	if l == nil || w.Account == nil || !connections.ValidPin(*w.Account) {
		return false
	}
	issued, e1 := time.Parse(time.RFC3339Nano, l.IssuedAt)
	expiry, e2 := time.Parse(time.RFC3339Nano, l.ExpiresAt)
	if l.AccountID != w.Account.AccountID || l.AccountRevision != w.Account.Revision || l.CredentialGeneration != w.Account.CredentialGeneration || l.ContextSHA256 != contextHash(w) {
		return false
	}
	return e1 == nil && e2 == nil && expiry.After(issued) && expiry.Sub(issued) <= 10*time.Minute && l.Version == 1 && strings.HasPrefix(l.ID, "lease-") && lowerHex(strings.TrimPrefix(l.ID, "lease-"), 32) && l.RequestID == w.RequestID && l.BotID == w.BotID && l.OwnerUserID == w.OwnerUserID && l.SpaceID == w.SpaceID && l.Audience == w.ConversationID && l.AttemptID == w.AttemptID && l.Epoch == w.Epoch && l.Fence > 0 && ((l.Scope == "local-controlled-fixture" && w.SavedAccountPolicy == nil) || (l.Scope == savedAccountLeaseScope && validSavedAccountPolicy(w.SavedAccountPolicy) && expiry.Sub(issued) == time.Duration(w.SavedAccountPolicy.Limits.MaxSeconds)*time.Second))
}
func (s *Store) accountGuard(w Work, lease bool) (func(), error) {
	if s.closed || s.poisoned {
		return nil, ErrUnavailable
	}
	if w.Account == nil {
		return func() {}, nil
	}
	if w.AccountExpiredAt != "" {
		return nil, ErrConflict
	}
	if w.AccountLease != nil && !s.clock().Before(mustExpiry(w.AccountLease)) {
		if err := s.expireAccount(w, s.clock()); err != nil {
			return nil, err
		}
		return nil, ErrConflict
	}
	if s.accounts == nil || (lease && (!validAccountLease(w) || !s.clock().Before(mustExpiry(w.AccountLease)) || s.clock().Before(mustIssued(w.AccountLease)))) {
		return nil, ErrConflict
	}
	release, err := s.accounts.AcquirePin(*w.Account)
	if err != nil {
		return nil, ErrConflict
	}
	return release, nil
}
func contextHash(w Work) string {
	if w.Context == nil {
		return ""
	}
	return w.Context.SHA256
}
func mustIssued(l *AccountLease) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, l.IssuedAt)
	return t
}
func mustExpiry(l *AccountLease) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, l.ExpiresAt)
	return t
}
func (s *Store) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now().UTC()
}
func (s *Store) accountAllowed(w Work) bool {
	release, err := s.accountGuard(w, w.Status == "starting" || w.Status == "running")
	if err != nil {
		return false
	}
	release()
	return true
}
func (s *Store) ReconcileAccounts() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, old := range s.sortedWorkLocked() {
		if old.Account == nil || !suspendible(old) || s.accountAllowed(old) {
			continue
		}
		w := copyWork(old)
		old = s.work[old.RequestID]
		if !suspendible(old) {
			continue
		}
		w = copyWork(old)
		w.Status = "suspending"
		if old.RuntimeStopped || (old.Status == "waiting_host" && old.AttemptID == "") {
			w.Status = "suspended"
		}
		w.DesiredState = "suspended"
		w.Epoch++
		w.Failure = "account_authority_changed"
		if err := s.commit(Event{Type: "work.account_suspended", Work: w}); err != nil {
			return err
		}
	}
	return nil
}
func validAccountSuspension(old, w Work) bool {
	if old.Account == nil || !suspendible(old) || w.Epoch != old.Epoch+1 || w.DesiredState != "suspended" || w.Failure != "account_authority_changed" {
		return false
	}
	status := "suspending"
	if old.RuntimeStopped || (old.Status == "waiting_host" && old.AttemptID == "") {
		status = "suspended"
	}
	return w.Status == status
}
func sameAccount(a, b Work) bool { return reflect.DeepEqual(a.Account, b.Account) }

func expiredAccount(w Work, observed time.Time) Work {
	w = copyWork(w)
	w.AccountExpiredAt = observed.UTC().Format(time.RFC3339Nano)
	if suspendible(w) {
		status := "suspending"
		if w.RuntimeStopped || (w.Status == "waiting_host" && w.AttemptID == "") {
			status = "suspended"
		}
		w.Status = status
		w.DesiredState = "suspended"
		w.Epoch++
		w.Failure = "account_lease_expired"
	}
	return w
}
func (s *Store) expireAccount(w Work, observed time.Time) error {
	if w.AccountLease == nil || w.AccountExpiredAt != "" {
		return ErrConflict
	}
	return s.commit(Event{Type: "work.account_expired", Work: expiredAccount(w, observed)})
}
func validAccountExpiration(old, w Work) bool {
	if old.Account == nil || old.AccountLease == nil || old.AccountExpiredAt != "" {
		return false
	}
	t, err := time.Parse(time.RFC3339Nano, w.AccountExpiredAt)
	return err == nil && !t.Before(mustExpiry(old.AccountLease)) && reflect.DeepEqual(expiredAccount(old, t), w)
}

func (s *Store) Dispatch(id string, epoch uint64, launch func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.poisoned {
		return ErrUnavailable
	}
	w, ok := s.work[id]
	if !ok || w.Epoch != epoch || w.Status != "starting" || w.DesiredState != "running" || !s.contextAllowed(w) {
		return ErrConflict
	}
	if w.CoreProfileGroupBinding != nil {
		if err := s.coreProfileGuard(w); err != nil {
			return err
		}
		if !s.collaborationBudgetAllows(w) || !s.contextAllowed(w) {
			return ErrConflict
		}
	}
	release, err := s.accountGuard(w, true)
	if err != nil {
		return err
	}
	defer release()
	return s.withExecutor(w, launch)
}
