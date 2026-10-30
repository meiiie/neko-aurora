"""Checked file-download UI using synthetic APIs and actual browser downloads."""
import functools,http.server,json,os,threading
from pathlib import Path
from urllib.parse import urlsplit,parse_qs
from playwright.sync_api import expect,sync_playwright
from fixture_http import FixtureHTTPServer
ROOT=Path(__file__).resolve().parents[3];OUT=ROOT/'.audit/2026-10-29-downloads';OUT.mkdir(exist_ok=True)
DATA=b'complete binary fixture\x00\xff'
class Handler(http.server.SimpleHTTPRequestHandler):
    def log_message(self,*_):pass
class Fixture:
    def __init__(self):self.scope='download-owner-a';self.mode='ok';self.requests=[];self.held=[];self.posts=[]
    def route(self,r):
        q=r.request;path=urlsplit(q.url).path.removeprefix('/app/api');data={}
        if q.method!='GET':self.posts.append(path)
        if path=='/state':data=dict(workspaceId=self.scope,agents=[dict(id='bot-a',name='Downloads',status='idle',av={})],pending=[],ai={},secrets=[],outbound={})
        elif path.endswith('/messages'):data=[dict(id='file',taskId='file-task',agentId='bot-a',role='agent',text='[Báo cáo.pdf](/app/api/agents/bot-a/computer/download?path=B%C3%A1o%20c%C3%A1o.pdf)',t='2026-10-29T04:00:00Z')]
        elif path.endswith('/computer'):data=dict(available=True,files=[dict(path='Báo cáo.pdf',size=len(DATA),dir=False)],log=[],usedBytes=len(DATA),quotaBytes=1<<20)
        elif path.endswith('/download'):
            self.requests.append(q.url)
            if self.mode=='hold':self.held.append(r);return
            if self.mode=='offline':r.abort('connectionreset');return
            if self.mode.isdigit():r.fulfill(status=int(self.mode),content_type='application/json',body='{"error":"fixture"}');return
            self.fulfill(r,self.mode);return
        r.fulfill(status=200,content_type='application/json',body=json.dumps(data))
    def fulfill(self,r,mode='ok',scope=None):
        headers={'Content-Type':'application/octet-stream','Content-Length':str(len(DATA)),'X-Aurora-Workspace':scope or self.scope,'X-Aurora-Bot':'bot-a','Content-Disposition':"attachment; filename*=UTF-8''B%C3%A1o%20c%C3%A1o.pdf"}
        if mode=='wrong-scope':headers['X-Aurora-Workspace']='other-owner'
        if mode=='wrong-bot':headers['X-Aurora-Bot']='other-bot'
        if mode=='html':headers['Content-Type']='text/html'
        if mode=='short':headers['Content-Length']=str(len(DATA)+8)
        r.fulfill(status=200,headers=headers,body=DATA)
