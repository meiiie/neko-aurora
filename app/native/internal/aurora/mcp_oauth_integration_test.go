package aurora

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMCPOAuthRoutesScopeOriginAndReceipts(t *testing.T) {
	f := newOAuthFlowFixture(t)
	do := func(path string, in any, origins ...string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(in)
		if raw, ok := in.(string); ok {
			body = []byte(raw)
		}
		r := httptest.NewRequest("POST", "/app/api/"+path, strings.NewReader(string(body)))
		for _, origin := range origins {
			r.Header.Add("Origin", origin)
		}
		w := httptest.NewRecorder()
		f.s.ServeHTTP(w, r)
		return w
	}
	path := "agents/" + f.bot + "/mcp/oauth/"
	for _, origins := range [][]string{nil, {"null"}, {"https://evil.example"}, {oauthFixtureOrigin, oauthFixtureOrigin}} {
		if w := do(path+"status", f.identity(nil), origins...); w.Code != 403 {
			t.Fatal("untrusted origin accepted", w.Code)
		}
	}
	for _, body := range []string{`{"name":"fixture","Name":"fixture"}`, `{"scopes":[],"Å¿copes":[]}`, `{"name":"fixture","unknown":1}`, strings.Repeat(" ", 65537)} {
		if w := do(path+"start", body, oauthFixtureOrigin); w.Code != 400 {
			t.Fatal("ambiguous input accepted", w.Code)
		}
	}
	stale := f.identity(nil)
	stale.MCPRevision++
	if w := do(path+"status", stale, oauthFixtureOrigin); w.Code != 409 {
		t.Fatal("stale revision accepted")
	}
	_, callback := f.start(t, nil)
	w := do("mcp/oauth/callback", callback, oauthFixtureOrigin)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("callback failed", w.Code)
	}
	for _, secret := range []string{callback.State, callback.Code, "fixture-access", "fixture-refresh", "verifier"} {
		if strings.Contains(w.Body.String(), secret) {
			t.Fatal("callback exposed private data")
		}
	}
	var connected mcpOAuthStatus
	if json.Unmarshal(w.Body.Bytes(), &connected) != nil {
		t.Fatal("invalid status")
	}
	if w := do(path+"disconnect", f.identity(&connected), oauthFixtureOrigin); w.Code != 200 {
		t.Fatal("disconnect failed", w.Code)
	}
	if w := do("mcp/oauth/callback", callback, oauthFixtureOrigin); w.Code != 409 || len(f.calls) != 1 {
		t.Fatal("callback revived disconnected account")
	}
}

func TestMCPOAuthMCPTransportBindsAndDropsCredentials(t *testing.T) {
	for _, mode := range []string{"expired", "configuration", "pending", "unknown", "storage", "disconnect", "unauthorized"} {
		t.Run(mode, func(t *testing.T) {
			f := newOAuthFlowFixture(t)
			_, callback := f.start(t, nil)
			connected, err := f.s.mcpOAuthFinish(context.Background(), oauthFixtureOrigin, callback)
			if err != nil {
				t.Fatal(err)
			}
			requests, unauthorized := 0, false
			f.s.oauth.http = func() *http.Client {
				return &http.Client{Transport: mcpDiscoveryTransport(func(r *http.Request) (*http.Response, error) {
					requests++
					if r.URL.String() != mcpResource || r.Header.Get("Authorization") != "Bearer fixture-access" || r.Header.Get("Cookie") != "" {
						t.Error("wrong MCP credential binding")
					}
					if unauthorized {
						return &http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}}, nil
					}
					var message struct {
						ID     json.RawMessage `json:"id"`
						Method string          `json:"method"`
					}
					json.NewDecoder(r.Body).Decode(&message)
					body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": message.ID, "result": map[string]any{"protocolVersion": "2025-06-18", "tools": []any{}}})
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body))), Header: http.Header{"Content-Type": []string{"application/json"}}}, nil
				})}
			}
			srv, _ := f.s.mcpServerOf(f.bot, "fixture")
			c, err := f.s.mcpConnect(f.s.ctx, f.bot, srv)
			if err != nil {
				t.Fatal("OAuth MCP connection failed", err)
			}
			count := requests
			id := mcpOAuthRecordID(f.s.workspaceID, f.bot, srv.Name)
			switch mode {
			case "expired":
				f.now = f.now.Add(2 * time.Hour)
			case "configuration":
				be := f.s.ext.bot(f.bot)
				be.MCPRevision++
				f.s.ext.setBot(f.bot, be)
			case "pending", "unknown", "storage":
				record, _ := f.s.oauth.read(id)
				record.Phase = map[string]string{"pending": "pending", "unknown": "refreshing", "storage": "unsupported"}[mode]
				f.s.oauth.save(id, record)
			case "disconnect":
				if _, err := f.s.mcpOAuthDisconnect(f.s.ctx, f.bot, oauthFixtureOrigin, f.identity(connected)); err != nil {
					t.Fatal(err)
				}
				if !c.ended() {
					t.Fatal("disconnect kept old connection")
				}
				return
			case "unauthorized":
				unauthorized = true
				if _, err := f.s.mcpTools(f.s.ctx, f.bot, srv); err == nil {
					t.Fatal("unauthorized tools accepted")
				}
				if _, err := f.s.mcpTools(f.s.ctx, f.bot, srv); err == nil || requests != count+1 || len(f.calls) != 1 {
					t.Fatal("401 replayed or refreshed automatically")
				}
				return
			}
			if _, err := f.s.mcpConnect(f.s.ctx, f.bot, srv); err == nil || requests != count || !c.ended() {
				t.Fatal("stale credential used or connection survived")
			}
		})
	}
}

