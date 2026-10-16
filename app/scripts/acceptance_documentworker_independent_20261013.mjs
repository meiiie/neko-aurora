import assert from 'node:assert/strict';
import path from 'node:path';
import { readFile, writeFile, mkdir, readdir } from 'node:fs/promises';
import { execFileSync, spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { fileURLToPath } from 'node:url';

const script = fileURLToPath(import.meta.url), root = path.resolve(path.dirname(script), '..');
const args = Object.fromEntries(process.argv.slice(2).flatMap((arg, index, all) => arg.startsWith('--') ? [[arg.slice(2), all[index + 1]]] : []));
const repo = 'E:/CodexScratch/office-worker-20261013/combined-source';
const base = 'E:/CodexScratch/ux-independent-20261013';
const git = 'C:/Program Files/Git/cmd/git.exe';
const hash = bytes => createHash('sha256').update(bytes).digest('hex');
const inheritedNames = ['SystemRoot', 'WINDIR', 'COMSPEC', 'PATHEXT', 'PATH', 'PROCESSOR_ARCHITECTURE', 'NUMBER_OF_PROCESSORS'];
const ownNames = ['USERPROFILE', 'HOMEDRIVE', 'HOMEPATH', 'APPDATA', 'LOCALAPPDATA', 'TEMP', 'TMP', 'GOCACHE', 'GOMODCACHE', 'GOTMPDIR'];
const fixedNames = ['NODE_OPTIONS', 'NODE_NO_WARNINGS', 'GIT_CONFIG_NOSYSTEM', 'GIT_CONFIG_GLOBAL', 'GIT_OPTIONAL_LOCKS', 'GOENV', 'GOWORK', 'GOPROXY', 'GOSUMDB', 'GOTOOLCHAIN', 'CGO_ENABLED'];
assert.match(args.commit ?? '', /^[a-f0-9]{40}$/);
assert.match(args['executable-sha256'] ?? '', /^[a-f0-9]{64}$/);
assert.match(args.label ?? '', /^[a-zA-Z0-9_-]{1,80}$/);
assert.ok(path.isAbsolute(args.executable ?? '') && path.isAbsolute(args.fixture ?? ''));
assert.equal(path.resolve(args.fixture), path.resolve('E:/CodexScratch/office-worker-20261013/published-fixtures/ordinary.docx'));
const output = path.resolve(base, 'documentworker-independent-' + args.label);

if (!args.child) {
  await mkdir(output, { recursive: false });
  for (const name of ['profile', 'cache', 'temp', 'gocache', 'gomodcache', 'gotmp', 'state']) await mkdir(path.join(output, name));
  const env = Object.fromEntries(inheritedNames.flatMap(key => process.env[key] === undefined ? [] : [[key, process.env[key]]]));
  Object.assign(env, { USERPROFILE: path.join(output, 'profile'), HOMEDRIVE: 'E:', HOMEPATH: path.join(output, 'profile').slice(2), APPDATA: path.join(output, 'cache'), LOCALAPPDATA: path.join(output, 'cache'), TEMP: path.join(output, 'temp'), TMP: path.join(output, 'temp'), GOCACHE: path.join(output, 'gocache'), GOMODCACHE: path.join(output, 'gomodcache'), GOTMPDIR: path.join(output, 'gotmp'), NODE_OPTIONS: '', NODE_NO_WARNINGS: '1', GIT_CONFIG_NOSYSTEM: '1', GIT_CONFIG_GLOBAL: 'NUL', GIT_OPTIONAL_LOCKS: '0', GOENV: 'off', GOWORK: 'off', GOPROXY: 'off', GOSUMDB: 'off', GOTOOLCHAIN: 'local', CGO_ENABLED: '0' });
  const childArgs = [script, ...process.argv.slice(2), '--child', output];
  const run = spawnSync(process.execPath, childArgs, { cwd: root, env, windowsHide: true, timeout: 240000, maxBuffer: 8 * 1024 * 1024, encoding: 'utf8' });
  const command = { executable: process.execPath, executableSHA256: hash(await readFile(process.execPath)), args: childArgs, cwd: root, envNames: Object.keys(env).sort(), ownPaths: Object.fromEntries(ownNames.map(key => [key, env[key]])), exitCode: run.status, signal: run.signal, error: run.error?.message ?? null };
  for (const [name, value] of [['command.json', JSON.stringify(command, null, 2) + '\n'], ['stdout.json', run.stdout ?? ''], ['stderr.txt', run.stderr ?? '']]) await writeFile(path.join(output, name), value, { flag: 'wx' });
  process.stdout.write(run.stdout ?? ''); if (run.stderr) process.stderr.write(run.stderr); if (run.error || run.status !== 0) process.exitCode = run.status || 2;
} else {
  assert.equal(path.resolve(args.child), output);
  const env = Object.fromEntries([...inheritedNames, ...ownNames, ...fixedNames].flatMap(key => process.env[key] === undefined ? [] : [[key, process.env[key]]]));
  const readGit = gitArgs => execFileSync(git, gitArgs, { cwd: repo, env, windowsHide: true, maxBuffer: 8 * 1024 * 1024 });
  assert.equal(readGit(['rev-parse', 'HEAD']).toString('ascii').trim(), args.commit);
  assert.equal(readGit(['status', '--short']).toString('utf8').trim(), '');
  assert.equal(hash(await readFile(args.executable)), args['executable-sha256']);
  const fixture = await readFile(args.fixture);
  assert.equal(hash(fixture), 'f0355169286a15993e8dc0f54641f812a1e6ca482ca241a0dd24b0a7df8d135e');
  assert.ok(fixture.length > 0 && fixture.length <= 1 << 20);
  await writeFile(path.join(output, 'fixture.docx'), fixture, { flag: 'wx' });

  const packages = ['documentworker', 'documents', 'ledger', 'connections', 'executors', 'protocol', 'identity', 'historyexport', 'retentionplan', 'syncstream', 'runtime', 'web'];
  const scopes = ['native/go.mod', ...packages.map(name => 'native/internal/' + name)];
  const entries = readGit(['ls-tree', '-r', '--name-only', args.commit, '--', ...scopes]).toString('utf8').trim().split('\n').filter(name => !name.endsWith('_test.go') && !name.includes('/testdata/'));
  const sourceRoot = path.join(output, 'source'), manifest = [];
  await mkdir(sourceRoot);
  const objects = execFileSync(git, ['cat-file', '--batch'], { cwd: repo, env, windowsHide: true, input: Buffer.from(entries.map(name => args.commit + ':' + name).join('\n') + '\n'), maxBuffer: 16 * 1024 * 1024 });
  let offset = 0;
  for (const name of entries) {
    const end = objects.indexOf(10, offset); assert.notEqual(end, -1);
    const [objectId, type, count] = objects.subarray(offset, end).toString('ascii').split(' '); assert.equal(type, 'blob');
    const size = Number(count), bytes = objects.subarray(end + 1, end + 1 + size); offset = end + 1 + size; assert.equal(objects[offset++], 10);
    const target = path.join(sourceRoot, name); await mkdir(path.dirname(target), { recursive: true }); await writeFile(target, bytes, { flag: 'wx' });
    manifest.push({ commit: args.commit, path: name, objectId, bytes: size, sha256: hash(bytes) });
  }
  assert.equal(offset, objects.length);
  const extraPaths = ['docs/local-document-worker-contract-20261013.md', 'native/cmd/neko-control/main.go'];
  const sources = {};
  for (const name of extraPaths) { const bytes = readGit(['show', args.commit + ':' + name]); sources[name] = bytes.toString('utf8'); manifest.push({ commit: args.commit, path: name, bytes: bytes.length, sha256: hash(bytes) }); const target = path.join(sourceRoot, name); await mkdir(path.dirname(target), { recursive: true }); await writeFile(target, bytes, { flag: 'wx' }); }
  const processProbe = await readFile(path.join(root, 'scripts/acceptance_documentworker_process_20261013.go'));
  const authorityProbe = await readFile(path.join(root, 'scripts/acceptance_documentworker_authority_20261013.go'));
  await writeFile(path.join(sourceRoot, 'native/internal/documentworker/acceptance_independent_process_20261013_test.go'), processProbe, { flag: 'wx' });
  await writeFile(path.join(sourceRoot, 'native/internal/ledger/acceptance_independent_authority_20261013_test.go'), authorityProbe, { flag: 'wx' });
  await mkdir(path.join(sourceRoot, 'native/internal/ledger/testdata'));
  await writeFile(path.join(sourceRoot, 'native/internal/ledger/testdata/independent-valid.docx'), fixture, { flag: 'wx' });

  const go = path.resolve(root, '../.tooling/go/bin/go.exe'), goRuns = [];
  for (const [label, filter, pkg] of [['process', '^TestIndependentWorker', './internal/documentworker'], ['authority', '^TestIndependentDocumentWorker', './internal/ledger']]) {
    const goArgs = ['test', '-json', '-count=1', '-timeout=40s', '-run', filter, pkg], cwd = path.join(sourceRoot, 'native');
    const run = spawnSync(go, goArgs, { cwd, env, windowsHide: true, timeout: 120000, encoding: 'utf8', maxBuffer: 8 * 1024 * 1024 });
    await writeFile(path.join(output, 'go-' + label + '.jsonl'), run.stdout ?? '', { flag: 'wx' }); await writeFile(path.join(output, 'go-' + label + '.stderr.txt'), run.stderr ?? '', { flag: 'wx' });
    const events = (run.stdout ?? '').split('\n').filter(Boolean).flatMap(line => { try { return [JSON.parse(line)]; } catch { return []; } });
    const checks = events.filter(event => event.Test && ['pass', 'fail', 'skip'].includes(event.Action)).map(event => ({ name: event.Test, status: event.Action.toUpperCase() }));
    goRuns.push({ label, command: { executable: go, args: goArgs, cwd }, exitCode: run.status, error: run.error?.message ?? null, checks });
  }

  const checks = [];
  const check = async (name, action) => { try { checks.push({ name, status: 'PASS', observed: await action() ?? null }); } catch (error) { checks.push({ name, status: 'FAIL', error: error.message }); } };
  const main = sources[extraPaths[1]], ledgerExtractor = (await readFile(path.join(sourceRoot, 'native/internal/ledger/file_text.go'))).toString('utf8');
  await check('source-main-worker-dispatch-before-flag-or-store', () => { const workerDispatch = main.indexOf('argument == "--document-worker-v1"'), stateFlag = main.indexOf('flag.String("state"'); assert.ok(workerDispatch >= 0 && stateFlag >= 0 && workerDispatch < stateFlag); assert.ok(main.includes('if len(os.Args) != 2')); assert.ok(main.includes('os.Exit(documentworker.Run(os.Stdin, os.Stdout))')); });
  await check('source-main-mandatory-self-client-composition', () => { assert.ok(main.includes('documentworker.NewSelf()')); assert.ok(main.includes('ledger.OpenWithDocumentWorker(*state, documentClient)')); assert.equal(main.includes('ledger.Open(*state)'), false); assert.equal(/flag\.(?:String|StringVar)\([^\n]*(?:document-worker-path|document-worker-executable)/.test(main), false); });
  await check('source-upload-preview-tool-use-same-extractor', async () => { const uploadSource = (await readFile(path.join(sourceRoot, 'native/internal/ledger/projects.go'))).toString('utf8'), toolSource = (await readFile(path.join(sourceRoot, 'native/internal/ledger/runtime.go'))).toString('utf8'); for (const [format, mime] of [['docx', 'docxMIME'], ['xlsx', 'xlsxMIME'], ['pptx', 'pptxMIME']]) assert.ok(ledgerExtractor.includes('format, mime = "' + format + '", ' + mime)); assert.ok(ledgerExtractor.includes('s.documentExtractor.Extract(format, raw)')); assert.ok(ledgerExtractor.includes('!validDocumentResult(out)')); assert.ok(uploadSource.includes('s.extractFileText(in.Filename, in.Content)')); assert.ok(ledgerExtractor.includes('s.extractFileText(f.Filename, raw)')); assert.ok(ledgerExtractor.includes('s.fileTextBytes(source, raw)')); assert.ok(toolSource.includes('s.preparePinnedFileText(id, epoch, read.FileID)')); assert.ok(toolSource.includes('preparedSource != *source')); assert.ok(toolSource.includes('w.Epoch != epoch')); assert.ok(toolSource.includes('!s.contextAllowed(w)')); });
  const request = (raw, format = 1, claimed = raw.length) => { const frame = Buffer.alloc(9 + raw.length); frame.write('NDW1'); frame[4] = format; frame.writeUInt32BE(claimed, 5); raw.copy(frame, 9); return frame; };
  const productionRuns = [];
  const cases = [
    ['valid-retained-synthetic-DOCX', request(fixture), ['--document-worker-v1'], true],
    ['closed-frame-refuses-trailing-path-metadata', Buffer.concat([request(fixture), Buffer.from('{"path":"fixture-only","runtime":"fixture-only"}')]), ['--document-worker-v1'], false],
    ['closed-frame-refuses-second-frame', Buffer.concat([request(fixture), request(fixture)]), ['--document-worker-v1'], false],
    ['closed-format-refuses-unknown-byte', request(fixture, 4), ['--document-worker-v1'], false],
    ['closed-size-refuses-over-raw-bound', request(fixture, 1, (1 << 20) + 1), ['--document-worker-v1'], false],
    ['private-mode-extra-state-argument-refuses-before-store', request(fixture), ['--document-worker-v1', '--state', 'forbidden-store'], false],
  ];
  for (const [name, input, commandArgs, success] of cases) {
    await check('pinned-executable-' + name, async () => {
      const cwd = path.join(output, 'state', name); await mkdir(cwd);
      const run = spawnSync(args.executable, commandArgs, { cwd, env, windowsHide: true, input, timeout: 18000, maxBuffer: (4 << 20) + 1024 });
      const stdout = run.stdout ?? Buffer.alloc(0), stderr = run.stderr ?? Buffer.alloc(0);
      assert.deepEqual(await readdir(cwd), [], 'worker mode created state in its owned fixture cwd');
      await writeFile(path.join(cwd, 'stdout.bin'), stdout, { flag: 'wx' }); await writeFile(path.join(cwd, 'stderr.bin'), stderr, { flag: 'wx' });
      productionRuns.push({ name, executable: args.executable, executableSHA256: args['executable-sha256'], args: commandArgs, cwd, inputSHA256: hash(input), exitCode: run.status, error: run.error?.message ?? null, stdoutBytes: stdout.length, stdoutSHA256: hash(stdout), stderrBytes: stderr.length, stderrSHA256: hash(stderr) });
      assert.equal(run.error, undefined);
      if (success) { assert.equal(run.status, 0); assert.equal(stdout.subarray(0, 4).toString('ascii'), 'NDR1'); assert.equal(stdout[4], 1); const size = stdout.readUInt32BE(5); assert.ok(size > 0 && size <= 512 << 10); assert.equal(stdout.length, 41 + size); assert.equal(hash(stdout.subarray(41)), stdout.subarray(9, 41).toString('hex')); assert.equal(Buffer.from(stdout.subarray(41).toString('utf8')).equals(stdout.subarray(41)), true); }
      else if (commandArgs.length === 1) { assert.equal(run.status, 2); assert.equal(stdout.length, 41); assert.equal(stdout.subarray(0, 4).toString('ascii'), 'NDR1'); assert.equal(stdout.subarray(4).equals(Buffer.alloc(37)), true); }
      else { assert.equal(run.status, 2); assert.equal(stdout.length, 0); }
      return { exitCode: run.status, stdoutBytes: stdout.length };
    });
  }
  const sourceQuiescent = manifest.every(entry => hash(readGit(['show', entry.commit + ':' + entry.path])) === entry.sha256);
  const sourceHead = readGit(['rev-parse', 'HEAD']).toString('ascii').trim(), sourceStatus = readGit(['status', '--short']).toString('utf8').trim();
  assert.equal(sourceHead, args.commit); assert.equal(sourceStatus, ''); assert.equal(hash(await readFile(args.executable)), args['executable-sha256']);
  const harness = await readFile(script);
  const result = { schema: 1, checkedUtc: new Date().toISOString(), sourceCommit: args.commit, sourceHead, sourceStatus, sourceQuiescent, manifest, executable: { path: args.executable, sha256: args['executable-sha256'] }, fixture: { path: args.fixture, sha256: hash(fixture), bytes: fixture.length }, harnessSHA256: hash(harness), processProbeSHA256: hash(processProbe), authorityProbeSHA256: hash(authorityProbe), node: process.version, goVersion: execFileSync(go, ['version'], { env, windowsHide: true }).toString('utf8').trim(), goRuns, checks, productionRuns, environment: { names: [...inheritedNames, ...ownNames, ...fixedNames].filter(key => process.env[key] !== undefined).sort(), ownPaths: Object.fromEntries(ownNames.map(key => [key, process.env[key]])) }, scope: 'Independent exact-Git Go helper-process/wire tests, authored ledger extractor composition/authority races, pinned real self-executable worker protocol. No writer/broad suite, API listener, Core execution, real profile/note/provider/grant, GUI/native/phone, hard OS sandbox/RSS or cloud/model acceptance. Short helper deadline is fixture supervision evidence; production deadline constant remains 15s.' };
  result.status = sourceQuiescent && goRuns.every(run => run.exitCode === 0 && run.checks.length > 0 && run.checks.every(check => check.status === 'PASS')) && checks.every(check => check.status === 'PASS') ? 'PASS' : 'FAIL';
  for (const [name, value] of [['results.json', JSON.stringify(result, null, 2) + '\n'], ['manifest.json', JSON.stringify(manifest, null, 2) + '\n'], ['harness.mjs', harness], ['process-probe.go', processProbe], ['authority-probe.go', authorityProbe]]) await writeFile(path.join(output, name), value, { flag: 'wx' });
  console.log(JSON.stringify({ ...result, manifest: { files: manifest.length, bytes: manifest.reduce((sum, entry) => sum + entry.bytes, 0) }, evidenceRoot: output }, null, 2));
  if (result.status !== 'PASS') process.exitCode = 1;
}
