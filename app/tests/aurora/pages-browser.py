"""Owner Pages UI in real Chrome; API replies are local deterministic fixtures."""
import functools, hashlib, http.server, json, os, threading, time
from pathlib import Path
from urllib.parse import parse_qs, urlsplit
from playwright.sync_api import expect, sync_playwright

ROOT = Path(__file__).resolve().parents[3]
OUT = Path(os.environ.get('AURORA_BROWSER_AUDIT_DIR', ROOT/'.audit/pages-browser'))
OUT.mkdir(parents=True, exist_ok=True)
class Server(http.server.ThreadingHTTPServer): request_queue_size = 64
class Handler(http.server.SimpleHTTPRequestHandler):
    def log_message(self, *_): pass
class Fixture:
    def __init__(self):
        self.scope = 'pages-owner-a'; self.pages = {}; self.receipts = {}; self.posts = []; self.pending = []
        self.fail_list = False; self.lose_ack = False; self.drop_save = False; self.hold_save = False; self.bad_export = False
    def save(self, payload):
        nonce = payload['clientNonce']
        if nonce in self.receipts:
            old, page = self.receipts[nonce]
            return (200, page) if old == payload else (409, None)
        ident = payload['id'] or 'page-'+format(len(self.pages)+1, '032x')
        history = self.pages.get(ident, [])
        if len(history) != payload['baseRevision']: return 409, None
        page = dict(id=ident, revision=len(history)+1, title=payload['title'], text=payload['text'], archived=payload['archived'], sha256=hashlib.sha256(payload['text'].encode()).hexdigest(), savedAt='2026-10-29T10:00:00Z')
        self.pages.setdefault(ident, []).append(page); self.receipts[nonce] = (dict(payload), page)
        return 200, page
    def route(self, r):
        u = urlsplit(r.request.url); path = u.path.removeprefix('/app/api'); q = parse_qs(u.query)
        data = {}; status = 200; headers = {}
        if path == '/state': data = dict(workspaceId=self.scope, agents=[], pending=[], ai={}, secrets=[], outbound={})
        elif path == '/pages/save':
            payload = r.request.post_data_json; self.posts.append(payload)
            if payload['workspaceId'] != self.scope: status = 409
            elif self.drop_save: r.abort(); return
            else: status, page = self.save(payload); data = dict(workspaceId=self.scope, page=page)
            if self.lose_ack: r.abort(); return
            if self.hold_save: self.pending.append((r, status, data)); return
        elif path == '/pages' or path.startswith(('/pages/', '/page-receipts/')):
            if q.get('workspaceId') != [self.scope]: status = 409
            elif path == '/pages':
                status = 503 if self.fail_list else 200
                data = dict(workspaceId=self.scope, pages=[{k:v for k,v in h[-1].items() if k!='text'} for h in self.pages.values()], maxPages=64, maxTextBytes=65536)
            elif path.startswith('/page-receipts/'):
                receipt = self.receipts.get(path.rsplit('/',1)[-1]); data = dict(workspaceId=self.scope, found=bool(receipt), page=receipt[1] if receipt else None)
            else:
                parts = path.split('/'); ident = parts[2]; history = self.pages.get(ident, [])
                if not history: status = 404
                elif path.endswith('/history'):
                    before = int(q.get('before',[len(history)+1])[0]); data = dict(workspaceId=self.scope, versions=[{k:v for k,v in x.items() if k!='text'} for x in history[::-1] if x['revision']<before][:50])
                else:
                    revision = int(q.get('revision',[len(history)])[0]); page = history[revision-1]
                    if path.endswith('/export'):
                        headers = {'X-Aurora-Workspace':self.scope, 'X-Aurora-Page':ident, 'X-Aurora-Revision':str(revision), 'X-Aurora-SHA256':page['sha256'], 'Content-Length':str(len(page['text'].encode()))}
                        if self.bad_export: headers['X-Aurora-Workspace']='wrong-workspace'
                        r.fulfill(status=200,content_type='text/markdown; charset=utf-8',headers=headers,body=page['text']); return
                    data = dict(workspaceId=self.scope, page=page)
        r.fulfill(status=status,content_type='application/json',headers=headers,body=json.dumps(data))

