package privatefs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrivateFileCreationReplacementAndRepair(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private.bin")
	f, err := CreateNew(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = Check(path); err != nil {
		f.Close()
		t.Fatal("file not private before first write", err)
	}
	if _, err = f.Write([]byte("fixture")); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if _, err = CreateNew(path); err == nil {
		t.Fatal("exclusive creation overwrote file")
	}
	if err = WriteFile(path, []byte("replacement")); err != nil {
		t.Fatal(err)
	}
	if err = Check(path); err != nil {
		t.Fatal("replacement lost protection", err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "replacement" {
		t.Fatal("replacement failed", err)
	}
	legacy := filepath.Join(filepath.Dir(path), "legacy.bin")
	if err = os.WriteFile(legacy, []byte("legacy fixture"), 0o666); err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(legacy, 0o666); err != nil {
		t.Fatal(err)
	}
	if Check(legacy) == nil {
		t.Fatal("unprotected fixture considered private")
	}
	if err = Protect(legacy); err != nil {
		t.Fatal(err)
	}
	if err = Check(legacy); err != nil {
		t.Fatal("legacy protection failed", err)
	}
}
