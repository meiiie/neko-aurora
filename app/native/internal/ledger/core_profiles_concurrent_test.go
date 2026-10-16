package ledger

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type reviewReplacingCapture struct {
	mu            sync.Mutex
	source        CoreProfileSource
	first         CoreProfileMetadataReport
	second        CoreProfileMetadataReport
	calls         int
	current       CoreProfileMetadataReport
	firstEntered  chan struct{}
	secondEntered chan struct{}
	releaseFirst  chan struct{}
	releaseSecond chan struct{}
}

func (f *reviewReplacingCapture) Sources() []CoreProfileSource { return []CoreProfileSource{f.source} }
func (f *reviewReplacingCapture) Inspect(ctx context.Context, _ string) (CoreProfileMetadataReport, error) {
	f.mu.Lock()
	f.calls++
	n := f.calls
	f.mu.Unlock()
	var r CoreProfileMetadataReport
	var release chan struct{}
	if n == 1 {
		close(f.firstEntered)
		release = f.releaseFirst
		r = f.first
	} else {
		close(f.secondEntered)
		release = f.releaseSecond
		r = f.second
	}
	select {
	case <-ctx.Done():
		return r, ctx.Err()
	case <-release:
	}
	f.mu.Lock()
	f.current = r
	f.mu.Unlock()
	return r, nil
}
func (f *reviewReplacingCapture) Verify(_ context.Context, p CoreProfilePin) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if p.CredentialSnapshotID != f.current.SourceSnapshot.ID || p.ProfileRevision != f.current.Profiles[0].ProfileRevision {
		return errors.New("owned current snapshot differs from committed metadata")
	}
	return nil
}
func TestProfileSameInspectionDoesNotRecaptureAndNewInspectionRotates(t *testing.T) {
	expiry := time.Now().Add(90 * time.Second).UnixMilli()
	report := authReport(t, "process_env", "static_key", "api_key", expiry, true)
	s, original, ic := authFixture(t, report)
	next := CoreProfileMetadataReport{Version: 2, SourceSnapshot: &CoreProfileSourceSnapshot{ID: "cs1_" + strings.Repeat("c", 32), CreatedAtUnixMS: report.SourceSnapshot.CreatedAtUnixMS, ExpiresAtUnixMS: report.SourceSnapshot.ExpiresAtUnixMS}, Profiles: append([]CoreProfileMetadata{}, report.Profiles...)}
	next.Profiles[0].CredentialSnapshotID = next.SourceSnapshot.ID
	next.Profiles[0].ProfileRevision = "pr2_" + strings.Repeat("d", 64)
	f := &reviewReplacingCapture{source: original.source, first: report, second: next, firstEntered: make(chan struct{}), secondEntered: make(chan struct{}), releaseFirst: make(chan struct{}), releaseSecond: make(chan struct{})}
	s.ConfigureCoreProfiles(f)
	type result struct {
		r   CoreProfilesResult
		err error
	}
	one, two := make(chan result, 1), make(chan result, 1)
	go func() { r, e := s.CoreProfiles(f.source.ID, ic.ID); one <- result{r, e} }()
	<-f.firstEntered
	intake := make(chan error, 1)
	go func() {
		_, _, err := s.Accept(Input{"bot-davin", "while-source-captures", "Independent request remains responsive", "foreground"})
		intake <- err
	}()
	select {
	case err := <-intake:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("source capture held global intake lock")
	}
	go func() { r, e := s.CoreProfiles(f.source.ID, ic.ID); two <- result{r, e} }()
	select {
	case <-f.secondEntered:
	case <-time.After(150 * time.Millisecond):
	}
	close(f.releaseFirst)
	a := <-one
	if a.err != nil {
		t.Fatal("first metadata commit failed", a.err)
	}
	close(f.releaseSecond)
	b := <-two
	if b.err != nil {
		t.Fatal("duplicate metadata read failed", b.err)
	}
	if a.r.SourceSnapshot.ID != report.SourceSnapshot.ID || b.r.SourceSnapshot.ID != report.SourceSnapshot.ID {
		t.Fatal("fixture did not return the durable original snapshot")
	}
	f.mu.Lock()
	captures, current := f.calls, f.current.SourceSnapshot.ID
	f.mu.Unlock()
	in := authGrantInput(original, "review-same-inspection-consent")
	_, _, err := s.ApproveCoreProfile(in)
	t.Logf("owned captures=%d; duplicate returns original=%v; current changed=%v; approval rejected=%v", captures, a.r.SourceSnapshot.ID == b.r.SourceSnapshot.ID, current != a.r.SourceSnapshot.ID, err != nil)
	if err != nil {
		t.Fatal("same-inspection retry replaced private snapshot after original metadata committed; displayed profile can no longer be approved")
	}
	if captures != 1 {
		t.Fatal("same inspection captured twice")
	}
	nextConsent, _, err := s.InspectCoreProfileConsent(CoreProfileInspectionInput{f.source.ID, 1, true, 300, "explicit-next-inspection"})
	if err != nil {
		t.Fatal(err)
	}
	replacement, err := s.CoreProfiles(f.source.ID, nextConsent.ID)
	if err != nil || replacement.SourceSnapshot.ID != next.SourceSnapshot.ID {
		t.Fatal("new explicit inspection must capture distinct snapshot", err)
	}
}
