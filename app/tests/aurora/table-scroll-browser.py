"""Wide table scroll controls against the real UI with loopback fixtures."""
import functools,http.server,json,os,threading
from pathlib import Path
from urllib.parse import urlsplit
from playwright.sync_api import expect,sync_playwright

ROOT=Path(__file__).resolve().parents[3]
OUT=ROOT/'.audit/2026-10-28-table-scroll';OUT.mkdir(exist_ok=True)
WIDE='| Cột A | Cột B | Cột C |\n|---|---|---|\n| Sao chép bảng | '+('nội dung tiếng Việt '*14)+' | 👩🏽‍💻 |'
class Server(http.server.ThreadingHTTPServer):request_queue_size=64
class Handler(http.server.SimpleHTTPRequestHandler):
    def log_message(self,*_):pass
def route(r):
    path=urlsplit(r.request.url).path.removeprefix('/app/api');data={}
    assert r.request.method=='GET'
    if path=='/state':data={'workspaceId':'scroll-owner','agents':[{'id':'bot-a','name':'Tables','status':'idle','av':{}}],'pending':[],'ai':{},'secrets':[],'outbound':{}}
    elif path=='/events':r.fulfill(status=200,content_type='text/event-stream',body='');return
    elif path.endswith('/messages'):
        data=[{'id':key,'agentId':'bot-a','role':'agent','text':value,'t':'2026-10-28T04:00:00Z'} for key,value in [('wide',WIDE),('narrow','| A | B |\n|---|---|\n| 1 | 2 |'),('resize','| Mã |\n|---|\n| '+'12345678'*5+' |')]]
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
            context=browser.new_context(viewport={'width':1280,'height':900},locale='vi-VN',service_workers='block',reduced_motion='reduce',permissions=['clipboard-read','clipboard-write'])
            page=context.new_page();errors=[];page.on('pageerror',lambda e:errors.append(str(e)));page.route('**/app/api/**',route)
            page.goto(f'http://127.0.0.1:{server.server_port}/app/#bot-a',wait_until='networkidle')
            wide=page.locator('[data-message-id="wide"] .table-card');wide.hover()
            right=wide.get_by_role('button',name='Cuộn bảng sang phải',exact=True)
            left=wide.get_by_role('button',name='Cuộn bảng sang trái',exact=True)
            expect(right).to_be_visible();expect(left).to_be_visible()
            expect(right).to_have_css('opacity','1')
            expect(left).to_have_attribute('aria-disabled','true');expect(right).to_have_attribute('aria-disabled','false')
            wrap=wide.locator('.tbl')
            right.click();page.wait_for_function('document.querySelector("[data-message-id=wide] .tbl").scrollLeft>0')
            expect(left).to_have_attribute('aria-disabled','false')
            check('wide table exposes directional controls on hover and scrolls by pointer activation')
            assert right.get_attribute('aria-controls')==wrap.get_attribute('id')
            wrap.evaluate('n=>{n.scrollLeft=n.scrollWidth}')
            expect(right).to_have_attribute('aria-disabled','true')
            end=wrap.evaluate('n=>n.scrollLeft');scroll=page.locator('#messages').evaluate('n=>n.scrollTop')
            right.focus();page.keyboard.press('Enter');expect(right).to_be_focused()
            assert wrap.evaluate('n=>n.scrollLeft')==end
            left.focus();page.keyboard.press('Enter')
            page.wait_for_function('(end)=>document.querySelector("[data-message-id=wide] .tbl").scrollLeft<end',arg=end)
            expect(left).to_be_focused();assert page.locator('#messages').evaluate('n=>n.scrollTop')==scroll
            check('edge controls stay focused, become no-ops at the boundary and scroll back by keyboard without moving the chat')
            page.locator('#input').focus();page.mouse.move(0,0);expect(right).to_have_css('opacity','0')
            right.focus();expect(right).to_have_css('opacity','1')
            check('controls hide outside hover/focus and reveal when reached by keyboard focus')
            narrow=page.locator('[data-message-id="narrow"] .table-card');narrow.hover()
            expect(narrow.locator('.table-scroll')).to_have_count(2)
            for b in narrow.locator('.table-scroll').all():expect(b).to_be_hidden()
            resize=page.locator('[data-message-id="resize"] .table-card')
            for b in resize.locator('.table-scroll').all():expect(b).to_be_hidden()
            page.set_viewport_size({'width':390,'height':844})
            expect(resize.locator('.table-scroll-next')).to_be_visible()
            page.set_viewport_size({'width':1280,'height':900})
            for b in resize.locator('.table-scroll').all():expect(b).to_be_hidden()
            check('fitting tables hide controls and viewport resize updates overflow state without a hover trigger')
            wide.get_by_role('button',name='Sao chép Markdown',exact=True).click()
            page.wait_for_function('text=>navigator.clipboard.readText().then(x=>x===text)',arg=WIDE)
            wide.get_by_role('button',name='Sao chép bảng',exact=True).click()
            expected='Cột A\tCột B\tCột C\nSao chép bảng\t'+('nội dung tiếng Việt '*14).strip()+'\t👩🏽‍💻'
            page.wait_for_function('text=>navigator.clipboard.readText().then(x=>x===text)',arg=expected)
            check('scroll UI leaves exact Markdown and TSV clipboard payloads unchanged')
            with page.expect_navigation(wait_until='networkidle'):page.evaluate('window.__I18N.set("en")')
            wide.hover();expect(wide.get_by_role('button',name='Scroll table right',exact=True)).to_have_css('opacity','1')
            expect(wide.locator('tbody td').first).to_have_text('Sao chép bảng')
            wide.get_by_role('button',name='Copy Markdown',exact=True).click()
            page.wait_for_function('text=>navigator.clipboard.readText().then(x=>x===text)',arg=WIDE)
            expect(wide.locator('.table-status')).to_have_text('Copied')
            page.evaluate('()=>{window.fixtureClipboardWrite=navigator.clipboard.writeText;navigator.clipboard.writeText=()=>Promise.reject(new DOMException("blocked","NotAllowedError"))}')
            wide.get_by_role('button',name='Copy table',exact=True).click()
            expect(wide.locator('.table-status')).to_have_text('Could not copy. Select text in the table.')
            page.wait_for_function('text=>navigator.clipboard.readText().then(x=>x===text)',arg=WIDE)
            page.evaluate('()=>{navigator.clipboard.writeText=window.fixtureClipboardWrite}')
            wide.get_by_role('button',name='Copy Markdown',exact=True).click();expect(wide.locator('.table-status')).to_have_text('Copied')
            check('clipboard refusal reports a translated failure without replacing the existing clipboard')
            wide.get_by_role('button',name='Expand table',exact=True).click()
            dialog=page.get_by_role('dialog',name='Expanded table',exact=True);expect(dialog).to_be_visible()
            expect(dialog.get_by_role('button',name='Close table',exact=True)).to_be_focused()
            expect(dialog.locator('tbody td').first).to_have_text('Sao chép bảng')
            dialog.get_by_role('button',name='Zoom table',exact=True).click();page.keyboard.press('Escape')
            expect(wide.get_by_role('button',name='Expand table',exact=True)).to_be_focused()
            check('English table controls and viewer labels translate while Vietnamese cells and copied source remain intact')
            wide.hover();page.screenshot(path=str(OUT/'table-scroll-desktop.png'))
            touch=browser.new_context(viewport={'width':390,'height':844},locale='vi-VN',is_mobile=True,has_touch=True,service_workers='block',reduced_motion='reduce')
            mobile=touch.new_page();mobile.route('**/app/api/**',route);mobile.on('pageerror',lambda e:errors.append(str(e)))
            mobile.goto(f'http://127.0.0.1:{server.server_port}/app/#bot-a',wait_until='networkidle')
            control=mobile.locator('[data-message-id="wide"] .table-scroll-next');control.scroll_into_view_if_needed()
            assert mobile.evaluate('matchMedia("(hover:none)").matches')
            expect(control).to_have_css('opacity','1');control.tap()
            mobile.wait_for_function('document.querySelector("[data-message-id=wide] .tbl").scrollLeft>0')
            assert mobile.locator('#app').evaluate('n=>n.scrollLeft')==0
            mobile.locator('#messages').evaluate('n=>{n.scrollTop=0}')
            control_box=control.bounding_box();table_box=mobile.locator('[data-message-id="wide"] .tbl').bounding_box()
            assert control_box['y']+control_box['height']<=table_box['y']
            mobile.screenshot(path=str(OUT/'table-scroll-touch.png'));touch.close()
            check('touch emulation exposes controls without hover and a tap scrolls only the intended table')
            assert not errors,errors
            (OUT/'table-scroll-result.json').write_text(json.dumps({'result':'PASS','scenarios':8,'browser':browser.version,'errors':errors}),encoding='utf-8')
        finally:browser.close()
finally:server.shutdown();server.server_close()
