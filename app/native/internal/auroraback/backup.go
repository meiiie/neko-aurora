// Package auroraback takes rotating, consistent snapshots of an Aurora state
// directory on any OS (the Windows-only statebackup package covers Wiii desktop).
//
//   - The ledger journal (transactions.jsonl) is append-only, so a snapshot copies
//     it up to its last complete line: a consistent prefix, never a torn record.
//   - Secrets stay encrypted: aurora/vault.json (AES-GCM ciphertext) is copied
//     as-is and aurora/vault.key is NEVER put in an archive. Restoring therefore
//     needs the key from the original host (or the owner re-enters secrets).
//   - Archives are tar.gz, 0600, in <state>/backups, newest `keep` retained.
package auroraback

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const Dir = "backups"

var (
	ErrUnsafe   = errors.New("backup_unsafe_entry")
	ErrNotEmpty = errors.New("restore_target_not_empty")
)

// skip reports state entries that never go into an archive.
func skip(rel string, d fs.DirEntry) bool {
	base := path.Base(rel)
	switch {
	case rel == Dir || strings.HasPrefix(rel, Dir+"/"):
		return true
	case rel == "aurora/vault.key", base == "writer.lock", strings.HasSuffix(base, ".tmp"):
		return true
	case rel == "aurora/core-home" || strings.HasPrefix(rel, "aurora/core-home/"): // Core CLI cache, rebuilt on demand
		return true
	}
	return d.Type()&fs.ModeSymlink != 0
}

// Create writes one snapshot of state into state/backups and prunes to keep.
func Create(state string, keep int, now time.Time) (string, error) {
	dir := filepath.Join(state, Dir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	name := filepath.Join(dir, "aurora-"+now.UTC().Format("20060102T150405Z")+".tar.gz")
	tmp := name + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	werr := filepath.WalkDir(state, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(state, p)
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if skip(rel, d) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return tw.WriteHeader(&tar.Header{Name: rel + "/", Mode: 0o700, Typeflag: tar.TypeDir, ModTime: now})
		}
		if !d.Type().IsRegular() {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if strings.HasSuffix(rel, ".jsonl") { // consistent prefix of an append-only journal
			if i := bytes.LastIndexByte(b, '\n'); i >= 0 {
				b = b[:i+1]
			} else {
				b = nil
			}
		}
		if err := tw.WriteHeader(&tar.Header{Name: rel, Mode: 0o600, Size: int64(len(b)), ModTime: now, Typeflag: tar.TypeReg}); err != nil {
			return err
		}
		_, err = tw.Write(b)
		return err
	})
	if err := errors.Join(werr, tw.Close(), gz.Close(), f.Sync(), f.Close()); err != nil {
		os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, name); err != nil {
		return "", err
	}
	prune(dir, keep)
	return name, nil
}

// List returns archives, newest first.
func List(state string) []string {
	m, _ := filepath.Glob(filepath.Join(state, Dir, "aurora-*.tar.gz"))
	sort.Sort(sort.Reverse(sort.StringSlice(m)))
	return m
}

func prune(dir string, keep int) {
	if keep < 1 {
		keep = 1
	}
	all := List(filepath.Dir(dir))
	for _, old := range all[min(keep, len(all)):] {
		os.Remove(old)
	}
}

// Restore unpacks an archive into an empty (or new) directory. Entries must be
// plain relative paths; anything else aborts before the target is written.
func Restore(archive, dest string) error {
	if ents, err := os.ReadDir(dest); err == nil && len(ents) > 0 {
		return ErrNotEmpty
	}
	type file struct {
		name string
		dir  bool
		data []byte
	}
	var files []file
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	var total int64
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		n := strings.TrimSuffix(h.Name, "/")
		if n == "" || path.IsAbs(n) || path.Clean(n) != n || strings.HasPrefix(n, "../") || n == ".." || strings.ContainsAny(n, "\\:") || n == "aurora/vault.key" {
			return fmt.Errorf("%w: %q", ErrUnsafe, h.Name)
		}
		switch h.Typeflag {
		case tar.TypeDir:
			files = append(files, file{name: n, dir: true})
		case tar.TypeReg:
			if total += h.Size; total > 2<<30 {
				return ErrUnsafe
			}
			b, err := io.ReadAll(io.LimitReader(tr, h.Size))
			if err != nil {
				return err
			}
			files = append(files, file{name: n, data: b})
		default:
			return fmt.Errorf("%w: type %c", ErrUnsafe, h.Typeflag)
		}
	}
	if err := os.MkdirAll(dest, 0o700); err != nil {
		return err
	}
	for _, x := range files {
		p := filepath.Join(dest, filepath.FromSlash(x.name))
		if x.dir {
			if err := os.MkdirAll(p, 0o700); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(p, x.data, 0o600); err != nil {
			return err
		}
	}
	return nil
}

// Loop snapshots at start and then every `every`; AURORA_BACKUP_EVERY=off disables.
func Loop(ctx context.Context, state string, every time.Duration, keep int) {
	run := func() {
		if p, err := Create(state, keep, time.Now()); err != nil {
			log.Printf("aurora backup failed: %v", err)
		} else {
			log.Printf("aurora backup %s", filepath.Base(p))
		}
	}
	run()
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			run()
		}
	}
}
