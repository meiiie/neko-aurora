package aurora

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"nekoaurora.local/native/internal/ledger"
)

func scheduleEditorFixture(t *testing.T) (*Service, func(string, map[string]any) (int, map[string]any)) {
	t.Helper()
	dir := t.TempDir()
	l, err := ledger.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	t.Cleanup(func() { l.Close() })
	s, err := New(ctx, l, filepath.Join(dir, "aurora"), Options{NoBriefLoop: true, DisableCloud: true})
	if err != nil {
		t.Fatal(err)
	}
	s.Now = func() time.Time { return vn("2026-10-28 09:00") }
	bot := l.Snapshot().Bots[0].ID
	if !s.sched.add(schedule{ID: "schedule-edit-fixture", Bot: bot, Text: "original", At: "08:00", Repeat: "daily"}) {
		t.Fatal("fixture schedule")
	}
	request := func(id string, body map[string]any) (int, map[string]any) {
		r := httptest.NewRecorder()
		s.ServeHTTP(r, httptest.NewRequest("POST", "/app/api/schedules/"+id+"/edit", strings.NewReader(mustJSON(body))))
		var out map[string]any
		json.Unmarshal(r.Body.Bytes(), &out)
		return r.Code, out
	}
	return s, request
}

func scheduleEditBody(s *Service) map[string]any {
	return map[string]any{"workspaceId": s.workspaceID, "revision": 1, "text": "edited prompt", "at": "10:00", "repeat": "daily", "date": ""}
}

func TestScheduleEditPreservesAdmittedOccurrenceAndPersists(t *testing.T) {
	s, request := scheduleEditorFixture(t)
	s.scheduleTick(s.Now())
	original := s.L.Snapshot().Work[0]
	if code, _ := request("schedule-edit-fixture", scheduleEditBody(s)); code != 200 {
		t.Fatal("edit", code)
	}
	x := s.sched.list("")[0]
	if x.Revision != 2 || x.At != "10:00" || x.Text != "edited prompt" || x.Bot != original.BotID || x.Last != "2026-10-28" {
		t.Fatal("lost schedule state", x)
	}
	s.sched = openSchedules(filepath.Dir(s.sched.path))
	s.scheduleTick(vn("2026-10-28 10:01"))
	if len(s.L.Snapshot().Work) != 1 || s.L.Snapshot().Work[0].Text != original.Text {
		t.Fatal("edit changed/repeated admitted work")
	}
	s.scheduleTick(vn("2026-10-29 10:01"))
	if len(s.L.Snapshot().Work) != 2 {
		t.Fatal("next occurrence missing")
	}
	for _, w := range s.L.Snapshot().Work {
		if w.RequestID != original.RequestID && !strings.Contains(w.Text, "edited prompt") {
			t.Fatal("next occurrence kept old payload")
		}
	}
	if code, _ := request("schedule-edit-fixture", scheduleEditBody(s)); code != 409 {
		t.Fatal("stale editor overwrote schedule", code)
	}
}

func TestScheduleEditStartsFutureAndKeepsPausedState(t *testing.T) {
	s, request := scheduleEditorFixture(t)
	body := scheduleEditBody(s)
	body["at"] = "08:30"
	if code, _ := request("schedule-edit-fixture", body); code != 200 {
		t.Fatal(code)
	}
	if s.sched.list("")[0].Last != "" {
		t.Fatal("future start fabricated a receipt")
	}
	s.sched = openSchedules(filepath.Dir(s.sched.path))
	s.scheduleTick(vn("2026-10-28 12:00"))
	if len(s.L.Snapshot().Work) != 0 {
		t.Fatal("past-time edit fired immediately")
	}
	s.scheduleTick(vn("2026-10-29 08:31"))
	if len(s.L.Snapshot().Work) != 1 {
		t.Fatal("future recurring occurrence missing")
	}
	pause := true
	if err := s.controlSchedule("schedule-edit-fixture", 2, &pause); err != nil {
		t.Fatal(err)
	}
	body["revision"] = 3
	body["repeat"] = "once"
	body["date"] = "2026-10-30"
	body["at"] = "11:00"
	if code, _ := request("schedule-edit-fixture", body); code != 200 {
		t.Fatal("change to future one-time", code)
	}
	x := s.sched.list("")[0]
	if !x.Paused || x.Last != "" || x.Revision != 4 {
		t.Fatal("pause or future once state lost", x)
	}
	s.scheduleTick(vn("2026-10-30 12:00"))
	if len(s.L.Snapshot().Work) != 1 {
		t.Fatal("edit resumed paused routine")
	}
}

