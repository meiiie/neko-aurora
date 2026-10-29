import { expect, test } from "bun:test";
import { spawn } from "node:child_process";
import { createInterface } from "node:readline";
import { fileURLToPath } from "node:url";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { McpOAuthCallback } from "../src/adapters/mcp-oauth-callback.ts";
import { NekoOAuthProvider } from "../src/adapters/mcp-oauth.ts";

const resource = "https://mcp.example.test/mcp", issuer = "https://auth.example.test";
function provider() {
  const p = new NekoOAuthProvider("shared-fixture", resource, new McpOAuthCallback());
  p.saveDiscoveryState({ authorizationServerUrl: issuer, resourceMetadata: { resource, authorization_servers: [issuer] },
    authorizationServerMetadata: { issuer, authorization_endpoint: issuer + "/authorize", token_endpoint: issuer + "/token", response_types_supported: ["code"], code_challenge_methods_supported: ["S256"] } });
  return p;
}
function claim(p: NekoOAuthProvider, kind: "refresh_token" | "authorization_code" = "refresh_token", token = "fixture-refresh") {
  p.beforeTokenRequest(issuer + "/token", { method: "POST", body: new URLSearchParams({ grant_type: kind, resource, [kind === "refresh_token" ? "refresh_token" : "code"]: token }) });
}
async function isolated(fn: () => void | Promise<void>) {
  const dir = mkdtempSync(join(tmpdir(), "neko-oauth-exclusion-"));
  const old = { HOME: process.env.HOME, USERPROFILE: process.env.USERPROFILE };
  process.env.HOME = dir; process.env.USERPROFILE = dir;
  try { await fn(); } finally {
    for (const key of ["HOME", "USERPROFILE"] as const) if (old[key] === undefined) delete process.env[key]; else process.env[key] = old[key];
    rmSync(dir, { recursive: true, force: true });
  }
}
test("a stale refresh reader cannot claim an already consuming token", () => isolated(() => {
  const a = provider(), b = provider();
  a.saveTokens({ access_token: "fixture-access", refresh_token: "fixture-refresh", token_type: "Bearer" });
  expect(a.tokens()?.refresh_token).toBe("fixture-refresh"); expect(b.tokens()?.refresh_token).toBe("fixture-refresh");
  claim(a); expect(() => claim(b)).toThrow(/reconnect/); expect(b.tokens()).toBeUndefined();
}));
test("a stale refresh reader cannot consume a newly rotated token", () => isolated(() => {
  const a = provider(), b = provider(); a.saveTokens({ access_token: "fixture-access", refresh_token: "fixture-refresh", token_type: "Bearer" });
  claim(a); a.saveTokens({ access_token: "new-access", refresh_token: "new-refresh", token_type: "Bearer" });
  expect(() => claim(b)).toThrow(/reconnect/); expect(b.tokens()?.refresh_token).toBe("new-refresh");
}));
test("late token success cannot overwrite a newer authorization attempt", () => isolated(() => {
  const a = provider(), b = provider(); a.saveTokens({ access_token: "fixture-access", refresh_token: "fixture-refresh", token_type: "Bearer" });
  claim(a); claim(b, "authorization_code", "fresh-fixture-code");
  expect(() => a.saveTokens({ access_token: "late-access", token_type: "Bearer" })).toThrow(/reconnect/);
  b.saveTokens({ access_token: "fresh-access", token_type: "Bearer" }); expect(provider().tokens()?.access_token).toBe("fresh-access");
}));
test("a dispatched refresh cannot republish credentials removed by another owner", () => isolated(() => {
  const a = provider(), b = provider(); a.saveTokens({ access_token: "fixture-access", refresh_token: "fixture-refresh", token_type: "Bearer" });
  claim(a); b.invalidateCredentials("tokens");
  expect(() => a.saveTokens({ access_token: "late-access", token_type: "Bearer" })).toThrow(/reconnect/); expect(provider().tokens()).toBeUndefined();
}));

test("two actual processes with the same cached refresh token admit only one claimant", () => isolated(async () => {
  for (let round = 0; round < 4; round++) {
    provider().saveTokens({ access_token: "fixture-access", refresh_token: "fixture-refresh", token_type: "Bearer" });
    const workers = Array.from({ length: 2 }, () => {
      const child = spawn(process.execPath, ["--no-env-file", fileURLToPath(new URL("./fixtures/oauth-exclusion-worker.ts", import.meta.url)), "claim"], { windowsHide: true, stdio: ["pipe", "pipe", "pipe"], env: { PATH: process.env.PATH, SystemRoot: process.env.SystemRoot, NODE_ENV: "test", HOME: process.env.HOME, USERPROFILE: process.env.USERPROFILE, PSModuleAnalysisCachePath: "NUL" } });
      const lines = createInterface({ input: child.stdout });
      const output: string[] = [];
      const ready = new Promise<void>((resolve, reject) => { lines.on("line", line => { output.push(line);if (line === "ready") resolve(); });child.once("error", reject);child.once("exit", () => reject(new Error("worker exited before readiness"))); });
      const exit = new Promise<number | null>(resolve => child.once("exit", resolve));
      return { child, ready, exit, output };
    });
    try {
      await Promise.all(workers.map(w => w.ready));workers.forEach(w => w.child.stdin.end("go\n"));
      expect(await Promise.all(workers.map(w => w.exit))).toEqual([0, 0]);
      expect(workers.map(w => w.output[1]).sort()).toEqual(["claimed", "refused"]);expect(provider().tokens()).toBeUndefined();
    } finally { workers.forEach(w => w.child.kill());await Promise.all(workers.map(w => w.exit)); }
  }
}), 15000);
