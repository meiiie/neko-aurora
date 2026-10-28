import { expect, spyOn, test } from "bun:test";
import { createHash } from "node:crypto";
import { mkdtempSync, mkdirSync, readdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { createServer, request } from "node:http";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { Client } from "@modelcontextprotocol/sdk/client/index.js";
import type { OAuthDiscoveryState } from "@modelcontextprotocol/sdk/client/auth.js";
import type { FetchLike } from "@modelcontextprotocol/sdk/shared/transport.js";
import { McpOAuthCallback } from "../src/adapters/mcp-oauth-callback.ts";
import { connectWithOAuth, NekoOAuthProvider } from "../src/adapters/mcp-oauth.ts";
import { McpHub } from "../src/adapters/mcp.ts";
import { isText } from "../src/shared/wire.ts";

const resource = "https://mcp.example.test/mcp", issuer = "https://auth.example.test";
function discovery(as = issuer, target = resource): OAuthDiscoveryState {
  return {
    authorizationServerUrl: as,
    resourceMetadata: { resource: target, authorization_servers: [as], scopes_supported: ["read"] },
    authorizationServerMetadata: {
      issuer: as, authorization_endpoint: as + "/authorize", token_endpoint: as + "/token", registration_endpoint: as + "/register",
      response_types_supported: ["code"], grant_types_supported: ["authorization_code", "refresh_token"],
      token_endpoint_auth_methods_supported: ["none"], code_challenge_methods_supported: ["S256"],
      authorization_response_iss_parameter_supported: true,
    },
  };
}
async function withHome(fn: (root: string) => Promise<void>): Promise<void> {
  const root = mkdtempSync(join(tmpdir(), "neko-oauth-"));
  const old = { USERPROFILE: process.env.USERPROFILE, HOME: process.env.HOME };
  process.env.USERPROFILE = root; process.env.HOME = root;
  try { await fn(root); } finally {
    for (const key of ["USERPROFILE", "HOME"] as const) {
      if (old[key] === undefined) delete process.env[key]; else process.env[key] = old[key];
    }
    rmSync(root, { recursive: true, force: true });
  }
}
function callbackRequest(redirect: string, params: URLSearchParams, options: { method?: string; path?: string; host?: string } = {}): Promise<{ status: number; body: string; cache: string | undefined }> {
  const u = new URL(redirect);
  return new Promise((resolve, reject) => {
    const req = request({ hostname: "127.0.0.1", port: u.port, method: options.method ?? "GET", path: (options.path ?? u.pathname) + "?" + params, headers: { Host: options.host ?? u.host }, agent: false }, res => {
      let body = "";
      res.setEncoding("utf8"); res.on("data", chunk => { body += chunk; });
      res.on("end", () => resolve({ status: res.statusCode!, body, cache: res.headers["cache-control"] }));
    });
    req.on("error", reject); req.end();
  });
}
async function assertPortReleased(redirect: string): Promise<void> {
  const next = new McpOAuthCallback();
  try { await next.listen(Number(new URL(redirect).port)); } finally { await next.close(); }
}

test("OAuth callback rejects forged, ambiguous and wrong-endpoint responses before accepting one code", async () => {
  const cb = new McpOAuthCallback(); await cb.listen(0);
  const valid = new URLSearchParams({ state: cb.state, code: "fixture-code", iss: issuer });
  try {
    expect((await callbackRequest(cb.redirectUrl, valid)).status).toBe(400); cb.arm(issuer, true);
    for (const options of [{ method: "POST" }, { path: "/other" }, { host: "evil.test" }]) expect((await callbackRequest(cb.redirectUrl, valid, options)).status).toBe(400);
    for (const [key, value] of [["state", "wrong"], ["state", "é".repeat(43)], ["iss", "https://other.test"], ["code", ""]]) {
      const p = new URLSearchParams(valid); p.set(key!, value!);
      expect((await callbackRequest(cb.redirectUrl, p)).status).toBe(400);
    }
    for (const key of ["state", "code", "iss", "error"]) {
      const p = new URLSearchParams(valid); p.append(key, "duplicate"); if (key === "error") p.append(key, "second");
      expect((await callbackRequest(cb.redirectUrl, p)).status).toBe(400);
    }
    const missing = new URLSearchParams(valid); missing.delete("iss");
    expect((await callbackRequest(cb.redirectUrl, missing)).status).toBe(400);
    const mixed = new URLSearchParams(valid); mixed.set("error", "");
    expect((await callbackRequest(cb.redirectUrl, mixed)).status).toBe(400);
    const accepted = await callbackRequest(cb.redirectUrl, valid);
    expect(accepted.status).toBe(200); expect(accepted.cache).toBe("no-store"); expect(accepted.body).not.toContain("fixture-code");
    expect(await cb.code).toBe("fixture-code"); expect((await callbackRequest(cb.redirectUrl, valid)).status).toBe(409);
  } finally { await cb.close(); }
  await assertPortReleased(cb.redirectUrl);
});

test("OAuth denial does not reflect provider details", async () => {
  const cb = new McpOAuthCallback(); await cb.listen(0); cb.arm(issuer, false);
  try {
    const res = await callbackRequest(cb.redirectUrl, new URLSearchParams({ state: cb.state, error: "access_denied", error_description: "sensitive-detail" }));
    expect(res.status).toBe(200); expect(res.body).not.toContain("sensitive-detail");
    await expect(cb.code).rejects.toThrow("authorization was denied");
  } finally { await cb.close(); }
});

test("OAuth credentials bind server/resource/issuer/redirect; PKCE is memory-only and legacy files remain intact", async () => withHome(async root => {
  const cb = new McpOAuthCallback(); await cb.listen(0);
  try {
    const p = new NekoOAuthProvider("a/b", resource, cb);
    expect(p.tokens()).toBeUndefined(); expect(() => p.saveTokens({ access_token: "fixture-access", token_type: "Bearer" })).toThrow("discovery");
    p.saveDiscoveryState(discovery()); p.saveClientInformation({ client_id: "fixture-client" });
    p.saveTokens({ access_token: "fixture-access", token_type: "Bearer", refresh_token: "fixture-refresh" }); p.saveCodeVerifier("fixture-verifier");
    expect(p.codeVerifier()).toBe("fixture-verifier");
    const reopened = new NekoOAuthProvider("a/b", resource, cb); reopened.saveDiscoveryState(discovery());
    expect(reopened.clientInformation()?.client_id).toBe("fixture-client"); expect(reopened.tokens()?.access_token).toBe("fixture-access");
    expect(() => reopened.codeVerifier()).toThrow("unavailable");
    for (const [name, target, as] of [["a_b", resource, issuer], ["a/b", resource + "/other", issuer], ["a/b", resource, "https://other.test"]]) {
      const other = new NekoOAuthProvider(name!, target!, cb); other.saveDiscoveryState(discovery(as, target));
      expect(other.tokens()).toBeUndefined(); expect(other.clientInformation()).toBeUndefined();
    }
    const otherCallback = new McpOAuthCallback(); await otherCallback.listen(0);
    try { const other = new NekoOAuthProvider("a/b", resource, otherCallback); other.saveDiscoveryState(discovery()); expect(other.tokens()).toBeUndefined(); } finally { await otherCallback.close(); }
    const parent = join(root, ".neko-core", "mcp-auth");
    const records = readdirSync(join(parent, "v2"), { recursive: true }).map(String);
    expect(records.filter(f => f.endsWith(".json")).length).toBe(2); expect(records.some(f => f.includes("verifier"))).toBe(false);
    const legacy = join(parent, "a_b"); mkdirSync(legacy); writeFileSync(join(legacy, "tokens.json"), "legacy fixture");
    p.invalidateCredentials("tokens"); expect(p.tokens()).toBeUndefined(); expect(p.clientInformation()?.client_id).toBe("fixture-client");
    p.invalidateCredentials("all"); expect(p.clientInformation()).toBeUndefined(); expect(() => p.codeVerifier()).toThrow("unavailable");
    expect(readFileSync(join(legacy, "tokens.json"), "utf8")).toBe("legacy fixture");
  } finally { await cb.close(); }
}));

test("OAuth rejects mismatched discovery and corrupt or oversized credentials", async () => withHome(async root => {
  const cb = new McpOAuthCallback(), p = new NekoOAuthProvider("fixture", resource, cb);
  for (const mutate of [
    (d: OAuthDiscoveryState) => { d.authorizationServerMetadata!.issuer = "https://evil.test"; },
    (d: OAuthDiscoveryState) => { d.resourceMetadata!.resource += "/other"; },
    (d: OAuthDiscoveryState) => { d.resourceMetadata!.authorization_servers = []; },
    (d: OAuthDiscoveryState) => { d.authorizationServerMetadata!.code_challenge_methods_supported = ["plain"]; },
    (d: OAuthDiscoveryState) => { delete d.authorizationServerMetadata!.code_challenge_methods_supported; },
    (d: OAuthDiscoveryState) => { d.authorizationServerMetadata!.token_endpoint = "http://auth.example.test/token"; },
    (d: OAuthDiscoveryState) => { d.authorizationServerMetadata!.registration_endpoint = "http://auth.example.test/register"; },
  ]) { const d = discovery(); mutate(d); expect(() => p.saveDiscoveryState(d)).toThrow("invalid"); }
  p.saveDiscoveryState(discovery());
  await expect(p.validateResourceURL(resource, resource + "/other")).rejects.toThrow("mismatch");
  await expect(p.validateResourceURL(resource + "/other", resource)).rejects.toThrow("mismatch");
  expect((await p.validateResourceURL(resource, resource)).href).toBe(resource);
  p.saveTokens({ access_token: "fixture", token_type: "Bearer" });
  const dir = join(root, ".neko-core", "mcp-auth", "v2"), file = join(dir, readdirSync(dir)[0]!, "tokens.json");
  for (const content of ["{broken", JSON.stringify({ refresh_token: "fixture-only" }), "x".repeat(65537)]) {
    writeFileSync(file, content); expect(() => p.tokens()).toThrow("cannot be read");
  }
  rmSync(file); mkdirSync(file); expect(() => p.tokens()).toThrow("cannot be read"); await cb.close();
}));

function sdkFixture() {
  let authorization: URL | undefined, tokenPosts = 0;
  const json = <T>(data: T, status = 200) => new Response(JSON.stringify(data), { status, headers: { "Content-Type": "application/json" } });
  const fetchFn: FetchLike = async (input, init) => {
    expect(init?.redirect).toBe("error"); expect(init?.credentials).toBe("omit");
    const url = input instanceof Request ? input.url : String(input);
    if (url === resource + "/metadata") return json(discovery().resourceMetadata);
    if (url === issuer + "/.well-known/oauth-authorization-server") return json(discovery().authorizationServerMetadata);
    if (url === issuer + "/register") return json({ ...JSON.parse(String(init?.body)), client_id: "fixture-client" }, 201);
    if (url === issuer + "/token") {
      tokenPosts++; const body = new URLSearchParams(String(init?.body));
      expect(body.get("resource")).toBe(resource); expect(body.get("code")).toBe("fixture-code");
      expect(body.get("grant_type")).toBe("authorization_code"); expect(body.get("redirect_uri")).toBe(authorization!.searchParams.get("redirect_uri"));
      expect(createHash("sha256").update(body.get("code_verifier")!).digest("base64url")).toBe(authorization!.searchParams.get("code_challenge")!);
      return json({ access_token: "fixture-access", token_type: "Bearer" });
    }
    if (url === resource) {
      if (init?.method === "GET") return new Response(null, { status: 405 });
      if (new Headers(init?.headers).get("Authorization") !== "Bearer fixture-access") return new Response(null, { status: 401, headers: { "WWW-Authenticate": 'Bearer resource_metadata="' + resource + '/metadata", scope="read"' } });
      const body = JSON.parse(String(init?.body));
      if (body.method === "initialize") return json({ jsonrpc: "2.0", id: body.id, result: { protocolVersion: "2025-06-18", capabilities: { tools: {} }, serverInfo: { name: "fixture", version: "1" } } });
      if (body.method === "tools/list") return json({ jsonrpc: "2.0", id: body.id, result: { tools: [] } });
      return new Response(null, { status: 202 });
    }
    throw new Error("Unexpected fixture route");
  };
  return { fetchFn, get tokenPosts() { return tokenPosts; }, authorize(url: string) { authorization = new URL(url); return authorization; } };
}

test("real MCP SDK completes discovery, PKCE and loopback callback with an injected OAuth service", async () => withHome(async () => {
  const fixture = sdkFixture(), client = new Client({ name: "fixture", version: "1" }); let redirect = "";
  try {
    const transport = await connectWithOAuth(client, "fixture", resource, { port: 0, timeoutMs: 5000, fetch: fixture.fetchFn, openUrl: async url => {
      const auth = fixture.authorize(url); redirect = auth.searchParams.get("redirect_uri")!;
      expect(auth.searchParams.get("resource")).toBe(resource); expect(auth.searchParams.get("code_challenge_method")).toBe("S256");
      const p = new URLSearchParams({ state: "forged", code: "fixture-code", iss: issuer });
      expect((await callbackRequest(redirect, p)).status).toBe(400); expect(fixture.tokenPosts).toBe(0);
      p.set("state", auth.searchParams.get("state")!); expect((await callbackRequest(redirect, p)).status).toBe(200);
    } });
    expect(transport).toBeDefined(); expect(fixture.tokenPosts).toBe(1); expect((await client.listTools()).tools).toEqual([]);
    await assertPortReleased(redirect);
  } finally { await client.close(); }
}));

for (const mode of ["deny", "timeout", "abort", "token-error"] as const) test("OAuth " + mode + " cleans listener and does not report success", async () => withHome(async () => {
  const fixture = sdkFixture(), client = new Client({ name: "fixture", version: "1" }), controller = new AbortController(); let redirect = "";
  const fetchFn: FetchLike = (input, init) => String(input) === issuer + "/token" && mode === "token-error" ? Promise.resolve(new Response("sensitive-provider-detail", { status: 500 })) : fixture.fetchFn(input, init);
  try {
    const connecting = connectWithOAuth(client, "fixture", resource, { port: 0, timeoutMs: mode === "timeout" ? 150 : 5000, signal: controller.signal, fetch: fetchFn, openUrl: async url => {
      const auth = fixture.authorize(url); redirect = auth.searchParams.get("redirect_uri")!;
      if (mode === "abort") controller.abort();
      if (mode === "deny" || mode === "token-error") {
        const p = new URLSearchParams({ state: auth.searchParams.get("state")!, iss: issuer }); p.set(mode === "deny" ? "error" : "code", mode === "deny" ? "access_denied" : "fixture-code");
        await callbackRequest(redirect, p);
      }
    } });
    await expect(connecting).rejects.toThrow(mode === "deny" ? "denied" : mode === "timeout" ? "timed out" : mode === "abort" ? "canceled" : "could not finish");
    expect(redirect).not.toBe(""); await assertPortReleased(redirect); expect(fixture.tokenPosts).toBe(0);
  } finally { await client.close(); }
}));

test("non-authentication failure returns without opening a browser", async () => withHome(async () => {
  const client = new Client({ name: "fixture", version: "1" }); let opened = false;
  await expect(connectWithOAuth(client, "fixture", resource, { port: 0, timeoutMs: 5000, fetch: async () => new Response("fixture failure", { status: 503 }), openUrl: () => { opened = true; } })).rejects.toThrow("before authorization");
  expect(opened).toBe(false);
}));

test("closing McpHub cancels pending OAuth and releases the callback port", async () => withHome(async () => {
  let received!: () => void; const started = new Promise<void>(resolve => { received = resolve; });
  const server = createServer(() => { received(); }); await new Promise<void>(resolve => server.listen(0, "127.0.0.1", resolve));
  const addr = server.address(); if (!addr || isText(addr)) throw new Error("fixture address");
  const hub = new McpHub(), errors = spyOn(console, "error").mockImplementation(() => {});
  try {
    const pending = hub.connectAll({ fixture: { url: "http://127.0.0.1:" + addr.port + "/mcp", oauth: true } });
    await started; await hub.close(); await assertPortReleased("http://localhost:41789/callback"); await pending;
    expect(hub.serverNames).toEqual([]);
    expect(errors.mock.calls.some(args => String(args[0]).includes("failed to connect"))).toBe(true);
  } finally { await hub.close(); errors.mockRestore(); await new Promise<void>(resolve => { server.close(() => resolve()); server.closeAllConnections(); }); }
}));
