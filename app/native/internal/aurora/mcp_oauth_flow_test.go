package aurora

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"nekoaurora.local/native/internal/vault"
)

const oauthFixtureOrigin = "http://127.0.0.1:41789"

type oauthFaultStore struct {
	mcpOAuthStore
	writes, failAt int
}

func (v *oauthFaultStore) SetOAuthRecord(id string, record vault.OAuthRecord) error {
	v.writes++
	if v.writes == v.failAt {
		return errors.New("fixture-save-failed")
	}
	return v.mcpOAuthStore.SetOAuthRecord(id, record)
}

type oauthFlowFixture struct {
	s        *Service
	stop     func()
	dir, bot string
	store    *oauthFaultStore
	docs     map[string]string
	calls    []url.Values
	mode     string
	now      time.Time
}

func newOAuthFlowFixture(t *testing.T) *oauthFlowFixture {
	t.Helper()
	f := &oauthFlowFixture{dir: t.TempDir(), now: time.Unix(1800000000, 0)}
	_, f.docs, _ = mcpDiscoveryFixture(t)
	f.docs["https://auth.example.com/.well-known/oauth-authorization-server/tenant"] = `{"issuer":"` + mcpIssuer + `","authorization_endpoint":"https://auth.example.com/authorize","token_endpoint":"https://auth.example.com/token","response_types_supported":["code"],"code_challenge_methods_supported":["S256"],"authorization_response_iss_parameter_supported":true}`
	f.reopen(t)
	if err := f.s.ext.setBot(f.bot, botExt{MCPRevision: 1, MCP: []mcpServer{{Name: "fixture", URL: mcpResource}}}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.stop() })
	return f
}

func (f *oauthFlowFixture) reopen(t *testing.T) {
	t.Helper()
	if f.stop != nil {
		f.stop()
	}
	f.s, f.stop = historyService(t, f.dir)
	f.bot = firstBot(f.s)
	f.store = &oauthFaultStore{mcpOAuthStore: f.s.V}
	f.s.oauth = newMCPOAuthManager(f.store, []string{oauthFixtureOrigin}, func() time.Time { return f.now })
	f.s.oauth.http = func() *http.Client {
		return &http.Client{Transport: mcpDiscoveryTransport(func(r *http.Request) (*http.Response, error) {
			if r.Method == "GET" {
				body, ok := f.docs[r.URL.String()]
				code := 200
				if !ok {
					code = 404
				}
				return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}, Request: r}, nil
			}
			if r.URL.String() != "https://auth.example.com/token" || r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" || r.GetBody != nil {
				t.Error("unexpected token request")
			}
			body, _ := io.ReadAll(r.Body)
			params, err := url.ParseQuery(string(body))
			if err != nil {
				t.Error(err)
			}
			f.calls = append(f.calls, params)
			if params.Get("resource") != mcpResource || params.Get("client_id") != "fixture-client" {
				t.Error("token request binding changed")
			}
			if f.mode == "lost" {
				return nil, errors.New("fixture response lost")
			}
			access, refresh := "fixture-access", "fixture-refresh"
			if params.Get("grant_type") == "refresh_token" {
				access, refresh = "fixture-new-access", "fixture-new-refresh"
			}
			payload, _ := json.Marshal(map[string]any{"access_token": access, "refresh_token": refresh, "token_type": "Bearer", "expires_in": 3600, "scope": "files:read"})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(payload))), Header: http.Header{}, Request: r}, nil
		})}
	}
}

func (f *oauthFlowFixture) identity(status *mcpOAuthStatus) mcpOAuthIdentity {
	i := mcpOAuthIdentity{WorkspaceID: f.s.workspaceID, Name: "fixture", MCPRevision: f.s.ext.bot(f.bot).MCPRevision}
	if status != nil {
		i.Generation, i.Revision = status.Generation, status.Revision
	}
	return i
}

func (f *oauthFlowFixture) start(t *testing.T, previous *mcpOAuthStatus) (*mcpOAuthStatus, mcpOAuthCallback) {
	t.Helper()
	status, err := f.s.mcpOAuthStart(context.Background(), f.bot, oauthFixtureOrigin, mcpOAuthStart{mcpOAuthIdentity: f.identity(previous), Issuer: mcpIssuer, ClientID: "fixture-client", Scopes: []string{"files:read"}})
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(status.AuthorizationURL)
	if err != nil || status.Phase != "pending" || u.Query().Get("state") == "" {
		t.Fatal("missing pending authorization", err)
	}
	issuer := mcpIssuer
	return status, mcpOAuthCallback{State: u.Query().Get("state"), Code: "fixture-code", Issuer: &issuer}
}

