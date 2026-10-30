package web

import (
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestEveryAppScriptIsServedWithJavaScriptContentType(t *testing.T) {
	page := httptest.NewRecorder()
	Serve(page, httptest.NewRequest("GET", "/app/", nil))
	scripts := regexp.MustCompile(`<script[^>]+src="([^"]+)"`).FindAllStringSubmatch(page.Body.String(), -1)
	if len(scripts) == 0 {
		t.Fatal("app has no scripts")
	}
	for _, script := range scripts {
		path := script[1]
		t.Run(path, func(t *testing.T) {
			if !IsPublicAsset(path) {
				t.Fatal("script missing from public asset registry")
			}
			w := httptest.NewRecorder()
			Serve(w, httptest.NewRequest("GET", path, nil))
			if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/javascript") || w.Body.Len() == 0 {
				t.Fatal("script unavailable", w.Code)
			}
			etag := w.Header().Get("ETag")
			r := httptest.NewRequest("GET", path, nil)
			r.Header.Set("If-None-Match", etag)
			cached := httptest.NewRecorder()
			Serve(cached, r)
			if cached.Code != 304 {
				t.Fatal("script cache did not revalidate")
			}
		})
	}
}
