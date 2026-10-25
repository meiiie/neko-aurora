import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, rm } from 'node:fs/promises';
import { createCipheriv, createHmac, randomBytes } from 'node:crypto';
import { openJournal } from '../journal.mjs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { createWorker } from '../server.mjs';

const key = 'fixture-only-cloud-key-not-a-credential-123';
const namespace = 'a'.repeat(64);
const base = { namespace, profile: 'bot-fixture', actor: 'owner', actorId: 'fixture-owner' };
const command = (kind, fields = {}) => ({ ...base, kind, ...fields });
const drivers = [];
function fakeDriver() {
  const page = {
    effects: [], closed: false, documentId: 'document-1', origin: 'https://chat.zalo.me',
    protectedLogin: false, echo: '', failNext: false, actionGate: null,
    async observe() { return {
      documentId: this.documentId, origin: this.origin, title: 'Fixture',
      text: this.unusable ? '' : `Fixture page ${this.echo}`, loginEvidence: Boolean(this.protectedLogin || this.protectedQR), usable: !this.unusable,
      targets: this.unusable ? [] : [{ key: 'button-1', role: 'button', name: 'Send', type: '', protected: false },
        { key: 'input-1', role: 'textbox', name: 'Email', type: 'email', protected: false },
        { key: this.replacedPassword ? 'password-2' : 'password-1', role: 'textbox', name: 'Password', type: 'password', protected: true }],
    }; },
    async frame() { return { mime: 'image/png', data: Buffer.from('fixture-png').toString('base64'), width: 800, height: 600 }; },
    async act(input) {
      this.effects.push(input);
      if (this.actionGate) await this.actionGate;
      if (this.failNext) { this.failNext = false; throw new Error('fixture response lost'); }
      if (input.kind === 'vault_fill') this.echo = Object.values(input.values).join(' ');
      if (input.kind === 'navigate') { this.origin = new URL(input.url).origin; this.documentId += '-next'; }
    },
    async close() { this.closed = true; },
  };
  drivers.push(page);
  return page;
}
async function setup(t) {
  const dataDir = await mkdtemp(join(tmpdir(), 'aurora-cloud-test-'));
  let current;
  const options = { key, dataDir, driver: { open: async () => fakeDriver() } };
  const worker = await createWorker(options); current = worker;
  t.after(async () => { await current.close(); await rm(dataDir, { recursive: true, force: true }); });
  return { worker, options, async restart() { await current.close(); current = await createWorker(options); return current; } };
}
async function ready(worker, actor = 'owner') {
  const opened = await worker.command(command('open', { nonce: 'open-once-001' }));
  assert.equal(opened.status, 'ready');
  let epoch = opened.epoch;
  if (actor === 'agent') {
    const owner = await worker.command(command('control', { mode: 'owner', epoch, nonce: 'control-owner-before-agent-001' }));
    epoch = owner.epoch;
  }
  const controlled = await worker.command(command('control', { mode: actor, epoch, nonce: 'control-once-001' }));
  const view = await worker.command(command('observe', { actor, epoch: controlled.epoch,
    ...(actor === 'agent' ? { workId: 'work-fixture', workEpoch: 1, attempt: 'fixture-attempt-1' } : {}) }));
  return { epoch: controlled.epoch, view };
}

test('opening creates an isolated paused cloud profile and no local browser fallback', async t => {
  const { worker } = await setup(t);
  const out = await worker.command(command('open', { nonce: 'open-once-001' }));
  assert.equal(out.status, 'ready');
  assert.equal(out.control, 'paused');
  assert.equal(out.authState, 'unknown');
  assert.equal(typeof out.bootId, 'string');
});

test('a duplicate action and a server restart never dispatch a second effect', async t => {
  const fixture = await setup(t);
  const { epoch, view } = await ready(fixture.worker);
  const page = drivers.at(-1);
  const input = command('click', { epoch, observationId: view.observationId, targetId: view.targets[0].id, nonce: 'effect-once-001' });
  const first = await fixture.worker.command(input);
  assert.equal(first.receipt.status, 'dispatched');
  assert.equal((await fixture.worker.command(input)).receipt.status, 'dispatched');
  assert.equal(page.effects.length, 1);
  await assert.rejects(fixture.worker.command({ ...input, targetId: 'changed' }), { code: 'nonce_conflict' });
  const restarted = await fixture.restart();
  assert.equal((await restarted.command(input)).receipt.status, 'dispatched');
  assert.equal(page.effects.length, 1);
  const state = await restarted.command(command('status'));
  assert.equal(state.control, 'paused');
  assert.ok(state.epoch > epoch);
});

