package ledger

import (
	"nekobot.local/native/internal/connections"
	"nekobot.local/native/internal/providerexec"
	"path/filepath"
	"testing"
)

func savedAdmissionFixture(t *testing.T, defaults bool) (*Store, *connections.Store, connections.Account, Work) {
	s, _ := executorFixture(t)
	s.ConfigureContextAdmission(true)
	c, e := connections.Open(filepath.Join(t.TempDir(), "connections"), authorityVault{})
	if e != nil {
		t.Fatal(e)
	}
	a, _, e := c.SaveAccount(connections.AccountInput{ClientNonce: "saved-account-create", Name: "Fixture", Provider: "openai-compatible", Endpoint: "https://example.invalid/v1", Model: "fixture-model", Secret: "fixture-only-canary-not-a-key"})
	if e != nil {
		t.Fatal(e)
	}
	s.EnableAccounts(c)
	scope := SubmissionScope{AccountID: a.ID, AccountRevision: a.Revision}
	if defaults {
		p := saveBotState(t, s, BotControlInput{BotID: "bot-meiiie", Action: "defaults", ClientNonce: "saved-bot-default", DefaultAccountID: a.ID, DefaultAccountRevision: a.Revision, DefaultModel: a.Model})
		scope = SubmissionScope{UseBotDefaults: true, ExpectedBotStateRevision: &p.Revision}
	}
	w, _, e := s.AcceptProduct(input("saved-task-send", "foreground"), scope)
	if e != nil {
		t.Fatal(e)
	}
	return s, c, a, w
}
func savedPolicy() SavedAccountPolicy {
	return SavedAccountPolicy{Version: 1, Limits: providerexec.SavedTaskLimits{MaxInputBytes: 64000, MaxOutputTokens: 4096, MaxRequests: 8, MaxSteps: 6, MaxSeconds: 600}}
}
func TestSavedAccountAdmissionSelectionAndCurrent(t *testing.T) {
	for _, defaults := range []bool{false, true} {
		t.Run(map[bool]string{false: "selected", true: "default"}[defaults], func(t *testing.T) {
			s, c, a, w := savedAdmissionFixture(t, defaults)
			claimed, e := s.ClaimSavedAccount(w.RequestID, w.Epoch, savedPolicy())
			if e != nil {
				t.Fatal(e)
			}
			b, e := s.SavedAccountTaskBinding(claimed.RequestID, claimed.AttemptID, claimed.Epoch)
			if e != nil {
				t.Fatal(e)
			}
			if b.AccountID != a.ID || b.CredentialGeneration != int64(a.CredentialGeneration) || b.ConsentID != w.RequestID || b.ConsentRevision != 1 || b.ContextSHA256 != claimed.RuntimeContextSHA256 || b.FixtureOnly || !s.CurrentSavedAccountTask(b) {
				t.Fatal("wrong binding")
			}
			wrong := b
			wrong.ContextSHA256 = "bad"
			if s.CurrentSavedAccountTask(wrong) {
				t.Fatal("context mismatch")
			}
			wrong = b
			wrong.AttemptID = "attempt-wrong"
			if s.CurrentSavedAccountTask(wrong) {
				t.Fatal("attempt mismatch")
			}
			if e = c.RemoveAccount(a.ID, a.Revision); e != nil {
				t.Fatal(e)
			}
			if s.CurrentSavedAccountTask(b) {
				t.Fatal("removed account current")
			}
		})
	}
}
func TestSavedAccountRejectsFixturePromotion(t *testing.T) {
	s, _, _, w := savedAdmissionFixture(t, false)
	claimed, e := s.Claim(w.RequestID, w.Epoch)
	if e != nil {
		t.Fatal(e)
	}
	if claimed.AccountLease.Scope != "local-controlled-fixture" {
		t.Fatal("legacy changed")
	}
	if _, e = s.SavedAccountTaskBinding(claimed.RequestID, claimed.AttemptID, claimed.Epoch); e == nil {
		t.Fatal("fixture promoted")
	}
}

func TestSavedAccountPolicyReplayAndRotation(t *testing.T) {
	s, c, a, w := savedAdmissionFixture(t, false)
	bad := savedPolicy()
	bad.Limits.MaxRequests = 9
	if _, e := s.ClaimSavedAccount(w.RequestID, w.Epoch, bad); e == nil {
		t.Fatal("invalid policy accepted")
	}
	claimed, e := s.ClaimSavedAccount(w.RequestID, w.Epoch, savedPolicy())
	if e != nil {
		t.Fatal(e)
	}
	b, e := s.SavedAccountTaskBinding(claimed.RequestID, claimed.AttemptID, claimed.Epoch)
	if e != nil {
		t.Fatal(e)
	}
	changed := claimed
	policy := *claimed.SavedAccountPolicy
	policy.Limits.MaxRequests--
	changed.SavedAccountPolicy = &policy
	if ExecutorAuthorityHash(changed) == ExecutorAuthorityHash(claimed) {
		t.Fatal("policy not bound")
	}
	got, _ := s.Get(claimed.RequestID)
	got.SavedAccountPolicy.Limits.MaxRequests = 1
	if !s.CurrentSavedAccountTask(b) {
		t.Fatal("public copy changed authority")
	}
	_, _, e = c.SaveAccount(connections.AccountInput{ID: a.ID, ExpectedRevision: a.Revision, ClientNonce: "saved-account-rotate", Name: a.Name, Provider: a.Provider, Endpoint: a.Endpoint, Model: a.Model, Secret: "fixture-only-rotated-key"})
	if e != nil {
		t.Fatal(e)
	}
	if s.CurrentSavedAccountTask(b) {
		t.Fatal("rotation remained current")
	}
	dir := s.dir
	s.Close()
	restored, e := Open(dir)
	if e != nil {
		t.Fatal("replay", e)
	}
	defer restored.Close()
	replayed, _ := restored.Get(claimed.RequestID)
	if replayed.SavedAccountPolicy == nil || *replayed.SavedAccountPolicy != savedPolicy() || replayed.AccountLease.Scope != savedAccountLeaseScope {
		t.Fatal("policy lost in replay")
	}
	if restored.CurrentSavedAccountTask(b) {
		t.Fatal("restart regained authority")
	}
}
func TestSavedAccountClaimRejectsStaleSelection(t *testing.T) {
	s, c, a, w := savedAdmissionFixture(t, false)
	if e := c.RemoveAccount(a.ID, a.Revision); e != nil {
		t.Fatal(e)
	}
	if _, e := s.ClaimSavedAccount(w.RequestID, w.Epoch, savedPolicy()); e == nil {
		t.Fatal("stale account claimed")
	}
}
