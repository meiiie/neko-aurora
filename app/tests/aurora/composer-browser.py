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
from fixture_http import FixtureHTTPServer
from urllib.parse import urlsplit

from playwright.sync_api import expect, sync_playwright

ROOT = Path(__file__).resolve().parents[3]
OUT = Path(os.environ.get('AURORA_BROWSER_AUDIT_DIR', ROOT / '.audit/2026-10-28-composer'))
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
        self.receipts = {}
        self.offline = False
        self.lose_response = False

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
                if self.offline:
                    route.abort('internetdisconnected')
                    return
                if body['clientNonce'] not in self.receipts:
                    data = {'id': body['clientNonce']+':u', 'taskId': body['clientNonce'], 'agentId': path.split('/')[2], 'role': 'user', 'text': body['text'], 'answerFor':body.get('answerFor'), 't': '2026-10-28T04:01:00Z'}
                    if body.get('replyTo'):
                        source=next(m for m in self.messages if m['id']==body['replyTo'])
                        data['quote']={'id':source['id'],'taskId':source['taskId'],'author':'Chat A','role':source['role'],'text':source['text']}
                    self.messages.append(data)
                    self.receipts[body['clientNonce']] = data
                data = self.receipts[body['clientNonce']]
                if self.lose_response:
                    self.lose_response = False
                    route.abort('connectionreset')
                    return
        elif path.endswith('/computer/upload'):
            self.uploads.append(route)
            return
        elif path.endswith('/ext'):
            data = {'workspaceId': self.scope, 'mcpRevision': 0, 'menu': [], 'allSkills': [], 'mcp': []}
        route.fulfill(status=202 if req.method == 'POST' and path.endswith('/messages') else 200, content_type='application/json', body=json.dumps(data))


def check(label):
    print('PASS ' + label)


