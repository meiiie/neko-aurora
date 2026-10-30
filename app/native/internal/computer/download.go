package computer

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"sync"

	"nekoaurora.local/native/internal/privatefs"
)

var ErrDownloadChanged = errors.New("download_source_changed")
var ErrDownloadBusy = errors.New("download_busy")

// Download owns a checked snapshot, independent of subsequent source writes.
// Close removes its private temporary file and releases the admission slot.
type Download struct {
	*os.File
	Filename string
	Size     int64
	SHA256   string
	once     sync.Once
	cleanup  func() error
	closeErr error
}

func (d *Download) Close() error {
	d.once.Do(func() { d.closeErr = d.cleanup() })
	return d.closeErr
}

func (m *Manager) OpenDownload(ctx context.Context, bot, p string) (*Download, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case m.downloads <- struct{}{}:
	default:
		return nil, ErrDownloadBusy
	}
	release := func() { <-m.downloads }
	retained := false
	defer func() {
		if !retained {
			release()
		}
	}()
	rel, err := cleanRel(p)
	if err != nil {
		return nil, err
	}
	root, err := m.root(bot)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	src, err := root.Open(rel)
	if err != nil {
		return nil, ErrPath
	}
	defer src.Close()
	before, err := src.Stat()
	if err != nil || !before.Mode().IsRegular() {
		return nil, ErrPath
	}
	limit := m.Limits.HomeBytes
	if limit <= 0 {
		limit = DefaultLimits().HomeBytes
	}
	if before.Size() < 0 || before.Size() > limit {
		return nil, ErrTooLarge
	}
	var nonce [16]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return nil, err
	}
	tmp, err := privatefs.CreateNew(filepath.Join(os.TempDir(), "aurora-download-"+hex.EncodeToString(nonce[:])))
	if err != nil {
		return nil, err
	}
	defer func() {
		if !retained {
			tmp.Close()
			os.Remove(tmp.Name())
		}
	}()
	current := func() (os.FileInfo, error) {
		opened, err := src.Stat()
		if err != nil || !sameDownloadFile(before, opened) {
			return nil, ErrDownloadChanged
		}
		now, err := root.Stat(rel)
		if err != nil {
			return nil, ErrDownloadChanged
		}
		return now, nil
	}
	digest, err := stageDownload(ctx, tmp, src, before, current)
	if err != nil {
		return nil, err
	}
	if _, err = tmp.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	retained = true
	return &Download{File: tmp, Filename: path.Base(rel), Size: before.Size(), SHA256: digest, cleanup: func() error {
		defer release()
		return errors.Join(tmp.Close(), os.Remove(tmp.Name()))
	}}, nil
}

func sameDownloadFile(a, b os.FileInfo) bool {
	return b != nil && b.Mode().IsRegular() && os.SameFile(a, b) && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime())
}

type downloadContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r downloadContextReader) Read(b []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(b)
}

func stageDownload(ctx context.Context, dst io.Writer, src io.ReadSeeker, before os.FileInfo, current func() (os.FileInfo, error)) (string, error) {
	copyHash := func(w io.Writer) (string, error) {
		h := sha256.New()
		n, err := io.Copy(io.MultiWriter(w, h), io.LimitReader(downloadContextReader{ctx, src}, before.Size()+1))
		if err != nil {
			return "", err
		}
		if n != before.Size() {
			return "", ErrDownloadChanged
		}
		now, err := current()
		if err != nil || !sameDownloadFile(before, now) {
			return "", ErrDownloadChanged
		}
		return hex.EncodeToString(h.Sum(nil)), ctx.Err()
	}
	first, err := copyHash(dst)
	if err != nil {
		return "", err
	}
	if _, err = src.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	second, err := copyHash(io.Discard)
	if err != nil {
		return "", err
	}
	if first != second {
		return "", ErrDownloadChanged
	}
	return first, nil
}
