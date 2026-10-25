package ledger

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func networkSession(t *testing.T, s *Store, bot, conversation, nonce string) NetworkSession {
	t.Helper()
	x, _, err := s.RegisterNetworkSession(NetworkSessionInput{BotID: bot, ConversationID: conversation, DeviceID: "server", Alias: bot, Tier: "turn", ExpectedChannelRevision: 1, ClientNonce: nonce})
	if err != nil {
		t.Fatal(err)
	}
	return x
}

func TestNetworkDispatchIsAtomicAndNeverReroutesPeerText(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	to := networkSession(t, s, "bot-davin", "conversation-bot-davin", "session-davin")
	m, _, err := s.SendNetworkMessage(NetworkMessageInput{ToSessionID: to.ID, Text: "@meiiie this stays addressed to Davin", Wake: true, ClientNonce: "network-message-original"})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	ids := make(chan string, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w, _, e := s.DispatchNetworkMessage(m.ID)
			if e != nil {
				t.Error(e)
				return
			}
			ids <- w.RequestID
		}()
	}
	wg.Wait()
	close(ids)
	id := ""
	for got := range ids {
		if id != "" && got != id {
			t.Fatal("duplicate task")
		}
		id = got
	}
	w, err := s.Get(id)
	if err != nil || w.BotID != "bot-davin" || w.PeerMessageID != m.ID {
		t.Fatal("wrong routed peer task", err)
	}
	if len(s.Snapshot().Work) != 1 {
		t.Fatal("duplicate work accepted")
	}
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal("replay", err)
	}
	defer s.Close()
	got, dup, err := s.DispatchNetworkMessage(m.ID)
	if err != nil || !dup || got.RequestID != id {
		t.Fatal("restart duplicated dispatched work", err)
	}
	if _, _, err := s.Accept(Input{BotID: "bot-davin", Text: "fake peer origin", ClientNonce: "network-spoof-nonce", Lane: "foreground"}); !errors.Is(err, ErrInvalid) {
		t.Fatal("owner reserved nonce admitted", err)
	}
}

