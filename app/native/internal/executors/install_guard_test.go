package executors

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallGuardPreservesOfflineSelectionAndRejectsRevokedGeneration(t *testing.T) {
	d := Descriptor{HostSHA256: strings.Repeat("1", 64), CoreSHA256: strings.Repeat("2", 64), RootSHA256: strings.Repeat("3", 64), CapabilitiesSHA256: strings.Repeat("4", 64)}
	r, e := Open(filepath.Join(t.TempDir(), "registry"), d)
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	p := r.Snapshot().Pin
	called := false
	if e = r.WithInstall(p, func() error { called = true; return nil }); e != nil || !called {
		t.Fatal("offline queue selection refused", e)
	}
	_, _, e = r.Revoke(p, "install-guard-revoke")
	if e != nil {
		t.Fatal(e)
	}
	called = false
	if e = r.WithInstall(p, func() error { called = true; return nil }); !errors.Is(e, ErrConflict) || called {
		t.Fatal("revoked generation committed", e)
	}
}
func TestInstallGuardCommitLinearizesBeforeConcurrentRevoke(t *testing.T) {
	d := Descriptor{HostSHA256: strings.Repeat("1", 64), CoreSHA256: strings.Repeat("2", 64), RootSHA256: strings.Repeat("3", 64), CapabilitiesSHA256: strings.Repeat("4", 64)}
	r, e := Open(filepath.Join(t.TempDir(), "registry"), d)
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	p := r.Snapshot().Pin
	entered := make(chan struct{})
	release := make(chan struct{})
	commitDone := make(chan error, 1)
	revokeDone := make(chan error, 1)
	go func() { commitDone <- r.WithInstall(p, func() error { close(entered); <-release; return nil }) }()
	<-entered
	started := make(chan struct{})
	go func() { close(started); _, _, e := r.Revoke(p, "install-guard-concurrent"); revokeDone <- e }()
	<-started
	select {
	case e := <-revokeDone:
		t.Fatal("revoked inside guarded commit", e)
	default:
	}
	close(release)
	if e = <-commitDone; e != nil {
		t.Fatal(e)
	}
	if e = <-revokeDone; e != nil {
		t.Fatal(e)
	}
	called := false
	if e = r.WithInstall(p, func() error { called = true; return nil }); e == nil || called {
		t.Fatal("post-revoke callback ran")
	}
}
