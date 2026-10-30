package web

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMCPOAuthCallbackAssetsNeverCacheOrReflectQuery(t *testing.T) {
	for _, ext := range []string{"html", "js"} {
		path := "/app/oauth/mcp/callback." + ext
		if !IsPublicAsset(path) {
			t.Fatal("callback unavailable before authentication")
		}
		r := httptest.NewRequest("GET", path+"?code=fixture-private-code", nil)
		r.Header.Set("If-None-Match", "*")
		w := httptest.NewRecorder()
		Serve(w, r)
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("ETag") != "" || w.Header().Get("Referrer-Policy") != "no-referrer" || w.Header().Get("Set-Cookie") != "" || !strings.Contains(w.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") || strings.Contains(w.Body.String(), "fixture-private-code") {
			t.Fatal("unsafe callback asset response", ext)
		}
	}
}
