export interface QaNativeProfile { readonly active: true; readonly serviceUrl: string | null }

export function qaNativeProfile(): QaNativeProfile | null {
  if (typeof window === "undefined") return null;
  const value = (window as unknown as Record<string, unknown>).__WIII_QA_NATIVE_PROFILE__;
  if (value === undefined) return null;
  if (typeof value !== "object" || value === null || Array.isArray(value)) throw new Error("invalid_qa_profile");
  const item = value as Record<string, unknown>;
  if (Object.keys(item).sort().join(",") !== "active,serviceUrl" || item.active !== true) throw new Error("invalid_qa_profile");
  if (item.serviceUrl !== null && (typeof item.serviceUrl !== "string" || !/^http:\/\/127\.0\.0\.1:[1-9][0-9]{0,4}$/.test(item.serviceUrl)
    || Number(item.serviceUrl.split(":").pop()) > 65535)) throw new Error("invalid_qa_profile");
  return { active: true, serviceUrl: item.serviceUrl as string | null };
}

export async function nativeStorePath(name: string): Promise<string> {
  if (typeof window === "undefined" || !("__TAURI_INTERNALS__" in window)) return name;
  const { invoke } = await import("@tauri-apps/api/core");
  const path = await invoke<unknown>("neko_qa_store_path", { name });
  if (typeof path !== "string" || !path || path.includes("\0")) throw new Error("invalid_native_store_path");
  return path;
}

export function requireQaFixtureUrl(url: string): void {
  const profile = qaNativeProfile();
  if (!profile) return;
  if (!("__TAURI_INTERNALS__" in window) || profile.serviceUrl === null || !url.startsWith(profile.serviceUrl + "/")) {
    throw new Error("qa_service_not_explicit_fixture");
  }
}
