package ledger

import (
	"errors"
	"reflect"
	"testing"
)

func TestProfileIntakeCASBindsTargetAndOriginalRetry(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	zero := uint64(0)
	one := uint64(1)
	in := Input{BotID: "bot-meiiie", Text: "@davin original scoped request", Lane: "foreground", ClientNonce: "profile-cas-original"}
	w, dup, err := s.AcceptProduct(in, SubmissionScope{ExpectedBotProfileRevision: &zero})
	if err != nil || dup || w.BotID != "bot-davin" {
		t.Fatal(err)
	}
	if _, _, err = s.SetBotProfile("bot-davin", BotProfileInput{Instructions: "Be precise", IncludeHistory: false, ClientNonce: "profile-cas-config"}); err != nil {
		t.Fatal(err)
	}
	sequence := s.Snapshot().Sequence
	if retry, dup, err := s.AcceptProduct(in, SubmissionScope{ExpectedBotProfileRevision: &zero}); err != nil || !dup || !reflect.DeepEqual(w, retry) || s.Snapshot().Sequence != sequence {
		t.Fatal("original CAS receipt rebind", err)
	}
	if _, _, err = s.AcceptProduct(in, SubmissionScope{ExpectedBotProfileRevision: &one}); !errors.Is(err, ErrConflict) {
		t.Fatal("changed CAS retried original nonce", err)
	}
	in.ClientNonce = "profile-cas-new-stale"
	if _, _, err = s.AcceptProduct(in, SubmissionScope{ExpectedBotProfileRevision: &zero}); !errors.Is(err, ErrConflict) || s.Snapshot().Sequence != sequence {
		t.Fatal("stale target profile imported history", err)
	}
	in.ClientNonce = "profile-cas-new-current"
	if newer, _, err := s.AcceptProduct(in, SubmissionScope{ExpectedBotProfileRevision: &one}); err != nil || newer.BotProfile == nil || newer.BotProfile.Revision != 1 || newer.BotProfile.IncludeHistory {
		t.Fatal("target profile CAS", err)
	}
}
