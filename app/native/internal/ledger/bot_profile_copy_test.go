package ledger

import "testing"

func TestReviewProfileAndCASReturnsAreDetached(t *testing.T) {
	surfaces := []string{"accept", "get", "snapshot", "events"}
	mutations := []string{"work_profile", "context_profile", "expected_revision"}
	for _, surface := range surfaces {
		for _, mutation := range mutations {
			t.Run(surface+"/"+mutation, func(t *testing.T) {
				s, err := Open(t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				defer s.Close()
				if _, _, err = s.SetBotProfile("bot-meiiie", BotProfileInput{Instructions: "original preference", IncludeHistory: false, ClientNonce: "review-profile-create"}); err != nil {
					t.Fatal(err)
				}
				revision := uint64(1)
				input := Input{BotID: "bot-meiiie", ClientNonce: "review-work-created", Text: "fixture request", Lane: "foreground"}
				accepted, _, err := s.AcceptProduct(input, SubmissionScope{ExpectedBotProfileRevision: &revision})
				if err != nil {
					t.Fatal(err)
				}
				returned := accepted
				switch surface {
				case "get":
					returned, _ = s.Get(accepted.RequestID)
				case "snapshot":
					returned = s.Snapshot().Work[0]
				case "events":
					events, e := s.Events(0)
					if e != nil {
						t.Fatal(e)
					}
					returned = events[len(events)-1].Work
				}
				switch mutation {
				case "work_profile":
					returned.BotProfile.Instructions = "caller changed preference"
				case "context_profile":
					returned.Context.Profile.Instructions = "caller changed context preference"
				case "expected_revision":
					*returned.ExpectedBotProfileRevision = 2
				}
				stored, err := s.Get(accepted.RequestID)
				if err != nil {
					t.Fatal(err)
				}
				if stored.BotProfile.Instructions != "original preference" || stored.Context.Profile.Instructions != "original preference" || *stored.ExpectedBotProfileRevision != 1 {
					t.Fatal("returned value mutated stored accepted profile/CAS without a journal event")
				}
			})
		}
	}
}
