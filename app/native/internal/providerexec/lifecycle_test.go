package providerexec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type fixtureAuthority struct {
	mu      sync.Mutex
	current bool
}

func (a *fixtureAuthority) Current(Input) bool { a.mu.Lock(); defer a.mu.Unlock(); return a.current }
func (a *fixtureAuthority) revoke()            { a.mu.Lock(); a.current = false; a.mu.Unlock() }

type fixtureBroker struct {
	fn func(context.Context, Attempt) (Result, error)
}

func (b fixtureBroker) Probe(ctx context.Context, a Attempt) (Result, error) { return b.fn(ctx, a) }
func fixtureInput(now time.Time) Input {
	return Input{Version: 1, ClientNonce: "fixture-operation-0001", Account: AccountPin{AccountID: "account-" + strings.Repeat("a", 32), Revision: 1, CredentialGeneration: 1, Provider: "openai-compatible", Endpoint: "https://example.invalid/v1", Model: "fixture-model"}, ConsentID: "fixture-explicit-key-probe", ConsentRevision: 1, ExpiresAtUnixMS: now.Add(50 * time.Second).UnixMilli()}
}
func fixtureController(t *testing.T, now time.Time) (*Controller, *fixtureAuthority, string) {
	t.Helper()
	dir := t.TempDir()
	a := &fixtureAuthority{current: true}
	c, e := Open(dir, a)
	if e != nil {
		t.Fatal(e)
	}
	c.now = func() time.Time { return now }
	return c, a, dir
}
func result(a Attempt, status int) Result {
	return Result{Version: 1, OperationID: a.OperationID, Attempt: a.Number, InputSHA256: a.InputSHA256, HTTPStatus: status}
}

func TestProbeReceiptSurvivesRestartWithoutAnotherBrokerCall(t *testing.T) {
	now := time.Now().UTC()
	c, a, dir := fixtureController(t, now)
	in := fixtureInput(now)
	calls := 0
	broker := fixtureBroker{func(ctx context.Context, at Attempt) (Result, error) {
		calls++
		raw, e := os.ReadFile(filepath.Join(dir, "saved-key-probes-v1.json"))
		if e != nil || !strings.Contains(string(raw), `"status":"running"`) {
			t.Fatal("attempt not durable before private consumer")
		}
		return result(at, 200), nil
	}}
	r, e := c.Probe(context.Background(), in, broker, false)
	if e != nil || r.Status != "authenticated_probe" || r.ExecutionGranted || r.Attempts != 1 {
		t.Fatalf("unexpected receipt: %+v %v", r, e)
	}
	reopened, e := Open(dir, a)
	if e != nil {
		t.Fatal(e)
	}
	same, e := reopened.Probe(context.Background(), in, broker, false)
	if e != nil || same.OperationID != r.OperationID || calls != 1 {
		t.Fatal("lost ACK/restart executed another probe")
	}
	changed := in
	changed.Account.Model = "other"
	if _, e = reopened.Probe(context.Background(), changed, broker, false); !errors.Is(e, ErrConflict) {
		t.Fatal("nonce rebound to new profile")
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "saved-key-probes-v1.json"))
	if strings.Contains(string(raw), "secretRef") || strings.Contains(string(raw), "provider response") {
		t.Fatal("private broker material stored")
	}
}

