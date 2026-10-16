import type { NekoBotTrustedTransport } from "./nekobot-local-bridge";
import { exact, opaque, parseSource, revision, type NekoBotLocalSource, NekoBotBridgeError } from "./nekobot-local-protocol";

export type NekoBotNativeInvoke = (command: string, args?: Record<string, unknown>) => Promise<unknown>;
export interface NativeBot { id: string; name: string }
export interface NativeChannel { id: string; name: string; revision: number; botIds: string[] }
export interface NativeCatalog { version: 1; ownerUserId: string; spaceId: string; channels: NativeChannel[]; bots: NativeBot[]; capabilities?: string[] }
export interface NativeStatus { connected: boolean; connecting: boolean; catalog: NativeCatalog | null }
export interface NativeLink { leaseId: string; source: NekoBotLocalSource; projectId: string; channelName: string; bots: NativeBot[]; capabilities?: string[] }
const fail = (code: string): never => { throw new NekoBotBridgeError(code); };
function name(x: unknown): string { if (typeof x !== "string" || !x || x.length > 512) fail("invalid_native_name"); return x as string; }
function bot(value: unknown): NativeBot { const x = exact(value, ["id", "name"]); return { id: opaque(x.id), name: name(x.name) }; }
function capabilities(value: unknown): string[] {
  if (!Array.isArray(value) || value.length > 64 || value.some(x => typeof x !== "string" || !/^[a-z][a-z0-9_]{1,80}$/.test(x)) || new Set(value).size !== value.length) fail("invalid_native_capabilities");
  return [...value as string[]];
}
export function parseNativeStatus(value: unknown): NativeStatus {
  const x = exact(value, ["connected", "connecting", "catalog"]);
  if (typeof x.connected !== "boolean" || typeof x.connecting !== "boolean" || x.connected && x.connecting) fail("invalid_native_status");
  if (!x.connected) { if (x.catalog !== null) fail("invalid_native_status"); return { connected: false, connecting: x.connecting as boolean, catalog: null }; }
  const c = exact(x.catalog, ["version", "ownerUserId", "spaceId", "channels", "bots"], ["capabilities"]);
  if (c.version !== 1 || !Array.isArray(c.bots) || c.bots.length > 256 || !Array.isArray(c.channels) || c.channels.length > 256) fail("invalid_native_catalog");
  const bots = (c.bots as unknown[]).map(bot);
  if (new Set(bots.map(b => b.id)).size !== bots.length) fail("invalid_native_catalog");
  const channels = (c.channels as unknown[]).map(value => {
    const v = exact(value, ["id", "name", "revision", "botIds"]);
    if (!Array.isArray(v.botIds) || !v.botIds.length || v.botIds.length > 256) fail("invalid_native_catalog");
    const botIds = (v.botIds as unknown[]).map(opaque);
    if (new Set(botIds).size !== botIds.length || botIds.some(id => !bots.some(b => b.id === id))) fail("invalid_native_catalog");
    return { id: opaque(v.id), name: name(v.name), revision: revision(v.revision), botIds };
  });
  if (new Set(channels.map(c => c.id)).size !== channels.length) fail("invalid_native_catalog");
  return { connected: true, connecting: false, catalog: { version: 1, ownerUserId: opaque(c.ownerUserId), spaceId: opaque(c.spaceId), channels, bots,
    ...(c.capabilities === undefined ? {} : { capabilities: capabilities(c.capabilities) }) } };
}

