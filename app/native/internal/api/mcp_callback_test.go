package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nekoaurora.local/native/internal/ledger"
)

func TestMCPOAuthCallbackRequiresExistingOwnerSession(t *testing.T) {
	for _, public := range []bool{false, true} {
		t.Run(map[bool]string{false: "local", true: "public"}[public], func(t *testing.T) {
			store, err := ledger.Open(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			s, _ := New(store, "127.0.0.1:41789")
			calls := 0
			s.EnableAurora(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(204) }))
			host, origin, cookie := "127.0.0.1:41789", "http://127.0.0.1:41789", &http.Cookie{Name: "neko_local", Value: s.session}
			if public {
				s.EnableAuroraPublic("aurora.example.com", "fixture-owner-code")
				host = "aurora.example.com"
				origin = "https://" + host
				cookie = &http.Cookie{Name: auroraCookie, Value: s.auroraPub.session}
			}
			do := func(method, path string, authenticated bool, requestOrigin string) *httptest.ResponseRecorder {
				r := httptest.NewRequest(method, path, strings.NewReader(`{}`))
				r.Host = host
				r.Header.Set("Content-Type", "application/json")
				if requestOrigin != "" {
					r.Header.Set("Origin", requestOrigin)
				}
				if authenticated {
					r.AddCookie(cookie)
				}
				w := httptest.NewRecorder()
				s.ServeHTTP(w, r)
				return w
			}
			w := do("GET", "/app/oauth/mcp/callback.html?code=fixture-code", false, "")
			if w.Code != 200 || w.Header().Get("Set-Cookie") != "" || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("callback asset created session or cached")
			}
			path := "/app/api/mcp/oauth/callback"
			if w := do("POST", path, false, origin); w.Code != 401 {
				t.Fatal("unauthenticated callback admitted")
			}
			if w := do("POST", path, true, "https://evil.example"); w.Code != 403 {
				t.Fatal("foreign callback origin admitted")
			}
			if w := do("POST", path, true, origin); w.Code != 204 || calls != 1 {
				t.Fatal("existing session rejected", w.Code)
			}
		})
	}
}
