package ledger

import (
	"errors"
	"testing"
	"time"
)

func TestExpiryPersistenceFailureNeverDispatchesAfterClockRewind(t *testing.T) {
	s, _, a, _, _ := accountFixture(t)
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	w := acceptBound(t, s, a, "failed-expiry-dispatch")
	w, err := s.Claim(w.RequestID, w.Epoch)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.file.Close(); err != nil {
		t.Fatal(err)
	}
	now = now.Add(11 * time.Minute)
	if s.RuntimeAllowed(w.RequestID, w.Epoch) || !s.poisoned {
		t.Fatal("failed expiry write did not fail closed")
	}
	now = now.Add(-10 * time.Minute)
	called := false
	err = s.Dispatch(w.RequestID, w.Epoch, func() error { called = true; return nil })
	if called || !errors.Is(err, ErrUnavailable) || s.RuntimeAllowed(w.RequestID, w.Epoch) {
		t.Fatal("poisoned journal dispatched after clock rewind", called, err)
	}
}

func TestUnavailableJournalNeverDispatchesUnboundWork(t *testing.T) {
	for _, closed := range []bool{false, true} {
		t.Run(map[bool]string{false: "poisoned", true: "closed"}[closed], func(t *testing.T) {
			s, _, _, _, _ := accountFixture(t)
			w, _, err := s.Accept(input("unavailable-unbound", "background"))
			if err != nil {
				t.Fatal(err)
			}
			w, err = s.Claim(w.RequestID, w.Epoch)
			if err != nil {
				t.Fatal(err)
			}
			if closed {
				s.Close()
			} else {
				s.poisoned = true
			}
			called := false
			err = s.Dispatch(w.RequestID, w.Epoch, func() error { called = true; return nil })
			if called || !errors.Is(err, ErrUnavailable) || s.RuntimeAllowed(w.RequestID, w.Epoch) {
				t.Fatal("unavailable unbound work launched", called, err)
			}
		})
	}
}
