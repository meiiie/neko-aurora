import { expect, test } from "bun:test";
import { render } from "ink-testing-library";
import { existsSync, mkdtempSync, realpathSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { isAbsolute, join, relative } from "node:path";
import { loadConfig } from "../src/adapters/config.ts";
import { ChatApp } from "../src/ui/chat.tsx";

test("ordinary UI fixtures own writable state and never start the real update lookup", async () => {
  const config = loadConfig();
  expect(config.resolvedHome).toBe(process.env.HOME!);
  expect(config.resolvedHome).toBe(process.env.USERPROFILE!);
  const rel = relative(realpathSync(tmpdir()), realpathSync(config.resolvedHome));
  expect(rel.startsWith("..")).toBe(false);
  expect(isAbsolute(rel)).toBe(false);
  expect(rel).not.toBe("");
  expect(config.autoUpdateCheck).toBe(false);
  expect(config.autoUpdate).toBe(false);
  const original = globalThis.fetch;
  let requests = 0;
  globalThis.fetch = Object.assign(async () => {
    requests++;
    throw new Error("UI fixture must not request network");
  }, { preconnect: original.preconnect });
  const app = render(<ChatApp yolo={false} fullscreen={false} provider={{complete:async()=>({content:"fixture",tool_calls:[]})}} />);
  try {
    await Bun.sleep(100);
    expect(existsSync(config.researchWriteRoot)).toBe(true);
    expect(app.frames.join("\n")).toContain("/help");
    expect(requests).toBe(0);
  } finally {
    app.unmount();
    globalThis.fetch = original;
  }
});


test("fixture network fence fails swallowed outbound attempts without logging URL secrets", () => {
  const root = mkdtempSync(join(tmpdir(), "neko-test-fence-"));
  try {
    const path = join(root, "blocked.test.ts");
    writeFileSync(path, `import { test } from "bun:test";
      test("swallowed outbound", async () => {
        try { await fetch("https://fixture-user:fixture-password@outside.invalid/test-path?fixture-query"); } catch {}
      });`);
    const child = Bun.spawnSync([process.execPath, "test", "--preload", join(import.meta.dir, "setup.ts"), path], {
      cwd: root, env: { ...process.env }, stdout: "pipe", stderr: "pipe",
    });
    const output = child.stdout.toString() + child.stderr.toString();
    expect(child.exitCode).not.toBe(0);
    expect(output).toContain("Unmocked external network blocked: https://outside.invalid/test-path");
    for (const sensitive of ["fixture-user", "fixture-password", "fixture-query"]) expect(output).not.toContain(sensitive);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("loopback redirects preserve explicit manual and error request modes", async () => {
  const server = Bun.serve({ hostname: "127.0.0.1", port: 0, fetch: () => Response.redirect("https://outside.invalid/redirect-target", 302) });
  try {
    const url = `http://127.0.0.1:${server.port}/fixture`;
    expect((await fetch(url, { redirect: "manual" })).status).toBe(302);
    await expect(fetch(url, { redirect: "error" })).rejects.toThrow("Unexpected redirect");
  } finally {
    await server.stop(true);
  }
});

test("fixture fence blocks automatic loopback redirects before any external follow", () => {
  const root = mkdtempSync(join(tmpdir(), "neko-redirect-fence-"));
  try {
    const path = join(root, "redirect.test.ts");
    writeFileSync(path, `import { test } from "bun:test";
      test("swallowed redirect", async () => {
        const server = Bun.serve({hostname:"127.0.0.1",port:0,fetch:()=>Response.redirect("https://outside.invalid/redirect-target",302)});
        try { await fetch("http://127.0.0.1:" + server.port); } catch {} finally { await server.stop(true); }
      });`);
    const child = Bun.spawnSync([process.execPath, "test", "--preload", join(import.meta.dir, "setup.ts"), path], {
      cwd: root, env: { ...process.env }, stdout: "pipe", stderr: "pipe",
    });
    expect(child.exitCode).not.toBe(0);
    expect(child.stderr.toString()).toContain("Unmocked external network blocked: https://outside.invalid/redirect-target");
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});
