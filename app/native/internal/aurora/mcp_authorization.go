package aurora

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"nekoaurora.local/native/internal/tools"
)

var errMCPDiscovery = errors.New("không xác minh được thông tin cấp quyền MCP")

type mcpAuthChallenge struct {
	Metadata     string
	Scopes       []string
	ScopePresent bool
}

func mcpAuthToken(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", c)) {
			return false
		}
	}
	return true
}

func mcpAuthValue(s string) (string, bool) {
	if !strings.HasPrefix(s, "\"") {
		return s, mcpAuthToken(s)
	}
	var value strings.Builder
	for i := 1; i < len(s); i++ {
		c := s[i]
		if c == '"' {
			return value.String(), i == len(s)-1
		}
		if c == '\\' {
			i++
			if i >= len(s) {
				return "", false
			}
			c = s[i]
		}
		if c < 32 || c >= 127 {
			return "", false
		}
		value.WriteByte(c)
	}
	return "", false
}

func mcpScopes(scopes []string) bool {
	if len(scopes) > 64 {
		return false
	}
	for _, scope := range scopes {
		if len(scope) == 0 || len(scope) > 128 {
			return false
		}
		for _, c := range scope {
			if c < 33 || c > 126 || c == '"' || c == '\\' {
				return false
			}
		}
	}
	return true
}

func parseMCPChallenge(headers []string) (*mcpAuthChallenge, error) {
	if len(headers) > 16 || len(strings.Join(headers, ",")) > 8192 {
		return nil, errMCPDiscovery
	}
	var parts []string
	for _, header := range headers {
		quoted, escaped, start := false, false, 0
		for i, c := range header {
			if c < 32 && c != '\t' || c == 127 {
				return nil, errMCPDiscovery
			}
			if escaped {
				escaped = false
			} else if quoted && c == '\\' {
				escaped = true
			} else if c == '"' {
				quoted = !quoted
			} else if !quoted && c == ',' {
				parts = append(parts, strings.TrimSpace(header[start:i]))
				start = i + 1
			}
		}
		if quoted || escaped {
			return nil, errMCPDiscovery
		}
		parts = append(parts, strings.TrimSpace(header[start:]))
	}
	var result *mcpAuthChallenge
	params, bearer := map[string]bool{}, false
	for _, part := range parts {
		if part == "" {
			continue
		}
		i := strings.IndexAny(part, " =\t")
		if i < 0 {
			i = len(part)
		}
		name, rest := part[:i], strings.TrimSpace(part[i:])
		if !strings.HasPrefix(rest, "=") {
			bearer = strings.EqualFold(name, "Bearer")
			if !bearer {
				continue
			}
			if result != nil {
				return nil, errMCPDiscovery
			}
			result = &mcpAuthChallenge{}
			if rest == "" {
				continue
			}
			name, rest, _ = strings.Cut(rest, "=")
			rest = "=" + rest
		}
		if !bearer {
			continue
		}
		name = strings.ToLower(strings.TrimSpace(name))
		value, ok := mcpAuthValue(strings.TrimSpace(strings.TrimPrefix(rest, "=")))
		if !mcpAuthToken(name) || !ok || params[name] {
			return nil, errMCPDiscovery
		}
		params[name] = true
		switch name {
		case "resource_metadata":
			if !mcpDiscoveryURL(value, true) {
				return nil, errMCPDiscovery
			}
			result.Metadata = value
		case "scope":
			result.ScopePresent = true
			if value != "" {
				result.Scopes = strings.Split(value, " ")
			}
			if !mcpScopes(result.Scopes) {
				return nil, errMCPDiscovery
			}
		}
	}
	return result, nil
}

func mcpDiscoveryURL(raw string, query bool) bool {
	u, err := url.Parse(raw)
	return err == nil && len(raw) <= 2048 && u.Fragment == "" && !strings.Contains(raw, "#") && (query || u.RawQuery == "" && !u.ForceQuery) && tools.PublicURL(raw, false)
}

