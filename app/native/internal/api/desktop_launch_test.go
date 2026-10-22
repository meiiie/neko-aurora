package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"nekoaurora.local/native/internal/ledger"
)

func TestDesktopLaunchBindsFreshStateAndChallenge(t *testing.T) {
	dir := t.TempDir()
	store, e := ledger.Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	server, e := New(store, "127.0.0.1:18767")
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(dir, "desktop-launch.json")
	if e = server.EnableDesktopLaunch(dir, path); e != nil {
		t.Fatal(e)
	}
	var receipt map[string]any
	raw, e := os.ReadFile(path)
	if e != nil || json.Unmarshal(raw, &receipt) != nil {
		t.Fatal("receipt unavailable")
	}
	if receipt["endpoint"] != "http://127.0.0.1:18767" || receipt["controlPid"] != float64(os.Getpid()) || len(receipt["stateSha256"].(string)) != 64 || len(receipt["controlSha256"].(string)) != 64 {
		t.Fatal("unbound instance")
	}
	for _, test := range []struct {
		name, token, origin string
		code                int
	}{{"valid", receipt["challenge"].(string), "", 200}, {"missing", "", "", 403}, {"wrong", "wrong", "", 403}, {"remote", receipt["challenge"].(string), "https://example.invalid", 403}} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", "http://127.0.0.1:18767/v1/local-instance", nil)
			request.Header.Set("X-Neko-Local-Launch", test.token)
			request.Header.Set("Origin", test.origin)
			response := httptest.NewRecorder()
			server.ServeHTTP(response, request)
			if response.Code != test.code {
				t.Fatalf("status %d", response.Code)
			}
			if test.code == 200 {
				var got map[string]any
				if json.Unmarshal(response.Body.Bytes(), &got) != nil || got["stateSha256"] != receipt["stateSha256"] || got["challenge"] != nil {
					t.Fatal("wrong/public challenge response")
				}
				if response.Header().Get("Content-Security-Policy") == "" {
					t.Fatal("CSP dropped")
				}
			}
		})
	}
	if e = server.EnableDesktopLaunch(dir, path); e == nil {
		t.Fatal("stale receipt overwritten")
	}
}

func TestDesktopLaunchRejectsRemoteAndForeignState(t *testing.T) {
	for _, host := range []string{"example.invalid:80", "0.0.0.0:18767", "[::1]:18767", "127.0.0.1:0"} {
		t.Run(host, func(t *testing.T) {
			dir := t.TempDir()
			store, e := ledger.Open(dir)
			if e != nil {
				t.Fatal(e)
			}
			defer store.Close()
			server, _ := New(store, host)
			if server.EnableDesktopLaunch(dir, filepath.Join(dir, "launch.json")) == nil {
				t.Fatal("remote or noncanonical endpoint accepted")
			}
		})
	}
	dir := t.TempDir()
	store, e := ledger.Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	server, _ := New(store, "127.0.0.1:18767")
	if server.EnableDesktopLaunch(dir, filepath.Join(t.TempDir(), "foreign.json")) == nil {
		t.Fatal("receipt escaped state")
	}
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:18767/v1/local-instance", nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != 404 {
		t.Fatal("disabled instance discovery exposed")
	}
}