test('lost input response remains unknown across restart and never replays', async t => {
  const fixture = await setup(t);
  const { epoch, view } = await ready(fixture.worker);
  const page = drivers.at(-1); page.failNext = true;
  const input = command('click', { epoch, observationId: view.observationId, targetId: view.targets[0].id, nonce: 'effect-lost-001' });
  assert.equal((await fixture.worker.command(input)).receipt.status, 'unknown');
  assert.equal((await fixture.worker.command(input)).receipt.status, 'unknown');
  const restarted = await fixture.restart();
  assert.equal((await restarted.command(input)).receipt.status, 'unknown');
  assert.equal(page.effects.length, 1);
});

test('stale documents and epochs cannot authorize input', async t => {
  const { worker } = await setup(t);
  const { epoch, view } = await ready(worker);
  const page = drivers.at(-1); page.documentId = 'other-document';
  const out = await worker.command(command('click', { epoch, observationId: view.observationId, targetId: view.targets[0].id, nonce: 'stale-once-001' }));
  assert.equal(out.receipt.status, 'rejected');
  assert.equal(out.receipt.code, 'stale_observation');
  assert.equal(page.effects.length, 0);
});

test('owner takeover fences queued agent input before acknowledging control', async t => {
  const { worker } = await setup(t);
  const { epoch, view } = await ready(worker, 'agent');
  const page = drivers.at(-1);
  let release; page.actionGate = new Promise(resolve => { release = resolve; });
  const input = { actor: 'agent', workId: 'work-1', workEpoch: 1, attempt: 'fixture-attempt-1', epoch, observationId: view.observationId, targetId: view.targets[0].id };
  const running = worker.command(command('click', { ...input, nonce: 'running-agent-001' }));
  await new Promise(resolve => setImmediate(resolve));
  const queued = worker.command(command('click', { ...input, nonce: 'queued-agent-001' }));
  const takeover = worker.command(command('control', { mode: 'owner', epoch, nonce: 'takeover-owner-001' }));
  release(); await running;
  assert.equal((await queued).receipt.status, 'rejected');
  const owner = await takeover;
  assert.equal(owner.control, 'owner');
  assert.equal(page.effects.length, 1);
});

test('protected login frames and contents are available only to the owner', async t => {
  const { worker } = await setup(t);
  await ready(worker);
  const page = drivers.at(-1); page.protectedLogin = true; page.echo = 'QR password 123456';
  const owner = await worker.command(command('observe'));
  assert.equal(owner.authState, 'login_required');
  assert.equal((await worker.command(command('frame'))).frame.width, 800);
  const model = await worker.command(command('observe', { actor: 'agent', workId: 'work-1', workEpoch: 1, attempt: 'fixture-attempt-1' }));
  assert.equal(model.text, 'Protected login or challenge. Owner control required.');
  assert.deepEqual(model.targets, []);
  assert.equal(model.frame, undefined);
  await assert.rejects(worker.command(command('frame', { actor: 'agent' })), { code: 'owner_required' });
});

test('only one active browser is allowed and profile identity survives close/reopen', async t => {
  const { worker } = await setup(t);
  const first = await worker.command(command('open', { nonce: 'open-once-001' }));
  await assert.rejects(worker.command(command('open', { profile: 'bot-second', nonce: 'open-second-001' })), { code: 'active_profile_limit' });
  await worker.command(command('close', { epoch: first.epoch, nonce: 'close-first-001' }));
  const next = await worker.command(command('open', { nonce: 'reopen-first-001' }));
  assert.equal(next.profileId, first.profileId);
  assert.ok(next.epoch > first.epoch);
});

test('unknown command fields and unsafe URLs fail closed', async t => {
  const { worker } = await setup(t);
  await assert.rejects(worker.command(command('status', { cdp: 'Runtime.evaluate' })), { code: 'invalid_command' });
  await assert.rejects(worker.command(command('open', { nonce: 'open-evil-001', url: 'https://127.0.0.1/' })), { code: 'url_not_allowed' });
  await assert.rejects(worker.command(command('status', { namespace: '../escape' })), { code: 'invalid_command' });
});

function sealed(fields, input) {
  const aesKey = createHmac('sha256', key).update(`aurora-cloud-vault-v1\n${input.namespace}\n${input.profile}`).digest();
  const iv = randomBytes(12); const cipher = createCipheriv('aes-256-gcm', aesKey, iv);
  cipher.setAAD(Buffer.from(`${input.namespace}\n${input.profile}\n${input.nonce}\n${input.expectedOrigin}\n${input.observationId}`));
  return Buffer.concat([iv, cipher.update(JSON.stringify(fields)), cipher.final(), cipher.getAuthTag()]).toString('base64');
}

