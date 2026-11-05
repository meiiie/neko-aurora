import { expect, test } from "bun:test";
import { auroraCompletion, auroraCompletionError, type AuroraMessages } from "../src/adapters/aurora-completion.ts";
import type { Provider } from "../src/core/ports.ts";

test("Aurora preserves authority, history and long Unicode files without a terminal-agent environment", async () => {
  const file = "<html>Tiếng Việt 中文</html>".repeat(4000);
  const input: AuroraMessages = [{ role: "system", content: "Aurora host tools policy" },
    { role: "user", content: "Earlier request" }, { role: "assistant", content: "Earlier reply" },
    { role: "user", content: file }, { role: "system", content: "Current capabilities and nudge" }];
  const output: string[] = [];
  let disposed = false;
  const provider: Provider = {
    complete: async (messages, tools, delta, signal, options) => {
      expect(messages.filter(m => m.role === "system").map(m => m.content)).toEqual([input[0].content, input[4].content]);
      expect(messages.slice(2)).toEqual(input.slice(1, 4));
      expect(messages.at(-1).content).toBe(file);
      expect(tools).toEqual([]);
      expect(options?.executeTool).toBeUndefined();
      expect(signal?.aborted).toBe(false);
      delta?.("private reasoning", "reasoning"); delta?.("TOOL: "); delta?.('{"name":"computer_list"}', "content");
      return { content: "must not duplicate streamed text", tool_calls: [] };
    },
    dispose: () => { disposed = true; },
  };
  await auroraCompletion(input, provider, new AbortController().signal, text => output.push(text));
  expect(output.join("")).toBe('TOOL: {"name":"computer_list"}');
  expect(disposed).toBe(true);
});

test("Aurora closes the provider on failure and never exposes private diagnostics", async () => {
  let disposed = false;
  const provider: Provider = {
    complete: async () => { throw new Error("HTTP 401 private response"); },
    dispose: () => { disposed = true; },
  };
  await expect(auroraCompletion([{ role: "user", content: "hello" }], provider, new AbortController().signal, () => {})).rejects.toThrow();
  expect(disposed).toBe(true);
  expect(auroraCompletionError(new Error("HTTP 401 private response"))).toBe("core_auth_required");
  expect(auroraCompletionError(new Error("private response"))).toBe("core_failed");
});

test("Aurora marks truncated file output so the host can recover instead of reporting success", async () => {
  const output: string[] = [];
  await auroraCompletion([{ role: "user", content: "create file" }], {
    complete: async () => ({ content: 'TOOL: {"content":"partial', tool_calls: [], truncated: true }),
  }, new AbortController().signal, text => output.push(text));
  expect(output.join("")).toBe('TOOL: {"content":"partial\n\0[aurora:cut]');
});
