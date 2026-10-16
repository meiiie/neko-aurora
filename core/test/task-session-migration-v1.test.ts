import { expect, test } from "bun:test";
import { createHash } from "node:crypto";
import { copyFileSync, existsSync, linkSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, truncateSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { basename, join, resolve, sep } from "node:path";
import { createTaskScope } from "../src/core/task-scope.ts";
import { loadTaskSession, type TaskRuntimeInput } from "../src/adapters/task-session.ts";
import { acquireNativeTaskWriter } from "../src/adapters/task-writer-lease.ts";
import { canonicalHostJson } from "../src/core/host-effect.ts";
import vectors from "./fixtures/task-session-v1-migration-digests.json";
import { migrateTaskSessionV1, readTaskSessionV1MigrationManifest, taskSessionV1MigrationManifestDigest, validateTaskSessionV1MigrationManifest } from "../src/adapters/task-session-migration-v1.ts";

const helper = resolve("native/writer-lease/target/release/neko-writer-lease.exe");
const available = process.platform === "win32" && existsSync(helper);
if (process.env.NEKO_TEST_NATIVE_WRITER_REQUIRED === "1" && !available) throw new Error("Native migration writer helper unavailable");
function native(name: string, body: () => Promise<void>, transactions = 1) {
  // Three leases each have bounded 10s acquire/release receipts and a bounded 10s helper exit.
  test.skipIf(!available)(name, body, transactions * (3 * 3 * 10_000 + 5_000));
}

const sourceId = "1".repeat(32);
const configId = "a".repeat(64);
function fixture() {
  const base = mkdtempSync(join(tmpdir(), "neko-explicit-migration-"));
  const home = join(base, "home"); const root = join(base, "root");
  const store = join(home, ".neko-core", "task-sessions");
  mkdirSync(store, { recursive: true }); mkdirSync(root);
  const canonicalRoot = createTaskScope("fixture", root).canonicalRoot;
  const source = { schemaVersion: 1, id: sourceId, createdAt: "2026-10-11T00:00:00.000Z",
    updatedAt: "2026-10-11T00:00:00.000Z", canonicalRoot, authorityId: "local", configId,
    revision: 2, activeTaskId: "2".repeat(32), tasks: [{ id: "2".repeat(32), label: "Legacy work",
      canonicalRoot, revision: 2, messages: [{ role: "user", content: "SYNTHETIC_PRIVATE_TRANSCRIPT" }] }] };
  const bytes = Buffer.from(JSON.stringify(source, null, 2) + "\n", "utf8");
  const sourcePath = join(store, sourceId + ".json"); writeFileSync(sourcePath, bytes);
  const manifest = { version: 1, contract: "neko.task-session.explicit-v1-migration", mode: "schema-v1-label-import", operationId: "3".repeat(32),
    source: { home, sessionId: sourceId, sha256: createHash("sha256").update(bytes).digest("hex"),
      bytes: bytes.length, coreSha256: "4".repeat(64) }, canonicalRoot, authorityId: "local", configId,
    targetCoreSha256: createHash("sha256").update(readFileSync(process.execPath)).digest("hex") };
  const backupPath = join(home, ".neko-core", "task-session-migrations", sourceId + ".v1.backup.json");
  const priorHelper = process.env.NEKO_TASK_WRITER_HELPER; const priorHash = process.env.NEKO_TASK_WRITER_HELPER_SHA256;
  if (available) {
    const deployed = join(base, "writer.exe"); copyFileSync(helper, deployed);
    process.env.NEKO_TASK_WRITER_HELPER = deployed;
    process.env.NEKO_TASK_WRITER_HELPER_SHA256 = createHash("sha256").update(readFileSync(deployed)).digest("hex");
  }
  const manifestPath = join(base, "manifest.json"); writeFileSync(manifestPath, JSON.stringify(manifest));
  return { base, home, root, store, sourcePath, source, bytes, manifest, backupPath, manifestPath,
    cleanup() {
      if (priorHelper === undefined) delete process.env.NEKO_TASK_WRITER_HELPER; else process.env.NEKO_TASK_WRITER_HELPER = priorHelper;
      if (priorHash === undefined) delete process.env.NEKO_TASK_WRITER_HELPER_SHA256; else process.env.NEKO_TASK_WRITER_HELPER_SHA256 = priorHash;
      if (!resolve(base).startsWith(resolve(tmpdir()) + sep) || !basename(base).startsWith("neko-explicit-migration-")) {
        throw new Error("Refusing cleanup outside owned migration fixture");
      }
      rmSync(base, { recursive: true, force: true });
    } };
}
async function migration(f: ReturnType<typeof fixture>) {
  return migrateTaskSessionV1(f.manifest);
}
native("writer release resolves only after its native helper executable is no longer owned by the process", async () => {
  const f = fixture();
  try {
    const release = await acquireNativeTaskWriter(f.store, sourceId);
    if (!release) throw new Error("Required native writer missing");
    await release();
    expect(() => rmSync(process.env.NEKO_TASK_WRITER_HELPER!)).not.toThrow();
  } finally { f.cleanup(); }
});
async function firstLine(stream: ReadableStream<Uint8Array>): Promise<string> {
  const reader = stream.getReader(); let text = "";
  const timer = setTimeout(() => { void reader.cancel(); }, 10_000);
  try {
    while (!text.includes("\n")) {
      const part = await reader.read(); if (part.done) throw new Error("Migration fixture ended before barrier");
      text += new TextDecoder().decode(part.value);
      if (text.length > 4096) throw new Error("Unbounded migration fixture barrier");
    }
    return text.slice(0, text.indexOf("\n"));
  } finally { clearTimeout(timer); reader.releaseLock(); }
}
native("explicit offline migration publishes a byte-identical backup before returning a new session", async () => {
  const f = fixture();
  try {
    const result = await migration(f);
    expect(result.target.sessionId).not.toBe(sourceId);
    expect(existsSync(f.backupPath)).toBe(true);
    expect(readFileSync(f.backupPath)).toEqual(f.bytes);
    expect(readFileSync(f.sourcePath)).toEqual(f.bytes);
    const target = readFileSync(join(f.store, result.target.sessionId + ".json"), "utf8");
    expect(target).not.toContain("SYNTHETIC_PRIVATE_TRANSCRIPT");
    expect(JSON.parse(target).tasks[0].messages).toEqual([]);
    expect(result.taskMap).toEqual([{ sourceTaskId: "2".repeat(32), importedTaskId: result.target.activeTaskId }]);
    expect(JSON.stringify(result)).not.toContain("SYNTHETIC_PRIVATE_TRANSCRIPT");
    expect(result.semantics).toBe("new-empty-session-with-labels-and-ancestry");
  } finally { f.cleanup(); }
});
native("the same migration operation replays its exact session mapping without creating a second target", async () => {
  const f = fixture();
  try {
    const first = await migration(f); const second = await migration(f);
    expect(second).toEqual(first);
    expect(readdirSync(f.store).filter((name) => name.endsWith(".json"))).toHaveLength(2);
  } finally { f.cleanup(); }
}, 2);

native("migration source and executable pins, strict manifest bounds and invalid records fail before a target is created", async () => {
  const f = fixture();
  try {
    for (const invalid of [
      { ...f.manifest, targetCoreSha256: "f".repeat(64) },
      { ...f.manifest, source: { ...f.manifest.source, sha256: "f".repeat(64) } },
      { ...f.manifest, source: { ...f.manifest.source, bytes: f.bytes.length + 1 } },
      { ...f.manifest, source: { ...f.manifest.source, coreSha256: "bad" } },
      { ...f.manifest, authorityId: "other" },
      { ...f.manifest, configId: "b".repeat(64) },
      { ...f.manifest, unexpected: "not allowed" },
    ]) {
      await expect(migrateTaskSessionV1(invalid)).rejects.toThrow();
      expect(existsSync(f.backupPath)).toBe(false);
      expect(readdirSync(f.store).filter((name) => name.endsWith(".json"))).toEqual([sourceId + ".json"]);
    }
    writeFileSync(f.manifestPath, " ".repeat(16 * 1024 + 1));
    expect(() => readTaskSessionV1MigrationManifest(f.manifestPath)).toThrow(/bounded/);
    writeFileSync(f.manifestPath, "{");
    expect(() => readTaskSessionV1MigrationManifest(f.manifestPath)).toThrow(/JSON/);
    writeFileSync(f.sourcePath, "{");
    await expect(migration(f)).rejects.toThrow(/source pin/);
  } finally { f.cleanup(); }
}, 8);

native("legacy stale, corrupt and live-owner locks are preserved and never reclaimed by explicit migration", async () => {
  const f = fixture();
  try {
    const lockPath = join(f.store, sourceId + ".lock");
    for (const text of ["", "{", JSON.stringify({ pid: 9999999, token: "5".repeat(32), acquiredAt: "old" }),
      JSON.stringify({ pid: process.pid, token: "6".repeat(32), acquiredAt: "now" })]) {
      writeFileSync(lockPath, text);
      await expect(migration(f)).rejects.toThrow(/writer|lock/);
      expect(readFileSync(lockPath, "utf8")).toBe(text);
      expect(existsSync(f.backupPath)).toBe(false);
    }
  } finally { f.cleanup(); }
}, 4);

native("immutable backup, intent, report and target corruption are retained for reconciliation", async () => {
  for (const artifact of ["backup", "intent", "report", "target"] as const) {
    const f = fixture();
    try {
      const result = await migration(f);
      const dir = join(f.home, ".neko-core", "task-session-migrations");
      const path = artifact === "backup" ? f.backupPath : artifact === "target"
        ? join(f.store, result.target.sessionId + ".json") : join(dir, `${sourceId}.${artifact}.json`);
      writeFileSync(path, "CORRUPTED_ARTIFACT");
      await expect(migration(f)).rejects.toThrow();
      expect(readFileSync(path, "utf8")).toBe("CORRUPTED_ARTIFACT");
      expect(readFileSync(f.sourcePath)).toEqual(f.bytes);
    } finally { f.cleanup(); }
  }
}, 8);

native("conflicting operation identity never creates a second session, and hardlinked or oversized inputs are refused", async () => {
  const f = fixture();
  try {
    const result = await migration(f);
    await expect(migrateTaskSessionV1({ ...f.manifest, operationId: "7".repeat(32) })).rejects.toThrow(/manifest mismatch/);
    expect(readdirSync(f.store).filter((name) => name.endsWith(".json"))).toHaveLength(2);
    linkSync(f.backupPath, join(f.base, "backup-alias"));
    await expect(migration(f)).rejects.toThrow(/one-link/);
    expect(readFileSync(join(f.store, result.target.sessionId + ".json"), "utf8")).not.toContain("SYNTHETIC_PRIVATE_TRANSCRIPT");
  } finally { f.cleanup(); }
  const oversized = fixture();
  try {
    truncateSync(oversized.sourcePath, 64 * 1024 * 1024 + 1);
    await expect(migration(oversized)).rejects.toThrow(/bounded/);
  } finally { oversized.cleanup(); }
}, 4);

native("self-consistent modified intents cannot carry unknown context fields or reuse old task identities", async () => {
  for (const variant of ["snapshot-extra", "task-extra", "mapping-extra", "old-task-id", "active-task", "mapping-targets"] as const) {
    const f = fixture();
    try {
      f.source.tasks.push({ ...f.source.tasks[0]!, id: "9".repeat(32), label: "Other historical work" });
      f.bytes = Buffer.from(JSON.stringify(f.source)); writeFileSync(f.sourcePath, f.bytes);
      f.manifest.source.bytes = f.bytes.length;
      f.manifest.source.sha256 = createHash("sha256").update(f.bytes).digest("hex");
      await expect(migrateTaskSessionV1(f.manifest, { afterPhase(phase) {
        if (phase === "intent") throw new Error("Simulated interruption");
      } })).rejects.toThrow(/interruption/);
      const intentPath = join(f.home, ".neko-core", "task-session-migrations", sourceId + ".intent.json");
      const intent = JSON.parse(readFileSync(intentPath, "utf8"));
      const snapshot = JSON.parse(intent.targetSnapshot);
      if (variant === "snapshot-extra") snapshot.legacyContext = "SYNTHETIC_PRIVATE_TRANSCRIPT";
      if (variant === "task-extra") snapshot.tasks[0].messagesShadow = "SYNTHETIC_PRIVATE_TRANSCRIPT";
      if (variant === "mapping-extra") snapshot.importedFrom.taskMap[0].context = "SYNTHETIC_PRIVATE_TRANSCRIPT";
      if (variant === "old-task-id") {
        snapshot.tasks[0].id = f.source.activeTaskId; snapshot.activeTaskId = f.source.activeTaskId;
        snapshot.importedFrom.importedActiveTaskId = f.source.activeTaskId;
        snapshot.importedFrom.taskMap[0].importedTaskId = f.source.activeTaskId;
      }
      if (variant === "active-task") snapshot.activeTaskId = snapshot.tasks[1].id;
      if (variant === "mapping-targets") {
        snapshot.importedFrom.taskMap[0].importedTaskId = snapshot.tasks[1].id;
        snapshot.importedFrom.taskMap[1].importedTaskId = snapshot.tasks[0].id;
        snapshot.importedFrom.importedActiveTaskId = snapshot.tasks[1].id;
        snapshot.activeTaskId = snapshot.tasks[1].id;
      }
      intent.targetSnapshot = JSON.stringify(snapshot);
      intent.targetSha256 = createHash("sha256").update(intent.targetSnapshot).digest("hex");
      const changed = JSON.stringify(intent); writeFileSync(intentPath, changed);
      await expect(migration(f)).rejects.toThrow(/migration field|target plan/);
      expect(readFileSync(intentPath, "utf8")).toBe(changed);
      expect(readFileSync(f.sourcePath)).toEqual(f.bytes);
      expect(readFileSync(f.backupPath)).toEqual(f.bytes);
      expect(existsSync(join(f.store, snapshot.id + ".json"))).toBe(false);
      expect(existsSync(join(f.home, ".neko-core", "task-session-migrations", sourceId + ".report.json"))).toBe(false);
    } finally { f.cleanup(); }
  }
}, 12);

native("restart after each acknowledged durable stage keeps the prepared exact mapping", async () => {
  for (const phase of ["backup", "intent", "target", "report"] as const) {
    const f = fixture();
    try {
      await expect(migrateTaskSessionV1(f.manifest, { afterPhase(current) {
        if (current === phase) throw new Error("Simulated interruption");
      } })).rejects.toThrow(/interruption/);
      const first = await migration(f); const retry = await migration(f);
      expect(retry).toEqual(first);
      expect(readFileSync(f.backupPath)).toEqual(f.bytes);
      expect(readdirSync(f.store).filter((name) => name.endsWith(".json"))).toHaveLength(2);
    } finally { f.cleanup(); }
  }
}, 12);

native("actual migration process hardkill after intent or target resumes the exact mapping and denies a live owner", async () => {
  for (const phase of ["intent", "target"] as const) {
    const f = fixture();
    const source = { ...f.source, schemaVersion: 2, tasks: f.source.tasks.map((task) => ({ ...task, sourceEvents: [] })) };
    const bytes = Buffer.from(JSON.stringify(source)); writeFileSync(f.sourcePath, bytes);
    f.manifest.mode = "writer-v1-metadata-rehome";
    f.manifest.source.sha256 = createHash("sha256").update(bytes).digest("hex");
    f.manifest.source.bytes = bytes.length;
    writeFileSync(f.manifestPath, JSON.stringify(f.manifest));
    const child = Bun.spawn({ cmd: [process.execPath, "test/fixtures/task-migration-process.ts", f.manifestPath, phase],
      stdin: "ignore", stdout: "pipe", stderr: "pipe", windowsHide: true,
      env: { SystemRoot: process.env.SystemRoot ?? "C:\\Windows", NODE_ENV: "test",
        NEKO_TASK_WRITER_HELPER: process.env.NEKO_TASK_WRITER_HELPER!,
        NEKO_TASK_WRITER_HELPER_SHA256: process.env.NEKO_TASK_WRITER_HELPER_SHA256! } });
    try {
      expect(JSON.parse(await firstLine(child.stdout)).phase).toBe(phase);
      const intentPath = join(f.home, ".neko-core", "task-session-migrations", sourceId + ".intent.json");
      const planned = JSON.parse(JSON.parse(readFileSync(intentPath, "utf8")).targetSnapshot);
      await expect(migration(f)).rejects.toThrow(/writer|lock/);
      child.kill(); await child.exited;
      // Retained parent HANDLE observes hardkill; this wait is not a lock-age reclaim heuristic.
      await Bun.sleep(150);
      const result = await migration(f);
      expect(result.target.sessionId).toBe(planned.id);
      expect(result.target.activeTaskId).toBe(planned.activeTaskId);
      expect(result.taskMap).toEqual(planned.importedFrom.taskMap);
      expect(readFileSync(f.backupPath)).toEqual(bytes);
      expect(readdirSync(f.store).filter((name) => name.endsWith(".json"))).toHaveLength(2);
    } finally { child.kill(); await child.exited; await Bun.sleep(100); f.cleanup(); }
  }
}, 6);

test("Windows migration manifest rejects drive-relative and drive-ambiguous paths independently of the runner OS", () => {
  const manifest = vectors.vectors[0]!.manifest;
  expect(validateTaskSessionV1MigrationManifest(manifest).source.home).toBe(manifest.source.home);
  for (const path of ["C:fixture", "\\fixture\\home", "/fixture/home"]) {
    expect(() => validateTaskSessionV1MigrationManifest({ ...manifest, source: { ...manifest.source, home: path } })).toThrow(/manifest/);
    expect(() => validateTaskSessionV1MigrationManifest({ ...manifest, canonicalRoot: path })).toThrow(/manifest/);
  }
});

test("migration manifest digest uses sorted canonical JSON independent of input property order", () => {
  const manifest = { version: 1, contract: "neko.task-session.explicit-v1-migration", mode: "schema-v1-label-import", operationId: "3".repeat(32),
    source: { home: "C:\\fixture\\猫 & bot", sessionId: sourceId, sha256: "8".repeat(64), bytes: 1024, coreSha256: "4".repeat(64) },
    canonicalRoot: "c:\\fixture\\root", authorityId: "local", configId, targetCoreSha256: "9".repeat(64) };
  const reversed = Object.fromEntries(Object.entries(manifest).reverse());
  expect(taskSessionV1MigrationManifestDigest(reversed)).toBe(taskSessionV1MigrationManifestDigest(manifest));
  expect(taskSessionV1MigrationManifestDigest(manifest)).toMatch(/^[a-f0-9]{64}$/);
  for (const vector of vectors.vectors) {
    expect(canonicalHostJson(vector.manifest)).toBe(vector.canonicalUtf8);
    expect(taskSessionV1MigrationManifestDigest(vector.manifest)).toBe(vector.sha256);
  }
});

native("writer-v1 schema2 rehome quarantines history and effects and produces an honestly distinct usable metadata session", async () => {
  const f = fixture();
  try {
    const source = { ...f.source, schemaVersion: 2, executionAuthorityId: "d".repeat(64),
      taskProtocol: { version: 1, mode: "fixed-active-task", activationEpoch: 7, activationId: "e".repeat(32),
        priorActivation: { activationEpoch: 6, activationId: "6".repeat(32) }, executionVersion: 1, hostEffectVersion: 1 },
      tasks: f.source.tasks.map((task) => ({ ...task, sourceEvents: [], hostEffects: [{ pending: {
        version: 1, status: "awaiting_approval", operationId: "f".repeat(32), effectId: "synthetic-effect",
        requestDigest: "c".repeat(64), bindingDigest: "b".repeat(64), effectStarted: false,
        operation: { version: 1, operationId: "f".repeat(32), sessionId: sourceId, taskId: task.id,
          root: task.canonicalRoot, activationEpoch: 7, activationId: "e".repeat(32) },
      } }] })) };
    const bytes = Buffer.from(JSON.stringify(source, null, 2) + "\n"); writeFileSync(f.sourcePath, bytes);
    const manifest = { ...f.manifest, mode: "writer-v1-metadata-rehome", source: { ...f.manifest.source,
      sha256: createHash("sha256").update(bytes).digest("hex"), bytes: bytes.length } };
    const result = await migrateTaskSessionV1(manifest);
    expect(result.mode).toBe("writer-v1-metadata-rehome");
    expect(result.source.schemaVersion).toBe(2);
    expect(result.target.sessionId).not.toBe(sourceId);
    expect(result.target.taskProtocol).toBe("legacy-metadata");
    expect(result.source.effectsRequireReconciliation).toBe(true);
    expect(result.source.quarantinedHostEffectCount).toBe(1);
    expect(readFileSync(f.sourcePath)).toEqual(bytes); expect(readFileSync(f.backupPath)).toEqual(bytes);
    const target = JSON.parse(readFileSync(join(f.store, result.target.sessionId + ".json"), "utf8"));
    expect(target.importedFrom.schemaVersion).toBe(2);
    expect(target.taskProtocol).toBeUndefined(); expect(target.executionAuthorityId).toBeUndefined();
    expect(target.tasks[0].messages).toEqual([]); expect(target.tasks[0].sourceEvents).toEqual([]);
    expect(target.tasks[0].hostEffects).toBeUndefined();
    expect(await migrateTaskSessionV1(manifest)).toEqual(result);
    const options = { home: f.home, root: f.root, authorityId: "local", configId,
      sessionId: result.target.sessionId,
      runtimeFactory: (input: TaskRuntimeInput) => ({
        getMessages: () => input.messages, getSourceEvents: () => input.sourceEvents,
        assertQuiescent() {}, close() {},
      }) };
    const session = await loadTaskSession(options);
    expect(session.active.id).toBe(result.target.activeTaskId);
    expect(session.active.runtime.getMessages()).toEqual([]);
    await session.close();
    await expect(migrateTaskSessionV1(manifest)).rejects.toThrow(/reconciliation/);
  } finally { f.cleanup(); }
}, 4);

native("writer-v1 legacy lock without trustworthy creation identity remains blocked, with source and lock bytes intact", async () => {
  const f = fixture();
  try {
    const source = { ...f.source, schemaVersion: 2, tasks: f.source.tasks.map((task) => ({ ...task, sourceEvents: [] })) };
    const bytes = Buffer.from(JSON.stringify(source)); writeFileSync(f.sourcePath, bytes);
    const lock = JSON.stringify({ pid: 9999999, token: "5".repeat(32), acquiredAt: "2000-01-01T00:00:00Z" });
    const lockPath = join(f.store, sourceId + ".lock"); writeFileSync(lockPath, lock);
    const manifest = { ...f.manifest, mode: "writer-v1-metadata-rehome", source: { ...f.manifest.source,
      sha256: createHash("sha256").update(bytes).digest("hex"), bytes: bytes.length } };
    await expect(migrateTaskSessionV1(manifest)).rejects.toThrow(/writer|lock/);
    expect(readFileSync(lockPath, "utf8")).toBe(lock);
    expect(readFileSync(f.sourcePath)).toEqual(bytes);
    expect(existsSync(f.backupPath)).toBe(false);
  } finally { f.cleanup(); }
});
