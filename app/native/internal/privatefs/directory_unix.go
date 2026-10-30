//go:build !windows

package privatefs

import "os"

func ProtectDirectory(path string) error {
	st, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !st.IsDir() {
		return ErrPrivate
	}
	return os.Chmod(path, 0700)
}
func CheckDirectory(path string) error {
	st, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !st.IsDir() || st.Mode().Perm() != 0700 {
		return ErrPrivate
	}
	return nil
}
