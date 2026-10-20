/**
 * Zalo Web bridge for neko-bot (local-first, session = user-scanned QR).
 *
 *   login      — open a Chrome window with a persistent profile at Zalo
 *                Web; the user scans the QR once.
 *   wait-login — attach over CDP and watch until the QR has been scanned;
 *                writes {loggedIn:true} to the status file.
 *   listen     — watch the open chat for new messages (observer) and
 *                report them; keeps {listening:true} in the status file.
 *   send "<t>" — type into the currently open conversation.
 *   probe      — dump candidate DOM selectors (iteration aid).
 */
import puppeteer from 'puppeteer-core';
import { writeFileSync, readFileSync, existsSync } from 'node:fs';

const MODE = process.argv[2] || 'login';
const TEXT = process.argv.slice(3).join(' ');
const CHROME = 'C:/Program Files/Google/Chrome/Application/chrome.exe';
const PROFILE = 'D:/NekoBot/neko-bot/app/.state/local/zalo-profile';
const STATUS = 'D:/NekoBot/neko-bot/app/.state/local/zalo-status.json';
const DEBUG_PORT = 9223;
const ZALO = 'https://chat.zalo.me/';
const SLEEP = ms => new Promise(r => setTimeout(r, ms));
const log = s => console.log(new Date().toISOString().slice(11, 19), s);

function writeStatus(patch) {
  try {
    const current = existsSync(STATUS) ? JSON.parse(readFileSync(STATUS, 'utf8')) : {};
    writeFileSync(STATUS, JSON.stringify({ ...current, ...patch, updatedAt: Date.now() }));
  } catch {}
}

async function launch() {
  const { spawn } = await import('node:child_process');
  const child = spawn(CHROME, [
    `--remote-debugging-port=${DEBUG_PORT}`,
    `--user-data-dir=${PROFILE}`,
    '--no-first-run', '--no-default-browser-check', '--start-maximized',
    ZALO,
  ], { detached: true, stdio: 'ignore' });
  child.unref();
  for (let i = 0; i < 20; i++) {
    await SLEEP(500);
    try {
      const r = await fetch(`http://127.0.0.1:${DEBUG_PORT}/json/version`);
      if (r.ok) { log('chrome up, CDP ready'); return true; }
    } catch {}
  }
  log('chrome CDP not ready');
  return false;
}

async function attach() {
  const browser = await puppeteer.connect({ browserURL: `http://127.0.0.1:${DEBUG_PORT}`, defaultViewport: null });
  const pages = await browser.pages();
  let page = pages.find(p => p.url().includes('zalo.me'));
  if (!page) { page = await browser.newPage(); await page.goto(ZALO, { waitUntil: 'domcontentloaded' }); }
  return { browser, page };
}

const INPUT_SELECTORS = [
  '#chatView div[contenteditable="true"]',
  'div[contenteditable="true"][data-placeholder]',
  '#richInput',
  'div[aria-label*="tin nhắn" i]',
];
const CHAT_SELECTORS = ['#chatView', '#chatViewContainer', 'div[class*="chat-view" i]'];
// After a successful QR scan the app leaves the login page and renders the
// conversation shell; any of these counts as "logged in".
const LOGGED_IN_HINTS = [
  '#chatView', '#chatViewContainer',
  'div[class*="conv-item" i]', 'div[class*="conversation" i]',
];

async function loggedIn(page) {
  return page.evaluate(hints => {
    if (location.hostname === 'id.zalo.me') return false;
    for (const sel of hints) if (document.querySelector(sel)) return true;
    return document.querySelectorAll('[contenteditable="true"]').length > 0;
  }, LOGGED_IN_HINTS).catch(() => false);
}

if (MODE === 'login') {
  const ok = await launch();
  process.exit(ok ? 0 : 1);
}

if (MODE === 'probe') {
  const { browser, page } = await attach();
  const info = await page.evaluate((inputs, chats) => {
    const found = sels => sels.filter(s => document.querySelector(s));
    return JSON.stringify({
      url: location.href, title: document.title,
      inputFound: found(inputs), chatFound: found(chats),
      contenteditables: document.querySelectorAll('[contenteditable="true"]').length,
    });
  }, INPUT_SELECTORS, CHAT_SELECTORS);
  log('probe: ' + info);
  browser.disconnect();
  process.exit(0);
}

if (MODE === 'wait-login') {
  writeStatus({ loggedIn: false });
  for (;;) {
    try {
      const { browser, page } = await attach();
      if (await loggedIn(page)) {
        writeStatus({ loggedIn: true });
        log('QR scanned — logged in');
        browser.disconnect();
        process.exit(0);
      }
      browser.disconnect();
    } catch {
      // window not up yet; keep waiting
    }
    await SLEEP(2500);
  }
}

if (MODE === 'send') {
  const { browser, page } = await attach();
  const input = await page.$(INPUT_SELECTORS.join(',')).catch(() => null);
  if (!input) { log('no chat input found — open a conversation first'); browser.disconnect(); process.exit(2); }
  await input.click();
  await page.keyboard.type(TEXT, { delay: 15 });
  await SLEEP(250);
  await page.keyboard.press('Enter');
  log('sent: ' + TEXT.slice(0, 60));
  browser.disconnect();
  process.exit(0);
}

if (MODE === 'listen') {
  writeStatus({ listening: true });
  const { browser, page } = await attach();
  log('listening on: ' + await page.title());
  await page.exposeFunction('nekoOnZaloMessage', async payload => {
    log('incoming: ' + JSON.stringify(payload).slice(0, 200));
    // v2: POST to the neko-bot adapter endpoint here.
  });
  await page.evaluate(() => {
    const seen = new Set();
    const scan = () => {
      const root = document.querySelector('#chatView') || document.body;
      root.querySelectorAll('div[id^="id_msg_"], div[data-id], div[class*="message-container" i]').forEach(el => {
        const id = el.id || el.getAttribute('data-id');
        const text = (el.innerText || '').trim();
        if (id && text && !seen.has(id) && text.length < 4000) {
          seen.add(id);
          const outgoing = Boolean(el.closest('[class*="outgoing" i], [class*="send" i]'));
          window.nekoOnZaloMessage({ id, outgoing, text: text.slice(0, 500), at: Date.now() });
        }
      });
    };
    scan();
    new MutationObserver(() => scan()).observe(document.body, { childList: true, subtree: true });
  });
  process.on('SIGTERM', () => { writeStatus({ listening: false }); process.exit(0); });
  setInterval(() => {}, 60000);
}
