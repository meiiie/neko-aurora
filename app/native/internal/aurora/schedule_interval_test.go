package aurora

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"nekoaurora.local/native/internal/ledger"
)

func TestScheduleIntervalEditCreatesAnchoredCadence(t *testing.T) {
	s, request := scheduleEditorFixture(t)
	body := scheduleEditBody(s)
	body["repeat"], body["at"], body["intervalMinutes"] = "interval", "", 60
	code, _ := request("schedule-edit-fixture", body)
	if code != 200 {
		t.Fatalf("interval edit returned %d, want 200", code)
	}
	x := s.sched.list("")[0]
	if x.IntervalAnchor != s.Now().UTC().Format(time.RFC3339Nano) || x.IntervalMinutes != 60 || x.IntervalGeneration != 2 || x.Revision != 2 || x.At != "" || x.Last != "" {
		t.Fatal("wrong anchor", x)
	}
	for _, now := range []time.Time{s.Now(), s.Now().Add(time.Hour - time.Nanosecond)} {
		s.scheduleTick(now)
		if len(s.L.Snapshot().Work) != 0 {
			t.Fatal("ran before one interval")
		}
	}
	s.scheduleTick(s.Now().Add(time.Hour))
	first := s.L.Snapshot().Work
	if len(first) != 1 || !strings.HasSuffix(first[0].ClientNonce, "-i2-1") {
		t.Fatal("missing first slot", first)
	}
	s.sched = openSchedules(filepath.Dir(s.sched.path))
	s.scheduleTick(s.Now().Add(5*time.Hour + 30*time.Minute))
	if len(s.L.Snapshot().Work) != 2 || s.sched.list("")[0].Last != "i2-5" {
		t.Fatal("late tick replayed backlog or missed latest slot")
	}
	s.scheduleTick(s.Now().Add(3 * time.Hour))
	s.scheduleTick(s.Now().Add(5*time.Hour + 59*time.Minute))
	if len(s.L.Snapshot().Work) != 2 {
		t.Fatal("rollback or duplicate ran again")
	}
	s.sched = openSchedules(filepath.Dir(s.sched.path))
	s.scheduleTick(s.Now().Add(6 * time.Hour))
	if len(s.L.Snapshot().Work) != 3 {
		t.Fatal("next anchored slot missing after restart")
	}
}

func TestScheduleIntervalDirectiveUsesHostAnchor(t *testing.T) {
	s, _ := scheduleEditorFixture(t)
	d := ParseDirectives(`SCHEDULE: {"repeat":"interval","intervalMinutes":60,"text":"hourly fixture","intervalAnchor":"2020-01-01T00:00:00Z","intervalGeneration":999,"last":"i999-300"}`)
	bot := s.L.Snapshot().Bots[0].ID
	s.addSchedules(ledger.Work{BotID: bot}, d.Schedules)
	var found schedule
	for _, x := range s.sched.list(bot) {
		if x.Repeat == "interval" {
			found = x
		}
	}
	if found.ID == "" || found.IntervalAnchor != s.Now().UTC().Format(time.RFC3339Nano) || found.IntervalGeneration != 1 || found.Last != "" {
		t.Fatal("directive supplied execution identity", found)
	}
	s.addSchedules(ledger.Work{BotID: bot}, d.Schedules)
	if len(s.sched.list(bot)) != 2 {
		t.Fatal("duplicate interval directive inserted")
	}
}

func TestScheduleIntervalExactBoundaryAndDST(t *testing.T) {
	for _, instant := range []string{"2026-03-08T06:30:00.123456789Z", "2026-11-01T05:30:00.123456789Z"} {
		now, _ := time.Parse(time.RFC3339Nano, instant)
		x, ok := (schedule{Repeat: "interval", IntervalMinutes: 60, Timezone: "America/New_York", Text: "fixture", Last: "forged", IntervalAnchor: "forged", IntervalGeneration: 99}).normalize(now)
		if !ok || x.IntervalAnchor != instant || x.Last != "" || x.IntervalGeneration != 1 {
			t.Fatal("directive controls anchor", x)
		}
		if x.due(now.Add(time.Hour-time.Nanosecond)) != "" || x.due(now.Add(time.Hour)) != "i1-1" || x.due(now.Add(2*time.Hour)) != "i1-2" {
			t.Fatal("DST or fractional boundary changed elapsed cadence")
		}
		x.Last = "i1-2"
		if x.due(now.Add(time.Hour)) != "" {
			t.Fatal("clock rollback replays older slot")
		}
		x.Paused = true
		if x.due(now.Add(3*time.Hour)) != "" {
			t.Fatal("paused interval runs")
		}
	}
}

