package ledger

import (
	"bytes"
	"errors"
	"os"
	"testing"
)

func TestIndependentGroupRecoveryCreateNonceCannotBecomeRecovery(t *testing.T) {
	s, _, g, w, _ := groupRecoveryFixture(t, 1)
	in := groupRecoverInput(g, w, g.ClientNonce)
	if _, e := s.CommandReceipt("collaboration_recover", in.ClientNonce, g.ID); !errors.Is(e, ErrConflict) {
		t.Fatal("wrong-kind lookup did not reject known create nonce", e)
	}
	before, _ := os.ReadFile(s.file.Name())
	seq := len(s.events)
	if _, _, e := s.RecoverCollaboration(g.ID, in); !errors.Is(e, ErrConflict) {
		t.Fatal("new recovery POST reuses already-committed collaboration_create nonce despite its own receipt lookup declaring a conflict", e)
	}
	after, _ := os.ReadFile(s.file.Name())
	if !bytes.Equal(before, after) || len(s.events) != seq {
		t.Fatal("known create nonce conflict wrote journal")
	}
}
