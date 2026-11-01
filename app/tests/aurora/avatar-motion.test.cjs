const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const path = require('node:path');
const source = fs.readFileSync(path.resolve(__dirname, '../../native/internal/web/assets/app/aurora/avatar-motion.js'), 'utf8');

function boot({ stored = {}, reduce = false, hidden = false, failStorage = false } = {}) {
  const events = {}, documentEvents = {}, observed = new Set();
  let intersect, mutate, mediaChange;
  const media = { matches: reduce, addEventListener: (_, fn) => { mediaChange = fn; } };
  const context = {
    Date, Event, console,
    document: { hidden, documentElement: {}, addEventListener: (type, fn) => { documentEvents[type] = fn; } },
    localStorage: { getItem: key => stored[key], setItem: (key, value) => { if (failStorage) throw Error('unavailable'); stored[key] = value; } },
    matchMedia: () => media,
    IntersectionObserver: class {
      constructor(fn) { intersect = fn; }
      observe(node) { observed.add(node); }
      unobserve(node) { observed.delete(node); }
    },
    MutationObserver: class { constructor(fn) { mutate = fn; } observe() {} },
    addEventListener: (type, fn) => { events[type] = fn; },
    dispatchEvent() {},
  };
  context.window = context;
  vm.runInNewContext(source, context);
  const node = () => ({ dataset: {}, properties: {}, isConnected: true, style: { setProperty() {} } });
  const visible = (target, value) => intersect([{ target, isIntersecting: value }]);
  return { api: context.AURORA_AVATARS, context, node, visible, observed, mutate, media, mediaChange: () => mediaChange(), events, documentEvents, stored };
}

test('motion only runs when visible and returns to rest offscreen or with a hidden document', () => {
  const b = boot(), node = b.node();
  b.api.attach(node, 'bot-1', 'working');
  assert.equal(node.dataset.motion, 'off');
  b.visible(node, true); assert.equal(node.dataset.motion, 'on');
  b.context.document.hidden = true; b.documentEvents.visibilitychange(); assert.equal(node.dataset.motion, 'off');
  b.context.document.hidden = false; b.documentEvents.visibilitychange(); assert.equal(node.dataset.motion, 'on');
  b.visible(node, false); assert.equal(node.dataset.motion, 'off');
});

test('reduced motion takes priority over automatic animation, including live OS preference changes', () => {
  const b = boot({ reduce: true }), node = b.node();
  b.api.attach(node, 'bot-1', 'working'); b.visible(node, true); assert.equal(node.dataset.motion, 'off');
  b.api.setMode('auto'); assert.equal(node.dataset.motion, 'off');
  b.media.matches = false; b.mediaChange(); assert.equal(node.dataset.motion, 'on');
  b.media.matches = true; b.mediaChange(); assert.equal(node.dataset.motion, 'off');
});

test('off preference survives reload, syncs across windows, and works when storage fails', () => {
  const b = boot(), node = b.node(); b.api.attach(node, 'bot-1', 'idle'); b.visible(node, true);
  assert.equal(b.api.setMode('off'), true); assert.equal(node.dataset.motion, 'off');
  assert.equal(boot({ stored: b.stored }).api.mode(), 'off');
  b.stored['aurora.avatar-motion.v1'] = 'auto'; b.events.storage({ key: 'aurora.avatar-motion.v1' });
  assert.equal(node.dataset.motion, 'on');
  b.api.setMode('off'); delete b.stored['aurora.avatar-motion.v1']; b.events.storage({ key: null });
  assert.equal(node.dataset.motion, 'on');
  assert.equal(b.api.setMode('always'), false); assert.equal(b.api.mode(), 'auto');
  const blocked = boot({ failStorage: true }); assert.equal(blocked.api.setMode('off'), false); assert.equal(blocked.api.mode(), 'off');
});

test('replaced avatars release their observer and stale intersection callbacks cannot revive them', () => {
  const b = boot(), old = b.node(), next = b.node();
  b.api.attach(old, 'bot-1', 'working'); b.visible(old, true);
  old.isConnected = false; b.api.attach(next, 'bot-1', 'idle'); b.mutate();
  assert.equal(b.observed.has(old), false); assert.equal(b.observed.has(next), true);
  b.visible(old, false); b.visible(next, true); b.api.setMode('off');
  assert.equal(next.dataset.motion, 'off');
  assert.equal(b.observed.size, 1);
});

test('avatar state follows supplied status and never upgrades unknown status to working', () => {
  const b = boot();
  for (const state of ['working', 'waiting', 'error', 'idle', undefined, 'unknown']) {
    const node = b.node(); b.api.attach(node, 'bot-1', state);
    assert.equal(node.dataset.avatarState, ['working', 'waiting', 'error'].includes(state) ? state : 'idle');
  }
});
