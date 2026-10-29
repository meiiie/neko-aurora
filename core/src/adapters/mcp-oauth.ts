import { createHash, randomBytes } from "node:crypto";
import { rmSync } from "node:fs";
import { join } from "node:path";
import type { Client } from "@modelcontextprotocol/sdk/client/index.js";
import { UnauthorizedError, type OAuthClientProvider, type OAuthDiscoveryState } from "@modelcontextprotocol/sdk/client/auth.js";
import { OAuthClientInformationSchema, OAuthTokensSchema, type OAuthClientInformationMixed, type OAuthTokens } from "@modelcontextprotocol/sdk/shared/auth.js";
import type { FetchLike } from "@modelcontextprotocol/sdk/shared/transport.js";
import { StreamableHTTPClientTransport } from "@modelcontextprotocol/sdk/client/streamableHttp.js";
import { homeDir } from "../shared/home.ts";
import { throwIfAborted } from "../shared/abort.ts";
import { isJsonObject, isText, type JsonValue } from "../shared/wire.ts";
import { openBrowser } from "./chatgpt-auth.ts";
import { McpOAuthCallback } from "./mcp-oauth-callback.ts";
import { boundedOAuthResponse } from "./mcp-oauth-http.ts";
import { readOAuthRecord, writeOAuthRecord, withOAuthStorageLock, type OAuthRecordCache } from "./mcp-oauth-storage.ts";

function httpsEndpoint(raw: string | undefined): boolean {
  try { const u = new URL(raw ?? ""); return u.protocol === "https:" && !u.username && !u.password && !u.hash; } catch { return false; }
}

type TokenRecord = { version: 2; phase: "consuming"; attempt: string } | { version: 1; phase: "ready"; tokens: OAuthTokens };
class TokenAttemptError extends Error {
  constructor() { super("MCP OAuth token recovery requires an explicit reconnect. Request outcome may be unknown."); }
}

export class NekoOAuthProvider implements OAuthClientProvider {
  private discovery?: OAuthDiscoveryState;
  private verifier?: string;
  private dir?: string;
  private tokenAttempted = false;
  private attempt?: string;
  private readonly records: OAuthRecordCache = new Map();
  private readonly root = join(homeDir(), ".neko-core", "mcp-auth", "v2");
  redirected = false;

  constructor(private server: string, private resource: string, private callback: McpOAuthCallback, private openUrl: (url: string) => void | Promise<void> = openBrowser) {}

  get redirectUrl(): string { return this.callback.redirectUrl; }
  state(): string { this.allowAuthorization(); return this.callback.state; }
  private allowAuthorization(): void { if (this.tokenAttempted) throw new TokenAttemptError(); }
  get clientMetadata() {
    return { client_name: "Neko Core", redirect_uris: [this.redirectUrl], grant_types: ["authorization_code", "refresh_token"], response_types: ["code"], token_endpoint_auth_method: "none", application_type: "native" };
  }

  saveDiscoveryState(state: OAuthDiscoveryState): void {
    const metadata = state.authorizationServerMetadata;
    if (!metadata || !httpsEndpoint(state.authorizationServerUrl) || metadata.issuer !== state.authorizationServerUrl || !httpsEndpoint(metadata.authorization_endpoint) || !httpsEndpoint(metadata.token_endpoint) || (metadata.registration_endpoint !== undefined && !httpsEndpoint(metadata.registration_endpoint)) || !metadata.code_challenge_methods_supported?.includes("S256") || state.resourceMetadata?.resource !== this.resource || !state.resourceMetadata.authorization_servers?.includes(state.authorizationServerUrl)) {
      throw new Error("MCP OAuth discovery identity or PKCE capability is invalid");
    }
    if (this.redirected && this.discovery?.authorizationServerUrl !== state.authorizationServerUrl) throw new Error("MCP OAuth issuer changed during authorization");
    this.discovery = structuredClone(state);
    this.records.clear();
    this.dir = join(this.root, createHash("sha256").update(JSON.stringify([this.server, this.resource, state.authorizationServerUrl, this.redirectUrl])).digest("hex"));
  }

  discoveryState(): OAuthDiscoveryState | undefined { return this.discovery && structuredClone(this.discovery); }
  async validateResourceURL(serverUrl: string | URL, resource?: string): Promise<URL> {
    if (new URL(serverUrl).href !== new URL(this.resource).href || resource !== this.resource) throw new Error("MCP OAuth resource mismatch");
    return new URL(this.resource);
  }

