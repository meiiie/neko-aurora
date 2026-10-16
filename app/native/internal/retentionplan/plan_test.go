package retentionplan

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func clock() time.Time {
	return time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
}

func item(id, status string, age time.Duration, size int, reasons ...string) Item {
	return Item{ID: id, CreatedAt: clock().Add(-age).Format(time.RFC3339Nano), Status: status, TextBytes: size, ProtectedReasons: reasons}
}

func TestOnlyUnprotectedOldTerminalItemsAreCandidates(t *testing.T) {
	items := []Item{
		item("z", "cancelled", 31*24*time.Hour, 3),
		item("a", "completed", 40*24*time.Hour, 7),
		item("m", "failed", 31*24*time.Hour, 5),
		item("pinned", "completed", 50*24*time.Hour, 11, "context_pin", "effect_receipt", "context_pin"),
		item("active", "running", 50*24*time.Hour, 13),
		item("cutoff", "completed", 30*24*time.Hour, 17),
		item("recent", "failed", 24*time.Hour, 19),
		item("future", "completed", -24*time.Hour, 23),
	}
	preview, err := Plan(items, 30, clock())
	if err != nil {
		t.Fatal(err)
	}
	if preview.Version != 1 || preview.Days != 30 || preview.AsOf != "2026-10-12T12:00:00Z" || preview.Enabled || preview.TotalBytes != 15 {
		t.Fatalf("incorrect preview header: %+v", preview)
	}
	wantIDs := []string{"a", "m", "z"}
	gotIDs := []string{}
	for _, c := range preview.Candidates {
		gotIDs = append(gotIDs, c.ID)
	}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("candidate IDs: %v", gotIDs)
	}
	want := []Protection{
		{ID: "active", Reasons: []string{"nonterminal_status"}},
		{ID: "cutoff", Reasons: []string{"within_retention_window"}},
		{ID: "future", Reasons: []string{"future_created_at"}},
		{ID: "pinned", Reasons: []string{"context_pin", "effect_receipt"}},
		{ID: "recent", Reasons: []string{"within_retention_window"}},
	}
	if !reflect.DeepEqual(preview.Protected, want) {
		t.Fatalf("protections: %+v", preview.Protected)
	}
}

func TestDaysZeroKeepsAllAndNeverEnablesDeletion(t *testing.T) {
	preview, err := Plan([]Item{item("old", "completed", 500*24*time.Hour, 9), item("running", "running", 50*24*time.Hour, 13, "group_dependency")}, 0, clock())
	if err != nil {
		t.Fatal(err)
	}
	if preview.Enabled || preview.Candidates == nil || len(preview.Candidates) != 0 || preview.TotalBytes != 0 || len(preview.Protected) != 2 {
		t.Fatalf("keep-all preview: %+v", preview)
	}
	if !reflect.DeepEqual(preview.Protected[0].Reasons, []string{"retention_disabled"}) || !reflect.DeepEqual(preview.Protected[1].Reasons, []string{"group_dependency", "nonterminal_status", "retention_disabled"}) {
		t.Fatalf("keep-all reasons: %+v", preview.Protected)
	}
}

func TestAllOtherStatesAndExplicitReasonsProtect(t *testing.T) {
	for _, status := range []string{"waiting_host", "queued", "starting", "running", "suspending", "suspended", "cancelling", "paused", "archived", "unknown"} {
		p, err := Plan([]Item{item("state", status, 100*24*time.Hour, 0)}, 1, clock())
		if err != nil || len(p.Candidates) != 0 || len(p.Protected) != 1 || !reflect.DeepEqual(p.Protected[0].Reasons, []string{"nonterminal_status"}) {
			t.Fatalf("status %s: %+v %v", status, p, err)
		}
	}
	p, err := Plan([]Item{item("pin", "completed", 100*24*time.Hour, 0, "historical_context_pin")}, 1, clock())
	if err != nil || len(p.Candidates) != 0 || !reflect.DeepEqual(p.Protected[0].Reasons, []string{"historical_context_pin"}) {
		t.Fatalf("explicit protection: %+v %v", p, err)
	}
}

