package ledger

import (
	"encoding/json"
	"errors"
	"nekoaurora.local/native/internal/connections"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type authorityVault struct{}

func (authorityVault) Seal(string, string, []byte) error          { return nil }
func (authorityVault) Match(string, string, []byte) (bool, error) { return true, nil }

func accountFixture(t *testing.T) (*Store, *connections.Store, connections.Account, connections.AccountInput, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	c, err := connections.Open(filepath.Join(dir, "connections"), authorityVault{})
	if err != nil {
		t.Fatal(err)
	}
	in := connections.AccountInput{ClientNonce: "authority-account-create", Name: "Local Fixture", Provider: "openai-compatible", Endpoint: "https://example.invalid/v1", Model: "fixture-model", Secret: "fixture-only-canary-not-a-key"}
	a, _, err := c.SaveAccount(in)
	if err != nil {
		t.Fatal(err)
	}
	s.EnableAccounts(c)
	return s, c, a, in, dir
}
func acceptBound(t *testing.T, s *Store, a connections.Account, nonce string) Work {
	t.Helper()
	w, _, err := s.AcceptAccount(input(nonce, "background"), a.ID, a.Revision)
	if err != nil {
		t.Fatal(err)
	}
	return w
}
func admitBound(t *testing.T, s *Store, w Work) Work {
	t.Helper()
	w, err := s.Claim(w.RequestID, w.Epoch)
	if err != nil {
		t.Fatal(err)
	}
	b := CoreBinding{NativeProcessID: 401, SessionID: strings.Repeat("a", 32), Task: json.RawMessage(`{"id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","root":"/fixture","activationEpoch":1,"activationId":"cccccccccccccccccccccccccccccccc"}`), Execution: json.RawMessage(`{"version":1,"authorityId":"fixture-authority"}`), CoreSHA256: "ac643e3440ccf8a77e4451688f585643a646d4ba798b128a2c5c89a929c0dab5"}
	if err = s.BindRuntimeAttempt(w.RequestID, w.AttemptID, b); err != nil {
		t.Fatal(err)
	}
	if err = s.Admit(w.RequestID, w.Epoch, b); err != nil {
		t.Fatal(err)
	}
	w, _ = s.Get(w.RequestID)
	return w
}
func TestAccountQueuedRevokeNonceAndRestartNeverFallback(t *testing.T) {
	s, c, a, _, dir := accountFixture(t)
	w := acceptBound(t, s, a, "account-queued-work")
	if err := c.RemoveAccount(a.ID, a.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Claim(w.RequestID, w.Epoch); !errors.Is(err, ErrConflict) {
		t.Fatal("revoked queued task dispatched", err)
	}
	if err := s.ReconcileAccounts(); err != nil {
		t.Fatal(err)
	}
	current, _ := s.Get(w.RequestID)
	if current.Status != "suspended" || current.Failure != "account_authority_changed" {
		t.Fatal("queued authority not suspended", current.Status)
	}
	duplicate, dup, err := s.AcceptAccount(input("account-queued-work", "background"), a.ID, a.Revision)
	if err != nil || !dup || duplicate.RequestID != w.RequestID {
		t.Fatal("lost ACK did not preserve identity", err)
	}
	if _, _, err := s.Accept(input("account-queued-work", "background")); !errors.Is(err, ErrConflict) {
		t.Fatal("retry removed account binding")
	}
	s.Close()
	for i := 0; i < 3; i++ {
		restored, err := Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		if err = restored.RecoverInterrupted(); err != nil {
			t.Fatal("deleted account poisoned historical replay", err)
		}
		got, _ := restored.Get(w.RequestID)
		if got.Status != "suspended" || got.RequestID != w.RequestID || got.Account.AccountID != a.ID {
			t.Fatal("restart revived authority")
		}
		restored.Close()
	}
}
func TestAccountActiveRevokeFencesOutputEffectCompletionButKeepsStopReceipt(t *testing.T) {
	s, c, a, _, dir := accountFixture(t)
	w := admitBound(t, s, acceptBound(t, s, a, "account-active-work"))
	if err := s.AppendText(w.RequestID, w.Epoch, "before revoke"); err != nil {
		t.Fatal(err)
	}
	if err := c.RemoveAccount(a.ID, a.Revision); err != nil {
		t.Fatal(err)
	}
	for _, action := range []func() error{func() error { return s.AppendText(w.RequestID, w.Epoch, "forbidden late") }, func() error { _, e := s.ExecuteTool(w.RequestID, w.Epoch, invocation("forbidden late file")); return e }, func() error { return s.Finish(w.RequestID, w.Epoch, "completed", "") }} {
		if err := action(); !errors.Is(err, ErrConflict) {
			t.Fatal("revoked action accepted", err)
		}
	}
	if err := s.ReconcileAccounts(); err != nil {
		t.Fatal(err)
	}
	current, _ := s.Get(w.RequestID)
	if current.Status != "suspending" || current.Epoch != w.Epoch+1 {
		t.Fatal("active fence missing")
	}
	if err := s.MarkRuntimeStopped(w.RequestID, w.AttemptID, StoppedReceipt{Version: 1, NativeProcessID: 401, SessionID: strings.Repeat("a", 32)}); err != nil {
		t.Fatal("revocation prevented stop receipt", err)
	}
	if err := s.Finish(w.RequestID, current.Epoch, "suspended", "account_authority_changed"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RefreshContext(RefreshInput{RequestID: w.RequestID, ExpectedEpoch: current.Epoch}); !errors.Is(err, ErrConflict) {
		t.Fatal("refresh revived removed account")
	}
	bytes, _ := os.ReadFile(filepath.Join(dir, "transactions.jsonl"))
	if strings.Contains(string(bytes), "fixture-only-canary") || strings.Contains(string(bytes), "secretRef") {
		t.Fatal("credential leaked to ledger")
	}
	if strings.Contains(w.RuntimePrompt(), a.ID) || strings.Contains(w.RuntimePrompt(), a.Endpoint) {
		t.Fatal("account descriptor entered model prompt")
	}
}
func TestAccountUpdateAndRotationFencePinnedRevision(t *testing.T) {
	for _, rotate := range []bool{false, true} {
		t.Run(map[bool]string{false: "metadata", true: "credential"}[rotate], func(t *testing.T) {
			s, c, a, in, _ := accountFixture(t)
			w := acceptBound(t, s, a, "account-update-work")
			in.ID, in.ExpectedRevision, in.ClientNonce, in.Secret = a.ID, a.Revision, "account-update-fixture", ""
			if rotate {
				in.Secret = "fixture-only-rotated-key"
			}
			next, _, err := c.SaveAccount(in)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.Claim(w.RequestID, w.Epoch); !errors.Is(err, ErrConflict) {
				t.Fatal("old revision dispatched", err)
			}
			fresh := acceptBound(t, s, next, "account-fresh-work")
			if _, err = s.Claim(fresh.RequestID, fresh.Epoch); err != nil {
				t.Fatal("explicit fresh selection failed", err)
			}
		})
	}
}
func TestAccountLeaseExpiryAndAttemptFence(t *testing.T) {
	s, _, a, _, _ := accountFixture(t)
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	w := admitBound(t, s, acceptBound(t, s, a, "account-expiry-work"))
	if !validAccountLease(w) || w.AccountLease.Fence != 1 {
		t.Fatal("lease binding missing")
	}
	if err := s.Finish(w.RequestID, w.Epoch, "recovery_required", "fixture_interruption"); err != nil {
		t.Fatal(err)
	}
	requeued, err := s.Requeue(w.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Claim(requeued.RequestID, requeued.Epoch)
	if err != nil {
		t.Fatal(err)
	}
	if second.AccountLease.ID == w.AccountLease.ID || second.AccountLease.Fence != 2 || second.AccountLease.AttemptID == w.AccountLease.AttemptID {
		t.Fatal("claim reused lease")
	}
	if err = s.AppendText(w.RequestID, w.Epoch, "old attempt"); !errors.Is(err, ErrConflict) {
		t.Fatal("old epoch accepted")
	}
	now = now.Add(11 * time.Minute)
	if s.RuntimeAllowed(second.RequestID, second.Epoch) {
		t.Fatal("expired lease live")
	}
	if err = s.ReconcileAccounts(); err != nil {
		t.Fatal(err)
	}
	current, _ := s.Get(second.RequestID)
	if current.Status != "suspending" {
		t.Fatal("expiry not suspended")
	}
}
func TestAccountPublicationGuardLinearizesBeforeRemoval(t *testing.T) {
	s, c, a, _, _ := accountFixture(t)
	w := acceptBound(t, s, a, "account-guard-work")
	entered, release, removed := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.mu.Lock()
		defer s.mu.Unlock()
		unlock, err := s.accountGuard(w, false)
		if err != nil {
			t.Error(err)
			close(entered)
			return
		}
		defer unlock()
		close(entered)
		<-release
	}()
	<-entered
	go func() { removed <- c.RemoveAccount(a.ID, a.Revision) }()
	select {
	case e := <-removed:
		t.Fatal("remove crossed held publication guard", e)
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	wg.Wait()
	if err := <-removed; err != nil {
		t.Fatal(err)
	}
	if _, err := s.Claim(w.RequestID, w.Epoch); !errors.Is(err, ErrConflict) {
		t.Fatal("action after revoke ACK accepted")
	}
}

func TestAccountObservedExpiryIsDurableAndCannotRenew(t *testing.T) {
	for _, recovering := range []bool{false, true} {
		t.Run(map[bool]string{false: "running", true: "recovery"}[recovering], func(t *testing.T) {
			s, c, a, _, dir := accountFixture(t)
			now := time.Now().UTC()
			s.now = func() time.Time { return now }
			w := admitBound(t, s, acceptBound(t, s, a, "account-sticky-expiry"))
			if recovering {
				if err := s.Finish(w.RequestID, w.Epoch, "recovery_required", "fixture_interruption"); err != nil {
					t.Fatal(err)
				}
			}
			now = now.Add(11 * time.Minute)
			if recovering {
				if err := s.ReconcileAccounts(); err != nil {
					t.Fatal(err)
				}
			} else if s.RuntimeAllowed(w.RequestID, w.Epoch) {
				t.Fatal("expiry did not deny runtime")
			}
			current, _ := s.Get(w.RequestID)
			if current.AccountExpiredAt == "" {
				t.Fatal("observed expiry not durable")
			}
			now = now.Add(-10 * time.Minute)
			if s.RuntimeAllowed(w.RequestID, current.Epoch) {
				t.Fatal("clock rewind revived expired lease")
			}
			if _, err := s.Requeue(w.RequestID); !errors.Is(err, ErrConflict) {
				t.Fatal("expired recovery renewed")
			}
			if err := s.MarkRuntimeStopped(w.RequestID, w.AttemptID, StoppedReceipt{Version: 1, NativeProcessID: 401, SessionID: w.Core.SessionID}); err != nil {
				t.Fatal(err)
			}
			if err := s.Finish(w.RequestID, current.Epoch, "suspended", "account_lease_expired"); err != nil {
				t.Fatal(err)
			}
			if _, err := s.RefreshContext(RefreshInput{w.RequestID, current.Epoch}); !errors.Is(err, ErrConflict) {
				t.Fatal("expired context refresh renewed")
			}
			s.Close()
			restored, err := Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer restored.Close()
			restored.EnableAccounts(c)
			restored.now = func() time.Time { return now }
			if err = restored.RecoverInterrupted(); err != nil {
				t.Fatal(err)
			}
			got, _ := restored.Get(w.RequestID)
			if got.AccountExpiredAt == "" || restored.RuntimeAllowed(w.RequestID, got.Epoch) {
				t.Fatal("restart revived observed expiry")
			}
		})
	}
}
