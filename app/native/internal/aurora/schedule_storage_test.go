package aurora

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"nekoaurora.local/native/internal/ledger"
	"nekoaurora.local/native/internal/privatefs"
)

func TestScheduleStorageUnreadableStateIsNotEmptyOrWritable(t *testing.T) {
	for _, mode := range []string{"broken", "partial", "oversized", "directory", "future-field", "duplicate-id", "invalid-utf8"} {
		t.Run(mode, func(t *testing.T) {
			s, _ := scheduleEditorFixture(t)
			path := s.sched.path
			original, err := json.Marshal(s.sched.list(""))
			if err != nil {
				t.Fatal(err)
			}
			bad := []byte("{")
			switch mode {
			case "partial":
				bad = []byte(strings.TrimSuffix(string(original), "]") + `,{"id":"bad","revision":"invalid"}]`)
			case "oversized":
				bad = []byte("[]" + strings.Repeat(" ", 262144))
			case "future-field":
				bad = []byte(strings.Replace(string(original), `"revision":1`, `"future":"fixture","revision":1`, 1))
			case "duplicate-id":
				bad = []byte("[" + string(original[1:len(original)-1]) + "," + string(original[1:len(original)-1]) + "]")
			case "invalid-utf8":
				bad = []byte(strings.Replace(string(original), "original", string([]byte{0xff}), 1))
			case "directory":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			}
			if mode != "directory" {
				if err := os.WriteFile(path, bad, 0600); err != nil {
					t.Fatal(err)
				}
			}
			s.sched = openSchedules(filepath.Dir(path))
			r := httptest.NewRecorder()
			s.ServeHTTP(r, httptest.NewRequest("GET", "/app/api/schedules", nil))
			if r.Code != 503 {
				t.Fatalf("unreadable store reported %d instead of unavailable", r.Code)
			}
			if s.sched.add(schedule{ID: "replacement", Bot: s.L.Snapshot().Bots[0].ID, Text: "fixture", At: "12:00", Repeat: "daily"}) {
				t.Fatal("unreadable store overwritten")
			}
			paused := true
			if s.controlSchedule("schedule-edit-fixture", 1, &paused) == nil {
				t.Fatal("corrupt state changed")
			}
			if _, err := s.runScheduleTest("schedule-edit-fixture", scheduleTestInput{WorkspaceID: s.workspaceID, Revision: 1, ClientNonce: "storage-fixture-nonce"}); err == nil {
				t.Fatal("corrupt state executed")
			}
			s.scheduleTick(s.Now())
			if len(s.L.Snapshot().Work) != 0 {
				t.Fatal("partially decoded schedule was dispatched")
			}
			if mode != "directory" {
				after, err := os.ReadFile(path)
				if err != nil || string(after) != string(bad) {
					t.Fatal("unreadable file changed")
				}
			}
		})
	}
}

func TestScheduleStorageRemovalFailureRestoresMemory(t *testing.T) {
	s, _ := scheduleEditorFixture(t)
	path := s.sched.path
	before := s.sched.list("")
	s.sched.path = filepath.Join(path+".missing", "schedules.json")
	if s.sched.remove(before[0].ID) {
		t.Fatal("failed removal acknowledged")
	}
	if !reflect.DeepEqual(before, s.sched.list("")) || !reflect.DeepEqual(before, openSchedules(filepath.Dir(path)).list("")) {
		t.Fatal("failed removal changed memory or disk")
	}
}

func TestScheduleStorageTickRetryKeepsLedgerAdmissionAndRestoresProgress(t *testing.T) {
	s, _ := scheduleEditorFixture(t)
	s.sched.L[0].Repeat = "once"
	s.sched.L[0].Date = "2026-10-28"
	if err := s.sched.saveLocked(); err != nil {
		t.Fatal(err)
	}
	path := s.sched.path
	before := s.sched.list("")
	s.sched.path = filepath.Join(path+".missing", "schedules.json")
	s.scheduleTick(s.Now())
	if len(s.L.Snapshot().Work) != 1 {
		t.Fatal("ledger admission missing")
	}
	if !reflect.DeepEqual(before, s.sched.list("")) {
		t.Fatal("failed tick save consumed in-memory occurrence")
	}
	s.sched.path = path
	s.scheduleTick(s.Now())
	if len(s.L.Snapshot().Work) != 1 || len(s.sched.list("")) != 0 || len(openSchedules(filepath.Dir(path)).list("")) != 0 {
		t.Fatal("retry duplicated admission or lost persisted progress")
	}
}