  private read<T>(file: string, parse: (value: JsonValue) => T, lockHeld = false): T | undefined {
    if (!this.dir) return undefined;
    return readOAuthRecord(join(this.dir, file), parse, lockHeld, this.records);
  }

  private save(file: string, value: OAuthClientInformationMixed | TokenRecord): void {
    if (!this.dir) throw new Error("MCP OAuth discovery is required before saving credentials");
    writeOAuthRecord(join(this.dir, file), JSON.stringify(value), this.records);
  }

  clientInformation(): OAuthClientInformationMixed | undefined { return this.read("client.json", value => OAuthClientInformationSchema.parse(value)); }
  saveClientInformation(value: OAuthClientInformationMixed): void {
    if (!this.dir) throw new Error("MCP OAuth discovery is required before saving credentials");
    withOAuthStorageLock(this.dir, () => this.save("client.json", OAuthClientInformationSchema.parse(value)));
  }
  tokens(): OAuthTokens | undefined { return this.readTokens(); }
  private readTokens(lockHeld = false): OAuthTokens | undefined {
    return this.read("tokens.json", value => {
      if (isJsonObject(value) && (Object.hasOwn(value, "version") || Object.hasOwn(value, "phase"))) {
        if (value.version === 2 && value.phase === "consuming" && Object.keys(value).length === 3 && isText(value.attempt) && /^[a-f0-9]{32}$/.test(value.attempt)) return undefined;
        if (value.version !== 1) throw new Error("invalid token record");
        if (value.phase === "consuming" && Object.keys(value).length === 2) return undefined;
        if (value.phase !== "ready" || Object.keys(value).length !== 3) throw new Error("invalid token record");
        return OAuthTokensSchema.parse(value.tokens);
      }
      return OAuthTokensSchema.parse(value);
    }, lockHeld);
  }
  saveTokens(value: OAuthTokens): void {
    if (!this.dir) throw new Error("MCP OAuth discovery is required before saving credentials");
    const tokens = OAuthTokensSchema.parse(value);
    withOAuthStorageLock(this.dir, () => {
      if (this.tokenAttempted) {
        const current = this.read("tokens.json", value => value, true);
        if (!this.attempt || !isJsonObject(current) || Object.keys(current).length !== 3 || current.version !== 2 || current.phase !== "consuming" || current.attempt !== this.attempt) throw new TokenAttemptError();
      }
      this.save("tokens.json", { version: 1, phase: "ready", tokens });
    });
  }
  beforeTokenRequest(input: Parameters<FetchLike>[0], init?: Parameters<FetchLike>[1], signal?: AbortSignal): void {
    const url = input instanceof Request ? input.url : String(input);
    const endpoint = this.discovery?.authorizationServerMetadata?.token_endpoint;
    if (!endpoint || new URL(url).href !== new URL(endpoint).href) return;
    const registration = this.discovery?.authorizationServerMetadata?.registration_endpoint;
    if (registration && new URL(url).href === new URL(registration).href && init?.method === "POST" && new Headers(init.headers).get("content-type") === "application/json") return;
    this.allowAuthorization();
    this.tokenAttempted = true;
    if (signal?.aborted) throw new TokenAttemptError();
    if (init?.method !== "POST" || !(init.body instanceof URLSearchParams || isText(init.body))) throw new TokenAttemptError();
    const params = new URLSearchParams(init.body);
    if (!["authorization_code", "refresh_token"].includes(params.get("grant_type") ?? "") || params.get("resource") !== new URL(this.resource).href) throw new TokenAttemptError();
    if (!this.dir) throw new TokenAttemptError();
    withOAuthStorageLock(this.dir, () => {
      if (params.get("grant_type") === "refresh_token") {
        const refresh = params.get("refresh_token");
        if (!refresh || this.readTokens(true)?.refresh_token !== refresh) throw new TokenAttemptError();
      } else if (!params.get("code")) throw new TokenAttemptError();
      const attempt = randomBytes(16).toString("hex");
      this.save("tokens.json", { version: 2, phase: "consuming", attempt });
      this.attempt = attempt;
    });
  }
  saveCodeVerifier(verifier: string): void { this.allowAuthorization(); this.verifier = verifier; }
  codeVerifier(): string { if (!this.verifier) throw new Error("MCP OAuth verifier is unavailable"); return this.verifier; }

