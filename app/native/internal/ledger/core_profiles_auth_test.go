package ledger

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

type authProfileFixture struct {
	mu     sync.Mutex
	source CoreProfileSource
	report CoreProfileMetadataReport
	verify func(CoreProfilePin) error
	reads  int
	checks int
}

func (f *authProfileFixture) Sources() []CoreProfileSource {
	f.mu.Lock()
	defer f.mu.Unlock()
	return []CoreProfileSource{f.source}
}
func (f *authProfileFixture) Inspect(context.Context, string) (CoreProfileMetadataReport, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads++
	raw, _ := json.Marshal(f.report)
	var out CoreProfileMetadataReport
	_ = json.Unmarshal(raw, &out)
	return out, nil
}
func (f *authProfileFixture) Verify(_ context.Context, p CoreProfilePin) error {
	f.mu.Lock()
	f.checks++
	fn := f.verify
	f.mu.Unlock()
	if fn != nil {
		return fn(p)
	}
	return nil
}
func authObject(t *testing.T, v any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if json.Unmarshal(raw, &out) != nil {
		t.Fatal("auth fixture DTO")
	}
	return out
}
func authReport(t *testing.T, source, policy, auth string, expiry int64, snapshot bool) CoreProfileMetadataReport {
	t.Helper()
	now := time.Now().UnixMilli()
	p := map[string]any{"id": "auth-fixture", "provider": "openai_compat", "model": "auth-fixture-model", "authKind": auth, "active": true, "configurationStatus": "configured", "credentialReadiness": "not_checked", "profileRevision": "pr2_" + strings.Repeat("a", 64), "providerOrigin": "http://127.0.0.1:12345", "bridgeTransport": "supported", "credentialSource": source, "authPolicy": policy}
	if auth == "kimi_oauth" {
		p["provider"] = "kimi"
	}
	if expiry != 0 {
		p["credentialExpiresAtUnixMs"] = expiry
	}
	r := map[string]any{"version": 2, "profiles": []any{p}}
	if snapshot {
		p["credentialSnapshotId"] = "cs1_" + strings.Repeat("b", 32)
		r["sourceSnapshot"] = map[string]any{"id": "cs1_" + strings.Repeat("b", 32), "createdAtUnixMs": now - 1000, "expiresAtUnixMs": now + 180000}
	}
	raw, _ := json.Marshal(r)
	var out CoreProfileMetadataReport
	if json.Unmarshal(raw, &out) != nil {
		t.Fatal("auth fixture report")
	}
	return out
}
func authFixture(t *testing.T, report CoreProfileMetadataReport) (*Store, *authProfileFixture, CoreProfileInspectionConsent) {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	raw, err := os.ReadFile("../protocol/core-runtimes-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var pins map[string]struct {
		SHA256 string `json:"sha256"`
	}
	if json.Unmarshal(raw, &pins) != nil {
		t.Fatal("auth fixture runtime pin")
	}
	f := &authProfileFixture{source: CoreProfileSource{1, "core-source-auth-fixture", 1, "Owned auth metadata fixture", pins["windows_x64"].SHA256, true}, report: report}
	s.ConfigureCoreProfiles(f)
	ic, _, err := s.InspectCoreProfileConsent(CoreProfileInspectionInput{f.source.ID, 1, true, 600, "auth-fixture-inspection"})
	if err != nil {
		t.Fatal(err)
	}
	return s, f, ic
}
func authGrantInput(f *authProfileFixture, nonce string) CoreProfileConsentInput {
	p := f.report.Profiles[0]
	return CoreProfileConsentInput{SourceID: f.source.ID, ExpectedSourceRevision: 1, ProfileID: p.ID, ProfileRevision: p.ProfileRevision, Provider: p.Provider, Model: p.Model, ProviderOrigin: p.ProviderOrigin, ChannelID: "conversation-bot-meiiie", ExpectedChannelRevision: 1, BotIDs: []string{"bot-meiiie"}, CoreProfileLimits: CoreProfileLimits{64000, 128, 3, 3, 600}, TTLSeconds: 600, ApproveExecution: true, ClientNonce: nonce}
}
func TestCoreAuthV2MetadataConsentExpiryAndImmutablePin(t *testing.T) {
	expiry := time.Now().Add(90 * time.Second).UnixMilli()
	report := authReport(t, "process_env", "static_key", "api_key", expiry, true)
	s, f, ic := authFixture(t, report)
	r, err := s.CoreProfiles(f.source.ID, ic.ID)
	if err != nil {
		t.Fatal("metadata v2 fixture rejected", err)
	}
	rObject := authObject(t, r)
	if r.Version != 2 || rObject["sourceSnapshot"] == nil {
		t.Fatal("report v2 snapshot provenance absent")
	}
	in := authGrantInput(f, "auth-v2-consent")
	c, dup, err := s.ApproveCoreProfile(in)
	if err != nil || dup || c.ExpiresAtUnixMS != expiry {
		t.Fatal("credential expiry must bound consent", err)
	}
	w, dup, err := s.AcceptProduct(Input{"bot-meiiie", "auth-v2-work", "Bounded selected auth request", "foreground"}, SubmissionScope{ChannelID: in.ChannelID, CoreProfileSelection: &CoreProfileSelection{c.ID, c.Revision}})
	if err != nil || dup {
		t.Fatal(err)
	}
	pin := authObject(t, w.CoreProfilePin)
	if pin["version"] != float64(2) || pin["authKind"] != "api_key" || pin["credentialSource"] != "process_env" || pin["authPolicy"] != "static_key" || pin["credentialSnapshotId"] != "cs1_"+strings.Repeat("b", 32) || pin["credentialExpiresAtUnixMs"] != float64(expiry) || w.CoreProfilePin.ExpiresAtUnixMS != expiry {
		t.Fatal("immutable pin2 auth tuple missing")
	}
	before := s.Snapshot().Sequence
	original := *w.CoreProfilePin
	cAgain, dup, err := s.ApproveCoreProfile(in)
	if err != nil || !dup || cAgain.ExpiresAtUnixMS != expiry || s.Snapshot().Sequence != before {
		t.Fatal("duplicate renewed bounded auth expiry", err)
	}
	dir := s.dir
	s.Close()
	restored, err := Open(dir)
	if err != nil {
		t.Fatal("auth2 replay", err)
	}
	defer restored.Close()
	if restored.RuntimeAllowed(w.RequestID, w.Epoch) {
		t.Fatal("replay silently restores broker authority")
	}
	restored.ConfigureCoreProfiles(f)
	again, _ := restored.Get(w.RequestID)
	if *again.CoreProfilePin != original {
		t.Fatal("auth snapshot changed after replay")
	}
	rReceipt, err := restored.CoreProfileReceipt("execution", in.ClientNonce)
	if err != nil || !rReceipt.Found || rReceipt.Consent.ExpiresAtUnixMS != expiry {
		t.Fatal("bounded receipt after replay", err)
	}
}