type mcpAuthorizationMetadata struct {
	Issuer        string `json:"issuer"`
	Authorization string `json:"authorizationEndpoint"`
	Token         string `json:"tokenEndpoint"`
	PKCES256      bool   `json:"pkceS256"`
	ClientIDURL   bool   `json:"clientIdMetadataDocument"`
}

type mcpAuthorizationDiscovery struct {
	Resource      string                    `json:"resource"`
	MetadataURL   string                    `json:"metadataUrl"`
	Issuers       []string                  `json:"issuers"`
	Scopes        []string                  `json:"scopes"`
	Authorization *mcpAuthorizationMetadata `json:"authorization,omitempty"`
}

var mcpDiscoveryHTTP = func() *http.Client { return tools.SafeClient(12 * time.Second) }

func mcpMetadata(ctx context.Context, client *http.Client, target string, out any) (bool, error) {
	if !mcpDiscoveryURL(target, true) {
		return false, errMCPDiscovery
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return false, errMCPDiscovery
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		return false, errMCPDiscovery
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 || resp.StatusCode == 405 {
		return false, nil
	}
	if resp.StatusCode != 200 {
		return false, errMCPDiscovery
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, (64<<10)+1))
	if err != nil || len(body) > 64<<10 || !utf8.Valid(body) {
		return false, errMCPDiscovery
	}
	d := json.NewDecoder(bytes.NewReader(body))
	if token, err := d.Token(); err != nil || token != json.Delim('{') {
		return false, errMCPDiscovery
	}
	seen := map[string]bool{}
	for d.More() {
		token, err := d.Token()
		key, ok := token.(string)
		key = strings.ToLower(key)
		if err != nil || !ok || seen[key] {
			return false, errMCPDiscovery
		}
		seen[key] = true
		var value json.RawMessage
		if d.Decode(&value) != nil {
			return false, errMCPDiscovery
		}
	}
	if _, err := d.Token(); err != nil {
		return false, errMCPDiscovery
	}
	if _, err := d.Token(); err != io.EOF || json.Unmarshal(body, out) != nil {
		return false, errMCPDiscovery
	}
	return true, nil
}

func mcpWellKnown(raw, suffix string) string {
	u, _ := url.Parse(raw)
	return u.Scheme + "://" + u.Host + "/.well-known/" + suffix + u.EscapedPath()
}

