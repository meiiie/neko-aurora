import path from 'node:path';
import { readFile, writeFile, mkdir } from 'node:fs/promises';
import { execFileSync, spawnSync } from 'node:child_process';
import { createHash, createHmac, timingSafeEqual } from 'node:crypto';
import { fileURLToPath } from 'node:url';
import assert from 'node:assert/strict';

const script = fileURLToPath(import.meta.url), root = path.resolve(path.dirname(script), '..');
const options = Object.fromEntries(process.argv.slice(2).flatMap((value, i, argv) => value.startsWith('--') ? [[value.slice(2), argv[i + 1]]] : []));
const sha = bytes => createHash('sha256').update(bytes).digest('hex');
const expectedVectorSHA = '97bd000d35de4a274fcb56b6f60f4938eb79edb70dc82eba9064b4293d3a886d';
if (!/^[a-f0-9]{40}$/.test(options['contract-commit'] ?? '') || !/^[A-Za-z0-9_-]{1,80}$/.test(options.label ?? '')) throw Error('Exact --contract-commit and fresh --label required');

if (!options.child) {
  const output = path.join('E:/CodexScratch/ux-independent-20261013', 'repair-current-vectors-' + options.label);
  await mkdir(output, { recursive: false });
  for (const name of ['profile', 'temp', 'cache']) await mkdir(path.join(output, name));
  const env = Object.fromEntries(['SystemRoot', 'WINDIR', 'COMSPEC', 'PATHEXT', 'PATH', 'PROCESSOR_ARCHITECTURE', 'NUMBER_OF_PROCESSORS'].flatMap(name => process.env[name] === undefined ? [] : [[name, process.env[name]]]));
  Object.assign(env, { USERPROFILE: path.join(output, 'profile'), HOMEDRIVE: 'E:', HOMEPATH: path.join(output, 'profile').slice(2), APPDATA: path.join(output, 'cache'), LOCALAPPDATA: path.join(output, 'cache'), TEMP: path.join(output, 'temp'), TMP: path.join(output, 'temp'), NODE_OPTIONS: '', GIT_CONFIG_NOSYSTEM: '1', GIT_CONFIG_GLOBAL: 'NUL', GIT_OPTIONAL_LOCKS: '0' });
  const argv = [script, ...process.argv.slice(2), '--child', output];
  const run = spawnSync(process.execPath, argv, { cwd: root, env, windowsHide: true, timeout: 15000, encoding: 'utf8', maxBuffer: 1024 * 1024 });
  const receipt = { executable: process.execPath, executableSHA256: sha(await readFile(process.execPath)), args: argv, cwd: root, envNames: Object.keys(env).sort(), ownPaths: Object.fromEntries(['USERPROFILE', 'HOMEDRIVE', 'HOMEPATH', 'APPDATA', 'LOCALAPPDATA', 'TEMP', 'TMP'].map(name => [name, env[name]])), exitCode: run.status, signal: run.signal, error: run.error?.message ?? null };
  for (const [filename, content] of [['command.json', JSON.stringify(receipt, null, 2) + '\n'], ['stdout.json', run.stdout ?? ''], ['stderr.txt', run.stderr ?? '']]) await writeFile(path.join(output, filename), content, { flag: 'wx' });
  process.stdout.write(run.stdout ?? ''); if (run.stderr) process.stderr.write(run.stderr);
  if (run.error || run.status !== 0) process.exitCode = run.status || 2;
} else {
  const output = path.resolve(options.child);
  assert.equal(output, path.resolve('E:/CodexScratch/ux-independent-20261013', 'repair-current-vectors-' + options.label));
  const contract = execFileSync('C:/Program Files/Git/cmd/git.exe', ['show', options['contract-commit'] + ':docs/adapter-history-repair-contract-20261013.md'], { cwd: root, maxBuffer: 1024 * 1024, windowsHide: true });
  assert.match(contract.toString('utf8'), /Status: FROZEN shared contract/);
  const vectors = await readFile(path.join(root, 'scripts/adapter_history_repair_vectors_20261013.json'));
  assert.equal(sha(vectors), expectedVectorSHA, 'Vector source differs from Root-provided fixed hash');
  const authored = JSON.parse(vectors.toString('utf8'));
  assert.equal(authored.version, 1);

  const invalidUnicode = () => { throw Error('reject_invalid_unicode'); };
  const utf8 = value => {
    if (typeof value !== 'string') throw Error('Expected string');
    for (let i = 0; i < value.length; i++) {
      const unit = value.charCodeAt(i);
      if (unit >= 0xd800 && unit <= 0xdbff) {
        const next = value.charCodeAt(++i);
        if (!(next >= 0xdc00 && next <= 0xdfff)) invalidUnicode();
      } else if (unit >= 0xdc00 && unit <= 0xdfff) invalidUnicode();
    }
    return Buffer.from(value, 'utf8');
  };
  const unsigned32 = value => {
    if (!Number.isInteger(value) || value < 0 || value > 0xffffffff) throw Error('Invalid u32');
    const bytes = Buffer.alloc(4); bytes.writeUInt32BE(value); return bytes;
  };
  const unsigned64 = value => {
    if (!Number.isSafeInteger(value) || value < 0) throw Error('Invalid safe u64');
    const bytes = Buffer.alloc(8); bytes.writeBigUInt64BE(BigInt(value)); return bytes;
  };
  const text = value => { const bytes = utf8(value); return Buffer.concat([unsigned32(bytes.length), bytes]); };
  const decodedSHA = value => { if (!/^[a-f0-9]{64}$/.test(value)) throw Error('Invalid SHA'); return Buffer.from(value, 'hex'); };
  const domain = value => utf8(value + '\0');
  const scopeBytes = scope => Buffer.concat([
    ...['adapterSessionId', 'sourceApp', 'sourceSessionId', 'sourceThreadId', 'channelId', 'ownerUserId', 'spaceId', 'returnRoute'].map(name => text(scope[name])),
    unsigned64(scope.channelRevision),
  ]);
  const rowBytes = row => Buffer.concat([
    ...['replyId', 'requestId', 'sourceMessageId', 'botId', 'role', 'status', 'availability'].map(name => text(row[name])),
    unsigned64(row.sequence), text(row.text), unsigned32(row.artifacts.length),
    ...row.artifacts.flatMap(file => [text(file.id), text(file.filename), text(file.sha256), unsigned64(file.bytes)]),
  ]);
  const calculate = sample => {
    if (sample.rows.length !== sample.livePredicates.length || sample.livePredicates.some(value => typeof value !== 'boolean')) throw Error('Predicate alignment');
    const scope = scopeBytes(sample.scope), rows = sample.rows.map(rowBytes);
    const authorityInput = Buffer.concat([domain('neko-adapter-authority-v1'), scope, unsigned64(sample.authorityEpoch), unsigned64(sample.rows.length), ...sample.rows.flatMap((row, i) => [text(row.requestId), Buffer.from([sample.livePredicates[i] ? 1 : 0])])]);
    const authorityDigest = sha(authorityInput);
    const historyInput = Buffer.concat([domain('neko-adapter-history-v1'), scope, unsigned64(sample.snapshotCursor), decodedSHA(authorityDigest), unsigned64(rows.length), ...rows]);
    const historyDigest = sha(historyInput);
    const snapshotInput = Buffer.concat([domain('neko-adapter-snapshot-v1'), scope, unsigned64(sample.snapshotCursor), decodedSHA(authorityDigest), decodedSHA(historyDigest)]);
    const cursorPrefix = Buffer.concat([Buffer.from([1]), decodedSHA(sha(scope)), unsigned64(sample.snapshotCursor), unsigned64(rows.length), unsigned64(sample.issuedAtUnixMs), unsigned64(sample.expiresAtUnixMs), decodedSHA(authorityDigest), decodedSHA(historyDigest)]);
    const fixtureKey = decodedSHA(sample.fixtureSigningKeyHex);
    const cursor = Buffer.concat([cursorPrefix, createHmac('sha256', fixtureKey).update(cursorPrefix).digest()]);
    return { scopeBytes: scope.length, scopeSHA256: sha(scope), authorityInputBytes: authorityInput.length, authorityDigest, rowByteLengths: rows.map(row => row.length), totalBytes: rows.reduce((sum, row) => sum + row.length, 0), historyInputBytes: historyInput.length, historyDigest, snapshotId: 'h1_' + sha(snapshotInput), cursorRawBytes: cursor.length, cursorEncodedChars: cursor.toString('base64url').length, confirmationCursor: cursor.toString('base64url') };
  };
  const checks = [], computed = [];
  const check = (name, action) => { try { checks.push({ name, status: 'PASS', observed: action() }); } catch (error) { checks.push({ name, status: 'FAIL', error: error.message }); } };
  for (const sample of authored.cases) check('vector:' + sample.name, () => {
    const value = calculate(sample); computed.push({ name: sample.name, ...value }); assert.deepEqual(value, sample.expected);
    return { scopeBytes: value.scopeBytes, rowByteLengths: value.rowByteLengths, totalBytes: value.totalBytes, authorityInputBytes: value.authorityInputBytes, historyInputBytes: value.historyInputBytes, rawCursorBytes: value.cursorRawBytes, encodedCursorChars: value.cursorEncodedChars };
  });
  const primary = authored.cases.find(sample => sample.name === 'unicode_control_artifact_order'), reversed = authored.cases.find(sample => sample.name === 'artifact_order_reversed');
  check('artifact-order-is-significant', () => {
    const a = calculate(primary), b = calculate(reversed); assert.equal(a.authorityDigest, b.authorityDigest); assert.equal(a.totalBytes, b.totalBytes); assert.notEqual(a.historyDigest, b.historyDigest); assert.notEqual(a.snapshotId, b.snapshotId); assert.notEqual(a.confirmationCursor, b.confirmationCursor);
    return 'Order changes history/snapshot/cursor, preserves authority and payload length';
  });
  const reorder = value => Array.isArray(value) ? value.map(reorder) : value && typeof value === 'object' ? Object.fromEntries(Object.entries(value).reverse().map(([key, entry]) => [key, reorder(entry)])) : value;
  check('json-object-key-order-is-irrelevant', () => {
    assert.notEqual(JSON.stringify(primary), JSON.stringify(reorder(primary))); assert.deepEqual(calculate(primary), calculate(reorder(primary))); return 'Reversed keys at every object level preserve canonical bytes';
  });
  check('unicode-controls-count-utf8-bytes', () => {
    const value = primary.rows[0].text;
    for (const marker of ['<', '>', '&', '\u2028', '\u2029', '\0', '\n', '猫', '😀']) assert.equal(value.includes(marker), true);
    assert.equal(value.length, 12); assert.equal(utf8(value).length, 20); assert.equal(text(value).readUInt32BE(0), 20); assert.equal(text(value).length, 24);
    assert.equal(domain('neko-adapter-history-v1').at(-1), 0); return '12 UTF-16 units /20 UTF-8 bytes /24 length-prefixed bytes; domains terminate in NUL';
  });
  check('cursor-layout-u64be-hmac-and-215-char-math', () => {
    for (const sample of authored.cases) {
      const value = calculate(sample), raw = Buffer.from(value.confirmationCursor, 'base64url');
      assert.equal(raw.length, 1 + 32 + 4 * 8 + 2 * 32 + 32); assert.equal(value.confirmationCursor.length, Math.ceil(raw.length * 8 / 6)); assert.equal(value.confirmationCursor.length, 215);
      assert.match(value.confirmationCursor, /^[A-Za-z0-9_-]{215}$/); assert.equal(raw[0], 1); assert.equal(raw.subarray(1, 33).toString('hex'), value.scopeSHA256);
      assert.equal(raw.readBigUInt64BE(33), BigInt(sample.snapshotCursor)); assert.equal(raw.readBigUInt64BE(41), BigInt(sample.rows.length)); assert.equal(raw.readBigUInt64BE(49), BigInt(sample.issuedAtUnixMs)); assert.equal(raw.readBigUInt64BE(57), BigInt(sample.expiresAtUnixMs)); assert.equal(sample.expiresAtUnixMs - sample.issuedAtUnixMs, 300000);
      assert.equal(raw.subarray(65, 97).toString('hex'), value.authorityDigest); assert.equal(raw.subarray(97, 129).toString('hex'), value.historyDigest);
      assert.equal(timingSafeEqual(raw.subarray(129), createHmac('sha256', decodedSHA(sample.fixtureSigningKeyHex)).update(raw.subarray(0, 129)).digest()), true);
      const worstPath = '/v1/adapters/sessions/' + 'a'.repeat(160) + '/history?cursor=' + value.confirmationCursor; assert.equal(worstPath.length <= 512, true);
    }
    return '161 raw bytes;129 signed prefix;32 MAC bytes;215 base64url characters;max session route412 bytes';
  });
  for (const sample of authored.invalidCases) check('invalid:' + sample.name, () => {
    if (sample.javascriptTextCodeUnits) assert.throws(() => text(String.fromCharCode(...sample.javascriptTextCodeUnits)), /reject_invalid_unicode/);
    else if (sample.goTextHex) assert.throws(() => new TextDecoder('utf-8', { fatal: true }).decode(Buffer.from(sample.goTextHex, 'hex')));
    else throw Error('Unknown invalid-vector format');
    assert.equal(sample.expected, 'reject_invalid_unicode'); return 'Malformed Unicode rejects instead of replacement encoding';
  });
  const harness = await readFile(script);
  const result = { status: checks.every(check => check.status === 'PASS') ? 'PASS' : 'FAIL', scope: 'Portable contract-vector verification only; independent byte encoder, no owner hash functions or product code invoked.', contractCommit: options['contract-commit'], contractSHA256: sha(contract), vectorSHA256: sha(vectors), vectorSource: 'Single hash-pinned authored vector file, not in a0af Git checkpoint', harnessSHA256: sha(harness), node: process.version, environment: { names: Object.keys(process.env).sort(), ownPaths: Object.fromEntries(['USERPROFILE', 'HOMEDRIVE', 'HOMEPATH', 'APPDATA', 'LOCALAPPDATA', 'TEMP', 'TMP'].map(name => [name, process.env[name]])) }, checks, summary: { pass: checks.filter(check => check.status === 'PASS').length, fail: checks.filter(check => check.status === 'FAIL').length }, computed, restrictions: ['Published fixed synthetic vector signing key only, no production signer or cookie.', 'No baseline rerun, product implementation, service, user state, provider, native UI or Git write.', 'Server denial/storage/reload/native binding acceptance awaits repaired immutable revision pair.'] };
  for (const [filename, contents] of [['results.json', JSON.stringify(result, null, 2) + '\n'], ['contract.md', contract], ['vectors.json', vectors], ['harness.mjs', harness]]) await writeFile(path.join(output, filename), contents, { flag: 'wx' });
  console.log(JSON.stringify({ ...result, evidenceRoot: output }, null, 2)); if (result.status !== 'PASS') process.exitCode = 1;
}
