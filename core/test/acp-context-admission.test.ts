import {type ContextManifest, contextManifestDigest, parseContextManifest, parseContextExpected, assertContextSourceObservations} from "../src/core/context/admission.ts";
import {expect, test} from "bun:test";
import * as acp from "@agentclientprotocol/sdk";
import {mkdirSync, mkdtempSync, readFileSync, rmSync} from "node:fs";
import {createHash} from "node:crypto";
import {tmpdir} from "node:os";
import {join} from "node:path";
import {createNekoAcpAgent} from "../src/adapters/acp.ts";
import {loadConfig} from "../src/adapters/config.ts";
import {NEKOBOT_HOST_PROFILE} from "../src/adapters/host-profile.ts";
import {buildAgentRuntime} from "../src/adapters/agent-runtime.ts";
import {Agent} from "../src/core/agent.ts";
import {isJsonObject} from "../src/shared/wire.ts";

function fixture() {
  const base = mkdtempSync(join(tmpdir(), "neko-context-admission-"));
  const root = join(base, "root"), home = join(base, "home");
  mkdirSync(root); mkdirSync(home);
  const config = loadConfig({cwd: root, home});
  config.data.context_mode = "structured";
  return {base, root, home, config};
}

test("trusted NekoBot context opt-in advertises protocol without widening its two-tool surface", async () => {
  const f = fixture();
  try {
    const app = createNekoAcpAgent({config: f.config, hostProfile: NEKOBOT_HOST_PROFILE, ...{contextProtocol: 1 as const}});
    await acp.client({name: "context-negotiation"}).connectWith(app, async ctx => {
      const init = await ctx.request(acp.methods.agent.initialize, {protocolVersion: 1, clientCapabilities: {}});
      expect(init._meta?.["neko.contextProtocol"]).toEqual({version: 1});
      expect(init._meta?.["neko.hostProfile"]).toMatchObject({id: "nekobot", version: 1,
        tools: ["mcp__nekobot__capabilities", "mcp__nekobot__execute_tool"]});
    });
  } finally {rmSync(f.base, {recursive: true, force: true});}
});

const digest = (value: string) => createHash("sha256").update(value).digest("hex");
const scope = {ownerUserId: "owner", spaceId: "space", botId: "bot-a", projectId: "project-a", channelId: "channel-a", audienceRevision: 1};
const text = "Giá cho Nguyễn An chưa xác định ngày áp dụng.";
function manifest(): ContextManifest {
  return {version: 1, contextPinSHA256: "a".repeat(64), scope,
    primaryMessage: {id: "message-current", senderId: "owner", text},
    sources: [{id: "message-current", kind: "original-user", version: 1, sha256: digest(text), bytes: Buffer.byteLength(text),
      scope, accessRevision: 0, observedAt: null, effectiveFrom: null, effectiveTo: null,
      entities: [{id: null, label: "Nguyễn An"}],
      span: {offset: 0, end: Buffer.byteLength(text), sha256: digest(text), text}}], factCandidates: []};
}
const protocol = {"neko.taskProtocol": {version: 1, mode: "fixed-active-task"}, "neko.executionProtocol": {version: 1},
  "neko.contextProtocol": {version: 1}, "neko.taskLabel": "Pinned source task"};