server = FixtureHTTPServer(('127.0.0.1', 0), functools.partial(Handler, directory=str(ROOT/'app/native/internal/web/assets')))
threading.Thread(target=server.serve_forever, daemon=True).start()
url = f'http://127.0.0.1:{server.server_port}'
try:
    with sync_playwright() as p:
        chrome = os.environ.get('AURORA_TEST_CHROME')
        if not chrome and Path('C:/Program Files/Google/Chrome/Application/chrome.exe').exists():
            chrome = 'C:/Program Files/Google/Chrome/Application/chrome.exe'
        browser = p.chromium.launch(headless=True, executable_path=chrome)
        try:
            context = browser.new_context(viewport={'width':1280, 'height':900}, locale='vi-VN', service_workers='block')
            context.add_init_script('window.SpeechRecognition=class{constructor(){window.fixtureRec=this}start(){}stop(){}}')
            page = context.new_page()
            fixture, errors = Fixture(), []
            requests=[]
            page.on('requestfailed',lambda q:requests.append({'url':q.url,'failure':q.failure}))
            page.on('response',lambda q:requests.append({'url':q.url,'status':q.status}))
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

            fixture.messages.append({'id':'question-fixture', 'taskId':'req-questions-fixture', 'agentId':'bot-a', 'role':'agent', 'text':'Bạn chọn giúp mình nhé.', 't':'2026-10-28T04:02:00Z', 'questions':[
                {'id':'q1','prompt':'Thời điểm?','options':['Sáng','Chiều']},
                {'id':'q2','prompt':'Định dạng?','options':['Bảng','Biểu đồ'],'multiple':True},
                {'id':'q3','prompt':'Nơi nhận?','options':['Tại đây','Email']},
            ]})
            page.reload()
            card=page.locator('.question-card')
            with page.expect_navigation(wait_until='load'):
                page.evaluate('window.__I18N.set("en")')
            expect(card.get_by_role('button',name='Next',exact=True)).to_be_disabled()
            expect(card.get_by_role('radio',name='Sáng',exact=True)).to_have_count(1)
            expect(card.get_by_role('radio',name='Light',exact=True)).to_have_count(0)
            with page.expect_navigation(wait_until='load'):
                page.evaluate('window.__I18N.set("vi")')
            check('UI language changes preserve model question wording')
            expect(card.get_by_role('button',name='Tiếp',exact=True)).to_be_disabled()
            page.locator('#input').fill('Tôi muốn hỏi thêm trước')
            page.locator('#composer').evaluate('(form)=>form.requestSubmit()')
            expect(page.locator('.pending')).to_have_count(0)
            expect(card).to_contain_text('Câu 1/3')
            check('ordinary follow-up leaves structured questions unanswered')
            card.get_by_role('radio',name='Sáng',exact=True).check()
            card.get_by_role('button',name='Tiếp',exact=True).click()
            card.get_by_role('checkbox',name='Bảng',exact=True).check()
            card.get_by_role('checkbox',name='Biểu đồ',exact=True).check()
            card.get_by_role('button',name='Quay lại',exact=True).click()
            expect(card.get_by_role('radio',name='Sáng',exact=True)).to_be_checked()
            card.get_by_role('button',name='Tiếp',exact=True).click()
            expect(card.get_by_role('checkbox',name='Bảng',exact=True)).to_be_checked()
            expect(card.get_by_role('checkbox',name='Biểu đồ',exact=True)).to_be_checked()
            check('step navigation preserves single and multiple selections')
            card.get_by_role('button',name='Tiếp',exact=True).click()
            card.get_by_role('button',name='Câu trả lời riêng',exact=True).click()
            card.get_by_role('textbox',name='Câu trả lời riêng',exact=True).fill('Trong ứng dụng, sau 18 giờ')
            page.reload()
            expect(card).to_contain_text('Câu 3/3')
            expect(card.get_by_role('textbox',name='Câu trả lời riêng',exact=True)).to_have_value('Trong ứng dụng, sau 18 giờ')
            check('partial structured answer survives full reload')
            page.wait_for_function('''()=>{const r=document.querySelector('#thread').getBoundingClientRect();return r.left>=0&&r.right<=innerWidth+0.5}''')
            assert card.evaluate('(el)=>el.scrollWidth<=el.clientWidth+1')
            assert page.evaluate('document.documentElement.scrollWidth<=innerWidth')
            page.screenshot(path=str(OUT/'questions-edit-mobile.png'),animations='disabled')
            check('editable question card fits a 390px viewport')
            card.get_by_role('button',name='Xem lại',exact=True).click()
            expect(card).to_contain_text('Bảng; Biểu đồ')
            expect(card).to_contain_text('Trong ứng dụng, sau 18 giờ')
            fixture.offline=True
            card.get_by_role('button',name='Gửi câu trả lời',exact=True).click()
            expect(card).to_contain_text('Đang chờ xác nhận')
            assert 'Đã trả lời' not in card.inner_text()
            page.reload()
            expect(card).to_contain_text('Đang chờ xác nhận')
            expect(card).to_contain_text('Bảng; Biểu đồ')
            check('offline submission remains pending with all answers after reload')
            fixture.offline=False
            fixture.lose_response=True
            expect(page.locator('.pending')).to_have_count(0,timeout=15000)
            expect(card).to_contain_text('Đã trả lời')
            accepted=[x for x in fixture.receipts.values() if x.get('answerFor')=='req-questions-fixture']
            assert len(accepted)==1
            sent=[x for x in fixture.posts if x.get('answerFor')=='req-questions-fixture']
            assert len({x['clientNonce'] for x in sent})==1
            assert sent[-1]['questionAnswers'][1]['selected']==[0,1]
            assert sent[-1]['questionAnswers'][2]['text']=='Trong ứng dụng, sau 18 giờ'
            check('lost response reuses the question receipt and complete structured payload')
            page.screenshot(path=str(OUT/'questions-mobile.png'))
            assert not errors,errors
            check('question card has no uncaught browser errors')
            quote_text='Q4 <img src=x onerror=alert(1)> stays text\n'+'Chi tiết phiên bản. '*20
            fixture.messages.append({'id':'quote-source','taskId':'quote-source','agentId':'bot-a','role':'agent','text':quote_text,'t':'2026-10-28T04:02:00Z'})
            page.reload()
            page.locator('.reply-action[data-id="quote-source"]').click()
            expect(page.locator('#replyChip')).to_contain_text(quote_text)
            expect(page.locator('#replyChip img')).to_have_count(0)
            page.locator('#input').fill('Phân tích mốc này')
            switch('b')
            expect(page.locator('#replyChip')).to_have_count(0)
            switch('a')
            expect(page.locator('#replyChip')).to_contain_text(quote_text)
            expect(page.locator('#input')).to_have_value('Phân tích mốc này')
            page.reload()
            expect(page.locator('#replyChip')).to_contain_text(quote_text)
            check('quoted reply draft is scoped to its chat and survives reload')
            original_scope=fixture.scope
            fixture.scope='quote-other-owner'
            page.evaluate('window.__AURORA.reload()')
            expect(page.locator('#replyChip')).to_have_count(0)
            fixture.scope=original_scope
            page.evaluate('window.__AURORA.reload()')
            page.locator('#rows .row[data-id="bot-a"] .rowin').click()
            expect(page.locator('#replyChip')).to_contain_text(quote_text)
            check('workspace changes isolate and restore quote drafts')
            fixture.offline=True
            page.locator('#composer').evaluate('(form)=>form.requestSubmit()')
            expect(page.locator('#replyChip')).to_have_count(0)
            expect(page.locator('.pending .quoted-message')).to_contain_text(quote_text)
            page.reload()
            expect(page.locator('.pending .quoted-message')).to_contain_text(quote_text)
            check('offline outbox retains the selected quote after reload')
            fixture.offline=False
            fixture.lose_response=True
            expect(page.locator('.pending')).to_have_count(0,timeout=15000)
            sent=[x for x in fixture.posts if x.get('replyTo')=='quote-source']
            assert len({x['clientNonce'] for x in sent})==1
            accepted=[x for x in fixture.receipts.values() if x.get('quote',{}).get('id')=='quote-source']
            assert len(accepted)==1
            fixture.messages[-1:-1]=[{'id':'quote-window-'+str(i),'taskId':'quote-window-'+str(i),'agentId':'bot-a','role':'agent','text':'Window fixture '+str(i),'t':'2026-10-28T04:02:01Z'} for i in range(65)]
            page.reload()
            expect(page.locator('.message-item[data-message-id="quote-source"]')).to_have_count(0)
            expect(page.locator('.message-item.mine>.quoted-message')).to_contain_text(quote_text)
            expect(page.locator('.quoted-message img')).to_have_count(0)
            expand=page.locator('.message-item.mine>.quoted-message .quote-expand')
            expand.click()
            expect(expand).to_have_attribute('aria-expanded','true')
            expand.click()
            page.wait_for_function('''()=>{const r=document.querySelector('#thread').getBoundingClientRect();return r.left>=0&&r.right<=innerWidth+0.5}''')
            page.screenshot(path=str(OUT/'quoted-reply-mobile.png'),animations='disabled')
            check('lost quote receipt is replayed once and history renders source as text')
            page.locator('.message-item.mine>.quoted-message .quote-source').click()
            expect(page.locator('.message-item[data-message-id="quote-source"]')).to_be_focused()
            check('quote navigation reveals its source beyond the current rendered window')
            page.locator('.reply-action[data-id="quote-source"]').click()
            page.locator('#input').fill('Giữ nội dung này')
            page.get_by_role('button',name='Bỏ trích dẫn',exact=True).click()
            page.reload()
            expect(page.locator('#replyChip')).to_have_count(0)
            expect(page.locator('#input')).to_have_value('Giữ nội dung này')
            assert not errors,errors
            check('removing a quote preserves typed text and remains removed after reload')
        except Exception:
            print('BROWSER ERRORS',json.dumps({'errors':errors,'requests':requests[-60:]}))
            print('BROWSER DIAGNOSTICS', json.dumps(page.evaluate('''()=>({ready:document.readyState,aurora:typeof window.__AURORA,workspace:window.__AURORA?.state().workspaceId,url:location.href,body:document.body?.innerText.slice(-800),screens:[...document.querySelectorAll('.screen')].map(e=>({id:e.id,cls:e.className,transform:getComputedStyle(e).transform,bounds:e.getBoundingClientRect().toJSON()})),scripts:[...document.scripts].map(e=>e.src)})''')))
            page.screenshot(path=str(OUT/'composer-failure.png'))
            raise
        finally:
            browser.close()
finally:
    server.shutdown()
    server.server_close()
