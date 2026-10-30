"""Credential-card ownership with synthetic secrets, held HTTP and SSE fixtures."""
import functools,http.server,json,os,re,threading,time,traceback
from pathlib import Path
from urllib.parse import urlsplit
from playwright.sync_api import expect,sync_playwright

ROOT=Path(__file__).resolve().parents[3];OUT=ROOT/'.audit/2026-10-29-secret-lifecycle';OUT.mkdir(exist_ok=True)
class Server(http.server.ThreadingHTTPServer):request_queue_size=64
class Handler(http.server.SimpleHTTPRequestHandler):
    def log_message(self,*_):pass
class Fixture:
    def __init__(self):
        self.scope='secret-owner-a';self.auth=200;self.status=200;self.hold=False;self.pending=[];self.posts=[];self.state_reads=0;self.secrets=[]
        self.secret={'name':'FIXTURE_SECRET','label':'Fixture secret'}
    def message(self,text='Save fixture credential'):
        return dict(id='credential-card',taskId='work-a',agentId='bot-a',role='agent',text=text,secret=self.secret,t='2026-10-29T00:00:00Z')
    def route(self,r):
        path=urlsplit(r.request.url).path;data={};status=200
        if path=='/app/auth/config':data={'hosted':False}
        elif path=='/app/api/state':
            self.state_reads+=1;status=self.auth
            data=dict(workspaceId=self.scope,agents=[dict(id='bot-a',name='Chat A',status='idle',av={}),dict(id='bot-b',name='Chat B',status='idle',av={})],pending=[],ai={},secrets=self.secrets,outbound={})
        elif path.endswith('/messages'):data=[self.message()] if '/bot-a/' in path else []
        elif path=='/app/api/secrets' and r.request.method=='POST':
            self.posts.append(r.request.post_data_json);status=self.status;data={'name':'FIXTURE_SECRET'}
            if self.hold:self.pending.append((r,status,data));return
            if status==200:self.secrets=['FIXTURE_SECRET']
        r.fulfill(status=status,content_type='application/json',body=json.dumps(data))

