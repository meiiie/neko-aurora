import { describe, expect, test } from "bun:test";
import { NekoBotLocalBridge, type NekoBotBridgeSnapshot, type NekoBotTrustedTransport } from "../../neko-chill/nekobot-local-bridge";
import { parseMessage, parsePage, parseSource, type NekoBotLocalSource } from "../../neko-chill/nekobot-local-protocol";
import { createWiiiNekoBotBridge, nekoBotBridgeStorageKey, projectNekoBotReply } from "../../neko-chill/nekobot-local-wiii";
import { decodeNekoBotHandoff, nekoBotHandoffDraft } from "../../neko-chill/nekobot-handoff";

const source: NekoBotLocalSource = { sourceApp: "wiii", sourceSessionId: "wiii-session-a", sourceThreadId: "wiii-thread-a",
  channelId: "channel-a", expectedChannelRevision: 2, ownerUserId: "local-owner", spaceId: "local-space" };
const session = { id: "adapter-a", version: 1 as const, sourceApp: "wiii" as const, sourceSessionId: source.sourceSessionId,
  sourceThreadId: source.sourceThreadId, channelId: source.channelId, channelRevision: 2,
  ownerUserId: source.ownerUserId, spaceId: source.spaceId, returnRoute: "/v1/adapters/sessions/adapter-a/replies" };
const message = { sourceMessageId: "wiii-message-a", botId: "bot-meiiie", text: "Kiểm tra trong kênh chung.", lane: "foreground" as const };
const reply = (seq = 4, text = "Đã kiểm tra.") => ({ replyId: "reply-request-a", requestId: "request-a", sourceMessageId: message.sourceMessageId,
  botId: message.botId, role: "assistant", text, status: "completed", sequence: seq, artifacts: [] });
const page = (seq = 4, text = "Đã kiểm tra.") => ({ adapterSessionId: session.id, sourceApp: source.sourceApp,
  sourceSessionId: source.sourceSessionId, sourceThreadId: source.sourceThreadId, channelId: source.channelId,
  ownerUserId: source.ownerUserId, spaceId: source.spaceId, channelRevision: session.channelRevision,
  returnRoute: session.returnRoute, cursor: seq, replies: [reply(seq, text)] });
function ack(input: any, requestId = "request-a") {
  return { duplicate: false, work: { requestId, clientNonce: input.clientNonce, ownerUserId: source.ownerUserId, spaceId: source.spaceId,
    conversationId: source.channelId, text: input.text, lane: input.lane,
    publication: { sourceChannelId: source.channelId, replyChannelId: source.channelId, audienceRevision: 2 },
    adapterPin: { adapterSessionId: session.id, sourceApp: source.sourceApp, sourceSessionId: source.sourceSessionId,
      sourceThreadId: source.sourceThreadId, sourceMessageId: input.sourceMessageId, clientNonce: input.clientNonce,
      channelRevision: session.channelRevision, returnRoute: session.returnRoute } } };
}
function fixture() {
  let stored: NekoBotBridgeSnapshot | null = null, writes = 0, counter = 0, failWrite = false;
  const calls: { method: string; path: string; body?: any }[] = [];
  let handler = async (method: string, path: string, body: any) => {
    if (path === "/v1/adapters/sessions") return { status: 201, body: JSON.stringify({ session: session, duplicate: false }) };
    if (method === "POST") return { status: 201, body: JSON.stringify(ack(body)) };
    return { status: 200, body: JSON.stringify(page()) };
  };
  const transport: NekoBotTrustedTransport = { request: async (method, path, body) => {
    calls.push({ method, path, body }); return handler(method, path, body);
  } };
  const storage = { read: async () => structuredClone(stored), write: async (value: NekoBotBridgeSnapshot) => {
    if (failWrite) throw new Error("fixture storage unavailable"); stored = structuredClone(value); writes++;
  } };
  const make = () => new NekoBotLocalBridge(source, transport, storage, () => `fixture-nonce-${++counter}`);
  return { make, calls, storage, transport, setHandler: (h: typeof handler) => { handler = h; },
    failWrite: (x: boolean) => { failWrite = x; }, saved: () => structuredClone(stored), writes: () => writes };
}

