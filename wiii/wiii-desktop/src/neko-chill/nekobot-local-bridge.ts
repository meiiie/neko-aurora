import {
  NekoBotBridgeError, NEKOBOT_LOCAL_MAX_PAGE_BYTES, exact, nonce, opaque, parseMessage, parsePage,
  parseReply, parseSession, parseSource, record, sequence,
  type NekoBotAdapterSession, type NekoBotLocalSource, type NekoBotMessageInput, type NekoBotReply,
} from "./nekobot-local-protocol";
import { NekoBotHistoryCache, type HistoryMetadata, type HistoryStorage } from "./history-cache";
import { HISTORY_CAPABILITY, HISTORY_ERRORS, HISTORY_PAGE_BYTES, historyWitness, parseHistoryPage, sameHistorySnapshot,
  sessionScope, uniqueHistoryRows, type HistoryPage, type HistoryRow } from "./history-repair";

export interface NekoBotTrustedTransport {
  isCurrent?(): boolean;
  capabilities?: readonly string[];
  request(method: "GET" | "POST", relativePath: string, body?: unknown): Promise<{ status: number; body: string }>;
}
export interface NekoBotBridgeStorage {
  history?: HistoryStorage;
  read(): Promise<unknown>;
  write(snapshot: NekoBotBridgeSnapshot): Promise<void>;
}
export interface NekoBotOutboxItem {
  input: NekoBotMessageInput;
  requestId: string | null;
  state: "queued" | "uncertain" | "rejected" | "accepted";
  errorCode: string | null;
}
export interface NekoBotBridgeSnapshot {
  version: 1;
  source: NekoBotLocalSource;
  registrationNonce: string;
  adapterSession: NekoBotAdapterSession | null;
  cursor: number;
  outbox: NekoBotOutboxItem[];
  replies: NekoBotReply[];
  blocked: boolean;
}

export function emptyNekoBotBridge(source: NekoBotLocalSource, registrationNonce: string): NekoBotBridgeSnapshot {
  return { version: 1, source: parseSource(source), registrationNonce: nonce(registrationNonce), adapterSession: null,
    cursor: 0, outbox: [], replies: [], blocked: false };
}

export function parseNekoBotBridgeSnapshot(value: unknown, source: NekoBotLocalSource): NekoBotBridgeSnapshot {
  const x = exact(value, ["version", "source", "registrationNonce", "adapterSession", "cursor", "outbox", "replies", "blocked"]);
  if (x.version !== 1 || JSON.stringify(parseSource(x.source)) !== JSON.stringify(parseSource(source))
    || typeof x.blocked !== "boolean" || !Array.isArray(x.outbox) || x.outbox.length > 100
    || !Array.isArray(x.replies) || x.replies.length > 100) throw new NekoBotBridgeError("invalid_saved_bridge");
  const outbox: NekoBotOutboxItem[] = x.outbox.map(entry => {
    const i = exact(entry, ["input", "requestId", "state", "errorCode"]);
    if (!["queued", "uncertain", "rejected", "accepted"].includes(String(i.state))
      || !(i.errorCode === null || typeof i.errorCode === "string" && /^[a-z0-9_]{1,80}$/.test(i.errorCode))) throw new NekoBotBridgeError("invalid_saved_outbox");
    const requestId = i.requestId === null ? null : opaque(i.requestId);
    if ((i.state === "accepted") !== (requestId !== null)) throw new NekoBotBridgeError("invalid_saved_receipt");
    return { input: parseMessage(i.input), requestId, state: i.state as NekoBotOutboxItem["state"], errorCode: i.errorCode as string | null };
  });
  if (new Set(outbox.map(i => i.input.sourceMessageId)).size !== outbox.length
    || new Set(outbox.map(i => i.input.clientNonce)).size !== outbox.length) throw new NekoBotBridgeError("duplicate_saved_message");
  const cursor = sequence(x.cursor), replies = x.replies.map(parseReply);
  if (new Set(replies.map(r => r.replyId)).size !== replies.length
    || new Set(replies.map(r => r.sourceMessageId)).size !== replies.length
    || replies.some(r => r.sequence > cursor || !outbox.some(i => i.input.sourceMessageId === r.sourceMessageId && i.requestId === r.requestId))) {
    throw new NekoBotBridgeError("invalid_saved_reply");
  }
  const adapterSession = x.adapterSession === null ? null : parseSession(x.adapterSession, source);
  if (!adapterSession && (cursor !== 0 || replies.length || outbox.some(i => i.requestId))) throw new NekoBotBridgeError("missing_saved_session");
  return { version: 1, source: parseSource(source), registrationNonce: nonce(x.registrationNonce), adapterSession, cursor,
    outbox, replies, blocked: x.blocked };
}

