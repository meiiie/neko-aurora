package aurora

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"nekoaurora.local/native/internal/tools"
	"nekoaurora.local/native/internal/vault"
)

const mcpOAuthCallbackPath = "/app/oauth/mcp/callback.html"

var (
	errMCPOAuthInvalid  = errors.New("mcp_oauth_invalid")
	errMCPOAuthChanged  = errors.New("mcp_oauth_changed")
	errMCPOAuthStorage  = errors.New("mcp_oauth_storage")
	errMCPOAuthUnknown  = errors.New("mcp_oauth_outcome_unknown")
	errMCPOAuthDisabled = errors.New("mcp_oauth_disabled")
)

type mcpOAuthStore interface {
	GetOAuthRecord(string) (vault.OAuthRecord, error)
	SetOAuthRecord(string, vault.OAuthRecord) error
}

type mcpOAuthManager struct {
	store   mcpOAuthStore
	origins []string
	http    func() *http.Client
	now     func() time.Time
}

type mcpOAuthBinding struct {
	Workspace             string    `json:"workspace"`
	Bot                   string    `json:"bot"`
	Server                mcpServer `json:"server"`
	ConfigRevision        uint64    `json:"configRevision"`
	Issuer                string    `json:"issuer"`
	ClientID              string    `json:"clientId"`
	RedirectURI           string    `json:"redirectUri"`
	AuthorizationEndpoint string    `json:"authorizationEndpoint"`
	TokenEndpoint         string    `json:"tokenEndpoint"`
	RequireIssuer         bool      `json:"requireIssuer"`
	Scopes                []string  `json:"scopes"`
}

type mcpOAuthState struct {
	Version       int             `json:"version"`
	Generation    string          `json:"generation"`
	Revision      uint64          `json:"revision"`
	Phase         string          `json:"phase"`
	Binding       mcpOAuthBinding `json:"binding"`
	PendingUntil  int64           `json:"pendingUntil,omitempty"`
	ExpiresAt     int64           `json:"expiresAt,omitempty"`
	GrantedScopes []string        `json:"grantedScopes,omitempty"`
	StateHash     string          `json:"stateHash,omitempty"`
	CallbackHash  string          `json:"callbackHash,omitempty"`
	RefreshNonce  string          `json:"refreshNonce,omitempty"`
	RefreshBase   uint64          `json:"refreshBase,omitempty"`
}

type mcpOAuthRecord struct {
	mcpOAuthState
	secrets map[string]string
}

