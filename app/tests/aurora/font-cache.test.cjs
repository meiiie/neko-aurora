const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const source = fs.readFileSync(path.resolve(__dirname, '../../native/internal/web/assets/app/sw.js'), 'utf8');

function boot() {
  const handlers = {}, precache = [], cached = new Map();
  let offline = false, status = 200;
  const context = {
    URL,
    self: { location: { origin: 'http://aurora.test' }, addEventListener: (name, fn) => { handlers[name] = fn; }, skipWaiting() {} },
    caches: {
      open: async () => ({ addAll: async paths => precache.push(...paths), put: async (request, response) => cached.set(request.url, response) }),
      match: async request => cached.get(request.url),
    },
    fetch: async () => { if (offline) throw Error('offline'); return { ok: status === 200, status, clone() { return this; } }; },
  };
  vm.runInNewContext(source, context);
  const request = (url, method = 'GET') => { let result; handlers.fetch({ request: { url, method }, respondWith: promise => { result = promise; } }); return result; };
  return { request, precache, cached, handlers, offline: value => { offline = value; }, status: value => { status = value; } };
}

test('CJK faces load on demand and a previously used subset remains available offline', async () => {
  const b = boot(); let installation;
  b.handlers.install({ waitUntil: promise => { installation = promise; } }); await installation;
  assert(b.precache.includes('/app/aurora/fonts/noto-sans-tc/index.css'));
  assert(!b.precache.some(path => path.includes('/noto-sans-') && path.endsWith('.woff2')));
  const url = 'http://aurora.test/app/aurora/fonts/noto-sans-tc/noto-sans-tc-122-wght-normal.woff2';
  const online = await b.request(url); assert.equal(online.status, 200);
  b.offline(true); assert.equal(await b.request(url), online);
});

test('font caching ignores API, cross-origin and non-GET requests', () => {
  const b = boot();
  for (const url of ['http://aurora.test/app/api/state', 'https://example.test/app/aurora/fonts/noto-sans-sc/noto-sans-sc-4-wght-normal.woff2']) assert.equal(b.request(url), undefined);
  assert.equal(b.request('http://aurora.test/app/aurora/fonts/noto-sans-sc/noto-sans-sc-4-wght-normal.woff2', 'POST'), undefined);
});

test('an error response cannot replace a usable cached font', async () => {
  const b = boot(), url = 'http://aurora.test/app/aurora/fonts/noto-sans-sc/noto-sans-sc-4-wght-normal.woff2';
  const good = await b.request(url);
  b.status(404); assert.equal((await b.request(url)).status, 404);
  b.offline(true); assert.equal(await b.request(url), good);
});
