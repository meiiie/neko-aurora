import { existsSync, lstatSync, mkdirSync, readFileSync } from "node:fs";
import { dirname, isAbsolute, join, parse, relative, sep } from "node:path";
import { atomicWriteFileSync } from "../shared/atomic.ts";
import { isJsonObject, isText, type JsonValue } from "../shared/wire.ts";
import { withOAuthRecordLock } from "./mcp-oauth-lock.ts";
import { cryptWindowsOAuth } from "./mcp-oauth-windows.ts";

export type OAuthRecordCache = Map<string, { envelope: string; plaintext: string }>;

function assertDirectory(dir: string): void {
  if (!isAbsolute(dir)) throw new Error("invalid credential directory");
  let current = parse(dir).root;
  if (process.platform === "win32") {
    if (!/^[A-Za-z]:[\\/]/.test(dir)) throw new Error("invalid credential directory");
    for (const part of relative(current, dir).split(sep)) {
      current = join(current, part);
      if (existsSync(current)) { const stat = lstatSync(current);if (!stat.isDirectory() || stat.isSymbolicLink()) throw new Error("invalid credential directory"); }
    }
  }
  if (existsSync(dir)) { const stat = lstatSync(dir);if (!stat.isDirectory() || stat.isSymbolicLink()) throw new Error("invalid credential directory"); }
}

export function withOAuthStorageLock<T>(dir: string, action: () => T): T {
  assertDirectory(dir);
  return withOAuthRecordLock(dir, action);
}

function load(path: string, cache?: OAuthRecordCache): { value: JsonValue; sealed: boolean } | undefined {
  assertDirectory(dirname(path));
  if (!existsSync(path)) return undefined;
  const stat = lstatSync(path);
  if (!stat.isFile() || stat.isSymbolicLink() || stat.nlink !== 1 || stat.size > 131072) throw new Error("invalid credential record");
  const raw = readFileSync(path, "utf8"), value: JsonValue = JSON.parse(raw);
  if (!isJsonObject(value) || !Object.hasOwn(value, "protection")) {
    if (Buffer.byteLength(raw) > 65536) throw new Error("invalid credential record");
    return { value, sealed: false };
  }
  if (Object.keys(value).length !== 3 || value.version !== 1 || value.protection !== "windows-dpapi-current-user" || !isText(value.ciphertext)) throw new Error("invalid credential envelope");
  const previous = cache?.get(path);
  if (previous?.envelope === raw) return { value: JSON.parse(previous.plaintext), sealed: true };
  const bytes = Buffer.from(value.ciphertext, "base64");
  if (!bytes.length || bytes.toString("base64") !== value.ciphertext) throw new Error("invalid credential envelope");
  const opened = cryptWindowsOAuth(path, bytes, "unprotect");
  try {
    const plaintext = new TextDecoder("utf-8", { fatal: true }).decode(opened), decoded: JsonValue = JSON.parse(plaintext);
    cache?.set(path, { envelope: raw, plaintext });
    return { value: decoded, sealed: true };
  }
  finally { opened.fill(0); }
}

// Caller holds the per-identity record lock; publication happens only after encryption and ACL setup succeed.
export function writeOAuthRecord(path: string, data: string, cache?: OAuthRecordCache): void {
  if (Buffer.byteLength(data) > 65536) throw new Error("MCP OAuth credential record is too large");
  try {
    assertDirectory(dirname(path)); mkdirSync(dirname(path), { recursive: true, mode: 0o700 });
    if (existsSync(path)) { const stat = lstatSync(path);if (!stat.isFile() || stat.isSymbolicLink() || stat.nlink !== 1) throw new Error("invalid credential record"); }
    const plaintext = data;
    if (process.platform === "win32") {
      const bytes = Buffer.from(data);
      try { data = JSON.stringify({ version: 1, protection: "windows-dpapi-current-user", ciphertext: cryptWindowsOAuth(path, bytes, "protect").toString("base64") }); }
      finally { bytes.fill(0); }
    }
    atomicWriteFileSync(path, data, 0o600);
    if (process.platform === "win32") cache?.set(path, { envelope: data, plaintext });
  } catch { throw new Error("MCP OAuth credential record cannot be saved"); }
}

export function readOAuthRecord<T>(path: string, validate: (value: JsonValue) => T, lockHeld = false, cache?: OAuthRecordCache): T | undefined {
  try {
    const record = load(path, cache);
    if (!record) return undefined;
    if (process.platform !== "win32" || record.sealed) return validate(record.value);
    const migrate = () => {
      // Reread under the lock: a concurrent claim/removal must never be undone by an old snapshot.
      const current = load(path, cache);
      if (!current) return undefined;
      const value = validate(current.value);
      if (!current.sealed) writeOAuthRecord(path, JSON.stringify(current.value), cache);
      return value;
    };
    return lockHeld ? migrate() : withOAuthStorageLock(dirname(path), migrate);
  } catch { throw new Error("MCP OAuth credential record cannot be read"); }
}
