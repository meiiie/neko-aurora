"""Routine test admission UI; loopback fixtures, no model or provider execution."""
import functools
import http.server
import json
import os
import re
import threading
from pathlib import Path
from fixture_http import FixtureHTTPServer
from urllib.parse import urlsplit, parse_qs
from playwright.sync_api import expect, sync_playwright

ROOT = Path(__file__).resolve().parents[3]
OUT = ROOT / '.audit/2026-10-28-schedule-test'
OUT.mkdir(parents=True, exist_ok=True)

class Handler(http.server.SimpleHTTPRequestHandler):
    def log_message(self, *_): pass

class Fixture:
    def __init__(self):
        self.scope = 'schedule-test-owner-a'
        self.items = [dict(id='schedule-fixture', bot='bot-a', text='Tóm tắt buổi sáng', at='08:00', repeat='daily', paused=True, revision=1)]
        self.posts, self.gets, self.held = [], [], []
        self.receipts = {}
        self.mode = 'ok'

    def route(self, route):
        req = route.request
        url = urlsplit(req.url)
        path = url.path.removeprefix('/app/api')
        data, status = {}, 200
        if path == '/events':
            route.fulfill(status=200, content_type='text/event-stream', body=''); return
        if path == '/state':
            data = dict(workspaceId=self.scope, agents=[dict(id='bot-a', name='Chat A', status='idle', av=dict(shape=0, color=4))], pending=[], ai={}, secrets=[], outbound={})
        elif path.endswith('/messages'): data = [dict(id='req-fixture:u',taskId='req-fixture',agentId='bot-a',role='user',text='Lượt thử đã nhận',created='2026-10-28T10:00:00Z')] if self.receipts else []
        elif path.endswith('/computer'): data = dict(available=False, files=[], log=[])
        elif path == '/schedules': data = dict(schedules=self.items)
        elif path == '/schedules/schedule-fixture/test':
            if req.method == 'GET':
                q = parse_qs(url.query); self.gets.append(q)
                assert q['workspaceId'] == [self.scope]
                data = self.receipts.get(q['clientNonce'][0], dict(found=False))
            else:
                body = req.post_data_json; self.posts.append(body)
                assert set(body) == {'workspaceId', 'revision', 'clientNonce'}
                assert body['workspaceId'] == self.scope
                if self.mode == 'held': self.held.append(route); return
                if self.mode == 'error': status = 500
                elif self.mode == 'conflict': status = 409
                elif self.mode == 'quota': status, data = 429, dict(error='quota_exceeded')
                else:
                    data = self.receipts.setdefault(body['clientNonce'], dict(found=True, requestId='req-fixture', botId='bot-a', duplicate=False))
                    if self.mode == 'lost': route.abort('connectionreset'); return
        route.fulfill(status=status, content_type='application/json', body=json.dumps(data))

def check(message): print('PASS ' + message, flush=True)

