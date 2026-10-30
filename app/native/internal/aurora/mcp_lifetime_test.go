package aurora

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type mcpLifetimeReadProbe struct {
	io.ReadCloser
	entered chan struct{}
	once    sync.Once
}

func (p *mcpLifetimeReadProbe) Read(b []byte) (int, error) {
	p.once.Do(func() { close(p.entered) })
	return p.ReadCloser.Read(b)
}

func mcpLifetimeServer(t *testing.T, blocked bool) (*httptest.Server, <-chan struct{}, *atomic.Int32) {
	t.Helper()
	entered, release := make(chan struct{}, 8), make(chan struct{})
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		var in struct {
			ID     *int64 `json:"id"`
			Method string `json:"method"`
		}
		if json.NewDecoder(r.Body).Decode(&in) != nil {
			w.WriteHeader(400)
			return
		}
		if blocked && in.Method == "tools/call" {
			entered <- struct{}{}
			select {
			case <-r.Context().Done():
			case <-release:
			}
			return
		}
		if in.ID == nil {
			w.WriteHeader(202)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"id": *in.ID, "result": map[string]any{"protocolVersion": "2025-06-18"}})
	}))
	t.Cleanup(func() { close(release); server.Close() })
	return server, entered, &requests
}

func TestMCPConnectionLifetimeDropAndShutdownCancelHTTP(t *testing.T) {
	for _, operation := range []string{"drop", "shutdown"} {
		t.Run(operation, func(t *testing.T) {
			server, entered, requests := mcpLifetimeServer(t, true)
			s := mcpFixtureService(t, "")
			var shutdown context.CancelFunc
			s.ctx, shutdown = context.WithCancel(s.ctx)
			defer shutdown()
			srv := mcpServer{Name: "fixture", URL: server.URL}
			c, err := s.mcpConnect(context.Background(), "bot", srv)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- c.call(ctx, "tools/call", map[string]any{"name": "write_note"}, nil) }()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("tool did not reach local server")
			}
			if operation == "drop" {
				if err := s.mcpDrop(context.Background(), "bot", srv); err != nil {
					t.Fatal(err)
				}
			} else {
				shutdown()
			}
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("expected lifetime cancellation, got %v", err)
				}
			case <-time.After(500 * time.Millisecond):
				t.Fatal("connection lifetime did not cancel HTTP request")
			}
			if !c.ended() {
				t.Fatal("closed HTTP connection remains usable")
			}
			if requests.Load() != 3 {
				t.Fatal("cancellation replayed a request")
			}
		})
	}
}

func TestMCPConnectionLifetimeClosedHandleNeverDispatches(t *testing.T) {
	server, _, requests := mcpLifetimeServer(t, false)
	s := mcpFixtureService(t, "")
	srv := mcpServer{Name: "fixture", URL: server.URL}
	c, err := s.mcpConnect(s.ctx, "bot", srv)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.mcpDrop(s.ctx, "bot", srv); err != nil {
		t.Fatal(err)
	}
	before := requests.Load()
	if err := c.call(s.ctx, "tools/call", map[string]any{"name": "write_note"}, nil); err == nil {
		t.Error("old handle dispatched tool")
	}
	if err := c.notify(s.ctx, "notifications/cancelled"); err == nil {
		t.Error("old handle dispatched notification")
	}
	if requests.Load() != before {
		t.Fatal("closed handle reached network")
	}
	fresh, err := s.mcpConnect(s.ctx, "bot", srv)
	if err != nil || fresh == c {
		t.Fatal("explicit reconnect failed", err)
	}
	if err := fresh.call(s.ctx, "tools/call", map[string]any{"name": "read_note"}, nil); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != before+3 {
		t.Fatal("fresh connection handshake or call duplicated")
	}
}

