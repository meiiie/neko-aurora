"""Loopback model fixture for truthful requester and live device-stop acceptance."""
import argparse
import json
import pathlib
import sys
import threading
from http.server import ThreadingHTTPServer

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))
from fixture_provider import Handler as BaseHandler, admitted_fixture_context


class Handler(BaseHandler):
    release = threading.Event()
    observations = []
    lock = threading.Lock()

    def do_GET(self):
        if self.path != '/observations':
            self.send_error(404)
            return
        with self.lock:
            raw = json.dumps(self.observations).encode()
        self.send_response(200)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Content-Length', str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)

    def do_POST(self):
        if self.path == '/release':
            self.release.set()
            self.send_response(200)
            self.end_headers()
            return
        if self.path != '/v1/chat/completions' or self.headers.get('Authorization') != 'Bearer fixture-only':
            self.send_error(403)
            return
        length = int(self.headers.get('Content-Length', 0))
        if not 0 < length <= 2_000_000:
            self.send_error(413)
            return
        body = json.loads(self.rfile.read(length))
        messages = body['messages']
        manifest = admitted_fixture_context(messages)
        if manifest is None:
            self.send_error(400, 'manifest_required')
            return
        text = manifest['primaryMessage']['text']
        actor = manifest['primaryMessage']['senderId']
        with self.lock:
            self.observations.append({'manifestVersion': manifest['version'], 'ownerUserId': manifest['scope']['ownerUserId'], 'senderId': actor, 'requesterActorId': (manifest.get('requester') or {}).get('actorId'), 'requestId': manifest['primaryMessage']['id'], 'held': '[principal:hold]' in text, 'privateCanaryPresent': 'ALICE-PRIVATE-CANARY' in json.dumps(manifest)})
        self.send_response(200)
        self.send_header('Content-Type', 'text/event-stream')
        self.send_header('Connection', 'close')
        self.end_headers()
        try:
            if '[principal:hold]' in text:
                self.chunk({'role': 'assistant', 'content': 'Fixture requester=' + actor + '; waiting for local stop check. '})
                if not self.release.wait(35):
                    self.chunk({'content': 'fixture hold expired'})
                self.chunk({}, 'stop')
            elif '[principal:artifact]' in text and not any(m.get('role') == 'tool' for m in messages):
                args = {'name': 'artifact.write_text', 'arguments': {'filename': 'participant-report.txt', 'text': 'Controlled fixture report by ' + actor + '\n'}}
                self.chunk({'role': 'assistant', 'tool_calls': [{'index': 0, 'id': 'principal-fixture-write', 'type': 'function', 'function': {'name': 'mcp__nekobot__execute_tool', 'arguments': json.dumps(args)}}]})
                self.chunk({}, 'tool_calls')
            else:
                self.chunk({'role': 'assistant', 'content': 'FIXTURE_SENDER=' + actor + '\nWORKSPACE_OWNER=' + manifest['scope']['ownerUserId'] + '\nMANIFEST_VERSION=' + str(manifest['version'])})
                self.chunk({}, 'stop')
            self.wfile.write(b'data: [DONE]\n\n')
            self.wfile.flush()
        except (BrokenPipeError, ConnectionResetError):
            pass


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--port', type=int, default=0)
    args = parser.parse_args()
    server = ThreadingHTTPServer(('127.0.0.1', args.port), Handler)
    print('READY http://127.0.0.1:%d/v1' % server.server_address[1], flush=True)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass
