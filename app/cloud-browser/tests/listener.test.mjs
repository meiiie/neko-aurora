import test from 'node:test';
import assert from 'node:assert/strict';
import { createServer } from 'node:net';
import { spawn } from 'node:child_process';
import { once } from 'node:events';
import { mkdtemp, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

test('production listener accepts private IPv4 and IPv6 without exposing unsigned commands', { timeout: 15000 }, async t => {
  const reservation = createServer();
  await new Promise((resolve, reject) => { reservation.once('error', reject); reservation.listen(0, '127.0.0.1', resolve); });
  const port = reservation.address().port;
  await new Promise(resolve => reservation.close(resolve));
  const dataDir = await mkdtemp(join(tmpdir(), 'aurora-listener-'));
  const child = spawn(process.execPath, [fileURLToPath(new URL('../server.mjs', import.meta.url))], {
    env: { PATH: process.env.PATH, SystemRoot: process.env.SystemRoot, HOME: dataDir, DATA_DIR: dataDir,
      PORT: String(port), AURORA_CLOUD_BROWSER_KEY: 'listener-fixture-not-a-real-service-key-20261025' },
    stdio: 'ignore', windowsHide: true,
  });
  t.after(async () => {
    if (child.exitCode === null && child.signalCode === null) { const exited = once(child, 'exit'); child.kill(); await exited; }
    await rm(dataDir, { recursive: true, force: true });
  });
  const deadline = Date.now() + 7000;
  let ipv4;
  do {
    try { ipv4 = await fetch(`http://127.0.0.1:${port}/healthz`); break; }
    catch { await new Promise(resolve => setTimeout(resolve, 50)); }
  } while (Date.now() < deadline && child.exitCode === null);
  assert.equal(ipv4?.status, 200, 'Production worker must start and answer health checks');
  for (const host of ['127.0.0.1', '[::1]']) {
    assert.equal((await fetch(`http://${host}:${port}/healthz`)).status, 200, host);
    assert.equal((await fetch(`http://${host}:${port}/v1/command`, { method: 'POST', body: '{}' })).status, 401, host);
  }
});
