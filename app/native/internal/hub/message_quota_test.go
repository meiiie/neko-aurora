package hub

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestMessageRetryReservationIsDurableAndBoundToPayload(t *testing.T) {
	h, _, _ := newHub(t, nil)
	u, err := h.users.signIn("retry@example.test", "email", "", "Retry", "free", 10, h.cfg.Now())
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := h.users.reserveMessage(u.ID, "message-key", "payload", h.cfg.Now(), 1); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if h.users.usedToday(u.ID, h.cfg.Now()) != 1 {
		t.Fatal("duplicate charged")
	}
	reopened, err := openUsers(h.users.path)
	if err != nil {
		t.Fatal(err)
	}
	if err = reopened.reserveMessage(u.ID, "message-key", "payload", h.cfg.Now(), 1); err != nil {
		t.Fatal("retry lost on restart", err)
	}
	if err = reopened.reserveMessage(u.ID, "message-key", "different", h.cfg.Now(), 1); err == nil || err.Error() != "nonce_conflict" {
		t.Fatal("mutable payload accepted", err)
	}
	if err = reopened.reserveMessage(u.ID, "new-key", "payload", h.cfg.Now(), 1); err == nil || err.Error() != "quota_exceeded" {
		t.Fatal("quota bypass", err)
	}
}

func TestMessageRetryHTTPReturnsSameReceiptAtExhaustedQuota(t *testing.T) {
	h, _, _ := newHub(t, nil)
	u, err := h.users.signIn("http-retry@example.test", "email", "", "Retry", "free", 10, h.cfg.Now())
	if err != nil {
		t.Fatal(err)
	}
	tenant, err := h.tenant(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	bot := tenant.store.Snapshot().Bots[0].ID
	cookie := httptest.NewRecorder()
	h.setSession(cookie, u)
	request := func(body string) (int, map[string]any) {
		r := httptest.NewRequest("POST", "/app/api/agents/"+bot+"/messages", strings.NewReader(body))
		r.AddCookie(cookie.Result().Cookies()[0])
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		var value map[string]any
		json.Unmarshal(w.Body.Bytes(), &value)
		return w.Code, value
	}
	body := `{"text":"retry fixture","clientNonce":"retry-receipt-20261028","workspaceId":"account-` + u.ID + `"}`
	code, first := request(body)
	if code != 202 {
		t.Fatal("initial", code)
	}
	for h.users.spend(u.ID, h.cfg.Now(), h.cfg.Plans[u.Plan]) {
	}
	code, again := request(body)
	if code != 202 || again["id"] != first["id"] {
		t.Fatal("retry", code, again["error"])
	}
	if code, _ = request(strings.Replace(body, "retry fixture", "different", 1)); code != 409 {
		t.Fatal("changed payload", code)
	}
	used := h.users.usedToday(u.ID, h.cfg.Now())
	if code, _ = request(strings.Replace(body, "account-"+u.ID, "account-other", 1)); code != 409 {
		t.Fatal("wrong workspace", code)
	}
	if h.users.usedToday(u.ID, h.cfg.Now()) != used {
		t.Fatal("wrong workspace charged")
	}
}
