export const NEKOBOT_LOCAL_MAX_TEXT_BYTES = 32768;
export const NEKOBOT_LOCAL_MAX_PAGE_BYTES = 1024 * 1024;

export interface NekoBotLocalSource {
  sourceApp: "wiii";
  sourceSessionId: string;
  sourceThreadId: string;
  channelId: string;
  expectedChannelRevision: number;
  ownerUserId: string;
  spaceId: string;
}

export interface NekoBotAdapterSession {
  id: string;
  version: 1;
  sourceApp: "wiii";
  sourceSessionId: string;
  sourceThreadId: string;
  channelId: string;
  channelRevision: number;
  ownerUserId: string;
  spaceId: string;
  returnRoute: string;
}

export interface NekoBotMessageInput {
  sourceMessageId: string;
  botId: string;
  text: string;
  lane: "foreground" | "background";
  clientNonce: string;
  accountId?: string;
  accountRevision?: number;
  projectId?: string;
  projectRevision?: number;
  fileIds?: string[];
}

export interface NekoBotReply {
  replyId: string;
  requestId: string;
  sourceMessageId: string;
  botId: string;
  role: "assistant";
  text: string;
  status: string;
  sequence: number;
  artifacts: { id: string; filename: string; sha256: string; bytes: number }[];
}

export interface NekoBotReplyPage {
  adapterSessionId: string;
  sourceApp: "wiii";
  sourceSessionId: string;
  sourceThreadId: string;
  channelId: string;
  ownerUserId: string;
  spaceId: string;
  channelRevision: number;
  returnRoute: string;
  cursor: number;
  replies: NekoBotReply[];
}

export class NekoBotBridgeError extends Error {
  constructor(readonly code: string, readonly status = 0) {
    super(`Neko Bot local: ${code}`);
    this.name = "NekoBotBridgeError";
  }
}

export function record(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) throw new NekoBotBridgeError("invalid_object");
  return value as Record<string, unknown>;
}

export function exact(value: unknown, required: readonly string[], optional: readonly string[] = []): Record<string, unknown> {
  const x = record(value);
  if (Object.keys(x).some(k => !required.includes(k) && !optional.includes(k))
    || required.some(k => !Object.prototype.hasOwnProperty.call(x, k))) throw new NekoBotBridgeError("unsupported_fields");
  return x;
}

export function opaque(value: unknown): string {
  if (typeof value !== "string" || !/^[A-Za-z0-9._:-]{1,160}$/.test(value)) throw new NekoBotBridgeError("invalid_id");
  return value;
}

export function sequence(value: unknown): number {
  if (typeof value !== "number" || !Number.isSafeInteger(value) || value < 0) throw new NekoBotBridgeError("invalid_sequence");
  return value;
}

export function revision(value: unknown): number {
  const x = sequence(value);
  if (x === 0) throw new NekoBotBridgeError("invalid_revision");
  return x;
}

export function nonce(value: unknown): string {
  const x = opaque(value);
  if (x.length < 8 || x.length > 128) throw new NekoBotBridgeError("invalid_nonce");
  return x;
}

export function parseSource(value: unknown): NekoBotLocalSource {
  const x = exact(value, ["sourceApp", "sourceSessionId", "sourceThreadId", "channelId", "expectedChannelRevision", "ownerUserId", "spaceId"]);
  if (x.sourceApp !== "wiii") throw new NekoBotBridgeError("unsupported_source_app");
  return { sourceApp: "wiii", sourceSessionId: opaque(x.sourceSessionId), sourceThreadId: opaque(x.sourceThreadId),
    channelId: opaque(x.channelId), expectedChannelRevision: revision(x.expectedChannelRevision),
    ownerUserId: opaque(x.ownerUserId), spaceId: opaque(x.spaceId) };
}

export function parseSession(value: unknown, source: NekoBotLocalSource): NekoBotAdapterSession {
  const x = exact(value, ["id", "version", "sourceApp", "sourceSessionId", "sourceThreadId", "channelId", "channelRevision", "ownerUserId", "spaceId", "returnRoute"]);
  if (x.version !== 1 || x.sourceApp !== source.sourceApp || x.sourceSessionId !== source.sourceSessionId
    || x.sourceThreadId !== source.sourceThreadId || x.channelId !== source.channelId
    || x.channelRevision !== source.expectedChannelRevision || x.ownerUserId !== source.ownerUserId || x.spaceId !== source.spaceId
    || x.returnRoute !== `/v1/adapters/sessions/${opaque(x.id)}/replies`) {
    throw new NekoBotBridgeError("session_binding_mismatch");
  }
  return { id: opaque(x.id), version: 1, sourceApp: "wiii", sourceSessionId: source.sourceSessionId,
    sourceThreadId: source.sourceThreadId, channelId: source.channelId, channelRevision: source.expectedChannelRevision,
    ownerUserId: source.ownerUserId, spaceId: source.spaceId, returnRoute: String(x.returnRoute) };
}

