import { expect, spyOn, test } from "bun:test";
import { Client } from "@modelcontextprotocol/sdk/client/index.js";
import { createServer, request as httpRequest, type IncomingMessage, type ServerResponse } from "node:http";
import { request as httpsRequest, type RequestOptions } from "node:https";
import { oauthHttpFetch } from "../src/adapters/mcp-oauth-network.ts";
import { isText } from "../src/shared/wire.ts";
import { connectWithOAuth } from "../src/adapters/mcp-oauth.ts";

test("default MCP OAuth transport refuses a discovered loopback URL before dispatch", async () => {
  const resource = "https://mcp.example.test/mcp";
  let resourceCalls = 0, discoveredCalls = 0;
  const network = spyOn(globalThis, "fetch").mockImplementation(Object.assign(async (input: RequestInfo | URL) => {
    if (String(input) === resource) {
      resourceCalls++;
      return new Response(null, { status: 401, headers: { "WWW-Authenticate": 'Bearer resource_metadata="https://127.0.0.1/metadata"' } });
    }
    discoveredCalls++;
    return new Response(null, { status: 503 });
  }, { preconnect() {} }));
  const client = new Client({ name: "fixture", version: "1" });
  try {
    await expect(connectWithOAuth(client, "fixture", resource, { port: 0, timeoutMs: 1000, openUrl: () => { throw new Error("unexpected authorization"); } })).rejects.toThrow();
    expect(resourceCalls).toBe(1);
    expect(discoveredCalls).toBe(0);
  } finally { await client.close(); network.mockRestore(); }
});

const publicAddress = { address: "93.184.216.34", family: 4 as const };
const target = "https://auth.example.test/token";

test("OAuth refuses private literals, mixed DNS, malformed endpoints and unsupported requests before dispatch", async () => {
  let sent = 0, lookedUp = 0;
  const dependencies = {
    lookup: async () => { lookedUp++; return [publicAddress, { address: "10.0.0.7", family: 4 as const }]; },
    request: () => { sent++; throw new Error("must not dispatch"); },
  };
  for (const url of ["https://127.1/", "https://2130706433/", "https://[::1]/", "https://[::ffff:127.0.0.1]/", "https://169.254.169.254/", "http://auth.example.test/", "https://user:fixture@auth.example.test/", target + "#fixture", target]) {
    await expect(oauthHttpFetch(url, {}, dependencies)).rejects.toThrow("network limits");
  }
  expect(lookedUp).toBe(1);
  for (const init of [{ method: "DELETE" }, { body: "GET body" }, { method: "POST", body: new Uint8Array(2) }, { method: "POST", body: "x".repeat(65537) }]) {
    await expect(oauthHttpFetch(target, init, dependencies)).rejects.toThrow("network limits");
  }
  expect(lookedUp).toBe(1); expect(sent).toBe(0);
});

test("OAuth DNS cancellation prevents a late resolver from dispatching", async () => {
  const controller = new AbortController();
  let resolve!: (rows: typeof publicAddress[]) => void, sent = 0;
  const pending = oauthHttpFetch(target, { signal: controller.signal }, {
    lookup: () => new Promise(done => { resolve = done; }),
    request: () => { sent++; throw new Error("must not dispatch"); },
  });
  controller.abort(new Error("fixture-private-reason"));
  await expect(pending).rejects.toThrow("network limits");
  resolve([publicAddress]); await Promise.resolve();
  expect(sent).toBe(0);
});

test("runtime HTTPS honors the pinned lookup while retaining TLS verification", async () => {
  let connections = 0, lookups = 0;
  const server = createServer();
  server.on("connection", () => { connections++; });
  server.on("clientError", (_error, socket) => { socket.destroy(); });
  await new Promise<void>(resolve => server.listen(0, "127.0.0.1", resolve));
  const address = server.address();
  if (!address || isText(address)) throw new Error("fixture address missing");
  try {
    const result = new Promise<void>((resolve, reject) => {
      const request = httpsRequest("https://unresolvable.invalid/", {
        port: address.port, agent: false, family: 4, rejectUnauthorized: true, signal: AbortSignal.timeout(1000),
        lookup: (_hostname, options, callback) => {
          lookups++;
          if (options.all) callback(null, [{ address: "127.0.0.1", family: 4 }]);
          else callback(null, "127.0.0.1", 4);
        },
      }, () => resolve());
      request.on("error", reject); request.end();
    });
    await expect(result).rejects.toThrow();
    expect(lookups).toBe(1); expect(connections).toBe(1);
  } finally { server.closeAllConnections(); await new Promise<void>(resolve => server.close(() => resolve())); }
});

