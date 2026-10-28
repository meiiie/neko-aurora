package aurora

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func timezoneFixture(t *testing.T, zone, at, repeat string) schedule {
	t.Helper()
	var x schedule
	b, err := json.Marshal(map[string]any{"id": "timezone-fixture", "bot": "bot-fixture", "text": "fixture", "at": at, "repeat": repeat, "timezone": zone})
	if err != nil || json.Unmarshal(b, &x) != nil {
		t.Fatal("invalid fixture")
	}
	return x
}

func utcFixture(t *testing.T, value string) time.Time {
	t.Helper()
	x, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatal(err)
	}
	return x
}

func savedTimezone(t *testing.T, x schedule) string {
	t.Helper()
	b, _ := json.Marshal(x)
	var fields map[string]any
	if err := json.Unmarshal(b, &fields); err != nil {
		t.Fatal(err)
	}
	zone, _ := fields["timezone"].(string)
	return zone
}

func TestScheduleTimezoneCalendarAndDST(t *testing.T) {
	for _, tc := range []struct{ name, zone, at, repeat, instant, want string }{
		{"UTC before", "UTC", "08:00", "daily", "2026-10-28T07:59:00Z", ""},
		{"UTC due", "UTC", "08:00", "daily", "2026-10-28T08:00:00Z", "2026-10-28"},
		{"Tokyo next day", "Asia/Tokyo", "08:00", "daily", "2026-10-28T23:01:00Z", "2026-10-29"},
		{"Kyiv before", "Europe/Kyiv", "08:00", "daily", "2026-10-29T05:59:00Z", ""},
		{"Kyiv due", "Europe/Kyiv", "08:00", "daily", "2026-10-29T06:00:00Z", "2026-10-29"},
		{"New York before", "America/New_York", "08:00", "daily", "2026-10-28T11:59:00Z", ""},
		{"New York Friday", "America/New_York", "23:30", "weekdays", "2026-10-31T03:31:00Z", "2026-10-30"},
		{"New York Saturday", "America/New_York", "08:00", "weekdays", "2026-10-31T12:01:00Z", ""},
		{"DST gap before", "America/New_York", "02:30", "daily", "2026-03-08T06:59:00Z", ""},
		{"DST gap late", "America/New_York", "02:30", "daily", "2026-03-08T07:05:00Z", "2026-03-08"},
		{"DST first hour", "America/New_York", "01:30", "daily", "2026-11-01T05:31:00Z", "2026-11-01"},
		{"DST second hour", "America/New_York", "01:30", "daily", "2026-11-01T06:31:00Z", "2026-11-01"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			x := timezoneFixture(t, tc.zone, tc.at, tc.repeat)
			if got := x.due(utcFixture(t, tc.instant)); got != tc.want {
				t.Fatalf("due=%q want=%q", got, tc.want)
			}
			if tc.want != "" {
				x.Last = tc.want
				if x.due(utcFixture(t, tc.instant)) != "" {
					t.Fatal("repeated calendar occurrence")
				}
			}
		})
	}
}

func TestScheduleTimezoneNormalizeAndLegacy(t *testing.T) {
	now := utcFixture(t, "2026-10-28T21:00:00Z")
	x, ok := timezoneFixture(t, "Asia/Tokyo", "7:00", "once").normalize(now)
	if !ok || x.Date != "2026-10-29" || x.At != "07:00" || savedTimezone(t, x) != "Asia/Tokyo" {
		t.Fatal("normalization lost the selected calendar", x)
	}
	legacy, ok := (schedule{At: "08:00", Repeat: "daily", Text: "legacy"}).normalize(now)
	if !ok || legacy.due(utcFixture(t, "2026-10-29T01:00:00Z")) != "2026-10-29" {
		t.Fatal("legacy Vietnam schedule changed")
	}
	for _, zone := range []string{"Local", "../UTC", "Missing/Zone", strings.Repeat("x", 200)} {
		x := timezoneFixture(t, zone, "00:00", "daily")
		if _, ok := x.normalize(now); ok {
			t.Errorf("invalid zone accepted: %q", zone)
		}
		if x.due(now) != "" {
			t.Errorf("invalid stored zone ran as a fallback: %q", zone)
		}
	}
}

func TestScheduleTimezoneEditPersistsAndLegacyEditKeepsZone(t *testing.T) {
	s, request := scheduleEditorFixture(t)
	s.Now = func() time.Time { return utcFixture(t, "2026-10-28T08:00:00Z") }
	body := scheduleEditBody(s)
	body["timezone"], body["at"] = "America/New_York", "05:00"
	if code, _ := request("schedule-edit-fixture", body); code != 200 {
		t.Fatal("timezone edit", code)
	}
	s.sched = openSchedules(filepath.Dir(s.sched.path))
	x := s.sched.list("")[0]
	if savedTimezone(t, x) != "America/New_York" || x.StartDate != "2026-10-28" || x.Revision != 2 {
		t.Fatal("wrong saved timezone/calendar", x)
	}
	delete(body, "timezone")
	body["revision"], body["text"] = 2, "legacy caller changes text"
	if code, _ := request("schedule-edit-fixture", body); code != 200 || savedTimezone(t, s.sched.list("")[0]) != "America/New_York" {
		t.Fatal("legacy caller reset the selected zone", code)
	}
	s.scheduleTick(utcFixture(t, "2026-10-28T08:59:00Z"))
	if len(s.L.Snapshot().Work) != 0 {
		t.Fatal("ran before local scheduled hour")
	}
	s.scheduleTick(utcFixture(t, "2026-10-28T09:01:00Z"))
	if len(s.L.Snapshot().Work) != 1 {
		t.Fatal("local scheduled hour did not run")
	}
}

