package hub

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScheduleTestHostedQuotaAndRestartReceipt(t *testing.T) {
	h, _, _ := newHub(t, func(c *Config) { c.Plans["free"] = 1 })
	u, err := h.users.signIn("routine-fixture@example.test", "email", "", "Routine", "free", 10, h.cfg.Now())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(h.userDir(u.ID), "aurora")
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "schedules.json"), []byte(`[{"id":"routine-fixture","bot":"bot-meiiie","text":"fixture test","at":"23:59","repeat":"daily","paused":true,"revision":1}]`), 0600); err != nil {
		t.Fatal(err)
	}
	cookie := httptest.NewRecorder()
	h.setSession(cookie, u)
	request := func(body string) (int, map[string]any) {
		r := httptest.NewRequest("POST", "/app/api/schedules/routine-fixture/test", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.AddCookie(cookie.Result().Cookies()[0])
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		var out map[string]any
		json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}
	body := `{"workspaceId":"account-` + u.ID + `","revision":1,"clientNonce":"routine-owner-test-0001"}`
	if code, _ := request(strings.Replace(body, "account-"+u.ID, "account-other", 1)); code != 409 || h.users.usedToday(u.ID, h.cfg.Now()) != 0 {
		t.Fatal("scope charged", code)
	}
	if code, _ := request(strings.Replace(body, `"revision":1`, `"revision":2`, 1)); code != 409 || h.users.usedToday(u.ID, h.cfg.Now()) != 0 {
		t.Fatal("stale revision charged", code)
	}
	code, receipt := request(body)
	if code != 202 || receipt["requestId"] == nil || h.users.usedToday(u.ID, h.cfg.Now()) != 1 {
		t.Fatal("initial reservation", code, receipt)
	}
	if code, out := request(body); code != 202 || out["requestId"] != receipt["requestId"] || h.users.usedToday(u.ID, h.cfg.Now()) != 1 {
		t.Fatal("retry charged", code)
	}
	if code, _ := request(strings.Replace(body, "0001", "0002", 1)); code != 429 {
		t.Fatal("new run bypassed quota", code)
	}
	h.close(u.ID)
	if code, out := request(body); code != 202 || out["requestId"] != receipt["requestId"] || h.users.usedToday(u.ID, h.cfg.Now()) != 1 {
		t.Fatal("restart receipt", code, out)
	}
	tenant, err := h.tenant(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tenant.store.Snapshot().Work) != 1 {
		t.Fatal("retry created extra work")
	}
}
