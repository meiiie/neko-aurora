package privatefs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrivateDirectoryProtection(t *testing.T) {
	dir := t.TempDir()
	if err := ProtectDirectory(dir); err != nil {
		t.Fatal(err)
	}
	if err := CheckDirectory(dir); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "fixture")
	if err := os.WriteFile(file, []byte("public fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if ProtectDirectory(file) == nil || CheckDirectory(file) == nil {
		t.Fatal("file accepted as directory")
	}
	link := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(dir, link); err == nil && (ProtectDirectory(link) == nil || CheckDirectory(link) == nil) {
		t.Fatal("directory alias accepted")
	}
}
