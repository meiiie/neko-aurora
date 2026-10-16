//go:build neko_native_fixture

package executors

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNativeClockFixtureRejectsInvalidClosedAndPoisoned(t *testing.T) {
	if e := FixtureSetClock(nil, time.Now); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
	d := Descriptor{HostSHA256: strings.Repeat("1", 64), CoreSHA256: strings.Repeat("2", 64), RootSHA256: strings.Repeat("3", 64), CapabilitiesSHA256: strings.Repeat("4", 64)}
	r, e := Open(filepath.Join(t.TempDir(), "registry"), d)
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	if e = FixtureSetClock(r, nil); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
	instant := time.Now().Add(time.Second)
	if e = FixtureSetClock(r, func() time.Time { return instant }); e != nil {
		t.Fatal(e)
	}
	if ms, e := r.now(); e != nil || ms != uint64(instant.UnixMilli()) {
		t.Fatal("fixture clock not installed", e)
	}
	r.mu.Lock()
	r.poisoned = true
	r.mu.Unlock()
	if e = FixtureSetClock(r, time.Now); !errors.Is(e, ErrUnavailable) {
		t.Fatal("poisoned registry clock replaced", e)
	}
	r.mu.Lock()
	r.poisoned = false
	r.mu.Unlock()
	if e = r.Close(); e != nil {
		t.Fatal(e)
	}
	if e = FixtureSetClock(r, time.Now); !errors.Is(e, ErrUnavailable) {
		t.Fatal("closed registry clock replaced", e)
	}
}
