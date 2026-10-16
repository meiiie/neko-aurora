package ledger

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestIndependentL04RawSensitivePublicTranscriptFailsClosed(t *testing.T) {
	for _, field := range []string{"group_goal", "owner_text", "assistant_reply"} {
		t.Run(field, func(t *testing.T) {
			s, err := Open(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			c := sharedChannel(t, s, "Public export fixture", "independent-export-channel")
			sensitive := `password = "` + strings.Repeat("x", 24) + `"`
			if !hasSecret([]byte(sensitive)) {
				t.Fatal("control: synthetic canary not recognized")
			}
			if field == "group_goal" {
				if _, _, err = s.CreateCollaboration(CollaborationInput{ChannelID: c.ID, ExpectedChannelRevision: c.Revision, Participants: []string{"bot-davin", "bot-meiiie"}, CoordinatorBotID: "bot-meiiie", Goal: sensitive, MaxStages: 3, ClientNonce: "independent-export-group"}); err != nil {
					t.Fatal("control: legacy public goal unavailable", err)
				}
			} else {
				text := "Public request"
				if field == "owner_text" {
					text = sensitive
				}
				w, _, e := s.AcceptProduct(Input{BotID: "bot-meiiie", Text: text, Lane: "background", ClientNonce: "independent-export-public"}, SubmissionScope{ChannelID: c.ID})
				if e != nil {
					t.Fatal("control: legacy public owner row unavailable", e)
				}
				if field == "assistant_reply" {
					completeChannelWork(t, s, w, sensitive)
				}
			}
			before := s.Snapshot().Sequence
			for _, format := range []string{"text", "json"} {
				_, err = s.ExportSharedHistory(c.ID, "", format)
				if !errors.Is(err, ErrInvalid) {
					t.Errorf("recognized raw sensitive public field exported after encoding: field=%s format=%s err=%v", field, format, err)
				}
			}
			if s.Snapshot().Sequence != before {
				t.Fatal("export modified ledger")
			}
		})
	}
}

func TestIndependentL03SourceMetadataScopeAndNoRawPayload(t *testing.T) {
	s, c, p := productFixture(t)
	private, _, err := s.Accept(Input{BotID: "bot-davin", ClientNonce: "independent-source-private", Text: "PRIVATE_TURN_CANARY", Lane: "foreground"})
	if err != nil {
		t.Fatal(err)
	}
	completeChannelWork(t, s, private, "PRIVATE_REPLY_CANARY")
	if _, _, err = s.SetBotProfile("bot-meiiie", BotProfileInput{Instructions: "PROFILE_RAW_CANARY", IncludeHistory: true, ClientNonce: "independent-source-profile"}); err != nil {
		t.Fatal(err)
	}
	continuity := ProjectContinuityInput{ProjectID: p.ID, ChannelID: c.ID, ExpectedProjectRevision: p.Revision, ExpectedChannelRevision: c.Revision, ClientNonce: "independent-source-continuity", Instructions: "CONTINUITY_RAW_CANARY"}
	if _, _, err = s.SaveProjectContinuity(continuity); err != nil {
		t.Fatal(err)
	}
	file, _, err := s.UploadFile(UploadInput{ProjectID: p.ID, ProjectRevision: p.Revision, ChannelID: c.ID, Filename: "authorized.md", Content: []byte("EXCERPT_RAW_CANARY"), ClientNonce: "independent-source-upload"})
	if err != nil {
		t.Fatal(err)
	}
	w, _, err := s.AcceptProduct(Input{BotID: "bot-meiiie", ClientNonce: "independent-source-intake", Text: "OWNER_REQUEST_CANARY", Lane: "background"}, SubmissionScope{ChannelID: c.ID, ProjectID: p.ID, ProjectRevision: p.Revision, FileIDs: []string{file.ID}})
	if err != nil {
		t.Fatal(err)
	}
	before := s.Snapshot().Sequence
	out, err := s.ExplainSources(w.RequestID, c.ID, p.ID)
	if err != nil || !out.Usable {
		t.Fatal("authorized source metadata rejected", err)
	}
	raw, _ := json.Marshal(out)
	for _, blocked := range []string{"PRIVATE_TURN_CANARY", "PRIVATE_REPLY_CANARY", "PROFILE_RAW_CANARY", "CONTINUITY_RAW_CANARY", "EXCERPT_RAW_CANARY", "OWNER_REQUEST_CANARY", private.RequestID, "contextPin", "accountPin", "runtimeContext", "clientNonce", "credentialGeneration"} {
		if strings.Contains(string(raw), blocked) {
			t.Fatal("source explanation widened privacy scope")
		}
	}
	for _, wrong := range [][2]string{{"conversation-bot-davin", p.ID}, {c.ID, ""}, {c.ID, "project-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}} {
		if _, err = s.ExplainSources(w.RequestID, wrong[0], wrong[1]); !errors.Is(err, ErrConflict) {
			t.Fatal("wrong source view scope admitted", err)
		}
	}
	if s.Snapshot().Sequence != before {
		t.Fatal("metadata inspection changed journal")
	}
	if err = s.SetFileAccess(FileAccessInput{FileID: file.ID, Granted: false, ClientNonce: "independent-source-revoke"}); err != nil {
		t.Fatal(err)
	}
	out, err = s.ExplainSources(w.RequestID, c.ID, p.ID)
	if err != nil || out.Usable {
		t.Fatal("revoked source usable", err)
	}
	found := false
	for _, src := range out.Sources {
		if src.ID == file.ID {
			found = true
			if src.Usable || src.Name != "" {
				t.Fatal("revoked file name/access leaked")
			}
		}
	}
	if !found {
		t.Fatal("revoked source metadata missing")
	}
}
