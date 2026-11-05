package ledger

import (
	"strings"
	"testing"
	"time"
)

func TestDirectPartialsSurviveReplayAndDoNotAliasSnapshots(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	bot := s.Snapshot().Bots[0].ID
	w, _, err := s.Accept(Input{BotID: bot, ClientNonce: "partial-replay-1", Text: "Hello"})
	if err != nil {
		t.Fatal(err)
	}
	w, err = s.DirectStart(w.RequestID, w.Epoch, DirectRun{Runtime: "direct-http"})
	if err != nil {
		t.Fatal(err)
	}
	part := DirectPartial{Text: "Visible answer so far", At: time.Now().UTC().Format(time.RFC3339Nano), BeforeRequestID: "owner-followup"}
	_, err = s.DirectUpdate(w.RequestID, w.Epoch, func(r *DirectRun) { r.Partials = []DirectPartial{part} })
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _ := s.Get(w.RequestID)
	snapshot.Direct.Partials[0].Text = "not journaled"
	again, _ := s.Get(w.RequestID)
	if again.Direct.Partials[0] != part {
		t.Fatal("snapshot mutated stored partial")
	}
	for _, bad := range []DirectPartial{{Text: "", At: part.At, BeforeRequestID: part.BeforeRequestID}, {Text: strings.Repeat("x", 60001), At: part.At, BeforeRequestID: part.BeforeRequestID}, {Text: part.Text, At: "invalid", BeforeRequestID: part.BeforeRequestID}} {
		if _, err := s.DirectUpdate(w.RequestID, w.Epoch, func(r *DirectRun) { r.Partials = []DirectPartial{bad} }); err == nil {
			t.Fatal("invalid partial accepted")
		}
	}
	if _, err := s.DirectFinish(w.RequestID, w.Epoch, "completed", "", nil); err != nil {
		t.Fatal(err)
	}
	s.Close()
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, _ := reopened.Get(w.RequestID)
	if len(got.Direct.Partials) != 1 || got.Direct.Partials[0] != part {
		t.Fatal("partial lost after replay")
	}
}
