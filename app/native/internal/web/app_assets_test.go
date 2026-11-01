package web

import (
	"bytes"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestFontStylesheetsUseServedWOFF2Assets(t *testing.T) {
	for _, sheet := range []string{"fonts.css", "typography.css", "fonts/noto-sans-sc/index.css", "fonts/noto-sans-tc/index.css"} {
		w := httptest.NewRecorder()
		Serve(w, httptest.NewRequest("GET", "/app/aurora/"+sheet, nil))
		if w.Code != 200 {
			t.Fatalf("stylesheet %s unavailable", sheet)
		}
		urls := regexp.MustCompile(`url\(["']?([^\s)"']+\.woff2)["']?\)`).FindAllStringSubmatch(w.Body.String(), -1)
		if len(urls) == 0 {
			t.Fatalf("stylesheet %s has no fonts", sheet)
		}
		for _, match := range urls {
			path := match[1]
			if !strings.HasPrefix(path, "/app/aurora/fonts/") || !IsPublicAsset(path) {
				t.Fatalf("font is not a public local asset: %s", path)
			}
			font := httptest.NewRecorder()
			Serve(font, httptest.NewRequest("GET", path, nil))
			if font.Code != 200 || !strings.HasPrefix(font.Header().Get("Content-Type"), "font/woff2") || !bytes.HasPrefix(font.Body.Bytes(), []byte("wOF2")) {
				t.Fatalf("font %s is not served as WOFF2", path)
			}
		}
	}
}

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
