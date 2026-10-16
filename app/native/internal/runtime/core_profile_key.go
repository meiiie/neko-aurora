package runtime

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
)

var errProfileKey = errors.New("core_profile_key_unavailable")

func profileRevisionKey(dir, binding string) (string, error) {
	if binding == "" || len(binding) > 1024 {
		return "", errProfileKey
	}
	path := filepath.Join(dir, "revision-key.dpapi")
	info, err := os.Lstat(path)
	if err == nil {
		if !info.Mode().IsRegular() || info.Size() > 8192 {
			return "", errProfileKey
		}
		cipher, e := os.ReadFile(path)
		if e != nil {
			return "", errProfileKey
		}
		plain, e := profileKeyCrypt(cipher, []byte("neko-profile-revision-v1\x00"+binding), false)
		if e != nil || len(plain) != 32 {
			clear(plain)
			return "", errProfileKey
		}
		key := hex.EncodeToString(plain)
		clear(plain)
		return key, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", errProfileKey
	}
	var plain [32]byte
	if _, err = io.ReadFull(rand.Reader, plain[:]); err != nil {
		return "", errProfileKey
	}
	defer clear(plain[:])
	cipher, err := profileKeyCrypt(plain[:], []byte("neko-profile-revision-v1\x00"+binding), true)
	if err != nil {
		return "", errProfileKey
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", errProfileKey
	}
	_, writeErr := file.Write(cipher)
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		return "", errProfileKey
	}
	return hex.EncodeToString(plain[:]), nil
}