func TestMCPConnectionLifetimeStaleConfigurationCannotReconnect(t *testing.T) {
	server, _, requests := mcpLifetimeServer(t, false)
	s, stop := historyService(t, t.TempDir())
	defer stop()
	bot := firstBot(s)
	srv := mcpServer{Name: "fixture", URL: server.URL}
	if err := s.ext.setBot(bot, botExt{MCP: []mcpServer{srv}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.mcpConnect(s.ctx, bot, srv); err != nil {
		t.Fatal(err)
	}
	if w := mcpConfigRequest(s, bot, "ext", map[string]any{"workspaceId": s.workspaceID, "mcpRevision": 0, "mcp": []mcpServer{}}); w.Code != 200 {
		t.Fatal(w.Code)
	}
	before := requests.Load()
	if _, err := s.mcpConnect(s.ctx, bot, srv); err == nil {
		t.Fatal("stale reader recreated removed server")
	}
	if requests.Load() != before {
		t.Fatal("stale configuration reached network")
	}
}

func TestMCPConnectionLifetimeCancelsResponseBodies(t *testing.T) {
	for _, mode := range []string{"json", "stream", "notification"} {
		t.Run(mode, func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var in struct {
					ID     *int64 `json:"id"`
					Method string `json:"method"`
				}
				json.NewDecoder(r.Body).Decode(&in)
				if in.Method == "tools/call" || in.Method == "notifications/cancelled" {
					if mode == "stream" {
						w.Header().Set("Content-Type", "text/event-stream")
					} else {
						w.Header().Set("Content-Type", "application/json")
					}
					w.WriteHeader(200)
					w.(http.Flusher).Flush()
					close(entered)
					select {
					case <-r.Context().Done():
					case <-release:
					}
					return
				}
				if in.ID == nil {
					w.WriteHeader(202)
					return
				}
				json.NewEncoder(w).Encode(map[string]any{"id": *in.ID, "result": map[string]any{"protocolVersion": "2025-06-18"}})
			}))
			defer server.Close()
			defer close(release)
			s := mcpFixtureService(t, "")
			srv := mcpServer{Name: "fixture", URL: server.URL}
			c, err := s.mcpConnect(s.ctx, "bot", srv)
			if err != nil {
				t.Fatal(err)
			}
			reading := make(chan struct{})
			base := c.client.Transport
			if base == nil {
				base = http.DefaultTransport
			}
			c.client.Transport = mcpDiscoveryTransport(func(r *http.Request) (*http.Response, error) {
				res, err := base.RoundTrip(r)
				if err == nil {
					res.Body = &mcpLifetimeReadProbe{ReadCloser: res.Body, entered: reading}
				}
				return res, err
			})
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() {
				if mode == "notification" {
					done <- c.notify(ctx, "notifications/cancelled")
				} else {
					done <- c.call(ctx, "tools/call", map[string]any{"name": "write_note"}, nil)
				}
			}()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("request did not arrive")
			}
			select {
			case <-reading:
			case <-time.After(time.Second):
				t.Fatal("client did not begin response body")
			}
			if err := s.mcpDrop(s.ctx, "bot", srv); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("canceled response body acknowledged: %v", err)
				}
			case <-time.After(500 * time.Millisecond):
				t.Fatal("response body ignored connection cancellation")
			}
		})
	}
}

func TestMCPConnectionLifetimeRequestCancellationDoesNotCloseHandle(t *testing.T) {
	server, _, requests := mcpLifetimeServer(t, false)
	s := mcpFixtureService(t, "")
	srv := mcpServer{Name: "fixture", URL: server.URL}
	c, err := s.mcpConnect(s.ctx, "bot", srv)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(s.ctx)
	cancel()
	if err := c.call(ctx, "tools/call", nil, nil); !errors.Is(err, context.Canceled) {
		t.Fatal("caller cancellation ignored", err)
	}
	if requests.Load() != 2 || c.ended() {
		t.Fatal("caller cancellation changed shared connection")
	}
	if err := c.call(s.ctx, "tools/call", nil, nil); err != nil {
		t.Fatal("next request could not use live connection", err)
	}
	if requests.Load() != 3 {
		t.Fatal("request duplicated")
	}
}
