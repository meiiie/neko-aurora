import { describe, expect, test } from "bun:test";
import { runAuroraAgent, AuroraAgentStart, type AuroraAgentHost, type AuroraAgentSnapshot } from "../src/adapters/aurora-agent.ts";
import type { Provider, ProviderResponse } from "../src/core/ports.ts";

const start: AuroraAgentStart = {
  version: 1, taskId: "work-fixture", instruction: "Create the requested landing page.",
  history: [{ role: "system", content: "Aurora fixture workspace. computer_write_file writes a file." }],
  maxSteps: 8, contextWindow: 131072,
};
const ready = { status: "ready" as const, revision: "artifact-1", summary: "Preview and download verified.", evidence: ["preview:artifact-1", "download:artifact-1"] };
function fixture(responses: ProviderResponse[]) {
  const snapshots: AuroraAgentSnapshot[] = [], requests: Parameters<Provider["complete"]>[0][] = [];
  const effects: string[] = [], events: string[] = [];
  let verifies = 0, disposed = false;
  const provider: Provider = {
    complete: async messages => {
      requests.push(structuredClone(messages));
      const response = responses.shift();
      if (!response) throw new Error("unexpected extra model call");
      return response;
    },
    dispose: () => { disposed = true; },
  };
  const host: AuroraAgentHost = {
    checkpoint: async snapshot => { snapshots.push(structuredClone(snapshot)); },
    inputs: async () => [],
    execute: async (request, operationId) => {
      expect(operationId).toMatch(/^[a-f0-9]{32}$/);
      expect(JSON.stringify(snapshots.at(-1))).toContain(operationId);
      effects.push(request.name);
      return { text: "File saved in the bot workspace." };
    },
    verify: async () => { verifies++; return ready; },
    event: event => { events.push(event.kind); },
  };
  return { provider, host, snapshots, requests, effects, events, verifies: () => verifies, disposed: () => disposed };
}
const write: ProviderResponse = { content: "I will create the page.", tool_calls: [{ id: "call1", name: "mcp__aurora__execute", arguments: {
  name: "computer_write_file", arguments: { path: "index.html", content: "<!doctype html><h1>Aurora</h1>" },
} }] };
const final: ProviderResponse = { content: "The page is ready to view and download.", tool_calls: [] };

