"""Real frontend with loopback fixtures: hosted segments, reload and four locales.

No real model, account or cloud service is contacted. Backend protocol and file
handoff are exercised separately by TestCoreActualAgentCLIWritesAndHandsOff.
"""
import functools
import http.server
import json
import os
import threading
from pathlib import Path
from urllib.parse import urlsplit

from fixture_http import FixtureHTTPServer
from playwright.sync_api import expect, sync_playwright

ROOT = Path(__file__).resolve().parents[3]
OUT = Path(os.environ.get('AURORA_BROWSER_AUDIT_DIR', ROOT / '.audit/2026-11-02-core-aurora-harness'))
OUT.mkdir(parents=True, exist_ok=True)


class Handler(http.server.SimpleHTTPRequestHandler):
    def log_message(self, *_):
        pass


server = FixtureHTTPServer(('127.0.0.1', 0), functools.partial(Handler, directory=str(ROOT / 'app/native/internal/web/assets')))
threading.Thread(target=server.serve_forever, daemon=True).start()
base = f'http://127.0.0.1:{server.server_port}'
bot = {'id': 'bot-fixture', 'name': 'Meiiie', 'status': 'working', 't': '2026-11-02T03:00:00Z', 'av': {'shape': 0, 'color': 4}}
user = {'id': 'task:u', 'taskId': 'task', 'agentId': bot['id'], 'role': 'user', 'text': 'Create the page', 't': '2026-11-02T03:00:00Z'}
segment = {'id': 'task:segment:first', 'taskId': 'task', 'agentId': bot['id'], 'role': 'agent', 'text': 'First complete message — tiếng Việt 中文', 'segment': True, 't': '2026-11-02T03:00:01Z'}
answer = {'id': 'task', 'taskId': 'task', 'agentId': bot['id'], 'role': 'agent', 'text': 'The result is saved.', 't': '2026-11-02T03:00:02Z', 'completion': {'status': 'incomplete', 'revision': 'fixture', 'summary': 'Still needs interaction testing.', 'evidence': []}}
messages = [user]


def route_api(route):
    path = urlsplit(route.request.url).path.removeprefix('/app/api')
    data = {}
    if path == '/state':
        data = {'workspaceId': 'core-browser-fixture', 'agents': [bot], 'pending': [], 'ai': {}, 'secrets': [], 'outbound': {}}
    elif path.endswith('/messages'):
        data = messages
    elif path.endswith('/ext'):
        data = {'workspaceId': 'core-browser-fixture', 'menu': [], 'allSkills': [], 'mcp': []}
    route.fulfill(content_type='application/json', body=json.dumps(data))


try:
    with sync_playwright() as p:
        chrome = os.environ.get('AURORA_TEST_CHROME')
        if not chrome and Path('C:/Program Files/Google/Chrome/Application/chrome.exe').exists():
            chrome = 'C:/Program Files/Google/Chrome/Application/chrome.exe'
        browser = p.chromium.launch(headless=True, executable_path=chrome)
        try:
            context = browser.new_context(viewport={'width': 1280, 'height': 900}, service_workers='block')
            context.add_init_script("""
              window.fixtureStreams=[];
              window.EventSource=class extends EventTarget {
                constructor(){super();this.readyState=1;window.fixtureStreams.push(this);queueMicrotask(()=>this.onopen?.())}
                close(){this.readyState=2}
              };
              window.fixtureEvent=(type,data)=>window.fixtureStreams.at(-1).dispatchEvent(new MessageEvent(type,{data:JSON.stringify(data)}));
            """)
            page = context.new_page()
            errors = []
            page.on('pageerror', lambda e: errors.append(str(e)))
            page.route('**/app/api/**', route_api)
            page.goto(base + '/app/#bot-fixture')
            page.wait_for_load_state('networkidle')
            expect(page.get_by_text(user['text'], exact=True)).to_be_visible()
            emit = lambda kind, data: page.evaluate('([type,data])=>window.fixtureEvent(type,data)', [kind, data])
            emit('delta', {'id': 'task', 'agentId': bot['id'], 'text': segment['text'], 'delta': segment['text']})
            expect(page.get_by_text(segment['text'], exact=True)).to_be_visible()
            emit('message', segment)
            emit('draft_end', {'id': 'task', 'agentId': bot['id']})
            emit('delta', {'id': 'task', 'agentId': bot['id'], 'text': 'Next message is streaming', 'delta': 'Next message is streaming'})
            expect(page.get_by_text(segment['text'], exact=True)).to_have_count(1)
            expect(page.get_by_text('Next message is streaming', exact=True)).to_be_visible()
            emit('message', answer)
            emit('draft_end', {'id': 'task', 'agentId': bot['id']})
            expect(page.get_by_text(segment['text'], exact=True)).to_have_count(1)
            expect(page.get_by_text(answer['text'], exact=True)).to_be_visible()
            assert page.locator('.interrupted-note').count() == 0
            messages[:] = [user, segment, answer]
            bot['status'] = 'idle'
            page.reload()
            page.wait_for_load_state('networkidle')
            expect(page.get_by_text(segment['text'], exact=True)).to_have_count(1)
            expect(page.get_by_text(answer['text'], exact=True)).to_be_visible()
            for lang, label in [('vi', 'Còn phần chưa hoàn tất'), ('en', 'Some work is still unfinished'), ('zh-Hans', '仍有工作未完成'), ('zh-Hant', '仍有工作未完成')]:
                page.evaluate('(lang)=>window.__I18N.set(lang)', lang)
                expect(page.locator('.completion-note')).to_have_text(label)
                expect(page.get_by_text(segment['text'], exact=True)).to_have_count(1)
            answer['completion']['status'] = 'waiting'
            emit('message', answer)
            expect(page.locator('.completion-note')).to_have_text('等待你確認後繼續')
            page.screenshot(path=str(OUT / 'core-segments.png'), full_page=True)
            assert not errors, errors
            print('PASS: streamed segments, separate next draft, reload, four locales, waiting status, no page errors')
        finally:
            browser.close()
finally:
    server.shutdown()
    server.server_close()
