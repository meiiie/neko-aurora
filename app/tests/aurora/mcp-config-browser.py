"""MCP configuration fixtures: persistence acknowledgements and stale panel isolation."""
import copy
import functools
import http.server
import json
import os
import threading
from pathlib import Path
from urllib.parse import urlsplit
from playwright.sync_api import expect, sync_playwright

ROOT = Path(__file__).resolve().parents[3]
OUT = ROOT / '.audit/2026-10-28-mcp-config'
OUT.mkdir(parents=True, exist_ok=True)

class Handler(http.server.SimpleHTTPRequestHandler):
    def log_message(self, *_): pass

class Fixture:
    def __init__(self):
        self.scope, self.revision = 'mcp-owner-a', 0
        self.servers, self.posts, self.held = [], [], []
        self.mode = 'ok'

    def snapshot(self):
        return dict(workspaceId=self.scope, mcpRevision=self.revision, menu=[], allSkills=[], mcp=copy.deepcopy(self.servers))

    def finish(self, route, data, status=200):
        route.fulfill(status=status, content_type='application/json', body=json.dumps(data))

    def route(self, route):
        req = route.request
        path = urlsplit(req.url).path.removeprefix('/app/api')
        data = {}
        if path == '/events': route.fulfill(status=200, content_type='text/event-stream', body=''); return
        if path == '/state':
            data = dict(workspaceId=self.scope, agents=[dict(id='bot-a', name='Aurora', status='idle', av=dict(shape=0,color=4))], pending=[], ai={}, secrets=[], outbound={})
        elif path.endswith('/messages'): data=[]
        elif path.endswith('/computer'): data=dict(backend='files', available=True, files=[], log=[], usedBytes=0, quotaBytes=1000)
        elif path == '/models': data=dict(providers=[], models=[], vision='fixture', browser=False)
        elif path == '/skills': data=dict(skills=[])
        elif path.endswith('/ext'):
            if req.method == 'GET':
                data = self.snapshot()
                if self.mode == 'hold-get': self.held.append((route,data)); return
            else:
                body=req.post_data_json; self.posts.append((path,body))
                if self.mode=='error': self.finish(route,dict(error='settings_save_failed'),500); return
                if body.get('workspaceId')!=self.scope or ('mcp' in body and body.get('mcpRevision')!=self.revision):
                    self.finish(route,dict(error='mcp_configuration_changed'),409); return
                if 'mcp' in body: self.servers=body['mcp']; self.revision+=1
                data=dict(ok=True)
                if self.mode=='lost': route.abort('connectionreset'); return
                if self.mode=='hold-save': self.held.append((route,data)); return
        elif path.endswith('/mcp/reconnect'):
            self.posts.append((path,req.post_data_json)); data=dict(ok=True)
            if self.mode=='hold-reconnect': self.held.append((route,data)); return
        self.finish(route,data)

def check(text): print('PASS '+text,flush=True)

