package aurora

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDirectStreamRetriesTransientOnce(t *testing.T) {
	retryDelay = 10 * time.Millisecond
	var n int32
	fa := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&n, 1) == 1 {
			http.Error(w, "busy", 503)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ổn\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer fa.Close()
	a := &AI{BaseURL: fa.URL, Model: "m", key: "k", http: fa.Client()}
	out, err := a.Stream(context.Background(), []ChatMsg{{"user", "hi"}}, func(string) {})
	if err != nil || out != "ổn" || n != 2 {
		t.Fatalf("%v %q %d", err, out, n)
	}
	// a 400 is not transient: no retry, honest error
	n = 10
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { atomic.AddInt32(&n, 1); http.Error(w, "bad", 400) }))
	defer bad.Close()
	a.BaseURL = bad.URL
	if _, err := a.Stream(context.Background(), []ChatMsg{{"user", "hi"}}, func(string) {}); err == nil || n != 11 {
		t.Fatalf("%v %d", err, n)
	}
}

func TestHybridFallsBackToCoreOnlyBeforeText(t *testing.T) {
	retryDelay = time.Millisecond
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "x", 502) }))
	defer down.Close()
	dir := t.TempDir()
	script := filepath.Join(dir, "bun")
	os.WriteFile(script, []byte("#!/bin/sh\nprintf 'Từ Core.'\n"), 0o755)
	h := &Hybrid{Fast: &AI{BaseURL: down.URL, Model: "glm-5.3", key: "k", http: down.Client()}, Core: &Core{Bun: script, Entry: "x", Home: dir, Model: "glm-5.3", Profile: "zai", key: "k"}}
	out, err := h.Stream(context.Background(), []ChatMsg{{"user", "hi"}}, func(string) {})
	if err != nil || out != "Từ Core." || h.Runtime() != "direct-http" {
		t.Fatalf("%v %q", err, out)
	}
}

func TestAPNsTokenStoredInVaultOnly(t *testing.T) {
	svc, _ := newSvc(t)
	srv := httptest.NewServer(svc)
	defer srv.Close()
	tok := strings.Repeat("ab", 32)
	for _, c := range []struct {
		body string
		code int
	}{{`{"token":"nothex!"}`, 400}, {`{"token":"` + tok + `"}`, 200}} {
		res, err := http.Post(srv.URL+"/app/api/push/apns", "application/json", strings.NewReader(c.body))
		if err != nil || res.StatusCode != c.code {
			t.Fatalf("%s → %v %v", c.body, res.StatusCode, err)
		}
	}
	if v, ok := svc.V.Get("APNS_DEVICE_TOKEN"); !ok || v != tok {
		t.Fatal("token not in vault")
	}
}
