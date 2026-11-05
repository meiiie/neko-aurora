package aurora

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"nekoaurora.local/native/internal/computer"
)

func TestComputerPreviewIsolationAndOwnership(t *testing.T) {
	t.Setenv("AURORA_COMPUTER", "files")
	s, _ := newSvc(t)
	id := firstBot(s)
	html := "<!doctype html><meta charset=utf-8><h1>Neko Aurora</h1><script>document.title='preview'</script>"
	if err := s.PC.WriteFile(id, "site/index.html", html); err != nil {
		t.Fatal(err)
	}
	if err := s.PC.WriteFile(id, "private.txt", "private"); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, bot, path, scope string
		code                   int
	}{
		{"preview", id, "site/index.html", s.workspaceID, 200},
		{"stale workspace", id, "site/index.html", "other", 409},
		{"scope required", id, "site/index.html", "", 409},
		{"missing", id, "missing.html", s.workspaceID, 404},
		{"another bot", "unknown", "site/index.html", s.workspaceID, 404},
		{"traversal", id, "../site/index.html", s.workspaceID, 404},
		{"not HTML", id, "private.txt", s.workspaceID, 415},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRecorder()
			s.ServeHTTP(r, httptest.NewRequest("GET", "/app/api/agents/"+tt.bot+"/computer/preview?path="+url.QueryEscape(tt.path)+"&workspaceId="+url.QueryEscape(tt.scope), nil))
			if r.Code != tt.code {
				t.Fatalf("HTTP %d want %d: %s", r.Code, tt.code, r.Body.String())
			}
			if tt.code != 200 {
				return
			}
			if r.Body.String() != html {
				t.Fatal("preview changed the file")
			}
			csp := r.Header().Get("Content-Security-Policy")
			for _, required := range []string{"sandbox allow-scripts allow-forms;", "default-src 'none'", "connect-src 'none'", "form-action 'none'", "frame-ancestors 'self'"} {
				if !strings.Contains(csp, required) {
					t.Fatalf("missing restriction %s", required)
				}
			}
			if strings.Contains(csp, "allow-same-origin") || r.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("preview is not isolated")
			}
		})
	}
}

func TestLandingPageWritesAndDeliversWithoutShell(t *testing.T) {
	t.Setenv("AURORA_COMPUTER", "files")
	html := "<!doctype html><html lang=\"vi\"><meta charset=\"utf-8\"><title>Neko Aurora</title><style>body{background:#101018;color:white}</style><h1>Ý tưởng của bạn. Cả đội cùng hành động.</h1></html>"
	write, _ := json.Marshal(toolReq{Name: "computer_write_file", Path: "site/index.html", Content: html})
	fa := scriptedAI(t, "TOOL: "+string(write), "TOOL: {\"name\":\"computer_read_file\",\"path\":\"site/index.html\"}", "TOOL: {\"name\":\"computer_list\",\"path\":\"site\"}", "Đã tạo trang Neko Aurora. [index.html](/app/api/agents/bot-meiiie/computer/download?path=site/index.html). Chưa chạy build/test.")
	defer fa.Close()
	t.Setenv("ZAI_API_KEY", "fixture-not-a-real-key")
	t.Setenv("AURORA_AI_BASE_URL", fa.URL)
	s, _ := newSvc(t)
	id := firstBot(s)
	if id != "bot-meiiie" {
		t.Fatalf("fixture bot changed: %s", id)
	}
	srv := httptest.NewServer(s)
	defer srv.Close()
	post(t, srv.URL+"/app/api/agents/"+id+"/messages", `{"text":"làm cho tôi một trang web landing page cho Neko Aurora"}`)
	waitTask(t, s, id)
	got, _, err := s.PC.ReadFile(id, "site/index.html")
	if err != nil || got != html {
		t.Fatalf("real file not delivered: %v %q", err, got)
	}
	if !strings.Contains(lastReply(s, id), "computer/download?path=site/index.html") {
		t.Fatalf("missing verified download: %s", lastReply(s, id))
	}
	st, _ := s.PC.Status(id)
	if len(st.Log) != 0 || st.Available || !st.CanWrite {
		t.Fatalf("fabricated execution: %+v", st)
	}
	if !wantsComputer.MatchString("làm landing page") || !wantsComputer.MatchString("tạo trang web") {
		t.Fatal("website creation was treated as lookup")
	}
	// Explicit owner read-only policy still prevents mutation.
	if err := s.PC.SetPolicy(id, computer.Policy{Preset: "readonly"}); err != nil {
		t.Fatal(err)
	}
	_, err = s.runComputer(context.Background(), id, &toolCtx{}, toolCall{name: "computer_write_file", arg: `{"path":"blocked.html","content":"blocked"}`})
	if err == nil {
		t.Fatal("read-only policy bypassed")
	}
}
