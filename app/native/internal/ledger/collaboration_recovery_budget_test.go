package ledger

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestGroupServiceRecoveryObservedBudgetFreshCASAndRestart(t *testing.T) {
	for _, reason := range []string{"deadline_observed", "clock_before_creation"} {
		t.Run(reason, func(t *testing.T) {
			s, _, g, w, _ := groupRecoveryFixture(t, 1)
			created, _ := time.Parse(time.RFC3339Nano, g.CreatedAt)
			deadline, _ := time.Parse(time.RFC3339Nano, g.DeadlineAt)
			observed, rewind := deadline, deadline.Add(-time.Nanosecond)
			if reason == "clock_before_creation" {
				observed, rewind = created.Add(-time.Nanosecond), created.Add(time.Nanosecond)
			}
			s.now = func() time.Time { return observed }
			before := s.Snapshot().Sequence
			if _, _, err := s.RecoverCollaboration(g.ID, groupRecoverInput(g, w, "group-observe-direct")); !errors.Is(err, ErrConflict) {
				t.Fatal(err)
			}
			closed := groupState(t, s, g.ID)
			unchanged := closed
			unchanged.BudgetClosedAt, unchanged.BudgetClosedReason, unchanged.Revision = "", "", g.Revision
			got, _ := s.Get(w.RequestID)
			if s.Snapshot().Sequence != before+1 || closed.BudgetClosedAt != observed.UTC().Format(time.RFC3339Nano) || closed.BudgetClosedReason != reason || !reflect.DeepEqual(unchanged, g) || !reflect.DeepEqual(got, w) {
				t.Fatal("observation changed obligation/proof/effects or original budget")
			}
			s.now = func() time.Time { return rewind }
			for i := 0; i < 3; i++ {
				current := groupState(t, s, g.ID)
				stage, _ := s.Get(w.RequestID)
				before = s.Snapshot().Sequence
				if _, _, err := s.RecoverCollaboration(g.ID, groupRecoverInput(current, stage, "group-observe-fresh")); !errors.Is(err, ErrConflict) || s.Snapshot().Sequence != before {
					t.Fatal("fresh CAS rewound irreversible marker", err)
				}
				if s.collaborationRecoveryReady(current, stage, rewind) {
					t.Fatal("pure recorded readiness ignored marker")
				}
				if _, _, err := s.Accept(input("group-observed-unrelated-"+string(rune('0'+i)), "foreground")); err != nil {
					t.Fatal("expiry blocks unrelated intake", err)
				}
				dir := s.dir
				if err := s.Close(); err != nil {
					t.Fatal(err)
				}
				var err error
				s, err = Open(dir)
				if err != nil {
					t.Fatal("genuine marker history replay", err)
				}
				s.now = func() time.Time { return rewind }
				if !reflect.DeepEqual(groupState(t, s, g.ID), closed) {
					t.Fatal("budget marker lost on restart")
				}
			}
			s.Close()
		})
	}
}

