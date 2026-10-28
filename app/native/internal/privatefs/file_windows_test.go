package privatefs

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestWindowsRejectsSharedACLAndHardlinks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic.bin")
	if err := WriteFile(path, []byte("public test fixture")); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("icacls.exe", path, "/grant", "*S-1-1-0:R").Run(); err != nil {
		t.Fatal("could not construct shared ACL fixture", err)
	}
	if Check(path) == nil {
		t.Fatal("Everyone read permission was not rejected")
	}
	if err := Protect(path); err != nil {
		t.Fatal(err)
	}
	if err := Check(path); err != nil {
		t.Fatal("shared ACE not removed", err)
	}
	alias := path + ".link"
	if err := os.Link(path, alias); err != nil {
		t.Fatal(err)
	}
	if Check(path) == nil || Protect(alias) == nil {
		t.Fatal("hardlinked secret path accepted")
	}
}
