//go:build !windows

package statebackup

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestUnsupportedPlatformProtectionFailsClosed(t *testing.T) {
	data, entropy := []byte("synthetic-private-fixture"), []byte("fixture-binding")
	for name, operation := range map[string]func([]byte, []byte) ([]byte, error){"protect": protect, "unprotect": unprotect} {
		t.Run(name, func(t *testing.T) {
			out, err := operation(data, entropy)
			if !errors.Is(err, ErrUnsupported) || out != nil {
				t.Fatal("unsupported platform returned protected data", err)
			}
			if string(data) != "synthetic-private-fixture" || string(entropy) != "fixture-binding" {
				t.Fatal("caller buffers changed")
			}
		})
	}
}

func TestUnsupportedPlatformBackupAPIsLeaveFilesUntouched(t *testing.T) {
	for _, operation := range []string{"create", "verify", "restore"} {
		t.Run(operation, func(t *testing.T) {
			root := t.TempDir()
			source := filepath.Join(root, "source")
			if err := os.Mkdir(source, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(source, "private-fixture")
			original := []byte("private synthetic content")
			if err := os.WriteFile(path, original, 0600); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(root, "destination")
			var err error
			switch operation {
			case "create":
				_, err = Create(source, target, "working-tree")
			case "verify":
				_, err = Verify(source)
			case "restore":
				_, err = RestoreDryRun(source, target)
			}
			if !errors.Is(err, ErrUnsupported) {
				t.Fatal("unsupported backup operation did not fail closed", err)
			}
			if _, err := os.Stat(target); !os.IsNotExist(err) {
				t.Fatal("unsupported operation created a destination", err)
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(original, after) {
				t.Fatal("unsupported operation changed source data", err)
			}
			entries, err := os.ReadDir(source)
			if err != nil || len(entries) != 1 {
				t.Fatal("unsupported operation added source state", err)
			}
		})
	}
}
