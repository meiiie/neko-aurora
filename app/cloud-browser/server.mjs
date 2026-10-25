import { createServer } from 'node:http';
import { createHash, createHmac, createCipheriv, createDecipheriv, randomBytes, randomUUID, timingSafeEqual } from 'node:crypto';
import { join, resolve, relative, sep } from 'node:path';
import { lstat, realpath, rm } from 'node:fs/promises';
import { pathToFileURL } from 'node:url';
import { fault } from './errors.mjs';
import { openJournal, privateDirectory } from './journal.mjs';
import { allowedURL, createEgress } from './egress.mjs';
import { createPlaywrightDriver } from './workstation.mjs';
import { loginFieldPins, sameFieldPin } from './login-fields.mjs';
import { assertNamespaceQuiescent as inspectNamespaceQuiescence } from './quiescence.mjs';

const fields = new Set('namespace profile kind actor actorId workId workEpoch attempt nonce epoch observationId frameId targetId text key x y deltaY url mode expectedOrigin sealed'.split(' '));
const kinds = new Set('status open observe frame control navigate click fill press scroll close vault_fill'.split(' '));
const reads = new Set(['status', 'observe', 'frame']);
const keys = new Set(['Enter', 'Tab', 'Escape', 'Backspace', 'Delete', 'ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight', 'Home', 'End', 'PageUp', 'PageDown', 'Space', 'Shift+Tab', 'Control+A']);
const sha = input => createHash('sha256').update(input).digest('hex');
const canonical = input => JSON.stringify(Object.fromEntries(Object.entries(input).sort(([a], [b]) => a.localeCompare(b))));
const identifier = value => typeof value === 'string' && /^[A-Za-z0-9_-]{1,120}$/.test(value);
function validate(c, urlAllowed) {
  if (!c || Array.isArray(c) || typeof c !== 'object' || Object.keys(c).some(k => !fields.has(k)) ||
      !/^[a-f0-9]{64}$/.test(c.namespace ?? '') || !/^bot-[A-Za-z0-9_-]{1,100}$/.test(c.profile ?? '') ||
      !kinds.has(c.kind) || !['owner', 'agent'].includes(c.actor) || !identifier(c.actorId)) throw fault('invalid_command', 400);
  if (!reads.has(c.kind) && (typeof c.nonce !== 'string' || !/^[A-Za-z0-9_-]{8,120}$/.test(c.nonce))) throw fault('invalid_nonce', 400);
  for (const k of ['epoch', 'workEpoch']) if (c[k] !== undefined && (!Number.isSafeInteger(c[k]) || c[k] < 1)) throw fault('invalid_command', 400);
  if (c.attempt !== undefined && (typeof c.attempt !== 'string' || !c.attempt.length || c.attempt.length > 160 || /[\u0000-\u001f]/.test(c.attempt))) throw fault('invalid_command', 400);
  for (const k of ['text', 'observationId', 'targetId', 'frameId', 'sealed', 'expectedOrigin', 'url', 'key', 'mode', 'workId'])
    if (c[k] !== undefined && (typeof c[k] !== 'string' || c[k].length > (k === 'sealed' ? 32768 : k === 'text' ? 16000 : 4096))) throw fault('invalid_command', 400);
  for (const k of ['x', 'y', 'deltaY']) if (c[k] !== undefined && (!Number.isFinite(c[k]) || Math.abs(c[k]) > 10000)) throw fault('invalid_command', 400);
  if (c.url !== undefined && !urlAllowed(c.url)) throw fault('url_not_allowed', 400);
  if (c.kind === 'control' && (c.actor !== 'owner' || !['owner', 'agent', 'paused'].includes(c.mode))) throw fault('owner_required', 403);
  if (c.kind === 'frame' && c.actor !== 'owner') throw fault('owner_required', 403);
  if (['open', 'close'].includes(c.kind) && c.actor !== 'owner') throw fault('owner_required', 403);
  if (c.actor === 'agent' && c.kind !== 'status' && (!identifier(c.workId) || !c.workEpoch || !c.attempt)) throw fault('work_required', 403);
  if (c.kind === 'navigate' && !c.url) throw fault('invalid_command', 400);
  if (c.kind === 'fill' && c.text === undefined) throw fault('invalid_command', 400);
  if (c.kind === 'press' && !keys.has(c.key)) throw fault('invalid_key', 400);
  if (c.kind === 'vault_fill') {
    let origin;
    try { origin = new URL(c.expectedOrigin).origin; } catch { throw fault('invalid_origin', 400); }
    if (origin !== c.expectedOrigin || !urlAllowed(c.expectedOrigin) || !c.sealed || !c.observationId) throw fault('invalid_vault_binding', 400);
  }
}
function decrypt(c, key) {
  try {
    if (!c.sealed || !/^[A-Za-z0-9+/]+={0,2}$/.test(c.sealed)) throw Error();
    const bytes = Buffer.from(c.sealed, 'base64');
    if (bytes.length < 29) throw Error();
    const derived = createHmac('sha256', key).update(`aurora-cloud-vault-v1\n${c.namespace}\n${c.profile}`).digest();
    const cipher = createDecipheriv('aes-256-gcm', derived, bytes.subarray(0, 12));
    cipher.setAAD(Buffer.from(`${c.namespace}\n${c.profile}\n${c.nonce}\n${c.expectedOrigin}\n${c.observationId}`));
    cipher.setAuthTag(bytes.subarray(-16));
    const values = JSON.parse(Buffer.concat([cipher.update(bytes.subarray(12, -16)), cipher.final()]).toString('utf8'));
    derived.fill(0); bytes.fill(0);
    if (!values || Array.isArray(values) || Object.keys(values).some(k => !['username', 'password', 'totp'].includes(k)) ||
      !Object.keys(values).length || Object.values(values).some(v => typeof v !== 'string' || v.length > 4096)) throw Error();
    return values;
  } catch { throw fault('sealed_invalid', 400); }
}

