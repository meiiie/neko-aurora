"""Schedule load errors, reconnection and view/request ownership in real Chrome."""
import functools,http.server,json,os,threading,time
from pathlib import Path
from urllib.parse import urlsplit
from playwright.sync_api import expect,sync_playwright
ROOT=Path(__file__).resolve().parents[3]
OUT=ROOT/'.audit/2026-10-29-schedule-loading';OUT.mkdir(exist_ok=True)
class Server(http.server.ThreadingHTTPServer):request_queue_size=64
class Handler(http.server.SimpleHTTPRequestHandler):
    def log_message(self,*_):pass
class Fixture:
    def __init__(self):
        self.scope='schedule-owner-a';self.fail=False;self.hold=False;self.pending=[];self.posts=[];self.gets=0;self.hold_post=False;self.fail_after_post=False;self.fail_messages=False;self.hold_messages=False
        self.item=dict(id='schedule-fixture',bot='bot-a',text='Initial schedule',at='08:00',repeat='daily',paused=False,revision=1)
    def route(self,r):
        path=urlsplit(r.request.url).path.removeprefix('/app/api');data={};status=200;hold=False
        if path=='/state':data=dict(workspaceId=self.scope,agents=[dict(id='bot-a',name='Chat A',status='idle',av=dict(shape=0,color=4))],pending=[],ai={},secrets=[],outbound={})
        elif path.endswith('/messages'):data=[];status=500 if self.fail_messages else 200;hold=self.hold_messages
        elif path.endswith('/computer'):data={'available':False,'log':[],'files':[]}
        elif path=='/schedules':
            self.gets+=1;data={'schedules':[dict(self.item)]} if self.item else {'schedules':[]};status=500 if self.fail else 200;hold=self.hold
        elif path.startswith('/schedules/') and r.request.method=='POST':
            self.posts.append((path,r.request.post_data_json));self.item['paused']=r.request.post_data_json.get('paused',False);self.item['revision']+=1;hold=self.hold_post
            if self.fail_after_post:self.fail=True
        if hold:self.pending.append((r,status,json.dumps(data)));return
        r.fulfill(status=status,content_type='application/json',body=json.dumps(data))
