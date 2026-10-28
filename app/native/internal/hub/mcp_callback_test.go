package hub

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMCPOAuthCallbackHostedAssetsDoNotCreateSession(t *testing.T) {
	h, server, _ := newHub(t, func(c *Config) { c.PublicURL = "https://aurora.example.com" })
	request := httptest.NewRequest("GET", "/app/oauth/mcp/callback.html?code=fixture-code", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, request)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Set-Cookie") != "" {
		t.Fatal("hosted callback bootstrapped or cached")
	}
	client := newClient(t, server)
	if status, _ := client.do(http.MethodPost, "/app/api/mcp/oauth/callback", map[string]string{"state": "fixture"}); status != 401 {
		t.Fatal("hosted callback accepted missing session", status)
	}
}
