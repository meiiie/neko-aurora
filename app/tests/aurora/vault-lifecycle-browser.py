"""Vault view ownership using fixture values, local HTTP and real Chrome DOM."""
import functools,http.server,json,os,threading,time
from pathlib import Path
from urllib.parse import urlsplit,parse_qs
from playwright.sync_api import expect,sync_playwright
ROOT=Path(__file__).resolve().parents[3]
OUT=ROOT/'.audit/2026-10-28-vault-lifecycle';OUT.mkdir(exist_ok=True)
class Server(http.server.ThreadingHTTPServer):request_queue_size=64
class Handler(http.server.SimpleHTTPRequestHandler):
    def log_message(self,*_):pass
class Fixture:
    def __init__(self):self.scope='vault-owner-a';self.label='Login A';self.hold=None;self.pending=[];self.calls=[]
    def route(self,r):
        req=r.request;url=urlsplit(req.url);path=url.path.removeprefix('/app/api');data={};kind=None
        if path=='/state':data={'workspaceId':self.scope,'agents':[{'id':'bot-a','name':'Chat A','status':'idle','t':'2026-10-28T04:00:00Z','av':{'shape':0,'color':4}}],'pending':[],'ai':{},'secrets':[],'outbound':{}}
        elif path.endswith('/messages'):data=[{'id':'vault-card','taskId':'vault-work','agentId':'bot-a','role':'agent','text':'Save fixture login','secret':{'vault':'login','label':'Card fixture'},'t':'2026-10-28T04:00:00Z'}]
        elif path.startswith('/vault'):
            body=req.post_data_json if req.method=='POST' else None
            self.calls.append({'method':req.method,'path':path,'scope':body.get('workspaceId') if body else parse_qs(url.query).get('workspaceId',[None])[0]})
            item={'id':'aabbccdd','type':'login','label':self.label,'hint':{'site':'fixture.test','user':'f***'},'filled':['password'],'updated':'2026-10-28T04:00:00Z'}
            if req.method=='GET':kind='list';data={'workspaceId':self.scope,'items':[item],'uses':[]}
            elif path.endswith('/reveal'):kind='reveal';data={'workspaceId':self.scope,'fields':{'password':'fixture-private-'+self.scope}}
            elif path.endswith('/delete'):kind='delete';data={'ok':True}
            else:kind='save';data=item
        if kind and self.hold==kind:self.pending.append((r,json.dumps(data)));return
        r.fulfill(status=200,content_type='application/json',body=json.dumps(data))
