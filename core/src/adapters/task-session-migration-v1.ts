/* eslint-disable anti-slop/no-unknown-parameters, anti-slop/no-unsafe-dictionary-type, anti-slop/no-runtime-typeof -- Validate bounded operator manifests and immutable offline journals at the disk boundary. */
import { createHash } from "node:crypto";
import { closeSync, constants, existsSync, fstatSync, lstatSync, mkdirSync, openSync, readFileSync, readSync, realpathSync } from "node:fs";
import { join, resolve, win32 } from "node:path";
import { createTaskScope } from "../core/task-scope.ts";
import { canonicalHostJson } from "../core/host-effect.ts";
import { acquireWriter, parseStored, parseStoredV1, planTaskSessionMetadataImport, taskStoreDir, writeNewStore } from "./task-session.ts";

const CONTRACT = "neko.task-session.explicit-v1-migration";
const MAX_SOURCE = 64 * 1024 * 1024;
const MAX_JOURNAL = 2 * 1024 * 1024;
const ID = /^[a-f0-9]{32}$/;
const HASH = /^[a-f0-9]{64}$/;

export interface TaskSessionV1MigrationManifest {
  version: 1;
  contract: typeof CONTRACT;
  mode: "schema-v1-label-import" | "writer-v1-metadata-rehome";
  operationId: string;
  source: { home: string; sessionId: string; sha256: string; bytes: number; coreSha256: string };
  canonicalRoot: string;
  authorityId: string;
  configId: string;
  targetCoreSha256: string;
}
export type MigrationPhase = "backup" | "intent" | "target" | "report";
interface MigrationHooks { afterPhase?(phase: MigrationPhase): void | Promise<void> }
interface MigrationIntent { version: 1; manifestDigest: string; manifest: TaskSessionV1MigrationManifest; targetSnapshot: string; targetSha256: string }

