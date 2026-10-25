import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
import { readFile, writeFile } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

// Real Go owner API -> real Go Vault -> signed Node worker -> real container Chromium.
// Fixed isolated fixture origin/port prevent accidentally writing test logins to the live app.
const base = 'http://127.0.0.1:8768', site = 'https://127.0.0.1';
const reportPath = resolve(dirname(fileURLToPath(import.meta.url)), '../.build/cloud/owner-api-acceptance.json');
const phase = process.argv[2] || 'prepare';
assert.ok(['prepare', 'check'].includes(phase));
const initial = await fetch(base + '/app/');
const cookie = initial.headers.getSetCookie().map(value => value.split(';')[0]).join('; ');
assert.ok(cookie.includes('neko_local='));
const prefix = '/app/api/agents/bot-davin/cloud';
const checks = [];
async function api(path, body, expected = 200) {
  const response = await fetch(base + path, { method: body === undefined ? 'GET' : 'POST',
    headers: { Cookie: cookie, Origin: base, ...(body === undefined ? {} : { 'Content-Type': 'application/json' }) },
    body: body === undefined ? undefined : JSON.stringify(body), redirect: 'error' });
  const value = await response.json();
  if (response.status !== expected) throw Object.assign(Error(`${path}: ${response.status}, ${value.error || 'unexpected status'}`), { status: response.status });
  return value;
}
const nonce = () => 'owner-acceptance-' + randomUUID();
async function mutation(kind, fields = {}) {
  const request = { ...fields, clientNonce: fields.clientNonce || nonce() };
  const result = await api(prefix + '/' + kind, request);
  assert.equal(result.receipt?.status, 'dispatched', `${kind}: ${result.receipt?.code || 'missing receipt'}`);
  return { result, request };
}
function pass(name) { checks.push(name); console.log('PASS: ' + name); }
async function observeUntil(text) {
  const deadline = Date.now() + 15000;
  while (Date.now() < deadline) {
    try {
      const view = await api(prefix + '/observe', {});
      if (view.text?.includes(text)) return view;
    } catch (error) { if (error.status !== 409) throw error; }
    await new Promise(resolveWait => setTimeout(resolveWait, 120));
  }
  throw Error('Owned fixture page did not show expected readback');
}
let status = await api(prefix);
assert.equal(status.configured, true); assert.equal(status.localDocker, true);
assert.equal((await fetch(base + prefix)).status, 401);
await api(prefix + '/open', { clientNonce: nonce(), localDocker: true }, 400);
pass('Existing owner cookie authorizes the real cloud API; anonymous and request-level mode overrides fail');
if (status.status === 'stopped' || status.status === 'recovery_required') status = (await mutation('open')).result;
assert.equal(status.origin, site, 'This runner must only connect to the owned fixture worker');
status = (await mutation('control', { epoch: status.epoch, mode: 'owner' })).result;
let view = await api(prefix + '/observe', {});
assert.equal(view.origin, site);
if (phase === 'prepare') {
  assert.ok(view.text.includes('Fixture sign in'), 'Use a fresh isolated controller state for Vault acceptance');
  const item = await api('/app/api/vault', { type: 'login', label: 'Owned Docker fixture login', fields: {
    site, username: 'aurora-owner-fixture-user', password: 'aurora-owner-fixture-password-20261025' } });
  assert.ok(item.id);
  await api(prefix + '/vault', { itemId: item.id, epoch: view.epoch, observationId: view.observationId,
    expectedOrigin: site, clientNonce: nonce(), confirmed: false }, 400);
  await mutation('vault', { itemId: item.id, epoch: view.epoch, observationId: view.observationId, expectedOrigin: site, confirmed: true });
  view = await api(prefix + '/observe', {});
  const submit = view.targets.find(target => target.name === 'Sign in to fixture'); assert.ok(submit);
  await mutation('action', { kind: 'click', epoch: view.epoch, observationId: view.observationId, targetId: submit.id });
  view = await observeUntil('Fixture account ready');
  view = await observeUntil('OWNER-UI-INDEXED-DB-20261025');
  assert.ok(view.text.includes('OWNER-UI-LOCAL-STORAGE-20261025'));
  pass('Real Go Vault requires confirmation and fills the exact HTTPS origin; fixture validates actual form credentials');
  const metadata = await api('/app/api/vault');
  assert.ok(!JSON.stringify(metadata).includes('aurora-owner-fixture-password-20261025'));
  assert.ok(!JSON.stringify(view).includes('aurora-owner-fixture-password-20261025'));
  pass('Vault metadata and browser observations contain no fake password');
  const frame = await api(prefix + '/frame', {});
  assert.ok(Buffer.from(frame.frame.data, 'base64').length > 1000);
  await writeFile(resolve(dirname(reportPath), 'owner-ui-fixture.jpg'), Buffer.from(frame.frame.data, 'base64'));
  pass('The real authenticated Go owner route returns the container screenshot');
  const target = view.targets.find(item => item.name === 'Record fixture effect'); assert.ok(target);
  const effect = await mutation('action', { kind: 'click', epoch: view.epoch, observationId: view.observationId, targetId: target.id });
  view = await observeUntil('Fixture effects recorded: 1');
  await api(prefix + '/action', effect.request);
  view = await api(prefix + '/observe', {});
  assert.ok(view.text.includes('Fixture effects recorded: 1'));
  pass('An identical action retry through Aurora creates exactly one actual website effect');
  await writeFile(reportPath, JSON.stringify({ phase, result: 'PASS', checks, botId: 'bot-davin', vaultItem: item.id,
    action: effect.request, origin: site, controlEpoch: view.epoch, timestampUTC: new Date().toISOString(),
    scope: 'Isolated fixture app/state; actual Go owner API, Go Vault, signed worker and container Chromium. No real account.' }, null, 2));
} else {
  const previous = JSON.parse(await readFile(reportPath, 'utf8'));
  view = await observeUntil('Fixture account ready');
  view = await observeUntil('OWNER-UI-INDEXED-DB-20261025');
  assert.ok(view.text.includes('OWNER-UI-LOCAL-STORAGE-20261025'));
  const duplicate = await api(prefix + '/action', previous.action);
  assert.equal(duplicate.receipt.status, 'dispatched');
  view = await api(prefix + '/observe', {});
  assert.ok(view.text.includes('Fixture effects recorded: 1'));
  pass('After replacing the worker container, Aurora reopens the saved login, storage and input journal without repeating the effect');
  await writeFile(reportPath, JSON.stringify({ ...previous, containerReplacement: { result: 'PASS', checks, timestampUTC: new Date().toISOString() } }, null, 2));
}
console.log(JSON.stringify({ result: 'PASS', phase, checks: checks.length, scope: 'Real Aurora/Vault/container fixture only' }));
