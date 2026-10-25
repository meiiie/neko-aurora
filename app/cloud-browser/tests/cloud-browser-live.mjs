// Run in the container image. This uses fixture profiles, never a Zalo account.
import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { mkdtemp, rm, readFile, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { createCipheriv, createHmac, randomBytes } from 'node:crypto';
import { createWorker } from '../server.mjs';
import { createPlaywrightDriver } from '../workstation.mjs';

const key = 'cloud-fixture-key-never-a-user-credential-7281';
const namespace = 'c'.repeat(64);
const profile = 'bot-live-fixture';
const checks = [];
const phase = process.env.AURORA_FIXTURE_PHASE ?? '';
assert.ok(['', 'prepare', 'check'].includes(phase), 'Unknown fixture phase');
let effects = 0, worker;
const fixture = createServer(async (request, response) => {
  if (request.url === '/login' && request.method === 'POST') {
    let encoded = '';
    for await (const chunk of request) {
      encoded += chunk;
      if (encoded.length > 2048) { response.writeHead(413); response.end(); return; }
    }
    const values = new URLSearchParams(encoded);
    if (values.get('username') !== 'fixture-user-7281' || values.get('password') !== 'fixture-password-7281') {
      response.writeHead(403); response.end('Fixture credentials rejected'); return;
    }
    response.writeHead(303, { 'Set-Cookie': 'fixture_session=accepted; Max-Age=86400; HttpOnly; SameSite=Strict; Path=/', Location: '/' });
    response.end(); return;
  }
  if (request.url === '/effect' && request.method === 'POST') {
    effects++; response.writeHead(200); response.end('recorded'); return;
  }
  response.setHeader('Content-Type', 'text/html; charset=utf-8');
  const signedIn = String(request.headers.cookie).includes('fixture_session=accepted');
  response.end(signedIn ? `<!doctype html><title>Fixture account</title><h1>Fixture account ready</h1>
    <p id="persistence"></p><p id="indexed"></p><button id="effect">Record fixture effect</button><p id="result"></p>
    <script>document.querySelector('#persistence').textContent=localStorage.getItem('fixture_marker')||'no marker';
    const db=indexedDB.open('fixture-session',1);db.onsuccess=()=>{const q=db.result.transaction('state').objectStore('state').get('marker');q.onsuccess=()=>document.querySelector('#indexed').textContent=q.result||'no indexed marker'};
    document.querySelector('#effect').onclick=async()=>{await fetch('/effect',{method:'POST'});document.querySelector('#result').textContent='Fixture effect recorded'};</script>` :
    `<!doctype html><title>Fixture login</title><h1>Fixture sign in</h1>
    <form action="/login" method="post"><label>Username<input name="username" autocomplete="username"></label>
    <label>Password<input name="password" type="password" autocomplete="current-password"></label>
    <button>Sign in to fixture</button></form>
    <script>localStorage.setItem('fixture_marker','PERSISTENT-LOCAL-STORAGE-7281');const db=indexedDB.open('fixture-session',1);db.onupgradeneeded=()=>db.result.createObjectStore('state');db.onsuccess=()=>db.result.transaction('state','readwrite').objectStore('state').put('PERSISTENT-INDEXED-DB-7281','marker');</script>`);
});
await new Promise(resolve => fixture.listen(phase ? 18991 : 0, '127.0.0.1', resolve));
const origin = `http://127.0.0.1:${fixture.address().port}`;
const allowed = raw => { try { return new URL(raw).origin === origin; } catch { return false; } };
const dataDir = phase ? '/data/acceptance' : await mkdtemp(join(tmpdir(), 'aurora-cloud-browser-live-'));
const options = { key, dataDir, urlAllowed: allowed, driver: createPlaywrightDriver({ allowURL: allowed }) };
const base = { namespace, profile, actor: 'owner', actorId: 'fixture-owner' };
const command = async (kind, fields = {}) => {
  const result = await worker.command({ ...base, kind, ...fields });
  if (!['status', 'observe', 'frame'].includes(kind)) assert.equal(result.receipt?.status, 'dispatched', `${kind}: ${result.receipt?.code ?? result.error ?? 'missing receipt'}`);
  return result;
};
const pass = name => { checks.push(name); console.log('PASS: ' + name); };
const nonce = name => 'live-fixture-' + name;
async function observeUntil(text) {
  const deadline = Date.now() + 10000;
  while (Date.now() < deadline) {
    try {
      const view = await command('observe');
      if (view.text?.includes(text)) return view;
    } catch (error) {
      // Navigation can invalidate a read in flight. Retry only this harmless read.
      if (error.code !== 'stale_observation') throw error;
    }
    await new Promise(resolve => setTimeout(resolve, 100));
  }
  throw Error('Fixture browser did not show expected readback');
}
function seal(nonceValue, observationId, values) {
  const aesKey = createHmac('sha256', key).update('aurora-cloud-vault-v1\n' + namespace + '\n' + profile).digest();
  const iv = randomBytes(12);
  const cipher = createCipheriv('aes-256-gcm', aesKey, iv);
  cipher.setAAD(Buffer.from([namespace, profile, nonceValue, origin, observationId].join('\n')));
  const encrypted = Buffer.concat([cipher.update(JSON.stringify(values)), cipher.final()]);
  return Buffer.concat([iv, encrypted, cipher.getAuthTag()]).toString('base64');
}
try {
  worker = await createWorker(options);
  if (phase === 'check') {
    let state = await command('open', { url: origin, nonce: nonce('second-container-open') });
    assert.equal(state.control, 'paused');
    state = await command('control', { mode: 'owner', epoch: state.epoch, nonce: nonce('second-container-owner') });
    const view = await observeUntil('Fixture account ready');
    assert.ok(view.text.includes('PERSISTENT-LOCAL-STORAGE-7281'));
    await observeUntil('PERSISTENT-INDEXED-DB-7281');
    pass('A new container on the named volume retains the persistent cookie, localStorage and IndexedDB');
    const oldAction = JSON.parse(await readFile(join(dataDir, 'fixture-action.json'), 'utf8'));
    const duplicate = await command('click', oldAction);
    assert.equal(duplicate.receipt.status, 'dispatched');
    assert.equal(effects, 0);
    pass('A nonce journal survives container replacement and does not replay the earlier website effect');
    await command('close', { epoch: state.epoch, nonce: nonce('second-container-close') });
    const second = await worker.command({ ...base, namespace: 'd'.repeat(64), kind: 'open', url: origin, nonce: nonce('second-container-isolation') });
    assert.ok(second.text.includes('Fixture sign in'));
    pass('Another namespace on the same named volume starts without the saved account cookie');
  } else {
  let state = await command('open', { url: origin, nonce: nonce('open-1') });
  assert.equal(state.control, 'paused');
  state = await command('control', { mode: 'owner', epoch: state.epoch, nonce: nonce('owner-1') });
  let view = await command('observe');
  assert.equal(view.authState, 'login_required');
  const frame = await command('frame');
  assert.ok(Buffer.from(frame.frame.data, 'base64').length > 1000);
  assert.ok(frame.frame.width > 0 && frame.frame.height > 0);
  if (phase === 'prepare') await writeFile(join(dataDir, 'fixture-owner-frame.jpg'), Buffer.from(frame.frame.data, 'base64'), { mode: 0o600 });
  pass('Real isolated Chromium starts with an owner-only login frame');

  const fillNonce = nonce('vault-fill');
  const sealed = seal(fillNonce, view.observationId, { username: 'fixture-user-7281', password: 'fixture-password-7281' });
  await command('vault_fill', { epoch: state.epoch, observationId: view.observationId, expectedOrigin: origin, nonce: fillNonce, sealed });
  view = await command('observe');
  assert.ok(!JSON.stringify(view).includes('fixture-password-7281'));
  pass('Encrypted one-use Vault payload fills a real form without exposing values');
  const submit = view.targets.find(target => target.name === 'Sign in to fixture');
  assert.ok(submit);
  await command('click', { epoch: state.epoch, observationId: view.observationId, targetId: submit.id, nonce: nonce('sign-in') });
  view = await observeUntil('Fixture account ready');
  assert.ok(view.text.includes('PERSISTENT-LOCAL-STORAGE-7281'));
  view = await observeUntil('PERSISTENT-INDEXED-DB-7281');
  assert.equal(view.authState, 'unknown');
  pass('Fixture login is observed while production authentication stays honestly unknown');

  const effect = view.targets.find(target => target.name === 'Record fixture effect');
  const action = { epoch: state.epoch, observationId: view.observationId, targetId: effect.id, nonce: nonce('effect') };
  await command('click', action);
  await observeUntil('Fixture effect recorded');
  await command('click', action);
  assert.equal(effects, 1);
  pass('Repeated accepted input nonce produces one actual website effect');

  if (phase === 'prepare') {
    await writeFile(join(dataDir, 'fixture-action.json'), JSON.stringify(action), { mode: 0o600 });
    pass('Fixture profile and input reference saved for the next container');
  } else {
  await worker.close(); worker = await createWorker(options);
  state = await command('open', { url: origin, nonce: nonce('open-after-restart') });
  assert.equal(state.control, 'paused');
  state = await command('control', { mode: 'owner', epoch: state.epoch, nonce: nonce('owner-after-restart') });
  view = await observeUntil('Fixture account ready');
  assert.ok(view.text.includes('PERSISTENT-LOCAL-STORAGE-7281'));
  await observeUntil('PERSISTENT-INDEXED-DB-7281');
  pass('Full browser profile preserves cookies, local storage and IndexedDB across worker restart');

  await command('close', { epoch: state.epoch, nonce: nonce('close-first') });
  const second = await worker.command({ ...base, namespace: 'd'.repeat(64), kind: 'open', url: origin, nonce: nonce('other-account') });
  assert.ok(second.text.includes('Fixture sign in'));
  assert.ok(!second.text.includes('Fixture account ready'));
  pass('Another workspace cannot inherit the first workspace login cookie');
  }
  }
  console.log(JSON.stringify({ result: 'PASS', phase: phase || 'single-container', checks, scope: 'Real Chromium fixture in isolated container; no Zalo account or user browser', timestampUTC: new Date().toISOString() }));
} finally {
  if (worker) await worker.close();
  await new Promise(resolve => fixture.close(resolve));
  if (!phase) await rm(dataDir, { recursive: true, force: true });
}