const servers: Array<acp.McpServerAcp & {type: "acp"}> = [{type: "acp", name: "nekobot", serverId: "context-host"}];
const identity = {parse: (value: any) => {if (!isJsonObject(value)) throw new Error("Invalid fixture object"); return value;}};
function host(onCall?: (params: any) => any) {
  return acp.client({name: "context-admission-host"})
    .onRequest("mcp/connect", identity, () => ({connectionId: "context-connection"}))
    .onRequest("mcp/disconnect", identity, () => ({}))
    .onNotification("mcp/message", identity, () => {})
    .onRequest("mcp/message", identity, ({params}) => {
      if (params.method === "initialize") return {protocolVersion: "2025-11-25", capabilities: {tools: {}}, serverInfo: {name: "fixture", version: "1"}};
      if (params.method === "tools/list") return {tools: NEKOBOT_HOST_PROFILE.tools.map(tool => ({name: tool.name, inputSchema: {type: "object", properties: {}, additionalProperties: true}}))};
      if (params.method === "tools/call" && onCall) return onCall(params.params);
      throw new Error("No tool call belongs to this admission fixture");
    });
}
function runtimeFixture(f: ReturnType<typeof fixture>, provider?: Parameters<Agent["setProvider"]>[0]) {
  const cleanup: Promise<void>[] = [], agents: Agent[] = [];
  let builds = 0;
  const app = () => createNekoAcpAgent({config: f.config, hostProfile: NEKOBOT_HOST_PROFILE, ...{contextProtocol: 1 as const},
    trackCleanup: promise => cleanup.push(promise), buildRuntime: async (cfg, options) => {
      builds++;
      const runtime = await buildAgentRuntime(cfg, options);
      runtime.agent.setProvider(provider ?? {complete: async () => ({content: "fixture only", tool_calls: []})});
      agents.push(runtime.agent); return runtime;
    }});
  return {app, cleanup, agents, builds: () => builds};
}

test("admission returns actual Core binding only after exact source metadata is durable", async () => {
  const f = fixture(), r = runtimeFixture(f);
  try {
    await host().connectWith(r.app(), async ctx => {
      const created = await ctx.request(acp.methods.agent.session.new, {cwd: f.root, mcpServers: servers,
        _meta: {...protocol, "neko.contextManifest": manifest()}});
      const admission = created._meta?.["neko.contextAdmission"];
      if (!isJsonObject(admission)) throw new Error("Missing context receipt");
      expect(admission).toMatchObject({version: 1, sessionId: created.sessionId, contextPinSHA256: "a".repeat(64),
        task: created._meta?.["neko.task"], execution: created._meta?.["neko.execution"]});
      const saved = JSON.parse(readFileSync(join(f.home, ".neko-core", "task-sessions", `${created.sessionId}.json`), "utf8"));
      expect(saved.schemaVersion).toBe(3);
      expect(saved.tasks[0].contextState.admission.manifest).toEqual(manifest());
      expect(saved.tasks[0].contextState.admission.sourceIds.length).toBeGreaterThan(0);
      expect(saved.tasks[0].contextState.admission.manifestSHA256).toBe(admission.manifestSHA256);
      expect(JSON.stringify(r.agents[0]?.messages)).toContain("unresolved");
      await ctx.request(acp.methods.agent.session.close, {sessionId: created.sessionId,
        _meta: {"neko.task": created._meta?.["neko.task"], "neko.execution": created._meta?.["neko.execution"], "neko.contextAdmission": admission}});
    });
  } finally {await Promise.allSettled(r.cleanup); rmSync(f.base, {recursive: true, force: true});}
});

test("forged source spans, hashes, scope and primary-user claims reject before runtime creation", async () => {
  const f = fixture(), r = runtimeFixture(f);
  try {
    const mutations = [
      (m: ReturnType<typeof manifest>) => {m.sources[0]!.span.sha256 = "f".repeat(64);},
      (m: ReturnType<typeof manifest>) => {m.sources[0]!.span.offset = 1;},
      (m: ReturnType<typeof manifest>) => {m.sources[0]!.scope = {...scope, channelId: "private-other"};},
      (m: ReturnType<typeof manifest>) => {m.sources[0]!.sha256 = "f".repeat(64);},
      (m: ReturnType<typeof manifest>) => {m.sources[0]!.span.text = "quoted trap"; m.sources[0]!.span.sha256 = digest("quoted trap"); m.sources[0]!.span.end = Buffer.byteLength("quoted trap");},
      (m: ReturnType<typeof manifest>) => {m.primaryMessage.senderId = "another-user";},
      (m: ReturnType<typeof manifest>) => {m.sources[0]!.effectiveFrom = "2026-02-30";},
    ];
    await host().connectWith(r.app(), async ctx => {
      for (const mutate of mutations) {
        const invalid = manifest(); mutate(invalid);
        await expect(ctx.request(acp.methods.agent.session.new, {cwd: f.root, mcpServers: servers,
          _meta: {...protocol, "neko.contextManifest": invalid}})).rejects.toThrow("context");
      }
      expect(r.builds()).toBe(0);
    });
  } finally {await Promise.allSettled(r.cleanup); rmSync(f.base, {recursive: true, force: true});}
});

