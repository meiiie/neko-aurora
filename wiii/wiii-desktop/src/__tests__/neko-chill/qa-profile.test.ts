import { afterEach, describe, expect, it, vi } from "vitest";
import { nativeStorePath, qaNativeProfile, requireQaFixtureUrl } from "@/lib/qa-profile";

const fixture = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock("@tauri-apps/api/core", () => ({ invoke: fixture.invoke }));
function profile(value: unknown) {
  Object.defineProperty(window, "__WIII_QA_NATIVE_PROFILE__", { value, configurable: true });
  Object.defineProperty(window, "__TAURI_INTERNALS__", { value: {}, configurable: true });
}
afterEach(() => {
  delete (window as unknown as Record<string, unknown>).__WIII_QA_NATIVE_PROFILE__;
  delete (window as unknown as Record<string, unknown>).__TAURI_INTERNALS__;
  fixture.invoke.mockReset();
});

describe("QA native profile", () => {
  it("default browser paths stay unchanged and no native call is made", async () => {
    expect(qaNativeProfile()).toBeNull();
    expect(await nativeStorePath("settings.json")).toBe("settings.json");
    requireQaFixtureUrl("https://wiii.holilihu.online/health");
    expect(fixture.invoke).not.toHaveBeenCalled();
  });
  it("native store authority is a fixed command/name only, never a renderer root", async () => {
    profile({ active: true, serviceUrl: null });
    fixture.invoke.mockResolvedValue("E:/owned-profile/state/stores/auth.json");
    expect(await nativeStorePath("auth.json")).toBe("E:/owned-profile/state/stores/auth.json");
    expect(fixture.invoke.mock.calls).toEqual([["neko_qa_store_path", { name: "auth.json" }]]);
  });
  it("native path failure never falls back to a relative user store", async () => {
    profile({ active: true, serviceUrl: null });
    fixture.invoke.mockRejectedValue(new Error("profile resolution unavailable"));
    await expect(nativeStorePath("tokens.json")).rejects.toThrow("profile resolution unavailable");
    fixture.invoke.mockResolvedValue("");
    await expect(nativeStorePath("tokens.json")).rejects.toThrow("invalid_native_store_path");
  });
  it("QA disables remote/default ports and absent fixture before transport", () => {
    profile({ active: true, serviceUrl: "http://127.0.0.1:12345" });
    requireQaFixtureUrl("http://127.0.0.1:12345/health");
    for (const value of ["https://wiii.holilihu.online/health", "http://localhost:8000/health", "http://127.0.0.1:123456/health", "http://127.0.0.1:12345@evil.invalid/health"]) {
      expect(() => requireQaFixtureUrl(value)).toThrow("qa_service_not_explicit_fixture");
    }
    profile({ active: true, serviceUrl: null });
    expect(() => requireQaFixtureUrl("http://127.0.0.1:12345/health")).toThrow();
  });
  it("unknown root/credential fields and malformed fixture metadata fail closed", () => {
    for (const value of [{ active:true, serviceUrl:null, root:"E:/override" }, { active:true, serviceUrl:"https://evil.invalid" }, { active:true, serviceUrl:"http://127.0.0.1:65536" }, { active:false, serviceUrl:null }]) {
      profile(value); expect(() => qaNativeProfile()).toThrow("invalid_qa_profile");
    }
  });
});
