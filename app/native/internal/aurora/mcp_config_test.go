package aurora

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

func TestMCPConfigFailedSaveKeepsLiveAndReopenedState(t *testing.T) {
	dir := t.TempDir()
	e := openExt(dir)
	before := botExt{MCP: []mcpServer{{Name: "fixture", URL: "https://first.example.com/mcp"}}, Off: map[string]bool{"web": true}}
	if err := e.setBot("fixture-bot", before); err != nil {
		t.Fatal(err)
	}
	stored, err := os.ReadFile(e.path)
	if err != nil {
		t.Fatal(err)
	}
	path := e.path
	e.path = t.TempDir()
	if err := e.setBot("fixture-bot", botExt{MCP: []mcpServer{{Name: "fixture", URL: "https://changed.example.com/mcp"}}}); err == nil {
		t.Fatal("save failure not returned")
	}
	e.path = path
	if !reflect.DeepEqual(e.bot("fixture-bot"), before) {
		t.Fatal("failed save changed live configuration")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(stored) {
		t.Fatal("failed save changed stored configuration")
	}
	if !reflect.DeepEqual(openExt(dir).bot("fixture-bot"), before) {
		t.Fatal("reopened configuration differs")
	}
}

func TestMCPConfigReadDoesNotAliasCapabilityState(t *testing.T) {
	e := openExt(t.TempDir())
	e.setBot("fixture-bot", botExt{Off: map[string]bool{"web": true}})
	copy := e.bot("fixture-bot")
	copy.Off["web"] = false
	if !e.bot("fixture-bot").Off["web"] {
		t.Fatal("read configuration aliases live capability map")
	}
}

func TestMCPConfigWriteDoesNotAliasCaller(t *testing.T) {
	e := openExt(t.TempDir())
	b := botExt{Off: map[string]bool{"web": true}, Skills: []string{"first"}, MCP: []mcpServer{{Name: "original", URL: "https://fixture.example.com/mcp"}}}
	if err := e.setBot("fixture-bot", b); err != nil {
		t.Fatal(err)
	}
	b.Off["web"] = false
	b.Skills[0] = "changed"
	b.MCP[0].Name = "changed"
	live := e.bot("fixture-bot")
	if !live.Off["web"] || live.Skills[0] != "first" || live.MCP[0].Name != "original" {
		t.Fatal("write retained mutable caller data")
	}
}

func mcpConfigRequest(s *Service, bot, action string, body map[string]any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	r := httptest.NewRequest("POST", "/app/api/agents/"+bot+"/"+action, strings.NewReader(string(b)))
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func TestMCPConfigScopeRevisionAndReopen(t *testing.T) {
	s, closeService := historyService(t, t.TempDir())
	defer closeService()
	bot := firstBot(s)
	servers := []mcpServer{{Name: "fixture", URL: "https://fixture.example.com/mcp"}}
	for _, body := range []map[string]any{
		{"mcp": servers},
		{"workspaceId": "wrong", "mcp": servers},
		{"workspaceId": s.workspaceID, "mcpRevision": 1, "mcp": servers},
	} {
		if w := mcpConfigRequest(s, bot, "ext", body); w.Code != 409 {
			t.Fatalf("conflict status %d", w.Code)
		}
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", "/app/api/agents/"+bot+"/ext", nil))
	var got struct {
		WorkspaceID string `json:"workspaceId"`
		MCPRevision uint64 `json:"mcpRevision"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || got.WorkspaceID != s.workspaceID || got.MCPRevision != 0 {
		t.Fatal("GET missing identity")
	}
	for revision, next := range [][]mcpServer{servers, {}, servers} {
		body := map[string]any{"workspaceId": s.workspaceID, "mcpRevision": revision, "mcp": next}
		if w := mcpConfigRequest(s, bot, "ext", body); w.Code != 200 {
			t.Fatalf("save status %d", w.Code)
		}
		if w := mcpConfigRequest(s, bot, "ext", body); w.Code != 409 {
			t.Fatalf("old revision replay status %d", w.Code)
		}
	}
	if w := mcpConfigRequest(s, bot, "ext", map[string]any{"workspaceId": s.workspaceID, "thinking": true}); w.Code != 200 {
		t.Fatal(w.Code)
	}
	be := s.ext.bot(bot)
	if be.MCPRevision != 3 || !be.Thinking || !reflect.DeepEqual(be.MCP, servers) {
		t.Fatal("remove/readd or unrelated settings lost revision")
	}
	if reopened := openExt(filepath.Dir(s.ext.path)).bot(bot); !reflect.DeepEqual(reopened, be) {
		t.Fatal("reopened revision differs from live configuration")
	}
}

func TestMCPConfigRejectedUpdateKeepsConnection(t *testing.T) {
	s, closeService := historyService(t, t.TempDir())
	defer closeService()
	bot := firstBot(s)
	srv := mcpServer{Name: "fixture", URL: "https://fixture.example.com/mcp"}
	before := botExt{MCPRevision: 4, MCP: []mcpServer{srv}}
	if err := s.ext.setBot(bot, before); err != nil {
		t.Fatal(err)
	}
	cache, key := s.mcpState(), mcpKey(bot, srv)
	var closed atomic.Int32
	conn := &mcpConn{cancel: func() { closed.Add(1) }}
	cache.conns[key] = conn
	for _, tc := range []struct {
		name   string
		body   map[string]any
		status int
	}{
		{"duplicate", map[string]any{"workspaceId": s.workspaceID, "mcpRevision": 4, "mcp": []mcpServer{srv, {Name: " FIXTURE ", URL: srv.URL}}}, 400},
		{"invalid", map[string]any{"workspaceId": s.workspaceID, "mcpRevision": 4, "mcp": []mcpServer{{Name: "bad name", URL: srv.URL}}}, 400},
		{"stale", map[string]any{"workspaceId": s.workspaceID, "mcpRevision": 3, "mcp": []mcpServer{}}, 409},
		{"wrong workspace", map[string]any{"workspaceId": "other", "mcpRevision": 4, "mcp": []mcpServer{}}, 409},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if w := mcpConfigRequest(s, bot, "ext", tc.body); w.Code != tc.status {
				t.Fatalf("status %d", w.Code)
			}
			if closed.Load() != 0 || cache.conns[key] != conn || !reflect.DeepEqual(s.ext.bot(bot), before) {
				t.Fatal("rejected edit changed live state")
			}
		})
	}
	path := s.ext.path
	s.ext.path = t.TempDir()
	body := map[string]any{"workspaceId": s.workspaceID, "mcpRevision": 4, "mcp": []mcpServer{}}
	w := mcpConfigRequest(s, bot, "ext", body)
	s.ext.path = path
	if w.Code != 500 || !strings.Contains(w.Body.String(), "settings_save_failed") || strings.Contains(w.Body.String(), path) {
		t.Fatal("failed save not reported safely")
	}
	if closed.Load() != 0 || cache.conns[key] != conn || !reflect.DeepEqual(s.ext.bot(bot), before) {
		t.Fatal("save failure changed live state")
	}
	if w := mcpConfigRequest(s, bot, "ext", body); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if closed.Load() != 1 || cache.conns[key] != nil || s.ext.bot(bot).MCPRevision != 5 {
		t.Fatal("committed edit did not invalidate old connection")
	}
}

func TestMCPConfigConcurrentChangesAndReconnect(t *testing.T) {
	s, closeService := historyService(t, t.TempDir())
	defer closeService()
	bot := firstBot(s)
	srv := mcpServer{Name: "fixture", URL: "https://fixture.example.com/mcp"}
	start, codes := make(chan struct{}), make(chan int, 2)
	for range 2 {
		go func() {
			<-start
			codes <- mcpConfigRequest(s, bot, "ext", map[string]any{"workspaceId": s.workspaceID, "mcpRevision": 0, "mcp": []mcpServer{srv}}).Code
		}()
	}
	close(start)
	counts := map[int]int{}
	counts[<-codes]++
	counts[<-codes]++
	if counts[200] != 1 || counts[409] != 1 {
		t.Fatalf("conflicting saves: %v", counts)
	}
	start = make(chan struct{})
	go func() {
		<-start
		codes <- mcpConfigRequest(s, bot, "ext", map[string]any{"workspaceId": s.workspaceID, "mcpRevision": 1, "mcp": []mcpServer{srv}}).Code
	}()
	go func() {
		<-start
		codes <- mcpConfigRequest(s, bot, "ext", map[string]any{"workspaceId": s.workspaceID, "thinking": true}).Code
	}()
	close(start)
	if a, b := <-codes, <-codes; a != 200 || b != 200 {
		t.Fatalf("independent changes: %d %d", a, b)
	}
	if be := s.ext.bot(bot); be.MCPRevision != 2 || !be.Thinking || !reflect.DeepEqual(be.MCP, []mcpServer{srv}) {
		t.Fatal("concurrent settings lost update")
	}
	for _, body := range []map[string]any{{"name": "fixture"}, {"name": "fixture", "workspaceId": "other", "mcpRevision": 2}, {"name": "fixture", "workspaceId": s.workspaceID, "mcpRevision": 1}} {
		if w := mcpConfigRequest(s, bot, "mcp/reconnect", body); w.Code != 409 {
			t.Fatalf("unscoped reconnect: %d", w.Code)
		}
	}
	if w := mcpConfigRequest(s, bot, "mcp/reconnect", map[string]any{"name": "fixture", "workspaceId": s.workspaceID, "mcpRevision": 2}); w.Code != 200 {
		t.Fatalf("scoped reconnect: %d", w.Code)
	}
}
