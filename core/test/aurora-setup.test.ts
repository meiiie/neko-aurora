import { expect, test } from "bun:test";
import { DEFAULTS } from "../src/adapters/config.ts";
import { auroraCatalogError, auroraChatGptLogin, auroraChatGptModels, auroraProviderCatalog } from "../src/adapters/aurora-setup.ts";

test("Aurora presents Core routes without credentials or conflating GLM billing", () => {
  const rows = auroraProviderCatalog();
  expect(rows.find(p => p.id === "zai")?.model).toBe(DEFAULTS.profiles.zai.model);
  expect(rows.find(p => p.id === "zai-openai")?.model).toBe(DEFAULTS.profiles["zai-openai"].model);
  expect(rows.find(p => p.id === "chatgpt")?.auth).toBe("chatgpt_oauth");
  expect(rows.find(p => p.id === "chatgpt")?.models).toEqual([]);
  expect(rows.every(p => !Object.hasOwn(p, "api_key") && !Object.hasOwn(p, "base_url"))).toBe(true);
});

test("Aurora uses the account catalog and exposes transport availability without falling back", async () => {
  const base={description:"",defaultEffort:"high",efforts:[],inputModalities:["text"],useResponsesLite:true,toolMode:"code_mode_only",minimalClientVersion:"0.155.0"};
  const catalog=async()=>[{...base,slug:"gpt-future-fixture",displayName:"Future fixture"},{...base,slug:"direct-fixture",displayName:"Direct fixture",useResponsesLite:false,toolMode:"function"}];
  const models=await auroraChatGptModels(new AbortController().signal,catalog,()=>({state:"missing",detail:"fixture"}));
  expect(models.models).toEqual([
    {id:"gpt-future-fixture",label:"Future fixture",available:false,issue:"core_support_required",transport:"codex"},
    {id:"direct-fixture",label:"Direct fixture",available:true,issue:"",transport:"responses"},
  ]);
  const ready=await auroraChatGptModels(new AbortController().signal,catalog,()=>({state:"ready",detail:"fixture",executable:{path:"fixture",kind:"app-server",source:"managed",version:"0.160.0"}}));
  expect(ready.models.every(m=>m.available)).toBe(true);
  expect(auroraCatalogError("ChatGPT model catalog HTTP 401: fixture-private")).toBe("core_auth_required");
  expect(auroraCatalogError("fixture-private")).toBe("core_catalog_unavailable");
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
