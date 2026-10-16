import { afterEach, setDefaultTimeout } from "bun:test";
import { createUITestHome } from "./ui-test-home.ts";

// UI mounts load config independently of their injected provider. Give them private, writable
// state and explicitly disable the real startup updater; NEKO_AUTO_UPDATE=0 only disables install.
const testHome = createUITestHome();
process.env.HOME = testHome;
process.env.USERPROFILE = testHome;

// Tests of HTTP adapters must supply a mock or a loopback fixture. Record attempts separately:
// production best-effort catches (including the updater's fallback) must not hide an escaped request.
const networkAttempts: string[] = [];
const nativeFetch = globalThis.fetch;
function assertFixtureURL(input: Parameters<typeof fetch>[0]): void {
  const url = new URL(input instanceof Request ? input.url : String(input));
  if (["data:", "blob:"].includes(url.protocol)) return; // in-memory assets, including Yoga WASM
  if (["http:", "https:"].includes(url.protocol) && ["127.0.0.1", "[::1]", "localhost"].includes(url.hostname)) return;
  networkAttempts.push(url.origin === "null" ? url.protocol : url.origin + url.pathname);
  throw new Error("Unmocked external network request in test");
}
globalThis.fetch = Object.assign(async (...args: Parameters<typeof fetch>) => {
  assertFixtureURL(args[0]);
  const redirect = args[1]?.redirect ?? (args[0] instanceof Request ? args[0].redirect : "follow");
  const response = await nativeFetch(args[0], { ...args[1], redirect: "manual" });
  if (![301, 302, 303, 307, 308].includes(response.status) || !response.headers.has("location") || redirect === "manual") return response;
  if (redirect === "error") throw new TypeError("Unexpected redirect");
  // A loopback server must not turn an allowed fixture request into external egress.
  // Tests of automatic redirects must mock them; explicit manual redirects retain native behavior.
  const target = new URL(response.headers.get("location")!, response.url);
  networkAttempts.push(target.origin === "null" ? target.protocol : target.origin + target.pathname);
  throw new Error("Unmocked automatic redirect in test");
}, { preconnect: (url: Parameters<typeof fetch.preconnect>[0], options?: Parameters<typeof fetch.preconnect>[1]) => {
  assertFixtureURL(url);
  nativeFetch.preconnect(url, options);
} });
afterEach(() => {
  const attempted = networkAttempts.splice(0);
  if (attempted.length) throw new Error(`Unmocked external network blocked: ${attempted.join(", ")}`);
});

setDefaultTimeout(30_000); // subprocess/UI polls have their own bounds; avoid Bun's 5s wall-clock preemption under CI load

/** Test preload (bunfig.toml). BELT ONLY - do not rely on its env mutation: bun >=1.3.14 can run test files in
 * workers where this preload's env mutation is NOT reliably visible (observed on GitHub runners AND
 * locally: NEKO_FULLSCREEN was undefined mid-suite, so inline tests mounted fullscreen). Every ChatApp
 * test therefore passes the mode EXPLICITLY (fullscreen={false} inline, cloneElement(...{fullscreen:
 * true}) in the fullscreen helpers); this baseline only covers stray non-ChatApp config reads. */
process.env.NEKO_FULLSCREEN ??= "0";
