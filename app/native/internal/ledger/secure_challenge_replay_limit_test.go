package ledger

import (
	"errors"
	"fmt"
	"testing"
)

func TestSecureChallengeReplayCreationLimitAllowsResolution(t *testing.T) {
	s, w, _ := independentL07Store(t)
	var first ChallengeResult
	for i := 0; i < 128; i++ {
		out, _, err := s.CreateSecureChallenge(independentL07Input(w, fmt.Sprintf("challenge-limit-%03d", i)))
		if err != nil {
			t.Fatal("creation below limit", err)
		}
		if i == 0 {
			first = out
		}
	}
	before := s.Snapshot().Sequence
	if _, _, err := s.CreateSecureChallenge(independentL07Input(w, "challenge-live-over-limit")); !errors.Is(err, ErrInvalid) || s.Snapshot().Sequence != before {
		t.Fatal("live limit changed", err)
	}
	if validReplay(s, independentL07Candidate(s, w, "challenge-replay-over-limit")) {
		t.Error("strict replay admits creation beyond live bound")
	}
	out, duplicate, err := s.ResolveSecureChallenge(independentL07Resolve(first, "challenge-resolve-at-limit"), first.Code)
	if err != nil || duplicate || out.Status != "approve_simulation" {
		t.Fatal("existing resolution at creation limit", err)
	}
	dir := s.dir
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	opened, err := Open(dir)
	if err != nil {
		t.Fatal("strict replay of bounded creations and resolution", err)
	}
	defer opened.Close()
	if opened.challenges[first.Challenge.ID].Status != "approve_simulation" {
		t.Error("strict replay loses resolution at creation limit")
	}
}
