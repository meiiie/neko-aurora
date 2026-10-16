import assert from 'node:assert/strict';
import path from 'node:path';
import { readFile, writeFile, mkdir } from 'node:fs/promises';
import { execFileSync, spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import vm from 'node:vm';
import { fileURLToPath } from 'node:url';

const script = fileURLToPath(import.meta.url);
const args = Object.fromEntries(process.argv.slice(2).flatMap((arg, index, all) => arg.startsWith('--') ? [[arg.slice(2), all[index + 1]]] : []));
const repo = 'E:/CodexScratch/continuity-clear-ui-20261013/source';
const evidenceBase = 'E:/CodexScratch/ux-independent-20261013';
const git = 'C:/Program Files/Git/cmd/git.exe';
const baseline = '0188e2b2be3d9bcfe1f5637da7885251d738d70b';
const hash = bytes => createHash('sha256').update(bytes).digest('hex');
const inheritedNames = ['SystemRoot', 'WINDIR', 'COMSPEC', 'PATHEXT', 'PATH', 'PROCESSOR_ARCHITECTURE', 'NUMBER_OF_PROCESSORS'];
const ownNames = ['USERPROFILE', 'HOMEDRIVE', 'HOMEPATH', 'APPDATA', 'LOCALAPPDATA', 'TEMP', 'TMP'];
const setNames = ['NODE_OPTIONS', 'NODE_NO_WARNINGS', 'GIT_CONFIG_NOSYSTEM', 'GIT_CONFIG_GLOBAL', 'GIT_OPTIONAL_LOCKS'];
assert.match(args.commit ?? '', /^[a-f0-9]{40}$/);
assert.match(args.label ?? '', /^[a-zA-Z0-9_-]{1,80}$/);
assert.ok(['baseline', 'acceptance'].includes(args.mode));
if (args.mode === 'baseline') assert.equal(args.commit, baseline);
const output = path.resolve(evidenceBase, 'continuity-clear-ui-' + args.label);

if (!args.child) {
  await mkdir(output, { recursive: false });
  for (const name of ['profile', 'cache', 'temp', 'state']) await mkdir(path.join(output, name));
  const env = Object.fromEntries(inheritedNames.flatMap(key => process.env[key] === undefined ? [] : [[key, process.env[key]]]));
  Object.assign(env, { USERPROFILE: path.join(output, 'profile'), HOMEDRIVE: 'E:', HOMEPATH: path.join(output, 'profile').slice(2), APPDATA: path.join(output, 'cache'), LOCALAPPDATA: path.join(output, 'cache'), TEMP: path.join(output, 'temp'), TMP: path.join(output, 'temp'), NODE_OPTIONS: '', NODE_NO_WARNINGS: '1', GIT_CONFIG_NOSYSTEM: '1', GIT_CONFIG_GLOBAL: 'NUL', GIT_OPTIONAL_LOCKS: '0' });
  const childArgs = [script, ...process.argv.slice(2), '--child', output];
  const run = spawnSync(process.execPath, childArgs, { cwd: path.dirname(script), env, windowsHide: true, encoding: 'utf8', timeout: 30000, maxBuffer: 4 * 1024 * 1024 });
  const command = { executable: process.execPath, executableSHA256: hash(await readFile(process.execPath)), args: childArgs, cwd: path.dirname(script), envNames: Object.keys(env).sort(), ownPaths: Object.fromEntries(ownNames.map(key => [key, env[key]])), exitCode: run.status, signal: run.signal, error: run.error?.message ?? null };
  for (const [name, value] of [['command.json', JSON.stringify(command, null, 2) + '\n'], ['stdout.json', run.stdout ?? ''], ['stderr.txt', run.stderr ?? '']]) await writeFile(path.join(output, name), value, { flag: 'wx' });
  process.stdout.write(run.stdout ?? '');
  if (run.stderr) process.stderr.write(run.stderr);
  if (run.error || run.status !== 0) process.exitCode = run.status || 2;
} else {
  assert.equal(path.resolve(args.child), output);
  const readGit = gitArgs => execFileSync(git, gitArgs, { cwd: repo, windowsHide: true, maxBuffer: 4 * 1024 * 1024 });
  const paths = ['native/internal/web/assets/app.js', 'native/internal/web/assets/index.html'];
  const sourceBytes = readGit(['show', args.commit + ':' + paths[0]]), htmlBytes = readGit(['show', args.commit + ':' + paths[1]]);
  const source = sourceBytes.toString('utf8'), html = htmlBytes.toString('utf8');
  const sourceLines = source.split(/\r?\n/);
  const checks = [];
  const check = async (name, action, feature = false) => {
    try { const observed = await action(); checks.push({ name, status: 'PASS', observed: observed ?? null }); }
    catch (error) { checks.push({ name, status: args.mode === 'baseline' && feature ? 'RED' : 'FAIL', error: error.message }); }
  };
  const functionNames = ['cacheKey', 'cacheGet', 'cacheSet', 'cacheRemove', 'sessionOwner', 'fixtureSession', 'sessionCurrent', 'requireSession', 'requestRaw', 'request', 'laneKey', 'projectChoice', 'viewKey', 'sharedChannel', 'privateConversation', 'activeChannel', 'selectedProject', 'canonicalJSON', 'journalSensitive', 'policyEnabled', 'continuityKey', 'acceptContinuityDescriptor', 'validPolicyMarker', 'reloadPolicyMarkers', 'savePolicyMarkers', 'policyMarkerLock', 'retainPolicyCommand', 'pendingPolicy', 'retirePolicyCommand', 'policyFocusFallback', 'runPolicyCommand', 'continuityEdited', 'clearContinuityEditor', 'fillContinuityEditor', 'continuityURL', 'continuityEditorCurrent', 'continuityDraftEditable', 'renderContinuityCommand', 'clearContinuityDraft', 'readContinuity', 'readContinuityDraft'];
  const excerpts = new Map();
  for (const name of functionNames) {
    const lines = sourceLines.filter(line => line.startsWith('function ' + name + '(') || line.startsWith('async function ' + name + '('));
    assert.ok(lines.length <= 1, 'ambiguous actual-source function ' + name);
    if (lines.length) excerpts.set(name, lines[0]);
    else assert.ok(['continuityDraftEditable', 'clearContinuityDraft'].includes(name), 'missing required actual-source function ' + name);
  }
  const eventLines = sourceLines.filter(line => line.startsWith('$("continuity-clear-draft").addEventListener(') || line.startsWith('$("continuity-form").addEventListener("submit"') || line.startsWith('$("continuity-open").addEventListener(') || line.startsWith('$("continuity-close").addEventListener('));
  assert.equal(eventLines.filter(line => line.startsWith('$("continuity-form")')).length, 1);
  const elementIds = [...html.matchAll(/\bid="([^"]+)"/g)].map(match => match[1]);
  const deferred = () => { let resolve, reject; const promise = new Promise((yes, no) => { resolve = yes; reject = no; }); return { promise, resolve, reject }; };
  const plain = value => JSON.parse(JSON.stringify(value));
  const tick = async () => { for (let i = 0; i < 8; i++) await Promise.resolve(); };

  function fresh() {
    const elements = new Map(), memory = new Map(), calls = { fetch: [], uuid: [], storage: [], locks: [] }, document = { activeElement: null };
    function element(id) {
      return { id, value: '', disabled: false, hidden: false, open: false, textContent: '', children: [], listeners: new Map(), focus(options) { document.activeElement = this; this.focusOptions = options; }, contains(candidate) { return this === candidate || this.children.includes(candidate); }, replaceChildren(...children) { this.children = children; }, addEventListener(type, callback) { const prior = this.listeners.get(type) ?? []; prior.push(callback); this.listeners.set(type, prior); }, async dispatch(type, event = {}) { for (const callback of this.listeners.get(type) ?? []) await callback({ target: this, preventDefault() {}, ...event }); }, showModal() { this.open = true; }, close() { this.open = false; for (const callback of this.listeners.get('close') ?? []) callback({ target: this }); } };
    }
    for (const id of elementIds) elements.set(id, element(id));
    const $ = id => elements.get(id) ?? null;
    document.getElementById = $;
    const decision = { id: 'decision-a', text: { value: 'original decision' }, status: { value: 'active' }, date: { value: '2026-10-13' }, entity: { value: 'entity-a' }, selectedRefs: new Set(['reference-a']) };
    const reference = { id: 'reference-a', title: { value: 'original reference' }, date: { value: '2026-10-11' }, entity: { value: 'entity-a' }, version: { value: 'version-a' }, kind: { value: 'url' }, url: { value: 'https://reference.invalid/original' } };
    const context = vm.createContext({ $, document, console: { log() {} }, TextEncoder, URLSearchParams, AbortController, AbortSignal,
      localStorage: { getItem(key) { return memory.get(key) ?? null; }, setItem(key, value) { calls.storage.push({ operation: 'set', key, value }); memory.set(key, value); }, removeItem(key) { calls.storage.push({ operation: 'remove', key }); memory.delete(key); } },
      navigator: { locks: { async request(key, callback) { calls.locks.push(key); return callback(); } } },
      crypto: { randomUUID() { const nonce = 'independent-save-nonce-' + (calls.uuid.length + 1); calls.uuid.push(nonce); return nonce; } },
      fetch(url, options) { const wait = deferred(), call = { url, method: options.method ?? 'GET', body: options.body ?? null, signal: options.signal, markerBeforeFetch: memory.get('neko.local-policy-review.v1') ?? null, wait }; calls.fetch.push(call); return wait.promise; },
      state: { capabilities: ['local_project_continuity_v1', 'local_projects_v1'], readOnly: false, botId: 'bot-meiiie', channelId: 'channel-a', channels: [{ id: 'channel-a', name: 'channel a', revision: 3, botIds: ['bot-meiiie'] }], conversations: [], projectChoices: { 'channel-a': { id: 'project-a', revision: 2 } }, projects: [{ id: 'project-a', name: 'project a', revision: 2, channelIds: ['channel-a'] }] },
      sessionState: { ready: true, generation: 4, metadata: { mode: 'local_owner', actorId: 'local-owner', cacheNamespace: 'local-owner' }, controllers: new Set() },
      navigationGeneration: 7, panelGeneration: 1, continuityGeneration: 9, continuityReadGeneration: 11,
      continuityTarget: { projectId: 'project-a', channelId: 'channel-a', expectedProjectRevision: 2, expectedChannelRevision: 3, expectedRevision: 5, editorGeneration: 9, valid: true },
      continuityReturn: { key: 'channel-a/project/project-a', generation: 7 }, continuityDecisionCards: new Map([['decision-a', decision]]), continuityReferenceCards: new Map([['reference-a', reference]]), continuityDescriptors: new Map([['project-a/channel-a', { projectId: 'project-a', channelId: 'channel-a', revision: 5, sha256: 'fixture-digest' }]]),
      policyMarkers: new Map(), policyCommands: new Map(), policyStorageError: false, policyMarkerKey: 'neko.local-policy-review.v1',
      reconcile(container, children) { container.replaceChildren(...children); }, policyNotice(marker) { const notice = element('authored-policy-notice'); notice.textContent = marker.clientNonce; return notice; }, renderPolicyUI() { if ($('continuity-dialog').open) context.renderContinuityCommand(); }, continuityScopedFiles() { return []; }, addContinuityReference() { throw Error('Unexpected nonempty late reference population'); }, addContinuityDecision() { throw Error('Unexpected nonempty late decision population'); }, containFocus() {}, restorePolicyDialogFocus() {},
      invalidateSession() { throw Error('Unexpected 401/binding fixture path'); }, sync() { throw Error('Unexpected background sync'); }, retrySync() {}, queueMicrotask,
    });
    for (const text of excerpts.values()) vm.runInContext(text, context, { timeout: 1000 });
    for (const text of eventLines) vm.runInContext(text, context, { timeout: 1000 });
    $('continuity-dialog').open = true;
    $('continuity-instructions').value = 'original local draft';
    $('continuity-decisions').children = [element('old-decision-card')];
    $('continuity-references').children = [element('old-reference-card')];
    context.renderContinuityCommand();
    const reply = (call, body, status = 200) => call.wait.resolve({ status, ok: status >= 200 && status < 300, headers: { get() { return null; } }, async json() { return body; } });
    const remote = (patch = {}) => ({ usable: true, continuity: { version: 1, projectId: 'project-a', channelId: 'channel-a', revision: 5, sha256: 'fixture-digest', instructions: 'late remote content', decisions: [], references: [], ...patch } });
    const draft = () => plain(context.readContinuityDraft());
    const actions = () => ({ nonce: calls.uuid.length, get: calls.fetch.filter(call => call.method === 'GET').length, post: calls.fetch.filter(call => call.method === 'POST').length, storage: calls.storage.length, lock: calls.locks.length });
    return { context, $, calls, memory, document, reply, remote, draft, actions };
  }

  await check('explicit-clear-label-type-and-default-disabled', () => { const button = html.match(/<button\b[^>]*\bid="continuity-clear-draft"[^>]*>[^<]*<\/button>/)?.[0]; assert.ok(button, 'missing explicit draft-clear control'); assert.match(button, /type="button"/); assert.match(button, /\bdisabled\b/); assert.match(button, /X\u00f3a b\u1ea3n nh\u00e1p/); }, true);
  await check('actual-clear-click-handler-is-registered', () => { const f = fresh(); assert.equal(typeof f.context.clearContinuityDraft, 'function', 'missing actual draft-clear function'); assert.equal(f.$('continuity-clear-draft')?.listeners.get('click')?.[0], f.context.clearContinuityDraft); }, true);

  await check('late-GET-after-input-edit-keeps-new-local-draft', async () => { const f = fresh(), pending = f.context.readContinuity(); assert.equal(f.$('continuity-save').disabled, true); if (f.$('continuity-clear-draft')) assert.equal(f.$('continuity-clear-draft').disabled, true); f.$('continuity-instructions').value = 'new authored local draft'; f.context.continuityEdited(); f.reply(f.calls.fetch[0], f.remote()); await pending; assert.equal(f.$('continuity-instructions').value, 'new authored local draft'); assert.equal(f.context.continuityTarget.valid, false); assert.equal(f.actions().post, 0); });
  await check('late-GET-after-navigation-ABA-cannot-fill-editor', async () => { const f = fresh(), pending = f.context.readContinuity(); f.context.navigationGeneration++; f.context.continuityReturn.generation = f.context.navigationGeneration; f.$('continuity-instructions').value = 'new draft after returning to same scope'; f.reply(f.calls.fetch[0], f.remote()); await pending; assert.equal(f.$('continuity-instructions').value, 'new draft after returning to same scope'); assert.equal(f.context.continuityTarget.valid, false); assert.equal(f.actions().post, 0); });
  await check('late-GET-after-new-editor-object-cannot-touch-new-target', async () => { const f = fresh(), pending = f.context.readContinuity(), old = f.context.continuityTarget; f.context.continuityTarget = { ...old, expectedRevision: 8, editorGeneration: 12, valid: false }; f.$('continuity-instructions').value = 'fresh editor object draft'; f.reply(f.calls.fetch[0], f.remote()); await pending; assert.equal(f.$('continuity-instructions').value, 'fresh editor object draft'); assert.equal(f.context.continuityTarget.expectedRevision, 8); assert.equal(f.context.continuityTarget.valid, false); });
  await check('actual-request-session-epoch-fences-late-GET', async () => { const f = fresh(), pending = f.context.readContinuity(); assert.equal(f.context.sessionState.controllers.size, 1); f.context.sessionState.generation++; f.$('continuity-instructions').value = 'draft in new session epoch'; f.reply(f.calls.fetch[0], f.remote()); await pending; assert.equal(f.$('continuity-instructions').value, 'draft in new session epoch'); assert.equal(f.context.continuityTarget.valid, false); assert.equal(f.context.sessionState.controllers.size, 0); assert.equal(f.actions().post, 0); });
  await check('wrong-scope-GET-refuses-without-body-or-command', async () => { const f = fresh(), pending = f.context.readContinuity(); f.reply(f.calls.fetch[0], f.remote({ channelId: 'other-channel' })); await pending; assert.equal(f.$('continuity-instructions').value, 'original local draft'); assert.equal(f.context.continuityTarget.valid, false); assert.equal(f.actions().post, 0); assert.equal(f.actions().storage, 0); });

  if (args.mode === 'acceptance') {
    assert.equal(typeof fresh().context.clearContinuityDraft, 'function');
    await check('clear-is-local-preserves-CAS-and-focuses-instructions', async () => { const f = fresh(), scope = plain(f.context.continuityTarget), descriptor = plain([...f.context.continuityDescriptors]); await f.$('continuity-clear-draft').dispatch('click'); assert.deepEqual(f.draft(), { instructions: '', decisions: [], references: [] }); for (const key of ['projectId', 'channelId', 'expectedRevision', 'expectedProjectRevision', 'expectedChannelRevision', 'valid']) assert.equal(f.context.continuityTarget[key], scope[key]); assert.deepEqual(plain([...f.context.continuityDescriptors]), descriptor); assert.equal(f.context.continuityGeneration, 10); assert.equal(f.context.continuityReadGeneration, 12); assert.equal(f.context.continuityTarget.editorGeneration, 10); assert.deepEqual(f.actions(), { nonce: 0, get: 0, post: 0, storage: 0, lock: 0 }); assert.equal(f.document.activeElement, f.$('continuity-instructions')); assert.deepEqual(plain(f.$('continuity-instructions').focusOptions), { preventScroll: true }); });
    await check('inflight-GET-clear-noop-then-edit-fences-response', async () => { const f = fresh(), original = f.draft(), pending = f.context.readContinuity(); await f.$('continuity-clear-draft').dispatch('click'); assert.deepEqual(f.draft(), original); f.$('continuity-instructions').value = 'new draft while read is outstanding'; f.context.continuityEdited(); f.reply(f.calls.fetch[0], f.remote()); await pending; assert.equal(f.$('continuity-instructions').value, 'new draft while read is outstanding'); assert.equal(f.context.continuityTarget.valid, false); assert.equal(f.actions().nonce, 0); assert.equal(f.actions().post, 0); });
    await check('later-GET-wins-older-GET-and-cannot-overwrite-clear', async () => { const f = fresh(), first = f.context.readContinuity(), second = f.context.readContinuity(); f.reply(f.calls.fetch[1], f.remote()); await second; assert.equal(f.context.continuityTarget.valid, true); await f.$('continuity-clear-draft').dispatch('click'); f.reply(f.calls.fetch[0], f.remote({ instructions: 'obsolete first read' })); await first; assert.deepEqual(f.draft(), { instructions: '', decisions: [], references: [] }); assert.equal(f.actions().post, 0); });
    const refusals = [
      ['project-scope', f => { f.context.state.projectChoices['channel-a'] = { id: 'project-other', revision: 2 }; }],
      ['channel-scope', f => { f.context.state.channelId = 'channel-other'; }],
      ['project-revision', f => { f.context.state.projects[0].revision++; }],
      ['channel-revision', f => { f.context.state.channels[0].revision++; }],
      ['navigation-generation', f => { f.context.navigationGeneration++; }],
      ['descriptor-revision', f => { f.context.continuityDescriptors.get('project-a/channel-a').revision++; }],
      ['capability', f => { f.context.state.capabilities = []; }],
      ['unloaded-editor', f => { f.context.continuityTarget.valid = false; }],
      ['storage-error', f => { f.context.policyStorageError = true; }],
      ['closed-dialog', f => { f.$('continuity-dialog').open = false; }],
    ];
    for (const [name, change] of refusals) await check('clear-refuses-' + name + '-without-side-effects', async () => { const f = fresh(), original = f.draft(), generations = [f.context.continuityGeneration, f.context.continuityReadGeneration]; change(f); await f.$('continuity-clear-draft').dispatch('click'); assert.deepEqual(f.draft(), original); assert.deepEqual([f.context.continuityGeneration, f.context.continuityReadGeneration], generations); assert.deepEqual(f.actions(), { nonce: 0, get: 0, post: 0, storage: 0, lock: 0 }); assert.equal(f.document.activeElement, null); });
    await check('pending-original-exact-body-and-nonce-survive-clear', async () => { const f = fresh(), body = { channelId: 'channel-a', expectedProjectRevision: 2, expectedChannelRevision: 3, expectedRevision: 5, clientNonce: 'original-pending-nonce', ...f.draft() }; const command = await f.context.retainPolicyCommand('continuity', 'project-a', body, '/v1/projects/project-a/continuity'); const protectedBody = JSON.stringify(command.body), marker = JSON.stringify([...f.context.policyMarkers]), storage = f.memory.get('neko.local-policy-review.v1'), before = f.actions(); f.context.renderContinuityCommand(); await f.$('continuity-clear-draft').dispatch('click'); assert.equal(JSON.stringify(command.body), protectedBody); assert.equal(JSON.stringify([...f.context.policyMarkers]), marker); assert.equal(f.memory.get('neko.local-policy-review.v1'), storage); assert.equal(command.marker.clientNonce, body.clientNonce); assert.deepEqual(f.draft(), { instructions: body.instructions, decisions: body.decisions, references: body.references }); assert.deepEqual(f.actions(), before); assert.equal(f.$('continuity-save').disabled, true); assert.equal(f.$('continuity-clear-draft').disabled, true); });
    for (const control of ['continuity-save', 'continuity-clear-draft']) await check('disabled-' + control + '-focus-stays-in-dialog', () => { const f = fresh(); f.$(control).focus(); f.context.policyStorageError = true; f.context.renderContinuityCommand(); assert.equal(f.$(control).disabled, true); assert.equal(f.document.activeElement, f.$('continuity-close')); assert.deepEqual(plain(f.$('continuity-close').focusOptions), { preventScroll: true }); });
    await check('clear-then-actual-Save-retains-before-one-empty-CAS-POST', async () => { const f = fresh(); await f.$('continuity-clear-draft').dispatch('click'); assert.equal(f.actions().post, 0); assert.equal(f.actions().nonce, 0); const submit = f.$('continuity-form').dispatch('submit'); await tick(); const posts = f.calls.fetch.filter(call => call.method === 'POST'); assert.equal(posts.length, 1); assert.equal(f.calls.uuid.length, 1); const post = posts[0], body = JSON.parse(post.body); assert.equal(post.url, '/v1/projects/project-a/continuity'); assert.deepEqual(body, { channelId: 'channel-a', expectedProjectRevision: 2, expectedChannelRevision: 3, expectedRevision: 5, clientNonce: 'independent-save-nonce-1', instructions: '', decisions: [], references: [] }); assert.equal(JSON.parse(post.markerBeforeFetch)[0].clientNonce, body.clientNonce); const command = f.context.policyCommands.get(body.clientNonce); assert.deepEqual(plain(command.body), body); await f.$('continuity-clear-draft').dispatch('click'); await f.$('continuity-form').dispatch('submit'); assert.equal(f.actions().post, 1); assert.equal(f.actions().nonce, 1); assert.deepEqual(plain(command.body), body); f.reply(post, { continuity: { version: 1, projectId: 'project-a', channelId: 'channel-a', revision: 6, sha256: 'empty-result-digest', instructions: '', decisions: [], references: [] } }); await submit; assert.equal(f.context.policyCommands.size, 0); assert.equal(f.context.policyMarkers.size, 0); assert.equal(f.memory.has('neko.local-policy-review.v1'), false); assert.equal(f.context.continuityTarget.valid, false); assert.equal(f.actions().post, 1); });
    await check('failed-empty-Save-keeps-original-empty-body-and-nonce', async () => { const f = fresh(); await f.$('continuity-clear-draft').dispatch('click'); const submit = f.$('continuity-form').dispatch('submit'); await tick(); const post = f.calls.fetch.find(call => call.method === 'POST'), body = JSON.parse(post.body); f.reply(post, {}, 409); await submit; const command = f.context.policyCommands.get(body.clientNonce); assert.equal(command.blocked, true); assert.deepEqual(plain(command.body), body); const before = f.actions(); await f.$('continuity-clear-draft').dispatch('click'); await f.$('continuity-form').dispatch('submit'); assert.deepEqual(f.actions(), before); assert.equal(f.context.policyMarkers.size, 1); assert.equal(command.marker.clientNonce, body.clientNonce); });
  }

  const manifest = paths.map((name, index) => ({ commit: args.commit, path: name, sha256: hash(index ? htmlBytes : sourceBytes), bytes: index ? htmlBytes.length : sourceBytes.length }));
  const sourceQuiescent = manifest.every(entry => hash(readGit(['show', entry.commit + ':' + entry.path])) === entry.sha256);
  const head = readGit(['rev-parse', 'HEAD']).toString('ascii').trim(), worktree = readGit(['status', '--short']).toString('utf8').trim();
  if (args.mode === 'acceptance') { assert.equal(head, args.commit); assert.equal(worktree, ''); }
  const harness = await readFile(script);
  const result = { schema: 1, sourceCommit: args.commit, baselineCommit: baseline, mode: args.mode, checkedUtc: new Date().toISOString(), status: checks.some(check => check.status === 'FAIL') ? 'FAIL' : checks.some(check => check.status === 'RED') ? 'RED' : 'PASS', checks, summary: { pass: checks.filter(check => check.status === 'PASS').length, red: checks.filter(check => check.status === 'RED').length, fail: checks.filter(check => check.status === 'FAIL').length }, manifest, sourceQuiescent, sourceHeadAtSeal: head, sourceStatusAtSeal: worktree, harnessSHA256: hash(harness), selectedFunctionManifest: [...excerpts].map(([name, text]) => ({ name, sha256: hash(text), bytes: Buffer.byteLength(text) })), selectedEventManifest: eventLines.map(text => ({ sha256: hash(text), bytes: Buffer.byteLength(text) })), environment: { node: process.version, names: [...inheritedNames, ...ownNames, ...setNames].filter(key => process.env[key] !== undefined).sort(), ownPaths: Object.fromEntries(ownNames.map(key => [key, process.env[key]])) }, scope: 'Exact-Git bounded named function/event VM fixtures using authored DOM, localStorage, locks and fetch. Actual requestRaw session checks and actual policy retain/run/retire execute. No full browser startup, native UI/phone/AT, real note/journal/provider/grant, or cloud/model acceptance.' };
  assert.equal(sourceQuiescent, true);
  for (const [name, value] of [['app.js', sourceBytes], ['index.html', htmlBytes], ['harness.mjs', harness], ['results.json', JSON.stringify(result, null, 2) + '\n']]) await writeFile(path.join(output, name), value, { flag: 'wx' });
  console.log(JSON.stringify({ ...result, selectedFunctionManifest: { functions: result.selectedFunctionManifest.length }, selectedEventManifest: { eventLines: eventLines.length }, evidenceRoot: output }, null, 2));
  if (result.status !== 'PASS') process.exitCode = 1;
}
