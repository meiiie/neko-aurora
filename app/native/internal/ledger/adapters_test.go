package ledger

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestAdapterCumulativePagesAreByteBoundedAndNeverLoseFinalReplies(t *testing.T) {
	s, c, _ := productFixture(t)
	a := adapterFixture(t, s, c, "register-large-pages")
	final := map[string]string{}
	for i := 0; i < 18; i++ {
		w, _, err := s.AcceptAdapter(a.ID, AdapterMessageInput{Input: Input{"bot-meiiie", fmt.Sprintf("large-page-nonce-%d", i), "answer", "foreground"}, SourceMessageID: fmt.Sprintf("large-message-%d", i)})
		if err != nil {
			t.Fatal(err)
		}
		body := strings.Repeat("x", 65536)
		w = completeChannelWork(t, s, w, body)
		final[w.RequestID] = body
	}
	seen := map[string]string{}
	cursor := uint64(0)
	pages := 0
	for cursor < s.Snapshot().Sequence {
		page, err := s.AdapterReplies(a.ID, cursor)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(page)
		if len(raw) > 900000 || page.Cursor <= cursor {
			t.Fatal("page oversized or cursor stuck", len(raw), page.Cursor, cursor)
		}
		ids := map[string]bool{}
		for _, r := range page.Replies {
			if ids[r.ReplyID] {
				t.Fatal("uncoalesced updates")
			}
			ids[r.ReplyID] = true
			seen[r.RequestID] = r.Text
		}
		cursor = page.Cursor
		pages++
	}
	if pages < 2 || !reflect.DeepEqual(seen, final) {
		t.Fatal("final replies lost", pages, len(seen))
	}
}

func adapterFixture(t *testing.T, s *Store, c Channel, nonce string) AdapterSession {
	t.Helper()
	a, duplicate, err := s.RegisterAdapter(AdapterSessionInput{SourceApp: "wiii", SourceSessionID: "wiii-session-1", SourceThreadID: "wiii-thread-1", ChannelID: c.ID, ExpectedChannelRevision: c.Revision, ClientNonce: nonce})
	if err != nil || duplicate {
		t.Fatal(a, err)
	}
	return a
}

func TestAdapterOriginNonceReplyScopeAndRestart(t *testing.T) {
	s, c, _ := productFixture(t)
	a := adapterFixture(t, s, c, "register-wiii-first")
	b := adapterFixture(t, s, c, "register-wiii-second")
	input := AdapterMessageInput{Input: Input{"bot-meiiie", "source-nonce-1", "@davin Answer in the linked channel", "foreground"}, SourceMessageID: "wiii-message-1"}
	w, duplicate, err := s.AcceptAdapter(a.ID, input)
	if err != nil || duplicate || w.BotID != "bot-davin" || w.SourceBotID != "bot-meiiie" || w.Text != input.Text || w.ClientNonce == input.ClientNonce || w.Adapter.ClientNonce != input.ClientNonce || w.ConversationID != c.ID {
		t.Fatal("origin/route", w, err)
	}
	w = completeChannelWork(t, s, w, "linked reply")
	other, _, err := s.AcceptAdapter(b.ID, input)
	if err != nil || other.RequestID == w.RequestID {
		t.Fatal("adapter nonce namespaces crossed", err)
	}
	completeChannelWork(t, s, other, "other-surface-reply")
	private, _, err := s.Accept(Input{"bot-davin", "private-surface-nonce", "private-source-canary", "foreground"})
	if err != nil {
		t.Fatal(err)
	}
	completeChannelWork(t, s, private, "private-reply-canary")
	page, err := s.AdapterReplies(a.ID, 0)
	if err != nil || len(page.Replies) == 0 || page.ChannelRevision != a.ChannelRevision || page.SourceThreadID != a.SourceThreadID || page.ReturnRoute != a.ReturnRoute {
		t.Fatal(page, err)
	}
	for _, r := range page.Replies {
		if r.RequestID != w.RequestID || r.SourceMessageID != input.SourceMessageID || (r.Text != "" && r.Text != "linked reply") {
			t.Fatal("reply escaped source link", r)
		}
	}
	empty, err := s.AdapterReplies(a.ID, page.Cursor)
	if err != nil || len(empty.Replies) != 0 || empty.Cursor != page.Cursor {
		t.Fatal("cursor repeated updates", empty, err)
	}
	if _, err = s.AdapterReplies(a.ID, page.Cursor+1); !errors.Is(err, ErrConflict) {
		t.Fatal("ahead cursor accepted", err)
	}
	dir := s.dir
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal("durable adapter replay", err)
	}
	defer s.Close()
	copy, dup, err := s.AcceptAdapter(a.ID, input)
	if err != nil || !dup || copy.RequestID != w.RequestID || !reflect.DeepEqual(copy.Adapter, w.Adapter) {
		t.Fatal("lost ACK changed identity", err)
	}
	copy.Adapter.SourceThreadID = "forged"
	unchanged, _ := s.Get(w.RequestID)
	if unchanged.Adapter.SourceThreadID != a.SourceThreadID {
		t.Fatal("mutable returned pin")
	}
	changed := input
	changed.SourceMessageID = "different-message"
	if _, _, err = s.AcceptAdapter(a.ID, changed); !errors.Is(err, ErrConflict) {
		t.Fatal("nonce rebound source message", err)
	}
	changed = input
	changed.ClientNonce = "different-nonce"
	if _, _, err = s.AcceptAdapter(a.ID, changed); !errors.Is(err, ErrConflict) {
		t.Fatal("source message forked request", err)
	}
}