server=http.server.ThreadingHTTPServer(('127.0.0.1',0),functools.partial(Handler,directory=str(ROOT/'app/native/internal/web/assets')))
threading.Thread(target=server.serve_forever,daemon=True).start()
try:
    with sync_playwright() as p:
        chrome=os.environ.get('AURORA_TEST_CHROME')
        if not chrome and Path('C:/Program Files/Google/Chrome/Application/chrome.exe').exists(): chrome='C:/Program Files/Google/Chrome/Application/chrome.exe'
        browser=p.chromium.launch(headless=True,executable_path=chrome)
        context=browser.new_context(viewport=dict(width=390,height=844),locale='vi-VN',service_workers='block')
        page=context.new_page(); fx=Fixture(); errors=[]
        page.on('pageerror',lambda e:errors.append(str(e)))
        page.route('**/app/api/**',lambda r:fx.route(r))
        url=f'http://127.0.0.1:{server.server_port}/app/#bot-a'
        def settle(): page.evaluate('() => new Promise(ok=>requestAnimationFrame(()=>requestAnimationFrame(ok)))')
        def open_ext():
            if 'off' in (page.locator('#profile').get_attribute('class') or '').split(): page.locator('#whoBtn').click()
            page.locator('#prExt').click()
        def load():
            fx.mode='ok'
            page.goto('about:blank');page.goto(url,wait_until='networkidle')
            open_ext(); expect(page.locator('#mcpName')).to_be_visible()
        def add(name):
            page.locator('#mcpName').fill(name);page.locator('#mcpUrl').fill('https://fixture.example.com/'+name)
            page.get_by_role('button',name='Thêm máy chủ',exact=True).click()
        def release():
            held,fx.held=fx.held,[]
            for route,data in held: fx.finish(route,data)
            settle()
        def switch_scope(scope):
            fx.scope=scope
            with page.expect_response('**/app/api/state'):
                page.evaluate('() => document.dispatchEvent(new Event("visibilitychange"))')
            settle()
        def close_ext():
            page.locator("#closeExt").click()
            page.wait_for_timeout(450)  # Existing sheet gesture guard ignores input for 400 ms.
        def open_models():
            close_ext()
            page.locator('#profBack').click();page.locator('#backBtn').click();page.locator('#meBtn').click()
            page.locator('#openModels').click();expect(page.locator('#provForm')).to_be_visible()

        load();add('first');expect(page.locator('#mcpName')).to_have_value('')
        assert fx.posts[-1][1]==dict(workspaceId=fx.scope,mcpRevision=0,mcp=[dict(name='first',url='https://fixture.example.com/first')])
        check('add carries the workspace and configuration revision')

        fx.mode='hold-save';add('second');expect(page.get_by_role('button',name='Thêm máy chủ',exact=True)).to_be_disabled()
        count=len(fx.posts)
        page.locator('.mcp-add').evaluate('(n)=>n.dispatchEvent(new Event("submit",{cancelable:true,bubbles:true}))')
        settle();assert len(fx.posts)==count
        fx.mode='ok';release();expect(page.locator('#mcpName')).to_have_value('')
        assert fx.posts[-1][1]['mcpRevision']==1
        check('pending save prevents duplicate submits and reloads after acknowledgement')

        fx.mode='error';add('unsaved');expect(page.locator('#extBody > [role=status]')).to_contain_text('Nhập liệu vẫn được giữ')
        expect(page.locator('#mcpName')).to_have_value('unsaved');assert not any(m['name']=='unsaved' for m in fx.servers)
        expect(page.locator('#extBody > [role=status]')).to_be_in_viewport()
        expect(page.locator('#extBody > [role=status]')).to_be_focused()
        page.screenshot(path=str(OUT/'mcp-config-mobile-error.png'))
        fx.mode='ok';fx.revision+=1;page.get_by_role('button',name='Thêm máy chủ',exact=True).click()
        expect(page.locator('#extBody > [role=status]')).to_contain_text('Cấu hình đã thay đổi')
        expect(page.locator('#mcpName')).to_have_value('unsaved')
        page.get_by_role('button',name='Tải lại cấu hình',exact=True).click();expect(page.locator('#mcpName')).to_have_value('')
        check('failed save and stale revision retain input until explicit reload')

        fx.mode='lost';add('received');expect(page.locator('#extBody > [role=status]')).to_contain_text('Chưa xác nhận')
        count=len(fx.posts);fx.mode='ok';page.get_by_role('button',name='Tải lại cấu hình',exact=True).click()
        expect(page.locator('#extBody')).to_contain_text('received');expect(page.locator('#mcpName')).to_have_value('')
        assert len(fx.posts)==count and sum(m['name']=='received' for m in fx.servers)==1
        check('lost acknowledgement reconciles by GET without resubmitting')

        revision=fx.revision
        page.locator('#extBody .cell').filter(has_text='received').click()
        expect(page.locator('#extBody')).not_to_contain_text('received')
        assert fx.posts[-1][1]['mcpRevision']==revision and len(fx.servers)==2
        check('remove is scoped and advances the server revision')

        fx.servers[0]['state']='reconnect_required';load();fx.mode='hold-reconnect'
        page.get_by_role('button',name='Kết nối lại · first',exact=True).click()
        assert fx.posts[-1][1]==dict(name='first',workspaceId=fx.scope,mcpRevision=fx.revision)
        open_models();release();expect(page.locator('#provForm')).to_be_visible()
        expect(page.locator('#mcpName')).to_have_count(0)
        check('reconnect carries scope and its late reply cannot replace the models panel')

        load();fx.mode='hold-save';add('delayed');expect(page.locator('#extBody > [role=status]')).to_contain_text('Đang lưu')
        open_models();fx.mode='ok';release();expect(page.locator('#provForm')).to_be_visible()
        expect(page.locator('#mcpName')).to_have_count(0)
        check('late successful save cannot reopen a panel that the user left')

        load();close_ext();fx.mode='hold-get';open_ext()
        expect(page.locator('#extBody')).to_contain_text('Đang tải')
        open_models();fx.mode='ok';release();expect(page.locator('#provForm')).to_be_visible()
        expect(page.locator('#mcpName')).to_have_count(0)
        check('late configuration read cannot overwrite a different settings view')

        load();close_ext();fx.mode='hold-get';open_ext()
        expect(page.locator('#extBody')).to_contain_text('Đang tải')
        switch_scope('mcp-owner-b');switch_scope('mcp-owner-a')
        fx.mode='ok';release();expect(page.locator('#extSheet')).not_to_be_visible()
        expect(page.locator('#mcpName')).to_have_count(0)
        check('workspace A to B to A invalidates the original pending read')

        load();page.locator('#mcpName').scroll_into_view_if_needed()
        assert page.evaluate('document.documentElement.scrollWidth<=innerWidth')
        page.screenshot(path=str(OUT/'mcp-config-mobile.png'))
        page.set_viewport_size(dict(width=1280,height=900));settle()
        assert page.evaluate('document.documentElement.scrollWidth<=innerWidth')
        page.screenshot(path=str(OUT/'mcp-config-desktop.png'))
        assert not errors,errors
        check('desktop and mobile widths with no unhandled script errors')
        context.close();browser.close()
finally:
    server.shutdown();server.server_close()
