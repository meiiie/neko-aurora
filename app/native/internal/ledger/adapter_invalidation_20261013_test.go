package ledger

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestAdapterIncrementalFileRevocationClearsConsumedReply(t *testing.T) {
	s, c, p := productFixture(t)
	a := adapterFixture(t, s, c, "adapter-delta-revoke-session")
	f, _, err := s.UploadFile(UploadInput{ProjectID: p.ID, ProjectRevision: p.Revision, ChannelID: c.ID, Filename: "delta-source.txt", Content: []byte("Owned local fixture source"), ClientNonce: "adapter-delta-file-upload"})
	if err != nil {
		t.Fatal(err)
	}
	w, _, err := s.AcceptAdapter(a.ID, AdapterMessageInput{Input: Input{BotID: "bot-meiiie", ClientNonce: "adapter-delta-message", Text: "Read the selected fixture source", Lane: "foreground"}, SourceMessageID: "adapter-delta-source-message", ProjectID: p.ID, ProjectRevision: p.Revision, FileIDs: []string{f.ID}})
	if err != nil {
		t.Fatal(err)
	}
	w = completeChannelWork(t, s, w, "Owned derived fixture reply")
	page, err := s.AdapterReplies(a.ID, 0)
	if err != nil || len(page.Replies) != 1 || page.Replies[0].Text != w.Reply {
		t.Fatal("fixture did not deliver the completed adapter reply", err)
	}
	if err = s.SetFileAccess(FileAccessInput{FileID: f.ID, ExpectedRevision: f.AccessRevision, Granted: false, ClientNonce: "adapter-delta-file-revoke"}); err != nil {
		t.Fatal(err)
	}
	delta, err := s.AdapterReplies(a.ID, page.Cursor)
	if err != nil {
		t.Fatal(err)
	}
	if len(delta.Replies) != 1 || delta.Replies[0].RequestID != w.RequestID || delta.Replies[0].ReplyID != page.Replies[0].ReplyID || delta.Replies[0].SourceMessageID != page.Replies[0].SourceMessageID || delta.Replies[0].BotID != page.Replies[0].BotID || delta.Replies[0].Sequence <= page.Cursor || delta.Replies[0].Sequence > delta.Cursor || delta.Replies[0].Text != "" || len(delta.Replies[0].Artifacts) != 0 {
		t.Fatalf("consumed adapter reply missing a newer identity-preserving clear: replies=%d oldCursor=%d newCursor=%d", len(delta.Replies), page.Cursor, delta.Cursor)
	}
}

func adapterInvalidationFixture20261013(t *testing.T) (*Store, Channel, Project, AdapterSession, UploadedFile, Work, Work, AdapterMessageInput) {
	t.Helper()
	s, c, p := productFixture(t)
	a := adapterFixture(t, s, c, "adapter-invalidation-session")
	f, _, err := s.UploadFile(UploadInput{ProjectID: p.ID, ProjectRevision: p.Revision, ChannelID: c.ID, Filename: "invalidation-source.txt", Content: []byte("Owned fixture source"), ClientNonce: "adapter-invalidation-file"})
	if err != nil {
		t.Fatal(err)
	}
	source, _, err := s.AcceptProduct(Input{BotID: "bot-meiiie", ClientNonce: "adapter-invalidation-history", Text: "Prior scoped fixture", Lane: "foreground"}, SubmissionScope{ChannelID: c.ID, ProjectID: p.ID, ProjectRevision: p.Revision})
	if err != nil {
		t.Fatal(err)
	}
	source = completeChannelWork(t, s, source, "Owned prior fixture reply")
	in := AdapterMessageInput{Input: Input{BotID: "bot-meiiie", ClientNonce: "adapter-invalidation-target", Text: "Read selected fixture source", Lane: "foreground"}, SourceMessageID: "invalidation-target-message", ProjectID: p.ID, ProjectRevision: p.Revision, FileIDs: []string{f.ID}}
	w, _, err := s.AcceptAdapter(a.ID, in)
	if err != nil || len(w.Context.Sources) != 1 {
		t.Fatal("missing scoped history fixture", err)
	}
	w = startPermissionWork(t, s, w)
	if _, err = s.ExecuteTool(w.RequestID, w.Epoch, invocation("Owned derived fixture artifact")); err != nil {
		t.Fatal(err)
	}
	if err = s.AppendText(w.RequestID, w.Epoch, "Owned derived fixture reply"); err != nil {
		t.Fatal(err)
	}
	if err = s.Finish(w.RequestID, w.Epoch, "completed", ""); err != nil {
		t.Fatal(err)
	}
	w, _ = s.Get(w.RequestID)
	return s, c, p, a, f, source, w, in
}