func TestInvalidInputReturnsTypedErrorAndNoPartialPreview(t *testing.T) {
	valid := item("valid", "completed", 40*24*time.Hour, 4)
	cases := []struct {
		name  string
		items []Item
		days  int
		now   time.Time
		code  ErrorCode
	}{
		{"negative days", []Item{valid}, -1, clock(), InvalidDays},
		{"too many days", []Item{valid}, 3651, clock(), InvalidDays},
		{"zero clock", []Item{valid}, 30, time.Time{}, InvalidClock},
		{"out of range clock", []Item{valid}, 30, time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC), InvalidClock},
		{"too many items", make([]Item, 501), 30, clock(), ItemLimit},
		{"duplicate ID", []Item{valid, valid}, 30, clock(), DuplicateID},
		{"empty ID", []Item{valid, {CreatedAt: valid.CreatedAt, Status: "completed"}}, 30, clock(), InvalidID},
		{"invalid ID", []Item{valid, {ID: "file/path", CreatedAt: valid.CreatedAt, Status: "completed"}}, 30, clock(), InvalidID},
		{"long ID", []Item{valid, {ID: strings.Repeat("a", 129), CreatedAt: valid.CreatedAt, Status: "completed"}}, 30, clock(), InvalidID},
		{"unicode ID", []Item{valid, {ID: "bé", CreatedAt: valid.CreatedAt, Status: "completed"}}, 30, clock(), InvalidID},
		{"missing creation", []Item{valid, {ID: "other", Status: "completed"}}, 30, clock(), InvalidTimestamp},
		{"invalid date", []Item{valid, {ID: "other", CreatedAt: "2026-02-30T12:00:00Z", Status: "completed"}}, 30, clock(), InvalidTimestamp},
		{"no timezone", []Item{valid, {ID: "other", CreatedAt: "2026-01-01T12:00:00", Status: "completed"}}, 30, clock(), InvalidTimestamp},
		{"bad timezone", []Item{valid, {ID: "other", CreatedAt: "2026-01-01T12:00:00+24:00", Status: "completed"}}, 30, clock(), InvalidTimestamp},
		{"comma fraction", []Item{valid, {ID: "other", CreatedAt: "2026-01-01T12:00:00,1Z", Status: "completed"}}, 30, clock(), InvalidTimestamp},
		{"negative bytes", []Item{valid, {ID: "other", CreatedAt: valid.CreatedAt, Status: "completed", TextBytes: -1}}, 30, clock(), InvalidBytes},
		{"total byte limit", []Item{valid, {ID: "other", CreatedAt: valid.CreatedAt, Status: "completed", TextBytes: MaxTextBytes}}, 30, clock(), BytesLimit},
		{"max int overflow", []Item{valid, {ID: "other", CreatedAt: valid.CreatedAt, Status: "completed", TextBytes: int(^uint(0) >> 1)}}, 30, clock(), BytesLimit},
		{"missing status", []Item{valid, {ID: "other", CreatedAt: valid.CreatedAt}}, 30, clock(), InvalidStatus},
		{"status control", []Item{valid, {ID: "other", CreatedAt: valid.CreatedAt, Status: "running\n"}}, 30, clock(), InvalidStatus},
		{"empty reason", []Item{valid, {ID: "other", CreatedAt: valid.CreatedAt, Status: "completed", ProtectedReasons: []string{""}}}, 30, clock(), InvalidReason},
		{"control reason", []Item{valid, {ID: "other", CreatedAt: valid.CreatedAt, Status: "completed", ProtectedReasons: []string{"pin\x00"}}}, 30, clock(), InvalidReason},
		{"long reason", []Item{valid, {ID: "other", CreatedAt: valid.CreatedAt, Status: "completed", ProtectedReasons: []string{strings.Repeat("x", 129)}}}, 30, clock(), InvalidReason},
		{"reason limit", []Item{valid, {ID: "other", CreatedAt: valid.CreatedAt, Status: "completed", ProtectedReasons: make([]string, 33)}}, 30, clock(), ReasonLimit},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := Plan(tc.items, tc.days, tc.now)
			var validation *ValidationError
			if !errors.As(err, &validation) || validation.Code != tc.code || !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("error: %v; want %s", err, tc.code)
			}
			if !reflect.DeepEqual(p, Preview{}) {
				t.Fatalf("partial preview on error: %+v", p)
			}
		})
	}
}

