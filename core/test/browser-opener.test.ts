import { expect, spyOn, test } from "bun:test";
import * as processes from "node:child_process";
import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { openBrowser } from "../src/adapters/chatgpt-auth.ts";
import { resolveWindowsSystemExecutable } from "../src/shared/windows-system.ts";

const windows = process.platform === "win32";

test.skipIf(!windows)("browser opener ignores cwd/PATH shims and keeps provider credentials out of its environment", () => {
  const home = mkdtempSync(join(tmpdir(), "neko-opener-")), cwd = process.cwd();
  const prior = { PATH: process.env.PATH, NEKO_API_KEY: process.env.NEKO_API_KEY, CUSTOM_SECRET: process.env.CUSTOM_SECRET };
  const child = new processes.ChildProcess(), launch = spyOn(processes, "spawn").mockReturnValue(child);
  const unref = spyOn(child, "unref");
  const exe = resolveWindowsSystemExecutable("rundll32.exe"), library = resolveWindowsSystemExecutable("url.dll");
  try {
    if (!exe || !library) throw new Error("Windows opener fixture requires the inbox executable and DLL");
    writeFileSync(join(home, "rundll32.exe"), "fixture-not-executable");
    writeFileSync(join(home, "url.dll"), "fixture-not-a-library");
    process.chdir(home); process.env.PATH = home + ";" + prior.PATH;
    process.env.NEKO_API_KEY = "fixture-credential-never-inherit"; process.env.CUSTOM_SECRET = "fixture-custom-never-inherit";
    const url = "https://auth.example.test/authorize?state=abc%26def&redirect_uri=http%3A%2F%2Flocalhost%3A41789%2Fcallback#help";
    openBrowser(url);
    expect(launch).toHaveBeenCalledTimes(1);
    const [command, args, options] = launch.mock.calls[0]!;
    expect(command).toBe(exe);
    expect(args).toEqual([library + ",FileProtocolHandler", url]);
    expect(options).toMatchObject({ cwd: dirname(exe), detached: true, stdio: "ignore", windowsHide: true });
    expect(options?.shell).not.toBe(true);
    expect(options?.env?.PATH).toBe(dirname(exe));
    expect(options?.env?.NEKO_API_KEY).toBeUndefined(); expect(options?.env?.CUSTOM_SECRET).toBeUndefined();
    for (const name of ["USERPROFILE", "APPDATA", "LOCALAPPDATA"] as const) expect(options?.env?.[name]).toBe(process.env[name]);
    expect(unref).toHaveBeenCalledTimes(1);
    expect(() => child.emit("error", new Error("fixture missing default browser"))).not.toThrow();
  } finally {
    launch.mockRestore(); unref.mockRestore(); process.chdir(cwd);
    for (const [name, value] of Object.entries(prior)) { if (value === undefined) delete process.env[name]; else process.env[name] = value; }
    rmSync(home, { recursive: true, force: true });
  }
});

test("browser opener declines non-web protocols, relative URLs and malformed targets", () => {
  const launch = spyOn(processes, "spawn").mockReturnValue(new processes.ChildProcess());
  try {
    for (const url of ["file:///C:/tmp/launch.exe", "javascript:alert(1)", "ms-msdt:/fixture", "mailto:a@example.test", "//example.test", "--flag", "https://user:pass@example.test", "https://example.test/\nother", "https://example.test/\0other", " https://example.test", "https://"]) {
      expect(() => openBrowser(url)).not.toThrow();
    }
    expect(launch).not.toHaveBeenCalled();
  } finally { launch.mockRestore(); }
});

for (const kind of ["missing-executable", "missing-library", "ambiguous-library"] as const) test.skipIf(!windows)("browser opener refuses " + kind + " instead of searching PATH", () => {
  const root = mkdtempSync(join(tmpdir(), kind === "ambiguous-library" ? "neko,opener-" : "neko-opener-"));
  const prior = process.env.SystemRoot, launch = spyOn(processes, "spawn").mockReturnValue(new processes.ChildProcess());
  try {
    const system32 = join(root, "System32"); mkdirSync(system32);
    if (kind !== "missing-executable") writeFileSync(join(system32, "rundll32.exe"), "fixture-not-executable");
    if (kind !== "missing-library") writeFileSync(join(system32, "url.dll"), "fixture-not-a-library");
    process.env.SystemRoot = root;
    openBrowser("https://example.test/");
    expect(launch).not.toHaveBeenCalled();
  } finally {
    launch.mockRestore(); if (prior === undefined) delete process.env.SystemRoot; else process.env.SystemRoot = prior;
    rmSync(root, { recursive: true, force: true });
  }
});


test("browser opener preserves encoded OAuth query parameters and supports loopback HTTP links", () => {
  const launch = spyOn(processes, "spawn").mockReturnValue(new processes.ChildProcess());
  try {
    const links = ["HTTPS://AUTH.EXAMPLE.TEST:443/login?state=abc%26def&scope=files%3Aread", "http://127.0.0.1:41789/callback?state=fixture", "https://docs.example.test/path%20with%20spaces#help"];
    for (const url of links) openBrowser(url);
    expect(launch).toHaveBeenCalledTimes(links.length);
    for (let i = 0; i < links.length; i++) expect(launch.mock.calls[i]![1]?.at(-1)).toBe(new URL(links[i]!).href);
  } finally { launch.mockRestore(); }
});
