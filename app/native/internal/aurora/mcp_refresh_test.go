package aurora

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMCPQueuedHandshakeHonorsCancellation(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() { close(entered) })
		<-release
		w.WriteHeader(503)
	}))
	defer server.Close()
	defer close(release)
	s := mcpFixtureService(t, "")
	srv := mcpServer{Name: "blocked", URL: server.URL}
	first := make(chan error, 1)
	go func() { _, err := s.mcpConnect(s.ctx, "bot", srv); first <- err }()
	<-entered
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Millisecond)
	defer cancel()
	second := make(chan error, 1)
	go func() { _, err := s.mcpConnect(ctx, "bot", srv); second <- err }()
	select {
	case err := <-second:
		if err == nil {
			t.Fatal("cancelled handshake succeeded")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("queued handshake ignored its own deadline")
	}
	ctxDrop, cancelDrop := context.WithTimeout(s.ctx, 30*time.Millisecond)
	defer cancelDrop()
	if err := s.mcpDrop(ctxDrop, "bot", srv); err == nil {
		t.Fatal("cancelled reconnect acquired a held gate")
	}
}

func TestMCPSSEEarlyEOFAndChangedCatalog(t *testing.T) {
	t.Run("endpoint EOF", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.(http.Flusher).Flush()
		}))
		defer server.Close()
		s := mcpFixtureService(t, "")
		ctx, cancel := context.WithTimeout(s.ctx, time.Second)
		defer cancel()
		start := time.Now()
		_, err := s.mcpConnect(ctx, "bot", mcpServer{Name: "eof", URL: server.URL + "/sse"})
		if err == nil || time.Since(start) > 700*time.Millisecond {
			t.Fatal("closed stream waited for the endpoint deadline", err)
		}
	})
	t.Run("refresh and unknown tool outcome", func(t *testing.T) {
		stream := make(chan string, 16)
		stop := make(chan struct{})
		var stopOnce sync.Once
		var version, calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/sse" {
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "event: endpoint\ndata: /messages\n\n")
				w.(http.Flusher).Flush()
				for {
					select {
					case data := <-stream:
						fmt.Fprint(w, "data: "+data+"\n\n")
						w.(http.Flusher).Flush()
					case <-stop:
						return
					case <-r.Context().Done():
						return
					}
				}
			}
			var in struct {
				ID     *int64 `json:"id"`
				Method string `json:"method"`
			}
			json.NewDecoder(r.Body).Decode(&in)
			if in.ID != nil {
				if in.Method == "tools/call" {
					calls.Add(1)
					stopOnce.Do(func() { close(stop) })
					w.WriteHeader(202)
					return
				}
				var result any = map[string]any{"protocolVersion": "2025-06-18"}
				if in.Method == "tools/list" {
					result = map[string]any{"tools": []mcpTool{{Name: fmt.Sprint("tool-", version.Load()), Schema: json.RawMessage(`{}`)}}}
				}
				data, _ := json.Marshal(map[string]any{"id": *in.ID, "result": result})
				stream <- string(data)
			}
			w.WriteHeader(202)
		}))
		defer server.Close()
		defer stopOnce.Do(func() { close(stop) })
		s := mcpFixtureService(t, "")
		srv := mcpServer{Name: "refresh", URL: server.URL + "/sse"}
		ctx, cancel := context.WithTimeout(s.ctx, 3*time.Second)
		defer cancel()
		list, err := s.mcpTools(ctx, "bot", srv)
		if err != nil || len(list) != 1 || list[0].Name != "tool-0" {
			t.Fatal("initial catalog", err)
		}
		c, err := s.mcpConnect(ctx, "bot", srv)
		if err != nil {
			t.Fatal(err)
		}
		version.Store(1)
		stream <- `{"jsonrpc":"2.0","method":"notifications/tools/list_changed"}`
		deadline := time.Now().Add(time.Second)
		for c.toolsRevision.Load() == 0 && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		if c.toolsRevision.Load() == 0 {
			t.Fatal("list notification lost")
		}
		list, err = s.mcpTools(ctx, "bot", srv)
		if err != nil || len(list) != 1 || list[0].Name != "tool-1" {
			t.Fatal("stale tools after notification", err)
		}
		start := time.Now()
		if _, err = s.mcpCall(ctx, "bot", srv, "write", nil); err == nil || time.Since(start) > time.Second {
			t.Fatal("closed stream acknowledged or hung", err)
		}
		if calls.Load() != 1 {
			t.Fatal("unknown tool outcome replayed")
		}
	})
}

