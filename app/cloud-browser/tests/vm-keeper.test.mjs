import test from 'node:test';
import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { createKeeper } from '../vm-keeper.mjs';

const token = 'fixture-keeper-token-never-a-real-credential';
const environmentId = '11111111-1111-4111-8111-111111111111';
const sandboxId = '22222222-2222-4222-8222-222222222222';
const domain = 'fixture-browser-vm.railway.internal';
const response = (fields = {}) => new Response(JSON.stringify({ data: { sandboxHeartbeat: { id: sandboxId, status: 'RUNNING', privateDomain: domain, ...fields } } }));
function clock() {
  let now = 1800000000000, next = 0;
  const timers = new Map();
  const add = (fn, delay, interval = 0) => { const id = ++next; timers.set(id, { fn, at: now + delay, interval }); return id; };
  return {
    now: () => now,
    setTimeout: (fn, delay) => add(fn, delay), clearTimeout: id => timers.delete(id),
    setInterval: (fn, delay) => add(fn, delay, delay), clearInterval: id => timers.delete(id),
    advance(ms) {
      const until = now + ms;
      while (true) {
        const due = [...timers].filter(([, timer]) => timer.at <= until).sort((a, b) => a[1].at - b[1].at)[0];
        if (!due) break;
        const [id, timer] = due; now = timer.at;
        if (timer.interval) timer.at += timer.interval; else timers.delete(id);
        timer.fn();
      }
      now = until;
    },
  };
}
const options = (extra = {}) => ({ token, environmentId, sandboxId, clock: clock(), fetch: async () => response(), ...extra });
const flush = () => new Promise(resolve => setImmediate(resolve));

test('keeper heartbeats only its fixed VM with Bearer auth and exposes no VM details in health', async () => {
  let called;
  const keeper = createKeeper(options({ fetch: async (url, request) => { called = { url, request }; return response(); } }));
  assert.deepEqual(keeper.health(), { ready: false });
  assert.equal(await keeper.heartbeat(), true);
  assert.equal(called.url, 'https://backboard.railway.com/graphql/v2');
  assert.equal(called.request.method, 'POST');
  assert.equal(called.request.headers.Authorization, `Bearer ${token}`);
  assert.equal(called.request.redirect, 'error');
  const body = JSON.parse(called.request.body);
  assert.deepEqual(body.variables, { environmentId, id: sandboxId });
  assert.equal(body.query, 'mutation($environmentId:String!,$id:String!){sandboxHeartbeat(environmentId:$environmentId,id:$id){id status privateDomain}}');
  assert.deepEqual(keeper.health(), { ready: true });
  keeper.stop(); assert.deepEqual(keeper.health(), { ready: false });
});

test('keeper rejects missing or invalid fixed configuration before making a request', () => {
  for (const extra of [{ token: '' }, { token: 'bad\nheader' }, { environmentId: 'not-an-environment' }, { sandboxId: '../other-vm' }])
    assert.throws(() => createKeeper(options(extra)), { message: 'keeper_configuration_invalid' });
});

test('mismatched identities, non-running VMs and invalid private domains never become ready', async () => {
  const logs = [], previous = console.error; console.error = (...args) => logs.push(args);
  try {
    for (const fields of [{ id: '33333333-3333-4333-8333-333333333333' }, { status: 'CREATING' }, { privateDomain: null },
      { privateDomain: 'railway.internal' }, { privateDomain: 'vm.railway.internal.evil.test' }, { privateDomain: 'http://vm.railway.internal' },
      { privateDomain: 'vm.railway.internal:8080' }, { privateDomain: 'vm._bad.railway.internal' }]) {
      const keeper = createKeeper(options({ fetch: async () => response(fields) }));
      assert.equal(await keeper.heartbeat(), false);
      assert.deepEqual(keeper.health(), { ready: false }); keeper.stop();
    }
    assert.ok(logs.length > 0);
    assert.ok(logs.every(args => args.length === 1 && args[0] === 'aurora_vm_keeper_heartbeat_failed'));
  } finally { console.error = previous; }
});

test('HTTP, GraphQL, malformed and oversized failures are unhealthy and log fixed sanitized text only', async () => {
  const logs = [], previous = console.error; console.error = (...args) => logs.push(args);
  try {
    for (const failure of [async () => { throw Error(token); }, async () => new Response(token, { status: 401 }),
      async () => new Response(JSON.stringify({ errors: [{ message: token }] })), async () => new Response('not JSON'),
      async () => new Response('x'.repeat(65537)), async () => new Response(JSON.stringify({ data: { sandboxHeartbeat: null } }))]) {
      const keeper = createKeeper(options({ fetch: failure }));
      assert.equal(await keeper.heartbeat(), false);
      assert.deepEqual(keeper.health(), { ready: false }); keeper.stop();
    }
    assert.equal(logs.length, 6);
    assert.ok(logs.every(args => args.length === 1 && args[0] === 'aurora_vm_keeper_heartbeat_failed'));
  } finally { console.error = previous; }
});

