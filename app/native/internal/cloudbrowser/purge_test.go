package cloudbrowser

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
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

func purgeFixtureReply() *http.Response {
	return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"profileId":"bot-fixture","status":"ready","control":"paused","epoch":2,"authState":"unknown","protected":true}`))}
}

func TestCloudOpenDurablyMarksUseBeforeUncertainDispatch(t *testing.T) {
	dir := t.TempDir()
	calls := 0
	c := fixtureClient(t, dir, transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		used, err := NamespaceUsed(dir)
		if err != nil || !used {
			t.Fatal("worker reached before durable use marker")
		}
		return nil, errors.New("fixture disconnected after request")
	}))
	_, err := c.Do(context.Background(), Command{Profile: "bot-fixture", Kind: "open", Actor: "owner", ActorID: "owner", Nonce: "fixture-open-once"})
	if !errors.Is(err, ErrUnknownOutcome) || calls != 1 {
		t.Fatal("open did not retain uncertain outcome")
	}
	used, err := NamespaceUsed(dir)
	if err != nil || !used {
		t.Fatal("uncertain open lost cleanup obligation")
	}
	if info, err := os.Stat(filepath.Join(dir, UsedMarkerFile)); err != nil || !info.Mode().IsRegular() {
		t.Fatal("use marker not persistent")
	}
}

func TestCloudUnusedNamespaceDeletionMakesNoWorkerRequest(t *testing.T) {
	calls := 0
	c := fixtureClient(t, t.TempDir(), transportFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, errors.New("must not call") }))
	if err := c.PurgeNamespace(context.Background()); err != nil || calls != 0 {
		t.Fatal("unused account contacted worker")
	}
	if _, err := c.Do(context.Background(), Command{Profile: "bot-fixture", Kind: "open", Actor: "owner", ActorID: "owner", Nonce: "fixture-late-open"}); !errors.Is(err, ErrCleanupPending) {
		t.Fatal("deleting unused namespace admitted a concurrent late open")
	}
}

func TestCloudPrivatePurgeSignsExactScopeAndSafelyRetries(t *testing.T) {
	dir := t.TempDir()
	purges := 0
	var firstBody string
	transport := transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/v1/command" {
			return purgeFixtureReply(), nil
		}
		if r.URL.Path != "/v1/purge" {
			t.Fatal("purge used wrong private route")
		}
		body, _ := io.ReadAll(r.Body)
		if len(body) > 1024 {
			t.Fatal("oversized purge")
		}
		var in map[string]string
		if json.Unmarshal(body, &in) != nil || len(in) != 2 || len(in["namespace"]) != 64 || !ValidNonce(in["nonce"]) {
			t.Fatal("purge scope not derived privately")
		}
		digest := sha256.Sum256(body)
		m := hmac.New(sha256.New, []byte(strings.Repeat("fixture!", 4)))
		m.Write([]byte(r.Header.Get("X-Aurora-Timestamp") + "\n" + hex.EncodeToString(digest[:])))
		if !hmac.Equal([]byte(r.Header.Get("X-Aurora-Signature")), []byte(hex.EncodeToString(m.Sum(nil)))) {
			t.Fatal("purge signature did not cover exact body")
		}
		purges++
		if firstBody == "" {
			firstBody = string(body)
		} else if firstBody != string(body) {
			t.Fatal("retry changed namespace or nonce")
		}
		if purges == 1 {
			return &http.Response{StatusCode: 503, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
		}
		out, _ := json.Marshal(map[string]string{"namespace": in["namespace"], "nonce": in["nonce"], "status": "purged"})
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(out)))}, nil
	})
	c := fixtureClient(t, dir, transport)
	if _, err := c.Do(context.Background(), Command{Profile: "bot-fixture", Kind: "open", Actor: "owner", ActorID: "owner", Nonce: "fixture-open-once"}); err != nil {
		t.Fatal(err)
	}
	if err := c.PurgeNamespace(context.Background()); err == nil {
		t.Fatal("failed cleanup reported success")
	}
	restarted := fixtureClient(t, dir, transport)
	if _, err := restarted.Do(context.Background(), Command{Profile: "bot-fixture", Kind: "status", Actor: "owner", ActorID: "owner"}); !errors.Is(err, ErrCleanupPending) {
		t.Fatal("restart resumed a namespace pending deletion")
	}
	if err := restarted.PurgeNamespace(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := restarted.PurgeNamespace(context.Background()); err != nil || purges != 2 {
		t.Fatal("completed purge was not idempotent")
	}
	if _, err := restarted.PrepareVault(Command{Profile: "bot-fixture", Kind: "vault_fill", Actor: "owner", ActorID: "owner", Nonce: "fixture-late-vault", ExpectedOrigin: "https://chat.zalo.me", ObservationID: "obs-fixture"}, Credentials{Password: "fixture-secret"}, "fixture-version"); !errors.Is(err, ErrCleanupPending) {
		t.Fatal("late Vault preparation rewrote state after namespace deletion")
	}
	if _, err := os.Stat(filepath.Join(dir, "cloud-browser-pending")); !os.IsNotExist(err) {
		t.Fatal("purged namespace recreated pending secret state")
	}
	afterSuccess := fixtureClient(t, dir, transport)
	if _, err := afterSuccess.Do(context.Background(), Command{Profile: "bot-fixture", Kind: "open", Actor: "owner", ActorID: "owner", Nonce: "fixture-reopen-after-purge"}); !errors.Is(err, ErrCleanupPending) {
		t.Fatal("purged namespace resurrected after restart")
	}
	used, err := NamespaceUsed(dir)
	if err != nil || !used {
		t.Fatal("cleanup locator removed before host account deletion")
	}
}

func TestCloudOpenRefusesDispatchWhenUsedMarkerCannotBeCommitted(t *testing.T) {
	dir := t.TempDir()
	c := fixtureClient(t, dir, transportFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("open dispatched without committed cleanup obligation")
		return nil, errors.New("unexpected")
	}))
	if err := os.Mkdir(filepath.Join(dir, UsedMarkerFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Do(context.Background(), Command{Profile: "bot-fixture", Kind: "open", Actor: "owner", ActorID: "owner", Nonce: "fixture-marker-failure"}); !errors.Is(err, ErrCleanupPending) {
		t.Fatal("uncommitted marker admitted browser open")
	}
}

func TestCloudPurgeReachesWorkerWhileAnEarlierOpenIsInFlight(t *testing.T) {
	started, release, purgeSeen := make(chan struct{}), make(chan struct{}), make(chan struct{})
	c := fixtureClient(t, t.TempDir(), transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/v1/command" {
			close(started)
			<-release
			return purgeFixtureReply(), nil
		}
		close(purgeSeen)
		var in map[string]string
		json.NewDecoder(r.Body).Decode(&in)
		body, _ := json.Marshal(map[string]string{"namespace": in["namespace"], "nonce": in["nonce"], "status": "purged"})
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	}))
	openDone := make(chan error, 1)
	go func() {
		_, err := c.Do(context.Background(), Command{Profile: "bot-fixture", Kind: "open", Actor: "owner", ActorID: "owner", Nonce: "fixture-inflight-open"})
		openDone <- err
	}()
	<-started
	purgeDone := make(chan error, 1)
	go func() { purgeDone <- c.PurgeNamespace(context.Background()) }()
	select {
	case <-purgeSeen:
	case <-time.After(2 * time.Second):
		t.Error("purge could not fence the worker until prior queued input completed")
	}
	close(release)
	if err := <-openDone; err != nil {
		t.Fatal(err)
	}
	if err := <-purgeDone; err != nil {
		t.Fatal(err)
	}
}

func TestCloudPurgeRejectsUnboundOrMalformedReceipt(t *testing.T) {
	for _, mode := range []string{"namespace", "nonce", "status", "extra", "trailing"} {
		t.Run(mode, func(t *testing.T) {
			c := fixtureClient(t, t.TempDir(), transportFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/v1/command" {
					return purgeFixtureReply(), nil
				}
				var in map[string]string
				json.NewDecoder(r.Body).Decode(&in)
				out := map[string]string{"namespace": in["namespace"], "nonce": in["nonce"], "status": "purged"}
				switch mode {
				case "namespace":
					out["namespace"] = strings.Repeat("a", 64)
				case "nonce":
					out["nonce"] = "another-nonce"
				case "status":
					out["status"] = "pending"
				case "extra":
					out["unexpected"] = "field"
				}
				body, _ := json.Marshal(out)
				if mode == "trailing" {
					body = append(body, []byte(`{}`)...)
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
			}))
			if _, err := c.Do(context.Background(), Command{Profile: "bot-fixture", Kind: "open", Actor: "owner", ActorID: "owner", Nonce: "fixture-open-once"}); err != nil {
				t.Fatal(err)
			}
			if err := c.PurgeNamespace(context.Background()); err == nil {
				t.Fatal("unbound purge receipt erased cleanup obligation")
			}
		})
	}
}
