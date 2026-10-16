import { expect, test } from "bun:test";
import { NekoBotNativeClient, nativeSourceIdentity, parseNativeStatus, type NativeCatalog } from "../../neko-chill/nekobot-native";

const catalog: NativeCatalog = { version: 1, ownerUserId: "local-owner", spaceId: "local-space",
  channels: [{ id: "channel-a", name: "Nhóm local", revision: 2, botIds: ["bot-meiiie"] }], bots: [{ id: "bot-meiiie", name: "Meiiie" }] };
const identity = { sourceSessionId: "wiii-session-a", sourceThreadId: "wiii-thread-a" };
const source = { sourceApp: "wiii", ...identity, channelId: "channel-a", expectedChannelRevision: 2, ownerUserId: "local-owner", spaceId: "local-space" };
const linked = { leaseId: "native-lease-a", source, projectId: "project-a", channelName: "Nhóm local", bots: catalog.bots };

test("construction and disconnected status do not bootstrap or send", async () => {
  const calls: unknown[] = [];
  const c = new NekoBotNativeClient(async (name, args) => { calls.push([name, args]); return { connected: false, connecting: false, catalog: null }; });
  expect(calls).toEqual([]); expect(await c.status()).toEqual({ connected: false, connecting: false, catalog: null });
  expect(calls).toEqual([["neko_bot_local_status", undefined]]);
});
test("connect sends only a literal port with no caller approval or credentials", async () => {
  const calls: unknown[] = [];
  const c = new NekoBotNativeClient(async (name, args) => { calls.push([name, args]); return { connected: true, connecting: false, catalog }; });
  await c.connect(8765); expect(calls).toEqual([["neko_bot_local_connect", { port: 8765 }]]);
});
test("invalid ports cannot reach native IPC", async () => {
  let calls = 0; const c = new NekoBotNativeClient(async () => { calls++; return null; });
  for (const port of [0, -1, 65536, 1.1, NaN, Infinity]) await expect(c.connect(port)).rejects.toThrow();
  expect(calls).toBe(0);
});
test("native confirmation denial stays a rejection and has no follow-up call", async () => {
  let calls = 0; const c = new NekoBotNativeClient(async () => { calls++; throw new Error("owner_declined"); });
  await expect(c.connect(8765)).rejects.toThrow("owner_declined"); expect(calls).toBe(1);
});
test("catalog rejects cookie and unknown metadata instead of persisting them", () => {
  expect(() => parseNativeStatus({ connected: true, connecting: false, catalog, cookie: "fixture-cookie" })).toThrow();
  expect(() => parseNativeStatus({ connected: true, connecting: false, catalog: { ...catalog, work: [] } })).toThrow();
  expect(() => parseNativeStatus({ connected: false, connecting: false, catalog })).toThrow();
});
test("catalog rejects duplicate IDs and bots outside current membership", () => {
  expect(() => parseNativeStatus({ connected: true, connecting: false, catalog: { ...catalog, bots: [...catalog.bots, ...catalog.bots] } })).toThrow();
  expect(() => parseNativeStatus({ connected: true, connecting: false, catalog: { ...catalog, channels: [{ ...catalog.channels[0], botIds: ["other-bot"] }] } })).toThrow();
});
test("explicit link pins catalog revision and forwards source IDs only as provenance", async () => {
  const calls: unknown[] = []; const c = new NekoBotNativeClient(async (name, args) => { calls.push([name, args]); return linked; });
  expect(await c.link(catalog, "channel-a", "project-a", identity)).toEqual(linked);
  expect(calls).toEqual([["neko_bot_local_link", { input: { channelId: "channel-a", expectedChannelRevision: 2, ...identity, wiiiProjectId: "project-a" } }]]);
});
for (const field of ["ownerUserId", "spaceId", "sourceSessionId", "sourceThreadId", "channelId"] as const) {
  test(`link rejects mismatched native ${field}`, async () => {
    const c = new NekoBotNativeClient(async () => ({ ...linked, source: { ...source, [field]: "wrong" } }));
    await expect(c.link(catalog, "channel-a", "project-a", identity)).rejects.toThrow("native_link_mismatch");
  });
}
test("link rejects revised audience and does not auto-select another channel", async () => {
  let calls = 0; const c = new NekoBotNativeClient(async () => { calls++; return { ...linked, source: { ...source, expectedChannelRevision: 3 } }; });
  await expect(c.link(catalog, "channel-a", "project-a", identity)).rejects.toThrow();
  await expect(c.link(catalog, "missing", "project-a", identity)).rejects.toThrow(); expect(calls).toBe(1);
});
test("transport keeps its native lease while forwarding the original message and nonce", async () => {
  const calls: unknown[] = []; const c = new NekoBotNativeClient(async (name, args) => { calls.push([name, args]); return { status: 201, body: "{}" }; });
  const transport = c.transport(linked as any), body = { sourceMessageId: "message-a", text: "Tin gốc", clientNonce: "same-nonce-a" };
  await transport.request("POST", "/v1/adapters/sessions/adapter-a/messages", body);
  await transport.request("GET", "/v1/adapters/sessions/adapter-a/replies?after=3");
  expect(calls).toEqual([["neko_bot_local_request", { input: { leaseId: "native-lease-a", method: "POST", path: "/v1/adapters/sessions/adapter-a/messages", body } }],
    ["neko_bot_local_request", { input: { leaseId: "native-lease-a", method: "GET", path: "/v1/adapters/sessions/adapter-a/replies?after=3" } }]]);
});
test("native response cannot add headers or exceed the established page cap", async () => {
  for (const response of [{ status: 200, body: "{}", headers: { cookie: "fixture-only" } }, { status: 200, body: "x".repeat(1048577) }]) {
    const c = new NekoBotNativeClient(async () => response); await expect(c.transport(linked as any).request("GET", "/v1/adapters/sessions/adapter-a/replies?after=0")).rejects.toThrow();
  }
});
test("source identity is durable before linking and survives reconnect unchanged", async () => {
  const map = new Map<string, unknown>(); let writes = 0, generated = 0;
  const storage = { read: async (key: string) => map.get(key) ?? null, write: async (key: string, value: unknown) => { writes++; map.set(key, value); } };
  const first = await nativeSourceIdentity(storage, catalog, "channel-a", "project-a", () => `identity-${++generated}`);
  expect(await nativeSourceIdentity(storage, catalog, "channel-a", "project-a", () => { throw new Error("must not regenerate"); })).toEqual(first);
  expect(writes).toBe(1); expect(generated).toBe(2);
  await nativeSourceIdentity(storage, { ...catalog, ownerUserId: "other-owner" }, "channel-a", "project-a", () => `identity-${++generated}`);
  expect(writes).toBe(2);
});
test("failed durable identity write cannot supply a link identity", async () => {
  await expect(nativeSourceIdentity({ read: async () => null, write: async () => { throw new Error("fixture disk unavailable"); } }, catalog, "channel-a", "project-a", () => "fixture-id")).rejects.toThrow();
});
test("hydrated identity cannot smuggle authority fields", async () => {
  await expect(nativeSourceIdentity({ read: async () => ({ ...identity, ownerUserId: "forged" }), write: async () => {} }, catalog, "channel-a", "project-a")).rejects.toThrow();
});
test("concurrent source selection cannot overwrite the original durable identity", async () => {
  let stored: unknown = null, writes = 0, generated = 0;
  const storage = { read: async () => stored, write: async (_key: string, value: unknown) => { await Promise.resolve(); stored = value; writes++; } };
  const make = () => nativeSourceIdentity(storage, catalog, "channel-a", "concurrent-project", () => `fixture-id-${++generated}`);
  const [a, b] = await Promise.all([make(), make()]); expect(a).toEqual(b); expect(writes).toBe(1); expect(generated).toBe(2);
});
