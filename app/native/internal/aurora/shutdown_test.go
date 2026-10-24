package aurora

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"nekoaurora.local/native/internal/ledger"
)

// SIGTERM mid-answer must not record a failure: the run stays open and the
// next process resumes it (root cause of live "ai_unreachable" after restart).
func TestShutdownMidTurnLeavesRunForResume(t *testing.T) {
	a := &slowAI{delay: 50 * time.Millisecond, next: func(int, string) string { return "một câu trả lời khá dài cho đủ thời gian" }}
	fa := a.server(t)
	defer fa.Close()
	t.Setenv("ZAI_API_KEY", "k")
	t.Setenv("AURORA_AI_BASE_URL", fa.URL)
	dir := t.TempDir()
	st, _ := ledger.Open(dir)
	defer st.Close()
	ctx, cancel := context.WithCancel(context.Background())
	svc, err := New(ctx, st, filepath.Join(dir, "aurora"), Options{RunTurns: true})
	if err != nil {
		t.Fatal(err)
	}
	svc.Tools = nil
	bot := firstBot(svc)
	w, _, _ := st.Accept(ledger.Input{BotID: bot, ClientNonce: "shutdown-1", Text: "chào"})
	svc.poke()
	for a.calls() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(80 * time.Millisecond)
	cancel()
	time.Sleep(300 * time.Millisecond)
	if g, _ := st.Get(w.RequestID); g.Status != "running" {
		t.Fatalf("want running (resumable), got %s %s", g.Status, g.Failure)
	}
}