export class NekoBotNativeClient {
  private generation = 0;
  private leases = new Map<string, number>();
  constructor(private readonly invoke: NekoBotNativeInvoke) {}
  async status(): Promise<NativeStatus> {
    const status = parseNativeStatus(await this.invoke("neko_bot_local_status"));
    if (!status.connected) { this.generation++; this.leases.clear(); } return status;
  }
  async connect(port: number): Promise<NativeStatus> {
    if (!Number.isInteger(port) || port < 1 || port > 65535) fail("invalid_native_port");
    this.generation++; this.leases.clear();
    // Approval is always native. There is deliberately no approved/owner/token argument.
    return parseNativeStatus(await this.invoke("neko_bot_local_connect", { port }));
  }
  async disconnect(): Promise<void> { this.generation++; this.leases.clear(); await this.invoke("neko_bot_local_disconnect"); }
  async link(catalog: NativeCatalog, channelId: string, projectId: string, identity: { sourceSessionId: string; sourceThreadId: string }): Promise<NativeLink> {
    const generation = this.generation;
    const channel = catalog.channels.find(c => c.id === channelId); if (!channel) fail("unknown_native_channel");
    const project = opaque(projectId), session = opaque(identity.sourceSessionId), thread = opaque(identity.sourceThreadId);
    const x = exact(await this.invoke("neko_bot_local_link", { input: { channelId: channel!.id, expectedChannelRevision: channel!.revision,
      sourceSessionId: session, sourceThreadId: thread, wiiiProjectId: project } }), ["leaseId", "source", "projectId", "channelName", "bots"], ["capabilities"]);
    const source = parseSource(x.source);
    if (generation !== this.generation) fail("connection_changed");
    if (source.ownerUserId !== catalog.ownerUserId || source.spaceId !== catalog.spaceId || source.channelId !== channel!.id
      || source.expectedChannelRevision !== channel!.revision || source.sourceSessionId !== session || source.sourceThreadId !== thread
      || x.projectId !== project || x.channelName !== channel!.name || !Array.isArray(x.bots)) fail("native_link_mismatch");
    const bots = (x.bots as unknown[]).map(bot), expected = catalog.bots.filter(b => channel!.botIds.includes(b.id));
    if (JSON.stringify(bots) !== JSON.stringify(expected)) fail("native_link_mismatch");
    const supported = x.capabilities === undefined ? catalog.capabilities : capabilities(x.capabilities);
    if (x.capabilities !== undefined && JSON.stringify(supported) !== JSON.stringify(catalog.capabilities ?? [])) fail("native_link_mismatch");
    const leaseId = opaque(x.leaseId); this.leases.set(leaseId, generation);
    return { leaseId, source, projectId: project, channelName: String(x.channelName), bots, ...(supported === undefined ? {} : { capabilities: supported }) };
  }
  transport(link: NativeLink): NekoBotTrustedTransport {
    const leaseId = opaque(link.leaseId);
    const generation = this.leases.get(leaseId) ?? this.generation;
    const isCurrent = () => generation === this.generation;
    return { isCurrent, ...(link.capabilities === undefined ? {} : { capabilities: [...link.capabilities] }), request: async (method, path, body) => {
      if (!isCurrent()) fail("connection_changed");
      let value: unknown;
      try { value = await this.invoke("neko_bot_local_request", { input: { leaseId, method, path, ...(body === undefined ? {} : { body }) } }); }
      catch (error) {
        const code = typeof error === "string" ? /^neko_bot_local:(connection_changed|unknown_link|disconnected)$/.exec(error)?.[1] : null;
        if (code) throw new NekoBotBridgeError(code); throw error;
      }
      if (!isCurrent()) fail("connection_changed");
      const x = exact(value, ["status", "body"]);
      if (typeof x.status !== "number" || !Number.isInteger(x.status) || x.status < 100 || x.status > 599 || typeof x.body !== "string"
        || new TextEncoder().encode(x.body).length > 1048576) fail("invalid_native_response");
      return { status: x.status as number, body: x.body as string };
    } };
  }
}

export interface NativeIdentityStorage { read(key: string): Promise<unknown>; write(key: string, value: unknown): Promise<void> }
const identityWrites = new Map<string, Promise<void>>();
export async function nativeSourceIdentity(storage: NativeIdentityStorage, catalog: NativeCatalog, channelId: string, projectId: string,
  makeId: () => string = () => crypto.randomUUID()): Promise<{ sourceSessionId: string; sourceThreadId: string }> {
  const key = [catalog.ownerUserId, catalog.spaceId, opaque(projectId), opaque(channelId)].map(encodeURIComponent).join("/");
  const prior = identityWrites.get(key) ?? Promise.resolve();
  let release!: () => void;
  const held = new Promise<void>(resolve => { release = resolve; });
  const queue = prior.then(() => held); identityWrites.set(key, queue);
  await prior;
  try {
    const value = await storage.read(key);
    if (value !== null) { const x = exact(value, ["sourceSessionId", "sourceThreadId"]); return { sourceSessionId: opaque(x.sourceSessionId), sourceThreadId: opaque(x.sourceThreadId) }; }
    const identity = { sourceSessionId: `wiii-${opaque(makeId())}`, sourceThreadId: `thread-${opaque(makeId())}` };
    opaque(identity.sourceSessionId); opaque(identity.sourceThreadId);
    await storage.write(key, identity);
    return identity;
  } finally { release(); if (identityWrites.get(key) === queue) identityWrites.delete(key); }
}

export function nativeRuntimeAvailable(): boolean {
  return typeof window !== "undefined" && "__TAURI_INTERNALS__" in window && !(window as unknown as Record<string, unknown>).__WIII_EMBED__;
}