export function parseMessage(value: unknown): NekoBotMessageInput {
  const x = exact(value, ["sourceMessageId", "botId", "text", "lane", "clientNonce"],
    ["accountId", "accountRevision", "projectId", "projectRevision", "fileIds"]);
  if (typeof x.text !== "string" || !x.text.trim() || new TextEncoder().encode(x.text).length > NEKOBOT_LOCAL_MAX_TEXT_BYTES
    || /[\uD800-\uDBFF](?![\uDC00-\uDFFF])|(?<![\uD800-\uDBFF])[\uDC00-\uDFFF]/u.test(x.text)) throw new NekoBotBridgeError("invalid_message_text");
  if (x.lane !== "foreground" && x.lane !== "background") throw new NekoBotBridgeError("invalid_lane");
  const result: NekoBotMessageInput = { sourceMessageId: opaque(x.sourceMessageId), botId: opaque(x.botId),
    text: x.text, lane: x.lane, clientNonce: nonce(x.clientNonce) };
  for (const kind of ["account", "project"] as const) {
    const id = x[`${kind}Id`], rev = x[`${kind}Revision`];
    if ((id === undefined) !== (rev === undefined)) throw new NekoBotBridgeError("incomplete_resource_pin");
    if (id !== undefined) { result[`${kind}Id`] = opaque(id); result[`${kind}Revision`] = revision(rev); }
  }
  if (x.fileIds !== undefined) {
    if (!Array.isArray(x.fileIds) || x.fileIds.length > 32) throw new NekoBotBridgeError("invalid_file_ids");
    result.fileIds = x.fileIds.map(opaque);
    if (new Set(result.fileIds).size !== result.fileIds.length) throw new NekoBotBridgeError("duplicate_file_id");
  }
  return result;
}

export function parseReply(value: unknown): NekoBotReply {
  const x = exact(value, ["replyId", "requestId", "sourceMessageId", "botId", "role", "text", "status", "sequence"], ["artifacts"]);
  const requestId = opaque(x.requestId);
  if (x.replyId !== `reply-${requestId}` || x.role !== "assistant" || typeof x.text !== "string"
    || new TextEncoder().encode(x.text).length > NEKOBOT_LOCAL_MAX_PAGE_BYTES || typeof x.status !== "string"
    || !/^[a-z_]{1,64}$/.test(x.status) || x.artifacts !== undefined && (!Array.isArray(x.artifacts) || x.artifacts.length > 128)) throw new NekoBotBridgeError("invalid_reply");
  return { replyId: opaque(x.replyId), requestId, sourceMessageId: opaque(x.sourceMessageId), botId: opaque(x.botId),
    role: "assistant", text: x.text, status: x.status, sequence: sequence(x.sequence), artifacts: ((x.artifacts ?? []) as unknown[]).map(a => {
      const v = exact(a, ["id", "filename", "sha256", "bytes"]);
      if (typeof v.filename !== "string" || !v.filename.trim() || v.filename.length > 256
        || /[\u0000-\u001f\u007f/\\]/.test(v.filename) || typeof v.sha256 !== "string" || !/^[a-f0-9]{64}$/.test(v.sha256)) throw new NekoBotBridgeError("invalid_artifact");
      return { id: opaque(v.id), filename: v.filename, sha256: v.sha256, bytes: sequence(v.bytes) };
    }) };
}

export function parsePage(value: unknown, session: NekoBotAdapterSession, after: number): NekoBotReplyPage {
  const x = exact(value, ["adapterSessionId", "sourceApp", "sourceSessionId", "sourceThreadId", "channelId", "ownerUserId", "spaceId", "channelRevision", "returnRoute", "cursor", "replies"]);
  if (x.adapterSessionId !== session.id || x.sourceApp !== session.sourceApp || x.sourceSessionId !== session.sourceSessionId
    || x.sourceThreadId !== session.sourceThreadId || x.channelId !== session.channelId || x.ownerUserId !== session.ownerUserId
    || x.spaceId !== session.spaceId || x.channelRevision !== session.channelRevision || x.returnRoute !== session.returnRoute) throw new NekoBotBridgeError("reply_binding_mismatch");
  const cursor = sequence(x.cursor);
  if (cursor < after || !Array.isArray(x.replies) || x.replies.length > 512) throw new NekoBotBridgeError("invalid_reply_cursor");
  const replies = x.replies.map(parseReply);
  if (replies.some(r => r.sequence > cursor || r.sequence <= after)) throw new NekoBotBridgeError("invalid_reply_sequence");
  return { adapterSessionId: session.id, sourceApp: "wiii", sourceSessionId: session.sourceSessionId,
    sourceThreadId: session.sourceThreadId, channelId: session.channelId, ownerUserId: session.ownerUserId,
    spaceId: session.spaceId, channelRevision: session.channelRevision, returnRoute: session.returnRoute, cursor, replies };
}