func TestScheduleIntervalEditsPreserveOrReplaceEpoch(t *testing.T) {
	s, request := scheduleEditorFixture(t)
	start := s.Now()
	body := scheduleEditBody(s)
	body["repeat"], body["at"], body["intervalMinutes"] = "interval", "", 60
	if code, _ := request("schedule-edit-fixture", body); code != 200 {
		t.Fatal(code)
	}
	anchor := s.sched.list("")[0].IntervalAnchor
	s.scheduleTick(start.Add(time.Hour))
	s.Now = func() time.Time { return start.Add(90 * time.Minute) }
	body["revision"], body["text"] = 2, "updated text"
	if code, _ := request("schedule-edit-fixture", body); code != 200 {
		t.Fatal(code)
	}
	x := s.sched.list("")[0]
	if x.IntervalAnchor != anchor || x.IntervalGeneration != 2 || x.Last != "i2-1" {
		t.Fatal("text edit reset cadence")
	}
	if code, _ := request("schedule-edit-fixture", body); code != 409 {
		t.Fatal("stale edit", code)
	}
	paused := true
	if err := s.controlSchedule(x.ID, 3, &paused); err != nil {
		t.Fatal(err)
	}
	s.scheduleTick(start.Add(3 * time.Hour))
	paused = false
	if err := s.controlSchedule(x.ID, 4, &paused); err != nil {
		t.Fatal(err)
	}
	s.scheduleTick(start.Add(3 * time.Hour))
	if len(s.L.Snapshot().Work) != 2 || s.sched.list("")[0].Last != "i2-3" {
		t.Fatal("resume changed identity or caught up multiple times")
	}
	body["revision"], body["intervalMinutes"] = 5, 30
	s.Now = func() time.Time { return start.Add(190 * time.Minute) }
	if code, _ := request(x.ID, body); code != 200 {
		t.Fatal(code)
	}
	x = s.sched.list("")[0]
	if x.IntervalGeneration != 6 || x.Last != "" || x.IntervalAnchor != s.Now().UTC().Format(time.RFC3339Nano) {
		t.Fatal("cadence edit did not start new epoch")
	}
	s.scheduleTick(s.Now())
	if len(s.L.Snapshot().Work) != 2 {
		t.Fatal("cadence edit fired immediately")
	}
	s.scheduleTick(s.Now().Add(30 * time.Minute))
	if len(s.L.Snapshot().Work) != 3 {
		t.Fatal("new cadence missing")
	}
	body["revision"], body["repeat"], body["at"], body["intervalMinutes"] = 6, "daily", "16:00", 0
	if code, _ := request(x.ID, body); code != 200 {
		t.Fatal(code)
	}
	x = s.sched.list("")[0]
	if x.IntervalAnchor != "" || x.IntervalGeneration != 0 || x.IntervalMinutes != 0 || x.Last != "" {
		t.Fatal("calendar inherited interval fields")
	}
	if openSchedules(filepath.Dir(s.sched.path)).loadErr != nil {
		t.Fatal("calendar roundtrip invalid")
	}
}