func TestAdapterInvalidationFences20261013(t *testing.T) {
	for _, kind := range []string{"file", "history_access", "own_access", "plugin", "continuity", "project"} {
		t.Run(kind, func(t *testing.T) {
			s, c, p, a, f, source, w, in := adapterInvalidationFixture20261013(t)
			page, err := s.AdapterReplies(a.ID, 0)
			if err != nil || len(page.Replies) != 1 || len(page.Replies[0].Artifacts) != 1 {
				t.Fatal("missing consumed fixture artifact", err)
			}
			switch kind {
			case "file":
				err = s.SetFileAccess(FileAccessInput{FileID: f.ID, Granted: false, ClientNonce: "adapter-invalidation-revoke-file"})
			case "history_access":
				_, _, err = s.SetSourceAccess(AccessInput{RequestID: source.RequestID, Granted: false, ClientNonce: "adapter-invalidation-revoke-history"})
			case "own_access":
				_, _, err = s.SetSourceAccess(AccessInput{RequestID: w.RequestID, Granted: false, ClientNonce: "adapter-invalidation-revoke-own"})
			case "plugin":
				_, _, err = s.ControlPlugin(PluginInput{Version: 1, ID: "neko-project-sources", Action: "disable", ClientNonce: "adapter-invalidation-disable-reader"})
			case "continuity":
				_, _, err = s.SaveProjectContinuity(ProjectContinuityInput{ProjectID: p.ID, ChannelID: c.ID, ExpectedProjectRevision: p.Revision, ExpectedChannelRevision: c.Revision, ClientNonce: "adapter-invalidation-new-notes", Instructions: "Owned new scope instructions"})
			case "project":
				_, _, err = s.SaveProject(ProjectInput{ID: p.ID, Name: p.Name + " revised", Goal: p.Goal, ChannelIDs: p.ChannelIDs, ExpectedRevision: p.Revision, ClientNonce: "adapter-invalidation-project-update"})
			}
			if err != nil {
				t.Fatal(err)
			}
			fence := s.Snapshot().Sequence
			delta, err := s.AdapterReplies(a.ID, page.Cursor)
			if err != nil || len(delta.Replies) != 1 {
				t.Fatal("missing dependent clear", kind, err, len(delta.Replies))
			}
			clear := delta.Replies[0]
			if clear.RequestID != w.RequestID || clear.ReplyID != page.Replies[0].ReplyID || clear.SourceMessageID != in.SourceMessageID || clear.BotID != w.BotID || clear.Text != "" || len(clear.Artifacts) != 0 || clear.Sequence != fence || clear.Sequence <= page.Cursor || clear.Sequence > delta.Cursor {
				t.Fatal("clear changed identity or lacked causal sequence", kind)
			}
			before := s.Snapshot().Sequence
			retry, err := s.AdapterReplies(a.ID, page.Cursor)
			if err != nil || !reflect.DeepEqual(delta, retry) || s.Snapshot().Sequence != before {
				t.Fatal("clear retry mutated or changed projection", err)
			}
			ack, duplicate, err := s.AcceptAdapter(a.ID, in)
			if err != nil || !duplicate || ack.RequestID != w.RequestID || !reflect.DeepEqual(ack.Adapter, w.Adapter) || !reflect.DeepEqual(ack.Resources, w.Resources) {
				t.Fatal("authority change replaced original ACK", err)
			}
			empty, err := s.AdapterReplies(a.ID, delta.Cursor)
			if err != nil || len(empty.Replies) != 0 {
				t.Fatal("clear repeated after consumed cursor", err)
			}
		})
	}
}

