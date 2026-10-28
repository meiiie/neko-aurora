import { expect, test } from "bun:test";
import { boundedOAuthResponse } from "../src/adapters/mcp-oauth-http.ts";

const limit = 64 * 1024;

test("OAuth response preserves status, headers and URL at the exact decoded byte limit", async () => {
  const data = "é".repeat(limit / 2);
  const original = new Response(data, { status: 201, statusText: "Created", headers: { "Content-Type": "application/json", "X-Fixture": "retained" } });
  Object.defineProperty(original, "url", { value: "https://fixture.test/metadata" });
  const result = await boundedOAuthResponse(original, new AbortController().signal, limit);
  expect(result.status).toBe(201); expect(result.statusText).toBe("Created"); expect(result.headers.get("x-fixture")).toBe("retained");
  expect(result.url).toBe(original.url); expect(result.type).toBe(original.type); expect(await result.text()).toBe(data);
});

for (const header of [undefined, "1", "invalid"] as const) test("OAuth counts streamed bytes independently of Content-Length " + header, async () => {
  let canceled = 0, reads = 0;
  const body = new ReadableStream<Uint8Array>({ pull(c) { reads++; c.enqueue(new Uint8Array(limit / 2 + 1)); }, cancel() { canceled++; } }, { highWaterMark: 0 });
  const response = new Response(body, { headers: header ? { "Content-Length": header } : {} });
  await expect(boundedOAuthResponse(response, new AbortController().signal, limit)).rejects.toThrow("within its limits");
  expect(reads).toBe(2); expect(canceled).toBe(1); expect(body.locked).toBe(false);
});

test("OAuth rejects a declared oversized body before pulling bytes", async () => {
  let reads = 0, canceled = 0;
  const body = new ReadableStream<Uint8Array>({ pull() { reads++; }, cancel() { canceled++; } }, { highWaterMark: 0 });
  await expect(boundedOAuthResponse(new Response(body, { headers: { "Content-Length": String(limit + 1) } }), new AbortController().signal, limit)).rejects.toThrow("within its limits");
  expect(reads).toBe(0); expect(canceled).toBe(1); expect(body.locked).toBe(false);
});

for (const mode of ["already-aborted", "abort", "deadline", "cancel-rejects"] as const) test("OAuth body stops on " + mode + " without awaiting a stuck cancel", async () => {
  const controller = new AbortController(); let canceled = 0;
  if (mode === "already-aborted") controller.abort();
  const body = new ReadableStream<Uint8Array>({
    pull(c) { c.enqueue(new Uint8Array(1)); if (mode !== "deadline") controller.abort(new Error("private abort reason")); },
    cancel() { canceled++; return mode === "cancel-rejects" ? Promise.reject(new Error("private cancel reason")) : new Promise<void>(() => {}); },
  }, { highWaterMark: 0 });
  const signal = mode === "deadline" ? AbortSignal.timeout(30) : controller.signal;
  // Deadline mode must be a quiet pending read, rather than a synchronous producer.
  const pending = mode === "deadline" ? new ReadableStream<Uint8Array>({ cancel() { canceled++; return new Promise<void>(() => {}); } }) : body;
  await expect(boundedOAuthResponse(new Response(pending), signal, limit)).rejects.toThrow("within its limits");
  expect(canceled).toBe(1); expect(pending.locked).toBe(false);
});

test("OAuth streaming errors are sanitized and release the reader", async () => {
  const body = new ReadableStream<Uint8Array>({ pull(c) { c.error(new Error("private upstream detail")); } });
  await expect(boundedOAuthResponse(new Response(body), new AbortController().signal, limit)).rejects.toThrow(/^MCP OAuth response could not be read within its limits$/);
  expect(body.locked).toBe(false);
});

test("OAuth preserves an empty response and rejects cancellation at EOF", async () => {
  const controller = new AbortController(), empty = new Response(null, { status: 204 });
  expect(await boundedOAuthResponse(empty, controller.signal, limit)).toBe(empty);
  const body = new ReadableStream<Uint8Array>({ pull(c) { controller.abort(); c.close(); } }, { highWaterMark: 0 });
  await expect(boundedOAuthResponse(new Response(body), controller.signal, limit)).rejects.toThrow("within its limits");
  expect(body.locked).toBe(false);
});
