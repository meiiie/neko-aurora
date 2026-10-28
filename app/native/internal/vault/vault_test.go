package vault

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nekoaurora.local/native/internal/privatefs"
)

const pw = "abcd efgh ijkl mnop"

func TestVaultSealsAndPersists(t *testing.T) {
	dir := t.TempDir()
	v, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Set(Meta{Name: "GMAIL_APP_PASSWORD", Label: "Mật khẩu ứng dụng"}, pw); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"vault.key", "vault.json"} {
		if err := privatefs.Check(filepath.Join(dir, f)); err != nil {
			t.Fatalf("%s is not owner-only: %v", f, err)
		}
	}
	filepath.Walk(dir, func(p string, _ os.FileInfo, _ error) error {
		b, _ := os.ReadFile(p)
		if bytes.Contains(b, []byte(pw)) || bytes.Contains(b, []byte("abcdefgh")) {
			t.Fatalf("plaintext on disk in %s", p)
		}
		return nil
	})
	v2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := v2.Get("GMAIL_APP_PASSWORD"); !ok || got != pw {
		t.Fatal("roundtrip failed")
	}
	for _, m := range v2.List() {
		if strings.Contains(m.Name+m.Label+m.Hint, pw) {
			t.Fatal("list leaked value")
		}
	}
	if err := v2.Delete("GMAIL_APP_PASSWORD"); err != nil || v2.Has("GMAIL_APP_PASSWORD") {
		t.Fatal("delete")
	}
	b, _ := os.ReadFile(filepath.Join(dir, "vault.json"))
	if strings.Contains(string(b), "GMAIL_APP_PASSWORD") {
		t.Fatal("deleted entry still on disk")
	}
}

func TestVaultRejectsBadInput(t *testing.T) {
	v, _ := Open(t.TempDir())
	for _, n := range []string{"", "lower", "A", "A-B", strings.Repeat("A", 65)} {
		if _, err := v.Set(Meta{Name: n}, "x"); err != ErrName {
			t.Fatalf("%q accepted", n)
		}
	}
	if _, err := v.Set(Meta{Name: "OK_NAME"}, ""); err != ErrValue {
		t.Fatal("empty value accepted")
	}
}

func TestVaultTamperDetected(t *testing.T) {
	dir := t.TempDir()
	v, _ := Open(dir)
	v.Set(Meta{Name: "SMTP_PASSWORD"}, "s3cretvalue")
	p := filepath.Join(dir, "vault.json")
	b, _ := os.ReadFile(p)
	b = bytes.Replace(b, []byte(`"SMTP_PASSWORD"`), []byte(`"SMTP_PASSWORX"`), 1)
	os.WriteFile(p, b, 0o600)
	if _, err := Open(dir); err != ErrCorrupt {
		t.Fatalf("tamper not detected: %v", err)
	}
}

func TestRedactLogs(t *testing.T) {
	v, _ := Open(t.TempDir())
	v.Set(Meta{Name: "GMAIL_APP_PASSWORD"}, pw)
	v.Set(Meta{Name: "GMAIL_ADDRESS", Kind: "plain"}, "me@example.com")
	var buf bytes.Buffer
	l := log.New(v.Writer(&buf), "", 0)
	l.Printf("smtp auth failed for me@example.com with %s / %s", pw, strings.ReplaceAll(pw, " ", ""))
	out := buf.String()
	if strings.Contains(out, "abcd") || !strings.Contains(out, "«GMAIL_APP_PASSWORD»") || !strings.Contains(out, "me@example.com") {
		t.Fatalf("bad redaction: %q", out)
	}
}

func TestVaultReadsLegacyV0AndWritesV1(t *testing.T) {
	dir := t.TempDir()
	v, _ := Open(dir)
	v.Set(Meta{Name: "OLD_KEY"}, "legacy-value")
	// rewrite the entry as a first-release v0 envelope (AAD = bare name)
	nonce := make([]byte, 12)
	v.mu.Lock()
	e := v.items["OLD_KEY"]
	e.V, e.Nonce, e.CT = 0, nonce, v.aead.Seal(nil, nonce, []byte("legacy-value"), []byte("OLD_KEY"))
	v.items["OLD_KEY"] = e
	v.save()
	v.mu.Unlock()
	v2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := v2.Get("OLD_KEY"); got != "legacy-value" {
		t.Fatal("legacy read")
	}
	b, _ := os.ReadFile(filepath.Join(dir, "vault.json"))
	v2.Set(Meta{Name: "NEW_KEY"}, "fresh-value")
	b, _ = os.ReadFile(filepath.Join(dir, "vault.json"))
	if !strings.Contains(string(b), `"v": 1`) {
		t.Fatalf("new entry not v1: %s", b)
	}
}
