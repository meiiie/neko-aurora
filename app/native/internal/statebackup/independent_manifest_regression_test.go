//go:build windows

package statebackup

import (
	"encoding/json"
	"fmt"
	"nekoaurora.local/native/internal/protocol"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIndependentPublishedManifestAboveWireFrameRemainsVerifiable(t *testing.T) {
	source, _ := fixture(t)
	for i := 0; i < 200; i++ {
		name := fmt.Sprintf("checkpoint-%s-%03d", strings.Repeat("x", 80), i)
		if err := os.WriteFile(filepath.Join(source, name), []byte("bounded fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	backup := filepath.Join(t.TempDir(), "many-files-backup")
	m, err := Create(source, backup, "0680b80897e53bb101c96af8781cd622263866d9")
	if err != nil {
		t.Fatal("within-quota create failed", err)
	}
	raw, _ := json.Marshal(m)
	if len(raw) <= protocol.MaxFrame || int64(len(raw)) > maxManifest || len(m.Files) > maxFiles {
		t.Fatal("fixture does not distinguish backup from wire limits")
	}
	if _, err = Verify(backup); err != nil {
		t.Errorf("published within-quota backup cannot verify: files=%d manifestBytes=%d error=%v", len(m.Files), len(raw), err)
	}
	if _, err = RestoreDryRun(backup, filepath.Join(t.TempDir(), "many-files-restored")); err != nil {
		t.Errorf("published within-quota backup cannot restore: %v", err)
	}
}
