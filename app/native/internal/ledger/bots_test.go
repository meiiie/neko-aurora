package ledger

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestCreatedBotAndRoutingSurviveRestartWithoutSourceContext(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	in := BotInput{Name: "Mây", Alias: "may", Description: "Đồng nghiệp viết nội dung", ClientNonce: "create-may-fixture"}
	bot, dup, err := s.CreateBot(in)
	if err != nil || dup {
		t.Fatal(err)
	}
	if again, dup, err := s.CreateBot(in); err != nil || !dup || again.ID != bot.ID {
		t.Fatal("bot retry duplicated", err)
	}
	changed := in
	changed.Name = "Other"
	if _, _, err = s.CreateBot(changed); !errors.Is(err, ErrConflict) {
		t.Fatal("nonce changed bot", err)
	}
	prior, _, err := s.Accept(Input{"bot-meiiie", "source-private-fixture", "private-source-canary", "foreground"})
	if err != nil {
		t.Fatal(err)
	}
	prior, err = s.Claim(prior.RequestID, prior.Epoch)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Admit(prior.RequestID, prior.Epoch, CoreBinding{SessionID: strings.Repeat("a", 32), Task: json.RawMessage(`{"id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","root":"/fixture","activationEpoch":1,"activationId":"cccccccccccccccccccccccccccccccc"}`), Execution: json.RawMessage(`{"version":1}`), CoreSHA256: "ac643e3440ccf8a77e4451688f585643a646d4ba798b128a2c5c89a929c0dab5"}); err != nil {
		t.Fatal(err)
	}
	s.AppendText(prior.RequestID, prior.Epoch, "private-reply-canary")
	s.Finish(prior.RequestID, prior.Epoch, "completed", "")
	routed := Input{"bot-meiiie", "route-may-fixture", "@may Please prepare this exact request", "background"}
	work, _, err := s.Accept(routed)
	if err != nil {
		t.Fatal(err)
	}
	if work.BotID != bot.ID || work.SourceBotID != "bot-meiiie" || work.RoutedAlias != "may" || work.Context.BotName != "Mây" || len(work.Context.Sources) != 0 || strings.Contains(work.RuntimePrompt(), "private-reply-canary") {
		t.Fatal("route leaked source history", work)
	}
	if retry, dup, err := s.Accept(routed); err != nil || !dup || retry.RequestID != work.RequestID {
		t.Fatal("route retry duplicated", err)
	}
	events, _ := s.Events(0)
	events[0].Bot.Name = "mutated"
	if s.Snapshot().Bots[2].Name != "Mây" {
		t.Fatal("bot event alias mutation")
	}
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.Snapshot().Bots[2].ID != bot.ID {
		t.Fatal("bot lost on restart")
	}
	if retry, dup, err := s.Accept(routed); err != nil || !dup || retry.RequestID != work.RequestID {
		t.Fatal("restart route changed identity", err)
	}
	if _, _, err = s.Accept(Input{"bot-meiiie", "unknown-route-fixture", "@unknown hello", "foreground"}); !errors.Is(err, ErrNotFound) {
		t.Fatal("unknown target accepted", err)
	}
}
func TestBotRegistryConflictsAndInvalidNamesDoNotCommit(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, in := range []BotInput{{"Meiiie", "newalias", "", "fixture-name-conflict"}, {"Other", "meiiie", "", "fixture-alias-conflict"}, {"Bad\nName", "bad", "", "fixture-control-name"}, {"Other", "x", "", "fixture-short-alias"}} {
		if _, _, err := s.CreateBot(in); err == nil {
			t.Fatal("invalid duplicate accepted")
		}
	}
	if s.Snapshot().Sequence != 0 {
		t.Fatal("invalid registry state committed")
	}
}
