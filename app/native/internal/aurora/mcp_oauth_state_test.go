package aurora

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"nekoaurora.local/native/internal/vault"
)

func oauthStateFixture(t *testing.T) (*mcpOAuthManager, *mcpOAuthRecord, string, string) {
	t.Helper()
	dir := t.TempDir()
	v, err := vault.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := newMCPOAuthManager(v, []string{"http://127.0.0.1:41789"}, func() time.Time { return time.Unix(1800000000, 0) })
	generation, _ := mcpOAuthRandom(16)
	secret, _ := mcpOAuthRandom(32)
	verifier, _ := mcpOAuthRandom(32)
	r := &mcpOAuthRecord{mcpOAuthState: mcpOAuthState{Version: 1, Generation: generation, Revision: 1, Phase: "pending", Binding: mcpOAuthBinding{
		Workspace: "fixture-workspace", Bot: "fixture-bot", Server: mcpServer{Name: "fixture", URL: mcpResource}, ConfigRevision: 7,
		Issuer: mcpIssuer, ClientID: "fixture-public-client", RedirectURI: m.origins[0] + mcpOAuthCallbackPath,
		AuthorizationEndpoint: "https://auth.example.com/authorize?client_id=untrusted&scope=excess&state=untrusted",
		TokenEndpoint:         "https://auth.example.com/token", Scopes: []string{"files:read"},
	}, PendingUntil: m.now().Unix() + 600}, secrets: map[string]string{}}
	id := mcpOAuthRecordID(r.Binding.Workspace, r.Binding.Bot, r.Binding.Server.Name)
	r.secrets["state"], r.secrets["verifier"] = id+"."+secret, verifier
	r.StateHash = mcpOAuthHash(r.secrets["state"])
	return m, r, id, dir
}

func TestMCPOAuthStateTrustedOriginsAndAuthorizationURL(t *testing.T) {
	for _, origin := range []string{"https://aurora.example.com", "http://127.0.0.1:41789", "http://[::1]:8080"} {
		if !mcpOAuthOrigin(origin) {
			t.Fatal("valid origin rejected", origin)
		}
	}
	for _, origin := range []string{"http://0.0.0.0:8080", "http://aurora.example.com", "https://127.0.0.1", "https://u:p@aurora.example.com", "https://aurora.example.com/", "https://aurora.example.com?", "https://aurora.example.com#", "https://aurora.example.com/path", "null"} {
		if mcpOAuthOrigin(origin) {
			t.Fatal("untrusted origin accepted", origin)
		}
	}
	m, r, _, _ := oauthStateFixture(t)
	if _, err := m.redirect("https://attacker.example.com"); !errors.Is(err, errMCPOAuthDisabled) {
		t.Fatal("unconfigured callback origin accepted")
	}
	raw, err := mcpOAuthAuthorizationURL(r)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(raw)
	q := u.Query()
	hash := sha256.Sum256([]byte(r.secrets["verifier"]))
	want := url.Values{"response_type": {"code"}, "client_id": {r.Binding.ClientID}, "redirect_uri": {r.Binding.RedirectURI}, "resource": {mcpResource}, "state": {r.secrets["state"]}, "code_challenge": {base64.RawURLEncoding.EncodeToString(hash[:])}, "code_challenge_method": {"S256"}, "scope": {"files:read"}}
	if !reflect.DeepEqual(q, want) {
		t.Fatal("authorization parameters not bound to the pending record")
	}
	if strings.Contains(raw, r.secrets["verifier"]) {
		t.Fatal("authorization URL exposed verifier")
	}
	r.Phase = "exchanging"
	if _, err := mcpOAuthAuthorizationURL(r); err == nil {
		t.Fatal("consumed attempt generated another login URL")
	}
}

func TestMCPOAuthStateEncryptedRoundTripAndExpectedGeneration(t *testing.T) {
	m, r, id, dir := oauthStateFixture(t)
	if !mcpOAuthExpected(nil, "", 0) || mcpOAuthExpected(nil, r.Generation, 1) || !mcpOAuthExpected(r, r.Generation, 1) || mcpOAuthExpected(r, r.Generation, 2) || mcpOAuthExpected(r, "other", 1) {
		t.Fatal("generation/revision comparison")
	}
	if err := m.save(id, r); err != nil {
		t.Fatal(err)
	}
	v, err := vault.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	reopened := newMCPOAuthManager(v, m.origins, m.now)
	got, err := reopened.read(id)
	if err != nil || !reflect.DeepEqual(got, r) {
		t.Fatal("pending authorization did not survive encrypted reopen", err)
	}
	got.secrets["verifier"] = "changed"
	second, err := m.read(id)
	if err != nil || second.secrets["verifier"] != r.secrets["verifier"] {
		t.Fatal("returned state aliases stored credential")
	}
	bad := r.mcpOAuthState
	bad.Binding.Bot = "other"
	data, _ := json.Marshal(bad)
	if err := v.SetOAuthRecord(id, vault.OAuthRecord{Data: data, Secrets: r.secrets}); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.read(id); !errors.Is(err, errMCPOAuthStorage) {
		t.Fatal("record identity mismatch accepted")
	}
}