server = FixtureHTTPServer(('127.0.0.1', 0), functools.partial(Handler, directory=str(ROOT/'app/native/internal/web/assets')))
threading.Thread(target=server.serve_forever, daemon=True).start()
try:
    with sync_playwright() as p:
        chrome = os.environ.get('AURORA_TEST_CHROME')
        if not chrome and Path('C:/Program Files/Google/Chrome/Application/chrome.exe').exists(): chrome = 'C:/Program Files/Google/Chrome/Application/chrome.exe'
        browser = p.chromium.launch(headless=True, executable_path=chrome)
        try:
            context = browser.new_context(viewport=dict(width=390, height=844), locale='vi-VN', service_workers='block')
            page = context.new_page(); fx = Fixture(); errors = []
            page.on('pageerror', lambda e: errors.append(str(e)))
            page.route('**/app/api/**', fx.route)
            url = f'http://127.0.0.1:{server.server_port}/app/#bot-a'
            def load():
                if page.url.startswith(url.split('#')[0]):
                    page.evaluate('(u) => history.replaceState(null,"",u)', url)
                    page.reload(wait_until='networkidle')
                else: page.goto(url, wait_until='networkidle')
                expect(page.locator('#whoBtn')).to_contain_text('Chat A'); page.locator('#whoBtn').click()
                expect(page.locator('#schedBox')).to_be_visible(); expect(page.locator('#profile')).not_to_have_class(re.compile(r'\boff\b'))
            def confirm_run(name='Chạy thử mới'):
                page.once('dialog', lambda d: d.accept())
                card.get_by_role('button', name=name, exact=True).click()
            load(); card = page.get_by_role('group', name='Chạy thử lịch', exact=True); status = card.get_by_role('status')
            assert not fx.posts
            dialogs = []
            def cancel(d): dialogs.append(d.message); d.dismiss()
            page.once('dialog', cancel); card.get_by_role('button', name='Chạy thử', exact=True).click()
            assert not fx.posts and 'hạn mức' in dialogs[-1]
            page.once('dialog', lambda d: d.accept()); card.get_by_role('button', name='Chạy thử', exact=True).focus(); page.keyboard.press('Enter')
            expect(status).to_contain_text('Đã nhận lượt thử'); assert len(fx.posts) == 1
            expect(page.locator('#schedBox')).to_contain_text('Đã tạm dừng')
            card.scroll_into_view_if_needed();page.wait_for_function('Math.abs(document.querySelector("#profile").getBoundingClientRect().left)<1')
            page.screenshot(path=str(OUT/'schedule-test-mobile.png'))
            card.get_by_role('button', name='Mở lượt thử', exact=True).click(); expect(page.locator('#profile')).to_have_class(re.compile(r'\boff\b'))
            expect(page.locator('[data-message-id="req-fixture:u"]')).to_be_focused()
            check('confirmation and keyboard test enqueue one turn, keep pause and open the bot chat')

            load(); fx.mode = 'lost'; before = len(fx.posts); confirm_run(); expect(status).to_contain_text('Chưa nhận được xác nhận')
            nonce = fx.posts[-1]['clientNonce']; load(); assert len(fx.posts) == before+1
            expect(card.get_by_role('button', name='Thử lại lượt này', exact=True)).to_be_visible()
            card.get_by_role('button', name='Kiểm tra lượt thử', exact=True).click(); expect(status).to_contain_text('Đã nhận lượt thử')
            assert len(fx.posts) == before+1 and fx.gets[-1]['clientNonce'] == [nonce]
            check('lost receipt survives reload and read-only lookup reconciles without resending')

            fx.mode = 'error'; confirm_run(); expect(status).to_contain_text('Chưa nhận được xác nhận'); first = fx.posts[-1].copy()
            load(); fx.mode = 'ok'; card.get_by_role('button', name='Thử lại lượt này', exact=True).click(); expect(status).to_contain_text('Đã nhận lượt thử')
            assert fx.posts[-1] == first
            check('explicit retry after reload preserves the complete attempt identity')

            for mode, message in [('conflict','Lịch hoặc tài khoản đã thay đổi'), ('quota','Hết hạn mức')]:
                fx.mode = mode; before = len(fx.posts); confirm_run(); expect(status).to_contain_text(message)
                assert len(fx.posts) == before+1
            check('revision conflicts and quota refusal stay visible with no automatic retry')

            page.evaluate('''() => {window.originalSetItem = Storage.prototype.setItem; Storage.prototype.setItem = function(k,v){if(k.startsWith("aurora.schedule-test.v1.")) throw Error("fixture storage full");return window.originalSetItem.call(this,k,v)}}''')
            before = len(fx.posts); confirm_run(); expect(status).to_contain_text('Chưa lưu được mã lượt thử'); assert len(fx.posts) == before
            page.evaluate('() => {Storage.prototype.setItem=window.originalSetItem}')
            check('storage failure refuses dispatch before a durable attempt can be recorded')

            fx.mode = 'lost'; confirm_run(); expect(status).to_contain_text('Chưa nhận được xác nhận'); before = len(fx.posts)
            fx.items = []; load(); expect(page.locator('#schedBox')).to_contain_text('lịch không còn trong danh sách')
            card.get_by_role('button', name='Kiểm tra lượt thử', exact=True).click(); expect(status).to_contain_text('Đã nhận lượt thử')
            assert len(fx.posts) == before
            expect(card.get_by_role('button', name='Chạy thử mới', exact=True)).to_be_hidden()
            check('pending receipt remains recoverable after the routine was deleted')

            fx.items = [dict(id='schedule-fixture', bot='bot-a', text='Routine mới', at='08:00', repeat='daily', paused=True, revision=2)]
            load(); fx.mode = 'held'; confirm_run(); expect(status).to_contain_text('Đang gửi lượt thử'); assert fx.held
            fx.scope = 'schedule-test-owner-b'; page.evaluate('window.__AURORA.reload()')
            expect(page.locator('#profile')).to_have_class(re.compile(r'\boff\b')); page.locator('#rows .row[data-id="bot-a"] .rowin').click(); page.locator('#whoBtn').click()
            expect(card.get_by_role('button', name='Chạy thử', exact=True)).to_be_visible()
            fx.held.pop().fulfill(status=202, content_type='application/json', body=json.dumps(dict(found=True, requestId='old-request', botId='bot-a')))
            expect(status).to_be_empty(); expect(card.get_by_role('button', name='Chạy thử', exact=True)).to_be_enabled()
            assert not page.evaluate('document.documentElement.scrollWidth>innerWidth')
            assert not errors, errors
            check('late old-account acknowledgement cannot update the new profile; mobile layout fits')
            load(); page.evaluate('const originalTimeout=AbortSignal.timeout.bind(AbortSignal);AbortSignal.timeout=ms=>originalTimeout(ms===30000?100:ms)')
            fx.mode = 'held'; before = len(fx.posts); confirm_run('Chạy thử')
            expect(status).to_contain_text('Chưa nhận được xác nhận'); assert len(fx.posts)==before+1
            fx.held.pop().abort()
            expect(card.get_by_role('button',name='Thử lại lượt này',exact=True)).to_be_enabled()
            check('POST timeout preserves the pending attempt for explicit reconciliation')
            page.evaluate('''() => {for(const k of Object.keys(localStorage))if(k.startsWith("aurora.schedule-test.v1.")&&k.includes("schedule-test-owner-b"))localStorage.setItem(k,"{broken")}''')
            before=len(fx.posts);load();expect(page.locator('#schedBox')).to_contain_text('Chưa đọc được lượt thử đã lưu')
            expect(card).to_have_count(0);assert len(fx.posts)==before
            check('corrupt pending storage blocks new dispatch instead of replacing its identity')
            (OUT/'schedule-test-browser-result.json').write_text(json.dumps(dict(result='PASS', scenarios=9, browser=browser.version, errors=errors), indent=2), encoding='utf-8')
        finally: browser.close()
finally: server.shutdown(); server.server_close()