def check(text):print('PASS '+text,flush=True)
server=Server(('127.0.0.1',0),functools.partial(Handler,directory=str(ROOT/'app/native/internal/web/assets')))
threading.Thread(target=server.serve_forever,daemon=True).start()
try:
    with sync_playwright() as p:
        chrome=os.environ.get('AURORA_TEST_CHROME')
        if not chrome and Path('C:/Program Files/Google/Chrome/Application/chrome.exe').exists():chrome='C:/Program Files/Google/Chrome/Application/chrome.exe'
        browser=p.chromium.launch(headless=True,executable_path=chrome)
        try:
            ctx=browser.new_context(viewport={'width':390,'height':844},locale='vi-VN',service_workers='block',reduced_motion='reduce')
            ctx.add_init_script('window.fixtureStreams=[];window.EventSource=class{constructor(){this.readyState=1;fixtureStreams.push(this)}addEventListener(){}close(){this.readyState=2}}')
            page=ctx.new_page();fx=Fixture();errors=[];page.on('pageerror',lambda e:errors.append(str(e)));page.route('**/app/api/**',fx.route)
            page.goto(f'http://127.0.0.1:{server.server_port}/app/#bot-a',wait_until='networkidle')
            def refresh():page.evaluate('()=>{void window.__AURORA.reload().catch(()=>{})}')
            def pending():
                end=time.monotonic()+3
                while not fx.pending and time.monotonic()<end:page.wait_for_timeout(20)
                assert fx.pending,'fixture request missing';return fx.pending.pop(0)
            def release(held):held[0].fulfill(status=held[1],content_type='application/json',body=held[2])
            def reopen():
                page.locator('#profBack').click();page.wait_for_timeout(120);page.locator('#whoBtn').click()
            box=page.locator('#schedBox');notice=box.locator('.schedule-load-status');retry=notice.get_by_role('button',name='Tải lại lịch',exact=True)
            fx.fail=True;page.locator('#whoBtn').click();expect(notice).to_contain_text('Chưa tải được lịch hẹn');expect(retry).to_be_enabled()
            box.scroll_into_view_if_needed();page.screenshot(path=str(OUT/'load-error-mobile.png'));fx.fail=False;retry.focus();page.keyboard.press('Enter');expect(box).to_contain_text('Initial schedule');expect(box).to_be_focused()
            check('initial load failure stays inside the schedule section and supports explicit retry')
            fx.item.update(text='After reconnect',paused=True,revision=2)
            page.evaluate('()=>{const stream=fixtureStreams.at(-1);stream.onerror();stream.onopen()}');expect(box).to_contain_text('After reconnect');expect(box.get_by_role('button',name='Tiếp tục',exact=True)).to_be_enabled()
            check('reconnecting refreshes the open schedule list without navigating or reloading the page')
            fx.fail=True;refresh();expect(notice).to_contain_text('Chưa tải được lịch hẹn');expect(box).to_contain_text('After reconnect')
            fx.fail=False;fx.item.update(text='Retry content',revision=3);retry.click();expect(box).to_contain_text('Retry content')
            check('refresh failure retains the last schedule list until a successful retry replaces it')
            fx.hold=True;refresh();held=pending();fx.hold=False;fx.item.update(text='Newest list',revision=4);refresh();expect(box).to_contain_text('Newest list');release(held);page.wait_for_timeout(100);expect(box).to_contain_text('Newest list')
            check('a late older successful list cannot overwrite the latest response')
            fx.hold=True;fx.fail=True;refresh();held=pending();fx.hold=False;fx.fail=False;refresh();expect(notice).to_have_count(0);release(held);page.wait_for_timeout(100);expect(notice).to_have_count(0)
            check('an older failed list cannot add an error after a newer success')
            fx.hold=True;reopen();held=pending();fx.hold=False;fx.item.update(text='Reopened profile',revision=5);reopen();expect(box).to_contain_text('Reopened profile');release(held);page.wait_for_timeout(100);expect(box).to_contain_text('Reopened profile')
            check('closing and reopening the same bot revokes the earlier profile request')
            box.get_by_role('button',name='Sửa lịch',exact=True).click();draft=box.get_by_label('Nội dung lịch');draft.fill('Owner draft stays here')
            fx.item.update(text='Changed elsewhere',revision=6);refresh();page.wait_for_timeout(150);expect(draft).to_have_value('Owner draft stays here');expect(draft).to_be_focused()
            box.get_by_role('button',name='Hủy chỉnh sửa').click();expect(box).to_contain_text('Changed elsewhere')
            check('automatic refresh preserves an active edit and cancel then loads the latest list')
            fx.hold=True;refresh();held=pending();box.get_by_role('button',name='Sửa lịch',exact=True).click();draft.fill('Draft opened during load');release(held);page.wait_for_timeout(100);expect(draft).to_have_value('Draft opened during load')
            fx.hold=False;box.get_by_role('button',name='Hủy chỉnh sửa').click();expect(box.locator('.schedule-editor')).to_have_count(0)
            check('an edit opened while a background read is pending is not discarded by its result')
            fx.hold_post=True;box.get_by_role('button',name='Tiếp tục',exact=True).click();held=pending();reopen();box.get_by_role('button',name='Sửa lịch',exact=True).click();draft.fill('New view draft');release(held);page.wait_for_timeout(100);expect(draft).to_have_value('New view draft')
            fx.hold_post=False;box.get_by_role('button',name='Hủy chỉnh sửa').click()
            check('a late mutation acknowledgement cannot reload a newer profile over its draft')
            before=len(fx.posts);fx.fail_after_post=True;box.get_by_role('button',name='Tạm dừng',exact=True).click();expect(notice).to_contain_text('Chưa tải được lịch hẹn')
            expect(box.get_by_role('button',name='Tạm dừng',exact=True)).to_be_disabled();fx.fail_after_post=False;fx.fail=False;retry.click();expect(box.get_by_role('button',name='Tiếp tục',exact=True)).to_be_enabled();assert len(fx.posts)==before+1
            check('accepted pause with failed list reload is resolved by GET retry without replaying the mutation')
            fx.hold=True;refresh();held=pending();fx.hold=False;fx.scope='schedule-owner-b';fx.item.update(text='Workspace B schedule',revision=1);refresh();page.wait_for_function('window.__AURORA.state().workspaceId==="schedule-owner-b"')
            page.locator('#rows .row[data-id="bot-a"] .rowin').click();page.locator('#whoBtn').click();expect(box).to_contain_text('Workspace B schedule');release(held);page.wait_for_timeout(100);expect(box).to_contain_text('Workspace B schedule')
            check('a pending list from another workspace cannot replace the current profile')
            fx.item=None;refresh();expect(box).to_contain_text('Chưa có lịch hẹn');expect(box.locator('.schedule-row')).to_have_count(0)
            page.set_viewport_size({'width':1280,'height':900});box.scroll_into_view_if_needed();page.screenshot(path=str(OUT/'empty-desktop.png'))
            check('a successful empty list is distinct from loading and failure')
            fx.item=dict(id='schedule-fixture',bot='bot-a',text='History failure does not block schedules',at='08:00',repeat='daily',paused=False,revision=2)
            fx.fail_messages=True;refresh();expect(box).to_contain_text('History failure does not block schedules');fx.fail_messages=False
            check('chat history failure does not block independent schedule recovery')
            fx.hold_messages=True;page.reload(wait_until='domcontentloaded');expect(page.locator('#whoBtn')).to_contain_text('Chat A');first=pending();page.locator('#whoBtn').click();second=pending()
            box.get_by_role('button',name='Sửa lịch',exact=True).click();draft.fill('Draft while profile history loads');fx.hold_messages=False;release(first);release(second);page.wait_for_timeout(150);expect(draft).to_have_value('Draft while profile history loads')
            check('late profile history loads cannot discard an active schedule draft')
            assert not errors,errors
            (OUT/'schedule-loading-result.json').write_text(json.dumps({'result':'PASS','scenarios':14,'browser':browser.version,'errors':errors}),encoding='utf-8')
        finally:browser.close()
finally:server.shutdown();server.server_close()