describe("Aurora uses the real Core loop with an exclusive host boundary", () => {
  test("accepts segmented history beyond the completion-only adapter's message cap", () => {
    const history = Array.from({ length: 300 }, () => ({ role: "assistant", content: "A saved segment." }));
    expect(AuroraAgentStart.parse({ ...start, history }).history).toHaveLength(300);
    expect(AuroraAgentStart.safeParse({ ...start, history: Array.from({ length: 4097 }, () => history[0]) }).success).toBe(false);
  });
  test("checkpoints before effects, returns observations to the model, and verifies once", async () => {
    const f = fixture([write, final]);
    const result = await runAuroraAgent(start, f.provider, f.host);
    expect(f.effects).toEqual(["computer_write_file"]);
    expect(f.requests).toHaveLength(2);
    expect(f.requests[1].some((message: { role: string; content?: string }) => message.role === "tool" && message.content?.includes("File saved"))).toBe(true);
    expect(result).toEqual({ text: final.content ?? "", completion: ready });
    expect(f.verifies()).toBe(1);
    expect(f.disposed()).toBe(true);
  });

  test("cannot escape to local shell or globally configured tools", async () => {
    const f = fixture([{ content: null, tool_calls: [{ id: "escape", name: "bash", arguments: { command: "echo this-must-not-execute" } }] }, final]);
    await runAuroraAgent(start, f.provider, f.host);
    expect(f.effects).toEqual([]);
    const observations = f.requests[1].filter((message: { role: string }) => message.role === "tool");
    expect(observations).toHaveLength(1);
    expect(observations[0].content).toBe("Tool 'bash' is not available to this sub-agent.");
  });

  test("admits a follow-up once, records it durably, and keeps local receipt metadata off the provider wire", async () => {
    const f = fixture([write, final]);
    f.host.inputs = async () => [{ id: "followup-1", text: "Use Vietnamese copy." }];
    await runAuroraAgent(start, f.provider, f.host);
    const latest = f.snapshots.at(-1)!;
    expect(latest.acceptedInputs).toEqual(["followup-1"]);
    expect(latest.messages.filter(message => message._neko_host_input === "followup-1")).toHaveLength(1);
    expect(f.requests[1].filter((message: { content?: string }) => message.content === "Use Vietnamese copy.")).toHaveLength(1);
    expect(JSON.stringify(f.requests)).not.toContain("_neko_host_input");
  });

  test("failed pre-effect persistence prevents execution", async () => {
    const f = fixture([write]);
    const persist = f.host.checkpoint;
    f.host.checkpoint = async snapshot => {
      if (snapshot.messages.some(message => message.tool_calls)) throw new Error("fixture disk full");
      await persist(snapshot);
    };
    await expect(runAuroraAgent(start, f.provider, f.host)).rejects.toThrow("fixture disk full");
    expect(f.effects).toEqual([]);
    expect(f.disposed()).toBe(true);
  });

  test("resuming compacted history preserves the last accepted input's segment anchor", async () => {
    const f = fixture([write, final]);
    await runAuroraAgent({ ...start, snapshot: { version: 1, acceptedInputs: ["followup-1"], messages: [
      { role: "user", content: "A compacted summary of the task.", _neko_internal: true },
    ] } }, f.provider, f.host);
    const segments = f.snapshots.at(-1)!.messages.filter(message => message._neko_host_segment);
    expect(segments).toHaveLength(2);
    expect(segments.every(message => message._neko_host_after_input === "followup-1")).toBe(true);
    expect(JSON.stringify(f.requests)).not.toContain("_neko_host_after_input");
  });

  test("input arriving during a long provider completion is admitted before handoff", async () => {
    const f = fixture([final, { content: "The added requirement is included.", tool_calls: [] }]);
    f.host.inputs = async () => f.requests.length ? [{ id: "late-input", text: "Also add a tagline." }] : [];
    const result = await runAuroraAgent(start, f.provider, f.host);
    expect(f.requests).toHaveLength(2);
    expect(f.verifies()).toBe(1);
    expect(f.requests[1].at(-1)?.content).toBe("Also add a tagline.");
    expect(f.snapshots.at(-1)?.acceptedInputs).toEqual(["late-input"]);
    expect(f.snapshots.at(-1)?.messages.some(message => message.content === final.content && message._neko_host_segment)).toBe(true);
    expect(result.text).toBe("The added requirement is included.");
  });

  test.each([true, false])("native provider progress remains separate from its aggregated final response (final streamed=%s)", async (streamFinal) => {
    const f = fixture([]);
    f.provider.complete = async (_messages, _tools, delta, _signal, options) => {
      delta?.("I am creating the page.", "content");
      await options!.executeTool!({ id: "native-write", name: "mcp__aurora__execute", arguments: write.tool_calls![0].arguments });
      if (streamFinal) delta?.("The page is ready.", "content");
      return { content: "I am creating the page.The page is ready.", tool_calls: [] };
    };
    const result = await runAuroraAgent(start, f.provider, f.host);
    expect(result.text).toBe("The page is ready.");
    const prose = f.snapshots.at(-1)!.messages.filter(message => message.role === "assistant").map(message => message.content);
    expect(prose).toEqual(["I am creating the page.", "The page is ready."]);
    expect(f.effects).toEqual(["computer_write_file"]);
  });

  test("the last step leaves new input queued instead of acknowledging work it cannot address", async () => {
    const f = fixture([final]);
    f.host.inputs = async () => f.requests.length ? [{ id: "late-input", text: "Also add a tagline." }] : [];
    await runAuroraAgent({ ...start, maxSteps: 1 }, f.provider, f.host);
    expect(f.requests).toHaveLength(1);
    expect(f.snapshots.at(-1)?.acceptedInputs).toEqual([]);
  });

  test("unchanged missing evidence stops boundedly instead of asking for checks forever", async () => {
    const f = fixture([final, final]);
    f.host.verify = async () => ({ status: "incomplete", revision: "unchanged", summary: "The preview is still unavailable.", evidence: [] });
    const result = await runAuroraAgent(start, f.provider, f.host);
    expect(f.requests).toHaveLength(2);
    expect(result.completion.status).toBe("incomplete");
    expect(result.text).toBe("The preview is still unavailable.");
  });

  test("a host approval pauses before another provider request or effect", async () => {
    const f = fixture([write]);
    f.host.execute = async () => ({ text: "Not executed.", waiting: { status: "waiting", revision: "approval-1", summary: "Please approve this command.", evidence: [] } });
    const result = await runAuroraAgent(start, f.provider, f.host);
    expect(f.requests).toHaveLength(1);
    expect(result.completion.status).toBe("waiting");
    expect(result.text).toBe("Please approve this command.");
  });

  test("restart retains completed tool history and consumed input identities", async () => {
    const f = fixture([write, final]);
    f.host.inputs = async () => [{ id: "followup-1", text: "Use Vietnamese copy." }];
    await runAuroraAgent(start, f.provider, f.host);
    const resumed = fixture([final]);
    resumed.host.inputs = f.host.inputs;
    await runAuroraAgent({ ...start, snapshot: f.snapshots.at(-1)! }, resumed.provider, resumed.host);
    expect(resumed.effects).toEqual([]);
    expect(resumed.snapshots.at(-1)!.acceptedInputs).toEqual(["followup-1"]);
    expect(resumed.requests[0].filter((message: { content?: string }) => message.content === "Use Vietnamese copy.")).toHaveLength(1);
    expect(JSON.stringify(resumed.requests[0])).toContain("File saved in the bot workspace.");
  });

  test("completed prose keeps its stable segment identity through checkpoints and resume", async () => {
    const f = fixture([write, final]);
    await runAuroraAgent(start, f.provider, f.host);
    const latest = f.snapshots.at(-1)!;
    const original = latest.messages.find(message => message.content === write.content)!;
    expect(original._neko_host_segment).toBeString();
    expect(original._neko_host_segment_at).toBeString();
    const resumed = fixture([final]);
    await runAuroraAgent({ ...start, snapshot: latest }, resumed.provider, resumed.host);
    expect(resumed.snapshots.at(-1)!.messages.find(message => message.content === write.content)!._neko_host_segment).toBe(original._neko_host_segment);
    expect(JSON.stringify(resumed.requests)).not.toContain("_neko_host_segment");
    expect(JSON.stringify(resumed.requests)).not.toContain("_neko_host_history");
  });

  test("cancellation during a host tool closes the provider and never declares readiness", async () => {
    const f = fixture([write]);
    const controller = new AbortController();
    f.host.execute = async (_request, _operation, signal) => {
      controller.abort();
      signal?.throwIfAborted();
      throw new Error("cancel signal not forwarded");
    };
    let result: Awaited<ReturnType<typeof runAuroraAgent>> | undefined;
    try { result = await runAuroraAgent(start, f.provider, f.host, controller.signal); }
    catch { expect(controller.signal.aborted).toBe(true); }
    if (result) expect(result.completion.status).not.toBe("ready");
    expect(f.requests).toHaveLength(1);
    expect(f.verifies()).toBe(0);
    expect(f.disposed()).toBe(true);
  });
});
