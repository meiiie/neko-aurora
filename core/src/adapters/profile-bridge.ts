import { createHmac, timingSafeEqual } from "node:crypto";
import { existsSync, lstatSync, readFileSync, realpathSync } from "node:fs";
import { join, relative, resolve, sep } from "node:path";
import { credentialHomeDir } from "../shared/credential-home.ts";
import { homeDir } from "../shared/home.ts";
import { isJsonObject, isJsonArray, isText, type JsonObject, type JsonValue } from "../shared/wire.ts";
import { DEFAULTS, NekoConfig, type Profile } from "./config.ts";
import { parseProfileBridgeLimits, ProfileBridgeBudget } from "./profile-bridge-budget.ts";
import { loadProfileBridgeV2, type ProfileAuthOptions } from "./profile-auth-v2.ts";

const ID = /^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$/;
const MODEL = /^[A-Za-z0-9][A-Za-z0-9_./:@+-]{0,119}$/;
const PROVIDERS = new Set(["openai_compat", "responses", "anthropic", "chatgpt", "gemini_cli", "kimi", "opencode", "opencode_account", "cline_account", "moa"]);
const AUTH = new Set(["api_key", "chatgpt_oauth", "gemini_oauth", "grok_oauth", "kimi_oauth", "opencode_oauth", "cline_oauth", "none"]);
const CONNECTION = ["provider", "base_url", "model", "auth", "api_key", "key_env", "key_env_fallbacks"] as const;
const AUTH_FILES = new Map<string, string[]>([
  ["chatgpt_oauth", [".neko-core/chatgpt-auth.json"]], ["grok_oauth", [".neko-core/grok-auth.json"]],
  ["kimi_oauth", [".neko-core/kimi-auth.json"]], ["opencode_oauth", [".neko-core/opencode-auth.json"]],
  ["cline_oauth", [".neko-core/cline-auth.json"]], ["gemini_oauth", [".neko-core/gemini-home/oauth_creds.json"]],
]);

export interface PublicCoreProfile {
  id: string;
  provider: string;
  model: string;
  authKind: string;
  active: boolean;
  configurationStatus: "configured" | "incomplete";
  credentialReadiness: "not_checked";
  profileRevision: string;
  providerOrigin: string;
  bridgeTransport: "supported" | "unsupported";
}

export interface SourceProfile { id: string; connection: JsonObject; authKind: string; origin: string; active: boolean }
export interface ProfileBridgeOptions extends ProfileAuthOptions { credentialHome?: string; revisionKey?: string; profile?: string; expectedRevision?: string; home?: string }

function fail(): never { throw new Error("Invalid profile bridge source"); }
export function keyOf(options: ProfileBridgeOptions): string {
  const key = options.revisionKey ?? process.env.NEKO_PROFILE_REVISION_KEY ?? "";
  if (!/^[0-9a-f]{64}$/.test(key)) throw new Error("Profile bridge revision key required");
  return key;
}

export function sourceBytes(home: string, name: string, max = 262_144): Buffer | null {
  const path = join(home, name);
  let current = home;
  for (const part of relative(home, path).split(sep)) {
    current = join(current, part);
    if (existsSync(current) && lstatSync(current).isSymbolicLink()) fail();
  }
  if (!existsSync(path)) return null;
  const stat = lstatSync(path);
  if (!stat.isFile() || stat.nlink !== 1 || stat.size > max) fail();
  if (relative(home, realpathSync.native(path)).startsWith("..")) fail();
  const bytes = readFileSync(path);
  if (bytes.length > max) fail();
  return bytes;
}

function sourceObject(home: string, name: string): JsonObject {
  const bytes = sourceBytes(home, name);
  if (!bytes) return {};
  try {
    // SAFETY: JSON syntax yields only the JsonValue domain, narrowed to an object below.
    const parsed = JSON.parse(bytes.toString("utf8").replace(/^\uFEFF/, "")) as JsonValue;
    if (!isJsonObject(parsed)) fail();
    return parsed;
  } catch { return fail(); }
}

function fields(value: JsonValue | undefined): JsonObject {
  if (!isJsonObject(value)) return {};
  const object = value;
  return Object.fromEntries(CONNECTION.filter((key) => Object.hasOwn(object, key)).map((key) => [key, object[key]]));
}