func TestNetworkFYIDoesNotCreateWorkAndContextSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	to := networkSession(t, s, "bot-davin", "conversation-bot-davin", "session-fyi-davin")
	m, _, err := s.SendNetworkMessage(NetworkMessageInput{ToSessionID: to.ID, Text: "FYI canary", ClientNonce: "fyi-original-message"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.DispatchNetworkMessage(m.ID); !errors.Is(err, ErrConflict) {
		t.Fatal("FYI dispatched autonomous work", err)
	}
	if len(s.Snapshot().Work) != 0 {
		t.Fatal("FYI woke a task")
	}
	w, _, err := s.Accept(Input{BotID: "bot-davin", Text: "A real owner turn", ClientNonce: "owner-followup-turn", Lane: "foreground"})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.DeliverNetworkContext(w.RequestID, []string{m.ID}); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	context, err := s.NetworkContext(w.RequestID)
	if err != nil || len(context) != 1 || context[0].ID != m.ID {
		t.Fatal("bound FYI context lost", err)
	}
}

func TestNetworkNonceAndAttachmentBoundary(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	to := networkSession(t, s, "bot-davin", "conversation-bot-davin", "session-limits")
	in := NetworkMessageInput{ToSessionID: to.ID, Text: "Original", ClientNonce: "message-payload-nonce"}
	m, _, err := s.SendNetworkMessage(in)
	if err != nil {
		t.Fatal(err)
	}
	got, dup, err := s.SendNetworkMessage(in)
	if err != nil || !dup || got.ID != m.ID {
		t.Fatal("retry failed", err)
	}
	in.Text = "Changed"
	if _, _, err = s.SendNetworkMessage(in); !errors.Is(err, ErrConflict) {
		t.Fatal("changed retry accepted", err)
	}
	for _, name := range []string{"../secret.txt", ".env", "C:\\secret.txt", "a/b.txt", "a\\b.txt"} {
		in.ClientNonce = "bad-attachment-" + strings.Repeat("x", len(name))
		in.Attachments = []NetworkAttachment{{Name: name, Content: "fixture"}}
		if _, _, err = s.SendNetworkMessage(in); !errors.Is(err, ErrInvalid) {
			t.Fatal("unsafe attachment accepted", name, err)
		}
	}
	in.ClientNonce = "oversized-attachment"
	in.Attachments = []NetworkAttachment{{Name: "note.txt", Content: strings.Repeat("a", 32769)}}
	if _, _, err = s.SendNetworkMessage(in); !errors.Is(err, ErrInvalid) {
		t.Fatal("oversize attachment accepted", err)
	}
}

func TestNetworkRejectsPrivateCrossBotAndStaleSourceAttempt(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	to := networkSession(t, s, "bot-davin", "conversation-bot-davin", "session-private-davin")
	w, _, err := s.Accept(Input{BotID: "bot-meiiie", Text: "Private work", ClientNonce: "private-source-work", Lane: "foreground"})
	if err != nil {
		t.Fatal(err)
	}
	w, err = s.DirectStart(w.RequestID, w.Epoch, DirectRun{Runtime: "direct-http", Model: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	scope := NetworkReadScope{SourceWorkID: w.RequestID, SourceEpoch: w.Epoch, SourceAttempt: NetworkAttempt(w)}
	if _, _, err = s.SendNetworkMessage(NetworkMessageInput{ToSessionID: to.ID, Text: "private leak", ClientNonce: "cross-private-canary", Scope: scope}); !errors.Is(err, ErrForbidden) {
		t.Fatal("private source authorized another private bot", err)
	}
	c := sharedChannel(t, s, "Network shared", "network-shared-channel")
	to = networkSession(t, s, "bot-davin", c.ID, "session-shared-davin")
	w, _, err = s.AcceptProduct(Input{BotID: "bot-meiiie", Text: "Shared work", ClientNonce: "shared-source-work", Lane: "foreground"}, SubmissionScope{ChannelID: c.ID, ExpectedChannelRevision: &c.Revision})
	if err != nil {
		t.Fatal(err)
	}
	w, err = s.DirectStart(w.RequestID, w.Epoch, DirectRun{Runtime: "direct-http", Model: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	scope = NetworkReadScope{SourceWorkID: w.RequestID, SourceEpoch: w.Epoch, SourceAttempt: NetworkAttempt(w)}
	if _, _, err = s.SendNetworkMessage(NetworkMessageInput{ToSessionID: to.ID, Text: "Shared allowed", ClientNonce: "shared-send-original", Scope: scope}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DirectResume(w.RequestID); err != nil {
		t.Fatal(err)
	}
	w, err = s.DirectStart(w.RequestID, w.Epoch, DirectRun{Runtime: "direct-http", Model: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.SendNetworkMessage(NetworkMessageInput{ToSessionID: to.ID, Text: "stale generation", ClientNonce: "shared-send-stale", Scope: scope}); !errors.Is(err, ErrForbidden) {
		t.Fatal("stale source generation accepted", err)
	}
}

func TestNetworkReplayRejectsForgedOrigin(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	to := networkSession(t, s, "bot-davin", "conversation-bot-davin", "session-replay")
	_, _, err = s.SendNetworkMessage(NetworkMessageInput{ToSessionID: to.ID, Text: "original", ClientNonce: "message-replay"})
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	path := filepath.Join(dir, "transactions.jsonl")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	previous := ""
	for i, line := range lines {
		var r record
		if err = json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatal(err)
		}
		if r.Event.Network != nil && r.Event.Network.Message != nil {
			r.Event.Network.Message.Origin = "agent"
		}
		r.Previous = previous
		r.Digest, err = digestRecord(r)
		if err != nil {
			t.Fatal(err)
		}
		previous = r.Digest
		b, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		lines[i] = string(b)
	}
	if err = os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if reopened, e := Open(dir); e == nil {
		reopened.Close()
		t.Fatal("forged typed origin replayed")
	} else if !errors.Is(e, ErrCorrupt) {
		t.Fatal(e)
	}
}

func TestNetworkHumanWakeCopiesFYIWithoutMutatingIt(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	to := networkSession(t, s, "bot-davin", "conversation-bot-davin", "session-wake-copy")
	original, _, err := s.SendNetworkMessage(NetworkMessageInput{ToSessionID: to.ID, Text: "Own FYI", ClientNonce: "wake-copy-original"})
	if err != nil {
		t.Fatal(err)
	}
	wake, dup, err := s.WakeNetworkMessage(original.ID, "wake-copy-action", NetworkReadScope{})
	if err != nil || dup || !wake.Wake || wake.ID == original.ID || wake.ReplyTo != original.ID {
		t.Fatal("explicit wake failed", err)
	}
	view, err := s.NetworkSnapshot(NetworkReadScope{})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range view.Messages {
		if m.ID == original.ID && m.Wake {
			t.Fatal("original FYI mutated")
		}
	}
	first, _, err := s.DispatchNetworkMessage(wake.ID)
	if err != nil {
		t.Fatal(err)
	}
	retry, dup, err := s.WakeNetworkMessage(original.ID, "wake-copy-action", NetworkReadScope{})
	if err != nil || !dup || retry.ID != wake.ID {
		t.Fatal("wake retry duplicated", err)
	}
	again, dup, err := s.DispatchNetworkMessage(retry.ID)
	if err != nil || !dup || again.RequestID != first.RequestID || len(s.Snapshot().Work) != 1 {
		t.Fatal("wake created second Work", err)
	}
}
