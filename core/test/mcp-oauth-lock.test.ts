import { expect, test } from "bun:test";
import { spawn } from "node:child_process";
import { mkdtempSync, mkdirSync, readdirSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { withOAuthRecordLock } from "../src/adapters/mcp-oauth-lock.ts";

test("OAuth lock excludes a live owner and releases after action failure", () => {
  const dir = mkdtempSync(join(tmpdir(), "neko-oauth-lock-"));
  try {
    withOAuthRecordLock(dir, () => expect(() => withOAuthRecordLock(dir, () => {})).toThrow(/busy/));
    expect(() => withOAuthRecordLock(dir, () => { throw new Error("fixture failure"); })).toThrow("fixture failure");
    expect(withOAuthRecordLock(dir, () => "recovered")).toBe("recovered");expect(readdirSync(dir)).toEqual([]);
  } finally { rmSync(dir, { recursive: true, force: true }); }
});

test("OAuth lock rejects ambiguous ownership and directory aliases without touching their contents", () => {
  const dir = mkdtempSync(join(tmpdir(), "neko-oauth-lock-")), lock = join(dir, ".oauth-write.lock"), outside = join(dir, "outside");
  try {
    mkdirSync(lock);writeFileSync(join(lock, "ambiguous"), "fixture");
    expect(() => withOAuthRecordLock(dir, () => {})).toThrow(/unavailable/);expect(readdirSync(lock)).toEqual(["ambiguous"]);
    rmSync(lock, { recursive: true });mkdirSync(outside);writeFileSync(join(outside, "keep"), "fixture");symlinkSync(outside, lock, process.platform === "win32" ? "junction" : "dir");
    expect(() => withOAuthRecordLock(dir, () => {})).toThrow(/unavailable/);expect(readdirSync(outside)).toEqual(["keep"]);
  } finally { rmSync(dir, { recursive: true, force: true }); }
});

test("a separate live process holds the lock and a confirmed dead process can be recovered", async () => {
  const root = mkdtempSync(join(tmpdir(), "neko-oauth-lock-")), dir = join(root, "records");
  const child = spawn(process.execPath, ["--no-env-file", fileURLToPath(new URL("./fixtures/oauth-exclusion-worker.ts", import.meta.url)), "lock", dir], {
    windowsHide: true, stdio: ["ignore", "pipe", "pipe"], env: { PATH: process.env.PATH, SystemRoot: process.env.SystemRoot, NODE_ENV: "test", HOME: root, USERPROFILE: root, PSModuleAnalysisCachePath: "NUL" },
  });
  const exited = new Promise(resolve => child.once("exit", resolve));
  try {
    const held = await new Promise<string>((resolve, reject) => { child.stdout.once("data", b => resolve(String(b).trim()));child.once("error", reject);child.once("exit", () => reject(new Error("fixture exited before locking"))); });
    expect(held).toBe("held");expect(() => withOAuthRecordLock(dir, () => {})).toThrow(/busy/);
    child.kill();await exited;
    expect(withOAuthRecordLock(dir, () => "recovered")).toBe("recovered");expect(readdirSync(dir)).toEqual([]);
  } finally { child.kill();await exited;rmSync(root, { recursive: true, force: true }); }
}, 15000);
