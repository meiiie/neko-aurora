"""User content stays literal while UI actions and statuses remain localized."""
import functools,http.server,json,os,threading
from pathlib import Path
from urllib.parse import urlsplit
from playwright.sync_api import expect,sync_playwright
ROOT=Path(__file__).resolve().parents[3]
OUT=ROOT/'.audit/2026-10-29-content-i18n';OUT.mkdir(exist_ok=True)
class Server(http.server.ThreadingHTTPServer):request_queue_size=64
class Handler(http.server.SimpleHTTPRequestHandler):
    def log_message(self,*_):pass
class Fixture:
    def __init__(self):
        self.calls=[]
        self.bots=[dict(id='plain',name='Phiên bản của tôi',desc='Công việc',snip='Hỏi nhóm của tôi',status='idle',av=dict(shape=0,color=4)),dict(id='pinned',name='Cài đặt',desc='Học tập',status='idle',pin=True,av=dict(shape=0,color=1)),dict(id='group',name='Nghiên cứu',kind='group',members=['plain','pinned'],status='idle',av={})]
    def route(self,r):
        path=urlsplit(r.request.url).path.removeprefix('/app/api');self.calls.append((path,r.request.method));data={}
        if path=='/state':data=dict(workspaceId='content-owner',agents=self.bots,pending=[],ai={},secrets=[],outbound={})
        elif path.endswith('/messages'):data=[]
        elif path.endswith('/computer'):data=dict(available=False,log=[],files=[])
        elif path=='/schedules':data={'schedules':[]}
        r.fulfill(status=200,content_type='application/json',body=json.dumps(data))
