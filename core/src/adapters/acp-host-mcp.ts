import {admitContextSourceObservation, type ContextSourceObservation, type ContextManifest} from "../core/context/admission.ts";
import { hostEffectRequestDigest, parseHostEffectPending, type HostEffectDescriptor } from "../core/host-effect.ts";
/** A deliberately small MCP client carried over the existing ACP connection.
 *
 * Unlike configured MCP, this adapter never starts a process and never accepts a URL. The launch
 * authorized host profile is the authority ceiling; the ACP client merely supplies an in-band
 * implementation of that exact surface for one session.
 */
import * as acp from "@agentclientprotocol/sdk";
import { LATEST_PROTOCOL_VERSION, SUPPORTED_PROTOCOL_VERSIONS } from "@modelcontextprotocol/sdk/types.js";

import type { McpTools } from "../core/ports.ts";
import { assertToolOperation, type ToolOperationContext } from "../core/tool-operation.ts";
import { assertTaskScope, type TaskScope } from "../core/task-scope.ts";
import { isJsonObject, isText, type JsonObject, type JsonValue } from "../shared/wire.ts";
import { hostToolName, type HostCapabilityProfile } from "./host-profile.ts";
import { containsCredentialJSON, type CredentialSecrets } from "../shared/credential-output.ts";

const CONNECT_TIMEOUT_MS = 15_000;
const CALL_TIMEOUT_MS = 120_000;
const MAX_TOOL_PAGES = 8;
const MAX_TOOL_SCHEMA_BYTES = 256 * 1024;

function cancellationSignal(parent: AbortSignal | undefined, timeoutMs: number): AbortSignal {
  const timeout = AbortSignal.timeout(timeoutMs);
  return parent ? AbortSignal.any([parent, timeout]) : timeout;
}

interface HostToolSchema {
  type: "function";
  function: {
    name: string;
    description: string;
    parameters: JsonObject;
  };
}

function objectResult(value: JsonValue, label: string): JsonObject {
  if (!isJsonObject(value)) throw new Error(`${label} returned an invalid response`);
  return value;
}

function contentText(result: JsonObject): string {
  const parts = Array.isArray(result.content) ? result.content.map((part: JsonValue) => {
    if (isJsonObject(part) && part.type === "text" && isText(part.text)) return part.text;
    try { return JSON.stringify(part) ?? String(part); } catch { return String(part); }
  }) : [];
  const text = parts.filter(Boolean).join("\n") || "(no content)";
  return result.isError === true && !/^Error(?:\s|:|$)/i.test(text.trimStart()) ? `Error: ${text}` : text;
}

interface HostOperationBinding {
  readonly contextManifest?: ContextManifest;
  readonly sessionId: string;
  readonly scope: TaskScope;
  readonly hostEffectProtocol?: 1;
}

export class AcpHostMcp implements McpTools {
  get requiresOperationContext(): boolean { return this.profile.operationProtocol === 1; }
  private readonly specs: HostToolSchema[] = [];
  private readonly rawNames = new Map<string, string>();
  private closed = false;
  private contextPolicyId?: string;
  private credential?: () => CredentialSecrets;
  bindCredential(reader: () => CredentialSecrets): void {
    if (this.credential) throw new Error("Host credential guard already bound");
    this.credential = reader;
  }
  private readonly pendingSources = new Map<string, ContextSourceObservation>();
  bindContextPolicy(policyId: string): void {
    if (this.contextPolicyId || !this.binding?.contextManifest || !/^[a-f0-9]{64}$/.test(policyId)) throw new Error("Invalid context source policy binding");
    this.contextPolicyId = policyId;
  }
  takeContextSource(operationId: string): ContextSourceObservation | undefined {
    const source = this.pendingSources.get(operationId); this.pendingSources.delete(operationId); return source;
  }
  private readonly pendingEffects = new Map<string, HostEffectDescriptor>();

  takeHostEffect(operationId: string): HostEffectDescriptor | undefined {
    const pending = this.pendingEffects.get(operationId);
    this.pendingEffects.delete(operationId);
    return pending;
  }

