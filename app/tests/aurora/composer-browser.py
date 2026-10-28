"""Composer lifecycle fixtures against the real local UI, with loopback APIs.

No model, upload service, microphone or cloud account is contacted.
"""
import base64
import functools
import http.server
import json
import os
import threading
import time
from pathlib import Path
from urllib.parse import urlsplit

from playwright.sync_api import expect, sync_playwright

ROOT = Path(__file__).resolve().parents[3]
OUT = ROOT / '.audit/2026-10-28-composer'
OUT.mkdir(parents=True, exist_ok=True)
PNG = base64.b64decode('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+a9wAAAABJRU5ErkJggg==')


class Handler(http.server.SimpleHTTPRequestHandler):
    def log_message(self, *_):
        pass


class Fixture:
    def __init__(self):
        self.scope = 'composer-owner-a'
        self.agents = [{'id': 'bot-' + x, 'name': 'Chat ' + x.upper(), 'status': 'idle', 't': '2026-10-28T04:00:00Z', 'av': {'shape': 0, 'color': 4}} for x in ['a', 'b']]
        self.uploads = []
        self.posts = []
        self.messages = []

    def route(self, route):
        req, data = route.request, {}
        path = urlsplit(req.url).path.removeprefix('/app/api')
        if path == '/events':
            route.fulfill(status=200, content_type='text/event-stream', body='')
            return
        if path == '/state':
            data = {'workspaceId': self.scope, 'agents': self.agents, 'pending': [], 'ai': {}, 'secrets': [], 'outbound': {}}
        elif path.endswith('/messages'):
            if req.method == 'GET':
                data = [m for m in self.messages if m['agentId'] in path]
            else:
                body = req.post_data_json
                self.posts.append(body)
                data = {'id': body['clientNonce']+':u', 'taskId': body['clientNonce'], 'agentId': path.split('/')[2], 'role': 'user', 'text': body['text'], 't': '2026-10-28T04:01:00Z'}
                self.messages.append(data)
        elif path.endswith('/computer/upload'):
            self.uploads.append(route)
            return
        elif path.endswith('/ext'):
            data = {'menu': [], 'allSkills': [], 'mcp': []}
        route.fulfill(status=202 if req.method == 'POST' and path.endswith('/messages') else 200, content_type='application/json', body=json.dumps(data))


def check(label):
    print('PASS ' + label)


