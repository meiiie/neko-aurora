"""Memory panel ownership and mutation feedback in real Chrome with loopback fixtures."""
import functools,hashlib,http.server,json,os,threading,time
from pathlib import Path
from urllib.parse import urlsplit,parse_qs
from playwright.sync_api import expect,sync_playwright
ROOT=Path(__file__).resolve().parents[3]
OUT=ROOT/'.audit/2026-10-29-memory-revisions';OUT.mkdir(exist_ok=True)
class Server(http.server.ThreadingHTTPServer):request_queue_size=64
class Handler(http.server.SimpleHTTPRequestHandler):
    def log_message(self,*_):pass
class Fixture:
    def __init__(self):
        self.scope='memory-owner-a';self.text='Initial fact';self.hold=None;self.pending=[];self.calls=[];self.fail=None;self.fail_after_save=False;self.queries=[];self.bad_response=False;self.lose_save=False;self.team=[]
    def revision(self):return hashlib.sha256(self.text.encode()).hexdigest()
    def route(self,r):
        req=r.request;path=urlsplit(req.url).path.removeprefix('/app/api');data={};kind=None;status=200
        if path=='/state':data=dict(workspaceId=self.scope,agents=[dict(id='bot-a',name='Chat A',status='idle',av=dict(shape=0,color=4)),dict(id='bot-b',name='Chat B',status='idle',av=dict(shape=1,color=2))],pending=[],ai={},secrets=[],outbound={})
        elif path.endswith('/messages'):data=[]
        elif path.endswith('/computer'):data=dict(available=False,log=[],files=[])
        elif path=='/schedules':data={'schedules':[]}
        elif '/memory' in path:
            body=req.post_data_json if req.method=='POST' else None
            self.calls.append((path,req.method,body));query=parse_qs(urlsplit(req.url).query);self.queries.append((path,req.method,query))
            if req.method=='GET':kind='read';data={'items':[{'text':self.text,'at':'2026-10-28T04:00:00Z'}],'team':self.team,'workspaceId':self.scope if not self.bad_response else 'wrong-response-scope','revision':self.revision(),'teamRevision':'b'*64,'journal':[{'op':'edit','after':self.text,'before':'Phiên bản trước','by':'owner','at':'2026-10-28T04:00:00Z'}]}
            elif path.endswith('/ask'):kind='ask';data={'answer':'Phiên bản trả lời cũ'}
            elif path.endswith('/summarize'):
                kind='apply' if body.get('apply') else 'summary';data={'proposal':['Phiên bản đề xuất'],'before':2,'revision':self.revision()}
                if kind=='apply' and query.get('revision')!=[self.revision()]:status=409
            elif path.endswith('/import/preview'):kind='preview';data={'items':['Phiên bản nhập'],'remaining':29,'duplicates':0,'rejected':0,'revision':'fixture-rev'}
            elif path.endswith('/import'):kind='import';data={'added':1}
            elif req.method=='DELETE':kind='delete';data={'ok':True}
            else:
                kind='save';data={'ok':True}
                expected='b'*64 if body.get('team') else self.revision()
                if self.fail!='save' and query.get('revision')==[expected]:
                    if body.get('team'):self.team=[{'text':body['text'],'at':'2026-10-28T04:00:00Z'}]
                    else:self.text=body['text']
                elif self.fail!='save':status=409
                if self.fail_after_save:self.fail='read'
            if self.fail==kind:status=500
            if query.get('workspaceId')!=[self.scope]:status=409
        if kind=='save' and self.lose_save and status==200:self.lose_save=False;r.abort('failed');return
        if kind and self.hold==kind:self.pending.append((r,status,json.dumps(data)));return
        r.fulfill(status=status,content_type='application/json',body=json.dumps(data))

