package ledger

import (
	"errors"
	"testing"
)

func TestIndependentL01CommandNonceCrossKind(t *testing.T) {
	for _, direction := range []string{"bot_then_task", "task_then_bot", "bot_then_group", "group_then_bot"} {
		t.Run(direction, func(t *testing.T) {
			s, err := Open(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			nonce := "independent-l01-cross-kind"
			bot := BotControlInput{BotID: "bot-davin", Action: "defaults", DefaultModel: "requested-metadata", ClientNonce: nonce}
			var before uint64
			switch direction {
			case "bot_then_task":
				saveBotState(t, s, bot)
				w, _, e := s.Accept(input("independent-cross-task-intake", "background"))
				if e != nil {
					t.Fatal(e)
				}
				before = s.Snapshot().Sequence
				_, _, err = s.ControlTask(controlInput(w, "pause", nonce))
			case "task_then_bot":
				w, _, e := s.Accept(input("independent-cross-task-intake", "background"))
				if e != nil {
					t.Fatal(e)
				}
				if _, _, e = s.ControlTask(controlInput(w, "pause", nonce)); e != nil {
					t.Fatal(e)
				}
				before = s.Snapshot().Sequence
				_, _, err = s.ControlBot(bot)
			case "bot_then_group":
				saveBotState(t, s, bot)
				c := sharedChannel(t, s, "Cross-kind fixture", "independent-cross-channel")
				before = s.Snapshot().Sequence
				_, _, err = s.CreateCollaboration(CollaborationInput{ChannelID: c.ID, ExpectedChannelRevision: c.Revision, Participants: []string{"bot-meiiie", "bot-davin"}, CoordinatorBotID: "bot-meiiie", Goal: "Existing scope", MaxStages: 3, ClientNonce: nonce})
			case "group_then_bot":
				c := sharedChannel(t, s, "Cross-kind fixture", "independent-cross-channel")
				if _, _, e := s.CreateCollaboration(CollaborationInput{ChannelID: c.ID, ExpectedChannelRevision: c.Revision, Participants: []string{"bot-meiiie", "bot-davin"}, CoordinatorBotID: "bot-meiiie", Goal: "Existing scope", MaxStages: 3, ClientNonce: nonce}); e != nil {
					t.Fatal(e)
				}
				before = s.Snapshot().Sequence
				_, _, err = s.ControlBot(bot)
			}
			if !errors.Is(err, ErrConflict) {
				t.Errorf("same command nonce committed in two receipt kinds: direction=%s err=%v", direction, err)
			}
			if s.Snapshot().Sequence != before {
				t.Errorf("wrong-kind command mutated journal: before=%d after=%d", before, s.Snapshot().Sequence)
			}
		})
	}
}
