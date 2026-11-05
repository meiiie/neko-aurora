import { DEFAULTS } from "./config.ts";
import { providerFetch } from "./provider-fetch.ts";
import { z } from "zod";

export const AURORA_ROUTES = ["zai", "zai-openai", "chatgpt", "openai", "claude", "gemini-api", "deepseek"] as const;
const MODEL_ID = /^[a-zA-Z0-9][a-zA-Z0-9._:/-]{0,119}$/;
const ModelRow = z.object({
  id: z.string().optional(), name: z.string().optional(), displayName: z.string().optional(), display_name: z.string().optional(),
  lifecycle: z.string().optional(), supportedGenerationMethods: z.array(z.string()).optional(), output_modalities: z.array(z.string()).optional(),
});
type Row = z.infer<typeof ModelRow>;
const Catalog = z.object({
  data: z.array(ModelRow).max(10000).optional(), models: z.array(ModelRow).max(10000).optional(),
  has_more: z.boolean().optional(), last_id: z.string().max(4096).nullable().optional(), nextPageToken: z.string().max(4096).optional(),
});

async function catalogBody(response: Response): Promise<z.infer<typeof Catalog>> {
  if (!response.ok) {
    await response.body?.cancel();
    throw new Error(`Model catalog HTTP ${response.status}`);
  }
  const reader = response.body?.getReader();
  if (!reader) throw new Error("Empty model catalog");
  const chunks: Uint8Array[] = [];
  let size = 0;
  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      size += value.length;
      if (size > 2 * 1024 * 1024) throw new Error("Model catalog too large");
      chunks.push(value);
    }
    return Catalog.parse(JSON.parse(Buffer.concat(chunks).toString("utf8")));
  } finally { await reader.cancel().catch(() => {}); reader.releaseLock(); }
}

function chatModel(provider: string, row: Row, id: string): boolean {
  if (!MODEL_ID.test(id) || row.lifecycle === "retired") return false;
  if (provider === "gemini-api" && (!Array.isArray(row.supportedGenerationMethods) || !row.supportedGenerationMethods.includes("generateContent"))) return false;
  if (Array.isArray(row.output_modalities) && !row.output_modalities.includes("text")) return false;
  // API lists also contain embeddings, media and realtime models the text-chat route cannot run.
  if (/(?:embedding|whisper|tts|dall-e|moderation|realtime|transcrib|image|audio|veo|aqa|robotics|computer-use)/i.test(id)) return false;
  if (provider === "openai") return /^(?:ft:)?(?:gpt-|chatgpt-|o\d)/.test(id) && !/(?:codex|pro(?:-|$)|search)/i.test(id);
  if (provider === "zai" || provider === "zai-openai") return /^glm-/i.test(id);
  if (provider === "claude") return /^claude-/i.test(id);
  return provider !== "gemini-api" || /^gemini-/i.test(id);
}

/** Strict discovery for Aurora: never merge presets or silently switch the account's billing route. */
export async function auroraApiModels(provider: string, key: string, signal: AbortSignal, request = providerFetch) {
  if (!AURORA_ROUTES.some(id => id === provider) || provider === "chatgpt") throw new Error("Unknown catalog route");
  if (!key || key.length > 8192 || /[\r\n\0]/.test(key)) throw new Error("Model catalog HTTP 401");
  const p = DEFAULTS.profiles[provider];
  const google = provider === "gemini-api", anthropic = provider === "claude";
  // Z.AI's Anthropic adapter uses the Coding Plan. Its catalog is on the same plan's OpenAI endpoint.
  const base = provider === "zai" ? "https://api.z.ai/api/coding/paas/v4/models"
    : google ? "https://generativelanguage.googleapis.com/v1beta/models"
    : `${p.base_url}${anthropic ? "/v1/models" : "/models"}`;
  const headers = new Headers({ Accept: "application/json" });
  if (google) headers.set("x-goog-api-key", key);
  else if (anthropic) { headers.set("x-api-key", key); headers.set("anthropic-version", "2023-06-01"); }
  else headers.set("Authorization", `Bearer ${key}`);
  const models = new Map<string, { id: string; label: string; available: boolean; transport: string }>();
  const cursors = new Set<string>();
  let cursor = "";
  for (let page = 0; page < 16; page++) {
    const url = new URL(base);
    if (google) { url.searchParams.set("pageSize", "1000"); if (cursor) url.searchParams.set("pageToken", cursor); }
    if (anthropic) { url.searchParams.set("limit", "1000"); if (cursor) url.searchParams.set("after_id", cursor); }
    signal.throwIfAborted();
    const data = await catalogBody(await request(url.href, { headers, signal, redirect: "manual" }));
    const rows = google ? data.models : data.data;
    if (!Array.isArray(rows)) throw new Error("Invalid model catalog");
    for (const row of rows) {
      const id = google ? (row.name || "").replace(/^models\//, "") : row.id || "";
      if (!chatModel(provider, row, id)) continue;
      const label = (row.displayName || row.display_name || row.name || id).replace(/[\x00-\x1f\x7f]/g, "").slice(0,160);
      models.set(id, { id, label, available: true, transport: p.provider });
      if (models.size > 256) throw new Error("Model catalog too large");
    }
    cursor = google ? data.nextPageToken || "" : anthropic && data.has_more === true ? data.last_id || "" : "";
    if (!cursor) {
      if (anthropic && data.has_more === true) throw new Error("Invalid catalog cursor");
      return { models: [...models.values()].sort((a,b) => a.label.localeCompare(b.label)), checkedAt: new Date().toISOString() };
    }
    if (cursor.length > 4096 || cursors.has(cursor)) throw new Error("Invalid catalog cursor");
    cursors.add(cursor);
  }
  throw new Error("Incomplete model catalog");
}
