//go:build windows

package statebackup

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDirectoryAliasesAreRejectedBeforeFilesystemMutation(t *testing.T) {
	source, _ := fixture(t)
	backup := filepath.Join(t.TempDir(), "original-backup")
	if _, err := Create(source, backup, "working-tree"); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{".", " ", "..", "~1"} {
		t.Run(suffix, func(t *testing.T) {
			destination := filepath.Join(backup+suffix, "unexpected-child")
			if _, err := RestoreDryRun(backup, destination); !errors.Is(err, ErrUnsafe) {
				t.Fatal("restore alias accepted", err)
			}
			if _, err := Create(source, filepath.Join(source+suffix, "unexpected-backup"), "working-tree"); !errors.Is(err, ErrUnsafe) {
				t.Fatal("create alias accepted", err)
			}
			if _, err := Create(source+suffix, filepath.Join(t.TempDir(), "new-backup"), "working-tree"); !errors.Is(err, ErrUnsafe) {
				t.Fatal("source alias accepted", err)
			}
			if _, err := Verify(backup + suffix); !errors.Is(err, ErrUnsafe) {
				t.Fatal("verify alias accepted", err)
			}
		})
	}
	if _, err := os.Stat(filepath.Join(backup, "unexpected-child")); !os.IsNotExist(err) {
		t.Fatal("original backup mutated")
	}
	if _, err := Verify(backup); err != nil {
		t.Fatal("original backup no longer valid", err)
	}
	if _, err := safeDirectory(filepath.Join(t.TempDir(), "normal interior space"), false); err != nil {
		t.Fatal("ordinary directory space refused", err)
	}
}

func TestDestinationChecksPhysicalDirectoryIdentity(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "IdentityRoot")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	first, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	alias, err := os.Stat(filepath.Join(parent, "IDENTITYROOT"))
	if err != nil || !os.SameFile(first, alias) {
		t.Fatal("Windows directory identity not recognized", err)
	}
	if _, err := newDestination(filepath.Join(parent, "IDENTITYROOT", "nested"), root); !errors.Is(err, ErrUnsafe) {
		t.Fatal("same physical ancestor accepted", err)
	}
	if _, err := newDestination(filepath.Join(parent, "distinct-new-root"), root); err != nil {
		t.Fatal("disjoint sibling refused", err)
	}
}
