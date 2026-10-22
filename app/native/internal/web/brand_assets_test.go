package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The PWA manifest, service worker precache and HTML must only point at
// brand assets that are embedded and served with the right content type.
func TestBrandAssetsServed(t *testing.T) {
	get := func(p string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		Serve(w, httptest.NewRequest(http.MethodGet, p, nil))
		return w
	}
	want := map[string]string{
		"/app/icon-180.png": "image/png", "/app/icon-192.png": "image/png", "/app/icon-512.png": "image/png",
		"/app/icon-maskable-512.png": "image/png", "/app/favicon.ico": "image/x-icon", "/app/icon.svg": "image/svg+xml",
		"/app/aurora/wordmark.svg": "image/svg+xml",
	}
	for p, ct := range want {
		w := get(p)
		if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), ct) || w.Body.Len() == 0 {
			t.Fatalf("%s: code %d type %q len %d", p, w.Code, w.Header().Get("Content-Type"), w.Body.Len())
		}
	}
	var m struct{ Icons []struct{ Src, Purpose string } }
	if err := json.Unmarshal(get("/app/manifest.webmanifest").Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	maskable := false
	for _, ic := range m.Icons {
		if get(ic.Src).Code != 200 {
			t.Fatalf("manifest icon %s not served", ic.Src)
		}
		maskable = maskable || strings.Contains(ic.Purpose, "maskable")
	}
	if !maskable {
		t.Fatal("manifest has no maskable icon")
	}
	sw := get("/app/sw.js").Body.String()
	for _, p := range []string{"/app/icon-192.png", "/app/favicon.ico"} {
		if !strings.Contains(sw, p) {
			t.Fatalf("sw precache missing %s", p)
		}
	}
}
