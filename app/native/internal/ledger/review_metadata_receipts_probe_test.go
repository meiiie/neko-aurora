package ledger

import (
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestIndependentL05MetadataReceiptsNormalizedCurrentCopiesAndReopen(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	create := BotInput{Name: "  Metadata bot  ", Alias: "METADATA-BOT", Description: "  Original description  ", ClientNonce: "independent-meta-create"}
	bot, _, err := s.CreateBot(create)
	if err != nil {
		t.Fatal(err)
	}
	create.Name = strings.TrimSpace(create.Name)
	create.Alias = strings.ToLower(create.Alias)
	create.Description = strings.TrimSpace(create.Description)
	name := BotNameInput{Name: "  Original visible name  ", ClientNonce: "independent-meta-name"}
	if _, _, err = s.RenameBot(bot.ID, name); err != nil {
		t.Fatal(err)
	}
	name.Name = strings.TrimSpace(name.Name)
	if _, _, err = s.RenameBot(bot.ID, BotNameInput{Name: "Current visible name", ExpectedDisplayRevision: 1, ClientNonce: "independent-meta-name-next"}); err != nil {
		t.Fatal(err)
	}
	profile := BotProfileInput{Instructions: "Original bounded profile", IncludeHistory: false, ClientNonce: "independent-meta-profile"}
	resolvedAvatar, resolvedColor := "", ""
	resolvedProfile := profile
	resolvedProfile.Avatar, resolvedProfile.Color = &resolvedAvatar, &resolvedColor
	if _, _, err = s.SetBotProfile(bot.ID, profile); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.SetBotProfile(bot.ID, BotProfileInput{Instructions: "Current bounded profile", IncludeHistory: true, ExpectedRevision: 1, ClientNonce: "independent-meta-profile-next"}); err != nil {
		t.Fatal(err)
	}
	c := sharedChannel(t, s, "Original metadata channel", "independent-meta-channel")
	project := ProjectInput{Name: "  Original project  ", Goal: "  Original goal  ", ChannelIDs: []string{c.ID}, ClientNonce: "independent-meta-project"}
	p, _, err := s.SaveProject(project)
	if err != nil {
		t.Fatal(err)
	}
	project.Name = strings.TrimSpace(project.Name)
	project.Goal = strings.TrimSpace(project.Goal)
	sort.Strings(project.ChannelIDs)
	if _, _, err = s.SaveProject(ProjectInput{ID: p.ID, Name: "Current project", Goal: "Current goal", ExpectedRevision: p.Revision, ChannelIDs: []string{c.ID}, ClientNonce: "independent-meta-project-next"}); err != nil {
		t.Fatal(err)
	}
	convCreate := ConversationCreateInput{BotID: bot.ID, Title: "  Original conversation  ", ClientNonce: "independent-meta-conversation"}
	conv, _, err := s.CreateConversation(convCreate)
	if err != nil {
		t.Fatal(err)
	}
	convCreate.Title = strings.TrimSpace(convCreate.Title)
	archive := ConversationArchiveInput{Archived: true, ExpectedRevision: 1, ClientNonce: "independent-meta-conversation-archive"}
	if _, _, err = s.SetConversationArchive(conv.ID, archive); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.SetConversationArchive(conv.ID, ConversationArchiveInput{Archived: false, ExpectedRevision: 2, ClientNonce: "independent-meta-conversation-restore"}); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		kind, nonce string
		want        any
	}{
		{"bot_create", create.ClientNonce, create}, {"bot_name", name.ClientNonce, BotNameReceiptInput{bot.ID, name}}, {"bot_profile", profile.ClientNonce, BotProfileReceiptInput{bot.ID, resolvedProfile}},
		{"project_save", project.ClientNonce, project}, {"conversation_create", convCreate.ClientNonce, convCreate}, {"conversation_archive", archive.ClientNonce, ConversationArchiveReceiptInput{conv.ID, archive}},
	}
	for round := 0; round < 3; round++ {
		before := s.Snapshot().Sequence
		for _, tc := range cases {
			out, e := s.CommandReceipt(tc.kind, tc.nonce, "")
			if e != nil || !out.Found || !reflect.DeepEqual(out.Input, tc.want) {
				t.Fatal("normalized original intent changed", tc.kind, round, e)
			}
			if out.Work != nil || out.Related != nil || out.Collaboration != nil || out.BotState != nil || out.ProjectState != nil {
				t.Fatal("metadata receipt widened graph projection", tc.kind)
			}
			if out.Bot != nil {
				if out.Bot.ID != bot.ID || out.Bot.Name != "Current visible name" || out.Bot.DisplayRevision != 2 {
					t.Fatal("current bot mismatch")
				}
				out.Bot.Name = "caller-mutated"
			}
			if out.BotProfile != nil {
				if out.BotProfile.Revision != 2 || out.BotProfile.Instructions != "Current bounded profile" {
					t.Fatal("current profile mismatch")
				}
				out.BotProfile.Instructions = "caller-mutated"
			}
			if out.Project != nil {
				if out.Project.Revision != 2 || out.Project.Name != "Current project" || out.Input.(ProjectInput).ID != "" {
					t.Fatal("created project intent rebound to update")
				}
				out.Project.ChannelIDs[0] = "caller-mutated"
				out.Input.(ProjectInput).ChannelIDs[0] = "caller-mutated"
			}
			if out.Conversation != nil {
				if out.Conversation.ID != conv.ID || out.Conversation.Revision != 3 || out.Conversation.Archived {
					t.Fatal("current conversation mismatch")
				}
				out.Conversation.Title = "caller-mutated"
			}
			for _, other := range cases {
				if other.kind == tc.kind {
					continue
				}
				if _, e = s.CommandReceipt(other.kind, tc.nonce, ""); !errors.Is(e, ErrConflict) {
					t.Fatal("wrong-kind receipt accepted", tc.kind, other.kind, e)
				}
			}
		}
		unknown, e := s.CommandReceipt("bot_name", "independent-meta-never-posted", "")
		if e != nil || unknown.Found || unknown.Input != nil || s.Snapshot().Sequence != before {
			t.Fatal("unknown/read-only lookup mutated", e)
		}
		if err = s.Close(); err != nil {
			t.Fatal(err)
		}
		s, err = Open(dir)
		if err != nil {
			t.Fatal("reopen", err)
		}
	}
}

