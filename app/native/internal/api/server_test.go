package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"nekoaurora.local/native/internal/ledger"
)

func setup(t *testing.T) (*httptest.Server, *http.Client) {
	t.Helper()
	store, err := ledger.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(nil)
	handler, err := New(store, server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	server.Config.Handler = handler
	server.Start()
	t.Cleanup(func() { server.Close(); store.Close() })
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 3 * time.Second}
	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	return server, client
}
func post(t *testing.T, client *http.Client, base, path, body, origin string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("POST", base+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return response
}
func TestLocalSessionOriginOwnerAndPayloadBoundary(t *testing.T) {
	server, client := setup(t)
	body := `{"botId":"bot-meiiie","clientNonce":"test-message-nonce","text":"hello","lane":"foreground"}`
	response := post(t, http.DefaultClient, server.URL, "/v1/messages", body, "")
	response.Body.Close()
	if response.StatusCode != 401 {
		t.Fatal("session not required")
	}
	response = post(t, client, server.URL, "/v1/messages", body, "https://untrusted.example")
	response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatal("cross-origin write allowed")
	}
	response = post(t, client, server.URL, "/v1/messages", strings.TrimSuffix(body, "}")+`,"ownerUserId":"forged"}`, "")
	response.Body.Close()
	if response.StatusCode != 400 {
		t.Fatal("client-supplied owner allowed")
	}
	response = post(t, client, server.URL, "/v1/messages", body, "")
	response.Body.Close()
	if response.StatusCode != 201 {
		t.Fatal("valid request rejected", response.StatusCode)
	}
	response = post(t, client, server.URL, "/v1/messages", body, "")
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("idempotent resend rejected")
	}
	response = post(t, client, server.URL, "/v1/messages", strings.Replace(body, "hello", "changed", 1), "")
	response.Body.Close()
	if response.StatusCode != 409 {
		t.Fatal("different-content nonce allowed")
	}
	response = post(t, client, server.URL, "/v1/messages", body+" {}", "")
	response.Body.Close()
	if response.StatusCode != 400 {
		t.Fatal("trailing frame allowed")
	}
}
func TestHostRebindingAndFrameSizeRejected(t *testing.T) {
	server, client := setup(t)
	req, _ := http.NewRequest("GET", server.URL+"/v1/snapshot", nil)
	req.Host = "evil.example"
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatal("host rebinding allowed")
	}
	response = post(t, client, server.URL, "/v1/messages", strings.Repeat(" ", 65537), "")
	response.Body.Close()
	if response.StatusCode != 400 {
		t.Fatal("oversize frame allowed")
	}
}
func TestIndependentClientsReplaySameCommittedSequence(t *testing.T) {
	server, first := setup(t)
	jar, _ := cookiejar.New(nil)
	second := &http.Client{Jar: jar, Timeout: 3 * time.Second}
	r, err := second.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	for _, lane := range []string{"background", "foreground"} {
		b, _ := json.Marshal(ledger.Input{BotID: "bot-meiiie", ClientNonce: "client-" + lane, Text: lane, Lane: lane})
		r = post(t, first, server.URL, "/v1/messages", string(b), "")
		r.Body.Close()
		if r.StatusCode != 201 {
			t.Fatal(r.StatusCode)
		}
	}
	var snapshots [2][]byte
	for i, client := range []*http.Client{first, second} {
		r, err = client.Get(server.URL + "/v1/snapshot")
		if err != nil {
			t.Fatal(err)
		}
		var b bytes.Buffer
		b.ReadFrom(r.Body)
		r.Body.Close()
		snapshots[i] = b.Bytes()
	}
	if !bytes.Equal(snapshots[0], snapshots[1]) {
		t.Fatal("clients do not converge")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/v1/events?after=0", nil)
	req.Header.Set("Last-Event-ID", "1")
	r, err = second.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	line, err := bufio.NewReader(r.Body).ReadString('\n')
	if err != nil || line != "id: 2\n" {
		t.Fatal("Last-Event-ID did not replay next event", line, err)
	}
	endpoint, _ := url.Parse(server.URL)
	for _, cookie := range second.Jar.Cookies(endpoint) {
		if cookie.Name == "neko_local" && cookie.Value == "" {
			t.Fatal("empty session")
		}
	}
}
