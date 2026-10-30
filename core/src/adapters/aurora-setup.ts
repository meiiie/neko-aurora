import { DEFAULTS } from "./config.ts";
import { loginChatGpt } from "./chatgpt-auth.ts";

// Curated routes; endpoint, protocol and model defaults remain owned by Core.
const routes = ["zai", "zai-openai", "chatgpt", "openai", "claude", "gemini-api", "deepseek"] as const;
export function auroraProviderCatalog() {
  return routes.map(id => {
    const p = DEFAULTS.profiles[id];
    return { id, label: p.label || id, auth: p.auth, model: p.model,
      models: p.models || [p.model], keyEnv: p.key_env || "" };
  });
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