func TestBoundsCutoffAndUTCNormalization(t *testing.T) {
	items := make([]Item, 500)
	for i := range items {
		items[i] = item(fmt.Sprintf("item_%03d", i), "completed", 3651*24*time.Hour, 0)
	}
	items[0].TextBytes = MaxTextBytes
	p, err := Plan(items, 3650, clock())
	if err != nil || len(p.Candidates) != 500 || p.TotalBytes != MaxTextBytes || p.Enabled {
		t.Fatalf("exact bounds: %+v %v", p, err)
	}
	tz := time.FixedZone("fixture", 2*60*60)
	localNow := clock().In(tz)
	created := clock().Add(-24*time.Hour - time.Nanosecond).In(tz)
	p, err = Plan([]Item{{ID: "one", CreatedAt: created.Format(time.RFC3339Nano), Status: "completed"}}, 1, localNow)
	if err != nil || len(p.Candidates) != 1 || p.Candidates[0].CreatedAt != created.UTC().Format(time.RFC3339Nano) || p.AsOf != clock().Format(time.RFC3339Nano) {
		t.Fatalf("instant/normalization: %+v %v", p, err)
	}
}

func TestStableOrderReasonsAndInputAreIndependent(t *testing.T) {
	items := []Item{item("z", "completed", 60*24*time.Hour, 7, "z_reason", "a_reason", "a_reason"), item("b", "completed", 60*24*time.Hour, 2), item("a", "failed", 60*24*time.Hour, 3)}
	before, _ := json.Marshal(items)
	first, err := Plan(items, 30, clock())
	if err != nil {
		t.Fatal(err)
	}
	second, err := Plan([]Item{items[2], items[0], items[1]}, 30, clock())
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("order-dependent preview: %v", err)
	}
	after, _ := json.Marshal(items)
	if string(before) != string(after) {
		t.Fatal("planner mutated input")
	}
	if len(first.Protected) != 1 || len(first.Protected[0].Reasons) != 2 {
		t.Fatal("planner omitted the protected item or its deduplicated reasons")
	}
	first.Protected[0].Reasons[0] = "changed output"
	if items[0].ProtectedReasons[0] != "z_reason" {
		t.Fatal("planner shared reason backing storage")
	}
	empty, err := Plan(nil, 0, clock())
	if err != nil || empty.Candidates == nil || empty.Protected == nil || empty.Enabled || empty.TotalBytes != 0 {
		t.Fatalf("empty preview: %+v %v", empty, err)
	}
}

func FuzzPlan(f *testing.F) {
	f.Add("id", "2026-09-01T12:00:00Z", "completed", 30, 12, "context_pin")
	f.Add("id", "2026-10-13T12:00:00Z", "running", 0, 0, "")
	f.Add("bad/id", "", "", -1, -1, "bad\x00")
	f.Fuzz(func(t *testing.T, id, created, status string, days, size int, reason string) {
		reasons := []string{}
		if reason != "" {
			reasons = append(reasons, reason)
		}
		p, err := Plan([]Item{{ID: id, CreatedAt: created, Status: status, TextBytes: size, ProtectedReasons: reasons}}, days, clock())
		if err != nil {
			var validation *ValidationError
			if !errors.As(err, &validation) || !reflect.DeepEqual(p, Preview{}) {
				t.Fatal("untyped error or partial plan")
			}
			return
		}
		if p.Enabled || p.TotalBytes < 0 || p.TotalBytes > MaxTextBytes || len(p.Candidates)+len(p.Protected) != 1 {
			t.Fatal("unsafe preview")
		}
		if days == 0 && len(p.Candidates) > 0 {
			t.Fatal("keep-all produced candidate")
		}
		if len(p.Candidates) > 0 {
			parsed, _ := time.Parse(time.RFC3339Nano, created)
			if len(reasons) > 0 || (status != "completed" && status != "failed" && status != "cancelled") || !parsed.Before(clock().Add(-time.Duration(days)*24*time.Hour)) {
				t.Fatal("protected or recent item became candidate")
			}
		}
	})
}