function validatePurge(value) {
  if (!value || Array.isArray(value) || typeof value !== 'object' || Object.keys(value).length !== 2 ||
      Object.keys(value).some(k => k !== 'namespace' && k !== 'nonce') || !/^[a-f0-9]{64}$/.test(value.namespace ?? '') ||
      typeof value.nonce !== 'string' || !/^[A-Za-z0-9_-]{8,120}$/.test(value.nonce)) throw fault('invalid_purge', 400);
}
async function purgeDirectory(dataDir, namespace) {
  const root = resolve(dataDir), profiles = join(root, 'profiles'), target = join(profiles, namespace);
  if (relative(root, target) !== `profiles${sep}${namespace}`) throw fault('unsafe_profile_directory');
  const same = (a, b) => process.platform === 'win32' ? a.toLowerCase() === b.toLowerCase() : a === b;
  for (const [path, optional] of [[root, false], [profiles, true], [target, true]]) {
    let info;
    try { info = await lstat(path); }
    catch (error) { if (optional && error.code === 'ENOENT') return null; throw fault('unsafe_profile_directory'); }
    let canonical;
    try { canonical = await realpath(path); } catch { throw fault('unsafe_profile_directory'); }
    if (!info.isDirectory() || info.isSymbolicLink() || !same(canonical, path)) throw fault('unsafe_profile_directory');
  }
  return target;
}

