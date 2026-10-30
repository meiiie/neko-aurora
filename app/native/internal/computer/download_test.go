package computer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"nekoaurora.local/native/internal/privatefs"
)

type changingDownloadReader struct {
	io.ReadSeeker
	change func()
	second bool
	phase  int
}

func (r *changingDownloadReader) Read(b []byte) (int, error) {
	n, err := r.ReadSeeker.Read(b)
	if n > 0 && r.change != nil && (!r.second || r.phase == 2) {
		f := r.change
		r.change = nil
		f()
	}
	return n, err
}
func (r *changingDownloadReader) Seek(n int64, w int) (int64, error) {
	r.phase = 2
	return r.ReadSeeker.Seek(n, w)
}

type downloadErrorWriter struct{ err error }

func (w downloadErrorWriter) Write([]byte) (int, error) { return 0, w.err }

func TestDownloadRejectsSourceChanges(t *testing.T) {
	for _, kind := range []string{"grow", "shrink", "same-size", "restored-mtime", "second-pass", "replace", "remove"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			name := filepath.Join(dir, "source")
			original := bytes.Repeat([]byte("A"), 128<<10)
			if err := os.WriteFile(name, original, 0600); err != nil {
				t.Fatal(err)
			}
			src, err := openDownloadTestSource(name)
			if err != nil {
				t.Fatal(err)
			}
			defer src.Close()
			before, _ := src.Stat()
			change := func() {
				data := bytes.Repeat([]byte("B"), len(original))
				if kind == "grow" {
					data = append(data, 'B')
				}
				if kind == "shrink" {
					data = data[:32]
				}
				if kind == "remove" {
					if err := os.Remove(name); err != nil {
						t.Fatal(err)
					}
					return
				}
				if kind == "replace" {
					if err := os.Rename(name, name+".old"); err != nil {
						t.Fatal(err)
					}
				}
				if err := os.WriteFile(name, data, 0600); err != nil {
					t.Fatal(err)
				}
				stamp := before.ModTime().Add(time.Second)
				if kind == "restored-mtime" || kind == "second-pass" {
					stamp = before.ModTime()
				}
				if err := os.Chtimes(name, stamp, stamp); err != nil {
					t.Fatal(err)
				}
			}
			r := &changingDownloadReader{ReadSeeker: src, change: change, second: kind == "second-pass"}
			var dst bytes.Buffer
			_, err = stageDownload(context.Background(), &dst, r, before, func() (os.FileInfo, error) { return os.Stat(name) })
			if !errors.Is(err, ErrDownloadChanged) {
				t.Fatal("changed source accepted", kind, err)
			}
		})
	}
}
func TestDownloadStableSnapshotAndCleanup(t *testing.T) {
	m, err := New(t.TempDir(), NewBwrap())
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range [][]byte{nil, []byte("private fixture bytes"), bytes.Repeat([]byte{0, 255, 1, 9}, 35000)} {
		if err := m.CreateFileOnce("bot", "report.bin", data); err != nil {
			h, _ := m.home("bot")
			if err := os.WriteFile(filepath.Join(h, "report.bin"), data, 0600); err != nil {
				t.Fatal(err)
			}
		}
		d, err := m.OpenDownload(context.Background(), "bot", "report.bin")
		if err != nil {
			t.Fatal(err)
		}
		path := d.Name()
		if err := privatefs.Check(path); err != nil {
			t.Fatal("snapshot not private", err)
		}
		h, _ := m.home("bot")
		if err := os.WriteFile(filepath.Join(h, "report.bin"), []byte("later version"), 0600); err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(d)
		sum := sha256.Sum256(data)
		if err != nil || !bytes.Equal(got, data) || d.Size != int64(len(data)) || d.SHA256 != hex.EncodeToString(sum[:]) || d.Filename != "report.bin" {
			t.Fatal("snapshot changed", err)
		}
		if err := d.Close(); err != nil {
			t.Fatal(err)
		}
		if err := d.Close(); err != nil {
			t.Fatal("close not idempotent", err)
		}
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("snapshot not removed", err)
		}
	}
}
func TestDownloadAdmissionPathsAndCancellation(t *testing.T) {
	m, err := New(t.TempDir(), NewBwrap())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.CreateFileOnce("bot", "file", []byte("fixture")); err != nil {
		t.Fatal(err)
	}
	a, err := m.OpenDownload(context.Background(), "bot", "file")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := m.OpenDownload(context.Background(), "bot", "file")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if _, err := m.OpenDownload(context.Background(), "bot", "file"); !errors.Is(err, ErrDownloadBusy) {
		t.Fatal("unbounded downloads", err)
	}
	a.Close()
	b.Close()
	for _, path := range []string{"../../outside", "missing", "."} {
		if d, err := m.OpenDownload(context.Background(), "bot", path); !errors.Is(err, ErrPath) {
			if d != nil {
				d.Close()
			}
			t.Fatal("invalid path accepted", path, err)
		}
	}
	m.Limits.HomeBytes = 3
	if _, err := m.OpenDownload(context.Background(), "bot", "file"); !errors.Is(err, ErrTooLarge) {
		t.Fatal("size bound ignored", err)
	}
	m.Limits.HomeBytes = 1 << 20
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.OpenDownload(ctx, "bot", "file"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	d, err := m.OpenDownload(context.Background(), "bot", "file")
	if err != nil {
		t.Fatal("failed paths leaked admission", err)
	}
	d.Close()
}
func TestDownloadCopyErrorsAndMidReadCancellation(t *testing.T) {
	name := filepath.Join(t.TempDir(), "file")
	os.WriteFile(name, bytes.Repeat([]byte("A"), 128<<10), 0600)
	src, err := os.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	before, _ := src.Stat()
	stat := func() (os.FileInfo, error) { return os.Stat(name) }
	want := errors.New("fixture disk full")
	if _, err := stageDownload(context.Background(), downloadErrorWriter{want}, src, before, stat); !errors.Is(err, want) {
		t.Fatal("write failure ignored", err)
	}
	src.Seek(0, io.SeekStart)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := &changingDownloadReader{ReadSeeker: src, change: cancel}
	if _, err := stageDownload(ctx, io.Discard, r, before, stat); !errors.Is(err, context.Canceled) {
		t.Fatal("mid-read cancellation ignored", err)
	}
}

type downloadCancelAfterTemp struct {
	context.Context
	dir    string
	cancel context.CancelFunc
}

func (c downloadCancelAfterTemp) Err() error {
	entries, _ := os.ReadDir(c.dir)
	if len(entries) > 0 {
		c.cancel()
	}
	return c.Context.Err()
}
func TestDownloadCancelledPreparationRemovesPrivateTemp(t *testing.T) {
	m, err := New(t.TempDir(), NewBwrap())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.CreateFileOnce("bot", "file", bytes.Repeat([]byte("x"), 4096)); err != nil {
		t.Fatal(err)
	}
	stageDir := t.TempDir()
	for _, key := range []string{"TMP", "TEMP", "TMPDIR"} {
		t.Setenv(key, stageDir)
	}
	if filepath.Clean(os.TempDir()) != filepath.Clean(stageDir) {
		t.Fatal("temporary directory isolation failed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, err := m.OpenDownload(downloadCancelAfterTemp{ctx, stageDir, cancel}, "bot", "file"); !errors.Is(err, context.Canceled) {
		t.Fatal("preparation not cancelled", err)
	}
	entries, err := os.ReadDir(stageDir)
	if err != nil || len(entries) != 0 {
		t.Fatal("cancelled snapshot left data", err, len(entries))
	}
	a, err := m.OpenDownload(context.Background(), "bot", "file")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := m.OpenDownload(context.Background(), "bot", "file")
	if err != nil {
		t.Fatal("cancelled snapshot kept admission", err)
	}
	b.Close()
	a.Close()
}
