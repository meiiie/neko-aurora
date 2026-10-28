"""Usage cache, range ownership and read recovery using loopback fixtures."""
import functools,http.server,json,os,threading,time
from pathlib import Path
from urllib.parse import urlsplit,parse_qs
from playwright.sync_api import expect,sync_playwright
ROOT=Path(__file__).resolve().parents[3]
OUT=ROOT/'.audit/2026-10-29-usage-recovery';OUT.mkdir(exist_ok=True)
class Server(http.server.ThreadingHTTPServer):request_queue_size=64
class Handler(http.server.SimpleHTTPRequestHandler):
    def log_message(self,*_):pass
class Fixture:
    def __init__(self):self.scope='usage-owner-a';self.tokens=1200;self.fail=False;self.hold=False;self.pending=[];self.calls=[];self.invalid=False
    def route(self,r):
        req=r.request;u=urlsplit(req.url);path=u.path.removeprefix('/app/api');data={};status=200
        self.calls.append((path,req.method,parse_qs(u.query)))
        if path=='/state':data=dict(workspaceId=self.scope,agents=[dict(id='bot-a',name='Phiên bản của tôi',status='idle',av=dict(shape=0,color=4))],pending=[],ai={},secrets=[],outbound={})
        elif path=='/usage':
            period=int(parse_qs(u.query).get('range',['7'])[0]);count=self.tokens if period==7 else self.tokens+period
            data=dict(workspaceId=self.scope,range=period,estimated=True,total=dict(key='total',calls=period,tokens=count),days=[],bots=[dict(key='bot-a',calls=period,tokens=count)],kinds=[dict(key='chat',calls=period,tokens=count)],names={'bot-a':'Phiên bản của tôi'})
            if self.invalid:data['total']['tokens']=-1
            if self.fail:status=500
            if self.hold:self.pending.append((r,status,json.dumps(data)));return
        elif path.endswith('/messages'):data=[]
        elif path.endswith('/computer'):data=dict(available=False,log=[],files=[])
        elif path=='/schedules':data={'schedules':[]}
        r.fulfill(status=status,content_type='application/json',body=json.dumps(data))