test('sealed Vault fills bind exact origin and observation without exposing or submitting values', async t => {
  const { worker } = await setup(t);
  const { epoch, view } = await ready(worker);
  const page = drivers.at(-1);
  const input = command('vault_fill', { epoch, observationId: view.observationId, expectedOrigin: 'https://chat.zalo.me', nonce: 'vault-fill-once-001' });
  input.sealed = sealed({ username: 'fixture-sensitive-user', password: 'fixture-sensitive-password', totp: '987654' }, input);
  const result = await worker.command(input);
  assert.equal(result.receipt.status, 'dispatched');
  assert.equal(page.effects.length, 1);
  assert.equal(page.effects[0].kind, 'vault_fill');
  assert.equal(page.effects[0].values.password, 'fixture-sensitive-password');
  const observed = await worker.command(command('observe'));
  assert.ok(!JSON.stringify(observed).includes('fixture-sensitive'));
  assert.ok(!JSON.stringify(observed).includes('987654'));
  assert.ok(!JSON.stringify(result).includes('fixture-sensitive'));
  assert.equal((await worker.command(input)).receipt.status, 'dispatched');
  const fresh = await worker.command(command('observe'));
  const wrong = command('vault_fill', { epoch, observationId: fresh.observationId, expectedOrigin: 'https://other.zalo.me', nonce: 'vault-wrong-origin-001' });
  wrong.sealed = sealed({ password: 'never-injected' }, wrong);
  assert.equal((await worker.command(wrong)).receipt.code, 'origin_mismatch');
  assert.equal(page.effects.length, 1);
});

test('owner lease expiration pauses input rather than silently resuming agent', async t => {
  let time = 100000;
  const dataDir = await mkdtemp(join(tmpdir(), 'aurora-cloud-lease-'));
  const worker = await createWorker({ key, dataDir, now: () => time, ownerLeaseMs: 100,
    driver: { open: async () => fakeDriver() } });
  t.after(async () => { await worker.close(); await rm(dataDir, { recursive: true, force: true }); });
  const { epoch, view } = await ready(worker); time += 101;
  const out = await worker.command(command('click', { epoch, observationId: view.observationId, targetId: view.targets[0].id, nonce: 'lease-expired-001' }));
  assert.equal(out.control, 'paused');
  assert.equal(out.receipt.status, 'rejected');
  assert.equal(drivers.at(-1).effects.length, 0);
});

test('a durably started receipt is unknown after journal restart', async t => {
  const dataDir = await mkdtemp(join(tmpdir(), 'aurora-cloud-journal-'));
  t.after(() => rm(dataDir, { recursive: true, force: true }));
  const first = await openJournal(dataDir);
  first.start(namespace, 'bot-fixture', 'interrupted-001', 'fixture-digest');
  await assert.rejects(openJournal(dataDir), { code: 'profile_volume_locked' });
  await first.close();
  const restarted = await openJournal(dataDir);
  assert.equal(restarted.receipt(namespace, 'bot-fixture', 'interrupted-001').status, 'unknown');
  await restarted.close();
});

test('profile scope isolation uses distinct directories and never inherits browser state', async t => {
  const dataDir = await mkdtemp(join(tmpdir(), 'aurora-cloud-isolation-'));
  const directories = [];
  const worker = await createWorker({ key, dataDir, driver: { open: async options => { directories.push(options.directory); return fakeDriver(); } } });
  t.after(async () => { await worker.close(); await rm(dataDir, { recursive: true, force: true }); });
  const first = await worker.command(command('open', { nonce: 'tenant-one-open-001' }));
  await worker.command(command('close', { epoch: first.epoch, nonce: 'tenant-one-close-001' }));
  await worker.command(command('open', { namespace: 'b'.repeat(64), nonce: 'tenant-two-open-001' }));
  assert.notEqual(directories[0], directories[1]);
});

