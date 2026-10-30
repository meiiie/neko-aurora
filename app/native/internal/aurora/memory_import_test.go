package aurora

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nekoaurora.local/native/internal/ledger"
)

func TestMemoryImportPreviewAndApply(t *testing.T) {
	dir := t.TempDir()
	m := openMemory(dir)
	m.Remember("bot", "Thích câu trả lời ngắn")
	m.mask = func(s string) string { return strings.ReplaceAll(s, "fixture-secret", "[redacted]") }
	p, err := m.previewImport("bot", "Thích câu trả lời ngắn\nDùng tiếng Việt\nDùng tiếng Việt\nfixture-secret\n"+strings.Repeat("x", 301))
	if err != nil || len(p.Items) != 1 || p.Duplicates != 2 || p.Rejected != 2 || len(m.List("bot")) != 1 {
		t.Fatalf("preview counts: %+v, %v", p, err)
	}
	if err := m.applyImport("bot", p.Revision, p.Items); err != nil {
		t.Fatal(err)
	}
	if got := openMemory(dir).List("bot"); len(got) != 2 || got[1].Text != "Dùng tiếng Việt" {
		t.Fatal("import did not persist")
	}
	if err := m.applyImport("bot", p.Revision, p.Items); !errors.Is(err, ledger.ErrConflict) {
		t.Fatal("reused stale review", err)
	}
	if len(m.List("other")) != 0 {
		t.Fatal("wrong bot scope")
	}
}

func TestMemoryImportConflictCapacityAndRollback(t *testing.T) {
	dir := t.TempDir()
	m := openMemory(dir)
	p, _ := m.previewImport("bot", "mục mới")
	m.Remember("bot", "thay đổi ở thiết bị khác")
	if err := m.applyImport("bot", p.Revision, p.Items); !errors.Is(err, ledger.ErrConflict) {
		t.Fatal("stale review accepted")
	}
	for i := 1; i < maxMemPerBot; i++ {
		m.Remember("bot", "điều "+itoa(i))
	}
	p, _ = m.previewImport("bot", "mục mới")
	if p.Remaining != 0 || m.applyImport("bot", p.Revision, p.Items) == nil || len(m.List("bot")) != maxMemPerBot {
		t.Fatal("import evicted existing facts")
	}
	p, _ = m.previewImport("empty", "mục mới")
	if err := os.Mkdir(filepath.Join(dir, "memory.json.tmp"), 0o700); err != nil {
		t.Fatal(err)
	}
	if m.applyImport("empty", p.Revision, p.Items) == nil || len(m.List("empty")) != 0 {
		t.Fatal("failed write changed memory")
	}
	if _, err := m.previewImport("empty", strings.Repeat("a\n", 101)); err == nil {
		t.Fatal("unbounded input accepted")
	}
}
