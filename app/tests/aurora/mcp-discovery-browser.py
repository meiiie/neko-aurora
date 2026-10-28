"""MCP authorization preview browser fixtures; no provider grants or token calls."""
import functools
import http.server
import json
import os
import threading
from pathlib import Path
from urllib.parse import urlsplit
from playwright.sync_api import expect, sync_playwright

ROOT = Path(__file__).resolve().parents[3]
OUT = ROOT / '.audit/2026-10-28-mcp-oauth'
OUT.mkdir(parents=True, exist_ok=True)
RESOURCE = 'https://mcp.example.com/team/mcp'
ISSUER = 'https://auth.example.com/tenant'

class Handler(http.server.SimpleHTTPRequestHandler):
    def log_message(self, *_): pass

class Fixture:
    def __init__(self):
        self.scope, self.mode = 'mcp-owner-a', 'ok'
        self.posts, self.held = [], []
        self.pkce = True

    def result(self, body):
        result = dict(resource=RESOURCE, metadataUrl=RESOURCE+'/metadata', issuers=[ISSUER], scopes=['files:read', 'notes:write'])
        if self.mode == 'multiple' and not body.get('issuer'):
            result['issuers'].append('https://second.example.com')
        else: result['authorization'] = dict(issuer=body.get('issuer') or ISSUER, pkceS256=self.pkce, clientIdMetadataDocument=True)
        return result

    def route(self, route):
        path = urlsplit(route.request.url).path.removeprefix('/app/api')
        data, status = {}, 200
        if path == '/events': route.fulfill(status=200, content_type='text/event-stream', body=''); return
        if path == '/state':
            data = dict(workspaceId=self.scope, agents=[dict(id='bot-a', name='Aurora', status='idle', av=dict(shape=0, color=4))], pending=[], ai={}, secrets=[], outbound={})
        elif path.endswith('/messages'): data=[]
        elif path.endswith('/computer'): data=dict(backend='files',available=True,files=[],log=[],usedBytes=0,quotaBytes=1000)
        elif path.endswith('/ext'):
            data=dict(workspaceId=self.scope,mcpRevision=0,menu=[],allSkills=[],mcp=[dict(name='Fixture MCP',url=RESOURCE,state='reconnect_required',error='Cần cấp quyền',tools=[])])
        elif path.endswith('/authorization/preview'):
            body=route.request.post_data_json;self.posts.append(body)
            if self.mode=='hold': self.held.append((route,body));return
            if self.mode=='error':status,data=422,dict(error='mcp_authorization_metadata')
            else:data=self.result(body)
        route.fulfill(status=status,content_type='application/json',body=json.dumps(data))

def check(text): print('PASS '+text,flush=True)

server=http.server.ThreadingHTTPServer(('127.0.0.1',0),functools.partial(Handler,directory=str(ROOT/'app/native/internal/web/assets')))
threading.Thread(target=server.serve_forever,daemon=True).start()
try:
    with sync_playwright() as p:
        chrome=os.environ.get('AURORA_TEST_CHROME')
        if not chrome and Path('C:/Program Files/Google/Chrome/Application/chrome.exe').exists():chrome='C:/Program Files/Google/Chrome/Application/chrome.exe'
        browser=p.chromium.launch(headless=True,executable_path=chrome)
        context=browser.new_context(viewport=dict(width=390,height=844),locale='vi-VN',service_workers='block')
        page=context.new_page();fx=Fixture();errors=[]
        page.on('pageerror',lambda e:errors.append(str(e)))
        page.route('**/app/api/**',lambda r:fx.route(r))
        url=f'http://127.0.0.1:{server.server_port}/app/#bot-a'
        def load():
            if page.url.startswith(url.split('#')[0]):page.reload(wait_until='networkidle')
            else:page.goto(url,wait_until='networkidle')
            page.locator('#whoBtn').click();page.locator('#prExt').click()
            expect(page.locator('.mcp-authorization')).to_be_visible()
        def preview():page.get_by_role('button',name='Xem thông tin cấp quyền · Fixture MCP').click()
        def status():return page.locator('.mcp-authorization [role=status]')
        def release():
            for route,body in fx.held:route.fulfill(status=200,content_type='application/json',body=json.dumps(fx.result(body)))
            fx.held.clear()

        load();assert not fx.posts
        preview();expect(status()).to_contain_text('Tài khoản chưa được cấp quyền')
        expect(page.locator('.mcp-authorization dl')).to_contain_text(ISSUER)
        expect(page.locator('.mcp-authorization dl')).to_contain_text('files:read, notes:write')
        assert fx.posts==[dict(workspaceId=fx.scope,name='Fixture MCP',url=RESOURCE,issuer='')]
        check('preview is explicit, scoped and shows issuer/scopes without granting access')

        fx.mode='multiple';preview()
        choice=page.get_by_label('Chọn máy chủ cấp quyền',exact=True)
        expect(choice).to_be_visible();count=len(fx.posts)
        choice.select_option('https://second.example.com');assert len(fx.posts)==count
        page.get_by_role('button',name='Xem máy chủ đã chọn').focus();page.keyboard.press('Enter')
        expect(page.locator('.mcp-authorization dl')).to_contain_text('https://second.example.com')
        assert fx.posts[-1]['issuer']=='https://second.example.com'
        check('multiple issuers need explicit choice with keyboard support')

        fx.mode='ok';fx.pkce=False;preview()
        expect(page.locator('.mcp-authorization')).to_contain_text('chưa công bố phương thức đăng nhập được hỗ trợ')
        check('missing PKCE capability does not claim supported login')

        fx.mode='error';preview();expect(status()).to_contain_text('Chưa xác minh')
        expect(page.locator('.mcp-authorization dl')).to_have_count(0)
        fx.mode='ok';preview();expect(status()).to_contain_text('Đã đọc')
        check('failed discovery clears stale metadata and supports explicit retry')

        fx.mode='hold';preview();expect(status()).to_contain_text('Đang kiểm tra')
        expect(page.get_by_role('button',name='Xem thông tin cấp quyền · Fixture MCP')).to_be_disabled()
        page.locator('#closeExt').click();release()
        expect(status()).to_contain_text('Đang kiểm tra')
        check('closed panel ignores late discovery response')

        fx.mode='ok';load();fx.mode='hold';preview();expect(status()).to_contain_text('Đang kiểm tra')
        fx.scope='mcp-owner-b'
        page.evaluate('() => document.dispatchEvent(new Event("visibilitychange"))')
        expect(page.locator('#extSheet')).not_to_be_visible();release()
        expect(status()).to_contain_text('Đang kiểm tra')
        fx.mode='ok';load();preview();expect(status()).to_contain_text('Đã đọc')
        assert fx.posts[-1]['workspaceId']=='mcp-owner-b'
        check('workspace change discards late result and binds next preview to new workspace')

        page.locator('.mcp-authorization').scroll_into_view_if_needed()
        assert page.evaluate('document.documentElement.scrollWidth<=innerWidth')
        page.screenshot(path=str(OUT/'mcp-authorization-mobile.png'))
        assert not errors,errors
        check('mobile width and no unhandled page errors')
        context.close();browser.close()
finally:
    server.shutdown();server.server_close()
