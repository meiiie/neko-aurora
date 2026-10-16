package executors

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestLiveIdentityTamperNeverAuthorizesCallback(t *testing.T) {
	for _, kind := range []string{"rewrite", "deleted"} {
		t.Run(kind, func(t *testing.T) {
			r, dir, _ := openFixture(t)
			s := readyFixture(t, r)
			l := issueFixture(t, r, s.Pin, fixtureScope())
			path := filepath.Join(dir, stateName)
			if kind == "rewrite" {
				if e := os.WriteFile(path, []byte(`{}`), 0600); e != nil {
					t.Fatal(e)
				}
			} else if e := os.Remove(path); e != nil {
				t.Fatal(e)
			}
			denied(t, r, s.Pin, l)
			if r.Snapshot().Reason != "registry_unavailable" {
				t.Fatal("live identity tamper did not poison registry")
			}
		})
	}
}
func TestDeletedIdentityOnRestartCannotRegenerateInstallation(t *testing.T) {
	r, dir, _ := openFixture(t)
	r.Close()
	path := filepath.Join(dir, stateName)
	if e := os.Remove(path); e != nil {
		t.Fatal(e)
	}
	if next, e := Open(dir, descriptor()); e == nil {
		next.Close()
		t.Fatal("existing identity silently regenerated")
	}
	if _, e := os.Stat(path); !os.IsNotExist(e) {
		t.Fatal("failed recovery published a new identity", e)
	}
}
func TestFailedRevokeKeepsOldDiskPinAndPoisonsAllGuards(t *testing.T) {
	r, dir, _ := openFixture(t)
	s := readyFixture(t, r)
	l := issueFixture(t, r, s.Pin, fixtureScope())
	before, _ := os.ReadFile(filepath.Join(dir, stateName))
	r.write = func([]byte) error { return errors.New("synthetic write fault") }
	if _, _, e := r.Revoke(s.Pin, "lost-revoke"); !errors.Is(e, ErrUnavailable) {
		t.Fatal(e)
	}
	after, _ := os.ReadFile(filepath.Join(dir, stateName))
	if !bytes.Equal(before, after) || r.Snapshot().Pin != s.Pin {
		t.Fatal("failed revoke changed receipt/pin")
	}
	denied(t, r, s.Pin, l)
}
func TestConcurrentIssueOneAttemptHasOneDurableFence(t *testing.T) {
	r, _, _ := openFixture(t)
	s := readyFixture(t, r)
	var wg sync.WaitGroup
	leases := make(chan AttemptLease, 16)
	errs := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); l, _, e := r.Issue(s.Pin, fixtureScope()); leases <- l; errs <- e }()
	}
	wg.Wait()
	close(leases)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var first AttemptLease
	for l := range leases {
		if first.Version == 0 {
			first = l
		}
		if l != first {
			t.Fatal("same attempt has multiple lease identities")
		}
	}
	if r.state.Fence != 1 || len(r.state.Leases) != 1 {
		t.Fatal("duplicate issued multiple durable fences")
	}
}
func TestWrongTrustedHeartbeatCannotRestoreReadiness(t *testing.T) {
	r, _, _ := openFixture(t)
	s := r.Snapshot()
	wrong := s.Pin
	wrong.Generation++
	if e := r.MarkReady(wrong, s.BootID); !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
	if e := r.MarkReady(s.Pin, "executor-boot-ffffffffffffffffffffffffffffffff"); !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
	if r.Snapshot().Ready {
		t.Fatal("wrong boot made ready")
	}
}
