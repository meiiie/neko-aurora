import test from 'node:test';
import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { createHash, createHmac } from 'node:crypto';
import { mkdtemp, mkdir, writeFile, readFile, rm, stat, symlink, unlink } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { DatabaseSync } from 'node:sqlite';
import { createWorker } from '../server.mjs';

const key = 'fixture-erasure-key-not-a-real-credential-12345';
const nsA = 'a'.repeat(64), nsB = 'b'.repeat(64);
const input = (namespace, kind, extra = {}) => ({ namespace, profile: 'bot-purge', actor: 'owner', actorId: 'fixture', kind, ...extra });
async function fixture(t, extra = {}) {
  const dataDir = await mkdtemp(join(tmpdir(), 'aurora-purge-'));
  const pages = [];
  const options = { key, dataDir, assertNamespaceQuiescent: async () => {}, driver: { async open({ directory }) {
    await writeFile(join(directory, 'cookie.fixture'), 'fixture-only-browser-cookie');
    await mkdir(join(directory, 'cache')); await writeFile(join(directory, 'cache', 'marker.fixture'), 'fixture-only-cache');
    const page = {
      effects: 0, closed: false, gate: null, closeFails: false,
      async observe() { return { documentId: 'fixture-doc', origin: 'https://chat.zalo.me', title: 'Fixture',
        text: 'Fixture account ready', targets: [{ key: 'button-1', role: 'button', name: 'Record fixture effect', type: '', protected: false },
          { key: 'username-1', role: 'textbox', name: 'Username', type: 'text', protected: false }], usable: true, loginEvidence: false }; },
      async act() { this.effects++; if (this.gate) await this.gate; },
      async close() { if (this.closeFails) throw Error('fixture close uncertain'); this.closed = true; },
    }; pages.push(page); return page;
  } }, ...extra };
  let current = await createWorker(options);
  t.after(async () => { pages.forEach(p => { p.closeFails = false; }); await current.close(); await rm(dataDir, { recursive: true, force: true }); });
  return { worker: current, dataDir, pages, async restart() { await current.close(); current = await createWorker(options); return current; } };
}
async function openOwner(worker, namespace, suffix = '001') {
  const opened = await worker.command(input(namespace, 'open', { nonce: `open-purge-${suffix}` }));
  const owner = await worker.command(input(namespace, 'control', { epoch: opened.epoch, mode: 'owner', nonce: `owner-purge-${suffix}` }));
  return { owner, view: await worker.command(input(namespace, 'observe')) };
}
const purgeInput = (namespace, nonce = 'purge-fixture-once-001') => ({ namespace, nonce });

test('purge erases only its tenant journal, redactions and profiles while another namespace stays usable', async t => {
  const f = await fixture(t);
  assert.equal(typeof f.worker.purge, 'function');
  const a = await openOwner(f.worker, nsA);
  await f.worker.command(input(nsA, 'fill', { epoch: a.owner.epoch, observationId: a.view.observationId,
    targetId: a.view.targets[1].id, text: 'fixture-private-form-value', nonce: 'erase-redaction-value-001' }));
  const before = new DatabaseSync(join(f.dataDir, 'worker.sqlite'));
  assert.equal(before.prepare('SELECT COUNT(*) AS n FROM redactions WHERE namespace=?').get(nsA).n, 1); before.close();
  await f.worker.command(input(nsA, 'close', { epoch: a.owner.epoch, nonce: 'close-purge-a-001' }));
  const b = await openOwner(f.worker, nsB, '002');
  const ack = await f.worker.purge(purgeInput(nsA));
  assert.deepEqual(ack, { namespace: nsA, nonce: 'purge-fixture-once-001', status: 'purged' });
  await assert.rejects(stat(join(f.dataDir, 'profiles', nsA)), { code: 'ENOENT' });
  assert.equal(await readFile(join(f.dataDir, 'profiles', nsB, 'bot-purge', 'cookie.fixture'), 'utf8'), 'fixture-only-browser-cookie');
  const effect = await f.worker.command(input(nsB, 'click', { epoch: b.owner.epoch, observationId: b.view.observationId,
    targetId: b.view.targets[0].id, nonce: 'other-tenant-click-001' }));
  assert.equal(effect.receipt.status, 'dispatched');
  assert.equal(f.pages.at(-1).closed, false);
  const db = new DatabaseSync(join(f.dataDir, 'worker.sqlite'));
  for (const table of ['profiles', 'receipts', 'redactions']) assert.equal(db.prepare(`SELECT COUNT(*) AS n FROM ${table} WHERE namespace=?`).get(nsA).n, 0);
  assert.ok(db.prepare('SELECT COUNT(*) AS n FROM profiles WHERE namespace=?').get(nsB).n > 0); db.close();
});