func TestAdapterSourceIdentityRaceAndChannelFence(t *testing.T) {
	s, c, _ := productFixture(t)
	a := adapterFixture(t, s, c, "register-race-adapter")
	var wg sync.WaitGroup
	out := make(chan error, 2)
	for i, bot := range []string{"bot-meiiie", "bot-davin"} {
		wg.Add(1)
		go func(i int, bot string) {
			defer wg.Done()
			_, _, err := s.AcceptAdapter(a.ID, AdapterMessageInput{Input: Input{bot, fmt.Sprintf("concurrent-nonce-%d", i), "Same source identity", "foreground"}, SourceMessageID: "same-message"})
			out <- err
		}(i, bot)
	}
	wg.Wait()
	close(out)
	passed, conflict := 0, 0
	for err := range out {
		if err == nil {
			passed++
		} else if errors.Is(err, ErrConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if passed != 1 || conflict != 1 {
		t.Fatal("two requests created for one origin", passed, conflict)
	}
	var original Work
	for _, w := range s.Snapshot().Work {
		if w.Adapter != nil {
			original = w
		}
	}
	_, _, err := s.SaveChannel(ChannelInput{ID: c.ID, Name: c.Name, BotIDs: []string{"bot-meiiie", "bot-davin"}, ExpectedRevision: c.Revision, ClientNonce: "changed-linked-channel"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AdapterReplies(a.ID, 0); !errors.Is(err, ErrConflict) {
		t.Fatal("old link disclosed after audience change", err)
	}
	if _, _, err = s.AcceptAdapter(a.ID, AdapterMessageInput{Input: Input{"bot-meiiie", "new-stale-message", "new task", "foreground"}, SourceMessageID: "new-message"}); !errors.Is(err, ErrConflict) {
		t.Fatal("old session renewed authority", err)
	}
	bot := original.BotID
	if original.SourceBotID != "" {
		bot = original.SourceBotID
	}
	w, dup, err := s.AcceptAdapter(a.ID, AdapterMessageInput{Input: Input{bot, original.Adapter.ClientNonce, original.Text, original.Lane}, SourceMessageID: original.Adapter.SourceMessageID})
	if err != nil || !dup || w.RequestID != original.RequestID || w.Status != "suspended" {
		t.Fatal("receipt renewed authority or lost identity", w, err)
	}
}

func TestAdapterRepliesSourceRevokeSuppressesHistoricalContent(t *testing.T) {
	s, c, p := productFixture(t)
	a := adapterFixture(t, s, c, "register-resource-adapter")
	f, _, err := s.UploadFile(UploadInput{ProjectID: p.ID, ProjectRevision: p.Revision, ChannelID: c.ID, Filename: "vendor.txt", Content: []byte("Vendor source 120 dollars"), ClientNonce: "adapter-source-file"})
	if err != nil {
		t.Fatal(err)
	}
	w, _, err := s.AcceptAdapter(a.ID, AdapterMessageInput{Input: Input{"bot-meiiie", "adapter-project-nonce", "Read vendor source", "foreground"}, SourceMessageID: "project-message", ProjectID: p.ID, ProjectRevision: p.Revision})
	if err != nil {
		t.Fatal(err)
	}
	completeChannelWork(t, s, w, "source-derived-private-detail")
	if err = s.SetFileAccess(FileAccessInput{FileID: f.ID, ExpectedRevision: f.AccessRevision, Granted: false, ClientNonce: "adapter-file-denial"}); err != nil {
		t.Fatal(err)
	}
	page, err := s.AdapterReplies(a.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range page.Replies {
		if r.Text != "" || len(r.Artifacts) > 0 {
			t.Fatal("replay disclosed source-derived reply after revoke", r)
		}
	}
}

func TestAdapterReplyPagesAdvanceAcrossUnrelatedEvents(t *testing.T) {
	s, c, _ := productFixture(t)
	a := adapterFixture(t, s, c, "register-page-adapter")
	for i := 0; i < 102; i++ {
		w, _, err := s.Accept(Input{"bot-meiiie", fmt.Sprintf("page-unrelated-%03d", i), "unrelated", "background"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.Cancel(w.RequestID); err != nil {
			t.Fatal(err)
		}
	}
	w, _, err := s.AcceptAdapter(a.ID, AdapterMessageInput{Input: Input{"bot-meiiie", "page-linked-message", "linked", "foreground"}, SourceMessageID: "page-message"})
	if err != nil {
		t.Fatal(err)
	}
	page, err := s.AdapterReplies(a.ID, 0)
	if err != nil || page.Cursor != 200 || len(page.Replies) != 0 {
		t.Fatal("bounded page skipped events", page, err)
	}
	next, err := s.AdapterReplies(a.ID, page.Cursor)
	if err != nil || next.Cursor != s.Snapshot().Sequence || len(next.Replies) != 1 || next.Replies[0].RequestID != w.RequestID {
		t.Fatal("later linked reply lost", next, err)
	}
}