func TestProbeHTTPPolicyUsesExplicitRetrySameOperation(t *testing.T) {
	for _, status := range []int{401, 403, 429, 500, 503, 302, 400, 204} {
		t.Run(string(rune(status)), func(t *testing.T) {
			now := time.Now().UTC()
			c, _, _ := fixtureController(t, now)
			in := fixtureInput(now)
			calls := 0
			broker := fixtureBroker{func(_ context.Context, at Attempt) (Result, error) { calls++; return result(at, status), nil }}
			r, e := c.Probe(context.Background(), in, broker, false)
			if e != nil {
				t.Fatal(e)
			}
			want := map[int]string{401: "unauthorized", 403: "forbidden", 429: "transient", 500: "transient", 503: "transient", 302: "refused", 400: "refused", 204: "authenticated_probe"}[status]
			if r.Status != want || r.ExecutionGranted {
				t.Fatalf("unsafe HTTP policy: %+v", r)
			}
			same, e := c.Probe(context.Background(), in, broker, false)
			if e != nil || same.OperationID != r.OperationID || calls != 1 {
				t.Fatal("automatic retry")
			}
			if want == "transient" {
				for i := 0; i < 2; i++ {
					next, e := c.Probe(context.Background(), in, broker, true)
					if e != nil || next.OperationID != r.OperationID {
						t.Fatal("retry changed operation")
					}
				}
				if _, e = c.Probe(context.Background(), in, broker, true); !errors.Is(e, ErrLimit) {
					t.Fatal("unbounded retry")
				}
			} else if _, e = c.Probe(context.Background(), in, broker, true); !errors.Is(e, ErrConflict) {
				t.Fatal("terminal response retried")
			}
		})
	}
}

func TestProbeRevocationAndExpiryDenyBeforeAndAfterConsumer(t *testing.T) {
	now := time.Now().UTC()
	c, a, _ := fixtureController(t, now)
	in := fixtureInput(now)
	calls := 0
	broker := fixtureBroker{func(_ context.Context, at Attempt) (Result, error) { calls++; a.revoke(); return result(at, 200), nil }}
	r, e := c.Probe(context.Background(), in, broker, false)
	if e != nil || r.Status != "revoked" || r.ExecutionGranted {
		t.Fatal("late success crossed revoke", r, e)
	}
	other := in
	other.ClientNonce = "fixture-operation-0002"
	if _, e = c.Probe(context.Background(), other, broker, false); !errors.Is(e, ErrDenied) || calls != 1 {
		t.Fatal("revoked authority reached broker")
	}
	c2, _, _ := fixtureController(t, now)
	expired := fixtureInput(now)
	expired.ExpiresAtUnixMS = now.UnixMilli()
	if _, e = c2.Probe(context.Background(), expired, broker, false); !errors.Is(e, ErrExpired) {
		t.Fatal("expired authority reached broker")
	}
}

func TestUnknownTransportAndWrongEchoNeverRetry(t *testing.T) {
	for _, mode := range []string{"transport", "wrong-operation", "wrong-hash", "wrong-attempt", "invalid-status"} {
		t.Run(mode, func(t *testing.T) {
			now := time.Now().UTC()
			c, _, _ := fixtureController(t, now)
			in := fixtureInput(now)
			calls := 0
			b := fixtureBroker{func(_ context.Context, at Attempt) (Result, error) {
				calls++
				r := result(at, 200)
				switch mode {
				case "transport":
					return r, errors.New("provider response fixture private data")
				case "wrong-operation":
					r.OperationID = "another"
				case "wrong-hash":
					r.InputSHA256 = strings.Repeat("0", 64)
				case "wrong-attempt":
					r.Attempt++
				case "invalid-status":
					r.HTTPStatus = 999
				}
				return r, nil
			}}
			r, e := c.Probe(context.Background(), in, b, false)
			if e != nil || r.Status != "unknown" || r.ExecutionGranted {
				t.Fatal("unknown promoted", r, e)
			}
			if _, e = c.Probe(context.Background(), in, b, true); !errors.Is(e, ErrConflict) || calls != 1 {
				t.Fatal("uncertain identity re-executed")
			}
			raw, _ := json.Marshal(r)
			if strings.Contains(string(raw), "private data") {
				t.Fatal("broker error leaked")
			}
		})
	}
}

func TestCrashRunningReceiptBecomesUnknown(t *testing.T) {
	now := time.Now().UTC()
	c, a, dir := fixtureController(t, now)
	in := fixtureInput(now)
	b := fixtureBroker{func(_ context.Context, at Attempt) (Result, error) { return result(at, 200), nil }}
	r, e := c.Probe(context.Background(), in, b, false)
	if e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile(filepath.Join(dir, "saved-key-probes-v1.json"))
	if e != nil {
		t.Fatal(e)
	}
	raw = []byte(strings.ReplaceAll(string(raw), `"status":"authenticated_probe"`, `"status":"running"`))
	if e = os.WriteFile(filepath.Join(dir, "saved-key-probes-v1.json"), raw, 0600); e != nil {
		t.Fatal(e)
	}
	reopened, e := Open(dir, a)
	if e != nil {
		t.Fatal(e)
	}
	same, e := reopened.Receipt(in.ClientNonce)
	if e != nil || same.OperationID != r.OperationID || same.Status != "unknown" {
		t.Fatal("crash was replayed as completed", same, e)
	}
}

