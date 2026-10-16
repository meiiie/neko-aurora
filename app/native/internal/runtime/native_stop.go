package runtime

import (
	"errors"
	"io"
	"nekobot.local/native/internal/ledger"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func stopIdentifier(s, prefix string) bool {
	if len(s) != len(prefix)+32 || !strings.HasPrefix(s, prefix) {
		return false
	}
	for _, c := range s[len(prefix):] {
		if !(c >= '0' && c <= '9') && !(c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func nativeStopPath(state, id, attempt string) (string, error) {
	if !stopIdentifier(id, "req-") || !stopIdentifier(attempt, "attempt-") {
		return "", ledger.ErrInvalid
	}
	root, e := filepath.EvalSymlinks(state)
	if e != nil {
		return "", e
	}
	dirs := []string{root, filepath.Join(root, "runtime-stops"), filepath.Join(root, "runtime-stops", id)}
	for _, dir := range dirs {
		info, e := os.Lstat(dir)
		if e != nil {
			return "", e
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", ledger.ErrInvalid
		}
	}
	return filepath.Join(dirs[2], attempt+".json"), nil
}
func readNativeStop(state, id, attempt string) (ledger.NativeStopRecord, error) {
	var record ledger.NativeStopRecord
	path, e := nativeStopPath(state, id, attempt)
	if e != nil {
		return record, e
	}
	f, e := openNativeStop(path)
	if e != nil {
		return record, e
	}
	defer f.Close()
	raw, e := io.ReadAll(io.LimitReader(f, ledger.NativeStopMaxBytes+1))
	if e != nil {
		return record, e
	}
	return ledger.ParseNativeStop(raw)
}
func (m *Manager) NativeStopDiagnostic(id string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopIssues[id]
}
func (m *Manager) stopIssue(id, code string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if code == "" {
		delete(m.stopIssues, id)
	} else {
		if m.stopIssues == nil {
			m.stopIssues = map[string]string{}
		}
		m.stopIssues[id] = code
	}
}
func (m *Manager) reconcileNativeStops() {
	if m.executor == nil {
		return
	}
	m.mu.Lock()
	if time.Since(m.lastStopReconcile) < time.Second {
		m.mu.Unlock()
		return
	}
	m.lastStopReconcile = time.Now()
	m.mu.Unlock()
	boot := m.executor.Snapshot().BootID
	for _, w := range m.store.Snapshot().Work {
		if w.RuntimeStopped {
			m.stopIssue(w.RequestID, "")
			continue
		}
		if w.CurrentAttemptLease == nil || w.CurrentAttemptLease.BootID == boot || w.AttemptID == "" {
			continue
		}
		if w.ContextProtocolVersion != 1 || w.RuntimeAttemptBinding == nil || !ledger.ValidProcessGeneration(w.RuntimeAttemptBinding.NativeProcessGeneration) {
			m.stopIssue(w.RequestID, "native_stop_legacy_or_unbound")
			continue
		}
		record, e := readNativeStop(m.state, w.RequestID, w.AttemptID)
		if e != nil {
			code := "native_stop_outbox_invalid"
			if errors.Is(e, os.ErrNotExist) {
				code = "native_stop_outbox_missing"
			}
			m.stopIssue(w.RequestID, code)
			continue
		}
		if e = m.store.ReconcileNativeStop(record); e != nil {
			m.stopIssue(w.RequestID, "native_stop_outbox_mismatch")
			continue
		}
		m.stopIssue(w.RequestID, "")
	}
}
