package ledger

import "context"

type CoreProfileSource struct {
	Version       int    `json:"version"`
	ID            string `json:"id"`
	Revision      uint64 `json:"revision"`
	Label         string `json:"label"`
	RuntimeSHA256 string `json:"runtimeSha256"`
	FixtureOnly   bool   `json:"fixtureOnly"`
}
type CoreProfileMetadata struct {
	CredentialSource          string `json:"credentialSource,omitempty"`
	AuthPolicy                string `json:"authPolicy,omitempty"`
	CredentialSnapshotID      string `json:"credentialSnapshotId,omitempty"`
	CredentialExpiresAtUnixMS int64  `json:"credentialExpiresAtUnixMs,omitempty"`
	ID                        string `json:"id"`
	Provider                  string `json:"provider"`
	Model                     string `json:"model"`
	AuthKind                  string `json:"authKind"`
	Active                    bool   `json:"active"`
	ConfigurationStatus       string `json:"configurationStatus"`
	CredentialReadiness       string `json:"credentialReadiness"`
	ProfileRevision           string `json:"profileRevision"`
	ProviderOrigin            string `json:"providerOrigin"`
	BridgeTransport           string `json:"bridgeTransport"`
}
type CoreProfileMetadataReport struct {
	SourceSnapshot *CoreProfileSourceSnapshot `json:"sourceSnapshot,omitempty"`
	Version        int                        `json:"version"`
	Profiles       []CoreProfileMetadata      `json:"profiles"`
}
type CoreProfileSourceSnapshot struct {
	ID              string `json:"id"`
	CreatedAtUnixMS int64  `json:"createdAtUnixMs"`
	ExpiresAtUnixMS int64  `json:"expiresAtUnixMs"`
}
type CoreProfileService interface {
	Sources() []CoreProfileSource
	Inspect(context.Context, string) (CoreProfileMetadataReport, error)
	Verify(context.Context, CoreProfilePin) error
}
type CoreProfileActorScope struct {
	Version        int    `json:"version"`
	Mode           string `json:"mode"`
	ActorID        string `json:"actorId"`
	SpaceID        string `json:"spaceId"`
	AuthorityID    string `json:"authorityId"`
	DeviceID       string `json:"deviceId"`
	DeviceRevision uint64 `json:"deviceRevision"`
}
type CoreProfileLimits struct {
	MaxInputBytes   int `json:"maxInputBytes"`
	MaxOutputTokens int `json:"maxOutputTokens"`
	MaxRequests     int `json:"maxRequests"`
	MaxSteps        int `json:"maxSteps"`
	MaxSeconds      int `json:"maxSeconds"`
}
type CoreProfilePin struct {
	AuthKind                  string                `json:"authKind,omitempty"`
	CredentialSource          string                `json:"credentialSource,omitempty"`
	AuthPolicy                string                `json:"authPolicy,omitempty"`
	CredentialSnapshotID      string                `json:"credentialSnapshotId,omitempty"`
	CredentialExpiresAtUnixMS int64                 `json:"credentialExpiresAtUnixMs,omitempty"`
	Version                   int                   `json:"version"`
	FixtureOnly               bool                  `json:"fixtureOnly"`
	RuntimeSHA256             string                `json:"runtimeSha256"`
	SourceID                  string                `json:"sourceId"`
	SourceRevision            uint64                `json:"sourceRevision"`
	ProfileID                 string                `json:"profileId"`
	ProfileRevision           string                `json:"profileRevision"`
	Provider                  string                `json:"provider"`
	Model                     string                `json:"model"`
	ProviderOrigin            string                `json:"providerOrigin"`
	ConsentID                 string                `json:"consentId"`
	ConsentRevision           uint64                `json:"consentRevision"`
	ActorScope                CoreProfileActorScope `json:"actorScope"`
	ChannelID                 string                `json:"channelId"`
	AudienceRevision          uint64                `json:"audienceRevision"`
	BotID                     string                `json:"botId"`
	ProjectID                 string                `json:"projectId,omitempty"`
	ProjectRevision           uint64                `json:"projectRevision,omitempty"`
	ClientNonce               string                `json:"clientNonce"`
	ContextSHA256             string                `json:"contextSha256"`
	ResourcesSHA256           string                `json:"resourcesSha256"`
	Limits                    CoreProfileLimits     `json:"limits"`
	ExpiresAtUnixMS           int64                 `json:"expiresAtUnixMs"`
}
type CoreProfileSelection struct {
	ConsentID               string `json:"consentId"`
	ExpectedConsentRevision uint64 `json:"expectedConsentRevision"`
}
