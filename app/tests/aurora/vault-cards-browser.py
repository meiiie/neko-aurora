"""Device-local Vault card markers and latest-state authentication expiry."""
import functools,http.server,json,os,re,threading,time
from pathlib import Path
from urllib.parse import urlsplit
from playwright.sync_api import expect,sync_playwright
ROOT=Path(__file__).resolve().parents[3]
OUT=ROOT/'.audit/2026-10-29-vault-cards';OUT.mkdir(exist_ok=True)
class Server(http.server.ThreadingHTTPServer):request_queue_size=64
class Handler(http.server.SimpleHTTPRequestHandler):
    def log_message(self,*_):pass
class Fixture:
    def __init__(self):self.scope='card-owner-a';self.posts=[];self.status=200;self.hold=None;self.pending=[];self.hosted=False;self.save_status=200
    def route(self,r):
        path=urlsplit(r.request.url).path;data={};status=200;kind=None
        if path=='/app/auth/config':kind='auth';data={'hosted':self.hosted}
        elif path=='/app/api/state':
            kind='state';status=self.status
            data={'workspaceId':self.scope,'agents':[{'id':'bot-a','name':'Chat A','status':'idle','t':'2026-10-28T04:00:00Z','av':{'shape':0,'color':4}}],'pending':[],'ai':{},'secrets':[],'outbound':{}} if status==200 else {'error':'unauthorized'}
        elif path.endswith('/messages'):data=[{'id':'card-same','taskId':'work-same','agentId':'bot-a','role':'agent','text':'Save fixture login','secret':{'vault':'login','label':'Fixture card'},'t':'2026-10-28T04:00:00Z'}]
        elif path.startswith('/app/api/vault'):
            it={'id':'aabbccdd','type':'login','label':'Fixture item','hint':{'site':'fixture.test','user':'f***'},'filled':['password']}
            if r.request.method=='GET':data={'workspaceId':self.scope,'items':[it],'uses':[]}
            elif path.endswith('/reveal'):data={'workspaceId':self.scope,'fields':{'password':'fixture-private-value'}}
            else:self.posts.append(r.request.post_data_json);data=it;status=self.save_status
        if kind and self.hold==kind:self.pending.append((r,status,json.dumps(data)));return
        r.fulfill(status=status,content_type='application/json',body=json.dumps(data))
