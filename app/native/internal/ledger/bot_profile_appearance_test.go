package ledger

import (
	"errors"
	"testing"
)

func TestBotProfileAppearanceEditPersistAndReplay(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	avatar := "🐱"
	color := "#7C5CFF"
	saved, dup, err := s.SetBotProfile("bot-meiiie", BotProfileInput{Instructions: "Trả lời ngắn gọn.", IncludeHistory: true, Avatar: &avatar, Color: &color, ClientNonce: "profile-appearance-first"})
	if err != nil || dup || saved.Avatar != avatar || saved.Color != color || saved.Revision != 1 {
		t.Fatal(saved, dup, err)
	}

	// Same nonce, same resolved input → duplicate, no new revision
	again, dup, err := s.SetBotProfile("bot-meiiie", BotProfileInput{Instructions: "Trả lời ngắn gọn.", IncludeHistory: true, Avatar: &avatar, Color: &color, ClientNonce: "profile-appearance-first"})
	if err != nil || !dup || again.Revision != 1 {
		t.Fatal("appearance duplicate mismatch", again, dup, err)
	}

	// nil pointers keep current appearance (pre-appearance writers)
	keep, _, err := s.SetBotProfile("bot-meiiie", BotProfileInput{Instructions: "Trả lời đủ ý hơn.", IncludeHistory: true, ExpectedRevision: 1, ClientNonce: "profile-appearance-instructions-only"})
	if err != nil || keep.Avatar != avatar || keep.Color != color || keep.Revision != 2 {
		t.Fatal("nil appearance pointers cleared stored values", keep, err)
	}

	// Empty pointer clears
	empty := ""
	cleared, _, err := s.SetBotProfile("bot-meiiie", BotProfileInput{Instructions: "Trả lời đủ ý hơn.", IncludeHistory: true, Avatar: &empty, ExpectedRevision: 2, ClientNonce: "profile-appearance-clear-avatar"})
	if err != nil || cleared.Avatar != "" || cleared.Color != color || cleared.Revision != 3 {
		t.Fatal("avatar clear failed", cleared, err)
	}

	// Only an appearance change still bumps revision
	newColor := "#22C55E"
	appearanceOnly, _, err := s.SetBotProfile("bot-meiiie", BotProfileInput{Instructions: "Trả lời đủ ý hơn.", IncludeHistory: true, Color: &newColor, ExpectedRevision: 3, ClientNonce: "profile-appearance-color-only"})
	if err != nil || appearanceOnly.Revision != 4 || appearanceOnly.Color != newColor {
		t.Fatal("appearance-only edit rejected", appearanceOnly, err)
	}

	// Nothing changed at all → conflict
	if _, _, err := s.SetBotProfile("bot-meiiie", BotProfileInput{Instructions: "Trả lời đủ ý hơn.", IncludeHistory: true, ExpectedRevision: 4, ClientNonce: "profile-appearance-noop"}); !errors.Is(err, ErrConflict) {
		t.Fatal("no-change profile accepted", err)
	}

	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	// Reopen from disk: event replay must restore appearance fields
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	snap := reopened.Snapshot()
	found := false
	for _, p := range snap.BotProfiles {
		if p.BotID == "bot-meiiie" {
			found = true
			if p.Avatar != "" || p.Color != newColor || p.Revision != 4 {
				t.Fatal("appearance lost after replay", p)
			}
		}
	}
	if !found {
		t.Fatal("profile missing after replay")
	}
}

func TestBotProfileAppearanceValidation(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	badAvatar := "🐱\n"
	if _, _, err := s.SetBotProfile("bot-meiiie", BotProfileInput{Instructions: "ok", IncludeHistory: true, Avatar: &badAvatar, ClientNonce: "profile-bad-avatar-control"}); !errors.Is(err, ErrInvalid) {
		t.Fatal("control-char avatar accepted", err)
	}
	longAvatar := "0123456789abcdef0"
	if _, _, err := s.SetBotProfile("bot-meiiie", BotProfileInput{Instructions: "ok", IncludeHistory: true, Avatar: &longAvatar, ClientNonce: "profile-bad-avatar-long"}); !errors.Is(err, ErrInvalid) {
		t.Fatal("oversized avatar accepted", err)
	}
	for _, badColor := range []string{"7C5CFF", "#7C5CFF0", "#7C5VFF", "#7c5cff\n"} {
		if _, _, err := s.SetBotProfile("bot-meiiie", BotProfileInput{Instructions: "ok", IncludeHistory: true, Color: &badColor, ClientNonce: "profile-bad-color"}); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid color accepted", badColor, err)
		}
	}
	lower := "#7c5cff"
	if _, _, err := s.SetBotProfile("bot-meiiie", BotProfileInput{Instructions: "ok", IncludeHistory: true, Color: &lower, ClientNonce: "profile-good-color-lower"}); err != nil {
		t.Fatal("valid lowercase hex color rejected", err)
	}
}
