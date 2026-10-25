import test from 'node:test';
import assert from 'node:assert/strict';
import { assertNamespaceQuiescent } from '../quiescence.mjs';

const namespace = `/data/profiles/${'a'.repeat(64)}`;
const uid = 10001;
const status = (owner = uid, state = 'S') => `Name:\tfixture\nState:\t${state} (fixture)\nUid:\t${owner}\t${owner}\t${owner}\t${owner}\n`;
const stat = (pid, state = 'S', start = '100') => {
  const fields = Array(20).fill('0'); fields[0] = state; fields[19] = start;
  return `${pid} (fixture chrome)) ${fields.join(' ')}\n`;
};
function proc(records, extra = {}) {
  const reads = [];
  return { reads,
    async lstat() { return { isDirectory: () => true, isSymbolicLink: () => false }; },
    async readdir() { return ['self', 'thread-self', 'sys', ...Object.keys(records)]; },
    async readFile(path) {
      reads.push(path);
      const [, , pid, file] = path.split('/'); const record = records[pid];
      if (!record) throw Object.assign(Error('fixture disappeared'), { code: 'ENOENT' });
      let value = file === 'status' ? record.status ?? status() : file === 'stat' ? record.stat ?? stat(pid) : record.cmdline;
      if (typeof value === 'function') value = await value();
      if (value instanceof Error) throw value;
      return Buffer.isBuffer(value) ? value : Buffer.from(value ?? '');
    }, ...extra };
}
const inspect = (procFS, path = namespace, options = {}) => assertNamespaceQuiescent(path, { platform: 'linux', uid, procFS, ...options });
const command = value => Buffer.from(`/usr/bin/chromium\0--user-data-dir=${value}\0--headless\0`);

test('live namespace profile and descendant argv paths prevent purge including trailing slash variants', async () => {
  for (const path of [namespace, `${namespace}/`, `${namespace}///`, `${namespace}/bot-fixture`, `${namespace}/bot-fixture/`])
    await assert.rejects(inspect(proc({ 123: { cmdline: command(path) } })), { code: 'purge_namespace_process_alive' });
});

test('different namespaces, exact-flag lookalikes and different UIDs do not block erasure', async () => {
  const otherUID = proc({ 123: { status: status(20000), cmdline: Object.assign(Error('must not inspect another user'), { code: 'EACCES' }) } });
  await inspect(otherUID); assert.deepEqual(otherUID.reads, ['/proc/123/status']);
  for (const argv of [command(`${namespace}-lookalike`), command(`/data/profiles/${'b'.repeat(64)}/bot-fixture`),
    Buffer.from(`/usr/bin/tool\0--not-user-data-dir=${namespace}\0`), command(`${namespace}/../${'b'.repeat(64)}`)]) await inspect(proc({ 123: { cmdline: argv } }));
});

test('zombie and dead matching processes cannot retain profile authority', async () => {
  for (const state of ['Z', 'X']) {
    const filesystem = proc({ 123: { status: status(uid, state), cmdline: Object.assign(Error('dead proc unreadable'), { code: 'EACCES' }) } });
    await inspect(filesystem); assert.deepEqual(filesystem.reads, ['/proc/123/status']);
  }
});

test('inaccessible or malformed own-process inspection fails closed', async () => {
  for (const record of [
    { status: Object.assign(Error('fixture denied'), { code: 'EACCES' }) },
    { status: 'State:\tS\nUid:\tbroken\n' }, { stat: '123 malformed stat', cmdline: command('/other') },
    { cmdline: Object.assign(Error('fixture denied'), { code: 'EACCES' }) },
    { cmdline: Buffer.from(`/usr/bin/chromium\0--user-data-dir=relative/path\0`) },
    { cmdline: Buffer.from(`/usr/bin/chromium\0--user-data-dir\0${namespace}\0`) },
    { cmdline: Buffer.from('/usr/bin/tool without terminal NUL') }, { cmdline: Buffer.alloc(0) },
    { cmdline: Buffer.alloc(131073, 65) }, { cmdline: Buffer.from([0xff, 0]) },
  ]) await assert.rejects(inspect(proc({ 123: record })), { code: 'purge_process_inspection_unavailable' });
});

test('process disappearance during inspection is safe but PID reuse is not proof of quiescence', async () => {
  await inspect(proc({ 123: { cmdline: Object.assign(Error('fixture disappeared'), { code: 'ENOENT' }) } }));
  let reads = 0;
  await assert.rejects(inspect(proc({ 123: { cmdline: command('/other'), stat: () => stat('123', 'S', reads++ ? '200' : '100') } })),
    { code: 'purge_process_inspection_unavailable' });
  let first = true;
  await inspect(proc({ 123: { cmdline: command(namespace), stat: () => first ? (first = false, stat('123')) : Object.assign(Error('fixture gone'), { code: 'ESRCH' }) } }));
});

test('unsupported platform, unsafe namespace and unsafe proc root fail closed', async () => {
  await assert.rejects(inspect(proc({}), namespace, { platform: 'win32' }), { code: 'purge_process_inspection_unavailable' });
  await assert.rejects(inspect(proc({}), '../outside'), { code: 'purge_process_inspection_unavailable' });
  await assert.rejects(inspect(proc({}, { async lstat() { return { isDirectory: () => true, isSymbolicLink: () => true }; } })), { code: 'purge_process_inspection_unavailable' });
  await assert.rejects(inspect(proc({}, { async readdir() { throw Object.assign(Error('fixture denied'), { code: 'EACCES' }); } })), { code: 'purge_process_inspection_unavailable' });
  const linked = proc({ 123: { cmdline: command('/other') } }, {
    async lstat(path) { return { isDirectory: () => true, isSymbolicLink: () => path === '/proc/123' }; },
  });
  await assert.rejects(inspect(linked), { code: 'purge_process_inspection_unavailable' });
  assert.deepEqual(linked.reads, []);
});
