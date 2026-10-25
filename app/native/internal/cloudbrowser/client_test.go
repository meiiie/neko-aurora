package cloudbrowser

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func fixtureClient(t *testing.T, dir string, f transportFunc) *Client {
	t.Helper()
	c, err := New(Config{URL: "https://browser.example.test", Key: []byte(strings.Repeat("fixture!", 4)), StateDir: dir, HTTP: &http.Client{Transport: f}, Now: func() time.Time { return time.Unix(1800000000, 0) }})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestCloudSignsExactBodyAndKeepsNamespacePrivate(t *testing.T) {
	dir := t.TempDir()
	var first string
	call := transportFunc(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		if r.URL.String() != "https://browser.example.test/v1/command" || r.Header.Get("X-Aurora-Timestamp") != "1800000000" {
			t.Fatal("wrong signed destination or timestamp")
		}
		digest := sha256.Sum256(body)
		m := hmac.New(sha256.New, []byte(strings.Repeat("fixture!", 4)))
		m.Write([]byte("1800000000\n" + hex.EncodeToString(digest[:])))
		if !hmac.Equal([]byte(r.Header.Get("X-Aurora-Signature")), []byte(hex.EncodeToString(m.Sum(nil)))) {
			t.Fatal("signature did not cover exact request body")
		}
		var cmd Command
		if json.Unmarshal(body, &cmd) != nil || len(cmd.Namespace) != 64 || cmd.Profile != "bot-fixture" {
			t.Fatal("missing scoped profile")
		}
		if first == "" {
			first = cmd.Namespace
		} else if first != cmd.Namespace {
			t.Fatal("namespace changed across restart")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"profileId":"bot-fixture","status":"stopped","control":"paused","epoch":1,"authState":"unknown","protected":true}`)), Header: http.Header{}}, nil
	})
	for range 2 {
		c := fixtureClient(t, dir, call)
		if _, err := c.Do(context.Background(), Command{Profile: "bot-fixture", Kind: "status", Actor: "owner", ActorID: "fixture-owner"}); err != nil {
			t.Fatal(err)
		}
	}
	other := fixtureClient(t, t.TempDir(), call)
	if other.Namespace() == first {
		t.Fatal("tenants shared namespace")
	}
	if _, err := os.Stat(filepath.Join(dir, NamespaceFile)); err != nil {
		t.Fatal(err)
	}
}

func TestCloudConfigurationNeverCreatesLocalFallback(t *testing.T) {
	for _, endpoint := range []string{"http://localhost:9", "https://127.0.0.1", "https://user:pass@browser.example", "https://browser.example/?token=x", "https://browser.example/#x", "http://10.0.0.1", "https://browser.railway.internal", "https://browser.aurora.internal", "http://browser.aurora.internal.evil.test", "http://.aurora.internal", "http://browser", "http://browser.internal"} {
		if _, err := New(Config{URL: endpoint, Key: []byte(strings.Repeat("k", 32)), StateDir: t.TempDir()}); !errors.Is(err, ErrConfiguration) {
			t.Fatalf("accepted invalid endpoint %q", endpoint)
		}
	}
	dir := t.TempDir()
	if _, err := New(Config{StateDir: dir}); !errors.Is(err, ErrNotConfigured) {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatal("unconfigured feature wrote state")
	}
	for _, endpoint := range []string{"http://browser.railway.internal", "http://browser.aurora.internal:8080"} {
		if _, err := New(Config{URL: endpoint, Key: []byte(strings.Repeat("k", 32)), StateDir: t.TempDir()}); err != nil {
			t.Fatalf("private worker endpoint %q: %v", endpoint, err)
		}
	}
}

func TestCloudTransportFailureDoesNotReplayInput(t *testing.T) {
	calls := 0
	c := fixtureClient(t, t.TempDir(), transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("fixture disconnected after dispatch")
	}))
	_, err := c.Do(context.Background(), Command{Profile: "bot-fixture", Kind: "click", Actor: "agent", ActorID: "bot-fixture", WorkID: "work-fixture", WorkEpoch: 1, Attempt: "attempt-fixture", Nonce: "fixture-click", Epoch: 2, ObservationID: "obs-fixture", TargetID: "target-fixture"})
	if !errors.Is(err, ErrUnknownOutcome) || calls != 1 {
		t.Fatalf("input replayed or certainty invented: calls=%d err=%v", calls, err)
	}
}

func TestCloudAgentCannotReadLoginOrFrames(t *testing.T) {
	c := fixtureClient(t, t.TempDir(), transportFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"profileId":"bot-fixture","status":"ready","control":"agent","epoch":2,"authState":"login_required","protected":true,"text":"fixture credential","targets":[{"id":"target","role":"textbox","name":"OTP","type":"text","protected":true}],"frame":{"id":"frame","mime":"image/png","data":"YWJj","width":10,"height":10}}`)), Header: http.Header{}}, nil
	}))
	res, err := c.Do(context.Background(), Command{Profile: "bot-fixture", Kind: "observe", Actor: "agent", ActorID: "bot-fixture", WorkID: "work-fixture", WorkEpoch: 1, Attempt: "attempt-fixture"})
	if err != nil || res.Frame != nil || res.Text != "" || len(res.Targets) != 0 {
		t.Fatal("protected login leaked to model")
	}
	if _, err := c.Do(context.Background(), Command{Profile: "bot-fixture", Kind: "frame", Actor: "agent", ActorID: "bot-fixture"}); !errors.Is(err, ErrInvalid) {
		t.Fatal("agent frame command accepted")
	}
}

