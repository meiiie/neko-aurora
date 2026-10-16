package ledger

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func createChallengeFor(t *testing.T, s *Store, w Work, nonce string) ChallengeResult {
	t.Helper()
	b := challengeBinding(w)
	out, _, err := s.CreateSecureChallenge(ChallengeInput{Action: "create", RequestID: w.RequestID, ExpectedEpoch: w.Epoch, AttemptID: w.AttemptID, AccountID: b.AccountID, AccountRevision: b.AccountRevision, Destination: w.ConversationID, Purpose: b.Purpose, ClientNonce: nonce})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func TestSecureChallengeSimulatorFencesScopeExpiryCancelAndReopen(t *testing.T) {
	for _, scenario := range []string{"wrong_proof", "wrong_fingerprint", "expiry", "clock_backwards", "cancel", "reopen"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			s, err := Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { s.Close() }()
			s.ConfigureChallengeSimulator(true)
			w, _, _ := s.Accept(input("challenge-fixture-work", "background"))
			out := createChallengeFor(t, s, w, "challenge-fixture-create")
			in := ChallengeInput{Action: "approve_simulation", ChallengeID: out.Challenge.ID, Fingerprint: out.Challenge.Fingerprint, ClientNonce: "challenge-fixture-decide"}
			code := out.Code
			switch scenario {
			case "wrong_proof":
				code = strings.Repeat("0", 64)
			case "wrong_fingerprint":
				in.Fingerprint = strings.Repeat("0", 64)
			case "expiry":
				created, _ := time.Parse(time.RFC3339Nano, out.Challenge.CreatedAt)
				s.now = func() time.Time { return created.Add(5 * time.Minute) }
			case "clock_backwards":
				created, _ := time.Parse(time.RFC3339Nano, out.Challenge.CreatedAt)
				s.now = func() time.Time { return created.Add(-time.Second) }
			case "cancel":
				if _, err = s.Cancel(w.RequestID); err != nil {
					t.Fatal(err)
				}
			case "reopen":
				s.Close()
				s, err = Open(dir)
				if err != nil {
					t.Fatal(err)
				}
				s.ConfigureChallengeSimulator(true)
			}
			before := s.Snapshot().Sequence
			if _, _, err = s.ResolveSecureChallenge(in, code); !errors.Is(err, ErrConflict) || s.Snapshot().Sequence != before {
				t.Fatal("invalid simulator proof committed", err)
			}
			if scenario == "expiry" || scenario == "clock_backwards" {
				created, _ := time.Parse(time.RFC3339Nano, out.Challenge.CreatedAt)
				s.now = func() time.Time { return created.Add(time.Minute) }
				if _, _, err = s.ResolveSecureChallenge(in, code); !errors.Is(err, ErrConflict) {
					t.Fatal("observed expiry/rollback proof revived", err)
				}
			}
			raw, _ := json.Marshal(s.Snapshot())
			if strings.Contains(string(raw), out.Code) || strings.Contains(string(raw), "simulationCode") {
				t.Fatal("proof in public projection")
			}
		})
	}
}
func TestSecureChallengeSimulatorAccountRevokeAndOriginalReceipt(t *testing.T) {
	s, c, a, _, dir := accountFixture(t)
	s.ConfigureChallengeSimulator(true)
	w := acceptBound(t, s, a, "challenge-account-work")
	out := createChallengeFor(t, s, w, "challenge-account-create")
	in := ChallengeInput{Action: "approve_simulation", ChallengeID: out.Challenge.ID, Fingerprint: out.Challenge.Fingerprint, ClientNonce: "challenge-account-resolve"}
	if err := c.RemoveAccount(a.ID, a.Revision); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ResolveSecureChallenge(in, out.Code); !errors.Is(err, ErrConflict) {
		t.Fatal("revoked account proof accepted", err)
	}
	s.Close()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r, err := s.CommandReceipt("secure_challenge", "challenge-account-create", "")
	if err != nil || !r.Found || r.Challenge.GrantsExecution || r.Challenge.Status != "pending" {
		t.Fatal("redacted durable receipt", err)
	}
}
