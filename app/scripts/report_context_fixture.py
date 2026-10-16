"""Controlled loopback responses for published-output Core acceptance, never a real model."""
import argparse
import hashlib
import json
import pathlib
import re
import sys
import threading
from http.server import ThreadingHTTPServer

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[1] / 'scripts'))
from fixture_provider import Handler as BaseHandler, admitted_fixture_context

PUBLIC_BODY = '# Shared market report\nRevenue=120\nCost=70\nMargin=50\nPlan=release-violet\nPUBLIC-REPORT-BODY\n'
PUBLIC_FILE = '# Market report\nMargin=50\nPlan=release-violet\nUTF8: kế hoạch Tím\n'
PRIVATE_NOTE = 'PRIVATE-NOTE-RESULT-CANARY'


def reports(manifest):
    out = []
    for source in (manifest or {}).get('sources', []):
        if source.get('kind') != 'derived':
            continue
        try:
            value = json.loads(source['span']['text'])
        except (ValueError, KeyError):
            continue
        if isinstance(value, dict) and value.get('deliveryId'):
            out.append(value)
    return out


def tool_text(messages):
    return [str(m.get('content', '')) for m in messages if m.get('role') == 'tool']


def read_value(value):
    if isinstance(value, str):
        try:
            return read_value(json.loads(value))
        except ValueError:
            return None
    if isinstance(value, dict):
        if set(value) == {'fileId', 'sha256', 'offset', 'nextOffset', 'eof', 'text', 'authority'}:
            return value
        value = list(value.values())
    if isinstance(value, list):
        for item in value:
            found = read_value(item)
            if found:
                return found
    return None


def prior_stage_margin(manifest, stage):
    for source in (manifest or {}).get('sources', []):
        if source.get('kind') != 'derived':
            continue
        try:
            value = json.loads(source['span']['text'])
        except ValueError:
            continue
        if value.get('kind') == 'pinned-prior-turn':
            for message in value.get('messages', []):
                text = message.get('text', '')
                if message.get('role') == 'assistant' and ('STAGE=' + str(stage-1) + '\n') in text:
                    match = re.search(r'FOLLOW_MARGIN=(\d+)', text)
                    if match:
                        return int(match.group(1))
    return -1


class Handler(BaseHandler):
    hold = threading.Event()

    def do_POST(self):
        if self.path == '/release':
            self.hold.set()
            self.send_response(200)
            self.end_headers()
            self.wfile.write(b'{}')
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
        user = (manifest or {}).get('primaryMessage', {}).get('text', '')
        tools = tool_text(messages)
        selected = reports(manifest)
        stage = 0
        for source in (manifest or {}).get('sources', []):
            if source.get('kind') != 'derived':
                continue
            try:
                value = json.loads(source['span']['text'])
            except ValueError:
                continue
            if value.get('kind') == 'generated-collaboration-stage':
                stage = int(value['stage'])
        self.send_response(200)
        self.send_header('Content-Type', 'text/event-stream')
        self.send_header('Connection', 'close')
        self.end_headers()
        try:
            if '[reportfixture:source]' in user:
                if len(tools) < 2:
                    filename, content = ('market-report.md', PUBLIC_FILE) if not tools else ('private-notes.txt', PRIVATE_NOTE)
                    self.call('artifact.write_text', {'filename': filename, 'text': content}, 'source-write-' + str(len(tools)))
                else:
                    self.answer(PUBLIC_BODY)
            elif '[reportfixture:empty]' in user:
                self.answer('EMPTY_PUBLISHED=' + str(len(selected)))
            elif not selected:
                self.answer('MISSING_PUBLISHED_CONTEXT')
            elif '[reportfixture:hold]' in user and not tools:
                self.chunk({'role': 'assistant', 'content': 'WAITING_PUBLISHED_READ '})
                self.hold.wait(60)
                self.answer('HELD_PROVIDER_RELEASED')
            else:
                report = selected[0]
                if not tools:
                    self.chunk({'role': 'assistant', 'tool_calls': [{'index': 0, 'id': 'discover-read', 'type': 'function', 'function': {'name': 'mcp__nekobot__capabilities', 'arguments': '{}'}}]})
                    self.chunk({}, 'tool_calls')
                elif len(tools) == 1:
                    artifact = report.get('artifacts', [])[0]
                    forced = re.search(r'UNPUBLISHED_ID=([a-f0-9]{64})', user)
                    selected_file = next(file for file in report.get('files', []) if file['artifact']['id'] == artifact['id'])
                    exact_span_bytes = len(selected_file['excerpt'].encode('utf-8'))
                    self.call('report.read_artifact', {'deliveryId': report['deliveryId'], 'artifactId': forced.group(1) if forced else artifact['id'], 'offset': 0, 'bytes': exact_span_bytes}, 'read-published')
                elif '[reportfixture:forbidden]' in user:
                    self.answer('UNPUBLISHED_DENIED=' + str('isError' in tools[-1]))
                elif stage == 3 and len(tools) == 2:
                    self.call('artifact.write_text', {'filename': 'group-followup.md', 'text': '# Group follow-up\nMargin=50\nPlan=release-violet\nAll three stages used the published result.\n'}, 'group-final-write')
                else:
                    reply = report.get('reply', '')
                    found = re.search(r'Margin=(\d+)', reply)
                    margin = int(found.group(1)) if found else -1
                    read = read_value(tools[1])
                    has_read = read is not None and 'kế hoạch Tím' in read['text']
                    previous = prior_stage_margin(manifest, stage) if stage > 1 else 0
                    self.answer('STAGE=' + str(stage) + '\nFOLLOW_MARGIN=' + str(margin) + '\nFOLLOW_DOUBLE=' + str(margin*2) + '\nPREVIOUS_STAGE_MARGIN=' + str(previous) + '\nFOLLOW_READ=' + str(has_read) + '\nREAD_RESULT=' + tools[1] + '\nPUBLISHED_ID=' + report['deliveryId'])
            self.wfile.write(b'data: [DONE]\n\n')
            self.wfile.flush()
        except (BrokenPipeError, ConnectionResetError):
            pass

    def call(self, name, arguments, identity):
        self.chunk({'role': 'assistant', 'tool_calls': [{'index': 0, 'id': identity, 'type': 'function', 'function': {'name': 'mcp__nekobot__execute_tool', 'arguments': json.dumps({'name': name, 'arguments': arguments}, ensure_ascii=False)}}]})
        self.chunk({}, 'tool_calls')

    def answer(self, text):
        self.chunk({'role': 'assistant', 'content': text})
        self.chunk({}, 'stop')


if __name__ == '__main__':
    server = ThreadingHTTPServer(('127.0.0.1', 0), Handler)
    def lifeline():
        sys.stdin.buffer.read()
        Handler.hold.set()
        server.shutdown()
    threading.Thread(target=lifeline, daemon=True).start()
    print('READY http://127.0.0.1:%d/v1' % server.server_port, flush=True)
    server.serve_forever()
