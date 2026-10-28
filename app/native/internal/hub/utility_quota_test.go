package hub

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMemoryModelCallHonorsExhaustedQuota(t *testing.T) {
	h, _, _ := newHub(t, nil)
	u, err := h.users.signIn("research@example.test", "email", "", "Research", "free", 10, h.cfg.Now())
	if err != nil {
		t.Fatal(err)
	}
	tenant, err := h.tenant(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	bot := tenant.store.Snapshot().Bots[0].ID
	if !tenant.svc.Mem.Remember(bot, "Bạn thích câu trả lời ngắn") {
		t.Fatal("memory setup failed")
	}
	for h.users.spend(u.ID, h.cfg.Now(), h.cfg.Plans[u.Plan]) {
	}
	cookie := httptest.NewRecorder()
	h.setSession(cookie, u)
	req := httptest.NewRequest("POST", "/app/api/agents/"+bot+"/memory/ask", strings.NewReader(`{"q":"Bạn nhớ gì?"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie.Result().Cookies()[0])
	aiBodies.Lock()
	before := len(aiBodies.l)
	aiBodies.Unlock()
	response := httptest.NewRecorder()
	h.ServeHTTP(response, req)
	aiBodies.Lock()
	calls := len(aiBodies.l) - before
	aiBodies.Unlock()
	if response.Code != http.StatusTooManyRequests || calls != 0 {
		t.Fatalf("quota exhausted: memory endpoint status=%d, fixture model calls=%d, used=%d", response.Code, calls, h.users.usedToday(u.ID, h.cfg.Now()))
	}
}

func TestUtilityQuotaCoversSummaryButNotApplyingReviewedMemory(t *testing.T) {
	h, _, _ := newHub(t, nil)
	u, err := h.users.signIn("utility@example.test", "email", "", "Utility", "free", 10, h.cfg.Now())
	if err != nil {
		t.Fatal(err)
	}
	tenant, err := h.tenant(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	bot := tenant.store.Snapshot().Bots[0].ID
	tenant.svc.Mem.Remember(bot, "Dùng tiếng Việt")
	tenant.svc.Mem.Remember(bot, "Câu trả lời ngắn")
	cookie := httptest.NewRecorder()
	h.setSession(cookie, u)
	request := func(path, body string) int {
		r := httptest.NewRequest("POST", "/app/api/agents/"+bot+path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.AddCookie(cookie.Result().Cookies()[0])
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	if request("/memory/ask", `{"q":"Bạn nhớ gì?"}`) != 200 || h.users.usedToday(u.ID, h.cfg.Now()) != 1 {
		t.Fatal("utility call unmetered")
	}
	for h.users.spend(u.ID, h.cfg.Now(), 3) {
	}
	aiBodies.Lock()
	before := len(aiBodies.l)
	aiBodies.Unlock()
	if request("/memory/summarize", `{}`) != 429 {
		t.Fatal("summary bypassed quota")
	}
	if request("/memory/summarize", `{"apply":["Dùng tiếng Việt ngắn gọn"]}`) != 200 {
		t.Fatal("owner edit incorrectly needs inference quota")
	}
	aiBodies.Lock()
	calls := len(aiBodies.l) - before
	aiBodies.Unlock()
	if calls != 0 {
		t.Fatal("unexpected inference after quota")
	}
}