test("context admission stays unavailable without trusted launch opt-in", async () => {
  const f = fixture();
  try {
    const app = createNekoAcpAgent({config: f.config, hostProfile: NEKOBOT_HOST_PROFILE});
    await acp.client({name: "context-default"}).connectWith(app, async ctx => {
      const init = await ctx.request(acp.methods.agent.initialize, {protocolVersion: 1, clientCapabilities: {}});
      expect(init._meta?.["neko.contextProtocol"]).toBeUndefined();
    });
    expect(() => createNekoAcpAgent({config: f.config, ...{contextProtocol: 1 as const}})).toThrow("NekoBot");
  } finally {rmSync(f.base, {recursive: true, force: true});}
});

function receipt(response: Pick<acp.NewSessionResponse, "_meta">) {
  const value = response._meta?.["neko.contextAdmission"];
  if (!isJsonObject(value) || !isJsonObject(value.task) || !isJsonObject(value.execution)) throw new Error("Missing context receipt");
  return parseContextExpected(value);
}
function echo(value: ReturnType<typeof receipt>) {
  return {"neko.task": value.task, "neko.execution": value.execution, "neko.contextAdmission": value};
}
function expected(value: ReturnType<typeof receipt>) {
  return {...protocol, "neko.taskExpected": value.task, "neko.executionExpected": value.execution, "neko.contextExpected": value};
}

test("v2 Bob attribution reaches the model and survives durable ACP resume while owner remains unchanged", async () => {
  const f = fixture();
  let modelSawBob = false;
  const r = runtimeFixture(f, {complete: async messages => {
    const declaration = messages.find(message => String(message.content).includes('"kind":"host_context_manifest"'));
    const payload = JSON.parse(String(declaration?.content));
    expect(payload.manifest.primaryMessage.senderId).toBe("Bob");
    expect(payload.manifest.requester.actorId).toBe("Bob");
    expect(payload.manifest.scope.ownerUserId).toBe("owner");
    modelSawBob = true;
    return {content: "fixture acknowledgement for Bob", tool_calls: []};
  }});
  let original: ReturnType<typeof receipt>, sessionId = "";
  try {
    await host().connectWith(r.app(), async ctx => {
      const m = {...manifest(), version: 2, requester: {version: 1, actorId: "Bob", spaceId: scope.spaceId,
        authorityId: "principal-authority-Bob", deviceId: "device-Bob", deviceRevision: 1,
        sessionId: "principal-session-Bob", sessionRevision: 1, issuerEpoch: 1, expiresAtUnixMs: 4102444800000}};
      m.primaryMessage.senderId = "Bob";
      const created = await ctx.request(acp.methods.agent.session.new, {cwd: f.root, mcpServers: servers,
        _meta: {...protocol, "neko.contextManifest": m}});
      original = receipt(created); sessionId = created.sessionId;
      expect(original.version).toBe(1);
      expect(original.manifestSHA256).toBe(contextManifestDigest(m));
      await ctx.request(acp.methods.agent.session.prompt, {sessionId, prompt: [{type: "text", text: "Bob follow-up"}], _meta: echo(original)});
      expect(modelSawBob).toBe(true);
      const saved = JSON.parse(readFileSync(join(f.home, ".neko-core", "task-sessions", `${sessionId}.json`), "utf8"));
      expect(saved.tasks[0].contextState.admission.manifest.requester).toEqual(m.requester);
      expect(saved.tasks[0].contextState.admission.manifest.scope.ownerUserId).toBe("owner");
      await ctx.request(acp.methods.agent.session.close, {sessionId, _meta: echo(original)});
    });
    await Promise.all(r.cleanup);
    await host().connectWith(r.app(), async ctx => {
      const wrong = {...original, manifestSHA256: "f".repeat(64)};
      await expect(ctx.request(acp.methods.agent.session.resume, {sessionId, cwd: f.root, mcpServers: servers,
        _meta: {...expected(original), "neko.contextExpected": wrong}})).rejects.toThrow("context");
      const resumed = await ctx.request(acp.methods.agent.session.resume, {sessionId, cwd: f.root, mcpServers: servers, _meta: expected(original)});
      const current = receipt(resumed);
      expect(current.version).toBe(1);
      expect(current.manifestSHA256).toBe(original.manifestSHA256);
      const declaration = r.agents.at(-1)!.messages.find(message => String(message.content).includes('"kind":"host_context_manifest"'));
      const payload = JSON.parse(String(declaration?.content));
      expect(payload.manifest.requester.actorId).toBe("Bob");
      expect(payload.manifest.primaryMessage.senderId).toBe("Bob");
      expect(payload.manifest.scope.ownerUserId).toBe("owner");
      await ctx.request(acp.methods.agent.session.close, {sessionId, _meta: echo(current)});
    });
  } finally {await Promise.allSettled(r.cleanup); rmSync(f.base, {recursive: true, force: true});}
});

