//go:build !windows

package executors

import (
	"os"
	"path/filepath"
	"syscall"
)

func lockWriter(path string) (*os.File, error) {
	fd, e := syscall.Open(path, syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if e != nil {
		return nil, e
	}
	f := os.NewFile(uintptr(fd), path)
	if e = syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		f.Close()
		return nil, e
	}
	return f, nil
}
func singleFile(f *os.File) bool {
	info, e := f.Stat()
	if e != nil || !info.Mode().IsRegular() {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Nlink == 1
}
func replaceFile(from, to string) error {
	if e := os.Rename(from, to); e != nil {
		return e
	}
	dir, e := os.Open(filepath.Dir(to))
	if e != nil {
		return e
	}
	defer dir.Close()
	return dir.Sync()
}
