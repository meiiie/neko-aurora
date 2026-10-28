package aurora

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"nekoaurora.local/native/internal/vault"
)

func mcpFixtureService(t *testing.T, credential string) *Service {
	t.Helper()
	v, err := vault.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if credential != "" {
		if _, err = v.Set(vault.Meta{Name: "MCP_FIXTURE"}, credential); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return &Service{ctx: ctx, V: v}
}

func TestMCPLifecycleWorkspaceIsolationRotationAndConcurrentInitialize(t *testing.T) {
	var initializations atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID     *int64 `json:"id"`
			Method string `json:"method"`
		}
		json.NewDecoder(r.Body).Decode(&in)
		if in.ID == nil {
			w.WriteHeader(202)
			return
		}
		var result any = map[string]any{"protocolVersion": "2025-06-18"}
		if in.Method == "initialize" {
			initializations.Add(1)
		}
		if in.Method == "tools/list" {
			result = map[string]any{"tools": []mcpTool{{Name: r.Header.Get("Authorization"), Schema: json.RawMessage(`{}`)}}}
		}
		json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": *in.ID, "result": result})
	}))
	defer server.Close()
	srv := mcpServer{Name: "same", URL: server.URL, Header: "Authorization", Secret: "MCP_FIXTURE"}
	a, b := mcpFixtureService(t, "fixture-owner-a"), mcpFixtureService(t, "fixture-owner-b")
	for _, test := range []struct {
		svc      *Service
		expected string
	}{{a, "fixture-owner-a"}, {b, "fixture-owner-b"}} {
		tools, err := test.svc.mcpTools(test.svc.ctx, "same-bot", srv)
		if err != nil || len(tools) != 1 || tools[0].Name != test.expected {
			t.Fatal("workspace credential or tools crossed service boundary", err)
		}
	}
	if _, err := a.V.Set(vault.Meta{Name: "MCP_FIXTURE"}, "fixture-rotated"); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tools, err := a.mcpTools(a.ctx, "same-bot", srv)
			if err != nil || len(tools) != 1 || tools[0].Name != "fixture-rotated" {
				t.Error("stale credential after rotation", err)
			}
		}()
	}
	wg.Wait()
	if initializations.Load() != 3 {
		t.Fatal("initialization was duplicated", initializations.Load())
	}
}

func TestMCPLifecycleHTTPFailureCooldownAndExplicitReconnect(t *testing.T) {
	for _, tc := range []struct {
		code  int
		state string
	}{{401, "reconnect_required"}, {403, "forbidden"}, {429, "rate_limited"}, {503, "unavailable"}} {
		t.Run(tc.state, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Retry-After", "60")
				w.WriteHeader(tc.code)
				w.Write([]byte("fixture remote secret must not be reflected"))
			}))
			defer server.Close()
			s := mcpFixtureService(t, "")
			srv := mcpServer{Name: "failure", URL: server.URL}
			for i := 0; i < 2; i++ {
				_, err := s.mcpTools(s.ctx, "bot", srv)
				var failure *mcpFailure
				if !errors.As(err, &failure) || failure.State != tc.state || strings.Contains(err.Error(), "fixture remote") {
					t.Fatal("failure not classified or redacted")
				}
				if tc.code == 429 && time.Until(failure.RetryAt) < 55*time.Second {
					t.Fatal("Retry-After ignored")
				}
			}
			if calls.Load() != 1 {
				t.Fatal("automatic repeat while blocked", calls.Load())
			}
			s.mcpDrop("bot", srv)
			s.mcpTools(s.ctx, "bot", srv)
			if calls.Load() != 2 {
				t.Fatal("explicit reconnect did not reset handshake")
			}
		})
	}
}

func TestMCPLifecycleNeverReplaysToolSideEffectsOrForwardsCredentials(t *testing.T) {
	var calls, leaks atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaks.Add(1); w.WriteHeader(500) }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	s := mcpFixtureService(t, "fixture-private-header")
	srv := mcpServer{Name: "redirect", URL: redirect.URL, Header: "X-Fixture", Secret: "MCP_FIXTURE"}
	if _, err := s.mcpTools(s.ctx, "bot", srv); err == nil || leaks.Load() != 0 {
		t.Fatal("credentials followed HTTP redirect")
	}
	if _, err := resolveRef(redirect.URL+"/sse", target.URL+"/messages"); err == nil {
		t.Fatal("cross-origin SSE endpoint accepted")
	}
	if _, err := resolveRef(redirect.URL+"/sse", "/messages"); err != nil {
		t.Fatal("same-origin endpoint rejected", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID     *int64 `json:"id"`
			Method string `json:"method"`
		}
		json.NewDecoder(r.Body).Decode(&in)
		if in.Method == "tools/call" {
			calls.Add(1)
			w.WriteHeader(401)
			return
		}
		if in.ID == nil {
			w.WriteHeader(202)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"id": *in.ID, "result": map[string]any{"protocolVersion": "2025-06-18"}})
	}))
	defer server.Close()
	srv.URL = server.URL
	for i := 0; i < 2; i++ {
		if _, err := s.mcpCall(s.ctx, "bot", srv, "write_note", nil); err == nil {
			t.Fatal("failed action acknowledged")
		}
	}
	if calls.Load() != 1 {
		t.Fatal("side effect replayed after authentication failure")
	}
}

func TestMCPLifecycleSSEHandshakeHonorsCallerDeadline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	s := mcpFixtureService(t, "")
	ctx, cancel := context.WithTimeout(s.ctx, 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := s.mcpTools(ctx, "bot", mcpServer{Name: "slow", URL: server.URL + "/sse"}); err == nil {
		t.Fatal("incomplete SSE handshake accepted")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("caller deadline ignored during GET handshake")
	}
}