describe("Wiii source-bound local bridge", () => {
  test("construction and hydration never register, send, import history or launch a process", async () => {
    const f = fixture(), b = f.make(); await b.hydrate();
    expect(f.calls).toEqual([]); expect(b.snapshot.cursor).toBe(0); expect(b.ready).toBe(true);
  });
  test("a failed initial durable write denies every transport attempt", async () => {
    const f = fixture(), b = f.make(); f.failWrite(true);
    await expect(b.hydrate()).rejects.toThrow(); await expect(b.open()).rejects.toThrow("storage_not_ready");
    await expect(b.enqueue(message)).rejects.toThrow("storage_not_ready"); expect(f.calls).toEqual([]);
  });
  test("durability precedes registration and message transport", async () => {
    const f = fixture(), b = f.make(); await b.hydrate();
    await b.enqueue(message); f.failWrite(true);
    await expect(b.dispatch(message.sourceMessageId)).rejects.toThrow();
    expect(f.calls).toHaveLength(1); expect(f.calls[0].path).toBe("/v1/adapters/sessions");
    expect(b.snapshot.adapterSession).toBeNull();
    f.failWrite(false); await b.dispatch(message.sourceMessageId);
    expect(f.calls.filter(c => c.path.endsWith("/messages"))).toHaveLength(1);
    expect(b.snapshot.outbox[0].state).toBe("accepted");
  });
  test("registration excludes client-proclaimed owner and preserves nonce through lost ACK/restart", async () => {
    const f = fixture(), b = f.make(); await b.hydrate(); let first = true;
    f.setHandler(async (_m, _p, _body) => { if (first) { first = false; throw new Error("fixture lost ACK"); }
      return { status: 200, body: JSON.stringify({ session: session, duplicate: true }) }; });
    await expect(b.open()).rejects.toThrow();
    const reopened = f.make(); await reopened.hydrate(); await reopened.open();
    expect(f.calls[0].body).toEqual(f.calls[1].body);
    expect(f.calls[0].body).not.toHaveProperty("ownerUserId"); expect(f.calls[0].body).not.toHaveProperty("spaceId");
  });
  test("registration accepts the actual session wire key and rejects its obsolete alias", async () => {
    const f = fixture(), b = f.make(); await b.hydrate();
    f.setHandler(async () => ({ status: 200, body: JSON.stringify({ adapterSession: session, duplicate: false }) }));
    await expect(b.open()).rejects.toThrow("unsupported_fields"); expect(b.snapshot.adapterSession).toBeNull();
    f.setHandler(async () => ({ status: 200, body: JSON.stringify({ session, duplicate: false }) }));
    await b.open(); expect(b.snapshot.adapterSession).toEqual(session);
  });
  test("same source message is immutable and a concurrent duplicate keeps one nonce/request", async () => {
    const f = fixture(), b = f.make(); await b.hydrate();
    await Promise.all([b.enqueue(message), b.enqueue(message)]); expect(b.snapshot.outbox).toHaveLength(1);
    await expect(b.enqueue({ ...message, text: "Changed" })).rejects.toThrow("source_message_conflict");
    await Promise.all([b.dispatch(message.sourceMessageId), b.dispatch(message.sourceMessageId)]);
    expect(f.calls.filter(c => c.path.endsWith("/messages"))).toHaveLength(1);
  });
  test("message lost ACK preserves intent until explicit retry proves typed intake receipt", async () => {
    const f = fixture(), b = f.make(); await b.hydrate(); await b.open(); await b.enqueue(message);
    f.setHandler(async (m) => { if (m === "POST") throw new Error("fixture response lost after commit"); return { status: 200, body: JSON.stringify(page()) }; });
    await expect(b.dispatch(message.sourceMessageId)).rejects.toThrow();
    expect(b.snapshot.outbox[0].state).toBe("uncertain");
    const restarted = f.make(); await restarted.hydrate(); expect(f.calls.filter(c => c.path.endsWith("/messages"))).toHaveLength(1);
    const pending = structuredClone(restarted.snapshot.outbox[0]);
    await expect(restarted.refresh()).rejects.toThrow("reply_unconfirmed_intake");
    expect(restarted.snapshot.outbox[0]).toEqual(pending); expect(restarted.snapshot.cursor).toBe(0);
    expect(f.calls.filter(c => c.path.endsWith("/messages"))).toHaveLength(1);
    f.setHandler(async (method, _path, body) => ({ status: 200, body: JSON.stringify(method === "POST" ? { ...ack(body), duplicate: true } : page()) }));
    await restarted.dispatch(message.sourceMessageId);
    const sends = f.calls.filter(c => c.path.endsWith("/messages")); expect(sends).toHaveLength(2); expect(sends[1]).toEqual(sends[0]);
    expect(restarted.snapshot.outbox[0].state).toBe("accepted"); expect(restarted.snapshot.outbox[0].requestId).toBe("request-a");
    await restarted.refresh(); expect(restarted.snapshot.cursor).toBe(4); expect(restarted.visibleReplies).toHaveLength(1);
  });
  test("explicit retry retains original resources, text, route and nonce across transport failures", async () => {
    const f = fixture(), b = f.make(); await b.hydrate(); await b.open();
    await b.enqueue({ ...message, accountId: "account-a", accountRevision: 5, projectId: "project-a", projectRevision: 3, fileIds: ["file-a"] });
    let first = true;
    f.setHandler(async (_m, _p, body) => { if (first) { first = false; return { status: 503, body: "fixture" }; }
      return { status: 200, body: JSON.stringify({ ...ack(body), duplicate: true }) }; });
    await expect(b.dispatch(message.sourceMessageId)).rejects.toThrow("http_503");
    const resumed = f.make(); await resumed.hydrate(); await resumed.dispatch(message.sourceMessageId);
    const sends = f.calls.filter(c => c.path.endsWith("/messages")); expect(sends[0]).toEqual(sends[1]);
  });
  test("permanent rejected message cannot block later valid intent", async () => {
    const f = fixture(), b = f.make(); await b.hydrate(); await b.open();
    await b.enqueue(message); await b.enqueue({ ...message, sourceMessageId: "wiii-message-b" });
    f.setHandler(async (_m, _p, body) => body.sourceMessageId === message.sourceMessageId
      ? { status: 404, body: "not_found" } : { status: 201, body: JSON.stringify(ack(body, "request-b")) });
    await b.dispatchQueued(); expect(b.snapshot.outbox.map(i => i.state)).toEqual(["rejected", "accepted"]);
    await expect(b.dispatch(message.sourceMessageId)).rejects.toThrow("message_rejected");
    await b.discard(message.sourceMessageId); expect(b.snapshot.outbox).toHaveLength(1);
  });
  test("uncertain dispatch may not be discarded as though it never reached the server", async () => {
    const f = fixture(), b = f.make(); await b.hydrate(); await b.open(); await b.enqueue(message);
    f.setHandler(async () => ({ status: 503, body: "fixture" })); await expect(b.dispatch(message.sourceMessageId)).rejects.toThrow();
    await expect(b.discard(message.sourceMessageId)).rejects.toThrow("reconcile_uncertain_message_first");
  });
  test("cumulative replies replace same Wiii answer block only at increasing server sequence", async () => {
    const f = fixture(), b = f.make(); await b.hydrate(); await b.enqueue(message); await b.dispatch(message.sourceMessageId); await b.refresh();
    f.setHandler(async () => ({ status: 200, body: JSON.stringify(page(6, "Đã kiểm tra thêm.")) })); await b.refresh();
    expect(b.visibleReplies).toHaveLength(1); expect(projectNekoBotReply(b.visibleReplies[0])).toEqual({ id: "reply-request-a", role: "assistant",
      blocks: [{ type: "answer", id: "reply-request-a:answer", content: "Đã kiểm tra thêm." }] });
    expect(f.calls.at(-1)?.path).toEndWith("?after=4");
  });
  test("cursor and replies commit atomically; a failed page write replays the same cursor", async () => {
    const f = fixture(), b = f.make(); await b.hydrate(); await b.enqueue(message); await b.dispatch(message.sourceMessageId);
    const before = f.saved(); f.failWrite(true); await expect(b.refresh()).rejects.toThrow();
    expect(b.snapshot.cursor).toBe(0); expect(f.saved()).toEqual(before);
    f.failWrite(false); const reopened = f.make(); await reopened.hydrate(); await reopened.refresh();
    expect(f.calls.filter(c => c.method === "GET").map(c => c.path)).toEqual(["/v1/adapters/sessions/adapter-a/replies?after=0", "/v1/adapters/sessions/adapter-a/replies?after=0"]);
  });
  test("empty page may advance global cursor without inventing replies", async () => {
    const f = fixture(), b = f.make(); await b.hydrate(); await b.open();
    f.setHandler(async () => ({ status: 200, body: JSON.stringify({ ...page(19), replies: [] }) }));
    await b.refresh(); expect(b.snapshot.cursor).toBe(19); expect(b.visibleReplies).toEqual([]);
  });
  test("revoked/stale audience hides cached text while preserving durable bytes and never rebinds", async () => {
    const f = fixture(), b = f.make(); await b.hydrate(); await b.enqueue(message); await b.dispatch(message.sourceMessageId); await b.refresh();
    const original = b.snapshot.replies;
    f.setHandler(async () => ({ status: 409, body: "stale_channel" })); await expect(b.refresh()).rejects.toThrow("http_409");
    expect(b.visibleReplies).toEqual([]); expect(b.snapshot.replies).toEqual(original); expect(b.snapshot.adapterSession).toEqual(session);
  });
  test("revocation hides cached text even if its durable hold write fails; restart needs fresh read authority", async () => {
    const f = fixture(), b = f.make(); await b.hydrate(); await b.enqueue(message); await b.dispatch(message.sourceMessageId); await b.refresh();
    expect(b.visibleReplies).toHaveLength(1); const preserved = f.saved(); f.failWrite(true);
    f.setHandler(async () => ({ status: 403, body: "revoked" })); await expect(b.refresh()).rejects.toThrow();
    expect(b.visibleReplies).toEqual([]); expect(f.saved()).toEqual(preserved);
    f.failWrite(false); const reopened = f.make(); await reopened.hydrate(); expect(reopened.visibleReplies).toEqual([]);
    f.setHandler(async () => ({ status: 200, body: JSON.stringify({ ...page(5), replies: [] }) })); await reopened.refresh();
    expect(reopened.visibleReplies).toHaveLength(1);
  });
  test("an accepted duplicate ACK is not a read authorization for preserved cached replies", async () => {
    const f = fixture(), b = f.make(); await b.hydrate(); await b.enqueue(message); await b.dispatch(message.sourceMessageId); await b.refresh();
    const reopened = f.make(); await reopened.hydrate(); expect(reopened.visibleReplies).toEqual([]);
    await reopened.enqueue({ ...message, sourceMessageId: "wiii-message-b" });
    f.setHandler(async (_m, _p, body) => ({ status: 200, body: JSON.stringify({ ...ack(body, "request-b"), duplicate: true }) }));
    await reopened.dispatch("wiii-message-b"); expect(reopened.visibleReplies).toEqual([]);
  });
  test.each(["sourceSessionId", "sourceThreadId", "channelId", "ownerUserId", "spaceId", "channelRevision", "returnRoute"])("registration rejects mismatched %s before binding", async field => {
    const f = fixture(), b = f.make(); await b.hydrate();
    f.setHandler(async () => ({ status: 200, body: JSON.stringify({ session: { ...session, [field]: field === "channelRevision" ? 3 : "other" }, duplicate: false }) }));
    await expect(b.open()).rejects.toThrow("session_binding_mismatch"); expect(b.snapshot.adapterSession).toBeNull();
  });
  test.each(["adapterSessionId", "sourceSessionId", "sourceThreadId", "channelId", "sourceApp", "ownerUserId", "spaceId", "channelRevision", "returnRoute"])("reply page rejects wrong %s without changing cursor/history", async field => {
    const f = fixture(), b = f.make(); await b.hydrate(); await b.enqueue(message); await b.dispatch(message.sourceMessageId);
    const saved = f.saved(); f.setHandler(async () => ({ status: 200, body: JSON.stringify({ ...page(), [field]: "other" }) }));
    await expect(b.refresh()).rejects.toThrow("reply_binding_mismatch"); expect(f.saved()).toEqual(saved);
  });
  test("wrong reply message/request cannot contaminate original session", async () => {
    const f = fixture(), b = f.make(); await b.hydrate(); await b.enqueue(message); await b.dispatch(message.sourceMessageId);
    for (const replacement of [{ sourceMessageId: "other-message" }, { requestId: "other-request", replyId: "reply-other-request" }]) {
      f.setHandler(async () => ({ status: 200, body: JSON.stringify({ ...page(), replies: [{ ...reply(), ...replacement }] }) }));
      await expect(b.refresh()).rejects.toThrow("reply_message_mismatch"); expect(b.snapshot.cursor).toBe(0);
    }
  });
  test("wrong ACK source pin/owner/channel/nonce never becomes an accepted receipt", async () => {
    for (const field of ["pinNonce", "ownerUserId", "conversationId", "adapterPin"]) {
      const f = fixture(), b = f.make(); await b.hydrate(); await b.open(); await b.enqueue(message);
      f.setHandler(async (_m, _p, body) => { const result = ack(body);
        if (field === "pinNonce") result.work.adapterPin.clientNonce = "fixture-other-nonce";
        else (result.work as any)[field] = field === "adapterPin" ? { ...result.work.adapterPin, sourceSessionId: "other" } : "other";
        return { status: 201, body: JSON.stringify(result) }; });
      await expect(b.dispatch(message.sourceMessageId)).rejects.toThrow("message_receipt_mismatch"); expect(b.snapshot.outbox[0].requestId).toBeNull();
    }
  });
  test("rehydration rejects a saved foreign owner/session instead of overwriting it", async () => {
    const f = fixture(), b = f.make(); await b.hydrate(); const original = f.saved();
    const other = new NekoBotLocalBridge({ ...source, sourceSessionId: "other" }, f.transport, f.storage, () => "fixture-new-nonce");
    await expect(other.hydrate()).rejects.toThrow("invalid_saved_bridge"); expect(f.saved()).toEqual(original); expect(other.ready).toBe(false);
  });
  test.each(["invalid_json", "oversized", "deep", "version", "unknown"])("rejects malformed response %s before binding", async kind => {
    const f = fixture(), b = f.make(); await b.hydrate();
    let body = JSON.stringify({ session: session, duplicate: false });
    if (kind === "invalid_json") body = "{";
    if (kind === "oversized") body = " ".repeat(1024 * 1024 + 1);
    if (kind === "deep") body = "[".repeat(25) + "0" + "]".repeat(25);
    if (kind === "version") body = JSON.stringify({ session: { ...session, version: 2 }, duplicate: false });
    if (kind === "unknown") body = JSON.stringify({ session: { ...session, core: { sessionId: "fake" } }, duplicate: false });
    f.setHandler(async () => ({ status: 201, body })); await expect(b.open()).rejects.toThrow();
    expect(b.snapshot.adapterSession).toBeNull();
  });
  test("real Wiii strict storage/projector/handoff modules remain separate from runtime authority", async () => {
    const data = new Map<string, string>(); const originalWindow = globalThis.window, originalLocalStorage = globalThis.localStorage;
    Object.defineProperty(globalThis, "window", { value: {}, configurable: true });
    Object.defineProperty(globalThis, "localStorage", { value: { getItem: (k: string) => data.get(k) ?? null, setItem: (k: string, v: string) => data.set(k, v) }, configurable: true });
    try {
      const f = fixture(), b = createWiiiNekoBotBridge(source, f.transport, () => "fixture-real-storage"); await b.hydrate();
      await b.enqueue(message); expect([...data.keys()]).toEqual(["wiii:nekobot-local-bridge-v1.json"]);
      const reopened = createWiiiNekoBotBridge(source, f.transport, () => "fixture-new-storage"); await reopened.hydrate();
      expect(reopened.snapshot.outbox[0].input).toEqual(b.snapshot.outbox[0].input); expect(f.calls).toEqual([]);
      const handoff = decodeNekoBotHandoff(JSON.stringify({ schemaVersion: "nekobot.task-handoff.v1", exportedAt: "2026-10-11T12:00:00Z",
        title: "Test", prompt: "Review only", source: { product: "NekoBot", threadId: "untrusted-thread", botId: "untrusted-bot" }, transcript: [] }));
      expect(nekoBotHandoffDraft(handoff)).toContain("không cấp quyền"); expect(b.snapshot.source.sourceSessionId).toBe(source.sourceSessionId);
      expect(nekoBotBridgeStorageKey({ ...source, ownerUserId: "another-owner" })).not.toBe(nekoBotBridgeStorageKey(source));
    } finally { Object.defineProperty(globalThis, "window", { value: originalWindow, configurable: true });
      Object.defineProperty(globalThis, "localStorage", { value: originalLocalStorage, configurable: true }); }
  });
});