func TestMCPRejectsIncompleteOrMalformedCatalog(t *testing.T) {
	for _, mode := range []string{"cursor-cycle", "duplicate", "schema", "page-limit"} {
		t.Run(mode, func(t *testing.T) {
			var lists atomic.Int32
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
				if in.Method == "tools/list" {
					n := lists.Add(1)
					switch mode {
					case "cursor-cycle":
						result = map[string]any{"tools": []mcpTool{}, "nextCursor": "same"}
					case "duplicate":
						result = map[string]any{"tools": []mcpTool{{Name: "same", Schema: json.RawMessage(`{}`)}}, "nextCursor": fmt.Sprint(n)}
					case "schema":
						result = map[string]any{"tools": []mcpTool{{Name: "bad", Schema: json.RawMessage(`[]`)}}}
					case "page-limit":
						result = map[string]any{"tools": []mcpTool{}, "nextCursor": fmt.Sprint(n)}
					}
				}
				json.NewEncoder(w).Encode(map[string]any{"id": *in.ID, "result": result})
			}))
			defer server.Close()
			s := mcpFixtureService(t, "")
			srv := mcpServer{Name: "bad", URL: server.URL}
			if _, err := s.mcpTools(s.ctx, "bot", srv); err == nil {
				t.Fatal("invalid catalog accepted")
			}
			if lists.Load() > 16 {
				t.Fatal("unbounded pagination")
			}
			cache := s.mcpState()
			cache.mu.Lock()
			_, ok := cache.tools[mcpKey("bot", srv)]
			cache.mu.Unlock()
			if ok {
				t.Fatal("partial catalog cached")
			}
		})
	}
}

func TestMCPIncompleteSSEHonorsCancellationAfterHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	s := mcpFixtureService(t, "")
	ctx, cancel := context.WithTimeout(s.ctx, 50*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := s.mcpConnect(ctx, "bot", mcpServer{Name: "slow", URL: server.URL + "/sse"})
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("missing endpoint accepted")
		}
	case <-time.After(600 * time.Millisecond):
		t.Fatal("SSE endpoint wait ignored caller cancellation")
	}
}

func TestMCPMalformedPersistedURLDoesNotPanic(t *testing.T) {
	defer func() {
		if recover() != nil {
			t.Error("invalid persisted URL panicked")
		}
	}()
	c := &mcpConn{srv: mcpServer{URL: "%zz/sse"}, client: http.DefaultClient}
	defer c.close()
	if err := c.openSSE(context.Background(), context.Background()); err == nil {
		t.Fatal("invalid URL accepted")
	}
}

func TestMCPCancelledHandshakeDoesNotPoisonOtherCallers(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID     *int64 `json:"id"`
			Method string `json:"method"`
		}
		json.NewDecoder(r.Body).Decode(&in)
		if in.Method == "initialize" && attempts.Add(1) == 1 {
			<-r.Context().Done()
			return
		}
		if in.ID == nil {
			w.WriteHeader(202)
			return
		}
		result := map[string]any{"protocolVersion": "2025-06-18", "tools": []mcpTool{}}
		json.NewEncoder(w).Encode(map[string]any{"id": *in.ID, "result": result})
	}))
	defer server.Close()
	s := mcpFixtureService(t, "")
	srv := mcpServer{Name: "cancellation", URL: server.URL}
	ctx, cancel := context.WithTimeout(s.ctx, 40*time.Millisecond)
	defer cancel()
	if _, err := s.mcpTools(ctx, "bot", srv); err == nil {
		t.Fatal("cancelled handshake completed")
	}
	if _, err := s.mcpTools(s.ctx, "bot", srv); err != nil {
		t.Fatal("caller cancellation poisoned the shared catalog", err)
	}
	if attempts.Load() != 2 {
		t.Fatal("fresh caller did not initialize")
	}
}

func TestMCPToolsPaginationCacheIsolationAndSingleFlight(t *testing.T) {
	var lists atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID     *int64 `json:"id"`
			Method string `json:"method"`
			Params struct {
				Cursor string `json:"cursor"`
			} `json:"params"`
		}
		json.NewDecoder(r.Body).Decode(&in)
		if in.ID == nil {
			w.WriteHeader(202)
			return
		}
		var result any = map[string]any{"protocolVersion": "2025-06-18"}
		if in.Method == "tools/list" {
			lists.Add(1)
			time.Sleep(20 * time.Millisecond)
			if in.Params.Cursor == "" {
				result = map[string]any{"tools": []mcpTool{{Name: "first", Schema: json.RawMessage(`{"type":"object"}`)}}, "nextCursor": "page-two"}
			} else {
				result = map[string]any{"tools": []mcpTool{{Name: "second", Schema: json.RawMessage(`{"type":"object"}`)}}}
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": *in.ID, "result": result})
	}))
	defer server.Close()
	s := mcpFixtureService(t, "")
	srv := mcpServer{Name: "pages", URL: server.URL}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			list, err := s.mcpTools(s.ctx, "bot", srv)
			if err != nil || len(list) != 2 {
				t.Error("incomplete tool pages", err)
				return
			}
			list[0].Name = "changed"
			list[0].Schema[0] = 'x'
		}()
	}
	wg.Wait()
	list, err := s.mcpTools(s.ctx, "bot", srv)
	if err != nil || len(list) != 2 || list[0].Name != "first" || !json.Valid(list[0].Schema) {
		t.Fatal("cached catalog mutated", err)
	}
	if lists.Load() != 2 {
		t.Fatal("concurrent readers fetched duplicate pages", lists.Load())
	}
}
