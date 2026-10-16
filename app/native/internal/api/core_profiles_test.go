package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"nekobot.local/native/internal/ledger"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestCoreProfilesDefaultOff(t *testing.T) {
	s, c := setup(t)
	r, err := c.Get(s.URL + "/v1/core-profile-sources")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	if r.StatusCode != 403 {
		t.Fatalf("unconfigured bridge must explicitly deny, got %d", r.StatusCode)
	}
}

type coreProfilesHTTPFixture struct {
	digest        string
	inspections   int
	verifications int
	profile       ledger.CoreProfileMetadata
	fail          bool
}

func (f *coreProfilesHTTPFixture) Sources() []ledger.CoreProfileSource {
	return []ledger.CoreProfileSource{{Version: 1, ID: "core-source-owned-fixture", Revision: 1, Label: "Owned API fixture", RuntimeSHA256: f.digest, FixtureOnly: true}}
}
func (f *coreProfilesHTTPFixture) Inspect(context.Context, string) (ledger.CoreProfileMetadataReport, error) {
	f.inspections++
	if f.fail {
		return ledger.CoreProfileMetadataReport{}, errors.New("private-fixture-error-never-export")
	}
	return ledger.CoreProfileMetadataReport{Version: 1, Profiles: []ledger.CoreProfileMetadata{f.profile}}, nil
}
func (f *coreProfilesHTTPFixture) Verify(context.Context, ledger.CoreProfilePin) error {
	f.verifications++
	if f.fail {
		return errors.New("private-fixture-error-never-export")
	}
	return nil
}
func profileJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
func profileHTTPDecode(t *testing.T, r *http.Response, code int, out any) {
	t.Helper()
	defer r.Body.Close()
	if r.StatusCode != code {
		raw, _ := io.ReadAll(r.Body)
		t.Fatalf("HTTP status %d want %d safe response %s", r.StatusCode, code, raw)
	}
	if out != nil && json.NewDecoder(r.Body).Decode(out) != nil {
		t.Fatal("profile DTO decode")
	}
}
func TestCoreProfilesAuthenticatedConsentIntakeAndReadonlyReceipts(t *testing.T) {
	raw, readErr := os.ReadFile("../protocol/core-runtimes-v1.json")
	if readErr != nil {
		t.Fatal(readErr)
	}
	var pins map[string]struct {
		SHA256 string `json:"sha256"`
	}
	if json.Unmarshal(raw, &pins) != nil {
		t.Fatal("fixture protocol pin")
	}
	server, client := setup(t)
	handler := server.Config.Handler.(*Server)
	f := &coreProfilesHTTPFixture{digest: pins["windows_x64"].SHA256, profile: ledger.CoreProfileMetadata{ID: "profile-owned-fixture", Provider: "openai_compat", Model: "source-fixture-model", AuthKind: "api_key", Active: true, ConfigurationStatus: "configured", CredentialReadiness: "not_checked", ProfileRevision: "pr1_" + strings.Repeat("b", 64), ProviderOrigin: "http://127.0.0.1:12345", BridgeTransport: "supported"}}
	handler.EnableCoreProfiles(f)
	r, err := http.Get(server.URL + "/v1/core-profile-sources")
	if err != nil {
		t.Fatal(err)
	}
	profileHTTPDecode(t, r, 401, nil)
	r, err = client.Get(server.URL + "/v1/core-profile-sources")
	if err != nil {
		t.Fatal(err)
	}
	var list map[string]any
	profileHTTPDecode(t, r, 200, &list)
	if f.inspections != 0 || f.verifications != 0 {
		t.Fatal("source listing inspected profiles")
	}
	r, err = client.Get(server.URL + "/v1/core-profiles?sourceId=core-source-owned-fixture&inspectionConsentId=core-inspection-" + strings.Repeat("a", 32))
	if err != nil {
		t.Fatal(err)
	}
	profileHTTPDecode(t, r, 403, nil)
	if f.inspections != 0 {
		t.Fatal("missing consent read Core metadata")
	}
	body := `{"sourceId":"core-source-owned-fixture","expectedSourceRevision":1,"approveInspection":true,"ttlSeconds":600,"clientNonce":"api-profile-inspection"}`
	for _, bad := range []string{strings.TrimSuffix(body, "}") + `,"actorId":"forged"}`, strings.TrimSuffix(body, "}") + `,"sourceId":"duplicate"}`, strings.Replace(body, `"approveInspection":true`, `"approveInspection":false`, 1)} {
		profileHTTPDecode(t, post(t, client, server.URL, "/v1/core-profile-inspection-consents", bad, ""), 400, nil)
	}
	var approval struct {
		InspectionConsent ledger.CoreProfileInspectionConsent `json:"inspectionConsent"`
		Duplicate         bool                                `json:"duplicate"`
	}
	profileHTTPDecode(t, post(t, client, server.URL, "/v1/core-profile-inspection-consents", body, ""), 200, &approval)
	if approval.Duplicate || approval.InspectionConsent.ActorScope.ActorID != "local-owner" {
		t.Fatal("inspection actor pin")
	}
	r, err = client.Get(server.URL + "/v1/core-profiles?sourceId=core-source-owned-fixture&inspectionConsentId=" + approval.InspectionConsent.ID)
	if err != nil {
		t.Fatal(err)
	}
	var metadata ledger.CoreProfilesResult
	profileHTTPDecode(t, r, 200, &metadata)
	if len(metadata.Profiles) != 1 || metadata.Profiles[0].CredentialReadiness != "not_checked" || f.inspections != 1 {
		t.Fatal("metadata readiness")
	}
	p := f.profile
	in := ledger.CoreProfileConsentInput{SourceID: "core-source-owned-fixture", ExpectedSourceRevision: 1, ProfileID: p.ID, ProfileRevision: p.ProfileRevision, Provider: p.Provider, Model: p.Model, ProviderOrigin: p.ProviderOrigin, ChannelID: "conversation-bot-meiiie", ExpectedChannelRevision: 1, BotIDs: []string{"bot-meiiie"}, CoreProfileLimits: ledger.CoreProfileLimits{MaxInputBytes: 64000, MaxOutputTokens: 128, MaxRequests: 3, MaxSteps: 3, MaxSeconds: 60}, TTLSeconds: 600, ApproveExecution: true, ClientNonce: "api-profile-execution"}
	var execution struct {
		Consent   ledger.CoreProfileConsent `json:"consent"`
		Duplicate bool                      `json:"duplicate"`
	}
	profileHTTPDecode(t, post(t, client, server.URL, "/v1/core-profile-consents", profileJSON(t, in), "https://untrusted.invalid"), 403, nil)
	profileHTTPDecode(t, post(t, client, server.URL, "/v1/core-profile-consents", profileJSON(t, in), ""), 200, &execution)
	if execution.Duplicate || execution.Consent.Profile.Model != "source-fixture-model" {
		t.Fatal("source model consent pin")
	}
	message := map[string]any{"botId": "bot-meiiie", "clientNonce": "api-profile-message", "text": "Use selected profile.", "lane": "foreground", "channelId": "conversation-bot-meiiie", "expectedChannelRevision": 1, "coreProfileSelection": ledger.CoreProfileSelection{ConsentID: execution.Consent.ID, ExpectedConsentRevision: 1}}
	var result struct {
		Work      ledger.Work `json:"work"`
		Duplicate bool        `json:"duplicate"`
	}
	profileHTTPDecode(t, post(t, client, server.URL, "/v1/messages", profileJSON(t, message), ""), 201, &result)
	if result.Work.CoreProfilePin == nil || result.Work.CoreProfilePin.Model != p.Model || result.Work.Account != nil {
		t.Fatal("immutable Work profile selection")
	}
	control := ledger.CoreProfileConsentControlInput{ConsentID: execution.Consent.ID, ExpectedRevision: 1, Action: "revoke", ClientNonce: "api-profile-revoke"}
	profileHTTPDecode(t, post(t, client, server.URL, "/v1/core-profile-consents/control", profileJSON(t, control), ""), 200, nil)
	before := handler.store.Snapshot().Sequence
	inspections, verifications := f.inspections, f.verifications
	r, err = client.Get(server.URL + "/v1/core-profile-receipts?kind=execution&clientNonce=" + in.ClientNonce)
	if err != nil {
		t.Fatal(err)
	}
	var receipt ledger.CoreProfileReceipt
	profileHTTPDecode(t, r, 200, &receipt)
	if !receipt.Found || receipt.Active || receipt.Consent.Revision != 1 || receipt.Consent.Status != "approved" {
		t.Fatal("immutable historical receipt")
	}
	if handler.store.Snapshot().Sequence != before || inspections != f.inspections || verifications != f.verifications {
		t.Fatal("receipt read inspected source or mutated journal")
	}
	profileHTTPDecode(t, post(t, client, server.URL, "/v1/messages", profileJSON(t, message), ""), 200, &result)
	if !result.Duplicate || handler.store.Snapshot().Sequence != before {
		t.Fatal("revoked Work nonce created execution")
	}
	message["clientNonce"] = "api-profile-after-revoke"
	profileHTTPDecode(t, post(t, client, server.URL, "/v1/messages", profileJSON(t, message), ""), 409, nil)
}
func TestCoreProfilesFixturePrincipalCannotInspectOrGrant(t *testing.T) {
	h := newFixtureHTTP(t)
	f := &coreProfilesHTTPFixture{}
	h.server.Config.Handler.(*Server).EnableCoreProfiles(f)
	before := h.store.Snapshot().Sequence
	for _, who := range []string{"owner", "bob", "reader", "eve"} {
		r, err := h.clients[who].Get(h.server.URL + "/v1/core-profile-sources")
		if err != nil {
			t.Fatal(err)
		}
		profileHTTPDecode(t, r, 403, nil)
	}
	if f.inspections != 0 || f.verifications != 0 || h.store.Snapshot().Sequence != before {
		t.Fatal("fixture principal granted profile authority")
	}
}