type WireCalls = { urls: string[]; options: RequestOptions[] };
async function localWire(handler: (request: IncomingMessage, response: ServerResponse) => void, run: (request: typeof httpsRequest, calls: WireCalls) => Promise<void>): Promise<void> {
  const server = createServer(handler);
  await new Promise<void>(resolve => server.listen(0, "127.0.0.1", resolve));
  const address = server.address();
  if (!address || isText(address)) throw new Error("fixture address missing");
  const calls: WireCalls = { urls: [], options: [] };
  // SAFETY: this test seam receives only the URL/options/callback overload used by oauthHttpFetch.
  // It replaces the TLS socket with loopback HTTP to exercise real body, abort and close behavior.
  const request = ((url: URL, options: RequestOptions, callback: (response: IncomingMessage) => void) => {
    calls.urls.push(url.href); calls.options.push(options);
    return httpRequest(`http://127.0.0.1:${address.port}${url.pathname}`, { ...options, headers: { ...options.headers, host: url.host } }, callback);
  }) as typeof httpsRequest;
  try { await run(request, calls); }
  finally { server.closeAllConnections(); await new Promise<void>(resolve => server.close(() => resolve())); }
}

test("OAuth pins the checked DNS result, keeps TLS host identity and sends a POST only once", async () => {
  let received = "", host = "", cookie: string | undefined, count = 0, lookups = 0;
  await localWire(async (req, res) => {
    count++; host = req.headers.host!; cookie = req.headers.cookie;
    for await (const chunk of req) received += chunk;
    res.writeHead(200, { "Content-Type": "application/json" }); res.end('{"ok":true}');
  }, async (request, calls) => {
    const body = new URLSearchParams({ grant_type: "refresh_token", refresh_token: "fixture-secret" });
    const response = await oauthHttpFetch(target, { method: "POST", body, headers: { "content-type": "application/x-www-form-urlencoded", cookie: "fixture-cookie", "proxy-authorization": "fixture-proxy", "content-length": "99999", host: "evil.test" } }, {
      lookup: async () => { lookups++; return lookups === 1 ? [publicAddress] : [{ address: "127.0.0.1", family: 4 }]; }, request,
    });
    expect(await response.json()).toEqual({ ok: true }); expect(response.url).toBe(target);
    expect(calls.urls).toEqual([target]);
    const options = calls.options[0]!;
    expect(options.agent).toBe(false); expect(options.rejectUnauthorized).toBe(true);
    expect(options.family).toBe(4); expect(options.maxHeaderSize).toBe(32768);
    const resolved = await new Promise<unknown>((resolve, reject) => options.lookup!("auth.example.test", { all: true }, (err, rows) => err ? reject(err) : resolve(rows)));
    expect(resolved).toEqual([publicAddress]); expect(lookups).toBe(1);
    expect(options.headers).not.toHaveProperty("proxy-authorization");
    expect(received).toBe(String(body)); expect(host).toBe("auth.example.test"); expect(cookie).toBeUndefined(); expect(count).toBe(1);
  });
});

for (const mode of ["redirect", "lost-response", "oversized-length", "oversized-stream", "compressed", "abort", "before-headers", "deadline"] as const) test("OAuth stops " + mode + " without another address or redirect attempt", async () => {
  let hits = 0, released!: () => void;
  const ready = new Promise<void>(resolve => { released = resolve; });
  const controller = new AbortController();
  await localWire((req, res) => {
    hits++; req.resume();
    if (mode === "redirect") { res.writeHead(307, { location: "https://other.example.test/token" }); res.end(); }
    if (mode === "lost-response") req.socket.destroy();
    if (mode === "oversized-length") { res.writeHead(200, { "content-length": "65537" }); res.flushHeaders(); }
    if (mode === "oversized-stream") { res.writeHead(200); res.end("x".repeat(65537)); }
    if (mode === "compressed") { res.writeHead(200, { "content-encoding": "gzip" }); res.end("fixture"); }
    if (mode === "abort" || mode === "deadline") { res.writeHead(200); res.write("{"); }
    released();
  }, async (request, calls) => {
    const result = oauthHttpFetch(target, { method: "POST", body: "fixture", signal: mode === "deadline" ? AbortSignal.timeout(100) : controller.signal }, {
      lookup: async () => [publicAddress, { address: "8.8.8.8", family: 4 }], request,
    });
    const failure = result.then(() => undefined, error => error);
    await ready;
    if (mode === "abort" || mode === "before-headers") controller.abort(new Error("fixture-secret"));
    expect(await failure).toMatchObject({ message: "MCP OAuth endpoint request failed within its network limits" }); expect(hits).toBe(1); expect(calls.urls).toEqual([target]);
  });
});

test("OAuth accepts exact response limits and preserves error status for the SDK", async () => {
  await localWire((req, res) => {
    req.resume();
    const status = req.url === "/error" ? 401 : 200;
    const size = req.method === "GET" && status === 200 ? 1048576 : 65536;
    res.writeHead(status, { "content-length": String(size) }); res.end("x".repeat(size));
  }, async request => {
    for (const [path, method, size, status] of [["/metadata", "GET", 1048576, 200], ["/token", "POST", 65536, 200], ["/error", "GET", 65536, 401]] as const) {
      const response = await oauthHttpFetch("https://auth.example.test" + path, { method }, { lookup: async () => [publicAddress], request });
      expect(response.status).toBe(status); expect((await response.text()).length).toBe(size);
    }
  });
});
