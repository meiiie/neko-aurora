import { McpOAuthCallback } from "../../src/adapters/mcp-oauth-callback.ts";
import { NekoOAuthProvider } from "../../src/adapters/mcp-oauth.ts";
import { withOAuthRecordLock } from "../../src/adapters/mcp-oauth-lock.ts";

const resource = "https://mcp.example.test/mcp", issuer = "https://auth.example.test";
if (process.argv[2] === "lock") {
  withOAuthRecordLock(process.argv[3]!, () => {
    console.log("held");
    Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, 30_000);
  });
} else {
  const p = new NekoOAuthProvider("shared-fixture", resource, new McpOAuthCallback());
  p.saveDiscoveryState({ authorizationServerUrl: issuer, resourceMetadata: { resource, authorization_servers: [issuer] },
    authorizationServerMetadata: { issuer, authorization_endpoint: issuer + "/authorize", token_endpoint: issuer + "/token", response_types_supported: ["code"], code_challenge_methods_supported: ["S256"] } });
  const refresh = p.tokens()?.refresh_token;
  if (!refresh) throw new Error("Fixture token missing");
  console.log("ready");
  process.stdin.once("data", () => {
    try {
      p.beforeTokenRequest(issuer + "/token", { method: "POST", body: new URLSearchParams({ grant_type: "refresh_token", resource, refresh_token: refresh }) });
      console.log("claimed");
    } catch { console.log("refused"); }
    process.stdin.destroy();
  });
}
