package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"nekobot.local/native/internal/connections"
	"nekobot.local/native/internal/executors"
	"nekobot.local/native/internal/ledger"
	"nekobot.local/native/internal/providerexec"
)

type savedSessionVault struct{}

func (savedSessionVault) Seal(string, string, []byte) error          { return nil }
func (savedSessionVault) Match(string, string, []byte) (bool, error) { return true, nil }

func savedSessionFixture(t *testing.T) (*Manager, ledger.Work, *connections.Store, connections.Account, *executors.Registry) {
	t.Helper()
	root := t.TempDir()
	s, e := ledger.Open(filepath.Join(root, "journal"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	r, e := executors.Open(filepath.Join(root, "executor"), executors.Descriptor{HostSHA256: strings.Repeat("1", 64), CoreSHA256: "ac643e3440ccf8a77e4451688f585643a646d4ba798b128a2c5c89a929c0dab5", RootSHA256: strings.Repeat("3", 64), CapabilitiesSHA256: strings.Repeat("4", 64)})
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
	c, e := connections.Open(filepath.Join(root, "connections"), savedSessionVault{})
	if e != nil {
		t.Fatal(e)
	}
	a, _, e := c.SaveAccount(connections.AccountInput{ClientNonce: "session-account-create", Name: "Synthetic", Provider: "openai-compatible", Endpoint: "https://example.invalid/v1", Model: "synthetic-model", Secret: "synthetic-session-material"})
	if e != nil {
		t.Fatal(e)
	}
	s.EnableAccounts(c)
	w, _, e := s.AcceptAccount(ledger.Input{BotID: "bot-meiiie", ClientNonce: "session-request-create", Text: "Synthetic task", Lane: "foreground"}, a.ID, a.Revision)
	if e != nil {
		t.Fatal(e)
	}
	w, e = s.ClaimSavedAccount(w.RequestID, w.Epoch, ledger.SavedAccountPolicy{Version: 1, Limits: providerexec.SavedTaskLimits{MaxInputBytes: 64000, MaxOutputTokens: 4096, MaxRequests: 8, MaxSteps: 6, MaxSeconds: 600}})
	if e != nil {
		t.Fatal(e)
	}
	return &Manager{store: s}, w, c, a, r
}
func readSessionWitness(t *testing.T, path string) providerexec.WitnessRecord {
	t.Helper()
	raw, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	var w providerexec.WitnessRecord
	if json.Unmarshal(raw, &w) != nil {
		t.Fatal("invalid witness")
	}
	return w
}
func TestSavedAccountSessionLifecycle(t *testing.T) {
	for _, change := range []string{"close", "cancel_context", "cancel_work", "rotate_account", "revoke_executor"} {
		t.Run(change, func(t *testing.T) {
			m, w, c, a, r := savedSessionFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			path := filepath.Join(t.TempDir(), "witness.json")
			session, e := m.startSavedAccountSession(ctx, w, path, strings.Repeat("6", 64))
			if e != nil {
				t.Fatal(e)
			}
			defer session.Close()
			first := readSessionWitness(t, path)
			if first.State != "active" || first.ServiceGeneration != w.CurrentAttemptLease.BootID || first.AuthoritySHA256 != w.CurrentAttemptLease.AuthoritySHA256 {
				t.Fatal("wrong attempt authority")
			}
			switch change {
			case "close":
				if e = session.Close(); e != nil {
					t.Fatal(e)
				}
			case "cancel_context":
				cancel()
			case "cancel_work":
				if _, e = m.store.Cancel(w.RequestID); e != nil {
					t.Fatal(e)
				}
			case "rotate_account":
				_, _, e = c.SaveAccount(connections.AccountInput{ID: a.ID, ExpectedRevision: a.Revision, ClientNonce: "session-account-rotate", Name: a.Name, Provider: a.Provider, Endpoint: a.Endpoint, Model: a.Model, Secret: "synthetic-rotated-material"})
				if e != nil {
					t.Fatal(e)
				}
			case "revoke_executor":
				snap := r.Snapshot()
				if _, _, e = r.Revoke(snap.Pin, "synthetic-revocation"); e != nil {
					t.Fatal(e)
				}
			}
			select {
			case <-session.Context().Done():
			case <-time.After(3 * time.Second):
				t.Fatal("authority loss did not cancel session")
			}
			if e = session.Close(); e != nil {
				t.Fatal(e)
			}
			last := readSessionWitness(t, path)
			if last.State != "retired" || last.Revision <= first.Revision {
				t.Fatal("witness not retired")
			}
		})
	}
}
func TestSavedAccountSessionRejectsStaleOrCancelledStart(t *testing.T) {
	m, w, _, _, _ := savedSessionFixture(t)
	for _, kind := range []string{"attempt", "epoch", "context"} {
		t.Run(kind, func(t *testing.T) {
			stale := w
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch kind {
			case "attempt":
				stale.AttemptID = "attempt-stale"
			case "epoch":
				stale.Epoch++
			case "context":
				cancel()
			}
			path := filepath.Join(t.TempDir(), "witness.json")
			if session, e := m.startSavedAccountSession(ctx, stale, path, strings.Repeat("6", 64)); e == nil {
				session.Close()
				t.Fatal("stale authority accepted")
			}
			if _, e := os.Stat(path); !os.IsNotExist(e) {
				t.Fatal("rejected start wrote witness")
			}
		})
	}
}
func TestSavedAccountExecuteCannotFallBackToFixture(t *testing.T) {
	m, w, _, _, _ := savedSessionFixture(t)
	m.config = Config{HostExecutable: filepath.Join(t.TempDir(), "must-not-launch"), FixtureEndpoint: "http://127.0.0.1:1/v1"}
	m.execute(context.Background(), w)
	got, e := m.store.Get(w.RequestID)
	if e != nil || got.Failure != "saved_account_runtime_unavailable" {
		t.Fatal("saved account used legacy launch", got.Failure, e)
	}
}

func TestSavedAccountSessionRenewsWithoutChangingBindingOrBudget(t *testing.T) {
	m, w, _, _, _ := savedSessionFixture(t)
	// A caller's copied lease is not witness authority.
	altered := *w.CurrentAttemptLease
	altered.BootID = "executor-boot-" + strings.Repeat("f", 32)
	altered.AuthoritySHA256 = strings.Repeat("f", 64)
	w.CurrentAttemptLease = &altered
	path := filepath.Join(t.TempDir(), "witness.json")
	session, e := m.startSavedAccountSession(context.Background(), w, path, strings.Repeat("6", 64))
	if e != nil {
		t.Fatal(e)
	}
	defer session.Close()
	before := readSessionWitness(t, path)
	deadline := time.Now().Add(3 * time.Second)
	for {
		after := readSessionWitness(t, path)
		if after.Revision > before.Revision {
			if after.BindingSHA256 != before.BindingSHA256 || after.AuthoritySHA256 != before.AuthoritySHA256 || after.ServiceGeneration == altered.BootID || after.ValidUntilUnixMS <= before.ValidUntilUnixMS {
				t.Fatal("renewal changed authority or did not renew")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("witness not renewed")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if next, e := m.startSavedAccountSession(context.Background(), w, path, strings.Repeat("6", 64)); e == nil {
		next.Close()
		t.Fatal("existing witness adopted")
	}
	current, e := m.store.Get(w.RequestID)
	if e != nil || current.SavedAccountPolicy.Limits.MaxRequests != 8 || current.AttemptID != w.AttemptID {
		t.Fatal("renewal rewrote durable work")
	}
	if e = session.Close(); e != nil {
		t.Fatal(e)
	}
	if next, e := m.startSavedAccountSession(context.Background(), w, path, strings.Repeat("6", 64)); e == nil {
		next.Close()
		t.Fatal("retired witness resurrected")
	}
}

func TestSavedAccountSessionWitnessLossCancelsAndReportsRetirementFailure(t *testing.T) {
	m, w, _, _, _ := savedSessionFixture(t)
	path := filepath.Join(t.TempDir(), "witness.json")
	session, e := m.startSavedAccountSession(context.Background(), w, path, strings.Repeat("6", 64))
	if e != nil {
		t.Fatal(e)
	}
	defer session.Close()
	if e = os.Remove(path); e != nil {
		t.Fatal(e)
	}
	select {
	case <-session.Context().Done():
	case <-time.After(3 * time.Second):
		t.Fatal("missing witness did not cancel")
	}
	if session.Close() == nil {
		t.Fatal("retirement failure hidden")
	}
	if _, e = os.Stat(path); !os.IsNotExist(e) {
		t.Fatal("missing witness recreated")
	}
}

func TestSavedAccountSessionConcurrentStartHasOneOwner(t *testing.T) {
	m, w, _, _, _ := savedSessionFixture(t)
	path := filepath.Join(t.TempDir(), "witness.json")
	start := make(chan struct{})
	winners := make(chan *savedAccountSession, 32)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			s, e := m.startSavedAccountSession(context.Background(), w, path, strings.Repeat("6", 64))
			if e == nil {
				winners <- s
			}
		}()
	}
	close(start)
	wg.Wait()
	close(winners)
	count := 0
	for s := range winners {
		count++
		if e := s.Close(); e != nil {
			t.Error(e)
		}
	}
	if count != 1 {
		t.Fatalf("expected one exclusive witness owner, got %d", count)
	}
}

func TestSavedAccountSessionExistingReservationIsNeverRemoved(t *testing.T) {
	m, w, _, _, _ := savedSessionFixture(t)
	path := filepath.Join(t.TempDir(), "witness.json")
	if e := os.Mkdir(path+".owner", 0700); e != nil {
		t.Fatal(e)
	}
	sentinel := filepath.Join(path+".owner", "retained")
	if e := os.WriteFile(sentinel, []byte("synthetic-owner-marker"), 0600); e != nil {
		t.Fatal(e)
	}
	if s, e := m.startSavedAccountSession(context.Background(), w, path, strings.Repeat("6", 64)); e == nil {
		s.Close()
		t.Fatal("reservation bypassed")
	}
	if b, e := os.ReadFile(sentinel); e != nil || string(b) != "synthetic-owner-marker" {
		t.Fatal("existing ownership altered")
	}
	if _, e := os.Stat(path); !os.IsNotExist(e) {
		t.Fatal("reserved witness created")
	}
}
