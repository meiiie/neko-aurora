package aurora

import (
	"strings"
	"testing"
	"time"

	"nekoaurora.local/native/internal/ledger"
	"nekoaurora.local/native/internal/vault"
)

func TestRestartResumesThenFails(t *testing.T) {
	dir := t.TempDir()
	st, _ := ledger.Open(dir)
	defer st.Close()
	v, _ := vault.Open(t.TempDir())
	s := &Service{L: st, V: v, running: map[string]bool{}, Now: time.Now}
	bot := st.Snapshot().Bots[0].ID
	w, _, _ := st.Accept(ledger.Input{BotID: bot, ClientNonce: "resume-svc-1", Text: "việc dài"})
	for i := 0; i <= ledger.MaxDirectResumes; i++ {
		w, _ = st.Get(w.RequestID)
		if !eligible(w) {
			t.Fatalf("round %d: resumed work must be eligible again: %+v", i, w)
		}
		w, _ = st.DirectStart(w.RequestID, w.Epoch, ledger.DirectRun{Runtime: "direct-http", LeaseUntil: s.leaseUntil()})
		s.recoverInterrupted() // process restarted mid-run
	}
	got, _ := st.Get(w.RequestID)
	if got.Status != "failed" || got.Failure != "interrupted_by_restart" || got.DirectResumes != ledger.MaxDirectResumes {
		t.Fatalf("after budget: %+v", got)
	}
}

func TestSweepRequeuesExpiredUnownedLease(t *testing.T) {
	st, _ := ledger.Open(t.TempDir())
	defer st.Close()
	v, _ := vault.Open(t.TempDir())
	now := time.Now()
	s := &Service{L: st, V: v, running: map[string]bool{}, Now: func() time.Time { return now }}
	bot := st.Snapshot().Bots[0].ID
	w, _, _ := st.Accept(ledger.Input{BotID: bot, ClientNonce: "resume-svc-2", Text: "x"})
	w, _ = st.DirectStart(w.RequestID, w.Epoch, ledger.DirectRun{Runtime: "direct-http", LeaseUntil: s.leaseUntil()})
	s.running[bot] = true
	now = now.Add(2 * leaseFor)
	s.sweepLeases()
	if g, _ := st.Get(w.RequestID); g.Status != "running" {
		t.Fatal("owned run must not be swept")
	}
	delete(s.running, bot)
	s.sweepLeases()
	if g, _ := st.Get(w.RequestID); g.Status != "waiting_host" || g.DirectResumes != 1 {
		t.Fatalf("%+v", g)
	}
}

func TestSteeredMessageShownBeforeRewrittenAnswer(t *testing.T) {
	st, _ := ledger.Open(t.TempDir())
	defer st.Close()
	bot := st.Snapshot().Bots[0].ID
	a, _, _ := st.Accept(ledger.Input{BotID: bot, ClientNonce: "steer-order-1", Text: "tin công nghệ"})
	b, _, _ := st.Accept(ledger.Input{BotID: bot, ClientNonce: "steer-order-2", Text: "chỉ tin AI"})
	a, _ = st.DirectStart(a.RequestID, a.Epoch, ledger.DirectRun{Runtime: "direct-http", Ack: "Được"})
	a, _ = st.DirectUpdate(a.RequestID, a.Epoch, func(r *ledger.DirectRun) { r.Steered = []string{b.RequestID} })
	_ = st.AppendText(a.RequestID, a.Epoch, "3 tin AI")
	_, _ = st.DirectFinish(a.RequestID, a.Epoch, "completed", "", nil)
	var ids []string
	for _, m := range newView(st.Snapshot()).messages(bot, 0) {
		ids = append(ids, m.ID)
	}
	want := []string{a.RequestID + ":u", a.RequestID + ":ack", b.RequestID + ":u", a.RequestID}
	got := strings.Join(ids, ",")
	if got != strings.Join(want, ",") {
		t.Fatalf("order %s", got)
	}
}