func TestConcurrentNonceDoesNotStartAnotherConsumer(t *testing.T) {
	now := time.Now().UTC()
	c, _, _ := fixtureController(t, now)
	in := fixtureInput(now)
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan Receipt, 1)
	b := fixtureBroker{func(_ context.Context, at Attempt) (Result, error) {
		close(entered)
		<-release
		return result(at, 200), nil
	}}
	go func() {
		r, e := c.Probe(context.Background(), in, b, false)
		if e != nil {
			t.Error(e)
		}
		done <- r
	}()
	<-entered
	r, e := c.Probe(context.Background(), in, b, false)
	if e != nil || r.Status != "running" {
		t.Fatal("concurrent ACK failed", r, e)
	}
	close(release)
	final := <-done
	if final.OperationID != r.OperationID {
		t.Fatal("nonce produced two identities")
	}
}

func TestAbsentAuthorityAndPersistenceFailurePreventPrivateConsumer(t *testing.T) {
	now := time.Now().UTC()
	dir := t.TempDir()
	c, e := Open(dir, nil)
	if e != nil {
		t.Fatal(e)
	}
	in := fixtureInput(now)
	calls := 0
	b := fixtureBroker{func(_ context.Context, at Attempt) (Result, error) { calls++; return result(at, 200), nil }}
	if _, e = c.Probe(context.Background(), in, b, false); !errors.Is(e, ErrDenied) || calls != 0 {
		t.Fatal("missing authority allowed")
	}
	c, a, dir := fixtureController(t, now)
	_ = a
	if e = os.Mkdir(filepath.Join(dir, "saved-key-probes-v1.json"), 0700); e != nil {
		t.Fatal(e)
	}
	if _, e = c.Probe(context.Background(), in, b, false); !errors.Is(e, ErrUnavailable) || calls != 0 {
		t.Fatal("consumer ran before durable preparation")
	}
}

func TestEverySavedProbeReceiptCanBeReopened(t *testing.T) {
	now := time.Now().UTC()
	c, a, dir := fixtureController(t, now)
	b := fixtureBroker{func(_ context.Context, at Attempt) (Result, error) { return result(at, 200), nil }}
	for i := 0; i < 140; i++ {
		in := fixtureInput(now)
		in.ClientNonce = fmt.Sprintf("fixture-capacity-%04d", i)
		_, e := c.Probe(context.Background(), in, b, false)
		if e != nil && !errors.Is(e, ErrLimit) && !errors.Is(e, ErrUnavailable) {
			t.Fatal(e)
		}
		if _, e = Open(dir, a); e != nil {
			t.Fatal("writer emitted an unreadable bounded journal", i, e)
		}
		if e != nil {
			break
		}
	}
}
func TestRunningProbeCancelsOnAuthorityRemoval(t *testing.T) {
	now := time.Now().UTC()
	c, a, _ := fixtureController(t, now)
	in := fixtureInput(now)
	started := make(chan struct{})
	done := make(chan Receipt, 1)
	b := fixtureBroker{func(ctx context.Context, at Attempt) (Result, error) {
		close(started)
		<-ctx.Done()
		return Result{}, ctx.Err()
	}}
	go func() {
		r, e := c.Probe(context.Background(), in, b, false)
		if e != nil {
			t.Error(e)
		}
		done <- r
	}()
	<-started
	a.revoke()
	select {
	case r := <-done:
		if r.Status != "revoked" {
			t.Fatal("live broker was not revoked", r)
		}
	case <-time.After(time.Second):
		t.Fatal("consumer did not cancel after revoke")
	}
}
