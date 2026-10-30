import { expect, test } from "bun:test";
import { DEFAULTS } from "../src/adapters/config.ts";
import { auroraChatGptLogin, auroraProviderCatalog } from "../src/adapters/aurora-setup.ts";

test("Aurora presents Core routes without credentials or conflating GLM billing", () => {
  const rows = auroraProviderCatalog();
  expect(rows.find(p => p.id === "zai")?.model).toBe(DEFAULTS.profiles.zai.model);
  expect(rows.find(p => p.id === "zai-openai")?.model).toBe(DEFAULTS.profiles["zai-openai"].model);
  expect(rows.find(p => p.id === "chatgpt")?.auth).toBe("chatgpt_oauth");
  expect(rows.every(p => !Object.hasOwn(p, "api_key") && !Object.hasOwn(p, "base_url"))).toBe(true);
});

test("Aurora relays sign-in link and completion, never OAuth credentials", async () => {
  const events: unknown[] = [];
  const opened: string[] = [];
  await auroraChatGptLogin(new AbortController().signal, e => events.push(e), async options => {
    options?.openUrl?.("https://auth.openai.com/oauth/authorize?state=fixture");
    return { accessToken: "fixture-private", refreshToken: "fixture-refresh", expiresAt: 123 };
  }, url => { opened.push(url); });
  expect(events).toEqual([{ state: "waiting", url: "https://auth.openai.com/oauth/authorize?state=fixture" }, { state: "connected" }]);
  expect(JSON.stringify(events)).not.toContain("fixture-private");
  expect(opened).toEqual(["https://auth.openai.com/oauth/authorize?state=fixture"]);
});

test("cancelled or failed sign-in cannot report connected", async () => {
  const abort = new AbortController(), events: unknown[] = [];
  await expect(auroraChatGptLogin(abort.signal, e => events.push(e), async () => {
    abort.abort();
    return { accessToken: "fixture", refreshToken: "fixture", expiresAt: 123 };
  })).rejects.toThrow("cancelled");
  expect(events).toEqual([]);
  await expect(auroraChatGptLogin(new AbortController().signal, e => events.push(e), async () => { throw new Error("fixture"); })).rejects.toThrow();
  expect(events).toEqual([]);
});
