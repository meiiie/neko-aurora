package vault

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestVaultCorruptOpenKeepsExistingLogRedaction(t *testing.T) {
	dir := t.TempDir()
	v, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { register(v); v.Close() })
	const secret = "fixture-still-private-after-open-failure"
	if _, err := v.Set(Meta{Name: "OPEN_FAILURE_FIXTURE"}, secret); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(v.path, []byte("{broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir); !errors.Is(err, ErrCorrupt) {
		t.Fatal("corrupt vault was accepted")
	}
	var log bytes.Buffer
	if _, err := LogWriter(&log).Write([]byte(secret)); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(log.String(), secret) {
		t.Fatal("failed open replaced the active log redactor")
	}
}

func TestVaultMalformedNonceReturnsCorruptWithoutPanic(t *testing.T) {
	dir := t.TempDir()
	v, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { register(v); v.Close() })
	if _, err := v.Set(Meta{Name: "NONCE_FIXTURE"}, "fixture-private-nonce"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(v.path)
	if err != nil {
		t.Fatal(err)
	}
	var entries []entry
	if err := json.Unmarshal(b, &entries); err != nil {
		t.Fatal(err)
	}
	entries[0].Nonce = []byte("short")
	b, err = json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(v.path, b, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir); !errors.Is(err, ErrCorrupt) {
		t.Fatal("malformed nonce was accepted")
	}
}