  private constructor(
    private readonly profile: HostCapabilityProfile,
    private readonly client: acp.AgentContext,
    private readonly connectionId: string,
    tools: JsonObject[],
    private readonly binding?: HostOperationBinding,
  ) {
    const declared = new Map(profile.tools.map((tool) => [tool.name, tool]));
    for (const tool of tools) {
      const rawName = isText(tool?.name) ? tool.name : "";
      if (!declared.has(rawName)) continue;
      const name = hostToolName(profile, rawName);
      this.rawNames.set(name, rawName);
      this.specs.push({
        type: "function",
        function: {
          name,
          description: isText(tool.description) ? tool.description : "",
          parameters: isJsonObject(tool.inputSchema)
            ? tool.inputSchema
            : { type: "object", properties: {}, additionalProperties: false },
        },
      });
    }
    const missing = profile.tools.filter((tool) => !this.rawNames.has(hostToolName(profile, tool.name)));
    if (missing.length) throw new Error(`host MCP is missing required tools: ${missing.map((tool) => tool.name).join(", ")}`);
  }

  static async connect(
    profile: HostCapabilityProfile,
    client: acp.AgentContext,
    server: acp.McpServerAcp & { type: "acp" },
    binding?: HostOperationBinding,
  ): Promise<AcpHostMcp> {
    if (profile.operationProtocol) {
      if (!binding || !/^[a-f0-9]{32}$/.test(binding.sessionId) || !binding.scope.executionAuthorityId) {
        throw new Error("Host operation protocol requires a bound fixed task and execution authority");
      }
      assertTaskScope(binding.scope);
    }
    const connected = objectResult(await client.request<JsonValue>("mcp/connect", {
      serverId: server.serverId,
    }, { cancellationSignal: cancellationSignal(undefined, CONNECT_TIMEOUT_MS) }), "mcp/connect");
    if (!isText(connected.connectionId) || !connected.connectionId) {
      throw new Error("mcp/connect did not return a connectionId");
    }
    const connectionId = connected.connectionId;
    try {
      const initialized = objectResult(await client.request<JsonValue>("mcp/message", {
        connectionId,
        method: "initialize",
        params: {
          protocolVersion: LATEST_PROTOCOL_VERSION,
          capabilities: {},
          clientInfo: { name: "neko-core", version: "1" },
        },
      }, { cancellationSignal: cancellationSignal(undefined, CONNECT_TIMEOUT_MS) }), "MCP initialize");
      if (!isText(initialized.protocolVersion) || !SUPPORTED_PROTOCOL_VERSIONS.includes(initialized.protocolVersion)) {
        throw new Error(`host MCP selected unsupported protocol '${String(initialized.protocolVersion ?? "")}'`);
      }
      await client.notify("mcp/message", {
        connectionId,
        method: "notifications/initialized",
        params: {},
      });

      const tools: JsonObject[] = [];
      let cursor: string | undefined;
      for (let page = 0; page < MAX_TOOL_PAGES; page++) {
        const listed = objectResult(await client.request<JsonValue>("mcp/message", {
          connectionId,
          method: "tools/list",
          params: cursor ? { cursor } : {},
        }, { cancellationSignal: cancellationSignal(undefined, CONNECT_TIMEOUT_MS) }), "MCP tools/list");
        if (!Array.isArray(listed.tools)) throw new Error("MCP tools/list returned no tools array");
        for (const tool of listed.tools) {
          if (!isJsonObject(tool)) throw new Error("MCP tools/list returned an invalid tool descriptor");
          tools.push(tool);
        }
        if (Buffer.byteLength(JSON.stringify(tools), "utf8") > MAX_TOOL_SCHEMA_BYTES) {
          throw new Error("host MCP tool surface exceeds 256 KiB");
        }
        cursor = isText(listed.nextCursor) && listed.nextCursor ? listed.nextCursor : undefined;
        if (!cursor) return new AcpHostMcp(profile, client, connectionId, tools, binding);
      }
      throw new Error(`host MCP tools/list exceeded ${MAX_TOOL_PAGES} pages`);
    } catch (error) {
      await client.request("mcp/disconnect", { connectionId }).catch(() => {});
      throw error;
    }
  }

