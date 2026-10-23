package aurora

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nekoaurora.local/native/internal/computer"
)

func TestComputerRunNeedsApproval(t *testing.T) {
	if computer.NewBwrap().Available() != nil {
		t.Skip("no bwrap")
	}
	fa := scriptedAI(t,
		"TOOL: {\"name\":\"computer_write_file\",\"path\":\"a.py\",\"content\":\"import os\\nprint(6*7, sorted(k for k in os.environ if 'KEY' in k))\"}",
		"TOOL: {\"name\":\"computer_run\",\"cmd\":\"python3 a.py\"}",
		"Kết quả là 42.")
	defer fa.Close()
	t.Setenv("ZAI_API_KEY", "k-should-not-leak")
	t.Setenv("AURORA_AI_BASE_URL", fa.URL)
	svc, _ := newSvc(t)
	if svc.PC == nil {
		t.Fatal("computers off")
	}
	if _, err := svc.PC.Run(context.Background(), "probe", "true"); err != nil {
		t.Skipf("sandbox unusable here: %v", err)
	}
	srv := httptest.NewServer(svc)
	defer srv.Close()
	bot := firstBot(svc)
	askFirst(svc, bot) // card flow
	post(t, srv.URL+"/app/api/agents/"+bot+"/messages", `{"text":"viết script python tính 6*7 rồi chạy"}`)
	waitTask(t, svc, bot)
	st, _ := svc.PC.Status(bot)
	if len(st.Files) != 1 || len(st.Log) != 0 {
		t.Fatalf("file written / nothing run yet: %+v", st)
	}
	p := svc.view().pending()
	if len(p) != 1 || p[0].Approval.Kind != "computer" || p[0].Approval.Body != "python3 a.py" {
		t.Fatalf("pending %+v", p)
	}
	post(t, srv.URL+"/app/api/approvals/"+p[0].ID, `{"decision":"approved"}`)
	var out string
	for i := 0; i < 300; i++ {
		if w, _ := svc.L.Get(p[0].TaskID); w.Direct.Approval.Delivery != nil && w.Direct.Approval.Delivery.Status == "sent" {
			out = w.Direct.Approval.Outcome
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.Contains(out, "42 []") {
		t.Fatalf("outcome %q", out)
	}
	if st, _ := svc.PC.Status(bot); len(st.Log) != 1 || st.Log[0].ExitCode != 0 {
		t.Fatalf("%+v", st.Log)
	}
	// the follow-up turn continues from the output; its prompt is hidden
	for i := 0; i < 500 && !strings.Contains(lastReply(svc, bot), "Kết quả là 42"); i++ {
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.Contains(lastReply(svc, bot), "Kết quả là 42") {
		t.Fatal("no follow-up reply")
	}
	for _, m := range svc.view().messages(bot, 100) {
		if m.Role == "agent" && strings.Contains(m.Text, "mã thoát") {
			t.Fatal("command output duplicated as a chat message")
		}
		if m.Role == "user" && strings.HasPrefix(m.Text, "[Máy tính]") {
			t.Fatal("follow-up prompt shown as a user message")
		}
	}
	// API
	r, _ := http.Get(srv.URL + "/app/api/agents/" + bot + "/computer")
	var s2 computer.Status
	json.NewDecoder(r.Body).Decode(&s2)
	if s2.Backend != "bubblewrap" || len(s2.Log) != 1 {
		t.Fatalf("%+v", s2)
	}
	r, _ = http.Get(srv.URL + "/app/api/agents/" + bot + "/computer/file?path=a.py")
	b, _ := io.ReadAll(r.Body)
	if !strings.Contains(string(b), "print(6*7") {
		t.Fatal(string(b))
	}
	if r := post(t, srv.URL+"/app/api/agents/"+bot+"/computer/reset", `{}`); r.StatusCode != 200 {
		t.Fatal(r.StatusCode)
	}
	if st, _ := svc.PC.Status(bot); len(st.Files)+len(st.Log) != 0 {
		t.Fatal("reset kept data")
	}
	if r, _ := http.Get(srv.URL + "/app/api/agents/nope/computer"); r.StatusCode != 404 {
		t.Fatal(r.StatusCode)
	}
}

func TestComputerAutoReadOnly(t *testing.T) {
	if computer.NewBwrap().Available() != nil {
		t.Skip("no bwrap")
	}
	fa := scriptedAI(t, "TOOL: {\"name\":\"computer_run\",\"cmd\":\"ls -la\"}", "Thư mục trống.")
	defer fa.Close()
	t.Setenv("ZAI_API_KEY", "k")
	t.Setenv("AURORA_AI_BASE_URL", fa.URL)
	svc, _ := newSvc(t)
	if _, err := svc.PC.Run(context.Background(), "probe", "true"); err != nil {
		t.Skipf("sandbox unusable here: %v", err)
	}
	srv := httptest.NewServer(svc)
	defer srv.Close()
	bot := firstBot(svc)
	if r := post(t, srv.URL+"/app/api/agents/"+bot+"/computer/policy", `{"autoReadOnly":true}`); r.StatusCode != 200 {
		t.Fatal(r.StatusCode)
	}
	post(t, srv.URL+"/app/api/agents/"+bot+"/messages", `{"text":"xem thư mục nhà"}`)
	waitTask(t, svc, bot)
	if len(svc.view().pending()) != 0 {
		t.Fatal("read-only command asked for approval")
	}
	if st, _ := svc.PC.Status(bot); len(st.Log) != 1 || !st.Log[0].ReadOnly {
		t.Fatalf("%+v", st.Log)
	}
}

func lastReply(svc *Service, bot string) string {
	ms := svc.view().messages(bot, 100)
	for i := len(ms) - 1; i >= 0; i-- {
		if ms[i].Role == "agent" {
			return ms[i].Text
		}
	}
	return ""
}
