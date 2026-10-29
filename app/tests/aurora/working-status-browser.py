"""Working feedback with controlled browser time and synthetic SSE events."""
import datetime,functools,http.server,json,os,threading
from pathlib import Path
from urllib.parse import urlsplit
from playwright.sync_api import expect,sync_playwright
from fixture_http import FixtureHTTPServer
ROOT=Path(__file__).resolve().parents[3];OUT=ROOT/'.audit/2026-10-29-working-status';OUT.mkdir(exist_ok=True)
class Handler(http.server.SimpleHTTPRequestHandler):
    def log_message(self,*_):pass
class Fixture:
    def __init__(self):
        self.scope='status-owner-a';self.posts=[];self.messages={'a':[],'b':[]};self.agents={x:dict(id=x,name='Status '+x,status='idle',av={}) for x in ['a','b']}
    def route(self,r):
        path=urlsplit(r.request.url).path.removeprefix('/app/api');data={}
        if r.request.method!='GET':self.posts.append(path)
        if path=='/state':data=dict(workspaceId=self.scope,agents=list(self.agents.values()),pending=[],ai={},secrets=[],outbound={})
        elif path.endswith('/messages'):data=self.messages[path.split('/')[2]]
        r.fulfill(status=200,content_type='application/json',body=json.dumps(data))
