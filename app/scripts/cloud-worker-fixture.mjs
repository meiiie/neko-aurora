// Wire integration fixture. The injected driver never starts a browser.
import { createServer } from 'node:http';
import { createWorker } from '../cloud-browser/server.mjs';

let effects = 0;
const worker = await createWorker({ key: 'fixture-only-wire-key-7281-not-a-credential', dataDir: process.argv[2],
  driver: { async open() { return {
    async observe() { return { documentId: 'fixture-document', origin: 'https://chat.zalo.me', title: 'Wire fixture',
      text: 'Fixture effects: ' + effects, loginEvidence: false,
      targets: [{ key: 'fixture-target', role: 'textbox', name: 'Username', type: 'text', protected: false },
        { key: 'fixture-password', role: 'textbox', name: 'Password', type: 'password', protected: true }] }; },
    async act(input) {
      if (input.kind === 'vault_fill' && (input.values?.username !== 'fixture-user-7281' || input.values?.password !== 'fixture-password-7281')) throw Error('fixture_payload_mismatch');
      effects++;
    },
    async frame() { return { mime: 'image/png', data: 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jp3sAAAAASUVORK5CYII=', width: 1, height: 1 }; },
    async close() {},
  }; } },
});
const server = createServer(worker.handle);
server.listen(0, '127.0.0.1', () => console.log('READY http://127.0.0.1:' + server.address().port));
const stop = async () => { server.close(); await worker.close(); };
process.once('SIGTERM', stop); process.once('SIGINT', stop);
