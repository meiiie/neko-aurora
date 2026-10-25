import { randomUUID } from 'node:crypto';
import { allowedURL } from './egress.mjs';
import { fault } from './errors.mjs';
import { sameFieldSignature } from './login-fields.mjs';

// Runs inside the browser, always against an existing handle rather than a replacement selector.
function describeControl(element) {
  const rect = element.getBoundingClientRect();
  if (!element.isConnected || rect.width <= 0 || rect.height <= 0 || getComputedStyle(element).visibility === 'hidden') return null;
  const type = (element.getAttribute('type') || '').toLowerCase();
  if (['hidden', 'file'].includes(type) || element.disabled) return null;
  const label = (element.getAttribute('aria-label') || element.labels?.[0]?.innerText ||
    element.getAttribute('placeholder') || element.innerText || element.getAttribute('title') || element.getAttribute('name') || '').slice(0, 200);
  const role = element.getAttribute('role') || (/^(INPUT|TEXTAREA)$/.test(element.tagName) ? 'textbox' : element.tagName === 'A' ? 'link' : element.tagName === 'SELECT' ? 'combobox' : 'button');
  const protectedField = type === 'password' || /password|passcode|one-time|otp|totp|mật khẩu|mã xác|verification/i.test(`${label} ${element.getAttribute('autocomplete') || ''}`);
  return { role, name: label, type, protected: protectedField };
}

function visualDescriptors() {
  const nodes = document.querySelectorAll('img,canvas,iframe,svg,[role="img"]');
  const selected = [];
  for (let i = 0; i < Math.min(nodes.length, 256); i++) {
    const node = nodes[i], rect = node.getBoundingClientRect();
    if (!node.isConnected || rect.width <= 0 || rect.height <= 0 || getComputedStyle(node).visibility === 'hidden') continue;
    const fields = [node.tagName];
    for (const attribute of ['id', 'class', 'alt', 'title', 'aria-label', 'data-testid', 'src']) fields.push((node.getAttribute(attribute) || '').slice(0, 256));
    let parent = node.parentElement;
    for (let depth = 0; parent && depth < 2; depth++, parent = parent.parentElement)
      fields.push(parent.id, parent.getAttribute('class') || '', parent.getAttribute('aria-label') || '');
    selected.push(fields.join(' ').slice(0, 1024));
  }
  return selected;
}

