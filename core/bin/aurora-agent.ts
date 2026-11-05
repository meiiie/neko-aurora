import { z } from "zod";
import { AuroraAgentStart, AuroraCompletion, AuroraHostInput, AuroraToolResult, runAuroraAgent, type AuroraAgentEvent, type AuroraAgentSnapshot, type AuroraToolRequest } from "../src/adapters/aurora-agent.ts";
import { auroraCompletionError } from "../src/adapters/aurora-completion.ts";
import { AURORA_ROUTES } from "../src/adapters/aurora-models.ts";
import { loadConfig } from "../src/adapters/config.ts";
import { getProvider } from "../src/adapters/providers.ts";

const LIMIT = 8 * 1024 * 1024;
const controller = new AbortController();
const HostResult = z.union([z.null(), AuroraCompletion, AuroraHostInput, AuroraToolResult]);
type HostResult = z.infer<typeof HostResult>;
type HostMethod = "checkpoint" | "inputs" | "tool" | "completion";
type HostParams = AuroraAgentSnapshot | Record<string, never> | { answer: string } | (AuroraToolRequest & { operationId: string });
type ChildFrame = { type: "request"; id: string; method: HostMethod; params: HostParams }
  | ({ type: "event" } & AuroraAgentEvent)
  | { type: "result"; text: string; completion: z.infer<typeof AuroraCompletion> }
  | { type: "error"; code: string };
const pending = new Map<string, { resolve(value: HostResult): void; reject(error: Error): void }>();
const Response = z.object({ type: z.literal("response"), id: z.string().regex(/^r[0-9]+$/),
  result: HostResult.optional(), error: z.string().max(500).optional() }).strict();
let sequence = 0, buffer = Buffer.alloc(0), initialized = false;
let acceptStart: (value: z.infer<typeof AuroraAgentStart>) => void;
let rejectStart: (error: Error) => void;
const initial = new Promise<z.infer<typeof AuroraAgentStart>>((resolve, reject) => { acceptStart = resolve; rejectStart = reject; });
function stop(reason: string): void {
  const error = new Error(reason);
  controller.abort(error);
  rejectStart(error);
  for (const call of pending.values()) call.reject(error);
  pending.clear();
}
function send(value: ChildFrame): void {
  const frame = JSON.stringify(value) + "\n";
  if (Buffer.byteLength(frame) > LIMIT) throw new Error("core_frame_too_large");
  process.stdout.write(frame);
}
function rpc(method: HostMethod, params: HostParams): Promise<HostResult> {
  if (controller.signal.aborted) return Promise.reject(new Error("core_host_disconnected"));
  const id = `r${++sequence}`;
  return new Promise((resolve, reject) => {
    pending.set(id, { resolve, reject });
    try { send({ type: "request", id, method, params }); }
    catch (error) { pending.delete(id); reject(error); }
  });
}
function receive(line: Buffer): void {
  const value: unknown = JSON.parse(line.toString("utf8"));
  if (!initialized) {
    const start = z.object({ type: z.literal("start"), request: AuroraAgentStart }).strict().parse(value);
    initialized = true;
    acceptStart(start.request);
    return;
  }
  const response = Response.parse(value), call = pending.get(response.id);
  if (!call || (response.error !== undefined && response.result !== undefined)) throw new Error("core_protocol_invalid");
  pending.delete(response.id);
  if (response.error) call.reject(new Error(response.error));
  else call.resolve(HostResult.parse(response.result));
}
process.on("SIGTERM", () => stop("core_cancelled"));
process.on("SIGINT", () => stop("core_cancelled"));
process.stdin.on("error", () => stop("core_host_disconnected"));
process.stdin.on("end", () => stop("core_host_disconnected"));
process.stdin.on("data", (chunk: Buffer) => {
  try {
    buffer = Buffer.concat([buffer, chunk]);
    let index: number;
    while ((index = buffer.indexOf(10)) >= 0) {
      if (index > LIMIT) throw new Error("core_frame_too_large");
      const line = buffer.subarray(0, index);
      buffer = buffer.subarray(index + 1);
      receive(line);
    }
    if (buffer.length > LIMIT) throw new Error("core_frame_too_large");
  } catch { stop("core_protocol_invalid"); }
});

try {
  const profile = process.argv[2];
  if (!AURORA_ROUTES.some(route => route === profile)) throw new Error("core_configuration_invalid");
  const start = await initial;
  const config = loadConfig({ profile });
  const result = await runAuroraAgent(start, getProvider(config), {
    checkpoint: async snapshot => { await rpc("checkpoint", snapshot); },
    inputs: async () => AuroraHostInput.parse(await rpc("inputs", {})),
    execute: async (request, operationId) => AuroraToolResult.parse(await rpc("tool", { ...request, operationId })),
    verify: async answer => AuroraCompletion.parse(await rpc("completion", { answer })),
    event: event => send({ type: "event", ...event }),
  }, controller.signal);
  send({ type: "result", ...result });
} catch (error) {
  send({ type: "error", code: auroraCompletionError(error instanceof Error ? error : new Error("core_failed")) });
  process.exitCode = 1;
} finally {
  stop("core_session_closed");
  process.stdin.destroy();
}
