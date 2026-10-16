package ledger

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestProfileHistorySelectionAndPinsStayImmutableAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	prior := completedPermissionSource(t, s, "profile-history-source", "prior history canary")
	old, _, err := s.Accept(input("profile-legacy-queued", "background"))
	if err != nil || len(old.Context.Sources) != 1 {
		t.Fatal(err)
	}
	first := BotProfileInput{Instructions: "Use clear Vietnamese.\nKeep equations intact.", IncludeHistory: false, ClientNonce: "profile-save-first"}
	p, dup, err := s.SetBotProfile("bot-meiiie", first)
	if err != nil || dup || p.Revision != 1 {
		t.Fatal(err)
	}
	s.ConfigureContextAdmission(true)
	w, _, err := s.Accept(input("profile-no-history", "foreground"))
	if err != nil || w.Context.Version != 4 || len(w.Context.Sources) != 0 || w.BotProfile == nil || w.BotProfile.Revision != 1 || strings.Contains(w.RuntimePrompt(), "prior history canary") {
		t.Fatal("history disabled pin", err)
	}
	found := false
	for _, source := range w.RuntimeContext.Sources {
		if source.ID == "profile:bot-meiiie" && strings.Contains(source.Span.Text, "Keep equations intact.") {
			found = true
		}
	}
	if !found {
		t.Fatal("profile missing from authenticated runtime context")
	}
	second := first
	second.ExpectedRevision = 1
	second.IncludeHistory = true
	second.ClientNonce = "profile-save-second"
	current, _, err := s.SetBotProfile("bot-meiiie", second)
	if err != nil {
		t.Fatal(err)
	}
	newer, _, err := s.Accept(input("profile-history-enabled", "foreground"))
	if err != nil || len(newer.Context.Sources) != 1 || newer.Context.Sources[0].RequestID != prior.RequestID || newer.BotProfile.Revision != 2 {
		t.Fatal("history reenabled", err)
	}
	if _, _, err = s.SetSourceAccess(AccessInput{RequestID: w.RequestID, ExpectedRevision: 0, Granted: false, ClientNonce: "profile-own-access-off"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.SetSourceAccess(AccessInput{RequestID: w.RequestID, ExpectedRevision: 1, Granted: true, ClientNonce: "profile-own-access-on"}); err != nil {
		t.Fatal(err)
	}
	fenced, _ := s.Get(w.RequestID)
	refreshed, err := s.RefreshContext(RefreshInput{RequestID: w.RequestID, ExpectedEpoch: fenced.Epoch})
	if err != nil || refreshed.BotProfile.Revision != 1 || refreshed.Context.Profile.Revision != 1 || len(refreshed.Context.Sources) != 0 {
		t.Fatal("refresh silently changed profile or imported disabled history", err)
	}
	w = refreshed
	for round := 0; round < 3; round++ {
		stored, _ := s.Get(w.RequestID)
		if !reflect.DeepEqual(stored, w) {
			t.Fatal("existing pin changed", round)
		}
		legacy, _ := s.Get(old.RequestID)
		if !reflect.DeepEqual(legacy, old) {
			t.Fatal("legacy pin changed", round)
		}
		sequence := s.Snapshot().Sequence
		if retry, duplicate, err := s.SetBotProfile("bot-meiiie", first); err != nil || !duplicate || retry != current || sequence != s.Snapshot().Sequence {
			t.Fatal("old preference receipt changed", err)
		}
		events, _ := s.Events(0)
		for i := range events {
			if events[i].BotProfile != nil {
				events[i].BotProfile.Instructions = "caller mutation"
			}
		}
		if retry, _, _ := s.SetBotProfile("bot-meiiie", first); retry != current {
			t.Fatal("receipt aliased")
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
		if round < 2 {
			s, err = Open(dir)
			if err != nil {
				t.Fatal("legacy and v4 replay", err)
			}
			s.ConfigureContextAdmission(true)
		}
	}
}

func TestProfileRejectsSecretBoundsForgedAuthorityAndCrossKindNonce(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, text := range []string{strings.Repeat("x", 4001), "bad\x00text", "api_key=\"" + strings.Repeat("x", 32) + "\""} {
		if _, _, err := s.SetBotProfile("bot-meiiie", BotProfileInput{Instructions: text, IncludeHistory: false, ClientNonce: "profile-invalid-input"}); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid preferences persisted", err)
		}
	}
	if s.Snapshot().Sequence != 0 {
		t.Fatal("invalid input mutated journal")
	}
	p, _, err := s.SetBotProfile("bot-meiiie", BotProfileInput{Instructions: "Be concise", IncludeHistory: false, ClientNonce: "profile-shared-nonce"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.RenameBot("bot-meiiie", BotNameInput{Name: "Other", ClientNonce: "profile-shared-nonce"}); !errors.Is(err, ErrConflict) {
		t.Fatal("profile nonce became rename", err)
	}
	if _, _, err := s.CreateBot(BotInput{Name: "Other", Alias: "other", ClientNonce: "profile-shared-nonce"}); !errors.Is(err, ErrConflict) {
		t.Fatal("profile nonce became create", err)
	}
	p.Revision++
	if validReplay(s, Event{Type: "bot.profile_updated", BotProfile: &p, Nonce: "profile-forged-event", ResourceRevision: 1}) {
		t.Fatal("profile event accepted resource authority")
	}
	w, _, err := s.Accept(input("profile-forgery-intake", "foreground"))
	if err != nil {
		t.Fatal(err)
	}
	w.BotProfile.Instructions = "forged"
	w.RequestID = "req-profile-forged-new"
	w.SourceMessageID = "msg-profile-forged-new"
	w.ClientNonce = "profile-forged-new-nonce"
	w.CreatedSequence = uint64(len(s.events) + 1)
	w.Publication.SourceMessageID = w.SourceMessageID
	w.Publication.ReplyThreadID = "thread-" + w.SourceMessageID
	w.Context = s.captureContext(w)
	if validReplay(s, Event{Sequence: w.CreatedSequence, Type: "work.accepted", Work: w}) {
		t.Fatal("intake accepted foreign preferences")
	}
}
