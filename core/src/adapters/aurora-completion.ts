import { z } from "zod";
import type { Provider } from "../core/ports.ts";

export const AuroraMessages = z.array(z.object({
  role: z.enum(["system", "user", "assistant"]),
  content: z.string().max(2 * 1024 * 1024),
}).strict()).min(1).max(256);
export type AuroraMessages = z.infer<typeof AuroraMessages>;

export const AURORA_INPUT_LIMIT = 8 * 1024 * 1024;

// Aurora owns the tool dispatch boundary. Do not instantiate the terminal Agent here:
// its local workspace/no-tools instructions describe a different execution environment.
export async function auroraCompletion(
  messages: AuroraMessages, provider: Provider, signal: AbortSignal, emit: (text: string) => void,
): Promise<void> {
  const policy = messages.filter(m => m.role === "system");
  const conversation = messages.filter(m => m.role !== "system");
  if (!conversation.length) throw new Error("core_configuration_invalid");
  let emitted = false;
  try {
    const result = await provider.complete([...policy, ...conversation], [], (text, kind) => {
      if (!kind || kind === "content") { emitted = true; emit(text); }
    }, signal);
    if (result.tool_calls.length) throw new Error("core_configuration_invalid");
    if (!emitted && result.content) emit(result.content);
    if (result.truncated) emit("\n\0[aurora:cut]");
  } finally { await provider.dispose?.(); }
}

export function auroraCompletionError(error: Error): string {
  const message = error.message;
  if (/core_configuration_invalid/.test(message) || error instanceof z.ZodError) return "core_configuration_invalid";
  if (/not signed in|HTTP 401|refresh failed \(HTTP 403\)/.test(message)) return "core_auth_required";
  if (/HTTP 403/.test(message)) return "core_access_denied";
  if (/HTTP 429|rate limited|insufficient_quota|usage_limit_reached/.test(message)) return "core_rate_limited";
  if (/model_not_found|HTTP 404/.test(message)) return "core_model_unavailable";
  if (/optional Codex support|one-time setup|Could not prepare ChatGPT/.test(message)) return "core_support_required";
  return "core_failed";
}