server=FixtureHTTPServer(('127.0.0.1',0),functools.partial(Handler,directory=str(ROOT/'app/native/internal/web/assets')))
threading.Thread(target=server.serve_forever,daemon=True).start()
try:
    with sync_playwright() as p:
        browser=p.chromium.launch(headless=True,executable_path=os.environ.get('AURORA_TEST_CHROME') or 'C:/Program Files/Google/Chrome/Application/chrome.exe')
        try:
            ctx=browser.new_context(viewport=dict(width=390,height=844),locale='vi-VN',service_workers='block',reduced_motion='reduce',accept_downloads=True);ctx.add_init_script('window.EventSource=class{addEventListener(){}close(){}}');page=ctx.new_page();fx=Fixture();errors=[];downloads=[];page.on('pageerror',lambda e:errors.append(str(e)));page.on('download',lambda d:downloads.append(d));page.route('**/app/api/**',fx.route)
            url=f'http://127.0.0.1:{server.server_port}/app/#bot-a';page.goto(url,wait_until='networkidle');link=page.locator('.file-chip');expect(link).to_be_visible()
            fx.mode='409';link.click();expect(page.locator('#toast')).to_contain_text('đã thay đổi',timeout=2000);assert page.url==url and not downloads
            assert parse_qs(urlsplit(fx.requests[-1]).query)['workspaceId']==[fx.scope];assert len(fx.requests)==1
            print('PASS changed-file errors stay in chat and never retry or save an error response')
            fx.mode='ok'
            with page.expect_download() as event:link.click()
            dl=event.value;assert dl.suggested_filename=='Báo cáo.pdf';assert Path(dl.path()).read_bytes()==DATA;expect(page.locator('#toast')).to_contain_text('Đã chuyển file')
            print('PASS a complete response saves exact binary bytes under the Unicode filename')
            before=len(downloads)
            for mode,needle in [('413','vượt giới hạn'),('503','đang bận'),('offline','Không tải được'),('wrong-scope','Không tải được'),('wrong-bot','Không tải được'),('html','Không tải được'),('short','Không tải được')]:
                fx.mode=mode;link.click();expect(page.locator('#toast')).to_contain_text(needle);page.wait_for_timeout(60);assert len(downloads)==before and page.url==url,mode
            print('PASS quota busy network metadata and truncated-response failures cannot save a file')
            fx.mode='hold';count=len(fx.requests);link.click();page.wait_for_timeout(60);link.click();page.wait_for_timeout(60);assert len(fx.requests)==count+1 and len(fx.held)==1
            with page.expect_download():fx.fulfill(fx.held.pop())
            print('PASS duplicate clicks share one preparation request')
            fx.mode='hold';link.click();page.wait_for_timeout(60);held=fx.held.pop();before=len(downloads);page.locator('#backBtn').click();fx.fulfill(held);page.wait_for_timeout(150);assert len(downloads)==before
            print('PASS leaving the source chat discards a late download')
            page.locator('#rows .row[data-id=bot-a] .rowin').click();expect(link).to_be_visible();fx.mode='hold';link.click();page.wait_for_timeout(60);held=fx.held.pop();old=fx.scope;fx.scope='download-owner-b';page.evaluate('window.__AURORA.reload()');page.wait_for_timeout(150);fx.fulfill(held,scope=old);page.wait_for_timeout(150);assert len(downloads)==before
            print('PASS a previous workspace cannot save its late response in the current workspace')
            fx.mode='ok';page.locator('#rows .row[data-id=bot-a] .rowin').click();expect(link).to_be_visible();page.locator('#whoBtn').click();cell=page.locator('#pcBox .cell').filter(has_text='Báo cáo.pdf');expect(cell).to_be_visible()
            with page.expect_download() as event:cell.click()
            assert Path(event.value.path()).read_bytes()==DATA
            print('PASS the Computer file list uses the same checked download contract')
            fx.mode='409';cell.click();expect(page.locator('#toast')).to_contain_text('đã thay đổi');page.screenshot(path=str(OUT/'download-error-mobile.png'))
            with page.expect_navigation(wait_until='load'):page.evaluate('window.__I18N.set("en")')
            page.set_viewport_size(dict(width=1280,height=900));expect(page.locator('.file-chip')).to_be_visible();page.locator('.file-chip').click();expect(page.locator('#toast')).to_contain_text('changed');page.screenshot(path=str(OUT/'download-error-desktop-en.png'))
            print('PASS Vietnamese mobile and English desktop show download failure feedback')
            count=len(fx.requests)
            rejected=page.evaluate('''async scope=>{
              const notices=[],hrefs=['https://example.invalid/app/api/agents/bot-a/computer/download?path=a.pdf','/app/api/agents/bot-a/computer/file?path=a.pdf','/app/api/agents/bot-a/computer/download?path=a.pdf&path=b.pdf','/app/api/agents/bot-a/computer/download?path='];
              const results=[];for(const href of hrefs)results.push(await AURORA_DOWNLOADS.save({href,workspaceId:scope,active:()=>true,notice:s=>notices.push(s)}));
              return {results,notices};
            }''',fx.scope)
            assert rejected['results']==[False]*4 and len(rejected['notices'])==4 and len(fx.requests)==count
            print('PASS foreign origins and malformed file links are rejected before any download request')
            fx.mode='hold';before=len(downloads)
            page.evaluate('''scope=>{window.downloadChecks=[];window.downloadNotices=[];for(const name of ['one.pdf','two.pdf'])window.downloadChecks.push(AURORA_DOWNLOADS.save({href:'/app/api/agents/bot-a/computer/download?path='+name,workspaceId:scope,active:()=>true,notice:s=>window.downloadNotices.push(s)}));}''',fx.scope)
            page.wait_for_function("window.downloadNotices.length===2");page.wait_for_timeout(80);assert len(fx.held)==2 and len(fx.requests)==count+2
            busy=page.evaluate("scope=>AURORA_DOWNLOADS.save({href:'/app/api/agents/bot-a/computer/download?path=three.pdf',workspaceId:scope,active:()=>true,notice:s=>window.downloadNotices.push(s)})",fx.scope)
            assert busy is False and len(fx.requests)==count+2
            assert 'two files' in page.evaluate('window.downloadNotices.at(-1)').lower()
            for held in fx.held:fx.fulfill(held)
            fx.held.clear();assert page.evaluate('Promise.all(window.downloadChecks)')==[True,True]
            page.wait_for_timeout(100);assert len(downloads)==before+2
            print('PASS two distinct downloads complete while a third is rejected without another request')
            assert not errors and not fx.posts,(errors,fx.posts)
        finally:browser.close()
finally:server.shutdown();server.server_close()