  toolSchemas(): HostToolSchema[] { return this.specs; }

  has(name: string): boolean { return !this.closed && this.rawNames.has(name); }

  permission(name: string): "safe" | "gated" {
    const rawName = this.rawNames.get(name);
    return this.profile.tools.find((tool) => tool.name === rawName)?.permission ?? "gated";
  }

  async call(name: string, args: any, signal?: AbortSignal, operation?: ToolOperationContext): Promise<string> {
    if (isJsonObject(args) && containsCredentialJSON(args, this.credential?.() ?? "")) {
      return "Error: configured credential in host tool input refused";
    }
    const rawName = this.rawNames.get(name);
    if (this.closed || !rawName) return `Error: unknown host MCP tool ${name}`;
    try {
      signal?.throwIfAborted();
      let meta: JsonObject | undefined;
      if (this.requiresOperationContext) {
        assertToolOperation(operation);
        if (!this.binding) throw new Error("Host operation binding is unavailable");
        const { sessionId, scope } = this.binding;
        assertTaskScope(scope);
        meta = { "neko.operation": {
          version: 1, operationId: operation.operationId, sessionId,
          taskId: scope.id, root: scope.canonicalRoot,
          activationEpoch: scope.activationEpoch, activationId: scope.activationId,
        } };
      }
      const result = objectResult(await this.client.request<JsonValue>("mcp/message", {
        connectionId: this.connectionId,
        method: "tools/call",
        params: { name: rawName, arguments: isJsonObject(args) ? args : {}, ...(meta ? { _meta: meta } : undefined) },
      }, { cancellationSignal: cancellationSignal(signal, CALL_TIMEOUT_MS) }), `MCP tool '${rawName}'`);
      const effect = isJsonObject(result._meta) ? result._meta["neko.hostEffect"] : undefined;
      if (effect !== undefined) {
        if (!this.binding?.hostEffectProtocol || rawName !== "execute_tool" || !operation || !meta || result.isError === true) {
          throw new Error("Host effect suspension was not negotiated or is not permitted for this call");
        }
        const pending = parseHostEffectPending(effect);
        if (pending.operationId !== operation.operationId
          || pending.requestDigest !== hostEffectRequestDigest(rawName, isJsonObject(args) ? args : {})) {
          throw new Error("Host effect suspension does not match the canonical invocation");
        }
        // SAFETY: this receipt was created here from the runtime-branded operation and task scope.
        const { sessionId, scope } = this.binding;
        const descriptor: HostEffectDescriptor = { ...pending, operation: { version: 1, operationId: operation.operationId,
          sessionId, taskId: scope.id, root: scope.canonicalRoot, activationEpoch: scope.activationEpoch, activationId: scope.activationId } };
        this.pendingEffects.set(operation.operationId, descriptor);
        return "[awaiting host approval; effect has not started]";
      }
      const source = isJsonObject(result._meta) ? result._meta["neko.contextSource"] : undefined;
      if (source !== undefined) {
        if (!this.binding?.contextManifest || !this.contextPolicyId || rawName !== "execute_tool" || !operation || !meta
          || !isJsonObject(meta["neko.operation"]) || result.isError === true) throw new Error("Unnegotiated context source observation");
        const admitted = admitContextSourceObservation(source, this.binding.contextManifest, meta["neko.operation"], args,
          contentText(result), this.binding.scope.executionAuthorityId!, this.contextPolicyId);
        this.pendingSources.set(operation.operationId, admitted);
      }
      return contentText(result);
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      return `Error: host MCP call outcome unknown; not retried: ${message}`;
    }
  }

  async close(): Promise<void> {
    if (this.closed) return;
    this.closed = true;
    this.pendingSources.clear();
    await this.client.request("mcp/disconnect", { connectionId: this.connectionId }, {
      cancellationSignal: cancellationSignal(undefined, CONNECT_TIMEOUT_MS),
    }).catch(() => {});
  }
}
