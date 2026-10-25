import { open, readdir, lstat } from 'node:fs/promises';
import { constants } from 'node:fs';
import { posix } from 'node:path';
import { fault } from './errors.mjs';

const unavailable = () => fault('purge_process_inspection_unavailable', 503);
const dead = state => state === 'Z' || state === 'X' || state === 'x';
const gone = error => error.code === 'ENOENT' || error.code === 'ESRCH';
const decoder = new TextDecoder('utf-8', { fatal: true });
const procFiles = {
  readdir, lstat,
  async readFile(path, limit) {
    const file = await open(path, constants.O_RDONLY | constants.O_NOFOLLOW);
    try {
      const buffer = Buffer.alloc(limit + 1); let length = 0;
      while (length < buffer.length) {
        const { bytesRead } = await file.read(buffer, length, buffer.length - length, length);
        if (!bytesRead) break;
        length += bytesRead;
      }
      if (length > limit) throw unavailable();
      return buffer.subarray(0, length);
    } finally { await file.close(); }
  },
};
function processStat(value, pid) {
  if (!value.startsWith(`${pid} (`)) throw unavailable();
  const close = value.lastIndexOf(') ');
  if (close < 0) throw unavailable();
  const fields = value.slice(close + 2).trim().split(/\s+/);
  if (!/^[A-Za-z]$/.test(fields[0] ?? '') || !/^[0-9]+$/.test(fields[19] ?? '')) throw unavailable();
  return { state: fields[0], start: fields[19] };
}
function profileArgument(argv, namespace) {
  let matches = false;
  for (const argument of argv) {
    if (argument === '--user-data-dir') throw unavailable();
    if (!argument.startsWith('--user-data-dir=')) continue;
    const path = argument.slice('--user-data-dir='.length);
    if (!posix.isAbsolute(path) || /[\u0000-\u001f]/.test(path)) throw unavailable();
    const normalized = posix.normalize(path).replace(/\/+$/, '') || '/';
    if (normalized === namespace || normalized.startsWith(`${namespace}/`)) matches = true;
  }
  return matches;
}

// Read-only Linux admission guard. Constructor dependencies exist only for raw-proc fixtures.
export async function assertNamespaceQuiescent(namespaceDirectory, {
  platform = process.platform, uid = process.getuid?.(), procFS = procFiles,
} = {}) {
  if (platform !== 'linux' || !Number.isSafeInteger(uid) || uid < 0 || uid > 4294967295 ||
      typeof namespaceDirectory !== 'string' || !posix.isAbsolute(namespaceDirectory) || /[\u0000-\u001f]/.test(namespaceDirectory)) throw unavailable();
  const namespace = posix.normalize(namespaceDirectory).replace(/\/+$/, '');
  if (!namespace || namespace === '/') throw unavailable();
  const read = async (path, limit) => {
    const bytes = await procFS.readFile(path, limit);
    if (!(bytes instanceof Uint8Array) || bytes.byteLength > limit) throw unavailable();
    return decoder.decode(bytes);
  };
  try {
    const root = await procFS.lstat('/proc');
    if (!root.isDirectory() || root.isSymbolicLink()) throw unavailable();
    const names = await procFS.readdir('/proc');
    if (!Array.isArray(names) || names.length > 32768 || names.some(name => typeof name !== 'string')) throw unavailable();
    for (const pid of names.filter(name => /^[1-9][0-9]{0,9}$/.test(name))) {
      try {
        const directory = `/proc/${pid}`, info = await procFS.lstat(directory);
        if (!info.isDirectory() || info.isSymbolicLink()) throw unavailable();
        const status = await read(`${directory}/status`, 16384);
        const owners = /^Uid:\s+([0-9]+)\s+([0-9]+)\s+([0-9]+)\s+([0-9]+)\s*$/m.exec(status);
        if (!owners || owners.slice(1).some(value => !Number.isSafeInteger(Number(value)) || Number(value) > 4294967295)) throw unavailable();
        if (!owners.slice(1).some(value => Number(value) === uid)) continue;
        const state = /^State:\s+([A-Za-z])(?:\s|$)/m.exec(status)?.[1];
        if (!state) throw unavailable();
        if (dead(state)) continue;
        const before = processStat(await read(`${directory}/stat`, 4096), pid);
        const command = await read(`${directory}/cmdline`, 131072);
        const after = processStat(await read(`${directory}/stat`, 4096), pid);
        if (before.start !== after.start) throw unavailable();
        if (dead(after.state)) continue;
        if (!command.length || !command.endsWith('\0')) throw unavailable();
        if (profileArgument(command.slice(0, -1).split('\0'), namespace)) throw fault('purge_namespace_process_alive', 503);
      } catch (error) { if (!gone(error)) throw error; }
    }
  } catch (error) {
    if (error.code === 'purge_namespace_process_alive') throw error;
    throw unavailable();
  }
}
