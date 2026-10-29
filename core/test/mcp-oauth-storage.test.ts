import { expect, spyOn, test } from "bun:test";
import { linkSync, mkdtempSync, mkdirSync, readdirSync, readFileSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import * as fs from "node:fs";
import { spawnSync } from "node:child_process";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { McpOAuthCallback } from "../src/adapters/mcp-oauth-callback.ts";
import { NekoOAuthProvider } from "../src/adapters/mcp-oauth.ts";
import * as atomic from "../src/shared/atomic.ts";
import * as windows from "../src/adapters/mcp-oauth-windows.ts";
import { readOAuthRecord, writeOAuthRecord } from "../src/adapters/mcp-oauth-storage.ts";
import { minimalWindowsSystemEnv, resolveWindowsSystemExecutable } from "../src/shared/windows-system.ts";

const resource = "https://mcp.example.test/mcp", issuer = "https://auth.example.test";
function provider(server = "storage-fixture") {
  const p = new NekoOAuthProvider(server, resource, new McpOAuthCallback());
  p.saveDiscoveryState({ authorizationServerUrl: issuer, resourceMetadata: { resource, authorization_servers: [issuer] },
    authorizationServerMetadata: { issuer, authorization_endpoint: issuer + "/authorize", token_endpoint: issuer + "/token", response_types_supported: ["code"], code_challenge_methods_supported: ["S256"] } });
  return p;
}
function isolated(fn: (dir: string) => void, label = "neko-oauth-storage-") {
  const root = mkdtempSync(join(tmpdir(), label));
  const old = { HOME: process.env.HOME, USERPROFILE: process.env.USERPROFILE };
  process.env.HOME = root; process.env.USERPROFILE = root;
  try { fn(join(root, ".neko-core", "mcp-auth", "v2")); } finally {
    for (const key of ["HOME", "USERPROFILE"] as const) if (old[key] === undefined) delete process.env[key]; else process.env[key] = old[key];
    rmSync(root, { recursive: true, force: true });
  }
}
const tokens = { access_token: "synthetic-access-storage", refresh_token: "synthetic-refresh-storage", token_type: "Bearer" };

test.skipIf(process.platform !== "win32")("Windows OAuth records seal tokens and confidential client data before publication", () => isolated(parent => {
  const p = provider(); p.saveTokens(tokens); p.saveClientInformation({ client_id: "fixture-client", client_secret: "synthetic-client-storage" });
  const dir = join(parent, readdirSync(parent)[0]!);
  for (const file of ["tokens.json", "client.json"]) {
    const raw = readFileSync(join(dir, file), "utf8");
    expect(raw).not.toContain("synthetic-"); expect(JSON.parse(raw).protection).toBe("windows-dpapi-current-user");
  }
  expect(provider().tokens()).toEqual(tokens); expect(provider().clientInformation()?.client_secret).toBe("synthetic-client-storage");
}), 15000);

test.skipIf(process.platform !== "win32")("legacy OAuth tokens migrate on read but invalid records remain untouched", () => isolated(parent => {
  const p = provider(); p.saveTokens(tokens); const file = join(parent, readdirSync(parent)[0]!, "tokens.json");
  writeFileSync(file, JSON.stringify(tokens)); expect(p.tokens()).toEqual(tokens); expect(readFileSync(file, "utf8")).not.toContain("synthetic-");
  const invalid = '{"version":1,"phase":"unknown"}'; writeFileSync(file, invalid);
  expect(() => p.tokens()).toThrow(/cannot be read/); expect(readFileSync(file, "utf8")).toBe(invalid);
}), 15000);

test.skipIf(process.platform !== "win32")("failed legacy migration returns no credential and preserves the previous file", () => isolated(parent => {
  const p = provider(); p.saveTokens(tokens); const file = join(parent, readdirSync(parent)[0]!, "tokens.json");
  const legacy = JSON.stringify(tokens); writeFileSync(file, legacy);
  const fault = spyOn(atomic, "atomicWriteFileSync").mockImplementation(() => { throw new Error("private fixture write failure"); });
  try { expect(() => p.tokens()).toThrow(/cannot be read/); expect(readFileSync(file, "utf8")).toBe(legacy); }
  finally { fault.mockRestore(); }
  expect(p.tokens()).toEqual(tokens); expect(readFileSync(file, "utf8")).not.toContain("synthetic-");
}), 15000);

test.skipIf(process.platform !== "win32")("OAuth ciphertext rejects tampering and substitution across files or server identities", () => isolated(parent => {
  const p = provider(); p.saveTokens(tokens); const dir = join(parent, readdirSync(parent)[0]!), file = join(dir, "tokens.json");
  const good = readFileSync(file, "utf8"), envelope = JSON.parse(good);
  writeFileSync(join(dir, "client.json"), good); expect(() => p.clientInformation()).toThrow(/cannot be read/);
  const other = provider("other-fixture"); other.saveTokens(tokens);
  const otherDir = readdirSync(parent).map(x => join(parent, x)).find(x => x !== dir)!;
  writeFileSync(join(otherDir, "tokens.json"), good); expect(() => other.tokens()).toThrow(/cannot be read/);
  const corrupt = Buffer.from(envelope.ciphertext, "base64"); corrupt[corrupt.length - 1] = corrupt[corrupt.length - 1]! ^ 1;
  for (const bad of [{ ...envelope, ciphertext: corrupt.toString("base64") }, { ...envelope, ciphertext: envelope.ciphertext + "\n" }, { ...envelope, version: 2 }, { ...envelope, protection: "unknown" }, { ...envelope, extra: true }]) {
    const raw = JSON.stringify(bad); writeFileSync(file, raw); expect(() => p.tokens()).toThrow(/cannot be read/); expect(readFileSync(file, "utf8")).toBe(raw);
  }
  writeFileSync(file, good); expect(p.tokens()).toEqual(tokens);
}), 15000);

test.skipIf(process.platform !== "win32")("OAuth encryption failures preserve existing bytes without a plaintext fallback", () => isolated(parent => {
  const p = provider(); p.saveTokens(tokens); const file = join(parent, readdirSync(parent)[0]!, "tokens.json"), before = readFileSync(file, "utf8");
  const fault = spyOn(windows, "cryptWindowsOAuth").mockImplementation(() => { throw new Error("private cryptographic detail"); });
  try { expect(() => p.saveTokens({ ...tokens, access_token: "new-fixture" })).toThrow("MCP OAuth credential record cannot be saved");expect(readFileSync(file, "utf8")).toBe(before); }
  finally { fault.mockRestore(); }
  expect(provider().tokens()).toEqual(tokens);
}), 15000);

test.skipIf(process.platform !== "win32")("OAuth migration rereads after a competing removal or consuming claim", () => isolated(parent => {
  for (const mode of ["remove", "claim"] as const) {
    const p = provider(); p.saveTokens(tokens); const file = join(parent, readdirSync(parent)[0]!, "tokens.json"); writeFileSync(file, JSON.stringify(tokens));
    const original = fs.readFileSync; let intercepted = false;
    // SAFETY: every overload forwards its exact arguments and returns the original result unchanged.
    const fault = spyOn(fs, "readFileSync").mockImplementation(((...args: Parameters<typeof fs.readFileSync>) => {
      const result = original(...args);
      if (String(args[0]) === file && !intercepted) {
        intercepted = true;
        if (mode === "remove") provider().invalidateCredentials("tokens");
        else provider().beforeTokenRequest(issuer + "/token", { method: "POST", body: new URLSearchParams({ grant_type: "refresh_token", resource, refresh_token: tokens.refresh_token }) });
      }
      return result;
    }) as typeof fs.readFileSync);
    try { expect(p.tokens()).toBeUndefined(); expect(intercepted).toBe(true); } finally { fault.mockRestore(); }
    expect(provider().tokens()).toBeUndefined();
  }
}), 15000);

test.skipIf(process.platform !== "win32")("OAuth publication uses an owner-only Windows directory and file ACL", () => isolated(parent => {
  const executable = resolveWindowsSystemExecutable("WindowsPowerShell\\v1.0\\powershell.exe")!;
  const check = (path: string, directory: boolean) => {
    const script = `$ErrorActionPreference='Stop';$p=[Console]::In.ReadToEnd();$sid=[Security.Principal.WindowsIdentity]::GetCurrent().User;$acl=[IO.${directory ? "Directory" : "File"}]::GetAccessControl($p);$r=$acl.GetAccessRules($true,$true,[Security.Principal.SecurityIdentifier]);if ($acl.GetOwner([Security.Principal.SecurityIdentifier]) -ne $sid -or $r.Count -ne 1 -or $r[0].IdentityReference -ne $sid -or $r[0].AccessControlType -ne 'Allow' -or $r[0].FileSystemRights -ne 'FullControl'${directory ? " -or -not $acl.AreAccessRulesProtected" : ""}) {exit 1};[Console]::Out.Write('private')`;
    const r = spawnSync(executable, ["-NoLogo", "-NoProfile", "-NonInteractive", "-Command", script], { input: path, env: minimalWindowsSystemEnv(), windowsHide: true, timeout: 10000 });
    expect(r.status).toBe(0); expect(r.stdout.toString()).toBe("private");
  };
  const original = atomic.atomicWriteFileSync; let checked = false;
  const publication = spyOn(atomic, "atomicWriteFileSync").mockImplementation((path, data, mode) => {
    check(join(path, ".."), true); expect(data).not.toContain("synthetic-"); checked = true; original(path, data, mode);
  });
  try { provider().saveTokens(tokens); } finally { publication.mockRestore(); }
  expect(checked).toBe(true); check(join(parent, readdirSync(parent)[0]!, "tokens.json"), false);
}), 15000);

test("OAuth records reject hardlinks without replacing or disclosing their target", () => isolated(parent => {
  const p = provider(); p.saveTokens(tokens); const dir = join(parent, readdirSync(parent)[0]!), file = join(dir, "tokens.json"), alias = join(dir, "fixture-alias");
  linkSync(file, alias); const before = readFileSync(alias, "utf8");
  expect(() => p.tokens()).toThrow(/cannot be read/); expect(() => p.saveTokens(tokens)).toThrow(/cannot be saved/); expect(readFileSync(alias, "utf8")).toBe(before);
}), 15000);

test.skipIf(process.platform !== "win32")("OAuth storage refuses a parent junction before creating or removing records", () => isolated(parent => {
  const outside = join(parent, "outside"), alias = join(parent, "alias"); mkdirSync(outside, { recursive: true }); symlinkSync(outside, alias, "junction");
  const file = join(alias, "nested", "tokens.json");
  expect(() => readOAuthRecord(file, v => v)).toThrow(/cannot be read/); expect(() => writeOAuthRecord(file, JSON.stringify(tokens))).toThrow(/cannot be saved/); expect(readdirSync(outside)).toEqual([]);
}));

test.skipIf(process.platform !== "win32")("OAuth read cache is scoped to exact on-disk bytes and never masks removal or rotation", () => isolated(() => {
  const p = provider(); p.saveTokens(tokens);
  const original = windows.cryptWindowsOAuth; let decryptions = 0;
  const observed = spyOn(windows, "cryptWindowsOAuth").mockImplementation((path, data, op) => { if (op === "unprotect") decryptions++; return original(path, data, op); });
  try {
    for (let i = 0; i < 5; i++) { const value = p.tokens()!; expect(value).toEqual(tokens); value.access_token = "mutated-fixture"; }
    expect(decryptions).toBe(0);
    provider().saveTokens({ ...tokens, access_token: "rotated-fixture" }); expect(p.tokens()?.access_token).toBe("rotated-fixture"); expect(decryptions).toBe(1);
    provider().invalidateCredentials("tokens"); expect(p.tokens()).toBeUndefined(); expect(decryptions).toBe(1);
  } finally { observed.mockRestore(); }
}), 15000);

test("OAuth record limits count UTF-8 plaintext bytes separately from the encrypted envelope", () => isolated(parent => {
  const p = provider(), large = { ...tokens, access_token: "x".repeat(55000) }; p.saveTokens(large);
  const file = join(parent, readdirSync(parent)[0]!, "tokens.json"), before = readFileSync(file, "utf8");
  if (process.platform === "win32") expect(Buffer.byteLength(before)).toBeGreaterThan(65536);
  expect(provider().tokens()?.access_token.length).toBe(55000);
  expect(() => p.saveTokens({ ...tokens, access_token: "é".repeat(32769) })).toThrow(/too large/); expect(readFileSync(file, "utf8")).toBe(before);
  writeFileSync(file, " ".repeat(131073)); expect(() => p.tokens()).toThrow(/cannot be read/);
}), 15000);

test.skipIf(process.platform !== "win32")("Windows OAuth supports Vietnamese and quoted credential paths", () => isolated(() => {
  provider().saveTokens(tokens); expect(provider().tokens()).toEqual(tokens);
}, "neko-tiếng Việt-'$-🐈-"), 15000);
