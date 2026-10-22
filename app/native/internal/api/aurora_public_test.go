package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nekoaurora.local/native/internal/ledger"
)

func TestAuroraPublicHost(t *testing.T) {
	store, err := ledger.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s, _ := New(store, "127.0.0.1:1")
	s.EnableAurora(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("aurora-ok")) }))
	s.EnableAuroraPublic("x.trycloudflare.com", "owner-code-123456")
	do := func(method, path, body, cookie, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Host = "x.trycloudflare.com"
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		if cookie != "" {
			r.Header.Set("Cookie", cookie)
		}
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	if w := do("GET", "/app/", "", "", ""); w.Code != 200 || strings.Contains(w.Header().Get("Set-Cookie"), "neko_local") {
		t.Fatalf("asset %d %q", w.Code, w.Header().Get("Set-Cookie"))
	}
	if w := do("GET", "/v1/session", "", "", ""); w.Code != 404 {
		t.Fatalf("v1 exposed: %d", w.Code)
	}
	if w := do("GET", "/app/api/state", "", "", ""); w.Code != 401 {
		t.Fatalf("unauth %d", w.Code)
	}
	if w := do("POST", "/app/login", `{"code":"wrong"}`, "", ""); w.Code != 401 {
		t.Fatalf("bad code %d", w.Code)
	}
	w := do("POST", "/app/login", `{"code":"owner-code-123456"}`, "", "https://x.trycloudflare.com")
	ck := w.Header().Get("Set-Cookie")
	if w.Code != 200 || !strings.Contains(ck, "Secure") || !strings.Contains(ck, "HttpOnly") {
		t.Fatalf("login %d %q", w.Code, ck)
	}
	cookie := strings.SplitN(ck, ";", 2)[0]
	if w := do("GET", "/app/api/state", "", cookie, ""); w.Code != 200 || w.Body.String() != "aurora-ok" {
		t.Fatalf("authed %d", w.Code)
	}
	if w := do("POST", "/app/api/brief", `{}`, cookie, "https://evil.example"); w.Code != 403 {
		t.Fatalf("cross-origin %d", w.Code)
	}
}
