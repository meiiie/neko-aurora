package aurora

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"

	"nekoaurora.local/native/internal/api"
)

func TestMCPOAuthBrowserLocalCookieFlow(t *testing.T) {
	script := os.Getenv("AURORA_TEST_MCP_BROWSER")
	if script == "" {
		t.Skip("opt-in local Chrome fixture")
	}
	f := newOAuthFlowFixture(t)
	server := httptest.NewUnstartedServer(nil)
	base := "http://" + server.Listener.Addr().String()
	handler, err := api.New(f.s.L, server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	handler.EnableAurora(f.s)
	server.Config.Handler = handler
	f.s.oauth.origins = []string{base}
	transport := f.s.oauth.http
	f.s.oauth.http = func() *http.Client {
		return &http.Client{Transport: mcpDiscoveryTransport(func(r *http.Request) (*http.Response, error) {
			if r.URL.String() != mcpResource {
				return transport().Transport.RoundTrip(r)
			}
			if r.Header.Get("Authorization") == "" {
				t.Error("MCP resource called without OAuth")
			}
			var message struct {
				ID json.RawMessage `json:"id"`
			}
			json.NewDecoder(r.Body).Decode(&message)
			payload, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": message.ID, "result": map[string]any{"protocolVersion": "2025-06-18", "tools": []any{}}})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(payload))), Header: http.Header{"Content-Type": []string{"application/json"}}}, nil
		})}
	}
	old := mcpDiscoveryHTTP
	mcpDiscoveryHTTP = f.s.oauth.http
	defer func() { mcpDiscoveryHTTP = old }()
	srv, _ := f.s.mcpServerOf(f.bot, "fixture")
	f.s.mcpState().failures[mcpKey(f.bot, srv)] = &mcpFailure{State: "reconnect_required"}
	server.Start()
	defer server.Close()
	command := exec.CommandContext(f.s.ctx, "python", "-u", script, base, f.bot)
	output, err := command.CombinedOutput()
	t.Log(string(output))
	if err != nil {
		t.Fatal("browser fixture failed", err)
	}
	if len(f.calls) != 2 {
		t.Fatal("unexpected token request count", len(f.calls))
	}
}
