package ledger

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"nekobot.local/native/internal/protocol"
	"net"
	"net/url"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type CoreProfileInspectionInput struct {
	SourceID               string `json:"sourceId"`
	ExpectedSourceRevision uint64 `json:"expectedSourceRevision"`
	ApproveInspection      bool   `json:"approveInspection"`
	TTLSeconds             int    `json:"ttlSeconds"`
	ClientNonce            string `json:"clientNonce"`
}
type CoreProfileInspectionConsent struct {
	Version         int                   `json:"version"`
	ID              string                `json:"id"`
	Source          CoreProfileSource     `json:"source"`
	ActorScope      CoreProfileActorScope `json:"actorScope"`
	CreatedAtUnixMS int64                 `json:"createdAtUnixMs"`
	ExpiresAtUnixMS int64                 `json:"expiresAtUnixMs"`
}
type CoreProfileConsentInput struct {
	SourceID                string   `json:"sourceId"`
	ExpectedSourceRevision  uint64   `json:"expectedSourceRevision"`
	ProfileID               string   `json:"profileId"`
	ProfileRevision         string   `json:"profileRevision"`
	Provider                string   `json:"provider"`
	Model                   string   `json:"model"`
	ProviderOrigin          string   `json:"providerOrigin"`
	ChannelID               string   `json:"channelId"`
	ExpectedChannelRevision uint64   `json:"expectedChannelRevision"`
	BotIDs                  []string `json:"botIds"`
	ProjectID               string   `json:"projectId,omitempty"`
	ExpectedProjectRevision uint64   `json:"expectedProjectRevision,omitempty"`
	CoreProfileLimits
	TTLSeconds       int    `json:"ttlSeconds"`
	ApproveExecution bool   `json:"approveExecution"`
	ClientNonce      string `json:"clientNonce"`
}
type CoreProfileConsentControlInput struct {
	ConsentID        string `json:"consentId"`
	ExpectedRevision uint64 `json:"expectedRevision"`
	Action           string `json:"action"`
	ClientNonce      string `json:"clientNonce"`
}
type CoreProfileConsent struct {
	Version             int                   `json:"version"`
	ID                  string                `json:"id"`
	Revision            uint64                `json:"revision"`
	Status              string                `json:"status"`
	Source              CoreProfileSource     `json:"source"`
	Profile             CoreProfileMetadata   `json:"profile"`
	InspectionConsentID string                `json:"inspectionConsentId"`
	ActorScope          CoreProfileActorScope `json:"actorScope"`
	ChannelID           string                `json:"channelId"`
	AudienceRevision    uint64                `json:"audienceRevision"`
	BotIDs              []string              `json:"botIds"`
	ProjectID           string                `json:"projectId,omitempty"`
	ProjectRevision     uint64                `json:"projectRevision,omitempty"`
	Limits              CoreProfileLimits     `json:"limits"`
	CreatedAtUnixMS     int64                 `json:"createdAtUnixMs"`
	ExpiresAtUnixMS     int64                 `json:"expiresAtUnixMs"`
}
type CoreProfilesResult struct {
	SourceSnapshot      *CoreProfileSourceSnapshot `json:"sourceSnapshot,omitempty"`
	CapturedAtUnixMS    int64                      `json:"capturedAtUnixMs"`
	Version             int                        `json:"version"`
	SourceID            string                     `json:"sourceId"`
	InspectionConsentID string                     `json:"inspectionConsentId"`
	Profiles            []CoreProfileMetadata      `json:"profiles"`
}
type CoreProfileEvent struct {
	ObservedAtUnixMS  int64                           `json:"observedAtUnixMs,omitempty"`
	InspectionInput   *CoreProfileInspectionInput     `json:"inspectionInput,omitempty"`
	InspectionConsent *CoreProfileInspectionConsent   `json:"inspectionConsent,omitempty"`
	Metadata          *CoreProfilesResult             `json:"metadata,omitempty"`
	ConsentInput      *CoreProfileConsentInput        `json:"consentInput,omitempty"`
	ControlInput      *CoreProfileConsentControlInput `json:"controlInput,omitempty"`
	Consent           *CoreProfileConsent             `json:"consent,omitempty"`
}
type CoreProfileReceipt struct {
	Version           int                           `json:"version"`
	Found             bool                          `json:"found"`
	Kind              string                        `json:"kind"`
	ClientNonce       string                        `json:"clientNonce"`
	Input             any                           `json:"input,omitempty"`
	InspectionConsent *CoreProfileInspectionConsent `json:"inspectionConsent,omitempty"`
	Consent           *CoreProfileConsent           `json:"consent,omitempty"`
	Active            bool                          `json:"active"`
}

