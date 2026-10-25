import test from 'node:test';
import assert from 'node:assert/strict';
import { createPlaywrightDriver } from '../workstation.mjs';
import { createWorker } from '../server.mjs';
import { mkdtemp, rm } from 'node:fs/promises';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { loginFieldPins } from '../login-fields.mjs';

function snapshotOf(handles) {
  return { async getProperties() { return new Map(handles.map((handle, i) => [String(i), { asElement: () => handle }])); }, async dispose() {} };
}

test('navigation during observation rejects the stale read without waiting for removed controls or closing the browser', async () => {
  let navigation, changed = false, closes = 0;
  const frame = {};
  const handle = {
    async evaluate(fn, prior) {
      if (!changed) { changed = true; navigation(frame); }
      if (Array.isArray(prior)) return undefined;
      return { role: 'button', name: 'Current page', type: '', protected: false };
    }, async dispose() {},
  };
  const page = {
    on(event, callback) { if (event === 'framenavigated') navigation = callback; }, mainFrame: () => frame,
    async goto() {}, url: () => 'https://chat.zalo.me/', async title() { return 'Current page'; }, async evaluate() { return 'Current page'; },
    async evaluateHandle() { return snapshotOf([handle]); },
    locator: () => ({ count: async () => 2, nth: i => ({ elementHandle: () => i === 0 ? Promise.resolve(handle) : new Promise(() => {}) }) }),
  };
  const context = { setDefaultTimeout() {}, setDefaultNavigationTimeout() {}, async clearPermissions() {}, async route() {}, async routeWebSocket() {},
    pages: () => [page], on() {}, async close() { closes++; } };
  const browser = await createPlaywrightDriver({ captureTimeoutMs: 25,
    chromium: { async launchPersistentContext() { return context; } } }).open({ directory: '/fixture/profile', url: 'https://chat.zalo.me/' });
  await assert.rejects(browser.observe(), { code: 'stale_observation' });
  assert.equal(closes, 0);
  assert.equal((await browser.observe()).text, 'Current page');
  await browser.close();
});

test('a navigation can destroy the read context before the new-document event arrives', async () => {
  let reads = 0, closes = 0;
  const page = {
    on() {}, async goto() {}, url: () => 'https://chat.zalo.me/', async title() { return 'Current page'; }, async evaluate() { return 'Current page'; },
    async evaluateHandle() {
      if (reads++ === 0) throw Error('Execution context was destroyed, most likely because of a navigation');
      return snapshotOf([]);
    },
  };
  const context = { setDefaultTimeout() {}, setDefaultNavigationTimeout() {}, async clearPermissions() {}, async route() {}, async routeWebSocket() {},
    pages: () => [page], on() {}, async close() { closes++; } };
  const browser = await createPlaywrightDriver({ chromium: { async launchPersistentContext() { return context; } } })
    .open({ directory: '/fixture/profile', url: 'https://chat.zalo.me/' });
  await assert.rejects(browser.observe(), { code: 'stale_observation' });
  assert.equal(closes, 0);
  assert.equal((await browser.observe()).text, 'Current page');
  await browser.close();
});

test('Chromium launch receives only browser environment and restricted transport configuration', async () => {
  let launch;
  const hooks = {};
  const page = { on() {}, async goto(url) { assert.equal(url, 'https://chat.zalo.me/'); } };
  const context = {
    setDefaultTimeout() {}, setDefaultNavigationTimeout() {}, async clearPermissions() {},
    async route(pattern, fn) { hooks.request = fn; }, async routeWebSocket(pattern, fn) { hooks.socket = fn; },
    pages() { return [page]; }, on() {}, async close() {},
  };
  const browser = await createPlaywrightDriver({ proxyURL: 'http://127.0.0.1:1234',
    chromium: { async launchPersistentContext(directory, options) { launch = { directory, options }; return context; } },
  }).open({ directory: '/fixture/profile', url: 'https://chat.zalo.me/' });
  assert.equal(launch.directory, '/fixture/profile');
  assert.deepEqual(Object.keys(launch.options.env).sort(), ['HOME', 'LANG', 'PATH']);
  assert.equal(launch.options.env.HOME, '/fixture/profile');
  assert.equal(launch.options.serviceWorkers, 'block');
  assert.equal(launch.options.acceptDownloads, false);
  assert.equal(launch.options.chromiumSandbox, true);
  assert.ok(launch.options.args.includes('--disable-quic'));
  assert.ok(launch.options.args.includes('--force-webrtc-ip-handling-policy=disable_non_proxied_udp'));
  assert.deepEqual(launch.options.proxy, { server: 'http://127.0.0.1:1234', bypass: '<-loopback>' });
  let blocked = 0, continued = 0;
  await hooks.request({ request: () => ({ url: () => 'https://169.254.169.254/latest/meta-data' }), abort() { blocked++; }, continue() { continued++; } });
  await hooks.request({ request: () => ({ url: () => 'https://chat.zalo.me/assets/app.js' }), abort() { blocked++; }, continue() { continued++; } });
  hooks.socket({ url: () => 'wss://evil.internal/socket', close() { blocked++; }, connectToServer() { continued++; } });
  assert.equal(blocked, 2); assert.equal(continued, 1);
  await browser.close();
});

