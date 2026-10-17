package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"nekobot.local/native/internal/connections"
	"nekobot.local/native/internal/ledger"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func launchFixture(t *testing.T) (*Manager, ledger.Work, *connections.Store, connections.Account, savedAccountLaunchPaths) {
	t.Helper()
	m, old, _, _, _ := savedSessionFixture(t)
	base := t.TempDir()
	m.state = base
	catalog := filepath.Join(base, "connections")
	c, e := connections.Open(catalog, savedSessionVault{})
	if e != nil {
		t.Fatal(e)
	}
	a, _, e := c.SaveAccount(connections.AccountInput{ClientNonce: "launch-account-create", Name: "Synthetic", Provider: "openai-compatible", Endpoint: "https://example.invalid/v1", Model: "synthetic-model", Secret: "synthetic-session-material"})
	if e != nil {
		t.Fatal(e)
	}
	m.store.EnableAccounts(c)
	w, _, e := m.store.AcceptAccount(ledger.Input{BotID: "bot-meiiie", ClientNonce: "launch-request-create", Text: "Synthetic task", Lane: "foreground"}, a.ID, a.Revision)
	if e != nil {
		t.Fatal(e)
	}
	w, e = m.store.ClaimSavedAccount(w.RequestID, w.Epoch, *old.SavedAccountPolicy)
	if e != nil {
		t.Fatal(e)
	}
	m.tasks = filepath.Join(base, "tasks")
	root := filepath.Join(m.tasks, w.RequestID)
	private := filepath.Join(base, "saved-accounts", w.RequestID, w.AttemptID)
	for _, p := range []string{filepath.Join(root, ".core-home"), private} {
		if e := os.MkdirAll(p, 0700); e != nil {
			t.Fatal(e)
		}
	}
	return m, w, c, a, savedAccountLaunchPaths{ConnectionsRoot: catalog, PrivateRoot: filepath.Join(base, "saved-accounts")}
}
func TestSavedAccountLaunchStaleDispatchAndPrivateEnvelope(t *testing.T) {
	for _, change := range []string{"none", "rotation", "removal", "cancel", "context", "transport", "post_transport_rotation"} {
		t.Run(change, func(t *testing.T) {
			m, w, c, a, paths := launchFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			launch, e := m.prepareSavedAccountLaunch(ctx, w, c, paths)
			if e != nil {
				t.Fatal(e)
			}
			defer launch.Close()
			selection, e := c.SelectConsumer(*w.Account)
			if e != nil {
				t.Fatal(e)
			}
			for _, value := range []any{selection, w, m.store.Snapshot(), launch} {
				raw, e := json.Marshal(value)
				if e != nil {
					t.Fatal(e)
				}
				if strings.Contains(string(raw), "secretRef") || strings.Contains(string(raw), "secret-") || strings.Contains(string(raw), "revisionKey") || strings.Contains(string(raw), "synthetic-session-material") {
					t.Fatal("public serialization exposed private material")
				}
			}
			switch change {
			case "rotation":
				_, _, e = c.SaveAccount(connections.AccountInput{ID: a.ID, ExpectedRevision: a.Revision, ClientNonce: "launch-rotate", Name: a.Name, Provider: a.Provider, Endpoint: a.Endpoint, Model: a.Model, Secret: "synthetic-replacement-material"})
			case "removal":
				e = c.RemoveAccount(a.ID, a.Revision)
			case "cancel":
				_, e = m.store.Cancel(w.RequestID)
			case "context":
				cancel()
			}
			if e != nil {
				t.Fatal(e)
			}
			calls := 0
			var retained []byte
			e = launch.dispatch(func(ctx context.Context, frame []byte) error {
				calls++
				retained = frame
				var envelope struct {
					Version int    `json:"version"`
					Purpose string `json:"purpose"`
					Request struct {
						Binding struct {
							AccountID string `json:"accountId"`
						} `json:"binding"`
						SecretRef   string `json:"secretRef"`
						RevisionKey string `json:"revisionKey"`
					} `json:"request"`
				}
				if json.Unmarshal(frame, &envelope) != nil || envelope.Version != 1 || envelope.Purpose != "saved_account_launch_v1" || envelope.Request.Binding.AccountID != a.ID || !strings.HasPrefix(envelope.Request.SecretRef, "secret-") || len(envelope.Request.RevisionKey) != 64 || len(frame) > 64000 || strings.Contains(string(frame), "synthetic-session-material") {
					t.Fatal("invalid private envelope")
				}
				if change == "post_transport_rotation" {
					_, _, err := c.SaveAccount(connections.AccountInput{ID: a.ID, ExpectedRevision: a.Revision, ClientNonce: "launch-post-rotate", Name: a.Name, Provider: a.Provider, Endpoint: a.Endpoint, Model: a.Model, Secret: "synthetic-post-rotation-material"})
					return err
				}
				if change == "transport" {
					return errors.New("synthetic transport failure")
				}
				return nil
			})
			if change == "none" {
				if e != nil || calls != 1 {
					t.Fatal("valid dispatch failed", e, calls)
				}
			} else {
				if e == nil {
					t.Fatal("invalid dispatch accepted")
				}
				want := 0
				if change == "transport" || change == "post_transport_rotation" {
					want = 1
				}
				if calls != want {
					t.Fatal("stale account reached transport")
				}
				if readSessionWitness(t, launch.witnessPath).State != "retired" {
					t.Fatal("failure retained witness")
				}
			}
			for _, b := range retained {
				if b != 0 {
					t.Fatal("private transport buffer retained")
				}
			}
			if e := launch.dispatch(func(context.Context, []byte) error { t.Fatal("duplicate dispatch"); return nil }); e == nil {
				t.Fatal("dispatch repeated")
			}
		})
	}
}