func TestCloudVaultSealBindsOriginProfileAndObservation(t *testing.T) {
	c := fixtureClient(t, t.TempDir(), nil)
	cmd := Command{Profile: "bot-fixture", Kind: "vault_fill", Actor: "owner", ActorID: "owner", Nonce: "vault-fixture", ExpectedOrigin: "https://chat.zalo.me", ObservationID: "obs-fixture"}
	sealed, err := c.Seal(cmd, Credentials{Username: "fixture-user", Password: "fixture-password"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := base64.StdEncoding.DecodeString(sealed)
	m := hmac.New(sha256.New, []byte(strings.Repeat("fixture!", 4)))
	m.Write([]byte("aurora-cloud-vault-v1\n" + c.Namespace() + "\n" + cmd.Profile))
	b, _ := aes.NewCipher(m.Sum(nil))
	a, _ := cipher.NewGCM(b)
	aad := []byte(c.Namespace() + "\n" + cmd.Profile + "\n" + cmd.Nonce + "\nhttps://chat.zalo.me\nobs-fixture")
	pt, err := a.Open(nil, raw[:12], raw[12:], aad)
	if err != nil || !strings.Contains(string(pt), "fixture-password") {
		t.Fatal("worker cannot decrypt documented envelope")
	}
	if _, err = a.Open(nil, raw[:12], raw[12:], []byte("changed binding")); err == nil {
		t.Fatal("seal accepted changed binding")
	}
}

func TestCloudFillCanClearFieldAndUncertainResponseNeverClaimsFailureBeforeDispatch(t *testing.T) {
	seen := false
	c := fixtureClient(t, t.TempDir(), transportFunc(func(r *http.Request) (*http.Response, error) {
		var body map[string]json.RawMessage
		json.NewDecoder(r.Body).Decode(&body)
		if string(body["text"]) != `""` {
			t.Error("empty fill omitted explicit text field")
		}
		seen = true
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"unexpected":"truncated worker result"}`)), Header: http.Header{}}, nil
	}))
	_, err := c.Do(context.Background(), Command{Profile: "bot-fixture", Kind: "fill", Actor: "owner", ActorID: "owner", Nonce: "fixture-clear", Epoch: 2, ObservationID: "obs-fixture", TargetID: "target-fixture"})
	if !seen || !errors.Is(err, ErrUnknownOutcome) {
		t.Fatalf("invalid reply after input invented certainty: %v", err)
	}
}

func TestCloudVaultDuplicateRetainsExactCiphertext(t *testing.T) {
	c := fixtureClient(t, t.TempDir(), nil)
	cmd := Command{Profile: "bot-fixture", Kind: "vault_fill", Actor: "owner", ActorID: "owner", Nonce: "fixture-duplicate", Epoch: 2, ObservationID: "obs-fixture", ExpectedOrigin: "https://chat.zalo.me"}
	first, err := c.PrepareVault(cmd, Credentials{Password: "fixture-secret"}, "item-version-1")
	if err != nil {
		t.Fatal(err)
	}
	second, err := c.PrepareVault(cmd, Credentials{Password: "fixture-secret"}, "item-version-1")
	if err != nil || first.Sealed != second.Sealed {
		t.Fatal("same nonce was re-encrypted")
	}
	if _, err := c.PrepareVault(cmd, Credentials{Password: "fixture-other"}, "item-version-2"); !errors.Is(err, ErrConflict) {
		t.Fatal("same nonce accepted changed item binding")
	}
}

func TestCloudOwnerClickIncludesZeroFrameCoordinates(t *testing.T) {
	c := fixtureClient(t, t.TempDir(), transportFunc(func(r *http.Request) (*http.Response, error) {
		var body map[string]json.RawMessage
		json.NewDecoder(r.Body).Decode(&body)
		if string(body["x"]) != "0" || string(body["y"]) != "0" {
			t.Error("top-left owner coordinates omitted")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"profileId":"bot-fixture","status":"ready","control":"owner","epoch":2,"authState":"unknown","protected":false,"receipt":{"nonce":"fixture-top-left","status":"dispatched","code":"input_delivered"}}`)), Header: http.Header{}}, nil
	}))
	if _, err := c.Do(context.Background(), Command{Profile: "bot-fixture", Kind: "click", Actor: "owner", ActorID: "owner", Nonce: "fixture-top-left", Epoch: 2, ObservationID: "obs-fixture", FrameID: "frame-fixture"}); err != nil {
		t.Fatal(err)
	}
}

