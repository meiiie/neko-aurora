package aurora

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nekoaurora.local/native/internal/computer"
	"nekoaurora.local/native/internal/ledger"
)

func tool(m map[string]any) string { b, _ := json.Marshal(m); return "TOOL: " + string(b) }

func liveSvc(t *testing.T, replies ...string) (*Service, string, string) {
	t.Helper()
	fa := scriptedAI(t, replies...)
	t.Cleanup(fa.Close)
	t.Setenv("ZAI_API_KEY", "k")
	t.Setenv("AURORA_AI_BASE_URL", fa.URL)
	svc, _ := newSvc(t)
	if svc.PC == nil {
		t.Skip("computers off")
	}
	srv := httptest.NewServer(svc)
	t.Cleanup(srv.Close)
	return svc, srv.URL, firstBot(svc)
}

func waitFor(t *testing.T, what string, f func() bool) {
	t.Helper()
	for end := time.Now().Add(10 * time.Second); time.Now().Before(end); time.Sleep(20 * time.Millisecond) {
		if f() {
			return
		}
	}
	t.Fatal("timed out waiting for " + what)
}

// Live run 7: a 15 KB script with a quote bug was parked for approval twice
// and failed with exit 2 each time. Now the syntax error comes straight back
// in the same turn, the bot patches the line, and only then is the card shown.
func TestPySyntaxErrorGoesBackBeforeApproval(t *testing.T) {
	bad := "x = 1\nprint('khong phai 'co the'')\n"
	svc, u, bot := liveSvc(t,
		tool(map[string]any{"name": "computer_write_file", "path": "deck/build.py", "content": bad}),
		tool(map[string]any{"name": "computer_run", "cmd": "mkdir -p out && python3 deck/build.py"}),
		tool(map[string]any{"name": "computer_edit_file", "path": "deck/build.py", "old": "print('khong phai 'co the'')", "new": `print("khong phai 'co the'")`}),
		tool(map[string]any{"name": "computer_run", "cmd": "mkdir -p out && python3 deck/build.py"}),
		"Xong.")
	askFirst(svc, bot) // card flow
	post(t, u+"/app/api/agents/"+bot+"/messages", `{"text":"viết script python rồi chạy trên máy tính"}`)
	waitTask(t, svc, bot)
	txt, _, _ := svc.PC.ReadFile(bot, "deck/build.py")
	if !strings.Contains(txt, `print("khong phai 'co the'")`) {
		t.Fatalf("not patched: %q", txt)
	}
	p := svc.view().pending()
	if len(p) != 1 {
		t.Fatalf("want one approval card after the fix, got %d", len(p))
	}
	st, _ := svc.PC.Status(bot)
	if len(st.Log) != 0 {
		t.Fatalf("syntax checks must not appear in the command log: %+v", st.Log)
	}
	for _, f := range st.Files {
		if strings.Contains(f.Path, "__pycache__") {
			t.Fatal("check left __pycache__")
		}
	}
	if svc.pyPrecheck(t.Context(), bot, "python3 deck/build.py") != "" {
		t.Fatal("fixed script still flagged")
	}
}

// Live run 7: rounds ended on "context deadline exceeded". Out of time, the
// turn now ends on a checkpoint note and the work continues as a new turn.
func TestOutOfTimeContinuesAsNewTurn(t *testing.T) {
	old := turnBudget
	turnBudget = 0
	t.Cleanup(func() { turnBudget = old })
	svc, u, bot := liveSvc(t,
		tool(map[string]any{"name": "computer_write_file", "path": "a.txt", "content": "1"}),
		tool(map[string]any{"name": "computer_write_file", "path": "b.txt", "content": "2"}),
		"Đã ghi a.txt và b.txt.")
	post(t, u+"/app/api/agents/"+bot+"/messages", `{"text":"ghi hai file trên máy tính"}`)
	waitFor(t, "final reply", func() bool { return strings.Contains(lastReply(svc, bot), "Đã ghi a.txt và b.txt") })
	for _, f := range []string{"a.txt", "b.txt"} {
		if _, _, err := svc.PC.ReadFile(bot, f); err != nil {
			t.Fatal(f, err)
		}
	}
	all, _ := json.Marshal(svc.L.Snapshot())
	if !strings.Contains(string(all), checkpointText) || strings.Contains(string(all), "deadline") {
		t.Fatal("expected a checkpoint note and no deadline error")
	}
	if n := svc.pcSnap(bot).resumes; n < 1 || n > maxResumes {
		t.Fatal("resumes", n)
	}
}

func TestCheckpointBounded(t *testing.T) {
	old := turnBudget
	turnBudget = 0
	t.Cleanup(func() { turnBudget = old })
	svc := quietSvc(t)
	if svc.PC == nil {
		t.Skip()
	}
	w := startWorkLite(firstBot(svc), "chạy script trên máy tính")
	n := 0
	for i := 0; i < maxResumes+3; i++ {
		if _, ok := svc.checkpoint(w, time.Now()); ok {
			n++
		}
	}
	if n != maxResumes {
		t.Fatalf("checkpoints %d, want %d", n, maxResumes)
	}
	turnBudget = time.Hour
	svc.newRequest(w.BotID, "", true)
	if _, ok := svc.checkpoint(w, time.Now()); ok {
		t.Fatal("within budget")
	}
}

