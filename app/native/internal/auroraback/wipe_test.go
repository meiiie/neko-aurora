package auroraback

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWipeIfRequested(t *testing.T) {
	st := t.TempDir()
	for _, f := range []string{"transactions.jsonl", "aurora/vault.key", "aurora/vault.json", "backups/aurora-1.tar.gz", "connections/x.json"} {
		os.MkdirAll(filepath.Dir(filepath.Join(st, f)), 0o700)
		os.WriteFile(filepath.Join(st, f), []byte("x"), 0o600)
	}
	if ok, err := WipeIfRequested(st); ok || err != nil {
		t.Fatalf("wiped without request: %v %v", ok, err)
	}
	if _, err := os.Stat(filepath.Join(st, "aurora/vault.key")); err != nil {
		t.Fatal("data lost without request")
	}
	if err := RequestWipe(st); err != nil {
		t.Fatal(err)
	}
	if ok, err := WipeIfRequested(st); !ok || err != nil {
		t.Fatalf("wipe: %v %v", ok, err)
	}
	if ents, _ := os.ReadDir(st); len(ents) != 0 {
		t.Fatalf("left %d entries", len(ents))
	}
}
