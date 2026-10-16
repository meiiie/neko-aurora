//go:build !windows

package providerexec

import (
	"io"
	"os"
	"syscall"
)

func privateDirectoryIdentity(path string) bool {
	st, err := os.Lstat(path)
	return err == nil && st.IsDir() && st.Mode()&os.ModeSymlink == 0
}
func witnessSingleLink(st os.FileInfo) bool {
	s, ok := st.Sys().(*syscall.Stat_t)
	return ok && s.Nlink == 1
}
func readPrivateWitness(path string) ([]byte, error) {
	if _, err := privateWitnessPath(path); err != nil {
		return nil, err
	}
	before, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || !witnessSingleLink(before) || before.Size() > 4096 {
		return nil, ErrSavedAuthority
	}
	// A regular file can be replaced after Lstat. Never block on a FIFO or
	// follow a replacement symlink before checking the opened identity.
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !opened.Mode().IsRegular() || opened.Size() > 4096 || !os.SameFile(before, opened) || !witnessSingleLink(opened) {
		return nil, ErrSavedAuthority
	}
	raw, err := io.ReadAll(io.LimitReader(f, 4097))
	after, e1 := f.Stat()
	named, e2 := os.Lstat(path)
	if err != nil || e1 != nil || e2 != nil || len(raw) > 4096 || !os.SameFile(opened, named) || opened.Size() != after.Size() || opened.ModTime() != after.ModTime() {
		return nil, ErrSavedAuthority
	}
	return raw, nil
}
