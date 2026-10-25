import test from 'node:test';
import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { createHash, createHmac } from 'node:crypto';
import { mkdtemp, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { createWorker } from '../server.mjs';

test('private command HTTP requires a fresh HMAC of the exact body and rejects extra fields', async t => {
  const key = 'fixture-http-key-not-a-real-credential-12345';
  const dataDir = await mkdtemp(join(tmpdir(), 'aurora-wire-'));
  const worker = await createWorker({ key, dataDir, driver: { open: async () => { throw Error('must never launch'); } } });
  const server = createServer(worker.handle); await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  const url = `http://127.0.0.1:${server.address().port}`;
  t.after(async () => { await new Promise(resolve => server.close(resolve)); await worker.close(); await rm(dataDir, { recursive: true, force: true }); });
  const input = { namespace: 'c'.repeat(64), profile: 'bot-wire', actor: 'owner', actorId: 'fixture', kind: 'status' };
  async function send(value, timestamp = Math.floor(Date.now() / 1000).toString(), tamper = false) {
    const body = JSON.stringify(value);
    const signature = createHmac('sha256', key).update(`${timestamp}\n${createHash('sha256').update(body).digest('hex')}`).digest('hex');
    return fetch(`${url}/v1/command`, { method: 'POST', headers: { 'Content-Type': 'application/json',
      'X-Aurora-Timestamp': timestamp, 'X-Aurora-Signature': signature }, body: tamper ? `${body} ` : body });
  }
  assert.equal((await fetch(`${url}/healthz`)).status, 200);
  assert.equal((await fetch(`${url}/v1/command`, { method: 'POST', body: '{}' })).status, 401);
  assert.equal((await send(input, '1000000000')).status, 401);
  assert.equal((await send(input, undefined, true)).status, 401);
  assert.equal((await send({ ...input, rawCDP: 'Runtime.evaluate' })).status, 400);
  const good = await send(input);
  assert.equal(good.status, 200);
  assert.equal((await good.json()).status, 'stopped');
  assert.equal((await fetch(`${url}/json/version`)).status, 404);
});