func TestScheduleEditFutureTodayAndJournalReceiptFence(t *testing.T) {
	s, request := scheduleEditorFixture(t)
	body := scheduleEditBody(s)
	if code, _ := request("schedule-edit-fixture", body); code != 200 {
		t.Fatal(code)
	}
	s.scheduleTick(vn("2026-10-28 09:30"))
	if len(s.L.Snapshot().Work) != 0 {
		t.Fatal("future-today edit ran early")
	}
	s.scheduleTick(vn("2026-10-28 10:01"))
	if len(s.L.Snapshot().Work) != 1 {
		t.Fatal("future-today edit did not run")
	}
	// Simulate ledger admission surviving while the display state lost Last.
	s.sched.mu.Lock()
	s.sched.L[0].Last = ""
	s.sched.saveLocked()
	s.sched.mu.Unlock()
	body["revision"] = 2
	body["at"] = "12:00"
	if code, _ := request("schedule-edit-fixture", body); code != 200 {
		t.Fatal(code)
	}
	x := s.sched.list("")[0]
	if x.StartDate != "2026-10-29" || x.due(vn("2026-10-28 12:01")) != "" {
		t.Fatal("existing journal receipt not honored", x)
	}
	body["revision"] = 3
	body["repeat"] = "once"
	body["date"] = "2026-10-28"
	body["at"] = "13:00"
	if code, _ := request("schedule-edit-fixture", body); code != 409 {
		t.Fatal("one-time edit reused an admitted occurrence key", code)
	}
}

func TestScheduleEditRejectsInvalidScopeFieldsAndSaveFailure(t *testing.T) {
	s, request := scheduleEditorFixture(t)
	for _, change := range []map[string]any{
		{"workspaceId": ""}, {"workspaceId": "other"}, {"at": "25:00"}, {"repeat": "hourly"}, {"text": ""}, {"text": strings.Repeat("x", 1001)},
		{"repeat": "once", "date": "bad"}, {"repeat": "once", "date": "2026-10-27"}, {"repeat": "once", "date": "2026-10-28", "at": "08:00"},
		{"bot": "different"}, {"paused": false}, {"last": "2026-10-28"},
	} {
		body := scheduleEditBody(s)
		for k, v := range change {
			body[k] = v
		}
		code, _ := request("schedule-edit-fixture", body)
		want := 400
		if _, mismatch := change["workspaceId"]; mismatch {
			want = 409
		}
		if code != want {
			t.Fatal("invalid edit response", code, want, change)
		}
		if s.sched.list("")[0].Revision != 1 {
			t.Fatal("invalid edit mutated schedule")
		}
	}
	before := s.sched.list("")
	storedPath := s.sched.path
	s.sched.path = filepath.Join(storedPath+".missing", "schedules.json")
	if code, _ := request("schedule-edit-fixture", scheduleEditBody(s)); code < 400 {
		t.Fatal("failed save acknowledged")
	}
	if !reflect.DeepEqual(before, s.sched.list("")) {
		t.Fatal("failed save was not rolled back")
	}
	if !reflect.DeepEqual(before, openSchedules(filepath.Dir(storedPath)).list("")) {
		t.Fatal("failed edit changed persisted schedule")
	}
}