function responseBody(raw: string): unknown {
  if (new TextEncoder().encode(raw).length > NEKOBOT_LOCAL_MAX_PAGE_BYTES) throw new NekoBotBridgeError("response_oversized");
  let value: unknown;
  try { value = JSON.parse(raw); } catch { throw new NekoBotBridgeError("invalid_response_json"); }
  let visited = 0;
  const walk = (x: unknown, depth: number) => {
    if (depth > 20 || ++visited > 20000) throw new NekoBotBridgeError("response_too_complex");
    if (x && typeof x === "object") Object.values(x).forEach(v => walk(v, depth + 1));
  };
  walk(value, 0);
  return value;
}

function permanent(status: number): boolean { return [400, 403, 404, 409, 410, 422].includes(status); }
function errorCode(error: unknown): string { return error instanceof NekoBotBridgeError ? error.code : "transport_unavailable"; }

/** Transport is owner-authenticated by the caller; this module never bootstraps credentials or grants. */
export class NekoBotLocalBridge {
  snapshot: NekoBotBridgeSnapshot;
  ready = false;
  error: string | null = null;
  private tail: Promise<unknown> = Promise.resolve();
  private listeners = new Set<() => void>();
  private authorityHeld = true;
  private history: NekoBotHistoryCache | null;
  private recoveredHistoryHeld = false;
  historySupport: "unknown" | "supported" | "unsupported" = "unknown";
  constructor(readonly source: NekoBotLocalSource, private transport: NekoBotTrustedTransport,
    private storage: NekoBotBridgeStorage, private makeNonce: () => string) {
    this.source = Object.freeze(parseSource(source));
    this.snapshot = emptyNekoBotBridge(this.source, makeNonce());
    this.history = storage.history ? new NekoBotHistoryCache(this.source, storage.history, value => parseNekoBotBridgeSnapshot(value, this.source)) : null;
    if (transport.capabilities) this.historySupport = transport.capabilities.includes(HISTORY_CAPABILITY) ? "supported" : "unsupported";
  }
  subscribe = (changed: () => void): (() => void) => { this.listeners.add(changed); return () => { this.listeners.delete(changed); }; };
  private changed() { this.listeners.forEach(listener => listener()); }
  private serial<T>(action: () => Promise<T>): Promise<T> {
    const result = this.tail.then(action);
    this.tail = result.catch(() => {});
    return result;
  }
  private async commit(next: NekoBotBridgeSnapshot, rows?: HistoryRow[], repaired?: HistoryMetadata | null, migrate = false): Promise<void> {
    const checked = parseNekoBotBridgeSnapshot(next, this.source);
    if (this.history && (this.history.current || migrate)) {
      const candidate = await this.history.stage(checked, this.history.current?.receipts ?? [],
        rows ?? this.history.current?.rows ?? checked.replies.map(row => ({ ...row, availability: "available" })), repaired === undefined ? this.history.current?.repaired ?? null : repaired);
      await this.history.activate(candidate); this.snapshot = candidate.state.snapshot;
    } else { await this.storage.write(structuredClone(checked)); this.snapshot = checked; }
    this.error = null;
    this.changed();
  }
  async hydrate(): Promise<void> {
    return this.serial(async () => {
      if (this.ready) return;
      try {
        const cache = await this.history?.load();
        this.recoveredHistoryHeld = !!cache;
        const saved = cache?.snapshot ?? await this.storage.read();
        const next = saved === undefined || saved === null ? this.snapshot : parseNekoBotBridgeSnapshot(saved, this.source);
        await this.commit(next);
        this.ready = true;
        if (this.recoveredHistoryHeld) this.error = "history_repair_required";
        this.changed();
      } catch (error) { this.error = errorCode(error); this.changed(); throw error; }
    });
  }
  private requireReady() { if (!this.ready) throw new NekoBotBridgeError("storage_not_ready"); }
  private async request(method: "GET" | "POST", path: string, body?: unknown): Promise<unknown> {
    if (this.transport.isCurrent?.() === false) throw new NekoBotBridgeError("connection_changed");
    const response = await this.transport.request(method, path, body === undefined ? undefined : structuredClone(body));
    if (this.transport.isCurrent?.() === false) throw new NekoBotBridgeError("connection_changed");
    if (response.status < 200 || response.status >= 300) {
      let code = `http_${response.status}`;
      if (path.includes("/history")) {
        try { const value = exact(responseBody(response.body), ["error"]); if (HISTORY_ERRORS.has(String(value.error))) code = String(value.error); } catch {}
      }
      throw new NekoBotBridgeError(code, response.status);
    }
    if (path.includes("/history") && new TextEncoder().encode(response.body).length > HISTORY_PAGE_BYTES) throw new NekoBotBridgeError("response_oversized");
    return responseBody(response.body);
  }
  private async openBound(): Promise<NekoBotAdapterSession> {
    this.requireReady();
    if (this.snapshot.adapterSession) return this.snapshot.adapterSession;
    const { ownerUserId: _owner, spaceId: _space, ...body } = this.source;
    const result = exact(await this.request("POST", "/v1/adapters/sessions", { ...body, clientNonce: this.snapshot.registrationNonce }), ["session", "duplicate"]);
    if (typeof result.duplicate !== "boolean") throw new NekoBotBridgeError("invalid_registration_receipt");
    const adapterSession = parseSession(result.session, this.source);
    await this.commit({ ...this.snapshot, adapterSession });
    return adapterSession;
  }
  open(): Promise<NekoBotAdapterSession> { return this.serial(() => this.openBound()); }