func TestCoreAuthReceiptReportsRetiredPrivateBindingWithoutNewCapture(t *testing.T) {
	report := authReport(t, "process_env", "static_key", "api_key", time.Now().Add(90*time.Second).UnixMilli(), true)
	s, f, ic := authFixture(t, report)
	if _, err := s.CoreProfiles(f.source.ID, ic.ID); err != nil {
		t.Fatal(err)
	}
	in := authGrantInput(f, "auth-receipt-source-current")
	c, _, err := s.ApproveCoreProfile(in)
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	reads := f.reads
	f.verify = func(CoreProfilePin) error { return errors.New("fixture current private snapshot changed") }
	f.mu.Unlock()
	before := s.Snapshot().Sequence
	r, err := s.CoreProfileReceipt("execution", in.ClientNonce)
	if err != nil || !r.Found || r.Active || !reflect.DeepEqual(r.Consent, &c) || s.Snapshot().Sequence != before {
		t.Fatal("stale private source receipt must remain historical and inactive", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.reads != reads {
		t.Fatal("receipt lookup recaptured credential source")
	}
}
func TestCoreAuthClosedPolicyAndSnapshotMatrix(t *testing.T) {
	now := time.Now().UnixMilli()
	for _, tc := range []struct {
		name     string
		source   string
		policy   string
		auth     string
		snapshot bool
		expiry   int64
		want     bool
	}{
		{"file key", "file_api_key", "static_key", "api_key", false, 0, true},
		{"local none", "local_none", "static_key", "none", false, 0, true},
		{"env key", "process_env", "static_key", "api_key", true, now + 90000, true},
		{"Kimi file cache", "oauth_cache", "cached_token_no_refresh", "kimi_oauth", false, now + 90000, true},
		{"Kimi broker cache", "oauth_cache", "cached_token_no_refresh", "kimi_oauth", true, now + 90000, true},
		{"env missing snapshot", "process_env", "static_key", "api_key", false, now + 90000, false},
		{"env missing expiry", "process_env", "static_key", "api_key", true, 0, false},
		{"Kimi missing expiry", "oauth_cache", "cached_token_no_refresh", "kimi_oauth", false, 0, false},
		{"Kimi refresh policy", "oauth_cache", "refresh", "kimi_oauth", false, now + 90000, false},
		{"OAuth other", "oauth_cache", "cached_token_no_refresh", "chatgpt_oauth", false, now + 90000, false},
		{"unknown source", "environment", "static_key", "api_key", true, now + 90000, false},
		{"file borrowed snapshot", "file_api_key", "static_key", "api_key", true, now + 90000, false},
		{"unsafe expiry", "process_env", "static_key", "api_key", true, 1 << 53, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := authReport(t, tc.source, tc.policy, tc.auth, tc.expiry, tc.snapshot)
			s, f, ic := authFixture(t, r)
			before := s.Snapshot().Sequence
			_, err := s.CoreProfiles(f.source.ID, ic.ID)
			if (err == nil) != tc.want {
				t.Fatal("closed metadata policy result", err)
			}
			if !tc.want && s.Snapshot().Sequence != before {
				t.Fatal("malformed metadata persisted")
			}
			if !tc.want {
				return
			}
			c, _, err := s.ApproveCoreProfile(authGrantInput(f, "auth-matrix-consent"))
			if err != nil {
				t.Fatal("supported policy rejected", err)
			}
			pin := profileConsentPin(c)
			if pin.Version != 2 || pin.AuthKind != tc.auth || pin.CredentialSource != tc.source || pin.AuthPolicy != tc.policy {
				t.Fatal("pin2 policy provenance")
			}
		})
	}
	for _, tc := range []struct {
		name   string
		modify func(*CoreProfileMetadataReport)
	}{
		{"env report snapshot absent", func(r *CoreProfileMetadataReport) { r.SourceSnapshot = nil }},
		{"snapshot id differs", func(r *CoreProfileMetadataReport) { r.SourceSnapshot.ID = "cs1_" + strings.Repeat("c", 32) }},
		{"credential exceeds snapshot", func(r *CoreProfileMetadataReport) {
			r.Profiles[0].CredentialExpiresAtUnixMS = r.SourceSnapshot.ExpiresAtUnixMS + 1
		}},
		{"snapshot reversed time", func(r *CoreProfileMetadataReport) {
			r.SourceSnapshot.ExpiresAtUnixMS = r.SourceSnapshot.CreatedAtUnixMS
		}},
		{"snapshot exceeds bounded duration", func(r *CoreProfileMetadataReport) {
			r.SourceSnapshot.ExpiresAtUnixMS = r.SourceSnapshot.CreatedAtUnixMS + 300001
		}},
		{"v1 with auth fields", func(r *CoreProfileMetadataReport) { r.Version = 1 }},
		{"v2 legacy revision", func(r *CoreProfileMetadataReport) { r.Profiles[0].ProfileRevision = "pr1_" + strings.Repeat("a", 64) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := authReport(t, "process_env", "static_key", "api_key", now+90000, true)
			tc.modify(&r)
			s, f, ic := authFixture(t, r)
			before := s.Snapshot().Sequence
			if _, err := s.CoreProfiles(f.source.ID, ic.ID); err == nil || s.Snapshot().Sequence != before {
				t.Fatal("snapshot relationship accepted or persisted", err)
			}
		})
	}
}
func TestCoreAuthCredentialExpiryRevokeNonceAndRestartFence(t *testing.T) {
	expiry := time.Now().Add(45 * time.Second).UnixMilli()
	s, f, ic := authFixture(t, authReport(t, "oauth_cache", "cached_token_no_refresh", "kimi_oauth", expiry, false))
	if _, err := s.CoreProfiles(f.source.ID, ic.ID); err != nil {
		t.Fatal(err)
	}
	in := authGrantInput(f, "auth-expiry-consent")
	c, _, err := s.ApproveCoreProfile(in)
	if err != nil {
		t.Fatal(err)
	}
	w, _, err := s.AcceptProduct(Input{"bot-meiiie", "auth-expiry-work", "Bounded cached token request", "foreground"}, SubmissionScope{ChannelID: in.ChannelID, CoreProfileSelection: &CoreProfileSelection{c.ID, c.Revision}})
	if err != nil {
		t.Fatal(err)
	}
	clock := time.UnixMilli(expiry + 1)
	s.now = func() time.Time { return clock }
	before := s.Snapshot().Sequence
	if _, _, err = s.ApproveCoreProfile(authGrantInput(f, "auth-expired-new-grant")); !errors.Is(err, ErrConflict) || s.Snapshot().Sequence != before {
		t.Fatal("expired credential minted consent", err)
	}
	if s.RuntimeAllowed(w.RequestID, w.Epoch) {
		t.Fatal("expired credential work runnable")
	}
	if _, err = s.Claim(w.RequestID, w.Epoch); err == nil {
		t.Fatal("expired credential claimed")
	}
	if err = s.ReconcileCoreProfiles(); err != nil {
		t.Fatal(err)
	}
	current, _ := s.Get(w.RequestID)
	if current.Status != "suspended" || current.Failure != "core_profile_authority_changed" {
		t.Fatal("credential expiry not suspended")
	}
	originalPin := *w.CoreProfilePin
	before = s.Snapshot().Sequence
	dupWork, dup, err := s.AcceptProduct(Input{"bot-meiiie", w.ClientNonce, w.Text, w.Lane}, SubmissionScope{ChannelID: in.ChannelID, CoreProfileSelection: &CoreProfileSelection{c.ID, 1}})
	if err != nil || !dup || *dupWork.CoreProfilePin != originalPin || s.Snapshot().Sequence != before {
		t.Fatal("expired nonce renewed broker snapshot", err)
	}
	clock = time.UnixMilli(expiry - 10000)
	r, _ := s.CoreProfileReceipt("execution", in.ClientNonce)
	if r.Active {
		t.Fatal("clock rewind revived expired cached token consent")
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
	r, err = restored.CoreProfileReceipt("execution", in.ClientNonce)
	if err != nil || r.Active {
		t.Fatal("restart revived expired credential consent", err)
	}
}
func TestCoreAuthSnapshotRotationAndExactPinTuple(t *testing.T) {
	expiry := time.Now().Add(90 * time.Second).UnixMilli()
	s, f, ic := authFixture(t, authReport(t, "process_env", "static_key", "api_key", expiry, true))
	if _, err := s.CoreProfiles(f.source.ID, ic.ID); err != nil {
		t.Fatal(err)
	}
	in := authGrantInput(f, "auth-rotation-consent")
	c, _, err := s.ApproveCoreProfile(in)
	if err != nil {
		t.Fatal(err)
	}
	w, _, err := s.AcceptProduct(Input{"bot-meiiie", "auth-rotation-work", "Bounded snapshot request", "foreground"}, SubmissionScope{ChannelID: in.ChannelID, CoreProfileSelection: &CoreProfileSelection{c.ID, 1}})
	if err != nil {
		t.Fatal(err)
	}
	for _, modify := range []func(*CoreProfilePin){func(p *CoreProfilePin) { p.AuthKind = "none" }, func(p *CoreProfilePin) { p.CredentialSource = "file_api_key" }, func(p *CoreProfilePin) { p.AuthPolicy = "cached_token_no_refresh" }, func(p *CoreProfilePin) { p.CredentialSnapshotID = "cs1_" + strings.Repeat("d", 32) }, func(p *CoreProfilePin) { p.CredentialExpiresAtUnixMS++ }, func(p *CoreProfilePin) { p.Version = 1 }} {
		probe := copyWork(w)
		modify(probe.CoreProfilePin)
		s.mu.Lock()
		accepted := s.coreProfileCurrent(probe) || s.validCoreProfileIntake(probe)
		s.mu.Unlock()
		if accepted {
			t.Fatal("different auth tuple admitted")
		}
	}
	f.mu.Lock()
	f.verify = func(p CoreProfilePin) error {
		if p.CredentialSnapshotID == w.CoreProfilePin.CredentialSnapshotID {
			return errors.New("owned snapshot retired")
		}
		return nil
	}
	f.mu.Unlock()
	if err = s.VerifyWorkCoreProfile(w.RequestID, w.Epoch); err == nil {
		t.Fatal("retired broker snapshot verified")
	}
	if err = s.ReconcileCoreProfiles(); err != nil {
		t.Fatal(err)
	}
	current, _ := s.Get(w.RequestID)
	if current.Status != "suspended" || current.Failure != "core_profile_authority_changed" || !reflect.DeepEqual(current.CoreProfilePin, w.CoreProfilePin) {
		t.Fatal("rotation replaced pin or failed to suspend")
	}
	control := CoreProfileConsentControlInput{c.ID, 1, "revoke", "auth-rotation-revoke"}
	if _, dup, err := s.ControlCoreProfile(control); err != nil || dup {
		t.Fatal(err)
	}
	before := s.Snapshot().Sequence
	if _, dup, err := s.ControlCoreProfile(control); err != nil || !dup || s.Snapshot().Sequence != before {
		t.Fatal("revoke nonce not durable", err)
	}
}
func TestCoreAuthLegacyJSONShapeAndNoPinMigration(t *testing.T) {
	p := CoreProfileMetadata{ID: "legacy", Provider: "openai_compat", Model: "model", AuthKind: "api_key", ConfigurationStatus: "configured", CredentialReadiness: "not_checked", ProfileRevision: "pr1_" + strings.Repeat("a", 64), ProviderOrigin: "https://api.example.invalid", BridgeTransport: "supported"}
	raw, _ := json.Marshal(CoreProfileMetadataReport{Version: 1, Profiles: []CoreProfileMetadata{p}})
	for _, field := range []string{"sourceSnapshot", "credentialSource", "authPolicy", "credentialSnapshotId", "credentialExpiresAtUnixMs"} {
		if strings.Contains(string(raw), field) {
			t.Fatal("legacy report shape gained auth field")
		}
	}
	c := CoreProfileConsent{Version: 1, ID: "core-consent-" + strings.Repeat("a", 32), Revision: 1, Profile: p}
	pin := profileConsentPin(c)
	pinRaw, _ := json.Marshal(pin)
	if pin.Version != 1 || pin.AuthKind != "" || strings.Contains(string(pinRaw), "authKind") || strings.Contains(string(pinRaw), "credential") || strings.Contains(string(pinRaw), "authPolicy") {
		t.Fatal("legacy pin silently migrated")
	}
	if !CoreProfileCredentialPolicyValid(p) {
		t.Fatal("legacy policy denied")
	}
}
func TestCoreAuthReplayRejectsExtendedCredentialConsent(t *testing.T) {
	expiry := time.Now().Add(60 * time.Second).UnixMilli()
	s, f, ic := authFixture(t, authReport(t, "process_env", "static_key", "api_key", expiry, true))
	if _, err := s.CoreProfiles(f.source.ID, ic.ID); err != nil {
		t.Fatal(err)
	}
	in := authGrantInput(f, "auth-replay-bounded-consent")
	c, _, err := s.ApproveCoreProfile(in)
	if err != nil {
		t.Fatal(err)
	}
	prefix, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer prefix.Close()
	for _, event := range s.events {
		if event.CoreProfile != nil && event.CoreProfile.ConsentInput != nil {
			if !prefix.validCoreProfileEvent(event) {
				t.Fatal("original clipped event rejected")
			}
			for _, modify := range []func(*CoreProfileConsent){func(c *CoreProfileConsent) { c.ExpiresAtUnixMS = c.CreatedAtUnixMS + 600000 }, func(c *CoreProfileConsent) { c.Profile.CredentialExpiresAtUnixMS++ }, func(c *CoreProfileConsent) { c.Profile.CredentialSnapshotID = "cs1_" + strings.Repeat("e", 32) }, func(c *CoreProfileConsent) { c.Profile.AuthPolicy = "refresh" }} {
				probe := event
				payload := *event.CoreProfile
				consent := copyCoreConsent(c)
				modify(&consent)
				payload.Consent = &consent
				probe.CoreProfile = &payload
				if prefix.validCoreProfileEvent(probe) {
					t.Fatal("tampered credential consent replay accepted")
				}
			}
			break
		}
		prefix.apply(event)
	}
	for _, expiry := range []int64{0, -1, c.CreatedAtUnixMS, c.CreatedAtUnixMS - 1, 1 << 53} {
		p := c.Profile
		p.CredentialExpiresAtUnixMS = expiry
		if expiry == 0 {
			p.CredentialSource = "process_env"
		}
		if expiry != 0 {
			if _, ok := coreProfileConsentExpiry(c.CreatedAtUnixMS, 600, p); ok {
				t.Fatal("nonpositive or unsafe credential lifetime accepted")
			}
		} else if CoreProfileCredentialPolicyValid(p) {
			t.Fatal("env source withoutexpiry accepted")
		}
	}
}
