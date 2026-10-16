package ledger

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSecureChallengeKnownProofNeverPublicEvenAfterUseOrExpiry(t *testing.T) {
	for _, state := range []string{"active", "used", "expired"} {
		t.Run(state, func(t *testing.T) {
			s, w, _ := independentL07Store(t)
			out := createChallengeFor(t, s, w, "challenge-public-create")
			if state == "used" {
				if _, _, err := s.ResolveSecureChallenge(independentL07Resolve(out, "challenge-private-resolution"), out.Code); err != nil {
					t.Fatal(err)
				}
			}
			if state == "expired" {
				created, _ := time.Parse(time.RFC3339Nano, out.Challenge.CreatedAt)
				s.now = func() time.Time { return created.Add(5 * time.Minute) }
				s.ResolveSecureChallenge(independentL07Resolve(out, "challenge-expired-resolution"), out.Code)
			}
			before := s.Snapshot().Sequence
			if _, _, err := s.CreateSecureChallenge(independentL07Input(w, "a"+strings.ToUpper(out.Code))); err == nil {
				t.Fatal("hex prefix or case bypassed known proof guard")
			}
			if _, _, err := s.RenameBot("bot-davin", BotNameInput{Name: out.Code, ClientNonce: "challenge-public-rename"}); !errors.Is(err, ErrInvalid) {
				t.Fatal("known proof stored in public metadata", err)
			}
			if _, err := s.CommandReceipt("secure_challenge", out.Code, ""); !errors.Is(err, ErrInvalid) {
				t.Fatal("lookup echoed proof", err)
			}
			if s.PublicInputAllowed([]byte(out.Code)) || s.Snapshot().Sequence != before {
				t.Fatal("known proof crossed public boundary")
			}
			independentL07AssertNoCode(t, s, out.Code)
		})
	}
}
