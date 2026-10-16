import { NekoBotBridgeError, exact, parseReply, sequence, type NekoBotAdapterSession, type NekoBotReply } from "./nekobot-local-protocol";

export const HISTORY_CAPABILITY = "local_adapter_history_repair_v1";
export const HISTORY_MAX_ROWS = 8192;
export const HISTORY_MAX_BYTES = 67108864;
export const HISTORY_PAGE_BYTES = 900000;
export interface HistoryScope {
  adapterSessionId: string; sourceApp: string; sourceSessionId: string; sourceThreadId: string;
  channelId: string; ownerUserId: string; spaceId: string; returnRoute: string; channelRevision: number;
}
export interface HistoryRow extends NekoBotReply { availability: "available" | "source_unavailable" }
export interface HistoryPage extends HistoryScope {
  version: 1; snapshotId: string; snapshotCursor: number; authorityDigest: string; historyDigest: string;
  offset: number; totalRows: number; totalBytes: number; nextCursor: string | null;
  confirmationCursor: string | null; complete: boolean; rows: HistoryRow[];
}
const fail = (code: string): never => { throw new NekoBotBridgeError(code); };
const encoder = new TextEncoder();
export function utf8(value: string): Uint8Array {
  if (typeof value !== "string" || /[\uD800-\uDBFF](?![\uDC00-\uDFFF])|(?<![\uD800-\uDBFF])[\uDC00-\uDFFF]/u.test(value)) fail("history_invalid_unicode");
  return encoder.encode(value);
}
export function concat(parts: Uint8Array[]): Uint8Array {
  const output = new Uint8Array(parts.reduce((n, p) => n + p.length, 0));
  let offset = 0; for (const part of parts) { output.set(part, offset); offset += part.length; } return output;
}
function u32(value: number): Uint8Array {
  if (!Number.isSafeInteger(value) || value < 0 || value > 0xffffffff) fail("history_invalid_integer");
  const out = new Uint8Array(4); new DataView(out.buffer).setUint32(0, value); return out;
}
function u64(value: number): Uint8Array {
  sequence(value); const out = new Uint8Array(8); new DataView(out.buffer).setBigUint64(0, BigInt(value)); return out;
}
function string(value: string): Uint8Array { const bytes = utf8(value); return concat([u32(bytes.length), bytes]); }
function hashBytes(value: unknown): Uint8Array {
  if (typeof value !== "string" || !/^[a-f0-9]{64}$/.test(value)) fail("history_invalid_digest");
  return Uint8Array.from((value as string).match(/../g)!, byte => parseInt(byte, 16));
}
export async function sha256(bytes: Uint8Array): Promise<string> {
  return Array.from(new Uint8Array(await crypto.subtle.digest("SHA-256", new Uint8Array(bytes).buffer)), b => b.toString(16).padStart(2, "0")).join("");
}
export function scopeBytes(scope: HistoryScope): Uint8Array {
  return concat([scope.adapterSessionId, scope.sourceApp, scope.sourceSessionId, scope.sourceThreadId,
    scope.channelId, scope.ownerUserId, scope.spaceId, scope.returnRoute].map(string).concat([u64(scope.channelRevision)]));
}
export function rowBytes(row: HistoryRow): Uint8Array {
  return concat([...[row.replyId, row.requestId, row.sourceMessageId, row.botId, row.role, row.status, row.availability].map(string),
    u64(row.sequence), string(row.text), u32(row.artifacts.length), ...row.artifacts.flatMap(a => [string(a.id), string(a.filename), string(a.sha256), u64(a.bytes)])]);
}
export function sessionScope(session: NekoBotAdapterSession): HistoryScope {
  return { adapterSessionId: session.id, sourceApp: session.sourceApp, sourceSessionId: session.sourceSessionId,
    sourceThreadId: session.sourceThreadId, channelId: session.channelId, ownerUserId: session.ownerUserId,
    spaceId: session.spaceId, returnRoute: session.returnRoute, channelRevision: session.channelRevision };
}
export async function historyWitness(scope: HistoryScope, cursor: number, authorityDigest: string, rows: HistoryRow[]) {
  const encoded = rows.map(rowBytes), totalBytes = encoded.reduce((n, bytes) => n + bytes.length, 0);
  if (rows.length > HISTORY_MAX_ROWS || totalBytes > HISTORY_MAX_BYTES) fail("adapter_history_budget_exceeded");
  const historyDigest = await sha256(concat([utf8("neko-adapter-history-v1\0"), scopeBytes(scope), u64(cursor), hashBytes(authorityDigest), u64(rows.length), ...encoded]));
  const snapshotId = "h1_" + await sha256(concat([utf8("neko-adapter-snapshot-v1\0"), scopeBytes(scope), u64(cursor), hashBytes(authorityDigest), hashBytes(historyDigest)]));
  return { totalBytes, historyDigest, snapshotId };
}
export function parseHistoryRow(value: unknown): HistoryRow {
  const x = exact(value, ["replyId", "requestId", "sourceMessageId", "botId", "role", "text", "status", "sequence", "artifacts", "availability"]);
  if (x.availability !== "available" && x.availability !== "source_unavailable") fail("history_invalid_availability");
  const { availability, ...reply } = x, parsed = parseReply(reply);
  if (utf8(parsed.text).length > 65536 || parsed.artifacts.length > 8 || parsed.artifacts.some(file => utf8(file.filename).length > 80)
    || availability === "source_unavailable" && (parsed.text !== "" || parsed.artifacts.length)) fail("history_invalid_row");
  const row = { ...parsed, availability } as HistoryRow; rowBytes(row); return row;
}
export function historyCursor(value: unknown): string {
  if (typeof value !== "string" || !/^[A-Za-z0-9_-]{215}$/.test(value)) fail("adapter_history_cursor_invalid");
  let decoded: string;
  try { decoded = atob((value as string).replace(/-/g, "+").replace(/_/g, "/")); } catch { return fail("adapter_history_cursor_invalid"); }
  if (decoded.length !== 161 || decoded.charCodeAt(0) !== 1 || btoa(decoded).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "") !== value) fail("adapter_history_cursor_invalid");
  return value as string;
}
export function parseHistoryPage(value: unknown, session: NekoBotAdapterSession): HistoryPage {
  const x = exact(value, ["version", "adapterSessionId", "sourceApp", "sourceSessionId", "sourceThreadId", "channelId", "channelRevision", "ownerUserId", "spaceId", "returnRoute",
    "snapshotId", "snapshotCursor", "authorityDigest", "historyDigest", "offset", "totalRows", "totalBytes", "nextCursor", "confirmationCursor", "complete", "rows"]);
  const scope = sessionScope(session);
  if (Object.entries(scope).some(([key, value]) => x[key] !== value)) fail("adapter_history_binding_mismatch");
  if (x.version !== 1 || typeof x.snapshotId !== "string" || !/^h1_[a-f0-9]{64}$/.test(x.snapshotId)) fail("history_invalid_snapshot");
  hashBytes(x.authorityDigest); hashBytes(x.historyDigest);
  const snapshotCursor = sequence(x.snapshotCursor), offset = sequence(x.offset), totalRows = sequence(x.totalRows), totalBytes = sequence(x.totalBytes);
  if (totalRows > HISTORY_MAX_ROWS || totalBytes > HISTORY_MAX_BYTES || !Array.isArray(x.rows) || x.rows.length > 128
    || offset + x.rows.length > totalRows || x.complete !== (offset + x.rows.length === totalRows)) fail("history_invalid_page");
  const rows = (x.rows as unknown[]).map(parseHistoryRow);
  if (rows.some(r => r.sequence > snapshotCursor) || !x.complete && !rows.length) fail("history_invalid_page");
  const nextCursor = x.nextCursor === null ? null : historyCursor(x.nextCursor);
  const confirmationCursor = x.confirmationCursor === null ? null : historyCursor(x.confirmationCursor);
  if (x.complete ? nextCursor !== null || confirmationCursor === null : nextCursor === null || confirmationCursor !== null) fail("history_invalid_cursor_state");
  if (utf8(JSON.stringify(value)).length > HISTORY_PAGE_BYTES) fail("response_oversized");
  return { ...scope, version: 1, snapshotId: String(x.snapshotId), snapshotCursor, authorityDigest: String(x.authorityDigest), historyDigest: String(x.historyDigest),
    offset, totalRows, totalBytes, nextCursor, confirmationCursor, complete: x.complete as boolean, rows };
}
export function sameHistorySnapshot(a: HistoryPage, b: HistoryPage): boolean {
  return ["snapshotId", "snapshotCursor", "authorityDigest", "historyDigest", "totalRows", "totalBytes"].every(key => a[key as keyof HistoryPage] === b[key as keyof HistoryPage]);
}
export function uniqueHistoryRows(rows: HistoryRow[]): void {
  for (const key of ["replyId", "requestId", "sourceMessageId"] as const) if (new Set(rows.map(row => row[key])).size !== rows.length) fail("history_duplicate_identity");
}
export const HISTORY_ERRORS = new Set(["adapter_history_cursor_invalid", "adapter_history_binding_mismatch", "adapter_history_scope_changed",
  "adapter_history_authority_changed", "adapter_history_snapshot_expired", "adapter_history_budget_exceeded", "not_found", "store_unavailable"]);
