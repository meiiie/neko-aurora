package aurora

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"nekoaurora.local/native/internal/ledger"
)

func TestParseExtDirectives(t *testing.T) {
	d := ParseDirectives("Bạn điền giúp nhé.\nFORM: {\"title\":\"Đặt phòng\",\"fields\":[{\"name\":\"d\",\"label\":\"Ngày\",\"type\":\"date\"},{\"name\":\"k\",\"label\":\"Loại\",\"type\":\"select\",\"options\":[\"Đơn\",\"Đôi\"]},{\"label\":\"x\",\"type\":\"evil\"}]}\nTOOL: {\"name\":\"mcp\",\"server\":\"wiki\",\"tool\":\"ask\",\"args\":{\"q\":1}}\nTOOL: {\"name\":\"bot_create\",\"bot\":\"Kế toán\",\"desc\":\"sổ sách\"}")
	if d.Form == nil || len(d.Form.Fields) != 3 || d.Form.Fields[2].Type != "text" || d.Form.Fields[1].Options[1] != "Đôi" || d.Text != "Bạn điền giúp nhé." {
		t.Fatalf("%+v %q", d.Form, d.Text)
	}
	if len(d.Tools) != 2 || string(d.Tools[0].Args) != `{"q":1}` {
		t.Fatalf("%+v", d.Tools)
	}
	c := callsOf(d.Tools)
	var back toolReq
	if json.Unmarshal([]byte(c[1].arg), &back) != nil || back.Bot != "Kế toán" {
		t.Fatal(c[1].arg)
	}
	if validTool(toolReq{Name: "bot_update", Target: "A"}) || !validTool(toolReq{Name: "memory_recall"}) {
		t.Fatal("validTool")
	}
}

func TestFolderCall(t *testing.T) {
	c := folderCall("dự án/q4", toolCall{"computer_run", "python3 a.py"})
	if c.arg != "mkdir -p ~/'dự án/q4' && cd ~/'dự án/q4' && python3 a.py" {
		t.Fatal(c.arg)
	}
	c = folderCall("p", toolCall{"computer_write_file", `{"path":"a.py","content":"x"}`})
	if !strings.Contains(c.arg, `"path":"p/a.py"`) {
		t.Fatal(c.arg)
	}
	if folderCall("p", toolCall{"computer_read_file", "/home/bot/z.txt"}).arg != "/home/bot/z.txt" || folderCall("p", toolCall{"computer_list", ""}).arg != "p" {
		t.Fatal("paths")
	}
	if validFolder("../etc") || !validFolder("Báo cáo/2026") {
		t.Fatal("validFolder")
	}
}

func TestBotChangeParse(t *testing.T) {
	a, id, n, d := parseBotChange("Hành động: sửa\nBot: bot-x\nTên: Kế toán\nVai trò: ghi sổ")
	if a != "sửa" || id != "bot-x" || n != "Kế toán" || d != "ghi sổ" {
		t.Fatal(a, id, n, d)
	}
}

// fake OpenAI-compatible provider that streams reasoning, then content.
func fakeProvider(t *testing.T, key *atomic.Value) *httptest.Server {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key.Store(r.Header.Get("Authorization"))
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"cân nhắc model=%v\"}}]}\n\n", in["model"])
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Xin chào từ nhà cung cấp riêng.\"}}]}\n\ndata: [DONE]\n\n")
	}))
	t.Cleanup(s.Close)
	return s
}

