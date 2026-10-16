package executors

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func openFixture(t *testing.T) (*Registry, string, *time.Time) {
	t.Helper()
	dir := t.TempDir()
	r, e := Open(dir, descriptor())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { r.Close() })
	now := time.Now().Add(time.Second)
	r.clock = func() time.Time { return now }
	return r, dir, &now
}
func readyFixture(t *testing.T, r *Registry) Snapshot {
	t.Helper()
	s := r.Snapshot()
	if e := r.MarkReady(s.Pin, s.BootID); e != nil {
		t.Fatal(e)
	}
	s = r.Snapshot()
	if !s.Ready {
		t.Fatal("trusted readiness missing")
	}
	return s
}
func issueFixture(t *testing.T, r *Registry, p InstallPin, s TaskScope) AttemptLease {
	t.Helper()
	l, duplicate, e := r.Issue(p, s)
	if e != nil || duplicate {
		t.Fatal("issue", duplicate, e)
	}
	return l
}
func denied(t *testing.T, r *Registry, p InstallPin, l AttemptLease) {
	t.Helper()
	called := false
	e := r.WithLease(p, l, func() error { called = true; return nil })
	if e == nil || called {
		t.Fatal("invalidated authority called guarded action", e)
	}
}

func TestRegistryPersistentIdentityAndFreshBootNotAvailability(t *testing.T) {
	r, dir, _ := openFixture(t)
	s := readyFixture(t, r)
	l := issueFixture(t, r, s.Pin, fixtureScope())
	if e := r.Close(); e != nil {
		t.Fatal(e)
	}
	next, e := Open(dir, descriptor())
	if e != nil {
		t.Fatal(e)
	}
	defer next.Close()
	n := next.Snapshot()
	if n.Pin != s.Pin || n.BootID == s.BootID || n.Ready {
		t.Fatal("restart regenerated install or replayed readiness")
	}
	denied(t, next, s.Pin, l)
	if _, e = Open(dir, descriptor()); e == nil {
		t.Fatal("concurrent registry writer admitted")
	}
}
func TestIssueIdempotentExactScopeAndGuardEveryLeaseField(t *testing.T) {
	r, _, _ := openFixture(t)
	s := readyFixture(t, r)
	l := issueFixture(t, r, s.Pin, fixtureScope())
	same, dup, e := r.Issue(s.Pin, fixtureScope())
	if e != nil || !dup || same != l {
		t.Fatal("lost ACK changed lease", e)
	}
	scope := fixtureScope()
	scope.AuthoritySHA256 = strings.Repeat("f", 64)
	if _, _, e = r.Issue(s.Pin, scope); !errors.Is(e, ErrConflict) {
		t.Fatal("same attempt different scope admitted", e)
	}
	called := 0
	if e = r.WithLease(s.Pin, l, func() error { called++; return nil }); e != nil || called != 1 {
		t.Fatal("current exact lease denied", e)
	}
	for _, edit := range []func(*AttemptLease){func(v *AttemptLease) { v.ID = "executor-lease-" + strings.Repeat("f", 32) }, func(v *AttemptLease) { v.ExecutorID = "executor-" + strings.Repeat("f", 32) }, func(v *AttemptLease) { v.Generation++ }, func(v *AttemptLease) { v.BootID = "executor-boot-" + strings.Repeat("f", 32) }, func(v *AttemptLease) { v.RequestID = "req-" + strings.Repeat("f", 32) }, func(v *AttemptLease) { v.AttemptID = "attempt-" + strings.Repeat("f", 32) }, func(v *AttemptLease) { v.WorkEpoch++ }, func(v *AttemptLease) { v.Fence++ }, func(v *AttemptLease) { v.AuthoritySHA256 = strings.Repeat("f", 64) }, func(v *AttemptLease) { v.IssuedAtMs++ }, func(v *AttemptLease) { v.ExpiresAtMs-- }} {
		v := l
		edit(&v)
		denied(t, r, s.Pin, v)
	}
}
func TestUnavailableQueueAndReconnectNeverRevivesLease(t *testing.T) {
	r, _, _ := openFixture(t)
	s := r.Snapshot()
	if _, _, e := r.Issue(s.Pin, fixtureScope()); !errors.Is(e, ErrUnavailable) {
		t.Fatal("unready issue", e)
	}
	s = readyFixture(t, r)
	l := issueFixture(t, r, s.Pin, fixtureScope())
	if e := r.MarkUnavailable(s.Pin, s.BootID, "host_disconnected"); e != nil {
		t.Fatal(e)
	}
	denied(t, r, s.Pin, l)
	readyFixture(t, r)
	denied(t, r, s.Pin, l)
	if _, _, e := r.Issue(s.Pin, fixtureScope()); e == nil {
		t.Fatal("same attempt revived")
	}
	scope := fixtureScope()
	scope.AttemptID = "attempt-" + strings.Repeat("f", 32)
	n := issueFixture(t, r, s.Pin, scope)
	if n.Fence <= l.Fence {
		t.Fatal("fence reused")
	}
}
func TestExpiryAndHeartbeatObservationSurviveClockRewindAndRestart(t *testing.T) {
	for _, kind := range []string{"lease", "heartbeat"} {
		t.Run(kind, func(t *testing.T) {
			r, dir, now := openFixture(t)
			s := readyFixture(t, r)
			l := issueFixture(t, r, s.Pin, fixtureScope())
			if kind == "lease" {
				*now = now.Add(10 * time.Minute)
				r.state.ReadyUntilMs = l.ExpiresAtMs + 1
			} else {
				*now = now.Add(HeartbeatMilliseconds * time.Millisecond)
			}
			denied(t, r, s.Pin, l)
			*now = time.UnixMilli(int64(l.IssuedAtMs))
			denied(t, r, s.Pin, l)
			if e := r.MarkReady(s.Pin, s.BootID); e == nil && kind == "lease" {
				denied(t, r, s.Pin, l)
			}
			if e := r.Close(); e != nil {
				t.Fatal(e)
			}
			next, e := Open(dir, descriptor())
			if e != nil {
				t.Fatal(e)
			}
			defer next.Close()
			next.clock = func() time.Time { return *now }
			denied(t, next, s.Pin, l)
		})
	}
}
func TestRevokeAndConfigureCASNonceRetainOldFence(t *testing.T) {
	r, _, _ := openFixture(t)
	s := readyFixture(t, r)
	l := issueFixture(t, r, s.Pin, fixtureScope())
	n, dup, e := r.Revoke(s.Pin, "revoke-1")
	if e != nil || dup || n.Pin.ExecutorID != s.Pin.ExecutorID || n.Pin.Generation != s.Pin.Generation+1 || n.Ready {
		t.Fatal("revoke", e)
	}
	denied(t, r, s.Pin, l)
	same, dup, e := r.Revoke(s.Pin, "revoke-1")
	if e != nil || !dup || same != n {
		t.Fatal("revoke lost ACK", e)
	}
	if _, _, e = r.Revoke(s.Pin, "other"); !errors.Is(e, ErrConflict) {
		t.Fatal("stale CAS", e)
	}
	if _, _, e = r.Configure(n.Pin, descriptor(), "revoke-1"); !errors.Is(e, ErrConflict) {
		t.Fatal("nonce reused for different operation", e)
	}
	next := descriptor()
	next.RootSHA256 = strings.Repeat("f", 64)
	c, dup, e := r.Configure(n.Pin, next, "configure-1")
	if e != nil || dup || c.Pin.Generation != n.Pin.Generation+1 || c.Ready {
		t.Fatal("configure", e)
	}
	if e = r.MarkReady(c.Pin, c.BootID); e != nil {
		t.Fatal(e)
	}
	denied(t, r, s.Pin, l)
}
func TestIdentityCorruptMismatchAndHardlinkNotSilentlyReplaced(t *testing.T) {
	for _, kind := range []string{"corrupt", "unknown", "duplicate", "mismatch", "hardlink"} {
		t.Run(kind, func(t *testing.T) {
			r, dir, _ := openFixture(t)
			path := filepath.Join(dir, stateName)
			r.Close()
			d := descriptor()
			b, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			switch kind {
			case "corrupt":
				b = []byte(`{`)
			case "unknown":
				b = append(bytes.TrimSuffix(b, []byte(`}`)), []byte(`,"unexpected":1}`)...)
			case "duplicate":
				b = append(bytes.TrimSuffix(b, []byte(`}`)), []byte(`,"version":1}`)...)
			case "mismatch":
				d.RootSHA256 = strings.Repeat("f", 64)
			case "hardlink":
				if e = os.Link(path, filepath.Join(dir, "state-copy")); e != nil {
					t.Fatal(e)
				}
			}
			if kind != "mismatch" && kind != "hardlink" {
				if e = os.WriteFile(path, b, 0600); e != nil {
					t.Fatal(e)
				}
			}
			before, _ := os.ReadFile(path)
			if bad, e := Open(dir, d); e == nil {
				bad.Close()
				t.Fatal("corruption/mismatch replaced identity")
			}
			after, _ := os.ReadFile(path)
			if !bytes.Equal(before, after) {
				t.Fatal("failed Open rewrote evidence")
			}
		})
	}
}
func TestWriteFailurePoisonsRegistryAndCannotLaunchAfterClockRewind(t *testing.T) {
	r, _, now := openFixture(t)
	s := readyFixture(t, r)
	l := issueFixture(t, r, s.Pin, fixtureScope())
	r.write = func([]byte) error { return errors.New("injected disk failure") }
	*now = now.Add(11 * time.Minute)
	denied(t, r, s.Pin, l)
	*now = time.UnixMilli(int64(l.IssuedAtMs))
	denied(t, r, s.Pin, l)
	if r.Snapshot().Reason != "registry_unavailable" {
		t.Fatal("poison not visible")
	}
	if e := r.MarkReady(s.Pin, s.BootID); !errors.Is(e, ErrUnavailable) {
		t.Fatal("poison recovered implicitly", e)
	}
}
func TestGuardSerializesRevokeAcrossCallback(t *testing.T) {
	r, _, _ := openFixture(t)
	s := readyFixture(t, r)
	l := issueFixture(t, r, s.Pin, fixtureScope())
	entered := make(chan struct{})
	release := make(chan struct{})
	action := make(chan error, 1)
	go func() { action <- r.WithLease(s.Pin, l, func() error { close(entered); <-release; return nil }) }()
	<-entered
	started := make(chan struct{})
	revoked := make(chan error, 1)
	go func() { close(started); _, _, e := r.Revoke(s.Pin, "racing-revoke"); revoked <- e }()
	<-started
	select {
	case e := <-revoked:
		t.Fatal("revoke interleaved commit callback", e)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	if e := <-action; e != nil {
		t.Fatal(e)
	}
	if e := <-revoked; e != nil {
		t.Fatal(e)
	}
	denied(t, r, s.Pin, l)
}
func TestClosedRegistryAndMalformedScopeAlwaysDeny(t *testing.T) {
	r, _, _ := openFixture(t)
	s := readyFixture(t, r)
	l := issueFixture(t, r, s.Pin, fixtureScope())
	bad := fixtureScope()
	bad.WorkEpoch = SafeInteger + 1
	if _, _, e := r.Issue(s.Pin, bad); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
	r.Close()
	denied(t, r, s.Pin, l)
	if r.Snapshot().Ready {
		t.Fatal("closed ready")
	}
	if e := r.Heartbeat(s.Pin, s.BootID); !errors.Is(e, ErrUnavailable) {
		t.Fatal(e)
	}
}
func TestHistoricalDescriptorsValidateWithoutLiveRegistryOrClock(t *testing.T) {
	r, _, _ := openFixture(t)
	s := readyFixture(t, r)
	l := issueFixture(t, r, s.Pin, fixtureScope())
	proof, e := ProofFor(l)
	if e != nil {
		t.Fatal(e)
	}
	r.Revoke(s.Pin, "revoke-history")
	r.Close()
	raw, _ := json.Marshal(l)
	if got, e := ParseLease(raw); e != nil || got != l || !MatchRuntimeProof(l, proof) {
		t.Fatal("historic descriptor required live authority", e)
	}
}
