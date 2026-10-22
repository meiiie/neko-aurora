//go:build !windows

package statebackup

import (
	"errors"
	"testing"
)

// The Wiii state backup relies on Windows DPAPI and pinned handles; elsewhere it
// must refuse cleanly instead of writing an unprotected archive.
func TestBackupRefusesOffWindows(t *testing.T) {
	if _, err := Create(t.TempDir(), t.TempDir()+"/b", "working-tree"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("got %v", err)
	}
}
