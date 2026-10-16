package ledger

import (
	"errors"
	"reflect"
	"testing"
)

func TestBotRenamePreservesPinnedWorkAndOriginalReceiptsAcrossReplay(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	create := BotInput{Name: "Research", Alias: "research", Description: "Local research bot", ClientNonce: "rename-create-fixture"}
	bot, _, err := s.CreateBot(create)
	if err != nil {
		t.Fatal(err)
	}
	old, _, err := s.Accept(Input{bot.ID, "rename-old-work-fixture", "Keep this original request", "background"})
	if err != nil {
		t.Fatal(err)
	}
	first := BotNameInput{Name: "Research New", ExpectedDisplayRevision: 0, ClientNonce: "rename-first-fixture"}
	updated, dup, err := s.RenameBot(bot.ID, first)
	if err != nil || dup || updated.ID != bot.ID || updated.Alias != bot.Alias || updated.Revision != bot.Revision || updated.DisplayRevision != 1 {
		t.Fatal("display rename changed identity or authority", updated, dup, err)
	}
	unchanged, _ := s.Get(old.RequestID)
	if !reflect.DeepEqual(old, unchanged) {
		t.Fatal("rename rewrote already accepted work")
	}
	newer, _, err := s.Accept(Input{"bot-meiiie", "rename-new-route-fixture", "@research Continue in the original alias", "foreground"})
	if err != nil || newer.BotID != bot.ID || newer.Context.BotName != updated.Name || newer.RoutedAlias != "research" {
		t.Fatal("new intake did not use current display name and stable alias", newer, err)
	}
	second := BotNameInput{Name: "Research Latest", ExpectedDisplayRevision: 1, ClientNonce: "rename-second-fixture"}
	latest, _, err := s.RenameBot(bot.ID, second)
	if err != nil {
		t.Fatal(err)
	}
	for round := 0; round < 4; round++ {
		seq := s.Snapshot().Sequence
		if retry, dup, err := s.RenameBot(bot.ID, first); err != nil || !dup || retry != latest {
			t.Fatal("old rename ACK rolled back current name", round, retry, dup, err)
		}
		if retry, dup, err := s.CreateBot(create); err != nil || !dup || retry != latest {
			t.Fatal("original create receipt changed after rename", round, retry, dup, err)
		}
		if retry, dup, err := s.Accept(Input{bot.ID, "rename-old-work-fixture", "Keep this original request", "background"}); err != nil || !dup || !reflect.DeepEqual(old, retry) {
			t.Fatal("old request changed on replay", round, err)
		}
		if s.Snapshot().Sequence != seq {
			t.Fatal("duplicate receipt committed a new event")
		}
		events, err := s.Events(0)
		if err != nil {
			t.Fatal(err)
		}
		for i := range events {
			if events[i].Bot != nil {
				events[i].Bot.Name = "caller-mutated"
			}
		}
		if retry, _, err := s.RenameBot(bot.ID, first); err != nil || retry != latest {
			t.Fatal("event caller mutated receipt")
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
		if round == 3 {
			break
		}
		s, err = Open(dir)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestBotRenameConflictsAndForgedMetadataNeverCommit(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, in := range []BotNameInput{
		{Name: "Davin", ClientNonce: "rename-later-bot-fixture"},
		{Name: "dAvIn", ClientNonce: "rename-case-conflict-fixture"},
		{Name: "Bad\nName", ClientNonce: "rename-control-fixture"},
		{Name: "Meiiie", ClientNonce: "rename-noop-fixture"},
		{Name: "New", ExpectedDisplayRevision: 1, ClientNonce: "rename-stale-fixture"},
	} {
		if _, _, err := s.RenameBot("bot-meiiie", in); err == nil {
			t.Fatal("invalid rename accepted", in)
		}
	}
	if s.Snapshot().Sequence != 0 {
		t.Fatal("rejected rename committed state")
	}
	bot := s.Snapshot().Bots[0]
	bot.Name = "Meiiie New"
	bot.DisplayRevision++
	if s.validBotNameEvent(Event{Type: "bot.renamed", Bot: &bot, Nonce: "rename-aux-fixture", ContextClaimVersion: 2}) {
		t.Fatal("rename accepted extra authority fields")
	}
	for _, mutation := range []func(*Bot){func(b *Bot) { b.Alias = "other" }, func(b *Bot) { b.Revision++ }, func(b *Bot) { b.Description = "forged" }, func(b *Bot) { b.ID = "other-id" }} {
		forged := bot
		mutation(&forged)
		if s.validBotNameEvent(Event{Type: "bot.renamed", Bot: &forged, Nonce: "rename-forged-fixture"}) {
			t.Fatal("rename accepted runtime metadata mutation", forged)
		}
	}
	if _, _, err := s.RenameBot("missing", BotNameInput{Name: "New", ClientNonce: "rename-missing-fixture"}); !errors.Is(err, ErrNotFound) {
		t.Fatal("unknown bot accepted", err)
	}
}
