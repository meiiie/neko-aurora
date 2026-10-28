"""Headless browser fixtures; not provider, cloud sync, or physical-phone acceptance.

Starts an isolated loopback static server, or uses AURORA_TEST_URL when set.
Requires Python Playwright and Chromium; no application credentials are used.
"""
import json
import os
import functools
import http.server
import threading
from pathlib import Path
from urllib.parse import urlsplit

from playwright.sync_api import expect, sync_playwright

ROOT = Path(__file__).resolve().parents[3]
OUT = ROOT / '.audit/2026-10-28-grok'
URL = os.environ.get('AURORA_TEST_URL', '')
server = None
if not URL:
    class StaticHandler(http.server.SimpleHTTPRequestHandler):
        def log_message(self, *_):
            pass
    handler = functools.partial(StaticHandler, directory=str(ROOT / 'app/native/internal/web/assets'))
    server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), handler)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    URL = f'http://127.0.0.1:{server.server_port}'
if urlsplit(URL).hostname not in ('127.0.0.1', 'localhost'):
    raise ValueError('fixture server must be loopback')
OUT.mkdir(parents=True, exist_ok=True)
MARKDOWN = '| Tên | Giá trị |\n| --- | --- |\n| Việt Nam | **42** |\n| <img src=x onerror=alert(1)> | `safe` |'


class Fixture:
    def __init__(self):
        self.scope = 'workspace-browser-fixture'
        self.bot = {'id': 'bot-fixture', 'name': 'Aurora Fixture', 'desc': 'Dữ liệu kiểm thử', 'status': 'idle', 'snip': 'Bảng và câu hỏi', 't': '2026-10-28T04:00:00Z', 'av': {'shape': 0, 'color': 4}}
        self.messages = [{'id': 'fixture-question', 'taskId': 'req-fixture', 'agentId': self.bot['id'], 'role': 'agent', 'text': MARKDOWN, 'choices': ['Sáng', 'Chiều'], 't': '2026-10-28T04:00:00Z'}]
        self.messages.append({'id': 'fixture-form', 'taskId': 'req-form', 'agentId': self.bot['id'], 'role': 'agent', 'text': '', 'form': {'title': 'Thông tin dự án', 'fields': [{'name': 'name', 'label': 'Tên dự án', 'type': 'text', 'required': True}]}, 't': '2026-10-28T04:00:00Z'})
        self.memory = []
        self.schedule = {'id': 'schedule-fixture', 'at': '08:00', 'text': 'Nhắc hẹn', 'repeat': 'daily', 'revision': 1, 'paused': False}
        self.posts = []
        self.receipts = {}
        self.offline = False
        self.lose_response = False
        self.refuse = False
        self.stale_memory = False
        self.stale_schedule = False
        self.premature_form_completions = 0
        self.mcp_state = 'reconnect_required'
        self.mcp_resets = 0

    def route(self, route):
        req = route.request
        path = urlsplit(req.url).path.removeprefix('/app/api')
        data, code = {}, 200
        if path == '/events':
            route.fulfill(status=200, content_type='text/event-stream', body='')
            return
        if path == '/state':
            data = {'workspaceId': self.scope, 'agents': [self.bot], 'pending': [], 'ai': {}, 'secrets': [], 'outbound': {}}
        elif path.endswith('/messages'):
            if req.method == 'GET':
                data = self.messages
            else:
                body = req.post_data_json
                self.posts.append(body)
                if self.offline:
                    route.abort('internetdisconnected')
                    return
                if self.refuse:
                    code, data = 401, {'error': 'signin_required'}
                else:
                    nonce = body['clientNonce']
                    if nonce not in self.receipts:
                        receipt = {'id': nonce + ':u', 'taskId': nonce, 'agentId': self.bot['id'], 'role': 'user', 'text': body['text'], 'answerFor': body.get('answerFor'), 'formFor': body.get('formFor'), 't': '2026-10-28T04:01:00Z'}
                        self.receipts[nonce] = receipt
                        self.messages.append(receipt)
                    code, data = 202, self.receipts[nonce]
                    if self.lose_response:
                        self.lose_response = False
                        route.abort('connectionreset')
                        return
        elif path.startswith('/forms/'):
            self.premature_form_completions += 1
            code, data = 409, {'error': 'receipt_required'}
        elif path.endswith('/mcp/reconnect'):
            self.mcp_resets += 1
            self.mcp_state = 'ready'
            data = {'ok': True}
        elif path.endswith('/ext'):
            data = {'workspaceId': self.scope, 'mcpRevision': 0, 'menu': [], 'allSkills': [], 'mcp': [{'name': 'Fixture MCP', 'url': 'https://example.invalid/mcp', 'state': self.mcp_state, 'error': '' if self.mcp_state == 'ready' else 'Cần kiểm tra kết nối', 'retryAt': '2099-01-01T00:00:00Z' if self.mcp_state == 'rate_limited' else '', 'tools': [{'name': 'fixture_lookup'}] if self.mcp_state == 'ready' else []}]}
        elif path.endswith('/memory/import/preview'):
            data = {'revision': 'fixture-revision', 'items': ['Dùng tiếng Việt', 'Câu trả lời ngắn'], 'remaining': 30, 'duplicates': 1, 'rejected': 1}
        elif path.endswith('/memory/import'):
            if self.stale_memory:
                self.stale_memory = False
                code, data = 409, {'error': 'nonce_conflict'}
            else:
                self.memory = [{'id': str(i), 'text': text} for i, text in enumerate(req.post_data_json['items'])]
                data = {'added': len(self.memory)}
        elif path.endswith('/memory'):
            data = {'items': self.memory, 'team': [], 'journal': []}
        elif path == '/schedules':
            data = {'schedules': [self.schedule] if self.schedule else []}
        elif path.startswith('/schedules/') and req.method == 'POST':
            body = req.post_data_json
            if self.stale_schedule:
                self.stale_schedule = False
                self.schedule['revision'] += 1
                code = 409
            elif body['revision'] != self.schedule['revision']:
                code = 409
            elif path.endswith('/delete'):
                self.schedule = None
            else:
                self.schedule['paused'] = body['paused']
                self.schedule['revision'] += 1
        route.fulfill(status=code, content_type='application/json', body=json.dumps(data))


