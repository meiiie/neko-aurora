"""Long-chat reading position, native anchoring and pagination in real Chrome."""
from pathlib import Path
import functools,http.server,json,os,threading
from urllib.parse import urlsplit
from playwright.sync_api import expect,sync_playwright
from fixture_http import FixtureHTTPServer
ROOT=Path(__file__).resolve().parents[3]
OUT=ROOT/'.audit/2026-10-29-chat-resize';OUT.mkdir(exist_ok=True)
class Handler(http.server.SimpleHTTPRequestHandler):
    def log_message(self,*_):pass
class Fixture:
    def __init__(self):self.posts=[]
    def route(self,r):
        path=urlsplit(r.request.url).path.removeprefix('/app/api');data={}
        if r.request.method!='GET':self.posts.append(path)
        if path=='/state':data=dict(workspaceId='resize-fixture',agents=[dict(id=x,name='Resize '+x.upper(),status='idle',av={}) for x in ['a','b']],pending=[],ai={},secrets=[],outbound={})
        elif path.endswith('/messages'):
            agent=path.split('/')[2];data=[dict(id=agent+str(i),agentId=agent,role='agent',text='Message '+str(i)+'\n\n'+('Long text with words and wrapping lines. '*12),t='2026-10-29T04:00:00Z') for i in range(100 if agent=='a' else 5)]
        r.fulfill(status=200,content_type='application/json',body=json.dumps(data))
READ='''()=>{const b=document.querySelector('#messages'),top=b.getBoundingClientRect().top,n=[...b.querySelectorAll('[data-message-id]')].find(n=>n.getBoundingClientRect().bottom>top+5);return {id:n?.dataset.messageId,offset:n?n.getBoundingClientRect().top-top:0,scroll:b.scrollTop,bottom:b.scrollHeight-b.clientHeight-b.scrollTop,width:b.clientWidth}}'''
server=FixtureHTTPServer(('127.0.0.1',0),functools.partial(Handler,directory=str(ROOT/'app/native/internal/web/assets')))
threading.Thread(target=server.serve_forever,daemon=True).start()
try:
    with sync_playwright() as p:
        browser=p.chromium.launch(headless=True,executable_path=os.environ.get('AURORA_TEST_CHROME') or 'C:/Program Files/Google/Chrome/Application/chrome.exe')
        try:
            ctx=browser.new_context(viewport=dict(width=1280,height=900),service_workers='block',reduced_motion='reduce');ctx.add_init_script('window.EventSource=class{addEventListener(){}close(){}}')
            page=ctx.new_page();fx=Fixture();errors=[];evidence=[];page.on('pageerror',lambda e:errors.append(str(e)));page.route('**/app/api/**',fx.route)
            page.goto(f'http://127.0.0.1:{server.server_port}/app/#a',wait_until='networkidle');expect(page.locator('[data-message-id]')).to_have_count(60)
            def settle():page.evaluate('()=>new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))')
            def resize(width,height):page.set_viewport_size(dict(width=width,height=height));settle();page.wait_for_timeout(80)
            def position():return page.evaluate(READ)
            def target(offset=0):
                page.locator('[data-message-id=a70]').evaluate('(n,offset)=>{n.scrollIntoView({block:"start"});document.querySelector("#messages").scrollTop+=offset}',offset);settle();page.wait_for_timeout(50);return position()
            before=target()
            for width,height in [(390,844),(900,650),(460,900),(1280,900),(390,620),(1280,700)]:
                resize(width,height);after=position();evidence.append(dict(before=before,after=after,viewport=[width,height]));assert after['id']==before['id'] and abs(after['offset']-before['offset'])<2,(before,after)
            print('PASS repeated width and height changes retain the same message within two pixels')
            before=target(85)
            for width,height in [(390,844),(1280,900),(390,650)]:
                resize(width,height);after=position();assert after['id']==before['id'] and abs(after['offset']-before['offset'])<2,(before,after)
            print('PASS resizing inside a long paragraph retains its reading offset')
            box=page.locator('#messages');bounds=box.bounding_box();page.mouse.move(bounds['x']+bounds['width']/2,bounds['y']+bounds['height']/2);start=position()['scroll'];page.mouse.wheel(0,260)
            page.wait_for_function('(start)=>document.querySelector("#messages").scrollTop>start+150',arg=start);page.wait_for_timeout(160);after=position();assert 150<after['scroll']-start<350 and after['bottom']>1000,(start,after)
            later=position()['scroll'];page.wait_for_timeout(160);assert abs(position()['scroll']-later)<2
            print('PASS wheel scrolling after resize moves locally without a later jump or bottom snap')
            before=target();page.locator('.older').evaluate('n=>n.click()');expect(page.locator('[data-message-id]')).to_have_count(100);settle();after=position();assert after['id']==before['id'] and abs(after['offset']-before['offset'])<2,(before,after)
            resize(1280,900);after=position();assert after['id']==before['id'] and abs(after['offset']-before['offset'])<2
            print('PASS older-message expansion and later resize preserve the visible context')
            target();selected=page.locator('[data-message-id=a70] .msg p').last.evaluate('''n=>{const t=n.firstChild,r=document.createRange();r.setStart(t,4);r.setEnd(t,40);const s=getSelection();s.removeAllRanges();s.addRange(r);return s.toString()}''');settle();before=position();resize(390,844)
            assert page.evaluate('getSelection().toString()')==selected;after=position();assert after['id']==before['id'] and after['bottom']>1000
            page.screenshot(path=str(OUT/'reading-mobile.png'));page.evaluate('getSelection().removeAllRanges()');settle()
            print('PASS a selected reading excerpt survives resize without following the conversation bottom')
            box.evaluate('n=>n.scrollTop=n.scrollHeight');settle();page.wait_for_timeout(80)
            for width,height in [(1280,900),(390,550),(1280,700)]:resize(width,height);assert position()['bottom']<3,position()
            print('PASS explicit bottom-follow remains at the latest message after resize')
            page.locator('#backBtn').click();page.locator('#rows .row[data-id=b] .rowin').click();expect(page.locator('#whoBtn')).to_contain_text('Resize B');expect(page.locator('[data-message-id]')).to_have_count(5);resize(390,844);assert position()['bottom']<3
            assert not errors and not fx.posts,(errors,fx.posts);(OUT/'reading-positions.json').write_text(json.dumps(evidence,indent=2));print('PASS switching chats discards old reading context without sending a message')
        finally:browser.close()
finally:server.shutdown();server.server_close()