func TestIndependentL05MetadataReceiptSensitiveOriginalAndCurrentFields(t *testing.T) {
	for _, field := range []string{"original_bot_alias", "current_bot_name", "current_project_goal"} {
		t.Run(field, func(t *testing.T) {
			s, err := Open(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			var kind, nonce string
			sensitive := `password = "` + strings.Repeat("x", 24) + `"`
			if field == "original_bot_alias" {
				alias := "sk-" + strings.Repeat("x", 25)
				if !hasSecret([]byte(alias)) {
					t.Fatal("control: alias canary not recognized")
				}
				nonce = "independent-sensitive-alias"
				kind = "bot_create"
				if _, _, err = s.CreateBot(BotInput{Name: "Legacy alias metadata", Alias: alias, ClientNonce: nonce}); err != nil {
					t.Fatal("control: legacy alias metadata unavailable", err)
				}
			} else if field == "current_bot_name" {
				if !hasSecret([]byte(sensitive)) {
					t.Fatal("control: quoted canary not recognized")
				}
				nonce = "independent-sensitive-name-original"
				kind = "bot_name"
				if _, _, err = s.RenameBot("bot-meiiie", BotNameInput{Name: "Safe original metadata", ClientNonce: nonce}); err != nil {
					t.Fatal(err)
				}
				if _, _, err = s.RenameBot("bot-meiiie", BotNameInput{Name: sensitive, ExpectedDisplayRevision: 1, ClientNonce: "independent-sensitive-name-next"}); err != nil {
					t.Fatal("control: legacy current name unavailable", err)
				}
			} else {
				if !hasSecret([]byte(sensitive)) {
					t.Fatal("control: quoted canary not recognized")
				}
				c := sharedChannel(t, s, "Legacy project metadata", "independent-sensitive-channel")
				nonce = "independent-sensitive-project-original"
				kind = "project_save"
				p, _, e := s.SaveProject(ProjectInput{Name: "Safe project metadata", Goal: "Safe original goal", ChannelIDs: []string{c.ID}, ClientNonce: nonce})
				if e != nil {
					t.Fatal(e)
				}
				if _, _, err = s.SaveProject(ProjectInput{ID: p.ID, Name: p.Name, Goal: sensitive, ExpectedRevision: p.Revision, ChannelIDs: p.ChannelIDs, ClientNonce: "independent-sensitive-project-next"}); err != nil {
					t.Fatal("control: legacy current goal unavailable", err)
				}
			}
			before := s.Snapshot().Sequence
			if _, err = s.CommandReceipt(kind, nonce, ""); !errors.Is(err, ErrInvalid) {
				t.Errorf("recognized sensitive metadata field projected: field=%s err=%v", field, err)
			}
			if s.Snapshot().Sequence != before {
				t.Fatal("receipt modified journal")
			}
		})
	}
}