test('a new login challenge after observation blocks every agent input including navigation', async t => {
  const { worker } = await setup(t);
  const { epoch, view } = await ready(worker, 'agent');
  const page = drivers.at(-1); page.protectedLogin = true;
  for (const [i, kind] of ['navigate', 'click', 'press', 'scroll'].entries()) {
    const out = await worker.command(command(kind, { actor: 'agent', workId: 'work-1', workEpoch: 1,
      attempt: 'fixture-attempt-1', epoch, observationId: view.observationId, targetId: view.targets[0].id,
      nonce: `challenge-block-${i}`, ...(kind === 'navigate' ? { url: 'https://chat.zalo.me/' } : {}),
      ...(kind === 'press' ? { key: 'Enter' } : {}) }));
    assert.equal(out.receipt.status, 'rejected', kind);
    assert.equal(out.receipt.code, i === 0 ? 'protected_login' : 'stale_epoch', kind);
  }
  assert.equal(page.effects.length, 0);
});

test('agent status cannot expose protected titles or live observation bindings', async t => {
  const { worker } = await setup(t);
  await ready(worker); drivers.at(-1).protectedLogin = true;
  await worker.command(command('observe'));
  const out = await worker.command(command('status', { actor: 'agent' }));
  assert.equal(out.title, undefined);
  assert.equal(out.observationId, undefined);
});

test('browser chrome shortcut combinations cannot be dispatched', async t => {
  const { worker } = await setup(t);
  for (const key of ['Control+L', 'Control+O', 'Control+J', 'F12', 'Meta+L', 'Alt+F4'])
    await assert.rejects(worker.command(command('press', { key, nonce: 'bad-key-once-001' })), { code: 'invalid_key' });
});

test('known credential echoes remain redacted after worker restart', async t => {
  const fixture = await setup(t);
  const { epoch, view } = await ready(fixture.worker);
  const input = command('vault_fill', { epoch, observationId: view.observationId,
    expectedOrigin: 'https://chat.zalo.me', nonce: 'persist-redaction-001' });
  input.sealed = sealed({ password: 'fixture-persisted-password' }, input);
  await fixture.worker.command(input);
  const restarted = await fixture.restart();
  await restarted.command(command('open', { nonce: 'redaction-reopen-001' }));
  drivers.at(-1).echo = 'fixture-persisted-password';
  const out = await restarted.command(command('observe'));
  assert.ok(!JSON.stringify(out).includes('fixture-persisted-password'));
});

test('agent keyboard input requires an observed target while owner keyboard input can be global', async t => {
  const { worker } = await setup(t);
  const { epoch, view } = await ready(worker, 'agent');
  const out = await worker.command(command('press', { actor: 'agent', workId: 'work-1', workEpoch: 1,
    attempt: 'fixture-attempt-1', epoch, observationId: view.observationId, key: 'Enter', nonce: 'untargeted-agent-key-001' }));
  assert.equal(out.receipt.status, 'rejected');
  assert.equal(out.receipt.code, 'target_required');
  assert.equal(drivers.at(-1).effects.length, 0);
  const state = await worker.command(command('control', { mode: 'owner', epoch, nonce: 'keyboard-owner-001' }));
  const ownerView = await worker.command(command('observe'));
  const owner = await worker.command(command('press', { epoch: state.epoch, observationId: ownerView.observationId,
    key: 'Enter', nonce: 'untargeted-owner-key-001' }));
  assert.equal(owner.receipt.status, 'dispatched');
  assert.equal(drivers.at(-1).effects.length, 1);
});

test('Vault field replacement in the same document rejects before any secret input', async t => {
  const { worker } = await setup(t);
  const { epoch, view } = await ready(worker);
  const page = drivers.at(-1);
  const input = command('vault_fill', { epoch, observationId: view.observationId,
    expectedOrigin: 'https://chat.zalo.me', nonce: 'replaced-vault-fields-001' });
  input.sealed = sealed({ password: 'fixture-never-injected-into-replacement' }, input);
  page.replacedPassword = true;
  const out = await worker.command(input);
  assert.equal(out.receipt.status, 'rejected');
  assert.equal(out.receipt.code, 'stale_target');
  assert.equal(page.effects.length, 0);
  const invalidEnvelope = await worker.command({ ...input, nonce: 'replaced-invalid-envelope-001', sealed: 'QQ==' });
  assert.equal(invalidEnvelope.receipt.code, 'stale_target');
});

const agentFields = { actor: 'agent', workId: 'work-privacy', workEpoch: 1, attempt: 'privacy-attempt-1' };

