import { afterEach, describe, expect, it, vi } from "vitest";

const fixture = vi.hoisted(() => ({ invoke: vi.fn(), load: vi.fn(), get: vi.fn(), set: vi.fn(), save: vi.fn() }));
vi.mock("@tauri-apps/api/core", () => ({ invoke: fixture.invoke }));
vi.mock("@tauri-apps/plugin-store", () => ({ Store: { load: fixture.load } }));
vi.mock("@/stores/auth-store", () => ({ useAuthStore: { getState: () => ({ authMode: "legacy", tokens: null }) } }));

function profile(serviceUrl: string | null) {
  Object.defineProperty(window, "__WIII_QA_NATIVE_PROFILE__", { value: { active: true, serviceUrl }, configurable: true });
  Object.defineProperty(window, "__TAURI_INTERNALS__", { value: {}, configurable: true });
  fixture.invoke.mockImplementation(async (_command, { name }) => `E:/owned-profile/state/stores/${name}`);
  fixture.load.mockResolvedValue({ get: fixture.get, set: fixture.set, save: fixture.save });
}
afterEach(() => {
  delete (window as unknown as Record<string, unknown>).__WIII_QA_NATIVE_PROFILE__;
  delete (window as unknown as Record<string, unknown>).__TAURI_INTERNALS__;
  vi.resetModules(); vi.clearAllMocks();
});

describe("QA actual settings startup", () => {
  it("absence of a fixture never selects the default remote service", async () => {
    profile(null); fixture.get.mockResolvedValue(undefined);
    const { useSettingsStore } = await import("@/stores/settings-store");
    expect(useSettingsStore.getState().settings.server_url).toBe("");
    await useSettingsStore.getState().loadSettings();
    expect(useSettingsStore.getState().settings.server_url).toBe("");
    expect(fixture.load).toHaveBeenCalledWith("E:/owned-profile/state/stores/settings.json");
  });
  it("a saved remote address cannot override the explicitly injected fixture", async () => {
    profile("http://127.0.0.1:12345"); fixture.get.mockResolvedValue({ server_url: "https://wiii.holilihu.online", user_id: "owned-test" });
    const { useSettingsStore } = await import("@/stores/settings-store");
    await useSettingsStore.getState().loadSettings();
    expect(useSettingsStore.getState().settings.server_url).toBe("http://127.0.0.1:12345");
    expect(fixture.set.mock.calls[0][1].server_url).toBe("http://127.0.0.1:12345");
  });
  it("store path failure reads no plugin file and keeps the no-service state", async () => {
    profile(null); fixture.invoke.mockRejectedValue(new Error("qa path unavailable"));
    const warn = vi.spyOn(console, "warn").mockImplementation(() => undefined);
    try {
      const { useSettingsStore } = await import("@/stores/settings-store");
      await useSettingsStore.getState().loadSettings();
      expect(fixture.load).not.toHaveBeenCalled();
      expect(useSettingsStore.getState().settings.server_url).toBe("");
    } finally { warn.mockRestore(); }
  });
  it("actual service entry point rejects remote before any fetch or native call", async () => {
    profile("http://127.0.0.1:12345");
    const fetch = vi.spyOn(globalThis, "fetch");
    try {
      const { wiiiServiceFetch } = await import("@/api/service-fetch-runtime");
      await expect(wiiiServiceFetch("https://wiii.holilihu.online/health")).rejects.toThrow("qa_service_not_explicit_fixture");
      await expect(wiiiServiceFetch("http://localhost:8000/health")).rejects.toThrow("qa_service_not_explicit_fixture");
      expect(fetch).not.toHaveBeenCalled(); expect(fixture.invoke).not.toHaveBeenCalled();
    } finally { fetch.mockRestore(); }
  });
});