function digest(bytes: string | Buffer): string { return createHash("sha256").update(bytes).digest("hex"); }
function record(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) throw new Error("Invalid migration record");
  // SAFETY: the boundary value is a non-array object; each schema field is validated below.
  return value as Record<string, unknown>;
}
function keys(value: Record<string, unknown>, expected: string): void {
  if (Object.keys(value).sort().join(",") !== expected) throw new Error("Unknown or missing migration field");
}
function absolute(value: unknown): value is string {
  return typeof value === "string" && win32.isAbsolute(value) && win32.parse(value).root.length > 1
    && Buffer.byteLength(value, "utf8") <= 4096
    && !/[\x00-\x1f\x7f]/.test(value);
}
export function validateTaskSessionV1MigrationManifest(value: unknown): TaskSessionV1MigrationManifest {
  const data = record(value);
  keys(data, "authorityId,canonicalRoot,configId,contract,mode,operationId,source,targetCoreSha256,version");
  const source = record(data.source);
  keys(source, "bytes,coreSha256,home,sessionId,sha256");
  if (data.version !== 1 || data.contract !== CONTRACT
    || (data.mode !== "schema-v1-label-import" && data.mode !== "writer-v1-metadata-rehome")
    || typeof data.operationId !== "string" || !ID.test(data.operationId)
    || !absolute(data.canonicalRoot) || typeof data.authorityId !== "string" || !/^[A-Za-z0-9:._-]{1,256}$/.test(data.authorityId)
    || typeof data.configId !== "string" || !HASH.test(data.configId)
    || typeof data.targetCoreSha256 !== "string" || !HASH.test(data.targetCoreSha256)
    || !absolute(source.home) || typeof source.sessionId !== "string" || !ID.test(source.sessionId)
    || typeof source.sha256 !== "string" || !HASH.test(source.sha256)
    || typeof source.coreSha256 !== "string" || !HASH.test(source.coreSha256)
    || !Number.isSafeInteger(source.bytes) || Number(source.bytes) < 1 || Number(source.bytes) > MAX_SOURCE) {
    throw new Error("Invalid migration manifest");
  }
  // SAFETY: all exact manifest/source fields were checked above; copy to prevent caller mutation.
  return JSON.parse(JSON.stringify(value)) as TaskSessionV1MigrationManifest;
}
function boundedBytes(path: string, limit: number): Buffer {
  const before = lstatSync(path);
  if (!before.isFile() || before.isSymbolicLink() || before.nlink !== 1 || before.size < 1 || before.size > limit) {
    throw new Error("Migration artifact must be a regular one-link bounded file");
  }
  const fd = openSync(path, constants.O_RDONLY | (process.platform === "win32" ? 0 : constants.O_NOFOLLOW));
  try {
    const opened = fstatSync(fd);
    if (opened.dev !== before.dev || opened.ino !== before.ino || opened.nlink !== 1 || opened.size !== before.size) {
      throw new Error("Migration artifact identity changed");
    }
    const bytes = Buffer.alloc(opened.size); let offset = 0;
    while (offset < bytes.length) {
      const count = readSync(fd, bytes, offset, bytes.length - offset, null);
      if (count === 0) break;
      offset += count;
    }
    const extra = readSync(fd, Buffer.alloc(1), 0, 1, null);
    const after = fstatSync(fd); const named = lstatSync(path);
    if (offset !== opened.size || extra !== 0 || after.size !== opened.size || after.mtimeMs !== opened.mtimeMs
      || after.nlink !== 1 || named.ino !== opened.ino || named.dev !== opened.dev || named.isSymbolicLink()) {
      throw new Error("Migration artifact changed during read");
    }
    return bytes;
  } finally { closeSync(fd); }
}
function utf8(bytes: Buffer): string {
  try { return new TextDecoder("utf-8", { fatal: true, ignoreBOM: true }).decode(bytes); }
  catch { throw new Error("Invalid migration artifact UTF-8"); }
}
function json(bytes: Buffer): Record<string, unknown> {
  try { return record(JSON.parse(utf8(bytes))); }
  catch { throw new Error("Invalid migration artifact JSON"); }
}
export function readTaskSessionV1MigrationManifest(path: string): TaskSessionV1MigrationManifest {
  if (!absolute(path)) throw new Error("Migration manifest path must be absolute");
  return validateTaskSessionV1MigrationManifest(json(boundedBytes(path, 16 * 1024)));
}
function ordered(manifest: TaskSessionV1MigrationManifest): TaskSessionV1MigrationManifest {
  return { version: 1, contract: CONTRACT, mode: manifest.mode, operationId: manifest.operationId,
    source: { home: manifest.source.home, sessionId: manifest.source.sessionId, sha256: manifest.source.sha256,
      bytes: manifest.source.bytes, coreSha256: manifest.source.coreSha256 }, canonicalRoot: manifest.canonicalRoot,
    authorityId: manifest.authorityId, configId: manifest.configId, targetCoreSha256: manifest.targetCoreSha256 };
}
export function taskSessionV1MigrationManifestDigest(input: unknown): string {
  const manifest = validateTaskSessionV1MigrationManifest(input);
  return digest(canonicalHostJson({ ...manifest, source: { ...manifest.source } }));
}
function intentFromDisk(path: string, manifest: TaskSessionV1MigrationManifest, manifestDigest: string): MigrationIntent {
  const data = record(json(boundedBytes(path, MAX_JOURNAL)));
  keys(data, "manifest,manifestDigest,targetSha256,targetSnapshot,version");
  if (data.version !== 1 || data.manifestDigest !== manifestDigest
    || JSON.stringify(ordered(validateTaskSessionV1MigrationManifest(data.manifest))) !== JSON.stringify(manifest)
    || typeof data.targetSnapshot !== "string" || typeof data.targetSha256 !== "string"
    || !HASH.test(data.targetSha256) || digest(data.targetSnapshot) !== data.targetSha256) {
    throw new Error("Migration intent or idempotency manifest mismatch");
  }
  return { version: 1, manifestDigest, manifest, targetSnapshot: data.targetSnapshot, targetSha256: data.targetSha256 };
}

