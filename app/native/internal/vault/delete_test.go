package vault

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestVaultDeleteFailurePreservesMemoryDiskAndRedaction(t *testing.T) {
	for _, stage := range []string{"create", "replace"} {
		t.Run(stage, func(t *testing.T) {
			dir := t.TempDir()
			v, err := Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer v.Close()
			const name, secret = "MCP_DELETE_FIXTURE", "fixture-delete-value-private"
			meta, err := v.Set(Meta{Name: name, Label: "Fixture credential"}, secret)
			if err != nil {
				t.Fatal(err)
			}
			originalPath := v.path
			before, err := os.ReadFile(originalPath)
			if err != nil {
				t.Fatal(err)
			}
			if stage == "create" {
				blocker := filepath.Join(t.TempDir(), "regular-file")
				if err := os.WriteFile(blocker, []byte("fixture"), 0600); err != nil {
					t.Fatal(err)
				}
				v.path = filepath.Join(blocker, "vault.json")
			} else {
				v.path = t.TempDir()
			}
			err = v.Delete(name)
			v.path = originalPath
			if err == nil {
				t.Fatal("fixture did not fail vault save")
			}
			if value, ok := v.Get(name); !ok || value != secret || !v.Has(name) {
				t.Fatal("failed deletion removed live credential")
			}
			if current, ok := v.Meta(name); !ok || !reflect.DeepEqual(meta, current) || len(v.List()) != 1 {
				t.Fatal("failed deletion changed credential metadata")
			}
			var log bytes.Buffer
			if _, err := v.Writer(&log).Write([]byte("test " + secret)); err != nil || strings.Contains(log.String(), secret) {
				t.Fatal("failed deletion stopped log redaction")
			}
			after, err := os.ReadFile(originalPath)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("failed deletion changed original stored bytes")
			}
			reopened, err := Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			if value, ok := reopened.Get(name); !ok || value != secret {
				t.Fatal("reopened credential differs after failed deletion")
			}
			if err := reopened.Delete(name); err != nil || reopened.Has(name) {
				t.Fatal("explicit retry did not delete credential")
			}
			final, err := Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer final.Close()
			if final.Has(name) {
				t.Fatal("successful deletion did not survive reopen")
			}
		})
	}
}