test('successful heartbeat becomes stale after 150 seconds and any later failure immediately clears readiness', async () => {
  const timers = clock(); let fail = false;
  const previous = console.error; console.error = () => {};
  try {
    const keeper = createKeeper(options({ clock: timers, fetch: async () => fail ? new Response('', { status: 503 }) : response() }));
    await keeper.heartbeat(); timers.advance(150000); assert.deepEqual(keeper.health(), { ready: true });
    timers.advance(1); assert.deepEqual(keeper.health(), { ready: false });
    await keeper.heartbeat(); assert.deepEqual(keeper.health(), { ready: true });
    fail = true; await keeper.heartbeat(); assert.deepEqual(keeper.health(), { ready: false }); keeper.stop();
  } finally { console.error = previous; }
});

test('timeout aborts at ten seconds and a still-pending request never overlaps another heartbeat', async () => {
  const timers = clock(); let calls = 0, resolveRequest, signal;
  const previous = console.error; console.error = () => {};
  try {
    const keeper = createKeeper(options({ clock: timers, fetch: (url, request) => {
      calls++; signal = request.signal; return new Promise(resolve => { resolveRequest = resolve; });
    } }));
    const first = keeper.start(); let bounded = false; first.then(() => { bounded = true; }); await flush();
    timers.advance(9999); assert.equal(signal.aborted, false);
    timers.advance(1); await flush(); assert.equal(signal.aborted, true); assert.deepEqual(keeper.health(), { ready: false });
    assert.equal(bounded, true);
    timers.advance(120000); await flush(); assert.equal(calls, 1);
    assert.equal(await keeper.heartbeat(), false);
    resolveRequest(response()); assert.equal(await first, false);
    assert.deepEqual(keeper.health(), { ready: false });
    keeper.stop(); timers.advance(120000); await flush(); assert.equal(calls, 1);
  } finally { console.error = previous; }
});

test('shutdown cancels its timer and active heartbeat without accepting a late successful response', async () => {
  const timers = clock(); let signal, resolveRequest, calls = 0;
  const keeper = createKeeper(options({ clock: timers, fetch: (url, request) => {
    calls++; signal = request.signal; return new Promise(resolve => { resolveRequest = resolve; });
  } }));
  const pending = keeper.start(); let settled = false; pending.then(() => { settled = true; }); await flush(); keeper.stop(); assert.equal(signal.aborted, true);
  await flush(); assert.equal(settled, true);
  resolveRequest(response()); assert.equal(await pending, false);
  timers.advance(600000); await flush(); assert.equal(calls, 1);
  assert.deepEqual(keeper.health(), { ready: false });
});

test('scheduled heartbeats run every sixty seconds and stop prevents a not-yet-dispatched request', async () => {
  const timers = clock(); let calls = 0;
  const keeper = createKeeper(options({ clock: timers, fetch: async () => { calls++; return response(); } }));
  assert.equal(await keeper.start(), true);
  timers.advance(59999); await flush(); assert.equal(calls, 1);
  timers.advance(1); await flush(); assert.equal(calls, 2);
  keeper.stop(); timers.advance(600000); await flush(); assert.equal(calls, 2);
  const pending = createKeeper(options({ fetch: async () => { calls++; return response(); } }));
  const started = pending.start(); pending.stop();
  assert.equal(await started, false);
  assert.equal(calls, 2);
});

test('HTTP surface exposes only unauthenticated minimal health and no control or proxy routes', async t => {
  const keeper = createKeeper(options());
  assert.equal(typeof keeper.handle, 'function');
  const server = createServer(keeper.handle); await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  t.after(async () => { keeper.stop(); await new Promise(resolve => server.close(resolve)); });
  const url = `http://127.0.0.1:${server.address().port}`;
  const unhealthy = await fetch(`${url}/healthz`); assert.equal(unhealthy.status, 503); assert.deepEqual(await unhealthy.json(), { ready: false });
  await keeper.heartbeat(); const healthy = await fetch(`${url}/healthz`); assert.equal(healthy.status, 200); assert.deepEqual(await healthy.json(), { ready: true });
  for (const path of ['/v1/command', '/v1/purge', '/proxy', '/healthz?vm=other']) assert.equal((await fetch(`${url}${path}`)).status, 404);
  assert.equal((await fetch(`${url}/healthz`, { method: 'POST', body: '{}' })).status, 404);
});
