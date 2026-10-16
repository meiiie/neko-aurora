package ledger

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestReadOnlyInspectionDoesNotRepairOrCreateState(t *testing.T) {
	dir := t.TempDir()
	s, e := Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	_, _, e = s.Accept(Input{BotID: "bot-meiiie", ClientNonce: "inspect-original-nonce", Text: "kept original", Lane: "background"})
	if e != nil {
		t.Fatal(e)
	}
	s.Close()
	original, _ := os.ReadFile(filepath.Join(dir, "transactions.jsonl"))
	before, _ := os.ReadFile(filepath.Join(dir, "local-sync.identity"))
	r, e := OpenReadOnly(dir)
	if e != nil {
		t.Fatal(e)
	}
	if !r.ReadOnly() {
		t.Fatal("inspection must be read only")
	}
	if _, _, e = r.Accept(Input{BotID: "bot-meiiie", ClientNonce: "inspect-new-nonce", Text: "must refuse", Lane: "foreground"}); e == nil {
		t.Fatal("read-only accepted new request")
	}
	r.Close()
	after, _ := os.ReadFile(filepath.Join(dir, "transactions.jsonl"))
	identity, _ := os.ReadFile(filepath.Join(dir, "local-sync.identity"))
	if !bytes.Equal(original, after) || !bytes.Equal(before, identity) {
		t.Fatal("inspection mutated state")
	}
	tail := append(append([]byte{}, original...), []byte(`{"unfinished":`)...)
	os.WriteFile(filepath.Join(dir, "transactions.jsonl"), tail, 0600)
	if r, e = OpenReadOnly(dir); e == nil {
		r.Close()
		t.Fatal("inspection repaired incomplete journal")
	}
	after, _ = os.ReadFile(filepath.Join(dir, "transactions.jsonl"))
	if !bytes.Equal(tail, after) {
		t.Fatal("read-only truncated source")
	}
	missing := filepath.Join(t.TempDir(), "missing")
	if r, e = OpenReadOnly(missing); e == nil {
		r.Close()
		t.Fatal("created missing state")
	}
	if _, e = os.Stat(missing); !os.IsNotExist(e) {
		t.Fatal("inspection created directory")
	}
}

func TestRestoredStateCannotCommitOrRecover(t *testing.T) {
	dir := t.TempDir()
	s, e := Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	w, _, e := s.Accept(Input{BotID: "bot-meiiie", ClientNonce: "restore-original-nonce", Text: "original obligation", Lane: "background"})
	if e != nil {
		t.Fatal(e)
	}
	s.Close()
	original, _ := os.ReadFile(filepath.Join(dir, "transactions.jsonl"))
	marker := []byte(`{"version":1,"executionDisabled":true,"manifestSHA256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`)
	if e = os.WriteFile(filepath.Join(dir, "restore-readonly.json"), marker, 0600); e != nil {
		t.Fatal(e)
	}
	r, e := Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	if !r.ReadOnly() || !r.Snapshot().ReadOnly || r.Snapshot().Runtime != "restored_read_only" {
		t.Fatal("missing read-only projection")
	}
	dup, duplicate, e := r.Accept(Input{BotID: "bot-meiiie", ClientNonce: "restore-original-nonce", Text: "original obligation", Lane: "background"})
	if e != nil || !duplicate || dup.RequestID != w.RequestID {
		t.Fatal("original duplicate identity changed", e)
	}
	if _, e = r.Cancel(w.RequestID); e == nil {
		t.Fatal("cancel mutated restore")
	}
	if e = r.RecoverInterrupted(); e == nil {
		t.Fatal("recovery enabled restored work")
	}
	after, _ := os.ReadFile(filepath.Join(dir, "transactions.jsonl"))
	if !bytes.Equal(original, after) {
		t.Fatal("restore mutated original journal")
	}
}