func TestMCPOAuthFlowConnectRefreshReceiptAndDisconnect(t *testing.T) {
	f := newOAuthFlowFixture(t)
	pending, callback := f.start(t, nil)
	f.reopen(t)
	connected, err := f.s.mcpOAuthFinish(context.Background(), oauthFixtureOrigin, callback)
	if err != nil || connected.Phase != "connected" || !connected.CanRefresh || connected.Generation != pending.Generation {
		t.Fatal("finish failed", err)
	}
	if len(f.calls) != 1 || f.calls[0].Get("grant_type") != "authorization_code" || f.calls[0].Get("redirect_uri") != oauthFixtureOrigin+mcpOAuthCallbackPath || len(f.calls[0].Get("code_verifier")) != 43 {
		t.Fatal("authorization code exchange mismatch")
	}
	f.reopen(t)
	again, err := f.s.mcpOAuthFinish(context.Background(), oauthFixtureOrigin, callback)
	if err != nil || again.Revision != connected.Revision || len(f.calls) != 1 {
		t.Fatal("callback receipt replayed exchange", err)
	}
	refresh := mcpOAuthRefresh{mcpOAuthIdentity: f.identity(connected), Nonce: "fixture-refresh-operation"}
	renewed, err := f.s.mcpOAuthRefresh(context.Background(), f.bot, oauthFixtureOrigin, refresh)
	if err != nil || renewed.Phase != "connected" || len(f.calls) != 2 || f.calls[1].Get("refresh_token") != "fixture-refresh" {
		t.Fatal("refresh failed", err)
	}
	f.reopen(t)
	if _, err := f.s.mcpOAuthRefresh(context.Background(), f.bot, oauthFixtureOrigin, refresh); err != nil || len(f.calls) != 2 {
		t.Fatal("refresh receipt replayed rotation", err)
	}
	disconnected, err := f.s.mcpOAuthDisconnect(context.Background(), f.bot, oauthFixtureOrigin, f.identity(renewed))
	if err != nil || disconnected.Phase != "disconnected" {
		t.Fatal("disconnect failed", err)
	}
	id := mcpOAuthRecordID(f.s.workspaceID, f.bot, "fixture")
	stored, err := f.s.V.GetOAuthRecord(id)
	if err != nil || len(stored.Secrets) != 0 {
		t.Fatal("disconnect retained credential", err)
	}
	if _, err := f.s.mcpOAuthFinish(context.Background(), oauthFixtureOrigin, callback); err == nil || len(f.calls) != 2 {
		t.Fatal("old callback revived disconnected grant")
	}
	if _, err := f.s.mcpOAuthStart(context.Background(), f.bot, oauthFixtureOrigin, mcpOAuthStart{mcpOAuthIdentity: f.identity(nil), Issuer: mcpIssuer, ClientID: "fixture-client", Scopes: []string{"files:read"}}); !errors.Is(err, errMCPOAuthChanged) {
		t.Fatal("disconnected tombstone did not fence stale start")
	}
	f.start(t, disconnected)
}

func TestMCPOAuthFlowCallbackValidation(t *testing.T) {
	for _, mode := range []string{"state", "missing_issuer", "wrong_issuer", "wrong_origin", "mixed_result", "expired", "configuration", "workspace"} {
		t.Run(mode, func(t *testing.T) {
			f := newOAuthFlowFixture(t)
			_, in := f.start(t, nil)
			origin := oauthFixtureOrigin
			switch mode {
			case "state":
				in.State = in.State[:len(in.State)-1] + "!"
			case "missing_issuer":
				in.Issuer = nil
			case "wrong_issuer":
				wrong := mcpIssuer + "/"
				in.Issuer = &wrong
			case "wrong_origin":
				origin = "http://127.0.0.1:41790"
			case "mixed_result":
				in.Error = "access_denied"
			case "expired":
				f.now = f.now.Add(11 * time.Minute)
			case "configuration":
				be := f.s.ext.bot(f.bot)
				be.MCPRevision++
				f.s.ext.setBot(f.bot, be)
			case "workspace":
				f.s.workspaceID = "changed"
			}
			if _, err := f.s.mcpOAuthFinish(context.Background(), origin, in); err == nil {
				t.Fatal("invalid callback accepted")
			}
			if len(f.calls) != 0 {
				t.Fatal("invalid callback reached token endpoint")
			}
		})
	}
}

