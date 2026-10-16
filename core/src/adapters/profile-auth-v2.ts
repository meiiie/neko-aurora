import { createHmac, timingSafeEqual } from "node:crypto";
import { credentialHomeDir } from "../shared/credential-home.ts";
import { homeDir } from "../shared/home.ts";
import { isJsonNumber, isJsonObject, isText, type JsonValue } from "../shared/wire.ts";
import { DEFAULTS, NekoConfig, type Profile } from "./config.ts";
import { isolatedStateHome, keyOf, profilesAt, sourceBytes, type ProfileBridgeOptions, type PublicCoreProfile, type SourceProfile } from "./profile-bridge.ts";
import { ProfileBridgeBudget, parseProfileBridgeLimits } from "./profile-bridge-budget.ts";
import { capsuleConnectionRevision, readCredentialCapsule, writeCredentialCapsule, type CapsuleEntry, type CredentialCapsule, type ProfileSourceSnapshot } from "./profile-credential-capsule.ts";

export type CredentialSource = "file_api_key" | "process_env" | "oauth_cache" | "local_none" | "unavailable";
export type AuthPolicy = "static_key" | "cached_token_no_refresh" | "unavailable";
export interface ProfileAuthOptions {
  capsulePath?: string; snapshotId?: string; credentialSource?: string; authPolicy?: string;
  credentialExpiresAtUnixMs?: number; deadlineUnixMs?: number;
}
export interface PublicCoreProfileV2 extends PublicCoreProfile {
  credentialSource: CredentialSource; authPolicy: AuthPolicy;
  credentialSnapshotId?: string; credentialExpiresAtUnixMs?: number;
}
export interface PublicCoreProfilesV2 { version: 2; profiles: PublicCoreProfileV2[]; sourceSnapshot?: ProfileSourceSnapshot }
interface Material { source: CredentialSource; policy: AuthPolicy; authKind: string; key: string; secrets: string[]; expires?: number; snapshot?: string }
const DIRECT = new Set(["openai_compat", "responses", "anthropic", "kimi"]);
function fail(): never { throw new Error("Profile auth bridge unavailable"); }
function optionalInteger(value: string | undefined): number | undefined {
  if (value === undefined || value === "") return undefined;
  if (!/^[1-9][0-9]*$/.test(value) || !Number.isSafeInteger(Number(value))) fail();
  return Number(value);
}
function capsuleFor(options: ProfileBridgeOptions, home: string, key: string): CredentialCapsule | undefined {
  const path = options.capsulePath ?? process.env.NEKO_PROFILE_CAPSULE_PATH;
  if (!path) return undefined;
  return readCredentialCapsule(path, home, key, options.snapshotId ?? process.env.NEKO_PROFILE_SNAPSHOT_ID, options.home ?? homeDir());
}
function material(profile: SourceProfile, home: string, revisionKey: string, capsule?: CredentialCapsule): Material {
  const unavailable: Material = { source: "unavailable", policy: "unavailable", authKind: profile.authKind, key: "", secrets: [] };
  if (!DIRECT.has(String(profile.connection.provider))) return unavailable;
  const fileKey = String(profile.connection.api_key ?? "").trim();
  if (["api_key", "none"].includes(profile.authKind)) {
    if (fileKey) return { source: "file_api_key", policy: "static_key", authKind: "api_key", key: fileKey, secrets: [fileKey] };
    if (profile.connection.key_env || Array.isArray(profile.connection.key_env_fallbacks) && profile.connection.key_env_fallbacks.length) {
      const entry = capsule?.entries.find(e => e.id === profile.id);
      if (!entry || entry.connectionRevision !== capsuleConnectionRevision(profile.connection, revisionKey) || !entry.apiKey) return unavailable;
      return { source: "process_env", policy: "static_key", authKind: "api_key", key: entry.apiKey, secrets: [entry.apiKey], snapshot: capsule!.snapshot.id, expires: capsule!.snapshot.expiresAtUnixMs };
    }
    if (/^http:\/\/(127\.0\.0\.1|localhost|\[::1\])(?::|$)/i.test(profile.origin)) return { source: "local_none", policy: "static_key", authKind: "none", key: "", secrets: [] };
    return unavailable;
  }
  if (profile.authKind !== "kimi_oauth" || profile.connection.provider !== "kimi") return unavailable;
  const raw = sourceBytes(home, ".neko-core/kimi-auth.json");
  if (!raw) return unavailable;
  let value: JsonValue;
  try {
    // SAFETY: bounded JSON is narrowed to the token schema below; no credential object is emitted.
    value = JSON.parse(raw.toString("utf8")) as JsonValue;
  } catch { return unavailable; }
  if (!isJsonObject(value)) return unavailable;
  const access = value.access_token ?? value.accessToken;
  const refresh = value.refresh_token ?? value.refreshToken ?? "";
  const seconds = value.expires_at ?? value.expiresAt;
  if (!isText(access) || !access || Buffer.byteLength(access) > 16_384 || !isText(refresh) || Buffer.byteLength(refresh) > 16_384
    || !isJsonNumber(seconds) || !Number.isSafeInteger(seconds) || seconds < 1 || !Number.isSafeInteger(seconds * 1000)) return unavailable;
  const expires = Math.min(seconds * 1000 - 300_000, capsule?.snapshot.expiresAtUnixMs ?? Number.MAX_SAFE_INTEGER);
  if (expires <= Date.now()) return unavailable;
  return { source: "oauth_cache", policy: "cached_token_no_refresh", authKind: "kimi_oauth", key: access, secrets: [access, refresh].filter(Boolean), expires,
    ...(capsule ? { snapshot: capsule.snapshot.id } : undefined) };
}
function revision(home: string, profile: SourceProfile, key: string, value: Material): string {
  const mac = createHmac("sha256", Buffer.from(key, "hex"));
  mac.update(JSON.stringify({ version: 2, sourceHome: process.platform === "win32" ? home.toLowerCase() : home, id: profile.id, connection: profile.connection,
    credentialSource: value.source, authPolicy: value.policy, snapshotId: value.snapshot ?? "", expires: value.expires ?? 0, credentials: value.secrets }));
  if (profile.authKind === "kimi_oauth") { const bytes = sourceBytes(home, ".neko-core/kimi-auth.json"); if (bytes) mac.update(bytes); }
  return `pr2_${mac.digest("hex")}`;
}
export function publicCoreProfilesV2(options: ProfileBridgeOptions = {}): PublicCoreProfilesV2 {
  const home = credentialHomeDir(options.credentialHome); isolatedStateHome(home, options.home ?? homeDir());
  const key = keyOf(options); const capsule = capsuleFor(options, home, key);
  const profiles = profilesAt(home, false).map(profile => {
    const value = material(profile, home, key, capsule);
    const configured = Boolean(profile.connection.provider && profile.connection.model && profile.origin);
    const dto: PublicCoreProfileV2 = { id: profile.id, provider: String(profile.connection.provider), model: String(profile.connection.model ?? ""),
      authKind: value.authKind, active: profile.active, configurationStatus: configured ? "configured" : "incomplete", credentialReadiness: "not_checked",
      providerOrigin: profile.origin, bridgeTransport: configured && value.policy !== "unavailable" ? "supported" : "unsupported",
      profileRevision: revision(home, profile, key, value), credentialSource: value.source, authPolicy: value.policy,
      ...(value.snapshot ? { credentialSnapshotId: value.snapshot } : undefined), ...(value.expires ? { credentialExpiresAtUnixMs: value.expires } : undefined) };
    for (const secret of value.secrets) if (secret.length >= 4 && [dto.id, dto.provider, dto.model, dto.providerOrigin].some(text => text.includes(secret))) fail();
    return dto;
  });
  return { version: 2, profiles, ...(capsule ? { sourceSnapshot: capsule.snapshot } : undefined) };
}