// A reply cut at max_tokens loses its TOOL line; the bot is told to write in
// chunks, and append builds the file.
func TestCutReplyLeadsToChunkedWrite(t *testing.T) {
	svc, u, bot := liveSvc(t,
		`TOOL: {"name":"computer_write_file","path":"big.py","content":"print(1)\nprint(<<CUT>>`,
		tool(map[string]any{"name": "computer_write_file", "path": "big.py", "content": "print(1)\n"}),
		tool(map[string]any{"name": "computer_write_file", "path": "big.py", "content": "print(2)\n", "append": true}),
		"Đã ghi big.py.")
	post(t, u+"/app/api/agents/"+bot+"/messages", `{"text":"ghi script dài trên máy tính"}`)
	waitTask(t, svc, bot)
	if txt, _, _ := svc.PC.ReadFile(bot, "big.py"); txt != "print(1)\nprint(2)\n" {
		t.Fatalf("%q", txt)
	}
}

func TestEditFileNeedsUniqueMatch(t *testing.T) {
	svc := quietSvc(t)
	if svc.PC == nil {
		t.Skip()
	}
	bot := firstBot(svc)
	svc.PC.WriteFile(bot, "a.py", "x=1\nx=1\n")
	arg := func(o, n string) string {
		b, _ := json.Marshal(map[string]string{"path": "a.py", "old": o, "new": n})
		return string(b)
	}
	if _, err := svc.editFile(bot, arg("x=1", "y")); err == nil || !strings.Contains(err.Error(), "2 lần") {
		t.Fatal(err)
	}
	if _, err := svc.editFile(bot, arg("z", "y")); err == nil {
		t.Fatal("missing match accepted")
	}
	if _, err := svc.editFile(bot, arg("x=1\nx=1", "x=2")); err != nil {
		t.Fatal(err)
	}
	if bigFileHint("a.py", 9000) == "" || bigFileHint("a.md", 9000) != "" {
		t.Fatal("hint")
	}
}

// Live run 9: a failed edit reached the bot and the card as "lỗi mạng".
func TestComputerToolErrorIsSpecific(t *testing.T) {
	if got := toolErr("computer_edit_file", errors.New("không tìm thấy đoạn old trong file")); !strings.Contains(got, "không tìm thấy") {
		t.Fatal(got)
	}
	if toolErr("web_fetch", errors.New("dial tcp 1.2.3.4: refused")) != "lỗi mạng" {
		t.Fatal("web errors must stay short")
	}
}

// Live run 9: turns ended on an empty bubble after tool steps while the video
// was still missing. Now the work continues as a new turn.
func TestEmptyReplyWithOpenItemsContinues(t *testing.T) {
	svc, u, bot := liveSvc(t,
		tool(map[string]any{"name": "computer_read_file", "path": "x.py"}),
		"",
		"", "", "",
		"Video đã xong.\nKHÔNG THỂ: video — thử nghiệm")
	svc.PC.WriteFile(bot, "x.py", "print(1)")
	post(t, u+"/app/api/agents/"+bot+"/messages", `{"text":"quay video xem trước trên máy tính"}`)
	waitFor(t, "continued turn", func() bool { return strings.Contains(lastReply(svc, bot), "KHÔNG THỂ: video") })
	all, _ := json.Marshal(svc.L.Snapshot())
	if !strings.Contains(string(all), checkpointText) {
		t.Fatal("no continuation note")
	}
}

func TestEmptyReplyNoItemsGetsStatus(t *testing.T) {
	svc := quietSvc(t)
	if svc.PC == nil {
		t.Skip()
	}
	w := startWorkLite(firstBot(svc), "chạy script trên máy tính")
	d, ok := svc.unfinished(w, directives{})
	if !ok || d.Text == "" {
		t.Fatal("blank bubble")
	}
	if _, ok := svc.unfinished(w, directives{Text: "Xong, đây là kết quả."}); ok {
		t.Fatal("finished reply touched")
	}
}

func ledgerInput(bot, nonce, text string) ledger.Input {
	return ledger.Input{BotID: bot, ClientNonce: nonce, Text: text}
}

// askFirst turns on "Hỏi trước khi chạy lệnh" for tests of the card flow.
func askFirst(svc *Service, bot string) { svc.PC.SetPolicy(bot, computer.Policy{AskFirst: true}) }

// Darren: the bot's own sandbox needs no approvals (writes, pip, scripts).
func TestSandboxCommandsRunWithoutCard(t *testing.T) {
	svc, u, bot := liveSvc(t,
		tool(map[string]any{"name": "computer_run", "cmd": "mkdir -p out && echo 42 > out/a.txt && python3 -c 'print(6*7)'"}),
		"Xong: out/a.txt")
	post(t, u+"/app/api/agents/"+bot+"/messages", `{"text":"chạy lệnh trên máy tính"}`)
	waitTask(t, svc, bot)
	if len(svc.view().pending()) != 0 {
		t.Fatal("an approval card was shown for a sandbox command")
	}
	if txt, _, err := svc.PC.ReadFile(bot, "out/a.txt"); err != nil || strings.TrimSpace(txt) != "42" {
		t.Fatalf("command did not run: %q %v", txt, err)
	}
}
