import { createServer } from 'node:http';
import { connect, isIP } from 'node:net';
import { lookup } from 'node:dns/promises';
import { fault } from './errors.mjs';

const vendors = ['zalo.me', 'zaloapp.com', 'zdn.vn'];
const observedCDNs = new Set(['stc-sp.zadn.vn', 'zalo-chat-static.zadn.vn', 'res-zalo.zadn.vn',
  's160-26-ava-talk.zadn.vn', 'ava-grp-talk.zadn.vn']);
const allowedHost = host => observedCDNs.has(host) || vendors.some(v => host === v || host.endsWith(`.${v}`));
export function allowedURL(raw) {
  try {
    const u = new URL(raw);
    return ['https:', 'wss:'].includes(u.protocol) && allowedHost(u.hostname) && !u.username && !u.password &&
      !u.hash && (!u.port || u.port === '443') && !/[\u0000-\u0020\\]/.test(raw);
  } catch { return false; }
}
export function publicAddress(ip) {
  const family = isIP(ip);
  if (family === 4) {
    const [a, b, c] = ip.split('.').map(Number);
    return !(a === 0 || a === 10 || a === 127 || a >= 224 || a === 169 && b === 254 ||
      a === 100 && b >= 64 && b <= 127 || a === 172 && b >= 16 && b <= 31 ||
      a === 192 && (b === 168 || b === 0 || b === 2 || b === 88 && c === 99) ||
      a === 198 && (b === 18 || b === 19 || b === 51 && c === 100) || a === 203 && b === 0 && c === 113);
  }
  if (family !== 6 || ip.includes('%') || ip.includes('.')) return false;
  const first = Number.parseInt(ip.split(':')[0], 16);
  const second = Number.parseInt(ip.split(':')[1] || '0', 16);
  // Admit ordinary global unicast only; mapped/NAT64/transition/documentation ranges are denied.
  return first >= 0x2000 && first <= 0x3fff && first !== 0x2002 &&
    !(first === 0x2001 && (second < 0x200 || second === 0xdb8));
}
export async function resolvePublic(host, resolver = lookup) {
  if (!allowedHost(host) || isIP(host)) throw fault('host_not_allowed', 403);
  const answers = await resolver(host, { all: true, verbatim: true });
  if (!answers.length || answers.some(a => !publicAddress(a.address))) throw fault('private_destination', 403);
  return answers[0];
}
export async function createEgress({ resolver = lookup, connector = connect } = {}) {
  const sockets = new Set();
  const server = createServer((req, res) => { res.writeHead(403); res.end(); });
  server.on('upgrade', (req, socket) => socket.destroy());
  server.on('connection', socket => { sockets.add(socket); socket.on('error', () => {}); socket.on('close', () => sockets.delete(socket)); });
  server.on('connect', async (req, client, head) => {
    let upstream;
    try {
      if (sockets.size > 64 || !/^[a-z0-9.-]+:443$/i.test(req.url ?? '')) throw fault('host_not_allowed');
      const host = req.url.slice(0, -4).toLowerCase();
      const pinned = await resolvePublic(host, resolver);
      if (client.destroyed) return;
      upstream = connector({ host: pinned.address, family: pinned.family, port: 443 });
      sockets.add(upstream);
      upstream.setTimeout(120000, () => upstream.destroy());
      upstream.on('error', () => client.destroy());
      upstream.on('close', () => { sockets.delete(upstream); client.destroy(); });
      client.on('close', () => upstream.destroy());
      upstream.once('connect', () => {
        client.write('HTTP/1.1 200 Connection Established\r\n\r\n');
        if (head.length) upstream.write(head);
        client.pipe(upstream); upstream.pipe(client);
      });
    } catch { client.end('HTTP/1.1 403 Forbidden\r\nConnection: close\r\n\r\n'); upstream?.destroy(); }
  });
  server.headersTimeout = 10000; server.requestTimeout = 10000;
  await new Promise((resolve, reject) => { server.once('error', reject); server.listen(0, '127.0.0.1', resolve); });
  return { url: `http://127.0.0.1:${server.address().port}`, async close() {
    for (const socket of sockets) socket.destroy();
    await new Promise(resolve => server.close(resolve));
  } };
}