func TestScheduleTimezoneEditInvalidAndRollback(t *testing.T) {
	s, request := scheduleEditorFixture(t)
	body := scheduleEditBody(s)
	original := s.sched.list("")[0]
	for _, zone := range []any{"", "Local", "Missing/Zone", "../UTC", true, 42} {
		body["timezone"] = zone
		if code, _ := request("schedule-edit-fixture", body); code != 400 {
			t.Errorf("invalid zone status=%d", code)
		}
		if !reflect.DeepEqual(original, s.sched.list("")[0]) {
			t.Fatal("invalid zone changed schedule")
		}
	}
	body["timezone"] = "Europe/Kyiv"
	s.sched.path = filepath.Join(t.TempDir(), "missing", "schedules.json")
	if code, _ := request("schedule-edit-fixture", body); code == 200 {
		t.Fatal("failed persistence acknowledged")
	}
	if !reflect.DeepEqual(original, s.sched.list("")[0]) {
		t.Fatal("failed persistence changed timezone or revision")
	}
}

func TestScheduleTimezoneRejectsNonexistentOnceTime(t *testing.T) {
	s, request := scheduleEditorFixture(t)
	s.Now = func() time.Time { return utcFixture(t, "2026-03-07T12:00:00Z") }
	body := scheduleEditBody(s)
	body["timezone"], body["repeat"], body["date"], body["at"] = "America/New_York", "once", "2026-03-08", "02:30"
	if code, _ := request("schedule-edit-fixture", body); code != 400 {
		t.Fatal("nonexistent local hour accepted", code)
	}
	body["at"] = "03:30"
	if code, _ := request("schedule-edit-fixture", body); code != 200 {
		t.Fatal("valid time after DST gap rejected", code)
	}
}

func TestScheduleTimezoneRepeatedHourSurvivesRestart(t *testing.T) {
	s, _ := scheduleEditorFixture(t)
	s.sched.mu.Lock()
	s.sched.L[0].Timezone, s.sched.L[0].At = "America/New_York", "01:30"
	err := s.sched.saveLocked()
	s.sched.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	s.scheduleTick(utcFixture(t, "2026-11-01T05:31:00Z"))
	if len(s.L.Snapshot().Work) != 1 {
		t.Fatal("first repeated hour missing")
	}
	s.sched = openSchedules(filepath.Dir(s.sched.path))
	s.scheduleTick(utcFixture(t, "2026-11-01T06:31:00Z"))
	if len(s.L.Snapshot().Work) != 1 {
		t.Fatal("second repeated hour replayed after restart")
	}
}

func TestScheduleTimezoneChangePreservesAdmittedCalendarDay(t *testing.T) {
	s, request := scheduleEditorFixture(t)
	s.scheduleTick(s.Now())
	original := s.L.Snapshot().Work[0]
	body := scheduleEditBody(s)
	body["timezone"], body["at"] = "UTC", "10:00"
	if code, _ := request("schedule-edit-fixture", body); code != 200 {
		t.Fatal("zone edit", code)
	}
	s.scheduleTick(utcFixture(t, "2026-10-28T10:01:00Z"))
	if works := s.L.Snapshot().Work; len(works) != 1 || works[0].Text != original.Text {
		t.Fatal("zone change repeated or changed an admitted day")
	}
	if code, _ := request("schedule-edit-fixture", body); code != 409 {
		t.Fatal("stale zone edit accepted", code)
	}
	s.scheduleTick(utcFixture(t, "2026-10-29T10:01:00Z"))
	if len(s.L.Snapshot().Work) != 2 {
		t.Fatal("next UTC day missing")
	}
}

func TestScheduleTimezoneInvalidStoredZoneBlocksAdmission(t *testing.T) {
	s, request := scheduleEditorFixture(t)
	s.sched.mu.Lock()
	s.sched.L[0].Timezone = "Missing/Zone"
	s.sched.mu.Unlock()
	s.scheduleTick(s.Now())
	if len(s.L.Snapshot().Work) != 0 || s.sched.list("")[0].Blocked != "invalid_timezone" {
		t.Fatal("invalid stored zone was not blocked")
	}
	body := scheduleEditBody(s)
	body["timezone"] = "UTC"
	if code, _ := request("schedule-edit-fixture", body); code != 200 || s.sched.list("")[0].Blocked != "" {
		t.Fatal("valid edit did not clear timezone error", code)
	}
}

func TestScheduleTimezoneDuplicateIdentityIncludesZone(t *testing.T) {
	st := openSchedules(t.TempDir())
	a := timezoneFixture(t, "UTC", "08:00", "daily")
	b := timezoneFixture(t, "Europe/Kyiv", "08:00", "daily")
	b.ID = "other-zone"
	if !st.add(a) || !st.add(b) || st.add(a) {
		t.Fatal("distinct zones collapsed or identical schedule duplicated")
	}
	legacy := schedule{ID: "legacy", Bot: "legacy-bot", Text: "fixture", At: "08:00", Repeat: "daily"}
	explicit := legacy
	explicit.Timezone = defaultScheduleTimezone
	if !st.add(legacy) || st.add(explicit) {
		t.Fatal("legacy and explicit Vietnam schedules differ in identity")
	}
}