def check(label):print('PASS '+label,flush=True)
server=Server(('127.0.0.1',0),functools.partial(Handler,directory=str(ROOT/'app/native/internal/web/assets')))
threading.Thread(target=server.serve_forever,daemon=True).start()
try:
    with sync_playwright() as p:
        chrome=os.environ.get('AURORA_TEST_CHROME')
        if not chrome and Path('C:/Program Files/Google/Chrome/Application/chrome.exe').exists():chrome='C:/Program Files/Google/Chrome/Application/chrome.exe'
        browser=p.chromium.launch(headless=True,executable_path=chrome)
        try:
            context=browser.new_context(viewport={'width':1280,'height':900},locale='vi-VN',service_workers='block',reduced_motion='reduce')
            context.add_init_script('window.EventSource=class{addEventListener(){}close(){}}')
            page=context.new_page();page.clock.install();fx=Fixture();errors=[]
            page.on('pageerror',lambda e:errors.append(str(e)));page.route('**/app/api/**',fx.route)
            page.goto(f'http://127.0.0.1:{server.server_port}/app/#bot-a',wait_until='networkidle')
            body=page.locator('#vaultBody');sheet=page.locator('#vaultSheet')
            def open_vault():
                page.evaluate('()=>{void window.__AURORA.openVault()}');expect(body.locator('.vault-row')).to_contain_text(fx.label)
            def row():body.locator('.vault-row').click()
            def reveal():body.locator('[data-vreveal]').click()
            def pending():
                end=time.monotonic()+3
                while not fx.pending and time.monotonic()<end:page.wait_for_timeout(20)
                assert fx.pending,'fixture request not received'
                return fx.pending.pop(0)
            def release(held,status=200):held[0].fulfill(status=status,content_type='application/json',body=held[1] if status==200 else '{"error":"fixture_error"}')
            open_vault();row();reveal();expect(body.locator('code.shown')).to_contain_text('fixture-private-vault-owner-a')
            page.keyboard.press('Escape');expect(body).to_have_text('')
            open_vault();row();expect(body.locator('code.shown')).to_have_count(0)
            check('Escape clears revealed values from hidden DOM and reopening starts masked')
            fx.hold='reveal';reveal();held=pending();page.keyboard.press('Escape');fx.hold=None;open_vault();row();release(held)
            page.wait_for_timeout(80);expect(body.locator('code.shown')).to_have_count(0)
            check('late reveal cannot repopulate a closed and reopened Vault')
            fx.hold='reveal';reveal();held=pending();body.get_by_role('button',name='Sửa',exact=True).click()
            password=body.locator('input[data-secret="password"]');password.fill('fixture-new-password');release(held);fx.hold=None
            expect(password).to_have_value('fixture-new-password');expect(password).to_have_attribute('type','password')
            expect(body.locator('code.shown')).to_have_count(0)
            check('late reveal cannot replace a masked edit form or its unsaved input')
            page.wait_for_timeout(400);page.locator('#scrim').click(position={'x':5,'y':5});expect(body).to_have_text('')
            check('scrim close scrubs entered secret fields through the common close path')
            fx.hold='list';page.evaluate('()=>{void window.__AURORA.openVault()}');held=pending();page.keyboard.press('Escape')
            fx.hold=None;fx.label='Latest list';open_vault();release(held);page.wait_for_timeout(80)
            expect(body.locator('.vault-row')).to_contain_text('Latest list')
            check('older list responses cannot overwrite a later opening')
            row();fx.hold='reveal';reveal();held=pending();fx.scope='vault-owner-b';fx.label='Other workspace'
            page.evaluate('window.__AURORA.reload()');expect(body).to_have_text('');fx.hold=None;open_vault();row();release(held)
            page.wait_for_timeout(80);expect(body.locator('code.shown')).to_have_count(0);expect(body.locator('.vault-row')).to_contain_text('Other workspace')
            check('workspace switches discard old values even when the Vault item identifier is reused')
            body.get_by_role('button',name='Sửa',exact=True).click();body.locator('input[data-secret="password"]').fill('fixture-pending-save')
            fx.hold='save';body.locator('#vaultSave').click();held=pending();page.keyboard.press('Escape');fx.hold=None;open_vault();row()
            body.get_by_role('button',name='Sửa',exact=True).click();body.locator('#vaultLabel').fill('Keep this new edit');release(held)
            page.wait_for_timeout(80);expect(body.locator('#vaultLabel')).to_have_value('Keep this new edit')
            check('an old save result cannot close or replace a newer edit form')
            page.keyboard.press('Escape');open_vault();row();fx.hold='delete';page.once('dialog',lambda d:d.accept())
            body.get_by_role('button',name='Xoá',exact=True).click();held=pending();page.keyboard.press('Escape');fx.hold=None;open_vault();row()
            body.get_by_role('button',name='Sửa',exact=True).click();body.locator('#vaultLabel').fill('Keep edit after delete');release(held)
            page.wait_for_timeout(80);expect(body.locator('#vaultLabel')).to_have_value('Keep edit after delete')
            check('an old delete result cannot redraw a newer Vault view')
            page.keyboard.press('Escape');open_vault();row();fx.hold='reveal';reveal();held=pending();page.keyboard.press('Escape');fx.hold=None
            page.evaluate('document.querySelector("#toast").textContent="new-view-notice"');release(held,500)
            page.wait_for_timeout(80);expect(page.locator('#toast')).to_have_text('new-view-notice')
            check('errors from a closed reveal do not publish stale notices')
            open_vault();row();fx.hold='reveal';reveal();held=pending();expect(body.locator('[data-vreveal]')).to_be_disabled()
            body.locator('[data-vreveal]').evaluate('n=>n.click()');assert not fx.pending;release(held);fx.hold=None
            expect(body.locator('code.shown')).to_have_count(1);page.clock.fast_forward(31000)
            expect(body.locator('code.shown')).to_have_count(0)
            check('reveal clicks are single-flight and automatic hiding removes visible values')
            page.keyboard.press('Escape');page.wait_for_timeout(450);page.locator('#rows .row[data-id="bot-a"] .rowin').click()
            card=page.locator('#messages .vault-form');expect(card).to_be_visible();card.locator('input[data-secret="password"]').fill('fixture-chat-secret')
            page.locator('#backBtn').click();expect(card.locator('input[data-secret="password"]')).to_have_value('')
            check('leaving a chat clears secret-card inputs even while the old transcript remains mounted')
            page.wait_for_timeout(450);page.locator('#rows .row[data-id="bot-a"] .rowin').click()
            card.locator('input[data-secret="password"]').fill('fixture-reopened-secret');fx.hold='save';card.locator('#vaultSave').click();held=pending()
            page.locator('#backBtn').click();page.locator('#rows .row[data-id="bot-a"] .rowin').click()
            card.locator('input[data-secret="password"]').fill('fixture-new-chat-edit');release(held);fx.hold=None
            expect(card.locator('input[data-secret="password"]')).to_have_value('fixture-new-chat-edit')
            check('reopened chat secret cards remain usable and ignore success from an earlier chat visit')
            assert fx.calls and all(c['scope'] in ['vault-owner-a','vault-owner-b'] for c in fx.calls),'every UI Vault request must name its originating workspace'
            assert not errors,errors
            (OUT/'vault-browser-result.json').write_text(json.dumps({'result':'PASS','scenarios':12,'browser':browser.version,'errors':errors}),encoding='utf-8')
        finally:browser.close()
finally:server.shutdown();server.server_close()
