package computer

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path"
)

var ErrFileConflict = errors.New("file_content_conflict")

// CreateFileOnce publishes complete bytes without replacing a prior attachment.
// A retry succeeds only while the existing regular file still has those bytes.
func (m *Manager) CreateFileOnce(bot, p string, content []byte) error {
	if len(content) > maxWriteBytes {
		return ErrTooLarge
	}
	rel, err := cleanRel(p)
	if err != nil || rel == "." {
		return ErrPath
	}
	unlock := m.lock(bot)
	defer unlock()
	r, err := m.root(bot)
	if err != nil {
		return err
	}
	defer r.Close()
	check := func() error {
		info, err := r.Lstat(rel)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() != int64(len(content)) {
			return ErrFileConflict
		}
		f, err := r.Open(rel)
		if err != nil {
			return ErrFileConflict
		}
		defer f.Close()
		opened, err := f.Stat()
		if err != nil || !os.SameFile(info, opened) {
			return ErrFileConflict
		}
		b, err := io.ReadAll(io.LimitReader(f, int64(len(content))+1))
		if err != nil || !bytes.Equal(b, content) {
			return ErrFileConflict
		}
		return nil
	}
	if err := check(); !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := r.MkdirAll(path.Dir(rel), 0o700); err != nil {
		return ErrPath
	}
	h, err := m.home(bot)
	if err != nil {
		return err
	}
	used, err := dirSize(h)
	if err != nil {
		return err
	}
	if used+int64(len(content)) > m.Limits.HomeBytes {
		return ErrQuota
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return err
	}
	tmp := path.Join(path.Dir(rel), ".upload-"+hex.EncodeToString(nonce[:]))
	f, err := r.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer r.Remove(tmp)
	if _, err = f.Write(content); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	// Link is exclusive, unlike Rename on Unix. A competing publisher can win,
	// but it can never have its file overwritten by this upload.
	if err := r.Link(tmp, rel); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	return check()
}
