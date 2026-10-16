import { AsyncLocalStorage } from "node:async_hooks";

const current = new AsyncLocalStorage<{ validate: () => void; beforeFetch?: (url: string, init: RequestInit) => void }>();

export function assertProfileBridgeCurrent(): void { current.getStore()?.validate(); }
export function admitProfileBridgeFetch(url: string, init: RequestInit): void {
  assertProfileBridgeCurrent();
  current.getStore()?.beforeFetch?.(url, init);
}

export function withProfileBridgeGuard<T>(validate: () => void, run: () => T, beforeFetch?: (url: string, init: RequestInit) => void): T {
  validate();
  return current.run({ validate, beforeFetch }, run);
}
