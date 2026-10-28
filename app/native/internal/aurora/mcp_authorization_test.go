package aurora

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
)

type mcpDiscoveryTransport func(*http.Request) (*http.Response, error)

func (f mcpDiscoveryTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

const mcpResource = "https://mcp.example.com/team/mcp"
const mcpIssuer = "https://auth.example.com/tenant"

func mcpDiscoveryFixture(t *testing.T) (*http.Client, map[string]string, *[]string) {
	t.Helper()
	docs := map[string]string{
		"https://mcp.example.com/.well-known/oauth-protected-resource/team/mcp":  `{"resource":"` + mcpResource + `","authorization_servers":["` + mcpIssuer + `"],"scopes_supported":["files:read"]}`,
		"https://auth.example.com/.well-known/oauth-authorization-server/tenant": `{"issuer":"` + mcpIssuer + `","authorization_endpoint":"https://auth.example.com/authorize","token_endpoint":"https://auth.example.com/token","response_types_supported":["code"],"code_challenge_methods_supported":["S256"],"client_id_metadata_document_supported":true}`,
	}
	calls := []string{}
	client := &http.Client{Transport: mcpDiscoveryTransport(func(r *http.Request) (*http.Response, error) {
		calls = append(calls, r.URL.String())
		if r.Method != "GET" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" || r.Header.Get("X-API-Key") != "" || r.URL.RawQuery != "" {
			t.Error("discovery forwarded credentials or made a mutating request")
		}
		body, ok := docs[r.URL.String()]
		status := 200
		if !ok {
			status, body = 404, "fixture missing"
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: r}, nil
	})}
	return client, docs, &calls
}

func TestMCPChallengeParsingAndFailureCapture(t *testing.T) {
	for _, tc := range []struct {
		name, header string
		invalid      bool
	}{
		{"simple", `Bearer resource_metadata="https://mcp.example.com/meta", scope="read write"`, false},
		{"mixed", `Basic realm="other,realm", Bearer scope="read", resource_metadata="https://mcp.example.com/meta"`, false},
		{"escaped", `Bearer realm="quoted\"value, x", scope="read"`, false},
		{"empty scope", `Bearer scope=""`, false},
		{"metadata query", `Bearer resource_metadata="https://mcp.example.com/meta?tenant=one"`, false},
		{"duplicate", `Bearer scope="read", scope="write"`, true},
		{"ambiguous", `Bearer scope="read", Bearer scope="write"`, true},
		{"unterminated", `Bearer scope="read`, true},
		{"malformed", `Bearer resource_metadata`, true},
		{"private", `Bearer resource_metadata="https://127.0.0.1/meta"`, true},
		{"http", `Bearer resource_metadata="http://mcp.example.com/meta"`, true},
		{"scope control", "Bearer scope=\"read\twrite\"", true},
		{"scope spaces", `Bearer scope="read  write"`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			challenge, err := parseMCPChallenge([]string{tc.header})
			if (err != nil) != tc.invalid || !tc.invalid && challenge == nil {
				t.Fatal("challenge result", err)
			}
			response := &http.Response{StatusCode: 401, Header: http.Header{"Www-Authenticate": []string{tc.header}}}
			failure := mcpHTTPFailure(response).(*mcpFailure)
			if failure.authInvalid != tc.invalid || !reflect.DeepEqual(failure.challenge, challenge) {
				t.Fatal("HTTP failure lost challenge validation")
			}
			b, _ := json.Marshal(failure)
			if strings.Contains(string(b), "resource_metadata") || strings.Contains(string(b), "scope") {
				t.Fatal("raw challenge leaked through failure JSON")
			}
		})
	}
	if _, err := parseMCPChallenge([]string{strings.Repeat("x", 8193)}); err == nil {
		t.Fatal("unbounded challenge accepted")
	}
	if challenge, err := parseMCPChallenge([]string{`Basic realm="other"`, `bEaReR scope="read"`}); err != nil || !challenge.ScopePresent || !reflect.DeepEqual(challenge.Scopes, []string{"read"}) {
		t.Fatal("multiple header lines or case insensitive scheme lost", err)
	}
	if challenge, err := parseMCPChallenge([]string{`,Bearer,, scope="read",, resource_metadata="https://mcp.example.com/meta",`}); err != nil || challenge == nil || !challenge.ScopePresent || challenge.Metadata == "" || !reflect.DeepEqual(challenge.Scopes, []string{"read"}) {
		t.Fatal("empty list members discarded authoritative challenge scope", err)
	}
}

