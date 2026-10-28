"""Computer recovery UI with loopback fixtures; no remote commands or accounts."""
import functools
import http.server
import json
import os
import threading
from pathlib import Path
from urllib.parse import urlsplit
from playwright.sync_api import expect, sync_playwright

ROOT = Path(__file__).resolve().parents[3]
OUT = ROOT / '.audit/2026-10-28-computer-recovery'
OUT.mkdir(parents=True, exist_ok=True)

class Handler(http.server.SimpleHTTPRequestHandler):
    def log_message(self, *_): pass

class Fixture:
    def __init__(self):
        self.scope = 'computer-owner-a'
        self.state = 'files'
        self.mode = 'ok'
        self.fail_get = False
        self.posts = []
        self.held = []

    def route(self, route):
        req = route.request
        path = urlsplit(req.url).path.removeprefix('/app/api')
        data, status = {}, 200
        if path == '/events':
            route.fulfill(status=200, content_type='text/event-stream', body='')
            return
        if path == '/state':
            data = dict(workspaceId=self.scope, agents=[dict(id='bot-a', name='Chat A', status='idle', av=dict(shape=0, color=4))], pending=[], ai={}, secrets=[], outbound={})
        elif path.endswith('/messages'): data = []
        elif path.endswith('/computer'):
            data = dict(backend='railway-sandbox', available=not self.state, files=[], log=[], usedBytes=0, quotaBytes=1000)
            if self.state:
                message = 'Lệnh đã kết thúc, file chờ tải.' if self.state == 'files' else 'Chưa xác nhận kết quả. Giữ bản local.'
                data.update(error=message, recovery=dict(state=self.state, canRecover=self.state == 'files', message=message))
            if self.fail_get: status = 500
        elif req.method == 'POST':
            body = req.post_data_json
            self.posts.append((path, body))
            assert path in ('/agents/bot-a/computer/recover', '/agents/bot-a/computer/reset'), path
            assert body == dict(workspaceId=self.scope)
            if self.mode == 'held':
                self.held.append(route)
                return
            if self.mode == 'conflict':
                status, data = 409, dict(error='computer_recovery_local_files_changed')
            elif self.mode == 'error': status = 500
            else:
                self.state = None
                data = dict(ok=True)
                if self.mode == 'lost':
                    route.abort('connectionreset')
                    return
                if self.mode == 'reload_error': self.fail_get = True
        route.fulfill(status=status, content_type='application/json', body=json.dumps(data))

def check(message): print('PASS '+message, flush=True)

