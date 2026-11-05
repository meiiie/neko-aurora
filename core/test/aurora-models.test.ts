import { expect, test } from "bun:test";
import { auroraApiModels } from "../src/adapters/aurora-models.ts";

const signal = () => new AbortController().signal;
test("each Aurora API catalog uses the selected provider, plan and key with no preset fallback", async () => {
  for (const [provider, base, model, header] of [
    ["zai","https://api.z.ai/api/coding/paas/v4/models","glm-future","authorization"],
    ["zai-openai","https://api.z.ai/api/paas/v4/models","glm-api-future","authorization"],
    ["openai","https://api.openai.com/v1/models","gpt-future","authorization"],
    ["deepseek","https://api.deepseek.com/models","deepseek-future","authorization"],
    ["claude","https://api.anthropic.com/v1/models?limit=1000","claude-future","x-api-key"],
    ["gemini-api","https://generativelanguage.googleapis.com/v1beta/models?pageSize=1000","gemini-future","x-goog-api-key"],
  ]) {
    const result = await auroraApiModels(provider!,"fixture-private",signal(),async (url, init) => {
      expect(url).toBe(base!);
      expect(url).not.toContain("fixture-private");
      expect(new Headers(init.headers).get(header!)).toBe(header === "authorization" ? "Bearer fixture-private" : "fixture-private");
      expect(init.redirect).toBe("manual");
      return Response.json(provider === "gemini-api" ? {models:[{name:`models/${model}`,displayName:"Live label",supportedGenerationMethods:["generateContent"]}]}
        : {data:[{id:model,display_name:"Live label"}]});
    });
    expect(result.models.map(m => m.id)).toEqual([model!]);
    expect(result.models[0]?.label).toBe("Live label");
  }
});

test("catalog follows provider pagination, deduplicates and excludes non-chat models", async () => {
  let calls = 0;
  const google = await auroraApiModels("gemini-api","fixture",signal(),async (url) => {
    calls++;
    if (calls === 1) return Response.json({models:[{name:"models/gemini-image",supportedGenerationMethods:["generateContent"]},{name:"models/gemini-embed",supportedGenerationMethods:["embedContent"]}],nextPageToken:"next+page"});
    expect(new URL(url).searchParams.get("pageToken")).toBe("next+page");
    return Response.json({models:[{name:"models/gemini-live",supportedGenerationMethods:["generateContent"]}]});
  });
  expect(google.models.map(m=>m.id)).toEqual(["gemini-live"]);
  calls = 0;
  const claude = await auroraApiModels("claude","fixture",signal(),async (url) => {
    if (++calls === 1) return Response.json({data:[{id:"claude-first"}],has_more:true,last_id:"claude-first"});
    expect(new URL(url).searchParams.get("after_id")).toBe("claude-first");
    return Response.json({data:[{id:"claude-first"},{id:"claude-retired",lifecycle:"retired"},{id:"claude-next"}],has_more:false});
  });
  expect(claude.models.map(m=>m.id)).toEqual(["claude-first","claude-next"]);
  const openai = await auroraApiModels("openai","fixture",signal(),async () => Response.json({data:["gpt-next","o4-mini","text-embedding-3-small","gpt-image-1","gpt-audio","gpt-realtime","gpt-5-pro","gpt-5-codex"].map(id=>({id}))}));
  expect(openai.models.map(m=>m.id)).toEqual(["gpt-next","o4-mini"]);
});

test("failed, malformed, empty and looping catalogs never invent usable models", async () => {
  expect((await auroraApiModels("zai","fixture",signal(),async()=>Response.json({data:[]}))).models).toEqual([]);
  for (const status of [401,403,429,500,302]) {
    await expect(auroraApiModels("zai","fixture",signal(),async()=>new Response("fixture-sensitive-body",{status}))).rejects.toThrow(`HTTP ${status}`);
  }
  await expect(auroraApiModels("zai","fixture",signal(),async()=>Response.json({bad:"shape"}))).rejects.toThrow("Invalid model catalog");
  await expect(auroraApiModels("claude","fixture",signal(),async()=>Response.json({data:[],has_more:true,last_id:"again"}))).rejects.toThrow("Invalid catalog cursor");
  await expect(auroraApiModels("zai","fixture",signal(),async()=>new Response("x".repeat(2*1024*1024+1)))).rejects.toThrow("too large");
  const aborted = new AbortController(); aborted.abort();
  await expect(auroraApiModels("zai","fixture",aborted.signal,async()=>{throw new Error("must not request");})).rejects.not.toThrow("must not request");
});