test('safe unknown-auth page becomes model-visible only after fresh explicit owner handback', async t => {
  const { worker } = await setup(t);
  const { epoch } = await ready(worker);
  const owner = await worker.command(command('observe'));
  assert.equal(owner.protected, false);
  assert.equal(owner.authState, 'unknown');
  const granted = await worker.command(command('control', { mode: 'agent', epoch, nonce: 'privacy-handback-001' }));
  assert.equal(granted.control, 'agent');
  assert.equal(granted.protected, false);
  assert.equal(granted.authState, 'unknown');
  assert.ok(granted.epoch > epoch);
  assert.equal(granted.observationId, undefined);
  const view = await worker.command(command('observe', agentFields));
  assert.match(view.text, /Fixture page/);
  assert.equal(view.title, 'Fixture');
  assert.ok(view.observationId);
  assert.ok(view.targets.length > 0);
  assert.equal(view.frame, undefined);
});

test('owner and paused control never expose agent DOM even when the page is safe', async t => {
  const { worker } = await setup(t);
  const { epoch } = await ready(worker);
  for (const mode of ['owner', 'paused']) {
    if (mode === 'paused') await worker.command(command('control', { mode, epoch, nonce: 'privacy-pause-001' }));
    const view = await worker.command(command('observe', agentFields));
    assert.equal(view.protected, false);
    assert.deepEqual(view.targets, []);
    assert.equal(view.title, undefined);
    assert.equal(view.observationId, undefined);
    assert.equal(view.frame, undefined);
    assert.ok(!view.text.includes('Fixture page'));
  }
});

test('agent handback rejects stopped browsers and a newly protected owner page', async t => {
  const { worker } = await setup(t);
  const stopped = await worker.command(command('status'));
  assert.equal(stopped.protected, true);
  assert.equal((await worker.command(command('control', { mode: 'agent', epoch: stopped.epoch, nonce: 'stopped-handback-001' }))).receipt.code, 'browser_stopped');
  const { epoch } = await ready(worker);
  drivers.at(-1).protectedLogin = true;
  const blocked = await worker.command(command('control', { mode: 'agent', epoch, nonce: 'protected-handback-001' }));
  assert.equal(blocked.receipt.status, 'rejected');
  assert.equal(blocked.receipt.code, 'protected_login');
  assert.equal(blocked.control, 'owner');
  assert.equal(blocked.protected, true);
});

for (const evidence of ['protectedLogin', 'protectedQR']) test(`new ${evidence} fences and pauses an already approved agent mutation`, async t => {
  const { worker } = await setup(t);
  const { epoch, view } = await ready(worker, 'agent');
  const page = drivers.at(-1); page[evidence] = true;
  const blocked = await worker.command(command('click', { ...agentFields, epoch, observationId: view.observationId,
    targetId: view.targets[0].id, nonce: 'dynamic-challenge-input-001' }));
  assert.equal(blocked.receipt.status, 'rejected');
  assert.equal(blocked.control, 'paused');
  assert.equal(blocked.protected, true);
  assert.ok(blocked.epoch > epoch);
  assert.equal(blocked.observationId, undefined);
  assert.equal(page.effects.length, 0);
});

test('agent handback requires current owner control even on a safe page', async t => {
  const { worker } = await setup(t);
  const opened = await worker.command(command('open', { nonce: 'paused-handback-open-001' }));
  const grant = await worker.command(command('control', { mode: 'agent', epoch: opened.epoch, nonce: 'paused-handback-grant-001' }));
  assert.equal(grant.receipt.status, 'rejected');
  assert.equal(grant.receipt.code, 'owner_control_required');
  assert.equal(grant.control, 'paused');
});

test('empty loading page stays authentication-unknown and cannot receive agent control', async t => {
  const { worker } = await setup(t);
  const { epoch } = await ready(worker);
  drivers.at(-1).unusable = true;
  const view = await worker.command(command('observe'));
  assert.equal(view.authState, 'unknown');
  assert.equal(view.protected, true);
  const grant = await worker.command(command('control', { mode: 'agent', epoch, nonce: 'unusable-handback-001' }));
  assert.equal(grant.receipt.status, 'rejected');
  assert.equal(grant.control, 'owner');
  assert.equal(grant.authState, 'unknown');
  assert.equal(grant.protected, true);
});

test('an empty page appearing after approval fences agent input without inventing a login status', async t => {
  const { worker } = await setup(t);
  const { epoch, view } = await ready(worker, 'agent');
  const page = drivers.at(-1); page.unusable = true;
  const out = await worker.command(command('click', { ...agentFields, epoch, observationId: view.observationId,
    targetId: view.targets[0].id, nonce: 'dynamic-unusable-input-001' }));
  assert.equal(out.receipt.status, 'rejected');
  assert.equal(out.authState, 'unknown');
  assert.equal(out.protected, true);
  assert.equal(out.control, 'paused');
  assert.ok(out.epoch > epoch);
  assert.equal(page.effects.length, 0);
});