describe("strict protocol inputs", () => {
  test.each(["ownerUserId", "spaceId", "actorId", "replyTo", "returnRoute", "core", "sessionId", "workspace", "profile", "permissions", "apiKey", "sourceApp"])("rejects injected authority field %s", field => {
    expect(() => parseMessage({ ...message, clientNonce: "fixture-nonce", [field]: "fixture-value" })).toThrow("unsupported_fields");
  });
  test("bounds UTF8 text and rejects incomplete pins, invalid source IDs and version/sequence fields", () => {
    expect(() => parseMessage({ ...message, clientNonce: "fixture-nonce", text: "💛".repeat(8193) })).toThrow();
    expect(() => parseMessage({ ...message, clientNonce: "fixture-nonce", text: "\ud800" })).toThrow();
    expect(() => parseMessage({ ...message, clientNonce: "fixture-nonce", accountId: "account-a" })).toThrow();
    expect(() => parseSource({ ...source, sourceSessionId: "../workspace" })).toThrow();
    expect(() => parsePage({ ...page(), cursor: Number.MAX_SAFE_INTEGER + 1 }, session, 0)).toThrow();
    expect(() => parsePage(page(), session, 5)).toThrow();
    expect(() => parsePage({ ...page(), replies: [{ ...reply(), sequence: 9 }] }, session, 0)).toThrow();
  });
});