test("compaction twice and restart preserve exact unresolved sources without promoting native user facts", async () => {
  const f = fixture(), r = runtimeFixture(f);
  let prior: ReturnType<typeof receipt>, sessionId = "";
  try {
    await host().connectWith(r.app(), async ctx => {
      const m = manifest();
      m.factCandidates.push({key: "price-an", sourceId: m.sources[0]!.id, entityId: null, state: "unresolved", span: m.sources[0]!.span});
      const created = await ctx.request(acp.methods.agent.session.new, {cwd: f.root, mcpServers: servers,
        _meta: {...protocol, "neko.contextManifest": m}});
      prior = receipt(created); sessionId = created.sessionId;
      const agent = r.agents[0]!;
      for (let cycle = 0; cycle < 2; cycle++) {
        for (let i = 0; i < 10; i++) agent.messages.push({role: "user", content: `Historical turn ${cycle}-${i}`},
          {role: "assistant", content: "omitted metadata from a lying fixture summary ".repeat(50)});
        expect(await agent.compact()).not.toBe("");
        const selected = agent.messages.filter(message => message._neko_context_admission);
        expect(selected).toHaveLength(2);
        expect(String(selected[1].content)).toContain('"effectiveFrom":null');
        expect(String(selected[1].content)).toContain('"id":null');
        expect(String(selected[1].content)).toContain('"state":"unresolved"');
        const capsule = agent.messages.find(message => message._neko_context_capsule);
        expect(String(capsule?.content)).not.toContain("Current verified user facts:");
      }
      await ctx.request(acp.methods.agent.session.close, {sessionId, _meta: echo(prior)});
    });
    await Promise.all(r.cleanup);
    const path = join(f.home, ".neko-core", "task-sessions", `${sessionId}.json`);
    const before = readFileSync(path), builds = r.builds();
    await host().connectWith(r.app(), async ctx => {
      const modifications = [
        (v: ReturnType<typeof receipt>) => {v.contextPinSHA256 = "b".repeat(64);},
        (v: ReturnType<typeof receipt>) => {v.manifestSHA256 = "c".repeat(64);},
        (v: ReturnType<typeof receipt>) => {v.sessionId = "f".repeat(32);},
        (v: ReturnType<typeof receipt>) => {v.task = {...v.task, root: f.home};},
        (v: ReturnType<typeof receipt>) => {v.execution = {...v.execution, authorityId: "d".repeat(64)};},
        (v: ReturnType<typeof receipt>) => {v.execution = {...v.execution, policyId: "e".repeat(64)};},
        (v: ReturnType<typeof receipt>) => {v.sourceIds = ["f".repeat(64)];},
      ];
      for (const mutate of modifications) {
        const changed = structuredClone(prior); mutate(changed);
        const meta = {...expected(prior), "neko.contextExpected": changed};
        await expect(ctx.request(acp.methods.agent.session.resume, {sessionId, cwd: f.root, mcpServers: servers, _meta: meta})).rejects.toThrow("context");
        expect(readFileSync(path)).toEqual(before);
      }
      expect(r.builds()).toBe(builds);
      const resumed = await ctx.request(acp.methods.agent.session.resume, {sessionId, cwd: f.root, mcpServers: servers, _meta: expected(prior)});
      const current = receipt(resumed);
      expect(current.manifestSHA256).toBe(prior.manifestSHA256);
      expect(current.sourceIds).toEqual(prior.sourceIds);
      expect(current.task.activationEpoch).toBe(Number(prior.task.activationEpoch) + 1);
      expect(r.agents.at(-1)!.messages.filter(message => message._neko_context_admission)).toHaveLength(2);
      await expect(ctx.request(acp.methods.agent.session.prompt, {sessionId, prompt: [{type: "text", text: "stale"}], _meta: echo(prior)})).rejects.toThrow("stale");
      await ctx.request(acp.methods.agent.session.close, {sessionId, _meta: echo(current)});
    });
  } finally {await Promise.allSettled(r.cleanup); rmSync(f.base, {recursive: true, force: true});}
});