// Only the production worker calls this factory. Optional dependencies are constructor-only harness seams.
export function createPlaywrightDriver({ proxyURL, allowURL = allowedURL, ignoreHTTPSErrors = false, chromium: suppliedChromium,
  captureTimeoutMs = 15000, operationTimeoutMs = 25000, cleanupTimeoutMs = 5000 } = {}) {
  return { async open({ directory, url }) {
    const chromium = suppliedChromium ?? (await import('playwright')).chromium;
    const context = await chromium.launchPersistentContext(directory, {
      headless: true, chromiumSandbox: true, acceptDownloads: false, serviceWorkers: 'block', timeout: 15000,
      viewport: { width: 1280, height: 800 }, deviceScaleFactor: 1, ignoreHTTPSErrors,
      env: { PATH: process.env.PATH ?? '/usr/bin:/bin', LANG: 'C.UTF-8', HOME: directory },
      proxy: proxyURL ? { server: proxyURL, bypass: '<-loopback>' } : undefined,
      args: ['--disable-quic', '--disable-sync', '--disable-extensions', '--disable-background-networking',
        '--disable-component-update', '--no-first-run', '--no-default-browser-check',
        '--force-webrtc-ip-handling-policy=disable_non_proxied_udp', '--disable-features=MediaRouter',
        ...(proxyURL ? ['--proxy-bypass-list=<-loopback>'] : [])],
    });
    let poisoned = false, closed = false, closePromise, busy = false;
    const ensureLive = () => { if (poisoned || closed) throw fault('browser_recovery_required'); };
    async function closeContext() {
      poisoned = true;
      if (!closePromise) closePromise = (async () => {
        let timer;
        try {
          await Promise.race([context.close(), new Promise((_, reject) => {
            timer = setTimeout(() => reject(Object.assign(fault('browser_cleanup_uncertain', 503), { safeRejection: false })), cleanupTimeoutMs);
          })]);
          closed = true;
        } finally { clearTimeout(timer); }
      })();
      await closePromise;
    }
    async function run(operation, budget = operationTimeoutMs) {
      ensureLive();
      if (busy) throw fault('browser_busy');
      busy = true;
      let timer;
      try {
        return await Promise.race([Promise.resolve().then(operation), new Promise((_, reject) => {
          timer = setTimeout(() => {
            poisoned = true;
            reject(Object.assign(fault('browser_operation_timeout', 503), { safeRejection: false, driverPoisoned: true }));
          }, budget);
        })]);
      } catch (error) {
        if (error.driverPoisoned) await closeContext();
        throw error;
      } finally { clearTimeout(timer); busy = false; }
    }
    try {
    let page;
    await run(async () => {
    context.setDefaultTimeout(10000); context.setDefaultNavigationTimeout(20000);
    await context.clearPermissions();
    ensureLive();
    await context.route('**/*', route => allowURL(route.request().url()) ? route.continue() : route.abort('blockedbyclient'));
    ensureLive();
    await context.routeWebSocket('**/*', route => {
      if (allowURL(route.url())) route.connectToServer(); else void Promise.resolve(route.close()).catch(() => {});
    });
    ensureLive();
    page = context.pages()[0] ?? await context.newPage();
    ensureLive();
    for (const other of context.pages()) if (other !== page) { await other.close(); ensureLive(); }
    context.on('page', other => { if (other !== page) void other.close().catch(() => {}); });
    page.on('dialog', dialog => void dialog.dismiss().catch(() => {}));
    page.on('download', download => void download.cancel().catch(() => {}));
    if (!allowURL(url)) throw fault('url_not_allowed', 400);
    await page.goto(url, { waitUntil: 'domcontentloaded' });
    ensureLive();
    });
    let documentId = randomUUID();
    page.on('framenavigated', frame => { if (frame === page.mainFrame()) documentId = randomUUID(); });
    let entries = new Map();
    async function capture() {
      ensureLive();
      const captureDocument = documentId;
      const previous = entries, nextEntries = new Map();
      const priorNodes = [...previous].map(([key, node]) => ({ key, node }));
      let handles = [], committed = false;
      try {
      const collection = await page.evaluateHandle(() => {
        const nodes = document.querySelectorAll('button,input,textarea,select,a[href],[role="button"],[role="textbox"],[contenteditable="true"]');
        const selected = [];
        for (let i = 0; i < Math.min(nodes.length, 256); i++) selected.push(nodes[i]);
        return selected;
      });
      try { handles = [...(await collection.getProperties()).values()].map(value => value.asElement()).filter(Boolean); }
      finally { await collection.dispose(); }
      ensureLive();
      const targets = [];
      for (const handle of handles) {
        ensureLive();
        if (!handle) continue;
        const data = await handle.evaluate(describeControl);
        ensureLive();
        if (!data) { await handle.dispose(); continue; }
        // Preserve keys only for the exact same element, never for a replacement matching its selector.
        let key;
        try { key = await handle.evaluate((node, prior) => prior.find(entry => entry.node === node)?.key, priorNodes); } catch {}
        ensureLive();
        key ??= randomUUID(); nextEntries.set(key, handle); targets.push({ key, ...data });
      }
      const text = await page.evaluate(() => {
        const walker = document.createTreeWalker(document.body || document.documentElement, NodeFilter.SHOW_TEXT);
        let node, count = 0, result = '';
        while ((node = walker.nextNode()) && count++ < 4000 && result.length < 16384) {
          const parent = node.parentElement;
          if (!parent || parent.closest('input,textarea,script,style,[contenteditable="true"],[aria-hidden="true"]')) continue;
          if (!parent.getClientRects().length || getComputedStyle(parent).visibility === 'hidden') continue;
          result += ` ${node.textContent}`;
        }
        return result.replace(/\s+/g, ' ').slice(0, 16384);
      });
      ensureLive();
      const visuals = await page.evaluate(visualDescriptors);
      ensureLive();
      const title = await page.title();
      ensureLive();
      if (captureDocument !== documentId) throw fault('stale_observation');
      for (const handle of previous.values()) { await handle.dispose().catch(() => {}); ensureLive(); }
      if (captureDocument !== documentId) throw fault('stale_observation');
      const sensitiveVisual = Array.isArray(visuals) && visuals.some(value => typeof value === 'string' &&
        /qr[\s_-]*code|login[\s_-]*qr|qr[\s_-]*login|(?:^|[\/_.\s=-])qr(?:$|[\/_.\s?=-])|captcha|verification|challenge|quét\s*mã|xác\s*minh|one[\s_-]*time|\botp\b/i.test(value));
      const loginEvidence = sensitiveVisual || targets.some(t => t.protected) || /quét mã|mã qr|qr code|đăng nhập|log[ -]?in|sign[ -]?in|captcha|xác minh|verification/i.test(text);
      const usable = text.trim().length > 0 || targets.length > 0;
      entries = nextEntries;
      committed = true;
      return { documentId: captureDocument, origin: new URL(page.url()).origin, title, text, targets,
        loginEvidence, usable, identity: null }; // Zalo identity readback is unverified: never invent authenticated.
      } catch (error) {
        if (!poisoned && (captureDocument !== documentId || String(error.message).includes('Execution context was destroyed'))) throw fault('stale_observation');
        throw error;
      } finally {
        if (!committed) await Promise.allSettled(handles.map(handle => handle.dispose()));
      }
    }
    async function act(c) {
      ensureLive();
      if (c.kind === 'navigate') {
        if (!allowURL(c.url)) throw fault('url_not_allowed', 400);
        await page.goto(c.url, { waitUntil: 'domcontentloaded' }); return;
      }
      const target = c.target ? entries.get(c.target) : null;
      if (c.kind === 'click') {
        if (target) await target.click({ timeout: 10000, noWaitAfter: true });
        else await page.mouse.click(c.x, c.y);
      } else if (c.kind === 'fill') {
        if (!target) throw fault('stale_target');
        await target.fill(c.text, { timeout: 10000 });
      } else if (c.kind === 'press') {
        if (target) await target.press(c.key, { timeout: 10000 }); else await page.keyboard.press(c.key);
      } else if (c.kind === 'scroll') await page.mouse.wheel(0, c.deltaY ?? 0);
      else if (c.kind === 'vault_fill') {
        const selected = Object.entries(c.values).filter(([, value]) => value !== '').map(([field, value]) => {
          const pin = c.fieldPins?.[field], element = entries.get(pin?.key);
          if (!pin || !element) throw fault('login_field_missing');
          return { element, pin, value };
        });
        const check = async ({ element, pin }) => {
          ensureLive();
          if (documentId !== c.expectedDocument || new URL(page.url()).origin !== c.expectedOrigin) throw fault('stale_observation');
          const actual = await element.evaluate(describeControl);
          ensureLive();
          if (!sameFieldSignature(pin, actual)) throw fault('stale_target');
        };
        // Validate all approved handles before input, then each exact handle again immediately before fill.
        // Detached controls never cause a new heuristic lookup. Filling never submits or presses Enter.
        for (const entry of selected) await check(entry);
        let filled = false;
        for (const entry of selected) {
          try { await check(entry); }
          catch (error) {
            if (filled) throw Object.assign(Error('vault_fill_outcome_unknown'), { safeRejection: false });
            throw error;
          }
          const { element, value } = entry;
          await element.fill(value, { timeout: 10000 });
          filled = true;
        }
      } else throw fault('invalid_command', 400);
    }
    return {
      observe: () => run(capture, captureTimeoutMs), act: c => run(() => act(c)),
      frame: () => run(async () => {
        const image = await page.screenshot({ type: 'jpeg', quality: 65, timeout: 10000, fullPage: false,
          mask: [page.locator('input,textarea,[contenteditable="true"]')] });
        ensureLive();
        return { mime: 'image/jpeg', data: image.toString('base64'), width: 1280, height: 800 };
      }, captureTimeoutMs),
      close: closeContext,
    };
    } catch (error) {
      try { await closeContext(); }
      catch { throw Object.assign(fault('browser_cleanup_uncertain', 503), { safeRejection: false }); }
      throw error;
    }
  } };
}
