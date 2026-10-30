package aurora

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nekoaurora.local/native/internal/privatefs"
)

func TestCoreActualCLIChatGPTWithoutProfileBridge(t *testing.T) {
	bun := findCoreBun()
	if bun == "" {
		t.Skip("Bun unavailable; real Core CLI integration not exercised")
	}
	entry, err := filepath.Abs("../../../../core/bin/neko.ts")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	home, bin := filepath.Join(root, "home"), filepath.Join(root, "bin")
	for _, dir := range []string{home, bin, filepath.Join(home, ".neko-core")} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := privatefs.ProtectDirectory(home); err != nil {
		t.Fatal(err)
	}
	entryJSON, _ := json.Marshal(entry)
	setupJSON, _ := json.Marshal(filepath.Join(filepath.Dir(entry), "aurora-setup.ts"))
	bootstrap := `import { pathToFileURL } from "node:url";
import { writeFileSync } from "node:fs";
import { join } from "node:path";
globalThis.fetch = async (input, init) => {
  const url = String(input);
  if (url.startsWith("https://chatgpt.com/backend-api/codex/models")) return Response.json({models: [{slug:"gpt-5.5",display_name:"Fixture direct model",visibility:"list"}]});
  if (url !== "https://chatgpt.com/backend-api/codex/responses") throw new Error("Unexpected fixture network destination");
  const body = JSON.parse(String(init?.body));
  if (body.model !== "gpt-5.5" || (body.tools?.length ?? 0) !== 0 || new Headers(init?.headers).get("authorization") !== "Bearer fixture-chatgpt-access") throw new Error("Wrong fixture route or credentials");
  writeFileSync(join(process.env.HOME, "request-reached.json"), JSON.stringify({model: body.model, tools: body.tools?.length ?? 0}));
  const events = [{type: "response.output_text.delta", delta: "Aurora CLI fixture reply."}, {type: "response.completed", response: {output: [], usage: {input_tokens: 4, output_tokens: 5}}}];
  return new Response(events.map(x => "data: " + JSON.stringify(x) + "\n\n").join("") + "data: [DONE]\n\n", {headers: {"content-type": "text/event-stream"}});
};
await import(pathToFileURL(` + string(entryJSON) + `).href);
`
	fixtureEntry := filepath.Join(bin, "neko-fixture.ts")
	for name, data := range map[string][]byte{
		fixtureEntry: []byte(bootstrap), filepath.Join(root, "bunfig.neko.toml"): {},
		filepath.Join(bin, "aurora-setup.ts"): []byte(strings.Replace(bootstrap, string(entryJSON), string(setupJSON), 1)),
	} {
		if err := os.WriteFile(name, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	creds, _ := json.Marshal(map[string]any{"accessToken": "fixture-chatgpt-access", "refreshToken": "fixture-chatgpt-refresh", "expiresAt": time.Now().Add(time.Hour).UnixMilli()})
	if err := os.WriteFile(filepath.Join(home, ".neko-core", "chatgpt-auth.json"), creds, 0600); err != nil {
		t.Fatal(err)
	}
	// Ambient bridge/auth controls must not enter the ordinary CLI subprocess.
	t.Setenv("NEKO_CREDENTIAL_HOME", "ambient-fixture-home")
	t.Setenv("OPENAI_API_KEY", "ambient-fixture-api-key")
	c := &Core{Bun: bun, Entry: fixtureEntry, Home: home, Model: "gpt-5.5", Profile: "chatgpt", Quick: true, SignedIn: true}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	var streamed strings.Builder
	reply, err := c.Stream(ctx, []ChatMsg{{Role: "user", Content: "Fixture request only."}}, func(s string) { streamed.WriteString(s) })
	if err != nil || strings.TrimSpace(reply) != "Aurora CLI fixture reply." || reply != streamed.String() {
		t.Fatalf("real CLI reply=%q streamed=%q error=%v", reply, streamed.String(), err)
	}
	if _, err := os.Stat(filepath.Join(home, "request-reached.json")); err != nil {
		t.Fatal("real Core provider was not reached", err)
	}
	c.Model = "retired-fixture"
	_, err = c.Stream(ctx, []ChatMsg{{Role: "user", Content: "Must not reach completion."}}, func(string) { t.Fatal("unavailable model emitted output") })
	if err == nil || err.Error() != "core_model_unavailable" {
		t.Fatalf("retired model was not blocked: %v", err)
	}
}

func TestCoreExitErrorsAreBoundedAndReadable(t *testing.T) {
	for _, tc := range []struct{ stderr, code string }{
		{"Credential home requires the trusted profile bridge entrypoint", "core_configuration_invalid"},
		{"ChatGPT is not signed in. Run neko login chatgpt.", "core_auth_required"},
		{"ChatGPT token refresh failed (HTTP 401). fixture-private-response", "core_auth_required"},
		{"HTTP 500 fixture-private-token-and-response", "core_failed"},
		{"ChatGPT Codex HTTP 404: fixture-private", "core_model_unavailable"},
		{"ChatGPT Codex HTTP 429: fixture-private", "core_rate_limited"},
		{"ChatGPT completion failed: fixture-private", "core_connection_failed"},
	} {
		if got := coreExitError(tc.stderr).Error(); got != tc.code {
			t.Fatalf("unexpected public code %q", got)
		}
		if strings.Contains(failureText(tc.code), tc.code) || strings.Contains(failureText(tc.code), "fixture-private") {
			t.Fatal("internal diagnostic exposed as chat copy")
		}
	}
}
