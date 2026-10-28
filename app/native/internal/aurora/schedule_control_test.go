package aurora

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"nekoaurora.local/native/internal/ledger"
)

func TestScheduleControlPersistsAndRejectsStaleChanges(t *testing.T) {
	dir := t.TempDir()
	l, err := ledger.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	s := &Service{L: l, sched: openSchedules(dir), wake: make(chan struct{}, 1)}
	bot := l.Snapshot().Bots[0].ID
	if !s.sched.add(schedule{ID: "test", Bot: bot, Text: "tin sáng", At: "08:00", Repeat: "daily"}) {
		t.Fatal("setup")
	}
	pause := true
	if err := s.controlSchedule("test", 1, &pause); err != nil {
		t.Fatal(err)
	}
	s.scheduleTick(vn("2026-10-28 09:00"))
	if len(l.Snapshot().Work) != 0 {
		t.Fatal("paused schedule ran")
	}
	s.sched = openSchedules(dir)
	x := s.sched.list(bot)[0]
	if !x.Paused || x.Revision != 2 {
		t.Fatal("pause lost on restart")
	}
	pause = false
	if err := s.controlSchedule("test", 1, &pause); !errors.Is(err, ledger.ErrConflict) {
		t.Fatal("stale device resumed schedule")
	}
	if err := s.controlSchedule("test", 2, &pause); err != nil {
		t.Fatal(err)
	}
	s.scheduleTick(vn("2026-10-28 09:00"))
	s.scheduleTick(vn("2026-10-28 09:01"))
	if len(l.Snapshot().Work) != 1 {
		t.Fatal("occurrence duplicated")
	}
	pause = true
	if err := s.controlSchedule("test", 3, &pause); err != nil {
		t.Fatal(err)
	}
	if l.Snapshot().Work[0].Status != "cancelled" {
		t.Fatal("queued occurrence survived pause")
	}
	if err := s.controlSchedule("test", 4, nil); err != nil || len(s.sched.list(bot)) != 0 {
		t.Fatal("delete", err)
	}
}

func TestScheduleControlWriteFailureAndAdmissionFailure(t *testing.T) {
	dir := t.TempDir()
	l, err := ledger.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	s := &Service{L: l, sched: openSchedules(dir), wake: make(chan struct{}, 1)}
	s.sched.add(schedule{ID: "missing", Bot: "missing", Text: "nhắc", At: "08:00", Repeat: "once", Date: "2026-10-28"})
	s.scheduleTick(vn("2026-10-28 09:00"))
	if len(s.sched.list("")) != 1 || s.sched.list("")[0].Last != "" {
		t.Fatal("failed admission consumed reminder")
	}
	if err := os.Mkdir(filepath.Join(dir, "schedules.json.tmp"), 0o700); err != nil {
		t.Fatal(err)
	}
	pause := true
	if s.controlSchedule("missing", 1, &pause) == nil || s.sched.list("")[0].Paused {
		t.Fatal("failed save changed schedule")
	}
}
