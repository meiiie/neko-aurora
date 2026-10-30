"""Schedule editing in Chromium against loopback fixtures, with no agent/provider run."""
import functools
import http.server
import json
import os
import threading
from pathlib import Path
from fixture_http import FixtureHTTPServer
from urllib.parse import urlsplit
from playwright.sync_api import expect, sync_playwright

ROOT = Path(__file__).resolve().parents[3]
OUT = Path(os.environ.get('AURORA_BROWSER_AUDIT_DIR', ROOT / '.audit/2026-10-28-schedule-edit'))
OUT.mkdir(parents=True, exist_ok=True)

class Handler(http.server.SimpleHTTPRequestHandler):
    def log_message(self, *_): pass

class Fixture:
    def __init__(self):
        self.scope = 'schedule-owner-a'
        self.item = dict(id='schedule-fixture', bot='bot-a', text='Tóm tắt buổi sáng', at='08:00', repeat='daily', paused=True, revision=1)
        self.edits = []
        self.other_posts = []
        self.mode = 'ok'
        self.fail_get = False
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
        elif path.endswith('/messages'):
            data = []
        elif path == '/schedules':
            data = dict(schedules=[self.item])
            if self.fail_get: status, data = 500, dict(error='fixture unavailable')
        elif path == '/schedules/schedule-fixture/edit' and req.method == 'POST':
            body = req.post_data_json
            self.edits.append(body)
            assert body['workspaceId'] == self.scope
            if self.mode == 'held':
                self.held.append((route, body))
                return
            if self.mode == 'conflict':
                self.item.update(revision=self.item['revision']+1, text='Phiên bản mới ở máy chủ')
                status = 409
            elif self.mode == 'error': status = 503
            elif body['revision'] != self.item['revision']: status = 409
            else:
                self.item.update({k: body[k] for k in ('text', 'at', 'repeat', 'date', 'timezone')})
                self.item['intervalMinutes'] = body.get('intervalMinutes',0)
                self.item['revision'] += 1
                data = dict(schedule=self.item)
                if self.mode == 'lost':
                    route.abort('connectionreset')
                    return
                if self.mode == 'reload_error': self.fail_get = True
        elif req.method == 'POST': self.other_posts.append(path)
        route.fulfill(status=status, content_type='application/json', body=json.dumps(data))

def check(text): print('PASS '+text, flush=True)