func TestCloudAgentDOMRequiresExplicitPrivacyAndOwnerControlConsent(t *testing.T) {
	for _, tc := range []struct {
		name, status, control, auth                   string
		protected                                     any
		includeProtected, allowDOM, wantProtocolError bool
	}{
		{"unknown auth safe agent control", "ready", "agent", "unknown", false, true, true, false},
		{"positive auth safe agent control", "ready", "agent", "authenticated", false, true, true, false},
		{"owner control does not consent to agent", "ready", "owner", "authenticated", false, true, false, false},
		{"paused control does not consent to agent", "ready", "paused", "authenticated", false, true, false, false},
		{"protected page", "ready", "agent", "authenticated", true, true, false, false},
		{"stopped page", "stopped", "agent", "authenticated", false, true, false, false},
		{"recovering page", "recovery_required", "agent", "authenticated", false, true, false, false},
		{"starting page", "starting", "agent", "authenticated", false, true, false, false},
		{"unavailable page", "unavailable", "agent", "authenticated", false, true, false, false},
		{"login overrides false privacy flag", "ready", "agent", "login_required", false, true, false, false},
		{"reauth overrides false privacy flag", "ready", "agent", "reauth_required", false, true, false, false},
		{"missing privacy flag", "ready", "agent", "authenticated", nil, false, false, true},
		{"null privacy flag", "ready", "agent", "authenticated", nil, true, false, true},
		{"string privacy flag", "ready", "agent", "authenticated", "false", true, false, true},
		{"numeric privacy flag", "ready", "agent", "authenticated", 0, true, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := map[string]any{"profileId": "bot-fixture", "status": tc.status, "control": tc.control, "epoch": 2, "authState": tc.auth,
				"title": "Fixture title", "text": "Fixture safe page content", "observationId": "fixture-observation",
				"targets": []map[string]any{{"id": "ordinary", "role": "button", "name": "Fixture button", "type": "button", "protected": false}, {"id": "secret", "role": "textbox", "name": "Protected field label", "type": "password", "protected": true}},
				"frame":   map[string]any{"id": "frame", "mime": "image/png", "data": "YWJj", "width": 10, "height": 10}}
			if tc.includeProtected {
				payload["protected"] = tc.protected
			}
			body, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			c := fixtureClient(t, t.TempDir(), transportFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body))), Header: http.Header{}}, nil
			}))
			out, err := c.Do(context.Background(), Command{Profile: "bot-fixture", Kind: "observe", Actor: "agent", ActorID: "bot-fixture", WorkID: "work-fixture", WorkEpoch: 1, Attempt: "attempt-fixture"})
			if tc.wantProtocolError {
				if !errors.Is(err, ErrProtocol) {
					t.Fatalf("missing/malformed privacy accepted: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if out.Frame != nil {
				t.Fatal("agent received an owner frame")
			}
			if out.AuthState != tc.auth {
				t.Fatal("privacy consent invented provider authentication evidence")
			}
			if tc.allowDOM {
				if out.Text != "Fixture safe page content" || out.Title != "Fixture title" || len(out.Targets) != 2 || out.Targets[0].Name != "Fixture button" {
					t.Fatal("safe owner-authorized agent DOM was discarded")
				}
				if out.Targets[1].Name != "" || !out.Targets[1].Protected {
					t.Fatal("protected target filtering was lost")
				}
			} else if out.Text != "" || out.Title != "" || len(out.Targets) != 0 {
				t.Fatal("unsafe/control-withheld DOM leaked to agent")
			}
		})
	}
}

func TestCloudOwnerResponseAlsoRequiresExplicitPrivacyFlag(t *testing.T) {
	c := fixtureClient(t, t.TempDir(), transportFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"profileId":"bot-fixture","status":"ready","control":"owner","epoch":2,"authState":"unknown"}`)), Header: http.Header{}}, nil
	}))
	if _, err := c.Do(context.Background(), Command{Profile: "bot-fixture", Kind: "status", Actor: "owner", ActorID: "owner"}); !errors.Is(err, ErrProtocol) {
		t.Fatal("owner response accepted a worker without explicit privacy protocol")
	}
}