checks=[]
def check(s):checks.append(s);print('PASS '+s,flush=True)
server=Server(('127.0.0.1',0),functools.partial(Handler,directory=str(ROOT/'app/native/internal/web/assets')))
threading.Thread(target=server.serve_forever,daemon=True).start()
try:
    with sync_playwright() as p:
        chrome=os.environ.get('AURORA_TEST_CHROME')
        if not chrome and Path('C:/Program Files/Google/Chrome/Application/chrome.exe').exists():chrome='C:/Program Files/Google/Chrome/Application/chrome.exe'
        browser=p.chromium.launch(headless=True,executable_path=chrome)
        try:
            ctx=browser.new_context(viewport={'width':390,'height':844},locale='vi-VN',service_workers='block',reduced_motion='reduce')
            ctx.add_init_script('window.EventSource=class{addEventListener(){}close(){}}')
            page=ctx.new_page();fx=Fixture();errors=[];page.on('pageerror',lambda e:errors.append(str(e)));page.route('**/app/api/**',fx.route)
            page.goto(f'http://127.0.0.1:{server.server_port}/app/#bot-a',wait_until='networkidle')
            page.locator('#whoBtn').click();expect(page.locator('#prMem .cell-v')).to_contain_text('1')
            body=page.locator('#memBody');fact=body.locator('.mem-t').first;draft=body.locator('.mem-edit')
            def open_mem():page.locator('#prMem').click()
            def close_mem():page.keyboard.press('Escape');page.wait_for_timeout(450)
            def pending():
                end=time.monotonic()+3
                while not fx.pending and time.monotonic()<end:page.wait_for_timeout(20)
                assert fx.pending,'fixture request missing';return fx.pending.pop(0)
            def release(held,status=None):held[0].fulfill(status=held[1] if status is None else status,content_type='application/json',body=held[2])
            def edit(text):fact.click();draft.fill(text)
            def save():body.locator('.mem-it.editing').get_by_role('button',name='Lưu',exact=True).click()
            fx.hold='read';open_mem();held=pending();close_mem();fx.hold=None;fx.text='Newest fact';open_mem();expect(fact).to_have_text('Newest fact');release(held);page.wait_for_timeout(100);expect(fact).to_have_text('Newest fact')
            expect(body.locator('#memAsk')).to_have_count(1);expect(body.locator('#memImport')).to_have_count(1)
            check('older reads cannot overwrite a reopened memory panel or duplicate its tools')
            close_mem();fx.hold='read';open_mem();held=pending();close_mem();fx.hold=None;open_mem();expect(fact).to_have_text('Newest fact');release(held,500);page.wait_for_timeout(100);expect(fact).to_have_text('Newest fact')
            check('a late read failure cannot clear a later successful panel')
            edit('Hidden draft');close_mem();expect(body).to_have_text('');expect(body.locator('input,textarea')).to_have_count(0)
            fx.hold='read';open_mem();held=pending();close_mem();release(held);fx.hold=None;page.wait_for_timeout(100);expect(body).to_have_text('')
            check('closing clears hidden memory content and revokes pending reads')
            open_mem();edit('Accepted earlier');fx.hold='save';save();held=pending()
            expect(body.locator('.mem-it.editing').get_by_role('button',name='Lưu',exact=True)).to_be_disabled()
            before=len(fx.calls);body.locator('.mem-it.editing').get_by_role('button',name='Lưu',exact=True).evaluate('n=>n.click()');assert len(fx.calls)==before
            close_mem();fx.hold=None;open_mem();edit('New draft stays');release(held);page.wait_for_timeout(100);expect(draft).to_have_value('New draft stays')
            check('save is single-flight and an old acknowledgement cannot erase a newer draft')
            fx.fail='save';save();expect(draft).to_have_value('New draft stays');expect(body.locator('.mem-it.editing').get_by_role('button',name='Lưu',exact=True)).to_be_enabled();expect(body.locator('.mem-it.editing .mem-action-status')).to_contain_text('Chưa nhận được xác nhận')
            check('failed edit retains its text and provides inline acknowledgement feedback')
            fx.fail=None;fx.fail_after_save=True;save();expect(body.locator('.memory-load-status')).to_contain_text('Chưa tải được bộ nhớ');expect(body.locator('.mem-it.editing').get_by_role('button',name='Lưu',exact=True)).to_be_disabled()
            before=len([c for c in fx.calls if c[1]=='POST']);fx.fail_after_save=False;fx.fail=None;body.get_by_role('button',name='Tải lại bộ nhớ',exact=True).click();expect(fact).to_have_text('New draft stays');assert len([c for c in fx.calls if c[1]=='POST'])==before
            check('accepted edit with failed reload retries only the read and cannot replay the save')
            edit('Delete old');delete=body.locator('.mem-it.editing .danger');delete.click();fx.hold='delete';delete.click();held=pending();close_mem();fx.hold=None;open_mem();edit('After delete draft');release(held);page.wait_for_timeout(100);expect(draft).to_have_value('After delete draft')
            check('a late delete acknowledgement cannot redraw a newer edit')
            fx.hold='save';save();held=pending();close_mem();fx.hold=None
            page.locator('#profBack').click();page.locator('#backBtn').click();page.locator('#rows .row[data-id="bot-b"] .rowin').click();page.locator('#whoBtn').click();expect(page.locator('#prMem .cell-v')).to_contain_text('1');open_mem();edit('Bot B draft');release(held);page.wait_for_timeout(100);expect(draft).to_have_value('Bot B draft');expect(page.locator('#memTitle')).to_contain_text('Chat B')
            assert [c for c in fx.calls if c[1]=='POST' and c[0].endswith('/memory/0')][-1][0]=='/agents/bot-a/memory/0'
            check('a pending edit remains owned by its original bot and cannot reload another bot panel')
            close_mem();open_mem();body.locator('#memAsk').click();question=body.locator('.mem-q');question.locator('input').fill('What do you know?');fx.hold='ask';question.get_by_role('button',name='Hỏi',exact=True).click();held=pending();close_mem();fx.hold=None;open_mem();release(held);page.wait_for_timeout(100);expect(body.locator('.mem-ans')).to_have_count(0)
            fx.hold='summary';body.locator('#memSum').click();held=pending();close_mem();fx.hold=None;open_mem();release(held);page.wait_for_timeout(100);expect(body.locator('.mem-prop')).to_have_count(0)
            check('late ask and summarize results remain bound to the view that requested them')
            body.locator('#memImport').click();form=body.locator('.memory-import');form.locator('textarea').fill('Import old');fx.hold='preview';form.get_by_role('button',name='Xem trước',exact=True).click();held=pending();close_mem();fx.hold=None;open_mem();release(held);page.wait_for_timeout(100);expect(body.locator('.memory-import-pick')).to_have_count(0)
            check('closing a pending import removes its input and ignores the preview result')
            body.locator('#memImport').click();form.locator('textarea').fill('Import accepted');form.get_by_role('button',name='Xem trước',exact=True).click();expect(form.locator('.memory-import-pick')).to_have_count(1)
            fx.hold='import';form.get_by_role('button',name='Lưu các mục đã chọn',exact=True).click();held=pending();close_mem();fx.hold=None;open_mem();edit('Keep draft after import');release(held);page.wait_for_timeout(100);expect(draft).to_have_value('Keep draft after import')
            check('an old import acknowledgement cannot reload a newer memory editor')
            fx.hold='read';close_mem();open_mem();held=pending();fx.hold=None;fx.scope='memory-owner-b';fx.text='Other workspace fact';page.evaluate('()=>{void window.__AURORA.reload()}');page.wait_for_function('window.__AURORA.state().workspaceId==="memory-owner-b"');expect(body).to_have_text('');page.wait_for_timeout(450)
            page.locator('#rows .row[data-id="bot-a"] .rowin').click();page.locator('#whoBtn').click();expect(page.locator('#prMem .cell-v')).to_contain_text('1');open_mem();expect(fact).to_have_text('Other workspace fact');release(held);page.wait_for_timeout(100);expect(fact).to_have_text('Other workspace fact')
            check('an old workspace response cannot repopulate memory under a reused bot ID')
            close_mem();fx.fail='read';open_mem();expect(body.locator('.memory-load-status')).to_contain_text('Chưa tải được bộ nhớ');page.screenshot(path=str(OUT/'memory-error-mobile.png'));fx.fail=None;body.get_by_role('button',name='Tải lại bộ nhớ',exact=True).focus();page.keyboard.press('Enter');expect(fact).to_have_text('Other workspace fact')
            fact.locator('..').focus();page.keyboard.press('Enter');expect(draft).to_be_visible();draft.fill('Keyboard edit');save();expect(fact).to_have_text('Keyboard edit')
            check('initial load retry and row editing work from the keyboard')
            switched=next(i for i,x in enumerate(fx.queries) if x[2].get('workspaceId')==['memory-owner-b'])
            assert all(q.get('workspaceId')==['memory-owner-a' if n<switched else 'memory-owner-b'] for n,(_,_,q) in enumerate(fx.queries))
            check('memory reads, asks, imports and mutations carry their originating workspace')
            edit('Keep conflicted draft');fx.text='Remote fact changed';save();expect(draft).to_have_value('Keep conflicted draft');expect(body.locator('.mem-it.editing .mem-action-status')).to_contain_text('Bộ nhớ đã thay đổi')
            before=len([c for c in fx.calls if c[1]!='GET']);body.get_by_role('button',name='Tải lại bộ nhớ',exact=True).click();expect(fact).to_have_text('Remote fact changed');expect(body.locator('.memory-recovery-draft textarea')).to_have_value('Keep conflicted draft');assert len([c for c in fx.calls if c[1]!='GET'])==before
            check('stale edit keeps its draft and reloads the server without replaying the mutation')
            page.screenshot(path=str(OUT/'memory-conflict-recovered-mobile.png'))
            close_mem();open_mem();body.locator('#memSum').click();expect(body.locator('.mem-prop')).to_be_visible();proposal_rev=fx.revision();summaries=len([c for c in fx.calls if c[0].endswith('/summarize') and c[2]=={}]);fx.text='Changed after proposal';body.locator('.mem-prop').get_by_role('button',name='Áp dụng',exact=True).click()
            expect(body.locator('.mem-prop .mem-action-status')).to_contain_text('Bộ nhớ đã thay đổi');assert fx.queries[-1][2]['revision']==[proposal_rev];assert len([c for c in fx.calls if c[0].endswith('/summarize') and c[2]=={}])==summaries
            check('summary apply carries the revision returned with its proposal')
            close_mem();fx.bad_response=True;open_mem();expect(body.locator('.memory-load-status')).to_be_visible();expect(body.locator('.mem-t')).to_have_count(0);fx.bad_response=False;body.get_by_role('button',name='Tải lại bộ nhớ',exact=True).click();expect(fact).to_have_text('Changed after proposal')
            check('a mismatched workspace response is rejected before memory renders')
            edit('Accepted but reply lost');before=len([c for c in fx.calls if c[1]=='POST']);fx.lose_save=True;save();expect(body.locator('.mem-it.editing .mem-action-status')).to_contain_text('Chưa nhận được xác nhận');expect(draft).to_have_value('Accepted but reply lost')
            body.get_by_role('button',name='Tải lại bộ nhớ',exact=True).click();expect(fact).to_have_text('Accepted but reply lost');expect(body.locator('.memory-recovery-draft textarea')).to_have_value('Accepted but reply lost');assert len([c for c in fx.calls if c[1]=='POST'])==before+1
            check('an accepted edit with a lost reply reloads once without replay and retains comparison text')
            close_mem();fx.team=[{'text':'Shared fact','at':'2026-10-28T04:00:00Z'}];open_mem();body.locator('.mem-t').last.click();draft.fill('Shared edit');save();expect(body.locator('.mem-t').last).to_have_text('Shared edit')
            mutation=next(x for x in reversed(fx.queries) if x[1]=='POST');assert mutation[2]['revision']==['b'*64];assert [x for x in fx.calls if x[1]=='POST'][-1][2]['team'] is True
            body.locator('.mem-t').last.click();delete=body.locator('.mem-it.editing .danger');delete.click();delete.click();expect(body.locator('.mem-it.editing')).to_have_count(0);mutation=next(x for x in reversed(fx.queries) if x[1]=='DELETE');assert mutation[2]['scope']==['team'] and mutation[2]['revision']==['b'*64]
            check('shared-memory edit and delete use the team revision and team scope')
            body.locator('#memInput').fill('Shared addition');body.locator('#memTeam').check();body.locator('.mem-add').first.get_by_role('button',name='Thêm',exact=True).click();expect(body.locator('.mem-t').last).to_have_text('Shared addition');mutation=next(x for x in reversed(fx.queries) if x[1]=='POST');assert mutation[2]['revision']==['b'*64]
            check('shared-memory add carries the team revision selected by the owner')
            fx.team=[]
            fx.text='Phiên bản giữ nguyên';close_mem()
            with page.expect_navigation(wait_until='networkidle'):page.evaluate('window.__I18N.set("en")')
            page.locator('#whoBtn').click();expect(page.locator('#prMem .cell-v')).to_contain_text('1');open_mem();expect(fact).to_have_text('Phiên bản giữ nguyên');body.locator('.mem-log summary').click();expect(body.locator('.mem-log li span')).to_have_text('Phiên bản giữ nguyên');expect(body.locator('.mem-log s')).to_have_text('Phiên bản trước')
            page.set_viewport_size({'width':1280,'height':900});page.screenshot(path=str(OUT/'memory-desktop-en.png'));assert not errors,errors
            check('English UI preserves user memory and both versions in the change history')
            (OUT/'memory-browser-result.json').write_bytes(json.dumps({'result':'PASS','scenarios':len(checks),'browser':browser.version,'errors':errors}).encode())
        finally:browser.close()
finally:server.shutdown();server.server_close()