server=FixtureHTTPServer(('127.0.0.1',0),functools.partial(Handler,directory=str(ROOT/'app/native/internal/web/assets')))
threading.Thread(target=server.serve_forever,daemon=True).start()
try:
    with sync_playwright() as p:
        browser=p.chromium.launch(headless=True,executable_path=os.environ.get('AURORA_TEST_CHROME') or 'C:/Program Files/Google/Chrome/Application/chrome.exe')
        try:
            ctx=browser.new_context(viewport=dict(width=390,height=844),locale='vi-VN',service_workers='block',reduced_motion='reduce');ctx.add_init_script('''window.EventSource=class{constructor(){this.listeners={};window.statusEvents=this;this.readyState=1}addEventListener(k,f){this.listeners[k]=f}close(){}emit(k,v){this.listeners[k]?.({data:JSON.stringify(v)})}}''')
            page=ctx.new_page();page.clock.install();fx=Fixture();errors=[];page.on('pageerror',lambda e:errors.append(str(e)));page.route('**/app/api/**',fx.route)
            page.goto(f'http://127.0.0.1:{server.server_port}/app/#a',wait_until='networkidle');expect(page.locator('#threadName')).to_have_text('Status a')
            page.clock.pause_at(datetime.datetime.now(datetime.timezone.utc)+datetime.timedelta(seconds=1))
            line=page.locator('#messages .status');words=line.locator('.st-t');stop=line.locator('.st-stop')
            def emit(kind,data):page.evaluate('([k,v])=>window.statusEvents.emit(k,v)',[kind,data])
            def state(status,bot='a',**extra):
                fx.agents[bot].update(status=status,**extra);emit('agent',fx.agents[bot])
            def message(id,role='user',text='Chào bạn',bot='a',**extra):
                m=dict(id=id,agentId=bot,taskId=id,role=role,text=text,t=page.evaluate('new Date().toISOString()'),**extra)
                fx.messages[bot]=[x for x in fx.messages[bot] if x['id']!=id]+[m];emit('message',m)
            message('hello');state('working');assert words.inner_text()=='';expect(stop).to_be_visible();page.clock.run_for(500);assert words.inner_text()==''
            page.clock.run_for(2200);expect(words.locator('span:not(.out)')).to_have_text('Đang xử lý');page.clock.run_for(25000);expect(words.locator('span:not(.out)')).to_have_text('Đang xử lý')
            print('PASS generic wording waits and a greeting never invents progress after elapsed time')
            message('thanks',text='Cảm ơn nhé');assert words.inner_text()=='';page.clock.run_for(2700);expect(words.locator('span:not(.out)')).to_have_text('Đang xử lý')
            print('PASS a new user turn resets generic feedback and thanks keep neutral wording')
            message('tool-user',text='Tìm tài liệu');message('tool',role='agent',text='',tools=[dict(name='web_search',status='running',arg='fixture')]);expect(words.locator('span:not(.out)')).to_have_text('Đang tìm trên web')
            page.screenshot(path=str(OUT/'specific-mobile.png'))
            message('tool',role='agent',text='',tools=[dict(name='web_search',status='done',arg='fixture')]);assert words.inner_text()=='';page.clock.run_for(2700);expect(words.locator('span:not(.out)')).to_have_text('Đang xử lý')
            print('PASS a specific tool appears immediately and completion returns to delayed neutral feedback')
            state('working',snip='Bước 2/4 · Kiểm tra');expect(words.locator('span:not(.out)')).to_have_text('Bước 2/4 · Kiểm tra');state('working',snip='')
            message('approval',role='agent',text='',approval=dict(id='ap',kind='computer',state='pending',body='echo fixture'));expect(words.locator('span:not(.out)')).to_have_text('Chờ bạn duyệt')
            message('approval',role='agent',text='',approval=dict(id='ap',kind='computer',state='rejected',body='echo fixture'));state('idle');expect(line).to_have_class('status off');page.clock.run_for(3000);expect(line).to_have_class('status off')
            print('PASS plan steps and approvals are immediate and idle never revives delayed text')
            message('stop-me',text='Làm tiếp');state('working');assert words.inner_text()=='';expect(stop).to_be_visible();stop.evaluate('n=>n.click()');page.wait_for_timeout(80);assert fx.posts==['/agents/a/stop'];state('idle');page.clock.run_for(3000);expect(line).to_have_class('status off')
            print('PASS stop remains usable before generic wording appears')
            message('stream-user',text='Giải thích');state('working');emit('delta',dict(id='streaming',agentId='a',text='Câu trả lời đang đến'));page.clock.run_for(600);expect(line).to_have_class('status off');page.clock.run_for(5000);expect(line).to_have_class('status off')
            message('streaming',role='agent',text='Câu trả lời hoàn tất');state('idle');page.clock.run_for(3000)
            print('PASS visible streamed text suppresses pending working feedback')
            message('short',text='Chào');state('working');state('idle');page.clock.run_for(3000);expect(line).to_have_class('status off')
            print('PASS a short completed turn never flashes generic wording')
            message('switch',text='Tiếp tục');state('working');page.clock.run_for(2700);page.locator('#backBtn').evaluate('n=>n.click()');page.locator('#rows .row[data-id=b] .rowin').evaluate('n=>n.click()');expect(page.locator('#threadName')).to_have_text('Status b');expect(line).to_have_class('status off')
            message('b-user',bot='b');state('working',bot='b');assert words.inner_text()=='';page.clock.run_for(2700);expect(words.locator('span:not(.out)')).to_have_text('Đang xử lý')
            print('PASS switching chats clears previous wording and starts a fresh delay')
            fx.scope='status-owner-b';fx.agents['b']['status']='idle';page.evaluate('window.__AURORA.reload()');page.locator('#rows .row[data-id=b] .rowin').evaluate('n=>n.click()');expect(line).to_have_class('status off');message('b-new',bot='b');state('working',bot='b');assert words.inner_text()==''
            print('PASS workspace replacement cannot reuse the former generic display state')
            page.clock.resume();page.set_viewport_size(dict(width=1280,height=900))
            with page.expect_navigation(wait_until='load'):page.evaluate('window.__I18N.set("en")')
            page.wait_for_timeout(2700);expect(page.locator('#messages .st-t')).to_have_text('Working on it');page.screenshot(path=str(OUT/'generic-desktop-en.png'))
            page.evaluate("window.statusWord=document.querySelector('#messages .st-t > span:not(.out)')");page.wait_for_timeout(2200);assert page.evaluate("window.statusWord===document.querySelector('#messages .st-t > span:not(.out)')")
            assert not errors,errors
            print('PASS English desktop renders neutral feedback without browser errors')
        finally:browser.close()
finally:server.shutdown();server.server_close()