  invalidateCredentials(scope: "all" | "client" | "tokens" | "verifier" | "discovery"): void {
    this.allowAuthorization();
    if (this.dir) withOAuthStorageLock(this.dir, () => {
      try {
        for (const [kind, file] of [["client", "client.json"], ["tokens", "tokens.json"]] as const) if (scope === "all" || scope === kind) rmSync(join(this.dir!, file), { force: true });
      } catch { throw new Error("MCP OAuth credential record cannot be removed"); }
    });
    this.records.clear();
    if (scope === "all" || scope === "verifier") this.verifier = undefined;
    if (scope === "all" || scope === "discovery") { this.discovery = undefined; this.dir = undefined; }
  }

  async redirectToAuthorization(url: URL): Promise<void> {
    this.allowAuthorization();
    const metadata = this.discovery?.authorizationServerMetadata;
    if (!metadata || !httpsEndpoint(url.href) || url.searchParams.get("state") !== this.state() || url.searchParams.get("redirect_uri") !== this.redirectUrl || url.origin !== new URL(metadata.authorization_endpoint!).origin) throw new Error("MCP OAuth authorization redirect is invalid");
    this.callback.arm(this.discovery!.authorizationServerUrl, "authorization_response_iss_parameter_supported" in metadata && metadata.authorization_response_iss_parameter_supported === true);
    this.redirected = true;
    await this.openUrl(url.href);
  }
}

type OAuthConnectOptions = { signal?: AbortSignal; port?: number; timeoutMs?: number; fetch?: FetchLike; openUrl?: (url: string) => void | Promise<void> };

export async function connectWithOAuth(client: Client, server: string, url: string, options: OAuthConnectOptions = {}): Promise<StreamableHTTPClientTransport> {
  const callback = new McpOAuthCallback(), controller = new AbortController();
  const stopped = () => { callback.cancel(new Error("MCP OAuth connection canceled")); controller.abort(); };
  options.signal?.addEventListener("abort", stopped, { once: true });
  let transport: StreamableHTTPClientTransport | undefined, provider: NekoOAuthProvider | undefined, connected = false;
  const timer = setTimeout(() => { callback.cancel(new Error("MCP OAuth authorization timed out")); controller.abort(); }, options.timeoutMs ?? 120_000);
  const fetchFn: FetchLike = async (input, init) => {
    const signal = AbortSignal.any([controller.signal, ...(init?.signal ? [init.signal] : []), ...(options.signal ? [options.signal] : [])]);
    throwIfAborted(signal);
    const target = new URL(input instanceof Request ? input.url : String(input));
    // SDK resource requests advertise SSE; discovery, registration and token requests do not,
    // including when a server uses the same URL for resource and OAuth endpoints.
    const resourceRequest = target.href === new URL(url).href && new Headers(init?.headers).get("accept")?.includes("text/event-stream");
    if (!resourceRequest) provider?.beforeTokenRequest(input, init, signal);
    const response = await (options.fetch ?? fetch)(input, { ...init, redirect: "error", credentials: "omit", signal });
    // The SDK refreshes and resends the original RPC on 401/403. After initialization that
    // RPC may already have mutated remote state; require an explicit reconnect instead.
    if (connected && (response.status === 401 || response.status === 403)) {
      void response.body?.cancel().catch(() => {});
      throw new Error("MCP OAuth authorization is required; reconnect explicitly. Request outcome is unknown.");
    }
    if (resourceRequest && response.ok) return response;
    const limit = !response.ok || init?.method === "POST" ? 64 * 1024 : 1024 * 1024;
    try { return await boundedOAuthResponse(response, signal, limit); }
    catch (error) { controller.abort(); throw error; }
  };
  try {
    if (options.signal?.aborted) throw new Error("MCP OAuth connection canceled");
    await callback.listen(options.port ?? 41789);
    provider = new NekoOAuthProvider(server, url, callback, options.openUrl);
    transport = new StreamableHTTPClientTransport(new URL(url), { authProvider: provider, fetch: fetchFn });
    try {
      await client.connect(transport);
    } catch (error) {
      if (error instanceof TokenAttemptError) throw error;
      if (!(error instanceof UnauthorizedError) || !provider.redirected) throw new Error("MCP OAuth connection failed before authorization");
      const code = await callback.code;
      await client.close();
      try {
        await transport.finishAuth(code);
        transport = new StreamableHTTPClientTransport(new URL(url), { authProvider: provider, fetch: fetchFn });
        await client.connect(transport);
      } catch (error) {
        if (error instanceof TokenAttemptError) throw error;
        throw new Error("MCP OAuth authorization could not finish; reconnect explicitly");
      }
    }
    connected = true;
    return transport;
  } finally {
    clearTimeout(timer);
    options.signal?.removeEventListener("abort", stopped);
    await callback.close();
    if (!connected) { controller.abort(); await transport?.close(); await client.close(); }
  }
}