server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), functools.partial(Handler, directory=str(ROOT/'app/native/internal/web/assets')))
threading.Thread(target=server.serve_forever, daemon=True).start()
try:
    with sync_playwright() as p:
        chrome = os.environ.get('AURORA_TEST_CHROME')
        if not chrome and Path('C:/Program Files/Google/Chrome/Application/chrome.exe').exists():
            chrome = 'C:/Program Files/Google/Chrome/Application/chrome.exe'
        browser = p.chromium.launch(headless=True, executable_path=chrome)
        try:
            context = browser.new_context(viewport=dict(width=390, height=844), locale='vi-VN', service_workers='block')
            page = context.new_page(); fx = Fixture(); errors = []
            page.on('pageerror', lambda error: errors.append(str(error)))
            page.route('**/app/api/**', fx.route)
            url = f'http://127.0.0.1:{server.server_port}/app/#bot-a'
            def load():
                page.goto(url)
                expect(page.locator('#whoBtn')).to_contain_text('Chat A')
                page.locator('#whoBtn').click()
                expect(page.locator('#pcReset')).to_be_visible()
            load()
            box = page.locator('#pcBox')
            recover = box.get_by_role('button', name='Tải lại file từ máy ảo', exact=True)
            refresh = box.get_by_role('button', name='Tải lại trạng thái', exact=True)
            status = box.get_by_role('status')
            expect(recover).to_be_visible(); expect(refresh).to_be_hidden()
            expect(box).to_contain_text('Chờ phục hồi')
            assert not fx.posts
            check('pending files offer explicit recovery without automatic dispatch')

            fx.mode = 'conflict'; recover.focus(); page.keyboard.press('Enter')
            expect(status).to_contain_text('File local đã thay đổi')
            expect(recover).to_be_enabled()
            before = len(fx.posts); refresh.click(); expect(status).to_be_empty()
            assert len(fx.posts) == before
            fx.mode = 'ok'; recover.click()
            expect(recover).to_have_count(0); expect(page.locator('#pcReset')).to_be_enabled()
            assert fx.posts[-1][0].endswith('/recover')
            check('local edit conflict keeps controls; read-only refresh and explicit recovery work')

            fx.state = 'unknown'; load()
            expect(recover).to_have_count(0)
            expect(box).to_contain_text('Chưa xác nhận kết quả')
            dialogs = []
            def dismiss(dialog): dialogs.append(dialog.message); dialog.dismiss()
            page.once('dialog', dismiss)
            before = len(fx.posts); page.locator('#pcReset').click()
            assert len(fx.posts) == before and 'file chưa tải về' in dialogs[-1]
            fx.mode = 'error'; page.once('dialog', lambda dialog: dialog.accept()); page.locator('#pcReset').click()
            expect(status).to_contain_text('Chưa nhận được xác nhận')
            expect(page.locator('#pcReset')).to_be_enabled()
            assert fx.posts[-1][0].endswith('/reset')
            check('unknown execution offers no replay; reset warns about remote files and retains failure state')

            fx.state = 'files'; fx.mode = 'lost'; load(); before = len(fx.posts); recover.click()
            expect(status).to_contain_text('Chưa nhận được xác nhận')
            assert len(fx.posts) == before+1
            fx.mode = 'ok'; refresh.click(); expect(recover).to_have_count(0)
            assert len(fx.posts) == before+1
            check('lost receipt is resolved by status reload with no automatic resubmission')

            fx.state = 'files'; fx.mode = 'reload_error'; load(); recover.click()
            expect(status).to_contain_text('Đã tải lại file')
            expect(status).to_contain_text('Chưa tải lại được trạng thái')
            expect(recover).to_be_disabled(); expect(refresh).to_be_enabled()
            fx.mode = 'ok'; fx.fail_get = False; refresh.click(); expect(recover).to_have_count(0)
            check('acknowledged transfer with failed status fetch stays acknowledged')

            fx.state = 'files'; fx.mode = 'held'; load()
            page.evaluate('const original=AbortSignal.timeout.bind(AbortSignal);AbortSignal.timeout=ms=>original(ms===30000?100:ms)')
            before = len(fx.posts); recover.click()
            expect(status).to_contain_text('Chưa nhận được xác nhận')
            assert len(fx.posts) == before+1
            page.screenshot(path=str(OUT/'computer-recovery-mobile.png'))
            assert not page.evaluate('document.documentElement.scrollWidth>innerWidth')
            check('timeout remains explicit and recovery status fits a 390px viewport')

            load(); recover.click(); expect(recover).to_be_disabled()
            route = fx.held[-1]
            fx.scope = 'computer-owner-b'; fx.state = 'unknown'
            page.evaluate('window.__AURORA.reload()')
            expect(page.locator('#profile')).to_have_class('screen profile off')
            page.locator('#rows .rowin').first.focus(); page.keyboard.press('Enter')
            page.wait_for_function('window.__AURORA.current()==="bot-a"')
            page.locator('#whoBtn').click()
            expect(box).to_contain_text('Chưa xác nhận kết quả')
            route.fulfill(status=200, content_type='application/json', body='{"ok":true}')
            page.wait_for_timeout(100)
            expect(box).to_contain_text('Chưa xác nhận kết quả')
            expect(recover).to_have_count(0); expect(page.locator('#pcReset')).to_be_enabled()
            assert not errors, errors
            check('late prior-account receipt cannot replace or disable the new account controls')
            (OUT/'recovery-browser-result.json').write_text(json.dumps(dict(result='PASS', scenarios=7, browser=browser.version, viewport=[390,844], requests=len(fx.posts), errors=errors), indent=2), encoding='utf-8')
        finally: browser.close()
finally:
    server.shutdown(); server.server_close()