export function brokerCoreProfiles(input: JsonValue): PublicCoreProfilesV2 {
  const keys = ["version", "sourceId", "credentialHome", "stateHome", "capsulePath", "revisionKey", "expiresAtUnixMs"];
  if (!isJsonObject(input) || Object.keys(input).sort().join() !== keys.sort().join() || input.version !== 1 || !isText(input.sourceId)
    || !/^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$/.test(input.sourceId) || !isText(input.credentialHome) || !isText(input.stateHome)
    || !isText(input.capsulePath) || !isText(input.revisionKey) || !isJsonNumber(input.expiresAtUnixMs) || !Number.isSafeInteger(input.expiresAtUnixMs)) fail();
  const home = credentialHomeDir(input.credentialHome); const state = isolatedStateHome(home, input.stateHome);
  const key = keyOf({ revisionKey: input.revisionKey }); const now = Date.now();
  if (input.expiresAtUnixMs <= now || input.expiresAtUnixMs - now > 300_000) fail();
  const entries: CapsuleEntry[] = profilesAt(home, false).filter(p => !p.connection.api_key && (p.connection.key_env || Array.isArray(p.connection.key_env_fallbacks) && p.connection.key_env_fallbacks.length) && ["api_key", "none"].includes(p.authKind)).map(p => {
    const names = [p.connection.key_env, ...(Array.isArray(p.connection.key_env_fallbacks) ? p.connection.key_env_fallbacks : [])].filter(isText);
    const selected = names.find(name => process.env[name]?.trim()) ?? "";
    const apiKey = selected ? process.env[selected]!.trim() : "";
    if (Buffer.byteLength(apiKey) > 16_384) fail();
    return { id: p.id, connectionRevision: capsuleConnectionRevision(p.connection, key), envNames: names, selectedEnv: selected, apiKey };
  });
  const snapshot = writeCredentialCapsule(input.capsulePath, home, state, input.sourceId, key, entries, input.expiresAtUnixMs);
  return publicCoreProfilesV2({ credentialHome: home, home: state, revisionKey: key, capsulePath: input.capsulePath, snapshotId: snapshot.id });
}

