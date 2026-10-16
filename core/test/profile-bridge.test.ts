import { afterEach, beforeEach, expect, test } from "bun:test";
import { createHmac } from "node:crypto";
import { mkdirSync, mkdtempSync, readFileSync, symlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { loadProfileBridge, publicCoreProfiles } from "../src/adapters/profile-bridge.ts";
import { getProvider } from "../src/adapters/providers.ts";
import { loadConfig } from "../src/adapters/config.ts";
import { loadChatGptCredentials } from "../src/adapters/chatgpt-auth.ts";
import { loadKimiCredentials } from "../src/adapters/kimi-auth.ts";
import { loadClineCredentials } from "../src/adapters/cline-auth.ts";
import { loadGrokCredentials } from "../src/adapters/grok-auth.ts";
import { loadOpenCodeCredentials } from "../src/adapters/opencode-auth.ts";
import { geminiCredentialsPath, startGeminiAcp } from "../src/adapters/gemini-cli.ts";
import { isJsonObject, type JsonObject, type JsonValue } from "../src/shared/wire.ts";
import { homeDir } from "../src/shared/home.ts";
import { scrubChildEnv } from "../src/shared/child-env.ts";
import { deniedCredentialPath, requiresPolicyAwareSearch } from "../src/core/read-policy.ts";
import { parseProfileBridgeLimits } from "../src/adapters/profile-bridge-budget.ts";
import { providerFetch } from "../src/adapters/provider-fetch.ts";

const KEY = createHmac("sha256", "fixture-only-key").update("profile bridge fixture").digest("hex");
const SECRET = "fixture-credential-not-real";
const limits = { maxInputBytes: 16384, maxOutputTokens: 4096, maxRequests: 2, maxSteps: 4, maxSeconds: 10 };
const saved = { ...process.env };
let source: string;
let state: string;
let configPath: string;

beforeEach(() => {
  for (const name of Object.keys(process.env)) {
    if (name.startsWith("NEKO_") || /(API_KEY|TOKEN|PASSWORD|CREDENTIAL|SECRET)/i.test(name)) delete process.env[name];
  }
  const root = mkdtempSync(join(tmpdir(), "core-profile-bridge-"));
  source = join(root, "credential-source"); state = join(root, "task-state");
  mkdirSync(join(source, ".neko-core"), { recursive: true }); mkdirSync(state);
  process.env.USERPROFILE = state; process.env.HOME = state;
  process.env.NEKO_CREDENTIAL_HOME = source; process.env.NEKO_PROFILE_REVISION_KEY = KEY;
  process.env.NEKO_PROFILE_LIMITS = JSON.stringify(limits);
  configPath = join(source, ".neko-core", "config.json");
  writeSource();
});

afterEach(() => {
  for (const name of Object.keys(process.env)) if (!(name in saved)) delete process.env[name];
  Object.assign(process.env, saved);
});

function writeSource(extra: JsonObject = {}, connection: JsonObject = {}): void {
  writeFileSync(configPath, JSON.stringify({
    active_profile: "fixture_bridge", mode: "auto", read_outside_root: true, additional_write_roots: [source],
    mcp_servers: { forbidden: { command: "never-start-this" } },
    profiles: { fixture_bridge: { provider: "openai_compat", model: "fixture-model", base_url: "http://127.0.0.1:1/v1", api_key: SECRET, ...connection } }, ...extra,
  }));
}

function publicProfile() { return publicCoreProfiles().profiles.find((p) => p.id === "fixture_bridge")!; }
function bridge() { process.env.NEKO_PROFILE_ORIGIN = publicProfile().providerOrigin; return loadProfileBridge({ profile: "fixture_bridge", expectedRevision: publicProfile().profileRevision }); }

test("new bridge resolves source profile while all task, memory, policy and MCP roots stay isolated", () => {
  const cfg = bridge();
  expect(cfg.model).toBe("fixture-model"); expect(cfg.apiKey).toBe(SECRET);
  expect(cfg.resolvedHome).toBe(state); expect(homeDir()).toBe(state);
  expect(cfg.mode).not.toBe("auto"); expect(cfg.mcpServers).toEqual({});
  expect(cfg.projectTrust).toEqual({ state: "none", files: [] });
  expect(cfg.additionalWriteRoots).toEqual([join(state, ".neko-core", "research")]);
  expect(JSON.stringify(cfg.data)).not.toContain(source);
  expect(JSON.stringify(cfg.data)).not.toContain(SECRET);
  expect(cfg.maxSteps).toBe(4); expect(cfg.maxRetries).toBe(0);
});

test("public exact schema is bounded, has safe origin, no secrets or source/account data, and does not fetch", () => {
  const old = globalThis.fetch;
  let calls = 0;
  // SAFETY: fixture fetch has the same callable shape; Bun-only preconnect is retained.
  globalThis.fetch = Object.assign(() => { calls++; throw new Error("network forbidden"); }, { preconnect: old.preconnect }) as typeof fetch;
  try {
    writeFileSync(join(source, ".mcp.json"), '{"mcpServers":{"forbidden":{"command":"never"}}}');
    const dto = publicCoreProfiles();
    expect(Object.keys(dto).sort()).toEqual(["profiles", "version"]);
    expect(Object.keys(publicProfile()).sort()).toEqual(["active", "authKind", "bridgeTransport", "configurationStatus", "credentialReadiness", "id", "model", "profileRevision", "provider", "providerOrigin"]);
    expect(dto.profiles.length).toBeLessThanOrEqual(64);
    expect(publicProfile()).toMatchObject({ active: true, providerOrigin: "http://127.0.0.1:1", credentialReadiness: "not_checked", bridgeTransport: "supported" });
    const text = JSON.stringify(dto);
    for (const forbidden of [SECRET, source, state, '"api_key":', "key_env", "base_url", "accountId", "mcp_servers"]) expect(text).not.toContain(forbidden);
    expect(calls).toBe(0);
  } finally { globalThis.fetch = old; }
});

test("opaque keyed revision rotates on connection/credential changes, not unrelated policy; no unkeyed fingerprint", () => {
  const original = publicProfile().profileRevision;
  expect(original).toMatch(/^pr1_[0-9a-f]{64}$/);
  writeSource({ mode: "default", mcp_servers: { other: { command: "ignored" } } });
  expect(publicProfile().profileRevision).toBe(original);
  writeSource({}, { api_key: SECRET + "-rotation" });
  expect(publicProfile().profileRevision).not.toBe(original);
  writeSource({}, { base_url: "http://127.0.0.1:2/v1" });
  expect(publicProfile().profileRevision).not.toBe(original);
  process.env.NEKO_PROFILE_REVISION_KEY = createHmac("sha256", KEY).update("rotated").digest("hex");
  expect(publicProfile().profileRevision).not.toBe(original);
});

test("wrong pin, rotated source, profile/model drift and ambient endpoint/key overrides refuse before provider use", () => {
  expect(() => loadProfileBridge({ profile: "fixture_bridge", expectedRevision: "pr1_" + "0".repeat(64) })).toThrow("revision changed");
  const cfg = bridge();
  expect(() => getProvider(cfg.withModel("other-model"))).toThrow("identity changed");
  process.env.NEKO_API_KEY = "foreign-fixture";
  expect(() => bridge()).toThrow("override refused");
  delete process.env.NEKO_API_KEY;
  writeSource({}, { api_key: SECRET + "-rotated" });
  expect(() => getProvider(cfg)).toThrow("revision changed");
});

test("OAuth/env-only/dynamic/sidecar routes are configured metadata but explicitly unsupported", () => {
  for (const p of publicCoreProfiles().profiles.filter((p) => p.authKind.endsWith("oauth"))) expect(p.bridgeTransport).toBe("unsupported");
  writeSource({}, { api_key: "", key_env: "FIXTURE_ENV_KEY" });
  process.env.FIXTURE_ENV_KEY = SECRET;
  expect(publicProfile().bridgeTransport).toBe("unsupported");
  expect(() => bridge()).toThrow("transport_unsupported");
  expect(() => startGeminiAcp({ path: "not-run", source: "path" })).toThrow("transport_unsupported");
});

test("all Core-owned OAuth readers use explicit credential source; Gemini path separates from state", () => {
  const token = { accessToken: "fixture-access", refreshToken: "fixture-refresh", expiresAt: Date.now() + 600000, expiresIn: 600, access_token: "fixture-access", refresh_token: "fixture-refresh", expires_at: Date.now() + 600000, user_id: "fixture-user", account_id: "fixture-account", email: "fixture@example.invalid" };
  for (const name of ["chatgpt", "kimi", "cline", "grok", "opencode"]) writeFileSync(join(source, ".neko-core", `${name}-auth.json`), JSON.stringify(token));
  expect(loadChatGptCredentials()?.accessToken).toBe(token.accessToken);
  expect(loadKimiCredentials()?.accessToken).toBe(token.accessToken);
  expect(loadClineCredentials()?.accessToken).toBe(token.accessToken);
  expect(loadGrokCredentials()?.accessToken).toBe(token.accessToken);
  expect(loadOpenCodeCredentials()?.accessToken).toBe(token.accessToken);
  expect(geminiCredentialsPath()).toBe(join(source, ".neko-core", "gemini-home", "oauth_creds.json"));
  expect(homeDir()).toBe(state);
});

test("source config and credential paths are shielded, bridge env excluded from config/children, legacy default unchanged", () => {
  expect(deniedCredentialPath(join(source, "neko.json"))).toBe("Core profile bridge source");
  expect(deniedCredentialPath(join(source, ".neko-core", "arbitrary.json"))).toBe("Core profile bridge source");
  expect(requiresPolicyAwareSearch(source)).toBe(true);
  const child = scrubChildEnv(process.env);
  for (const key of ["NEKO_CREDENTIAL_HOME", "NEKO_PROFILE_REVISION", "NEKO_PROFILE_REVISION_KEY", "NEKO_PROFILE_LIMITS", "NEKO_PROFILE_ORIGIN"]) expect(child[key]).toBeUndefined();
  const legacyFile = join(state, "fixture-legacy.json");
  writeFileSync(legacyFile, JSON.stringify({ model: "legacy-model", api_key: "test-key" }));
  expect(JSON.stringify(loadConfig({ path: legacyFile }).data)).not.toContain(KEY);
  delete process.env.NEKO_CREDENTIAL_HOME;
  writeFileSync(join(state, "neko.json"), JSON.stringify({ model: "legacy-model", api_key: "test-key" }));
  expect(loadConfig({ home: state }).model).toBe("legacy-model");
  expect(loadConfig({ home: state }).apiKey).toBe("test-key");
});

test("malformed/bounded metadata rejects endpoint userinfo/query/hash and extra or invalid limits", () => {
  for (const endpoint of ["https://user:pass@example.test/v1", "https://example.test/v1?q=fixture", "https://example.test/v1#fragment", "file:///private", "x".repeat(2049)]) {
    writeSource({}, { base_url: endpoint }); expect(() => publicCoreProfiles()).toThrow("Invalid profile bridge source");
  }
  for (const value of [{ ...limits, extra: 1 }, { ...limits, maxSeconds: 0 }, { ...limits, maxSteps: 513 }, { ...limits, maxRequests: NaN }]) expect(() => parseProfileBridgeLimits(JSON.stringify(value))).toThrow("Invalid profile bridge limits");
});

test("incomplete catalog rows do not guess endpoints or gain execution; source cannot also be task home", () => {
  writeSource({ profiles: { empty_fixture: {} }, active_profile: "empty_fixture" });
  const empty = publicCoreProfiles().profiles.find((p) => p.id === "empty_fixture")!;
  expect(empty).toMatchObject({ provider: "", model: "", providerOrigin: "", configurationStatus: "incomplete", bridgeTransport: "unsupported" });
  writeSource();
  expect(() => loadProfileBridge({ profile: "fixture_bridge", expectedRevision: publicProfile().profileRevision, home: source })).toThrow("isolated task home");
  const alias = join(state, "source-alias");
  symlinkSync(source, alias, "junction");
  expect(() => loadProfileBridge({ profile: "fixture_bridge", expectedRevision: publicProfile().profileRevision, home: alias })).toThrow("Invalid credential home");
  expect(() => publicCoreProfiles({ home: alias })).toThrow("Invalid credential home");
  expect(() => publicCoreProfiles({ home: source })).toThrow("isolated task home");
});

test("credential values cannot be smuggled into allowed public id/model/origin fields", () => {
  writeSource({}, { model: SECRET });
  expect(() => publicCoreProfiles()).toThrow("Invalid profile bridge source");
  writeSource();
  writeFileSync(join(source, ".neko-core", "chatgpt-auth.json"), JSON.stringify({ accessToken: "fixture-model" }));
  expect(() => publicCoreProfiles()).toThrow("Invalid profile bridge source");
});

test("runtime limits reject input and off-origin dispatch before any network, and stale source before next request", async () => {
  const cfg = bridge();
  const old = globalThis.fetch;
  let calls = 0;
  // SAFETY: fixture fetch has the same callable shape; Bun-only preconnect is retained.
  globalThis.fetch = Object.assign(() => { calls++; throw new Error("network forbidden"); }, { preconnect: old.preconnect }) as typeof fetch;
  try {
    await expect(getProvider(cfg).complete([{ role: "user", content: "x".repeat(17000) }])).rejects.toThrow("input limit");
    expect(calls).toBe(0);
    const arbitrary = { complete: async () => {
      await providerFetch("http://127.0.0.1:2/v1/chat/completions", { method: "POST", body: "{}" });
      return { content: "never", tool_calls: [] };
    } };
    await expect(cfg.profileBridge!.complete(arbitrary, cfg, [[]])).rejects.toThrow("origin refused");
    expect(calls).toBe(0);
    writeSource({}, { api_key: SECRET + "-rotated" });
    await expect(getProvider(cfg).complete([])).rejects.toThrow("revision changed");
  } catch (error) {
    if (error instanceof Error && error.message === "Profile bridge revision changed") expect(calls).toBe(0);
    else throw error;
  } finally { globalThis.fetch = old; }
});

test("failed/unknown attempt reservation remains across reconstruction and blocks replay beyond request cap", async () => {
  process.env.NEKO_PROFILE_LIMITS = JSON.stringify({ ...limits, maxRequests: 1 });
  const cfg = bridge();
  const fake = { complete: async () => {
    await providerFetch("http://127.0.0.1:1/v1/chat/completions", { method: "POST", body: "{}" });
    throw new Error("fixture-lost-ACK");
  } };
  const old = globalThis.fetch;
  let calls = 0;
  // SAFETY: fixture fetch has the same callable shape; Bun-only preconnect is retained.
  globalThis.fetch = Object.assign(async () => { calls++; return new Response("{}"); }, { preconnect: old.preconnect }) as typeof fetch;
  try {
    await expect(cfg.profileBridge!.complete(fake, cfg, [[]])).rejects.toThrow("fixture-lost-ACK");
    const resumed = bridge();
    await expect(resumed.profileBridge!.complete(fake, resumed, [[]])).rejects.toThrow("request limit");
    expect(calls).toBe(1);
  } finally { globalThis.fetch = old; }
});

test("output/time caps stop before publishing excess bytes; expired restored budget cannot restart its clock", async () => {
  process.env.NEKO_PROFILE_LIMITS = JSON.stringify({ ...limits, maxOutputTokens: 32, maxRequests: 1 });
  const cfg = bridge();
  let output = "";
  const overflow = { complete: async (...args: Parameters<ReturnType<typeof getProvider>["complete"]>) => {
    args[2]?.("x".repeat(33)); return { content: "x".repeat(33), tool_calls: [] };
  } };
  await expect(cfg.profileBridge!.complete(overflow, cfg, [[], undefined, (text) => { output += text; }])).rejects.toThrow("output limit");
  expect(output).toBe("");
  mkdirSync(join(state, ".neko-core"), { recursive: true });
  writeFileSync(join(state, ".neko-core", "profile-bridge-budget-v1.json"), JSON.stringify({ version: 1, profileRevision: publicProfile().profileRevision,
    limits: { ...limits, maxOutputTokens: 32, maxRequests: 1 }, startedAt: Date.now() - 20000, requests: 0, reservedOutput: 0 }));
  expect(() => bridge()).toThrow("time limit");
});

test("CLI json inspection emits one public DTO, no update cleanup or source/home side effects; ordinary ACP refuses bridge", () => {
  const env = { ...process.env };
  const cli = Bun.spawnSync([process.execPath, "bin/neko.ts", "profiles", "--json", "--credential-home", source], { cwd: process.cwd(), env, stdout: "pipe", stderr: "pipe" });
  expect(cli.exitCode).toBe(0);
  const text = cli.stdout.toString();
  expect(JSON.parse(text).version).toBe(1); expect(text).not.toContain(SECRET); expect(text).not.toContain(KEY);
  const denied = Bun.spawnSync([process.execPath, "bin/neko.ts", "acp", "--credential-home", source], { cwd: process.cwd(), env, stdout: "pipe", stderr: "pipe" });
  expect(denied.exitCode).not.toBe(0); expect(denied.stdout.toString()).toBe("");
  expect(denied.stderr.toString()).not.toContain(SECRET); expect(denied.stderr.toString()).not.toContain(KEY);
});

test("real loopback receives only selected fixture credential; attempts/input/output are bounded and restart retains budget", async () => {
  let calls = 0;
  let bearer = "";
  const server = Bun.serve({ hostname: "127.0.0.1", port: 0, fetch: async (request) => {
    calls++; bearer = request.headers.get("authorization") ?? "";
    // SAFETY: the fake loopback provider only consumes JSON and checks object shape below.
    const body = await request.json() as JsonValue;
    if (!isJsonObject(body)) throw new Error("invalid fixture request");
    expect(Number(body.max_tokens ?? body.max_completion_tokens)).toBeLessThanOrEqual(2048);
    return new Response(`data: ${JSON.stringify({ choices: [{ delta: { content: "fixture reply" }, finish_reason: "stop" }], usage: { completion_tokens: 4 } })}\n\ndata: [DONE]\n\n`, { headers: { "content-type": "text/event-stream" } });
  } });
  try {
    writeSource({}, { base_url: `http://127.0.0.1:${server.port}/v1` });
    const cfg = bridge();
    const provider = getProvider(cfg);
    expect((await provider.complete([{ role: "user", content: "fixture prompt" }])).content).toBe("fixture reply");
    expect(bearer).toBe(`Bearer ${SECRET}`); expect(calls).toBe(1);
    const resumed = getProvider(bridge());
    expect((await resumed.complete([{ role: "user", content: "second fixture prompt" }])).content).toBe("fixture reply");
    expect(calls).toBe(2);
    await expect(getProvider(bridge()).complete([{ role: "user", content: "third" }])).rejects.toThrow("request limit");
    expect(calls).toBe(2);
    const receipt = JSON.parse(readFileSync(join(state, ".neko-core", "profile-bridge-budget-v1.json"), "utf8"));
    expect(receipt.requests).toBe(2); expect(receipt.reservedOutput).toBe(4096);
    expect(JSON.stringify(receipt)).not.toContain(SECRET);
  } finally { server.stop(true); }
});

