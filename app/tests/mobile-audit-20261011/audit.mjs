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
const fixture = { work: [], sequence: 0, events: [], streams: new Set(), messageMode: 'normal', snapshotMode: 'normal', requests: [], snapshotRequests: 0, eventConnections: 0, held: [], cancelMode: 'normal' };
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
    return fixture.snapshotMode === 'fail' ? json({ error: 'fixture-unavailable' }, 503) : json({ bots, work: fixture.work, sequence: fixture.sequence, runtime: 'core_unavailable' });
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
  const key = async (name, shift = false) => { const keyCode = { Tab: 9, Escape: 27, Enter: 13, ArrowRight: 39 }[name]; await call('Input.dispatchKeyEvent', { type: 'keyDown', key: name, code: name, windowsVirtualKeyCode: keyCode, modifiers: shift ? 8 : 0 }); await call('Input.dispatchKeyEvent', { type: 'keyUp', key: name, code: name, windowsVirtualKeyCode: keyCode, modifiers: shift ? 8 : 0 }); await delay(30); };
  const ax = async selector => { const obj = await call('Runtime.evaluate', { expression: `document.querySelector(${JSON.stringify(selector)})` }); if (!obj.result.objectId) return null; const tree = await call('Accessibility.getPartialAXTree', { objectId: obj.result.objectId, fetchRelatives: false }); const n = tree.nodes[0]; return { name: n?.name?.value || '', role: n?.role?.value, ignored: n?.ignored, properties: n?.properties }; };
  const screenshot = async name => { const shot = await call('Page.captureScreenshot', { format: 'png' }); await writeFile(path.join(output, name + '.png'), Buffer.from(shot.data, 'base64')); };
  const touch = async selectors => evaluate(`(${function (selectors) { return selectors.flatMap(selector => [...document.querySelectorAll(selector)].filter(e => {const r=e.getBoundingClientRect(),s=getComputedStyle(e);return r.width&&r.height&&s.visibility!=='hidden'&&!e.closest('[inert]');}).map(e => {const r=e.getBoundingClientRect();return {selector,id:e.id,width:r.width,height:r.height,text:e.textContent.trim().slice(0,60)};}));}.toString()})(${JSON.stringify(selectors)})`);
  await call('Page.enable'); await call('Runtime.enable'); await call('Accessibility.enable');
  const version = await call('Browser.getVersion'); observations.browser = version.product;
  await viewport(390, 844); await call('Page.navigate', { url: base });
  const ready = await until(() => evaluate('document.querySelectorAll("#bots button").length === 2'));
  record('UI_initialization', ready, { botCount: await evaluate('document.querySelectorAll("#bots button").length') });
  if (!ready) throw new Error('UI did not initialize against the explicit fixture contract');
  await run('empty_layout_breakpoint_matrix', async () => {
    const measurements = [];
    for (const [width, height] of [[320, 640], [360, 640], [390, 844], [414, 896], [700, 700], [701, 700], [768, 1024], [1000, 700], [1440, 1000], [844, 390], [390, 260]]) {
      await viewport(width, height);
      measurements.push(await evaluate(`(()=>{const r=document.getElementById('send').getBoundingClientRect(),c=document.getElementById('composer').getBoundingClientRect(),p=document.getElementById('prompt').getBoundingClientRect();return {width:innerWidth,height:innerHeight,documentWidth:document.documentElement.scrollWidth,bodyWidth:document.body.scrollWidth,send:{left:r.left,right:r.right,top:r.top,bottom:r.bottom},composer:{left:c.left,right:c.right,top:c.top,bottom:c.bottom},prompt:{top:p.top,bottom:p.bottom},appHeight:document.querySelector('.app').getBoundingClientRect().height}})()`));
    }
    const failed = measurements.filter(m => m.documentWidth > m.width + 1 || m.bodyWidth > m.width + 1 || m.send.right > m.width + 1 || m.send.left < 0 || m.send.bottom > m.height + 1 || m.send.top < 0 || m.prompt.top < 0);
    observations.emptyLayouts = measurements; return { ok: !failed.length, evidence: { failed, measured: measurements.length, keyboard: '260px is only resized layout viewport; no OS keyboard' } };
  });
  await viewport(390, 844); await screenshot('empty-390');
  await run('mobile_toolbar_accessible_names', async () => { const controls = {}; for (const id of ['nav-open', 'work-open', 'files-open', 'send']) { if (await evaluate(`!!document.getElementById('${id}')`)) controls[id] = await ax('#' + id); } return { ok: Object.values(controls).every(c => c && !c.ignored && c.name && !/^\d+$/.test(c.name)), evidence: controls }; });
  await run('main_controls_touch44', async () => { const values = await touch(['#nav-open', '#work-open', '#files-open', '#send', '.composer-footer label', '[data-suggestion]']); return { ok: values.every(v => v.width >= 44 && v.height >= 44), evidence: values }; });
  await run('wide_touch_landscape_controls44', async () => {
    await call('Emulation.setTouchEmulationEnabled', { enabled: true, maxTouchPoints: 5 }); await viewport(844, 390, true);
    const media = await evaluate(`({width:innerWidth,height:innerHeight,coarse:matchMedia('(pointer:coarse)').matches,touchPoints:navigator.maxTouchPoints})`);
    const controls = await touch(['#work-open', '#files-open', '#send', '.composer-footer label']);
    await call('Emulation.setTouchEmulationEnabled', { enabled: false }); await viewport(390, 844);
    return { ok: controls.every(v => v.width >= 43.99 && v.height >= 43.99), evidence: { media, controls, scope: 'Coarse pointer + landscape viewport simulation only; no physical phone' } };
  });
  await run('navigation_modal_keyboard_scope', async () => { await click('#nav-open'); const before = await evaluate(`({mainInert:document.querySelector('main').inert,role:document.getElementById('sidebar').getAttribute('role'),modal:document.getElementById('sidebar').getAttribute('aria-modal'),focusInside:document.getElementById('sidebar').contains(document.activeElement)})`); let escaped = false; for (let i = 0; i < 12; i++) { await key('Tab'); if (!await evaluate(`document.getElementById('sidebar').contains(document.activeElement)`)) escaped = true; } await key('Escape'); const after = await evaluate(`({closed:document.getElementById('sidebar').inert,focus:document.activeElement.id,mainInert:document.querySelector('main').inert})`); return { ok: before.mainInert && before.modal === 'true' && before.focusInside && !escaped && after.closed && after.focus === 'nav-open' && !after.mainInert, evidence: { before, escaped, after } }; });
  await run('work_panel_mobile_keyboard_scope', async () => { await click('#work-open'); const before = await evaluate(`({mainInert:document.querySelector('main').inert,modal:document.getElementById('work-panel').getAttribute('aria-modal'),role:document.getElementById('work-panel').getAttribute('role'),focusInside:document.getElementById('work-panel').contains(document.activeElement)})`); let escaped = false; for (let i = 0; i < 12; i++) { await key('Tab'); if (!await evaluate(`document.getElementById('work-panel').contains(document.activeElement)`)) escaped = true; } await evaluate(`document.getElementById('work-close').focus()`); await key('Escape'); const after = await evaluate(`({hidden:document.getElementById('work-panel').hidden,focus:document.activeElement.id,mainInert:document.querySelector('main').inert})`); return { ok: before.mainInert && before.modal === 'true' && before.focusInside && !escaped && after.hidden && after.focus === 'work-open' && !after.mainInert, evidence: { before, escaped, after } }; });
  await run('connections_dialog_tabs_escape_secret_clear', async () => {
    await click('#nav-open'); await click('#connections-open'); const name = await ax('#connections-dialog');
    await click('#account-add'); await evaluate(`document.getElementById('account-secret').value='fixture-only-dismiss'`); await key('Escape');
    const after = await evaluate(`({closed:!document.getElementById('connections-dialog').open,secretEmpty:document.getElementById('account-secret').value==='',focus:document.activeElement.id,mainInert:document.querySelector('main').inert})`);
    await click('#nav-open'); await click('#connections-open'); await evaluate(`document.getElementById('accounts-tab').focus()`); await key('ArrowRight');
    const tabs = await evaluate(`({selected:document.getElementById('plugins-tab').getAttribute('aria-selected'),focus:document.activeElement.id,hidden:document.getElementById('accounts-section').hidden})`); await key('Escape');
    return { ok: !!name.name && after.closed && after.secretEmpty && !after.mainInert && ['nav-open', 'connections-open'].includes(after.focus) && tabs.selected === 'true' && tabs.focus === 'plugins-tab' && tabs.hidden, evidence: { name, after, tabs } };
  });
  await run('dialog_small_height_reflow_input16', async () => {
    await viewport(320, 320); await click('#nav-open'); await click('#connections-open'); await click('#account-add');
    const dimensions = await evaluate(`(()=>{const d=document.getElementById('connections-dialog'),f=document.getElementById('account-secret');f.scrollIntoView({block:'center'});const r=f.getBoundingClientRect();return {viewport:{width:innerWidth,height:innerHeight},dialog:{width:d.clientWidth,scrollWidth:d.scrollWidth,height:d.getBoundingClientRect().height},secret:{top:r.top,bottom:r.bottom,font:getComputedStyle(f).fontSize},inputs:[...d.querySelectorAll('input,select')].filter(i=>i.getBoundingClientRect().width>2&&i.getBoundingClientRect().height>2).map(i=>({name:i.name,font:parseFloat(getComputedStyle(i).fontSize)}))}})()`);
    await screenshot('dialog-320x320'); await key('Escape'); await viewport(390, 844);
    return { ok: dimensions.dialog.scrollWidth <= dimensions.dialog.width + 1 && dimensions.secret.top >= 0 && dimensions.secret.bottom <= dimensions.viewport.height && dimensions.inputs.every(i => i.font >= 16), evidence: { ...dimensions, input16: 'Product guard against small mobile form text; iOS focus zoom requires real-device verification' } };
  });
  const long = 'fixture-averylongunbrokentoken'.repeat(35);
  const seeded = [
    { botId: 'bot-meiiie', text: long, lane: 'foreground', clientNonce: 'fixture-seed-complete' },
    { botId: 'bot-meiiie', text: 'Fixture background cancellable obligation', lane: 'background', clientNonce: 'fixture-seed-running' },
    { botId: 'bot-meiiie', text: 'Fixture interrupted obligation', lane: 'background', clientNonce: 'fixture-seed-recovery' },
  ].map(input => admit(input).work);
  seeded[0].status = 'completed'; seeded[0].reply = 'Fixture reply ' + long; seeded[0].artifacts = [{ id: 'fixture-artifact', filename: long.slice(0, 180) + '.md', bytes: 123, sha256: '0'.repeat(64) }];
  seeded[1].status = 'running'; seeded[2].status = 'recovery_required';
  for (const w of seeded) push(w); await reload();
  await run('long_content_internal_overflow', async () => {
    const values = [];
    for (const width of [320, 390, 700, 701, 768]) { await viewport(width, 640); values.push(await evaluate(`(()=>{const selectors=['#conversation','#messages','#composer','.topbar','.artifact'];return {width:innerWidth,document:document.documentElement.scrollWidth,nodes:selectors.flatMap(s=>[...document.querySelectorAll(s)].map(e=>({selector:s,client:e.clientWidth,scroll:e.scrollWidth,left:e.getBoundingClientRect().left,right:e.getBoundingClientRect().right})))}})()`)); }
    await viewport(390, 844); await screenshot('history-390');
    return { ok: values.every(v => v.document <= v.width + 1 && v.nodes.every(n => n.scroll <= n.client + 1 && n.right <= v.width + 1 && n.left >= 0)), evidence: values };
  });
  await run('message_actions_touch44', async () => { const message = await touch(['.reply-actions button', '.recover']); await click('#work-open'); const panel = await touch(['.task button', '#work-close']); await key('Escape'); const all = [...message, ...panel]; return { ok: all.length > 2 && all.every(v => v.width >= 43.99 && v.height >= 43.99), evidence: all }; });
  await run('background_cancel_failure_preserves_obligation', async () => { fixture.cancelMode = 'reject'; await click('#work-open'); await click('#tasks .task button'); const value = await evaluate(`({status:document.querySelector('.message-status[data-status=running]')?.textContent,cancelStillAvailable:!!document.querySelector('#tasks .task button:not([hidden])'),feedback:document.getElementById('hint').textContent})`); fixture.cancelMode = 'normal'; await evaluate(`document.getElementById('work-close').focus()`); await key('Escape'); return { ok: !!value.status && value.cancelStillAvailable && /huỷ|hủy|xác nhận/.test(value.feedback), evidence: value }; });
  await run('completion_status_announcer_semantics', async () => { const before = await evaluate(`document.getElementById('messages').textContent`); seeded[1].status = 'completed'; seeded[1].reply = 'Fixture background result'; push(seeded[1]); await until(() => evaluate(`document.getElementById('messages').textContent.includes('Fixture background result')`)); const evidence = await evaluate(`({newContent:document.getElementById('messages').textContent.includes('Fixture background result'),announcers:[...document.querySelectorAll('[aria-live], [role=status], [role=log], [role=alert]')].filter(e=>getComputedStyle(e).display!=='none'&&!e.hidden).map(e=>({id:e.id,role:e.getAttribute('role'),live:e.getAttribute('aria-live'),text:e.textContent.trim().slice(0,160),containsTranscript:e.id==='messages'||e.contains(document.getElementById('messages'))}))})`); return { ok: evidence.newContent && evidence.announcers.some(a => !a.containsTranscript && /hoàn thành|kết quả|trả lời/i.test(a.text)), evidence: { ...evidence, scope: 'DOM live-region contract only; actual screen-reader speech untested', beforeLength: before.length } }; });
  await run('reduced_motion_computed_styles', async () => { await call('Emulation.setEmulatedMedia', { features: [{ name: 'prefers-reduced-motion', value: 'reduce' }] }); const result = await evaluate(`({scroll:getComputedStyle(document.getElementById('conversation')).scrollBehavior,animations:[...document.querySelectorAll('.sidebar,.work-panel,dialog,.message-status')].map(e=>({class:e.className,animation:getComputedStyle(e).animationName,transition:getComputedStyle(e).transitionDuration}))})`); return { ok: result.scroll === 'auto' && result.animations.every(e => e.animation === 'none' && e.transition.split(',').every(t => parseFloat(t) === 0)), evidence: result }; });
  const submit = async text => { await evaluate(`(()=>{const e=document.getElementById('prompt');e.value=${JSON.stringify(text)};e.dispatchEvent(new Event('input'));document.getElementById('composer').requestSubmit()})()`); await until(() => evaluate(`!document.getElementById('send').disabled`)); };
  await run('Enter_ShiftEnter_composition_guard', async () => {
    const before = fixture.requests.length;
    await evaluate(`(()=>{const p=document.getElementById('prompt');p.value='Fixture keyboard draft';p.focus();p.dispatchEvent(new KeyboardEvent('keydown',{key:'Enter',isComposing:true,bubbles:true,cancelable:true}));p.dispatchEvent(new KeyboardEvent('keydown',{key:'Enter',shiftKey:true,bubbles:true,cancelable:true}))})()`);
    await delay(100); const guarded = fixture.requests.length === before; await key('Enter'); await until(() => fixture.requests.length > before);
    return { ok: guarded && fixture.requests.length === before + 1, evidence: { guarded, plainEnterRequests: fixture.requests.length - before, composition: 'Synthetic isComposing flag only; physical phone IME is pending' } };
  });
  await run('stream_update_preserves_reading_focus_selection', async () => {
    await evaluate(`(()=>{window.__auditAnchor=document.querySelector('.message.user');const c=document.getElementById('conversation');c.scrollTop=0;c.dispatchEvent(new Event('scroll'));const p=document.getElementById('prompt');p.value='Fixture retained selection';p.focus();p.setSelectionRange(4,9)})()`);
    const oldReply = seeded[0].reply; seeded[0].reply += '\\nFixture stream update while reading'; push(seeded[0]); await delay(150);
    const result = await evaluate(`({sameNode:window.__auditAnchor===document.querySelector('.message.user'),top:document.getElementById('conversation').scrollTop,focus:document.activeElement.id,selection:[document.getElementById('prompt').selectionStart,document.getElementById('prompt').selectionEnd],draft:document.getElementById('prompt').value})`);
    seeded[0].reply = oldReply; push(seeded[0]);
    return { ok: result.sameNode && result.top < 2 && result.focus === 'prompt' && result.selection[0] === 4 && result.selection[1] === 9 && result.draft === 'Fixture retained selection', evidence: result };
  });
  await run('mobile_reconnect_status_accessible', async () => {
    const previous = fixture.eventConnections; for (const stream of [...fixture.streams]) stream.end(); await until(() => fixture.eventConnections > previous);
    const connection = await ax('.connection');
    const region = await evaluate(`(()=>{const e=document.getElementById('connection');let current=e;const ancestors=[];while(current&&current!==document.body){ancestors.push({tag:current.tagName,id:current.id,display:getComputedStyle(current).display,ariaHidden:current.getAttribute('aria-hidden')});current=current.parentElement;}return {text:e.textContent,ancestors}})()`);
    return { ok: !connection.ignored && !region.ancestors.some(a => a.display === 'none' || a.ariaHidden === 'true'), evidence: { connection, region, scope: 'DOM/AX exposed status; actual AT delivery requires AT session' } };
  });
  await run('SSE_duplicate_and_gap_resync', async () => {
    const old = fixture.events.at(-1), beforeCount = await evaluate(`document.querySelectorAll('.message.user').length`), snapshots = fixture.snapshotRequests;
    for (const stream of fixture.streams) stream.write(`id: ${old.sequence}\nevent: work\ndata: ${JSON.stringify(old)}\n\n`);
    await delay(150); const duplicateCount = await evaluate(`document.querySelectorAll('.message.user').length`);
    const gap = { sequence: fixture.sequence + 2, type: 'work', work: seeded[0] };
    for (const stream of fixture.streams) stream.write(`id: ${gap.sequence}\nevent: work\ndata: ${JSON.stringify(gap)}\n\n`);
    await until(() => fixture.snapshotRequests > snapshots); await delay(100);
    return { ok: duplicateCount === beforeCount && fixture.snapshotRequests > snapshots, evidence: { beforeCount, duplicateCount, resnapshot: fixture.snapshotRequests > snapshots, scope: 'Client replay handling against fixture; no durable server claim' } };
  });
  await run('outbox_retry_same_nonce', async () => {
    fixture.messageMode = 'reject'; const count = fixture.requests.length; await submit('Fixture retry-identical'); await submit('Fixture retry-identical');
    const entries = await evaluate(`JSON.parse(localStorage.getItem('neko.local-message-outbox.v1')||'[]')`), requests = fixture.requests.slice(count);
    fixture.messageMode = 'normal'; await reload(); await until(() => evaluate(`!localStorage.getItem('neko.local-message-outbox.v1')`));
    return { ok: requests.length >= 2 && entries.length === 1 && requests.every(r => r.clientNonce === entries[0].clientNonce) && fixture.work.filter(w => w.clientNonce === entries[0].clientNonce).length === 1, evidence: { requests: requests.length, queued: entries.length, sameNonce: entries.length === 1 && requests.every(r => r.clientNonce === entries[0].clientNonce), committedFixtureItems: entries.length === 1 ? fixture.work.filter(w => w.clientNonce === entries[0].clientNonce).length : null } };
  });
  await run('outbox_reconnect_without_reload', async () => {
    fixture.messageMode = 'reject'; await submit('Fixture reconnect-only pending'); const queued = await evaluate(`JSON.parse(localStorage.getItem('neko.local-message-outbox.v1')||'[]')`);
    fixture.messageMode = 'normal'; const connections = fixture.eventConnections; for (const stream of [...fixture.streams]) stream.end();
    const reopened = await until(() => fixture.eventConnections > connections); await evaluate(`window.dispatchEvent(new Event('online'))`);
    const cleared = await until(() => evaluate(`!localStorage.getItem('neko.local-message-outbox.v1')`), 2200);
    const committed = queued[0] && fixture.work.filter(w => w.clientNonce === queued[0].clientNonce).length;
    const evidence = { reopened, cleared, queued: queued.length, committedFixtureItems: committed, observationWindowMs: 2200 };
    await reload(); await until(() => evaluate(`!localStorage.getItem('neko.local-message-outbox.v1')`));
    return { ok: reopened && cleared && committed === 1, evidence };
  });
  await run('lost_ack_snapshot_clears_outbox_no_duplicate', async () => {
    fixture.messageMode = 'lost-ack'; await submit('Fixture committed response unavailable');
    const work = fixture.work.find(w => w.text === 'Fixture committed response unavailable'); fixture.messageMode = 'normal'; await reload();
    await until(() => evaluate(`!localStorage.getItem('neko.local-message-outbox.v1')`)); const queued = await evaluate(`JSON.parse(localStorage.getItem('neko.local-message-outbox.v1')||'[]').length`);
    return { ok: !!work && !queued && fixture.work.filter(w => w.clientNonce === work.clientNonce).length === 1, evidence: { queued, committedFixtureItems: work && fixture.work.filter(w => w.clientNonce === work.clientNonce).length, ack: '503 after fixture acceptance; event/snapshot reconcile', scope: 'Mock acceptance only; backend durability is outside this test' } };
  });
  await run('late_ack_preserves_new_bot_and_draft', async () => {
    fixture.messageMode = 'hold'; await evaluate(`document.getElementById('prompt').value='Fixture held response';document.getElementById('composer').requestSubmit()`); await until(() => fixture.held.length);
    await click('#nav-open'); await click('#bots button[data-bot-id="bot-davin"]'); await evaluate(`document.getElementById('prompt').value='Fixture new bot draft'`);
    const held = fixture.held.shift(); fixture.messageMode = 'normal'; held.json(admit(held.input)); await delay(250);
    const actual = await evaluate(`({selected:document.getElementById('bot-name').textContent,draft:document.getElementById('prompt').value})`);
    await click('#nav-open'); await click('#bots button[data-bot-id="bot-meiiie"]');
    return { ok: actual.selected === 'Davin' && actual.draft === 'Fixture new bot draft', evidence: actual };
  });
  await run('initial_snapshot_failure_recovers_online', async () => {
    fixture.snapshotMode = 'fail'; await call('Page.reload'); await delay(250); fixture.snapshotMode = 'normal'; await evaluate(`window.dispatchEvent(new Event('online'))`);
    const recovered = await until(() => evaluate(`document.querySelectorAll('#bots button').length===2`), 2200); const evidence = await evaluate(`({bots:document.querySelectorAll('#bots button').length,hint:document.getElementById('hint').textContent,connection:document.getElementById('connection').textContent})`);
    await reload(); return { ok: recovered, evidence: { ...evidence, observationWindowMs: 2200 } };
  });
  await run('storage_unavailable_startup', async () => {
    const injected = await call('Page.addScriptToEvaluateOnNewDocument', { source: `Object.defineProperty(window,'localStorage',{get(){throw new DOMException('Isolated fixture storage blocked','SecurityError')}})` });
    await call('Page.reload'); const initialized = await until(() => evaluate(`document.querySelectorAll('#bots button').length===2`), 1600);
    const evidence = { initialized, errors: pageErrors.filter(e => /storage blocked/.test(e)) };
    await call('Page.removeScriptToEvaluateOnNewDocument', { identifier: injected.identifier }); await reload(); return { ok: initialized, evidence };
  });
  record('unexpected_page_exceptions', !pageErrors.filter(e => !/storage blocked/.test(e)).length, pageErrors.filter(e => !/storage blocked/.test(e)));
  observations.finalFixture = { requests: fixture.requests.length, workItems: fixture.work.length, snapshots: fixture.snapshotRequests, eventConnections: fixture.eventConnections };
  await call('Browser.close').catch(() => {});
} catch (error) { checks.push({ id: 'audit_execution', status: 'blocked', evidence: error.message }); }
finally {
  socket?.close(); for (const item of fixture.held) { try { item.json({ error: 'fixture-stopped' }, 503); } catch {} }
  for (const stream of fixture.streams) stream.end(); server.closeAllConnections(); await new Promise(resolve => server.close(resolve));
  if (browser.exitCode === null) { await delay(250); if (browser.exitCode === null) browser.kill(); }
  const report = {
    audit: 'Neko mobile/accessibility 2026-10-11', timestampUTC: new Date().toISOString(), commit: options.commit,
    source, sourceIdentity, harnessSha256, sourceHashes: Object.fromEntries([...files].map(([name, bytes]) => [name, { sha256: sha(bytes), bytes: bytes.length }])),
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
