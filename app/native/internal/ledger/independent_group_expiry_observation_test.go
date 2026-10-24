package ledger

import (
	"errors"
	"testing"
	"time"
)

func TestIndependentGroupRecoveryDirectExpiryFreshCASCannotRewind(t *testing.T) {
	s, _, g, w, _ := groupRecoveryFixture(t, 1)
	deadline, e := time.Parse(time.RFC3339Nano, g.DeadlineAt)
	if e != nil {
		t.Fatal(e)
	}
	in := groupRecoverInput(g, w, "independent-direct-expiry-observation")
	s.now = func() time.Time { return deadline }
	if _, _, e = s.RecoverCollaboration(g.ID, in); !errors.Is(e, ErrConflict) {
		t.Fatal("positive direct deadline denial", e)
	}
	s.now = func() time.Time { return deadline.Add(-time.Nanosecond) }
	currentG := groupState(t, s, g.ID)
	currentW, e := s.Get(w.RequestID)
	if e != nil {
		t.Fatal(e)
	}
	if currentG.DeadlineAt != g.DeadlineAt {
		t.Fatal("observation resets original deadline")
	}
	fresh := groupRecoverInput(currentG, currentW, in.ClientNonce)
	if _, _, e = s.RecoverCollaboration(g.ID, fresh); !errors.Is(e, ErrConflict) {
		t.Fatal("direct observed expiry revived after clock rewind with refreshed CAS and no manager tick", e)
	}
}
func TestIndependentGroupRecoveryClaimObservedExpiryCannotRewind(t *testing.T) {
	s, _, g, w, _ := groupRecoveryFixture(t, 1)
	in := groupRecoverInput(g, w, "independent-claim-expiry-observation")
	out, _, e := s.RecoverCollaboration(g.ID, in)
	if e != nil {
		t.Fatal("positive valid original recovery", e)
	}
	deadline, e := time.Parse(time.RFC3339Nano, g.DeadlineAt)
	if e != nil {
		t.Fatal(e)
	}
	s.now = func() time.Time { return deadline }
	if _, e = s.Claim(w.RequestID, out.Work.Epoch); !errors.Is(e, ErrConflict) {
		t.Fatal("positive claim deadline denial", e)
	}
	s.now = func() time.Time { return deadline.Add(-time.Nanosecond) }
	currentG := groupState(t, s, g.ID)
	currentW, e := s.Get(w.RequestID)
	if e != nil {
		t.Fatal(e)
	}
	if currentG.DeadlineAt != g.DeadlineAt {
		t.Fatal("claim observation resets original deadline")
	}
	if _, e = s.Claim(w.RequestID, currentW.Epoch); !errors.Is(e, ErrConflict) {
		t.Fatal("Claim already observed original expiry but fresh current epoch revives attempt after clock rewind", e)
	}
}
