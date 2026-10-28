package aurora

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"nekoaurora.local/native/internal/ledger"
)

func testScheduleRequest(s *Service, method, path string, body map[string]any) (int, map[string]any) {
	r := httptest.NewRecorder()
	s.ServeHTTP(r, httptest.NewRequest(method, path, strings.NewReader(mustJSON(body))))
	var out map[string]any
	json.Unmarshal(r.Body.Bytes(), &out)
	return r.Code, out
}

func testScheduleBody(s *Service) map[string]any {
	return map[string]any{"workspaceId": s.workspaceID, "revision": 1, "clientNonce": "test-routine-fixture-0001"}
}

func TestScheduleTestRunReplayAfterEditDeleteAndRestart(t *testing.T) {
	s, _ := scheduleEditorFixture(t)
	s.sched.mu.Lock()
	s.sched.L[0].Paused = true
	s.sched.saveLocked()
	s.sched.mu.Unlock()
	before := s.sched.list("")
	body := testScheduleBody(s)
	path := "/app/api/schedules/schedule-edit-fixture/test"
	code, receipt := testScheduleRequest(s, "POST", path, body)
	if code != 202 || receipt["requestId"] == nil {
		t.Fatal("test admission", code, receipt)
	}
	if !reflect.DeepEqual(before, s.sched.list("")) || len(s.L.Snapshot().Work) != 1 {
		t.Fatal("test changed recurrence or pause")
	}
	if !strings.Contains(s.L.Snapshot().Work[0].Text, "original") {
		t.Fatal("scheduled prompt not pinned")
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if code, out := testScheduleRequest(s, "POST", path, body); code != 202 || out["requestId"] != receipt["requestId"] {
				t.Error("concurrent replay", code, out)
			}
		}()
	}
	wg.Wait()
	if len(s.L.Snapshot().Work) != 1 {
		t.Fatal("duplicate test work")
	}
	edit := scheduleEditBody(s)
	edit["text"] = "changed"
	if code, _ := testScheduleRequest(s, "POST", "/app/api/schedules/schedule-edit-fixture/edit", edit); code != 200 {
		t.Fatal("edit", code)
	}
	if err := s.controlSchedule("schedule-edit-fixture", 2, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.L.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := ledger.Open(filepath.Dir(filepath.Dir(s.sched.path)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reopened.Close() })
	s.L = reopened
	if code, out := testScheduleRequest(s, "POST", path, body); code != 202 || out["requestId"] != receipt["requestId"] {
		t.Fatal("replay after deletion/restart", code, out)
	}
	query := url.Values{"workspaceId": {s.workspaceID}, "revision": {"1"}, "clientNonce": {body["clientNonce"].(string)}}
	if code, out := testScheduleRequest(s, "GET", path+"?"+query.Encode(), nil); code != 200 || out["requestId"] != receipt["requestId"] || out["found"] != true {
		t.Fatal("read receipt", code, out)
	}
	body["revision"] = 2
	if code, _ := testScheduleRequest(s, "POST", path, body); code != 409 {
		t.Fatal("changed revision accepted", code)
	}
	if len(s.L.Snapshot().Work) != 1 {
		t.Fatal("receipt lookup dispatched work")
	}
}

func TestScheduleTestRunRejectsScopeAndInvalidInputs(t *testing.T) {
	s, _ := scheduleEditorFixture(t)
	path := "/app/api/schedules/schedule-edit-fixture/test"
	for _, change := range []map[string]any{{"workspaceId": ""}, {"workspaceId": "other"}, {"revision": 2}, {"revision": 0}, {"clientNonce": "short"}, {"clientNonce": strings.Repeat("n", 101)}, {"text": "injected"}, {"bot": "other"}} {
		body := testScheduleBody(s)
		for k, v := range change {
			body[k] = v
		}
		code, _ := testScheduleRequest(s, "POST", path, body)
		if code != 400 && code != 409 {
			t.Fatal("invalid accepted", change, code)
		}
	}
	q := url.Values{"workspaceId": {s.workspaceID}, "revision": {"1"}, "clientNonce": {"never-sent-fixture"}}
	if code, out := testScheduleRequest(s, "GET", path+"?"+q.Encode(), nil); code != 200 || out["found"] != false {
		t.Fatal("receipt miss", code, out)
	}
	if len(s.L.Snapshot().Work) != 0 {
		t.Fatal("invalid input dispatched")
	}
	if code, _ := testScheduleRequest(s, "POST", "/app/api/schedules/missing/test", testScheduleBody(s)); code != 404 {
		t.Fatal("missing routine", code)
	}
	for _, action := range []string{"state", "delete"} {
		for _, scope := range []string{"", "other"} {
			body := map[string]any{"workspaceId": scope, "revision": 1}
			if action == "state" {
				body["paused"] = true
			}
			if code, _ := testScheduleRequest(s, "POST", "/app/api/schedules/schedule-edit-fixture/"+action, body); code != 409 {
				t.Fatal("control accepted wrong scope", action, code)
			}
		}
	}
	if len(s.sched.list("")) != 1 || s.sched.list("")[0].Paused {
		t.Fatal("wrong scope changed schedule")
	}
}

