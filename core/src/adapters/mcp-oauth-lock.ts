import { randomBytes } from "node:crypto";
import { lstatSync, mkdirSync, readdirSync, renameSync, rmdirSync, unlinkSync, writeFileSync } from "node:fs";
import { join } from "node:path";

export function withOAuthRecordLock<T>(dir: string, action: () => T): T {
  const owner = `${process.pid}-${randomBytes(16).toString("hex")}`;
  const candidate = join(dir, `.oauth-candidate-${owner}`), lock = join(dir, ".oauth-write.lock");
  const unavailable = () => new Error("MCP OAuth credentials are busy or unavailable; reconnect after the current sign-in finishes");
  const removeOwned = (path: string) => {
    try { const stat = lstatSync(path);if (!stat.isDirectory() || stat.isSymbolicLink()) return; } catch { return; }
    try { unlinkSync(join(path, owner)); } catch { return; }
    // A successor always publishes a nonempty directory, so this cannot remove its lease.
    try { rmdirSync(path); } catch {}
  };
  let acquired = false;
  try {
    mkdirSync(dir, { recursive: true, mode: 0o700 });
    const directory = lstatSync(dir);
    if (!directory.isDirectory() || directory.isSymbolicLink()) throw unavailable();
    mkdirSync(candidate, { mode: 0o700 });
    writeFileSync(join(candidate, owner), "", { flag: "wx", mode: 0o600 });
    for (let attempt = 0; attempt < 3; attempt++) {
      try { renameSync(candidate, lock); acquired = true; break; } catch (error) {
        if (!(error instanceof Error) || !("code" in error) || !["EEXIST", "ENOTEMPTY", "EPERM", "EACCES"].includes(String(error.code))) throw unavailable();
      }
      const stat = lstatSync(lock);
      if (!stat.isDirectory() || stat.isSymbolicLink()) throw unavailable();
      const owners = readdirSync(lock);
      if (owners.length > 1 || owners.length === 1 && !/^[1-9]\d{0,9}-[a-f0-9]{32}$/.test(owners[0]!)) throw unavailable();
      if (owners.length) {
        const pid = Number(owners[0]!.split("-")[0]);
        if (pid > 0x7fffffff) throw unavailable();
        try { process.kill(pid, 0); throw unavailable(); } catch (error) {
          if (!(error instanceof Error) || !("code" in error) || error.code !== "ESRCH") throw unavailable();
        }
        try { unlinkSync(join(lock, owners[0]!)); } catch { throw unavailable(); }
      }
      try { rmdirSync(lock); } catch { throw unavailable(); }
    }
    if (!acquired) throw unavailable();
  } catch { throw unavailable(); }
  finally {
    if (!acquired) {
      removeOwned(candidate);
      try { rmdirSync(candidate); } catch {}
    }
  }
  try { return action(); } finally { removeOwned(lock); }
}
