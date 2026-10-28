package aurora

import (
	"context"
	"strings"
	"testing"

	"nekoaurora.local/native/internal/ledger"
)

func TestHarnessCapacityMatchesLedger(t *testing.T) {
	dir := t.TempDir()
	st, err := ledger.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	bot := st.Snapshot().Bots[0].ID
	w, _, err := st.Accept(ledger.Input{BotID: bot, ClientNonce: "research-capacity-20261026", Text: "capacity fixture", Lane: "foreground"})
	if err != nil {
		t.Fatal(err)
	}
	w, err = st.DirectStart(w.RequestID, w.Epoch, ledger.DirectRun{Runtime: "direct-http"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < maxToolsPerTurn; i++ {
		_, err = st.DirectUpdate(w.RequestID, w.Epoch, func(d *ledger.DirectRun) {
			d.Tools = append(d.Tools, ledger.DirectTool{Name: "web_search", Arg: itoa(i), Status: "done"})
		})
		if err != nil {
			t.Fatalf("harness allows %d tools, ledger rejects tool %d: %v", maxToolsPerTurn, i+1, err)
		}
	}
	if _, err := st.DirectUpdate(w.RequestID, w.Epoch, func(d *ledger.DirectRun) {
		d.Tools = append(d.Tools, ledger.DirectTool{Name: "web_search", Arg: "overflow", Status: "done"})
	}); err == nil {
		t.Fatal("capacity overflow accepted")
	}
	st.Close()
	reopened, err := ledger.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := reopened.Get(w.RequestID)
	if err != nil || got.Direct == nil || len(got.Direct.Tools) != maxToolsPerTurn {
		t.Fatal("capacity lost on replay", err)
	}
}

func TestToolAdmissionStopsWhenJournalCannotSave(t *testing.T) {
	st, err := ledger.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	bot := st.Snapshot().Bots[0].ID
	w, _, err := st.Accept(ledger.Input{BotID: bot, ClientNonce: "capacity-admission-failure", Text: "test"})
	if err != nil {
		t.Fatal(err)
	}
	w, err = st.DirectStart(w.RequestID, w.Epoch, ledger.DirectRun{Runtime: "direct-http"})
	if err != nil {
		t.Fatal(err)
	}
	st.Close()
	s := &Service{L: st}
	tc := &toolCtx{}
	s.runTools(context.Background(), w, tc, []toolCall{{name: "web_search", arg: "fixture"}})
	if len(tc.blocks) != 1 || !strings.Contains(tc.blocks[0], "Chưa chạy thao tác") {
		t.Fatal("tool crossed failed journal admission")
	}
}