func TestSavedAccountLaunchRejectsPathsAndCallerAuthority(t *testing.T) {
	for _, kind := range []string{"catalog", "private", "existing_capsule", "symlink", "stale_attempt", "caller_copy"} {
		t.Run(kind, func(t *testing.T) {
			m, w, c, _, paths := launchFixture(t)
			private := filepath.Join(paths.PrivateRoot, w.RequestID, w.AttemptID)
			switch kind {
			case "catalog":
				paths.ConnectionsRoot = t.TempDir()
			case "private":
				paths.PrivateRoot = filepath.Join(m.tasks, w.RequestID)
			case "existing_capsule":
				if e := os.WriteFile(filepath.Join(private, "capsule.json"), []byte("synthetic"), 0600); e != nil {
					t.Fatal(e)
				}
			case "symlink":
				other := t.TempDir()
				if e := os.Remove(private); e != nil {
					t.Fatal(e)
				}
				if e := os.Symlink(other, private); e != nil {
					t.Skip("symlink unavailable")
				}
			case "stale_attempt":
				w.AttemptID = "attempt-stale"
			case "caller_copy":
				pin := *w.Account
				pin.Model = "forged"
				w.Account = &pin
				lease := *w.CurrentAttemptLease
				lease.AuthoritySHA256 = strings.Repeat("f", 64)
				w.CurrentAttemptLease = &lease
				w.Text = "forged"
			}
			launch, e := m.prepareSavedAccountLaunch(context.Background(), w, c, paths)
			if kind == "caller_copy" {
				if e != nil {
					t.Fatal(e)
				}
				defer launch.Close()
				if strings.Contains(string(launch.frame), "forged") {
					t.Fatal("caller copy used as authority")
				}
				return
			}
			if e == nil {
				launch.Close()
				t.Fatal("invalid launch prepared")
			}
			if _, e := os.Stat(filepath.Join(private, "witness.json")); !os.IsNotExist(e) {
				t.Fatal("rejected assembly issued witness")
			}
		})
	}
}
