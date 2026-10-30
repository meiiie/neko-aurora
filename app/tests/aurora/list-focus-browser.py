"""Keyboard focus across real Aurora list rebuilds with local state fixtures."""
import functools,http.server,json,os,threading
from pathlib import Path
from urllib.parse import urlsplit
from playwright.sync_api import expect,sync_playwright

ROOT=Path(__file__).resolve().parents[3]
OUT=ROOT/'.audit/2026-10-28-list-focus';OUT.mkdir(exist_ok=True)
class Server(http.server.ThreadingHTTPServer):request_queue_size=64
class Handler(http.server.SimpleHTTPRequestHandler):
    def log_message(self,*_):pass
class Fixture:
    def __init__(self):
        self.scope='focus-owner-a';self.gets=[]
        self.bots=[{'id':'bot-'+str(i),'name':'Chat '+str(i),'status':'idle','t':f'2026-10-28T04:{i:02}:00Z','av':{'shape':0,'color':4},'pin':i==0} for i in range(20)]
    def route(self,r):
        path=urlsplit(r.request.url).path.removeprefix('/app/api');data={}
        if path=='/state':data={'workspaceId':self.scope,'agents':self.bots,'pending':[],'ai':{},'secrets':[],'outbound':{}}
        elif path.endswith('/messages'):
            assert r.request.method=='GET';self.gets.append(path)
            data=[{'id':'msg','agentId':path.split('/')[2],'role':'agent','text':'Focused chat message','t':'2026-10-28T04:00:00Z'}]
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
            context=browser.new_context(viewport={'width':390,'height':844},locale='vi-VN',service_workers='block',reduced_motion='reduce')
            context.add_init_script('''window.fixtureStreams=[];window.EventSource=class{constructor(){this.listeners={};this.readyState=1;fixtureStreams.push(this);queueMicrotask(()=>this.onopen?.({}))}addEventListener(t,f){(this.listeners[t]??=[]).push(f)}close(){this.closed=true}emit(t,d){for(const f of this.listeners[t]||[])f({data:JSON.stringify(d)})}}''')
            page=context.new_page();fx=Fixture();errors=[]
            page.on('pageerror',lambda e:errors.append(str(e)));page.route('**/app/api/**',fx.route)
            page.goto(f'http://127.0.0.1:{server.server_port}/app/',wait_until='networkidle')
            row=lambda n:page.locator(f'#rows .row[data-id="bot-{n}"] .rowin')
            row(15).focus();expect(row(15)).to_be_focused()
            page.evaluate('window.__AURORA.reload()')
            expect(row(15)).to_be_focused()
            page.keyboard.press('Enter');expect(page.locator('#threadName')).to_have_text('Chat 15')
            expect(page.locator('#messages')).to_contain_text('Focused chat message')
            expect(page.locator('#input')).to_be_focused()
            check('same-workspace refresh retains row focus and Enter opens the intended conversation')
            page.locator('#backBtn').click();row(15).focus()
            fx.bots[15]['t']='2026-10-28T05:00:00Z'
            page.locator('#listScroll').evaluate('n=>{n.scrollTop=400}')
            scroll=page.locator('#listScroll').evaluate('n=>n.scrollTop');assert scroll>0
            page.evaluate('window.__AURORA.reload()');expect(row(15)).to_be_focused()
            assert abs(page.locator('#listScroll').evaluate('n=>n.scrollTop')-scroll)<1
            check('a reordered chat retains identity and refresh does not scroll to its focused row')
            event=dict(fx.bots[15],name='Updated chat',status='working')
            page.evaluate('a=>fixtureStreams.at(-1).emit("agent",a)',event)
            expect(row(15)).to_contain_text('Updated chat');expect(row(15)).to_be_focused()
            check('an agent event keeps focus through the scheduled list render')
            featured=lambda n:page.locator(f'#featured .feat[data-id="bot-{n}"]')
            featured(0).focus();page.evaluate('window.__AURORA.reload()');expect(featured(0)).to_be_focused()
            fx.bots[0]['pin']=False;page.evaluate('window.__AURORA.reload()');expect(row(0)).to_be_focused()
            check('pinned focus survives refresh and follows the same bot when it is unpinned')
            pin=page.locator('#rows .row[data-id="bot-15"] .pin')
            pin.focus();page.evaluate('window.__AURORA.reload()');expect(pin).to_be_focused()
            fx.bots[15]['pin']=True;page.evaluate('window.__AURORA.reload()');expect(featured(15)).to_be_focused()
            fx.bots[15]['pin']=False;page.evaluate('window.__AURORA.reload()');expect(row(15)).to_be_focused()
            check('action focus survives refresh and pin/unpin changes follow the same bot')
            archived=page.locator('#rows .row[data-id="bot-16"] .arch')
            archived.focus();page.evaluate('window.__AURORA.reload()');expect(archived).to_be_focused()
            fx.bots=[b for b in fx.bots if b['id']!='bot-16'];page.evaluate('window.__AURORA.reload()')
            expect(page.locator('#search')).to_be_focused()
            check('removal of the focused bot moves focus to search without selecting a different bot')
            search=page.locator('#search');search.fill('Chat');search.evaluate('n=>n.setSelectionRange(1,3)')
            page.evaluate('window.__AURORA.reload()');expect(search).to_be_focused()
            assert search.evaluate('n=>[n.selectionStart,n.selectionEnd]')==[1,3]
            search.fill('');row(15).click(button='right')
            menu=page.locator('#ctxUnread');expect(menu).to_be_visible();menu.focus()
            page.evaluate('window.__AURORA.reload()');expect(menu).to_be_focused()
            page.keyboard.press('Escape');expect(page.locator('.ctx')).to_have_count(0)
            check('refresh does not steal search selection or context-menu focus')
            row(15).press('Enter');expect(page.locator('#messages')).to_contain_text('Focused chat message')
            inp=page.locator('#input');inp.fill('Preserve this draft');inp.focus()
            page.evaluate('window.__AURORA.reload()');expect(inp).to_be_focused();expect(inp).to_have_value('Preserve this draft')
            check('list refresh while a conversation is open leaves composer focus and draft untouched')
            page.locator('#backBtn').click();row(15).focus();fx.scope='focus-owner-b'
            page.evaluate('window.__AURORA.reload()');expect(row(15)).not_to_be_focused()
            assert page.evaluate('document.activeElement===document.body')
            check('workspace changes do not restore focus by a reused bot identifier')
            page.set_viewport_size({'width':1280,'height':900});row(15).focus()
            page.evaluate('window.__AURORA.reload()');expect(row(15)).to_be_focused()
            page.screenshot(path=str(OUT/'list-focus-desktop.png'))
            page.set_viewport_size({'width':390,'height':844});row(15).scroll_into_view_if_needed()
            page.screenshot(path=str(OUT/'list-focus-mobile.png'))
            page.keyboard.press('Enter');expect(page.locator('#threadName')).to_have_text('Chat 15')
            check('desktop and mobile-sized layouts retain operable focused chat controls')
            page.locator('#app').evaluate('n=>{n.scrollLeft=154}')
            assert page.locator('#app').evaluate('n=>n.scrollLeft')==0,'The fixed app shell must not scroll horizontally'
            page.wait_for_function('Math.abs(document.querySelector("#thread").getBoundingClientRect().left)<1')
            check('offscreen panels cannot shift the fixed app shell through programmatic scrolling')
            page.locator('#backBtn').click();row(15).focus();page.keyboard.down('Space')
            expect(page.locator('#thread')).to_have_class('screen thread off')
            page.keyboard.up('Space');expect(page.locator('#threadName')).to_have_text('Chat 15');expect(inp).to_be_focused()
            expect(inp).to_have_value('')
            page.locator('#backBtn').click();fx.bots[15]['pin']=True;page.evaluate('window.__AURORA.reload()')
            featured(15).focus();page.keyboard.press('Enter');expect(inp).to_be_focused()
            check('native Space activates on release and pinned keyboard activation focuses the composer')
            assert not errors,errors
            (OUT/'list-focus-result.json').write_text(json.dumps({'result':'PASS','scenarios':12,'browser':browser.version,'errors':errors}),encoding='utf-8')
        finally:browser.close()
finally:server.shutdown();server.server_close()