test('completed purge is idempotent across restart and cannot resurrect any namespace data', async t => {
  const f = await fixture(t);
  assert.equal(typeof f.worker.purge, 'function');
  await openOwner(f.worker, nsA);
  await f.worker.purge(purgeInput(nsA));
  assert.deepEqual(await f.worker.purge(purgeInput(nsA)), { ...purgeInput(nsA), status: 'purged' });
  const worker = await f.restart();
  assert.deepEqual(await worker.purge(purgeInput(nsA, 'duplicate-after-restart-001')), { ...purgeInput(nsA, 'duplicate-after-restart-001'), status: 'purged' });
  for (const kind of ['status', 'open']) await assert.rejects(worker.command(input(nsA, kind, { nonce: 'resurrection-open-001' })), { code: 'namespace_purged' });
  await assert.rejects(stat(join(f.dataDir, 'profiles', nsA)), { code: 'ENOENT' });
});

test('partial filesystem cleanup retains tombstone and safely retries to completion', async t => {
  let attempts = 0;
  const f = await fixture(t, { async removeProfiles(path, options) {
    attempts++;
    if (attempts === 1) { await rm(join(path, 'bot-purge', 'cookie.fixture')); throw Error('fixture interrupted cleanup'); }
    await rm(path, options);
  } });
  assert.equal(typeof f.worker.purge, 'function');
  await openOwner(f.worker, nsA);
  await assert.rejects(f.worker.purge(purgeInput(nsA)), { code: 'purge_cleanup_failed' });
  await assert.rejects(f.worker.command(input(nsA, 'status')), { code: 'namespace_purged' });
  assert.deepEqual(await f.worker.purge(purgeInput(nsA)), { ...purgeInput(nsA), status: 'purged' });
  await f.worker.purge(purgeInput(nsA)); assert.equal(attempts, 2);
});

test('purge fences queued writes immediately, drains in-flight input and keeps other namespaces responsive', async t => {
  const f = await fixture(t);
  assert.equal(typeof f.worker.purge, 'function');
  const { owner, view } = await openOwner(f.worker, nsA);
  const page = f.pages.at(-1); let release; page.gate = new Promise(resolve => { release = resolve; });
  const click = { epoch: owner.epoch, observationId: view.observationId, targetId: view.targets[0].id };
  const running = f.worker.command(input(nsA, 'click', { ...click, nonce: 'in-flight-before-purge-001' }));
  await new Promise(resolve => setImmediate(resolve));
  const queued = f.worker.command(input(nsA, 'click', { ...click, nonce: 'queued-before-purge-001' }));
  const purging = f.worker.purge(purgeInput(nsA));
  const duplicate = f.worker.purge(purgeInput(nsA, 'concurrent-purge-duplicate-001'));
  await assert.rejects(f.worker.command(input(nsA, 'status')), { code: 'namespace_purged' });
  assert.equal((await f.worker.command(input(nsB, 'status'))).status, 'stopped');
  release(); await running;
  await assert.rejects(queued, { code: 'namespace_purged' });
  assert.equal((await purging).status, 'purged');
  assert.deepEqual(await duplicate, { ...purgeInput(nsA, 'concurrent-purge-duplicate-001'), status: 'purged' });
  assert.equal(page.effects, 1); assert.equal(page.closed, true);
});