checks=[]
def check(s):checks.append(s);print('PASS '+s,flush=True)
server=Server(('127.0.0.1',0),functools.partial(Handler,directory=str(ROOT/'app/native/internal/web/assets')))
threading.Thread(target=server.serve_forever,daemon=True).start()
try:
 with sync_playwright() as p:
    chrome=os.environ.get('AURORA_TEST_CHROME') or 'C:/Program Files/Google/Chrome/Application/chrome.exe'
    browser=p.chromium.launch(headless=True,executable_path=chrome)
    try:
        context=browser.new_context(viewport={'width':390,'height':844},locale='vi-VN',service_workers='block',reduced_motion='reduce');context.add_init_script('window.EventSource=class{addEventListener(){}close(){}}')
        page=context.new_page();fx=Fixture();errors=[];page.on('pageerror',lambda e:errors.append(str(e)));page.route('**/app/api/**',fx.route)
        url=f'http://127.0.0.1:{server.server_port}/app/';page.goto(url,wait_until='networkidle');body=page.locator('#usageBody');hero=body.locator('.use-hero');status=body.locator('.usage-status')
        def open_usage():page.locator('#meBtn').click();page.locator('#openUsage').click()
        def close_usage():page.keyboard.press('Escape');page.wait_for_timeout(450)
        def pending():
            end=time.monotonic()+3
            while not fx.pending and time.monotonic()<end:page.wait_for_timeout(20)
            assert fx.pending;return fx.pending.pop(0)
        def release(h):h[0].fulfill(status=h[1],content_type='application/json',body=h[2])
        open_usage();expect(hero).to_contain_text('1.2K');close_usage();fx.fail=True;open_usage();expect(status).to_contain_text('Chưa tải được');expect(hero).to_contain_text('1.2K');expect(body.get_by_role('button',name='Thử lại',exact=True)).to_be_enabled()
        check('read failure retains the last reading with explicit retry')
        fx.fail=False;fx.tokens=2400;body.get_by_role('button',name='Thử lại',exact=True).focus();page.keyboard.press('Enter');expect(hero).to_contain_text('2.4K');expect(status).not_to_contain_text('Chưa tải được')
        check('keyboard retry updates the reading through GET only')
        close_usage();fx.fail=True;page.reload(wait_until='networkidle');open_usage();expect(hero).to_contain_text('2.4K');expect(status).to_contain_text('Chưa tải được');expect(body.locator('time')).to_have_count(1)
        check('last reading survives reload and is labeled with its saved time')
        fx.fail=False;fx.hold=True;body.get_by_role('button',name='24 giờ',exact=True).click();old=pending();fx.hold=False;body.get_by_role('button',name='30 ngày',exact=True).click();expect(body.get_by_role('button',name='30 ngày',exact=True)).to_have_attribute('aria-checked','true');expect(hero).to_contain_text('2.4K');release(old);page.wait_for_timeout(80);expect(body.get_by_role('button',name='30 ngày',exact=True)).to_have_attribute('aria-checked','true')
        expect(hero).to_contain_text('30 lượt');assert fx.calls[-1][0]=='/usage';check('a slower range response cannot overwrite the selected period or its numbers')
        fx.fail=True;body.get_by_role('button',name='12 tháng',exact=True).click();expect(status).to_contain_text('Chưa tải được');expect(hero).to_have_count(0)
        check('a period without a reading never borrows numbers from another period')
        fx.fail=False;body.get_by_role('button',name='7 ngày',exact=True).click();expect(hero).to_contain_text('2.4K');fx.invalid=True;close_usage();open_usage();expect(status).to_contain_text('Chưa tải được');expect(hero).to_contain_text('2.4K');fx.invalid=False
        check('malformed server numbers cannot replace a valid cached reading')
        close_usage();fx.hold=True;open_usage();old=pending();close_usage();fx.hold=False;fx.tokens=4800;open_usage();expect(hero).to_contain_text('4.8K');release(old);page.wait_for_timeout(80);expect(hero).to_contain_text('4.8K')
        check('a response from a closed view cannot overwrite a reopened view in the same workspace')
        close_usage();fx.hold=True;open_usage();old=pending();close_usage();expect(body).to_have_text('');fx.hold=False;fx.scope='usage-owner-b';page.evaluate('()=>{void window.__AURORA.reload()}');page.wait_for_function('window.__AURORA.state().workspaceId==="usage-owner-b"');fx.fail=True;open_usage();expect(status).to_contain_text('Chưa tải được');expect(hero).to_have_count(0);release(old);page.wait_for_timeout(80);expect(hero).to_have_count(0)
        check('workspace changes and close revoke stale responses and cached identities')
        close_usage();page.evaluate('localStorage.setItem("aurora.usage.v1.usage-owner-b.7","{broken")');page.reload(wait_until='networkidle');open_usage();expect(status).to_contain_text('Chưa tải được');expect(hero).to_have_count(0)
        check('corrupt cache is ignored without displaying fabricated zero usage')
        page.evaluate('''()=>{window.originalSetItem=Storage.prototype.setItem;Storage.prototype.setItem=function(k,v){if(k.startsWith('aurora.usage.v1.'))throw Error('fixture full');return window.originalSetItem.call(this,k,v)}}''');fx.fail=False;fx.tokens=3600;body.get_by_role('button',name='Thử lại',exact=True).click();expect(hero).to_contain_text('3.6K');expect(status).to_contain_text('Chưa lưu được');close_usage();fx.fail=True;open_usage();expect(hero).to_contain_text('3.6K');expect(status).to_contain_text('Chưa tải được')
        page.screenshot(path=str(OUT/'usage-recovery-mobile.png'));check('storage failure preserves the reading in this window and explains durability')
        assert not any(method!='GET' for _,method,_ in fx.calls)
        assert all(q.get('workspaceId') in [['usage-owner-a'],['usage-owner-b']] for path,_,q in fx.calls if path=='/usage')
        check('usage reads bind their workspace and never dispatch mutations')
        close_usage();fx.fail=False
        with page.expect_navigation(wait_until='networkidle'):page.evaluate('window.__I18N.set("en")')
        open_usage();expect(hero).to_contain_text('3.6K');expect(body).to_contain_text('Phiên bản của tôi');page.set_viewport_size({'width':1280,'height':900});page.screenshot(path=str(OUT/'usage-desktop-en.png'));assert not errors,errors
        check('English UI keeps user-defined bot names unchanged')
        (OUT/'usage-browser-result.json').write_bytes(json.dumps({'result':'PASS','scenarios':len(checks),'browser':browser.version,'errors':errors}).encode())
    finally:browser.close()
finally:server.shutdown();server.server_close()
