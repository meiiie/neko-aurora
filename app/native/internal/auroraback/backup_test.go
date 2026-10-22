package auroraback

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func write(t *testing.T, p, s string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(p), 0o700)
	if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotRestoreRotateAndKeyNeverArchived(t *testing.T) {
	st := t.TempDir()
	write(t, filepath.Join(st, "transactions.jsonl"), "{\"a\":1}\n{\"b\":2}\n{\"torn\":")
	write(t, filepath.Join(st, "aurora", "vault.json"), "CIPHERTEXT")
	write(t, filepath.Join(st, "aurora", "vault.key"), "SECRET-KEY")
	write(t, filepath.Join(st, "aurora", "vapid.pem"), "pem")
	write(t, filepath.Join(st, "aurora", "core-home", "cache"), "x")
	write(t, filepath.Join(st, "writer.lock"), "")
	t0 := time.Date(2026, 10, 23, 1, 0, 0, 0, time.UTC)
	var last string
	for i := 0; i < 5; i++ {
		p, err := Create(st, 3, t0.Add(time.Duration(i)*time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		last = p
		if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o600 {
			t.Fatal("archive not private")
		}
	}
	if l := List(st); len(l) != 3 || l[0] != last {
		t.Fatalf("rotation %v", l)
	}
	dst := filepath.Join(t.TempDir(), "restored")
	if err := Restore(last, dst); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dst, "transactions.jsonl"))
	v, _ := os.ReadFile(filepath.Join(dst, "aurora", "vault.json"))
	if string(b) != "{\"a\":1}\n{\"b\":2}\n" || string(v) != "CIPHERTEXT" {
		t.Fatalf("%q %q", b, v)
	}
	for _, no := range []string{"aurora/vault.key", "writer.lock", "aurora/core-home/cache", "backups"} {
		if _, err := os.Stat(filepath.Join(dst, no)); err == nil {
			t.Fatalf("%s restored", no)
		}
	}
	if err := Restore(last, dst); !errors.Is(err, ErrNotEmpty) {
		t.Fatal("restore over existing state")
	}
}

func TestRestoreRefusesTraversalBeforeWriting(t *testing.T) {
	for _, name := range []string{"../evil", "/abs", "a/../../b", "aurora/vault.key", "c:x"} {
		arc := filepath.Join(t.TempDir(), "x.tar.gz")
		f, _ := os.Create(arc)
		gz := gzip.NewWriter(f)
		tw := tar.NewWriter(gz)
		tw.WriteHeader(&tar.Header{Name: "ok.txt", Mode: 0o600, Size: 1, Typeflag: tar.TypeReg})
		tw.Write([]byte("1"))
		tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: 1, Typeflag: tar.TypeReg})
		tw.Write([]byte("1"))
		tw.Close()
		gz.Close()
		f.Close()
		dst := filepath.Join(t.TempDir(), "d")
		if err := Restore(arc, dst); !errors.Is(err, ErrUnsafe) {
			t.Fatalf("%s: %v", name, err)
		}
		if _, err := os.Stat(dst); err == nil {
			t.Fatalf("%s: target created", name)
		}
	}
}
