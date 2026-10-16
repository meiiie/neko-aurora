import { contextCases } from './context-cases.mjs';
import { createServer } from 'node:http';
import { spawn, execFileSync } from 'node:child_process';
import { readFile, writeFile, mkdir, mkdtemp } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { createHash } from 'node:crypto';
import vm from 'node:vm';

const options = Object.fromEntries(process.argv.slice(2).reduce((a, value, i, all) => {
  if (value.startsWith('--')) a.push([value.slice(2), all[i + 1]]);
  return a;
}, []));
if (!options.source || !options.output || !options.commit) throw new Error('--source, --output and --commit required');
const source = path.resolve(options.source), output = path.resolve(options.output);
const chrome = options.chrome || 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe';
await mkdir(output, { recursive: true });
const files = new Map();
for (const name of ['index.html', 'style.css', 'app.js', 'connections.js']) {
  files.set(name, options.committed === 'true'
    ? execFileSync('git', ['-C', source, 'show', options.commit + ':native/internal/web/assets/' + name], { windowsHide: true })
    : await readFile(path.join(source, 'native/internal/web/assets', name)));
}
const sha = data => createHash('sha256').update(data).digest('hex');
const harnessSha256 = sha(await readFile(new URL(import.meta.url)));
const casesSha256 = sha(await readFile(new URL('./context-cases.mjs', import.meta.url)));
const checks = [], observations = {};
const record = (id, ok, evidence, scope = 'desktop Chromium viewport + fixture API') => checks.push({ id, status: ok ? 'pass' : 'fail', scope, evidence });
const sourceIdentity = { declaredCommit: options.commit, actualHEAD: null, mode: options.committed === 'true' ? 'immutable Git object bytes' : 'frozen working checkout bytes', matchesCommit: {}, rawByteMatch: {}, comparison: 'UTF-8 text after CRLF to LF only; raw hashes retained', error: null };
try {
  sourceIdentity.actualHEAD = execFileSync('git', ['-C', source, 'rev-parse', 'HEAD'], { encoding: 'utf8', windowsHide: true }).trim();
  for (const [name, bytes] of files) {
    const committed = execFileSync('git', ['-C', source, 'show', options.commit + ':native/internal/web/assets/' + name], { windowsHide: true });
    sourceIdentity.rawByteMatch[name] = sha(committed) === sha(bytes);
    sourceIdentity.matchesCommit[name] = committed.toString('utf8').replaceAll('\r\n', '\n') === bytes.toString('utf8').replaceAll('\r\n', '\n');
  }
  record('source_matches_declared_commit', Object.values(sourceIdentity.matchesCommit).every(Boolean), sourceIdentity, 'Four frozen UI assets compared with declared Git object text, accounting for Windows CRLF; current HEAD may advance independently; no Git index write');
} catch (error) { sourceIdentity.error = error.message; checks.push({ id: 'source_matches_declared_commit', status: 'blocked', scope: 'Git object read', evidence: sourceIdentity }); }
for (const name of ['app.js', 'connections.js']) {
  try { new vm.Script(files.get(name).toString('utf8')); record('syntax_' + name, true, 'V8 parse'); }
  catch (error) { record('syntax_' + name, false, error.message); }
}
const bots = [
  { id: 'bot-meiiie', name: 'Meiiie', alias: 'meiiie', color: '#4b6259', revision: 1 },
  { id: 'bot-davin', name: 'Davin', alias: 'davin', color: '#4b5862', revision: 1 },
];
const fixture = { work: [], sequence: 0, events: [], streams: new Set(), messageMode: 'normal', snapshotMode: 'normal', requests: [], snapshotRequests: 0, eventConnections: 0, held: [], cancelMode: 'normal', capabilities: [], contextRequests: [], contextHeld: [], contextMode: 'hold' };
const clone = value => structuredClone(value);
function push(work) {
  const event = { sequence: ++fixture.sequence, type: 'work', work: clone(work) };
  fixture.events.push(event);
  for (const stream of fixture.streams) stream.write(`id: ${event.sequence}\nevent: work\ndata: ${JSON.stringify(event)}\n\n`);
  return event;
}
function admit(input) {
  const old = fixture.work.find(w => w.clientNonce === input.clientNonce && (w.sourceBotId || w.botId) === input.botId);
  if (old) return { work: old, duplicate: true };
  const work = { ...input, requestId: 'fixture-' + (fixture.work.length + 1), sourceMessageId: 'message-' + (fixture.work.length + 1), ownerUserId: 'local-fixture', spaceId: 'local-fixture', conversationId: input.botId, status: 'waiting_host', desiredState: 'running', epoch: 1, botConfigRevision: 1, createdAt: '2026-10-11T00:00:00Z', createdSequence: fixture.sequence + 1 };
  fixture.work.push(work); push(work); return { work, duplicate: false };
}
const server = createServer(async (req, res) => {
  const url = new URL(req.url, 'http://127.0.0.1');
  const json = (body, status = 200) => { res.writeHead(status, { 'Content-Type': 'application/json', 'Cache-Control': 'no-store' }); res.end(JSON.stringify(body)); };
  if (url.pathname === '/v1/snapshot') {
    fixture.snapshotRequests++;
    return fixture.snapshotMode === 'fail' ? json({ error: 'fixture-unavailable' }, 503) : json({ bots, work: fixture.work, sequence: fixture.sequence, runtime: 'core_unavailable', capabilities: fixture.capabilities });
  }
  if (url.pathname === '/v1/events') {
    fixture.eventConnections++;
    res.writeHead(200, { 'Content-Type': 'text/event-stream', 'Cache-Control': 'no-cache' });
    res.write('retry: 120\n: isolated fixture\n\n');
    const after = Number(req.headers['last-event-id'] || url.searchParams.get('after') || 0);
    for (const e of fixture.events.filter(e => e.sequence > after)) res.write(`id: ${e.sequence}\nevent: work\ndata: ${JSON.stringify(e)}\n\n`);
    fixture.streams.add(res); req.on('close', () => fixture.streams.delete(res)); return;
  }
  if (url.pathname === '/v1/connections') return json({ accounts: [], plugins: [{ id: 'fixture-built-in', name: 'Fixture artifact declaration', version: '1.0.0', status: 'available_for_requested_task_artifacts' }] });
  if (req.method === 'POST') {
    const parts = []; for await (const part of req) parts.push(part);
    const input = JSON.parse(Buffer.concat(parts).toString('utf8'));
    if (url.pathname === '/v1/context-access' || url.pathname === '/v1/context-refresh') {
      const kind = url.pathname.endsWith('access') ? 'access' : 'refresh';
      fixture.contextRequests.push({ kind, input: clone(input) });
      fixture.contextHeld.push({ kind, input, json }); return;
    }
    if (url.pathname === '/v1/messages') {
      fixture.requests.push(clone(input));
      if (fixture.messageMode === 'reject') return json({ error: 'fixture-no-write' }, 503);
      if (fixture.messageMode === 'hold') { fixture.held.push({ input, json }); return; }
      const result = admit(input);
      return fixture.messageMode === 'lost-ack' ? json({ error: 'fixture-committed-ack-unavailable' }, 503) : json(result);
    }
    if (url.pathname === '/v1/cancel') {
      if (fixture.cancelMode === 'reject') return json({ error: 'fixture-not-confirmed' }, 503);
      const work = fixture.work.find(w => w.requestId === input.requestId);
      if (!work) return json({ error: 'fixture-not-found' }, 404);
      work.status = 'cancelled'; push(work); return json(work);
    }
    return json({ error: 'unsupported-fixture-route' }, 501);
  }
  const name = url.pathname === '/' ? 'index.html' : url.pathname.slice(1);
  if (files.has(name)) { res.writeHead(200, { 'Content-Type': name.endsWith('.html') ? 'text/html; charset=utf-8' : name.endsWith('.css') ? 'text/css; charset=utf-8' : 'text/javascript; charset=utf-8', 'Cache-Control': 'no-store' }); res.end(files.get(name)); }
  else { res.writeHead(404); res.end(); }
});
await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
const base = 'http://127.0.0.1:' + server.address().port;
const profile = await mkdtemp(path.join(tmpdir(), 'neko-mobile-audit-'));
const environment = Object.fromEntries(['SystemRoot', 'WINDIR', 'TEMP', 'TMP', 'USERPROFILE', 'LOCALAPPDATA', 'APPDATA'].filter(k => process.env[k]).map(k => [k, process.env[k]]));
const browser = spawn(chrome, ['--headless=new', '--disable-gpu', '--disable-background-networking', '--disable-sync', '--disable-extensions', '--no-first-run', '--remote-debugging-address=127.0.0.1', '--remote-debugging-port=0', '--user-data-dir=' + profile, 'about:blank'], { windowsHide: true, stdio: ['ignore', 'pipe', 'pipe'], env: environment });
let launchDiagnostics = ''; browser.stderr.on('data', chunk => { launchDiagnostics = (launchDiagnostics + chunk.toString()).slice(-3000); });
browser.on('error', error => { launchDiagnostics += error.message; });
const delay = ms => new Promise(r => setTimeout(r, ms));
async function until(fn, ms = 6000) { const end = Date.now() + ms; while (Date.now() < end) { const value = await fn(); if (value) return value; await delay(80); } return false; }
let socket, debugPort, pageErrors = [];
try {
  const active = await until(async () => { try { return await readFile(path.join(profile, 'DevToolsActivePort'), 'utf8'); } catch { return false; } }, 10000);
  if (!active) throw new Error('Owned headless browser failed to start: exit=' + browser.exitCode + '; ' + launchDiagnostics);
  debugPort = Number(active.split('\n')[0]);
  const target = await fetch(`http://127.0.0.1:${debugPort}/json/new?about:blank`, { method: 'PUT' }).then(r => r.json());
  const debuggerURL = new URL(target.webSocketDebuggerUrl);
  if (debuggerURL.hostname !== '127.0.0.1' || Number(debuggerURL.port) !== debugPort) throw new Error('Debugger ownership check failed');
  socket = new WebSocket(debuggerURL);
  await new Promise((resolve, reject) => { socket.addEventListener('open', resolve, { once: true }); socket.addEventListener('error', reject, { once: true }); });
  let sequence = 0; const pending = new Map();
  socket.addEventListener('message', event => {
    const data = JSON.parse(event.data), item = pending.get(data.id);
    if (item) { pending.delete(data.id); clearTimeout(item.timer); data.error ? item.reject(new Error(data.error.message)) : item.resolve(data.result); }
    if (data.method === 'Runtime.exceptionThrown') pageErrors.push(data.params.exceptionDetails.text + ' ' + (data.params.exceptionDetails.exception?.description || ''));
  });
  const call = (method, params = {}) => new Promise((resolve, reject) => { const id = ++sequence, timer = setTimeout(() => { pending.delete(id); reject(new Error('CDP timeout ' + method)); }, 7000); pending.set(id, { resolve, reject, timer }); socket.send(JSON.stringify({ id, method, params })); });
  const evaluate = async expression => { const result = await call('Runtime.evaluate', { expression, returnByValue: true, awaitPromise: true }); if (result.exceptionDetails) throw new Error(result.exceptionDetails.exception?.description || result.exceptionDetails.text); return result.result.value; };
  const run = async (id, fn) => {
    try { const value = await fn(); record(id, value.ok, value.evidence); }
    catch (error) { checks.push({ id, status: 'blocked', evidence: { harnessError: error.message } }); }
    finally { await evaluate(`(()=>{for(const d of document.querySelectorAll('dialog[open]'))d.close();if(!document.getElementById('work-panel').hidden)document.getElementById('work-close').click();if(document.getElementById('sidebar').classList.contains('open'))document.getElementById('drawer-backdrop').click()})()`).catch(()=>{}); await delay(220); }
  };
  const viewport = async (width, height, mobile = width <= 700) => { await call('Emulation.setDeviceMetricsOverride', { width, height, deviceScaleFactor: 1, mobile }); await delay(80); };
  const reload = async () => { await call('Page.reload'); return until(() => evaluate('document.querySelectorAll("#bots button").length === 2')); };
  const click = async selector => {
    const rect = await evaluate(`(()=>{const e=document.querySelector(${JSON.stringify(selector)}); if(!e) return null; e.scrollIntoView({block:'nearest'}); const r=e.getBoundingClientRect(); return {x:r.x+r.width/2,y:r.y+r.height/2,width:r.width,height:r.height}})()`);
    if (!rect || !rect.width || !rect.height) throw new Error('Control not visible: ' + selector);
    await call('Input.dispatchMouseEvent', { type: 'mousePressed', x: rect.x, y: rect.y, button: 'left', clickCount: 1 });
    await call('Input.dispatchMouseEvent', { type: 'mouseReleased', x: rect.x, y: rect.y, button: 'left', clickCount: 1 }); await delay(240);
  };
  const key = async (name, shift = false) => { const keyCode = { Tab: 9, Escape: 27, Enter: 13, ArrowRight: 39 }[name]; await call('Input.dispatchKeyEvent', { type: 'keyDown', key: name, code: name, windowsVirtualKeyCode: keyCode, nativeVirtualKeyCode: keyCode, ...(name === 'Enter' ? { text: '\r', unmodifiedText: '\r' } : {}), modifiers: shift ? 8 : 0 }); await call('Input.dispatchKeyEvent', { type: 'keyUp', key: name, code: name, windowsVirtualKeyCode: keyCode, nativeVirtualKeyCode: keyCode, ...(name === 'Enter' ? { text: '\r', unmodifiedText: '\r' } : {}), modifiers: shift ? 8 : 0 }); await delay(30); };
  const ax = async selector => { const obj = await call('Runtime.evaluate', { expression: `document.querySelector(${JSON.stringify(selector)})` }); if (!obj.result.objectId) return null; const tree = await call('Accessibility.getPartialAXTree', { objectId: obj.result.objectId, fetchRelatives: false }); const n = tree.nodes[0]; return { name: n?.name?.value || '', role: n?.role?.value, ignored: n?.ignored, properties: n?.properties }; };
  const screenshot = async name => { const shot = await call('Page.captureScreenshot', { format: 'png' }); await writeFile(path.join(output, name + '.png'), Buffer.from(shot.data, 'base64')); };
  const touch = async selectors => evaluate(`(${function (selectors) { return selectors.flatMap(selector => [...document.querySelectorAll(selector)].filter(e => {const r=e.getBoundingClientRect(),s=getComputedStyle(e);return r.width&&r.height&&s.visibility!=='hidden'&&!e.closest('[inert]');}).map(e => {const r=e.getBoundingClientRect();return {selector,id:e.id,width:r.width,height:r.height,text:e.textContent.trim().slice(0,60)};}));}.toString()})(${JSON.stringify(selectors)})`);
  await call('Page.enable'); await call('Runtime.enable'); await call('Accessibility.enable');
  const version = await call('Browser.getVersion'); observations.browser = version.product;
  await viewport(390, 844); await call('Page.navigate', { url: base });
  const ready = await until(() => evaluate('document.querySelectorAll("#bots button").length === 2'));
  record('UI_initialization', ready, { botCount: await evaluate('document.querySelectorAll("#bots button").length') });
  if (!ready) throw new Error('UI did not initialize against the explicit fixture contract');
  await contextCases({ run, fixture, clone, push, evaluate, call, viewport, reload, click, key, ax, screenshot, touch, until, delay });
  record('unexpected_page_exceptions', !pageErrors.filter(e => !/storage blocked/.test(e)).length, pageErrors.filter(e => !/storage blocked/.test(e)));
  observations.finalFixture = { requests: fixture.requests.length, workItems: fixture.work.length, snapshots: fixture.snapshotRequests, eventConnections: fixture.eventConnections };
  await call('Browser.close').catch(() => {});
} catch (error) { checks.push({ id: 'audit_execution', status: 'blocked', evidence: error.message }); }
finally {
  socket?.close(); for (const item of fixture.held) { try { item.json({ error: 'fixture-stopped' }, 503); } catch {} }
  for (const item of fixture.contextHeld) { try { item.json({ error: 'fixture-stopped' }, 503); } catch {} }
  for (const stream of fixture.streams) stream.end(); server.closeAllConnections(); await new Promise(resolve => server.close(resolve));
  if (browser.exitCode === null) { await delay(250); if (browser.exitCode === null) browser.kill(); }
  const report = {
    audit: 'Neko context mobile/accessibility supplement 2026-10-11', timestampUTC: new Date().toISOString(), commit: options.commit,
    source, sourceIdentity, harnessSha256, casesSha256, sourceHashes: Object.fromEntries([...files].map(([name, bytes]) => [name, { sha256: sha(bytes), bytes: bytes.length }])),
    environment: { os: process.platform, node: process.version, chrome, headless: true, fixtureOrigin: base, ownedProfile: profile, debuggerPort: debugPort },
    command: [process.execPath, ...process.argv.slice(1)], scope: 'Authored UI loaded from frozen source bytes with in-memory local API fixture. No Rust/Go/Core/provider, real phone, DPAPI, durable store, cloud/device sync, external grant or network settings tested.',
    totals: Object.fromEntries(['pass', 'fail', 'blocked'].map(status => [status, checks.filter(c => c.status === status).length])), checks, observations,
    physicalPhone: 'blocked: no physical phone session; resizing is not real keyboard/IME/safe area/back/lifecycle/AT acceptance',
    multiDevice: 'blocked: no authenticated multi-host transport; this fixture tests client replay only; separate client/nonce rules documented in review',
  };
  await writeFile(path.join(output, 'results.json'), JSON.stringify(report, null, 2) + '\n');
  console.log(JSON.stringify({ commit: report.commit, output: path.join(output, 'results.json'), totals: report.totals, failures: checks.filter(c => c.status !== 'pass').map(c => ({ id: c.id, status: c.status, evidence: c.evidence })) }, null, 2));
  process.exitCode = report.totals.blocked ? 2 : report.totals.fail ? 1 : 0;
}