func TestScheduleIntervalRejectsInvalidEditsAndDirectives(t *testing.T) {
	s, request := scheduleEditorFixture(t)
	before := s.sched.list("")
	for _, change := range []map[string]any{{"intervalMinutes": 0}, {"intervalMinutes": 14}, {"intervalMinutes": 10081}, {"intervalMinutes": 15.5}, {"at": "08:00"}, {"date": "2026-10-29"}, {"intervalAnchor": "2026-10-28T00:00:00Z"}, {"intervalGeneration": 3}, {"timezone": "not/a/zone"}, {"workspaceId": "other"}} {
		body := scheduleEditBody(s)
		body["repeat"], body["at"], body["intervalMinutes"] = "interval", "", 60
		for k, v := range change {
			body[k] = v
		}
		if code, _ := request("schedule-edit-fixture", body); code != 400 && code != 409 {
			t.Fatal("invalid edit accepted", change, code)
		}
		if !reflect.DeepEqual(before, s.sched.list("")) {
			t.Fatal("invalid edit mutated store")
		}
	}
	for _, minutes := range []int{-1, 0, 14, 10081} {
		if _, ok := (schedule{Repeat: "interval", Text: "x", IntervalMinutes: minutes}).normalize(s.Now()); ok {
			t.Fatal("invalid interval normalized", minutes)
		}
	}
	for _, minutes := range []int{15, 60, 10080} {
		if _, ok := (schedule{Repeat: "interval", Text: "x", IntervalMinutes: minutes}).normalize(s.Now()); !ok {
			t.Fatal("valid interval rejected", minutes)
		}
	}
}

func TestScheduleIntervalStorageFailurePreservesReceipt(t *testing.T) {
	s, request := scheduleEditorFixture(t)
	body := scheduleEditBody(s)
	body["repeat"], body["at"], body["intervalMinutes"] = "interval", "", 60
	if code, _ := request("schedule-edit-fixture", body); code != 200 {
		t.Fatal(code)
	}
	path := s.sched.path
	s.sched.path = filepath.Join(path+".missing", "schedules.json")
	s.scheduleTick(s.Now().Add(time.Hour))
	if len(s.L.Snapshot().Work) != 1 || s.sched.list("")[0].Last != "" {
		t.Fatal("receipt or failed progress wrong")
	}
	before := s.sched.list("")
	body["revision"], body["intervalMinutes"] = 2, 30
	if code, _ := request("schedule-edit-fixture", body); code != 503 {
		t.Fatal("failed edit acknowledged", code)
	}
	if !reflect.DeepEqual(before, s.sched.list("")) {
		t.Fatal("failed edit changed anchor")
	}
	s.sched = openSchedules(filepath.Dir(path))
	s.scheduleTick(s.Now().Add(time.Hour))
	if len(s.L.Snapshot().Work) != 1 || s.sched.list("")[0].Last != "i2-1" {
		t.Fatal("restart repeated admitted slot")
	}
}

func TestScheduleIntervalCorruptStateFailsClosed(t *testing.T) {
	s, request := scheduleEditorFixture(t)
	body := scheduleEditBody(s)
	body["repeat"], body["at"], body["intervalMinutes"] = "interval", "", 60
	if code, _ := request("schedule-edit-fixture", body); code != 200 {
		t.Fatal(code)
	}
	base := s.sched.list("")[0]
	for _, change := range []func(*schedule){func(x *schedule) { x.IntervalAnchor = "" }, func(x *schedule) { x.IntervalAnchor = "2026-10-28T09:00:00+07:00" }, func(x *schedule) { x.IntervalMinutes = 14 }, func(x *schedule) { x.IntervalGeneration = 0 }, func(x *schedule) { x.IntervalGeneration = x.Revision + 1 }, func(x *schedule) { x.At = "08:00" }, func(x *schedule) { x.Date = "2026-10-29" }, func(x *schedule) { x.StartDate = "2026-10-29" }, func(x *schedule) { x.Last = "i2-01" }, func(x *schedule) { x.Last = "i1-1" }, func(x *schedule) { x.Last = "i2-0" }} {
		x := base
		change(&x)
		raw, _ := json.Marshal([]schedule{x})
		path := filepath.Join(t.TempDir(), "schedules.json")
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := readSchedules(path); err == nil {
			t.Fatal("corrupt interval accepted", x)
		}
		if x.due(s.Now().Add(time.Hour)) != "" {
			t.Fatal("invalid in-memory interval dispatched")
		}
	}
}
