package ledger

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestConversationArchiveDoesNotMoveWorkPinsFilesOrRuntime(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	oldSource := completedPermissionSource(t, s, "session-old-source", "old private history canary")
	old, _, err := s.Accept(input("session-old-background", "background"))
	if err != nil {
		t.Fatal(err)
	}
	c, dup, err := s.CreateConversation(ConversationCreateInput{BotID: "bot-meiiie", Title: "New conversation", ClientNonce: "session-new-create"})
	if err != nil || dup || c.Revision != 1 {
		t.Fatal(c, err)
	}
	fresh, _, err := s.AcceptProduct(input("session-new-request", "foreground"), SubmissionScope{ChannelID: c.ID})
	if err != nil || fresh.ConversationID != c.ID || len(fresh.Context.Sources) != 0 {
		t.Fatal("new history crossed scope", err)
	}
	archived, dup, err := s.SetConversationArchive("conversation-bot-meiiie", ConversationArchiveInput{Archived: true, ExpectedRevision: 0, ClientNonce: "session-old-archive"})
	if err != nil || dup || !archived.Archived {
		t.Fatal(err)
	}
	stored, _ := s.Get(old.RequestID)
	source, _ := s.Get(oldSource.RequestID)
	if !reflect.DeepEqual(stored, old) || !reflect.DeepEqual(source, oldSource) {
		t.Fatal("archive modified work/context")
	}
	before := s.Snapshot().Sequence
	if _, _, err = s.Accept(input("session-archived-new", "foreground")); !errors.Is(err, ErrConflict) || s.Snapshot().Sequence != before {
		t.Fatal("fresh archived intake", err)
	}
	retry, duplicate, err := s.Accept(input("session-old-background", "background"))
	if err != nil || !duplicate || !reflect.DeepEqual(retry, old) {
		t.Fatal("archive blocked original receipt", err)
	}
	if _, _, err = s.SetConversationArchive("conversation-bot-meiiie", ConversationArchiveInput{Archived: false, ExpectedRevision: 1, ClientNonce: "session-old-restore"}); err != nil {
		t.Fatal(err)
	}
	again, _, err := s.Accept(input("session-restored-new", "foreground"))
	if err != nil || len(again.Context.Sources) != 1 || again.Context.Sources[0].RequestID != oldSource.RequestID {
		t.Fatal("restore lost original history", err)
	}
	for i := 0; i < 3; i++ {
		if err = s.Close(); err != nil {
			t.Fatal(err)
		}
		s, err = Open(dir)
		if err != nil {
			t.Fatal("replay", err)
		}
		stored, _ = s.Get(old.RequestID)
		if !reflect.DeepEqual(stored, old) {
			t.Fatal("replay changed old pin")
		}
		got, duplicate, e := s.CreateConversation(ConversationCreateInput{BotID: "bot-meiiie", Title: "New conversation", ClientNonce: "session-new-create"})
		if e != nil || !duplicate || got != c {
			t.Fatal("create replay receipt", e)
		}
	}
	s.Close()
}

