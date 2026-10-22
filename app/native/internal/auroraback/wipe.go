package auroraback

import (
	"os"
	"path/filepath"
)

// WipeMarker is written before an account deletion so an interrupted wipe
// finishes on the next start instead of leaving half the data behind.
const WipeMarker = ".delete-account"

// RequestWipe records that the owner asked to delete everything in state.
func RequestWipe(state string) error {
	return os.WriteFile(filepath.Join(state, WipeMarker), []byte("1\n"), 0o600)
}

// WipeIfRequested removes every entry of state (ledger, Aurora data, vault
// and key, connections, backups) when the marker is present. The marker goes
// last so a crash mid-wipe repeats it. It reports whether a wipe happened.
func WipeIfRequested(state string) (bool, error) {
	if _, err := os.Stat(filepath.Join(state, WipeMarker)); err != nil {
		return false, nil
	}
	ents, err := os.ReadDir(state)
	if err != nil {
		return false, err
	}
	for _, e := range ents {
		if e.Name() == WipeMarker {
			continue
		}
		if err := os.RemoveAll(filepath.Join(state, e.Name())); err != nil {
			return false, err
		}
	}
	return true, os.Remove(filepath.Join(state, WipeMarker))
}
