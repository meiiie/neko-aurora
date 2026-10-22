package ledger

import (
	"errors"
	"fmt"
	"testing"
)

// Archived bots must not count toward the 32 live-bot cap, before and after replay.
func TestArchivedBotsDoNotCountTowardLimit(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	var made []Bot
	for i := 0; ; i++ {
		b, _, err := s.CreateBot(BotInput{Name: fmt.Sprintf("Limit %d", i), Alias: fmt.Sprintf("limit%d", i), ClientNonce: fmt.Sprintf("limit-nonce-%04d", i)})
		if err != nil {
			if !errors.Is(err, ErrConflict) || s.activeBotCount() != MaxActiveBots {
				t.Fatalf("stopped at %d active with %v", s.activeBotCount(), err)
			}
			break
		}
		made = append(made, b)
	}
	if _, _, err = s.ControlBot(BotControlInput{BotID: made[0].ID, Action: "archive", ClientNonce: "limit-archive-0001"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.CreateBot(BotInput{Name: "After archive", Alias: "afterarchive", ClientNonce: "limit-after-archive"}); err != nil {
		t.Fatalf("create after archive: %v", err)
	}
	if _, _, err = s.ControlBot(BotControlInput{BotID: made[0].ID, Action: "restore", ExpectedRevision: 1, ClientNonce: "limit-restore-0001"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("restore past the cap = %v", err)
	}
	s.Close()
	r, err := Open(dir)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	defer r.Close()
	if r.activeBotCount() != MaxActiveBots {
		t.Fatalf("replayed active = %d", r.activeBotCount())
	}
}
