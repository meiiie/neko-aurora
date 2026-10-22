package ledger

import (
	"context"
	"encoding/json"
	"errors"
	"nekoaurora.local/native/internal/protocol"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type profileServiceFixture struct {
	mu            sync.Mutex
	source        CoreProfileSource
	profiles      []CoreProfileMetadata
	inspections   int
	verifications int
	verify        func(CoreProfilePin) error
}

func (f *profileServiceFixture) Sources() []CoreProfileSource {
	f.mu.Lock()
	defer f.mu.Unlock()
	return []CoreProfileSource{f.source}
}
func (f *profileServiceFixture) Inspect(context.Context, string) (CoreProfileMetadataReport, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inspections++
	return CoreProfileMetadataReport{Version: 1, Profiles: append([]CoreProfileMetadata{}, f.profiles...)}, nil
}
func (f *profileServiceFixture) Verify(_ context.Context, p CoreProfilePin) error {
	f.mu.Lock()
	f.verifications++
	fn := f.verify
	f.mu.Unlock()
	if fn != nil {
		return fn(p)
	}
	return nil
}
func profileFixtureDigest(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("../protocol/core-runtimes-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var pins map[string]struct {
		SHA256  string `json:"sha256"`
		Version int    `json:"profileBridgeVersion"`
	}
	if json.Unmarshal(raw, &pins) != nil || (pins["windows_x64"].Version != 1 && pins["windows_x64"].Version != 2) {
		t.Fatal("profile bridge pin fixture unavailable")
	}
	return pins["windows_x64"].SHA256
}
func profileFixture(t *testing.T) (*Store, *profileServiceFixture, Channel, CoreProfileInspectionConsent, CoreProfileConsent, CoreProfileConsentInput) {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	f := &profileServiceFixture{source: CoreProfileSource{1, "core-source-fixture", 1, "Owned profile fixture", profileFixtureDigest(t), true}, profiles: []CoreProfileMetadata{{ID: "fixture-profile", Provider: "openai_compat", Model: "fixture-model", AuthKind: "api_key", Active: true, ConfigurationStatus: "configured", CredentialReadiness: "not_checked", ProfileRevision: "pr1_" + strings.Repeat("a", 64), ProviderOrigin: "http://127.0.0.1:12345", BridgeTransport: "supported"}}}
	s.ConfigureCoreProfiles(f)
	c := sharedChannel(t, s, "Profile fixture", "profile-channel-create")
	ic, dup, err := s.InspectCoreProfileConsent(CoreProfileInspectionInput{f.source.ID, 1, true, 3600, "profile-inspection-create"})
	if err != nil || dup {
		t.Fatal("inspection approval", err)
	}
	if _, err = s.CoreProfiles(f.source.ID, ic.ID); err != nil {
		t.Fatal("metadata", err)
	}
	p := f.profiles[0]
	in := CoreProfileConsentInput{SourceID: f.source.ID, ExpectedSourceRevision: 1, ProfileID: p.ID, ProfileRevision: p.ProfileRevision, Provider: p.Provider, Model: p.Model, ProviderOrigin: p.ProviderOrigin, ChannelID: c.ID, ExpectedChannelRevision: c.Revision, BotIDs: []string{"bot-meiiie"}, CoreProfileLimits: CoreProfileLimits{64000, 128, 3, 3, 600}, TTLSeconds: 3600, ApproveExecution: true, ClientNonce: "profile-execution-create"}
	grant, dup, err := s.ApproveCoreProfile(in)
	if err != nil || dup {
		t.Fatal("execution approval", err)
	}
	return s, f, c, ic, grant, in
}
func acceptProfile(t *testing.T, s *Store, c Channel, grant CoreProfileConsent, nonce string) Work {
	t.Helper()
	rev := c.Revision
	w, dup, err := s.AcceptProduct(Input{"bot-meiiie", nonce, "Use the selected profile for this bounded request.", "foreground"}, SubmissionScope{ChannelID: c.ID, ExpectedChannelRevision: &rev, CoreProfileSelection: &CoreProfileSelection{grant.ID, grant.Revision}})
	if err != nil || dup || w.CoreProfilePin == nil {
		t.Fatal("profile intake", err)
	}
	return w
}
func TestCoreProfileInspectionConsentAndMetadataBoundaries(t *testing.T) {
	s, f, c, ic, _, _ := profileFixture(t)
	f.mu.Lock()
	calls := f.inspections
	f.mu.Unlock()
	before := s.Snapshot().Sequence
	if _, err := s.CoreProfiles(f.source.ID, "core-inspection-"+strings.Repeat("b", 32)); !errors.Is(err, ErrForbidden) {
		t.Fatal("inspection without consent", err)
	}
	if _, err := s.CoreProfiles("core-source-other", ic.ID); !errors.Is(err, ErrForbidden) {
		t.Fatal("inspection wrong source", err)
	}
	s.CoreProfileSources()
	s.CoreProfileReceipt("inspection", "profile-inspection-create")
	s.CoreProfileReceipt("execution", "unknown-profile-receipt")
	s.Snapshot()
	f.mu.Lock()
	if f.inspections != calls {
		t.Fatal("read-only descriptor/receipt inspected source")
	}
	f.mu.Unlock()
	if s.Snapshot().Sequence != before {
		t.Fatal("read-only calls mutated journal")
	}
	for _, p := range []CoreProfileMetadata{{ID: "gemini", Provider: "gemini_cli", Model: "auto", AuthKind: "gemini_oauth", ConfigurationStatus: "incomplete", CredentialReadiness: "not_checked", ProfileRevision: "pr1_" + strings.Repeat("b", 64), BridgeTransport: "unsupported"}, {ID: "openrouter", Provider: "openai_compat", AuthKind: "api_key", ConfigurationStatus: "incomplete", CredentialReadiness: "not_checked", ProfileRevision: "pr1_" + strings.Repeat("c", 64), ProviderOrigin: "https://api.example.invalid", BridgeTransport: "unsupported"}} {
		if !validCoreProfile(p, f.source) {
			t.Fatal("safe incomplete profile projection denied")
		}
	}
	blank := CoreProfileMetadata{ID: "unset", AuthKind: "none", ConfigurationStatus: "incomplete", CredentialReadiness: "not_checked", ProfileRevision: "pr1_" + strings.Repeat("d", 64), BridgeTransport: "unsupported"}
	if !validCoreProfile(blank, f.source) || executableCoreProfile(blank) {
		t.Fatal("safe empty incomplete profile")
	}
	unknown := f.source
	unknown.RuntimeSHA256 = strings.Repeat("a", 64)
	if validCoreSource(unknown) {
		t.Fatal("unknown Core acquired bridge authority")
	}
	unknown.RuntimeSHA256 = "c760a31fad34a27215353e77f39dc654d0c566b1d7ad47ea811fe76eeb2f4990"
	if validCoreSource(unknown) {
		t.Fatal("installed legacy Core acquired bridge authority")
	}
	for _, origin := range []string{"http://localhost:80", "http://192.0.2.1:80", "http://127.0.0.1:80/v1", "http://user@127.0.0.1:80", "http://127.0.0.1:80?key=hidden", "http://127.0.0.1:80#hidden"} {
		if validProfileOrigin(origin, true) {
			t.Fatal("unsafe fixture origin admitted")
		}
	}
	if _, _, err := s.ApproveCoreProfile(CoreProfileConsentInput{ChannelID: c.ID, ApproveExecution: false, ClientNonce: "profile-no-approval"}); !errors.Is(err, ErrInvalid) {
		t.Fatal("missing human approval", err)
	}
}
func TestCoreProfileUnsupportedLoopbackPresetMetadata(t *testing.T) {
	s, f, _, _, _, _ := profileFixture(t)
	for i, origin := range []string{"http://localhost:1234", "http://localhost:11434", "http://127.0.0.1:8080", "http://[::1]:8080", "http://localhost", "http://127.0.0.1", "http://[::1]"} {
		p := f.profiles[0]
		p.ID = "preset-" + strings.Repeat("a", i+1)
		p.ProviderOrigin = origin
		p.BridgeTransport = "unsupported"
		if !validCoreProfile(p, f.source) || executableCoreProfile(p) {
			t.Fatal("safe unsupported preset metadata rejected or executable", origin)
		}
		f.mu.Lock()
		f.profiles = []CoreProfileMetadata{p}
		f.mu.Unlock()
		ic, _, err := s.InspectCoreProfileConsent(CoreProfileInspectionInput{f.source.ID, 1, true, 600, "profile-preset-inspect-" + strings.Repeat("a", i+1)})
		if err != nil {
			t.Fatal(err)
		}
		result, err := s.CoreProfiles(f.source.ID, ic.ID)
		if err != nil || len(result.Profiles) != 1 || result.Profiles[0].ProviderOrigin != origin {
			t.Fatal("unsupported catalog preset capture", err)
		}
		in := CoreProfileConsentInput{SourceID: f.source.ID, ExpectedSourceRevision: 1, ProfileID: p.ID, ProfileRevision: p.ProfileRevision, Provider: p.Provider, Model: p.Model, ProviderOrigin: p.ProviderOrigin, ChannelID: "conversation-bot-meiiie", ExpectedChannelRevision: 1, BotIDs: []string{"bot-meiiie"}, CoreProfileLimits: CoreProfileLimits{64000, 128, 3, 3, 60}, TTLSeconds: 600, ApproveExecution: true, ClientNonce: "profile-preset-no-execution-" + strings.Repeat("a", i+1)}
		if _, _, err = s.ApproveCoreProfile(in); !errors.Is(err, ErrConflict) {
			t.Fatal("unsupported preset acquired execution consent", err)
		}
	}
	for _, origin := range []string{"http://192.0.2.1:8080", "http://localhost.example:8080", "http://localhost:8080/v1", "http://user@localhost:8080", "http://localhost:8080?secret=fixture", "http://localhost:8080#fragment", "http://localhost:99999"} {
		p := f.profiles[0]
		p.ProviderOrigin = origin
		if validCoreProfile(p, f.source) {
			t.Fatal("unsafe unsupported origin admitted", origin)
		}
	}
}
func TestCoreProfileImmutableIntakeNonceRevokeAndReplay(t *testing.T) {
	s, f, c, _, grant, in := profileFixture(t)
	w := acceptProfile(t, s, c, grant, "profile-selected-work")
	original := copyWork(w)
	before := s.Snapshot().Sequence
	raw, err := MarshalRuntimeLaunch(w, nil)
	if err != nil || !strings.Contains(string(raw), `"coreProfilePin"`) || strings.Contains(string(raw), "credentialHome") || strings.Contains(string(raw), "revisionKey") {
		t.Fatal("launch safe profile pin", err)
	}
	if _, err = MarshalRuntimeLaunch(w, map[string]any{"coreProfilePin": nil}); !errors.Is(err, ErrInvalid) {
		t.Fatal("launch pin override")
	}
	if _, dup, err := s.ApproveCoreProfile(in); err != nil || !dup || s.Snapshot().Sequence != before {
		t.Fatal("grant retry renewed", err)
	}
	changed := in
	changed.Model = "different-model"
	if _, _, err := s.ApproveCoreProfile(changed); !errors.Is(err, ErrConflict) {
		t.Fatal("grant nonce changed payload", err)
	}
	for _, scope := range []SubmissionScope{{ChannelID: c.ID, CoreProfileSelection: w.CoreProfileSelection, AccountID: "account-fixture", AccountRevision: 1}, {ChannelID: c.ID, CoreProfileSelection: w.CoreProfileSelection, UseBotDefaults: true}} {
		if _, _, err := s.AcceptProduct(Input{"bot-meiiie", "profile-mixed-selection", "Mixed", "foreground"}, scope); !errors.Is(err, ErrInvalid) {
			t.Fatal("mixed account/profile authority", err)
		}
	}
	control := CoreProfileConsentControlInput{grant.ID, 1, "revoke", "profile-revoke-consent"}
	revoked, dup, err := s.ControlCoreProfile(control)
	if err != nil || dup || revoked.Revision != 2 || revoked.Status != "revoked" {
		t.Fatal("revocation", err)
	}
	if _, err := s.Claim(w.RequestID, w.Epoch); err == nil {
		t.Fatal("revoked profile claimed")
	}
	if err = s.ReconcileCoreProfiles(); err != nil {
		t.Fatal(err)
	}
	current, _ := s.Get(w.RequestID)
	if current.Status != "suspended" || current.Failure != "core_profile_authority_changed" {
		t.Fatal("revoked queued work not suspended")
	}
	if _, dup, err := s.ControlCoreProfile(control); err != nil || !dup {
		t.Fatal("revoke duplicate", err)
	}
	before = s.Snapshot().Sequence
	again, dup, err := s.AcceptProduct(Input{"bot-meiiie", w.ClientNonce, w.Text, w.Lane}, SubmissionScope{ChannelID: c.ID, ExpectedChannelRevision: &c.Revision, CoreProfileSelection: w.CoreProfileSelection})
	if err != nil || !dup || again.RequestID != w.RequestID || !reflect.DeepEqual(again.CoreProfilePin, original.CoreProfilePin) || s.Snapshot().Sequence != before {
		t.Fatal("work receipt rebound/reactivated", err)
	}
	if _, _, err = s.AcceptProduct(Input{"bot-meiiie", w.ClientNonce, w.Text, w.Lane}, SubmissionScope{ChannelID: c.ID}); !errors.Is(err, ErrConflict) {
		t.Fatal("retry removed profile authority", err)
	}
	receipt, err := s.CoreProfileReceipt("execution", in.ClientNonce)
	if err != nil || !receipt.Found || receipt.Active || receipt.Consent.Status != "approved" || receipt.Consent.Revision != 1 {
		t.Fatal("original immutable grant receipt", err)
	}
	dir := s.dir
	s.Close()
	restored, err := Open(dir)
	if err != nil {
		t.Fatal("profile replay", err)
	}
	defer restored.Close()
	if restored.RuntimeAllowed(w.RequestID, current.Epoch) {
		t.Fatal("replayed profile implicit service grant")
	}
	restored.ConfigureCoreProfiles(f)
	r, err := restored.CoreProfileReceipt("execution", in.ClientNonce)
	if err != nil || r.Active || !r.Found {
		t.Fatal("replay revoked grant", err)
	}
}
func TestCoreProfileNonceCompositionAndReplayTampering(t *testing.T) {
	s, _, c, _, grant, _ := profileFixture(t)
	if _, _, err := s.AcceptProduct(Input{"bot-meiiie", "profile-inspection-create", "nonce reused", "foreground"}, SubmissionScope{ChannelID: c.ID}); !errors.Is(err, ErrConflict) {
		t.Fatal("profile nonce reused for work", err)
	}
	w := acceptProfile(t, s, c, grant, "profile-work-nonce-composition")
	if _, _, err := s.InspectCoreProfileConsent(CoreProfileInspectionInput{"core-source-fixture", 1, true, 30, w.ClientNonce}); !errors.Is(err, ErrConflict) {
		t.Fatal("work nonce reused for consent", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.events {
		if e.CoreProfile == nil || e.CoreProfile.ConsentInput == nil {
			continue
		}
		probe := e
		probe.Nonce = "profile-forged-event"
		copyPayload := *e.CoreProfile
		copyInput := *copyPayload.ConsentInput
		copyInput.ClientNonce = probe.Nonce
		copyInput.ProviderOrigin = "http://127.0.0.1:23456"
		copyPayload.ConsentInput = &copyInput
		probe.CoreProfile = &copyPayload
		if s.validCoreProfileEvent(probe) {
			t.Fatal("forged execution origin replay admitted")
		}
	}
	probe := copyWork(w)
	pin := *probe.CoreProfilePin
	pin.ProfileRevision = "pr1_" + strings.Repeat("d", 64)
	probe.CoreProfilePin = &pin
	if s.validCoreProfileIntake(probe) {
		t.Fatal("forged intake profile revision admitted")
	}
	probe = copyWork(w)
	probe.CoreProfilePin = nil
	if s.validCoreProfileIntake(probe) {
		t.Fatal("selection without profile pin admitted")
	}
}
func TestCoreProfileCurrentSourceAudienceExpiryAndHistory(t *testing.T) {
	s, f, c, _, grant, _ := profileFixture(t)
	w := acceptProfile(t, s, c, grant, "profile-current-work")
	f.mu.Lock()
	f.source.Revision++
	f.mu.Unlock()
	if s.RuntimeAllowed(w.RequestID, w.Epoch) {
		t.Fatal("source revision changed work runnable")
	}
	f.mu.Lock()
	f.source.Revision--
	f.mu.Unlock()
	if !s.RuntimeAllowed(w.RequestID, w.Epoch) {
		t.Fatal("unchanged source unavailable")
	}
	clock := time.Now()
	s.now = func() time.Time { return clock }
	clock = clock.Add(2 * time.Hour)
	r, err := s.CoreProfileReceipt("execution", "profile-execution-create")
	if err != nil || r.Active {
		t.Fatal("expired grant active")
	}
	clock = clock.Add(-2 * time.Hour)
	r, _ = s.CoreProfileReceipt("execution", "profile-execution-create")
	if r.Active {
		t.Fatal("clock rewind revived grant")
	}
	if err = s.ReconcileCoreProfiles(); err != nil {
		t.Fatal(err)
	}
	dir := s.dir
	s.Close()
	restored, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	restored.ConfigureCoreProfiles(f)
	restored.now = func() time.Time { return clock }
	r, _ = restored.CoreProfileReceipt("execution", "profile-execution-create")
	if r.Active {
		t.Fatal("restart revived expired grant")
	}
}
func TestCoreProfileVerifyOffLockAndCommitRace(t *testing.T) {
	s, f, c, _, grant, _ := profileFixture(t)
	entered := make(chan struct{})
	finish := make(chan struct{})
	f.mu.Lock()
	f.verify = func(CoreProfilePin) error { close(entered); <-finish; return nil }
	f.mu.Unlock()
	done := make(chan error, 1)
	go func() {
		_, _, err := s.AcceptProduct(Input{"bot-meiiie", "profile-race-work", "bounded profile race", "foreground"}, SubmissionScope{ChannelID: c.ID, CoreProfileSelection: &CoreProfileSelection{grant.ID, 1}})
		done <- err
	}()
	<-entered
	readDone := make(chan struct{})
	go func() { s.Snapshot(); close(readDone) }()
	select {
	case <-readDone:
	case <-time.After(time.Second):
		t.Fatal("Verify held global Store mutex")
	}
	_, _, err := s.SaveChannel(ChannelInput{ID: c.ID, Name: "Changed audience", ExpectedRevision: c.Revision, BotIDs: c.BotIDs, ClientNonce: "profile-audience-race"})
	if err != nil {
		t.Fatal(err)
	}
	close(finish)
	if err = <-done; err == nil {
		t.Fatal("audience revoked during Verify still committed")
	}
	if len(s.Snapshot().Work) != 0 {
		t.Fatal("race minted Work")
	}
}
func TestCoreProfileSelectedPrivateScopeAndCompletedHistory(t *testing.T) {
	s, f, _, ic, _, _ := profileFixture(t)
	p := f.profiles[0]
	in := CoreProfileConsentInput{SourceID: f.source.ID, ExpectedSourceRevision: 1, ProfileID: p.ID, ProfileRevision: p.ProfileRevision, Provider: p.Provider, Model: p.Model, ProviderOrigin: p.ProviderOrigin, ChannelID: "conversation-bot-meiiie", ExpectedChannelRevision: 1, BotIDs: []string{"bot-meiiie"}, CoreProfileLimits: CoreProfileLimits{64000, 128, 3, 3, 600}, TTLSeconds: 3600, ApproveExecution: true, ClientNonce: "profile-private-consent"}
	grant, _, err := s.ApproveCoreProfile(in)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := s.channel(grant.ChannelID)
	w := acceptProfile(t, s, c, grant, "profile-private-work")
	w, err = s.Claim(w.RequestID, w.Epoch)
	if err != nil {
		t.Fatal(err)
	}
	b := CoreBinding{SessionID: strings.Repeat("a", 32), Task: json.RawMessage(`{"id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","root":"/fixture","activationEpoch":1,"activationId":"cccccccccccccccccccccccccccccccc"}`), Execution: json.RawMessage(`{"version":1}`), CoreSHA256: f.source.RuntimeSHA256}
	if err = s.Admit(w.RequestID, w.Epoch, b); err != nil {
		t.Fatal(err)
	}
	if err = s.AppendText(w.RequestID, w.Epoch, "Completed selected-profile fixture history."); err != nil {
		t.Fatal(err)
	}
	if err = s.Finish(w.RequestID, w.Epoch, "completed", ""); err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	if err = s.ReconcileCoreProfiles(); err != nil {
		t.Fatal(err)
	}
	snap := s.Snapshot()
	for _, current := range snap.Work {
		if current.RequestID == w.RequestID && (current.Status != "completed" || current.Reply != "Completed selected-profile fixture history.") {
			t.Fatal("expiry erased completed history")
		}
	}
	if _, err = s.CoreProfiles(f.source.ID, ic.ID); !errors.Is(err, ErrForbidden) {
		t.Fatal("expired inspection read source", err)
	}
}
func TestCoreProfileUnrelatedProgressDoesNotRejectIntake(t *testing.T) {
	s, f, c, _, grant, _ := profileFixture(t)
	entered := make(chan struct{})
	finish := make(chan struct{})
	f.mu.Lock()
	f.verify = func(CoreProfilePin) error { close(entered); <-finish; return nil }
	f.mu.Unlock()
	done := make(chan error, 1)
	go func() {
		_, _, err := s.AcceptProduct(Input{"bot-meiiie", "profile-unrelated-progress", "Bounded current request", "foreground"}, SubmissionScope{ChannelID: c.ID, CoreProfileSelection: &CoreProfileSelection{grant.ID, 1}})
		done <- err
	}()
	<-entered
	if _, _, err := s.Accept(Input{"bot-davin", "profile-other-progress", "Unrelated private progress", "foreground"}); err != nil {
		t.Fatal(err)
	}
	close(finish)
	if err := <-done; err != nil {
		t.Fatal("unrelated journal progress rejected intake", err)
	}
	if len(s.Snapshot().Work) != 2 {
		t.Fatal("intake did not commit beside unrelated work")
	}
	dir := s.dir
	s.Close()
	restored, err := Open(dir)
	if err != nil {
		t.Fatal("fresh captured context replay", err)
	}
	restored.Close()
}
func TestCoreProfileReconcilePreservesConcurrentCompletion(t *testing.T) {
	s, f, c, _, grant, _ := profileFixture(t)
	w := acceptProfile(t, s, c, grant, "profile-reconcile-completion")
	w, err := s.Claim(w.RequestID, w.Epoch)
	if err != nil {
		t.Fatal(err)
	}
	b := CoreBinding{SessionID: strings.Repeat("a", 32), Task: json.RawMessage(`{"id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","root":"/fixture","activationEpoch":1,"activationId":"cccccccccccccccccccccccccccccccc"}`), Execution: json.RawMessage(`{"version":1}`), CoreSHA256: f.source.RuntimeSHA256}
	if err = s.Admit(w.RequestID, w.Epoch, b); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	finish := make(chan struct{})
	var calls atomic.Int32
	f.mu.Lock()
	f.verify = func(CoreProfilePin) error {
		if calls.Add(1) == 1 {
			close(entered)
			<-finish
			return errors.New("owned-fixture-verification-failure")
		}
		return nil
	}
	f.mu.Unlock()
	done := make(chan error, 1)
	go func() { done <- s.ReconcileCoreProfiles() }()
	<-entered
	if err = s.AppendText(w.RequestID, w.Epoch, "Concurrent completed result."); err != nil {
		t.Fatal(err)
	}
	if err = s.Finish(w.RequestID, w.Epoch, "completed", ""); err != nil {
		t.Fatal(err)
	}
	close(finish)
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	current, _ := s.Get(w.RequestID)
	if current.Status != "completed" || current.Reply != "Concurrent completed result." || current.Failure != "" || current.Epoch != w.Epoch {
		t.Fatal("reconciliation overwrote completed work with stale snapshot")
	}
	dir := s.dir
	s.Close()
	restored, err := Open(dir)
	if err != nil {
		t.Fatal("concurrent completion replay", err)
	}
	restored.Close()
}
func TestCoreProfileRevokeDuringToolVerificationHasNoEffect(t *testing.T) {
	s, f, c, _, grant, _ := profileFixture(t)
	w := acceptProfile(t, s, c, grant, "profile-effect-revoke")
	w, err := s.Claim(w.RequestID, w.Epoch)
	if err != nil {
		t.Fatal(err)
	}
	b := CoreBinding{SessionID: strings.Repeat("a", 32), Task: json.RawMessage(`{"id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","root":"/fixture","activationEpoch":1,"activationId":"cccccccccccccccccccccccccccccccc"}`), Execution: json.RawMessage(`{"version":1}`), CoreSHA256: f.source.RuntimeSHA256}
	if err = s.Admit(w.RequestID, w.Epoch, b); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	finish := make(chan struct{})
	f.mu.Lock()
	f.verify = func(CoreProfilePin) error { close(entered); <-finish; return nil }
	f.mu.Unlock()
	done := make(chan error, 1)
	go func() {
		_, err := s.ExecuteTool(w.RequestID, w.Epoch, invocation("Revoked profile must never write this output."))
		done <- err
	}()
	<-entered
	if _, _, err = s.ControlCoreProfile(CoreProfileConsentControlInput{grant.ID, 1, "revoke", "profile-effect-revoke-consent"}); err != nil {
		t.Fatal(err)
	}
	close(finish)
	if err = <-done; err == nil {
		t.Fatal("revoked effect returned success")
	}
	current, _ := s.Get(w.RequestID)
	if len(current.Artifacts) != 0 || len(current.Operations) != 0 {
		t.Fatal("revocation race recorded an effect")
	}
}
func TestCoreProfileHistoricalBridgeReplayCannotAcquireFreshAuthority(t *testing.T) {
	const historical = "af3b2c1c9c64c02356840d7ec4be970522496ff657eb1139a84425786f06b71a"
	if !protocol.CoreProfileBridgeKnown(historical) || protocol.CoreProfileBridgeAllowed(historical) {
		t.Fatal("historical bridge protocol policy")
	}
	s, f, c, _, grant, _ := profileFixture(t)
	completed := acceptProfile(t, s, c, grant, "profile-known-history-completed")
	completed, err := s.Claim(completed.RequestID, completed.Epoch)
	if err != nil {
		t.Fatal(err)
	}
	b := CoreBinding{NativeProcessID: 12345, SessionID: strings.Repeat("a", 32), Task: json.RawMessage(`{"id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","root":"/fixture","activationEpoch":1,"activationId":"cccccccccccccccccccccccccccccccc"}`), Execution: json.RawMessage(`{"version":1,"authorityId":"fixture-authority"}`), CoreSHA256: f.source.RuntimeSHA256}
	if err = s.BindRuntimeAttempt(completed.RequestID, completed.AttemptID, b); err != nil {
		t.Fatal(err)
	}
	if err = s.Admit(completed.RequestID, completed.Epoch, b); err != nil {
		t.Fatal(err)
	}
	if err = s.AppendText(completed.RequestID, completed.Epoch, "Historical owned fixture result."); err != nil {
		t.Fatal(err)
	}
	if err = s.Finish(completed.RequestID, completed.Epoch, "completed", ""); err != nil {
		t.Fatal(err)
	}
	queued := acceptProfile(t, s, c, grant, "profile-known-history-queued")
	starting := acceptProfile(t, s, c, grant, "profile-known-history-starting")
	starting, err = s.Claim(starting.RequestID, starting.Epoch)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	raw, err := os.ReadFile(filepath.Join(s.dir, "transactions.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var rewritten []byte
	previous := ""
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var r record
		if json.Unmarshal([]byte(line), &r) != nil {
			t.Fatal("owned fixture record")
		}
		p := r.Event.CoreProfile
		if p != nil {
			if p.InspectionConsent != nil && p.InspectionConsent.Source.RuntimeSHA256 != "" {
				p.InspectionConsent.Source.RuntimeSHA256 = historical
			}
			if p.Consent != nil {
				p.Consent.Source.RuntimeSHA256 = historical
			}
		}
		w := &r.Event.Work
		if w.CoreProfilePin != nil {
			w.CoreProfilePin.RuntimeSHA256 = historical
		}
		if w.Core != nil {
			w.Core.CoreSHA256 = historical
		}
		if w.RuntimeAttemptBinding != nil {
			w.RuntimeAttemptBinding.CoreSHA256 = historical
		}
		r.Previous = previous
		r.Digest, err = digestRecord(r)
		if err != nil {
			t.Fatal(err)
		}
		previous = r.Digest
		encoded, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		rewritten = append(rewritten, encoded...)
		rewritten = append(rewritten, '\n')
	}
	dir := t.TempDir()
	if err = os.WriteFile(filepath.Join(dir, "transactions.jsonl"), rewritten, 0600); err != nil {
		t.Fatal(err)
	}
	restored, err := Open(dir)
	if err != nil {
		t.Fatal("known historical bridge replay rejected", err)
	}
	defer restored.Close()
	old, _ := restored.Get(completed.RequestID)
	if old.Status != "completed" || old.Reply != "Historical owned fixture result." || old.Core.CoreSHA256 != historical {
		t.Fatal("historical completed data changed")
	}
	f.mu.Lock()
	f.source.RuntimeSHA256 = historical
	f.mu.Unlock()
	restored.ConfigureCoreProfiles(f)
	if _, err = restored.CoreProfileSources(); err == nil {
		t.Fatal("historical descriptor exposed as live source")
	}
	if restored.RuntimeAllowed(queued.RequestID, queued.Epoch) {
		t.Fatal("historical profile queued work runnable")
	}
	if _, err = restored.Claim(queued.RequestID, queued.Epoch); err == nil {
		t.Fatal("historical profile claim granted")
	}
	if _, _, err = restored.InspectCoreProfileConsent(CoreProfileInspectionInput{f.source.ID, 1, true, 600, "profile-historical-new-inspection"}); err == nil {
		t.Fatal("historical Core acquired inspection authority")
	}
	in := CoreProfileConsentInput{SourceID: f.source.ID, ExpectedSourceRevision: 1, ProfileID: grant.Profile.ID, ProfileRevision: grant.Profile.ProfileRevision, Provider: grant.Profile.Provider, Model: grant.Profile.Model, ProviderOrigin: grant.Profile.ProviderOrigin, ChannelID: c.ID, ExpectedChannelRevision: c.Revision, BotIDs: grant.BotIDs, CoreProfileLimits: grant.Limits, TTLSeconds: 600, ApproveExecution: true, ClientNonce: "profile-historical-new-grant"}
	if _, _, err = restored.ApproveCoreProfile(in); err == nil {
		t.Fatal("historical Core acquired execution consent")
	}
	b.CoreSHA256 = historical
	historicalStarting, _ := restored.Get(starting.RequestID)
	if !validAttemptBinding(historicalStarting, b) || !validContextBinding(historicalStarting, b) {
		t.Fatal("historical runtime receipt shape denied")
	}
	if err = restored.BindRuntimeAttempt(starting.RequestID, starting.AttemptID, b); err == nil {
		t.Fatal("historical Core bound new runtime")
	}
	if err = restored.Admit(starting.RequestID, starting.Epoch, b); err == nil {
		t.Fatal("historical Core admission granted")
	}
	before := restored.Snapshot().Sequence
	r, err := restored.CoreProfileReceipt("execution", "profile-execution-create")
	if err != nil || !r.Found || r.Active || r.Consent.Source.RuntimeSHA256 != historical || restored.Snapshot().Sequence != before {
		t.Fatal("historical receipt restored authority", err)
	}
}
func TestCoreProfileGenuineHistoricalJournalReplay(t *testing.T) {
	path := os.Getenv("NEKOBOT_PROFILE_REPLAY_JOURNAL")
	if path == "" {
		t.Skip("explicit owned historical journal fixture not supplied")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err = os.WriteFile(filepath.Join(dir, "transactions.jsonl"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatal("genuine historical producer journal rejected", err)
	}
	defer s.Close()
	if len(s.coreProfileInspections) == 0 || len(s.coreProfileConsents) == 0 {
		t.Fatal("producer profile receipts absent")
	}
	for _, grant := range s.coreProfileConsents {
		if grant.Source.RuntimeSHA256 != "af3b2c1c9c64c02356840d7ec4be970522496ff657eb1139a84425786f06b71a" {
			t.Fatal("producer historical fingerprint differs")
		}
		if s.profileConsentActive(grant) {
			t.Fatal("replay-only producer grant active without registry")
		}
	}
}
