import { NekoBotBridgeError, exact, opaque, parseMessage, parseSource, type NekoBotLocalSource } from "./nekobot-local-protocol";
import { HISTORY_MAX_BYTES, HISTORY_MAX_ROWS, HISTORY_PAGE_BYTES, parseHistoryRow, rowBytes, sha256, uniqueHistoryRows, utf8, type HistoryRow } from "./history-repair";
import type { NekoBotBridgeSnapshot, NekoBotOutboxItem } from "./nekobot-local-bridge";

export interface HistoryStorage { read(key: string): Promise<unknown>; write(key: string, value: unknown): Promise<void> }
export interface HistoryMetadata { snapshotId: string; snapshotCursor: number; authorityDigest: string; historyDigest: string }
export interface HistoryState { snapshot: NekoBotBridgeSnapshot; receipts: NekoBotOutboxItem[]; rows: HistoryRow[]; repaired: HistoryMetadata | null }
interface Manifest { version: 2; source: NekoBotLocalSource; snapshot: NekoBotBridgeSnapshot; pending: string[]; receipts: string[]; rows: string[]; repaired: HistoryMetadata | null }
export interface HistoryCandidate { generation: string; state: HistoryState }
const fail = (): never => { throw new NekoBotBridgeError("invalid_saved_history"); };
const digest = (x: unknown): string => typeof x === "string" && /^[a-f0-9]{64}$/.test(x) ? x : fail();
export function canonicalStorageJSON(value: unknown): string {
  const order = (entry: unknown): unknown => Array.isArray(entry) ? entry.map(order)
    : entry && typeof entry === "object" ? Object.fromEntries(Object.keys(entry).sort().map(key => [key, order((entry as Record<string, unknown>)[key])])) : entry;
  return JSON.stringify(order(value));
}
function receipt(value: unknown): NekoBotOutboxItem {
  const x = exact(value, ["input", "requestId", "state", "errorCode"]);
  if (x.state !== "accepted" || x.errorCode !== null) fail();
  return { input: parseMessage(x.input), requestId: opaque(x.requestId), state: "accepted", errorCode: null };
}
function pendingItem(value: unknown): NekoBotOutboxItem {
  const x = exact(value, ["input", "requestId", "state", "errorCode"]);
  if (!["queued", "uncertain", "rejected"].includes(String(x.state)) || x.requestId !== null
    || !(x.errorCode === null || typeof x.errorCode === "string" && /^[a-z0-9_]{1,80}$/.test(x.errorCode))) fail();
  return { input: parseMessage(x.input), requestId: null, state: x.state as NekoBotOutboxItem["state"], errorCode: x.errorCode as string | null };
}
function metadata(value: unknown): HistoryMetadata | null {
  if (value === null) return null;
  const x = exact(value, ["snapshotId", "snapshotCursor", "authorityDigest", "historyDigest"]);
  if (typeof x.snapshotId !== "string" || !/^h1_[a-f0-9]{64}$/.test(x.snapshotId) || !Number.isSafeInteger(x.snapshotCursor) || Number(x.snapshotCursor) < 0) fail();
  return { snapshotId: x.snapshotId as string, snapshotCursor: Number(x.snapshotCursor), authorityDigest: digest(x.authorityDigest), historyDigest: digest(x.historyDigest) };
}
export class NekoBotHistoryCache {
  current: HistoryState | null = null;
  private stored = new Set<string>();
  constructor(private readonly source: NekoBotLocalSource, private readonly storage: HistoryStorage,
    private readonly parseSnapshot: (value: unknown) => NekoBotBridgeSnapshot) {}
  private async content(key: string): Promise<unknown> {
    const value = await this.storage.read(key);
    if (await sha256(utf8(canonicalStorageJSON(value))) !== key.slice(key.indexOf("/") + 1)) fail();
    this.stored.add(key); return value;
  }
  async load(): Promise<HistoryState | null> {
    const pointer = await this.storage.read("active");
    if (pointer === undefined || pointer === null) return null;
    const p = exact(pointer, ["version", "generation"]); if (p.version !== 2) fail();
    const generation = digest(p.generation), x = exact(await this.content("generation/" + generation), ["version", "source", "snapshot", "pending", "receipts", "rows", "repaired"]);
    if (x.version !== 2 || JSON.stringify(parseSource(x.source)) !== JSON.stringify(parseSource(this.source))) fail();
    const read = async <T>(keys: unknown, kind: string, parse: (entry: unknown) => T): Promise<T[]> => {
      if (!Array.isArray(keys) || keys.length > HISTORY_MAX_ROWS) fail();
      const all: T[] = []; let bytes = 0;
      for (const hash of keys as unknown[]) {
        const shard = exact(await this.content("shard/" + digest(hash)), ["version", "kind", "entries"]);
        const size = utf8(JSON.stringify(shard)).length; bytes += size;
        if (size > HISTORY_PAGE_BYTES || bytes > HISTORY_MAX_BYTES || shard.version !== 2 || shard.kind !== kind
          || !Array.isArray(shard.entries) || !shard.entries.length || shard.entries.length > 100) fail();
        all.push(...(shard.entries as unknown[]).map(parse)); if (all.length > HISTORY_MAX_ROWS) fail();
      }
      return all;
    };
    const base = this.parseSnapshot(x.snapshot);
    if (base.outbox.length || base.replies.length) fail();
    const pending = await read(x.pending, "pending", pendingItem);
    const snapshot = this.parseSnapshot({ ...base, outbox: pending }), receipts = await read(x.receipts, "receipts", receipt), rows = await read(x.rows, "rows", parseHistoryRow);
    const state = { snapshot, receipts, rows, repaired: metadata(x.repaired) }; this.validate(state); this.current = state; return structuredClone(state);
  }
  private validate(state: HistoryState): void {
    if (state.receipts.length > HISTORY_MAX_ROWS || state.rows.length > HISTORY_MAX_ROWS
      || state.rows.reduce((n, row) => n + rowBytes(row).length, 0) > HISTORY_MAX_BYTES) fail();
    uniqueHistoryRows(state.rows);
    const originals = [...state.receipts, ...state.snapshot.outbox];
    for (const key of ["sourceMessageId", "clientNonce"] as const) if (new Set(originals.map(item => item.input[key])).size !== originals.length) fail();
    if (new Set(state.receipts.map(item => item.requestId)).size !== state.receipts.length) fail();
    for (const row of state.rows) {
      const item = originals.find(item => item.input.sourceMessageId === row.sourceMessageId);
      if (item && (item.input.botId !== row.botId || item.requestId !== null && item.requestId !== row.requestId || item.state === "rejected")) fail();
    }
  }
  private async immutable(kind: string, value: unknown): Promise<string> {
    const encoded = utf8(canonicalStorageJSON(value)); if (encoded.length > HISTORY_PAGE_BYTES) throw new NekoBotBridgeError("history_storage_budget_exceeded");
    const hash = await sha256(encoded), key = kind + "/" + hash;
    if (!this.stored.has(key)) { await this.storage.write(key, structuredClone(value)); this.stored.add(key); }
    return hash;
  }
  async stage(snapshot: NekoBotBridgeSnapshot, receipts: NekoBotOutboxItem[], rows: HistoryRow[], repaired: HistoryMetadata | null): Promise<HistoryCandidate> {
    const accepted = snapshot.outbox.filter(item => item.state === "accepted"), originalReceipts = [...receipts, ...accepted];
    const pending = snapshot.outbox.filter(item => item.state !== "accepted");
    const state: HistoryState = { snapshot: this.parseSnapshot({ ...snapshot, outbox: pending, replies: [] }), receipts: originalReceipts.map(receipt), rows: rows.map(parseHistoryRow), repaired: metadata(repaired) };
    this.validate(state);
    const shards = async (kind: string, entries: unknown[]): Promise<string[]> => {
      const keys: string[] = []; let chunk: unknown[] = [], bytes = 0;
      const flush = async () => { if (chunk.length) { const value = { version: 2, kind, entries: chunk }; bytes += utf8(JSON.stringify(value)).length;
        if (bytes > HISTORY_MAX_BYTES) throw new NekoBotBridgeError("history_storage_budget_exceeded"); keys.push(await this.immutable("shard", value)); chunk = []; } };
      for (const entry of entries) {
        if (chunk.length >= 100 || utf8(JSON.stringify({ version: 2, kind, entries: [...chunk, entry] })).length > HISTORY_PAGE_BYTES) await flush();
        chunk.push(entry);
      }
      await flush(); return keys;
    };
    const manifest: Manifest = { version: 2, source: this.source, snapshot: { ...state.snapshot, outbox: [] }, pending: await shards("pending", state.snapshot.outbox),
      receipts: await shards("receipts", state.receipts), rows: await shards("rows", state.rows), repaired: state.repaired };
    const generation = await this.immutable("generation", manifest); return { generation, state };
  }
  async activate(candidate: HistoryCandidate): Promise<void> {
    await this.storage.write("active", { version: 2, generation: candidate.generation });
    this.current = structuredClone(candidate.state);
  }
}
