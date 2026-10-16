import { expect, mock, test } from "bun:test";

const calls: { name: string; args: any }[] = [];
class FixtureChannel { onmessage!: (event: any) => void; }
mock.module("@tauri-apps/api/core", () => ({
  Channel: FixtureChannel,
  invoke: async (name: string, args: any) => {
    calls.push({ name, args });
    if (name !== "neko_wiii_service_fetch") return;
    const stream = args.request.path.includes("stream");
    args.onEvent.onmessage({ kind: "headers", status: 200, headers: [["content-type", stream ? "text/event-stream" : "application/json"]] });
    args.onEvent.onmessage({ kind: "chunk", bytes: [...new TextEncoder().encode(stream ? "data: client-native\n\n" : '{"ok":true}')] });
    args.onEvent.onmessage({ kind: "done" });
  },
}));
(globalThis as any).window = { __TAURI_INTERNALS__: {}, location: { hostname: "tauri.localhost", origin: "http://tauri.localhost" } };
const { WiiiClient } = await import("../../api/client");

test("actual restored WiiiClient migrates JSON and SSE through the fixed runtime module", async () => {
  calls.length = 0;
  const client = new WiiiClient("http://localhost:8000", { "X-Session-ID": "fixture-client" });
  expect(await client.get("/api/v1/health")).toEqual({ ok: true });
  const stream = await client.postStream("/api/v1/chat/stream/v3", { text: "owned fixture" });
  expect(await new Response(stream).text()).toBe("data: client-native\n\n");
  expect(calls.map(c => c.name)).toEqual(["neko_wiii_service_fetch", "neko_wiii_service_fetch"]);
  expect(calls[1].args.request.headers).toContainEqual(["x-session-id", "fixture-client"]);
  expect(calls[1].args.request.port).toBe(8000);
});

test("actual WiiiClient preserves pre-aborted stream signal with zero native admission", async () => {
  calls.length = 0;
  const controller = new AbortController(); controller.abort();
  const client = new WiiiClient("http://localhost:8000");
  await expect(client.postStream("/api/v1/chat/stream/v3", {}, undefined, controller.signal)).rejects.toThrow();
  expect(calls).toEqual([]);
});

test("actual WiiiClient cannot use the service runtime for Neko authority routes", async () => {
  calls.length = 0;
  const client = new WiiiClient("http://127.0.0.1:8765");
  await expect(client.get("/v1/adapters/catalog")).rejects.toThrow("route denied");
  expect(calls).toEqual([]);
});

test("actual WiiiClient encoded opaque IDs survive the fixed native transport", async () => {
  calls.length = 0;
  const client = new WiiiClient("http://localhost:8000");
  const path = "/api/v1/organizations/" + encodeURIComponent("org:đội") + "/settings";
  expect(await client.get(path)).toEqual({ ok: true });
  expect(calls[0].args.request.path).toBe(path);
});