test('stalled observation closes the context before owner handback and rejects all later input', async t => {
  let closeCount = 0, captures = 0, rejectCapture, effects = 0;
  const page = {
    on() {}, async goto() {}, url() { return 'https://chat.zalo.me/'; }, async title() { return 'Fixture'; },
    async evaluate() { return 'Fixture'; },
    evaluateHandle() {
      captures++;
      if (captures === 1) return Promise.resolve(snapshotOf([]));
      return new Promise((resolve, reject) => { rejectCapture = reject; });
    },
    keyboard: { async press() { effects++; } },
  };
  const context = {
    setDefaultTimeout() {}, setDefaultNavigationTimeout() {}, async clearPermissions() {},
    async route() {}, async routeWebSocket() {}, pages() { return [page]; }, on() {},
    async close() { closeCount++; rejectCapture?.(Error('context closed')); },
  };
  const driver = createPlaywrightDriver({ captureTimeoutMs: 20,
    chromium: { async launchPersistentContext() { return context; } } });
  const dataDir = await mkdtemp(join(tmpdir(), 'aurora-stalled-capture-'));
  const worker = await createWorker({ key: 'fixture-only-stall-key-never-a-credential-123', dataDir, driver });
  t.after(async () => { await worker.close(); await rm(dataDir, { recursive: true, force: true }); });
  const base = { namespace: 'd'.repeat(64), profile: 'bot-stalled', actor: 'owner', actorId: 'fixture' };
  let state = await worker.command({ ...base, kind: 'open', nonce: 'stall-open-001' });
  state = await worker.command({ ...base, kind: 'control', mode: 'owner', epoch: state.epoch, nonce: 'stall-owner-001' });
  const stalled = worker.command({ ...base, kind: 'observe' });
  const handback = worker.command({ ...base, kind: 'control', mode: 'agent', epoch: state.epoch, nonce: 'stall-handback-001' });
  const timeoutGuard = setTimeout(() => rejectCapture?.(Error('test harness guard: missing deadline')), 300);
  await assert.rejects(stalled, { code: 'browser_operation_timeout' }); clearTimeout(timeoutGuard);
  assert.ok(closeCount >= 1);
  assert.equal((await handback).receipt.status, 'rejected');
  const after = await worker.command({ ...base, kind: 'status' });
  assert.equal(after.status, 'recovery_required');
  assert.equal(after.control, 'paused');
  assert.equal(effects, 0);
});

test('slow multi-target capture has one overall budget rather than one timeout per element', async () => {
  let closed = false, effects = 0;
  const handle = {
    async evaluate() { await new Promise(resolve => setTimeout(resolve, 15)); if (closed) throw Error('closed'); return { role: 'button', name: 'Fixture', type: '', protected: false }; },
    async dispose() {}, async click() { effects++; },
  };
  const page = {
    on() {}, async goto() {}, url: () => 'https://chat.zalo.me/', async title() { return 'Fixture'; }, async evaluate() { return 'Fixture'; },
    evaluateHandle: async () => snapshotOf(Array(256).fill(handle)),
    mouse: { async click() { effects++; } },
  };
  const context = { setDefaultTimeout() {}, setDefaultNavigationTimeout() {}, async clearPermissions() {}, async route() {}, async routeWebSocket() {},
    pages: () => [page], on() {}, async close() { closed = true; } };
  const browser = await createPlaywrightDriver({ captureTimeoutMs: 25,
    chromium: { async launchPersistentContext() { return context; } } }).open({ directory: '/fixture/profile', url: 'https://chat.zalo.me/' });
  await assert.rejects(browser.observe(), { code: 'browser_operation_timeout' });
  assert.equal(closed, true);
  await assert.rejects(browser.act({ kind: 'click', x: 1, y: 1 }), { code: 'browser_recovery_required' });
  assert.equal(effects, 0);
});

