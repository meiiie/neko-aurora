//go:build windows

package runtime

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestProfileRevisionKeyProtectedAndStable(t *testing.T) {
	dir := t.TempDir()
	key, err := profileRevisionKey(dir, "fixture-source-v1")
	if err != nil || len(key) != 64 {
		t.Fatal("private revision key unavailable", err)
	}
	stored, err := os.ReadFile(filepath.Join(dir, "revision-key.dpapi"))
	if err != nil || bytes.Contains(stored, []byte(key)) {
		t.Fatal("revision key must only persist as DPAPI ciphertext", err)
	}
	reopened, err := profileRevisionKey(dir, "fixture-source-v1")
	if err != nil || reopened != key {
		t.Fatal("restart changed consent revision identity", err)
	}
	if _, err = profileRevisionKey(dir, "different-source-v1"); err == nil {
		t.Fatal("source rebinding opened another source's revision key")
	}
	stored[len(stored)-1] ^= 1
	if err = os.WriteFile(filepath.Join(dir, "revision-key.dpapi"), stored, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = profileRevisionKey(dir, "fixture-source-v1"); err == nil {
		t.Fatal("corrupt ciphertext silently regenerated profile authority")
	}
}