export function profilesAt(home: string, allowEnvironment = true): SourceProfile[] {
  const a = sourceObject(home, ".neko-core/config.json");
  const b = sourceObject(home, "neko.json");
  const profileMap = (value: JsonValue | undefined): JsonObject => isJsonObject(value) ? value : {};
  const custom = { ...profileMap(a.profiles), ...profileMap(b.profiles) };
  const ids = [...new Set([...Object.keys(DEFAULTS.profiles), ...Object.keys(custom)])].sort();
  if (ids.length > 64 || ids.some((id) => !ID.test(id) || ["__proto__", "constructor", "prototype"].includes(id))) fail();
  const active = b.active_profile ?? a.active_profile ?? null;
  if (active !== null && (!isText(active) || !ids.includes(active))) fail();
  const profiles = ids.map((id) => {
    const connection = { ...fields(DEFAULTS.profiles[id]), ...fields(a.profiles && profileMap(a.profiles)[id]), ...fields(b.profiles && profileMap(b.profiles)[id]) };
    if (id === active) Object.assign(connection, fields(a), fields(b));
    const provider = connection.provider ?? "";
    connection.provider = provider;
    const model = connection.model ?? "";
    const authKind = String(connection.auth ?? (provider === "chatgpt" ? "chatgpt_oauth" : provider === "gemini_cli" ? "gemini_oauth" : "api_key"));
    if (!isText(provider) || (provider !== "" && !PROVIDERS.has(provider)) || !isText(model) || (model && !MODEL.test(model)) || !AUTH.has(authKind)) fail();
    if (connection.api_key !== undefined && (!isText(connection.api_key) || connection.api_key.length > 16_384)) fail();
    for (const env of [connection.key_env, ...(isJsonArray(connection.key_env_fallbacks) ? connection.key_env_fallbacks : [])]) {
      if (env !== undefined && (!isText(env) || !/^[A-Z][A-Z0-9_]{0,63}$/.test(env))) fail();
    }
    if (connection.key_env_fallbacks !== undefined && (!isJsonArray(connection.key_env_fallbacks) || connection.key_env_fallbacks.length > 8)) fail();
    const rawEndpoint = connection.base_url ?? "";
    if (!isText(rawEndpoint) || rawEndpoint.length > 2048) fail();
    let origin = "";
    if (rawEndpoint) {
      try {
        const url = new URL(rawEndpoint);
        if (!["http:", "https:"].includes(url.protocol) || url.username || url.password || url.search || url.hash) fail();
        origin = url.origin;
        if (Buffer.byteLength(origin) > 320) fail();
      } catch { fail(); }
    }
    return { id, connection, authKind, origin, active: id === active };
  });
  const secrets = new Set<string>();
  const collect = (value: JsonValue | undefined, key = "", depth = 0): void => {
    if (depth > 16) fail();
    if (isText(value) && /(api.?key|token|password|credential|secret|account.?id|user.?id|email)/i.test(key) && value.length >= 4) secrets.add(value);
    else if (isJsonObject(value) || isJsonArray(value)) for (const [name, child] of Object.entries(value)) collect(child, name, depth + 1);
  };
  collect(a); collect(b);
  for (const profile of profiles) {
    const privateKey = allowEnvironment ? apiKeyFor(profile) : String(profile.connection.api_key ?? "").trim(); if (privateKey.length >= 4) secrets.add(privateKey);
    for (const name of AUTH_FILES.get(profile.authKind) ?? []) {
      const bytes = sourceBytes(home, name);
      if (bytes) { try {
        // SAFETY: the source is bounded JSON bytes; this traversal emits no payload values.
        collect(JSON.parse(bytes.toString("utf8")) as JsonValue); } catch { /* A malformed token is not declared ready by metadata. */ } }
    }
  }
  for (const profile of profiles) {
    const publicFields = [profile.id, String(profile.connection.provider), String(profile.connection.model ?? ""), profile.origin];
    if (publicFields.some((value) => [...secrets].some((secret) => value.includes(secret)))) fail();
  }
  return profiles;
}

function apiKeyFor(profile: SourceProfile): string {
  if (profile.connection.api_key) return String(profile.connection.api_key).trim();
  for (const name of [profile.connection.key_env, ...(isJsonArray(profile.connection.key_env_fallbacks) ? profile.connection.key_env_fallbacks : [])]) {
    if (isText(name) && process.env[name]?.trim()) return process.env[name]!.trim();
  }
  return "";
}

function revision(home: string, profile: SourceProfile, key: string): string {
  const hmac = createHmac("sha256", Buffer.from(key, "hex"));
  const connection = Object.fromEntries(CONNECTION.map((field) => [field, profile.connection[field] ?? null]));
  hmac.update(JSON.stringify({ version: 1, sourceHome: process.platform === "win32" ? home.toLowerCase() : home, id: profile.id, connection, keyValue: apiKeyFor(profile) }));
  for (const name of AUTH_FILES.get(profile.authKind) ?? []) {
    const bytes = sourceBytes(home, name);
    hmac.update(JSON.stringify({ name, present: bytes !== null, length: bytes?.length ?? 0 }));
    if (bytes) hmac.update(bytes);
  }
  return `pr1_${hmac.digest("hex")}`;
}

function supported(profile: SourceProfile): boolean {
  if (profile.authKind !== "api_key" && profile.authKind !== "none") return false;
  const fileKey = isText(profile.connection.api_key) && Boolean(profile.connection.api_key.trim());
  const localNoKey = /^http:\/\/(127\.0\.0\.1|localhost|\[::1\])(?::|$)/i.test(profile.origin) && !profile.connection.key_env;
  return (fileKey || localNoKey) && ["openai_compat", "responses", "anthropic", "kimi"].includes(String(profile.connection.provider));
}

export interface PublicCoreProfileCatalog { version: 1; profiles: PublicCoreProfile[] }

