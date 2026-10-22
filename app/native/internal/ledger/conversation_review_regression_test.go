package ledger

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestReviewRetentionProtectsHistoricalRefreshPin(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	source := completedPermissionSource(t, s, "review-historical-source", "historical dependency")
	if err = s.MarkRuntimeStopped(source.RequestID, source.AttemptID, StoppedReceipt{Version: 1, NativeProcessID: 401, SessionID: strings.Repeat("a", 32)}); err != nil {
		t.Fatal(err)
	}
	consumer, _, err := s.Accept(input("review-historical-consumer", "background"))
	if err != nil || len(consumer.Context.Sources) != 1 {
		t.Fatal("setup context", err)
	}
	if _, _, err = s.SetSourceAccess(AccessInput{source.RequestID, 0, false, "review-historical-revoke"}); err != nil {
		t.Fatal(err)
	}
	consumer, _ = s.Get(consumer.RequestID)
	consumer, err = s.RefreshContext(RefreshInput{consumer.RequestID, consumer.Epoch})
	if err != nil {
		t.Fatal(err)
	}
	if len(consumer.Context.Sources) != 0 || len(consumer.ContextAttempts) != 1 {
		t.Fatal("setup refresh did not remove revoked current source")
	}
	if consumer.ContextAttempts[0].PinSHA256 == "" {
		t.Fatal("historical pin not preserved")
	}
	for restart := 0; restart < 2; restart++ {
		s.now = func() time.Time { return time.Now().UTC().Add(400 * 24 * time.Hour) }
		p, err := s.PreviewRetention(source.ConversationID, 30)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range p.Candidates {
			if c.ID == source.RequestID {
				t.Errorf("restart=%d: historical context source incorrectly retention-candidate after refresh; archived pin remains", restart)
			}
		}
		s.Close()
		s, err = Open(dir)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestReviewReplayRejectsFreshIntakeAfterArchive(t *testing.T) {
	for _, private := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "private"}[private], func(t *testing.T) {
			dir := t.TempDir()
			s, err := Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { s.Close() }()
			id := "conversation-bot-meiiie"
			if private {
				c, _, e := s.CreateConversation(ConversationCreateInput{BotID: "bot-meiiie", Title: "Review", ClientNonce: "review-replay-create"})
				if e != nil {
					t.Fatal(e)
				}
				id = c.ID
			}
			in := input("review-replay-original", "foreground")
			w, _, err := s.AcceptProduct(in, SubmissionScope{ChannelID: id})
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err = s.SetConversationArchive(id, ConversationArchiveInput{Archived: true, ExpectedRevision: uint64(map[bool]int{false: 0, true: 1}[private]), ClientNonce: "review-replay-archive"}); err != nil {
				t.Fatal(err)
			}
			in.ClientNonce = "review-replay-fresh"
			if _, _, err = s.AcceptProduct(in, SubmissionScope{ChannelID: id}); !errors.Is(err, ErrConflict) {
				t.Fatal("API-intake control", err)
			}
			fresh := copyWork(w)
			fresh.RequestID = "req-" + strings.Repeat("1", 32)
			fresh.SourceMessageID = "msg-" + strings.Repeat("2", 32)
			fresh.ClientNonce = in.ClientNonce
			fresh.CreatedSequence = uint64(len(s.events) + 1)
			fresh.Publication.SourceMessageID = fresh.SourceMessageID
			fresh.Publication.ReplyThreadID = "thread-" + fresh.SourceMessageID
			fresh.Context = s.captureContext(fresh)
			if err = s.attachRuntimeContext(&fresh); err != nil {
				t.Fatal(err)
			}
			e := Event{Sequence: fresh.CreatedSequence, Type: "work.accepted", Work: fresh}
			if validReplay(s, e) {
				t.Error("replay accepts fresh work.accepted after conversation archive despite live intake conflict")
				if err = s.commit(e); err != nil {
					t.Fatal(err)
				}
				s.Close()
				s, err = Open(dir)
				if err != nil {
					t.Fatal("forged valid-digest record failed reopen", err)
				}
				if _, e := s.Get(fresh.RequestID); e != nil {
					t.Fatal("reopened record missing")
				}
			}
		})
	}
}