export async function createWorker({ key, dataDir, driver, now = () => Date.now(), idleMs = 600000, ownerLeaseMs = 300000, urlAllowed = allowedURL,
  removeProfiles = rm, assertNamespaceQuiescent = inspectNamespaceQuiescence } = {}) {
  key = Buffer.from(key ?? '');
  if (key.length < 32 || !dataDir) throw fault('invalid_worker_configuration', 500);
  const journal = await openJournal(dataDir);
  const bootId = randomUUID();
  const actors = new Map();
  const purging = new Map();
  let active, proxy, closing = false;
  const browserDriver = driver ?? { async open(options) {
    proxy ??= await createEgress();
    return createPlaywrightDriver({ proxyURL: proxy.url }).open(options);
  } };
  function actor(c) {
    const id = `${c.namespace}/${c.profile}`;
    if (!actors.has(id)) {
      const stored = journal.profile(c.namespace, c.profile);
      actors.set(id, { namespace: c.namespace, profile: c.profile, epoch: stored?.epoch ?? 1,
        control: stored?.control ?? 'paused', status: stored?.status ?? 'stopped', saved: stored?.saved,
        active: false, chain: Promise.resolve(), view: null, page: null, ownerUntil: 0, fence: 0, lastActivity: now(),
        authState: 'unknown', protected: true, secrets: new Set(), origin: '', title: '' });
      const a = actors.get(id);
      const sealed = journal.redactions(c.namespace, c.profile);
      if (sealed) {
        try {
          const bytes = Buffer.from(sealed, 'base64');
          const aesKey = createHmac('sha256', key).update(`aurora-cloud-redaction-v1\n${id}`).digest();
          const cipher = createDecipheriv('aes-256-gcm', aesKey, bytes.subarray(0, 12));
          cipher.setAAD(Buffer.from(id)); cipher.setAuthTag(bytes.subarray(-16));
          a.secrets = new Set(JSON.parse(Buffer.concat([cipher.update(bytes.subarray(12, -16)), cipher.final()]).toString('utf8')));
          aesKey.fill(0); bytes.fill(0);
        } catch { a.redactionUnavailable = true; }
      }
    }
    return actors.get(id);
  }
  function save(a) { journal.save(a); }
  function remember(a, values) {
    for (const value of values) if (value) a.secrets.add(value);
    const raw = JSON.stringify([...a.secrets]);
    if (Buffer.byteLength(raw) > 1048576) throw fault('redaction_capacity');
    const id = `${a.namespace}/${a.profile}`;
    const aesKey = createHmac('sha256', key).update(`aurora-cloud-redaction-v1\n${id}`).digest();
    const iv = randomBytes(12); const cipher = createCipheriv('aes-256-gcm', aesKey, iv);
    cipher.setAAD(Buffer.from(id));
    journal.saveRedactions(a.namespace, a.profile, Buffer.concat([iv, cipher.update(raw), cipher.final(), cipher.getAuthTag()]).toString('base64'));
    aesKey.fill(0);
  }
  function expire(a) {
    if (a.control === 'owner' && now() >= a.ownerUntil) {
      a.control = 'paused'; a.epoch++; a.fence++; a.view = null; save(a);
    }
  }
  function state(a) {
    return { profileId: a.profile, status: a.status, control: a.control, epoch: a.epoch, bootId,
      protected: a.protected, authState: a.authState, origin: a.origin, title: a.title, observationId: a.view?.id, lastSavedAt: a.saved ?? undefined };
  }
  const modelSafe = a => a.status === 'ready' && a.control === 'agent' && a.protected === false &&
    !['login_required', 'reauth_required'].includes(a.authState);
  function redact(a, text) {
    let out = String(text ?? '');
    for (const secret of a.secrets) if (secret) out = out.split(secret).join('[redacted]');
    return out.slice(0, 16384);
  }
  function observePrivacy(a, raw) {
    a.origin = raw.origin; a.title = redact(a, raw.title).slice(0, 200);
    a.protected = Boolean(raw.loginEvidence || raw.usable === false || a.redactionUnavailable);
    if (raw.loginEvidence) a.authState = a.authState === 'authenticated' || a.authState === 'reauth_required' ? 'reauth_required' : 'login_required';
    else if (!a.protected && raw.identity?.verified === true && typeof raw.identity.id === 'string' && raw.identity.id) a.authState = 'authenticated';
    else a.authState = a.authState === 'reauth_required' ? 'reauth_required' : 'unknown';
    if (a.control === 'agent' && (a.protected || ['login_required', 'reauth_required'].includes(a.authState))) {
      a.control = 'paused'; a.epoch++; a.fence++; a.view = null; save(a);
    }
  }
  async function snapshot(a, c, observed) {
    if (!a.page) return state(a);
    const raw = observed ?? await a.page.observe();
    observePrivacy(a, raw);
    const observationId = `${bootId}:${a.epoch}:${randomUUID()}`;
    const targets = (raw.targets ?? []).slice(0, 256).map(t => ({ ...t, id: randomUUID() }));
    a.view = { id: observationId, raw, targets, frame: null };
    if (c.actor === 'agent' && !modelSafe(a)) return state(a);
    return { ...state(a), observationId, text: redact(a, raw.text), targets: targets.map(t => ({ id: t.id,
      role: redact(a, t.role), name: redact(a, t.name), type: t.type, protected: Boolean(t.protected) })) };
  }
  async function closeProfile(a) {
    if (!a.page) return;
    a.view = null; a.fence++; a.epoch++; a.control = 'paused'; a.protected = true;
    try { await a.page.close(); }
    catch { a.status = 'recovery_required'; save(a); throw Object.assign(fault('close_uncertain'), { safeRejection: false }); }
    a.page = null; a.active = false; active = null; a.status = 'stopped'; a.saved = new Date(now()).toISOString(); save(a);
  }
  async function recover(a) {
    a.control = 'paused'; a.protected = true; a.status = 'recovery_required'; a.epoch++; a.fence++; a.view = null;
    try { if (a.page) await closeProfile(a); } catch { a.cleanupUncertain = true; }
    a.status = 'recovery_required'; save(a);
  }
  async function execute(a, c, fence) {
    expire(a); a.lastActivity = now();
    if (c.kind === 'status') {
      if (c.actor === 'agent' && a.control === 'agent' && a.status === 'ready' && a.page) {
        const raw = await a.page.observe(); observePrivacy(a, raw);
        if (a.view && (raw.documentId !== a.view.raw.documentId || raw.origin !== a.view.raw.origin)) a.view = null;
      }
      return state(a);
    }
    if (a.cleanupUncertain) throw fault('recovery_required');
    if (c.kind === 'open') {
      if (a.page) return state(a);
      if (active && active !== a) throw fault('active_profile_limit');
      active = a; a.status = 'starting'; a.active = true; a.control = 'paused'; a.protected = true; a.epoch++; save(a);
      const profiles = join(dataDir, 'profiles'); await privateDirectory(profiles);
      const ns = join(profiles, a.namespace); await privateDirectory(ns);
      const directory = join(ns, a.profile); await privateDirectory(directory);
      try { a.page = await browserDriver.open({ directory, url: c.url ?? 'https://chat.zalo.me/' }); }
      catch (error) {
        if (error.code === 'browser_cleanup_uncertain') {
          a.cleanupUncertain = true; a.status = 'recovery_required'; save(a); throw error;
        }
        a.status = 'unavailable'; a.active = false; active = null; save(a); throw fault('browser_unavailable', 503);
      }
      a.status = 'ready'; save(a); return snapshot(a, c);
    }
    if (c.kind === 'observe') return c.actor === 'agent' && a.control !== 'agent' ? state(a) : snapshot(a, c);
    if (c.kind === 'frame') {
      if (!a.page) throw fault('browser_stopped');
      const raw = await a.page.observe(); observePrivacy(a, raw);
      if (!a.view || raw.documentId !== a.view.raw.documentId || raw.origin !== a.view.raw.origin) await snapshot(a, c, raw);
      const image = await a.page.frame();
      if (Buffer.byteLength(image.data, 'base64') > 1048576) throw fault('frame_too_large');
      const frame = { ...image, id: randomUUID() };
      a.view.frame = frame;
      return { ...state(a), observationId: a.view.id, frame };
    }
    if (c.epoch !== a.epoch) throw fault('stale_epoch');
    if (c.kind === 'control') {
      if (c.mode === 'agent') {
        if (!a.page || a.status !== 'ready') throw fault('browser_stopped');
        if (a.control !== 'owner') throw fault('owner_control_required', 403);
        await snapshot(a, c);
        if (a.protected || ['login_required', 'reauth_required'].includes(a.authState)) throw fault('protected_login', 403);
        expire(a);
        if (c.epoch !== a.epoch || a.control !== 'owner' || fence !== a.fence) throw fault('control_revoked');
      }
      a.control = c.mode; a.epoch++; a.view = null; a.ownerUntil = c.mode === 'owner' ? now() + ownerLeaseMs : 0;
      save(a); return state(a);
    }
    if (c.kind === 'close') { await closeProfile(a); return state(a); }
    if (!a.page) throw fault('browser_stopped');
    if (a.control !== c.actor || fence !== a.fence) throw fault('control_revoked');
    if (c.actor === 'agent' && a.protected) throw fault('protected_login', 403);
    if (c.kind === 'navigate' && c.actor === 'owner') { await a.page.act(c); a.view = null; return state(a); }
    const view = a.view;
    if (!view || c.observationId !== view.id) throw fault('stale_observation');
    const current = await a.page.observe();
    observePrivacy(a, current);
    if (c.actor === 'agent' && !modelSafe(a)) throw fault('protected_login', 403);
    if (current.documentId !== view.raw.documentId || current.origin !== view.raw.origin) throw fault('stale_observation');
    let target;
    if (c.targetId) {
      target = view.targets.find(t => t.id === c.targetId);
      const actual = (current.targets ?? []).find(t => t.key === target?.key);
      if (!target || !actual || ['role', 'name', 'type', 'protected'].some(k => (target[k] ?? '') !== (actual[k] ?? ''))) throw fault('stale_target');
      if (target.protected && c.actor === 'agent' && c.kind !== 'vault_fill') throw fault('protected_target', 403);
    } else if (c.kind === 'click') {
      if (c.actor !== 'owner' || !view.frame || c.frameId !== view.frame.id || c.x === undefined || c.y === undefined ||
          c.x < 0 || c.y < 0 || c.x >= view.frame.width || c.y >= view.frame.height) throw fault('stale_frame');
      const image = await a.page.frame();
      if (image.width !== view.frame.width || image.height !== view.frame.height || image.data !== view.frame.data) throw fault('stale_frame');
    } else if (c.kind === 'fill' || c.kind === 'press' && c.actor === 'agent') throw fault('target_required', 400);
    expire(a);
    if (c.epoch !== a.epoch || a.control !== c.actor || fence !== a.fence) throw fault('control_revoked');
    let values, fieldPins;
    if (c.kind === 'vault_fill') {
      if (!c.expectedOrigin || c.expectedOrigin !== current.origin || new URL(c.expectedOrigin).origin !== c.expectedOrigin) throw fault('origin_mismatch');
      fieldPins = loginFieldPins(view.raw.targets ?? []);
      const currentPins = loginFieldPins(current.targets ?? []);
      if (Object.keys(fieldPins).some(field => !sameFieldPin(fieldPins[field], currentPins[field]))) throw fault('stale_target');
      values = decrypt(c, key); remember(a, Object.values(values));
    }
    if (c.kind === 'fill' && c.actor === 'owner') remember(a, [c.text]);
    try { await a.page.act({ ...c, target: target?.key, values, fieldPins, expectedDocument: current.documentId }); }
    finally { values = null; a.view = null; }
    if (c.actor === 'owner') a.ownerUntil = now() + ownerLeaseMs;
    return state(a);
  }
  async function command(c) {
    validate(c, urlAllowed);
    if (closing) throw fault('worker_closing', 503);
    if (journal.purgeStatus(c.namespace)) throw fault('namespace_purged', 410);
    const a = actor(c);
    const digest = sha(canonical(c));
    // Fence agent queues synchronously, before waiting for the in-flight operation.
    if (c.kind === 'control' && c.actor === 'owner') a.fence++;
    const fence = a.fence;
    const run = async () => {
      if (journal.purgeStatus(c.namespace)) throw fault('namespace_purged', 410);
      if (reads.has(c.kind)) {
        expire(a);
        try { return await execute(a, c, fence); }
        catch (error) { if (!error.safeRejection) await recover(a); throw error; }
      }
      const prior = journal.receipt(c.namespace, c.profile, c.nonce);
      if (prior) {
        if (prior.digest !== digest) throw fault('nonce_conflict');
        return { ...state(a), receipt: { nonce: c.nonce, status: prior.status === 'started' ? 'unknown' : prior.status, code: prior.code } };
      }
      if (c.kind === 'open' && active && active !== a) throw fault('active_profile_limit');
      journal.start(c.namespace, c.profile, c.nonce, digest);
      try {
        const result = await execute(a, c, fence);
        journal.finish(c.namespace, c.profile, c.nonce, 'dispatched');
        return { ...result, receipt: { nonce: c.nonce, status: 'dispatched', code: '' } };
      } catch (err) {
        const status = err.safeRejection ? 'rejected' : 'unknown';
        const code = err.code ?? 'input_outcome_unknown';
        journal.finish(c.namespace, c.profile, c.nonce, status, code);
        if (status === 'unknown') await recover(a);
        return { ...state(a), receipt: { nonce: c.nonce, status, code } };
      }
    };
    const result = a.chain.then(run).then(out => {
      if (c.actor === 'agent') {
        delete out.frame;
        if (!modelSafe(a)) {
          delete out.title; delete out.observationId;
          out.text = a.protected ? 'Protected login or challenge. Owner control required.' : 'Owner control required before agent access.';
          out.targets = [];
        } else if (out.targets) out.targets = out.targets.map(t => t.protected ? { ...t, name: '' } : t);
      }
      return out;
    }); a.chain = result.catch(() => {}); return result;
  }
  async function purge(value) {
    validatePurge(value);
    if (closing) throw fault('worker_closing', 503);
    const { namespace, nonce } = value;
    const ack = () => ({ namespace, nonce, status: 'purged' });
    // FULL synchronous SQLite commit happens before the first await or browser/file cleanup.
    journal.beginPurge(namespace);
    if (purging.has(namespace)) { await purging.get(namespace); return ack(); }
    const owned = [...actors.values()].filter(a => a.namespace === namespace);
    for (const a of owned) {
      a.purging = true; a.fence++; a.epoch++; a.control = 'paused'; a.protected = true; a.view = null;
    }
    const cleanup = (async () => {
      await Promise.all(owned.map(a => a.chain));
      for (const a of owned) {
        if (a.page) {
          try { await closeProfile(a); a.cleanupUncertain = false; }
          catch { throw fault('purge_close_uncertain', 503); }
        }
        if (a.cleanupUncertain) throw fault('purge_close_uncertain', 503);
      }
      const directory = await purgeDirectory(dataDir, namespace);
      await assertNamespaceQuiescent(join(resolve(dataDir), 'profiles', namespace));
      if (directory) {
        try { await removeProfiles(directory, { recursive: true, force: true }); }
        catch { throw fault('purge_cleanup_failed', 503); }
        let absent = false;
        try { await lstat(directory); } catch (error) { absent = error.code === 'ENOENT'; }
        if (!absent) throw fault('purge_cleanup_failed', 503);
      }
      try { journal.completePurge(namespace); }
      catch { throw fault('purge_cleanup_failed', 503); }
      for (const a of owned) {
        a.secrets.clear(); a.title = ''; a.origin = ''; a.view = null;
        actors.delete(`${a.namespace}/${a.profile}`);
      }
    })();
    purging.set(namespace, cleanup);
    try { await cleanup; return ack(); }
    finally { purging.delete(namespace); }
  }
  async function handle(req, res) {
    const send = (status, body) => { res.writeHead(status, { 'Content-Type': 'application/json', 'Cache-Control': 'no-store' }); res.end(JSON.stringify(body)); };
    if (req.method === 'GET' && req.url === '/healthz') return send(200, { healthy: true, protocol: 1 });
    const isPurge = req.url === '/v1/purge';
    if (req.method !== 'POST' || req.url !== '/v1/command' && !isPurge) return send(404, { error: 'not_found' });
    try {
      const timestamp = req.headers['x-aurora-timestamp'];
      const signature = req.headers['x-aurora-signature'];
      if (!/^\d{10}$/.test(timestamp ?? '') || Math.abs(now() / 1000 - Number(timestamp)) > 30 || !/^[a-f0-9]{64}$/.test(signature ?? '')) throw fault('unauthorized', 401);
      const limit = isPurge ? 1024 : 1048576;
      if (Number(req.headers['content-length']) > limit) { req.resume(); throw fault('body_too_large', 413); }
      const chunks = []; let size = 0;
      for await (const part of req) { size += part.length; if (size > limit) throw fault('body_too_large', 413); chunks.push(part); }
      const body = Buffer.concat(chunks);
      const expected = createHmac('sha256', key).update(`${timestamp}\n${sha(body)}`).digest();
      if (!timingSafeEqual(expected, Buffer.from(signature, 'hex'))) throw fault('unauthorized', 401);
      let input; try { input = JSON.parse(body.toString('utf8')); } catch { throw fault('invalid_json', 400); }
      send(200, await (isPurge ? purge(input) : command(input)));
    } catch (err) { send(err.status ?? 500, { error: err.code ?? 'worker_error' }); }
  }
  const idle = setInterval(() => {
    for (const a of actors.values()) if (!a.purging && a.page && now() - a.lastActivity >= idleMs) {
      a.fence++; a.chain = a.chain.then(() => closeProfile(a)).catch(() => {});
    }
  }, Math.min(idleMs, 30000)); idle.unref();
  return { command, purge, handle, async close() {
    if (closing) return; closing = true; clearInterval(idle);
    await Promise.allSettled([...purging.values()]);
    for (const a of actors.values()) { a.fence++; await a.chain; if (a.page) await closeProfile(a); }
    if (proxy) await proxy.close(); await journal.close(); key.fill(0);
  } };
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const worker = await createWorker({ key: process.env.AURORA_CLOUD_BROWSER_KEY, dataDir: process.env.DATA_DIR ?? '/data' });
  const server = createServer(worker.handle);
  server.requestTimeout = 35000; server.headersTimeout = 10000;
  server.listen(Number(process.env.PORT ?? 8080));
  const stop = async () => { server.close(); await worker.close(); };
  process.once('SIGTERM', stop); process.once('SIGINT', stop);
}