func TestProviderModelThinkingAndForm(t *testing.T) {
	var key atomic.Value
	prov := fakeProvider(t, &key)
	svc, u, bot := liveSvc(t, "không dùng")
	r := post(t, u+"/app/api/providers", `{"id":"my-ai","name":"My AI","base":"`+prov.URL+`","models":["m-1"],"key":"sk-test-123456"}`)
	b, _ := io.ReadAll(r.Body)
	if r.StatusCode != 200 || strings.Contains(string(b), "sk-test") || !strings.Contains(string(b), `"hasKey":true`) {
		t.Fatalf("%d %s", r.StatusCode, b)
	}
	if r := post(t, u+"/app/api/agents/"+bot+"/ext", `{"model":"nope:x"}`); r.StatusCode != 400 {
		t.Fatal("bad model accepted")
	}
	if r := post(t, u+"/app/api/agents/"+bot+"/ext", `{"model":"my-ai:m-1","thinking":true}`); r.StatusCode != 200 {
		t.Fatal(r.StatusCode)
	}
	post(t, u+"/app/api/agents/"+bot+"/messages", `{"text":"chào","clientNonce":"pv-1"}`)
	waitTask(t, svc, bot)
	if key.Load() != "Bearer sk-test-123456" {
		t.Fatal("provider key not used")
	}
	var ms []*Message
	getJSON(t, u+"/app/api/agents/"+bot+"/messages", &ms)
	last := ms[len(ms)-1]
	if !strings.Contains(last.Text, "nhà cung cấp riêng") || !strings.Contains(last.Thinking, "model=m-1") {
		t.Fatalf("%+v", last)
	}
	var st map[string]any
	getJSON(t, u+"/app/api/models", &st)
	if !strings.Contains(mustJSON(st), "my-ai:m-1") || strings.Contains(mustJSON(st), "sk-test") {
		t.Fatal(st)
	}
	// form side data
	svc.ext.setForm(last.TaskID, parseForm(`{"title":"T","fields":[{"label":"Tên"}]}`))
	getJSON(t, u+"/app/api/agents/"+bot+"/messages", &ms)
	if f := ms[len(ms)-1].Form; f == nil || f.Fields[0].Label != "Tên" {
		t.Fatal("form")
	}
	post(t, u+"/app/api/forms/"+last.TaskID+"/done", `{}`)
	if _, f := svc.ext.side(last.TaskID); !f.Done {
		t.Fatal("done")
	}
}