func TestGroupServiceRecoveryBudgetClosureReplayAndDuplicate(t *testing.T) {
	s, _, g, w, _ := groupRecoveryFixture(t, 1)
	deadline, _ := time.Parse(time.RFC3339Nano, g.DeadlineAt)
	closed := copyCollaboration(g)
	closed.Revision++
	closed.BudgetClosedAt, closed.BudgetClosedReason = deadline.UTC().Format(time.RFC3339Nano), "deadline_observed"
	event := Event{Type: "collaboration.budget_closed", Collaboration: &closed}
	s.now = func() time.Time { return deadline.Add(48 * time.Hour) }
	if !validReplay(s, event) {
		t.Fatal("recorded lifecycle fact consulted current clock")
	}
	for _, name := range []string{"inside", "reason", "timestamp", "goal", "work", "claim-version", "missing", "version"} {
		t.Run(name, func(t *testing.T) {
			bad := event
			x := copyCollaboration(closed)
			bad.Collaboration = &x
			switch name {
			case "inside":
				x.BudgetClosedAt = deadline.Add(-time.Nanosecond).UTC().Format(time.RFC3339Nano)
			case "reason":
				x.BudgetClosedReason = "clock_before_creation"
			case "timestamp":
				x.BudgetClosedAt = deadline.Format("2006-01-02T15:04:05.999999999+00:00")
			case "goal":
				x.Goal += " drift"
			case "work":
				bad.Work = w
			case "claim-version":
				bad.ExecutorClaimVersion = 1
			case "missing":
				x.BudgetClosedAt = ""
			case "version":
				x.Version++
			}
			if validReplay(s, bad) {
				t.Fatal("forged observation accepted")
			}
		})
	}
	s.now = time.Now
	in := groupRecoverInput(g, w, "group-success-before-observation")
	if _, _, err := s.RecoverCollaboration(g.ID, in); err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return deadline }
	if s.RuntimeAllowed(w.RequestID, w.Epoch+1) {
		t.Fatal("live prompt allowed after expiry")
	}
	before := s.Snapshot().Sequence
	if _, duplicate, err := s.RecoverCollaboration(g.ID, in); err != nil || !duplicate || s.Snapshot().Sequence != before {
		t.Fatal("successful historical duplicate became new recovery or lost receipt", err)
	}
	if groupState(t, s, g.ID).BudgetClosedAt == "" {
		t.Fatal("duplicate erased irreversible observation")
	}
}

func TestGroupServiceRecoveryObservedBudgetKeepsLateStopAndCancel(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		t.Run(map[bool]string{false: "pending", true: "cancel"}[cancel], func(t *testing.T) {
			s, r, g := configuredRecoveryGroup(t)
			_, w := advanceGroup(t, s, g.ID)
			w, binding := admittedRecoveryStage(t, s, w)
			p := r.Snapshot()
			if err := r.MarkUnavailable(p.Pin, p.BootID, "host_disconnected"); err != nil {
				t.Fatal(err)
			}
			if err := s.ReconcileExecutors(); err != nil {
				t.Fatal(err)
			}
			if err := s.AdvanceCollaborations(); err != nil {
				t.Fatal(err)
			}
			deadline, _ := time.Parse(time.RFC3339Nano, g.DeadlineAt)
			s.now = func() time.Time { return deadline }
			if err := s.AdvanceCollaborations(); err != nil {
				t.Fatal(err)
			}
			closed := groupState(t, s, g.ID)
			pending, _ := s.Get(w.RequestID)
			if closed.BudgetClosedAt == "" || closed.Status != "suspended" || pending.Status != "suspending" || pending.DesiredState != "suspended" || pending.RuntimeStopped {
				t.Fatal("expiry fabricated stop/cancel or changed suspended intent")
			}
			s.now = func() time.Time { return deadline.Add(-time.Nanosecond) }
			if cancel {
				if _, _, err := s.CancelCollaboration(g.ID, CollaborationCancelInput{ExpectedRevision: closed.Revision, ClientNonce: "group-observed-late-cancel"}); err != nil {
					t.Fatal(err)
				}
			}
			later := serviceStopped(t, s, w, binding)
			if err := s.AdvanceCollaborations(); err != nil {
				t.Fatal(err)
			}
			current := groupState(t, s, g.ID)
			later, _ = s.Get(w.RequestID)
			if current.BudgetClosedAt != closed.BudgetClosedAt || !reflect.DeepEqual(later.Core, w.Core) || !stoppedTask(later) {
				t.Fatal("late stop lost original proof or closed budget")
			}
			if cancel {
				if current.Status != "cancelled" || later.Status != "cancelled" {
					t.Fatal("valid late proof cannot settle explicit cancel")
				}
			} else if current.Status != "suspended" || later.Status != "suspended" || !errors.Is(func() error {
				_, _, err := s.RecoverCollaboration(g.ID, groupRecoverInput(current, later, "group-observed-late-recover"))
				return err
			}(), ErrConflict) {
				t.Fatal("late proof auto-cancelled or reopened expired obligation")
			}
		})
	}
}