func localProfileActor() CoreProfileActorScope {
	return CoreProfileActorScope{1, "local_owner", "local-owner", "local-space", "nekobot-local", "local-executor", 1}
}
func safeProfileText(s string, n int) bool {
	if s == "" || len(s) > n || strings.TrimSpace(s) != s {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

var coreProfileID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

func validCoreSource(s CoreProfileSource) bool {
	return s.Version == 1 && coreProfileID.MatchString(s.ID) && s.Revision > 0 && s.Revision < 1<<53 && safeProfileText(s.Label, 120) && lowerHex(s.RuntimeSHA256, 64) && protocol.CoreProfileBridgeKnown(s.RuntimeSHA256)
}
func validProfileOrigin(raw string, fixture bool) bool {
	if len(raw) > 256 {
		return false
	}
	u, e := url.Parse(raw)
	if e != nil || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Path != "" || u.RawPath != "" || u.Opaque != "" || u.Host == "" || u.String() != raw {
		return false
	}
	if u.Port() != "" {
		port, err := strconv.Atoi(u.Port())
		if err != nil || port < 1 || port > 65535 {
			return false
		}
	}
	for _, r := range u.Hostname() {
		if r > 127 || !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == ':') {
			return false
		}
	}
	if fixture {
		ip := net.ParseIP(u.Hostname())
		return u.Scheme == "http" && ip != nil && ip.IsLoopback() && u.Port() != ""
	}
	return u.Scheme == "https" && strings.ToLower(u.Host) == u.Host
}
func validCoreProfile(p CoreProfileMetadata, source CoreProfileSource) bool {
	auth := profileContains([]string{"api_key", "none", "chatgpt_oauth", "gemini_oauth", "grok_oauth", "kimi_oauth", "opencode_oauth", "cline_oauth"}, p.AuthKind)
	model := safeProfileText(p.Model, 120)
	provider := safeProfileText(p.Provider, 64)
	origin := validProfileOrigin(p.ProviderOrigin, source.FixtureOnly)
	if p.BridgeTransport == "unsupported" {
		origin = origin || validProfileOrigin(p.ProviderOrigin, false) || unsupportedLoopbackOrigin(p.ProviderOrigin)
	}
	if p.ConfigurationStatus == "incomplete" && p.BridgeTransport == "unsupported" {
		model = model || p.Model == ""
		origin = origin || p.ProviderOrigin == ""
		provider = provider || p.Provider == ""
	}
	return coreProfileID.MatchString(p.ID) && provider && model && auth && (p.ConfigurationStatus == "configured" || p.ConfigurationStatus == "incomplete") && p.CredentialReadiness == "not_checked" && CoreProfileCredentialPolicyValid(p) && (p.BridgeTransport == "supported" || p.BridgeTransport == "unsupported") && origin
}
func credentialPolicyVersion(p CoreProfileMetadata) int {
	if p.CredentialSource == "" && p.AuthPolicy == "" && p.CredentialSnapshotID == "" && p.CredentialExpiresAtUnixMS == 0 {
		return 1
	}
	return 2
}
func validCredentialSnapshotID(id string) bool {
	return strings.HasPrefix(id, "cs1_") && lowerHex(strings.TrimPrefix(id, "cs1_"), 32)
}
func CoreProfileCredentialPolicyValid(p CoreProfileMetadata) bool {
	version := credentialPolicyVersion(p)
	prefix := "pr1_"
	if version == 2 {
		prefix = "pr2_"
	}
	if !strings.HasPrefix(p.ProfileRevision, prefix) || !lowerHex(strings.TrimPrefix(p.ProfileRevision, prefix), 64) {
		return false
	}
	if version == 1 {
		return true
	}
	noSnapshot := p.CredentialSnapshotID == "" && p.CredentialExpiresAtUnixMS == 0
	expiry := p.CredentialExpiresAtUnixMS > 0 && p.CredentialExpiresAtUnixMS < 1<<53
	switch p.CredentialSource {
	case "file_api_key":
		return p.AuthKind == "api_key" && p.AuthPolicy == "static_key" && noSnapshot
	case "local_none":
		return p.AuthKind == "none" && p.AuthPolicy == "static_key" && noSnapshot
	case "process_env":
		return p.AuthKind == "api_key" && p.AuthPolicy == "static_key" && validCredentialSnapshotID(p.CredentialSnapshotID) && expiry
	case "oauth_cache":
		return p.Provider == "kimi" && p.AuthKind == "kimi_oauth" && p.AuthPolicy == "cached_token_no_refresh" && expiry && (p.CredentialSnapshotID == "" || validCredentialSnapshotID(p.CredentialSnapshotID))
	case "unavailable":
		return p.AuthPolicy == "unavailable" && p.BridgeTransport == "unsupported" && noSnapshot
	}
	return false
}
func validProfileSnapshot(snapshot *CoreProfileSourceSnapshot) bool {
	return snapshot != nil && validCredentialSnapshotID(snapshot.ID) && snapshot.CreatedAtUnixMS > 0 && snapshot.ExpiresAtUnixMS > snapshot.CreatedAtUnixMS && snapshot.ExpiresAtUnixMS < 1<<53 && snapshot.ExpiresAtUnixMS-snapshot.CreatedAtUnixMS <= 300000
}
func validProfileReport(r CoreProfilesResult, source CoreProfileSource) bool {
	if r.Version != 1 && r.Version != 2 || len(r.Profiles) > 64 {
		return false
	}
	if r.Version == 1 && r.SourceSnapshot != nil {
		return false
	}
	if r.SourceSnapshot != nil && (!validProfileSnapshot(r.SourceSnapshot) || r.CapturedAtUnixMS < r.SourceSnapshot.CreatedAtUnixMS || r.CapturedAtUnixMS >= r.SourceSnapshot.ExpiresAtUnixMS) {
		return false
	}
	seen := map[string]bool{}
	for _, p := range r.Profiles {
		if !validCoreProfile(p, source) || credentialPolicyVersion(p) != r.Version || seen[p.ID] {
			return false
		}
		seen[p.ID] = true
		if p.CredentialSnapshotID != "" && (r.SourceSnapshot == nil || p.CredentialSnapshotID != r.SourceSnapshot.ID || p.CredentialExpiresAtUnixMS > r.SourceSnapshot.ExpiresAtUnixMS) {
			return false
		}
	}
	return true
}
func coreProfileConsentExpiry(created int64, ttl int, p CoreProfileMetadata) (int64, bool) {
	if created <= 0 || ttl < 30 || ttl > 3600 || created >= 1<<53-int64(ttl)*1000 {
		return 0, false
	}
	expires := created + int64(ttl)*1000
	if p.CredentialExpiresAtUnixMS != 0 {
		if p.CredentialExpiresAtUnixMS <= created || p.CredentialExpiresAtUnixMS >= 1<<53 {
			return 0, false
		}
		if p.CredentialExpiresAtUnixMS < expires {
			expires = p.CredentialExpiresAtUnixMS
		}
	}
	return expires, expires > created
}
func unsupportedLoopbackOrigin(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.String() != raw || u.Scheme != "http" {
		return false
	}
	local := u.Hostname() == "localhost"
	if ip := net.ParseIP(u.Hostname()); ip != nil {
		local = ip.IsLoopback()
	}
	if !local {
		return false
	}
	u.Scheme = "https"
	return validProfileOrigin(u.String(), false)
}
func executableCoreProfile(p CoreProfileMetadata) bool {
	auth := p.AuthKind == "api_key" || p.AuthKind == "none"
	if credentialPolicyVersion(p) == 2 && p.Provider == "kimi" && p.AuthKind == "kimi_oauth" && p.CredentialSource == "oauth_cache" && p.AuthPolicy == "cached_token_no_refresh" {
		auth = true
	}
	return p.ConfigurationStatus == "configured" && p.BridgeTransport == "supported" && auth && CoreProfileCredentialPolicyValid(p) && profileContains([]string{"openai_compat", "responses", "anthropic", "kimi"}, p.Provider)
}
func validProfileLimits(l CoreProfileLimits) bool {
	return l.MaxInputBytes >= 1 && l.MaxInputBytes <= 64000 && l.MaxOutputTokens >= 1 && l.MaxOutputTokens <= 4096 && l.MaxRequests >= 1 && l.MaxRequests <= 8 && l.MaxSteps >= 1 && l.MaxSteps <= 6 && l.MaxSeconds >= 1 && l.MaxSeconds <= 600
}
func profileNow(s *Store) int64 {
	now := time.Now().UnixMilli()
	if s.now != nil {
		now = s.now().UnixMilli()
	}
	if now > s.coreProfileObservedNow {
		s.coreProfileObservedNow = now
	}
	return s.coreProfileObservedNow
}
func profileHash(v any) string {
	raw, _ := json.Marshal(v)
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}
func profileContains(ids []string, id string) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}
func (s *Store) ConfigureCoreProfiles(service CoreProfileService) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.coreProfiles = service
}
func (s *Store) coreProfilesEnabled() bool {
	return s.coreProfiles != nil && s.identity == nil && !s.readOnly
}
func (s *Store) coreSource(id string) (CoreProfileSource, bool) {
	if s.coreProfiles == nil {
		return CoreProfileSource{}, false
	}
	list := s.coreProfiles.Sources()
	if len(list) > 8 {
		return CoreProfileSource{}, false
	}
	seen := map[string]bool{}
	for _, c := range list {
		if !validCoreSource(c) || !protocol.CoreProfileBridgeAllowed(c.RuntimeSHA256) || seen[c.ID] {
			return CoreProfileSource{}, false
		}
		seen[c.ID] = true
	}
	for _, c := range list {
		if c.ID == id {
			return c, true
		}
	}
	return CoreProfileSource{}, false
}
func (s *Store) CoreProfileSources() ([]CoreProfileSource, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.coreProfilesEnabled() {
		return nil, ErrForbidden
	}
	list := append([]CoreProfileSource{}, s.coreProfiles.Sources()...)
	if len(list) > 8 {
		return nil, ErrUnavailable
	}
	seen := map[string]bool{}
	for _, c := range list {
		if !validCoreSource(c) || !protocol.CoreProfileBridgeAllowed(c.RuntimeSHA256) || seen[c.ID] {
			return nil, ErrUnavailable
		}
		seen[c.ID] = true
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	return list, nil
}
func (s *Store) profileNonceUsed(nonce string) bool {
	if nonce == "" {
		return false
	}
	_, own := s.coreProfileNonces[nonce]
	return own || s.challengeOtherNonceUsed(nonce) || s.reportDeliveryNonces[nonce] != "" || s.challengeNonces[nonce].Nonce != ""
}
func (s *Store) InspectCoreProfileConsent(in CoreProfileInspectionInput) (CoreProfileInspectionConsent, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.coreProfilesEnabled() {
		return CoreProfileInspectionConsent{}, false, ErrForbidden
	}
	if old, ok := s.coreProfileNonces[in.ClientNonce]; ok {
		if old.Type != "core_profile.inspection_approved" || !reflect.DeepEqual(old.CoreProfile.InspectionInput, &in) {
			return CoreProfileInspectionConsent{}, false, ErrConflict
		}
		return *old.CoreProfile.InspectionConsent, true, nil
	}
	if !in.ApproveInspection || in.TTLSeconds < 30 || in.TTLSeconds > 3600 || !validAccessNonce(in.ClientNonce) {
		return CoreProfileInspectionConsent{}, false, ErrInvalid
	}
	if s.profileNonceUsed(in.ClientNonce) || len(s.coreProfileNonces) >= 512 || len(s.coreProfileInspections) >= 128 {
		return CoreProfileInspectionConsent{}, false, ErrConflict
	}
	source, ok := s.coreSource(in.SourceID)
	if !ok || source.Revision != in.ExpectedSourceRevision {
		return CoreProfileInspectionConsent{}, false, ErrConflict
	}
	id, err := newID("core-inspection-")
	if err != nil {
		return CoreProfileInspectionConsent{}, false, err
	}
	now := profileNow(s)
	c := CoreProfileInspectionConsent{1, id, source, localProfileActor(), now, now + int64(in.TTLSeconds)*1000}
	e := Event{Type: "core_profile.inspection_approved", Nonce: in.ClientNonce, CoreProfile: &CoreProfileEvent{InspectionInput: &in, InspectionConsent: &c}}
	if !s.validCoreProfileEvent(e) {
		return c, false, ErrInvalid
	}
	if err := s.commit(e); err != nil {
		return c, false, err
	}
	return c, false, nil
}
func (s *Store) inspectionActive(c CoreProfileInspectionConsent) bool {
	if !s.coreProfilesEnabled() || s.coreProfileRetired[c.ID] || profileNow(s) < c.CreatedAtUnixMS || profileNow(s) >= c.ExpiresAtUnixMS {
		return false
	}
	current, ok := s.coreSource(c.Source.ID)
	return ok && current == c.Source
}
func (s *Store) CoreProfiles(sourceID, inspectionID string) (CoreProfilesResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.coreProfileInspections[inspectionID]
	if !ok || c.Source.ID != sourceID || !s.inspectionActive(c) {
		return CoreProfilesResult{}, ErrForbidden
	}
	if r, ok := s.coreProfileMetadata[inspectionID]; ok {
		return copyProfileMetadata(r), nil
	}
	if pending, ok := s.coreProfileLoads[inspectionID]; ok {
		s.mu.Unlock()
		timer := time.NewTimer(3 * time.Second)
		var result CoreProfilesResult
		var err error
		select {
		case <-pending:
			result, err = s.CoreProfiles(sourceID, inspectionID)
		case <-timer.C:
			err = ErrConflict
		}
		timer.Stop()
		s.mu.Lock()
		return result, err
	}
	if s.coreProfileLoads == nil {
		s.coreProfileLoads = map[string]chan struct{}{}
	}
	pending := make(chan struct{})
	s.coreProfileLoads[inspectionID] = pending
	defer func() { delete(s.coreProfileLoads, inspectionID); close(pending) }()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	service := s.coreProfiles
	s.mu.Unlock()
	report, err := service.Inspect(ctx, sourceID)
	s.mu.Lock()
	if err != nil || !s.inspectionActive(c) || (report.Version != 1 && report.Version != 2) || len(report.Profiles) > 64 {
		return CoreProfilesResult{}, ErrConflict
	}
	if r, ok := s.coreProfileMetadata[inspectionID]; ok {
		return copyProfileMetadata(r), nil
	}
	r := CoreProfilesResult{Version: report.Version, SourceSnapshot: report.SourceSnapshot, SourceID: sourceID, InspectionConsentID: inspectionID, Profiles: append([]CoreProfileMetadata{}, report.Profiles...), CapturedAtUnixMS: profileNow(s)}
	r = copyProfileMetadata(r)
	e := Event{Type: "core_profile.metadata_captured", CoreProfile: &CoreProfileEvent{Metadata: &r}}
	if !s.validCoreProfileEvent(e) {
		return CoreProfilesResult{}, ErrInvalid
	}
	if err = s.commit(e); err != nil {
		return CoreProfilesResult{}, err
	}
	return copyProfileMetadata(r), nil
}
func copyProfileMetadata(r CoreProfilesResult) CoreProfilesResult {
	if r.SourceSnapshot != nil {
		snapshot := *r.SourceSnapshot
		r.SourceSnapshot = &snapshot
	}
	r.Profiles = append([]CoreProfileMetadata{}, r.Profiles...)
	return r
}
func copyCoreConsent(c CoreProfileConsent) CoreProfileConsent {
	c.BotIDs = append([]string{}, c.BotIDs...)
	return c
}
func (s *Store) profileMatching(in CoreProfileConsentInput) (CoreProfileInspectionConsent, CoreProfileMetadata, bool) {
	ids := make([]string, 0, len(s.coreProfileMetadata))
	for id := range s.coreProfileMetadata {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	sort.SliceStable(ids, func(i, j int) bool {
		return s.coreProfileMetadata[ids[i]].CapturedAtUnixMS > s.coreProfileMetadata[ids[j]].CapturedAtUnixMS
	})
	for _, id := range ids {
		c := s.coreProfileInspections[id]
		if c.Source.ID != in.SourceID || c.Source.Revision != in.ExpectedSourceRevision {
			continue
		}
		for _, p := range s.coreProfileMetadata[id].Profiles {
			if p.ID == in.ProfileID && p.ProfileRevision == in.ProfileRevision && p.Provider == in.Provider && p.Model == in.Model && p.ProviderOrigin == in.ProviderOrigin {
				return c, p, true
			}
		}
	}
	return CoreProfileInspectionConsent{}, CoreProfileMetadata{}, false
}
func (s *Store) validConsentScope(c CoreProfileConsent) bool {
	if conversation, exists := s.conversation(c.ChannelID); exists && conversation.Archived {
		return false
	}
	channel, ok := s.channel(c.ChannelID)
	if !ok || channel.Revision != c.AudienceRevision || channel.OwnerUserID != "local-owner" || channel.SpaceID != "local-space" {
		return false
	}
	if len(c.BotIDs) < 1 || len(c.BotIDs) > 8 {
		return false
	}
	last := ""
	for _, id := range c.BotIDs {
		if id <= last || !profileContains(channel.BotIDs, id) || s.archivedBot(id) {
			return false
		}
		last = id
	}
	if c.ProjectID == "" {
		return c.ProjectRevision == 0
	}
	p, ok := s.projects[c.ProjectID]
	return ok && p.Revision == c.ProjectRevision && profileContains(p.ChannelIDs, c.ChannelID) && !s.archivedProject(p.ID)
}
func (s *Store) ApproveCoreProfile(in CoreProfileConsentInput) (CoreProfileConsent, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.coreProfilesEnabled() {
		return CoreProfileConsent{}, false, ErrForbidden
	}
	if old, ok := s.coreProfileNonces[in.ClientNonce]; ok {
		if old.Type != "core_profile.execution_approved" || !reflect.DeepEqual(old.CoreProfile.ConsentInput, &in) {
			return CoreProfileConsent{}, false, ErrConflict
		}
		return copyCoreConsent(*old.CoreProfile.Consent), true, nil
	}
	if !in.ApproveExecution || in.TTLSeconds < 30 || in.TTLSeconds > 3600 || !validProfileLimits(in.CoreProfileLimits) || !validAccessNonce(in.ClientNonce) {
		return CoreProfileConsent{}, false, ErrInvalid
	}
	if s.profileNonceUsed(in.ClientNonce) || len(s.coreProfileNonces) >= 512 || len(s.coreProfileConsents) >= 128 {
		return CoreProfileConsent{}, false, ErrConflict
	}
	ic, p, ok := s.profileMatching(in)
	if !ok || !s.inspectionActive(ic) || !executableCoreProfile(p) || !validProfileOrigin(p.ProviderOrigin, ic.Source.FixtureOnly) {
		return CoreProfileConsent{}, false, ErrConflict
	}
	id, err := newID("core-consent-")
	if err != nil {
		return CoreProfileConsent{}, false, err
	}
	now := profileNow(s)
	expires, expiryOK := coreProfileConsentExpiry(now, in.TTLSeconds, p)
	if !expiryOK {
		return CoreProfileConsent{}, false, ErrConflict
	}
	c := CoreProfileConsent{Version: 1, ID: id, Revision: 1, Status: "approved", Source: ic.Source, Profile: p, InspectionConsentID: ic.ID, ActorScope: localProfileActor(), ChannelID: in.ChannelID, AudienceRevision: in.ExpectedChannelRevision, BotIDs: append([]string{}, in.BotIDs...), ProjectID: in.ProjectID, ProjectRevision: in.ExpectedProjectRevision, Limits: in.CoreProfileLimits, CreatedAtUnixMS: now, ExpiresAtUnixMS: expires}
	if !s.validConsentScope(c) {
		return CoreProfileConsent{}, false, ErrConflict
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	service := s.coreProfiles
	s.mu.Unlock()
	err = service.Verify(ctx, profileConsentPin(c))
	s.mu.Lock()
	if err != nil || !s.inspectionActive(ic) || !s.validConsentScope(c) || s.profileNonceUsed(in.ClientNonce) || profileNow(s) >= c.ExpiresAtUnixMS {
		return CoreProfileConsent{}, false, ErrConflict
	}
	e := Event{Type: "core_profile.execution_approved", Nonce: in.ClientNonce, CoreProfile: &CoreProfileEvent{ConsentInput: &in, Consent: &c}}
	if !s.validCoreProfileEvent(e) {
		return c, false, ErrInvalid
	}
	if err := s.commit(e); err != nil {
		return c, false, err
	}
	return copyCoreConsent(c), false, nil
}
func (s *Store) ControlCoreProfile(in CoreProfileConsentControlInput) (CoreProfileConsent, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.coreProfilesEnabled() {
		return CoreProfileConsent{}, false, ErrForbidden
	}
	if old, ok := s.coreProfileNonces[in.ClientNonce]; ok {
		if old.Type != "core_profile.execution_revoked" || !reflect.DeepEqual(old.CoreProfile.ControlInput, &in) {
			return CoreProfileConsent{}, false, ErrConflict
		}
		return copyCoreConsent(*old.CoreProfile.Consent), true, nil
	}
	if in.Action != "revoke" || !validAccessNonce(in.ClientNonce) {
		return CoreProfileConsent{}, false, ErrInvalid
	}
	if s.profileNonceUsed(in.ClientNonce) || len(s.coreProfileNonces) >= 512 {
		return CoreProfileConsent{}, false, ErrConflict
	}
	c, ok := s.coreProfileConsents[in.ConsentID]
	if !ok || c.Revision != in.ExpectedRevision || c.Status != "approved" {
		return c, false, ErrConflict
	}
	c = copyCoreConsent(c)
	c.Revision++
	c.Status = "revoked"
	e := Event{Type: "core_profile.execution_revoked", Nonce: in.ClientNonce, CoreProfile: &CoreProfileEvent{ControlInput: &in, Consent: &c}}
	if !s.validCoreProfileEvent(e) {
		return c, false, ErrInvalid
	}
	if err := s.commit(e); err != nil {
		return c, false, err
	}
	return c, false, nil
}
func profileConsentPin(c CoreProfileConsent) CoreProfilePin {
	p := CoreProfilePin{Version: 1, FixtureOnly: c.Source.FixtureOnly, RuntimeSHA256: c.Source.RuntimeSHA256, SourceID: c.Source.ID, SourceRevision: c.Source.Revision, ProfileID: c.Profile.ID, ProfileRevision: c.Profile.ProfileRevision, Provider: c.Profile.Provider, Model: c.Profile.Model, ProviderOrigin: c.Profile.ProviderOrigin, ConsentID: c.ID, ConsentRevision: c.Revision, ActorScope: c.ActorScope, ChannelID: c.ChannelID, AudienceRevision: c.AudienceRevision, ProjectID: c.ProjectID, ProjectRevision: c.ProjectRevision, Limits: c.Limits, ExpiresAtUnixMS: c.ExpiresAtUnixMS}
	if credentialPolicyVersion(c.Profile) == 2 {
		p.Version = 2
		p.AuthKind = c.Profile.AuthKind
		p.CredentialSource = c.Profile.CredentialSource
		p.AuthPolicy = c.Profile.AuthPolicy
		p.CredentialSnapshotID = c.Profile.CredentialSnapshotID
		p.CredentialExpiresAtUnixMS = c.Profile.CredentialExpiresAtUnixMS
	}
	return p
}
func (s *Store) profileConsentActive(c CoreProfileConsent) bool {
	if !s.coreProfilesEnabled() || s.coreProfileRetired[c.ID] || c.Status != "approved" || profileNow(s) < c.CreatedAtUnixMS || profileNow(s) >= c.ExpiresAtUnixMS || !s.validConsentScope(c) {
		return false
	}
	source, ok := s.coreSource(c.Source.ID)
	return ok && source == c.Source
}
func (s *Store) CoreProfileReceipt(kind, nonce string) (CoreProfileReceipt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := CoreProfileReceipt{Version: 1, Kind: kind, ClientNonce: nonce}
	if s.identity != nil {
		return r, ErrForbidden
	}
	if !validAccessNonce(nonce) || (kind != "inspection" && kind != "execution" && kind != "revoke") {
		return r, ErrInvalid
	}
	e, ok := s.coreProfileNonces[nonce]
	if !ok {
		return r, nil
	}
	p := e.CoreProfile
	switch {
	case kind == "inspection" && p.InspectionInput != nil:
		c := *p.InspectionConsent
		r.Input = *p.InspectionInput
		r.InspectionConsent = &c
		r.Active = s.inspectionActive(c)
	case kind == "execution" && p.ConsentInput != nil:
		c := copyCoreConsent(*p.Consent)
		input := *p.ConsentInput
		input.BotIDs = append([]string{}, input.BotIDs...)
		r.Input = input
		r.Consent = &c
		current := s.coreProfileConsents[c.ID]
		r.Active = current.Revision == c.Revision && s.profileConsentActive(current)
		if r.Active {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			service := s.coreProfiles
			s.mu.Unlock()
			err := service.Verify(ctx, profileConsentPin(current))
			cancel()
			s.mu.Lock()
			latest := s.coreProfileConsents[c.ID]
			r.Active = err == nil && reflect.DeepEqual(latest, current) && s.profileConsentActive(latest) && !s.closed && !s.poisoned
		}
	case kind == "revoke" && p.ControlInput != nil:
		c := copyCoreConsent(*p.Consent)
		r.Input = *p.ControlInput
		r.Consent = &c
	default:
		return r, ErrConflict
	}
	r.Found = true
	return r, nil
}
func (s *Store) validCoreProfileEvent(e Event) bool {
	p := e.CoreProfile
	if p == nil || !reflect.DeepEqual(e, Event{Sequence: e.Sequence, Type: e.Type, Nonce: e.Nonce, CoreProfile: p}) {
		return false
	}
	if e.Nonce != "" && (!validAccessNonce(e.Nonce) || s.profileNonceUsed(e.Nonce) || len(s.coreProfileNonces) >= 512) {
		return false
	}
	switch e.Type {
	case "core_profile.inspection_approved":
		if p.InspectionInput == nil || p.InspectionConsent == nil || !reflect.DeepEqual(*p, CoreProfileEvent{InspectionInput: p.InspectionInput, InspectionConsent: p.InspectionConsent}) {
			return false
		}
		in, c := *p.InspectionInput, *p.InspectionConsent
		_, exists := s.coreProfileInspections[c.ID]
		return !exists && len(s.coreProfileInspections) < 128 && resourceID(c.ID, "core-inspection-") && c.Version == 1 && validCoreSource(c.Source) && c.ActorScope == localProfileActor() && c.CreatedAtUnixMS > 0 && c.ExpiresAtUnixMS > c.CreatedAtUnixMS && c.ExpiresAtUnixMS < 1<<53 && c.ExpiresAtUnixMS-c.CreatedAtUnixMS == int64(in.TTLSeconds)*1000 && in.TTLSeconds >= 30 && in.TTLSeconds <= 3600 && in.ApproveInspection && in.SourceID == c.Source.ID && in.ExpectedSourceRevision == c.Source.Revision && in.ClientNonce == e.Nonce
	case "core_profile.metadata_captured":
		if e.Nonce != "" || p.Metadata == nil || !reflect.DeepEqual(*p, CoreProfileEvent{Metadata: p.Metadata}) {
			return false
		}
		r := p.Metadata
		c, ok := s.coreProfileInspections[r.InspectionConsentID]
		_, exists := s.coreProfileMetadata[r.InspectionConsentID]
		if !ok || exists || r.SourceID != c.Source.ID || r.CapturedAtUnixMS < c.CreatedAtUnixMS || r.CapturedAtUnixMS >= c.ExpiresAtUnixMS || !validProfileReport(*r, c.Source) {
			return false
		}
		seen := map[string]bool{}
		for _, profile := range r.Profiles {
			if !validCoreProfile(profile, c.Source) || seen[profile.ID] {
				return false
			}
			seen[profile.ID] = true
		}
		return true
	case "core_profile.execution_approved":
		if p.ConsentInput == nil || p.Consent == nil || !reflect.DeepEqual(*p, CoreProfileEvent{ConsentInput: p.ConsentInput, Consent: p.Consent}) {
			return false
		}
		in, c := *p.ConsentInput, *p.Consent
		ic, profile, ok := s.profileMatching(in)
		_, exists := s.coreProfileConsents[c.ID]
		expires, expiryOK := coreProfileConsentExpiry(c.CreatedAtUnixMS, in.TTLSeconds, profile)
		return ok && !exists && len(s.coreProfileConsents) < 128 && resourceID(c.ID, "core-consent-") && c.Version == 1 && c.Revision == 1 && c.Status == "approved" && c.Source == ic.Source && c.Profile == profile && c.InspectionConsentID == ic.ID && c.ActorScope == localProfileActor() && executableCoreProfile(profile) && validProfileOrigin(profile.ProviderOrigin, c.Source.FixtureOnly) && c.ChannelID == in.ChannelID && c.AudienceRevision == in.ExpectedChannelRevision && reflect.DeepEqual(c.BotIDs, in.BotIDs) && c.ProjectID == in.ProjectID && c.ProjectRevision == in.ExpectedProjectRevision && c.Limits == in.CoreProfileLimits && validProfileLimits(c.Limits) && s.validConsentScope(c) && c.CreatedAtUnixMS >= ic.CreatedAtUnixMS && c.CreatedAtUnixMS < ic.ExpiresAtUnixMS && c.ExpiresAtUnixMS > c.CreatedAtUnixMS && c.ExpiresAtUnixMS < 1<<53 && expiryOK && c.ExpiresAtUnixMS == expires && in.TTLSeconds >= 30 && in.TTLSeconds <= 3600 && in.ApproveExecution && e.Nonce == in.ClientNonce
	case "core_profile.execution_revoked":
		if p.ControlInput == nil || p.Consent == nil || !reflect.DeepEqual(*p, CoreProfileEvent{ControlInput: p.ControlInput, Consent: p.Consent}) {
			return false
		}
		in, c := *p.ControlInput, *p.Consent
		old, ok := s.coreProfileConsents[in.ConsentID]
		if !ok || old.Status != "approved" || old.Revision != in.ExpectedRevision || in.Action != "revoke" || e.Nonce != in.ClientNonce {
			return false
		}
		old.Revision++
		old.Status = "revoked"
		return reflect.DeepEqual(old, c)
	case "core_profile.expired":
		if e.Nonce != "" || p.InspectionConsent == nil || !reflect.DeepEqual(*p, CoreProfileEvent{InspectionConsent: p.InspectionConsent, ObservedAtUnixMS: p.ObservedAtUnixMS}) {
			return false
		}
		c := p.InspectionConsent
		expires := s.coreProfileInspections[c.ID].ExpiresAtUnixMS
		if grant, ok := s.coreProfileConsents[c.ID]; ok {
			expires = grant.ExpiresAtUnixMS
		}
		return c.ID != "" && !s.coreProfileRetired[c.ID] && expires > 0 && p.ObservedAtUnixMS >= expires && p.ObservedAtUnixMS < 1<<53 && *c == (CoreProfileInspectionConsent{Version: 1, ID: c.ID})
	}
	return false
}
func (s *Store) applyCoreProfile(e Event) {
	if s.coreProfileInspections == nil {
		s.coreProfileInspections = map[string]CoreProfileInspectionConsent{}
		s.coreProfileConsents = map[string]CoreProfileConsent{}
		s.coreProfileMetadata = map[string]CoreProfilesResult{}
		s.coreProfileNonces = map[string]Event{}
		s.coreProfileRetired = map[string]bool{}
	}
	p := *e.CoreProfile
	if p.InspectionConsent != nil {
		c := *p.InspectionConsent
		p.InspectionConsent = &c
		if e.Type == "core_profile.expired" {
			s.coreProfileRetired[c.ID] = true
		} else {
			s.coreProfileInspections[c.ID] = c
		}
	}
	if p.Metadata != nil {
		r := copyProfileMetadata(*p.Metadata)
		p.Metadata = &r
		s.coreProfileMetadata[r.InspectionConsentID] = r
	}
	if p.Consent != nil {
		c := copyCoreConsent(*p.Consent)
		p.Consent = &c
		s.coreProfileConsents[c.ID] = c
	}
	if p.ConsentInput != nil {
		in := *p.ConsentInput
		in.BotIDs = append([]string{}, in.BotIDs...)
		p.ConsentInput = &in
	}
	e.CoreProfile = &p
	if e.Nonce != "" {
		s.coreProfileNonces[e.Nonce] = e
	}
	s.events = append(s.events, e)
}
func (s *Store) pinCoreProfile(w *Work, selection *CoreProfileSelection) error {
	if selection == nil {
		return nil
	}
	if !s.coreProfilesEnabled() || w.RequesterPin != nil {
		return ErrForbidden
	}
	c, ok := s.coreProfileConsents[selection.ConsentID]
	if !ok || c.Revision != selection.ExpectedConsentRevision || !s.profileConsentActive(c) || w.Account != nil || w.Adapter != nil || w.Collaboration != nil || w.TaskLink != nil || w.UseBotDefaults || w.ConversationID != c.ChannelID || !profileContains(c.BotIDs, w.BotID) || (c.ProjectID == "" && w.Resources != nil) || (c.ProjectID != "" && (w.Resources == nil || w.Resources.Project.ID != c.ProjectID || w.Resources.Project.Revision != c.ProjectRevision)) {
		return ErrConflict
	}
	p := profileConsentPin(c)
	p.BotID = w.BotID
	p.ClientNonce = w.ClientNonce
	p.ContextSHA256 = contextHash(*w)
	p.ResourcesSHA256 = profileHash(w.Resources)
	created, err := time.Parse(time.RFC3339Nano, w.CreatedAt)
	if err != nil {
		return ErrInvalid
	}
	deadline := created.UnixMilli() + int64(p.Limits.MaxSeconds)*1000
	if deadline < p.ExpiresAtUnixMS {
		p.ExpiresAtUnixMS = deadline
	}
	w.CoreProfilePin = &p
	if !s.validCoreProfileIntake(*w) {
		return ErrInvalid
	}
	if !s.coreProfileCurrent(*w) {
		return ErrForbidden
	}
	return nil
}
func (s *Store) verifyProfileSelection(scope SubmissionScope) error {
	if !s.coreProfilesEnabled() {
		return ErrForbidden
	}
	selection := scope.CoreProfileSelection
	c, ok := s.coreProfileConsents[selection.ConsentID]
	if !ok || c.Revision != selection.ExpectedConsentRevision || !s.profileConsentActive(c) {
		return ErrConflict
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	service := s.coreProfiles
	s.mu.Unlock()
	err := service.Verify(ctx, profileConsentPin(c))
	s.mu.Lock()
	current, ok := s.coreProfileConsents[c.ID]
	if err != nil || !ok || !reflect.DeepEqual(current, c) || !s.profileConsentActive(current) || s.closed || s.poisoned {
		return ErrConflict
	}
	return nil
}
func (s *Store) validCoreProfileIntake(w Work) bool {
	p := w.CoreProfilePin
	if p == nil {
		return w.CoreProfileSelection == nil
	}
	c, ok := s.coreProfileConsents[p.ConsentID]
	if !ok || c.Status != "approved" || c.Revision != p.ConsentRevision || w.RequesterPin != nil || w.OwnerUserID != "local-owner" || w.SpaceID != "local-space" || w.Account != nil || w.Adapter != nil || w.Collaboration != nil || w.TaskLink != nil || w.UseBotDefaults || w.ConversationID != c.ChannelID || !profileContains(c.BotIDs, w.BotID) || w.Publication == nil || w.Publication.AudienceRevision != c.AudienceRevision || w.CoreProfileSelection == nil || w.CoreProfileSelection.ConsentID != c.ID || w.CoreProfileSelection.ExpectedConsentRevision != c.Revision {
		return false
	}
	expected := profileConsentPin(c)
	expected.BotID = w.BotID
	expected.ClientNonce = w.ClientNonce
	expected.ContextSHA256 = contextHash(w)
	expected.ResourcesSHA256 = profileHash(w.Resources)
	created, err := time.Parse(time.RFC3339Nano, w.CreatedAt)
	if err != nil || created.UnixMilli() < c.CreatedAtUnixMS || created.UnixMilli() >= c.ExpiresAtUnixMS {
		return false
	}
	deadline := created.UnixMilli() + int64(c.Limits.MaxSeconds)*1000
	if deadline < expected.ExpiresAtUnixMS {
		expected.ExpiresAtUnixMS = deadline
	}
	return *p == expected && !s.coreProfileRetired[c.ID] && (c.ProjectID == "" && w.Resources == nil || c.ProjectID != "" && w.Resources != nil && w.Resources.Project.ID == c.ProjectID && w.Resources.Project.Revision == c.ProjectRevision) && profileInputBound(w)
}
func profileInputBound(w Work) bool {
	if w.CoreProfilePin == nil {
		return true
	}
	n := len(w.RuntimePrompt())
	if w.RuntimeContext != nil {
		raw, _ := json.Marshal(w.RuntimeContext)
		n = len(raw)
	}
	return n <= w.CoreProfilePin.Limits.MaxInputBytes
}
func (s *Store) coreProfileCurrent(w Work) bool {
	p := w.CoreProfilePin
	if p == nil {
		return true
	}
	c, ok := s.coreProfileConsents[p.ConsentID]
	base := profileConsentPin(c)
	authEqual := p.Version == base.Version && p.AuthKind == base.AuthKind && p.CredentialSource == base.CredentialSource && p.AuthPolicy == base.AuthPolicy && p.CredentialSnapshotID == base.CredentialSnapshotID && p.CredentialExpiresAtUnixMS == base.CredentialExpiresAtUnixMS
	return ok && authEqual && s.profileConsentActive(c) && c.Revision == p.ConsentRevision && profileNow(s) < p.ExpiresAtUnixMS && w.BotID == p.BotID && w.ConversationID == p.ChannelID && s.publicationAllowed(w) && contextHash(w) == p.ContextSHA256 && profileHash(w.Resources) == p.ResourcesSHA256 && profileInputBound(w)
}
func (s *Store) coreProfileGuard(w Work) error {
	if w.CoreProfilePin == nil {
		return nil
	}
	if !s.coreProfileCurrent(w) {
		return ErrForbidden
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	service := s.coreProfiles
	old, exists := s.work[w.RequestID]
	s.mu.Unlock()
	err := service.Verify(ctx, *w.CoreProfilePin)
	s.mu.Lock()
	if exists {
		current, ok := s.work[w.RequestID]
		if !ok || !reflect.DeepEqual(current, old) {
			return ErrConflict
		}
	}
	if err != nil || !s.coreProfileCurrent(w) || s.closed || s.poisoned {
		return ErrForbidden
	}
	return nil
}
func (s *Store) VerifyWorkCoreProfile(id string, epoch uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.work[id]
	if !ok {
		return ErrNotFound
	}
	if w.Epoch != epoch || w.DesiredState != "running" {
		return ErrConflict
	}
	return s.coreProfileGuard(w)
}
func (s *Store) ReconcileCoreProfiles() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := profileNow(s)
	for _, c := range s.coreProfileInspections {
		if now >= c.ExpiresAtUnixMS && !s.coreProfileRetired[c.ID] {
			marker := CoreProfileInspectionConsent{Version: 1, ID: c.ID}
			if err := s.commit(Event{Type: "core_profile.expired", CoreProfile: &CoreProfileEvent{InspectionConsent: &marker, ObservedAtUnixMS: now}}); err != nil {
				return err
			}
		}
	}
	for _, c := range s.coreProfileConsents {
		if now >= c.ExpiresAtUnixMS && !s.coreProfileRetired[c.ID] {
			marker := CoreProfileInspectionConsent{Version: 1, ID: c.ID}
			if err := s.commit(Event{Type: "core_profile.expired", CoreProfile: &CoreProfileEvent{InspectionConsent: &marker, ObservedAtUnixMS: now}}); err != nil {
				return err
			}
		}
	}
	for _, old := range s.sortedWorkLocked() {
		current, exists := s.work[old.RequestID]
		if !exists || !reflect.DeepEqual(current, old) || old.CoreProfilePin == nil || !suspendible(old) {
			continue
		}
		err := s.coreProfileGuard(old)
		if err == nil || err == ErrConflict {
			continue
		}
		w := copyWork(old)
		w.Status = "suspending"
		if old.RuntimeStopped || old.Status == "waiting_host" && old.AttemptID == "" {
			w.Status = "suspended"
		}
		w.DesiredState = "suspended"
		w.Epoch++
		w.Failure = "core_profile_authority_changed"
		if err := s.commit(Event{Type: "work.core_profile_suspended", Work: w}); err != nil {
			return err
		}
	}
	return nil
}
