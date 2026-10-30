// Package privatefs creates owner-only files before any sensitive bytes are written.
package privatefs

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
)

var ErrPrivate = errors.New("file_not_private")

func WriteFile(path string, data []byte) error {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return err
	}
	tmp := path + "-" + hex.EncodeToString(nonce[:]) + ".tmp"
	f, err := CreateNew(tmp)
	if err != nil {
		return err
	}
	defer os.Remove(tmp)
	_, err = f.Write(data)
	if err = errors.Join(err, f.Sync(), f.Close()); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
