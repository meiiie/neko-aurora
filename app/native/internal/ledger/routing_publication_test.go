package ledger

import (
	"strings"
	"testing"
)

func TestTaggedColleagueKeepsOriginAndExcludesTargetPrivateHistory(t *testing.T) {
	s, private := running(t, t.TempDir())
	defer s.Close()
	if err := s.AppendText(private.RequestID, private.Epoch, "target-private-canary"); err != nil {
		t.Fatal(err)
	}
	if err := s.Finish(private.RequestID, private.Epoch, "completed", ""); err != nil {
		t.Fatal(err)
	}
	tagged, _, err := s.Accept(Input{"bot-davin", "origin-tag-fixture", "@meiiie Reply here to the current request.", "foreground"})
	if err != nil {
		t.Fatal(err)
	}
	if tagged.BotID != "bot-meiiie" || tagged.ConversationID != "conversation-bot-davin" {
		t.Fatal("tag changed reply destination", tagged.ConversationID)
	}
	if strings.Contains(tagged.RuntimePrompt(), "target-private-canary") || len(tagged.Context.Sources) != 0 {
		t.Fatal("target private context leaked to origin")
	}
}