checks=[];failures=[];errors=[]
server=Server(('127.0.0.1',0),functools.partial(Handler,directory=str(ROOT/'app/native/internal/web/assets')));threading.Thread(target=server.serve_forever,daemon=True).start()
try:
 with sync_playwright() as p:
    browser=p.chromium.launch(headless=True,executable_path=os.environ.get('AURORA_TEST_CHROME') or 'C:/Program Files/Google/Chrome/Application/chrome.exe')
    def run(name,action,secret=None):
        fx=Fixture()
        if secret:fx.secret=secret.copy()
        ctx=browser.new_context(viewport={'width':390,'height':844},locale='vi-VN',service_workers='block',reduced_motion='reduce')
        ctx.add_init_script('window.fixtureStreams=[];window.EventSource=class{constructor(){this.listeners={};fixtureStreams.push(this)}addEventListener(t,f){(this.listeners[t]??=[]).push(f)}close(){}emit(t,d){for(const f of this.listeners[t]||[])f({data:JSON.stringify(d)})}}')
        page=ctx.new_page();page.on('pageerror',lambda e:errors.append(str(e)));page.route('**/app/api/**',fx.route);page.route('**/app/auth/config',fx.route)
        try:
            page.goto(f'http://127.0.0.1:{server.server_port}/app/#bot-a',wait_until='networkidle');expect(page.locator('#messages input[data-secret]').first).to_be_visible()
            action(page,fx);checks.append(name);print('PASS '+name,flush=True)
        except Exception as e:failures.append(name);print('FAIL '+name+' ('+type(e).__name__+' at '+str(traceback.extract_tb(e.__traceback__)[-2].lineno)+')',flush=True)
        finally:ctx.close()
    def field(page):
        password=page.locator('#messages input[data-secret="password"]')
        return password if password.count() else page.locator('#messages input[data-secret]').first
    def remember(page):field(page).evaluate('(i)=>window.oldCredential=i')
    def reopen(page):page.locator('#rows .rowin').filter(has_text='Chat A').click();expect(field(page)).to_be_visible()
    def scrub(page,fx,reason):
        field(page).fill('fixture-unsaved');remember(page)
        if reason=='leave':page.locator('#backBtn').click()
        else:
            if reason=='workspace':fx.scope='secret-owner-b'
            else:fx.auth=401
            page.evaluate('window.__AURORA.reload().catch(()=>{})')
            if reason=='expiry':expect(page.locator('#login')).to_be_visible()
            else:page.wait_for_function('window.__AURORA.state().workspaceId==="secret-owner-b"')
        assert page.evaluate('oldCredential.value')==''
        if reason=='leave':
            reopen(page);expect(field(page)).to_have_value('');field(page).fill('fixture-new');page.locator('#messages .sec-save').click();expect(page.locator('#messages .secret')).to_have_class(re.compile(r'\bsaved\b'));assert len(fx.posts)==1 and fx.posts[0]['workspaceId']=='secret-owner-a'
    def delayed(page,fx,status,reason):
        fx.hold=True;fx.status=status;field(page).fill('fixture-pending');remember(page);page.locator('#messages .sec-save').click()
        end=time.monotonic()+3
        while not fx.pending and time.monotonic()<end:page.wait_for_timeout(20)
        assert len(fx.pending)==1 and len(fx.posts)==1
        page.locator('#backBtn').click()
        if reason=='workspace':
            fx.scope='secret-owner-b';page.evaluate('window.__AURORA.reload()');page.wait_for_function('window.__AURORA.state().workspaceId==="secret-owner-b"')
        reopen(page);field(page).fill('fixture-current');reads=fx.state_reads
        r,code,data=fx.pending.pop();r.fulfill(status=code,content_type='application/json',body=json.dumps(data));page.wait_for_timeout(150)
        expect(field(page)).to_have_value('fixture-current');expect(page.locator('#messages .sec-save')).to_be_enabled();expect(page.locator('#messages .sec-err')).to_have_text('')
        assert fx.state_reads==reads and page.evaluate('oldCredential.value')=='' and page.evaluate('window.__AURORA.state().secrets.length')==0
        assert 'Đã lưu FIXTURE_SECRET' not in page.locator('#toast').inner_text()
        assert fx.posts[0]['workspaceId']=='secret-owner-a'
    def failed_retry(page,fx):
        fx.status=500;field(page).fill('fixture-retry');page.locator('#messages .sec-save').click();expect(page.locator('#messages .sec-err')).to_contain_text('Không lưu được');expect(field(page)).to_have_value('fixture-retry');expect(page.locator('#messages .sec-save')).to_be_enabled()
        fx.status=200;page.locator('#messages .sec-save').click();expect(page.locator('#messages .secret')).to_have_class(re.compile(r'\bsaved\b'));assert len(fx.posts)==2
        assert not page.evaluate('Object.values(localStorage).some(v=>v.includes("fixture-retry"))')
        page.screenshot(path=str(OUT/'secret-saved-mobile.png'))
    def update(page,fx,change):
        field(page).fill('fixture-draft');remember(page)
        page.evaluate('(m)=>fixtureStreams.at(-1).emit("message",m)',fx.message('Refreshed prompt'))
        expect(field(page)).to_have_value('fixture-draft')
        fx.secret.update(change);page.evaluate('(m)=>fixtureStreams.at(-1).emit("message",m)',fx.message('Changed credential context'))
        expect(field(page)).to_have_value('')
        assert page.evaluate('oldCredential.value')==''
    def preserve_site(page,fx):
        site=page.locator('#messages input[data-secret="site"]');site.fill('draft.example.test');field(page).fill('fixture-draft')
        page.evaluate('(m)=>fixtureStreams.at(-1).emit("message",m)',fx.message('Refreshed prompt'))
        expect(site).to_have_value('draft.example.test');expect(field(page)).to_have_value('fixture-draft')
        site.fill('');page.evaluate('(m)=>fixtureStreams.at(-1).emit("message",m)',fx.message('Another update'));expect(site).to_have_value('')
    def saved_state(page,fx):
        field(page).fill('fixture-draft');remember(page);fx.secrets=['FIXTURE_SECRET'];page.evaluate('window.__AURORA.reload()')
        expect(page.locator('#messages .secret')).to_have_class(re.compile(r'\bsaved\b'));assert page.evaluate('oldCredential.value')==''
        fx.secrets=[];page.evaluate('window.__AURORA.reload()');expect(field(page)).to_be_visible();expect(field(page)).to_have_value('')
        page.set_viewport_size({'width':1280,'height':900});page.screenshot(path=str(OUT/'secret-entry-desktop.png'))
    try:
        for reason in ['leave','workspace','expiry']:run('generic credential scrub on '+reason,lambda page,fx,reason=reason:scrub(page,fx,reason))
        for status in [200,500]:
            for reason in ['reopen','workspace']:run(f'late HTTP{status} cannot mutate {reason}',lambda page,fx,status=status,reason=reason:delayed(page,fx,status,reason))
        run('failed save preserves draft and explicit retry acknowledges completion',failed_retry)
        run('server secret state refresh invalidates cached credential cards',saved_state)
        run('generic credential draft belongs to its label and name',lambda page,fx:update(page,fx,{'label':'Different owner label'}))
        run('same typed card preserves edits to prefilled fields',preserve_site,{'vault':'login','label':'Fixture login','site':'example.test'})
        for change in [{'site':'other.example.test'},{'label':'Changed target'},{'vault':'identity'}]:
            run('typed credential draft rejects changed '+next(iter(change)),lambda page,fx,change=change:update(page,fx,change),{'vault':'login','label':'Fixture login','site':'example.test'})
        assert not errors,'browser page error'
        (OUT/'secret-browser-result.json').write_bytes(json.dumps(dict(result='FAIL' if failures else 'PASS',scenarios=len(checks),failures=failures,browser=browser.version,errors=errors)).encode())
        assert not failures,'credential lifecycle scenarios failed'
    finally:browser.close()
finally:server.shutdown();server.server_close()