test('pending erasure survives restart and cleanup retry completes without reopening the namespace', async t => {
  let attempts = 0;
  const f = await fixture(t, { async removeProfiles(path, options) {
    if (attempts++ === 0) throw Error('fixture cleanup paused');
    await rm(path, options);
  } });
  await openOwner(f.worker, nsA);
  await assert.rejects(f.worker.purge(purgeInput(nsA)), { code: 'purge_cleanup_failed' });
  const worker = await f.restart();
  await assert.rejects(worker.command(input(nsA, 'open', { nonce: 'pending-restart-open-001' })), { code: 'namespace_purged' });
  assert.equal((await worker.purge(purgeInput(nsA))).status, 'purged');
  await assert.rejects(stat(join(f.dataDir, 'profiles', nsA)), { code: 'ENOENT' });
});

test('actorless restart cannot acknowledge erasure while a namespace profile process is still alive', async t => {
  let live = false, calls = 0;
  const f = await fixture(t, { async assertNamespaceQuiescent(path) {
    assert.equal(path, join(f.dataDir, 'profiles', nsA)); calls++;
    if (live) throw Object.assign(Error('fixture live process'), { code: 'purge_namespace_process_alive', status: 503 });
  } });
  await openOwner(f.worker, nsA);
  const worker = await f.restart(); live = true;
  await assert.rejects(worker.purge(purgeInput(nsA)), { code: 'purge_namespace_process_alive' });
  assert.equal((await stat(join(f.dataDir, 'profiles', nsA))).isDirectory(), true);
  await assert.rejects(worker.command(input(nsA, 'status')), { code: 'namespace_purged' });
  live = false;
  assert.equal((await worker.purge(purgeInput(nsA))).status, 'purged');
  assert.equal(calls, 2);
});

test('even a completed duplicate purge requires current process quiescence before acknowledging', async t => {
  let live = false, calls = 0;
  const f = await fixture(t, { async assertNamespaceQuiescent() {
    calls++; if (live) throw Object.assign(Error('fixture live process'), { code: 'purge_namespace_process_alive', status: 503 });
  } });
  await openOwner(f.worker, nsA); await f.worker.purge(purgeInput(nsA));
  live = true;
  await assert.rejects(f.worker.purge(purgeInput(nsA)), { code: 'purge_namespace_process_alive' });
  live = false; assert.equal((await f.worker.purge(purgeInput(nsA))).status, 'purged');
  assert.equal(calls, 3);
  await assert.rejects(stat(join(f.dataDir, 'profiles', nsA)), { code: 'ENOENT' });
});

test('production Windows purge has no implicit process-inspection bypass', { skip: process.platform !== 'win32' }, async t => {
  const dataDir = await mkdtemp(join(tmpdir(), 'aurora-purge-unsupported-'));
  const worker = await createWorker({ key, dataDir });
  t.after(async () => { await worker.close(); await rm(dataDir, { recursive: true, force: true }); });
  await assert.rejects(worker.purge(purgeInput(nsA)), { code: 'purge_process_inspection_unavailable' });
  await assert.rejects(worker.command(input(nsA, 'status')), { code: 'namespace_purged' });
});

test('cleanup cannot acknowledge success while the namespace directory still exists', async t => {
  let attempts = 0;
  const f = await fixture(t, { async removeProfiles(path, options) { if (attempts++ > 0) await rm(path, options); } });
  await openOwner(f.worker, nsA);
  await assert.rejects(f.worker.purge(purgeInput(nsA)), { code: 'purge_cleanup_failed' });
  await assert.rejects(f.worker.command(input(nsA, 'status')), { code: 'namespace_purged' });
  assert.equal((await f.worker.purge(purgeInput(nsA))).status, 'purged');
});

test('uncertain browser close blocks erasure and preserves a retryable tombstone', async t => {
  const f = await fixture(t);
  assert.equal(typeof f.worker.purge, 'function');
  await openOwner(f.worker, nsA); const page = f.pages.at(-1); page.closeFails = true;
  await assert.rejects(f.worker.purge(purgeInput(nsA)), { code: 'purge_close_uncertain' });
  assert.equal((await stat(join(f.dataDir, 'profiles', nsA))).isDirectory(), true);
  await assert.rejects(f.worker.command(input(nsA, 'open', { nonce: 'must-stay-erased-001' })), { code: 'namespace_purged' });
  page.closeFails = false;
  assert.equal((await f.worker.purge(purgeInput(nsA))).status, 'purged');
});

