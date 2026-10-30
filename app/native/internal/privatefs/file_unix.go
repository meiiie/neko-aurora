//go:build !windows

package privatefs

import "os"

func CreateNew(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
}

func Protect(path string) error {
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() {
		return ErrPrivate
	}
	return os.Chmod(path, 0o600)
}

func Check(path string) error {
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() || fi.Mode().Perm() != 0o600 {
		return ErrPrivate
	}
	return nil
}