export function publicCoreProfiles(options: ProfileBridgeOptions = {}): PublicCoreProfileCatalog {
  const home = credentialHomeDir(options.credentialHome);
  if (options.credentialHome || process.env.NEKO_CREDENTIAL_HOME) isolatedStateHome(home, options.home ?? homeDir());
  const key = keyOf(options);
  return { version: 1, profiles: profilesAt(home).map((profile) => ({
    id: profile.id, provider: String(profile.connection.provider), model: String(profile.connection.model ?? ""),
    authKind: profile.authKind, active: profile.active,
    configurationStatus: profile.connection.provider && profile.connection.model && profile.origin ? "configured" : "incomplete",
    credentialReadiness: "not_checked", profileRevision: revision(home, profile, key),
    providerOrigin: profile.origin, bridgeTransport: supported(profile) ? "supported" : "unsupported",
  })) };
}

export function isolatedStateHome(source: string, home: string): string {
  const stateHome = credentialHomeDir(home);
  const rel = relative(source, stateHome);
  const back = relative(stateHome, source);
  const nested = (value: string) => value === "" || (!value.startsWith(".." + sep) && value !== ".." && !/^[A-Za-z]:|^[\\/]/.test(value));
  if (nested(rel) || nested(back)) throw new Error("Profile bridge requires an isolated task home");
  return stateHome;
}

export function loadProfileBridge(options: ProfileBridgeOptions): NekoConfig {
  // The trusted launcher (NekoBot host) selects the profile via NEKO_PROFILE;
  // fall back to it when no saved session profile or CLI --profile exists.
  const envProfile = options.profile ?? (process.env.NEKO_PROFILE?.trim() || undefined);
  if (envProfile !== options.profile) options = { ...options, profile: envProfile };
  if ((options.expectedRevision ?? process.env.NEKO_PROFILE_REVISION ?? "").startsWith("pr2_")) return loadProfileBridgeV2(options);
  const source = credentialHomeDir(options.credentialHome);
  const stateHome = isolatedStateHome(source, options.home ?? homeDir());
  const key = keyOf(options);
  const expected = options.expectedRevision ?? process.env.NEKO_PROFILE_REVISION ?? "";
  const selected = options.profile;
  if (!selected || !ID.test(selected) || !/^pr1_[0-9a-f]{64}$/.test(expected)) throw new Error("Profile bridge pin required");
  if (["NEKO_API_KEY", "NEKO_BASE_URL", "NEKO_PROVIDER", "NEKO_MODEL", "NEKO_GEMINI_HOME"].some((name) => process.env[name])) throw new Error("Profile bridge override refused");
  const find = () => profilesAt(source).find((profile) => profile.id === selected) ?? fail();
  const pinned = find();
  const validateRevision = (profile: SourceProfile) => {
    if (!timingSafeEqual(Buffer.from(expected), Buffer.from(revision(source, profile, key)))) throw new Error("Profile bridge revision changed");
  };
  validateRevision(pinned);
  if (!supported(pinned)) throw new Error("profile_bridge_transport_unsupported");
  if (!pinned.connection.provider || !pinned.connection.model || !pinned.origin) throw new Error("Profile bridge configuration incomplete");
  if (process.env.NEKO_PROFILE_ORIGIN !== pinned.origin) throw new Error("Profile bridge origin pin changed");
  const limits = parseProfileBridgeLimits();
  const data = structuredClone(DEFAULTS);
  delete data.profiles; delete data.active_profile; delete data.api_key;
  for (const field of ["provider", "base_url", "model"]) data[field] = pinned.connection[field];
  data.auto_update = false; data.auto_update_check = false; data.mcp_servers = {}; data.mode = "default"; data.read_outside_root = false;
  data.max_steps = limits.maxSteps; data.max_retries = 0; data.offline_retry_seconds = 0;
  data.timeout_seconds = limits.maxSeconds;
  // SAFETY: authKind was checked against the closed AUTH enum at source parsing.
  const profile: Profile = { provider: String(pinned.connection.provider), model: String(pinned.connection.model), auth: pinned.authKind as Profile["auth"] };
  const keyEnvs = [pinned.connection.key_env, ...(isJsonArray(pinned.connection.key_env_fallbacks) ? pinned.connection.key_env_fallbacks : [])].filter((name): name is string => isText(name));
  const config = new NekoConfig(data, selected, { [selected]: profile }, apiKeyFor(pinned), null, keyEnvs, { state: "none", files: [] }, stateHome);
  const budget = new ProfileBridgeBudget(config.resolvedHome, expected, limits, pinned.origin);
  const assertCurrent = (cfg: NekoConfig) => {
    if (process.env.NEKO_PROFILE_ORIGIN !== pinned.origin || credentialHomeDir() !== source || cfg.profile !== selected || cfg.provider !== pinned.connection.provider || cfg.model !== pinned.connection.model || cfg.baseUrl !== String(pinned.connection.base_url).replace(/\/+$/, "")) throw new Error("Profile bridge identity changed");
    validateRevision(find());
  };
  config.profileBridge = {
    assertCurrent,
    complete: (provider, cfg, args) => budget.complete(provider, () => assertCurrent(cfg), (n) => { cfg.data.max_tokens = n; }, args),
  };
  return config;
}
