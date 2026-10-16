package ledger

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestSecureChallengeSimulatorWrongTypedScopeAndReplayDoNotGrant(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.ConfigureChallengeSimulator(true)
	w, _, _ := s.Accept(input("challenge-scope-task", "background"))
	original := ChallengeInput{Action: "create", RequestID: w.RequestID, ExpectedEpoch: w.Epoch, Destination: w.ConversationID, Purpose: "secure_input_simulation", ClientNonce: "challenge-scope-create"}
	for _, mutate := range []func(*ChallengeInput){func(p *ChallengeInput) { p.Destination = "conversation-bot-davin" }, func(p *ChallengeInput) { p.ExpectedEpoch++ }, func(p *ChallengeInput) { p.AttemptID = "attempt-" + strings.Repeat("a", 32) }, func(p *ChallengeInput) { p.AccountID = "account-" + strings.Repeat("a", 32); p.AccountRevision = 1 }, func(p *ChallengeInput) { p.Purpose = "real_provider_authorization" }} {
		bad := original
		mutate(&bad)
		before := s.Snapshot().Sequence
		if _, _, err = s.CreateSecureChallenge(bad); !errors.Is(err, ErrConflict) || before != s.Snapshot().Sequence {
			t.Fatal("swapped binding created", err)
		}
	}
	out, _, err := s.CreateSecureChallenge(original)
	if err != nil {
		t.Fatal(err)
	}
	c := out.Challenge
	c.ID = "challenge-" + strings.Repeat("a", 32)
	c.GrantsExecution = true
	e := Event{Type: "challenge.created", Challenge: &c, ChallengeInput: &original, Nonce: original.ClientNonce}
	if s.validChallengeEvent(e) {
		t.Fatal("replay grants execution")
	}
	c = out.Challenge
	events, _ := s.Events(0)
	for i := range events {
		if events[i].Challenge != nil {
			events[i].Challenge.GrantsExecution = true
			events[i].ChallengeInput.Destination = "other"
		}
	}
	r, err := s.CommandReceipt("secure_challenge", original.ClientNonce, "")
	if err != nil || r.Challenge.GrantsExecution || !reflect.DeepEqual(r.Input, original) {
		t.Fatal("copy mutated original", err)
	}
	if _, _, err = s.ControlBot(BotControlInput{BotID: "bot-davin", Action: "archive", ClientNonce: original.ClientNonce}); !errors.Is(err, ErrConflict) {
		t.Fatal("challenge nonce reused", err)
	}
}
