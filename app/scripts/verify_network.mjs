import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { createServer } from 'node:http';
import { mkdir, writeFile } from 'node:fs/promises';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '../..');
const binary = process.env.AURORA_VERIFY_BINARY || join(root, 'app/.build/neko-aurora-network' + (process.platform === 'win32' ? '.exe' : ''));
const artifacts = process.env.AURORA_VERIFY_ARTIFACTS || join(root, 'app/.build/network');
const state = join(root, 'app/.state/network-acceptance-' + Date.now());
const checks = [];
const bodies = [];
const keepOpen = process.argv.includes('--serve');
let child, baseURL, cookie = '', targetGroupSession = '';
let malformedNetworkSent = false;
let failure;
const processEnv = {};
for (const [name, value] of Object.entries(process.env)) {
  if (/^(PATH|PATHEXT|SYSTEMROOT|WINDIR|TEMP|TMP|USERPROFILE|APPDATA|LOCALAPPDATA|COMSPEC|PROGRAMFILES(?:\(X86\))?|PROGRAMDATA|SYSTEMDRIVE|PROCESSOR_ARCHITECTURE|NUMBER_OF_PROCESSORS)$/i.test(name)) processEnv[name] = value;
}

function pass(name) { checks.push({ name, result: 'PASS' }); console.log('PASS: ' + name); }
const pause = ms => new Promise(resolveWait => setTimeout(resolveWait, ms));
async function until(fn, seconds = 20) {
  const deadline = Date.now() + seconds * 1000;
  while (Date.now() < deadline) {
    const value = await fn();
    if (value) return value;
    await pause(100);
  }
  throw new Error('Acceptance condition timed out');
}

const provider = createServer(async (request, response) => {
  let body = '';
  for await (const chunk of request) body += chunk;
  const parsed = JSON.parse(body);
  bodies.push(parsed);
  const text = parsed.messages.map(message => String(message.content)).join('\n');
  const latest = parsed.messages.filter(message => message.role === 'user').at(-1)?.content ?? '';
  let answer = 'Đã hoàn tất kiểm tra nội bộ.';
  if (String(latest).includes('NETWORK-PEER-SEND') && !text.includes('DỮ LIỆU GIAO TIẾP AGENT.')) {
    const args = { toSessionId: targetGroupSession, text: 'PEER-TRIAL: đây là ghi chú có nguồn từ agent, không phải lời người dùng.', wake: true };
    answer = 'TOOL: ' + JSON.stringify(malformedNetworkSent ? { name: 'network_send_message', ...args } : { name: 'network_send_message', args });
    malformedNetworkSent = true;
  }
  response.writeHead(200, { 'Content-Type': 'text/event-stream' });
  response.end('data: ' + JSON.stringify({ choices: [{ delta: { content: answer }, finish_reason: 'stop' }] }) + '\n\ndata: [DONE]\n\n');
});

async function start() {
  cookie = '';
  child = spawn(binary, ['--state', state, '--listen', '127.0.0.1:0'], {
    cwd: root, windowsHide: true, stdio: ['pipe', 'pipe', 'pipe'], env: { ...processEnv,
      ZAI_API_KEY: 'fixture-key-not-a-real-account', AURORA_AI_BASE_URL: `http://127.0.0.1:${provider.address().port}`,
      AURORA_RUNTIME: 'http', AURORA_COMPUTER: 'off', AURORA_BRIEF_LOOP: 'off', AURORA_BACKUP_EVERY: 'off',
    },
  });
  child.stdout.setEncoding('utf8');
  child.stderr.setEncoding('utf8');
  let output = '';
  let diagnostics = '';
  child.stderr.on('data', text => { diagnostics = (diagnostics + text).slice(-8000); });
  baseURL = await new Promise((resolveReady, reject) => {
    const timeout = setTimeout(() => reject(new Error('Fixture server did not start: ' + diagnostics)), 15000);
    child.once('error', error => { clearTimeout(timeout); reject(error); });
    child.once('exit', code => { clearTimeout(timeout); reject(new Error(`Fixture server exited ${code}: ${diagnostics}`)); });
    child.stdout.on('data', text => {
      output += text;
      const match = output.match(/READY (http:\/\/127\.0\.0\.1:\d+)/);
      if (match) { clearTimeout(timeout); resolveReady(match[1]); }
    });
  });
  const response = await fetch(baseURL + '/app/');
  assert.equal(response.status, 200);
  cookie = response.headers.getSetCookie().map(value => value.split(';')[0]).join('; ');
  assert.ok(cookie);
}

