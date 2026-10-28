"""Saved credential editor removal lifecycle with synthetic owner API fixtures."""
import functools,http.server,json,os,threading,time
from pathlib import Path
from urllib.parse import urlsplit
from playwright.sync_api import expect,sync_playwright
ROOT=Path(__file__).resolve().parents[3];OUT=ROOT/'.audit/2026-10-29-vault-editor';OUT.mkdir(exist_ok=True)
class Server(http.server.ThreadingHTTPServer):request_queue_size=64
class Handler(http.server.SimpleHTTPRequestHandler):
    def log_message(self,*_):pass
class Fixture:
    def __init__(self):
        self.scope='editor-owner-a';self.status=200;self.hold='';self.pending=[];self.posts=[]
        self.items=[dict(id='aabbccdd',type='login',label='Fixture login',hint={'site':'example.test'},filled=['password'],useHint='Travel only'),dict(id='11223344',type='card',label='Fixture card',hint={'brand':'Visa','last4':'1111'},filled=['number'])]
    def route(self,r):
        path=urlsplit(r.request.url).path.removeprefix('/app/api');data={};status=200;kind=''
        if path=='/state':data=dict(workspaceId=self.scope,agents=[],pending=[],ai={},secrets=[],outbound={})
        elif path=='/vault':
            if r.request.method=='GET':data=dict(workspaceId=self.scope,items=self.items,uses=[])
            else:kind='save';status=self.status;data=self.items[0]
        elif path.endswith('/reveal'):raise AssertionError('editing a saved password must not fetch it')
        elif path.endswith('/delete'):
            kind='delete';status=self.status
            if status==200:self.items=[it for it in self.items if it['id']!=path.split('/')[2]]
        if kind:
            self.posts.append(dict(kind=kind,path=path,body=r.request.post_data_json))
            if self.hold==kind:self.pending.append((r,status,data));return
        r.fulfill(status=status,content_type='application/json',body=json.dumps(data))

