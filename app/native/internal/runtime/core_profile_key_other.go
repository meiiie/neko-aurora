//go:build !windows

package runtime

func profileKeyCrypt(data, entropy []byte, seal bool) ([]byte, error) {
	return nil, errProfileKey
}