func discoverMCPAuthorization(ctx context.Context, base *http.Client, resource string, challenge *mcpAuthChallenge, issuer string) (*mcpAuthorizationDiscovery, error) {
	if !mcpDiscoveryURL(resource, false) {
		return nil, errMCPDiscovery
	}
	client := *base
	client.Jar = nil
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	u, _ := url.Parse(resource)
	paths := []string{mcpWellKnown(resource, "oauth-protected-resource")}
	root := u.Scheme + "://" + u.Host + "/.well-known/oauth-protected-resource"
	if paths[0] != root {
		paths = append(paths, root)
	}
	if challenge != nil && challenge.Metadata != "" {
		paths = []string{challenge.Metadata}
	}
	var protected struct {
		Resource string   `json:"resource"`
		Issuers  []string `json:"authorization_servers"`
		Scopes   []string `json:"scopes_supported"`
	}
	metadataURL := ""
	for _, path := range paths {
		ok, err := mcpMetadata(ctx, &client, path, &protected)
		if err != nil {
			return nil, err
		}
		if ok {
			metadataURL = path
			break
		}
	}
	if metadataURL == "" || protected.Resource != resource || len(protected.Issuers) < 1 || len(protected.Issuers) > 16 || !mcpScopes(protected.Scopes) {
		return nil, errMCPDiscovery
	}
	for i, candidate := range protected.Issuers {
		if !mcpDiscoveryURL(candidate, false) || slices.Contains(protected.Issuers[:i], candidate) {
			return nil, errMCPDiscovery
		}
	}
	out := &mcpAuthorizationDiscovery{Resource: resource, MetadataURL: metadataURL, Issuers: protected.Issuers, Scopes: protected.Scopes}
	if challenge != nil && challenge.ScopePresent {
		out.Scopes = append([]string(nil), challenge.Scopes...)
	}
	if issuer == "" && len(protected.Issuers) == 1 {
		issuer = protected.Issuers[0]
	}
	if issuer == "" {
		return out, nil
	}
	if !slices.Contains(protected.Issuers, issuer) {
		return nil, errMCPDiscovery
	}
	paths = []string{mcpWellKnown(issuer, "oauth-authorization-server"), mcpWellKnown(issuer, "openid-configuration")}
	u, _ = url.Parse(issuer)
	if u.Path != "" {
		paths = append(paths, strings.TrimSuffix(issuer, "/")+"/.well-known/openid-configuration")
	}
	var auth struct {
		Issuer        string   `json:"issuer"`
		Authorization string   `json:"authorization_endpoint"`
		Token         string   `json:"token_endpoint"`
		PKCE          []string `json:"code_challenge_methods_supported"`
		ResponseTypes []string `json:"response_types_supported"`
		ClientIDURL   bool     `json:"client_id_metadata_document_supported"`
	}
	for _, path := range paths {
		ok, err := mcpMetadata(ctx, &client, path, &auth)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		if auth.Issuer != issuer || !mcpDiscoveryURL(auth.Authorization, true) || !mcpDiscoveryURL(auth.Token, true) || !slices.Contains(auth.ResponseTypes, "code") {
			return nil, errMCPDiscovery
		}
		out.Authorization = &mcpAuthorizationMetadata{Issuer: issuer, Authorization: auth.Authorization, Token: auth.Token, PKCES256: slices.Contains(auth.PKCE, "S256"), ClientIDURL: auth.ClientIDURL}
		return out, nil
	}
	return nil, errMCPDiscovery
}

func (s *Service) mcpAuthorizationRoutes() {
	s.mux.HandleFunc("POST /app/api/agents/{id}/mcp/authorization/preview", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			WorkspaceID string `json:"workspaceId"`
			Name        string `json:"name"`
			URL         string `json:"url"`
			Issuer      string `json:"issuer"`
		}
		if decodeStrict(r, &in) != nil {
			fail(w, ErrInvalid)
			return
		}
		if in.WorkspaceID != s.workspaceID {
			writeJSON(w, 409, map[string]string{"error": "workspace_changed"})
			return
		}
		id := r.PathValue("id")
		srv, ok := s.mcpServerOf(id, in.Name)
		if !ok || srv.URL != in.URL {
			writeJSON(w, 409, map[string]string{"error": "mcp_changed"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		cache, key := s.mcpState(), mcpKey(id, srv)
		unlock, err := cache.lock(ctx, "authorization-preview\x00"+key)
		if err != nil {
			fail(w, err)
			return
		}
		defer unlock()
		cache.mu.Lock()
		failure := cache.failures[key]
		cache.mu.Unlock()
		var challenge *mcpAuthChallenge
		if failure != nil {
			if failure.authInvalid {
				writeJSON(w, 422, map[string]string{"error": "mcp_authorization_metadata"})
				return
			}
			challenge = failure.challenge
		}
		out, err := discoverMCPAuthorization(ctx, mcpDiscoveryHTTP(), srv.URL, challenge, in.Issuer)
		if err != nil {
			writeJSON(w, 422, map[string]string{"error": "mcp_authorization_metadata"})
			return
		}
		if current, ok := s.mcpServerOf(id, in.Name); !ok || current != srv {
			writeJSON(w, 409, map[string]string{"error": "mcp_changed"})
			return
		}
		writeJSON(w, 200, out)
	})
}
