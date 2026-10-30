"""Quick provider onboarding against local fixtures, with no live keys or sign-in."""
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
OUT = Path(os.environ.get('AURORA_BROWSER_AUDIT_DIR', ROOT/'.audit/quick-setup-browser'))
OUT.mkdir(parents=True, exist_ok=True)

class Static(http.server.SimpleHTTPRequestHandler):
    def log_message(self, *args): pass

class Fixture:
    def __init__(self):
        self.selection = ''
        self.scope = 'quick-fixture'
        self.configured = set()
        self.posts = []
        self.failure = 0
        self.login = 'waiting'
        self.available = True
    def route(self, route):
        req = route.request
        path = urlsplit(req.url).path.removeprefix('/app/api')
        status, data = 200, {}
        if req.method != 'GET': self.posts.append((path, req.post_data_json))
        if path == '/state':
            data = {'workspaceId':'quick-fixture','agents':[{'id':'bot-a','name':'An','status':'idle','snip':'Sẵn sàng nhận việc.'}], 'pending':[], 'ai':{'configured':False}, 'secrets':[], 'outbound':{}}
        elif path == '/events':
            route.fulfill(content_type='text/event-stream',body='');return
        elif path.endswith('/messages'):
            data = [{'id':'failure','agentId':'bot-a','role':'system','text':'Tin nhắn chưa được xử lý.','recovery':'configure_ai','t':'2026-10-30T15:00:00Z'}]
        elif path == '/models':
            data = {'models':[],'providers':[],'vision':'glm:fixture','browser':False,'defaultConfigured':False}
        elif path == '/quick-setup':
            if req.method == 'POST':
                if self.failure: status = self.failure
                else:
                    body = req.post_data_json
                    assert body['workspaceId'] == 'quick-fixture' and body['botId'] == 'bot-a'
                    assert body['expectedSelection'] == self.selection
                    self.selection = 'core-'+body['provider']+':'+body['model']
                    self.configured.add(body['provider'])
            data = {'workspaceId':self.scope,'available':self.available,'selection':self.selection,'providers':[
                {'id':pid,'label':pid,'auth':'chatgpt_oauth' if pid=='chatgpt' else 'api_key','model':model,'models':[model], 'configured':pid in self.configured}
                for pid,model in [('zai','glm-fixture'),('zai-openai','glm-api-fixture'),('chatgpt','gpt-fixture')]
            ]}
        elif path == '/quick-setup/login': data = {'id':'fixture-attempt','state':'starting'}
        elif path == '/quick-setup/login/fixture-attempt/cancel': self.login='cancelled';data={'state':self.login}
        elif path == '/quick-setup/login/fixture-attempt':
            data={'id':'fixture-attempt','state':self.login}
            if self.login=='waiting': data['url']='https://auth.openai.com/oauth/authorize?state=fixture'
            if self.login=='connected': self.configured.add('chatgpt')
        route.fulfill(status=status,content_type='application/json',body=json.dumps(data))

