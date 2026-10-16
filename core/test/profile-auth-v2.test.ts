import { afterEach, beforeEach, expect, test } from "bun:test";
import { linkSync, mkdirSync, mkdtempSync, readFileSync, symlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { brokerCoreProfiles, publicCoreProfilesV2, type PublicCoreProfileV2 } from "../src/adapters/profile-auth-v2.ts";
import { loadProfileBridge, publicCoreProfiles } from "../src/adapters/profile-bridge.ts";
import { getProvider } from "../src/adapters/providers.ts";
import { CredentialOutputFilter, redactCredentialJSON } from "../src/shared/credential-output.ts";
import { scrubChildEnv } from "../src/shared/child-env.ts";
import { deniedCredentialPath, requiresPolicyAwareSearch } from "../src/core/read-policy.ts";
import type { JsonObject } from "../src/shared/wire.ts";
import type { Provider } from "../src/core/ports.ts";

const KEY = "2".repeat(64), ENV_KEY = "fixture-env-captured", ACCESS = "fixture-cached-access", REFRESH = "fixture-cached-refresh";
const saved = { ...process.env };
let source: string, state: string, capsule: string, config: string, token: string;
beforeEach(() => {
  for (const name of Object.keys(process.env)) if (name.startsWith("NEKO_") || name.startsWith("FIXTURE_")) delete process.env[name];
  const root = mkdtempSync(join(tmpdir(), "core-auth-v2-"));
  source = join(root, "source"); state = join(root, "state"); const privateRoot = join(root, "private-registry");
  mkdirSync(join(source, ".neko-core"), { recursive: true }); mkdirSync(state); mkdirSync(privateRoot);
  config = join(source, ".neko-core", "config.json"); token = join(source, ".neko-core", "kimi-auth.json"); capsule = join(privateRoot, "env-capsule.json");
  process.env.HOME = state; process.env.USERPROFILE = state; process.env.NEKO_CREDENTIAL_HOME = source; process.env.NEKO_PROFILE_REVISION_KEY = KEY;
  process.env.NEKO_PROFILE_LIMITS = JSON.stringify({ maxInputBytes: 16384, maxOutputTokens: 4096, maxRequests: 3, maxSteps: 4, maxSeconds: 10 });
  writeConfig(); writeToken();
});
afterEach(() => { for (const name of Object.keys(process.env)) if (!(name in saved)) delete process.env[name]; Object.assign(process.env, saved); });
function writeConfig(endpoint = "http://127.0.0.1:1/v1", extra: JsonObject = {}) {
  writeFileSync(config, JSON.stringify({ active_profile: "fixture_env", profiles: {
    fixture_env: { provider: "openai_compat", model: "fixture-model", base_url: endpoint, key_env: "FIXTURE_ENV_KEY", key_env_fallbacks: ["FIXTURE_FALLBACK"] },
    fixture_file: { provider: "openai_compat", model: "fixture-model", base_url: endpoint, api_key: "fixture-file" },
    fixture_local: { provider: "openai_compat", auth: "none", model: "fixture-model", base_url: endpoint },
    fixture_kimi: { provider: "kimi", auth: "kimi_oauth", model: "fixture-model", base_url: endpoint },
  }, ...extra }));
}
function writeToken(seconds = Math.floor(Date.now()/1000) + 1200) { writeFileSync(token, JSON.stringify({ access_token: ACCESS, refresh_token: REFRESH, expires_at: seconds })); }
function broker(extra: JsonObject = {}) { return brokerCoreProfiles({ version: 1, sourceId: "fixture_source", credentialHome: source, stateHome: state, capsulePath: capsule, revisionKey: KEY, expiresAtUnixMs: Date.now() + 120000, ...extra }); }
function view() { return publicCoreProfilesV2({ capsulePath: capsule }); }
function pin(row: PublicCoreProfileV2) {
  process.env.NEKO_PROFILE_ORIGIN = row.providerOrigin; process.env.NEKO_PROFILE_AUTH_POLICY = row.authPolicy; process.env.NEKO_PROFILE_CREDENTIAL_SOURCE = row.credentialSource;
  process.env.NEKO_PROFILE_DEADLINE_UNIX_MS = String(Math.min(Date.now() + 10000, row.credentialExpiresAtUnixMs ?? Number.MAX_SAFE_INTEGER));
  if (row.credentialExpiresAtUnixMs) process.env.NEKO_PROFILE_CREDENTIAL_EXPIRES_AT_UNIX_MS = String(row.credentialExpiresAtUnixMs); else delete process.env.NEKO_PROFILE_CREDENTIAL_EXPIRES_AT_UNIX_MS;
  if (row.credentialSnapshotId) { process.env.NEKO_PROFILE_CAPSULE_PATH = capsule; process.env.NEKO_PROFILE_SNAPSHOT_ID = row.credentialSnapshotId; }
  else { delete process.env.NEKO_PROFILE_CAPSULE_PATH; delete process.env.NEKO_PROFILE_SNAPSHOT_ID; }
  return loadProfileBridge({ profile: row.id, expectedRevision: row.profileRevision });
}

test("broker seals configured environment and emits secret-free exact v2 metadata", () => {
  process.env.FIXTURE_ENV_KEY = ENV_KEY; process.env.FIXTURE_UNRELATED = "never-captured";
  const report = broker(); const env = report.profiles.find(p => p.id === "fixture_env")!;
  expect(report.version).toBe(2); expect(report.sourceSnapshot?.id).toMatch(/^cs1_[a-f0-9]{32}$/);
  expect(env).toMatchObject({ credentialSource: "process_env", authPolicy: "static_key", bridgeTransport: "supported", credentialReadiness: "not_checked" });
  expect(env.credentialSnapshotId).toBe(report.sourceSnapshot!.id); expect(env.credentialExpiresAtUnixMs).toBe(report.sourceSnapshot!.expiresAtUnixMs);
  const encoded = JSON.stringify(report) + readFileSync(capsule, "utf8") + readFileSync(capsule + ".current", "utf8");
  for (const value of [ENV_KEY, ACCESS, REFRESH, KEY, source, state, "FIXTURE_ENV_KEY", "never-captured"]) expect(encoded.includes(value)).toBe(false);
});

test("clear-env Verify reuses snapshot; task credential is private memory", () => {
  process.env.FIXTURE_ENV_KEY = ENV_KEY; const report = broker(); delete process.env.FIXTURE_ENV_KEY;
  const row = report.profiles.find(p => p.id === "fixture_env")!; const bytes = readFileSync(capsule);
  for (let i = 0; i < 3; i++) expect(view()).toEqual(report);
  expect(readFileSync(capsule).equals(bytes)).toBe(true);
  const cfg = pin(row); expect(cfg.apiKey === ENV_KEY).toBe(true); expect(JSON.stringify(cfg.data).includes(ENV_KEY)).toBe(false);
  expect(process.env.FIXTURE_ENV_KEY).toBeUndefined(); expect(cfg.resolvedHome).toBe(state);
});

test("file/local revisions ignore global snapshot and execute without capsule", () => {
  const without = publicCoreProfilesV2(); const withCapsule = broker();
  for (const id of ["fixture_file", "fixture_local"]) {
    const a = without.profiles.find(p => p.id === id)!, b = withCapsule.profiles.find(p => p.id === id)!;
    expect(a).toEqual(b); expect(a.credentialSnapshotId).toBeUndefined(); expect(a.credentialExpiresAtUnixMs).toBeUndefined();
    expect(() => getProvider(pin(a))).not.toThrow();
  }
  expect(publicCoreProfiles().version).toBe(1);
});

test("new capture retires old snapshot; pair mismatch and valid old-pair replay deny current id", () => {
  process.env.FIXTURE_ENV_KEY = ENV_KEY; const first = broker(); const cfg = pin(first.profiles.find(p => p.id === "fixture_env")!);
  const bytes = readFileSync(capsule), marker = readFileSync(capsule + ".current");
  process.env.FIXTURE_ENV_KEY = ENV_KEY + "-rotated"; const second = broker();
  expect(second.sourceSnapshot!.id).not.toBe(first.sourceSnapshot!.id); expect(() => getProvider(cfg)).toThrow();
  writeFileSync(capsule, bytes); expect(() => view()).toThrow();
  writeFileSync(capsule + ".current", marker);
  expect(() => publicCoreProfilesV2({ capsulePath: capsule, snapshotId: second.sourceSnapshot!.id })).toThrow();
});

test("capsule marker, key, snapshot and source connection changes deny before provider", () => {
  process.env.FIXTURE_ENV_KEY = ENV_KEY; const row = broker().profiles.find(p => p.id === "fixture_env")!;
  const original = readFileSync(capsule, "utf8"); const marker = readFileSync(capsule + ".current", "utf8");
  for (const path of [capsule, capsule + ".current"]) {
    const value = JSON.parse(readFileSync(path, "utf8")); value.snapshotId = "cs1_" + "0".repeat(32); writeFileSync(path, JSON.stringify(value));
    expect(() => pin(row)).toThrow(); writeFileSync(capsule, original); writeFileSync(capsule + ".current", marker);
  }
  expect(() => publicCoreProfilesV2({ capsulePath: capsule, revisionKey: "3".repeat(64) })).toThrow();
  writeConfig("http://127.0.0.1:2/v1"); expect(() => pin(row)).toThrow();
});

test("broker bounds schema/time and never captures ambient key override", () => {
  const invalid: JsonObject[] = [{ extra: true }, { expiresAtUnixMs: Date.now() + 301000 }, { expiresAtUnixMs: 1 }, { sourceId: "bad/id" }, { revisionKey: "bad" }];
  for (const extra of invalid) expect(() => broker(extra)).toThrow();
  process.env.NEKO_API_KEY = ENV_KEY;
  expect(broker().profiles.find(p => p.id === "fixture_env")!.bridgeTransport).toBe("unsupported");
});

test("actual CLI broker/Verify emit only versioned DTO and leave inspection read-only", () => {
  process.env.FIXTURE_ENV_KEY = ENV_KEY;
  const input = { version: 1, sourceId: "fixture_source", credentialHome: source, stateHome: state, capsulePath: capsule, revisionKey: KEY, expiresAtUnixMs: Date.now() + 120000 };
  const capture = Bun.spawnSync([process.execPath, "bin/neko.ts", "profiles", "--json", "--broker-stdin"], { env: { ...process.env }, stdin: Buffer.from(JSON.stringify(input)), stdout: "pipe", stderr: "pipe" });
  expect(capture.exitCode).toBe(0);
  const dto = JSON.parse(capture.stdout.toString()); expect(dto.version).toBe(2); expect(capture.stdout.toString().includes(ENV_KEY)).toBe(false);
  const original = readFileSync(capsule); const marker = readFileSync(capsule + ".current"); delete process.env.FIXTURE_ENV_KEY;
  const verify = Bun.spawnSync([process.execPath, "bin/neko.ts", "profiles", "--json", "--profile-v2", "--credential-home", source], {
    env: { ...process.env, NEKO_PROFILE_CAPSULE_PATH: capsule, NEKO_PROFILE_SNAPSHOT_ID: dto.sourceSnapshot.id }, stdout: "pipe", stderr: "pipe",
  });
  expect(verify.exitCode).toBe(0); expect(JSON.parse(verify.stdout.toString())).toEqual(dto);
  expect(readFileSync(capsule).equals(original)).toBe(true); expect(readFileSync(capsule + ".current").equals(marker)).toBe(true);
});

test("capsule refuses source/state roots, aliases and hardlinks", () => {
  expect(() => broker({ capsulePath: join(source, "capsule.json") })).toThrow();
  expect(() => broker({ capsulePath: join(state, "capsule.json") })).toThrow();
  const alias = join(state, "registry-alias"); symlinkSync(dirname(capsule), alias, "junction");
  expect(() => broker({ capsulePath: join(alias, "capsule.json") })).toThrow();
  broker(); linkSync(capsule, capsule + ".hardlink"); expect(() => view()).toThrow();
});

test("capsule paths/auth-control env are hidden from tools and printable config", () => {
  process.env.FIXTURE_ENV_KEY = ENV_KEY; const cfg = pin(broker().profiles.find(p => p.id === "fixture_env")!);
  expect(deniedCredentialPath(capsule)).toBe("Core credential capsule"); expect(deniedCredentialPath(capsule + ".current")).toBe("Core credential capsule");
  expect(requiresPolicyAwareSearch(dirname(capsule))).toBe(true);
  const child = scrubChildEnv(process.env, cfg.childSecretEnvNames);
  for (const name of Object.keys(process.env).filter(n => n.startsWith("NEKO_PROFILE_"))) expect(child[name]).toBeUndefined();
});

test("cached Kimi supports file-only or capsule policy with token expiry minus 300s", () => {
  const seconds = Math.floor(Date.now()/1000) + 1200; writeToken(seconds);
  const row = publicCoreProfilesV2().profiles.find(p => p.id === "fixture_kimi")!;
  expect(row).toMatchObject({ credentialSource: "oauth_cache", authPolicy: "cached_token_no_refresh", bridgeTransport: "supported", credentialExpiresAtUnixMs: seconds * 1000 - 300000 });
  expect(row.credentialSnapshotId).toBeUndefined(); const cfg = pin(row);
  expect(cfg.apiKey === ACCESS).toBe(true); expect(cfg.credentialSecrets.includes(REFRESH)).toBe(true);
  const bounded = broker().profiles.find(p => p.id === "fixture_kimi")!;
  expect(bounded.credentialSnapshotId).toBeDefined(); expect(bounded.credentialExpiresAtUnixMs).toBeLessThan(row.credentialExpiresAtUnixMs!);
});

test("Kimi expiry/rotation or mismatched source/policy/expiry deny a pin", () => {
  const row = publicCoreProfilesV2().profiles.find(p => p.id === "fixture_kimi")!; const cfg = pin(row);
  expect(() => pin({ ...row, authPolicy: "static_key" })).toThrow();
  expect(() => pin({ ...row, credentialSource: "file_api_key" })).toThrow();
  expect(() => pin({ ...row, credentialExpiresAtUnixMs: row.credentialExpiresAtUnixMs! + 1 })).toThrow();
  pin(row); writeToken(Math.floor(Date.now()/1000) + 300); expect(() => getProvider(cfg)).toThrow();
  expect(publicCoreProfilesV2().profiles.find(p => p.id === "fixture_kimi")!.bridgeTransport).toBe("unsupported");
});

test("genuine cached Kimi loopback HTTP401 cannot refresh/retry/write source", async () => {
  let count = 0, authorized = false;
  const server = Bun.serve({ hostname: "127.0.0.1", port: 0, fetch(req) { count++; authorized = req.headers.get("authorization") === "Bearer " + ACCESS;
    return Response.json({ error: { message: "synthetic denial" } }, { status: 401 }); } });
  writeConfig("http://127.0.0.1:" + server.port + "/v1"); const sourceBytes = readFileSync(token);
  const row = publicCoreProfilesV2().profiles.find(p => p.id === "fixture_kimi")!;
  try { await expect(getProvider(pin(row)).complete([{ role: "user", content: "synthetic request" }])).rejects.toThrow("HTTP 401"); }
  finally { server.stop(true); }
  expect(count).toBe(1); expect(authorized).toBe(true); expect(readFileSync(token).equals(sourceBytes)).toBe(true);
});

test("runtime cannot relabel cached Kimi as a refresh-capable route", () => {
  const row = publicCoreProfilesV2().profiles.find(p => p.id === "fixture_kimi")!;
  const cfg = pin(row); cfg.profiles.fixture_kimi!.auth = "api_key";
  expect(() => getProvider(cfg)).toThrow();
  const fresh = pin(row);
  expect(() => { fresh.profileBridge!.authPolicy = "static_key"; }).toThrow();
});

test("source recapture during stream stops public output", async () => {
  process.env.FIXTURE_ENV_KEY = ENV_KEY; const cfg = pin(broker().profiles.find(p => p.id === "fixture_env")!); let emitted = false;
  const delegate: Provider = { complete: async (_messages, _tools, delta) => {
    broker(); delta?.("must not escape"); return { content: "must not escape", tool_calls: [] };
  } };
  await expect(cfg.profileBridge!.complete(delegate, cfg, [[], undefined, () => { emitted = true; }])).rejects.toThrow(); expect(emitted).toBe(false);
});

test("absolute auth deadline cancels silent provider without waiting declared maxSeconds", async () => {
  const cfg = pin(publicCoreProfilesV2().profiles.find(p => p.id === "fixture_file")!);
  process.env.NEKO_PROFILE_DEADLINE_UNIX_MS = String(Date.now() + 200);
  const bounded = loadProfileBridge({ profile: cfg.profile!, expectedRevision: publicCoreProfilesV2().profiles.find(p => p.id === "fixture_file")!.profileRevision });
  const hanging: Provider = { complete: async (_messages, _tools, _delta, signal) => new Promise((_resolve, reject) => {
    signal!.addEventListener("abort", () => reject(signal!.reason), { once: true });
  }) };
  const began = performance.now();
  await expect(bounded.profileBridge!.complete(hanging, bounded, [[]])).rejects.toThrow("time limit");
  expect(performance.now() - began).toBeLessThan(1000);
});

test("access+refresh are redacted across splits, mixed channels and nested public JSON", () => {
  for (const secret of [ACCESS, REFRESH]) for (let i = 1; i < secret.length; i++) {
    const filter = new CredentialOutputFilter([ACCESS, REFRESH]);
    const chunks = [...filter.push("safe " + secret.slice(0, i), "content"), ...filter.push(secret.slice(i) + " done", "reasoning"), ...filter.flush()];
    const text = chunks.map(c => c.text).join(""); expect(text.includes(ACCESS)).toBe(false); expect(text.includes(REFRESH)).toBe(false); expect(text.includes("safe")).toBe(true);
  }
  expect(JSON.stringify(redactCredentialJSON({ one: ACCESS, two: REFRESH }, [ACCESS, REFRESH])).includes(REFRESH)).toBe(false);
});