test('symlink namespace root fails closed and never removes outside-path contents', async t => {
  const f = await fixture(t);
  assert.equal(typeof f.worker.purge, 'function');
  const a = await openOwner(f.worker, nsA);
  await f.worker.command(input(nsA, 'close', { epoch: a.owner.epoch, nonce: 'close-symlink-fixture-001' }));
  const outside = await mkdtemp(join(tmpdir(), 'aurora-purge-outside-'));
  t.after(() => rm(outside, { recursive: true, force: true }));
  await writeFile(join(outside, 'must-remain.fixture'), 'outside fixture survives');
  const target = join(f.dataDir, 'profiles', nsA);
  await rm(target, { recursive: true, force: true }); await symlink(outside, target, 'junction');
  await assert.rejects(f.worker.purge(purgeInput(nsA)), { code: 'unsafe_profile_directory' });
  assert.equal(await readFile(join(outside, 'must-remain.fixture'), 'utf8'), 'outside fixture survives');
  await assert.rejects(f.worker.command(input(nsA, 'status')), { code: 'namespace_purged' });
  await unlink(target);
  assert.equal((await f.worker.purge(purgeInput(nsA))).status, 'purged');
});

test('symlink profiles parent cannot redirect erasure outside the private volume', async t => {
  const f = await fixture(t);
  const a = await openOwner(f.worker, nsA);
  await f.worker.command(input(nsA, 'close', { epoch: a.owner.epoch, nonce: 'close-symlink-parent-001' }));
  const outside = await mkdtemp(join(tmpdir(), 'aurora-purge-parent-outside-'));
  t.after(() => rm(outside, { recursive: true, force: true }));
  await mkdir(join(outside, nsA)); await writeFile(join(outside, nsA, 'must-remain.fixture'), 'outside parent survives');
  const profiles = join(f.dataDir, 'profiles');
  await rm(profiles, { recursive: true, force: true }); await symlink(outside, profiles, 'junction');
  await assert.rejects(f.worker.purge(purgeInput(nsA)), { code: 'unsafe_profile_directory' });
  assert.equal(await readFile(join(outside, nsA, 'must-remain.fixture'), 'utf8'), 'outside parent survives');
  await unlink(profiles);
  assert.equal((await f.worker.purge(purgeInput(nsA))).status, 'purged');
});

test('purge HTTP accepts only fresh signed strict bodies within one KiB', async t => {
  const f = await fixture(t);
  const server = createServer(f.worker.handle); await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  t.after(() => new Promise(resolve => server.close(resolve)));
  const endpoint = `http://127.0.0.1:${server.address().port}/v1/purge`;
  async function send(value, signed = true, timestamp = Math.floor(Date.now() / 1000).toString()) {
    const body = JSON.stringify(value), headers = { 'Content-Type': 'application/json' };
    if (signed) { headers['X-Aurora-Timestamp'] = timestamp; headers['X-Aurora-Signature'] = createHmac('sha256', key)
      .update(`${timestamp}\n${createHash('sha256').update(body).digest('hex')}`).digest('hex'); }
    return fetch(endpoint, { method: 'POST', headers, body });
  }
  assert.equal((await send(purgeInput(nsA), false)).status, 401);
  assert.equal((await send(purgeInput(nsA), true, '1000000000')).status, 401);
  assert.equal((await send({ ...purgeInput(nsA), extra: 'not-accepted' })).status, 400);
  assert.equal((await send(purgeInput('../escape'))).status, 400);
  assert.equal((await send({ ...purgeInput(nsA), extra: 'x'.repeat(1100) })).status, 413);
  assert.equal((await f.worker.command(input(nsA, 'status'))).status, 'stopped');
  const accepted = await send(purgeInput(nsA)); assert.equal(accepted.status, 200);
  assert.deepEqual(await accepted.json(), { ...purgeInput(nsA), status: 'purged' });
});
