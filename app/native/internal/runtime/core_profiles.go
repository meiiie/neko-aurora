package runtime

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"time"

	"nekobot.local/native/internal/ledger"
	"nekobot.local/native/internal/protocol"
)

var errCoreProfile = errors.New("core_profile_unavailable")
var profileIDGrammar = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)
var profileRevisionGrammar = regexp.MustCompile(`^pr[12]_[a-f0-9]{64}$`)

type CoreProfileSourceConfig struct {
	ID                string `json:"id"`
	Label             string `json:"label"`
	Kind              string `json:"kind"`
	CredentialHome    string `json:"credentialHome,omitempty"`
	ProviderOrigin    string `json:"providerOrigin,omitempty"`
	EnvironmentBroker bool   `json:"environmentBroker,omitempty"`
}

type CoreProfileConfig struct {
	Version        int                       `json:"version"`
	HostExecutable string                    `json:"hostExecutable"`
	CoreExecutable string                    `json:"coreExecutable"`
	Supervisor     string                    `json:"supervisor"`
	Sources        []CoreProfileSourceConfig `json:"sources"`
}

type coreProfileSource struct {
	public            ledger.CoreProfileSource
	home              string
	key               string
	origin            string
	environmentBroker bool
	capsule           string
	snapshotID        string
	snapshotExpiry    int64
	mu                sync.Mutex
	verifyMu          sync.Mutex
	verifyGeneration  uint64
	verification      *profileCatalogVerification
}

type profileCatalogVerification struct {
	done       chan struct{}
	generation uint64
	report     ledger.CoreProfileMetadataReport
	err        error
}

func copyProfileCatalog(report ledger.CoreProfileMetadataReport) ledger.CoreProfileMetadataReport {
	if report.SourceSnapshot != nil {
		value := *report.SourceSnapshot
		report.SourceSnapshot = &value
	}
	if report.Profiles != nil {
		report.Profiles = append([]ledger.CoreProfileMetadata{}, report.Profiles...)
	}
	return report
}

func (source *coreProfileSource) verificationCatalog(ctx context.Context, collect func(context.Context) (ledger.CoreProfileMetadataReport, error)) (ledger.CoreProfileMetadataReport, error) {
	if ctx.Err() != nil {
		return ledger.CoreProfileMetadataReport{}, errCoreProfile
	}
	source.verifyMu.Lock()
	flight := source.verification
	if flight != nil && flight.generation != source.verifyGeneration {
		source.verifyMu.Unlock()
		select {
		case <-ctx.Done():
			return ledger.CoreProfileMetadataReport{}, errCoreProfile
		case <-flight.done:
			return source.verificationCatalog(ctx, collect)
		}
	}
	if flight == nil {
		flight = &profileCatalogVerification{done: make(chan struct{}), generation: source.verifyGeneration}
		source.verification = flight
		go func() {
			readCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			report, err := collect(readCtx)
			source.verifyMu.Lock()
			flight.report, flight.err = copyProfileCatalog(report), err
			if source.verification == flight {
				source.verification = nil
			}
			close(flight.done)
			source.verifyMu.Unlock()
		}()
	}
	source.verifyMu.Unlock()
	select {
	case <-ctx.Done():
		return ledger.CoreProfileMetadataReport{}, errCoreProfile
	case <-flight.done:
	}
	source.verifyMu.Lock()
	defer source.verifyMu.Unlock()
	if ctx.Err() != nil || flight.err != nil || flight.generation != source.verifyGeneration || source.environmentBroker && (flight.report.SourceSnapshot == nil || flight.report.SourceSnapshot.ID != source.snapshotID || flight.report.SourceSnapshot.ExpiresAtUnixMS != source.snapshotExpiry) {
		return ledger.CoreProfileMetadataReport{}, errCoreProfile
	}
	return copyProfileCatalog(flight.report), nil
}

type CoreProfiles struct {
	core    string
	home    string
	digest  string
	sources map[string]*coreProfileSource
	ordered []ledger.CoreProfileSource
	authV2  bool
}

func validProfileText(value string, bound int) bool {
	if value == "" || len(value) > bound {
		return false
	}
	for _, c := range value {
		if c < 32 || c == 127 || c == '\ufffd' {
			return false
		}
	}
	return true
}

func validProfileOrigin(origin string, fixture bool) bool {
	if !validProfileText(origin, 512) {
		return false
	}
	u, e := url.Parse(origin)
	if e != nil || u.Host == "" || u.User != nil || u.Path != "" || u.RawPath != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" {
		return false
	}
	if fixture {
		port, err := strconv.Atoi(u.Port())
		return err == nil && port > 0 && port <= 65535 && u.Scheme == "http" && u.Hostname() == "127.0.0.1" && u.String() == origin
	}
	return u.Scheme == "https" && u.Hostname() != "" && u.String() == origin
}