export function loadProfileBridgeV2(options: ProfileBridgeOptions): NekoConfig {
  const home = credentialHomeDir(options.credentialHome); const state = isolatedStateHome(home, options.home ?? homeDir()); const key = keyOf(options);
  const selected = options.profile; const expected = options.expectedRevision ?? process.env.NEKO_PROFILE_REVISION ?? "";
  if (!selected || !/^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$/.test(selected) || !/^pr2_[a-f0-9]{64}$/.test(expected)) fail();
  if (["NEKO_API_KEY", "NEKO_BASE_URL", "NEKO_PROVIDER", "NEKO_MODEL", "NEKO_GEMINI_HOME"].some(name => process.env[name])) fail();
  const capsule = capsuleFor(options, home, key); const pinned = profilesAt(home, false).find(p => p.id === selected); if (!pinned) fail();
  const value = material(pinned, home, key, capsule);
  const policy = options.authPolicy ?? process.env.NEKO_PROFILE_AUTH_POLICY;
  const source = options.credentialSource ?? process.env.NEKO_PROFILE_CREDENTIAL_SOURCE;
  const expiry = options.credentialExpiresAtUnixMs ?? optionalInteger(process.env.NEKO_PROFILE_CREDENTIAL_EXPIRES_AT_UNIX_MS);
  const deadline = options.deadlineUnixMs ?? optionalInteger(process.env.NEKO_PROFILE_DEADLINE_UNIX_MS);
  if (policy !== value.policy || source !== value.source || expiry !== value.expires || !deadline || !Number.isSafeInteger(deadline) || deadline <= Date.now()
    || value.expires && deadline > value.expires || !pinned.connection.provider || !pinned.connection.model || !pinned.origin || value.policy === "unavailable"
    || process.env.NEKO_PROFILE_ORIGIN !== pinned.origin || !timingSafeEqual(Buffer.from(expected), Buffer.from(revision(home, pinned, key, value)))) fail();
  if (value.snapshot && (options.snapshotId ?? process.env.NEKO_PROFILE_SNAPSHOT_ID) !== value.snapshot) fail();
  const limits = parseProfileBridgeLimits(); const data = structuredClone(DEFAULTS);
  delete data.profiles; delete data.active_profile; delete data.api_key;
  for (const field of ["provider", "base_url", "model"]) data[field] = pinned.connection[field];
  data.auto_update = false; data.auto_update_check = false; data.mcp_servers = {}; data.mode = "default"; data.read_outside_root = false;
  data.max_steps = limits.maxSteps; data.max_retries = 0; data.offline_retry_seconds = 0; data.timeout_seconds = limits.maxSeconds;
  // SAFETY: material derives authKind from the closed validated source AUTH enum or the constant none.
  const profile: Profile = { provider: String(pinned.connection.provider), model: String(pinned.connection.model), auth: value.authKind as Profile["auth"] };
  const envNames = [pinned.connection.key_env, ...(Array.isArray(pinned.connection.key_env_fallbacks) ? pinned.connection.key_env_fallbacks : [])].filter(isText);
  const cfg = new NekoConfig(data, selected, { [selected]: profile }, value.key, null, envNames, { state: "none", files: [] }, state);
  let observed = Date.now(); const initial = observed; const began = performance.now();
  const validate = (config: NekoConfig) => {
    const now = Date.now(); if (now < observed || now >= deadline || performance.now() - began >= deadline - initial) fail(); observed = now;
    if (credentialHomeDir() !== home || process.env.NEKO_PROFILE_ORIGIN !== pinned.origin || config.provider !== profile.provider || config.model !== profile.model
      || config.profile !== selected || config.baseUrl !== String(pinned.connection.base_url).replace(/\/+$/, "") || config.profiles[selected]?.auth !== value.authKind
      || config.profileBridge?.authPolicy !== value.policy || config.apiKey !== value.key) fail();
    const currentCapsule = capsuleFor(options, home, key); const current = profilesAt(home, false).find(p => p.id === selected); if (!current) fail();
    const live = material(current, home, key, currentCapsule);
    if (live.policy !== value.policy || live.source !== value.source || live.expires !== value.expires || live.snapshot !== value.snapshot
      || !timingSafeEqual(Buffer.from(expected), Buffer.from(revision(home, current, key, live)))) fail();
  };
  const budget = new ProfileBridgeBudget(state, expected, limits, pinned.origin, deadline);
  cfg.profileBridge = { assertCurrent: validate, credentialSecrets: value.secrets, authPolicy: value.policy,
    complete: (provider, config, args) => budget.complete(provider, () => validate(config), n => { config.data.max_tokens = n; }, args) };
  Object.freeze(cfg.profileBridge.credentialSecrets); Object.freeze(cfg.profileBridge);
  return cfg;
}
