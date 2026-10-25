import { createServer } from 'node:http';
import { request as httpsRequest, Agent } from 'node:https';
import { Readable } from 'node:stream';
import { pathToFileURL } from 'node:url';

const endpoint = 'https://backboard.railway.com/graphql/v2';
const query = 'mutation($environmentId:String!,$id:String!){sandboxHeartbeat(environmentId:$environmentId,id:$id){id status privateDomain}}';
const uuid = /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/i;
const timers = {
  now: () => Date.now(), setTimeout, clearTimeout, setInterval, clearInterval,
};
const privateDomain = value => typeof value === 'string' && value.length <= 253 &&
  /^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+railway\.internal$/.test(value);

async function boundedJSON(response) {
  if (!response?.ok || !response.body?.getReader) {
    await response?.body?.cancel().catch(() => {});
    throw Error('heartbeat_response_invalid');
  }
  const reader = response.body.getReader(), chunks = [];
  let length = 0, done = false;
  try {
    while (true) {
      const chunk = await reader.read();
      if (chunk.done) { done = true; break; }
      length += chunk.value.byteLength;
      if (length > 65536) throw Error('heartbeat_response_too_large');
      chunks.push(Buffer.from(chunk.value));
    }
    return JSON.parse(Buffer.concat(chunks).toString('utf8'));
  } finally { if (!done) await reader.cancel().catch(() => {}); }
}

export function createKeeper({ token, environmentId, sandboxId, fetch: suppliedFetch, clock = timers } = {}) {
  if (typeof token !== 'string' || !token.length || token.length > 4096 || token.trim() !== token || /[\r\n]/.test(token) ||
      !uuid.test(environmentId ?? '') || !uuid.test(sandboxId ?? '')) throw Error('keeper_configuration_invalid');
  const agent = new Agent({ keepAlive: true, maxSockets: 1, maxFreeSockets: 1 });
  // node:https ignores environment proxy variables and never follows redirects.
  const fetch = suppliedFetch ?? ((url, options) => new Promise((resolve, reject) => {
    if (url !== endpoint) return reject(Error('heartbeat_destination_invalid'));
    const request = httpsRequest(endpoint, { method: 'POST', headers: options.headers, signal: options.signal, agent }, response => {
      resolve({ ok: response.statusCode >= 200 && response.statusCode < 300, body: Readable.toWeb(response) });
    });
    request.on('error', reject); request.end(options.body);
  }));
  let stopped = false, interval, deadline, inFlight, controller, finishWait, lastSuccess = null;
  const failure = () => { lastSuccess = null; console.error('aurora_vm_keeper_heartbeat_failed'); };
  function health() {
    const age = lastSuccess === null ? Infinity : clock.now() - lastSuccess;
    return { ready: !stopped && lastSuccess !== null && age >= 0 && age <= 150000 };
  }
  function heartbeat() {
    if (stopped || inFlight) return Promise.resolve(false);
    let timedOut = false;
    const current = new AbortController(); controller = current;
    const bounded = new Promise(resolve => { finishWait = resolve; });
    deadline = clock.setTimeout(() => {
      timedOut = true; failure(); current.abort(); finishWait(false);
    }, 10000);
    deadline?.unref?.();
    const task = Promise.resolve().then(async () => {
      try {
        if (stopped || timedOut) return false;
        const response = await fetch(endpoint, { method: 'POST', redirect: 'error', signal: current.signal,
          headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
          body: JSON.stringify({ query, variables: { environmentId, id: sandboxId } }) });
        const body = await boundedJSON(response);
        if (stopped || timedOut) return false;
        const vm = body?.data?.sandboxHeartbeat;
        if (body.errors || !vm || vm.id !== sandboxId || vm.status !== 'RUNNING' || !privateDomain(vm.privateDomain)) throw Error('heartbeat_identity_invalid');
        lastSuccess = clock.now(); return true;
      } catch {
        if (!stopped && !timedOut) failure();
        return false;
      } finally {
        clock.clearTimeout(deadline); deadline = undefined;
        if (controller === current) controller = undefined;
        inFlight = undefined; finishWait = undefined;
      }
    });
    // Keep the request slot until the underlying fetch settles, even after its public deadline.
    inFlight = task; return Promise.race([task, bounded]);
  }
  function start() {
    if (stopped || interval !== undefined) return Promise.resolve(false);
    interval = clock.setInterval(() => { void heartbeat(); }, 60000);
    interval?.unref?.();
    return heartbeat();
  }
  function stop() {
    if (stopped) return;
    stopped = true; lastSuccess = null;
    clock.clearInterval(interval); clock.clearTimeout(deadline);
    controller?.abort(); finishWait?.(false); agent.destroy();
  }
  function handle(request, response) {
    response.setHeader('Content-Type', 'application/json'); response.setHeader('Cache-Control', 'no-store');
    if (request.method !== 'GET' || request.url !== '/healthz') {
      response.writeHead(404); response.end('{"error":"not_found"}'); return;
    }
    const state = health(); response.writeHead(state.ready ? 200 : 503); response.end(JSON.stringify(state));
  }
  return { heartbeat, health, start, stop, handle };
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  let keeper;
  const port = Number(process.env.PORT ?? 8080);
  try {
    if (!Number.isInteger(port) || port < 1 || port > 65535) throw Error();
    keeper = createKeeper({ token: process.env.RAILWAY_TOKEN, environmentId: process.env.RAILWAY_ENVIRONMENT_ID,
      sandboxId: process.env.AURORA_BROWSER_SANDBOX_ID });
  } catch { console.error('aurora_vm_keeper_configuration_invalid'); process.exitCode = 1; }
  if (keeper) {
    const server = createServer(keeper.handle);
    server.headersTimeout = 10000; server.requestTimeout = 10000;
    server.on('error', () => { console.error('aurora_vm_keeper_http_failed'); keeper.stop(); process.exitCode = 1; });
    server.listen(port); void keeper.start();
    const stop = () => { keeper.stop(); server.close(); server.closeAllConnections(); };
    process.once('SIGTERM', stop); process.once('SIGINT', stop);
  }
}
