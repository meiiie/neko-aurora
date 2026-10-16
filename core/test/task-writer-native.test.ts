import { expect, test } from "bun:test";
import { createHash } from "node:crypto";
import { copyFileSync, existsSync, linkSync, lstatSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { basename, join, resolve, sep } from "node:path";

const helper = resolve("native/writer-lease/target/release/neko-writer-lease.exe");
const available = process.platform === "win32" && existsSync(helper);
if (process.env.NEKO_TEST_NATIVE_WRITER_REQUIRED === "1" && !available) throw new Error("Native writer helper unavailable");
const helperHash = available ? createHash("sha256").update(readFileSync(helper)).digest("hex") : "";
const parentHash = available ? createHash("sha256").update(readFileSync(process.execPath)).digest("hex") : "";
const sessionId = "c".repeat(32);
const token = "d".repeat(32);

function fixture() {
  const base = mkdtempSync(join(tmpdir(), "neko-native-writer-"));
  const home = join(base, "home"); const root = join(base, "root");
  mkdirSync(home); mkdirSync(root);
  return { base, home, root, cleanup() {
    if (!resolve(base).startsWith(resolve(tmpdir()) + sep) || !basename(base).startsWith("neko-native-writer-")) {
      throw new Error("Refusing fixture cleanup outside owned temporary directory");
    }
    rmSync(base, { recursive: true, force: true });
  } };
}
function helperProcess(directory: string) {
  const child = Bun.spawn({ cmd: [helper, "--directory", directory, "--session", sessionId,
    "--parent", String(process.pid), "--parent-sha256", parentHash],
    stdin: "pipe", stdout: "pipe", stderr: "pipe", env: { SystemRoot: process.env.SystemRoot ?? "C:\\Windows" } });
  child.stdin.write(JSON.stringify({ version: 2, action: "acquire", token }) + "\n");
  return child;
}
async function firstLine(stream: ReadableStream<Uint8Array>): Promise<string> {
  const reader = stream.getReader(); let text = "";
  const timer = setTimeout(() => { void reader.cancel(); }, 10_000);
  try {
    while (!text.includes("\n")) {
      const part = await reader.read(); if (part.done) throw new Error("Fixture process ended before readiness");
      text += new TextDecoder().decode(part.value);
      if (text.length > 8192) throw new Error("Unbounded fixture receipt");
    }
    return text.slice(0, text.indexOf("\n"));
  } finally { clearTimeout(timer); reader.releaseLock(); }
}
async function exitWithin(child: Pick<ReturnType<typeof helperProcess>, "exited" | "kill">): Promise<number> {
  let timer: ReturnType<typeof setTimeout> | undefined;
  try {
    return await Promise.race([child.exited, new Promise<never>((_, reject) => {
      timer = setTimeout(() => { child.kill(); reject(new Error("Native fixture process exit exceeded 5s")); }, 5000);
    })]);
  } finally { if (timer) clearTimeout(timer); }
}
function native(name: string, body: () => Promise<void>, receipts: number, exits: number) {
  // A series of bounded native operations needs their sum, rather than Bun's single 5s default.
  test.skipIf(!available)(name, body, receipts * 10_000 + exits * 5000 + 1000);
}
async function rejected(directory: string) {
  const child = helperProcess(directory);
  const exited = await exitWithin(child);
  expect(exited).not.toBe(0);
  expect(await new Response(child.stdout).text()).toBe("");
  expect(await new Response(child.stderr).text()).toContain("fail closed");
}

test.skipIf(process.platform !== "win32")("Concurrent Windows new-session publishers cannot overwrite the winning complete snapshot", async () => {
  const f = fixture(); const target = join(f.home, "f".repeat(32) + ".json");
  const children = ["left", "right"].map((marker) => Bun.spawn({
    cmd: [process.execPath, "test/fixtures/task-store-publisher.ts", target, marker],
    stdin: "pipe", stdout: "pipe", stderr: "pipe",
    env: { SystemRoot: process.env.SystemRoot ?? "C:\\Windows", NODE_ENV: "test" },
  }));
  try {
    for (const child of children) expect(await firstLine(child.stdout)).toBe("ready");
    for (const child of children) child.stdin.write("publish\n");
    const results = await Promise.all(children.map(async (child) => {
      // SAFETY: the bounded receipt comes only from our in-repo fixture, and its result fields are asserted below.
      const result = JSON.parse(await firstLine(child.stdout)) as { published: boolean; code?: string };
      expect(await exitWithin(child)).toBe(0);
      expect(await new Response(child.stderr).text()).toBe("");
      return result;
    }));
    expect(results.filter((result) => result.published)).toHaveLength(1);
    expect(results.find((result) => !result.published)?.code).toBe("EEXIST");
    expect(readFileSync(target, "utf8")).toBe(results[0]!.published ? "left" : "right");
    expect(lstatSync(target).nlink).toBe(1);
    expect(readdirSync(f.home)).toEqual(["f".repeat(32) + ".json"]);
  } finally {
    for (const child of children) { child.kill(); await exitWithin(child); }
    f.cleanup();
  }
}, 10_000);

native("Windows kernel handle excludes simultaneous writers; helper crash leaves live-parent lock blocked", async () => {
  const f = fixture(); const owner = helperProcess(f.home);
  try {
    const receipt = JSON.parse(await firstLine(owner.stdout));
    expect(receipt.state).toBe("held"); expect(receipt.parentPid).toBe(process.pid);
    expect(receipt.parentCreated).toMatch(/^[1-9][0-9]{0,19}$/); expect(receipt.contract).toBe("neko.windows.writer-lease.v2");
    await rejected(f.home);
    owner.kill(); await exitWithin(owner);
    await rejected(f.home);
    expect(JSON.parse(readFileSync(join(f.home, sessionId + ".lock"), "utf8")).state).toBe("held");
  } finally { owner.kill(); await exitWithin(owner); f.cleanup(); }
}, 1, 4);

native("Released v2 is reusable; legacy, empty, corrupt, unknown and hardlinked records remain fail closed", async () => {
  const f = fixture(); let owner = helperProcess(f.home);
  try {
    await firstLine(owner.stdout);
    owner.stdin.write(JSON.stringify({ version: 2, action: "release", token }) + "\n");
    expect(JSON.parse(await firstLine(owner.stdout)).state).toBe("released");
    expect(await exitWithin(owner)).toBe(0);
    const lock = join(f.home, sessionId + ".lock"); const valid = readFileSync(lock, "utf8");
    owner = helperProcess(f.home); await firstLine(owner.stdout);
    owner.stdin.write(JSON.stringify({ version: 2, action: "release", token }) + "\n");
    expect(JSON.parse(await firstLine(owner.stdout)).state).toBe("released"); expect(await exitWithin(owner)).toBe(0);
    for (const bytes of ["", "{", JSON.stringify({ pid: process.pid, token, acquiredAt: new Date().toISOString() }),
      JSON.stringify({ ...JSON.parse(valid), extra: true }), JSON.stringify({ ...JSON.parse(valid), state: "unknown" }),
      JSON.stringify({ ...JSON.parse(valid), state: "held", parentCreated: "0" + JSON.parse(valid).parentCreated }),
      JSON.stringify({ ...JSON.parse(valid), state: "held", parentCreated: "+" + JSON.parse(valid).parentCreated })]) {
      writeFileSync(lock, bytes); await rejected(f.home); expect(readFileSync(lock, "utf8")).toBe(bytes);
    }
    writeFileSync(lock, valid); linkSync(lock, join(f.home, "alias.lock"));
    await rejected(f.home); expect(readFileSync(lock, "utf8")).toBe(valid);
  } finally { owner.kill(); await exitWithin(owner); f.cleanup(); }
}, 4, 11);

native("A reused PID must have a different exact creation identity; wrong parent descriptor is rejected", async () => {
  const f = fixture(); let owner = helperProcess(f.home);
  try {
    const before = JSON.parse(await firstLine(owner.stdout));
    owner.stdin.write(JSON.stringify({ version: 2, action: "release", token }) + "\n");
    await firstLine(owner.stdout); expect(await exitWithin(owner)).toBe(0);
    const lock = join(f.home, sessionId + ".lock");
    // Synthetic PID reuse metadata: this current PID is alive, but it is not the old FILETIME identity.
    writeFileSync(lock, JSON.stringify({ ...before, state: "held", parentCreated: "1" }));
    owner = helperProcess(f.home);
    const after = JSON.parse(await firstLine(owner.stdout));
    expect(after.parentPid).toBe(before.parentPid); expect(after.parentCreated).toBe(before.parentCreated);
    owner.stdin.write(JSON.stringify({ version: 2, action: "release", token }) + "\n");
    await firstLine(owner.stdout); expect(await exitWithin(owner)).toBe(0);
    const wrong = Bun.spawn({ cmd: [helper, "--directory", f.home, "--session", "e".repeat(32),
      "--parent", String(process.pid), "--parent-sha256", "f".repeat(64)], stdin: "pipe", stdout: "pipe", stderr: "pipe",
      env: { SystemRoot: process.env.SystemRoot ?? "C:\\Windows" } });
    wrong.stdin.write(JSON.stringify({ version: 2, action: "acquire", token }) + "\n");
    expect(await exitWithin(wrong)).not.toBe(0); expect(existsSync(join(f.home, "e".repeat(32) + ".lock"))).toBe(false);
  } finally { owner.kill(); await exitWithin(owner); f.cleanup(); }
}, 4, 4);

function coreProcess(f: ReturnType<typeof fixture>, resume?: string, descriptors = true) {
  // Cargo may hardlink incremental output to deps; deployed executables must have one link.
  const deployed = join(f.base, "neko-writer-lease.exe");
  if (!existsSync(deployed)) copyFileSync(helper, deployed);
  return Bun.spawn({ cmd: [process.execPath, "test/fixtures/task-writer-process.ts", f.home, f.root,
    ...(resume ? [resume] : [])], stdin: "pipe", stdout: "pipe", stderr: "pipe",
    env: { SystemRoot: process.env.SystemRoot ?? "C:\\Windows", NODE_ENV: "test", HOME: f.home, USERPROFILE: f.home,
      ...(descriptors ? { NEKO_TASK_WRITER_HELPER: deployed, NEKO_TASK_WRITER_HELPER_SHA256: helperHash } : undefined) } });
}
native("Hardkill actual Core coordinator restores identical session/task, durable messages and next activation", async () => {
  const f = fixture(); let core = coreProcess(f);
  try {
    const before = JSON.parse(await firstLine(core.stdout));
    expect(before.messages).toEqual([{ role: "user", content: "durable fixture" }]);
    const concurrent = coreProcess(f, JSON.stringify(before));
    expect(JSON.parse(await firstLine(concurrent.stdout)).error).toBe("TaskSessionWriterUnavailableError");
    expect(await exitWithin(concurrent)).not.toBe(0);
    core.kill(); await exitWithin(core);
    // Parent death is observed through a retained kernel process handle, not a TTL.
    await Bun.sleep(150);
    core = coreProcess(f, JSON.stringify(before));
    const after = JSON.parse(await firstLine(core.stdout));
    expect(after.sessionId).toBe(before.sessionId); expect(after.task.id).toBe(before.task.id);
    expect(after.task.activationEpoch).toBe(before.task.activationEpoch + 1);
    expect(after.messages).toEqual(before.messages);
    core.stdin.write("close\n"); expect(await firstLine(core.stdout)).toBe("closed"); expect(await exitWithin(core)).toBe(0);
    const lock = join(f.home, ".neko-core", "task-sessions", after.sessionId + ".lock");
    expect(JSON.parse(readFileSync(lock, "utf8")).state).toBe("released");
    const legacy = coreProcess(f, JSON.stringify(after), false);
    expect(JSON.parse(await firstLine(legacy.stdout)).error).toBe("TaskSessionWriterUnavailableError");
    expect(await exitWithin(legacy)).not.toBe(0);
  } finally { core.kill(); await exitWithin(core); await Bun.sleep(100); f.cleanup(); }
}, 5, 5);