server = FixtureHTTPServer(('127.0.0.1', 0), functools.partial(Handler, directory=str(ROOT/'app/native/internal/web/assets')))
threading.Thread(target=server.serve_forever, daemon=True).start()
try:
    with sync_playwright() as p:
        chrome = os.environ.get('AURORA_TEST_CHROME')
        if not chrome and Path('C:/Program Files/Google/Chrome/Application/chrome.exe').exists():
            chrome = 'C:/Program Files/Google/Chrome/Application/chrome.exe'
        browser = p.chromium.launch(headless=True, executable_path=chrome)
        try:
            context = browser.new_context(viewport=dict(width=390, height=844), locale='vi-VN', service_workers='block')
            page = context.new_page()
            fx, errors = Fixture(), []
            page.on('pageerror', lambda e: errors.append(str(e)))
            page.route('**/app/api/**', fx.route)
            page.goto(f'http://127.0.0.1:{server.server_port}/app/#bot-a')
            expect(page.locator('#whoBtn')).to_contain_text('Chat A')
            page.locator('#whoBtn').click()
            box = page.locator('#schedBox')
            edit = box.get_by_role('button', name='Sửa lịch', exact=True)
            form = box.get_by_role('form', name='Sửa lịch hẹn')
            prompt = form.get_by_label('Nội dung lịch')
            save = form.get_by_role('button', name='Lưu lịch', exact=True)
            refresh = form.get_by_role('button', name='Tải lại lịch', exact=True)
            status = form.get_by_role('status')

            edit.click()
            expect(prompt).to_be_focused()
            expect(box.get_by_role('button', name='Tiếp tục', exact=True)).to_be_disabled()
            expect(box.get_by_role('button', name='Xóa lịch', exact=True)).to_be_disabled()
            prompt.fill('Nháp sẽ hủy')
            form.get_by_role('button', name='Hủy chỉnh sửa').click()
            expect(form).to_have_count(0)
            expect(edit).to_be_focused()
            assert not fx.edits
            check('cancel keeps server data, unlocks controls and restores keyboard focus')

            edit.click()
            prompt.fill('Báo cáo ngày trong tuần')
            form.get_by_label('Giờ hẹn').fill('10:30')
            form.get_by_label('Lặp lại').select_option('weekdays')
            save.focus(); page.keyboard.press('Enter')
            expect(form).to_have_count(0)
            expect(edit).to_be_focused()
            assert fx.edits[-1] == dict(workspaceId='schedule-owner-a', revision=1, text='Báo cáo ngày trong tuần', at='10:30', repeat='weekdays', date='', timezone='Asia/Ho_Chi_Minh')
            assert fx.item['paused'] and fx.item['revision'] == 2
            check('keyboard save sends exact scope and revision while preserving pause')

            edit.click(); prompt.fill('Giữ bản nháp khi xung đột')
            fx.mode = 'conflict'; save.click()
            expect(status).to_contain_text('Lịch đã thay đổi')
            expect(prompt).to_have_value('Giữ bản nháp khi xung đột')
            fx.mode = 'ok'; refresh.click()
            expect(form).to_have_count(0)
            edit.click(); expect(prompt).to_have_value('Phiên bản mới ở máy chủ')
            check('conflict retains draft until explicit reload accepts newer server state')

            prompt.fill('Máy chủ đã nhận nhưng mất phản hồi')
            fx.mode = 'lost'; before = len(fx.edits); save.click()
            expect(status).to_contain_text('Chưa nhận được xác nhận')
            expect(prompt).to_have_value('Máy chủ đã nhận nhưng mất phản hồi')
            assert len(fx.edits) == before+1
            fx.mode = 'ok'; refresh.click(); expect(form).to_have_count(0)
            edit.click(); expect(prompt).to_have_value('Máy chủ đã nhận nhưng mất phản hồi')
            assert len(fx.edits) == before+1
            check('lost response is resolved by read-only reload without automatic replay')

            prompt.fill('Thử lại sau lỗi máy chủ')
            fx.mode = 'error'; save.click()
            expect(status).to_contain_text('Chưa nhận được xác nhận')
            failed = fx.edits[-1].copy()
            fx.mode = 'ok'; save.click(); expect(form).to_have_count(0)
            assert fx.edits[-1] == failed
            check('failed save preserves fields and explicit retry retains revision')

            edit.click(); prompt.fill('Đã lưu nhưng danh sách chưa tải')
            fx.mode = 'reload_error'; save.click()
            expect(status).to_contain_text('Đã lưu lịch.')
            expect(save).to_be_disabled(); expect(refresh).to_be_enabled()
            refresh.click()
            expect(status).to_contain_text('Chưa tải lại được lịch')
            expect(save).to_be_disabled()
            fx.mode = 'ok'; fx.fail_get = False; refresh.click(); expect(form).to_have_count(0)
            check('committed edit with failed reload never becomes an unsaved retry')

            edit.click(); form.get_by_label('Lặp lại').select_option('once')
            date = form.get_by_label('Ngày hẹn'); expect(date).to_be_visible()
            before = len(fx.edits); save.click()
            assert len(fx.edits) == before
            date.fill('2030-10-30'); save.click(); expect(form).to_have_count(0)
            assert fx.edits[-1]['date'] == '2030-10-30'
            check('one-time schedule requires explicit date before submission')

            edit.click(); prompt.fill('x'*1001); before = len(fx.edits); save.click()
            assert len(fx.edits) == before
            prompt.fill('Bản nháp trên màn hình nhỏ')
            page.screenshot(path=str(OUT/'schedule-mobile.png'))
            assert not page.evaluate('document.documentElement.scrollWidth>innerWidth')
            check('long prompt is blocked and 390px editor has no page overflow')

            fx.mode = 'held'; save.click()
            page.wait_for_function('document.querySelector(".schedule-editor textarea").disabled')
            assert len(fx.held) == 1
            fx.scope = 'schedule-owner-b'
            fx.item = dict(id='schedule-fixture', bot='bot-a', text='Lịch của tài khoản B', at='07:00', repeat='daily', paused=False, revision=1)
            page.evaluate('window.__AURORA.reload()')
            expect(page.locator('#profile')).to_have_class('screen profile off')
            page.locator('#rows .rowin').first.focus(); page.keyboard.press('Enter')
            assert not errors, errors
            page.wait_for_function('window.__AURORA.current()==="bot-a"')
            expect(page.locator('#thread')).not_to_have_class('screen thread off')
            expect(page.locator('#whoBtn')).to_contain_text('Chat A')
            page.locator('#whoBtn').click()
            assert not errors, errors
            expect(page.locator('#profile')).to_have_class('screen profile')
            expect(box).to_contain_text('Lịch của tài khoản B')
            edit.click(); prompt.fill('Nháp của tài khoản B')
            route, body = fx.held.pop()
            route.fulfill(status=200, content_type='application/json', body=json.dumps(dict(schedule=body)))
            page.wait_for_timeout(100)
            expect(box).to_contain_text('Lịch của tài khoản B')
            expect(prompt).to_have_value('Nháp của tài khoản B')
            assert not fx.other_posts and not errors, errors
            check('late edit receipt cannot reopen or replace another account editor')
            fx.mode = 'ok'
            expect(form.locator('option[value="interval"]')).to_have_count(1,timeout=1500)
            form.get_by_label('Lặp lại').select_option('interval')
            interval = form.get_by_label('Khoảng lặp (phút)',exact=True)
            expect(interval).to_be_visible();expect(form.get_by_label('Giờ hẹn',exact=True)).to_be_hidden();expect(form.get_by_label('Múi giờ',exact=True)).to_be_hidden();expect(date).to_be_hidden()
            before=len(fx.edits)
            for value in ['14','15.5','10081']:
                interval.fill(value);save.click();assert len(fx.edits)==before
            interval.fill('60');page.screenshot(path=str(OUT/'interval-mobile.png'));save.click();expect(form).to_have_count(0)
            assert fx.edits[-1]==dict(workspaceId='schedule-owner-b',revision=1,text='Nháp của tài khoản B',at='',repeat='interval',date='',timezone='Asia/Ho_Chi_Minh',intervalMinutes=60)
            expect(box).to_contain_text('Mỗi 60 phút')
            check('interval mode validates whole minutes and sends exact workspace revision and cadence')
            edit.click();expect(interval).to_have_value('60');prompt.fill('Nội dung mới cùng nhịp');save.click();expect(form).to_have_count(0)
            assert fx.edits[-1]['intervalMinutes']==60 and fx.edits[-1]['revision']==2
            check('reopening and a text-only edit retain the interval value')
            edit.click();form.get_by_label('Lặp lại').select_option('daily');expect(interval).to_be_hidden()
            expect(form.get_by_label('Giờ hẹn',exact=True)).to_be_visible();form.get_by_label('Giờ hẹn').fill('08:30');form.get_by_label('Múi giờ').select_option('America/New_York');save.click();expect(form).to_have_count(0)
            assert 'intervalMinutes' not in fx.edits[-1] and fx.edits[-1]['at']=='08:30' and fx.edits[-1]['timezone']=='America/New_York'
            assert not errors, errors
            check('switching back to a calendar schedule sends no interval metadata')
            (OUT/'schedule-browser-result.json').write_text(json.dumps(dict(result='PASS', scenarios=12, browser=browser.version, viewport=[390,844], editRequests=len(fx.edits), otherPosts=fx.other_posts, errors=errors), indent=2), encoding='utf-8')
        finally: browser.close()
finally:
    server.shutdown(); server.server_close()
