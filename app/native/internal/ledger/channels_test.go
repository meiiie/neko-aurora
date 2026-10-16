package ledger

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func sharedChannel(t *testing.T, s *Store, name, nonce string) Channel {
	t.Helper()
	c, _, err := s.SaveChannel(ChannelInput{Name: name, BotIDs: []string{"bot-meiiie", "bot-davin"}, ClientNonce: nonce})
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func completeChannelWork(t *testing.T, s *Store, w Work, text string) Work {
	t.Helper()
	w, err := s.Claim(w.RequestID, w.Epoch)
	if err != nil {
		t.Fatal(err)
	}
	b := CoreBinding{SessionID: strings.Repeat("a", 32), Task: json.RawMessage(`{"id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","root":"/fixture","activationEpoch":1,"activationId":"cccccccccccccccccccccccccccccccc"}`), Execution: json.RawMessage(`{"version":1}`), CoreSHA256: "ac643e3440ccf8a77e4451688f585643a646d4ba798b128a2c5c89a929c0dab5"}
	if err = s.Admit(w.RequestID, w.Epoch, b); err != nil {
		t.Fatal(err)
	}
	if err = s.AppendText(w.RequestID, w.Epoch, text); err != nil {
		t.Fatal(err)
	}
	if err = s.Finish(w.RequestID, w.Epoch, "completed", ""); err != nil {
		t.Fatal(err)
	}
	w, _ = s.Get(w.RequestID)
	return w
}
func TestTwoChannelsRoutingProvenanceAndPrivateContextBoundary(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a := sharedChannel(t, s, "Alpha", "create-channel-alpha")
	b := sharedChannel(t, s, "Beta", "create-channel-beta")
	private, _, err := s.Accept(Input{"bot-davin", "private-davin-history", "private-davin-canary", "foreground"})
	if err != nil {
		t.Fatal(err)
	}
	completeChannelWork(t, s, private, "private-davin-reply")
	first, _, err := s.AcceptScoped(Input{"bot-meiiie", "same-channel-nonce", "@davin Answer inside Alpha.", "foreground"}, a.ID, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if first.BotID != "bot-davin" || first.ConversationID != a.ID || first.Publication.ReplyChannelID != a.ID || first.Publication.ActorID != "local-owner" || strings.Contains(first.RuntimePrompt(), "private-davin-canary") {
		t.Fatal("wrong audience/private history")
	}
	completeChannelWork(t, s, first, "alpha-public-reply")
	second, _, err := s.AcceptScoped(Input{"bot-meiiie", "same-channel-nonce", "@davin Answer inside Beta.", "foreground"}, b.ID, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if second.RequestID == first.RequestID || len(second.Context.Sources) != 0 || strings.Contains(second.RuntimePrompt(), "alpha-public-reply") {
		t.Fatal("nonce or context crossed channel")
	}
	newer, _, err := s.AcceptScoped(Input{"bot-meiiie", "alpha-followup-nonce", "@davin Continue Alpha.", "foreground"}, a.ID, "", 0)
	if err != nil || len(newer.Context.Sources) != 1 || newer.Context.Sources[0].RequestID != first.RequestID {
		t.Fatal("eligible same-channel bot context missing", err)
	}
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal("provenance replay failed", err)
	}
	defer s.Close()
	restored, _ := s.Get(first.RequestID)
	if restored.Publication.ReplyThreadID != first.Publication.ReplyThreadID {
		t.Fatal("restart changed origin thread")
	}
	retry, dup, err := s.AcceptScoped(Input{"bot-meiiie", "same-channel-nonce", "@davin Answer inside Alpha.", "foreground"}, a.ID, "", 0)
	if err != nil || !dup || retry.RequestID != first.RequestID {
		t.Fatal("channel nonce retry changed identity", err)
	}
}
func TestChannelMembershipChangeFencesQueuedAndActivePublication(t *testing.T) {
	s, _, _, _, _ := accountFixture(t)
	c := sharedChannel(t, s, "Scoped channel", "channel-membership-create")
	active, _, err := s.AcceptScoped(Input{"bot-meiiie", "active-channel-work", "@davin local reply", "background"}, c.ID, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	active = admitBound(t, s, active)
	queued, _, err := s.AcceptScoped(Input{"bot-meiiie", "queued-channel-work", "@davin queued reply", "foreground"}, c.ID, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	current, _, err := s.SaveChannel(ChannelInput{ID: c.ID, ExpectedRevision: c.Revision, Name: c.Name, BotIDs: []string{"bot-meiiie"}, ClientNonce: "remove-channel-davin"})
	if err != nil || current.Revision != 2 {
		t.Fatal(err)
	}
	q, _ := s.Get(queued.RequestID)
	running, _ := s.Get(active.RequestID)
	if q.Status != "suspended" || running.Status != "suspending" {
		t.Fatal("audience revision did not fence work")
	}
	if err = s.AppendText(active.RequestID, active.Epoch, "forbidden"); !errors.Is(err, ErrConflict) {
		t.Fatal("late publication accepted")
	}
	if _, err = s.ExecuteTool(active.RequestID, running.Epoch, invocation("forbidden file")); !errors.Is(err, ErrConflict) {
		t.Fatal("current epoch bypassed audience")
	}
	if _, _, err = s.AcceptScoped(Input{"bot-meiiie", "denied-channel-tag", "@davin missing membership", "foreground"}, c.ID, "", 0); !errors.Is(err, ErrConflict) {
		t.Fatal("tag granted missing bot membership")
	}
	if _, _, err = s.SaveChannel(ChannelInput{ID: c.ID, ExpectedRevision: 1, Name: c.Name, BotIDs: []string{"bot-meiiie", "bot-davin"}, ClientNonce: "stale-channel-regrant"}); !errors.Is(err, ErrConflict) {
		t.Fatal("stale audience edit accepted")
	}
	if _, _, err = s.SaveChannel(ChannelInput{ID: c.ID, ExpectedRevision: 2, Name: c.Name, BotIDs: []string{"bot-meiiie", "bot-davin"}, ClientNonce: "current-channel-regrant"}); err != nil {
		t.Fatal(err)
	}
	if s.RuntimeAllowed(active.RequestID, running.Epoch) {
		t.Fatal("regrant revived old audience")
	}
	events, _ := s.Events(0)
	for i := range events {
		if events[i].Channel != nil {
			events[i].Channel.BotIDs[0] = "forged"
		}
	}
	if s.Snapshot().Channels[0].BotIDs[0] == "forged" {
		t.Fatal("event pointer changed durable ACL")
	}
}