  enqueue(value: Omit<NekoBotMessageInput, "clientNonce">): Promise<NekoBotOutboxItem> {
    return this.serial(async () => {
      this.requireReady();
      const prior = [...this.snapshot.outbox, ...this.history?.current?.receipts ?? []].find(i => i.input.sourceMessageId === value.sourceMessageId);
      const input = parseMessage({ ...value, clientNonce: prior?.input.clientNonce ?? this.makeNonce() });
      if (prior) {
        if (JSON.stringify(prior.input) !== JSON.stringify(input)) throw new NekoBotBridgeError("source_message_conflict");
        return structuredClone(prior);
      }
      if (this.snapshot.outbox.length >= 100 && this.snapshot.outbox.some(i => i.state === "accepted") && this.history) await this.commit(this.snapshot, undefined, undefined, true);
      if (this.snapshot.outbox.length >= 100) throw new NekoBotBridgeError("outbox_full");
      const item: NekoBotOutboxItem = { input, requestId: null, state: "queued", errorCode: null };
      await this.commit({ ...this.snapshot, outbox: [...this.snapshot.outbox, item] });
      return structuredClone(item);
    });
  }
  private receipt(value: unknown, item: NekoBotOutboxItem, session: NekoBotAdapterSession): string {
    const result = exact(value, ["work", "duplicate"]);
    if (typeof result.duplicate !== "boolean") throw new NekoBotBridgeError("invalid_message_receipt");
    const work = record(result.work), publication = record(work.publication), pin = record(work.adapterPin);
    opaque(work.clientNonce);
    if (pin.clientNonce !== item.input.clientNonce || work.ownerUserId !== session.ownerUserId || work.spaceId !== session.spaceId
      || work.conversationId !== session.channelId || work.text !== item.input.text || work.lane !== item.input.lane
      || publication.sourceChannelId !== session.channelId || publication.replyChannelId !== session.channelId
      || publication.audienceRevision !== session.channelRevision || pin.adapterSessionId !== session.id
      || pin.sourceApp !== session.sourceApp || pin.sourceSessionId !== session.sourceSessionId
      || pin.sourceThreadId !== session.sourceThreadId || pin.sourceMessageId !== item.input.sourceMessageId
      || pin.channelRevision !== session.channelRevision || pin.returnRoute !== session.returnRoute) {
      throw new NekoBotBridgeError("message_receipt_mismatch");
    }
    return opaque(work.requestId);
  }
  dispatch(sourceMessageId: string): Promise<void> {
    return this.serial(async () => {
      this.requireReady();
      const item = this.snapshot.outbox.find(i => i.input.sourceMessageId === sourceMessageId);
      if (!item || item.state === "accepted") return;
      if (item.state === "rejected") throw new NekoBotBridgeError("message_rejected");
      const session = await this.openBound();
      await this.commit({ ...this.snapshot, outbox: this.snapshot.outbox.map(i => i.input.sourceMessageId === sourceMessageId ? { ...i, state: "uncertain", errorCode: null } : i) });
      try {
        const requestId = this.receipt(await this.request("POST", `/v1/adapters/sessions/${session.id}/messages`, item.input), item, session);
        await this.commit({ ...this.snapshot, outbox: this.snapshot.outbox.map(i => i.input.sourceMessageId === sourceMessageId
          ? { ...i, requestId, state: "accepted", errorCode: null } : i) });
      } catch (error) {
        const rejected = error instanceof NekoBotBridgeError && permanent(error.status);
        this.authorityHeld = true; this.changed();
        const next: NekoBotBridgeSnapshot = { ...this.snapshot, blocked: this.snapshot.blocked || error instanceof NekoBotBridgeError && [401, 403, 409].includes(error.status),
          outbox: this.snapshot.outbox.map(i => i.input.sourceMessageId === sourceMessageId
            ? { ...i, state: rejected ? "rejected" : "uncertain", errorCode: errorCode(error) } : i) };
        try { await this.commit(next); } catch { this.error = "storage_write_failed"; this.changed(); throw new NekoBotBridgeError("storage_write_failed"); }
        this.error = errorCode(error); this.changed(); throw error;
      }
    });
  }
  /** Each permanently rejected item is held independently; it cannot block later valid user intent. */
  async dispatchQueued(): Promise<void> {
    const ids = this.snapshot.outbox.filter(i => i.state === "queued").map(i => i.input.sourceMessageId);
    for (const id of ids) {
      try { await this.dispatch(id); } catch (error) {
        if (!(error instanceof NekoBotBridgeError) || !permanent(error.status)) throw error;
      }
    }
  }
  refresh(): Promise<void> {
    return this.serial(async () => {
      this.requireReady();
      const session = await this.openBound();
      this.authorityHeld = true; this.changed();
      try {
        const page = parsePage(await this.request("GET", `/v1/adapters/sessions/${session.id}/replies?after=${this.snapshot.cursor}`), session, this.snapshot.cursor);
        const next = structuredClone(this.snapshot);
        let rows = this.history?.current ? structuredClone(this.history.current.rows) : null;
        let migrate = false;
        for (const reply of page.replies) {
          const item = [...next.outbox, ...this.history?.current?.receipts ?? []].find(i => i.input.sourceMessageId === reply.sourceMessageId);
          if (!item && !rows || item && (item.state === "rejected" || item.input.botId !== reply.botId || item.requestId !== null && item.requestId !== reply.requestId)) throw new NekoBotBridgeError("reply_message_mismatch");
          if (item && item.requestId === null) {
            if (!this.history) throw new NekoBotBridgeError("reply_unconfirmed_intake");
            if (!rows) { rows = next.replies.map(row => ({ ...row, availability: "available" })); next.replies = []; migrate = true; }
          }
          const target = rows ?? next.replies;
          const index = target.findIndex(r => r.replyId === reply.replyId);
          if (index >= 0) {
            const prior = target[index];
            if (prior.sourceMessageId !== reply.sourceMessageId || prior.botId !== reply.botId) throw new NekoBotBridgeError("reply_identity_changed");
            if (reply.sequence < prior.sequence) continue;
            const { availability: _availability, ...oldReply } = prior as HistoryRow;
            if (reply.sequence === prior.sequence && JSON.stringify(oldReply) !== JSON.stringify(reply)) throw new NekoBotBridgeError("reply_sequence_conflict");
            const availability = (prior as HistoryRow).availability === "source_unavailable" && !reply.text && !reply.artifacts.length ? "source_unavailable" : "available";
            const updated: HistoryRow = { ...reply, availability };
            target[index] = rows ? updated : reply;
          } else { const updated: HistoryRow = { ...reply, availability: "available" }; target.push(rows ? updated : reply); }
        }
        next.cursor = page.cursor; next.blocked = false;
        await this.commit(next, rows ?? undefined, undefined, migrate);
        if (this.transport.isCurrent?.() === false) throw new NekoBotBridgeError("connection_changed");
        this.authorityHeld = this.recoveredHistoryHeld;
        if (this.recoveredHistoryHeld) this.error = "history_repair_required";
        this.changed();
      } catch (error) {
        this.authorityHeld = true; this.changed();
        if (error instanceof NekoBotBridgeError && [401, 403, 409].includes(error.status)) {
          this.authorityHeld = true; this.changed();
          await this.commit({ ...this.snapshot, blocked: true });
        }
        this.error = errorCode(error); this.changed(); throw error;
      }
    });
  }
  repairHistory(stillCurrent: () => boolean = () => true): Promise<void> {
    return this.serial(async () => {
      this.requireReady(); this.authorityHeld = true; this.changed();
      try {
        const requireCurrent = () => {
          if (!stillCurrent()) throw new NekoBotBridgeError("history_navigation_changed");
          if (this.transport.isCurrent?.() === false) throw new NekoBotBridgeError("connection_changed");
        };
        const session = this.snapshot.adapterSession;
        if (!session) throw new NekoBotBridgeError("history_requires_existing_session");
        if (!this.history) throw new NekoBotBridgeError("history_storage_unavailable");
        if (this.historySupport === "unknown") {
          try {
            const capabilities = record(await this.request("GET", "/v1/adapters/capabilities"));
            if (capabilities.version !== 1 || !Array.isArray(capabilities.capabilities) || capabilities.capabilities.some(x => typeof x !== "string")) throw new NekoBotBridgeError("invalid_capabilities");
            this.historySupport = capabilities.capabilities.includes(HISTORY_CAPABILITY) ? "supported" : "unsupported";
          } catch (error) { if (error instanceof NekoBotBridgeError && error.status === 404) this.historySupport = "unsupported"; else throw error; }
        }
        if (this.historySupport !== "supported") throw new NekoBotBridgeError("history_repair_unsupported");
        const path = `/v1/adapters/sessions/${session.id}/history`, rows: HistoryRow[] = [];
        let first: HistoryPage | null = null, page: HistoryPage, next = path;
        do {
          requireCurrent();
          page = parseHistoryPage(await this.request("GET", next), session);
          if (page.offset !== rows.length || first && !sameHistorySnapshot(first, page)) throw new NekoBotBridgeError("history_snapshot_changed");
          first ??= page; rows.push(...page.rows); uniqueHistoryRows(rows);
          next = path + "?cursor=" + page.nextCursor;
        } while (!page.complete);
        const witness = await historyWitness(sessionScope(session), first.snapshotCursor, first.authorityDigest, rows);
        if (witness.totalBytes !== first.totalBytes || witness.historyDigest !== first.historyDigest || witness.snapshotId !== first.snapshotId) throw new NekoBotBridgeError("history_digest_mismatch");
        const snapshot = structuredClone(this.snapshot);
        for (const row of rows) {
          const item = [...snapshot.outbox, ...this.history.current?.receipts ?? []].find(item => item.input.sourceMessageId === row.sourceMessageId);
          if (item && (item.input.botId !== row.botId || item.state === "rejected" || item.requestId !== null && item.requestId !== row.requestId)) throw new NekoBotBridgeError("reply_message_mismatch");
        }
        snapshot.cursor = first.snapshotCursor; snapshot.blocked = false;
        const candidate = await this.history.stage(snapshot, this.history.current?.receipts ?? [], rows,
          { snapshotId: first.snapshotId, snapshotCursor: first.snapshotCursor, authorityDigest: first.authorityDigest, historyDigest: first.historyDigest });
        const confirmation = parseHistoryPage(await this.request("GET", path + "?cursor=" + page.confirmationCursor), session);
        if (!sameHistorySnapshot(first, confirmation) || confirmation.offset !== rows.length || confirmation.rows.length || !confirmation.complete) throw new NekoBotBridgeError("history_confirmation_mismatch");
        requireCurrent();
        await this.history.activate(candidate); this.snapshot = candidate.state.snapshot;
        requireCurrent();
        this.recoveredHistoryHeld = false; this.authorityHeld = false; this.error = null; this.changed();
      } catch (error) { this.authorityHeld = true; this.error = errorCode(error); this.changed(); throw error; }
    });
  }
  discard(sourceMessageId: string): Promise<void> {
    return this.serial(async () => {
      this.requireReady();
      const item = this.snapshot.outbox.find(i => i.input.sourceMessageId === sourceMessageId);
      if (!item || item.state === "accepted") throw new NekoBotBridgeError("accepted_message_is_history");
      if (item.state === "uncertain") throw new NekoBotBridgeError("reconcile_uncertain_message_first");
      await this.commit({ ...this.snapshot, outbox: this.snapshot.outbox.filter(i => i !== item) });
    });
  }
  get acceptedReceipts(): NekoBotOutboxItem[] { return structuredClone([...(this.history?.current?.receipts ?? []), ...this.snapshot.outbox.filter(item => item.state === "accepted")]); }
  get replyCount(): number { return this.authorityHeld || this.snapshot.blocked ? 0 : (this.history?.current?.rows ?? this.snapshot.replies).length; }
  replyWindow(offset: number, size = 100): (NekoBotReply | HistoryRow)[] {
    if (!Number.isSafeInteger(offset) || offset < 0 || !Number.isSafeInteger(size) || size < 1 || size > 100) throw new NekoBotBridgeError("invalid_history_window");
    return this.authorityHeld || this.snapshot.blocked ? [] : structuredClone((this.history?.current?.rows ?? this.snapshot.replies).slice(offset, offset + size));
  }
  get visibleReplies(): NekoBotReply[] { return this.authorityHeld || this.snapshot.blocked ? [] : structuredClone(this.history?.current?.rows ?? this.snapshot.replies); }
}
