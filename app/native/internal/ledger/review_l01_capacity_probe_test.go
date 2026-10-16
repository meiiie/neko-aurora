package ledger

import (
	"fmt"
	"testing"
)

func TestIndependentL01StateCapacityCannotPublishUnreplayableJournal(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if s != nil {
			s.Close()
		}
	}()
	var last BotControlInput
	for n := 0; n < 2048; n++ {
		last = BotControlInput{BotID: "bot-meiiie", Action: "defaults", ExpectedRevision: uint64(n), ClientNonce: fmt.Sprintf("independent-capacity-%04d", n), DefaultModel: fmt.Sprintf("wanted-model-%d", n%2)}
		if _, _, err = s.ControlBot(last); err != nil {
			t.Fatal("control: valid within-capacity write rejected", n, err)
		}
	}
	before := s.Snapshot().Sequence
	extra := BotControlInput{BotID: "bot-meiiie", Action: "defaults", ExpectedRevision: 2048, ClientNonce: "independent-capacity-overflow", DefaultModel: "wanted-model-overflow"}
	if _, _, err = s.ControlBot(extra); err == nil {
		t.Error("new bot-state command published past replay nonce bound")
	}
	if s.Snapshot().Sequence != before {
		t.Errorf("capacity overflow changed journal: before=%d after=%d", before, s.Snapshot().Sequence)
	}
	if _, dup, e := s.ControlBot(last); e != nil || !dup {
		t.Error("existing original receipt blocked at capacity", e)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s = nil
	reopened, e := Open(dir)
	if e != nil {
		t.Error("successfully acknowledged bot-state journal cannot reopen", e)
	}
	if reopened != nil {
		reopened.Close()
	}
}
