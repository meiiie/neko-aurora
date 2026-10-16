package ledger

import (
	"errors"
	"reflect"
	"testing"
)

func TestMetadataReceiptsOriginalTypedInputsCopiesAndRestart(t *testing.T) {
	s, c, p := productFixture(t)
	dir := s.dir
	create := BotInput{Name: "Receipt bot", Alias: "receiptbot", Description: "Owner metadata", ClientNonce: "meta-create-bot"}
	bot, _, err := s.CreateBot(create)
	if err != nil {
		t.Fatal(err)
	}
	name := BotNameInput{Name: "First name", ClientNonce: "meta-name-first"}
	if _, _, err = s.RenameBot(bot.ID, name); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.RenameBot(bot.ID, BotNameInput{Name: "Latest name", ExpectedDisplayRevision: 1, ClientNonce: "meta-name-next"}); err != nil {
		t.Fatal(err)
	}
	profile := BotProfileInput{Instructions: "Owner authorized instruction", IncludeHistory: false, ClientNonce: "meta-profile-first"}
	if _, _, err = s.SetBotProfile(bot.ID, profile); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.SetBotProfile(bot.ID, BotProfileInput{Instructions: "Current instruction", IncludeHistory: true, ExpectedRevision: 1, ClientNonce: "meta-profile-next"}); err != nil {
		t.Fatal(err)
	}
	project := ProjectInput{ID: p.ID, Name: "First project name", Goal: "Original authorized goal", ExpectedRevision: p.Revision, ChannelIDs: []string{c.ID}, ClientNonce: "meta-project-first"}
	if _, _, err = s.SaveProject(project); err != nil {
		t.Fatal(err)
	}
	next := project
	next.Name = "Latest project name"
	next.ExpectedRevision++
	next.ClientNonce = "meta-project-next"
	if _, _, err = s.SaveProject(next); err != nil {
		t.Fatal(err)
	}
	conversation := ConversationCreateInput{BotID: bot.ID, Title: "Receipt conversation", ClientNonce: "meta-conversation-create"}
	conv, _, err := s.CreateConversation(conversation)
	if err != nil {
		t.Fatal(err)
	}
	archive := ConversationArchiveInput{Archived: true, ExpectedRevision: 1, ClientNonce: "meta-conversation-archive"}
	if _, _, err = s.SetConversationArchive(conv.ID, archive); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		kind, nonce string
		input       any
	}{{"bot_create", create.ClientNonce, create}, {"bot_name", name.ClientNonce, BotNameReceiptInput{bot.ID, name}}, {"bot_profile", profile.ClientNonce, BotProfileReceiptInput{bot.ID, profile}}, {"project_save", project.ClientNonce, project}, {"conversation_create", conversation.ClientNonce, conversation}, {"conversation_archive", archive.ClientNonce, ConversationArchiveReceiptInput{conv.ID, archive}}}
	for round := 0; round < 3; round++ {
		before := s.Snapshot().Sequence
		for _, test := range cases {
			out, err := s.CommandReceipt(test.kind, test.nonce, "")
			if err != nil || !out.Found || !reflect.DeepEqual(out.Input, test.input) {
				t.Fatal("original metadata proof", test.kind, err)
			}
			if out.Bot != nil && out.Bot.Name != "Latest name" {
				t.Fatal("current bot projection")
			}
			if out.Project != nil {
				out.Project.ChannelIDs[0] = "caller-mutated"
				out.Input.(ProjectInput).ChannelIDs[0] = "caller-mutated"
			}
			if out.BotProfile != nil {
				out.BotProfile.Instructions = "caller mutation"
			}
		}
		if _, err = s.CommandReceipt("project_save", name.ClientNonce, ""); !errors.Is(err, ErrConflict) {
			t.Fatal("wrong kind accepted", err)
		}
		unknown, err := s.CommandReceipt("bot_name", "meta-never-posted", "")
		if err != nil || unknown.Found || unknown.Input != nil || s.Snapshot().Sequence != before {
			t.Fatal("lookup mutated or invented", err)
		}
		s.Close()
		s, err = Open(dir)
		if err != nil {
			t.Fatal("replay", err)
		}
	}
	s.Close()
}
