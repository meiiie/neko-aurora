package providerexec

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func witnessFixture(t *testing.T) (WitnessConfig, *bool) {
	t.Helper()
	current := true
	b := SavedTaskBinding{FixtureOnly: true, AccountID: "account-" + strings.Repeat("a", 32), AccountRevision: 1, CredentialGeneration: 1, Provider: "openai-compatible", Endpoint: "http://127.0.0.1:49152/v1", Model: "fixture-model", RequestID: "fixture-request", AttemptID: "fixture-attempt", Epoch: 1, LeaseFence: 1, ContextSHA256: strings.Repeat("b", 64), ConsentID: "fixture-consent", ConsentRevision: 1, ExpiresAtUnixMS: time.Now().UnixMilli() + 30000, Limits: SavedTaskLimits{16000, 256, 2, 2, 30}}
	cfg := WitnessConfig{Path: filepath.Join(t.TempDir(), "authority.json"), Binding: b, ServiceGeneration: "executor-boot-" + strings.Repeat("c", 32), AuthoritySHA256: strings.Repeat("d", 64), RevisionKey: strings.Repeat("6", 64), CurrentTask: func(got SavedTaskBinding) bool { return current && got == b }}
	return cfg, &current
}

func TestWitnessDefaultDenyAndDurableRetirement(t *testing.T) {
	cfg, live := witnessFixture(t)
	cfg.CurrentTask = nil
	c, err := OpenWitness(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.Issue(); err == nil {
		t.Fatal("missing authority accepted")
	}
	cfg, live = witnessFixture(t)
	c, err = OpenWitness(cfg)
	if err != nil {
		t.Fatal(err)
	}
	one, err := c.Issue()
	if err != nil || one.Revision != 1 {
		t.Fatal("initial issuance", err)
	}
	two, err := c.Issue()
	if err != nil || two.Revision != 2 {
		t.Fatal("heartbeat", err)
	}
	*live = false
	if _, err = c.Issue(); err == nil {
		t.Fatal("revoked accepted")
	}
	*live = true
	reopened, err := OpenWitness(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = reopened.Issue(); err == nil {
		t.Fatal("retired regrant")
	}
	raw, _ := os.ReadFile(cfg.Path)
	if strings.Contains(string(raw), cfg.RevisionKey) || !strings.Contains(string(raw), `"state":"retired"`) {
		t.Fatal("retirement/key boundary")
	}
}
func TestWitnessPostWriteRevokeAndRetirementIOPoison(t *testing.T) {
	cfg, _ := witnessFixture(t)
	calls := 0
	cfg.CurrentTask = func(SavedTaskBinding) bool { calls++; return calls < 3 }
	c, err := OpenWitness(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.Issue(); err == nil {
		t.Fatal("postwrite revoke accepted")
	}
	reopened, err := OpenWitness(cfg)
	if err != nil {
		t.Fatal(err)
	}
	reopened.currentTask = func(SavedTaskBinding) bool { return true }
	if _, err = reopened.Issue(); err == nil {
		t.Fatal("revoke not durable")
	}
	cfg, _ = witnessFixture(t)
	c, _ = OpenWitness(cfg)
	if _, err = c.Issue(); err != nil {
		t.Fatal(err)
	}
	os.Remove(cfg.Path)
	os.Mkdir(cfg.Path, 0700)
	if err = c.Retire(); err == nil {
		t.Fatal("retirement IO expected")
	}
	os.Remove(cfg.Path)
	if _, err = c.Issue(); err == nil {
		t.Fatal("IO poison reset")
	}
}
func TestWitnessTamperForeignRollbackDeletionExpiryClock(t *testing.T) {
	for _, kind := range []string{"tamper", "foreign", "rollback", "delete", "expired", "clock"} {
		t.Run(kind, func(t *testing.T) {
			cfg, _ := witnessFixture(t)
			c, _ := OpenWitness(cfg)
			if _, err := c.Issue(); err != nil {
				t.Fatal(err)
			}
			one, _ := os.ReadFile(cfg.Path)
			if _, err := c.Issue(); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "tamper":
				os.WriteFile(cfg.Path, []byte(`{}`), 0600)
			case "foreign":
				cfg2, _ := witnessFixture(t)
				cfg2.Path = filepath.Join(filepath.Dir(cfg.Path), "other.json")
				cfg2.Binding.RequestID = "other-request"
				cfg2.CurrentTask = func(SavedTaskBinding) bool { return true }
				other, _ := OpenWitness(cfg2)
				other.Issue()
				bytes, _ := os.ReadFile(cfg2.Path)
				os.WriteFile(cfg.Path, bytes, 0600)
			case "rollback":
				os.WriteFile(cfg.Path, one, 0600)
			case "delete":
				os.Remove(cfg.Path)
			case "expired":
				c.now = func() time.Time { return time.Now().Add(6 * time.Second) }
			case "clock":
				c.now = func() time.Time { return time.Now().Add(-time.Second) }
			}
			if _, err := c.Issue(); err == nil {
				t.Fatal("unsafe renewal", kind)
			}
			c.now = time.Now
			if _, err := c.Issue(); err == nil {
				t.Fatal("terminal reset", kind)
			}
		})
	}
}
func TestWitnessClosedBindingAndPrivatePath(t *testing.T) {
	cfg, _ := witnessFixture(t)
	raw, _ := json.Marshal(cfg.Binding)
	if _, err := ParseSavedTaskBinding(raw); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{strings.Replace(string(raw), `"fixtureOnly":true,`, "", 1), strings.Replace(string(raw), `"accountRevision":1`, `"accountRevision":1,"accountRevision":1`, 1), strings.Replace(string(raw), `"epoch":1`, `"epoch":0`, 1)} {
		if _, err := ParseSavedTaskBinding([]byte(bad)); err == nil {
			t.Fatal("binding accepted")
		}
	}
	target := filepath.Join(t.TempDir(), "linked.json")
	os.WriteFile(target, []byte(`{}`), 0600)
	if err := os.Link(target, cfg.Path); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenWitness(cfg); err == nil {
		t.Fatal("hardlink accepted")
	}
}
func TestWitnessCrossLanguageVector(t *testing.T) {
	cfg, _ := witnessFixture(t)
	cfg.Binding.ExpiresAtUnixMS = 2000000030000
	hash, err := SavedTaskBindingHash(cfg.Binding)
	if err != nil {
		t.Fatal(err)
	}
	body := WitnessBody{1, "saved_account_authority_v1", hash, cfg.ServiceGeneration, cfg.AuthoritySHA256, 1, "active", 2000000000000, 2000000005000}
	raw, err := signedWitness(body, cfg.RevisionKey)
	if err != nil {
		t.Fatal(err)
	}
	if hash != "27867e36eb37e8a769ebcbfc19294290da63aa13c7b108ebcf994c1575b6e0c3" || string(raw) != `{"authoritySHA256":"dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd","bindingSHA256":"27867e36eb37e8a769ebcbfc19294290da63aa13c7b108ebcf994c1575b6e0c3","issuedAtUnixMS":2000000000000,"mac":"1d1a9eff90e41bff8a6364052504efcf2ee3c9fb486952c678bc8ba5493f1148","purpose":"saved_account_authority_v1","revision":1,"serviceGeneration":"executor-boot-cccccccccccccccccccccccccccccccc","state":"active","validUntilUnixMS":2000000005000,"version":1}` {
		t.Fatalf("vector mismatch hash=%s record=%s", hash, raw)
	}
}

// These rejection vectors are accepted by the WIP Go parser but rejected by
// Core's URL.href identity check or Rust's numeric fixture port check.
func TestSavedBindingCanonicalEndpoint(t *testing.T) {
	for _, endpoint := range []string{
		"http://127.0.0.1:65536/v1", "http://127.0.0.1:00/v1",
		"http://127.0.0.1:080/v1", "http://127.0.0.1:80/v1",
		"http://127.0.0.1:49152", "http://127.0.0.1:49152/a/../v1",
		"http://127.0.0.1:49152/%2e/v1", "http://127.0.0.1:49152/space here",
	} {
		t.Run(endpoint, func(t *testing.T) {
			cfg, _ := witnessFixture(t)
			cfg.Binding.Endpoint = endpoint
			raw, _ := json.Marshal(cfg.Binding)
			if _, err := ParseSavedTaskBinding(raw); err == nil {
				t.Fatal("noncanonical fixture endpoint accepted")
			}
		})
	}
	for _, endpoint := range []string{"https://EXAMPLE.invalid/v1", "https://example.invalid:443/v1", "https://example.invalid", "https://example.invalid/a/../v1", "https://example.invalid/é", "https://example.invalid:65536/v1"} {
		t.Run(endpoint, func(t *testing.T) {
			cfg, _ := witnessFixture(t)
			cfg.Binding.FixtureOnly = false
			cfg.Binding.Endpoint = endpoint
			raw, _ := json.Marshal(cfg.Binding)
			if _, err := ParseSavedTaskBinding(raw); err == nil {
				t.Fatal("noncanonical production endpoint accepted")
			}
		})
	}
	for _, endpoint := range []string{"https://example.invalid/v1", "https://example.invalid:8443/v1", "https://example.invalid/v1/%C3%A9"} {
		cfg, _ := witnessFixture(t)
		cfg.Binding.FixtureOnly = false
		cfg.Binding.Endpoint = endpoint
		raw, _ := json.Marshal(cfg.Binding)
		if _, err := ParseSavedTaskBinding(raw); err != nil {
			t.Fatalf("canonical endpoint rejected: %s", endpoint)
		}
	}
}

func TestSavedBindingRejectsUnknownAndUnsafeFields(t *testing.T) {
	cfg, _ := witnessFixture(t)
	raw, _ := json.Marshal(cfg.Binding)
	for _, bad := range []string{
		strings.Replace(string(raw), `"fixtureOnly":true`, `"fixtureOnly":null`, 1),
		strings.Replace(string(raw), `"fixtureOnly":true`, `"fixtureOnly":true,"extra":1`, 1),
		strings.Replace(string(raw), `"maxSteps":2`, `"maxSteps":2,"extra":1`, 1),
		strings.Replace(string(raw), `"maxSteps":2`, `"maxSteps":7`, 1),
		strings.Replace(string(raw), `"epoch":1`, `"epoch":9007199254740992`, 1),
		strings.Replace(string(raw), `"epoch":1`, `"epoch":1e0`, 1),
		strings.Replace(string(raw), `"epoch":1`, `"epoch":1.0`, 1),
	} {
		if _, err := ParseSavedTaskBinding([]byte(bad)); err == nil {
			t.Fatal("unsafe binding accepted")
		}
	}
}

func TestWitnessAuthorityPanicIsTerminal(t *testing.T) {
	cfg, _ := witnessFixture(t)
	cfg.CurrentTask = func(SavedTaskBinding) bool { panic("authored authority failure") }
	c, err := OpenWitness(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.Issue(); err == nil {
		t.Fatal("authority panic accepted")
	}
	c.currentTask = func(SavedTaskBinding) bool { return true }
	if _, err = c.Issue(); err == nil {
		t.Fatal("authority panic did not poison controller")
	}
	reopened, err := OpenWitness(cfg)
	if err != nil {
		t.Fatal(err)
	}
	reopened.currentTask = func(SavedTaskBinding) bool { return true }
	if _, err = reopened.Issue(); err == nil {
		t.Fatal("authority panic retirement lost on reopen")
	}
}

func TestWitnessRefusesForeignBootAndAuthority(t *testing.T) {
	for _, field := range []string{"boot", "authority", "key"} {
		t.Run(field, func(t *testing.T) {
			cfg, _ := witnessFixture(t)
			c, err := OpenWitness(cfg)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = c.Issue(); err != nil {
				t.Fatal(err)
			}
			switch field {
			case "boot":
				cfg.ServiceGeneration = "executor-boot-" + strings.Repeat("e", 32)
			case "authority":
				cfg.AuthoritySHA256 = strings.Repeat("e", 64)
			case "key":
				cfg.RevisionKey = strings.Repeat("e", 64)
			}
			if _, err = OpenWitness(cfg); err == nil {
				t.Fatal("foreign witness accepted")
			}
		})
	}
}

func TestSavedBindingUnicodeCanonicalVector(t *testing.T) {
	cfg, _ := witnessFixture(t)
	cfg.Binding.ExpiresAtUnixMS = 2000000030000
	cfg.Binding.Model = "模型<&>\u2028"
	hash, err := SavedTaskBindingHash(cfg.Binding)
	if err != nil || hash != "b0bcf8392f94481c623170a1281cc162c68e7882bc0e7c78bb0fb0dc78512b72" {
		t.Fatal("cross-language Unicode binding digest differs", hash, err)
	}
}

func TestWitnessPostWriteAuthorityCannotOutliveWitness(t *testing.T) {
	cfg, _ := witnessFixture(t)
	c, err := OpenWitness(cfg)
	if err != nil {
		t.Fatal(err)
	}
	initial := time.Now()
	c.now = func() time.Time { return initial }
	calls := 0
	c.currentTask = func(SavedTaskBinding) bool {
		calls++
		if calls == 3 {
			c.now = func() time.Time { return initial.Add(5 * time.Second) }
		}
		return true
	}
	if _, err = c.Issue(); err == nil {
		t.Fatal("expired witness returned after authority callback")
	}
	c.now = func() time.Time { return initial }
	c.currentTask = func(SavedTaskBinding) bool { return true }
	if _, err = c.Issue(); err == nil {
		t.Fatal("expired witness did not retire controller")
	}
	raw, err := os.ReadFile(cfg.Path)
	if err != nil || !strings.Contains(string(raw), `"state":"retired"`) {
		t.Fatal("expired witness retirement not durable", err)
	}
}

func TestWitnessOpenRejectsExtendedBindingDeadline(t *testing.T) {
	cfg, _ := witnessFixture(t)
	cfg.Binding.ExpiresAtUnixMS = time.Now().Add(24 * time.Hour).UnixMilli()
	if _, err := OpenWitness(cfg); err == nil {
		t.Fatal("binding exceeds original maxSeconds budget")
	}
	cfg, _ = witnessFixture(t)
	cfg.Binding.ExpiresAtUnixMS = time.Now().Add(-time.Second).UnixMilli()
	if _, err := OpenWitness(cfg); err == nil {
		t.Fatal("expired binding opened")
	}
}

func TestWitnessAuthorityCannotRenewExpiredPriorWitness(t *testing.T) {
	for _, delayCall := range []int{4, 5} {
		t.Run(string(rune('0'+delayCall)), func(t *testing.T) {
			cfg, _ := witnessFixture(t)
			c, err := OpenWitness(cfg)
			if err != nil {
				t.Fatal(err)
			}
			initial := time.Now()
			c.now = func() time.Time { return initial }
			calls := 0
			c.currentTask = func(SavedTaskBinding) bool {
				calls++
				if calls == delayCall {
					c.now = func() time.Time { return initial.Add(5 * time.Second) }
				}
				return true
			}
			if _, err = c.Issue(); err != nil {
				t.Fatal(err)
			}
			if _, err = c.Issue(); err == nil {
				t.Fatal("expired prior witness renewed after authority callback")
			}
		})
	}
}

func TestWitnessFrozenWallCannotExtendPriorProof(t *testing.T) {
	cfg, _ := witnessFixture(t)
	c, err := OpenWitness(cfg)
	if err != nil {
		t.Fatal(err)
	}
	initial := time.Now()
	c.now = func() time.Time { return initial }
	if _, err = c.Issue(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5100 * time.Millisecond)
	if _, err = c.Issue(); err == nil {
		t.Fatal("frozen wall clock extended prior proof beyond five seconds")
	}
}
