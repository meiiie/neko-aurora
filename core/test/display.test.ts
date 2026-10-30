import { afterAll, beforeAll, expect, test } from "bun:test";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { cachedRefreshRate, detectRefreshRate, clampFps, normalizeHz, resolveUiFps } from "../src/adapters/display.ts";
import { savePrefs } from "../src/adapters/prefs.ts";

// Isolate ~/.neko-core (display cache + prefs are read/written). See session.test.ts for the pattern.
const TEST_HOME = mkdtempSync(join(tmpdir(), "neko-disp-home-"));
const SAVED = { up: process.env.USERPROFILE, home: process.env.HOME, fps: process.env.NEKO_FPS };
beforeAll(() => { process.env.USERPROFILE = TEST_HOME; process.env.HOME = TEST_HOME; delete process.env.NEKO_FPS; });
afterAll(() => {
  if (SAVED.up === undefined) delete process.env.USERPROFILE; else process.env.USERPROFILE = SAVED.up;
  if (SAVED.home === undefined) delete process.env.HOME; else process.env.HOME = SAVED.home;
  if (SAVED.fps === undefined) delete process.env.NEKO_FPS; else process.env.NEKO_FPS = SAVED.fps;
  rmSync(TEST_HOME, { recursive: true, force: true });
});

const writeCache = (hz: number, at = Date.now()) => {
  mkdirSync(join(TEST_HOME, ".neko-core"), { recursive: true });
  writeFileSync(join(TEST_HOME, ".neko-core", ".display.json"), JSON.stringify({ hz, at }));
};

test("cachedRefreshRate: valid, stale, and garbage cache entries", () => {
  writeCache(144);
  expect(cachedRefreshRate()).toBe(144);
  writeCache(144, Date.now() - 8 * 24 * 3600 * 1000);
  expect(cachedRefreshRate()).toBe(null); // stale (> 7 days) -> re-probe
  writeCache(999);
  expect(cachedRefreshRate()).toBe(null); // insane value rejected
});

test("resolveUiFps layering: env > config > /fps pref > detected display > 60", () => {
  writeCache(144);
  // auto: follows the detected display
  expect(resolveUiFps(null)).toEqual({ fps: 144, mode: "auto", detected: 144, source: "display (auto)" });
  // /fps pref beats detection
  savePrefs({ uiFps: 90 });
  expect(resolveUiFps(null).fps).toBe(90);
  expect(resolveUiFps(null).source).toBe("/fps");
  // "auto" pref returns to detection
  savePrefs({ uiFps: "auto" });
  expect(resolveUiFps(null).fps).toBe(144);
  // config beats pref + detection
  expect(resolveUiFps(120).fps).toBe(120);
  expect(resolveUiFps(120).source).toBe("config ui_fps");
  // env beats everything
  process.env.NEKO_FPS = "75";
  expect(resolveUiFps(120)).toMatchObject({ fps: 75, source: "NEKO_FPS" });
  delete process.env.NEKO_FPS;
  // nothing anywhere -> 60
  rmSync(join(TEST_HOME, ".neko-core", ".display.json"), { force: true });
  savePrefs({ uiFps: "auto" });
  expect(resolveUiFps(null)).toMatchObject({ fps: 60, mode: "auto", detected: null, source: "default" });
});

test("clampFps bounds 30..240", () => {
  expect(clampFps(144)).toBe(144);
  expect(clampFps(10)).toBe(30);
  expect(clampFps(1000)).toBe(240);
});

test("normalizeHz snaps floor-reported fractional rates up, passes exact reads through", () => {
  // WMI/system_profiler report the floor of NTSC-style fractional timings: 59.94 -> "59".
  expect(normalizeHz(59)).toBe(60);
  expect(normalizeHz(119)).toBe(120);
  expect(normalizeHz(143)).toBe(144);
  expect(normalizeHz(164)).toBe(165);
  // Exact and uncommon rates are untouched - never invent a display the user doesn't have.
  expect(normalizeHz(60)).toBe(60);
  expect(normalizeHz(144)).toBe(144);
  expect(normalizeHz(61)).toBe(61);
  expect(normalizeHz(48)).toBe(48);
});


test("display probes pin the originating home and discard late aborted results", async () => {
  const first = mkdtempSync(join(tmpdir(), "neko-display-origin-"));
  const second = mkdtempSync(join(tmpdir(), "neko-display-other-"));
  const saved = {home: process.env.HOME, profile: process.env.USERPROFILE};
  const output = process.platform === "win32" ? "144" : process.platform === "darwin" ? "144 Hz" : "144.0*";
  try {
    process.env.HOME = first; process.env.USERPROFILE = first;
    let release!: (value: string) => void;
    const pending = detectRefreshRate(undefined, async () => await new Promise<string>(r => { release = r; }));
    process.env.HOME = second; process.env.USERPROFILE = second;
    release(output); expect(await pending).toBe(144);
    expect(JSON.parse(readFileSync(join(first, ".neko-core", ".display.json"), "utf8")).hz).toBe(144);
    expect(existsSync(join(second, ".neko-core", ".display.json"))).toBe(false);
    const controller = new AbortController();
    const aborted = detectRefreshRate(controller.signal, async () => await new Promise<string>(r => { release = r; }));
    controller.abort(); release(output); expect(await aborted).toBeNull();
    expect(existsSync(join(second, ".neko-core", ".display.json"))).toBe(false);
    let dispatched = false;
    expect(await detectRefreshRate(controller.signal, async () => { dispatched = true; return output; })).toBeNull();
    expect(dispatched).toBe(false);
  } finally {
    if (saved.home === undefined) delete process.env.HOME; else process.env.HOME = saved.home;
    if (saved.profile === undefined) delete process.env.USERPROFILE; else process.env.USERPROFILE = saved.profile;
    rmSync(first, {recursive:true, force:true}); rmSync(second, {recursive:true, force:true});
  }
});
