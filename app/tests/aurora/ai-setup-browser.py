"""AI setup recovery with isolated HTTP fixtures; no provider calls or credentials."""
import functools
import http.server
import json
import os
import threading
from pathlib import Path
from urllib.parse import urlsplit

from playwright.sync_api import expect, sync_playwright
from fixture_http import FixtureHTTPServer

ROOT = Path(__file__).resolve().parents[3]
OUT = Path(os.environ.get('AURORA_BROWSER_AUDIT_DIR', ROOT / '.audit/ai-setup-browser'))
OUT.mkdir(parents=True, exist_ok=True)
TEXT = 'Tin nhắn đã được lưu nhưng chưa được xử lý vì chưa có kết nối AI. Hãy thiết lập AI, rồi gửi lại tin nhắn khi bạn muốn tiếp tục.'


class Static(http.server.SimpleHTTPRequestHandler):
    def log_message(self, *_):
        pass


class Fixture:
    def __init__(self):
        self.configured = False
        self.fail_models = False
        self.posts = []
        self.ext_reads = []

    def route(self, route):
        request = route.request
        path = urlsplit(request.url).path.removeprefix('/app/api')
        if request.method != 'GET':
            self.posts.append(path)
        status, data = 200, {}
        if path == '/state':
            data = {'workspaceId': 'setup-fixture', 'agents': [
                {'id': 'bot-a', 'name': 'An', 'status': 'error', 'snip': 'Cần thiết lập AI để trả lời.'},
                {'id': 'bot-b', 'name': 'Bình', 'status': 'idle', 'snip': 'Sẵn sàng nhận việc.'},
            ], 'pending': [], 'ai': {'configured': False}, 'secrets': [], 'outbound': {}}
        elif path == '/events':
            route.fulfill(content_type='text/event-stream', body='')
            return
        elif path.endswith('/messages'):
            bot = path.split('/')[2]
            data = [
                {'id': bot + '-user', 'agentId': bot, 'role': 'user', 'text': 'Giúp mình bắt đầu.', 't': '2026-10-30T15:00:00Z', 'recovery': 'configure_ai'},
                {'id': bot + '-failure', 'agentId': bot, 'role': 'system', 'text': TEXT, 'recovery': 'configure_ai', 't': '2026-10-30T15:00:01Z'},
            ]
        elif path == '/models':
            if self.fail_models:
                status, data = 503, {'error': 'fixture_unavailable'}
            else:
                data = {'models': [], 'providers': [], 'vision': 'glm:fixture', 'browser': False}
                if self.configured is not None:
                    data['defaultConfigured'] = self.configured
        elif path.endswith('/ext'):
            self.ext_reads.append(path)
            data = {'workspaceId': 'setup-fixture', 'mcpRevision': 0, 'model': '', 'menu': [], 'allSkills': [], 'mcp': []}
        route.fulfill(status=status, content_type='application/json', body=json.dumps(data))


server = FixtureHTTPServer(('127.0.0.1', 0), functools.partial(Static, directory=str(ROOT / 'app/native/internal/web/assets')))
threading.Thread(target=server.serve_forever, daemon=True).start()
fixture = Fixture()
checks = []


def passed(label):
    checks.append(label)
    print('PASS ' + label, flush=True)


with sync_playwright() as playwright:
    browser = playwright.chromium.launch(executable_path='C:/Program Files/Google/Chrome/Application/chrome.exe', headless=True)
    context = browser.new_context(viewport={'width': 1440, 'height': 1000}, reduced_motion='reduce', service_workers='block')
    context.add_init_script("if(!localStorage.getItem('neko-aurora-lang'))localStorage.setItem('neko-aurora-lang','vi'); window.EventSource=class extends EventTarget {close(){}};")
    page = context.new_page()
    errors = []
    page.on('pageerror', lambda error: errors.append(str(error)))
    page.route('**/app/api/**', fixture.route)
    try:
        page.goto(f'http://127.0.0.1:{server.server_port}/app/#bot-a')
        card = page.locator('.ai-setup-card')
        expect(card).to_have_count(1)
        expect(card).to_contain_text('đã được lưu nhưng chưa được xử lý')
        expect(card.get_by_role('button', name='Thiết lập AI', exact=True)).to_be_visible()
        assert 'ZAI_API_KEY' not in page.locator('#messages').inner_text()
        passed('clear saved/unprocessed message; recovery only on system message')

        page.locator('#input').fill('Bản nháp đang viết, chưa gửi.')
        card.get_by_role('button', name='Thiết lập AI', exact=True).click()
        expect(page.locator('#extBody')).to_contain_text('Chưa thiết lập kết nối AI')
        expect(page.locator('#extBody')).not_to_contain_text('Có sẵn trên máy chủ')
        expect(page.locator('#extBody')).to_contain_text('không tự gửi lại')
        page.get_by_text('Cấu hình nâng cao', exact=True).click()
        page.get_by_role('button', name='Chọn mô hình cho bot này', exact=True).click()
        expect(page.locator('#extModel')).to_be_visible()
        assert fixture.ext_reads == ['/agents/bot-a/ext']
        page.locator('#closeExt').click()
        page.wait_for_timeout(450)
        expect(page.locator('#input')).to_have_value('Bản nháp đang viết, chưa gửi.')
        passed('setup opens real settings and correct bot selection without sending draft')

        fixture.fail_models = True
        card.get_by_role('button', name='Thiết lập AI', exact=True).click()
        expect(page.locator('#extBody')).to_contain_text('Chưa tải được cấu hình AI')
        fixture.fail_models = False
        page.locator('#extBody').get_by_role('button', name='Thử lại', exact=True).click()
        expect(page.locator('#extBody')).to_contain_text('Chưa thiết lập kết nối AI')
        page.locator('#closeExt').click()
        page.wait_for_timeout(450)
        passed('settings load failure can retry')

        for state, wording in [(True, 'Đã có cấu hình · chưa kiểm tra kết nối'), (None, 'Chưa xác định trạng thái cấu hình')]:
            fixture.configured = state
            card.get_by_role('button', name='Thiết lập AI', exact=True).click()
            expect(page.locator('#extBody')).to_contain_text(wording)
            page.locator('#closeExt').click()
            page.wait_for_timeout(450)
        passed('configured and legacy API states do not claim verified connectivity')

        page.reload()
        expect(card).to_have_count(1)
        expect(page.locator('#input')).to_have_value('Bản nháp đang viết, chưa gửi.')
        page.screenshot(path=str(OUT / 'setup-desktop.png'), full_page=True)
        page.set_viewport_size({'width': 390, 'height': 844})
        expect(card.get_by_role('button', name='Thiết lập AI', exact=True)).to_be_in_viewport()
        assert page.evaluate('document.documentElement.scrollWidth <= innerWidth')
        page.screenshot(path=str(OUT / 'setup-mobile.png'), full_page=True)
        passed('reload retains unsent draft; desktop/mobile recovery stays visible')

        page.evaluate("localStorage.setItem('neko-aurora-lang','en')")
        page.reload()
        expect(card).to_contain_text('Your message was saved but could not be processed')
        expect(card.get_by_role('button', name='Set up AI', exact=True)).to_be_visible()
        assert not fixture.posts, fixture.posts
        assert not errors, errors
        passed('English translation; no mutations/provider requests or JavaScript errors')
        (OUT / 'result.json').write_text(json.dumps({'checks': checks, 'mutations': fixture.posts, 'errors': errors}, indent=2), encoding='utf-8')
    finally:
        context.close()
        browser.close()
        server.shutdown()