func validProfileMetadataOrigin(origin string) bool {
	if validProfileOrigin(origin, false) {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || !validProfileText(origin, 512) || u.User != nil || u.Scheme != "http" || u.Path != "" || u.RawPath != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || u.String() != origin {
		return false
	}
	port := 80
	if u.Port() != "" {
		port, err = strconv.Atoi(u.Port())
	}
	return err == nil && port > 0 && port <= 65535 && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1")
}

func parseProfileCatalog(raw []byte, fixture bool, origin string) (ledger.CoreProfileMetadataReport, error) {
	var report ledger.CoreProfileMetadataReport
	if len(raw) > 65536 {
		return report, errCoreProfile
	}
	canonical, e := protocol.Canonical(bytes.TrimSpace(raw))
	if e != nil {
		return report, errCoreProfile
	}
	d := json.NewDecoder(bytes.NewReader(canonical))
	d.DisallowUnknownFields()
	if d.Decode(&report) != nil || (report.Version != 1 && report.Version != 2) || report.Profiles == nil || len(report.Profiles) > 64 {
		return ledger.CoreProfileMetadataReport{}, errCoreProfile
	}
	rendered, _ := json.Marshal(report)
	exact, e := protocol.Canonical(rendered)
	if e != nil || !bytes.Equal(exact, canonical) {
		return ledger.CoreProfileMetadataReport{}, errCoreProfile
	}
	seen := map[string]bool{}
	if report.SourceSnapshot != nil {
		snapshot := report.SourceSnapshot
		if report.Version != 2 || !regexp.MustCompile(`^cs1_[a-f0-9]{32}$`).MatchString(snapshot.ID) || snapshot.CreatedAtUnixMS <= 0 || snapshot.ExpiresAtUnixMS <= snapshot.CreatedAtUnixMS || snapshot.ExpiresAtUnixMS >= 1<<53 || snapshot.ExpiresAtUnixMS-snapshot.CreatedAtUnixMS > 300000 {
			return ledger.CoreProfileMetadataReport{}, errCoreProfile
		}
	}
	for i, p := range report.Profiles {
		modelValid := validProfileText(p.Model, 128) || p.Model == "" && p.ConfigurationStatus == "incomplete" && p.BridgeTransport == "unsupported"
		providerValid := validProfileText(p.Provider, 64) || p.Provider == "" && p.ConfigurationStatus == "incomplete" && p.BridgeTransport == "unsupported"
		if !profileIDGrammar.MatchString(p.ID) || seen[p.ID] || !providerValid || !modelValid || !validProfileText(p.AuthKind, 64) || !profileRevisionGrammar.MatchString(p.ProfileRevision) || p.CredentialReadiness != "not_checked" || (p.ConfigurationStatus != "configured" && p.ConfigurationStatus != "incomplete") || (p.BridgeTransport != "supported" && p.BridgeTransport != "unsupported") {
			return ledger.CoreProfileMetadataReport{}, errCoreProfile
		}
		seen[p.ID] = true
		legacy := p.CredentialSource == "" && p.AuthPolicy == "" && p.CredentialSnapshotID == "" && p.CredentialExpiresAtUnixMS == 0
		if (report.Version == 1) != legacy || !ledger.CoreProfileCredentialPolicyValid(p) {
			return ledger.CoreProfileMetadataReport{}, errCoreProfile
		}
		if p.CredentialSnapshotID != "" && (report.SourceSnapshot == nil || p.CredentialSnapshotID != report.SourceSnapshot.ID || p.CredentialExpiresAtUnixMS > report.SourceSnapshot.ExpiresAtUnixMS) {
			return ledger.CoreProfileMetadataReport{}, errCoreProfile
		}
		if p.ProviderOrigin != "" && !validProfileMetadataOrigin(p.ProviderOrigin) {
			return ledger.CoreProfileMetadataReport{}, errCoreProfile
		}
		if p.BridgeTransport == "supported" && fixture && p.ProviderOrigin != origin {
			report.Profiles[i].BridgeTransport = "unsupported"
		}
		if p.BridgeTransport == "supported" && (p.ConfigurationStatus != "configured" || p.ProviderOrigin == "" || (p.AuthKind != "api_key" && p.AuthKind != "none" && !(report.Version == 2 && p.AuthKind == "kimi_oauth")) || (p.Provider != "openai_compat" && p.Provider != "responses" && p.Provider != "anthropic" && p.Provider != "kimi")) {
			return ledger.CoreProfileMetadataReport{}, errCoreProfile
		}
		if p.BridgeTransport == "supported" && !fixture && !validProfileOrigin(p.ProviderOrigin, false) {
			report.Profiles[i].BridgeTransport = "unsupported"
		}
	}
	return report, nil
}

func LoadCoreProfiles(path, state string) (Config, *CoreProfiles, error) {
	var cfg CoreProfileConfig
	raw, e := os.ReadFile(path)
	if e != nil || len(raw) > 16384 {
		return Config{}, nil, errCoreProfile
	}
	canonical, e := protocol.Canonical(raw)
	if e != nil {
		return Config{}, nil, errCoreProfile
	}
	d := json.NewDecoder(bytes.NewReader(canonical))
	d.DisallowUnknownFields()
	if d.Decode(&cfg) != nil || cfg.Version != 1 || len(cfg.Sources) == 0 || len(cfg.Sources) > 8 {
		return Config{}, nil, errCoreProfile
	}
	for _, executable := range []string{cfg.HostExecutable, cfg.CoreExecutable, cfg.Supervisor} {
		info, err := os.Lstat(executable)
		if !filepath.IsAbs(executable) || err != nil || !info.Mode().IsRegular() {
			return Config{}, nil, errCoreProfile
		}
	}
	core, e := os.ReadFile(cfg.CoreExecutable)
	if e != nil || !protocol.CoreProfileBridgeAllowed(hashBytes(core)) {
		return Config{}, nil, errCoreProfile
	}
	state, e = filepath.Abs(state)
	if e != nil {
		return Config{}, nil, errCoreProfile
	}
	private := filepath.Join(state, "core-profile-bridge")
	if e = os.MkdirAll(private, 0700); e != nil {
		return Config{}, nil, errCoreProfile
	}
	service := &CoreProfiles{core: cfg.CoreExecutable, digest: hashBytes(core), home: filepath.Join(private, "inspection-home"), sources: map[string]*coreProfileSource{}, authV2: protocol.CoreProfileAuthBridgeAllowed(hashBytes(core))}
	if e = os.MkdirAll(service.home, 0700); e != nil {
		return Config{}, nil, errCoreProfile
	}
	for _, source := range cfg.Sources {
		if !profileIDGrammar.MatchString(source.ID) || !validProfileText(source.Label, 120) {
			return Config{}, nil, errCoreProfile
		}
		if _, exists := service.sources[source.ID]; exists {
			return Config{}, nil, errCoreProfile
		}
		if source.EnvironmentBroker && !service.authV2 {
			return Config{}, nil, errCoreProfile
		}
		home, fixture := source.CredentialHome, source.Kind == "fixture"
		if fixture {
			if !filepath.IsAbs(home) || !validProfileOrigin(source.ProviderOrigin, true) {
				return Config{}, nil, errCoreProfile
			}
		} else {
			if source.Kind != "owner_home" || home != "" || source.ProviderOrigin != "" {
				return Config{}, nil, errCoreProfile
			}
			home, e = os.UserHomeDir()
			if e != nil || !filepath.IsAbs(home) {
				return Config{}, nil, errCoreProfile
			}
		}
		home = filepath.Clean(home)
		keyDir := filepath.Join(private, source.ID)
		if e = os.MkdirAll(keyDir, 0700); e != nil {
			return Config{}, nil, errCoreProfile
		}
		key, err := profileRevisionKey(keyDir, source.ID+"\x00"+home)
		if err != nil {
			return Config{}, nil, errCoreProfile
		}
		mac := hmac.New(sha256.New, []byte(key))
		binding, _ := json.Marshal([]any{source.ID, home, service.digest, source.Kind, source.ProviderOrigin})
		if source.EnvironmentBroker {
			var generation [32]byte
			if _, e := rand.Read(generation[:]); e != nil {
				return Config{}, nil, errCoreProfile
			}
			binding, _ = json.Marshal([]any{source.ID, home, service.digest, source.Kind, source.ProviderOrigin, true, hex.EncodeToString(generation[:])})
		}
		mac.Write(binding)
		revision := 1 + binary.BigEndian.Uint64(mac.Sum(nil)[:8])%9_007_199_254_740_991
		public := ledger.CoreProfileSource{Version: 1, ID: source.ID, Revision: revision, Label: source.Label, RuntimeSHA256: service.digest, FixtureOnly: fixture}
		service.sources[source.ID] = &coreProfileSource{public: public, home: home, key: key, origin: source.ProviderOrigin, environmentBroker: source.EnvironmentBroker, capsule: filepath.Join(keyDir, "environment.capsule")}
		service.ordered = append(service.ordered, public)
	}
	c := Config{HostExecutable: cfg.HostExecutable, CoreExecutable: cfg.CoreExecutable, Supervisor: cfg.Supervisor, CoreProfiles: service}
	return c, service, nil
}

func (s *CoreProfiles) Sources() []ledger.CoreProfileSource {
	return append([]ledger.CoreProfileSource(nil), s.ordered...)
}

type boundedProfileOutput struct {
	bytes.Buffer
	overflow bool
}

func (b *boundedProfileOutput) Write(p []byte) (int, error) {
	n := len(p)
	if b.Len()+n > 65536 {
		b.overflow = true
		return n, nil
	}
	return b.Buffer.Write(p)
}

func (s *CoreProfiles) Inspect(ctx context.Context, id string) (ledger.CoreProfileMetadataReport, error) {
	source, ok := s.sources[id]
	if !ok {
		return ledger.CoreProfileMetadataReport{}, errCoreProfile
	}
	source.mu.Lock()
	defer source.mu.Unlock()
	if ctx.Err() != nil {
		return ledger.CoreProfileMetadataReport{}, errCoreProfile
	}
	if source.environmentBroker {
		source.verifyMu.Lock()
		source.verifyGeneration++
		source.verifyMu.Unlock()
	}
	report, err := s.catalog(ctx, source, source.environmentBroker)
	if err != nil {
		return ledger.CoreProfileMetadataReport{}, err
	}
	if source.environmentBroker {
		if report.SourceSnapshot == nil {
			return ledger.CoreProfileMetadataReport{}, errCoreProfile
		}
		source.verifyMu.Lock()
		source.snapshotID = report.SourceSnapshot.ID
		source.snapshotExpiry = report.SourceSnapshot.ExpiresAtUnixMS
		source.verifyMu.Unlock()
	}
	return report, nil
}

func (s *CoreProfiles) catalog(ctx context.Context, source *coreProfileSource, captureEnvironment bool) (ledger.CoreProfileMetadataReport, error) {
	binaryBytes, e := os.ReadFile(s.core)
	if e != nil || hashBytes(binaryBytes) != s.digest || !protocol.CoreProfileBridgeAllowed(s.digest) {
		return ledger.CoreProfileMetadataReport{}, errCoreProfile
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	args := []string{"profiles", "--json", "--credential-home", source.home}
	if s.authV2 {
		args = append(args, "--profile-v2")
	}
	if captureEnvironment {
		if !s.authV2 || !source.environmentBroker {
			return ledger.CoreProfileMetadataReport{}, errCoreProfile
		}
		args = []string{"profiles", "--json", "--broker-stdin"}
	}
	cmd := exec.CommandContext(ctx, s.core, args...)
	hideWindow(cmd)
	if captureEnvironment {
		request, err := json.Marshal(struct {
			Version         int    `json:"version"`
			SourceID        string `json:"sourceId"`
			CredentialHome  string `json:"credentialHome"`
			StateHome       string `json:"stateHome"`
			CapsulePath     string `json:"capsulePath"`
			RevisionKey     string `json:"revisionKey"`
			ExpiresAtUnixMS int64  `json:"expiresAtUnixMs"`
		}{1, source.public.ID, source.home, s.home, source.capsule, source.key, time.Now().UnixMilli() + 300000})
		if err != nil || len(request) > 16384 {
			return ledger.CoreProfileMetadataReport{}, errCoreProfile
		}
		cmd.Stdin = bytes.NewReader(request)
		// The pre-agent Core broker inherits its source environment; this process does not inspect credential values.
	} else {
		cmd.Env = append(hostEnvironment(), "USERPROFILE="+s.home, "HOME="+s.home, "NEKO_CREDENTIAL_HOME="+source.home, "NEKO_PROFILE_REVISION_KEY="+source.key, "NEKO_AUTO_UPDATE=0", "NEKO_AUTO_UPDATE_CHECK=false")
		if source.environmentBroker {
			if source.snapshotID == "" || source.snapshotExpiry <= time.Now().UnixMilli() {
				return ledger.CoreProfileMetadataReport{}, errCoreProfile
			}
			cmd.Env = append(cmd.Env, "NEKO_PROFILE_CAPSULE_PATH="+source.capsule, "NEKO_PROFILE_SNAPSHOT_ID="+source.snapshotID)
		}
	}
	cmd.Dir = s.home
	cmd.Stderr = io.Discard
	var output boundedProfileOutput
	cmd.Stdout = &output
	if cmd.Run() != nil || output.overflow {
		return ledger.CoreProfileMetadataReport{}, errCoreProfile
	}
	report, err := parseProfileCatalog(output.Bytes(), source.public.FixtureOnly, source.origin)
	if err == nil && !captureEnvironment && source.environmentBroker && (report.SourceSnapshot == nil || report.SourceSnapshot.ID != source.snapshotID || report.SourceSnapshot.ExpiresAtUnixMS != source.snapshotExpiry) {
		return ledger.CoreProfileMetadataReport{}, errCoreProfile
	}
	return report, err
}

func (s *CoreProfiles) Verify(ctx context.Context, pin ledger.CoreProfilePin) error {
	source, ok := s.sources[pin.SourceID]
	if !ok || source.public.Revision != pin.SourceRevision || source.public.RuntimeSHA256 != pin.RuntimeSHA256 || source.public.FixtureOnly != pin.FixtureOnly {
		return errCoreProfile
	}
	metadata, e := source.verificationCatalog(ctx, func(readCtx context.Context) (ledger.CoreProfileMetadataReport, error) {
		source.mu.Lock()
		defer source.mu.Unlock()
		if readCtx.Err() != nil {
			return ledger.CoreProfileMetadataReport{}, errCoreProfile
		}
		return s.catalog(readCtx, source, false)
	})
	if e != nil {
		return errCoreProfile
	}
	for _, p := range metadata.Profiles {
		policyMatches := pin.Version == 1 && p.CredentialSource == "" && p.AuthPolicy == "" || pin.Version == 2 && p.AuthKind == pin.AuthKind && p.CredentialSource == pin.CredentialSource && p.AuthPolicy == pin.AuthPolicy && p.CredentialSnapshotID == pin.CredentialSnapshotID && p.CredentialExpiresAtUnixMS == pin.CredentialExpiresAtUnixMS
		if p.ID == pin.ProfileID && p.ProfileRevision == pin.ProfileRevision && p.Provider == pin.Provider && p.Model == pin.Model && p.ProviderOrigin == pin.ProviderOrigin && p.ConfigurationStatus == "configured" && p.BridgeTransport == "supported" && policyMatches && (p.CredentialExpiresAtUnixMS == 0 || p.CredentialExpiresAtUnixMS > time.Now().UnixMilli()) {
			return nil
		}
	}
	return errCoreProfile
}

func (s *CoreProfiles) launchEnvironment(pin ledger.CoreProfilePin) ([]string, error) {
	source, ok := s.sources[pin.SourceID]
	if !ok || source.public.Revision != pin.SourceRevision || source.public.RuntimeSHA256 != pin.RuntimeSHA256 || source.public.FixtureOnly != pin.FixtureOnly {
		return nil, errCoreProfile
	}
	source.mu.Lock()
	defer source.mu.Unlock()
	if pin.CredentialSnapshotID != "" && (!source.environmentBroker || source.snapshotID != pin.CredentialSnapshotID || source.snapshotExpiry <= time.Now().UnixMilli()) {
		return nil, errCoreProfile
	}
	limits, e := json.Marshal(pin.Limits)
	if e != nil {
		return nil, errCoreProfile
	}
	fixture := "0"
	if source.public.FixtureOnly {
		fixture = "1"
	}
	environment := []string{"NEKO_CREDENTIAL_HOME=" + source.home, "NEKO_PROFILE_REVISION_KEY=" + source.key, "NEKO_PROFILE_REVISION=" + pin.ProfileRevision, "NEKO_PROFILE=" + pin.ProfileID, "NEKO_PROFILE_LIMITS=" + string(limits), "NEKO_BRIDGE_SOURCE_ID=" + pin.SourceID, "NEKO_BRIDGE_SOURCE_REVISION=" + fmtProfileRevision(pin.SourceRevision), "NEKO_BRIDGE_FIXTURE_ONLY=" + fixture}
	if pin.CredentialSnapshotID != "" {
		environment = append(environment, "NEKO_PROFILE_CAPSULE_PATH="+source.capsule, "NEKO_PROFILE_SNAPSHOT_ID="+pin.CredentialSnapshotID)
	}
	return environment, nil
}

func fmtProfileRevision(v uint64) string {
	bytes, _ := json.Marshal(v)
	return string(bytes)
}

var _ ledger.CoreProfileService = (*CoreProfiles)(nil)
