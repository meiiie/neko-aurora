package ledger

import (
	"bytes"
	"encoding/json"
	"io"
	"nekoaurora.local/native/internal/protocol"
	"os"
	"path/filepath"
)

func restoredReadOnly(dir string) (bool, error) {
	path := filepath.Join(dir, "restore-readonly.json")
	info, e := os.Lstat(path)
	if os.IsNotExist(e) {
		return false, nil
	}
	if e != nil || !info.Mode().IsRegular() || info.Size() > 1024 {
		return false, ErrUnavailable
	}
	raw, e := os.ReadFile(path)
	if e != nil {
		return false, ErrUnavailable
	}
	raw, e = protocol.Canonical(raw)
	if e != nil {
		return false, ErrUnavailable
	}
	var marker struct {
		Version           int    `json:"version"`
		ExecutionDisabled bool   `json:"executionDisabled"`
		ManifestSHA256    string `json:"manifestSHA256"`
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&marker) != nil || marker.Version != 1 || !marker.ExecutionDisabled || !lowerHex(marker.ManifestSHA256, 64) {
		return false, ErrUnavailable
	}
	var more any
	if d.Decode(&more) != io.EOF {
		return false, ErrUnavailable
	}
	return true, nil
}
