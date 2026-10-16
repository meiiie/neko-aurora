package ledger

import (
	"encoding/json"
	"errors"
	"nekobot.local/native/internal/historyexport"
	"strings"
	"testing"
)

func TestSharedExportOneGoalNoStageInstructionsAndBoundedScope(t *testing.T) {
	s, g, _, _ := collaborationFixture(t)
	for n := 0; n < 3; n++ {
		_, w := advanceGroup(t, s, g.ID)
		w = admitGroup(t, s, w)
		if err := s.AppendText(w.RequestID, w.Epoch, "PUBLIC_REPLY"); err != nil {
			t.Fatal(err)
		}
		if err := s.Finish(w.RequestID, w.Epoch, "completed", ""); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.AdvanceCollaborations(); err != nil {
		t.Fatal(err)
	}
	private, _, err := s.Accept(Input{"bot-davin", "export-private-turn", "PRIVATE_HISTORY_CANARY", "foreground"})
	if err != nil {
		t.Fatal(err)
	}
	completeChannelWork(t, s, private, "PRIVATE_REPLY_CANARY")
	before := s.Snapshot().Sequence
	raw, err := s.ExportSharedHistory(g.ChannelID, "", "json")
	if err != nil || s.Snapshot().Sequence != before {
		t.Fatal("export mutated", err)
	}
	var h historyexport.History
	if json.Unmarshal(raw, &h) != nil || len(h.Messages) != 1 || h.Messages[0].RequestID != g.ID || h.Messages[0].Text != g.Goal || h.Messages[0].Reply != "PUBLIC_REPLY" {
		t.Fatal("group owner transcript")
	}
	for _, forbidden := range []string{"Finite collaboration", "Propose a plan", "PRIVATE_HISTORY_CANARY", "PRIVATE_REPLY_CANARY", "contextPin", "accountPin", "runtimeContext"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatal("export exposes", forbidden)
		}
	}
	if _, err = s.ExportSharedHistory(g.ChannelID, "project-"+strings.Repeat("a", 32), "json"); !errors.Is(err, ErrConflict) {
		t.Fatal("foreign project scope", err)
	}
	if _, err = s.ExportSharedHistory(g.ChannelID, "", "html"); !errors.Is(err, ErrInvalid) {
		t.Fatal("unexpected format", err)
	}
}
