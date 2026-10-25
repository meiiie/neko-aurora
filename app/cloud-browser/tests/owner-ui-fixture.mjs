// Container-only owner UI acceptance fixture. No real account or production auth adapter.
import { createServer as createHTTPServer } from 'node:http';
import { createServer as createHTTPSServer } from 'node:https';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { mkdir, readFile, writeFile, rename, chmod } from 'node:fs/promises';
import { join } from 'node:path';
import { createWorker } from '../server.mjs';
import { createPlaywrightDriver } from '../workstation.mjs';

const origin = 'https://127.0.0.1';
const username = 'aurora-owner-fixture-user';
const password = 'aurora-owner-fixture-password-20261025';
const session = 'aurora_owner_fixture=accepted-v1';
const dataDir = '/data';
const serviceKey = process.env.AURORA_CLOUD_BROWSER_KEY;
if (Buffer.byteLength(serviceKey ?? '') < 32) throw Error('owner_fixture_service_key_required');
if (process.platform !== 'linux' || process.getuid() !== 10001) throw Error('owner_fixture_requires_container_user');

const tlsDir = join(dataDir, 'fixture-tls');
await mkdir(tlsDir, { recursive: true, mode: 0o700 });
await chmod(tlsDir, 0o700);
const keyPath = join(tlsDir, 'fixture.key');
const certPath = join(tlsDir, 'fixture.crt');
// Ephemeral fixture TLS material is generated on the private fixture volume, never embedded or printed.
await promisify(execFile)('openssl', ['req', '-x509', '-newkey', 'rsa:2048', '-nodes', '-days', '2',
  '-subj', '/CN=Aurora owner acceptance fixture', '-addext', 'subjectAltName=IP:127.0.0.1',
  '-keyout', keyPath, '-out', certPath], {
  env: { PATH: process.env.PATH ?? '/usr/bin:/bin', LANG: 'C.UTF-8', HOME: '/data' },
  timeout: 15000, maxBuffer: 65536, windowsHide: true,
}).catch(() => { throw Error('owner_fixture_tls_generation_failed'); });
await chmod(keyPath, 0o600);
await chmod(certPath, 0o600);

