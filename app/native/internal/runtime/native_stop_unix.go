//go:build !windows

package runtime

import (
	"nekobot.local/native/internal/ledger"
	"os"
	"syscall"
)

func openNativeStop(path string) (*os.File, error) {
	fd, e := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if e != nil {
		return nil, e
	}
	f := os.NewFile(uintptr(fd), path)
	info, e := f.Stat()
	if e != nil || !info.Mode().IsRegular() || info.Size() > ledger.NativeStopMaxBytes {
		f.Close()
		return nil, ledger.ErrInvalid
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Nlink != 1 {
		f.Close()
		return nil, ledger.ErrInvalid
	}
	return f, nil
}
