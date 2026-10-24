package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Fake data only: 4242… is the public Stripe test card.
func TestItemsSealedMaskedAndListedWithoutValues(t *testing.T) {
	dir := t.TempDir()
	key := make([]byte, 32)
	v, err := OpenWithKey(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.SaveItem("", "card", "", map[string]string{"number": "4242 4242 4242 4241"}); err == nil {
		t.Fatal("bad luhn accepted")
	}
	c, err := v.SaveItem("", "card", "", map[string]string{"number": "4242 4242 4242 4242", "name": "Test Owner", "exp": "12/34", "cvc": "123"})
	if err != nil {
		t.Fatal(err)
	}
	if c.Label != "Visa •••• 4242" || c.Hint["last4"] != "4242" {
		t.Fatalf("meta %+v", c)
	}
	l, err := v.SaveItem("", "login", "Shop test", map[string]string{"site": "shop.example", "username": "tester@example.com", "password": "hunter2-fake-pass", "totp": "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"})
	if err != nil {
		t.Fatal(err)
	}
	if l.Hint["user"] != "te•••@example.com" || l.Hint["site"] != "shop.example" {
		t.Fatalf("login hint %+v", l.Hint)
	}
	if len(v.List()) != 0 || len(v.Items()) != 2 {
		t.Fatalf("typed items must not show as named secrets: %v %v", v.List(), v.Items())
	}
	// edit keeps values left empty
	if _, err := v.SaveItem(l.ID, "login", "Shop", map[string]string{"password": ""}); err != nil {
		t.Fatal(err)
	}
	if f, _ := v.Fields(l.ID); f["password"] != "hunter2-fake-pass" {
		t.Fatalf("edit lost password: %v", f)
	}
	out := v.Redact("card 4242 4242 4242 4242 / 4242-4242-4242-4242 / 4242424242424242 cvc: 123 order 123 pw hunter2-fake-pass site shop.example")
	for _, bad := range []string{"4242 4242", "4242-4242", "4242424242424242", "cvc: 123", "hunter2"} {
		if strings.Contains(out, bad) {
			t.Fatalf("leak %q in %s", bad, out)
		}
	}
	if !strings.Contains(out, "order 123") || !strings.Contains(out, "shop.example") {
		t.Fatalf("over-redacted: %s", out)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "vault.json"))
	if strings.Contains(string(b), "4242424242424242") || strings.Contains(string(b), "hunter2") {
		t.Fatal("plaintext at rest")
	}
	if _, err := OpenWithKey(dir, make([]byte, 32)); err != nil {
		t.Fatal("reopen", err)
	}
	other := make([]byte, 32)
	other[0] = 1
	if _, err := OpenWithKey(dir, other); err == nil {
		t.Fatal("another user's key opened the vault")
	}
	v.LogUse(Use{Item: c.ID, Label: c.Label, Action: "fill", Result: "filled 4242424242424242"})
	if u := v.Uses(5); len(u) != 1 || strings.Contains(u[0].Result, "4242424242424242") {
		t.Fatalf("use log %+v", u)
	}
	if err := v.DeleteItem(c.ID); err != nil || len(v.Items()) != 1 {
		t.Fatal("delete", err)
	}
}

func TestTOTPRFC6238(t *testing.T) {
	// RFC 6238 SHA-1 vector: T=59 → 94287082 (6 digits: 287082)
	got, err := TOTP("GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ", time.Unix(59, 0))
	if err != nil || got != "287082" {
		t.Fatalf("%s %v", got, err)
	}
}

func TestLogWriterRedactsEveryVault(t *testing.T) {
	a, _ := OpenWithKey(t.TempDir(), make([]byte, 32))
	b, _ := OpenWithKey(t.TempDir(), make([]byte, 32))
	_, _ = a.SaveItem("", "login", "x", map[string]string{"site": "a.example", "password": "alpha-fake-secret"})
	_, _ = b.SaveItem("", "login", "y", map[string]string{"site": "b.example", "password": "bravo-fake-secret"})
	var sb strings.Builder
	_, _ = LogWriter(&sb).Write([]byte("alpha-fake-secret bravo-fake-secret"))
	if strings.Contains(sb.String(), "fake-secret") {
		t.Fatal(sb.String())
	}
	a.Close()
	b.Close()
}