func TestMCPOAuthStateTokenRequestAndValidation(t *testing.T) {
	m, r, _, _ := oauthStateFixture(t)
	jar, _ := cookiejar.New(nil)
	endpoint, _ := url.Parse(r.Binding.TokenEndpoint)
	jar.SetCookies(endpoint, []*http.Cookie{{Name: "fixture", Value: "must-not-leak"}})
	response := `{"access_token":"fixture-access","refresh_token":"fixture-refresh","token_type":"Bearer","expires_in":3600,"scope":"files:read"}`
	calls := 0
	client := &http.Client{Jar: jar, Transport: mcpDiscoveryTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.GetBody != nil || req.Method != "POST" || req.URL.String() != r.Binding.TokenEndpoint || req.Header.Get("Cookie") != "" || req.Header.Get("Authorization") != "" {
			t.Error("token request leaked credentials or allowed replay")
		}
		body, _ := io.ReadAll(req.Body)
		params, err := url.ParseQuery(string(body))
		if err != nil || params.Get("resource") != mcpResource || params.Get("client_id") != r.Binding.ClientID || params.Get("code") != "fixture-code" || params.Get("code_verifier") != r.secrets["verifier"] || params.Get("redirect_uri") != r.Binding.RedirectURI {
			t.Error("token request lost binding")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(response)), Header: http.Header{}, Request: req}, nil
	})}
	m.http = func() *http.Client { return client }
	params := url.Values{"grant_type": {"authorization_code"}, "code": {"fixture-code"}, "code_verifier": {r.secrets["verifier"]}, "redirect_uri": {r.Binding.RedirectURI}}
	tokens, err := m.token(context.Background(), r.Binding, params, r.Binding.Scopes)
	if err != nil || tokens.access != "fixture-access" || tokens.refresh != "fixture-refresh" || tokens.expiresAt != m.now().Unix()+3600 || !reflect.DeepEqual(tokens.scopes, r.Binding.Scopes) {
		t.Fatal("valid response rejected", err)
	}
	for _, body := range []string{
		`{"access_token":"first-token","ACCESS_TOKEN":"second-token","token_type":"Bearer"}`,
		`{"access_token":"fixture-access","token_type":"Basic"}`,
		`{"access_token":"header\r\nInjection","token_type":"Bearer"}`,
		`{"access_token":"fixture-access","token_type":"Bearer","scope":"files:write"}`,
		`{"access_token":"fixture-access","token_type":"Bearer","scope":"files:read files:read"}`,
		`{"access_token":"fixture-access","token_type":"Bearer","expires_in":-1}`,
		`{"access_token":"fixture-access","token_type":"Bearer","expires_in":1.5}`,
		`{"access_token":"fixture-access","token_type":"Bearer","expires_in":9999999999999}`,
		`{"access_token":"fixture-access","token_type":"Bearer","expires_in":"3600"}`,
		`{"access_token":"fixture-access","token_type":"Bearer"} {}`,
		strings.Repeat(" ", 64<<10) + `{}`,
	} {
		response = body
		before := calls
		if _, err := m.token(context.Background(), r.Binding, params, r.Binding.Scopes); !errors.Is(err, errMCPOAuthUnknown) {
			t.Errorf("invalid token response accepted: %d bytes", len(body))
		}
		if calls != before+1 {
			t.Fatal("token request replayed")
		}
	}
}

func TestMCPOAuthStateTokenRedirectAndFailuresNeverRetry(t *testing.T) {
	for _, mode := range []string{"redirect", "transport", "rate_limit", "private_endpoint"} {
		t.Run(mode, func(t *testing.T) {
			m, r, _, _ := oauthStateFixture(t)
			calls := 0
			m.http = func() *http.Client {
				return &http.Client{Transport: mcpDiscoveryTransport(func(req *http.Request) (*http.Response, error) {
					calls++
					if mode == "transport" {
						return nil, errors.New("fixture-secret-value")
					}
					code := 429
					if mode == "redirect" {
						code = 307
					}
					return &http.Response{StatusCode: code, Header: http.Header{"Location": {"https://other.example.com/token"}}, Body: io.NopCloser(strings.NewReader("fixture-secret-value")), Request: req}, nil
				})}
			}
			if mode == "private_endpoint" {
				r.Binding.TokenEndpoint = "https://127.0.0.1/token"
			}
			_, err := m.token(context.Background(), r.Binding, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {"fixture-refresh"}}, r.Binding.Scopes)
			if err == nil || strings.Contains(err.Error(), "fixture-secret-value") {
				t.Fatal("failure accepted or leaked remote details")
			}
			want := 1
			if mode == "private_endpoint" {
				want = 0
			}
			if calls != want {
				t.Fatalf("unexpected network count %d", calls)
			}
		})
	}
}