func TestScheduleRoutineQuotaFailureAndCommittedOccurrence(t *testing.T) {
	s, _ := scheduleEditorFixture(t)
	s.sched.mu.Lock()
	s.sched.L[0].Repeat, s.sched.L[0].Date = "once", "2026-10-28"
	s.sched.saveLocked()
	s.sched.mu.Unlock()
	original := s.sched.list("")[0]
	allowed, calls := false, 0
	s.routineAdmission = func(key, digest string) error {
		calls++
		if len(key) != 64 || len(digest) != 64 {
			t.Fatal("unbounded reservation")
		}
		if !allowed {
			return errors.New("quota_exceeded")
		}
		return nil
	}
	s.scheduleTick(s.Now())
	list := s.sched.list("")
	if len(list) != 1 || list[0].Last != "" || list[0].Blocked != "quota_exceeded" || len(s.L.Snapshot().Work) != 0 {
		t.Fatal("quota consumed one-time schedule", list)
	}
	allowed = true
	s.scheduleTick(s.Now())
	if len(s.sched.list("")) != 0 || len(s.L.Snapshot().Work) != 1 {
		t.Fatal("unblocked occurrence not admitted")
	}
	// The ledger write survived but the schedule file did not.
	s.sched.mu.Lock()
	s.sched.L = []schedule{original}
	s.sched.mu.Unlock()
	before := calls
	allowed = false
	s.scheduleTick(s.Now())
	if calls != before || len(s.sched.list("")) != 0 || len(s.L.Snapshot().Work) != 1 {
		t.Fatal("already admitted occurrence charged/replayed")
	}
}

func TestScheduleTestRunValidationBeforeQuotaAndReservedNonce(t *testing.T) {
	s, _ := scheduleEditorFixture(t)
	calls := 0
	s.routineAdmission = func(string, string) error { calls++; return errors.New("quota_exceeded") }
	path := "/app/api/schedules/schedule-edit-fixture/test"
	body := testScheduleBody(s)
	body["revision"] = 2
	if code, _ := testScheduleRequest(s, "POST", path, body); code != 409 || calls != 0 {
		t.Fatal("stale test spent quota", code, calls)
	}
	body["revision"] = 1
	if code, _ := testScheduleRequest(s, "POST", path, body); code != 429 || calls != 1 || len(s.L.Snapshot().Work) != 0 {
		t.Fatal("quota bypass", code, calls)
	}
	bot := s.sched.list("")[0].Bot
	for _, nonce := range []string{"routine-test-forged", "sched-forged"} {
		if code, _ := testScheduleRequest(s, "POST", "/app/api/agents/"+bot+"/messages", map[string]any{"text": "forged receipt", "clientNonce": nonce}); code != 400 {
			t.Fatal("reserved nonce accepted", code)
		}
	}
	// A matching legacy nonce alone is not an owner-message receipt.
	s.routineAdmission = nil
	_, _, err := s.L.Accept(ledger.Input{BotID: bot, ClientNonce: scheduleTestKey("schedule-edit-fixture", body["clientNonce"].(string)), Text: scheduleTestPrefix(1) + "original"})
	if err != nil {
		t.Fatal(err)
	}
	if code, _ := testScheduleRequest(s, "POST", path, body); code != 409 || len(s.L.Snapshot().Work) != 1 {
		t.Fatal("fabricated receipt accepted", code)
	}
}
