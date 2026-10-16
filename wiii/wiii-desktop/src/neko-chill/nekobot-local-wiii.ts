import type { AnswerBlockData } from "../api/types";
import { loadStoreStrict, saveStoreStrict } from "../lib/storage";
import { NekoBotLocalBridge, type NekoBotTrustedTransport } from "./nekobot-local-bridge";
import { parseSource, type NekoBotLocalSource, type NekoBotReply } from "./nekobot-local-protocol";

export interface WiiiNekoBotTranscriptMessage {
  id: string;
  role: "assistant";
  blocks: AnswerBlockData[];
}

export function projectNekoBotReply(reply: NekoBotReply): WiiiNekoBotTranscriptMessage {
  return { id: reply.replyId, role: "assistant", blocks: [{ type: "answer", id: `${reply.replyId}:answer`, content: reply.text }] };
}

export function nekoBotBridgeStorageKey(value: NekoBotLocalSource): string {
  const source = parseSource(value);
  return [source.ownerUserId, source.spaceId, source.sourceApp, source.sourceSessionId, source.sourceThreadId,
    source.channelId, String(source.expectedChannelRevision)].map(encodeURIComponent).join("/");
}

/** Called only after the local host has supplied an owner-authenticated transport and reviewed channel binding. */
export function createWiiiNekoBotBridge(source: NekoBotLocalSource, transport: NekoBotTrustedTransport,
  makeNonce: () => string = () => crypto.randomUUID()): NekoBotLocalBridge {
  const key = nekoBotBridgeStorageKey(source);
  return new NekoBotLocalBridge(source, transport, {
    read: () => loadStoreStrict("nekobot-local-bridge-v1.json", key, null),
    write: snapshot => saveStoreStrict("nekobot-local-bridge-v1.json", key, snapshot),
    history: {
      read: entry => loadStoreStrict(historyFile(entry), key, null),
      write: (entry, value) => saveStoreStrict(historyFile(entry), key, value),
    },
  }, makeNonce);
}

function historyFile(entry: string): string {
  if (entry === "active") return "nekobot-local-history-v2-active.json";
  const match = /^(shard|generation)\/([a-f0-9]{64})$/.exec(entry);
  if (!match) throw new Error("Invalid Neko Bot history storage key");
  return `nekobot-local-history-v2-${match[1]}-${match[2]}.json`;
}