func TestScheduleStorageReadOnlyTickDoesNotChangeProgress(t *testing.T) {
	dir := t.TempDir()
	l, err := ledger.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	st := openSchedules(dir)
	if !st.add(schedule{ID: "readonly-fixture", Bot: l.Snapshot().Bots[0].ID, Text: "fixture", At: "08:00", Repeat: "daily"}) {
		t.Fatal("fixture creation")
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	ro, err := ledger.OpenReadOnly(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	before, err := os.ReadFile(st.path)
	if err != nil {
		t.Fatal(err)
	}
	s := &Service{L: ro, sched: st, wake: make(chan struct{}, 1)}
	s.scheduleTick(vn("2026-10-28 09:00"))
	after, err := os.ReadFile(st.path)
	if err != nil || string(before) != string(after) {
		t.Fatal("read-only tick wrote routine state")
	}
}

func TestScheduleStoragePublicationCreatesPrivateFile(t *testing.T) {
	st := openSchedules(t.TempDir())
	if !st.add(schedule{ID: "private-fixture", Bot: "fixture", Text: "fixture", At: "08:00", Repeat: "daily"}) {
		t.Fatal("fixture creation")
	}
	if err := privatefs.Check(st.path); err != nil {
		t.Fatal("schedule file is not private")
	}
}

func TestScheduleStorageCompletedLegacyCleanupRollsBackOnWriteFailure(t *testing.T) {
	s, _ := scheduleEditorFixture(t)
	s.sched.L[0].Repeat = "once"
	s.sched.L[0].Date = "2026-10-28"
	s.sched.L[0].Last = "2026-10-28"
	if err := s.sched.saveLocked(); err != nil {
		t.Fatal(err)
	}
	path := s.sched.path
	before := s.sched.list("")
	s.sched.path = filepath.Join(path+".missing", "schedules.json")
	s.scheduleTick(s.Now())
	if !reflect.DeepEqual(before, s.sched.list("")) {
		t.Fatal("completed legacy reminder disappeared despite failed save")
	}
	s.sched.path = path
	s.scheduleTick(s.Now())
	if len(s.sched.list("")) != 0 || len(openSchedules(filepath.Dir(path)).list("")) != 0 || len(s.L.Snapshot().Work) != 0 {
		t.Fatal("legacy cleanup did not persist or created new work")
	}
}

func TestScheduleStorageLegacyEmptyAndUnknownTimezoneRemainRecoverable(t *testing.T) {
	for _, data := range []string{"null", "[]", `[{"id":"legacy","bot":"fixture","text":"fixture","at":"08:00","repeat":"daily","timezone":"Missing/Zone"}]`} {
		dir := t.TempDir()
		path := filepath.Join(dir, "schedules.json")
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		st := openSchedules(dir)
		if !st.add(schedule{ID: "added-fixture", Bot: "fixture", Text: "added fixture", At: "09:00", Repeat: "daily"}) {
			t.Fatal("valid legacy state was treated as corrupt")
		}
	}
}

func TestScheduleStorageInvalidTimingDoesNotBecomeADailyRoutine(t *testing.T) {
	for _, field := range []string{"repeat", "at", "date", "last", "startDate"} {
		t.Run(field, func(t *testing.T) {
			s, _ := scheduleEditorFixture(t)
			x := s.sched.list("")[0]
			switch field {
			case "repeat":
				x.Repeat = "future-mode"
			case "at":
				x.At = "25:00"
			case "date":
				x.Repeat = "once"
				x.Date = "2026-02-30"
			case "last":
				x.Last = "not-a-date"
			case "startDate":
				x.StartDate = "not-a-date"
			}
			data, err := json.Marshal([]schedule{x})
			if err != nil {
				t.Fatal(err)
			}
			path := s.sched.path
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			s.sched = openSchedules(filepath.Dir(path))
			r := httptest.NewRecorder()
			s.ServeHTTP(r, httptest.NewRequest("GET", "/app/api/schedules", nil))
			if r.Code != 503 {
				t.Fatalf("invalid timing reported %d", r.Code)
			}
			s.scheduleTick(s.Now())
			if len(s.L.Snapshot().Work) != 0 {
				t.Fatal("invalid schedule dispatched")
			}
			after, err := os.ReadFile(path)
			if err != nil || string(after) != string(data) {
				t.Fatal("invalid schedule overwritten")
			}
		})
	}
}
