"""Schedule editing in Chromium against loopback fixtures, with no agent/provider run."""
import functools
import http.server
import json
import os
import threading
from pathlib import Path
from urllib.parse import urlsplit
from playwright.sync_api import expect, sync_playwright

ROOT = Path(__file__).resolve().parents[3]
OUT = ROOT / '.audit/2026-10-29-schedule-timezones'
OUT.mkdir(parents=True, exist_ok=True)

class Server(http.server.ThreadingHTTPServer): request_queue_size=64

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
        elif path.endswith('/computer'):data=dict(available=False,log=[],files=[])
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
            elif self.mode == 'error': status = 500
            elif body['revision'] != self.item['revision']: status = 409
            else:
                self.item.update({k: body[k] for k in ('text', 'at', 'repeat', 'date', 'timezone')})
                self.item['revision'] += 1
                data = dict(schedule=self.item)
                if self.mode == 'lost':
                    route.abort('connectionreset')
                    return
                if self.mode == 'reload_error': self.fail_get = True
        elif req.method == 'POST': self.other_posts.append(path)
        route.fulfill(status=status, content_type='application/json', body=json.dumps(data))

def check(text): print('PASS '+text, flush=True)

server = Server(('127.0.0.1', 0), functools.partial(Handler, directory=str(ROOT/'app/native/internal/web/assets')))
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

            zone = form.get_by_label('Múi giờ', exact=True)
            edit.click();expect(zone).to_have_value('Asia/Ho_Chi_Minh');expect(form.get_by_label('Giờ hẹn',exact=True)).to_have_value('08:00')
            expect(zone.locator('option[value="Europe/Kyiv"]')).to_have_count(1);expect(zone.locator('option[value="UTC"]')).to_have_count(1)
            assert not fx.edits
            check('legacy schedule starts in Vietnam with explicit Kyiv and UTC choices, without changing the server')
            zone.select_option('Europe/Kyiv');prompt.fill('Kyiv morning');form.get_by_label('Giờ hẹn',exact=True).fill('09:30');save.focus();page.keyboard.press('Enter')
            expect(form).to_have_count(0);expect(box).to_contain_text('Europe/Kyiv')
            assert fx.edits[-1]==dict(workspaceId='schedule-owner-a',revision=1,text='Kyiv morning',at='09:30',repeat='daily',date='',timezone='Europe/Kyiv')
            assert fx.item['paused'] and fx.item['revision']==2
            check('keyboard save binds the chosen timezone to the existing workspace and revision and preserves pause')
            page.reload();expect(page.locator('#whoBtn')).to_contain_text('Chat A');page.locator('#whoBtn').click();edit.click();expect(zone).to_have_value('Europe/Kyiv')
            zone.select_option('UTC');before=len(fx.edits);form.get_by_role('button',name='Hủy chỉnh sửa').click();expect(form).to_have_count(0);expect(box).to_contain_text('Europe/Kyiv');assert len(fx.edits)==before
            check('reload displays the saved zone and cancel does not change it')
            edit.click();zone.select_option('America/New_York');prompt.fill('Keep timezone draft');fx.mode='error';save.click();expect(status).to_contain_text('Chưa nhận được xác nhận');expect(zone).to_have_value('America/New_York');expect(prompt).to_have_value('Keep timezone draft')
            fx.mode='conflict';save.click();expect(status).to_contain_text('Lịch đã thay đổi');expect(zone).to_have_value('America/New_York')
            fx.mode='ok';refresh.click();expect(form).to_have_count(0);edit.click();expect(zone).to_have_value('Europe/Kyiv')
            check('failed or conflicting save retains the selected zone until explicit server reload')
            zone.select_option('UTC');save.click();expect(form).to_have_count(0);expect(box).to_contain_text('UTC')
            edit.click();form.scroll_into_view_if_needed();page.screenshot(path=str(OUT/'timezone-mobile.png'))
            assert not fx.other_posts and not errors,errors
            check('selected zone is visible in the list and editor with no schedule execution or unrelated mutation')
            with page.expect_navigation(wait_until='networkidle'):page.evaluate('window.__I18N.set("en")')
            page.locator('#whoBtn').click();page.locator('#schedBox').get_by_role('button',name='Edit schedule',exact=True).click()
            english=page.locator('#schedBox .schedule-editor');expect(english.get_by_label('Time zone',exact=True)).to_have_value('UTC');expect(english.get_by_label('Scheduled time',exact=True)).to_have_value('09:30');expect(english).to_contain_text('Times use the selected time zone.')
            expect(english.get_by_label('Scheduled prompt')).to_have_value('Phiên bản mới ở máy chủ')
            expect(page.locator('#schedBox .schedule-row > .cell .cell-t small')).to_have_text('Phiên bản mới ở máy chủ')
            page.set_viewport_size({'width':1280,'height':900});english.scroll_into_view_if_needed();page.screenshot(path=str(OUT/'timezone-desktop-en.png'))
            check('English labels and guidance translate without changing the saved zone or Vietnamese prompt')
            (OUT/'timezone-browser-result.json').write_text(json.dumps({'result':'PASS','scenarios':6,'browser':browser.version,'errors':errors}),encoding='utf-8')
        finally: browser.close()
finally:
    server.shutdown(); server.server_close()
