package cloudbrowser

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCloudLocalDockerIsExplicitAndLoopbackOnly(t *testing.T) {
	for _, endpoint := range []string{"", "http://localhost:8877", "http://127.0.0.2:8877", "http://[::1]:8877", "http://127.0.0.1", "http://127.0.0.1:80", "http://127.0.0.1:08877", "https://127.0.0.1:8877", "https://worker.example.test", "http://browser.aurora.internal:8877", "http://127.0.0.1:8877/?key=x", "http://user:pass@127.0.0.1:8877"} {
		_, err := New(Config{LocalDocker: true, URL: endpoint, Key: []byte(strings.Repeat("fixture!", 4)), StateDir: t.TempDir()})
		if !errors.Is(err, ErrConfiguration) {
			t.Fatalf("accepted Docker endpoint %q", endpoint)
		}
	}
	if _, err := New(Config{URL: "http://127.0.0.1:8877", Key: []byte(strings.Repeat("fixture!", 4)), StateDir: t.TempDir()}); !errors.Is(err, ErrConfiguration) {
		t.Fatal("loopback accepted without explicit startup mode")
	}
}

func TestCloudLocalDockerIgnoresProxyAndRefusesRedirect(t *testing.T) {
	var unwanted atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { unwanted.Add(1); w.WriteHeader(502) }))
	defer other.Close()
	t.Setenv("HTTP_PROXY", other.URL)
	t.Setenv("NO_PROXY", "")
	redirect := false
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if redirect {
			http.Redirect(w, r, other.URL, http.StatusTemporaryRedirect)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"profileId":"bot-fixture","status":"stopped","control":"paused","epoch":1,"authState":"unknown","protected":true}`))
	}))
	defer worker.Close()
	c, err := New(Config{LocalDocker: true, URL: worker.URL, Key: []byte(strings.Repeat("fixture!", 4)), StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if !c.LocalDocker() {
		t.Fatal("Docker location lost")
	}
	request := Command{Profile: "bot-fixture", Kind: "status", Actor: "owner", ActorID: "fixture-owner"}
	if _, err := c.Do(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	redirect = true
	if _, err := c.Do(context.Background(), request); err == nil {
		t.Fatal("redirect accepted")
	}
	if unwanted.Load() != 0 {
		t.Fatal("private browser command reached an environment proxy or redirect target")
	}
}
