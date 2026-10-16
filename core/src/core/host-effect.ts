/** Trusted host suspension receipts. Text/model arguments cannot create a pending operation. */
/* eslint-disable anti-slop/no-unknown-parameters, anti-slop/no-runtime-typeof -- Validates persisted and protocol JSON at a trust boundary. */
import { createHash } from "node:crypto";
import { isJsonObject, type JsonObject, type JsonValue } from "../shared/wire.ts";

export interface HostOperationReceipt {
  version: 1; operationId: string; sessionId: string; taskId: string; root: string;
  activationEpoch: number; activationId: string;
}
export interface HostEffectPending {
  version: 1; status: "awaiting_approval"; operationId: string; effectId: string;
  requestDigest: string; bindingDigest: string; effectStarted: false;
}
export interface HostEffectDescriptor extends HostEffectPending { operation: HostOperationReceipt; }
export interface HostEffectResolution {
  version: 1; operationId: string; effectId: string; requestDigest: string; bindingDigest: string;
  status: "completed" | "denied" | "unknown"; result: string; resultDigest: string;
}
/** Each entry is append-only: one immutable pending event, at most one immutable terminal event. */
export interface HostEffectRecord { pending: HostEffectDescriptor; resolution?: HostEffectResolution; }
const id = /^[a-f0-9]{32}$/;
const digest = /^[a-f0-9]{64}$/;
const exact = (value: JsonObject, keys: string) => Object.keys(value).sort().join(",") === keys.split(",").sort().join(",");
export function hostEffectResultDigest(value: string): string { return createHash("sha256").update(value).digest("hex"); }
export function canonicalHostJson(value: JsonValue): string {
  if (Array.isArray(value)) return `[${value.map(canonicalHostJson).join(",")}]`;
  if (isJsonObject(value)) return `{${Object.keys(value).sort().map(key => `${JSON.stringify(key)}:${canonicalHostJson(value[key]!)}`).join(",")}}`;
  return JSON.stringify(value);
}
export function hostEffectRequestDigest(name: string, args: JsonObject): string {
  return hostEffectResultDigest(canonicalHostJson({ name, arguments: args }));
}
export function parseHostEffectPending(value: unknown): HostEffectPending {
  if (!isJsonObject(value) || !exact(value, "version,status,operationId,effectId,requestDigest,bindingDigest,effectStarted")
    || value.version !== 1 || value.status !== "awaiting_approval" || value.effectStarted !== false
    || typeof value.operationId !== "string" || !id.test(value.operationId)
    || typeof value.effectId !== "string" || !/^[A-Za-z0-9:._-]{1,256}$/.test(value.effectId)
    || typeof value.requestDigest !== "string" || !digest.test(value.requestDigest)
    || typeof value.bindingDigest !== "string" || !digest.test(value.bindingDigest)) throw new Error("Invalid host effect pending receipt");
  // SAFETY: every member and the exact schema were validated above.
  return { version: 1, status: "awaiting_approval", operationId: value.operationId, effectId: value.effectId,
    requestDigest: value.requestDigest, bindingDigest: value.bindingDigest, effectStarted: false };
}
export function parseHostEffectResolution(value: unknown): HostEffectResolution {
  if (!isJsonObject(value) || !exact(value, "version,status,operationId,effectId,requestDigest,bindingDigest,result,resultDigest")
    || value.version !== 1 || !["completed", "denied", "unknown"].includes(String(value.status))
    || typeof value.result !== "string" || Buffer.byteLength(value.result) > 256 * 1024
    || typeof value.resultDigest !== "string" || value.resultDigest !== hostEffectResultDigest(value.result)) {
    throw new Error("Invalid host effect resolution");
  }
  // SAFETY: exact schema, discriminants, digest and shared binding fields were validated above.
  const pending = parseHostEffectPending({ version: value.version, status: "awaiting_approval", operationId: value.operationId,
    effectId: value.effectId, requestDigest: value.requestDigest, bindingDigest: value.bindingDigest, effectStarted: false });
  // SAFETY: status was validated against the exact three terminal status values above.
  return { version: 1, operationId: pending.operationId, effectId: pending.effectId,
    requestDigest: pending.requestDigest, bindingDigest: pending.bindingDigest,
    status: value.status as HostEffectResolution["status"], result: value.result, resultDigest: value.resultDigest };
}
export function sameHostEffectBinding(a: HostEffectPending, b: HostEffectResolution): boolean {
  return a.operationId === b.operationId && a.effectId === b.effectId && a.requestDigest === b.requestDigest && a.bindingDigest === b.bindingDigest;
}
export function validateHostEffects(value: unknown, sessionId: string, taskId: string, root: string): HostEffectRecord[] {
  if (!Array.isArray(value) || value.length > 10_000) throw new Error("Invalid host effect journal");
  const ids = new Set<string>();
  const effects = new Set<string>();
  return value.map(entry => {
    if (!isJsonObject(entry) || !isJsonObject(entry.pending)
      || !exact(entry, entry.resolution === undefined ? "pending" : "pending,resolution")) throw new Error("Invalid host effect record");
    const { operation, ...receipt } = entry.pending;
    const pending = parseHostEffectPending(receipt);
    if (!isJsonObject(operation) || !exact(operation, "version,operationId,sessionId,taskId,root,activationEpoch,activationId")
      || operation.version !== 1 || operation.operationId !== pending.operationId
      || operation.sessionId !== sessionId || operation.taskId !== taskId || operation.root !== root
      || !Number.isSafeInteger(operation.activationEpoch) || Number(operation.activationEpoch) < 1
      || typeof operation.activationId !== "string" || !id.test(operation.activationId)
      || ids.has(pending.operationId) || effects.has(pending.effectId)) throw new Error("Invalid host effect operation binding");
    ids.add(pending.operationId); effects.add(pending.effectId);
    const resolution = entry.resolution === undefined ? undefined : parseHostEffectResolution(entry.resolution);
    if (resolution && !sameHostEffectBinding(pending, resolution)) throw new Error("Host effect resolution binding changed");
    // SAFETY: exact operation schema and each scope field checked above.
    return { pending: { ...pending, operation: { version: 1 as const, operationId: pending.operationId, sessionId, taskId, root,
      activationEpoch: Number(operation.activationEpoch), activationId: operation.activationId } }, ...(resolution ? { resolution } : undefined) };
  });
}