func mcpOAuthRecordID(workspace, bot, name string) string {
	b, _ := json.Marshal([]string{"mcp-oauth-v1", workspace, bot, name})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func mcpOAuthHash(value string) string {
	h := sha256.Sum256([]byte(value))
	return hex.EncodeToString(h[:])
}

func mcpOAuthEqual(a, b string) bool {
	return len(a) == len(b) && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func mcpOAuthRandom(size int) (string, error) {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return "", errMCPOAuthStorage
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func mcpOAuthOpaque(value string, maximum int) bool {
	if value == "" || len(value) > maximum || !utf8.ValidString(value) {
		return false
	}
	for _, c := range value {
		if unicode.IsControl(c) {
			return false
		}
	}
	return true
}

func mcpOAuthOrigin(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 2048 || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(raw, "#") || u.Hostname() == "" {
		return false
	}
	if raw != u.Scheme+"://"+u.Host {
		return false
	}
	if u.Scheme == "https" {
		return tools.PublicURL(raw, false)
	}
	ip := net.ParseIP(u.Hostname())
	return u.Scheme == "http" && ip != nil && ip.IsLoopback()
}

func newMCPOAuthManager(store mcpOAuthStore, origins []string, now func() time.Time) *mcpOAuthManager {
	allowed := []string{}
	for _, origin := range origins {
		if mcpOAuthOrigin(origin) && !slices.Contains(allowed, origin) {
			allowed = append(allowed, origin)
		}
	}
	if now == nil {
		now = time.Now
	}
	return &mcpOAuthManager{store: store, origins: allowed, now: now, http: func() *http.Client { return tools.SafeClient(15 * time.Second) }}
}

func (m *mcpOAuthManager) redirect(origin string) (string, error) {
	if m == nil || !slices.Contains(m.origins, origin) {
		return "", errMCPOAuthDisabled
	}
	return origin + mcpOAuthCallbackPath, nil
}

func mcpOAuthUniqueJSON(body []byte, out any, strict bool) error {
	if len(body) == 0 || len(body) > 64<<10 || !utf8.Valid(body) {
		return errMCPOAuthInvalid
	}
	d := json.NewDecoder(bytes.NewReader(body))
	if token, err := d.Token(); err != nil || token != json.Delim('{') {
		return errMCPOAuthInvalid
	}
	seen := []string{}
	for d.More() {
		token, err := d.Token()
		key, ok := token.(string)
		if err != nil || !ok || len(seen) >= 128 {
			return errMCPOAuthInvalid
		}
		for _, previous := range seen {
			if strings.EqualFold(previous, key) {
				return errMCPOAuthInvalid
			}
		}
		seen = append(seen, key)
		var value json.RawMessage
		if d.Decode(&value) != nil {
			return errMCPOAuthInvalid
		}
	}
	if _, err := d.Token(); err != nil {
		return errMCPOAuthInvalid
	}
	if _, err := d.Token(); err != io.EOF {
		return errMCPOAuthInvalid
	}
	d = json.NewDecoder(bytes.NewReader(body))
	if strict {
		d.DisallowUnknownFields()
	}
	if d.Decode(out) != nil {
		return errMCPOAuthInvalid
	}
	return nil
}

func (m *mcpOAuthManager) read(id string) (*mcpOAuthRecord, error) {
	if m == nil || m.store == nil {
		return nil, errMCPOAuthDisabled
	}
	stored, err := m.store.GetOAuthRecord(id)
	if errors.Is(err, vault.ErrMissing) {
		return nil, nil
	}
	if err != nil {
		return nil, errMCPOAuthStorage
	}
	var state mcpOAuthState
	if mcpOAuthUniqueJSON(stored.Data, &state, true) != nil || state.Version != 1 || len(state.Generation) != 22 || state.Revision == 0 || state.Revision >= 9007199254740991 || !slices.Contains([]string{"pending", "exchanging", "connected", "refreshing", "denied", "disconnected"}, state.Phase) || id != mcpOAuthRecordID(state.Binding.Workspace, state.Binding.Bot, state.Binding.Server.Name) {
		return nil, errMCPOAuthStorage
	}
	return &mcpOAuthRecord{mcpOAuthState: state, secrets: stored.Secrets}, nil
}

func (m *mcpOAuthManager) save(id string, record *mcpOAuthRecord) error {
	data, err := json.Marshal(record.mcpOAuthState)
	if err != nil || m.store.SetOAuthRecord(id, vault.OAuthRecord{Data: data, Secrets: record.secrets}) != nil {
		return errMCPOAuthStorage
	}
	return nil
}

func mcpOAuthExpected(record *mcpOAuthRecord, generation string, revision uint64) bool {
	if record == nil {
		return generation == "" && revision == 0
	}
	return record.Generation == generation && record.Revision == revision
}

func mcpOAuthAuthorizationURL(record *mcpOAuthRecord) (string, error) {
	if record.Phase != "pending" || record.secrets["state"] == "" || record.secrets["verifier"] == "" || !mcpDiscoveryURL(record.Binding.AuthorizationEndpoint, true) {
		return "", errMCPOAuthInvalid
	}
	u, _ := url.Parse(record.Binding.AuthorizationEndpoint)
	q := u.Query()
	challenge := sha256.Sum256([]byte(record.secrets["verifier"]))
	q.Set("response_type", "code")
	q.Set("client_id", record.Binding.ClientID)
	q.Set("redirect_uri", record.Binding.RedirectURI)
	q.Set("resource", record.Binding.Server.URL)
	q.Set("state", record.secrets["state"])
	q.Set("code_challenge", base64.RawURLEncoding.EncodeToString(challenge[:]))
	q.Set("code_challenge_method", "S256")
	q.Del("scope")
	if len(record.Binding.Scopes) > 0 {
		q.Set("scope", strings.Join(record.Binding.Scopes, " "))
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

type mcpOAuthTokens struct {
	access, refresh string
	expiresAt       int64
	scopes          []string
}

func mcpOAuthBearer(value string) bool {
	if len(value) < 4 || len(value) > 8192 {
		return false
	}
	padding := false
	for _, c := range value {
		if c == '=' {
			padding = true
			continue
		}
		if padding || !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || strings.ContainsRune("-._~+/", c)) {
			return false
		}
	}
	return value[0] != '='
}

func (m *mcpOAuthManager) token(ctx context.Context, binding mcpOAuthBinding, params url.Values, allowedScopes []string) (*mcpOAuthTokens, error) {
	if !mcpDiscoveryURL(binding.TokenEndpoint, true) {
		return nil, errMCPOAuthInvalid
	}
	params.Set("client_id", binding.ClientID)
	params.Set("resource", binding.Server.URL)
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", binding.TokenEndpoint, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, errMCPOAuthUnknown
	}
	req.GetBody = nil
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	client := *m.http()
	client.Jar = nil
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, err := client.Do(req)
	if err != nil {
		return nil, errMCPOAuthUnknown
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, errMCPOAuthUnknown
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, (64<<10)+1))
	if err != nil {
		return nil, errMCPOAuthUnknown
	}
	var response struct {
		Access  string          `json:"access_token"`
		Refresh string          `json:"refresh_token"`
		Type    string          `json:"token_type"`
		Expires json.RawMessage `json:"expires_in"`
		Scope   *string         `json:"scope"`
	}
	if mcpOAuthUniqueJSON(body, &response, false) != nil || !strings.EqualFold(response.Type, "Bearer") || !mcpOAuthBearer(response.Access) || response.Refresh != "" && (!mcpOAuthOpaque(response.Refresh, 8192) || len(response.Refresh) < 4) {
		return nil, errMCPOAuthUnknown
	}
	tokens := &mcpOAuthTokens{access: response.Access, refresh: response.Refresh, scopes: append([]string(nil), allowedScopes...)}
	if response.Expires != nil {
		var seconds int64
		if json.Unmarshal(response.Expires, &seconds) != nil || seconds <= 0 || seconds > 366*24*3600 {
			return nil, errMCPOAuthUnknown
		}
		tokens.expiresAt = m.now().Unix() + seconds
	}
	if response.Scope != nil {
		tokens.scopes = nil
		if *response.Scope != "" {
			tokens.scopes = strings.Split(*response.Scope, " ")
		}
		if !mcpScopes(tokens.scopes) {
			return nil, errMCPOAuthUnknown
		}
		for i, scope := range tokens.scopes {
			if !slices.Contains(allowedScopes, scope) || slices.Contains(tokens.scopes[:i], scope) {
				return nil, errMCPOAuthUnknown
			}
		}
	}
	return tokens, nil
}
