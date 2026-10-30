package privatefs

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
)

func TestWindowsPrivateDirectoryChildrenInheritOwnerOnlyACL(t *testing.T) {
	dir := t.TempDir()
	if err := ProtectDirectory(dir); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(dir, "public-fixture.json")
	if err := os.WriteFile(child, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	h, err := openFile(child, syscall.GENERIC_READ)
	if err != nil {
		t.Fatal(err)
	}
	err = checkHandleSecurity(h, 0x10, false)
	syscall.CloseHandle(h)
	if err != nil {
		t.Fatal("child did not inherit owner-only access", err)
	}
	if err := exec.Command("icacls.exe", dir, "/grant", "*S-1-1-0:(OI)(CI)R").Run(); err != nil {
		t.Fatal("could not construct ACL fixture")
	}
	if CheckDirectory(dir) == nil {
		t.Fatal("shared directory accepted")
	}
	if err := ProtectDirectory(dir); err != nil {
		t.Fatal(err)
	}
	h, err = openFile(child, syscall.GENERIC_READ)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.CloseHandle(h)
	if err := checkHandleSecurity(h, 0x10, false); err != nil {
		t.Fatal("shared child inheritance not repaired", err)
	}
}

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
