import { afterEach, describe, expect, it, vi } from "vitest";
import { loadStoreStrict, saveStoreStrict } from "@/lib/storage";

const fixture = vi.hoisted(() => ({ invoke:vi.fn(), load:vi.fn(), get:vi.fn(), set:vi.fn(), save:vi.fn() }));
vi.mock("@tauri-apps/api/core", () => ({ invoke: fixture.invoke }));
vi.mock("@tauri-apps/plugin-store", () => ({ Store: { load:fixture.load } }));
Object.defineProperty(window,"__TAURI_INTERNALS__",{ value:{},configurable:true });
afterEach(() => vi.clearAllMocks());

describe("actual storage wrapper profile routing", () => {
  it("reads and writes only the path returned by native authority", async () => {
    fixture.invoke.mockImplementation(async (_command,{name}) => `E:/owned-profile/state/stores/${name}`);
    fixture.load.mockResolvedValue({get:fixture.get,set:fixture.set,save:fixture.save});
    fixture.get.mockResolvedValue("owned-canary");
    expect(await loadStoreStrict("auth.json","user",null)).toBe("owned-canary");
    await saveStoreStrict("auth.json","user","owned-fixture");
    expect(fixture.load.mock.calls).toEqual([["E:/owned-profile/state/stores/auth.json"],["E:/owned-profile/state/stores/auth.json"]]);
    expect(fixture.invoke.mock.calls).toEqual([["neko_qa_store_path",{name:"auth.json"}],["neko_qa_store_path",{name:"auth.json"}]]);
    expect(fixture.save).toHaveBeenCalledTimes(1);
  });
  it("resolution failure performs no plugin store read/write or default fallback", async () => {
    fixture.invoke.mockRejectedValue(new Error("qa path unavailable"));
    await expect(loadStoreStrict("auth.json","user",null)).rejects.toThrow();
    await expect(saveStoreStrict("auth.json","user","owned-fixture")).rejects.toThrow();
    expect(fixture.load).not.toHaveBeenCalled(); expect(fixture.set).not.toHaveBeenCalled();
  });
});
