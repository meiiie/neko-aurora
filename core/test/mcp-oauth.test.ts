import { expect, spyOn, test } from "bun:test";
import { createHash } from "node:crypto";
import { mkdtempSync, mkdirSync, readdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { createServer, request } from "node:http";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { Client } from "@modelcontextprotocol/sdk/client/index.js";
import { ToolListChangedNotificationSchema } from "@modelcontextprotocol/sdk/types.js";
import type { OAuthDiscoveryState } from "@modelcontextprotocol/sdk/client/auth.js";
import type { FetchLike } from "@modelcontextprotocol/sdk/shared/transport.js";
import { McpOAuthCallback } from "../src/adapters/mcp-oauth-callback.ts";
import { connectWithOAuth, NekoOAuthProvider } from "../src/adapters/mcp-oauth.ts";
import { McpHub } from "../src/adapters/mcp.ts";
import { isText } from "../src/shared/wire.ts";
import * as atomic from "../src/shared/atomic.ts";
import { readOAuthRecord } from "../src/adapters/mcp-oauth-storage.ts";

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

function sdkFixture(toolStatus?: 401 | 403) {
  let authorization: URL | undefined, tokenPosts = 0, toolCalls = 0;
  const json = <T>(data: T, status = 200) => new Response(JSON.stringify(data), { status, headers: { "Content-Type": "application/json" } });
  const fetchFn: FetchLike = async (input, init) => {
    expect(init?.redirect).toBe("error"); expect(init?.credentials).toBe("omit");
    const url = input instanceof Request ? input.url : String(input);
    if (url === resource + "/metadata") return json(discovery().resourceMetadata);
    if (url === issuer + "/.well-known/oauth-authorization-server") return json(discovery().authorizationServerMetadata);
    if (url === issuer + "/register") return json({ ...JSON.parse(String(init?.body)), client_id: "fixture-client" }, 201);
    if (url === issuer + "/token") {
      tokenPosts++; const body = new URLSearchParams(String(init?.body));
      expect(body.get("resource")).toBe(resource);
      if (body.get("grant_type") === "refresh_token") {
        expect(body.get("refresh_token")).toBe("fixture-refresh");
        return json({ access_token: "fixture-access", token_type: "Bearer", refresh_token: "fixture-refresh" });
      }
      expect(body.get("code")).toBe("fixture-code");
      expect(body.get("grant_type")).toBe("authorization_code"); expect(body.get("redirect_uri")).toBe(authorization!.searchParams.get("redirect_uri"));
      expect(createHash("sha256").update(body.get("code_verifier")!).digest("base64url")).toBe(authorization!.searchParams.get("code_challenge")!);
      return json({ access_token: "fixture-access", token_type: "Bearer", refresh_token: "fixture-refresh" });
    }
    if (url === resource) {
      if (init?.method === "GET") return new Response(null, { status: 405 });
      if (new Headers(init?.headers).get("Authorization") !== "Bearer fixture-access") return new Response(null, { status: 401, headers: { "WWW-Authenticate": 'Bearer resource_metadata="' + resource + '/metadata", scope="read"' } });
      const body = JSON.parse(String(init?.body));
      if (body.method === "initialize") return json({ jsonrpc: "2.0", id: body.id, result: { protocolVersion: "2025-06-18", capabilities: { tools: {} }, serverInfo: { name: "fixture", version: "1" } } });
      if (body.method === "tools/call" && toolStatus) {
        toolCalls++;
        if (toolCalls === 1) return new Response("private-provider-error", { status: toolStatus, headers: { "WWW-Authenticate": 'Bearer error="insufficient_scope", scope="write"' } });
        return json({ jsonrpc: "2.0", id: body.id, result: { content: [{ type: "text", text: "mutation repeated" }] } });
      }
      if (body.method === "tools/list") return json({ jsonrpc: "2.0", id: body.id, result: { tools: [] } });
      return new Response(null, { status: 202 });
    }
    throw new Error("Unexpected fixture route");
  };
  return { fetchFn, get toolCalls() { return toolCalls; }, get tokenPosts() { return tokenPosts; }, authorize(url: string) { authorization = new URL(url); return authorization; } };
}

for (const stage of ["resource", "metadata", "issuer", "registration", "token"] as const) test("SDK rejects oversized OAuth " + stage + " without continuing authorization", async () => withHome(async root => {
  const fixture = sdkFixture(), client = new Client({ name: "fixture", version: "1" });
  let canceled = 0, opened = 0, posts = 0, hit = false, requestSignal: AbortSignal | null | undefined;
  const target = { resource, metadata: resource + "/metadata", issuer: issuer + "/.well-known/oauth-authorization-server", registration: issuer + "/register", token: issuer + "/token" }[stage];
  try {
    await expect(connectWithOAuth(client, "fixture", resource, { port: 0, timeoutMs: 2000, fetch: async (input, init) => {
      if (String(input) === issuer + "/token") posts++;
      if (String(input) !== target) return fixture.fetchFn(input, init);
      hit = true; requestSignal = init?.signal;
      const payload = stage === "resource" ? {} : stage === "metadata" ? discovery().resourceMetadata : stage === "issuer" ? discovery().authorizationServerMetadata : stage === "registration" ? { ...JSON.parse(String(init?.body)), client_id: "fixture-client" } : { access_token: "fixture-access", refresh_token: "fixture-refresh", token_type: "Bearer" };
      const data = new TextEncoder().encode(JSON.stringify({ ...payload, padding: "x".repeat(1024 * 1024 + 1) }));
      let sent = false;
      return new Response(new ReadableStream<Uint8Array>({ pull(c) { if (sent) c.close(); else { sent = true; c.enqueue(data); } }, cancel() { canceled++; } }, { highWaterMark: 0 }), { status: stage === "resource" ? 401 : 200, headers: { "Content-Type": "application/json", "Content-Length": "1", "WWW-Authenticate": 'Bearer resource_metadata="' + resource + '/metadata"' } });
    }, openUrl: async url => {
      opened++; const auth = fixture.authorize(url);
      await callbackRequest(auth.searchParams.get("redirect_uri")!, new URLSearchParams({ state: auth.searchParams.get("state")!, code: "fixture-code", iss: issuer }));
    } })).rejects.toThrow();
    expect(hit).toBe(true); expect(canceled).toBe(1); expect(requestSignal?.aborted).toBe(true);
    expect(posts).toBe(stage === "token" ? 1 : 0); expect(opened).toBe(stage === "token" ? 1 : 0);
    if (stage === "token") {
      const parent = join(root, ".neko-core", "mcp-auth", "v2");
      expect(readOAuthRecord(join(parent, readdirSync(parent)[0]!, "tokens.json"), value => value)).toEqual({ version: 2, phase: "consuming", attempt: expect.stringMatching(/^[a-f0-9]{32}$/) });
    }
  } finally { await client.close(); }
}));

test("SDK keeps large RPC responses and live SSE outside the OAuth body budget even with shared endpoint URLs", async () => withHome(async () => {
  const fixture = sdkFixture(), client = new Client({ name: "fixture", version: "1" });
  const text = "x".repeat(1024 * 1024 + 1); let sseOpened = false;
  let notify!: () => void;
  const notification = new Promise<void>(resolve => { notify = resolve; });
  client.setNotificationHandler(ToolListChangedNotificationSchema, () => { notify(); });
  try {
    await connectWithOAuth(client, "fixture", resource, { port: 0, timeoutMs: 2000, fetch: async (input, init) => {
      if (String(input) === issuer + "/.well-known/oauth-authorization-server") return Response.json({ ...discovery().authorizationServerMetadata, token_endpoint: resource, registration_endpoint: resource });
      if (String(input) === resource) {
        const headers = new Headers(init?.headers);
        if (!headers.get("accept")?.includes("text/event-stream")) return fixture.fetchFn(issuer + (headers.get("content-type") === "application/json" ? "/register" : "/token"), init);
        if (init?.method === "GET") { sseOpened = true; return new Response(new ReadableStream<Uint8Array>({ start(c) {
          c.enqueue(new TextEncoder().encode(": " + text + '\n\ndata: {"jsonrpc":"2.0","method":"notifications/tools/list_changed"}\n\n'));
          init.signal?.addEventListener("abort", () => { c.close(); }, { once: true });
        } }), { headers: { "Content-Type": "text/event-stream" } }); }
        const body = JSON.parse(String(init?.body));
        if (body.method === "tools/call") return Response.json({ jsonrpc: "2.0", id: body.id, result: { content: [{ type: "text", text }] } });
      }
      return fixture.fetchFn(input, init);
    }, openUrl: async url => {
      const auth = fixture.authorize(url);
      await callbackRequest(auth.searchParams.get("redirect_uri")!, new URLSearchParams({ state: auth.searchParams.get("state")!, code: "fixture-code", iss: issuer }));
    } });
    expect((await client.callTool({ name: "fixture", arguments: {} })).content).toEqual([{ type: "text", text }]);
    let deadline: ReturnType<typeof setTimeout> | undefined;
    try { await Promise.race([notification, new Promise<never>((_, reject) => { deadline = setTimeout(() => reject(new Error("fixture SSE notification deadline")), 500); })]); }
    finally { clearTimeout(deadline); }
    expect(fixture.tokenPosts).toBe(1); expect(sseOpened).toBe(true);
  } finally { await client.close(); }
}));

for (const mode of ["abort", "deadline", "stream-error", "oversized-error", "oversized-success"] as const) test("SDK refresh response " + mode + " leaves a consumed token and never retries", async () => withHome(async root => {
  const callback = new McpOAuthCallback(); await callback.listen(0);
  const port = Number(new URL(callback.redirectUrl).port), seed = new NekoOAuthProvider("fixture", resource, callback);
  seed.saveDiscoveryState(discovery()); seed.saveClientInformation({ client_id: "fixture-client" });
  seed.saveTokens({ access_token: "fixture-access", refresh_token: "fixture-refresh", token_type: "Bearer" });
  await callback.close();
  const fixture = sdkFixture(), client = new Client({ name: "fixture", version: "1" }), controller = new AbortController();
  let posts = 0, opened = 0, canceled = 0;
  try {
    await expect(connectWithOAuth(client, "fixture", resource, { port, timeoutMs: mode === "deadline" ? 80 : 2000, signal: controller.signal, fetch: async (input, init) => {
      if (String(input) !== issuer + "/token") return fixture.fetchFn(input, init);
      posts++;
      const stream = new ReadableStream<Uint8Array>({ pull(c) {
        if (mode === "abort") controller.abort();
        else if (mode === "stream-error") c.error(new Error("private token response"));
        else if (mode.startsWith("oversized")) c.enqueue(new TextEncoder().encode(" ".repeat(64 * 1024 + 1)));
      }, cancel() { canceled++; return new Promise<void>(() => {}); } }, { highWaterMark: 0 });
      return new Response(stream, { status: mode === "oversized-error" ? 400 : 200 });
    }, openUrl: () => { opened++; } })).rejects.toThrow();
    expect(posts).toBe(1); expect(opened).toBe(0); expect(canceled).toBe(mode === "stream-error" ? 0 : 1);
    const parent = join(root, ".neko-core", "mcp-auth", "v2");
    expect(readOAuthRecord(join(parent, readdirSync(parent)[0]!, "tokens.json"), value => value)).toEqual({ version: 2, phase: "consuming", attempt: expect.stringMatching(/^[a-f0-9]{32}$/) });
    await assertPortReleased(callback.redirectUrl);
  } finally { await client.close(); }
}));

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

for (const status of [401, 403] as const) test("OAuth tool " + status + " never refreshes or replays a mutation", async () => withHome(async () => {
  const fixture = sdkFixture(status), client = new Client({ name: "fixture", version: "1" }); let redirect = "";
  try {
    await connectWithOAuth(client, "fixture", resource, { port: 0, timeoutMs: 5000, fetch: fixture.fetchFn, openUrl: async url => {
      const auth = fixture.authorize(url);
      redirect = auth.searchParams.get("redirect_uri")!;
      await callbackRequest(redirect, new URLSearchParams({ state: auth.searchParams.get("state")!, code: "fixture-code", iss: issuer }));
    } });
    let failure = "";
    try { await client.callTool({ name: "mutate", arguments: {} }); } catch (error) { if (error instanceof Error) failure = error.message; }
    expect(fixture.toolCalls).toBe(1); expect(fixture.tokenPosts).toBe(1);
    expect(failure).toContain("reconnect"); expect(failure).not.toContain("private-provider-error");
    await client.close();
    const next = new Client({ name: "fixture", version: "1" });
    try {
      await connectWithOAuth(next, "fixture", resource, { port: Number(new URL(redirect).port), timeoutMs: 5000, fetch: fixture.fetchFn, openUrl: () => { throw new Error("unexpected new authorization"); } });
      expect(fixture.toolCalls).toBe(1); expect(fixture.tokenPosts).toBe(2);
      await next.callTool({ name: "mutate", arguments: {} }); expect(fixture.toolCalls).toBe(2);
    } finally { await next.close(); }
  } finally { await client.close(); }
}));

test("OAuth SSE authentication failure after connection does not start hidden refresh", async () => withHome(async () => {
  const fixture = sdkFixture(), client = new Client({ name: "fixture", version: "1" });
  let release!: () => void, started!: () => void;
  const gate = new Promise<void>(resolve => { release = resolve; }), requested = new Promise<void>(resolve => { started = resolve; });
  const fetchFn: FetchLike = async (input, init) => {
    if (String(input) === resource && init?.method === "GET") {
      started(); await gate; return new Response("private-stream-error", { status: 401 });
    }
    return fixture.fetchFn(input, init);
  };
  try {
    await connectWithOAuth(client, "fixture", resource, { port: 0, timeoutMs: 5000, fetch: fetchFn, openUrl: async url => {
      const auth = fixture.authorize(url);
      await callbackRequest(auth.searchParams.get("redirect_uri")!, new URLSearchParams({ state: auth.searchParams.get("state")!, code: "fixture-code", iss: issuer }));
    } });
    await requested;
    const failed = new Promise<Error>(resolve => { client.onerror = resolve; }); release();
    const error = await failed;
    expect(error.message).toContain("reconnect"); expect(error.message).not.toContain("private-stream-error"); expect(fixture.tokenPosts).toBe(1);
  } finally { release(); await client.close(); }
}));

test("pre-connect lost refresh response never opens a fallback browser or reuses the consumed token", async () => withHome(async () => {
  const callback = new McpOAuthCallback(); await callback.listen(0);
  const port = Number(new URL(callback.redirectUrl).port);
  const seed = new NekoOAuthProvider("fixture", resource, callback);
  seed.saveDiscoveryState(discovery()); seed.saveClientInformation({ client_id: "fixture-client" });
  seed.saveTokens({ access_token: "fixture-access", refresh_token: "fixture-refresh", token_type: "Bearer" });
  await callback.close();
  const fixture = sdkFixture(); let tokenPosts = 0, opened = 0;
  const fetchFn: FetchLike = async (input, init) => {
    if (String(input) === issuer + "/token") {
      tokenPosts++; throw new Error("fixture consumed refresh token then lost response");
    }
    return fixture.fetchFn(input, init);
  };
  const first = new Client({ name: "fixture", version: "1" });
  try {
    await expect(connectWithOAuth(first, "fixture", resource, { port, timeoutMs: 5000, fetch: fetchFn, openUrl: () => { opened++; throw new Error("unexpected fallback"); } })).rejects.toThrow();
    expect(tokenPosts).toBe(1); expect(opened).toBe(0);
  } finally { await first.close(); }
  const next = new Client({ name: "fixture", version: "1" });
  try {
    await connectWithOAuth(next, "fixture", resource, { port, timeoutMs: 5000, fetch: async (input, init) => {
      if (String(input) === issuer + "/token") {
        expect(new URLSearchParams(String(init?.body)).get("grant_type")).toBe("authorization_code"); tokenPosts++;
      }
      return fixture.fetchFn(input, init);
    }, openUrl: async url => {
      opened++; const auth = fixture.authorize(url);
      await callbackRequest(auth.searchParams.get("redirect_uri")!, new URLSearchParams({ state: auth.searchParams.get("state")!, code: "fixture-code", iss: issuer }));
    } });
    expect(tokenPosts).toBe(2); expect(opened).toBe(1);
  } finally { await next.close(); }
}), 15000);

for (const mode of ["before-save", "result-save", "invalid-client", "invalid-grant", "server-error", "malformed", "second-401", "abort-before-token", "canonical-endpoint"] as const) test("SDK token recovery fences " + mode, async () => withHome(async root => {
  const callback = new McpOAuthCallback(); await callback.listen(0);
  const port = Number(new URL(callback.redirectUrl).port);
  const seed = new NekoOAuthProvider("fixture", resource, callback);
  seed.saveDiscoveryState(discovery()); seed.saveClientInformation({ client_id: "fixture-client" });
  seed.saveTokens({ access_token: "fixture-access", refresh_token: "fixture-refresh", token_type: "Bearer" });
  const parent = join(root, ".neko-core", "mcp-auth", "v2"), file = join(parent, readdirSync(parent)[0]!, "tokens.json");
  const before = readFileSync(file, "utf8"); await callback.close();
  const original = atomic.atomicWriteFileSync; let writes = 0;
  const fault = spyOn(atomic, "atomicWriteFileSync").mockImplementation((path, data, permissions) => {
    if (path === file && (++writes === (mode === "before-save" ? 1 : mode === "result-save" ? 2 : -1))) throw new Error("private fixture storage detail");
    original(path, data, permissions);
  });
  const fixture = sdkFixture(), client = new Client({ name: "fixture", version: "1" }); let posts = 0, opened = 0;
  const controller = new AbortController();
  try {
    const fetchFn: FetchLike = async (input, init) => {
      if (String(input) === issuer + "/token") {
        posts++;
        expect(readFileSync(file, "utf8")).not.toContain("fixture-refresh");
        if (mode === "invalid-client" || mode === "invalid-grant" || mode === "server-error") return new Response(JSON.stringify({ error: mode.replaceAll("-", "_"), error_description: "private-provider-detail" }), { status: mode === "server-error" ? 500 : 400 });
        if (mode === "malformed") return new Response("{broken", { status: 200 });
        if (mode === "canonical-endpoint") throw new Error("fixture response lost");
      }
      if (mode === "canonical-endpoint" && String(input) === issuer + "/.well-known/oauth-authorization-server") return new Response(JSON.stringify({ ...discovery().authorizationServerMetadata, token_endpoint: "https://AUTH.EXAMPLE.TEST:443/token" }));
      if (mode === "second-401" && posts && String(input) === resource && init?.method === "POST") return new Response(null, { status: 401 });
      if (mode === "abort-before-token" && String(input) === issuer + "/.well-known/oauth-authorization-server") controller.abort();
      return fixture.fetchFn(input, init);
    };
    let failure = "";
    try { await connectWithOAuth(client, "fixture", resource, { port, timeoutMs: 5000, signal: controller.signal, fetch: fetchFn, openUrl: () => { opened++; throw new Error("unexpected fallback"); } }); }
    catch (error) { if (error instanceof Error) failure = error.message; }
    expect(failure).not.toBe(""); expect(failure).not.toContain("private"); expect(opened).toBe(0);
    expect(posts).toBe(mode === "before-save" || mode === "abort-before-token" ? 0 : 1);
    const reopenedCallback = new McpOAuthCallback(); await reopenedCallback.listen(port);
    try {
      const reopened = new NekoOAuthProvider("fixture", resource, reopenedCallback); reopened.saveDiscoveryState(discovery());
      if (mode === "before-save" || mode === "abort-before-token") { expect(readFileSync(file, "utf8")).toBe(before); expect(reopened.tokens()?.refresh_token).toBe("fixture-refresh"); }
      else if (mode === "second-401") expect(reopened.tokens()?.access_token).toBe("fixture-access");
      else expect(reopened.tokens()).toBeUndefined();
    } finally { await reopenedCallback.close(); }
  } finally { fault.mockRestore(); await client.close(); }
}));

test("token records migrate legacy credentials and reject malformed envelopes", async () => withHome(async root => {
  const callback = new McpOAuthCallback(); await callback.listen(0);
  try {
    const provider = new NekoOAuthProvider("fixture", resource, callback); provider.saveDiscoveryState(discovery()); provider.saveClientInformation({ client_id: "fixture-client" });
    const parent = join(root, ".neko-core", "mcp-auth", "v2"), file = join(parent, readdirSync(parent)[0]!, "tokens.json");
    const tokens = { access_token: "legacy-fixture", token_type: "Bearer", refresh_token: "legacy-refresh" };
    writeFileSync(file, JSON.stringify(tokens)); expect(provider.tokens()).toEqual(tokens);
    provider.saveTokens(tokens); expect(readOAuthRecord(file, value => value)).toMatchObject({ phase: "ready" }); expect(provider.tokens()).toEqual(tokens);
    for (const value of [{ version: 2, phase: "consuming" }, { version: 1, phase: "unknown" }, { version: 1, phase: "consuming", tokens }, { version: 1, phase: "ready" }]) {
      writeFileSync(file, JSON.stringify(value)); expect(() => provider.tokens()).toThrow("cannot be read");
    }
  } finally { await callback.close(); }
}));

for (const mode of ["invalid_client", "invalid_grant", "lost_response"] as const) test("authorization code failure " + mode + " cannot replay through SDK recovery", async () => withHome(async root => {
  const fixture = sdkFixture(), client = new Client({ name: "fixture", version: "1" }); let posts = 0, opened = 0, redirect = "";
  try {
    await expect(connectWithOAuth(client, "fixture", resource, { port: 0, timeoutMs: 5000, fetch: async (input, init) => {
      if (String(input) === issuer + "/token") {
        posts++;
        if (mode === "lost_response") throw new Error("private provider response lost");
        return new Response(JSON.stringify({ error: mode, error_description: "private provider detail" }), { status: 400 });
      }
      return fixture.fetchFn(input, init);
    }, openUrl: async url => {
      opened++; const auth = fixture.authorize(url); redirect = auth.searchParams.get("redirect_uri")!;
      await callbackRequest(redirect, new URLSearchParams({ state: auth.searchParams.get("state")!, code: "fixture-code", iss: issuer }));
    } })).rejects.toThrow(/reconnect/);
    expect(posts).toBe(1); expect(opened).toBe(1); await assertPortReleased(redirect);
    const parent = join(root, ".neko-core", "mcp-auth", "v2"), stored = readFileSync(join(parent, readdirSync(parent)[0]!, "tokens.json"), "utf8");
    expect(readOAuthRecord(join(parent, readdirSync(parent)[0]!, "tokens.json"), value => value)).toEqual({ version: 2, phase: "consuming", attempt: expect.stringMatching(/^[a-f0-9]{32}$/) });
    expect(stored).not.toContain("fixture-code"); expect(stored).not.toContain("fixture-refresh");
  } finally { await client.close(); }
}));
