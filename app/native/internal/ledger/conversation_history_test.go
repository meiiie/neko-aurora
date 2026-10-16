package ledger

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestConversationExportProjectsOnlyPublishedHistoryAndNoAuthority(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	source := completedPermissionSource(t, s, "export-old-source", "visible owner message")
	work, _, err := s.Accept(input("export-new-work", "background"))
	if err != nil {
		t.Fatal(err)
	}
	c, _, err := s.CreateConversation(ConversationCreateInput{BotID: "bot-meiiie", Title: "Separate", ClientNonce: "export-separate-create"})
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := s.AcceptProduct(Input{BotID: "bot-meiiie", ClientNonce: "export-other-scope", Text: "other conversation private canary", Lane: "foreground"}, SubmissionScope{ChannelID: c.ID})
	if err != nil {
		t.Fatal(err)
	}
	seq := s.Snapshot().Sequence
	raw, err := s.ExportConversation("conversation-bot-meiiie", "json")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, forbidden := range []string{"other conversation private canary", "contextPin", "runtimeContext", "clientNonce", "accountPin", "operations", "toolPolicyVersion"} {
		if strings.Contains(text, forbidden) {
			t.Fatal("export expanded scope", forbidden)
		}
	}
	var dto struct {
		Messages []struct {
			RequestID string `json:"requestId"`
		}
	}
	if json.Unmarshal(raw, &dto) != nil || len(dto.Messages) != 2 || dto.Messages[0].RequestID != source.RequestID || dto.Messages[1].RequestID != work.RequestID {
		t.Fatal("public history order", text)
	}
	if s.Snapshot().Sequence != seq {
		t.Fatal("export mutated journal")
	}
	old, _ := s.Get(other.RequestID)
	if old.Text != other.Text {
		t.Fatal("export moved work")
	}
	if _, err = s.ExportConversation("conversation-bot-davin", "html"); !errors.Is(err, ErrInvalid) {
		t.Fatal("active export format", err)
	}
}
func TestRetentionPreferenceIsPreviewOnlyAndProtectsPinsEffectsAndRunningWork(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	source := completedPermissionSource(t, s, "retention-context-source", "retained pin source")
	active, _, err := s.Accept(input("retention-background", "background"))
	if err != nil {
		t.Fatal(err)
	}
	terminal, _, err := s.Accept(input("retention-cancelled", "foreground"))
	if err != nil {
		t.Fatal(err)
	}
	terminal, err = s.Cancel(terminal.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return time.Now().UTC().Add(400 * 24 * time.Hour) }
	before := s.Snapshot()
	p, dup, err := s.SetRetentionPreference("conversation-bot-meiiie", RetentionInput{Days: 30, ClientNonce: "retention-preview-choice"})
	if err != nil || dup || p.Revision != 1 || p.Days != 30 {
		t.Fatal(err)
	}
	preview, err := s.PreviewRetention("conversation-bot-meiiie", 30)
	if err != nil || preview.Enabled || len(preview.Candidates) != 1 || preview.Candidates[0].ID != terminal.RequestID {
		t.Fatal("candidate or policy", err, preview)
	}
	protected := map[string]bool{}
	for _, item := range preview.Protected {
		protected[item.ID] = true
	}
	if !protected[source.RequestID] || !protected[active.RequestID] {
		t.Fatal("pins/running unprotected")
	}
	for _, w := range before.Work {
		after, _ := s.Get(w.RequestID)
		if !reflect.DeepEqual(after, w) {
			t.Fatal("preview modified work")
		}
	}
	seq := s.Snapshot().Sequence
	zero, err := s.PreviewRetention("conversation-bot-meiiie", 0)
	if err != nil || len(zero.Candidates) != 0 || zero.Enabled || s.Snapshot().Sequence != seq {
		t.Fatal("disabled preview", err)
	}
	if _, _, err = s.SetRetentionPreference("conversation-bot-meiiie", RetentionInput{Days: 40, ExpectedRevision: 0, ClientNonce: "retention-stale-choice"}); !errors.Is(err, ErrConflict) {
		t.Fatal("retention CAS", err)
	}
	for i := 0; i < 3; i++ {
		s.Close()
		s, err = Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		retry, duplicate, e := s.SetRetentionPreference("conversation-bot-meiiie", RetentionInput{Days: 30, ClientNonce: "retention-preview-choice"})
		if e != nil || !duplicate || retry != p {
			t.Fatal("durable preview preference", e)
		}
	}
	s.Close()
}
