import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

const fixtureHomes = new Set<string>();
process.once("exit", () => {
  for (const home of fixtureHomes) rmSync(home, { recursive: true, force: true });
});

/** UI fixtures need writable state, but startup release discovery is not part of their contract. */
export function createUITestHome(prefix = "neko-test-home-"): string {
  const home = mkdtempSync(join(tmpdir(), prefix));
  fixtureHomes.add(home);
  mkdirSync(join(home, ".neko-core"));
  writeFileSync(join(home, ".neko-core", "config.json"), JSON.stringify({
    auto_update_check: false,
    auto_update: false,
  }));
  return home;
}
