import test from 'node:test';
import assert from 'node:assert/strict';
import { allowedURL, publicAddress, resolvePublic, createEgress } from '../egress.mjs';
import { createServer, connect } from 'node:net';

test('only vendor HTTPS/WSS destinations are allowed', () => {
  for (const url of ['https://chat.zalo.me/', 'wss://socket.zaloapp.com/', 'https://a.zdn.vn/img']) assert.equal(allowedURL(url), true, url);
  for (const url of ['http://chat.zalo.me/', 'https://zalo.me.evil.test/', 'https://user:pass@zalo.me/', 'https://127.0.0.1/', 'https://zalo.me:444/', 'file:///etc/passwd', 'https://zalo.me/#secret']) assert.equal(allowedURL(url), false, url);
});

test('observed Zalo CDN hosts are allowed exactly, without opening the zadn.vn parent domain', async () => {
  for (const host of ['stc-sp.zadn.vn', 'zalo-chat-static.zadn.vn', 'res-zalo.zadn.vn', 's160-26-ava-talk.zadn.vn', 'ava-grp-talk.zadn.vn']) {
    assert.equal(allowedURL(`https://${host}/asset.js`), true);
    assert.equal(allowedURL(`http://${host}/asset.js`), false);
    assert.deepEqual(await resolvePublic(host, async () => [{ address: '1.1.1.1', family: 4 }]), { address: '1.1.1.1', family: 4 });
    await assert.rejects(resolvePublic(host, async () => [{ address: '127.0.0.1', family: 4 }]), { code: 'private_destination' });
  }
  for (const host of ['zadn.vn', 'evil.zadn.vn', 'nested.stc-sp.zadn.vn', 'evil-stc-sp.zadn.vn', 'stc-sp.zadn.vn.evil.test', 'zalo-chat-static.zadn.vn.evil.test', 'nested.res-zalo.zadn.vn', 'res-zalo.zadn.vn.evil.test', 's160-26-ava-talk.zadn.vn.evil.test', 'ava-grp-talk.zadn.vn.evil.test']) {
    assert.equal(allowedURL(`https://${host}/asset.js`), false, host);
    await assert.rejects(resolvePublic(host, async () => [{ address: '1.1.1.1', family: 4 }]), { code: 'host_not_allowed' });
  }
});

test('all private, mapped, metadata and non-unicast addresses are rejected', () => {
  for (const ip of ['127.0.0.1', '10.0.0.1', '172.16.0.1', '192.168.1.1', '169.254.169.254', '0.0.0.0', '100.64.0.1', '224.0.0.1', '::1', 'fc00::1', 'fe80::1', '::ffff:127.0.0.1', '::ffff:7f00:1', '2002:7f00:1::', '2001:db8::1']) assert.equal(publicAddress(ip), false, ip);
  for (const ip of ['8.8.8.8', '1.1.1.1', '2606:4700:4700::1111']) assert.equal(publicAddress(ip), true, ip);
});

test('DNS answers are pinned and any private answer rejects the connection', async () => {
  assert.deepEqual(await resolvePublic('chat.zalo.me', async () => [{ address: '1.1.1.1', family: 4 }]), { address: '1.1.1.1', family: 4 });
  await assert.rejects(resolvePublic('chat.zalo.me', async () => [{ address: '1.1.1.1', family: 4 }, { address: '127.0.0.1', family: 4 }]), { code: 'private_destination' });
  await assert.rejects(resolvePublic('evil.internal', async () => [{ address: '1.1.1.1', family: 4 }]), { code: 'host_not_allowed' });
});

test('CONNECT uses the resolved IP and never connects for disallowed or rebinding hosts', async t => {
  const destination = createServer(socket => socket.on('data', bytes => socket.write(bytes)));
  await new Promise(resolve => destination.listen(0, '127.0.0.1', resolve));
  const calls = [];
  const proxy = await createEgress({
    resolver: async host => host === 'private.zalo.me' ? [{ address: '127.0.0.1', family: 4 }] : [{ address: '1.1.1.1', family: 4 }],
    connector: options => { calls.push(options); return connect(destination.address().port, '127.0.0.1'); },
  });
  t.after(async () => { await proxy.close(); await new Promise(resolve => destination.close(resolve)); });
  async function tunnel(host, payload = '') {
    const socket = connect(Number(new URL(proxy.url).port), '127.0.0.1');
    let text = '';
    const result = new Promise((resolve, reject) => {
      socket.on('error', reject);
      socket.on('data', chunk => { text += chunk; if (text.includes(payload || '\r\n\r\n')) { socket.destroy(); resolve(text); } });
    });
    socket.write(`CONNECT ${host}:443 HTTP/1.1\r\nHost: ${host}:443\r\n\r\n${payload}`);
    return result;
  }
  assert.match(await tunnel('chat.zalo.me', 'fixture-tunnel-payload'), /200 Connection Established[\s\S]*fixture-tunnel-payload/);
  assert.deepEqual(calls, [{ host: '1.1.1.1', family: 4, port: 443 }]);
  assert.match(await tunnel('private.zalo.me'), /403 Forbidden/);
  assert.match(await tunnel('evil.internal'), /403 Forbidden/);
  assert.equal(calls.length, 1);
});