for (const operation of ['navigate', 'frame', 'setup']) test(`${operation} deadline closes the browser and never returns an active driver`, async () => {
  let closed = false, rejectPending, navigations = 0;
  const pending = () => new Promise((resolve, reject) => { rejectPending = reject; });
  const page = {
    on() {}, locator() { return {}; },
    async goto() { navigations++; if (operation === 'setup' || navigations > 1) return pending(); },
    screenshot: pending,
  };
  const context = { setDefaultTimeout() {}, setDefaultNavigationTimeout() {}, async clearPermissions() {},
    async route() {}, async routeWebSocket() {}, pages: () => [page], on() {},
    async close() { closed = true; rejectPending?.(Error('closed')); } };
  const opening = createPlaywrightDriver({ captureTimeoutMs: 20, operationTimeoutMs: 20,
    chromium: { async launchPersistentContext() { return context; } } }).open({ directory: '/fixture/profile', url: 'https://chat.zalo.me/' });
  if (operation === 'setup') await assert.rejects(opening, { code: 'browser_operation_timeout' });
  else {
    const browser = await opening;
    await assert.rejects(operation === 'frame' ? browser.frame() : browser.act({ kind: 'navigate', url: 'https://chat.zalo.me/next' }),
      { code: 'browser_operation_timeout' });
    await assert.rejects(browser.act({ kind: 'scroll', deltaY: 1 }), { code: 'browser_recovery_required' });
  }
  assert.equal(closed, true);
});

test('Vault fill never reselects a replacement login field after the approved capture', async () => {
  const effects = [];
  let lists = 0;
  const handle = {
    connected: true,
    async evaluate(fn, prior) {
      if (Array.isArray(prior)) return prior.find(entry => entry.node === this)?.key;
      return this.connected ? { role: 'textbox', name: 'Password', type: 'password', protected: true } : null;
    },
    async dispose() {}, async fill(value) { effects.push(value); },
  };
  const page = {
    on() {}, async goto() {}, url: () => 'https://chat.zalo.me/', async title() { return 'Fixture'; }, async evaluate() { return 'Fixture'; },
    evaluateHandle() { lists++; return Promise.resolve(snapshotOf([handle])); },
  };
  const context = { setDefaultTimeout() {}, setDefaultNavigationTimeout() {}, async clearPermissions() {}, async route() {}, async routeWebSocket() {},
    pages: () => [page], on() {}, async close() {} };
  const browser = await createPlaywrightDriver({ chromium: { async launchPersistentContext() { return context; } } })
    .open({ directory: '/fixture/profile', url: 'https://chat.zalo.me/' });
  const approved = await browser.observe();
  handle.connected = false;
  await assert.rejects(browser.act({ kind: 'vault_fill', values: { password: 'fixture-not-injected' },
    fieldPins: loginFieldPins(approved.targets), expectedOrigin: approved.origin, expectedDocument: approved.documentId }), { code: 'stale_target' });
  assert.deepEqual(effects, []);
  assert.equal(lists, 1);
  await browser.close();
});

for (const [description, sensitive] of [
  ['CANVAS id=loginQrCode class=scan-qr', true],
  ['IMG alt=Verification challenge src=/captcha-image.png', true],
  ['IMG alt=Profile photograph src=/avatars/person.png', false],
]) test(`visual privacy evidence classifies ${description}`, async () => {
  const page = { on() {}, async goto() {}, url: () => 'https://chat.zalo.me/', async title() { return 'Application'; },
    async evaluateHandle() { return snapshotOf([]); },
    async evaluate(fn) { return fn.name === 'visualDescriptors' ? [description] : 'Application ready'; } };
  const context = { setDefaultTimeout() {}, setDefaultNavigationTimeout() {}, async clearPermissions() {}, async route() {}, async routeWebSocket() {},
    pages: () => [page], on() {}, async close() {} };
  const browser = await createPlaywrightDriver({ chromium: { async launchPersistentContext() { return context; } } })
    .open({ directory: '/fixture/profile', url: 'https://chat.zalo.me/' });
  const view = await browser.observe();
  assert.equal(view.loginEvidence, sensitive);
  assert.equal(view.identity, null);
  await browser.close();
});

for (const qrOnly of [false, true]) test(`blank capture is unusable while QR evidence remains independent (${qrOnly})`, async () => {
  const page = { on() {}, async goto() {}, url: () => 'https://chat.zalo.me/', async title() { return 'Application'; },
    async evaluateHandle() { return snapshotOf([]); },
    async evaluate(fn) { return fn.name === 'visualDescriptors' ? qrOnly ? ['CANVAS id=qrcode-login'] : [] : '   '; } };
  const context = { setDefaultTimeout() {}, setDefaultNavigationTimeout() {}, async clearPermissions() {}, async route() {}, async routeWebSocket() {},
    pages: () => [page], on() {}, async close() {} };
  const browser = await createPlaywrightDriver({ chromium: { async launchPersistentContext() { return context; } } })
    .open({ directory: '/fixture/profile', url: 'https://chat.zalo.me/' });
  const view = await browser.observe();
  assert.equal(view.usable, false);
  assert.equal(view.loginEvidence, qrOnly);
  assert.equal(view.identity, null);
  await browser.close();
});