async function stop() {
  if (!child || child.exitCode !== null) return;
  const stopped = new Promise(resolveStop => child.once('exit', resolveStop));
  child.kill();
  await stopped;
  await pause(150);
}

async function http(path, body, expected = 200) {
  const response = await fetch(baseURL + '/app/api' + path, { method: body === undefined ? 'GET' : 'POST',
    headers: { Cookie: cookie, ...(body === undefined ? {} : { 'Content-Type': 'application/json', Origin: baseURL }) },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  assert.equal(response.status, expected, `${path}: ${await response.clone().text()}`);
  return response;
}
const api = async (path, body, expected) => (await http(path, body, expected)).json();

await mkdir(artifacts, { recursive: true });
await new Promise(resolveListen => provider.listen(0, '127.0.0.1', resolveListen));
try {
  await start();
  let directory = await api('/network');
  const davin = directory.sessions.find(session => session.botId === 'bot-davin' && session.conversationId === 'conversation-bot-davin');
  assert.ok(davin);
  assert.equal(directory.limits.maxTotalBytes, 64 * 1024);
  pass('Authenticated directory exposes exact runnable sessions and limits');

  const note = { toSessionId: davin.id, text: 'FYI-CONTEXT-NEKO: số tham chiếu nội bộ là 4827.', wake: false,
    clientNonce: 'acceptance-fyi-0001', attachments: [{ name: 'context.txt', content: 'Coordination note, no real secrets.\n' }] };
  const fyi = await api('/network/messages', note, 202);
  await pause(450);
  assert.equal(bodies.length, 0);
  directory = await api('/network');
  assert.equal(directory.messages.find(message => message.id === fyi.message.id).workId, '');
  pass('FYI persists without admitting Work or calling the model');

  const duplicate = await api('/network/messages', note, 202);
  assert.equal(duplicate.message.id, fyi.message.id);
  assert.equal(duplicate.duplicate, true);
  await api('/network/messages', { ...note, text: 'changed under same nonce' }, 409);
  pass('Retry returns the same message and changed nonce payload conflicts');

  const download = await http(`/network/messages/${fyi.message.id}/files/0`);
  assert.match(download.headers.get('content-type'), /text\/plain/);
  assert.match(download.headers.get('content-disposition'), /attachment/);
  assert.equal(await download.text(), note.attachments[0].content);
  pass('Scoped attachment download returns the accepted text without execution');

  await api(`/network/messages/${fyi.message.id}/ack`, { clientNonce: 'acceptance-read-0001' });
  const read = (await api('/network')).messages.find(message => message.id === fyi.message.id);
  assert.ok(read.readAt);
  assert.equal(read.deliveryState, 'queued');
  assert.equal(bodies.length, 0);
  pass('Owner read receipt does not pretend runtime delivery or start inference');

  await api('/network/messages', { ...note, clientNonce: 'acceptance-spoof-0001', origin: 'human_decision' }, 400);
  await api('/network/messages', { ...note, clientNonce: 'acceptance-file-0001', attachments: [{ name: '.env', content: 'not-a-real-key' }] }, 400);
  pass('Forged provenance and hidden attachment names are rejected');

  const beforeRestartID = davin.id;
  await stop();
  await start();
  directory = await api('/network');
  assert.ok(directory.sessions.some(session => session.id === beforeRestartID));
  assert.ok(directory.messages.some(message => message.id === fyi.message.id && message.readAt));
  assert.equal(bodies.length, 0);
  pass('Server restart preserves stable session, mail and owner read identity');

  const wake = await api(`/network/messages/${fyi.message.id}/wake`, { clientNonce: 'acceptance-wake-0001' });
  assert.notEqual(wake.message.id, fyi.message.id);
  const wakeAgain = await api(`/network/messages/${fyi.message.id}/wake`, { clientNonce: 'acceptance-wake-0001' });
  assert.equal(wakeAgain.message.id, wake.message.id);
  await until(async () => (await api('/network')).messages.find(message => message.id === wake.message.id && message.taskState === 'completed'));
  assert.equal(bodies.length, 1);
  assert.ok(JSON.stringify(bodies[0]).includes('FYI-CONTEXT-NEKO'));
  pass('Explicit wake creates one new task and preserves FYI context on retry');

  const deferred = await api('/network/messages', { toSessionId: davin.id, text: 'NEXT-TURN-FYI: kiểm tra ngữ cảnh có nguồn.', wake: false, clientNonce: 'acceptance-fyi-0002' }, 202);
  await pause(350);
  assert.equal(bodies.length, 1);
  await api('/agents/bot-davin/messages', { text: 'Trả lời một câu ngắn để kiểm tra lượt tự nhiên.', clientNonce: 'acceptance-owner-0001' }, 202);
  await until(async () => (await api('/network')).messages.find(message => message.id === deferred.message.id && message.deliveryState === 'delivered'));
  await until(async () => (await api('/state')).agents.find(agent => agent.id === 'bot-davin' && agent.status === 'done'));
  assert.ok(JSON.stringify(bodies.at(-1)).includes('NEXT-TURN-FYI'));
  pass('Next genuine owner turn consumes durable FYI context without another wake');

  const callsBeforeAttachment = bodies.length;
  const attachedWake = await api('/network/messages', { toSessionId: davin.id, text: 'Tóm tắt file đánh thức vừa đính kèm.', wake: true,
    clientNonce: 'acceptance-attached-wake-0001', attachments: [{ name: 'wake-note.txt', content: 'HUMAN-WAKE-FILE-NEKO-9173' }] }, 202);
  await until(async () => (await api('/network')).messages.find(message => message.id === attachedWake.message.id && message.taskState === 'completed'));
  assert.equal(bodies.length, callsBeforeAttachment + 1);
  assert.ok(JSON.stringify(bodies.at(-1)).includes('HUMAN-WAKE-FILE-NEKO-9173'));
  pass('Human wake attachments reach the model with their attributed source');

  const group = await api('/groups', { name: 'Network acceptance', bots: ['bot-meiiie', 'bot-davin'] }, 201);
  directory = await api('/network');
  const groupID = group.id;
  targetGroupSession = directory.sessions.find(session => session.botId === 'bot-davin' && session.conversationId === groupID)?.id;
  assert.ok(targetGroupSession);
  await api(`/agents/${groupID}/messages`, { text: '@Meiiie NETWORK-PEER-SEND', clientNonce: 'acceptance-group-0001' }, 202);
  await until(async () => (await api('/network')).messages.find(message => message.origin === 'agent' && message.text.startsWith('PEER-TRIAL') && message.taskState === 'completed'));
  const groupMessages = await api(`/agents/${groupID}/messages`);
  assert.ok(groupMessages.some(message => message.role === 'peer' && message.text.includes('PEER-TRIAL')));
  pass('Real runtime network tool routes an attributed agent message inside shared scope');
  const sendCards = groupMessages.flatMap(message => message.tools ?? []).filter(tool => tool.name === 'network_send_message');
  assert.ok(sendCards.some(tool => tool.status === 'failed'));
  assert.ok(sendCards.some(tool => tool.status === 'done'));
  pass('Invalid fields return a recoverable tool error before the valid send proceeds');

  for (const asset of ['/app/network.js', '/app/network.css']) {
    const response = await fetch(baseURL + asset);
    assert.equal(response.status, 200);
  }
  pass('New web assets are actually embedded and served');

  await writeFile(join(artifacts, 'acceptance.json'), JSON.stringify({ result: 'PASS', checks, modelCalls: bodies.length,
    scope: 'Actual Go process, two authenticated HTTP clients across restarts, controlled loopback model, no real provider or hosted deployment.', state, url: baseURL, timestampUTC: new Date().toISOString() }, null, 2));
  console.log(`PASS: ${checks.length} acceptance checks. ${baseURL}/app/`);
  if (keepOpen) {
    console.log('SERVING: controlled fixture for browser verification; interrupt this session to stop.');
    await new Promise(resolveInterrupt => { process.once('SIGINT', resolveInterrupt); process.once('SIGTERM', resolveInterrupt); });
  }
} catch (error) {
  failure = String(error.stack ?? error);
  console.error(failure);
  await writeFile(join(artifacts, 'acceptance.json'), JSON.stringify({ result: 'FAIL', checks, error: failure, state, url: baseURL, timestampUTC: new Date().toISOString() }, null, 2));
  process.exitCode = 1;
} finally {
  await stop();
  await new Promise(resolveClose => provider.close(resolveClose));
}
