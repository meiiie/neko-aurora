package api

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nekoaurora.local/native/internal/ledger"
)

func TestEmbeddedFontResponseAndStaticBoundary(t *testing.T) {
	store, err := ledger.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	server, err := New(store, "127.0.0.1:8123")
	if err != nil {
		t.Fatal(err)
	}
	fonts := map[string]string{
		"/fonts/geist-latin-wght-normal.woff2":      "19f9c92546aa300c312235e3125af1b81394d8db9a4bc4a425cd5b641d2d54e1",
		"/fonts/geist-latin-ext-wght-normal.woff2":  "824f485b5d26e2f2da3c2b236132ece1bc8e4e43373452950bb0e40548b4313f",
		"/fonts/geist-vietnamese-wght-normal.woff2": "8fa40e5d248247735eb97a0bd593b8852440430600d6ba01364c31fe0abc1fe1",
	}
	for path, expected := range fonts {
		t.Run(path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8123"+path, nil)
			response := httptest.NewRecorder()
			server.ServeHTTP(response, request)
			if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "font/woff2" {
				t.Fatal("font response", response.Code, response.Header())
			}
			if fmt.Sprintf("%x", sha256.Sum256(response.Body.Bytes())) != expected || !strings.HasPrefix(response.Body.String(), "wOF2") {
				t.Fatal("licensed font bytes changed")
			}
			policy := response.Header().Get("Content-Security-Policy")
			for _, directive := range []string{"default-src 'none'", "font-src 'self'", "script-src 'self'", "style-src 'self'", "connect-src 'self'"} {
				if !strings.Contains(policy, directive) {
					t.Fatal("missing CSP boundary", directive)
				}
			}
			if strings.Contains(policy, "data:") || strings.Contains(policy, "unsafe-inline") || response.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatal("static policy was broadened")
			}
		})
	}
	for _, path := range []string{"/fonts/unknown.woff2", "/fonts/Geist-OFL-1.1.txt", "/fonts/../style.css", "/index.html", "/brand/unknown.svg"} {
		t.Run("denied"+path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8123"+path, nil)
			request.AddCookie(&http.Cookie{Name: "neko_local", Value: server.session})
			response := httptest.NewRecorder()
			server.ServeHTTP(response, request)
			if response.Code != http.StatusNotFound {
				t.Fatal("unlisted static path escaped boundary", response.Code)
			}
		})
	}
	for _, method := range []string{http.MethodPost, http.MethodHead} {
		t.Run(method, func(t *testing.T) {
			request := httptest.NewRequest(method, "http://127.0.0.1:8123/fonts/geist-latin-wght-normal.woff2", nil)
			request.AddCookie(&http.Cookie{Name: "neko_local", Value: server.session})
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			server.ServeHTTP(response, request)
			if response.Code != http.StatusNotFound {
				t.Fatal("non-GET font path exposed", response.Code)
			}
		})
	}
	request := httptest.NewRequest(http.MethodGet, "http://untrusted.example/fonts/geist-latin-wght-normal.woff2", nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatal("font serving bypassed host gate", response.Code)
	}
}