server=FixtureHTTPServer(('127.0.0.1',0),functools.partial(Static,directory=str(ROOT/'app/native/internal/web/assets')))
threading.Thread(target=server.serve_forever,daemon=True).start()
fixture=Fixture()
with sync_playwright() as p:
    browser=p.chromium.launch(executable_path='C:/Program Files/Google/Chrome/Application/chrome.exe',headless=True)
    context=browser.new_context(viewport={'width':1440,'height':1000},service_workers='block',reduced_motion='reduce')
    context.add_init_script("if(!localStorage.getItem('neko-aurora-lang'))localStorage.setItem('neko-aurora-lang','vi');window.EventSource=class extends EventTarget{close(){}};")
    page=context.new_page();errors=[];page.on('pageerror',lambda e:errors.append(str(e)));page.route('**/app/api/**',fixture.route)
    page.route('https://**/*',lambda r:r.abort())
    def setup(): page.locator('.ai-setup-card').get_by_role('button',name='Thiết lập AI',exact=True).click()
    def close(): page.locator('#closeExt').click();page.wait_for_timeout(450)
    try:
        page.goto(f'http://127.0.0.1:{server.server_port}/app/#bot-a')
        page.locator('#input').fill('Bản nháp cần giữ.')
        setup();expect(page.locator('#quickProvider')).to_have_value('zai')
        expect(page.locator('#provForm')).not_to_be_visible();expect(page.locator('#quickKey')).to_have_attribute('type','password')
        page.screenshot(path=str(OUT/'quick-desktop.png'),full_page=True)
        page.locator('#quickKey').fill('fixture-ui-private')
        fixture.failure=503
        page.get_by_role('button',name='Lưu và dùng',exact=True).click()
        expect(page.locator('#quickSetup')).to_contain_text('Chưa lưu được')
        expect(page.locator('#quickKey')).to_have_value('fixture-ui-private')
        fixture.failure=0
        page.get_by_role('button',name='Lưu và dùng',exact=True).click()
        expect(page.locator('#quickSetup')).to_contain_text('Đã chọn AI.')
        expect(page.locator('#quickKey')).to_have_value('')
        assert fixture.selection=='core-zai:glm-fixture'
        page.get_by_role('button',name='Quay lại trò chuyện',exact=True).click();page.wait_for_timeout(450)
        expect(page.locator('#input')).to_have_value('Bản nháp cần giữ.')
        page.reload();setup();expect(page.locator('#quickKey')).to_have_attribute('placeholder','Đã lưu khóa · để trống để dùng tiếp')
        print('PASS GLM setup, save failure/retry, write-only key, reload and draft retention')

        page.locator('#quickProvider').select_option('zai-openai')
        expect(page.locator('#quickSetup')).to_contain_text('trả phí theo mức sử dụng')
        page.locator('#quickKey').fill('fixture-api-private');fixture.failure=409
        page.get_by_role('button',name='Lưu và dùng',exact=True).click()
        expect(page.locator('#quickSetup')).to_contain_text('Cấu hình đã thay đổi')
        assert fixture.selection=='core-zai:glm-fixture';fixture.failure=0
        print('PASS GLM plan/API distinction and stale selection refusal')

        page.locator('#quickProvider').select_option('chatgpt')
        expect(page.locator('#quickKey')).to_have_count(0)
        page.get_by_role('button',name='Đăng nhập ChatGPT',exact=True).click()
        expect(page.get_by_role('link',name='Mở trang đăng nhập')).to_have_attribute('href','https://auth.openai.com/oauth/authorize?state=fixture')
        expect(page.locator('#quickProvider')).to_be_disabled()
        page.get_by_role('button',name='Hủy đăng nhập',exact=True).click()
        expect(page.locator('#quickSetup')).to_contain_text('Đã hủy đăng nhập.')
        expect(page.locator('#quickProvider')).to_be_enabled()
        fixture.login='failed';page.get_by_role('button',name='Đăng nhập ChatGPT',exact=True).click()
        expect(page.locator('#quickSetup')).to_contain_text('Chưa đăng nhập được.')
        fixture.login='connected';page.get_by_role('button',name='Đăng nhập ChatGPT',exact=True).click()
        expect(page.locator('#quickSetup')).to_contain_text('Chọn dùng kết nối để hoàn tất.')
        assert fixture.selection=='core-zai:glm-fixture'
        page.get_by_role('button',name='Dùng kết nối này',exact=True).click()
        expect(page.locator('#quickSetup')).to_contain_text('Đã chọn AI.')
        assert fixture.selection=='core-chatgpt:gpt-fixture'
        assert fixture.posts[-1][1]['key']==''
        assert not any(path.endswith('/messages') for path,_ in fixture.posts)
        print('PASS OAuth link, cancel, failed retry, explicit activation and no API fallback/no auto-send')

        close();fixture.scope='another-workspace';setup()
        expect(page.locator('#quickSetup')).to_contain_text('Tài khoản đã thay đổi.')
        expect(page.locator('#quickProvider')).to_have_count(0)
        close();fixture.scope='quick-fixture';fixture.available=False;setup()
        expect(page.locator('#quickSetup')).to_contain_text('cần Neko Core')
        page.get_by_text('Cấu hình nâng cao',exact=True).click();expect(page.locator('#provForm')).to_be_visible()
        print('PASS workspace mismatch and unavailable Core with advanced fallback')

        close();fixture.available=True;fixture.selection='';fixture.configured.clear();page.set_viewport_size({'width':390,'height':844});setup()
        expect(page.locator('#quickKey')).to_be_in_viewport()
        assert page.evaluate('document.documentElement.scrollWidth<=innerWidth')
        page.screenshot(path=str(OUT/'quick-mobile.png'),full_page=True)
        page.locator('#quickProvider').select_option('chatgpt')
        page.screenshot(path=str(OUT/'quick-chatgpt-mobile.png'),full_page=True)
        close();page.evaluate("localStorage.setItem('neko-aurora-lang','en')");page.reload()
        page.locator('.ai-setup-card').get_by_role('button',name='Set up AI',exact=True).click()
        expect(page.get_by_role('heading',name='Choose your AI')).to_be_visible()
        page.locator('#quickProvider').select_option('chatgpt')
        expect(page.get_by_role('button',name='Sign in to ChatGPT',exact=True)).to_be_visible()
        assert not errors,errors
        print('PASS mobile layout, English and no browser errors')
    finally:
        browser.close();server.shutdown()
