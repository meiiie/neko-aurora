import { createInterface } from "node:readline";
import { readFileSync, writeFileSync, existsSync } from "node:fs";
import { createWiiiNekoBotBridge, projectNekoBotReply } from "../src/neko-chill/nekobot-local-wiii";
import type { NekoBotLocalSource } from "../src/neko-chill/nekobot-local-protocol";

const lines = createInterface({ input: process.stdin });
const pending = new Map<number, (value: { status: number; body: string }) => void>();
let nextId = 0;
let configResolve: (value: Config) => void;
interface Config { source: NekoBotLocalSource; phase: "send" | "replay" | "revoked"; storagePath: string; largeInput: boolean }
const config = await new Promise<Config>(resolve => {
  configResolve = resolve;
  lines.on("line", line => {
    const x = JSON.parse(line);
    if (x.op === "config") configResolve(x.value);
    else if (x.op === "response") { pending.get(x.id)?.(x.value); pending.delete(x.id); }
  });
});
let requestCount = 0;
const transport = { request: async (method: "GET" | "POST", relativePath: string, body?: unknown) => {
  requestCount++;
  const id = ++nextId;
  const result = new Promise<{ status: number; body: string }>(resolve => pending.set(id, resolve));
  process.stdout.write(JSON.stringify({ op: "request", id, method, relativePath, body }) + "\n");
  return result;
} };
Object.defineProperty(globalThis, "window", { value: {}, configurable: true });
Object.defineProperty(globalThis, "localStorage", { value: {
  getItem(key: string) { const data = existsSync(config.storagePath) ? JSON.parse(readFileSync(config.storagePath, "utf8")) : {}; return data[key] ?? null; },
  setItem(key: string, value: string) {
    const data = existsSync(config.storagePath) ? JSON.parse(readFileSync(config.storagePath, "utf8")) : {};
    data[key] = value; writeFileSync(config.storagePath, JSON.stringify(data));
  },
}, configurable: true });
const bridge = createWiiiNekoBotBridge(config.source, transport);
const checks: { name: string; pass: boolean }[] = [];
function check(name: string, condition: boolean) { if (!condition) throw new Error(name); checks.push({ name, pass: true }); }
await bridge.hydrate();
check("Wiii_hydration_does_not_contact_or_register_runtime", requestCount === 0);
if (config.phase === "send") {
  await bridge.open();
  const primaryText = "@davin [fixture:artifact] Reply to the Wiii source channel."
    + (config.largeInput ? "\nWiii owned initial frame boundary. ".repeat(256) : "");
  if (config.largeInput) check("Wiii_explicit_primary_UTF8_input_above_8KiB_below_32KiB",
    new TextEncoder().encode(primaryText).length > 8192 && new TextEncoder().encode(primaryText).length < 32768);
  await bridge.enqueue({ sourceMessageId: "wiii-live-primary", botId: "bot-meiiie", text: primaryText, lane: "foreground" });
  await bridge.dispatch("wiii-live-primary");
  const waitReply = async (id: string) => {
    const deadline = Date.now() + 30000;
    while (Date.now() < deadline) {
      await bridge.refresh();
      const found = bridge.visibleReplies.find(reply => reply.sourceMessageId === id && reply.status === "completed");
      if (found) return found;
      await new Promise(resolve => setTimeout(resolve, 100));
    }
    throw new Error("owned fixture reply timeout");
  };
  const primary = await waitReply("wiii-live-primary");
  check("actual_Core_tagged_reply_keeps_Wiii_source_message_and_projects_Wiii_answer", primary.botId === "bot-davin"
    && primary.text.length > 0 && primary.artifacts.length === 1 && projectNekoBotReply(primary).blocks[0].content === primary.text);
  await bridge.enqueue({ sourceMessageId: "wiii-live-lost-ack", botId: "bot-meiiie", text: "@davin [fixture:context-inspect] Inspect eligible Wiii channel context.", lane: "foreground" });
  let lost = false;
  try { await bridge.dispatch("wiii-live-lost-ack"); } catch { lost = true; }
  check("owned_transport_lost_ACK_keeps_uncertain_durable_intent", lost && bridge.snapshot.outbox.find(i => i.input.sourceMessageId === "wiii-live-lost-ack")?.state === "uncertain");
  const inspection = await waitReply("wiii-live-lost-ack");
  check("actual_Go_reply_reconciles_lost_ACK_same_request_without_resend", bridge.snapshot.outbox.find(i => i.input.sourceMessageId === inspection.sourceMessageId)?.requestId === inspection.requestId
    && bridge.snapshot.outbox.filter(i => i.input.sourceMessageId === inspection.sourceMessageId).length === 1);
} else if (config.phase === "replay") {
  const before = JSON.stringify(bridge.snapshot.outbox), session = bridge.snapshot.adapterSession?.id;
  check("reopened_Wiii_hides_cache_until_fresh_authorized_read", bridge.visibleReplies.length === 0);
  await bridge.refresh();
  check("actual_Go_restart_keeps_registered_Wiii_session_cursor_and_receipts", bridge.snapshot.adapterSession?.id === session
    && JSON.stringify(bridge.snapshot.outbox) === before && bridge.visibleReplies.length === 2 && requestCount === 1);
} else {
  const prior = JSON.stringify(bridge.snapshot.replies); let denied = false;
  try { await bridge.refresh(); } catch { denied = true; }
  check("actual_channel_revision_change_denies_without_rebind_or_cached_reply_export", denied && bridge.snapshot.blocked
    && bridge.visibleReplies.length === 0 && JSON.stringify(bridge.snapshot.replies) === prior && requestCount === 1);
}
process.stdout.write(JSON.stringify({ op: "result", checks, requestCount, adapterSessionId: bridge.snapshot.adapterSession?.id,
  source: bridge.snapshot.source, cursor: bridge.snapshot.cursor,
  mappings: bridge.snapshot.outbox.map(i => ({ sourceMessageId: i.input.sourceMessageId, requestId: i.requestId,
    state: i.state, clientNonce: i.input.clientNonce })), replies: bridge.snapshot.replies }) + "\n");
lines.close(); process.exit(0);