server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), functools.partial(Handler, directory=str(ROOT/'app/native/internal/web/assets')))
threading.Thread(target=server.serve_forever, daemon=True).start()
url = f'http://127.0.0.1:{server.server_port}'
try:
    with sync_playwright() as p:
        chrome = os.environ.get('AURORA_TEST_CHROME')
        if not chrome and Path('C:/Program Files/Google/Chrome/Application/chrome.exe').exists():
            chrome = 'C:/Program Files/Google/Chrome/Application/chrome.exe'
        browser = p.chromium.launch(headless=True, executable_path=chrome)
        try:
            context = browser.new_context(viewport={'width':1280, 'height':900}, service_workers='block')
            context.add_init_script('window.SpeechRecognition=class{constructor(){window.fixtureRec=this}start(){}stop(){}}')
            page = context.new_page()
            fixture, errors = Fixture(), []
            page.on('pageerror', lambda e: errors.append(str(e)))
            page.route('**/app/api/**', fixture.route)
            page.goto(url+'/app/#bot-a')
            page.wait_for_load_state('networkidle')
            expect(page.locator('#whoBtn')).to_contain_text('Chat A')

            def switch(bot):
                page.locator('#backBtn').click()
                expect(page.locator('#thread')).to_have_class('screen thread off')
                page.locator(f'#rows .row[data-id="bot-{bot}"] .rowin').click()
                expect(page.locator('#whoBtn')).to_contain_text('Chat '+bot.upper())

            def upload():
                count = len(fixture.uploads)
                page.locator('#imgPick').set_input_files({'name':'same-name.png', 'mimeType':'image/png', 'buffer':PNG})
                deadline = time.monotonic()+10
                while len(fixture.uploads) == count and time.monotonic() < deadline:
                    page.wait_for_timeout(30)
                assert len(fixture.uploads) == count+1, 'upload never reached fixture'
                return fixture.uploads[-1]

            page.locator('#input').fill('Nháp A\nGiữ khoảng trắng ')
            page.locator('#plusBtn').click()
            page.locator('#pmRef').click()
            page.locator('#refBody').get_by_text('Chat B', exact=True).click()
            expect(page.locator('#refChip')).to_contain_text('Chat B')
            page.wait_for_timeout(450)  # Existing sheet-close ghost-click guard lasts 400 ms.
            switch('b')
            expect(page.locator('#input')).to_have_value('')
            expect(page.locator('#refChip')).to_have_count(0)
            page.locator('#input').fill('Nháp B')
            switch('a')
            expect(page.locator('#input')).to_have_value('Nháp A\nGiữ khoảng trắng ')
            expect(page.locator('#refChip')).to_contain_text('Chat B')
            page.reload()
            expect(page.locator('#input')).to_have_value('Nháp A\nGiữ khoảng trắng ')
            expect(page.locator('#refChip')).to_contain_text('Chat B')
            check('independent text and references survive chat switches and reload')

            pending = upload()
            assert pending.request.post_data_json['workspaceId'] == fixture.scope
            switch('b')
            pending.fulfill(status=200, content_type='application/json', body='{"path":"uploads/first.jpg"}')
            expect(page.locator('#toast')).to_contain_text('chat ban đầu')
            expect(page.locator('#input')).to_have_value('Nháp B')
            switch('a')
            expect(page.locator('#input')).to_have_value('Nháp A\nGiữ khoảng trắng \n[Ảnh: uploads/first.jpg]')
            page.reload()
            expect(page.locator('#input')).to_have_value('Nháp A\nGiữ khoảng trắng \n[Ảnh: uploads/first.jpg]')
            check('delayed upload returns to original draft and survives reload')

            before = page.locator('#input').input_value()
            pending = upload()
            pending.fulfill(status=500, content_type='application/json', body='{"error":"fixture_failed"}')
            expect(page.locator('#toast')).to_contain_text('Nháp vẫn được giữ')
            expect(page.locator('#input')).to_have_value(before)
            check('failed upload preserves text, reference and earlier attachment')

            page.locator('#micBtn').click()
            switch('b')
            page.evaluate('window.fixtureRec.onresult({results:[[{transcript:"late voice"}]]})')
            expect(page.locator('#input')).to_have_value('Nháp B')
            check('late speech callback cannot write into another chat')

            pending = upload()
            fixture.scope = 'composer-owner-b'
            page.evaluate('window.__AURORA.reload()')
            page.locator('#rows .row[data-id="bot-b"] .rowin').click()
            expect(page.locator('#input')).to_have_value('')
            pending.fulfill(status=200, content_type='application/json', body='{"path":"uploads/old-account.jpg"}')
            page.wait_for_timeout(100)
            expect(page.locator('#input')).to_have_value('')
            page.locator('#input').fill('Tài khoản B')
            page.reload()
            expect(page.locator('#input')).to_have_value('Tài khoản B')
            fixture.scope = 'composer-owner-a'
            page.reload()
            expect(page.locator('#input')).to_have_value('Nháp B\n[Ảnh: uploads/old-account.jpg]')
            check('workspace change isolates both drafts and late upload results')

            page.evaluate('''()=>{const original=Storage.prototype.setItem;Storage.prototype.setItem=function(k,v){if(k.startsWith('aurora.composer.'))throw Error('quota');return original.call(this,k,v)}}''')
            page.locator('#input').fill('Nháp còn khi bộ nhớ đầy')
            expect(page.locator('#toast')).to_contain_text('Chưa lưu được nháp')
            switch('a')
            switch('b')
            expect(page.locator('#input')).to_have_value('Nháp còn khi bộ nhớ đầy')
            check('storage failure retains unsaved draft in the current window')

            pending = upload()
            pending.fulfill(status=200, content_type='application/json', body='{"path":"uploads/diskfull.jpg"}')
            expect(page.locator('#toast')).to_contain_text('Nháp chưa được lưu')
            expect(page.locator('#input')).to_have_value('Nháp còn khi bộ nhớ đầy\n[Ảnh: uploads/diskfull.jpg]')
            check('upload success does not hide a draft storage failure')

            page.locator('#composer').evaluate('(form)=>form.requestSubmit()')
            expect(page.locator('.pending')).to_have_count(0)
            page.reload()
            expect(page.locator('#input')).to_have_value('')
            assert len(fixture.posts) == 1
            check('successful enqueue clears only the submitted chat draft')
            page.set_viewport_size({'width':390, 'height':844})
            switch('a')
            expect(page.locator('#input')).to_have_value(before)
            expect(page.locator('#refChip')).to_contain_text('Chat B')
            page.screenshot(path=str(OUT/'composer-mobile.png'))
            assert not errors, errors
            check('mobile viewport retains the other draft with no browser errors')
        finally:
            browser.close()
finally:
    server.shutdown()
    server.server_close()