server = Server(('127.0.0.1',0), functools.partial(Handler,directory=str(ROOT/'app/native/internal/web/assets')))
threading.Thread(target=server.serve_forever,daemon=True).start()
passed = []
def check(text): passed.append(text); print('PASS '+text,flush=True)
try:
    with sync_playwright() as p:
        chrome = os.environ.get('AURORA_TEST_CHROME')
        if not chrome and Path('C:/Program Files/Google/Chrome/Application/chrome.exe').exists(): chrome = 'C:/Program Files/Google/Chrome/Application/chrome.exe'
        browser = p.chromium.launch(headless=True,executable_path=chrome)
        try:
            ctx = browser.new_context(viewport={'width':390,'height':844},locale='vi-VN',service_workers='block',reduced_motion='reduce',accept_downloads=True)
            ctx.add_init_script('window.EventSource=class{constructor(){this.readyState=1}addEventListener(){}close(){}}')
            ctx.add_init_script('Object.defineProperty(Crypto.prototype,"randomUUID",{value:undefined})')
            page = ctx.new_page(); fx = Fixture(); errors = []
            page.on('pageerror',lambda e:errors.append(str(e))); page.route('**/app/api/**',fx.route)
            page.goto(f'http://127.0.0.1:{server.server_port}/app/',wait_until='networkidle')
            page.locator('#newBtn').click()
            expect(page.locator('#menuPage')).to_be_visible(timeout=1500)
            page.locator('#menuPage').click()
            dialog = page.locator('#pagesDialog')
            expect(dialog).to_be_visible()
            check('New menu opens the owner Pages editor')
            title = dialog.get_by_label('Tiêu đề trang',exact=True); text = dialog.get_by_label('Nội dung Markdown',exact=True)
            save = dialog.get_by_role('button',name='Lưu phiên bản',exact=True)
            notice = dialog.locator('.pages-notice')
            def new():
                page.locator('#newBtn').click();page.locator('#menuPage').click();expect(title).to_be_visible()
            def library():
                page.locator('#meBtn').click();page.locator('#openPages').click();page.wait_for_timeout(450)
            def back():dialog.get_by_role('button',name='Về thư viện',exact=True).click()
            def saved(revision):
                expect(notice).to_contain_text('Đã lưu phiên bản '+str(revision)+'.');expect(save).to_be_enabled()
            expect(title).to_be_focused();expect(save).to_be_disabled()
            original = '# Ghi chú đầu tiên\n\nNội dung **đậm**.\n\n<img src=x onerror="window.pageInjected=true">'
            title.fill('Tài liệu α');text.fill(original)
            dialog.get_by_text('Xem trước Markdown',exact=True).click()
            expect(dialog.locator('.pages-preview strong')).to_have_text('đậm')
            assert not page.evaluate('window.pageInjected === true')
            assert dialog.locator('.pages-preview img').count()==0
            save.click();saved(1);ident=next(iter(fx.pages))
            assert len(fx.posts)==1 and fx.pages[ident][0]['text']==original
            check('Markdown preview is rendered as safe DOM and creation saves one scoped version')
            text.fill('Bản nháp chưa lưu 🐾');page.keyboard.press('Escape');expect(dialog).to_have_count(0);expect(page.locator('#newBtn')).to_be_focused()
            page.reload(wait_until='networkidle');library();dialog.locator('.pages-row').filter(has_text='Tài liệu α').click()
            expect(text).to_have_value('Bản nháp chưa lưu 🐾');assert len(fx.posts)==1
            check('Escape restores focus; a reload recovers the page draft without posting')
            text.fill('Phiên bản thứ hai');save.click();saved(2)
            dialog.get_by_role('button',name='Lịch sử phiên bản',exact=True).first.click()
            dialog.locator('.pages-history .pages-row').filter(has_text='Phiên bản 1').click()
            with page.expect_download() as downloaded:
                dialog.get_by_role('button',name='Tải Markdown phiên bản này',exact=True).click()
            item=downloaded.value;target=OUT/'version-one.md';item.save_as(target)
            assert target.read_text(encoding='utf-8')==original and item.suggested_filename==ident+'-v1.md'
            fx.bad_export=True;dialog.get_by_role('button',name='Tải Markdown phiên bản này',exact=True).click()
            expect(notice).to_contain_text('Chưa tạo tệp tải xuống');fx.bad_export=False
            dialog.get_by_role('button',name='Dùng phiên bản này làm bản nháp',exact=True).click()
            expect(text).to_have_value(original);assert len(fx.pages[ident])==2
            save.click();saved(3);assert fx.pages[ident][1]['text']=='Phiên bản thứ hai'
            check('Exact-version export verifies scope and restoration creates a new immutable revision')
            text.fill('Bản nháp giữ lại')
            remote=dict(workspaceId=fx.scope,id=ident,baseRevision=3,title='Tên đổi ở nơi khác',text='Nội dung trên máy chủ',archived=False,clientNonce='fixture-other-writer')
            fx.save(remote);save.click()
            expect(dialog.locator('.pages-conflict')).to_contain_text('Trang đã có phiên bản mới')
            expect(text).to_have_value('Bản nháp giữ lại');expect(save).to_be_disabled()
            dialog.get_by_text('Xem bản mới trên máy chủ',exact=True).click();expect(dialog.locator('.pages-conflict pre')).to_have_text('Nội dung trên máy chủ')
            before=len(fx.posts);dialog.get_by_role('button',name='Tiếp tục với bản nháp',exact=True).click();assert len(fx.posts)==before
            save.click();saved(5);assert fx.posts[-1]['baseRevision']==4
            check('Conflicts preserve the owner draft and require an explicit rebase followed by save')
            dialog.get_by_label('Lưu trữ trang',exact=True).check();save.click();saved(6);back()
            expect(dialog.locator('.pages-row')).to_have_count(0)
            dialog.get_by_label('Hiện trang đã lưu trữ',exact=True).check();dialog.locator('.pages-row').click()
            dialog.get_by_label('Lưu trữ trang',exact=True).uncheck();save.click();saved(7)
            check('Archive and unarchive are explicit versions and the library filters archived pages')
            fx.lose_ack=True;text.fill('Lưu thành công nhưng mất ACK');before=len(fx.posts);save.click()
            expect(notice).to_contain_text('Chưa xác nhận được lần lưu');expect(text).to_be_disabled();fx.lose_ack=False
            page.reload(wait_until='networkidle');library();dialog.locator('.pages-row').click();saved(8)
            assert len(fx.posts)==before+1
            check('Reload reconciles a lost save acknowledgement by receipt GET without replay')
            fx.drop_save=True;text.fill('Lần gửi chưa đến máy chủ');save.click();expect(notice).to_contain_text('Chưa xác nhận được lần lưu')
            first=fx.posts[-1];fx.drop_save=False;dialog.get_by_role('button',name='Kiểm tra lần lưu',exact=True).click();expect(notice).to_contain_text('Chưa có biên nhận')
            dialog.get_by_role('button',name='Gửi lại lần lưu',exact=True).click();saved(9);assert fx.posts[-1]==first
            check('An explicit retry uses the identical immutable payload and nonce')
            page.evaluate('()=>{window.realPageSet=Storage.prototype.setItem;Storage.prototype.setItem=function(k,v){if(k.startsWith("aurora.pages.v1."))throw new Error("fixture quota");return realPageSet.call(this,k,v)}}')
            text.fill('Bản nháp khi đầy bộ nhớ');expect(notice).to_contain_text('Không lưu được bản nháp');before=len(fx.posts);save.click()
            expect(notice).to_contain_text('Chưa gửi lần lưu');assert len(fx.posts)==before
            with page.expect_download() as downloaded:dialog.get_by_role('button',name='Tải bản nháp',exact=True).click()
            target=OUT/'storage-draft.md';downloaded.value.save_as(target);assert target.read_text(encoding='utf-8')=='Bản nháp khi đầy bộ nhớ'
            page.evaluate('()=>{Storage.prototype.setItem=window.realPageSet}');text.fill('Lưu sau khi có dung lượng');save.click();saved(10)
            check('Storage failure is visible, blocks dispatch and leaves a downloadable draft')
            text.fill('a'*65537);expect(save).to_be_disabled();text.fill('Nội dung tiếng Việt')
            before=len(fx.posts);text.focus();text.dispatch_event('compositionstart');text.press('Enter');text.dispatch_event('compositionend');assert len(fx.posts)==before
            check('UTF-8 size bounds disable save and IME/Enter typing does not submit')
            back();fx.fail_list=True;dialog.get_by_role('button',name='Làm mới',exact=True).click();expect(dialog).to_contain_text('Chưa tải được trang tài liệu.')
            page.screenshot(path=str(OUT/'library-error-mobile.png'));fx.fail_list=False;dialog.get_by_role('button',name='Tải lại',exact=True).click();expect(dialog.locator('.pages-row')).to_have_count(1)
            check('Library loading failure stays distinct from an empty list and supports retry')
            # Add enough immutable fixture revisions to exercise the second history page.
            while len(fx.pages[ident])<55:
                n=len(fx.pages[ident]);fx.save(dict(workspaceId=fx.scope,id=ident,baseRevision=n,title='Tài liệu α',text='History '+str(n+1),archived=False,clientNonce='fixture-history-'+str(n)))
            dialog.locator('.pages-row').click();expect(dialog.locator('.pages-conflict')).to_be_visible()
            dialog.get_by_role('button',name='Lịch sử phiên bản',exact=True).first.click();expect(dialog.locator('.pages-history .pages-row')).to_have_count(50)
            dialog.get_by_role('button',name='Xem thêm phiên bản',exact=True).click();expect(dialog.locator('.pages-history .pages-row')).to_have_count(55)
            page.set_viewport_size({'width':1280,'height':900});page.screenshot(path=str(OUT/'history-desktop.png'))
            check('History pagination preserves all 55 versions and an older unsaved draft')
            dialog.get_by_role('button',name='Tiếp tục với bản nháp',exact=True).click()
            fx.hold_save=True;text.fill('Workspace A pending');save.click()
            deadline=time.monotonic()+4
            while not fx.pending and time.monotonic()<deadline:page.wait_for_timeout(20)
            assert fx.pending;held=fx.pending.pop();fx.hold_save=False
            fx.scope='pages-owner-b';fx.pages={};fx.receipts={}
            page.evaluate('void window.__AURORA.reload()');expect(dialog).to_have_count(0)
            try:held[0].fulfill(status=held[1],content_type='application/json',body=json.dumps(held[2]))
            except Exception:pass
            new();expect(title).to_have_value('');expect(text).to_have_value('');assert len(fx.pages)==0
            check('Workspace switching closes the editor and rejects a late save acknowledgement')
            page.set_viewport_size({'width':390,'height':844});title.fill('Nhật ký Aurora');text.fill('# Hôm nay\n\n- Ý tưởng mới\n- Việc cần làm')
            page.screenshot(path=str(OUT/'editor-mobile.png'))
            assert page.evaluate('document.querySelector("#pagesDialog").scrollWidth <= innerWidth')
            for _ in range(18):
                page.keyboard.press('Tab');assert page.evaluate('document.querySelector("#pagesDialog").contains(document.activeElement)')
            check('Keyboard focus stays inside the modal at the mobile viewport')
            fx.lose_ack=True;save.click();expect(notice).to_contain_text('Chưa xác nhận được lần lưu');before=len(fx.posts);fx.lose_ack=False
            page.reload(wait_until='networkidle');new();saved(1);assert len(fx.posts)==before and len(fx.pages)==1
            check('A pending new-page draft recovers its original creation receipt without duplicating the page')
            page.evaluate('localStorage.setItem("neko-aurora-lang","en")');page.reload(wait_until='networkidle');library()
            expect(dialog.get_by_role('heading',name='Page library',exact=True)).to_be_visible()
            expect(dialog.get_by_label('Find pages',exact=True)).to_be_visible()
            dialog.locator('.pages-row').click();expect(dialog.get_by_label('Page title',exact=True)).to_have_value('Nhật ký Aurora')
            expect(dialog.get_by_label('Markdown text',exact=True)).to_have_value('# Hôm nay\n\n- Ý tưởng mới\n- Việc cần làm')
            page.evaluate('document.documentElement.dataset.theme="dark"');page.screenshot(path=str(OUT/'editor-english-dark.png'))
            check('English controls preserve the literal Vietnamese document title and content')
            assert not errors, errors
            (OUT/'result.json').write_text(json.dumps(dict(result='PASS',scenarios=passed,browser=browser.version,errors=errors),indent=2),encoding='utf-8')
        except:
            page.screenshot(path=str(OUT/'failure.png')); raise
        finally: browser.close()
finally: server.shutdown(); server.server_close()
