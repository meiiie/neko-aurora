import path from 'node:path';
import { readFile, writeFile, mkdir } from 'node:fs/promises';
import { execFileSync, spawnSync } from 'node:child_process';
import { createHash, webcrypto } from 'node:crypto';
import { stripTypeScriptTypes } from 'node:module';
import { createContext, SourceTextModule } from 'node:vm';
import { fileURLToPath } from 'node:url';
import assert from 'node:assert/strict';

const script = fileURLToPath(import.meta.url);
const root = path.resolve(path.dirname(script), '..');
const args = Object.fromEntries(process.argv.slice(2).flatMap((v, i, a) => v.startsWith('--') ? [[v.slice(2), a[i + 1]]] : []));
const hash = value => createHash('sha256').update(value).digest('hex');
const git = args.git ?? 'C:/Program Files/Git/cmd/git.exe';
const wiiiRoot = args['wiii-root'] ?? 'C:/Users/Admin/Documents/Codex/2026-10-11/task/wiii-local-bridge/native-ipc-overlay';
for (const name of ['neko-commit', 'wiii-commit']) if (!/^[a-f0-9]{40}$/.test(args[name] ?? '')) throw Error(`Exact --${name} required`);
if (!/^[A-Za-z0-9_-]{1,80}$/.test(args.label ?? '')) throw Error('Fresh --label required');