const effectsPath = join(dataDir, 'fixture-effects.json');
let effects = 0;
try {
  const saved = JSON.parse(await readFile(effectsPath, 'utf8'));
  if (saved.version !== 1 || !Number.isSafeInteger(saved.effects) || saved.effects < 0) throw Error('invalid');
  effects = saved.effects;
} catch (error) {
  if (error.code !== 'ENOENT') throw Error('owner_fixture_effect_store_invalid');
  await writeFile(effectsPath, JSON.stringify({ version: 1, effects: 0 }), { mode: 0o600, flag: 'wx' });
}
let effectQueue = Promise.resolve();
const signedIn = request => (request.headers.cookie ?? '').split(';').some(cookie => cookie.trim() === session);
const allowed = raw => { try { const u = new URL(raw); return u.origin === origin && !u.username && !u.password; } catch { return false; } };
const markerScript = write => `<script>
const localMarker='OWNER-UI-LOCAL-STORAGE-20261025';
const indexedMarker='OWNER-UI-INDEXED-DB-20261025';
${write ? "localStorage.setItem('owner_fixture_marker',localMarker);" : ''}
document.querySelector('#local-marker').textContent=localStorage.getItem('owner_fixture_marker')||'LOCAL STORAGE MARKER MISSING';
const db=indexedDB.open('aurora-owner-fixture',1);
db.onupgradeneeded=()=>db.result.createObjectStore('state');
db.onsuccess=()=>{
  const transaction=db.result.transaction('state',${write ? "'readwrite'" : "'readonly'"});
  const store=transaction.objectStore('state');
  ${write ? "store.put(indexedMarker,'owner-marker');" : ''}
  const marker=store.get('owner-marker');
  marker.onsuccess=()=>document.querySelector('#indexed-marker').textContent=marker.result||'INDEXED DB MARKER MISSING';
};
db.onerror=()=>document.querySelector('#indexed-marker').textContent='INDEXED DB UNAVAILABLE';
</script>`;
function page(body, writeMarkers) {
  return `<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Aurora owner acceptance fixture</title><style>
body{font:18px system-ui,sans-serif;background:#f4f7fb;color:#183044;padding:36px;max-width:960px;margin:auto}
main{padding:30px;background:white;border:1px solid #ccd9e5;border-radius:16px}label{display:block;margin:20px 0}
input{display:block;font:inherit;padding:10px;width:420px;max-width:90%;border:1px solid #8b9eaf;border-radius:6px}
button{font:inherit;padding:12px 20px;border:0;border-radius:7px;background:#125c91;color:white;cursor:pointer}
.fact{padding:12px;background:#edf4fa;border-radius:6px;overflow-wrap:anywhere}</style><main>${body}
<h2>Profile persistence markers</h2><p id="local-marker" class="fact">Local marker pending</p>
<p id="indexed-marker" class="fact">Indexed marker pending</p></main>${markerScript(writeMarkers)}</html>`;
}
const fixture = createHTTPSServer({ key: await readFile(keyPath), cert: await readFile(certPath) }, async (request, response) => {
  const send = (status, body, type = 'text/plain; charset=utf-8', headers = {}) => {
    response.writeHead(status, { 'Content-Type': type, 'Cache-Control': 'no-store', ...headers }); response.end(body);
  };
  try {
    if (request.headers.origin && request.headers.origin !== origin) return send(403, 'Fixture origin rejected');
    if (request.url === '/login' && request.method === 'POST') {
      let encoded = ''; let bytes = 0;
      for await (const chunk of request) {
        bytes += chunk.length;
        if (bytes > 2048) return send(413, 'Fixture form too large');
        encoded += chunk.toString('utf8');
      }
      const values = new URLSearchParams(encoded);
      if (values.get('username') !== username || values.get('password') !== password) return send(403, 'Fixture credentials rejected');
      return send(303, '', 'text/plain', { 'Set-Cookie': `${session}; Max-Age=604800; HttpOnly; Secure; SameSite=Strict; Path=/`, Location: '/' });
    }
    if (request.url === '/effect' && request.method === 'POST') {
      if (!signedIn(request)) return send(401, 'Fixture session required');
      const record = effectQueue.then(async () => {
        if (!Number.isSafeInteger(effects + 1)) throw Error('fixture_counter_limit');
        const next = effects + 1;
        const temporary = `${effectsPath}.${process.pid}.tmp`;
        await writeFile(temporary, JSON.stringify({ version: 1, effects: next }), { mode: 0o600 });
        await rename(temporary, effectsPath); effects = next;
        return next;
      });
      effectQueue = record.catch(() => {});
      return send(200, JSON.stringify({ recorded: true, effects: await record }), 'application/json');
    }
    if (request.url !== '/' || request.method !== 'GET') return send(404, 'Fixture route not found');
    const body = signedIn(request)
      ? `<h1>Fixture account ready</h1><p>This is a source-owned fake account.</p>
<button id="effect" type="button">Record fixture effect</button><p id="effects" class="fact">Fixture effects recorded: ${effects}</p>
<script>document.querySelector('#effect').onclick=async()=>{const button=document.querySelector('#effect');button.disabled=true;
try{const response=await fetch('/effect',{method:'POST'});if(!response.ok)throw Error('Fixture effect failed');
const result=await response.json();document.querySelector('#effects').textContent='Fixture effects recorded: '+result.effects}
catch{document.querySelector('#effects').textContent='Fixture effect result unknown'}finally{button.disabled=false}};</script>`
      : `<h1>Fixture sign in</h1><p>Use only the fake credentials provided with this acceptance fixture.</p>
<form action="/login" method="post"><label>Username<input name="username" autocomplete="username" required></label>
<label>Password<input name="password" type="password" autocomplete="current-password" required></label>
<button type="submit">Sign in to fixture</button></form>`;
    return send(200, page(body, !signedIn(request)), 'text/html; charset=utf-8', {
      'Content-Security-Policy': "default-src 'self'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'; object-src 'none'",
    });
  } catch { if (!response.headersSent) send(500, 'Fixture request failed'); else response.destroy(); }
});
fixture.headersTimeout = 10000; fixture.requestTimeout = 15000;
await new Promise((resolve, reject) => { fixture.once('error', reject); fixture.listen(443, '127.0.0.1', resolve); });

let worker, server;
try {
  const browser = createPlaywrightDriver({ allowURL: allowed, ignoreHTTPSErrors: true });
  worker = await createWorker({ key: serviceKey, dataDir, urlAllowed: allowed,
    driver: { open: options => browser.open({ ...options, url: origin }) } });
  server = createHTTPServer(worker.handle);
  server.requestTimeout = 35000; server.headersTimeout = 10000;
  await new Promise((resolve, reject) => { server.once('error', reject); server.listen(8080, '0.0.0.0', resolve); });
} catch (error) {
  await worker?.close().catch(() => {});
  await new Promise(resolve => fixture.close(resolve));
  throw error;
}
let stopping;
function stop() {
  if (stopping) return stopping;
  stopping = (async () => {
    const closeHTTP = new Promise(resolve => server.close(resolve)); server.closeIdleConnections();
    const closedWorker = await Promise.allSettled([worker.close(), effectQueue]);
    const closeHTTPS = new Promise(resolve => fixture.close(resolve)); fixture.closeIdleConnections();
    server.closeAllConnections(); fixture.closeAllConnections();
    await Promise.all([closeHTTP, closeHTTPS]);
    if (closedWorker.some(result => result.status === 'rejected')) process.exitCode = 1;
  })().catch(() => { process.exitCode = 1; });
  return stopping;
}
process.once('SIGTERM', stop); process.once('SIGINT', stop);
