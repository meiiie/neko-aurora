package aurora

import (
	"os"
	"path/filepath"
	"strings"
)

func loadWorkspaceID(dir, configured string) (string, error) {
	if configured != "" {
		return configured, nil
	}
	path := filepath.Join(dir, "workspace-id")
	b, err := os.ReadFile(path)
	if err == nil {
		id := strings.TrimSpace(string(b))
		if len(id) < 16 || len(id) > 128 {
			return "", ErrInvalid
		}
		return id, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	id := newID("workspace-")
	if err = os.WriteFile(path+".tmp", []byte(id), 0o600); err != nil {
		return "", err
	}
	if err = os.Rename(path+".tmp", path); err != nil {
		return "", err
	}
	return id, nil
}