func TestAdapterInvalidationRegrantRestartAndUnrelatedEvents20261013(t *testing.T) {
	s, _, _, a, f, _, w, _ := adapterInvalidationFixture20261013(t)
	page, _ := s.AdapterReplies(a.ID, 0)
	if _, _, err := s.SaveChannel(ChannelInput{Name: "Unrelated authority", BotIDs: []string{"bot-meiiie"}, ClientNonce: "adapter-invalidation-unrelated-channel"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetFileAccess(FileAccessInput{FileID: f.ID, Granted: false, ClientNonce: "adapter-invalidation-file-denial"}); err != nil {
		t.Fatal(err)
	}
	fence := s.Snapshot().Sequence
	delta, err := s.AdapterReplies(a.ID, page.Cursor)
	if err != nil || len(delta.Replies) != 1 || delta.Replies[0].Sequence != fence {
		t.Fatal("unrelated authority chosen as cause", err)
	}
	if err = s.SetFileAccess(FileAccessInput{FileID: f.ID, ExpectedRevision: 1, Granted: true, ClientNonce: "adapter-invalidation-explicit-file-regrant"}); err != nil {
		t.Fatal(err)
	}
	next, err := s.AdapterReplies(a.ID, delta.Cursor)
	if err != nil || len(next.Replies) != 1 || next.Replies[0].Sequence <= delta.Cursor || next.Replies[0].Text != "" || len(next.Replies[0].Artifacts) != 0 {
		t.Fatal("new regrant revision resurrected stale result or lacked current fence", err)
	}
	dir := s.dir
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	replayed, err := s.AdapterReplies(a.ID, page.Cursor)
	if err != nil || len(replayed.Replies) != 1 || replayed.Replies[0].Sequence <= fence || replayed.Replies[0].Text != "" || len(replayed.Replies[0].Artifacts) != 0 || replayed.Replies[0].RequestID != w.RequestID {
		t.Fatal("restart lost immutable clear", err)
	}
	b := adapterFixture(t, s, mustAdapterChannel20261013(t, s, a.ChannelID), "adapter-invalidation-other-session")
	other, err := s.AdapterReplies(b.ID, 0)
	if err != nil || len(other.Replies) != 0 {
		t.Fatal("another session received the clear", err)
	}
}

func mustAdapterChannel20261013(t *testing.T, s *Store, id string) Channel {
	t.Helper()
	c, ok := s.channel(id)
	if !ok {
		t.Fatal("missing fixture channel")
	}
	return c
}

func TestAdapterInvalidationPublishedReportSource20261013(t *testing.T) {
	for _, kind := range []string{"source_access", "source_bot_state", "source_archive_preserves_history"} {
		t.Run(kind, func(t *testing.T) {
			s, c, source := reportFixture(t, true)
			if _, _, err := s.DeliverReport(reportSubmission(c, "adapter-invalidation-published-report")); err != nil {
				t.Fatal(err)
			}
			a := adapterFixture(t, s, c, "adapter-invalidation-report-session")
			w, _, err := s.AcceptAdapter(a.ID, AdapterMessageInput{Input: Input{BotID: "bot-davin", ClientNonce: "adapter-invalidation-report-target", Text: "Use the published fixture report", Lane: "foreground"}, SourceMessageID: "report-target-message"})
			if err != nil || len(w.Context.PublishedReports) != 1 {
				t.Fatal("missing published report pin", err)
			}
			w = completeChannelWork(t, s, w, "Owned report-derived fixture reply")
			page, _ := s.AdapterReplies(a.ID, 0)
			switch kind {
			case "source_access":
				_, _, err = s.SetSourceAccess(AccessInput{RequestID: source.RequestID, Granted: false, ClientNonce: "adapter-invalidation-report-source-revoke"})
			case "source_bot_state":
				_, _, err = s.ControlBot(BotControlInput{BotID: source.BotID, Action: "archive", ClientNonce: "adapter-invalidation-report-bot-archive"})
			case "source_archive_preserves_history":
				_, _, err = s.SetConversationArchive(source.ConversationID, ConversationArchiveInput{Archived: true, ExpectedRevision: 0, ClientNonce: "adapter-invalidation-source-conversation-archive"})
			}
			if err != nil {
				t.Fatal(err)
			}
			delta, err := s.AdapterReplies(a.ID, page.Cursor)
			if kind == "source_archive_preserves_history" {
				if err != nil || len(delta.Replies) != 0 || !s.contextAllowed(w) {
					t.Fatal("archive invented historical revoke", err)
				}
			} else if err != nil || len(delta.Replies) != 1 || delta.Replies[0].RequestID != w.RequestID || delta.Replies[0].Text != "" || len(delta.Replies[0].Artifacts) != 0 || delta.Replies[0].Sequence <= page.Cursor {
				t.Fatal("published report dependency missing clear", err)
			}
		})
	}
}

func TestAdapterInvalidationUnknownFenceNoInventedSequence20261013(t *testing.T) {
	s, _, _, a, f, _, _, _ := adapterInvalidationFixture20261013(t)
	page, _ := s.AdapterReplies(a.ID, 0)
	f.Granted = false
	s.uploads[f.ID] = f
	if _, err := s.AdapterReplies(a.ID, page.Cursor); !errors.Is(err, ErrConflict) {
		t.Fatal("unrecorded authority change minted or skipped a clear", err)
	}
}

func TestAdapterInvalidationFanoutLimit20261013(t *testing.T) {
	s, c, p := productFixture(t)
	if _, _, err := s.SetBotProfile("bot-meiiie", BotProfileInput{IncludeHistory: false, ClientNonce: "adapter-invalidation-fanout-history-off"}); err != nil {
		t.Fatal(err)
	}
	a := adapterFixture(t, s, c, "adapter-invalidation-fanout-session")
	f, _, err := s.UploadFile(UploadInput{ProjectID: p.ID, ProjectRevision: p.Revision, ChannelID: c.ID, Filename: "fanout.txt", Content: []byte("Owned fixture source"), ClientNonce: "adapter-invalidation-fanout-file"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 513; i++ {
		w, _, err := s.AcceptAdapter(a.ID, AdapterMessageInput{Input: Input{BotID: "bot-meiiie", ClientNonce: fmt.Sprintf("adapter-fanout-%03d", i), Text: "Read the selected fixture", Lane: "foreground"}, SourceMessageID: fmt.Sprintf("fanout-source-%03d", i), ProjectID: p.ID, ProjectRevision: p.Revision, FileIDs: []string{f.ID}})
		if err != nil {
			t.Fatal(err)
		}
		completeChannelWork(t, s, w, "Owned fixture output")
	}
	after := s.Snapshot().Sequence
	if err = s.SetFileAccess(FileAccessInput{FileID: f.ID, Granted: false, ClientNonce: "adapter-invalidation-fanout-revoke"}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AdapterReplies(a.ID, after); !errors.Is(err, ErrConflict) {
		t.Fatal("oversized whole-event clear fanout skipped or escaped client row bound", err)
	}
	before := s.Snapshot().Sequence
	dir := s.dir
	s.Close()
	s, err = Open(dir)
	if err != nil || s.Snapshot().Sequence != before+512 {
		t.Fatal("startup migration did not obey 512-checkpoint bound", err)
	}
	if _, err = s.AdapterReplies(a.ID, after); !errors.Is(err, ErrConflict) {
		t.Fatal("partial migration acknowledged unresolved pre-open output", err)
	}
	s.Close()
	s, err = Open(dir)
	if err != nil || s.Snapshot().Sequence != before+513 {
		t.Fatal("second startup lost prior checkpoints or repeated repaired keys", err)
	}
	defer s.Close()
	if _, err = s.AdapterReplies(a.ID, after); !errors.Is(err, ErrConflict) {
		t.Fatal("fully migrated fanout escaped wire row limit", err)
	}
}

func TestAdapterInvalidationByteAndEventLimit20261013(t *testing.T) {
	for _, kind := range []string{"bytes", "events"} {
		t.Run(kind, func(t *testing.T) {
			s, c, _, a, f, _, _, _ := adapterInvalidationFixture20261013(t)
			if _, _, err := s.SetBotProfile("bot-meiiie", BotProfileInput{IncludeHistory: false, ClientNonce: "adapter-invalidation-backlog-history-off"}); err != nil {
				t.Fatal(err)
			}
			page, _ := s.AdapterReplies(a.ID, 0)
			count, body := 14, strings.Repeat("x", 65536)
			if kind == "events" {
				count = 2048
			}
			for i := 0; i < count; i++ {
				if kind == "events" {
					w, _, err := s.Accept(Input{BotID: "bot-meiiie", ClientNonce: fmt.Sprintf("adapter-hardcap-unrelated-%04d", i), Text: "Owned unrelated fixture", Lane: "background"})
					if err != nil {
						t.Fatal(err)
					}
					if _, err = s.Cancel(w.RequestID); err != nil {
						t.Fatal(err)
					}
					continue
				}
				w, _, err := s.AcceptAdapter(a.ID, AdapterMessageInput{Input: Input{BotID: "bot-meiiie", ClientNonce: fmt.Sprintf("adapter-backlog-%03d", i), Text: "Independent fixture", Lane: "foreground"}, SourceMessageID: fmt.Sprintf("backlog-source-%03d", i)})
				if err != nil {
					t.Fatal(err)
				}
				completeChannelWork(t, s, w, body)
			}
			if err := s.SetFileAccess(FileAccessInput{FileID: f.ID, Granted: false, ClientNonce: "adapter-invalidation-backlog-revoke"}); err != nil {
				t.Fatal(err)
			}
			before := s.Snapshot().Sequence
			reads := 1
			if kind == "events" {
				reads = 100
			}
			start := time.Now()
			for i := 0; i < reads; i++ {
				if _, err := s.AdapterReplies(a.ID, page.Cursor); !errors.Is(err, ErrConflict) {
					t.Fatal("pending clear exceeded bounded page or advanced without clearing", kind, c.ID, err)
				}
			}
			elapsed := time.Since(start)
			if s.Snapshot().Sequence != before {
				t.Fatal("conflict read mutated journal")
			}
			if kind == "events" {
				t.Logf("bounded conflict projection cost: %s average across %d reads, %d journal events; snapshot excluded", elapsed/time.Duration(reads), reads, before)
				boundary, err := s.AdapterReplies(a.ID, page.Cursor+1)
				if err != nil || boundary.Cursor != before || len(boundary.Replies) != 1 || boundary.Replies[0].Sequence != before || boundary.Replies[0].Text != "" || len(boundary.Replies[0].Artifacts) != 0 {
					t.Fatal("exact 4096-event reconciliation cap rejected safe projection", err)
				}
			}
		})
	}
}

func TestAdapterInvalidationExactPageBoundary20261013(t *testing.T) {
	for _, distance := range []int{200, 201} {
		t.Run(fmt.Sprintf("distance_%d", distance), func(t *testing.T) {
			s, _, _, a, f, _, _, _ := adapterInvalidationFixture20261013(t)
			page, err := s.AdapterReplies(a.ID, 0)
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < (distance-1)/2; i++ {
				w, _, err := s.Accept(Input{BotID: "bot-meiiie", ClientNonce: fmt.Sprintf("adapter-boundary-unrelated-%03d", i), Text: "Owned unrelated fixture", Lane: "background"})
				if err != nil {
					t.Fatal(err)
				}
				if _, err = s.Cancel(w.RequestID); err != nil {
					t.Fatal(err)
				}
			}
			if distance == 200 {
				if _, _, err = s.SaveChannel(ChannelInput{Name: "Boundary unrelated fixture", BotIDs: []string{"bot-meiiie"}, ClientNonce: "adapter-boundary-unrelated-channel"}); err != nil {
					t.Fatal(err)
				}
			}
			if err = s.SetFileAccess(FileAccessInput{FileID: f.ID, Granted: false, ClientNonce: "adapter-boundary-file-revoke"}); err != nil {
				t.Fatal(err)
			}
			fence := s.Snapshot().Sequence
			if fence-page.Cursor != uint64(distance) {
				t.Fatal("fixture did not land at exact event boundary")
			}
			check := func() {
				t.Helper()
				delta, err := s.AdapterReplies(a.ID, page.Cursor)
				if err != nil || delta.Cursor != fence || len(delta.Replies) != 1 || delta.Replies[0].Sequence != fence || delta.Replies[0].ReplyID != page.Replies[0].ReplyID || delta.Replies[0].Text != "" || len(delta.Replies[0].Artifacts) != 0 {
					t.Fatal("last-event clear missing from atomic bounded page", err)
				}
				next, err := s.AdapterReplies(a.ID, delta.Cursor)
				if err != nil || next.Cursor != fence || len(next.Replies) != 0 {
					t.Fatal("next-page cursor repeated or lost clear", err)
				}
			}
			check()
			check()
			dir := s.dir
			s.Close()
			s, err = Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			fence = s.Snapshot().Sequence
			check()
		})
	}
}

func TestAdapterInvalidationReconciliationRetainsInterveningUpdates20261013(t *testing.T) {
	s, _, _, a, f, _, original, _ := adapterInvalidationFixture20261013(t)
	page, err := s.AdapterReplies(a.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.SetBotProfile("bot-meiiie", BotProfileInput{IncludeHistory: false, ClientNonce: "adapter-reconcile-history-off"}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		w, _, err := s.Accept(Input{BotID: "bot-meiiie", ClientNonce: fmt.Sprintf("adapter-reconcile-unrelated-%03d", i), Text: "Owned unrelated fixture", Lane: "background"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.Cancel(w.RequestID); err != nil {
			t.Fatal(err)
		}
	}
	independent, _, err := s.AcceptAdapter(a.ID, AdapterMessageInput{Input: Input{BotID: "bot-meiiie", ClientNonce: "adapter-reconcile-independent", Text: "Independent fixture", Lane: "foreground"}, SourceMessageID: "reconcile-independent-message"})
	if err != nil {
		t.Fatal(err)
	}
	independent = completeChannelWork(t, s, independent, "Owned independent fixture reply")
	if err = s.SetFileAccess(FileAccessInput{FileID: f.ID, Granted: false, ClientNonce: "adapter-reconcile-revoke"}); err != nil {
		t.Fatal(err)
	}
	fence := s.Snapshot().Sequence
	delta, err := s.AdapterReplies(a.ID, page.Cursor)
	if err != nil || delta.Cursor != fence || len(delta.Replies) != 2 || fence-page.Cursor <= 200 {
		t.Fatal("bounded reconciliation did not include intervening session update", err)
	}
	for _, reply := range delta.Replies {
		if reply.RequestID == original.RequestID {
			if reply.Sequence != fence || reply.Text != "" || len(reply.Artifacts) != 0 {
				t.Fatal("reconciliation lost durable clear")
			}
		} else if reply.RequestID != independent.RequestID || reply.Text != independent.Reply || reply.Sequence <= page.Cursor || reply.Sequence >= fence {
			t.Fatal("reconciliation skipped, retargeted or replaced independent reply")
		}
	}
	nextWork, _, err := s.AcceptAdapter(a.ID, AdapterMessageInput{Input: Input{BotID: "bot-meiiie", ClientNonce: "adapter-reconcile-later", Text: "Later fixture", Lane: "foreground"}, SourceMessageID: "reconcile-later-message"})
	if err != nil {
		t.Fatal(err)
	}
	nextWork = completeChannelWork(t, s, nextWork, "Owned later fixture reply")
	next, err := s.AdapterReplies(a.ID, delta.Cursor)
	if err != nil || len(next.Replies) != 1 || next.Replies[0].RequestID != nextWork.RequestID || next.Replies[0].Text != nextWork.Reply || next.Replies[0].Sequence <= delta.Cursor {
		t.Fatal("reconciliation blocked or skipped the next valid update", err)
	}
	dir := s.dir
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	replayed, err := s.AdapterReplies(a.ID, page.Cursor)
	if err != nil || len(replayed.Replies) != 3 || replayed.Cursor <= next.Cursor {
		t.Fatal("restart lost reconciliation history or its durable checkpoint", err)
	}
	for _, reply := range replayed.Replies {
		switch reply.RequestID {
		case original.RequestID:
			if reply.Sequence <= next.Cursor || reply.Text != "" || len(reply.Artifacts) != 0 {
				t.Fatal("restart did not preserve greater-sequence clear")
			}
		case independent.RequestID:
			if reply.Text != independent.Reply {
				t.Fatal("restart lost intervening independent reply")
			}
		case nextWork.RequestID:
			if reply.Text != nextWork.Reply {
				t.Fatal("restart lost later independent reply")
			}
		default:
			t.Fatal("restart retargeted a reply")
		}
	}
	replayedNext, err := s.AdapterReplies(a.ID, replayed.Cursor)
	if err != nil || len(replayedNext.Replies) != 0 || replayedNext.Cursor != replayed.Cursor {
		t.Fatal("restart blocked next poll after durable checkpoint", err)
	}
}

func TestAdapterInvalidationFanoutTieOrder20261013(t *testing.T) {
	s, c, p := productFixture(t)
	if _, _, err := s.SetBotProfile("bot-meiiie", BotProfileInput{IncludeHistory: false, ClientNonce: "adapter-order-history-off"}); err != nil {
		t.Fatal(err)
	}
	a := adapterFixture(t, s, c, "adapter-order-session")
	f, _, err := s.UploadFile(UploadInput{ProjectID: p.ID, ProjectRevision: p.Revision, ChannelID: c.ID, Filename: "order.txt", Content: []byte("Owned fixture source"), ClientNonce: "adapter-order-file"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 16; i++ {
		w, _, err := s.AcceptAdapter(a.ID, AdapterMessageInput{Input: Input{BotID: "bot-meiiie", ClientNonce: fmt.Sprintf("adapter-order-%03d", i), Text: "Read selected fixture", Lane: "foreground"}, SourceMessageID: fmt.Sprintf("order-source-%03d", i), ProjectID: p.ID, ProjectRevision: p.Revision, FileIDs: []string{f.ID}})
		if err != nil {
			t.Fatal(err)
		}
		completeChannelWork(t, s, w, "Owned fixture reply")
	}
	after := s.Snapshot().Sequence
	if err = s.SetFileAccess(FileAccessInput{FileID: f.ID, Granted: false, ClientNonce: "adapter-order-revoke"}); err != nil {
		t.Fatal(err)
	}
	page, err := s.AdapterReplies(a.ID, after)
	if err != nil || len(page.Replies) != 16 {
		t.Fatal("missing whole-event clear fanout", err)
	}
	for i, reply := range page.Replies {
		if reply.Sequence != page.Cursor || reply.Text != "" || len(reply.Artifacts) != 0 || i > 0 && page.Replies[i-1].ReplyID >= reply.ReplyID {
			t.Fatal("same-sequence clear fanout lacked deterministic reply identity ordering")
		}
	}
	retry, err := s.AdapterReplies(a.ID, after)
	if err != nil || !reflect.DeepEqual(page, retry) {
		t.Fatal("whole-event fanout retry changed order", err)
	}
}

func TestAdapterInvalidationLegacyConsumedAuthorityCursor20261013(t *testing.T) {
	s, _, _, a, f, _, w, in := adapterInvalidationFixture20261013(t)
	page, err := s.AdapterReplies(a.ID, 0)
	if err != nil || len(page.Replies) != 1 || page.Replies[0].Text != w.Reply {
		t.Fatal("missing prior consumed reply", err)
	}
	if err = s.SetFileAccess(FileAccessInput{FileID: f.ID, Granted: false, ClientNonce: "adapter-legacy-file-revoke"}); err != nil {
		t.Fatal(err)
	}
	legacyCursor := s.Snapshot().Sequence
	dir := s.dir
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	delta, err := s.AdapterReplies(a.ID, legacyCursor)
	if err != nil || len(delta.Replies) != 1 || delta.Replies[0].Sequence <= legacyCursor || delta.Replies[0].Sequence > delta.Cursor || delta.Replies[0].ReplyID != page.Replies[0].ReplyID || delta.Replies[0].Text != "" || len(delta.Replies[0].Artifacts) != 0 {
		t.Fatalf("legacy consumed authority cursor retained stale cached reply: oldReplySequence=%d legacyCursor=%d replies=%d error=%v", page.Replies[0].Sequence, legacyCursor, len(delta.Replies), err)
	}
	ack, duplicate, err := s.AcceptAdapter(a.ID, in)
	if err != nil || !duplicate || ack.RequestID != w.RequestID || !reflect.DeepEqual(ack.Adapter, w.Adapter) || !reflect.DeepEqual(ack.Resources, w.Resources) || ack.Reply != w.Reply {
		t.Fatal("legacy reconciliation replaced original durable ACK", err)
	}
	before := s.Snapshot().Sequence
	next, err := s.AdapterReplies(a.ID, delta.Cursor)
	if err != nil || len(next.Replies) != 0 || s.Snapshot().Sequence != before {
		t.Fatal("consumed new clear bricked next poll or mutated journal", err)
	}
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s.Snapshot().Sequence != before {
		t.Fatal("second restart repeated durable repair")
	}
	retry, err := s.AdapterReplies(a.ID, legacyCursor)
	if err != nil || !reflect.DeepEqual(delta, retry) {
		t.Fatal("restart changed legacy repair projection", err)
	}
}

func TestAdapterInvalidationLegacyReadOnlyNoMutation20261013(t *testing.T) {
	s, _, _, a, f, _, _, _ := adapterInvalidationFixture20261013(t)
	if err := s.SetFileAccess(FileAccessInput{FileID: f.ID, Granted: false, ClientNonce: "adapter-legacy-readonly-revoke"}); err != nil {
		t.Fatal(err)
	}
	legacyCursor := s.Snapshot().Sequence
	dir := s.dir
	s.Close()
	s, err := OpenReadOnly(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.AdapterReplies(a.ID, legacyCursor); !errors.Is(err, ErrConflict) || s.Snapshot().Sequence != legacyCursor {
		t.Fatal("read-only legacy history was acknowledged without a clear or mutated", err)
	}
}

func TestAdapterInvalidationCheckpointShapeAndNonceDomain20261013(t *testing.T) {
	s, _, _, _, f, _, w, _ := adapterInvalidationFixture20261013(t)
	if err := s.SetFileAccess(FileAccessInput{FileID: f.ID, Granted: false, ClientNonce: "adapter-checkpoint-shape-revoke"}); err != nil {
		t.Fatal(err)
	}
	causal := s.Snapshot().Sequence
	good := Event{Type: "adapter.reply_clear_checkpoint", Sequence: causal + 1, Work: Work{RequestID: w.RequestID}, Nonce: adapterReplyClearNonce(w, causal)}
	if !s.validAdapterReplyClearEvent(good) {
		t.Fatal("canonical minimal checkpoint rejected")
	}
	for _, kind := range []string{"body", "target", "binding", "nonce", "mixed", "type"} {
		t.Run(kind, func(t *testing.T) {
			e := good
			switch kind {
			case "body":
				e.Work.Reply = "Owned forbidden checkpoint body"
			case "target":
				e.Work.RequestID = "request-not-in-this-journal"
			case "binding":
				e.Work.Adapter = &AdapterPin{AdapterSessionID: "adapter-other-session"}
			case "nonce":
				e.Nonce = "adapter-clear-" + strings.Repeat("0", 64)
			case "mixed":
				e.Channel = &Channel{ID: "channel-other"}
			case "type":
				e.Type = "adapter.reply_clear_unrecognized"
			}
			if s.validAdapterReplyClearEvent(e) {
				t.Fatal("malformed checkpoint accepted", kind)
			}
			if kind != "type" {
				before := uint64(len(s.events))
				if err := s.commit(e); !errors.Is(err, ErrInvalid) || uint64(len(s.events)) != before || s.poisoned {
					t.Fatal("malformed checkpoint commit altered journal", kind, err)
				}
			}
		})
	}
	before := []int{len(s.nonces), len(s.adapterNonces), len(s.productNonces), len(s.pluginNonces), len(s.taskControlNonces), len(s.coreProfileNonces)}
	workCount := len(s.work)
	if err := s.commit(good); err != nil {
		t.Fatal(err)
	}
	after := []int{len(s.nonces), len(s.adapterNonces), len(s.productNonces), len(s.pluginNonces), len(s.taskControlNonces), len(s.coreProfileNonces)}
	if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(w, s.work[w.RequestID]) || len(s.work) != workCount {
		t.Fatal("checkpoint polluted command nonce domain or rewrote durable Work")
	}
	if _, exists := s.work[""]; exists {
		t.Fatal("checkpoint inserted phantom empty Work")
	}
	public, err := s.Events(causal)
	if err != nil || len(public) != 1 || !reflect.DeepEqual(public[0].Work, Work{RequestID: w.RequestID}) || public[0].Type != "adapter.reply_clear_checkpoint" {
		t.Fatal("checkpoint public event invented a full Work, body or pin", err)
	}
	if len(s.Snapshot().Work) != workCount {
		t.Fatal("checkpoint snapshot invented or lost Work")
	}
	good.Sequence++
	if s.validAdapterReplyClearEvent(good) {
		t.Fatal("duplicate checkpoint key accepted")
	}
	if err := s.commit(good); !errors.Is(err, ErrInvalid) {
		t.Fatal("duplicate checkpoint commit accepted", err)
	}
}

func TestAdapterInvalidationCheckpointUnknownCauseSkipsMigration20261013(t *testing.T) {
	s, _, _, a, f, _, _, _ := adapterInvalidationFixture20261013(t)
	f.Granted = false
	s.uploads[f.ID] = f
	before := s.Snapshot().Sequence
	if err := s.recoverAdapterReplyClears(); err != nil || s.Snapshot().Sequence != before || s.poisoned {
		t.Fatal("unknown in-memory authority poisoned or rewrote journal", err)
	}
	if _, err := s.AdapterReplies(a.ID, before); !errors.Is(err, ErrConflict) {
		t.Fatal("unknown cause escaped per-adapter hold", err)
	}
}

func TestAdapterInvalidationCheckpointIOFailureRetainsCommittedRepair20261013(t *testing.T) {
	s, _, p, a, f, _, w, _ := adapterInvalidationFixture20261013(t)
	second, _, err := s.AcceptAdapter(a.ID, AdapterMessageInput{Input: Input{BotID: "bot-meiiie", ClientNonce: "adapter-checkpoint-io-second", Text: "Owned second fixture", Lane: "foreground"}, SourceMessageID: "checkpoint-io-second-message", ProjectID: p.ID, ProjectRevision: p.Revision, FileIDs: []string{f.ID}})
	if err != nil {
		t.Fatal(err)
	}
	completeChannelWork(t, s, second, "Owned second derived fixture reply")
	if err = s.SetFileAccess(FileAccessInput{FileID: f.ID, Granted: false, ClientNonce: "adapter-checkpoint-io-revoke"}); err != nil {
		t.Fatal(err)
	}
	causal := s.Snapshot().Sequence
	nonce := adapterReplyClearNonce(w, causal)
	if err = s.commit(Event{Type: "adapter.reply_clear_checkpoint", Work: Work{RequestID: w.RequestID}, Nonce: nonce}); err != nil {
		t.Fatal(err)
	}
	committed := s.Snapshot().Sequence
	if err = s.file.Close(); err != nil {
		t.Fatal(err)
	}
	if err = s.recoverAdapterReplyClears(); !errors.Is(err, ErrUnavailable) || !s.poisoned || uint64(len(s.events)) != committed {
		t.Fatal("checkpoint write failure changed or misclassified committed journal", err)
	}
	dir := s.dir
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal("durable earlier repair could not replay after write failure", err)
	}
	defer s.Close()
	if s.adapterReplyClearCheckpoints()[nonce] != committed || s.Snapshot().Sequence != committed+1 {
		t.Fatal("startup lost earlier committed repair or repeated its key")
	}
}

func TestAdapterInvalidationPartialOutput20261013(t *testing.T) {
	for _, kind := range []string{"file", "file_commit_before_suspend", "plugin", "history"} {
		t.Run(kind, func(t *testing.T) {
			s, c, p := productFixture(t)
			a := adapterFixture(t, s, c, "adapter-partial-session")
			f, _, err := s.UploadFile(UploadInput{ProjectID: p.ID, ProjectRevision: p.Revision, ChannelID: c.ID, Filename: "partial.txt", Content: []byte("Owned partial fixture source"), ClientNonce: "adapter-partial-file"})
			if err != nil {
				t.Fatal(err)
			}
			source, _, err := s.AcceptProduct(Input{BotID: "bot-meiiie", ClientNonce: "adapter-partial-history", Text: "Owned prior fixture", Lane: "foreground"}, SubmissionScope{ChannelID: c.ID, ProjectID: p.ID, ProjectRevision: p.Revision})
			if err != nil {
				t.Fatal(err)
			}
			completeChannelWork(t, s, source, "Owned prior fixture reply")
			w, _, err := s.AcceptAdapter(a.ID, AdapterMessageInput{Input: Input{BotID: "bot-meiiie", ClientNonce: "adapter-partial-message", Text: "Read selected fixture", Lane: "foreground"}, SourceMessageID: "partial-source-message", ProjectID: p.ID, ProjectRevision: p.Revision, FileIDs: []string{f.ID}})
			if err != nil || len(w.Context.Sources) != 1 {
				t.Fatal("missing scoped partial fixture", err)
			}
			w = startPermissionWork(t, s, w)
			if err = s.AppendText(w.RequestID, w.Epoch, "Owned partially streamed reply"); err != nil {
				t.Fatal(err)
			}
			page, err := s.AdapterReplies(a.ID, 0)
			if err != nil || len(page.Replies) != 1 || page.Replies[0].Text == "" || page.Replies[0].Status != "running" {
				t.Fatal("missing consumed live partial reply", err)
			}
			switch kind {
			case "file":
				err = s.SetFileAccess(FileAccessInput{FileID: f.ID, Granted: false, ClientNonce: "adapter-partial-file-revoke"})
			case "file_commit_before_suspend":
				revoked := s.uploads[f.ID]
				revoked.Granted = false
				revoked.AccessRevision++
				e := Event{Type: "file.access", Upload: &revoked, Nonce: "adapter-partial-commit-only-revoke"}
				if !s.validProductEvent(e) {
					t.Fatal("invalid metadata-only crash-boundary fixture")
				}
				err = s.commit(e)
				if s.work[w.RequestID].Status != "running" {
					t.Fatal("fixture executed later work suspension")
				}
			case "plugin":
				_, _, err = s.ControlPlugin(PluginInput{Version: 1, ID: "neko-project-sources", Action: "disable", ClientNonce: "adapter-partial-plugin-revoke"})
			case "history":
				_, _, err = s.SetSourceAccess(AccessInput{RequestID: source.RequestID, Granted: false, ClientNonce: "adapter-partial-history-revoke"})
			}
			if err != nil {
				t.Fatal(err)
			}
			causal := s.Snapshot().Sequence
			delta, err := s.AdapterReplies(a.ID, page.Cursor)
			if err != nil || len(delta.Replies) != 1 || delta.Replies[0].Sequence != causal || delta.Replies[0].Text != "" || len(delta.Replies[0].Artifacts) != 0 || delta.Replies[0].ReplyID != page.Replies[0].ReplyID {
				t.Fatal("live partial output remained cached after typed authority event", err)
			}
			dir := s.dir
			s.Close()
			s, err = Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			repaired, err := s.AdapterReplies(a.ID, causal)
			if err != nil || len(repaired.Replies) != 1 || repaired.Replies[0].Sequence <= causal || repaired.Replies[0].Text != "" || len(repaired.Replies[0].Artifacts) != 0 || repaired.Replies[0].ReplyID != page.Replies[0].ReplyID {
				t.Fatal("restart lost partial-output durable repair", err)
			}
			next, err := s.AdapterReplies(a.ID, repaired.Cursor)
			if err != nil || len(next.Replies) != 0 {
				t.Fatal("partial-output repair bricked next poll", err)
			}
		})
	}
}

func TestAdapterInvalidationCheckpointReportNonceCollision20261013(t *testing.T) {
	s, c, _ := reportFixture(t, false)
	a := adapterFixture(t, s, c, "adapter-checkpoint-collision-session")
	w, _, err := s.AcceptAdapter(a.ID, AdapterMessageInput{Input: Input{BotID: "bot-davin", ClientNonce: "adapter-checkpoint-collision-message", Text: "Owned independent adapter fixture", Lane: "foreground"}, SourceMessageID: "checkpoint-collision-source-message"})
	if err != nil {
		t.Fatal(err)
	}
	w = completeChannelWork(t, s, w, "Owned adapter fixture reply")
	if _, _, err = s.SetSourceAccess(AccessInput{RequestID: w.RequestID, Granted: false, ClientNonce: "adapter-checkpoint-collision-revoke"}); err != nil {
		t.Fatal(err)
	}
	causal := s.Snapshot().Sequence
	nonce := adapterReplyClearNonce(w, causal)
	delivery, _, err := s.DeliverReport(reportSubmission(c, nonce))
	if err != nil || s.reportDeliveryNonces[nonce] != delivery.ID {
		t.Fatal("valid independent report command could not use hash-shaped nonce", err)
	}
	before := s.Snapshot().Sequence
	if err = s.recoverAdapterReplyClears(); err != nil || s.Snapshot().Sequence != before+1 || s.reportDeliveryNonces[nonce] != delivery.ID || s.adapterReplyClearCheckpoints()[nonce] != before+1 {
		t.Fatal("independent report nonce blocked internal repair or was overwritten", err)
	}
}
