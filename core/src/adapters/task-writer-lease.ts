/* eslint-disable anti-slop/no-runtime-typeof, anti-slop/no-unsafe-dictionary-type -- Validate bounded private helper receipts at this process boundary. */
import { spawn } from "node:child_process";
import { createHash, randomBytes } from "node:crypto";
import { createReadStream, lstatSync, realpathSync } from "node:fs";
import { isAbsolute } from "node:path";

const CONTRACT = "neko.windows.writer-lease.v2";
const HEX = /^[a-f0-9]{64}$/;

async function digestFile(path: string): Promise<string> {
  const digest = createHash("sha256");
  for await (const block of createReadStream(path)) digest.update(block);
  return digest.digest("hex");
}

/** Trusted launcher opt-in only; no model, client or project configuration selects the helper. */
export async function acquireNativeTaskWriter(dir: string, sessionId: string): Promise<(() => Promise<void>) | undefined> {
  const executable = process.env.NEKO_TASK_WRITER_HELPER;
  const expectedHash = process.env.NEKO_TASK_WRITER_HELPER_SHA256;
  if (executable === undefined && expectedHash === undefined) return undefined;
  if (process.platform !== "win32" || !executable || !isAbsolute(executable)
    || !expectedHash || !HEX.test(expectedHash)) throw new Error("Invalid native task writer descriptor");
  const stat = lstatSync(executable);
  if (!stat.isFile() || stat.isSymbolicLink() || stat.nlink !== 1
    || await digestFile(executable) !== expectedHash) throw new Error("Native task writer executable identity mismatch");
  const parentHash = await digestFile(process.execPath);
  const token = randomBytes(16).toString("hex");
  const directory = realpathSync.native(dir);
  const child = spawn(executable, ["--directory", directory, "--session", sessionId,
    "--parent", String(process.pid), "--parent-sha256", parentHash], {
    windowsHide: true, stdio: ["pipe", "pipe", "pipe"],
    // The native lease helper needs no model/account environment or user home.
    env: { SystemRoot: process.env.SystemRoot ?? "C:\\Windows" },
  });
  const exited = new Promise<number | null>((resolve) => child.once("exit", resolve));
  let buffered = "";
  let failed = false;
  let receiptCount = 0;
  let waiter: { resolve: (line: string) => void; reject: (error: Error) => void } | undefined;
  const pending: string[] = [];
  const unavailable = () => new Error("Native task writer ownership unavailable");
  const fail = () => { failed = true; waiter?.reject(unavailable()); waiter = undefined; };
  child.on("error", fail);
  child.stdin.on("error", fail);
  child.stdout.on("error", fail);
  child.stderr.on("error", fail);
  child.on("exit", (code) => { if (code !== 0 || receiptCount !== 2) fail(); });
  // Diagnostics are bounded and never include paths, tokens or account bytes in an outer error.
  let stderrBytes = 0;
  child.stderr.on("data", (chunk: Buffer) => { stderrBytes += chunk.length; if (stderrBytes > 8192) { fail(); child.kill(); } });
  child.stdout.on("data", (chunk: Buffer) => {
    if (failed) return;
    buffered += chunk.toString("utf8");
    if (Buffer.byteLength(buffered, "utf8") > 8192) { fail(); child.kill(); return; }
    let split: number;
    while ((split = buffered.indexOf("\n")) >= 0) {
      const line = buffered.slice(0, split); buffered = buffered.slice(split + 1);
      if (++receiptCount > 2) { fail(); child.kill(); return; }
      if (waiter) { const current = waiter; waiter = undefined; current.resolve(line); }
      else pending.push(line);
    }
  });
  const readReceipt = async (state: "held" | "released"): Promise<void> => {
    if (failed) throw unavailable();
    let timer: ReturnType<typeof setTimeout> | undefined;
    try {
      const line = pending.shift() ?? await new Promise<string>((resolve, reject) => {
        waiter = { resolve, reject };
        timer = setTimeout(() => { fail(); child.kill(); }, 10_000);
      });
      // SAFETY: JSON may have arbitrary fields; this private receipt is checked before authority is retained.
      const receipt = JSON.parse(line) as Record<string, unknown>;
      if (Object.keys(receipt).sort().join(",") !== "contract,directory,parentCreated,parentPid,parentSha256,sessionId,state,token,version"
        || receipt.version !== 2 || receipt.contract !== CONTRACT || receipt.sessionId !== sessionId
        || receipt.parentPid !== process.pid || receipt.parentSha256 !== parentHash || receipt.token !== token
        || receipt.state !== state || typeof receipt.directory !== "string" || !isAbsolute(receipt.directory)
        || realpathSync.native(receipt.directory).toLowerCase() !== directory.toLowerCase()
        || typeof receipt.parentCreated !== "string" || !/^[1-9][0-9]{0,19}$/.test(receipt.parentCreated)) {
        throw unavailable();
      }
    } finally { if (timer) clearTimeout(timer); }
  };
  try {
    child.stdin.write(JSON.stringify({ version: 2, action: "acquire", token }) + "\n");
    await readReceipt("held");
  } catch { child.kill(); throw unavailable(); }
  let released = false;
  return async () => {
    if (released) return;
    if (failed) throw unavailable();
    child.stdin.write(JSON.stringify({ version: 2, action: "release", token }) + "\n");
    let timer: ReturnType<typeof setTimeout> | undefined;
    try {
      await readReceipt("released"); child.stdin.end();
      const code = await Promise.race([exited, new Promise<never>((_, reject) => {
        timer = setTimeout(() => reject(unavailable()), 10_000);
      })]);
      if (code !== 0 || failed) throw unavailable();
      released = true;
    } catch { child.kill(); throw unavailable(); }
    finally { if (timer) clearTimeout(timer); }
  };
}
