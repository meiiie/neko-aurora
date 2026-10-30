package computer

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestCreateFileOnceConcurrentPublishAndQuota(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, NewBwrap())
	if err != nil {
		t.Fatal(err)
	}
	m.Limits.HomeBytes = 5
	var wg sync.WaitGroup
	results := make(chan error, 12)
	for range 12 {
		wg.Add(1)
		go func() { defer wg.Done(); results <- m.CreateFileOnce("bot", "uploads/one.jpg", []byte("hello")) }()
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := m.CreateFileOnce("bot", "uploads/two.jpg", []byte("x")); !errors.Is(err, ErrQuota) {
		t.Fatal("new bytes bypassed quota", err)
	}
	if err := m.CreateFileOnce("bot", "uploads/one.jpg", []byte("other")); !errors.Is(err, ErrFileConflict) {
		t.Fatal("content conflict", err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "bot", "home", "uploads", "one.jpg"))
	if err != nil || string(b) != "hello" {
		t.Fatal("original content", err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, "bot", "home", "uploads"))
	if err != nil || len(entries) != 1 {
		t.Fatal("partial files leaked", err)
	}
	if err := m.CreateFileOnce("bot", "../outside", []byte("x")); !errors.Is(err, ErrPath) {
		t.Fatal("path escape", err)
	}
}

func TestCreateFileOnceTwoManagersNeverReplaceWinner(t *testing.T) {
	dir := t.TempDir()
	a, _ := New(dir, NewBwrap())
	b, _ := New(dir, NewBwrap())
	start := make(chan struct{})
	results := make(chan error, 2)
	for i, m := range []*Manager{a, b} {
		go func() { <-start; results <- m.CreateFileOnce("bot", "uploads/winner.jpg", []byte{byte(i)}) }()
	}
	close(start)
	x, y := <-results, <-results
	if !((x == nil && errors.Is(y, ErrFileConflict)) || (y == nil && errors.Is(x, ErrFileConflict))) {
		t.Fatal("expected exactly one publisher", x, y)
	}
}

func TestCreateFileOnceRejectsSymlinkReceipt(t *testing.T) {
	m, err := New(t.TempDir(), NewBwrap())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.WriteFile("bot", "original", "same"); err != nil {
		t.Fatal(err)
	}
	h, _ := m.home("bot")
	if err := os.Symlink(filepath.Join(h, "original"), filepath.Join(h, "link")); err != nil {
		t.Skip("host cannot create symlinks:", err)
	}
	if err := m.CreateFileOnce("bot", "link", []byte("same")); !errors.Is(err, ErrFileConflict) {
		t.Fatal("symlink accepted as receipt", err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("same"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(h, "escape")); err != nil {
		t.Fatal(err)
	}
	if err := m.CreateFileOnce("bot", "escape", []byte("same")); !errors.Is(err, ErrFileConflict) {
		t.Fatal("outside link accepted", err)
	}
}
