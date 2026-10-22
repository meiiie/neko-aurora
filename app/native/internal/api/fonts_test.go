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
		"/app/aurora/fonts/inter-var.woff2":                       "90b7f11cafe7a22f64b2279fc08e3be05082dc59e39a01facae9e25d375c4f2e",
		"/app/aurora/fonts/bevietnampro-400-latin.woff2":          "48365fa93eeb5b730d595061e2c48d97e2bdf4990ef725a7da4193bfa60f485c",
		"/app/aurora/fonts/bevietnampro-400-vietnamese.woff2":     "b62689bd59a16789eedb19bde3ac7837c42a06ed766be995a5d7ccd87eb4687d",
		"/app/aurora/fonts/bevietnampro-600-latin.woff2":          "5d85274fcb42d8c2b744368f7d2d5bd7b639ad1988edc5d0ab5441b2fec623a7",
		"/app/aurora/fonts/bevietnampro-600-vietnamese.woff2":     "d01ed2481cb5d300cf8e685c9380a5cd0fda04e1a6e9daaa23ca40ad3186d414",
		"/app/aurora/fonts/bevietnampro-700-latin.woff2":          "9b36f6531727ab5ddc12156b9ce5fcb8e79ee9dd9fae91fc013cd570ea17787c",
		"/app/aurora/fonts/bevietnampro-700-vietnamese.woff2":     "d3972cb7f4d4b7954be61d515706b52301e73a4e2b9d9c530349aa79571a00c3",
		"/app/aurora/fonts/jetbrainsmono-var-latin.woff2":         "f07ae4ea44a1b15709eb075142b37ddf784bce5ee4a0b8dd5547453bdd19ff13",
		"/app/aurora/fonts/jetbrainsmono-var-vietnamese.woff2":    "d438b2cf700ed5c992cad3cb5f255e30dded054288c44c01a574b06284b0ceac",
		"/app/aurora/fonts/notoserifdisplay-var-latin.woff2":      "77e4829f262a580c876ec2742a1fce82196e0a2771b874ba348847fd0117b4d5",
		"/app/aurora/fonts/notoserifdisplay-var-vietnamese.woff2": "d8557080a2bbc82d6301f481f34063046fd57b016a577f4a6ace9979d97a5fc9",
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
			if strings.Contains(policy, "font-src 'self' data:") || strings.Contains(policy, "unsafe-inline") || response.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatal("static policy was broadened")
			}
		})
	}
	for _, path := range []string{"/fonts/geist-latin-wght-normal.woff2", "/app/aurora/fonts/unknown.woff2", "/app/aurora/fonts/../style.css", "/index.html", "/brand/unknown.svg"} {
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
			request := httptest.NewRequest(method, "http://127.0.0.1:8123/app/aurora/fonts/bevietnampro-400-latin.woff2", nil)
			request.AddCookie(&http.Cookie{Name: "neko_local", Value: server.session})
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			server.ServeHTTP(response, request)
			if response.Code != http.StatusNotFound && response.Code != http.StatusMethodNotAllowed {
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