func TestConversationArchiveRestoreAndRetentionKeepPhysicalFiles(t *testing.T) {
	dir := t.TempDir()
	s, w := running(t, dir)
	if _, err := s.ExecuteTool(w.RequestID, w.Epoch, invocation("session physical report canary")); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendText(w.RequestID, w.Epoch, "File completed"); err != nil {
		t.Fatal(err)
	}
	if err := s.Finish(w.RequestID, w.Epoch, "completed", ""); err != nil {
		t.Fatal(err)
	}
	w, _ = s.Get(w.RequestID)
	artifact := w.Artifacts[0]
	path := filepath.Join(dir, "artifacts", artifact.ID)
	before, _ := os.Stat(path)
	c, _, err := s.CreateConversation(ConversationCreateInput{BotID: "bot-meiiie", Title: "Fresh with no old files", ClientNonce: "file-session-create"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.SetConversationArchive(w.ConversationID, ConversationArchiveInput{Archived: true, ClientNonce: "file-session-archive"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.AcceptProduct(input("file-new-session-request", "foreground"), SubmissionScope{ChannelID: c.ID}); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.SetRetentionPreference(w.ConversationID, RetentionInput{Days: 30, ClientNonce: "file-preview-preference"}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.PreviewRetention(w.ConversationID, 30); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.SetConversationArchive(w.ConversationID, ConversationArchiveInput{Archived: false, ExpectedRevision: 1, ClientNonce: "file-session-restore"}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		stored, _ := s.Get(w.RequestID)
		if !reflect.DeepEqual(stored, w) {
			t.Fatal("file task changed")
		}
		a, bytes, e := s.ReadChannelArtifact(w.ConversationID, artifact.ID)
		if e != nil || string(bytes) != "session physical report canary" || a != artifact {
			t.Fatal("original download lost", e)
		}
		if _, _, e = s.ReadChannelArtifact(c.ID, artifact.ID); !errors.Is(e, ErrNotFound) {
			t.Fatal("file moved to fresh scope", e)
		}
		after, _ := os.Stat(path)
		if !after.ModTime().Equal(before.ModTime()) {
			t.Fatal("archive rewrote physical file")
		}
		s.Close()
		s, err = Open(dir)
		if err != nil {
			t.Fatal(err)
		}
	}
	s.Close()
}

func TestConversationReceiptsAreScopedDetachedAndStrict(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	in := ConversationCreateInput{BotID: "bot-meiiie", Title: "Scoped session", ClientNonce: "session-create-copy"}
	c, _, err := s.CreateConversation(in)
	if err != nil {
		t.Fatal(err)
	}
	a := ConversationArchiveInput{Archived: true, ExpectedRevision: 1, ClientNonce: "session-archive-once"}
	current, _, err := s.SetConversationArchive(c.ID, a)
	if err != nil {
		t.Fatal(err)
	}
	seq := s.Snapshot().Sequence
	retry, dup, err := s.CreateConversation(in)
	if err != nil || !dup || retry != current || s.Snapshot().Sequence != seq {
		t.Fatal("create retry repinned", err)
	}
	a.Archived = false
	if _, _, err = s.SetConversationArchive(c.ID, a); !errors.Is(err, ErrConflict) {
		t.Fatal("changed intent accepted", err)
	}
	if _, _, err = s.CreateConversation(ConversationCreateInput{BotID: "bot-davin", Title: in.Title, ClientNonce: in.ClientNonce}); !errors.Is(err, ErrConflict) {
		t.Fatal("cross bot nonce", err)
	}
	events, _ := s.Events(0)
	for i := range events {
		if events[i].Conversation != nil {
			events[i].Conversation.Title = "caller mutation"
		}
	}
	retry, _, _ = s.CreateConversation(in)
	if retry != current {
		t.Fatal("event pointer aliases metadata")
	}
	snapshot := s.Snapshot()
	for i := range snapshot.Conversations {
		snapshot.Conversations[i].Archived = false
	}
	retry, _, _ = s.CreateConversation(in)
	if retry != current {
		t.Fatal("snapshot aliases metadata")
	}
	if _, _, err = s.SetConversationArchive(c.ID, ConversationArchiveInput{Archived: false, ExpectedRevision: 1, ClientNonce: "session-stale-restore"}); !errors.Is(err, ErrConflict) {
		t.Fatal("stale CAS", err)
	}
}

func TestConversationArchivedExistingSupplementKeepsParentContext(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	old, _, err := s.Accept(input("archive-linked-original", "background"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.SetConversationArchive(old.ConversationID, ConversationArchiveInput{Archived: true, ClientNonce: "archive-linked-metadata"}); err != nil {
		t.Fatal(err)
	}
	result, duplicate, err := s.ControlTask(TaskControlInput{Version: 1, RequestID: old.RequestID, ExpectedEpoch: old.Epoch, Action: "supplement", ClientNonce: "archive-linked-control", Text: "Add detail to the original obligation"})
	if err != nil || duplicate || result.Related == nil || result.Related.ConversationID != old.ConversationID || result.Related.TaskLink.ParentRequestID != old.RequestID || result.Related.TaskLink.ParentPinSHA256 != old.Context.SHA256 {
		t.Fatal("archived typed task lineage", err)
	}
	if !reflect.DeepEqual(result.Work.Context, old.Context) {
		t.Fatal("archive repinned existing task")
	}
}