test("source text cannot mint selected facts, authority, automatic entity or chronological decisions", () => {
  for (const change of [
    (m: any) => {m.sources[0].authority = "system";},
    (m: any) => {m.factCandidates = [{key: "x", sourceId: m.sources[0]!.id, entityId: null, state: "current", span: m.sources[0]!.span}];},
    (m: any) => {m.factCandidates = [{key: "x", sourceId: m.sources[0]!.id, entityId: "unmentioned-person", state: "unresolved", span: m.sources[0]!.span}];},
  ]) {const m = manifest(); change(m); expect(() => parseContextManifest(m)).toThrow("context");}
  expect(contextManifestDigest(manifest())).toBe(contextManifestDigest(JSON.parse(JSON.stringify(manifest()))));
});

test("only scoped project reader operation envelope mints provenance; ordinary MCP text and forged echoes do not", async () => {
  const cases = ["valid", "published-valid", "ordinary-text", "stale-operation", "task", "root", "source-version", "hash", "span", "scope", "manifest", "authority", "raw-tool"];
  for (const scenario of cases) {
    const f = fixture(), m = manifest();
    const fileText = "Literal price 18; effective date unknown; An remains unresolved.";
    const source = {...m.sources[0]!, id: "file-price", kind: "retrieved" as const, sha256: digest(fileText), bytes: Buffer.byteLength(fileText),
      span: {offset: 0, end: Buffer.byteLength(fileText), sha256: digest(fileText), text: fileText}};
    const deliveryId = "report-delivery-" + "b".repeat(32);
    if (scenario === "published-valid") {
      source.id = "c".repeat(64);
      const artifact = {bytes: source.bytes, filename: "report.md", id: source.id, sha256: source.sha256};
      const pin = {artifacts: [artifact], audienceRevision: scope.audienceRevision, botId: "publisher-bot", deliveryId,
        destinationChannelId: scope.channelId, files: [{artifact, excerpt: fileText}], messageId: "message-published", reply: "Published result",
        sequence: 7, sha256: "d".repeat(64), source: {accessRevision: source.accessRevision, botStateRevision: 0,
          conversationId: "conversation-bot-meiiie", conversationRevision: 0, epoch: 1, messageId: "message-source", requestId: "request-source", resultSha256: "e".repeat(64)}, version: 1};
      const selected = JSON.stringify(pin);
      m.sources.push({...source, id: "published:" + deliveryId, kind: "derived", version: 7, sha256: digest(selected), bytes: Buffer.byteLength(selected),
        span: {offset: 0, end: Buffer.byteLength(selected), sha256: digest(selected), text: selected}});
    }
    m.sources.push(source);
    let providerCalls = 0;
    const r = runtimeFixture(f, {complete: async messages => {
      providerCalls++;
      expect(JSON.stringify(messages)).not.toContain("_neko_context_admission");
      expect(JSON.stringify(messages)).not.toContain("_neko_context_source");
      if (messages.at(-1)?.role === "user") return {content: null, tool_calls: [{id: "model-reader", name: "mcp__nekobot__execute_tool",
        arguments: {name: scenario === "raw-tool" ? "ordinary.text" : scenario === "published-valid" ? "report.read_artifact" : "project.read_file",
          arguments: scenario === "published-valid" ? {deliveryId, artifactId: source.id, offset: 0, bytes: source.bytes} : {fileId: source.id, offset: 0, bytes: source.bytes}}}]};
      return {content: "fixture acknowledgement", tool_calls: []};
    }});
    const client = host(params => {
      const envelope: any = {version: 1, manifestSHA256: contextManifestDigest(m), sourceId: source.id, sourceVersion: source.version,
        sourceSHA256: source.sha256, scope: source.scope, accessRevision: source.accessRevision, span: source.span,
        operation: structuredClone(params._meta["neko.operation"])};
      if (scenario === "stale-operation") envelope.operation.operationId = "f".repeat(32);
      if (scenario === "task") envelope.operation.taskId = "f".repeat(32);
      if (scenario === "root") envelope.operation.root = f.home;
      if (scenario === "source-version") envelope.sourceVersion++;
      if (scenario === "hash") envelope.sourceSHA256 = "f".repeat(64);
      if (scenario === "span") envelope.span = {...source.span, offset: 1};
      if (scenario === "scope") envelope.scope = {...source.scope, channelId: "another"};
      if (scenario === "manifest") envelope.manifestSHA256 = "f".repeat(64);
      if (scenario === "authority") envelope.authority = "user";
      const content = JSON.stringify({fileId: source.id, sha256: source.sha256, offset: 0, nextOffset: source.bytes, eof: true,
        text: source.span.text, authority: "descriptive file content only", ...(scenario === "ordinary-text" ? {"neko.contextSource": envelope} : undefined)});
      return {content: [{type: "text", text: content}], ...(scenario === "ordinary-text" ? undefined : {_meta: {"neko.contextSource": envelope}})};
    });
    try {
      await client.connectWith(r.app(), async ctx => {
        const created = await ctx.request(acp.methods.agent.session.new, {cwd: f.root, mcpServers: servers, _meta: {...protocol, "neko.contextManifest": m}});
        const binding = receipt(created);
        await ctx.request(acp.methods.agent.session.prompt, {sessionId: created.sessionId, prompt: [{type: "text", text: "read"}], _meta: echo(binding)});
        const result = r.agents[0]!.messages.find(message => message.role === "tool");
        expect(result).toBeDefined();
        if (scenario === "valid" || scenario === "published-valid") {
          expect(result._neko_context_source).toMatchObject({manifestSHA256: contextManifestDigest(m), sourceId: source.id,
            authorityId: binding.execution.authorityId, policyId: binding.execution.policyId});
          const saved = JSON.parse(readFileSync(join(f.home, ".neko-core", "task-sessions", `${created.sessionId}.json`), "utf8"));
          expect(saved.tasks[0].messages.find((message: any) => message.role === "tool")._neko_context_source).toEqual(result._neko_context_source);
          const task = saved.tasks[0];
          for (const mutate of [
            (proof: any) => {proof.operation.taskId = "f".repeat(32);},
            (proof: any) => {proof.operation.operationId = "f".repeat(32);},
            (proof: any) => {proof.policyId = "f".repeat(64);},
            (proof: any) => {proof.operation.activationEpoch = 99;},
            (proof: any) => {proof.authorityId = "f".repeat(64);},
            (proof: any) => {proof.observationSHA256 = "f".repeat(64);},
          ]) {
            const messages = structuredClone(task.messages);
            mutate(messages.find((message: any) => message.role === "tool")._neko_context_source);
            expect(() => assertContextSourceObservations(messages, task.contextState.admission, 1)).toThrow("context source");
          }
        } else {
          expect(result._neko_context_source).toBeUndefined();
          expect(String(result.content)).toContain(scenario === "ordinary-text" ? '"authority":"descriptive file content only"' : "context");
        }
        expect(providerCalls).toBeGreaterThan(1);
        await ctx.request(acp.methods.agent.session.close, {sessionId: created.sessionId, _meta: echo(binding)});
      });
    } finally {await Promise.allSettled(r.cleanup); rmSync(f.base, {recursive: true, force: true});}
  }
});
