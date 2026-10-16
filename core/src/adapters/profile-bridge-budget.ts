import { closeSync, existsSync, fsyncSync, lstatSync, mkdirSync, openSync, readFileSync, renameSync, unlinkSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { isJsonObject, isJsonNumber, isText, type JsonValue } from "../shared/wire.ts";
import { withProfileBridgeGuard } from "../shared/profile-bridge-guard.ts";
import type { Provider } from "../core/ports.ts";

export interface ProfileBridgeLimits {
  maxInputBytes: number; maxOutputTokens: number; maxRequests: number; maxSteps: number; maxSeconds: number;
}

export function parseProfileBridgeLimits(raw = process.env.NEKO_PROFILE_LIMITS): ProfileBridgeLimits {
  try {
    // SAFETY: JSON syntax yields JsonValue; exact keys and integer bounds are validated below.
    const value = JSON.parse(raw ?? "") as JsonValue;
    const keys = ["maxInputBytes", "maxOutputTokens", "maxRequests", "maxSteps", "maxSeconds"];
    if (!isJsonObject(value) || Object.keys(value).sort().join() !== [...keys].sort().join()
      || keys.some((key) => !Number.isSafeInteger(value[key]) || Number(value[key]) < 1)) throw new Error();
    if (Number(value.maxSteps) > 512 || Number(value.maxRequests) > 1000 || Number(value.maxInputBytes) > 4_194_304
      || Number(value.maxOutputTokens) > 1_000_000 || Number(value.maxSeconds) > 86_400) throw new Error();
    return { maxInputBytes: Number(value.maxInputBytes), maxOutputTokens: Number(value.maxOutputTokens),
      maxRequests: Number(value.maxRequests), maxSteps: Number(value.maxSteps), maxSeconds: Number(value.maxSeconds) };
  } catch { throw new Error("Invalid profile bridge limits"); }
}

interface BudgetState { version: 1; profileRevision: string; limits: ProfileBridgeLimits; startedAt: number; requests: number; reservedOutput: number }

export class ProfileBridgeBudget {
  private readonly path: string;
  private readonly initial: number;
  private readonly monotonic = performance.now();
  private active = false;
  constructor(private readonly home: string, private readonly revision: string, readonly limits: ProfileBridgeLimits, private readonly origin: string, private readonly deadline = Infinity) {
    this.path = join(home, ".neko-core", "profile-bridge-budget-v1.json");
    const state = this.read();
    this.initial = state?.startedAt ?? Date.now();
    this.assertTime();
  }

  private read(): BudgetState | null {
    const folder = join(this.home, ".neko-core");
    if (existsSync(folder) && lstatSync(folder).isSymbolicLink()) throw new Error("Profile bridge budget alias refused");
    if (!existsSync(this.path)) return null;
    try {
      const stat = lstatSync(this.path);
      if (!stat.isFile() || stat.isSymbolicLink() || stat.nlink !== 1 || stat.size > 4096) throw new Error();
      // SAFETY: private budget JSON is narrowed to its exact versioned counter schema below.
      const value = JSON.parse(readFileSync(this.path, "utf8")) as JsonValue;
      if (!isJsonObject(value) || Object.keys(value).sort().join() !== ["version", "profileRevision", "limits", "startedAt", "requests", "reservedOutput"].sort().join()
        || value.version !== 1 || value.profileRevision !== this.revision || JSON.stringify(value.limits) !== JSON.stringify(this.limits)
        || !isJsonNumber(value.startedAt) || !Number.isSafeInteger(value.startedAt) || value.startedAt < 1 || !isJsonNumber(value.requests) || !Number.isSafeInteger(value.requests) || value.requests < 0
        || value.requests > this.limits.maxRequests || !isJsonNumber(value.reservedOutput) || !Number.isSafeInteger(value.reservedOutput) || value.reservedOutput < 0 || value.reservedOutput > this.limits.maxOutputTokens) throw new Error();
      return { version: 1, profileRevision: this.revision, limits: this.limits, startedAt: value.startedAt,
        requests: value.requests, reservedOutput: value.reservedOutput };
    } catch { throw new Error("Profile bridge budget unavailable"); }
  }

  private assertTime(): void {
    if (Date.now() < this.initial || Date.now() >= this.deadline || Date.now() - this.initial >= this.limits.maxSeconds * 1000
      || performance.now() - this.monotonic >= this.limits.maxSeconds * 1000) throw new Error("Profile bridge time limit reached");
  }

  async complete(provider: Provider, validate: () => void, setTokens: (n: number) => void, args: Parameters<Provider["complete"]>): ReturnType<Provider["complete"]> {
    if (this.active) throw new Error("Profile bridge concurrent request refused");
    this.assertTime();
    const state = this.read();
    const requests = state?.requests ?? 0;
    const remaining = this.limits.maxOutputTokens - (state?.reservedOutput ?? 0);
    if (requests >= this.limits.maxRequests || remaining < 1) throw new Error("Profile bridge request limit reached");
    const reservation = Math.floor(remaining / (this.limits.maxRequests - requests));
    if (reservation < 1) throw new Error("Profile bridge output limit reached");
    setTokens(reservation);
    this.active = true;
    const controller = new AbortController();
    const originalSignal = args[3];
    const abort = () => controller.abort(originalSignal?.reason);
    if (originalSignal?.aborted) abort();
    originalSignal?.addEventListener("abort", abort, { once: true });
    const timer = setTimeout(() => controller.abort(new Error("Profile bridge time limit reached")), Math.max(1, Math.min(this.deadline, this.initial + this.limits.maxSeconds * 1000) - Date.now()));
    let emitted = 0;
    const onDelta = args[2];
    const next: Parameters<Provider["complete"]> = [args[0], args[1], args[2], args[3], args[4]];
    next[2] = (text, kind) => {
      validate();
      this.assertTime();
      emitted += Buffer.byteLength(text);
      if (emitted > reservation) { controller.abort(); throw new Error("Profile bridge output limit reached"); }
      onDelta?.(text, kind);
    };
    next[3] = controller.signal;
    const beforeFetch = (url: string, init: RequestInit) => {
      this.assertTime();
      if (new URL(url).origin !== this.origin) throw new Error("Profile bridge origin refused");
      if (!isText(init.body) || Buffer.byteLength(init.body) > this.limits.maxInputBytes) throw new Error("Profile bridge input limit reached");
      const current = this.read() ?? { version: 1 as const, profileRevision: this.revision, limits: this.limits, startedAt: this.initial, requests: 0, reservedOutput: 0 };
      if (current.requests >= this.limits.maxRequests || current.reservedOutput + reservation > this.limits.maxOutputTokens) throw new Error("Profile bridge request limit reached");
      const folder = join(this.home, ".neko-core");
      if (existsSync(folder) && lstatSync(folder).isSymbolicLink()) throw new Error("Profile bridge budget alias refused");
      mkdirSync(folder, { recursive: true, mode: 0o700 });
      const lockPath = this.path + ".lock";
      let lock: number;
      try { lock = openSync(lockPath, "wx", 0o600); } catch { throw new Error("Profile bridge budget locked"); }
      const temp = this.path + ".tmp";
      try {
        const fresh = this.read();
        if ((fresh?.requests ?? 0) !== current.requests || (fresh?.reservedOutput ?? 0) !== current.reservedOutput) throw new Error("Profile bridge budget changed");
        const file = openSync(temp, "wx", 0o600);
        try {
          writeFileSync(file, JSON.stringify({ ...current, requests: current.requests + 1, reservedOutput: current.reservedOutput + reservation }) + "\n");
          fsyncSync(file);
        } finally { closeSync(file); }
        renameSync(temp, this.path);
      } finally { closeSync(lock); unlinkSync(lockPath); }
    };
    try {
      const result = await withProfileBridgeGuard(validate, () => provider.complete(...next), beforeFetch);
      validate();
      this.assertTime();
      const bytes = Buffer.byteLength(result.content ?? "") + Buffer.byteLength(result.reasoning ?? "") + Buffer.byteLength(JSON.stringify(result.tool_calls));
      const tokens = result.usage?.completion_tokens ?? 0;
      if (!Number.isSafeInteger(tokens) || tokens < 0 || Math.max(bytes, emitted, tokens) > reservation) throw new Error("Profile bridge output limit reached");
      return result;
    } finally {
      clearTimeout(timer); originalSignal?.removeEventListener("abort", abort); this.active = false;
    }
  }
}