func TestMCPOAuthDenialCancellationAndStartSaveFailure(t *testing.T) {
	f := newOAuthFlowFixture(t)
	_, callback := f.start(t, nil)
	callback.Code = ""
	callback.Error = "access_denied"
	denied, err := f.s.mcpOAuthFinish(f.s.ctx, oauthFixtureOrigin, callback)
	if err != nil || denied.Phase != "denied" || len(f.calls) != 0 {
		t.Fatal("denial failed", err)
	}
	if _, err = f.s.mcpOAuthFinish(f.s.ctx, oauthFixtureOrigin, callback); err != nil {
		t.Fatal("denial receipt failed")
	}
	callback.Error = "different"
	if _, err = f.s.mcpOAuthFinish(f.s.ctx, oauthFixtureOrigin, callback); !errors.Is(err, errMCPOAuthChanged) {
		t.Fatal("changed callback accepted")
	}
	f.store.failAt = f.store.writes + 1
	in := mcpOAuthStart{mcpOAuthIdentity: f.identity(denied), Issuer: mcpIssuer, ClientID: "fixture-client", Scopes: []string{"files:read"}}
	if out, err := f.s.mcpOAuthStart(f.s.ctx, f.bot, oauthFixtureOrigin, in); out != nil || !errors.Is(err, errMCPOAuthStorage) {
		t.Fatal("start acknowledged failed persistence")
	}
	status, err := f.s.mcpOAuthStatus(f.s.ctx, f.bot, oauthFixtureOrigin, f.identity(nil))
	if err != nil || status.Phase != "denied" || status.Generation != denied.Generation {
		t.Fatal("failed start changed existing record")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	writes := f.store.writes
	if _, err := f.s.mcpOAuthStart(ctx, f.bot, oauthFixtureOrigin, in); err == nil || f.store.writes != writes {
		t.Fatal("canceled request changed state")
	}
	f.stop()
	f.stop = func() {}
	if _, err := f.s.mcpOAuthStart(context.Background(), f.bot, oauthFixtureOrigin, in); err == nil || f.store.writes != writes {
		t.Fatal("stopped service changed state")
	}
}

func TestMCPOAuthStartRequiresFreshScopePKCEAndConfiguration(t *testing.T) {
	for _, mode := range []string{"scopes", "pkce", "manual_header", "stale_generation", "canceled_discovery"} {
		t.Run(mode, func(t *testing.T) {
			f := newOAuthFlowFixture(t)
			input := mcpOAuthStart{mcpOAuthIdentity: f.identity(nil), Issuer: mcpIssuer, ClientID: "fixture-client", Scopes: []string{"files:read"}}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch mode {
			case "scopes":
				input.Scopes = []string{"files:write"}
			case "pkce":
				for key, body := range f.docs {
					f.docs[key] = strings.ReplaceAll(body, `["S256"]`, `["plain"]`)
				}
			case "manual_header":
				be := f.s.ext.bot(f.bot)
				be.MCP[0].Header = "Authorization"
				be.MCP[0].Secret = "fixture"
				f.s.ext.setBot(f.bot, be)
			case "stale_generation":
				input.Generation = "stale"
			case "canceled_discovery":
				old := f.s.oauth.http
				f.s.oauth.http = func() *http.Client {
					return &http.Client{Transport: mcpDiscoveryTransport(func(r *http.Request) (*http.Response, error) {
						response, err := old().Transport.RoundTrip(r)
						if strings.Contains(r.URL.Path, "oauth-authorization-server") {
							cancel()
						}
						return response, err
					})}
				}
			}
			if out, err := f.s.mcpOAuthStart(ctx, f.bot, oauthFixtureOrigin, input); err == nil || out != nil || f.store.writes != 0 || len(f.calls) != 0 {
				t.Fatal("invalid start persisted or dispatched")
			}
		})
	}
}