checks=[]
def check(s):checks.append(s);print('PASS '+s,flush=True)
server=Server(('127.0.0.1',0),functools.partial(Handler,directory=str(ROOT/'app/native/internal/web/assets')))
threading.Thread(target=server.serve_forever,daemon=True).start()
try:
 with sync_playwright() as p:
    browser=p.chromium.launch(headless=True,executable_path=os.environ.get('AURORA_TEST_CHROME') or 'C:/Program Files/Google/Chrome/Application/chrome.exe')
    try:
        context=browser.new_context(viewport={'width':390,'height':844},locale='en-US',service_workers='block',reduced_motion='reduce')
        context.add_init_script('''window.fixtureStreams=[];window.EventSource=class{constructor(){this.listeners={};fixtureStreams.push(this)}addEventListener(t,f){(this.listeners[t]??=[]).push(f)}close(){}emit(t,d){for(const f of this.listeners[t]||[])f({data:JSON.stringify(d)})}}''')
        page=context.new_page();fx=Fixture();errors=[];page.on('pageerror',lambda e:errors.append(str(e)));page.route('**/app/api/**',fx.route)
        page.goto(f'http://127.0.0.1:{server.server_port}/app/',wait_until='networkidle')
        row=lambda id:page.locator(f'#rows .row[data-id="{id}"]')
        expect(page.locator('html')).to_have_attribute('lang','en')
        expect(row('plain').locator('.rowh b')).to_have_text('Phiên bản của tôi');expect(row('plain').locator('.snip')).to_have_text('Hỏi nhóm của tôi')
        expect(row('group').locator('.rowh b')).to_have_text('Nghiên cứu');expect(row('plain').locator('.pin')).to_have_text('Pin')
        check('English list preserves bot/group names and message snippets while actions translate')
        pin=page.locator('#featured .feat[data-id="pinned"]');expect(pin.locator('.feat-name')).to_have_text('Cài đặt');expect(pin).to_have_accessible_name('Cài đặt')
        check('pinned bot text and accessible name preserve user content')
        page.evaluate('localStorage.setItem("neko-aurora-sections",JSON.stringify({list:[{id:"mine",name:"Công việc"}],of:{plain:"mine"}}))');page.evaluate('window.__AURORA.reload()')
        expect(page.locator('#rows .sec-h')).to_have_text('Công việc')
        row('plain').locator('.rowin').click(button='right');expect(page.locator('.ctx-h b')).to_have_text('Phiên bản của tôi');expect(page.locator('#ctxPin')).to_have_text('Pin')
        page.wait_for_timeout(750);page.locator('#ctxSection').click();expect(page.locator('.ctx-form')).to_be_visible();expect(page.locator('.ctx-menu .ctx-i span').filter(has_text='Công việc')).to_have_text('Công việc')
        page.keyboard.press('Escape');expect(page.locator('.ctx')).to_have_count(0)
        check('custom section headings and context menus keep names while system actions translate')
        row('plain').locator('.rowin').click();expect(page.locator('#threadName')).to_have_text('Phiên bản của tôi');expect(page.locator('.thread-empty > b')).to_have_text('Phiên bản của tôi');expect(page.locator('.thread-empty > p')).to_have_text('Công việc')
        expect(page.locator('#input')).to_have_attribute('placeholder','Ask Phiên bản của tôi')
        page.locator('#whoBtn').click();expect(page.locator('#profBody .hero h2')).to_have_text('Phiên bản của tôi');expect(page.locator('#profBody .hero p')).to_have_text('Công việc');expect(page.locator('#prChat')).to_contain_text('Conversation')
        check('thread, empty state and profile keep names/descriptions alongside translated controls')
        page.locator('#profBack').click();page.locator('#backBtn').click();row('group').locator('.rowin').click();expect(page.locator('#threadName')).to_have_text('Nghiên cứu');expect(page.locator('#threadState')).to_have_text('Phiên bản của tôi, Cài đặt')
        page.evaluate('fixtureStreams.at(-1).emit("message",{id:"m1",agentId:"group",from:"pinned",role:"agent",text:"Công việc",t:"2026-10-29T00:00:00Z"})')
        expect(page.locator('#messages .speaker')).to_have_text('Cài đặt');expect(page.locator('#messages .md')).to_contain_text('Công việc')
        check('group header and speaker labels preserve member names and model text')
        page.locator('#backBtn').click();page.evaluate('a=>fixtureStreams.at(-1).emit("agent",a)',dict(fx.bots[0],status='working'));expect(row('plain').locator('.snip')).to_have_text('Working…')
        page.evaluate('a=>fixtureStreams.at(-1).emit("agent",a)',dict(fx.bots[0],status='waiting'));expect(row('plain').locator('.snip')).to_have_text('Awaiting your approval')
        page.evaluate('a=>fixtureStreams.at(-1).emit("agent",a)',dict(fx.bots[0],desc='Cài đặt',snip='',status='idle'));expect(row('plain').locator('.snip')).to_have_text('Cài đặt')
        check('live system statuses translate and switching back to a user description remains literal')
        page.evaluate('a=>fixtureStreams.at(-1).emit("agent",a)',dict(fx.bots[1],t='2099-01-01T00:00:00Z'));expect(pin).to_have_accessible_name('Cài đặt, unread')
        check('unread pinned accessibility label localizes only its system suffix')
        page.locator('#newBtn').click();page.locator('#menuGroup').click();expect(page.locator('#groupSheet')).to_be_visible()
        picked=page.locator('#groupList .pick-row').filter(has=page.locator('b',has_text='Phiên bản của tôi'))
        expect(picked.locator('b')).to_have_text('Phiên bản của tôi');expect(picked.locator('small')).to_have_text('Cài đặt');picked.click()
        expect(page.locator('#groupChips .gchip span').last).to_have_text('Phiên bản của tôi')
        page.keyboard.press('Escape');page.wait_for_timeout(450)
        check('group picker descriptions and selected member chips retain their original content')
        page.evaluate('''()=>{const root=document.createElement('div');root.id='literal-fixture';root.dataset.noi18n='';document.body.append(root)}''')
        page.wait_for_timeout(50)
        page.evaluate('''()=>{const n=document.createElement('button');n.textContent='Công việc';n.setAttribute('aria-label','Cài đặt');document.querySelector('#literal-fixture').append(n)}''')
        expect(page.locator('#literal-fixture button')).to_have_text('Công việc');expect(page.locator('#literal-fixture button')).to_have_accessible_name('Cài đặt')
        page.locator('#literal-fixture button').evaluate('n=>{n.title="Học tập";n.firstChild.nodeValue="Nghiên cứu";n.setAttribute("aria-label","Công việc")}');page.wait_for_timeout(80)
        expect(page.locator('#literal-fixture button')).to_have_text('Nghiên cứu');expect(page.locator('#literal-fixture button')).to_have_attribute('title','Học tập');expect(page.locator('#literal-fixture button')).to_have_accessible_name('Công việc');page.locator('#literal-fixture').evaluate('n=>n.remove()')
        check('dynamically inserted descendants and changed attributes respect ancestor content boundaries')
        page.screenshot(path=str(OUT/'content-mobile-en.png'));page.set_viewport_size({'width':1280,'height':900});page.screenshot(path=str(OUT/'content-desktop-en.png'))
        with page.expect_navigation(wait_until='networkidle'):page.evaluate('window.__I18N.set("vi")')
        expect(row('plain').locator('.rowh b')).to_have_text('Phiên bản của tôi');expect(row('plain').locator('.pin')).to_have_text('Ghim');expect(pin).to_have_accessible_name('Cài đặt')
        with page.expect_navigation(wait_until='networkidle'):page.evaluate('window.__I18N.set("en")')
        expect(row('plain').locator('.rowh b')).to_have_text('Phiên bản của tôi');expect(row('plain').locator('.pin')).to_have_text('Pin')
        check('language changes and reloads preserve user content and localize actions')
        assert all(method=='GET' for _,method in fx.calls),fx.calls;assert not errors,errors
        (OUT/'content-browser-result.json').write_bytes(json.dumps(dict(result='PASS',scenarios=len(checks),browser=browser.version,errors=errors)).encode())
    finally:browser.close()
finally:server.shutdown();server.server_close()