if (!args.child) {
  const evidence = path.join('E:/CodexScratch/ux-independent-20261013', 'repair-current-' + args.label);
  await mkdir(evidence, { recursive: false });
  for (const name of ['profile', 'temp', 'cache']) await mkdir(path.join(evidence, name));
  const env = Object.fromEntries(['SystemRoot', 'WINDIR', 'COMSPEC', 'PATHEXT', 'PATH', 'PROCESSOR_ARCHITECTURE', 'NUMBER_OF_PROCESSORS'].flatMap(k => process.env[k] === undefined ? [] : [[k, process.env[k]]]));
  Object.assign(env, { USERPROFILE: path.join(evidence, 'profile'), HOMEDRIVE: 'E:', HOMEPATH: path.join(evidence, 'profile').slice(2), TEMP: path.join(evidence, 'temp'), TMP: path.join(evidence, 'temp'), LOCALAPPDATA: path.join(evidence, 'cache'), APPDATA: path.join(evidence, 'cache'), NODE_OPTIONS: '', NODE_NO_WARNINGS: '1', GIT_CONFIG_NOSYSTEM: '1', GIT_CONFIG_GLOBAL: 'NUL', GIT_OPTIONAL_LOCKS: '0' });
  const commandArgs = ['--experimental-vm-modules', script, ...process.argv.slice(2), '--child', evidence];
  const run = spawnSync(process.execPath, commandArgs, { cwd: root, env, windowsHide: true, encoding: 'utf8', timeout: 30000, maxBuffer: 4 * 1024 * 1024 });
  await writeFile(path.join(evidence, 'stdout.json'), run.stdout ?? '', { flag: 'wx' });
  await writeFile(path.join(evidence, 'stderr.txt'), run.stderr ?? '', { flag: 'wx' });
  const receipt = { executable: process.execPath, executableSHA256: hash(await readFile(process.execPath)), args: commandArgs, cwd: root, envNames: Object.keys(env).sort(), isolatedPaths: Object.fromEntries(['USERPROFILE', 'HOMEDRIVE', 'HOMEPATH', 'TEMP', 'TMP', 'LOCALAPPDATA', 'APPDATA'].map(k => [k, env[k]])), exitCode: run.status, signal: run.signal, error: run.error?.message ?? null };
  await writeFile(path.join(evidence, 'command.json'), JSON.stringify(receipt, null, 2) + '\n', { flag: 'wx' });
  process.stdout.write(run.stdout ?? '');
  if (run.stderr) process.stderr.write(run.stderr);
  if (run.error || run.status !== 0) process.exitCode = run.status || 2;
} else {
  const evidence = path.resolve(args.child);
  const allowed = path.resolve('E:/CodexScratch/ux-independent-20261013', 'repair-current-' + args.label);
  assert.equal(evidence, allowed);
  const manifest = [];
  const source = new Map();
  const readGit = (repo, commit, filename) => execFileSync(git, ['show', commit + ':' + filename], { cwd: repo, maxBuffer: 4 * 1024 * 1024, windowsHide: true });
  const files = [
    'wiii-desktop/src/neko-chill/nekobot-local-protocol.ts',
    'wiii-desktop/src/neko-chill/nekobot-local-bridge.ts',
    'wiii-desktop/src/neko-chill/nekobot-native.ts',
    'wiii-desktop/src/neko-chill/components/NekoBotLocalPanel.tsx',
    'wiii-desktop/src-tauri/src/neko/local_bot_bridge.rs',
    'wiii-desktop/src-tauri/src/commands/neko_bot.rs',
    'wiii-desktop/src-tauri/capabilities/workbench-nekobot-local.json',
    'wiii-desktop/src-tauri/Cargo.toml',
    'wiii-desktop/src-tauri/src/lib.rs',
    'wiii-desktop/src/lib/storage.ts',
  ];
  await mkdir(path.join(evidence, 'source'));
  for (const [repoName, repo, commit, paths] of [
    ['wiii', wiiiRoot, args['wiii-commit'], files],
    ['neko', root, args['neko-commit'], ['native/internal/ledger/adapters.go', 'native/internal/ledger/runtime.go', 'native/internal/ledger/report_delivery.go']],
  ]) for (const filename of paths) {
    const bytes = readGit(repo, commit, filename);
    source.set(filename, bytes.toString('utf8'));
    const saved = path.join(evidence, 'source', repoName, filename);
    await mkdir(path.dirname(saved), { recursive: true });
    await writeFile(saved, bytes, { flag: 'wx' });
    manifest.push({ repo: repoName, commit, path: filename, sha256: hash(bytes), bytes: bytes.length });
  }
  const context = createContext({ TextEncoder, structuredClone, crypto: webcrypto });
  const modulePaths = files.slice(0, 3);
  const modules = new Map(modulePaths.map(filename => [filename, new SourceTextModule(stripTypeScriptTypes(source.get(filename), { mode: 'transform', sourceMap: false }), { context, identifier: filename })]));
  const link = (specifier, parent) => {
    const filename = path.posix.normalize(path.posix.join(path.posix.dirname(parent.identifier), specifier + '.ts'));
    if (!modules.has(filename)) throw Error('Unprovided import ' + specifier);
    return modules.get(filename);
  };
  for (const mod of modules.values()) { if (mod.status === 'unlinked') await mod.link(link); if (mod.status === 'linked') await mod.evaluate({ timeout: 1000 }); }
  const protocol = modules.get(modulePaths[0]).namespace;
  const { NekoBotLocalBridge, emptyNekoBotBridge } = modules.get(modulePaths[1]).namespace;
  const { NekoBotNativeClient, parseNativeStatus } = modules.get(modulePaths[2]).namespace;
  const binding = { sourceApp: 'wiii', sourceSessionId: 'source-a', sourceThreadId: 'thread-a', channelId: 'channel-a', expectedChannelRevision: 1, ownerUserId: 'owner-a', spaceId: 'space-a' };
  const session = { id: 'adapter-a', version: 1, sourceApp: 'wiii', sourceSessionId: binding.sourceSessionId, sourceThreadId: binding.sourceThreadId, channelId: binding.channelId, channelRevision: 1, ownerUserId: binding.ownerUserId, spaceId: binding.spaceId, returnRoute: '/v1/adapters/sessions/adapter-a/replies' };
  const item = n => ({ input: { sourceMessageId: 'message-' + n, botId: 'bot-a', text: 'intent-' + n, lane: 'foreground', clientNonce: 'nonce-original-' + n }, requestId: 'request-' + n, state: 'accepted', errorCode: null });
  const reply = (n, text = 'safe-old-' + n, sequence = 10) => ({ replyId: 'reply-request-' + n, requestId: 'request-' + n, sourceMessageId: 'message-' + n, botId: 'bot-a', role: 'assistant', text, status: 'completed', sequence, artifacts: [] });
  const page = (replies, cursor = 10, overrides = {}) => ({ adapterSessionId: session.id, sourceApp: 'wiii', sourceSessionId: binding.sourceSessionId, sourceThreadId: binding.sourceThreadId, channelId: binding.channelId, ownerUserId: binding.ownerUserId, spaceId: binding.spaceId, channelRevision: 1, returnRoute: session.returnRoute, cursor, replies, ...overrides });
  const seed = count => ({ ...emptyNekoBotBridge(binding, 'registration-original'), adapterSession: session, cursor: 10, outbox: Array.from({ length: count }, (_, i) => item(i + 1)), replies: Array.from({ length: Math.min(count, 2) }, (_, i) => reply(i + 1)) });
  const checks = [];
  const details = {};
  const check = async (family, name, expected, action) => {
    try { const observed = await action(); checks.push({ family, name, status: 'PASS', expected, observed }); }
    catch (error) { checks.push({ family, name, status: 'RED', expected, observed: error.message, code: error.code ?? null }); }
  };
  const expectCode = async (action, code) => {
    try { await action(); throw Error('Expected rejection ' + code); } catch (e) { assert.equal(e.code, code); return e.code; }
  };
  const setup = async (count = 2) => {
    let saved = structuredClone(seed(count)), response = { status: 200, body: JSON.stringify(page(seed(count).replies)) }, failWrite = false;
    const calls = [], writes = [];
    const bridge = new NekoBotLocalBridge(binding, { request: async (...input) => { calls.push(structuredClone(input)); return typeof response === 'function' ? response(...input) : structuredClone(response); } }, {
      read: async () => structuredClone(saved), write: async next => { if (failWrite) throw Error('owned fixture storage denied'); saved = structuredClone(next); writes.push(structuredClone(next)); },
    }, () => 'nonce-authored-next');
    await bridge.hydrate();
    response = { status: 200, body: JSON.stringify(page([], 10)) };
    await bridge.refresh();
    return { bridge, calls, writes, saved: () => structuredClone(saved), respond: value => { response = value; }, writeFails: value => { failWrite = value; } };
  };

  const capacity = await setup(100), original = JSON.stringify(capacity.bridge.snapshot);
  await check('accepted-history-budget', 'new-intent-after-100-accepted', 'An accepted transcript does not consume the entire pending-intent budget needed for >512-row repair.', async () => {
    try { await capacity.bridge.enqueue({ sourceMessageId: 'message-101', botId: 'bot-a', text: 'fresh explicit intent', lane: 'foreground' }); }
    catch (e) { details.capacity = { accepted: 100, pending: 0, error: e.code, executionPOSTs: capacity.calls.filter(c => c[0] === 'POST').length }; throw e; }
    return 'Message 101 queued without execution';
  });
  await check('accepted-history-budget', 'capacity-refusal-keeps-receipts-and-nonces', 'Refusal keeps all prior accepted receipts and original nonces; no POST.', async () => {
    assert.equal(JSON.stringify(capacity.bridge.snapshot), original); assert.equal(capacity.calls.filter(c => c[0] === 'POST').length, 0); return '100 accepted receipts unchanged';
  });

  const conflict = await setup(), beforeConflict = structuredClone(conflict.bridge.snapshot);
  conflict.respond({ status: 409, body: JSON.stringify({ error: 'history_limit', limit: 4096 }) });
  await check('conflict-and-native-drift', 'limit-409-safe-receipts', '409 holds presentation, retains cursor/receipts/nonces, and performs GET only.', async () => {
    await expectCode(() => conflict.bridge.refresh(), 'http_409'); assert.equal(conflict.bridge.snapshot.blocked, true); assert.equal(conflict.bridge.visibleReplies.length, 0);
    assert.equal(conflict.bridge.snapshot.cursor, beforeConflict.cursor); assert.equal(JSON.stringify(conflict.bridge.snapshot.outbox), JSON.stringify(beforeConflict.outbox)); assert.equal(conflict.calls.filter(c => c[0] === 'POST').length, 0);
    details.conflict = { error: conflict.bridge.error, blocked: conflict.bridge.snapshot.blocked, visible: conflict.bridge.visibleReplies.length, calls: conflict.calls.map(c => c.slice(0, 2)) }; return details.conflict;
  });
  await check('conflict-and-native-drift', 'explicit-repair-method', 'A bound history-limit failure exposes an explicit read-only repair operation.', async () => {
    assert.equal(typeof conflict.bridge.repairHistory, 'function', 'Current full module has no repairHistory operation'); return 'repairHistory present';
  });
  const nativeCalls = [], nativeClient = new NekoBotNativeClient(async (...input) => { nativeCalls.push(structuredClone(input)); throw 'neko_bot_local:connection_changed'; });
  const nativeTransport = nativeClient.transport({ leaseId: 'lease-original', source: binding, projectId: 'project-a', channelName: 'channel-a', bots: [{ id: 'bot-a', name: 'Bot A' }] });
  const drift = await setup(); drift.respond((method, route, body) => nativeTransport.request(method, route, body));
  await check('conflict-and-native-drift', 'native-generation-rejection-holds-old-transcript', 'A native connection-generation rejection hides old presentation until the exact binding is reverified.', async () => {
    await expectCode(() => drift.bridge.refresh(), undefined);
    details.nativeDrift = { error: drift.bridge.error, visible: drift.bridge.visibleReplies.map(r => r.text), invokes: nativeCalls };
    assert.equal(drift.bridge.visibleReplies.length, 0, 'Native connection_changed becomes generic transport_unavailable while old replies stay visible'); return details.nativeDrift;
  });

  const wrongBinding = await setup(), beforeBinding = JSON.stringify(wrongBinding.bridge.snapshot);
  wrongBinding.respond({ status: 200, body: JSON.stringify(page([reply(1, 'foreign-page', 11)], 11, { sourceThreadId: 'thread-foreign' })) });
  await check('atomic-page-and-revocation', 'cross-thread-page-no-partial-commit', 'Cross-thread pages reject without changing cache/cursor/receipts/nonces.', async () => {
    await expectCode(() => wrongBinding.bridge.refresh(), 'reply_binding_mismatch'); assert.equal(JSON.stringify(wrongBinding.bridge.snapshot), beforeBinding); return 'Wrong thread rejected atomically';
  });
  const mixedClear = await setup(), beforeClear = structuredClone(mixedClear.bridge.snapshot);
  const clear = { ...reply(1, '', 11), status: 'needs_owner', artifacts: [] };
  mixedClear.respond({ status: 200, body: JSON.stringify(page([clear, reply(999, 'unrecorded same-session result', 12)], 12)) });
  await check('atomic-page-and-revocation', 'mixed-clear-error-keeps-intent-without-partial-cache', 'An unrecorded same-session reply does not fabricate a nonce or commit the earlier clear row partially.', async () => {
    await expectCode(() => mixedClear.bridge.refresh(), 'reply_message_mismatch'); assert.equal(JSON.stringify(mixedClear.bridge.snapshot), JSON.stringify(beforeClear)); return 'No fabricated row or partial commit';
  });
  await check('atomic-page-and-revocation', 'observed-clear-with-rejected-companion-holds-old-text', 'A page that carries an authenticated clear cannot leave that old result visible after a companion-row refusal.', async () => {
    details.mixedClear = { error: mixedClear.bridge.error, cacheCursor: mixedClear.bridge.snapshot.cursor, pageCursor: 12, visible: mixedClear.bridge.visibleReplies.map(r => ({ sourceMessageId: r.sourceMessageId, text: r.text })), outboxCount: mixedClear.bridge.snapshot.outbox.length };
    assert.equal(mixedClear.bridge.visibleReplies.length, 0, 'Rejected companion leaves previously cleared text visible'); return details.mixedClear;
  });
  const storage = await setup(), beforeStorage = JSON.stringify(storage.bridge.snapshot);
  storage.respond({ status: 200, body: JSON.stringify(page([reply(1, 'provisional-new-text', 11)], 11)) }); storage.writeFails(true);
  await check('atomic-page-and-revocation', 'final-storage-failure-and-reload-no-provisional-text', 'Failed persistence leaves prior receipts/nonces and never publishes provisional replacement text; fresh hydrate remains authority-held.', async () => {
    await expectCode(() => storage.bridge.refresh(), undefined); assert.equal(JSON.stringify(storage.bridge.snapshot), beforeStorage);
    assert.equal(storage.bridge.visibleReplies.some(r => r.text === 'provisional-new-text'), false);
    const reloaded = new NekoBotLocalBridge(binding, { request: async () => { throw Error('No reload transport'); } }, { read: async () => storage.saved(), write: async () => {} }, () => 'nonce-reload-original');
    await reloaded.hydrate(); assert.equal(reloaded.visibleReplies.length, 0); assert.equal(JSON.stringify(reloaded.snapshot.outbox), JSON.stringify(storage.bridge.snapshot.outbox)); return 'No provisional text, reload requires authority refresh';
  });

  const controlled = '\u0001'.repeat(65536), encodedRows = [1, 2, 3].map(n => reply(n, controlled, 11 + n));
  const encodedOne = Buffer.byteLength(JSON.stringify(page([encodedRows[0]], 14))), encodedThree = JSON.stringify(page(encodedRows, 14));
  details.byteBudget = { rawTextBytesPerRow: Buffer.byteLength(controlled), rows: 3, encodedSinglePageBytes: encodedOne, encodedThreePageBytes: Buffer.byteLength(encodedThree), frontendLimit: protocol.NEKOBOT_LOCAL_MAX_PAGE_BYTES, nativeLimit: 1048576, backendEncodedPageLimitSourceInspected: 900000, backendReplyRawLimitSourceInspected: 65536, genuineLedgerReceipt: false };
  await check('encoded-valid-text-budget', 'valid-single-row-is-bounded', 'A legitimate 65536-byte control-text row is below the front-end and backend page budget; do not invent an impossible oversized raw reply.', async () => {
    assert.equal(protocol.parseReply(encodedRows[0]).text.length, 65536); assert.equal(encodedOne < 900000, true); return details.byteBudget;
  });
  const large = await setup(3), beforeLarge = JSON.stringify(large.bridge.snapshot); large.respond({ status: 200, body: encodedThree });
  await check('encoded-valid-text-budget', 'aggregate-oversize-rejects-without-partial-cache', 'Three individually valid rows exceed the old page budget; rejection preserves all original intent and cache.', async () => {
    await expectCode(() => large.bridge.refresh(), 'response_oversized'); assert.equal(JSON.stringify(large.bridge.snapshot), beforeLarge); assert.equal(large.calls.filter(c => c[0] === 'POST').length, 0); return 'Authored encoded-page refusal only; real backend would gate before returning this frame';
  });
  await check('encoded-valid-text-budget', 'catalog-capability-extension-needs-current-parser-change', 'Current catalog rejects undeclared repair capability fields, so capability rollout requires an explicit parser/schema update.', async () => {
    try { parseNativeStatus({ connected: true, connecting: false, catalog: { version: 1, ownerUserId: binding.ownerUserId, spaceId: binding.spaceId, channels: [], bots: [], capabilities: ['local_adapter_history_repair_v1'] } }); throw Error('Unexpected capability acceptance'); }
    catch (e) { assert.equal(e.code, 'unsupported_fields'); return 'Exact current catalog parser rejects extension'; }
  });

  const rust = source.get('wiii-desktop/src-tauri/src/neko/local_bot_bridge.rs');
  const panel = source.get('wiii-desktop/src/neko-chill/components/NekoBotLocalPanel.tsx');
  const seams = {
    rustSourceInspectionOnly: true,
    routeVariants: rust.match(/enum Route \{[\s\S]*?\n\}/)?.[0],
    getRoute: rust.includes('.strip_prefix("replies?after=")'),
    errorsEraseBody: rust.includes('body: "{\\"error\\":\\"local_request_rejected\\"}".to_owned()') && rust.includes('return Ok((status, Vec::new()));'),
    generationCheckedAfterAwait: rust.includes('self.current(generation)?;'),
    hydratedPostVerificationUsesAfterZero: rust.includes('&format!("/v1/adapters/sessions/{adapter}/replies?after=0")'),
    panelStaticOnly: true,
    panelRepairAction: /repairHistory|repair-history/.test(panel),
    panelBlockedReconnectWording: panel.includes('Cần nối lại đúng kênh'),
    typedCatalogCapabilitiesAbsent: !source.get(modulePaths[2]).includes('capabilities'),
    overlayBuildLimit: 'Git tree is a source overlay without package.json and referenced composer-drafts module; no full Rust/React app build or installed binary pairing established.',
  };
  const harness = await readFile(script);
  const result = { nekoCommit: args['neko-commit'], wiiiCommit: args['wiii-commit'], sourceMode: 'Exact immutable Git bytes; full protocol/bridge/native TS modules transformed by Node stripTypeScriptTypes and linked in one VM. Rust/React source inspection only.', harnessSHA256: hash(harness), node: process.version, environment: { names: Object.keys(process.env).sort(), ownPaths: Object.fromEntries(['USERPROFILE', 'HOMEDRIVE', 'HOMEPATH', 'TEMP', 'TMP', 'LOCALAPPDATA', 'APPDATA'].map(k => [k, process.env[k]])) }, checks, summary: { pass: checks.filter(c => c.status === 'PASS').length, red: checks.filter(c => c.status === 'RED').length }, details, seams, manifest, sourceUnchanged: manifest.every(m => hash(readGit(m.repo === 'wiii' ? wiiiRoot : root, m.commit, m.path)) === m.sha256), limits: ['Authored in-memory transport/storage fixtures; no genuine ledger or native IPC execution in this runner.', 'No current installed Wiii pairing, browser/native UI, phone, provider/model/cloud acceptance.', 'No actual account/profile/store/8765/grant/bootstrap reads or requests.'] };
  for (const [filename, value] of [['results.json', JSON.stringify(result, null, 2) + '\n'], ['manifest.json', JSON.stringify(manifest, null, 2) + '\n'], ['harness.mjs', harness]]) await writeFile(path.join(evidence, filename), value, { flag: 'wx' });
  console.log(JSON.stringify({ ...result, manifest: { files: manifest.length, bytes: manifest.reduce((n, m) => n + m.bytes, 0) }, evidenceRoot: evidence }, null, 2));
  if (!result.sourceUnchanged || result.summary.red) process.exitCode = 1;
}
