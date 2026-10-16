package runtime

import (
	"nekobot.local/native/internal/ledger"
	"os"
	"path/filepath"
	"testing"
)

func TestRestoredRuntimeRefusedBeforeStateInitialization(t *testing.T) {
	dir := t.TempDir()
	s, e := ledger.Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	s.Close()
	os.WriteFile(filepath.Join(dir, "restore-readonly.json"), []byte(`{"version":1,"executionDisabled":true,"manifestSHA256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`), 0600)
	s, e = ledger.Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	_, e = New(s, Config{}, dir)
	if e != ledger.ErrReadOnly {
		t.Fatal("runtime not refused before configure", e)
	}
	if _, e = os.Stat(filepath.Join(dir, "tasks")); !os.IsNotExist(e) {
		t.Fatal("restored runtime initialized task state")
	}
	if _, e = os.Stat(filepath.Join(dir, "executors")); !os.IsNotExist(e) {
		t.Fatal("restored runtime initialized executor")
	}
}
