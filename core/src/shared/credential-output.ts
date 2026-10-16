import { isJsonArray, isJsonObject, isText, type JsonValue } from "./wire.ts";

const MASK = "[redacted credential]";
export type CredentialSecrets = string | readonly string[];
function values(input: CredentialSecrets): string[] { return [...new Set((isText(input) ? [input] : input).filter(Boolean))]; }
function mask(secrets: string[]): string { return secrets.some(s => MASK.includes(s)) ? "" : MASK; }
export function redactCredentialText(text: string, secret: CredentialSecrets): string {
  const secrets = values(secret); const replacement = mask(secrets);
  for (const value of secrets) text = text.split(value).join(replacement);
  return text;
}

export function redactCredentialJSON(value: JsonValue, secret: CredentialSecrets): JsonValue {
  const secrets = values(secret); if (!secrets.length) return value;
  if (isText(value)) return redactCredentialText(value, secret);
  if (isJsonArray(value)) return value.map((part) => redactCredentialJSON(part, secret));
  if (isJsonObject(value)) {
    const result: { [key: string]: JsonValue } = {};
    for (const [key, part] of Object.entries(value)) {
      if (secrets.some(s => key.includes(s))) throw new Error("Credential in public field name refused");
      result[key] = redactCredentialJSON(part, secret);
    }
    return result;
  }
  return value;
}

export function containsCredentialJSON(value: JsonValue, secret: CredentialSecrets): boolean {
  const secrets = values(secret); if (!secrets.length) return false;
  if (isText(value)) return secrets.some(s => value.includes(s));
  if (isJsonArray(value)) return value.some((part) => containsCredentialJSON(part, secret));
  if (isJsonObject(value)) return Object.entries(value).some(([key, part]) => secrets.some(s => key.includes(s)) || containsCredentialJSON(part, secret));
  return false;
}

export interface CredentialOutputChunk { text: string; kind: "content" | "reasoning" }

/** Holds only a possible key prefix, including across content/thought chunks. */
class SingleCredentialOutputFilter {
  private matched = 0;
  private readonly pending: CredentialOutputChunk[] = [];
  private readonly prefix: number[];
  constructor(private readonly secret: string, private readonly replacement: string) {
    this.prefix = Array(secret.length).fill(0);
    for (let i = 1, matched = 0; i < secret.length; i++) {
      while (matched && secret[i] !== secret[matched]) matched = this.prefix[matched - 1]!;
      if (secret[i] === secret[matched]) matched++;
      this.prefix[i] = matched;
    }
  }

  push(text: string, kind: CredentialOutputChunk["kind"]): CredentialOutputChunk[] {
    if (!this.secret) return [{ text, kind }];
    const out: CredentialOutputChunk[] = [];
    const append = (text: string, channel: CredentialOutputChunk["kind"]) => {
      const last = out.at(-1);
      if (last?.kind === channel) last.text += text;
      else out.push({ text, kind: channel });
    };
    for (let i = 0; i < text.length; i++) {
      const char = text[i]!;
      while (this.matched && char !== this.secret[this.matched]) {
        const fallback = this.prefix[this.matched - 1]!;
        for (let count = this.matched - fallback; count > 0; count--) {
          const head = this.pending.shift()!; append(head.text, head.kind);
        }
        this.matched = fallback;
      }
      if (char === this.secret[this.matched]) {
        this.pending.push({ text: char, kind }); this.matched++;
        if (this.matched === this.secret.length) {
          append(this.replacement, this.pending[0]!.kind); this.pending.length = 0; this.matched = 0;
        }
      } else append(char, kind);
    }
    return out;
  }

  flush(): CredentialOutputChunk[] {
    if (!this.pending.length) return [];
    const kind = this.pending[0]!.kind;
    this.pending.length = 0; this.matched = 0;
    return [{ text: this.replacement, kind }];
  }
}

export class CredentialOutputFilter {
  private readonly filters: SingleCredentialOutputFilter[];
  constructor(secrets: CredentialSecrets) { const all = values(secrets); this.filters = all.map(s => new SingleCredentialOutputFilter(s, mask(all))); }
  push(text: string, kind: CredentialOutputChunk["kind"]): CredentialOutputChunk[] { return this.pipe([{ text, kind }], 0); }
  private pipe(chunks: CredentialOutputChunk[], start: number): CredentialOutputChunk[] {
    for (let i = start; i < this.filters.length; i++) chunks = chunks.flatMap(chunk => this.filters[i]!.push(chunk.text, chunk.kind));
    return chunks;
  }
  flush(): CredentialOutputChunk[] { return this.filters.flatMap((filter, index) => this.pipe(filter.flush(), index + 1)); }
}