// minimal Streamable-HTTP MCP server with two tools.
func fakeMCP(t *testing.T, calls *atomic.Int32) *httptest.Server {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID     *int64          `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in.ID == nil {
			w.WriteHeader(202)
			return
		}
		var res any
		switch in.Method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "sess-1")
			res = map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}}
		case "tools/list":
			if r.Header.Get("Mcp-Session-Id") != "sess-1" {
				t.Error("no session id")
			}
			res = map[string]any{"tools": []map[string]any{
				{"name": "lookup", "description": "tra cứu", "inputSchema": map[string]any{"type": "object"}},
				{"name": "post_note", "description": "đăng ghi chú", "inputSchema": map[string]any{"type": "object"}},
			}}
		case "tools/call":
			calls.Add(1)
			var p struct{ Name string }
			_ = json.Unmarshal(in.Params, &p)
			res = map[string]any{"content": []map[string]any{{"type": "text", "text": "kết quả " + p.Name}}}
		}
		b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": *in.ID, "result": res})
		// answer as SSE to exercise that path
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "event: message\ndata: %s\n\n", b)
	}))
	t.Cleanup(s.Close)
	return s
}

func TestRecallSkillsCapsAndMemorySummary(t *testing.T) {
	svc, u, bot := liveSvc(t, `["Darren thích bảng ngắn","Gọi là anh Darren"]`)
	svc.Mem.Remember(bot, "Darren thích bảng")
	svc.Mem.Remember(bot, "Darren thích bảng ngắn gọn")
	svc.Mem.Remember(bot, "Gọi là anh Darren")
	r := svc.recall(ledger.Work{BotID: bot, ConversationID: "conversation-" + bot}, "bảng")
	if !strings.Contains(r.Text, "thích bảng ngắn gọn") || strings.Contains(r.Text, "anh Darren") {
		t.Fatal(r.Text)
	}
	var sum struct{ Proposal []string }
	resp := post(t, u+"/app/api/agents/"+bot+"/memory/summarize", `{}`)
	_ = json.NewDecoder(resp.Body).Decode(&sum)
	if len(sum.Proposal) != 2 || len(svc.Mem.List(bot)) != 3 {
		t.Fatalf("%+v", sum)
	}
	post(t, u+"/app/api/agents/"+bot+"/memory/summarize", `{"apply":`+mustJSON(sum.Proposal)+`}`)
	if l := svc.Mem.List(bot); len(l) != 2 || svc.Mem.Journal(bot, 1)[0].Op != "summarize" {
		t.Fatalf("%+v", l)
	}
	post(t, u+"/app/api/skills", `{"name":"Viết thơ","desc":"thơ lục bát","body":"Viết 4 câu lục bát."}`)
	note := svc.extNote(bot)
	if !strings.Contains(note, "Viết thơ: thơ lục bát") || !strings.Contains(note, "Báo cáo từ CSV") {
		t.Fatal(note)
	}
	post(t, u+"/app/api/agents/"+bot+"/ext", `{"off":{"web":true,"skills":true}}`)
	if note := svc.extNote(bot); strings.Contains(note, "KỸ NĂNG") || !strings.Contains(note, "tìm/đọc web") {
		t.Fatal(note)
	}
	tc := &toolCtx{}
	svc.runTools(context.Background(), ledger.Work{BotID: bot, RequestID: "x"}, tc, []toolCall{{"web_search", "x"}})
	if strings.Contains(strings.Join(tc.blocks, ""), "KẾT QUẢ") && !strings.Contains(strings.Join(tc.blocks, ""), "tắt") {
		t.Fatal(tc.blocks)
	}
}

func TestImageFallbackAndBrowserGuard(t *testing.T) {
	var gotImage atomic.Bool
	vis := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotImage.Store(strings.Contains(string(b), `"image_url":{"url":"data:image/png;base64,`))
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Ảnh có chữ DOANH THU 120\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer vis.Close()
	svc, u, bot := liveSvc(t, "x")
	post(t, u+"/app/api/providers", `{"id":"vis","name":"Vision","base":"`+vis.URL+`","models":["v-1"]}`)
	if r := post(t, u+"/app/api/vision", `{"model":"vis:v-1"}`); r.StatusCode != 200 {
		t.Fatal(r.StatusCode)
	}
	if err := svc.PC.WriteFile(bot, "uploads/a.png", "\x89PNG\r\n\x1a\nfake"); err != nil {
		t.Fatal(err)
	}
	res, err := svc.describeImage(context.Background(), bot, "uploads/a.png", "")
	if err != nil || !gotImage.Load() || !strings.Contains(res.Text, "DOANH THU 120") {
		t.Fatal(err, res.Text)
	}
	if _, err := svc.browserOpen(context.Background(), "http://127.0.0.1:8765/"); err == nil {
		t.Fatal("browser opened a loopback URL")
	}
}

func TestAutoImageAndBrowser(t *testing.T) {
	if c := imageAuto("xem giúp [Ảnh: uploads/doanh-thu.jpg]"); len(c) != 1 || !strings.Contains(c[0].arg, `"path":"uploads/doanh-thu.jpg"`) {
		t.Fatal(c)
	}
	if c := browserAuto("Mở bằng trình duyệt trang https://example.com rồi cho biết tiêu đề"); len(c) != 1 || !strings.Contains(c[0].arg, "https://example.com") {
		t.Fatal(c)
	}
	if browserAuto("đọc https://example.com") != nil {
		t.Fatal("no browser intent")
	}
}

// DS Bot defaults: bot changes and MCP calls run directly; only the main bot
// creates bots; a bot may update itself (model, thinking, folder).
func TestDefaultsDirectLikeDSBot(t *testing.T) {
	var calls atomic.Int32
	m := fakeMCP(t, &calls)
	svc, u, bot := liveSvc(t,
		tool(map[string]any{"name": "mcp", "server": "kb", "tool": "post_note", "args": map[string]any{}}), "Đã đăng.",
		tool(map[string]any{"name": "bot_create", "bot": "Kế toán", "desc": "ghi sổ", "folder": "so-sach"}), "Đã tạo.",
		tool(map[string]any{"name": "bot_update", "thinking": true}), "Đã bật.")
	post(t, u+"/app/api/agents/"+bot+"/ext", `{"mcp":[{"name":"kb","url":"`+m.URL+`"}]}`)
	post(t, u+"/app/api/agents/"+bot+"/messages", `{"text":"đăng ghi chú","clientNonce":"d-1"}`)
	waitTask(t, svc, bot)
	if calls.Load() != 1 {
		t.Fatal("mcp not called directly")
	}
	post(t, u+"/app/api/agents/"+bot+"/messages", `{"text":"tạo bot kế toán","clientNonce":"d-2"}`)
	waitTask(t, svc, bot)
	var nb string
	for _, b := range svc.L.Snapshot().Bots {
		if b.Name == "Kế toán" {
			nb = b.ID
		}
	}
	if nb == "" || svc.ext.bot(nb).Folder != "so-sach" {
		t.Fatal("bot not created directly", nb)
	}
	post(t, u+"/app/api/agents/"+bot+"/messages", `{"text":"bật suy nghĩ cho bạn","clientNonce":"d-3"}`)
	waitTask(t, svc, bot)
	if !svc.ext.bot(bot).Thinking {
		t.Fatal("self update")
	}
	// a non-main bot cannot create bots
	tc := &toolCtx{}
	b, _ := json.Marshal(toolReq{Name: "bot_create", Bot: "X"})
	if _, err := svc.runExt(context.Background(), ledger.Work{BotID: nb}, tc, toolCall{"bot_create", string(b)}); err == nil {
		t.Fatal("non-main bot created a bot")
	}
}
