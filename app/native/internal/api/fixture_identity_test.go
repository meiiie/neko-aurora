package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"nekoaurora.local/native/internal/identity"
	"nekoaurora.local/native/internal/ledger"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fixtureHTTP struct {
	server    *httptest.Server
	store     *ledger.Store
	authority *identity.Authority
	handler   *Server
	clients   map[string]*http.Client
	status    map[string]identity.SessionStatus
	channel   ledger.Channel
}

func fixturePost(t *testing.T, h *fixtureHTTP, who, path, body string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("POST", h.server.URL+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", h.server.URL)
	req.Header.Set("X-Neko-Session-Binding", h.status[who].SessionBinding)
	response, e := h.clients[who].Do(req)
	if e != nil {
		t.Fatal(e)
	}
	return response
}
func TestFixtureHTTPStaleTabSessionBindingCannotReattributeHumanIntent(t *testing.T) {
	h := newFixtureHTTP(t)
	before := h.store.Snapshot().Sequence
	body := fixtureBody(h.channel, "http-stale-tab-owner-intent", "Original owner intent from older tab")
	for _, expected := range []string{"", h.status["owner"].SessionBinding, h.status["bob2"].SessionBinding} {
		req, _ := http.NewRequest("POST", h.server.URL+"/v1/messages", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Neko-Session-Binding", expected)
		response, e := h.clients["bob"].Do(req)
		if e != nil {
			t.Fatal(e)
		}
		response.Body.Close()
		if response.StatusCode != 409 || h.store.Snapshot().Sequence != before {
			t.Fatal("old tab request reassigned to cookie actor", response.StatusCode)
		}
		if response.Header.Get("X-Neko-Session-Binding") != h.status["bob"].SessionBinding {
			t.Fatal("response omitted resolved binding")
		}
	}
}

func newFixtureHTTP(t *testing.T) *fixtureHTTP {
	t.Helper()
	s, e := ledger.Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	private, _, e := s.Accept(ledger.Input{BotID: "bot-meiiie", ClientNonce: "http-private-owner", Text: "HTTP_PRIVATE_CANARY_FIXTURE", Lane: "foreground"})
	if e != nil || private.RequesterPin != nil {
		t.Fatal(e)
	}
	_, _, e = s.SetBotProfile("bot-meiiie", ledger.BotProfileInput{Instructions: "HTTP_PROFILE_CANARY_FIXTURE", IncludeHistory: true, ClientNonce: "http-private-profile"})
	if e != nil {
		t.Fatal(e)
	}
	a, e := identity.NewFixture(identity.Config{Version: 1, FixtureOnly: true, AuthorityID: "fixture-http", SpaceID: "local-space", SessionTTLSeconds: 120, Actors: []identity.ActorDevice{{ActorID: "local-owner", DeviceID: "owner-device", DeviceRevision: 1}, {ActorID: "fixture-bob", DeviceID: "bob-device", DeviceRevision: 1}, {ActorID: "fixture-bob", DeviceID: "bob-device-two", DeviceRevision: 1}, {ActorID: "fixture-reader", DeviceID: "reader-device", DeviceRevision: 1}, {ActorID: "fixture-eve", DeviceID: "eve-device", DeviceRevision: 1}}})
	if e != nil {
		t.Fatal(e)
	}
	server := httptest.NewUnstartedServer(nil)
	handler, e := New(s, server.Listener.Addr().String())
	if e != nil {
		t.Fatal(e)
	}
	handler.EnableFixtureIdentity(a)
	server.Config.Handler = handler
	server.Start()
	t.Cleanup(func() { server.Close(); s.Close() })
	h := &fixtureHTTP{server: server, store: s, authority: a, handler: handler, clients: map[string]*http.Client{}, status: map[string]identity.SessionStatus{}}
	for _, item := range []struct{ actor, device, key string }{{"local-owner", "owner-device", "owner"}, {"fixture-bob", "bob-device", "bob"}, {"fixture-bob", "bob-device-two", "bob2"}, {"fixture-reader", "reader-device", "reader"}, {"fixture-eve", "eve-device", "eve"}} {
		ticket, e := a.BootstrapTicket(item.actor, item.device)
		if e != nil {
			t.Fatal(e)
		}
		jar, _ := cookiejar.New(nil)
		client := &http.Client{Jar: jar, Timeout: 3 * time.Second}
		h.clients[item.key] = client
		response := post(t, client, server.URL, "/v1/fixture-sessions", `{"ticket":"`+ticket+`"}`, server.URL)
		if response.StatusCode != 200 {
			response.Body.Close()
			t.Fatalf("bootstrap %s status %d", item.key, response.StatusCode)
		}
		var status identity.SessionStatus
		if json.NewDecoder(response.Body).Decode(&status) != nil {
			t.Fatal("bootstrap metadata decode")
		}
		response.Body.Close()
		h.status[item.key] = status
		if status.ActorID != item.actor || status.AuthorityID != a.AuthorityID() || status.CacheNamespace == "" || status.SessionBinding == "" {
			t.Fatal("wrong session metadata")
		}
		for _, cookie := range response.Cookies() {
			if cookie.Name == "neko_fixture" && (!cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode) {
				t.Fatal("fixture cookie unsafe")
			}
		}
	}
	body := `{"name":"Fixture shared","botIds":["bot-meiiie","bot-davin"],"clientNonce":"http-fixture-channel","expectedRevision":0,"humanAcl":{"version":1,"authorityId":"fixture-http","members":[{"actorId":"fixture-reader","role":"reader"},{"actorId":"fixture-bob","role":"writer"}]}}`
	response := fixturePost(t, h, "owner", "/v1/channels", body)
	if response.StatusCode != 200 {
		t.Fatalf("channel status %d", response.StatusCode)
	}
	var result struct {
		Channel ledger.Channel `json:"channel"`
	}
	if json.NewDecoder(response.Body).Decode(&result) != nil {
		t.Fatal("channel decode")
	}
	response.Body.Close()
	h.channel = result.Channel
	return h
}
func fixtureBody(c ledger.Channel, nonce, text string) string {
	return fmt.Sprintf(`{"botId":"bot-meiiie","clientNonce":%q,"text":%q,"lane":"foreground","channelId":%q,"expectedChannelRevision":%d}`, nonce, text, c.ID, c.Revision)
}
func TestFixtureHTTPBootstrapSpoofACLScopedRoutesAndRotation(t *testing.T) {
	h := newFixtureHTTP(t)
	response, e := http.Get(h.server.URL)
	if e != nil {
		t.Fatal(e)
	}
	for _, c := range response.Cookies() {
		if c.Name == "neko_local" || c.Name == "neko_fixture" {
			t.Fatal("root silently issued identity")
		}
	}
	response.Body.Close()
	req, _ := http.NewRequest("GET", h.server.URL+"/v1/snapshot", nil)
	req.AddCookie(&http.Cookie{Name: "neko_local", Value: h.handler.session})
	response, e = http.DefaultClient.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	response.Body.Close()
	if response.StatusCode != 401 {
		t.Fatal("legacy fallback in fixturemode")
	}
	body := fixtureBody(h.channel, "http-shared-human", "PUBLIC_HTTP_HUMAN_FIXTURE")
	for _, suffix := range []string{`,"actorId":"local-owner"}`, `,"requesterPin":{"actorId":"local-owner"}}`, `,"botId":"bot-davin"}`} {
		response = fixturePost(t, h, "bob", "/v1/messages", strings.TrimSuffix(body, "}")+suffix)
		response.Body.Close()
		if response.StatusCode != 400 {
			t.Fatal("identity/duplicate-field spoof accepted", response.StatusCode)
		}
	}
	response = fixturePost(t, h, "bob", "/v1/messages", body)
	if response.StatusCode != 201 {
		t.Fatal("writer send", response.StatusCode)
	}
	var accepted struct {
		Work      ledger.Work `json:"work"`
		Duplicate bool        `json:"duplicate"`
	}
	if json.NewDecoder(response.Body).Decode(&accepted) != nil {
		t.Fatal("accept decode")
	}
	response.Body.Close()
	if accepted.Work.RequesterPin == nil || accepted.Work.RequesterPin.ActorID != "fixture-bob" || accepted.Work.Publication.ActorID != "fixture-bob" || accepted.Work.RuntimeContext != nil || accepted.Work.Context != nil || accepted.Work.BotProfile != nil || accepted.Work.Core != nil {
		t.Fatal("member send projection wrong")
	}
	response = fixturePost(t, h, "bob2", "/v1/messages", body)
	if response.StatusCode != 200 {
		t.Fatal("same actor second device retry", response.StatusCode)
	}
	response.Body.Close()
	for _, who := range []string{"reader", "eve"} {
		response = fixturePost(t, h, who, "/v1/messages", body)
		response.Body.Close()
		if response.StatusCode != 403 {
			t.Fatalf("%s sent", who)
		}
	}
	for _, path := range []string{"/v1/plugin-catalog", "/v1/account-receipts?clientNonce=fixture-read", "/v1/conversations", "/v1/tasks/any/sources", "/v1/artifacts/any", "/v1/adapters/catalog", "/v1/adapters/sessions/any/history"} {
		response, e = h.clients["bob"].Get(h.server.URL + path)
		if e != nil {
			t.Fatal(e)
		}
		response.Body.Close()
		if response.StatusCode != 403 {
			t.Fatal("member owner route not denied", path, response.StatusCode)
		}
	}
	for _, who := range []string{"owner", "bob"} {
		for _, path := range []string{"/v1/collaborations", "/v1/task-controls", "/v1/adapters/sessions", "/v1/recover", "/v1/cancel"} {
			response = fixturePost(t, h, who, path, `{}`)
			response.Body.Close()
			if response.StatusCode != 403 {
				t.Fatal("unpinned executable/control route allowed", who, path)
			}
		}
	}
	response, e = h.clients["reader"].Get(h.server.URL + "/v1/snapshot")
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := io.ReadAll(response.Body)
	response.Body.Close()
	for _, canary := range []string{"HTTP_PRIVATE_CANARY_FIXTURE", "HTTP_PROFILE_CANARY_FIXTURE", "contextPin", "runtimeContext", "coreSha256", "accountPin"} {
		if strings.Contains(string(raw), canary) {
			t.Fatal("snapshot private field leaked", canary)
		}
	}
	if !strings.Contains(string(raw), `"selfRole":"reader"`) {
		t.Fatal("reader role unavailable")
	}
	response = fixturePost(t, h, "owner", "/v1/fixture-devices/control", `{"deviceId":"bob-device","action":"rotate","expectedRevision":1}`)
	if response.StatusCode != 200 {
		t.Fatal("rotate status", response.StatusCode)
	}
	var rotation identity.DeviceControlResult
	if json.NewDecoder(response.Body).Decode(&rotation) != nil {
		t.Fatal("rotate decode")
	}
	response.Body.Close()
	response, e = h.clients["bob"].Get(h.server.URL + "/v1/session")
	if e != nil {
		t.Fatal(e)
	}
	response.Body.Close()
	if response.StatusCode != 401 {
		t.Fatal("rotated bearer remained valid")
	}
	response = post(t, h.clients["bob"], h.server.URL, "/v1/fixture-sessions", `{"ticket":"`+rotation.Ticket+`"}`, h.server.URL)
	var fresh identity.SessionStatus
	if response.StatusCode != 200 || json.NewDecoder(response.Body).Decode(&fresh) != nil {
		t.Fatal("rotated reconnect")
	}
	response.Body.Close()
	if fresh.CacheNamespace != h.status["bob"].CacheNamespace || fresh.SessionBinding == h.status["bob"].SessionBinding {
		t.Fatal("rotation cache/transport binding")
	}
	h.status["bob"] = fresh
	before := h.store.Snapshot().Sequence
	response = fixturePost(t, h, "bob", "/v1/messages", body)
	if response.StatusCode != 200 {
		t.Fatal("manual same actor rotated retry", response.StatusCode)
	}
	var retried struct {
		Work ledger.Work `json:"work"`
	}
	json.NewDecoder(response.Body).Decode(&retried)
	response.Body.Close()
	if retried.Work.RequestID != accepted.Work.RequestID || retried.Work.RequesterPin.SessionID != accepted.Work.RequesterPin.SessionID || h.store.Snapshot().Sequence != before || retried.Work.Status != "suspended" {
		t.Fatal("retry repinned/resumed execution")
	}
}
func TestFixtureHTTPStreamCursorPrivacyInvalidationAndRevocation(t *testing.T) {
	h := newFixtureHTTP(t)
	body := fixtureBody(h.channel, "stream-public-human", "STREAM_PUBLIC_CANARY_FIXTURE")
	response := fixturePost(t, h, "bob", "/v1/messages", body)
	response.Body.Close()
	if response.StatusCode != 201 {
		t.Fatal("fixture message")
	}
	binding := h.status["bob"].SessionBinding
	for _, item := range []struct {
		path, last string
		code       int
	}{{"/v1/events?after=0", "", 409}, {"/v1/events?after=0&sessionBinding=" + h.status["owner"].SessionBinding, "", 409}, {"/v1/events?after=0&sessionBinding=" + binding, h.status["bob2"].SessionBinding + ":0", 409}, {"/v1/events?after=0&sessionBinding=" + binding, "0", 409}} {
		req, _ := http.NewRequest("GET", h.server.URL+item.path, nil)
		if item.last != "" {
			req.Header.Set("Last-Event-ID", item.last)
		}
		response, e := h.clients["bob"].Do(req)
		if e != nil {
			t.Fatal(e)
		}
		response.Body.Close()
		if response.StatusCode != item.code {
			t.Fatal("unbound stream cursor accepted", response.StatusCode)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", h.server.URL+"/v1/events?after=0&sessionBinding="+binding, nil)
	response, e := h.clients["bob"].Do(req)
	if e != nil {
		t.Fatal(e)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("bound stream rejected")
	}
	reader := bufio.NewReader(response.Body)
	seen := false
	for !seen {
		line, e := reader.ReadString('\n')
		if e != nil {
			t.Fatal("stream before public event", e)
		}
		if strings.Contains(line, "HTTP_PRIVATE_CANARY_FIXTURE") || strings.Contains(line, "HTTP_PROFILE_CANARY_FIXTURE") || strings.Contains(line, "runtimeContext") {
			t.Fatal("stream private content")
		}
		if strings.HasPrefix(line, "id: ") && !strings.HasPrefix(line, "id: "+binding+":") {
			t.Fatal("stream id unbound")
		}
		if strings.Contains(line, "STREAM_PUBLIC_CANARY_FIXTURE") {
			seen = true
		}
	}
	acl := fmt.Sprintf(`{"id":%q,"name":"Fixture shared","botIds":["bot-meiiie","bot-davin"],"expectedRevision":1,"clientNonce":"stream-remove-bob","humanAcl":{"version":1,"authorityId":"fixture-http","members":[{"actorId":"fixture-reader","role":"reader"}]}}`, h.channel.ID)
	updated := fixturePost(t, h, "owner", "/v1/channels", acl)
	updated.Body.Close()
	if updated.StatusCode != 200 {
		t.Fatal("ACL update")
	}
	invalidated := false
	for !invalidated {
		line, e := reader.ReadString('\n')
		if e != nil {
			t.Fatal("stream invalidation missing", e)
		}
		if strings.Contains(line, "scope_invalidated") {
			if strings.Contains(line, h.channel.ID) || strings.Contains(line, "STREAM_PUBLIC_CANARY_FIXTURE") {
				t.Fatal("invalidation disclosed private ID/content")
			}
			invalidated = true
		}
	}
	control := fixturePost(t, h, "owner", "/v1/fixture-devices/control", `{"deviceId":"bob-device","action":"revoke","expectedRevision":1}`)
	control.Body.Close()
	if control.StatusCode != 200 {
		t.Fatal("revoke")
	}
	closed := false
	for !closed {
		_, e := reader.ReadString('\n')
		if e != nil {
			closed = true
			if ctx.Err() != nil {
				t.Fatal("stream closed only by client")
			}
		}
	}
}