func TestMCPDiscoveryPathFallbackScopeAndIssuerChoice(t *testing.T) {
	for _, variant := range []string{"oauth", "oidc-insert", "oidc-append", "root"} {
		t.Run(variant, func(t *testing.T) {
			client, docs, calls := mcpDiscoveryFixture(t)
			original := mcpWellKnown(mcpIssuer, "oauth-authorization-server")
			if strings.HasPrefix(variant, "oidc") {
				target := mcpWellKnown(mcpIssuer, "openid-configuration")
				if variant == "oidc-append" {
					target = mcpIssuer + "/.well-known/openid-configuration"
				}
				docs[target] = docs[original]
				delete(docs, original)
			}
			if variant == "root" {
				docs["https://mcp.example.com/.well-known/oauth-protected-resource"] = docs[mcpWellKnown(mcpResource, "oauth-protected-resource")]
				delete(docs, mcpWellKnown(mcpResource, "oauth-protected-resource"))
			}
			jar, _ := cookiejar.New(nil)
			u, _ := url.Parse(mcpResource)
			jar.SetCookies(u, []*http.Cookie{{Name: "fixture-session", Value: "must-stay-local"}})
			client.Jar = jar
			out, err := discoverMCPAuthorization(context.Background(), client, mcpResource, &mcpAuthChallenge{Scopes: []string{"write"}, ScopePresent: true}, "")
			if err != nil || out.Authorization == nil || out.Authorization.Issuer != mcpIssuer || !out.Authorization.PKCES256 || !out.Authorization.ClientIDURL || !reflect.DeepEqual(out.Scopes, []string{"write"}) {
				t.Fatal("discovery result", err)
			}
			if len(*calls) < 2 || client.Jar != jar {
				t.Fatal("unexpected transport behavior")
			}
		})
	}
	client, docs, calls := mcpDiscoveryFixture(t)
	metadata := "https://metadata.example.com/resource"
	docs[metadata] = strings.Replace(docs[mcpWellKnown(mcpResource, "oauth-protected-resource")], `"authorization_servers":["`+mcpIssuer+`"]`, `"authorization_servers":["`+mcpIssuer+`","https://second.example.com"]`, 1)
	challenge := &mcpAuthChallenge{Metadata: metadata}
	out, err := discoverMCPAuthorization(context.Background(), client, mcpResource, challenge, "")
	if err != nil || len(out.Issuers) != 2 || out.Authorization != nil || len(*calls) != 1 || (*calls)[0] != metadata {
		t.Fatal("issuer choice or advertised metadata priority lost", err)
	}
	if _, err := discoverMCPAuthorization(context.Background(), client, mcpResource, challenge, "https://unlisted.example.com"); err == nil {
		t.Fatal("unadvertised issuer admitted")
	}
	if out, err := discoverMCPAuthorization(context.Background(), client, mcpResource, challenge, mcpIssuer); err != nil || out.Authorization == nil {
		t.Fatal("selected issuer not discovered", err)
	}
	challenge.ScopePresent = true
	if out, err := discoverMCPAuthorization(context.Background(), client, mcpResource, challenge, mcpIssuer); err != nil || len(out.Scopes) != 0 {
		t.Fatal("explicit empty scope was replaced by broader resource scopes", err)
	}
}

