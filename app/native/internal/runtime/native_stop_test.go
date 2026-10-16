package runtime

import (
	"encoding/json"
	"nekobot.local/native/internal/ledger"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeStopConsumerFilesystemBoundary(t *testing.T) {
	id, attempt := "req-"+strings.Repeat("a", 32), "attempt-"+strings.Repeat("b", 32)
	for _, kind := range []string{"valid", "missing", "partial", "oversize", "directory", "hardlink", "symlink", "escape"} {
		t.Run(kind, func(t *testing.T) {
			state := t.TempDir()
			dir := filepath.Join(state, "runtime-stops", id)
			os.MkdirAll(dir, 0700)
			path := filepath.Join(dir, attempt+".json")
			raw, _ := json.Marshal(ledger.NativeStopRecord{Version: 1, RequestID: id, AttemptID: attempt})
			switch kind {
			case "missing":
			case "partial":
				os.WriteFile(path, raw[:len(raw)-1], 0600)
			case "oversize":
				os.WriteFile(path, []byte(strings.Repeat(" ", ledger.NativeStopMaxBytes+1)), 0600)
			case "directory":
				os.Mkdir(path, 0700)
			case "hardlink":
				os.WriteFile(path, raw, 0600)
				if e := os.Link(path, filepath.Join(state, "other.json")); e != nil {
					t.Fatal(e)
				}
			case "symlink":
				target := filepath.Join(state, "other.json")
				os.WriteFile(target, raw, 0600)
				if e := os.Symlink(target, path); e != nil {
					t.Skip("OS symlink privilege unavailable; hardlink and resolved-handle checks still run")
				}
			case "escape":
				id = "../outside"
			default:
				os.WriteFile(path, raw, 0600)
			}
			_, e := readNativeStop(state, id, attempt)
			if (kind == "valid") != (e == nil) {
				t.Fatal("file boundary", kind, e)
			}
		})
	}
}
func TestNativeStopDiagnosticIsTaskLocal(t *testing.T) {
	m := &Manager{}
	m.stopIssue("request-a", "native_stop_outbox_invalid")
	if m.NativeStopDiagnostic("request-b") != "" || m.NativeStopDiagnostic("request-a") == "" {
		t.Fatal("diagnostic escaped scope")
	}
	m.stopIssue("request-a", "")
	if m.NativeStopDiagnostic("request-a") != "" {
		t.Fatal("stale diagnostic")
	}
}
