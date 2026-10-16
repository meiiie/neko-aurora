package ledger

import (
	"errors"
	"reflect"
	"testing"
)

func TestFixtureLegacyOwnerNoncePreservesOriginalReceiptWithoutGrant(t *testing.T) {
	s, a, p, _ := fixtureLedger(t)
	s.ConfigureIdentity(nil)
	c := sharedChannel(t, s, "Legacy shared", "legacy-fixture-channel")
	in := Input{BotID: "bot-meiiie", ClientNonce: "legacy-owner-retry", Text: "Legacy original exact request", Lane: "foreground"}
	scope := SubmissionScope{ChannelID: c.ID, ExpectedChannelRevision: &c.Revision}
	old, _, e := s.AcceptProduct(in, scope)
	if e != nil {
		t.Fatal(e)
	}
	old = completeChannelWork(t, s, old, "Legacy completed output")
	s.ConfigureIdentity(a)
	before := s.Snapshot().Sequence
	got, dup, e := s.AcceptFor(p["owner"], in, scope)
	if e != nil || !dup || got.RequestID != old.RequestID || got.RequesterPin != nil || !reflect.DeepEqual(got, old) || s.Snapshot().Sequence != before {
		t.Fatal("legacy owner retry created execution or inferred grant", e)
	}
	in.Text = "Changed original"
	if _, _, e = s.AcceptFor(p["owner"], in, scope); !errors.Is(e, ErrConflict) || s.Snapshot().Sequence != before {
		t.Fatal("legacy nonce accepted changed payload", e)
	}
	s.Close()
	reopened, e := Open(s.dir)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	reopened.ConfigureIdentity(a)
	in.Text = old.Text
	got, dup, e = reopened.AcceptFor(p["owner"], in, scope)
	if e != nil || !dup || got.RequesterPin != nil || got.Status != "completed" || got.RequestID != old.RequestID {
		t.Fatal("restart inferred legacy fixture grant", e)
	}
}
