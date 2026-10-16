import { createCipheriv, createDecipheriv, createHash, createHmac, randomBytes, timingSafeEqual } from "node:crypto";
import { closeSync, existsSync, fstatSync, fsyncSync, lstatSync, openSync, readFileSync, realpathSync, renameSync, unlinkSync, writeFileSync } from "node:fs";
import { dirname, isAbsolute, join, relative, resolve, sep } from "node:path";
import { isJsonArray, isJsonNumber, isJsonObject, isText, type JsonObject, type JsonValue } from "../shared/wire.ts";

export interface ProfileSourceSnapshot { id: string; createdAtUnixMs: number; expiresAtUnixMs: number }
export interface CapsuleEntry { id: string; connectionRevision: string; envNames: string[]; selectedEnv: string; apiKey: string }
export interface CredentialCapsule { snapshot: ProfileSourceSnapshot; sourceId: string; entries: CapsuleEntry[] }
const ID = /^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$/;
const SNAPSHOT = /^cs1_[a-f0-9]{32}$/;
const HEX = /^[a-f0-9]{64}$/;
const MAX = 2_097_152;
function fail(): never { throw new Error("Profile credential capsule unavailable"); }
function exact(value: JsonValue, keys: string[]): value is JsonObject {
  return isJsonObject(value) && Object.keys(value).sort().join() === keys.sort().join();
}
function canonical(value: JsonValue): string {
  if (isJsonArray(value)) return `[${value.map(canonical).join(",")}]`;
  if (isJsonObject(value)) return `{${Object.keys(value).sort().map(k => `${JSON.stringify(k)}:${canonical(value[k]!)}`).join(",")}}`;
  return JSON.stringify(value);
}
function key(revisionKey: string, purpose: string): Buffer {
  if (!HEX.test(revisionKey)) fail();
  return createHmac("sha256", Buffer.from(revisionKey, "hex")).update(`neko/profile-capsule/v1/${purpose}`).digest();
}
function rootName(home: string): string { return process.platform === "win32" ? home.toLowerCase() : home; }
export function capsuleConnectionRevision(connection: JsonObject, revisionKey: string): string {
  return createHmac("sha256", key(revisionKey, "connection")).update(canonical(connection)).digest("hex");
}
export function safeCapsulePath(path: string, sourceHome: string, stateHome?: string): string {
  if (!isAbsolute(path) || /[\x00-\x1f]/.test(path)) fail();
  const absolute = resolve(path);
  for (let current = absolute;; current = dirname(current)) {
    if (existsSync(current) && lstatSync(current).isSymbolicLink()) fail();
    if (dirname(current) === current) break;
  }
  if (!existsSync(dirname(absolute)) || !lstatSync(dirname(absolute)).isDirectory()) fail();
  const canonicalPath = join(realpathSync.native(dirname(absolute)), absolute.slice(absolute.lastIndexOf(sep) + 1));
  const inside = (home: string) => { const rel = relative(home, canonicalPath); return rel === "" || (!rel.startsWith(".." + sep) && rel !== ".." && !isAbsolute(rel)); };
  if (inside(sourceHome) || stateHome && inside(stateHome)) fail();
  return canonicalPath;
}
function bytesAt(path: string): Buffer {
  const before = lstatSync(path);
  if (!before.isFile() || before.isSymbolicLink() || before.nlink !== 1 || before.size > MAX) fail();
  const fd = openSync(path, "r");
  try {
    const stat = fstatSync(fd);
    if (!stat.isFile() || stat.nlink !== 1 || stat.ino !== before.ino || stat.dev !== before.dev || stat.size > MAX) fail();
    const bytes = readFileSync(fd); const after = fstatSync(fd); const named = lstatSync(path);
    if (bytes.length > MAX || stat.size !== after.size || stat.mtimeMs !== after.mtimeMs || named.ino !== stat.ino || named.dev !== stat.dev || named.isSymbolicLink()) fail();
    return bytes;
  } finally { closeSync(fd); }
}
function jsonAt(path: string): JsonValue {
  try {
    // SAFETY: bounded JSON syntax has the JsonValue domain; the caller checks exact keys/types.
    return JSON.parse(bytesAt(path).toString("utf8")) as JsonValue;
  } catch { return fail(); }
}
function header(sourceId: string, home: string, snapshot: ProfileSourceSnapshot): JsonObject {
  if (!ID.test(sourceId) || !SNAPSHOT.test(snapshot.id) || !Number.isSafeInteger(snapshot.createdAtUnixMs) || !Number.isSafeInteger(snapshot.expiresAtUnixMs)
    || snapshot.createdAtUnixMs < 1 || snapshot.expiresAtUnixMs <= snapshot.createdAtUnixMs || snapshot.expiresAtUnixMs - snapshot.createdAtUnixMs > 300_000) fail();
  return { version: 1, sourceId, sourceHome: rootName(home), snapshotId: snapshot.id, createdAtUnixMs: snapshot.createdAtUnixMs, expiresAtUnixMs: snapshot.expiresAtUnixMs, policy: "configured_env_only" };
}
function b64(value: JsonValue | undefined, length?: number): Buffer {
  if (!isText(value) || !/^[A-Za-z0-9_-]+$/.test(value)) fail();
  const bytes = Buffer.from(value, "base64url");
  if (bytes.toString("base64url") !== value || length !== undefined && bytes.length !== length) fail();
  return bytes;
}
function atomic(path: string, value: JsonValue): void {
  if (existsSync(path)) { const stat = lstatSync(path); if (!stat.isFile() || stat.isSymbolicLink() || stat.nlink !== 1) fail(); }
  const temp = path + ".tmp-" + randomBytes(8).toString("hex"); const fd = openSync(temp, "wx", 0o600);
  try { writeFileSync(fd, canonical(value) + "\n"); fsyncSync(fd); }
  finally { closeSync(fd); }
  try { renameSync(temp, path); } catch { if (existsSync(temp)) unlinkSync(temp); fail(); }
}
export function writeCredentialCapsule(path: string, home: string, stateHome: string, sourceId: string, revisionKey: string, entries: CapsuleEntry[], expiresAtUnixMs: number): ProfileSourceSnapshot {
  const snapshot = { id: `cs1_${randomBytes(16).toString("hex")}`, createdAtUnixMs: Date.now(), expiresAtUnixMs };
  const aad = header(sourceId, home, snapshot); const target = safeCapsulePath(path, home, stateHome);
  if (Date.now() >= expiresAtUnixMs) fail();
  const plaintext = Buffer.from(canonical({ version: 1, entries: entries.map(e => ({ ...e })) }));
  if (plaintext.length > 1_048_576) fail();
  const nonce = randomBytes(12); const cipher = createCipheriv("aes-256-gcm", key(revisionKey, "encryption"), nonce);
  cipher.setAAD(Buffer.from(canonical(aad)));
  const ciphertext = Buffer.concat([cipher.update(plaintext), cipher.final()]); plaintext.fill(0);
  const sealed: JsonObject = { version: 1, sourceId, snapshotId: snapshot.id, createdAtUnixMs: snapshot.createdAtUnixMs, expiresAtUnixMs,
    nonce: nonce.toString("base64url"), tag: cipher.getAuthTag().toString("base64url"), ciphertext: ciphertext.toString("base64url") };
  const sealedDigest = createHash("sha256").update(canonical(sealed)).digest("hex");
  const marker: JsonObject = { version: 1, sourceId, snapshotId: snapshot.id, createdAtUnixMs: snapshot.createdAtUnixMs, expiresAtUnixMs, sealedDigest };
  marker.mac = createHmac("sha256", key(revisionKey, "marker")).update(canonical({ ...marker, sourceHome: rootName(home) })).digest("hex");
  safeCapsulePath(target + ".current", home, stateHome);
  atomic(target, sealed); atomic(target + ".current", marker);
  return snapshot;
}
export function readCredentialCapsule(path: string, home: string, revisionKey: string, expectedId?: string, stateHome?: string): CredentialCapsule {
  try {
    const target = safeCapsulePath(path, home, stateHome); safeCapsulePath(target + ".current", home, stateHome);
    const sealed = jsonAt(target); const marker = jsonAt(target + ".current");
    const common = ["version", "sourceId", "snapshotId", "createdAtUnixMs", "expiresAtUnixMs"];
    if (!exact(sealed, [...common, "nonce", "tag", "ciphertext"]) || !exact(marker, [...common, "sealedDigest", "mac"]) || sealed.version !== 1 || marker.version !== 1) fail();
    if (!isText(sealed.sourceId) || !isText(sealed.snapshotId) || !isJsonNumber(sealed.createdAtUnixMs) || !isJsonNumber(sealed.expiresAtUnixMs)) fail();
    const snapshot = { id: sealed.snapshotId, createdAtUnixMs: sealed.createdAtUnixMs, expiresAtUnixMs: sealed.expiresAtUnixMs };
    const aad = header(sealed.sourceId, home, snapshot);
    if (expectedId !== undefined && expectedId !== snapshot.id || Date.now() < snapshot.createdAtUnixMs || Date.now() >= snapshot.expiresAtUnixMs) fail();
    if (common.some(k => sealed[k] !== marker[k]) || marker.sealedDigest !== createHash("sha256").update(canonical(sealed)).digest("hex") || !isText(marker.mac) || !HEX.test(marker.mac)) fail();
    const { mac, ...body } = marker;
    const expected = createHmac("sha256", key(revisionKey, "marker")).update(canonical({ ...body, sourceHome: rootName(home) })).digest();
    if (!timingSafeEqual(Buffer.from(mac, "hex"), expected)) fail();
    const decipher = createDecipheriv("aes-256-gcm", key(revisionKey, "encryption"), b64(sealed.nonce, 12));
    decipher.setAAD(Buffer.from(canonical(aad))); decipher.setAuthTag(b64(sealed.tag, 16));
    const raw = Buffer.concat([decipher.update(b64(sealed.ciphertext)), decipher.final()]);
    if (raw.length > 1_048_576) fail();
    const data: JsonValue = JSON.parse(raw.toString("utf8")); raw.fill(0);
    if (!exact(data, ["version", "entries"]) || data.version !== 1 || !isJsonArray(data.entries) || data.entries.length > 64) fail();
    const seen = new Set<string>(); const entries = data.entries.map((e): CapsuleEntry => {
      if (!exact(e, ["id", "connectionRevision", "envNames", "selectedEnv", "apiKey"]) || !isText(e.id) || !ID.test(e.id) || seen.has(e.id)
        || !isText(e.connectionRevision) || !HEX.test(e.connectionRevision) || !isJsonArray(e.envNames) || e.envNames.length > 9
        || e.envNames.some(n => !isText(n) || !/^[A-Z][A-Z0-9_]{0,63}$/.test(n)) || !isText(e.selectedEnv) || e.selectedEnv && !e.envNames.includes(e.selectedEnv)
        || !isText(e.apiKey) || Buffer.byteLength(e.apiKey) > 16_384 || Boolean(e.selectedEnv) !== Boolean(e.apiKey)) fail();
      seen.add(e.id);
      return { id: e.id, connectionRevision: e.connectionRevision, envNames: e.envNames.map(String), selectedEnv: e.selectedEnv, apiKey: e.apiKey };
    });
    return { snapshot, sourceId: sealed.sourceId, entries };
  } catch { return fail(); }
}
