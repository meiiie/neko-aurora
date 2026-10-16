import {spawn} from 'node:child_process';
import {createHash} from 'node:crypto';
import {mkdtemp, readFile, rm} from 'node:fs/promises';
import path from 'node:path';

const [chrome, base] = process.argv.slice(2);
const directory = await mkdtemp(path.resolve('.build/font-browser-'));
let browser, socket, nextId = 0;
const pending = new Map();
const delay = ms => new Promise(resolve => setTimeout(resolve, ms));
const call = (method, params = {}) => new Promise((resolve, reject) => {
  const id = ++nextId;
  pending.set(id, {resolve, reject});
  socket.send(JSON.stringify({id, method, params}));
});
try {
  browser = spawn(chrome, ['--headless=new', '--remote-debugging-port=0', `--user-data-dir=${directory}`, '--no-first-run', '--no-default-browser-check', 'about:blank'], {stdio: 'ignore'});
  let port;
  for (let attempt = 0; attempt < 100; attempt++) {
    try { port = Number((await readFile(path.join(directory, 'DevToolsActivePort'), 'utf8')).split('\n')[0]); break; } catch { await delay(100); }
  }
  if (!port) throw new Error('Owned headless Chrome did not start');
  const targets = await (await fetch(`http://127.0.0.1:${port}/json/list`)).json();
  const target = targets.find(item => item.type === 'page' && item.url === 'about:blank');
  if (!target) throw new Error('Owned browser page missing');
  socket = new WebSocket(target.webSocketDebuggerUrl);
  await new Promise((resolve, reject) => { socket.addEventListener('open', resolve, {once: true}); socket.addEventListener('error', reject, {once: true}); });
  socket.addEventListener('message', event => {
    const result = JSON.parse(event.data);
    const waiter = pending.get(result.id);
    if (waiter) { pending.delete(result.id); result.error ? waiter.reject(new Error(JSON.stringify(result.error))) : waiter.resolve(result.result); }
  });
  await call('Page.enable');
  await call('Runtime.enable');
  await call('Page.navigate', {url: base});
  let ready = false;
  for (let attempt = 0; attempt < 100; attempt++) {
    const result = await call('Runtime.evaluate', {expression: `document.readyState === 'complete' && location.href.startsWith(${JSON.stringify(base)})`, returnByValue: true});
    if (result.result.value) { ready = true; break; }
    await delay(100);
  }
  if (!ready) throw new Error('Owned Go page did not load');
  const names = ['geist-latin-wght-normal.woff2', 'geist-latin-ext-wght-normal.woff2', 'geist-vietnamese-wght-normal.woff2'];
  const result = await call('Runtime.evaluate', {
    expression: `(async () => {
      const violations = [];
      document.addEventListener('securitypolicyviolation', event => violations.push({directive: event.effectiveDirective, blocked: event.blockedURI}));
      const fonts = [];
      for (const [index, name] of ${JSON.stringify(names)}.entries()) {
        const face = new FontFace('NekoFontProbe' + index, 'url(/fonts/' + name + ')', {weight: '100 900'});
        await face.load(); document.fonts.add(face);
        fonts.push({name, status: face.status, available: document.fonts.check('450 16px "NekoFontProbe' + index + '"')});
      }
      const bytes = new Uint8Array(await (await fetch('/fonts/${names[0]}')).arrayBuffer());
      let dataBlocked = false;
      const encoded = btoa(Array.from(bytes, byte => String.fromCharCode(byte)).join(''));
      try { await new FontFace('NekoDataProbe', 'url(data:font/woff2;base64,' + encoded + ')').load(); } catch { dataBlocked = true; }
      await new Promise(resolve => setTimeout(resolve, 50));
      return {fonts, dataBlocked, violations};
    })()`,
    awaitPromise: true,
    returnByValue: true,
  });
  if (result.exceptionDetails) throw new Error(JSON.stringify(result.exceptionDetails));
  const report = result.result.value;
  if (!report.fonts.every(font => font.status === 'loaded' && font.available) || !report.dataBlocked || !report.violations.some(item => item.directive === 'font-src' && item.blocked === 'data')) throw new Error(JSON.stringify(report));
  report.responses = [];
  for (const name of names) {
    const response = await fetch(new URL('/fonts/' + name, base));
    const bytes = new Uint8Array(await response.arrayBuffer());
    if (response.status !== 200 || response.headers.get('content-type') !== 'font/woff2') throw new Error('Actual Go font response rejected');
    report.responses.push({name, status: response.status, contentType: response.headers.get('content-type'), sha256: createHash('sha256').update(bytes).digest('hex')});
  }
  report.status = 'pass';
  report.scope = 'Actual owned Go service CSP and licensed embedded fonts loaded by isolated headless Chrome; no Core, provider, UI visual or phone acceptance';
  console.log(JSON.stringify(report));
  await call('Browser.close');
} finally {
  socket?.close();
  if (browser && browser.exitCode === null) {
    await Promise.race([new Promise(resolve => browser.once('exit', resolve)), delay(1000)]);
    if (browser.exitCode === null) browser.kill();
  }
  if (!directory.startsWith(path.resolve('.build') + path.sep)) throw new Error('Owned profile boundary mismatch');
  await rm(directory, {recursive: true, force: true, maxRetries: 3});
}
