import { afterEach, beforeEach, expect, test } from "bun:test";
import * as acp from "@agentclientprotocol/sdk";
import { z } from "zod";
import { mkdirSync, mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { createNekoAcpAgent } from "../src/adapters/acp.ts";
import { loadProfileBridge, publicCoreProfiles } from "../src/adapters/profile-bridge.ts";
import { setSessionsDir } from "../src/adapters/session.ts";
import { Agent } from "../src/core/agent.ts";
import { ToolRegistry } from "../src/core/tool-runtime.ts";
import type { Provider } from "../src/core/ports.ts";
import { AcpHostMcp } from "../src/adapters/acp-host-mcp.ts";
import { hostToolNames, NEKOCUT_HOST_PROFILE } from "../src/adapters/host-profile.ts";
import { getProvider } from "../src/adapters/providers.ts";

const secret = "fixture-acp-file-backed-key";
const refresh = "fixture-acp-cached-refresh";
const saved = { ...process.env };
let source: string;
let state: string;
let root: string;

beforeEach(() => {
  for (const key of Object.keys(process.env)) if (key.startsWith("NEKO_") || /(API_KEY|TOKEN|PASSWORD|CREDENTIAL|SECRET)/i.test(key)) delete process.env[key];
  const base = mkdtempSync(join(tmpdir(), "acp-credential-output-"));
  source = join(base, "source"); state = join(base, "state"); root = join(base, "work");
  mkdirSync(join(source, ".neko-core"), { recursive: true }); mkdirSync(state); mkdirSync(root);
  process.env.USERPROFILE = state; process.env.HOME = state;
  process.env.NEKO_CREDENTIAL_HOME = source; process.env.NEKO_PROFILE_REVISION_KEY = "0".repeat(64);
  process.env.NEKO_PROFILE_LIMITS = JSON.stringify({ maxInputBytes: 64000, maxOutputTokens: 4096, maxRequests: 8, maxSteps: 6, maxSeconds: 600 });
  writeFileSync(join(source, ".neko-core", "config.json"), JSON.stringify({ active_profile: "fixture_output", profiles: {
    fixture_output: { provider: "openai_compat", model: "fixture", base_url: "http://127.0.0.1:1/v1", api_key: secret },
  } }));
  setSessionsDir(join(state, "session-fixture"));
});

afterEach(() => {
  setSessionsDir(null);
  for (const key of Object.keys(process.env)) if (!(key in saved)) delete process.env[key];
  Object.assign(process.env, saved);
});

async function run(kind: "full" | "split" | "mixed" | "tool" | "error" | "loopback" | "refresh_split" | "refresh_error" | "refresh_tool"): Promise<{ text: string; wire: string; error: string }> {
  let loopbackCalls = 0;
  const server = kind === "loopback" ? Bun.serve({ hostname: "127.0.0.1", port: 0, fetch: (request) => {
    loopbackCalls++; expect(request.headers.get("authorization")).toBe(`Bearer ${secret}`);
    const parts = [secret.slice(0, 11), secret.slice(11)];
    return new Response(parts.map((text) => `data: ${JSON.stringify({ choices: [{ delta: { content: text }, finish_reason: null }] })}\n\n`).join("")
      + `data: ${JSON.stringify({ choices: [{ delta: {}, finish_reason: "stop" }], usage: { completion_tokens: 8 } })}\n\ndata: [DONE]\n\n`, { headers: { "content-type": "text/event-stream" } });
  } }) : undefined;
  if (server) writeFileSync(join(source, ".neko-core", "config.json"), JSON.stringify({ active_profile: "fixture_output", profiles: {
    fixture_output: { provider: "openai_compat", model: "fixture", base_url: `http://127.0.0.1:${server.port}/v1`, api_key: secret },
  } }));
  const meta = publicCoreProfiles().profiles.find((p) => p.id === "fixture_output")!;
  process.env.NEKO_PROFILE_ORIGIN = meta.providerOrigin;
  const cfg = loadProfileBridge({ profile: meta.id, expectedRevision: meta.profileRevision });
  const notifications: acp.SessionUpdate[] = [];
  const cleanup: Promise<void>[] = [];
  const app = createNekoAcpAgent({ config: cfg, trackCleanup: (p) => cleanup.push(p), buildRuntime: async (config, options) => {
    const registry = new ToolRegistry(options.root, "default", options.approval);
    if (kind.startsWith("refresh_")) config.profileBridge!.credentialSecrets = [secret, refresh];
    const provider: Provider = { complete: async (_messages, _tools, onDelta) => {
      if (kind === "refresh_error") throw new acp.RequestError(-32000, "synthetic provider " + refresh, { nested: [secret, refresh] });
      if (kind === "refresh_tool") {
        const call = { id: "fixture-refresh", name: "read_file", arguments: { path: refresh } };
        options.onEvent?.("tool_call", call); options.onEvent?.("tool_result", { call, observation: refresh });
        return { content: "synthetic done", tool_calls: [] };
      }
      if (kind === "refresh_split") {
        onDelta?.(refresh.slice(0, 9), "content"); onDelta?.(refresh.slice(9), "reasoning");
        return { content: refresh, tool_calls: [] };
      }
      if (kind === "error") throw new acp.RequestError(-32000, `fixture provider rejected ${secret}`, { nested: secret });
      if (kind === "tool") {
        const call = { id: "fixture-tool", name: "read_file", arguments: { path: secret } };
        options.onEvent?.("tool_call", call);
        options.onEvent?.("tool_result", { call, observation: `fixture tool output ${secret}` });
        return { content: "tool fixture complete", tool_calls: [] };
      }
      if (kind === "full") onDelta?.(`before ${secret} after`);
      else { onDelta?.(`before ${secret.slice(0, 11)}`); onDelta?.(`${secret.slice(11)} after`, kind === "mixed" ? "reasoning" : "content"); }
      return { content: `before ${secret} after`, tool_calls: [] };
    } };
    return { agent: new Agent({ provider: kind === "loopback" ? getProvider(config) : provider, tools: registry, onDelta: options.onDelta, onEvent: options.onEvent, verifyBeforeExit: false, verifyStateChangesBeforeExit: false }), registry, config, close: async () => {} };
  } });
  let error = "";
  const client = acp.client({ name: "credential-output-fixture" }).onNotification(acp.methods.client.session.update, ({ params }) => { notifications.push(params.update); });
  try {
    await client.connectWith(app, async (ctx) => {
      const created = await ctx.request(acp.methods.agent.session.new, { cwd: root, mcpServers: [] });
      try { await ctx.request(acp.methods.agent.session.prompt, { sessionId: created.sessionId, prompt: [{ type: "text", text: "Answer the synthetic fixture." }] }); }
      catch (value) { error = value instanceof Error ? `${value.message}\n${JSON.stringify(value)}` : "error"; }
      await ctx.request(acp.methods.agent.session.close, { sessionId: created.sessionId });
    });
  } finally { await Promise.allSettled(cleanup); server?.stop(true); }
  if (server) expect(loopbackCalls).toBe(1);
  return { text: notifications.filter((u) => u.sessionUpdate === "agent_message_chunk" || u.sessionUpdate === "agent_thought_chunk").map((u) => "content" in u && u.content.type === "text" ? u.content.text : "").join(""), wire: JSON.stringify(notifications), error };
}

for (const kind of ["refresh_split", "refresh_error", "refresh_tool"] as const) {
  test("cached refresh token never reaches public ACP " + kind, async () => {
    const result = await run(kind);
    for (const value of [result.text, result.wire, result.error]) {
      expect(value.includes(secret)).toBe(false); expect(value.includes(refresh)).toBe(false);
    }
  });
}

for (const kind of ["full", "split", "mixed", "tool", "error", "loopback"] as const) {
  test(`known file-backed credential never reaches ACP public ${kind} output`, async () => {
    const result = await run(kind);
    expect(result.text.includes(secret)).toBe(false);
    expect(result.wire.includes(secret)).toBe(false);
    expect(result.error.includes(secret)).toBe(false);
    if (kind !== "error") expect(result.wire).toContain("redacted credential");
    else expect(result.error).toContain("redacted credential");
  });
}

test("known source credential in actual in-band host tool args refuses locally before any parent tool request", async () => {
  let calls = 0;
  const identity = z.record(z.string(), z.json());
  const agent = acp.agent({ name: "credential-host-guard" }).onRequest("fixture/guard", identity, async ({ client }) => {
    const port = await AcpHostMcp.connect(NEKOCUT_HOST_PROFILE, client, { type: "acp", name: "nekocut", serverId: "fixture" });
    port.bindCredential(() => [secret, refresh]);
    const denied = await port.call(hostToolNames(NEKOCUT_HOST_PROFILE)[0]!, { nested: { text: secret } });
    const deniedRefresh = await port.call(hostToolNames(NEKOCUT_HOST_PROFILE)[0]!, { nested: { text: refresh } });
    const safe = await port.call(hostToolNames(NEKOCUT_HOST_PROFILE)[0]!, { text: "safe fixture" });
    await port.close(); return { denied, deniedRefresh, safe };
  });
  const client = acp.client({ name: "parent-host-fixture" })
    .onRequest("mcp/connect", identity, () => ({ connectionId: "fixture-connection" }))
    .onRequest("mcp/disconnect", identity, () => ({}))
    .onRequest("mcp/message", identity, ({ params }) => {
      if (params.method === "initialize") return { protocolVersion: "2025-11-25", capabilities: { tools: {} }, serverInfo: { name: "fixture", version: "1" } };
      if (params.method === "notifications/initialized") return {};
      if (params.method === "tools/list") return { tools: NEKOCUT_HOST_PROFILE.tools.map((tool) => ({ name: tool.name, inputSchema: { type: "object", properties: {} } })) };
      if (params.method === "tools/call") { calls++; expect(JSON.stringify(params).includes(secret)).toBe(false); return { content: [{ type: "text", text: "safe host response" }] }; }
      throw new Error("unexpected fixture method");
    });
  await client.connectWith(agent, async (ctx) => {
    const response = await ctx.request("fixture/guard", {});
    expect(response).toMatchObject({ denied: "Error: configured credential in host tool input refused", safe: "safe host response" });
    expect(response).toMatchObject({ deniedRefresh: "Error: configured credential in host tool input refused" });
    expect(calls).toBe(1);
  });
});
