import { DEFAULTS } from "./config.ts";
import { loginChatGpt } from "./chatgpt-auth.ts";
import { chatGptCatalogClientVersion, chatGptMinimumCodexVersion, listChatGptModelCatalog, needsCodexTransport } from "./chatgpt-provider.ts";
import { compareCodexVersions, discoverCodexSupport } from "./codex-app-server.ts";

// Curated routes; endpoint, protocol and model defaults remain owned by Core.
const routes = ["zai", "zai-openai", "chatgpt", "openai", "claude", "gemini-api", "deepseek"] as const;
export function auroraProviderCatalog() {
  return routes.map(id => {
    const p = DEFAULTS.profiles[id];
    return { id, label: p.label || id, auth: p.auth, model: p.model,
      models: id === "chatgpt" ? [] : p.models || [p.model], modelSource: id === "chatgpt" ? "account_required" : "preset", keyEnv: p.key_env || "" };
  });
}

export async function auroraChatGptModels(signal: AbortSignal, catalog = listChatGptModelCatalog, discover = discoverCodexSupport) {
  const support = discover();
  const rows = await catalog(undefined, signal, chatGptCatalogClientVersion(support.executable?.version));
  return { models: rows.filter(m => /^[a-zA-Z0-9][a-zA-Z0-9._:/-]{0,119}$/.test(m.slug)).slice(0,256).map(m => {
    const native = needsCodexTransport(m.slug, m);
    const available = !native || support.state === "ready" && compareCodexVersions(support.executable?.version || "0.0.0", chatGptMinimumCodexVersion(m.slug,m)) >= 0;
    return { id:m.slug, label:m.displayName.slice(0,160), available, issue:available ? "" : "core_support_required", transport:native ? "codex" : "responses" };
  }), checkedAt:new Date().toISOString() };
}

export function auroraCatalogError(message: string): string {
  if (/not signed in|HTTP (401|403)|authentication failed|refresh failed/.test(message)) return "core_auth_required";
  if (/HTTP 429/.test(message)) return "core_rate_limited";
  return "core_catalog_unavailable";
}

export async function auroraChatGptLogin(
  signal: AbortSignal,
  emit: (event: { state: "waiting" | "connected"; url?: string }) => void,
  login = loginChatGpt,
  open: (url: string) => void = () => {},
) {
  await login({ signal, openUrl: url => { emit({ state: "waiting", url }); open(url); } });
  if (signal.aborted) throw new Error("Sign-in cancelled");
  emit({ state: "connected" });
}