func TestMCPDiscoveryRejectsInvalidMetadata(t *testing.T) {
	for _, tc := range []struct{ name, target, body string }{
		{"resource mismatch", "resource", `{"resource":"https://wrong.example.com","authorization_servers":["` + mcpIssuer + `"]}`},
		{"duplicate resource", "resource", `{"resource":"` + mcpResource + `","RESOURCE":"` + mcpResource + `","authorization_servers":["` + mcpIssuer + `"]}`},
		{"private issuer", "resource", `{"resource":"` + mcpResource + `","authorization_servers":["https://169.254.169.254"]}`},
		{"no issuers", "resource", `{"resource":"` + mcpResource + `"}`},
		{"oversized", "resource", strings.Repeat(" ", 65537)},
		{"trailing JSON", "resource", `{} {}`},
		{"null", "resource", `null`},
		{"issuer mismatch", "auth", `{"issuer":"https://wrong.example.com","authorization_endpoint":"https://auth.example.com/a","token_endpoint":"https://auth.example.com/t","response_types_supported":["code"]}`},
		{"private endpoint", "auth", `{"issuer":"` + mcpIssuer + `","authorization_endpoint":"https://auth.example.com/a","token_endpoint":"https://localhost/token","response_types_supported":["code"]}`},
		{"missing code", "auth", `{"issuer":"` + mcpIssuer + `","authorization_endpoint":"https://auth.example.com/a","token_endpoint":"https://auth.example.com/t"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, docs, _ := mcpDiscoveryFixture(t)
			target := mcpWellKnown(mcpResource, "oauth-protected-resource")
			if tc.target == "auth" {
				target = mcpWellKnown(mcpIssuer, "oauth-authorization-server")
			}
			docs[target] = tc.body
			if _, err := discoverMCPAuthorization(context.Background(), client, mcpResource, nil, ""); err == nil {
				t.Fatal("invalid metadata admitted")
			}
		})
	}
	client, docs, _ := mcpDiscoveryFixture(t)
	target := mcpWellKnown(mcpIssuer, "oauth-authorization-server")
	docs[target] = strings.ReplaceAll(docs[target], `"S256"`, `"plain"`)
	if out, err := discoverMCPAuthorization(context.Background(), client, mcpResource, nil, ""); err != nil || out.Authorization.PKCES256 {
		t.Fatal("unsupported PKCE claimed ready", err)
	}
}

func TestMCPDiscoveryRootIssuerAndMetadataFailurePolicy(t *testing.T) {
	for _, suffix := range []string{"oauth-authorization-server", "openid-configuration"} {
		t.Run(suffix, func(t *testing.T) {
			client, docs, _ := mcpDiscoveryFixture(t)
			rootIssuer := "https://auth.example.com"
			docs[mcpWellKnown(mcpResource, "oauth-protected-resource")] = `{"resource":"` + mcpResource + `","authorization_servers":["` + rootIssuer + `"]}`
			docs[mcpWellKnown(rootIssuer, suffix)] = strings.ReplaceAll(docs[mcpWellKnown(mcpIssuer, "oauth-authorization-server")], mcpIssuer, rootIssuer)
			out, err := discoverMCPAuthorization(context.Background(), client, mcpResource, nil, "")
			if err != nil || out.Authorization.Issuer != rootIssuer {
				t.Fatal("root issuer metadata not discovered", err)
			}
		})
	}
	for _, status := range []int{401, 403, 429, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			client := &http.Client{Transport: mcpDiscoveryTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader("private-provider-error")), Header: make(http.Header), Request: r}, nil
			})}
			_, err := discoverMCPAuthorization(context.Background(), client, mcpResource, nil, "")
			if err == nil || calls != 1 || strings.Contains(err.Error(), "private-provider-error") {
				t.Fatal("authorization/transient failure bypassed or disclosed", calls)
			}
		})
	}
	client, _, calls := mcpDiscoveryFixture(t)
	if _, err := discoverMCPAuthorization(context.Background(), client, mcpResource, &mcpAuthChallenge{Metadata: "https://missing.example.com/meta"}, ""); err == nil || len(*calls) != 1 {
		t.Fatal("advertised metadata failure silently replaced by another source")
	}
}

func TestMCPDiscoveryNetworkPolicyAndCancellation(t *testing.T) {
	for _, raw := range []string{"http://example.com", "https://u:p@example.com", "https://127.0.0.1/meta", "https://[::1]/meta", "https://10.0.0.1/meta", "https://localhost/meta", "https://example.com/meta#", "https://example.com/meta?"} {
		if mcpDiscoveryURL(raw, false) {
			t.Fatal("unsafe discovery URL accepted")
		}
	}
	calls := 0
	client := &http.Client{Transport: mcpDiscoveryTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{"https://redirect.example.com"}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
	})}
	if _, err := discoverMCPAuthorization(context.Background(), client, mcpResource, nil, ""); err == nil || calls != 1 {
		t.Fatal("redirect followed or accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := discoverMCPAuthorization(ctx, mcpDiscoveryHTTP(), mcpResource, nil, ""); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost", err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("private network reached") }))
	defer server.Close()
	u, _ := url.Parse(server.URL)
	u.Host = "localhost:" + u.Port()
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if response, err := mcpDiscoveryHTTP().Do(req); err == nil {
		response.Body.Close()
		t.Fatal("private DNS target reached")
	}
}

func TestMCPAuthorizationPreviewScopeConfigAndNoGrant(t *testing.T) {
	s, closeService := historyService(t, t.TempDir())
	defer closeService()
	bot := firstBot(s)
	s.V = mcpFixtureService(t, "fixture-private-header").V
	srv := mcpServer{Name: "fixture", URL: mcpResource, Header: "Authorization", Secret: "MCP_FIXTURE"}
	s.ext.mu.Lock()
	s.ext.d.Bots[bot] = &botExt{MCP: []mcpServer{srv}}
	s.ext.mu.Unlock()
	client, _, calls := mcpDiscoveryFixture(t)
	old := mcpDiscoveryHTTP
	mcpDiscoveryHTTP = func() *http.Client { return client }
	t.Cleanup(func() { mcpDiscoveryHTTP = old })
	request := func(workspace, serverURL string) *httptest.ResponseRecorder {
		b, _ := json.Marshal(map[string]string{"workspaceId": workspace, "name": srv.Name, "url": serverURL})
		r := httptest.NewRequest("POST", "/app/api/agents/"+bot+"/mcp/authorization/preview", strings.NewReader(string(b)))
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	if w := request("wrong", mcpResource); w.Code != 409 || len(*calls) != 0 {
		t.Fatal("wrong workspace reached discovery", w.Code)
	}
	if w := request(s.workspaceID, "https://wrong.example.com"); w.Code != 409 || len(*calls) != 0 {
		t.Fatal("stale server reached discovery", w.Code)
	}
	if w := request(s.workspaceID, mcpResource); w.Code != 200 || strings.Contains(w.Body.String(), "MCP_FIXTURE") {
		t.Fatal("preview failed or exposed credential name", w.Code)
	}
	cache := s.mcpState()
	cache.mu.Lock()
	cache.failures[mcpKey(bot, srv)] = &mcpFailure{State: "reconnect_required", authInvalid: true}
	cache.mu.Unlock()
	before := len(*calls)
	if w := request(s.workspaceID, mcpResource); w.Code != 422 || len(*calls) != before {
		t.Fatal("invalid challenge used fallback discovery", w.Code)
	}
	cache.mu.Lock()
	delete(cache.failures, mcpKey(bot, srv))
	cache.mu.Unlock()
	original := client.Transport
	client.Transport = mcpDiscoveryTransport(func(r *http.Request) (*http.Response, error) {
		s.ext.mu.Lock()
		s.ext.d.Bots[bot].MCP[0].Secret = "changed-during-discovery"
		s.ext.mu.Unlock()
		return original.RoundTrip(r)
	})
	if w := request(s.workspaceID, mcpResource); w.Code != 409 {
		t.Fatal("config changed while reading metadata but stale result returned", w.Code)
	}
}
