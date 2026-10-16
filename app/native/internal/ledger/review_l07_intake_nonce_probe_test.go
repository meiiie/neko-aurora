package ledger

import (
	"errors"
	"testing"
)

func TestIndependentL07ChallengeFirstFreshIntakeNonceFence(t *testing.T) {
	for _, scope := range []string{"direct", "shared"} {
		t.Run(scope, func(t *testing.T) {
			s, w, _ := independentL07Store(t)
			channelID := ""
			if scope == "shared" {
				channelID = sharedChannel(t, s, "Independent intake nonce channel", "independent-l07-intake-channel").ID
			}
			accept := func(nonce string) (Work, bool, error) {
				in := input(nonce, "background")
				if scope == "direct" {
					return s.Accept(in)
				}
				return s.AcceptScoped(in, channelID, "", 0)
			}
			if _, dup, err := accept("independent-l07-intake-healthy-control"); err != nil || dup {
				t.Fatal("control: fresh intake unavailable", err)
			}
			nonce := "independent-l07-challenge-intake-collision"
			createChallengeFor(t, s, w, nonce)
			before := s.Snapshot()
			_, dup, err := accept(nonce)
			after := s.Snapshot()
			if !errors.Is(err, ErrConflict) || dup {
				t.Errorf("fresh intake reused challenge nonce: conflict=%t duplicate=%t", errors.Is(err, ErrConflict), dup)
			}
			if after.Sequence != before.Sequence || len(after.Work) != len(before.Work) {
				t.Errorf("rejected composition changed journal/work: sequenceDelta=%d workDelta=%d", after.Sequence-before.Sequence, len(after.Work)-len(before.Work))
			}
		})
	}
}