/** Offline labels/ancestry import. No runtime, provider, ACP activation or stale-v1 recovery is performed. */
export async function migrateTaskSessionV1(input: unknown, hooks: MigrationHooks = {}) {
  const manifest = ordered(validateTaskSessionV1MigrationManifest(input));
  if (process.platform !== "win32" || !process.env.NEKO_TASK_WRITER_HELPER || !process.env.NEKO_TASK_WRITER_HELPER_SHA256) {
    throw new Error("Explicit migration requires the trusted native Windows writer-v2 descriptor");
  }
  if (digest(readFileSync(process.execPath)) !== manifest.targetCoreSha256) throw new Error("Migration target executable pin mismatch");
  const canonicalRoot = createTaskScope("migration", manifest.canonicalRoot).canonicalRoot;
  if (canonicalRoot !== manifest.canonicalRoot) throw new Error("Migration root is not canonical");
  const home = resolve(manifest.source.home);
  if (!lstatSync(home).isDirectory() || lstatSync(home).isSymbolicLink()
    || realpathSync.native(home).toLowerCase() !== home.toLowerCase()) throw new Error("Migration home cannot use path aliases");
  const store = taskStoreDir(home);
  const sourceId = manifest.source.sessionId;
  const sourcePath = join(store, sourceId + ".json");
  const sourceLease = await acquireWriter(store, sourceId);
  try {
    const bytes = boundedBytes(sourcePath, MAX_SOURCE);
    if (bytes.length !== manifest.source.bytes || digest(bytes) !== manifest.source.sha256) throw new Error("Migration source pin mismatch");
    const text = utf8(bytes);
    const source = manifest.mode === "schema-v1-label-import" ? parseStoredV1(text, sourceId) : parseStored(text, sourceId);
    if (manifest.mode === "writer-v1-metadata-rehome" && source.schemaVersion !== 2) {
      throw new Error("Writer-v1 metadata rehome requires task JSON schema 2");
    }
    if (source.canonicalRoot !== canonicalRoot || source.authorityId !== manifest.authorityId || source.configId !== manifest.configId) {
      throw new Error("Migration source scope or configuration mismatch");
    }
    const dir = join(home, ".neko-core", "task-session-migrations");
    if (!existsSync(dir)) mkdirSync(dir, { mode: 0o700 });
    if (!lstatSync(dir).isDirectory() || lstatSync(dir).isSymbolicLink()) throw new Error("Migration journal cannot use a path alias");
    const operationLease = await acquireWriter(dir, sourceId);
    try {
      const backupPath = join(dir, sourceId + ".v1.backup.json");
      const intentPath = join(dir, sourceId + ".intent.json");
      const reportPath = join(dir, sourceId + ".report.json");
      const manifestDigest = taskSessionV1MigrationManifestDigest(manifest);
      if (existsSync(backupPath)) {
        if (!boundedBytes(backupPath, MAX_SOURCE).equals(bytes)) throw new Error("Migration backup pin mismatch");
      } else {
        if (existsSync(intentPath) || existsSync(reportPath)) throw new Error("Migration durable backup is missing");
        writeNewStore(backupPath, text, sourceId);
        if (!boundedBytes(backupPath, MAX_SOURCE).equals(bytes)) throw new Error("Migration backup byte verification failed");
      }
      await hooks.afterPhase?.("backup");
      let intent: MigrationIntent;
      if (existsSync(intentPath)) intent = intentFromDisk(intentPath, manifest, manifestDigest);
      else {
        if (existsSync(reportPath)) throw new Error("Migration durable intent is missing");
        const targetSnapshot = JSON.stringify(planTaskSessionMetadataImport(source, manifest.source.sha256));
        intent = { version: 1, manifestDigest, manifest, targetSnapshot, targetSha256: digest(targetSnapshot) };
        const intentText = JSON.stringify(intent);
        if (Buffer.byteLength(intentText, "utf8") > MAX_JOURNAL) throw new Error("Migration intent exceeds size limit");
        writeNewStore(intentPath, intentText, sourceId);
      }
      await hooks.afterPhase?.("intent");
      const snapshot = record(json(Buffer.from(intent.targetSnapshot)));
      keys(snapshot, "activeTaskId,authorityId,canonicalRoot,configId,createdAt,id,importedFrom,revision,schemaVersion,tasks,updatedAt");
      if (typeof snapshot.id !== "string" || !ID.test(snapshot.id) || snapshot.id === sourceId) throw new Error("Invalid migration target identity");
      const target = parseStored(intent.targetSnapshot, snapshot.id); const origin = target.importedFrom;
      if (origin) {
        keys(record(origin), "importedActiveTaskId,schemaVersion,sourceActiveTaskId,sourceRevision,sourceSessionId,sourceSha256,taskMap");
        for (const item of origin.taskMap) keys(record(item), "importedTaskId,sourceTaskId");
      }
      for (const task of target.tasks) keys(record(task), "canonicalRoot,id,label,messages,revision,sourceEvents");
      const oldIds = new Set([source.id, ...source.tasks.map((task) => task.id)]);
      if (oldIds.has(target.id) || target.schemaVersion !== 2 || target.revision !== 0 || target.taskProtocol !== undefined
        || target.executionAuthorityId !== undefined || target.canonicalRoot !== canonicalRoot
        || target.authorityId !== manifest.authorityId || target.configId !== manifest.configId
        || !origin || origin.schemaVersion !== source.schemaVersion || origin.sourceSessionId !== sourceId || origin.sourceSha256 !== manifest.source.sha256
        || origin.sourceRevision !== source.revision || origin.sourceActiveTaskId !== source.activeTaskId
        || target.activeTaskId !== origin.importedActiveTaskId
        || target.tasks.length !== source.tasks.length || target.tasks.some((task, index) =>
          oldIds.has(task.id) || task.id === target.id || task.label !== source.tasks[index]!.label
          || origin.taskMap[index]?.sourceTaskId !== source.tasks[index]!.id || origin.taskMap[index]?.importedTaskId !== task.id
          || task.revision !== 0 || task.messages.length !== 0 || task.sourceEvents.length !== 0)) {
        throw new Error("Migration target plan changed or contains legacy context");
      }
      const targetLease = await acquireWriter(store, target.id);
      try {
        const targetPath = join(store, target.id + ".json");
        if (digest(boundedBytes(sourcePath, MAX_SOURCE)) !== manifest.source.sha256) throw new Error("Migration source changed before commit");
        if (existsSync(targetPath)) {
          if (digest(boundedBytes(targetPath, MAX_SOURCE)) !== intent.targetSha256) throw new Error("Migration target changed; reconciliation required");
        } else {
          if (existsSync(reportPath)) throw new Error("Migration committed target is missing; reconciliation required");
          writeNewStore(targetPath, intent.targetSnapshot, target.id);
        }
        await hooks.afterPhase?.("target");
        if (digest(boundedBytes(targetPath, MAX_SOURCE)) !== intent.targetSha256
          || digest(boundedBytes(sourcePath, MAX_SOURCE)) !== manifest.source.sha256) {
          throw new Error("Migration committed snapshot changed; reconciliation required");
        }
        const report = { version: 1, contract: "neko.task-session.v1-migration", mode: manifest.mode, operationId: manifest.operationId,
          manifestDigest, source: { ...manifest.source, path: sourcePath, schemaVersion: source.schemaVersion,
            revision: source.revision, activeTaskId: source.activeTaskId,
            quarantinedHostEffectCount: source.tasks.reduce((count, task) => count + (task.hostEffects?.length ?? 0), 0),
            effectsRequireReconciliation: source.tasks.some((task) => task.hostEffects?.some((effect) =>
              !effect.resolution || effect.resolution.status === "unknown")) },
          backup: { path: backupPath, sha256: manifest.source.sha256, bytes: bytes.length },
          target: { sessionId: target.id, activeTaskId: target.activeTaskId, schemaVersion: 2,
            snapshotSha256: intent.targetSha256, coreSha256: manifest.targetCoreSha256, taskProtocol: "legacy-metadata" },
          taskMap: origin.taskMap, canonicalRoot, authorityId: manifest.authorityId, configId: manifest.configId,
          semantics: "new-empty-session-with-labels-and-ancestry", sourceCorePin: "operator-attestation",
          writerHelperSha256: process.env.NEKO_TASK_WRITER_HELPER_SHA256 };
        const reportText = JSON.stringify(report);
        if (existsSync(reportPath)) {
          if (!boundedBytes(reportPath, MAX_JOURNAL).equals(Buffer.from(reportText))) throw new Error("Migration report mapping mismatch");
        } else writeNewStore(reportPath, reportText, sourceId);
        await hooks.afterPhase?.("report");
        return report;
      } finally { await targetLease.release(); }
    } finally { await operationLease.release(); }
  } finally { await sourceLease.release(); }
}