def check(label, condition=True):
    assert condition, label
    print('PASS ' + label)


with sync_playwright() as p:
    executable = os.environ.get('AURORA_TEST_CHROME')
    windows_chrome = Path('C:/Program Files/Google/Chrome/Application/chrome.exe')
    if not executable and windows_chrome.exists():
        executable = str(windows_chrome)
    browser = p.chromium.launch(headless=True, executable_path=executable)
    try:
        context = browser.new_context(viewport={'width': 1280, 'height': 900}, locale='vi-VN', service_workers='block', permissions=['clipboard-read', 'clipboard-write'])
        context.add_init_script("Object.defineProperty(Crypto.prototype,'randomUUID',{value:undefined})")
        page = context.new_page()
        fixture, errors = Fixture(), []
        page.on('pageerror', lambda error: errors.append(str(error)))
        page.route('**/app/api/**', fixture.route)

        def load():
            if page.url.startswith(URL):
                page.reload()
            else:
                page.goto(URL + '/app/#bot-fixture')
            page.wait_for_load_state('networkidle')
            try:
                expect(page.locator('#whoBtn')).to_contain_text('Aurora Fixture')
            except AssertionError:
                print('LOAD DIAGNOSTICS ' + json.dumps({'url': page.url, 'errors': errors, 'body': page.locator('body').inner_text()[-1600:]}, ensure_ascii=True))
                page.screenshot(path=str(OUT / 'load-failure.png'))
                raise

        def submit(text):
            page.locator('#input').fill(text)
            page.locator('#composer').evaluate('(form)=>form.requestSubmit()')

        load()
        (OUT / 'browser-dom.json').write_text(json.dumps({'buttons': page.get_by_role('button').all_text_contents()}, ensure_ascii=False), encoding='utf-8')
        check('rendered conversation and controls', page.locator('.table-card').count() == 1)
        check('table treats HTML as text', page.locator('.table-card img').count() == 0)
        page.get_by_role('button', name='Sao chép bảng', exact=True).click()
        page.wait_for_function('navigator.clipboard.readText().then(x=>x.includes("Việt Nam\\t42"))')
        check('table copies TSV')
        page.get_by_role('button', name='Sao chép Markdown', exact=True).click()
        page.wait_for_function('(text)=>navigator.clipboard.readText().then(x=>x===text)', arg=MARKDOWN)
        check('table copies source Markdown')
        expand = page.get_by_role('button', name='Mở rộng bảng')
        expand.click()
        expect(page.get_by_role('dialog')).to_be_visible()
        page.get_by_role('button', name='Phóng to bảng').click()
        expect(page.get_by_role('button', name='Phóng to bảng')).to_have_text('125%')
        page.screenshot(path=str(OUT / 'table-desktop.png'))
        page.keyboard.press('Escape')
        expect(page.get_by_role('dialog')).to_have_count(0)
        expect(expand).to_be_focused()
        check('table zoom, Escape and focus return')

        submit('Tôi muốn hỏi thêm trước khi chọn')
        expect(page.locator('.pending')).to_have_count(0)
        expect(page.get_by_role('button', name='Sáng', exact=True)).to_be_enabled()
        check('ordinary follow-up leaves question open')
        page.get_by_role('textbox', name='Câu trả lời khác').fill('Tối cuối tuần')
        page.get_by_role('button', name='Trả lời', exact=True).click()
        expect(page.get_by_role('button', name='Sáng', exact=True)).to_be_disabled()
        load()
        expect(page.get_by_role('button', name='Sáng', exact=True)).to_be_disabled()
        check('custom answer stays associated after reload', any(x.get('answerFor') == 'req-fixture' for x in fixture.posts))

        fixture.offline = True
        form = page.locator('.form-card[data-id="req-form"]')
        form.get_by_role('textbox', name='Tên dự án').fill('Aurora offline')
        form.get_by_role('button', name='Gửi', exact=True).click()
        expect(form.get_by_role('button')).to_have_text('Đang chờ xác nhận…')
        load()
        expect(form.get_by_role('textbox')).to_have_value('Aurora offline')
        expect(form.get_by_role('button')).to_have_text('Đang chờ xác nhận…')
        check('form reload preserves pending values without claiming acceptance', fixture.premature_form_completions == 0)
        fixture.offline = False
        fixture.lose_response = True
        expect(form.get_by_role('button')).to_have_text('Đã gửi', timeout=12000)
        load()
        expect(form.get_by_role('button')).to_have_text('Đã gửi')
        check('form becomes complete only after idempotent receipt', len([x for x in fixture.receipts.values() if x.get('formFor') == 'req-form']) == 1)

        page.locator('#whoBtn').click()
        page.locator('#prExt').click()
        page.get_by_role('button', name='Kết nối lại · Fixture MCP').click()
        expect(page.get_by_role('button', name='Kết nối lại · Fixture MCP')).to_have_count(0)
        expect(page.locator('#extBody')).to_contain_text('fixture_lookup')
        check('MCP reconnect uses explicit action and refreshes tools', fixture.mcp_resets == 1)
        fixture.mcp_state = 'rate_limited'
        load()
        page.locator('#whoBtn').click()
        page.locator('#prExt').click()
        expect(page.get_by_role('button', name='Kết nối lại · Fixture MCP')).to_be_disabled()
        check('MCP rate limit shows a disabled retry during cooldown')
        load()

        page.locator('#whoBtn').click()
        page.get_by_role('button', name='Tạm dừng', exact=True).click()
        expect(page.get_by_role('button', name='Tiếp tục', exact=True)).to_be_visible()
        fixture.stale_schedule = True
        page.get_by_role('button', name='Tiếp tục', exact=True).click()
        expect(page.get_by_role('button', name='Tiếp tục', exact=True)).to_be_enabled()
        check('stale schedule refreshes without changing it', fixture.schedule['paused'])
        page.get_by_role('button', name='Tiếp tục', exact=True).click()
        expect(page.get_by_role('button', name='Tạm dừng', exact=True)).to_be_visible()
        page.get_by_role('button', name='Xóa lịch', exact=True).click()
        expect(page.locator('#schedBox')).to_contain_text('Chưa có lịch hẹn')
        check('schedule pause, resume and delete controls')
        page.locator('#prMem').click()
        page.get_by_role('button', name='Nhập bộ nhớ').click()
        page.get_by_role('textbox', name='Nội dung bộ nhớ cần nhập').fill('Dùng tiếng Việt\nCâu trả lời ngắn\nDòng trùng\nDòng không phù hợp')
        page.get_by_role('button', name='Xem trước', exact=True).click()
        expect(page.locator('.memory-import-pick')).to_have_count(2)
        page.locator('.memory-import-pick input').nth(1).uncheck()
        fixture.stale_memory = True
        page.get_by_role('button', name='Lưu các mục đã chọn').click()
        expect(page.locator('.memory-import')).to_contain_text('Bộ nhớ đã thay đổi')
        expect(page.get_by_role('textbox', name='Nội dung bộ nhớ cần nhập')).not_to_be_empty()
        page.get_by_role('button', name='Xem trước', exact=True).click()
        page.locator('.memory-import-pick input').nth(1).uncheck()
        page.get_by_role('button', name='Lưu các mục đã chọn').click()
        expect(page.locator('.memory-import')).to_have_count(0)
        check('memory review handles stale revision and only saves selected facts', len(fixture.memory) == 1)

        load()
        fixture.offline = True
        submit('Tin chưa gửi phải còn sau tải lại')
        expect(page.locator('.pending')).to_contain_text('Tin chưa gửi phải còn')
        load()
        expect(page.locator('.pending')).to_contain_text('Tin chưa gửi phải còn')
        check('outbox survives full reload offline')
        fixture.offline = False
        fixture.lose_response = True
        expect(page.locator('.pending')).to_have_count(0, timeout=12000)
        copies = [x for x in fixture.receipts.values() if x['text'] == 'Tin chưa gửi phải còn sau tải lại']
        check('lost receipt retries use one nonce and one accepted message', len(copies) == 1)

        fixture.offline = True
        submit('Chỉ được gửi ở tài khoản A')
        expect(page.locator('.pending')).to_contain_text('tài khoản A')
        count = len(fixture.posts)
        fixture.scope = 'workspace-browser-other'
        load()
        expect(page.locator('.pending')).to_have_count(0)
        page.wait_for_timeout(2200)
        check('old queue never dispatches into a different workspace', len(fixture.posts) == count)

        fixture.offline = False
        fixture.refuse = True
        submit('Nội dung bị từ chối vẫn còn')
        expect(page.locator('.pending')).to_contain_text('Phiên đăng nhập đã hết')
        count = len(fixture.posts)
        load()
        expect(page.locator('.pending')).to_contain_text('Nội dung bị từ chối vẫn còn')
        check('final refusal is retained and not retried after reload', len(fixture.posts) == count)
        page.get_by_role('button', name='Bỏ khỏi hàng đợi').click()
        page.evaluate('''()=>{const original=Storage.prototype.setItem;Storage.prototype.setItem=function(k,v){if(k.startsWith('aurora.outbox.'))throw new DOMException('fixture','QuotaExceededError');return original.call(this,k,v)}}''')
        submit('Không được mất khi đĩa đầy')
        expect(page.locator('#input')).to_have_value('Không được mất khi đĩa đầy')
        check('storage failure preserves composer text')

        page.set_viewport_size({'width': 390, 'height': 844})
        expand.click()
        expect(page.get_by_role('dialog')).to_be_visible()
        bounds = page.get_by_role('dialog').bounding_box()
        check('expanded table fits mobile viewport', bounds['x'] >= 0 and bounds['x'] + bounds['width'] <= 391)
        page.screenshot(path=str(OUT / 'table-mobile.png'))
        page.keyboard.press('Escape')
        check('no uncaught browser errors', not errors)
        print('PASS browser fixture suite; desktop 1280x900 and mobile viewport 390x844')
    except Exception:
        print('BROWSER DIAGNOSTICS', json.dumps(page.evaluate('''()=>({ready:document.readyState,url:location.href,body:document.body.innerText.slice(-800),app:!!window.__AURORA,resources:performance.getEntriesByType('resource').map(e=>({name:e.name,duration:e.duration,size:e.transferSize}))})''')))
        page.screenshot(path=str(OUT/'adoption-failure.png'))
        raise
    finally:
        browser.close()
        if server:
            server.shutdown()
            server.server_close()