func TestMCPOAuthFlowExchangeFailuresSurviveRestartWithoutReplay(t *testing.T) {
	for _, mode := range []string{"before_dispatch", "lost_response", "save_result"} {
		t.Run(mode, func(t *testing.T) {
			f := newOAuthFlowFixture(t)
			_, callback := f.start(t, nil)
			if mode == "before_dispatch" {
				f.store.failAt = f.store.writes + 1
			}
			if mode == "save_result" {
				f.store.failAt = f.store.writes + 2
			}
			if mode == "lost_response" {
				f.mode = "lost"
			}
			_, err := f.s.mcpOAuthFinish(context.Background(), oauthFixtureOrigin, callback)
			want := errMCPOAuthUnknown
			if mode == "before_dispatch" {
				want = errMCPOAuthStorage
			}
			if !errors.Is(err, want) {
				t.Fatal("wrong failure classification", err)
			}
			count := len(f.calls)
			if (mode == "before_dispatch" && count != 0) || (mode != "before_dispatch" && count != 1) {
				t.Fatal("wrong dispatch count")
			}
			f.mode = ""
			f.reopen(t)
			_, err = f.s.mcpOAuthFinish(context.Background(), oauthFixtureOrigin, callback)
			if mode == "before_dispatch" {
				if err != nil || len(f.calls) != 1 {
					t.Fatal("safe retry after pre-dispatch save failure", err)
				}
			} else if !errors.Is(err, errMCPOAuthUnknown) || len(f.calls) != count {
				t.Fatal("unknown exchange retried after restart", err)
			}
		})
	}
}

func TestMCPOAuthFlowRefreshFailuresNeverReuseRotatedToken(t *testing.T) {
	for _, mode := range []string{"before_dispatch", "lost_response", "save_result"} {
		t.Run(mode, func(t *testing.T) {
			f := newOAuthFlowFixture(t)
			_, callback := f.start(t, nil)
			connected, err := f.s.mcpOAuthFinish(context.Background(), oauthFixtureOrigin, callback)
			if err != nil {
				t.Fatal(err)
			}
			input := mcpOAuthRefresh{mcpOAuthIdentity: f.identity(connected), Nonce: "fixture-refresh-operation"}
			if mode == "before_dispatch" {
				f.store.failAt = f.store.writes + 1
			}
			if mode == "save_result" {
				f.store.failAt = f.store.writes + 2
			}
			if mode == "lost_response" {
				f.mode = "lost"
			}
			_, err = f.s.mcpOAuthRefresh(context.Background(), f.bot, oauthFixtureOrigin, input)
			want := errMCPOAuthUnknown
			if mode == "before_dispatch" {
				want = errMCPOAuthStorage
			}
			if !errors.Is(err, want) {
				t.Fatal("wrong refresh failure", err)
			}
			count := len(f.calls)
			f.mode = ""
			f.reopen(t)
			_, err = f.s.mcpOAuthRefresh(context.Background(), f.bot, oauthFixtureOrigin, input)
			if mode == "before_dispatch" {
				if err != nil || len(f.calls) != count+1 {
					t.Fatal("safe refresh retry failed", err)
				}
			} else if !errors.Is(err, errMCPOAuthUnknown) || len(f.calls) != count {
				t.Fatal("rotated token retried after restart", err)
			}
		})
	}
}

func TestMCPOAuthFlowDuplicateCallbacksAndFailedDisconnect(t *testing.T) {
	f := newOAuthFlowFixture(t)
	_, callback := f.start(t, nil)
	var wg sync.WaitGroup
	results := make(chan *mcpOAuthStatus, 2)
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := f.s.mcpOAuthFinish(context.Background(), oauthFixtureOrigin, callback)
			results <- r
			errs <- err
		}()
	}
	wg.Wait()
	a, b := <-results, <-results
	if e1, e2 := <-errs, <-errs; e1 != nil || e2 != nil || a.Revision != b.Revision || len(f.calls) != 1 {
		t.Fatal("concurrent callback exchange duplicated", e1, e2)
	}
	f.store.failAt = f.store.writes + 1
	if _, err := f.s.mcpOAuthDisconnect(context.Background(), f.bot, oauthFixtureOrigin, f.identity(a)); !errors.Is(err, errMCPOAuthStorage) {
		t.Fatal("disconnect save failure ignored")
	}
	f.reopen(t)
	status, err := f.s.mcpOAuthStatus(context.Background(), f.bot, oauthFixtureOrigin, f.identity(nil))
	if err != nil || status.Phase != "connected" || !status.CanRefresh {
		t.Fatal("failed disconnect changed persisted grant", err)
	}
}