def check(label):print('PASS '+label,flush=True)
server=Server(('127.0.0.1',0),functools.partial(Handler,directory=str(ROOT/'app/native/internal/web/assets')))
threading.Thread(target=server.serve_forever,daemon=True).start()
try:
    with sync_playwright() as p:
        chrome=os.environ.get('AURORA_TEST_CHROME')
        if not chrome and Path('C:/Program Files/Google/Chrome/Application/chrome.exe').exists():chrome='C:/Program Files/Google/Chrome/Application/chrome.exe'
        browser=p.chromium.launch(headless=True,executable_path=chrome)
        try:
            errors=[]
            def setup(fx,script='',wait='networkidle'):
                ctx=browser.new_context(locale='vi-VN',service_workers='block',reduced_motion='reduce')
                ctx.add_init_script('window.EventSource=class{addEventListener(){}close(){}};'+script)
                pg=ctx.new_page();pg.on('pageerror',lambda e:errors.append(str(e)))
                pg.route('**/app/api/**',fx.route);pg.route('**/app/auth/config',fx.route)
                pg.goto(f'http://127.0.0.1:{server.server_port}/app/#bot-a',wait_until=wait)
                return ctx,pg
            fx=Fixture();ctx,page=setup(fx)
            card=page.locator('#messages .vault-card');password=card.locator('input[data-secret="password"]')
            expect(password).to_be_visible();password.fill('fixture-owner-password')
            page.evaluate('''()=>{const put=Storage.prototype.setItem;Storage.prototype.setItem=function(k,v){if(k.startsWith('vault-saved:')||k.startsWith('aurora.vault.saved.'))throw Error('fixture quota');return put.call(this,k,v)}}''')
            card.locator('#vaultSave').click();expect(card).to_have_class(re.compile(r'\bsaved\b'))
            assert len(fx.posts)==1;expect(card.locator('input')).to_have_count(0)
            expect(page.locator('#toast')).to_contain_text('Đã lưu vào Vault')
            page.locator('#backBtn').click();page.locator('#rows .row[data-id="bot-a"] .rowin').click();expect(card).to_have_class(re.compile(r'\bsaved\b'))
            page.screenshot(path=str(OUT/'saved-desktop.png'))
            page.set_viewport_size({'width':390,'height':844});page.screenshot(path=str(OUT/'saved-mobile.png'))
            check('accepted save remains successful in memory when local marker persistence fails')
            ctx.close()
            fx=Fixture();ctx,page=setup(fx,'''const get=Storage.prototype.getItem;Storage.prototype.getItem=function(k){if(k.startsWith('vault-saved:')||k.startsWith('aurora.vault.saved.'))throw Error('fixture blocked read');return get.call(this,k)}''')
            card=page.locator('#messages .vault-card');expect(card.locator('input[data-secret="password"]')).to_be_visible()
            check('blocked marker reads do not prevent the chat or Vault form from rendering')
            ctx.close()
            fx=Fixture();ctx,page=setup(fx,'localStorage.setItem("vault-saved:card-same","legacy-other-owner-label")')
            card=page.locator('#messages .vault-card');expect(card.locator('input[data-secret="password"]')).to_be_visible()
            expect(card).not_to_contain_text('legacy-other-owner-label')
            card.locator('input[data-secret="password"]').fill('fixture-owner-password');card.locator('#vaultSave').click();expect(card).to_have_class(re.compile(r'\bsaved\b'))
            markers=page.evaluate('Object.fromEntries(Object.entries(localStorage).filter(([k])=>k.startsWith("aurora.vault.saved.")))')
            assert len(markers)==1 and 'fixture-owner-password' not in json.dumps(markers) and 'Fixture item' not in json.dumps(markers)
            assert page.evaluate('localStorage.getItem("vault-saved:card-same")')=='legacy-other-owner-label'
            page.reload();expect(card).to_have_class(re.compile(r'\bsaved\b'))
            check('new markers survive reload without secret or label values and leave unowned legacy markers untouched')
            fx.scope='card-owner-b';page.evaluate('window.__AURORA.reload()');page.locator('#rows .row[data-id="bot-a"] .rowin').click()
            expect(card.locator('input[data-secret="password"]')).to_be_visible()
            fx.scope='card-owner-a';page.evaluate('window.__AURORA.reload()');page.locator('#rows .row[data-id="bot-a"] .rowin').click();expect(card).to_have_class(re.compile(r'\bsaved\b'))
            check('the same work/message identifiers cannot share saved state across workspaces')
            key=next(iter(markers))
            for value in ['{broken',json.dumps({'version':2,'itemId':'aabbccdd'}),json.dumps({'version':1,'itemId':'invalid'}),'x'*1025]:
                page.evaluate('([key,value])=>localStorage.setItem(key,value)',[key,value]);page.reload();expect(card.locator('input[data-secret="password"]')).to_be_visible()
            check('a corrupt completion marker is ignored without hiding or breaking the form')
            ctx.close()
            fx=Fixture();ctx,page=setup(fx)
            def reload_state():page.evaluate('()=>{void window.__AURORA.reload().catch(()=>{})}')
            def pending():
                end=time.monotonic()+3
                while not fx.pending and time.monotonic()<end:page.wait_for_timeout(20)
                assert fx.pending,'pending fixture request missing'
                return fx.pending.pop(0)
            def release(held):held[0].fulfill(status=held[1],content_type='application/json',body=held[2])
            def reveal():
                page.evaluate('()=>{void window.__AURORA.openVault()}');body=page.locator('#vaultBody');body.locator('.vault-row').click();body.locator('[data-vreveal]').click();expect(body.locator('code.shown')).to_have_text('fixture-private-value')
            page.locator('#messages input[data-secret="password"]').fill('fixture-entered-secret');reveal()
            fx.status=401;reload_state();expect(page.locator('#vaultBody')).to_have_text('');expect(page.locator('#login')).to_be_visible()
            expect(page.locator('#messages input[data-secret="password"]')).to_have_value('')
            check('latest state authentication expiry scrubs revealed and entered Vault values before showing login')
            ctx.close()
            fx=Fixture();ctx,page=setup(fx);fx.status=401;fx.hold='state';reload_state();held=pending();fx.status=200;fx.hold=None
            page.evaluate('window.__AURORA.reload()');reveal();release(held)
            page.wait_for_timeout(100);expect(page.locator('#vaultBody code.shown')).to_have_text('fixture-private-value');expect(page.locator('#login')).to_be_hidden()
            check('an older state 401 cannot purge a newer authenticated view')
            page.keyboard.press('Escape');fx.status=401;fx.hold='auth';reload_state();held=pending();fx.status=200;fx.hold=None
            page.evaluate('window.__AURORA.reload()');reveal();release(held)
            page.wait_for_timeout(100);expect(page.locator('#vaultBody code.shown')).to_have_text('fixture-private-value');expect(page.locator('#login')).to_be_hidden()
            check('a delayed auth-config response cannot show login over a newer state')
            ctx.close()
            fx=Fixture();fx.hold='state';ctx,page=setup(fx,wait='domcontentloaded');held=pending();fx.status=401;fx.hold=None
            reload_state();expect(page.locator('#login')).to_be_visible();release(held);page.wait_for_timeout(100)
            expect(page.locator('#login')).to_be_visible();assert page.evaluate('window.__AURORA.current()') is None
            check('an older boot success cannot open a chat after a newer state expires')
            ctx.close()
            fx=Fixture();fx.save_status=500;ctx,page=setup(fx);card=page.locator('#messages .vault-card')
            card.locator('input[data-secret="password"]').fill('fixture-unsaved-password');card.locator('#vaultSave').click()
            expect(card.locator('.sec-err')).to_have_text('Không lưu được, thử lại.');expect(card.locator('input[data-secret="password"]')).to_have_value('fixture-unsaved-password')
            assert page.evaluate('Object.keys(localStorage).filter(k=>k.startsWith("aurora.vault.saved.")).length')==0
            expect(card.locator('#vaultSave')).to_be_enabled();fx.save_status=200;card.locator('#vaultSave').click();expect(card).to_have_class(re.compile(r'\bsaved\b'))
            check('a rejected server save leaves values available for an explicit retry without a completion marker')
            ctx.close();assert not errors,errors
            (OUT/'vault-cards-result.json').write_text(json.dumps({'result':'PASS','scenarios':10,'browser':browser.version,'errors':errors}),encoding='utf-8')
        finally:browser.close()
finally:server.shutdown();server.server_close()
