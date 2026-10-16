package runtime

import (
	"context"
	"nekobot.local/native/internal/ledger"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestProfileVerifyCancellationDoesNotWaitForSourceMutex(t *testing.T) {
	digest := strings.Repeat("a", 64)
	source := &coreProfileSource{public: ledger.CoreProfileSource{Version: 1, ID: "owned-cancel-fixture", Revision: 1, RuntimeSHA256: digest, FixtureOnly: true}}
	service := &CoreProfiles{sources: map[string]*coreProfileSource{source.public.ID: source}}
	source.mu.Lock()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- service.Verify(ctx, ledger.CoreProfilePin{SourceID: source.public.ID, SourceRevision: 1, RuntimeSHA256: digest, FixtureOnly: true})
	}()
	select {
	case err := <-done:
		source.verifyMu.Lock()
		flight := source.verification
		source.verifyMu.Unlock()
		source.mu.Unlock()
		if err == nil {
			t.Fatal("cancelled verification became ready")
		}
		if flight != nil {
			select {
			case <-flight.done:
			case <-time.After(time.Second):
				t.Fatal("owned read did not finish after source unlock")
			}
		}
	case <-time.After(250 * time.Millisecond):
		source.mu.Unlock()
		<-done
		t.Fatal("source mutex consumed caller deadline and blocked cancelled verification")
	}
}

func TestProfileVerifySharesOnlyInflightRead(t *testing.T) {
	source := &coreProfileSource{}
	started, release := make(chan struct{}), make(chan struct{})
	var reads atomic.Int32
	collect := func(ctx context.Context) (ledger.CoreProfileMetadataReport, error) {
		n := reads.Add(1)
		if n == 1 {
			close(started)
			select {
			case <-release:
			case <-ctx.Done():
				return ledger.CoreProfileMetadataReport{}, ctx.Err()
			}
		}
		return ledger.CoreProfileMetadataReport{Version: 2, Profiles: []ledger.CoreProfileMetadata{{ID: "owned", ProfileRevision: fmtProfileRevision(uint64(n))}}, SourceSnapshot: &ledger.CoreProfileSourceSnapshot{ID: "copy-fixture"}}, nil
	}
	firstCtx, cancel := context.WithCancel(context.Background())
	first := make(chan error, 1)
	go func() { _, e := source.verificationCatalog(firstCtx, collect); first <- e }()
	<-started
	results := make(chan ledger.CoreProfileMetadataReport, 8)
	failures := make(chan error, 8)
	for i := 0; i < 8; i++ {
		go func() { r, e := source.verificationCatalog(context.Background(), collect); results <- r; failures <- e }()
	}
	time.Sleep(50 * time.Millisecond)
	cancel()
	if <-first == nil {
		t.Fatal("cancelled caller accepted a catalog")
	}
	close(release)
	for i := 0; i < 8; i++ {
		r := <-results
		if e := <-failures; e != nil {
			t.Fatal(e)
		}
		if r.Profiles[0].ProfileRevision != "1" {
			t.Fatal("overlapping caller did not share original read")
		}
		r.Profiles[0].ID = "mutated"
		r.SourceSnapshot.ID = "mutated"
	}
	if reads.Load() != 1 {
		t.Fatalf("overlapping reads: %d", reads.Load())
	}
	next, e := source.verificationCatalog(context.Background(), collect)
	if e != nil || reads.Load() != 2 || next.Profiles[0].ProfileRevision != "2" || next.Profiles[0].ID != "owned" || next.SourceSnapshot.ID != "copy-fixture" {
		t.Fatal("completed catalog was reused or shared result was mutable")
	}
}

func TestProfileVerifyReadFencesSnapshotChanges(t *testing.T) {
	for _, change := range []string{"generation", "snapshot", "expiry"} {
		t.Run(change, func(t *testing.T) {
			source := &coreProfileSource{environmentBroker: true, snapshotID: "original", snapshotExpiry: 1234}
			started, release := make(chan struct{}), make(chan struct{})
			done := make(chan error, 1)
			go func() {
				_, e := source.verificationCatalog(context.Background(), func(context.Context) (ledger.CoreProfileMetadataReport, error) {
					close(started)
					<-release
					return ledger.CoreProfileMetadataReport{SourceSnapshot: &ledger.CoreProfileSourceSnapshot{ID: "original", ExpiresAtUnixMS: 1234}}, nil
				})
				done <- e
			}()
			<-started
			source.verifyMu.Lock()
			switch change {
			case "generation":
				source.verifyGeneration++
			case "snapshot":
				source.snapshotID = "replacement"
			case "expiry":
				source.snapshotExpiry = 5678
			}
			source.verifyMu.Unlock()
			close(release)
			if <-done == nil {
				t.Fatal("catalog silently rebound changed snapshot authority")
			}
		})
	}
}

func TestProfileVerifyCancelledReadDoesNotCollect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var reads atomic.Int32
	source := &coreProfileSource{}
	_, e := source.verificationCatalog(ctx, func(context.Context) (ledger.CoreProfileMetadataReport, error) {
		reads.Add(1)
		return ledger.CoreProfileMetadataReport{}, nil
	})
	if e == nil || reads.Load() != 0 {
		t.Fatal("cancelled verification started a read")
	}
}

func TestProfileVerifySourcesAreIndependent(t *testing.T) {
	a, b := &coreProfileSource{}, &coreProfileSource{}
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		_, e := a.verificationCatalog(context.Background(), func(context.Context) (ledger.CoreProfileMetadataReport, error) {
			close(started)
			<-release
			return ledger.CoreProfileMetadataReport{}, nil
		})
		done <- e
	}()
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, e := b.verificationCatalog(ctx, func(context.Context) (ledger.CoreProfileMetadataReport, error) {
		return ledger.CoreProfileMetadataReport{}, nil
	})
	close(release)
	if e != nil || <-done != nil {
		t.Fatal("one source blocked another")
	}
}

func TestProfileVerifyExpiredInspectionDoesNotCaptureAfterWaiting(t *testing.T) {
	source := &coreProfileSource{environmentBroker: true}
	service := &CoreProfiles{sources: map[string]*coreProfileSource{"owned": source}}
	source.mu.Lock()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, e := service.Inspect(ctx, "owned"); done <- e }()
	cancel()
	source.mu.Unlock()
	if <-done == nil || source.verifyGeneration != 0 {
		t.Fatal("expired inspection entered the capture path")
	}
}
