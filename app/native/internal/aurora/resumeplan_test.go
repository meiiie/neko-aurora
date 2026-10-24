package aurora

import (
	"strings"
	"testing"
	"time"

	"nekoaurora.local/native/internal/ledger"
)

// A restart mid-plan continues from the next step: finished steps (and
// their notes) are kept, the model is never asked to redo them.
func TestRestartMidPlanContinuesFromCheckpoint(t *testing.T) {
	dir := t.TempDir()
	st, _ := ledger.Open(dir)
	bot := st.Snapshot().Bots[0].ID
	w, _, _ := st.Accept(ledger.Input{BotID: bot, ClientNonce: "plan-resume-1", Text: "Làm giúp: (1) tra A (2) tra B (3) tóm tắt"})
	w, _ = st.DirectStart(w.RequestID, w.Epoch, ledger.DirectRun{Runtime: "direct-http", LeaseUntil: time.Now().Add(time.Minute).UTC().Format(time.RFC3339Nano)})
	w, _ = st.DirectUpdate(w.RequestID, w.Epoch, func(r *ledger.DirectRun) {
		r.PlanIntro = "Mình sẽ làm 3 bước."
		r.Plan = []ledger.DirectStep{{Title: "tra A", Status: "done", Note: "A = 42"}, {Title: "tra B", Status: "running"}, {Title: "tóm tắt", Status: "pending"}}
	})
	st.Close() // process dies mid step 2

	a := &slowAI{delay: time.Millisecond, next: func(n int, body string) string { return "B = 7" }}
	fa := a.server(t)
	defer fa.Close()
	t.Setenv("ZAI_API_KEY", "k")
	t.Setenv("AURORA_AI_BASE_URL", fa.URL)
	svc, st2 := newSvcIn(t, dir) // replays the journal (validates resumeFrom) and recovers
	svc.Tools = nil
	svc.poke()
	waitTask(t, svc, bot)
	got, _ := st2.Get(w.RequestID)
	if got.Status != "completed" || got.Direct == nil || len(got.Direct.Plan) != 3 {
		t.Fatalf("%s %s %+v", got.Status, got.Failure, got.Direct)
	}
	if p := got.Direct.Plan[0]; p.Status != "done" || p.Note != "A = 42" {
		t.Fatalf("step 1 lost: %+v", p)
	}
	for _, b := range a.bodies {
		if strings.Contains(b, "bước 1/3") {
			t.Fatal("step 1 was redone after restart")
		}
	}
	if !strings.Contains(a.bodies[0], "A = 42") || !strings.Contains(a.bodies[0], "bước 2/3") {
		t.Fatalf("first call after restart should be step 2 with step 1's result: %s", a.bodies[0])
	}
	st2.Close()
	st3, err := ledger.Open(dir) // whole journal, including the resumed run, must replay
	if err != nil {
		t.Fatal(err)
	}
	defer st3.Close()
	if g, _ := st3.Get(w.RequestID); g.Status != "completed" || g.ResumeFrom == nil {
		t.Fatalf("replay: %s %+v", g.Status, g.ResumeFrom)
	}
}