checks=[];errors=[]
def check(s):checks.append(s);print('PASS '+s,flush=True)
server=Server(('127.0.0.1',0),functools.partial(Handler,directory=str(ROOT/'app/native/internal/web/assets')));threading.Thread(target=server.serve_forever,daemon=True).start()
try:
 with sync_playwright() as p:
    browser=p.chromium.launch(headless=True,executable_path=os.environ.get('AURORA_TEST_CHROME') or 'C:/Program Files/Google/Chrome/Application/chrome.exe')
    def setup():
        ctx=browser.new_context(viewport={'width':390,'height':844},locale='vi-VN',service_workers='block',reduced_motion='reduce');ctx.add_init_script('window.EventSource=class{addEventListener(){}close(){}}');page=ctx.new_page();fx=Fixture();page.on('pageerror',lambda e:errors.append(str(e)));page.route('**/app/api/**',fx.route);page.goto(f'http://127.0.0.1:{server.server_port}/app/',wait_until='networkidle');open_vault(page);return ctx,page,fx
    def open_vault(page):page.locator('#meBtn').click();page.locator('#openVault').click();expect(page.locator('#vaultBody .vault-row')).not_to_have_count(0)
    def edit(page,id='aabbccdd'):
        body=page.locator('#vaultBody');body.locator('.vault-it[data-id="'+id+'"] .vault-row').click();body.get_by_role('button',name='Sửa',exact=True).click();expect(body.locator('form')).to_be_visible()
    def close(page):page.keyboard.press('Escape');page.wait_for_timeout(450)
    def held(page,fx):
        end=time.monotonic()+3
        while not fx.pending and time.monotonic()<end:page.wait_for_timeout(20)
        assert fx.pending;return fx.pending.pop()
    try:
        ctx,page,fx=setup();edit(page);body=page.locator('#vaultBody');password=body.locator('input[data-secret="password"]');remove=body.locator('[data-vdelete]')
        expect(password).to_have_value('');expect(password).to_have_attribute('type','password');expect(password).to_have_attribute('placeholder','•••••••• · để trống nếu giữ nguyên');expect(remove).to_have_count(1);expect(remove).to_have_attribute('type','button');assert not fx.posts
        check('saved password is represented by a mask and Remove without revealing stored values')
        for mode in ['light','dark']:
            page.emulate_media(color_scheme=mode);page.wait_for_function('(dark)=>(document.documentElement.dataset.theme==="dark")===dark',arg=mode=='dark')
            visual=remove.evaluate(r'''b=>{const s=getComputedStyle(b),rgb=c=>c.match(/[\d.]+/g).slice(0,3).map(Number),lum=c=>rgb(c).map(v=>v/255).map(v=>v<=.04045?v/12.92:((v+.055)/1.055)**2.4).reduce((n,v,i)=>n+v*[.2126,.7152,.0722][i],0),a=lum(s.color),z=lum(s.backgroundColor),icon=b.querySelector("svg").getBoundingClientRect(),label=b.querySelector("span").getBoundingClientRect();return{contrast:(Math.max(a,z)+.05)/(Math.min(a,z)+.05),offset:Math.abs(icon.y+icon.height/2-label.y-label.height/2)}}''')
            assert visual['contrast']>=4.5 and visual['offset']<2,'removal control contrast/alignment failed'
        page.emulate_media(color_scheme='light');page.wait_for_function('document.documentElement.dataset.theme!=="dark"')
        check('removal control has readable contrast and aligned content in light and dark themes')
        password.fill('fixture-unsaved');body.locator('textarea[name=useHint]').fill('Changed draft');page.once('dialog',lambda d:d.dismiss());remove.click();assert not fx.posts;expect(password).to_have_value('fixture-unsaved');expect(body.locator('#vaultSave')).to_be_enabled()
        check('cancelled removal sends nothing and keeps the draft')
        fx.status=500;page.once('dialog',lambda d:d.accept());remove.click();expect(remove).to_be_enabled();expect(body.locator('#vaultSave')).to_be_enabled();expect(password).to_have_value('fixture-unsaved');expect(body.locator('textarea[name=useHint]')).to_have_value('Changed draft');assert len(fx.posts)==1
        check('failed deletion preserves edits and restores both actions')
        fx.status=200;fx.hold='delete';page.evaluate('window.oldEditor=document.querySelector("#vaultBody form")');page.once('dialog',lambda d:d.accept());remove.click();r,status,data=held(page,fx);expect(remove).to_be_disabled();expect(body.locator('#vaultSave')).to_be_disabled();page.evaluate('document.querySelector("#vaultBody form").requestSubmit()');password.press('Enter');assert len(fx.posts)==2
        r.fulfill(status=status,content_type='application/json',body=json.dumps(data));expect(body.locator('form')).to_have_count(0);expect(body.locator('.vault-it[data-id="aabbccdd"]')).to_have_count(0);assert page.evaluate('Array.from(oldEditor.querySelectorAll("input,textarea")).every(i=>i.value==="")');assert fx.posts[-1]['body']=={'workspaceId':'editor-owner-a'}
        check('pending deletion blocks saves and success scrubs the detached editor')
        body.locator('[data-vadd="login"]').click();expect(body.locator('form')).to_be_visible();expect(body.locator('[data-vdelete]')).to_have_count(0)
        check('new item forms never offer deletion');ctx.close()
        ctx,page,fx=setup();edit(page);body=page.locator('#vaultBody');fx.hold='save';body.locator('input[data-secret="password"]').fill('fixture-save');body.locator('#vaultSave').click();r,status,data=held(page,fx);expect(body.locator('[data-vdelete]')).to_be_disabled();body.locator('[data-vdelete]').evaluate('(b)=>b.onclick()');assert len(fx.posts)==1 and fx.posts[0]['kind']=='save';r.fulfill(status=status,content_type='application/json',body=json.dumps(data));expect(body.locator('form')).to_have_count(0)
        check('pending save blocks removal before any confirmation or second request');ctx.close()
        for status in [200,500]:
            ctx,page,fx=setup();edit(page);body=page.locator('#vaultBody');fx.hold='delete';fx.status=status;page.once('dialog',lambda d:d.accept());body.locator('[data-vdelete]').click();r,code,data=held(page,fx);close(page);fx.hold='';fx.status=200;open_vault(page);edit(page,'11223344');body.locator('#vaultLabel').fill('Current card draft');r.fulfill(status=code,content_type='application/json',body=json.dumps(data));page.wait_for_timeout(100);expect(body.locator('#vaultLabel')).to_have_value('Current card draft');expect(body.locator('#vaultSave')).to_be_enabled()
            check(f'late delete HTTP{status} leaves a reopened editor intact');ctx.close()
        ctx,page,fx=setup();edit(page);body=page.locator('#vaultBody');fx.hold='delete';page.once('dialog',lambda d:d.accept());body.locator('[data-vdelete]').click();r,status,data=held(page,fx);fx.scope='editor-owner-b';page.evaluate('window.__AURORA.reload()');expect(body).to_have_text('');page.wait_for_timeout(450);fx.hold='';open_vault(page);edit(page,'11223344');body.locator('#vaultLabel').fill('Workspace B');r.fulfill(status=status,content_type='application/json',body=json.dumps(data));page.wait_for_timeout(100);expect(body.locator('#vaultLabel')).to_have_value('Workspace B');assert fx.posts[0]['body']['workspaceId']=='editor-owner-a'
        check('workspace change rejects old deletion acknowledgement');ctx.close()
        ctx,page,fx=setup();body=page.locator('#vaultBody');body.locator('.vault-it[data-id="aabbccdd"] .vault-row').click();page.once('dialog',lambda d:d.accept());body.locator('[data-vdelete]').click();expect(body.locator('.vault-it[data-id="aabbccdd"]')).to_have_count(0);assert len(fx.posts)==1 and fx.posts[0]['kind']=='delete'
        check('detail removal uses the same scoped action');ctx.close()
        ctx,page,fx=setup();close(page)
        with page.expect_navigation(wait_until='networkidle'):page.evaluate('window.__I18N.set("en")')
        open_vault(page);body=page.locator('#vaultBody');body.locator('.vault-it[data-id="aabbccdd"] .vault-row').click();body.get_by_role('button',name='Edit',exact=True).click();expect(body.locator('input[data-secret="password"]')).to_have_attribute('placeholder','•••••••• · leave blank to keep');expect(body.get_by_role('button',name='Remove',exact=True)).to_be_visible();body.locator('[data-vdelete]').scroll_into_view_if_needed();page.screenshot(path=str(OUT/'editor-remove-mobile-en.png'));page.set_viewport_size({'width':1280,'height':900});page.screenshot(path=str(OUT/'editor-remove-desktop-en.png'));body.locator('[data-vdelete]').focus();page.once('dialog',lambda d:d.accept());page.keyboard.press('Enter');expect(body.locator('form')).to_have_count(0)
        check('English masked editing and keyboard removal are available');ctx.close()
        assert not errors,errors
        (OUT/'editor-browser-result.json').write_bytes(json.dumps(dict(result='PASS',scenarios=len(checks),browser=browser.version,errors=errors)).encode())
    finally:browser.close()
finally:server.shutdown();server.server_close()
