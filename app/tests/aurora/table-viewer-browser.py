"""Table dialog interactions and source ownership in the real local UI."""
import functools
import http.server
import json
import os
import threading
from pathlib import Path
from urllib.parse import urlsplit
from playwright.sync_api import expect, sync_playwright

ROOT=Path(__file__).resolve().parents[3]
OUT=ROOT/'.audit/2026-10-28-table-viewer'
OUT.mkdir(exist_ok=True)
class Server(http.server.ThreadingHTTPServer):
    request_queue_size=64
class Handler(http.server.SimpleHTTPRequestHandler):
    def log_message(self,*_):pass
class Fixture:
    def __init__(self): self.scope='owner-a';self.version=0;self.posts=[]
    def route(self,route):
        req=route.request;path=urlsplit(req.url).path.removeprefix('/app/api');data={}
        if req.method=='POST':self.posts.append(path)
        if path=='/events':route.fulfill(status=200,content_type='text/event-stream',body='');return
        if path=='/state':
            data={'workspaceId':self.scope,'agents':[{'id':'bot-'+x,'name':'Chat '+x,'status':'idle','av':{'shape':0,'color':4}} for x in ['a','b']],'pending':[],'ai':{},'secrets':[],'outbound':{}}
        elif path.endswith('/messages'):
            table='| Cột A | Cột B | Cột C |\n|---|---|---|\n'+'\n'.join('| '+self.scope+'-'+str(self.version)+' dòng '+str(i)+' | '+('nội dung tiếng Việt '*10)+' | 👩🏽‍💻 |' for i in range(40))
            data=[{'id':'table','agentId':'bot-a','role':'agent','text':table,'t':'2026-10-28T04:00:00Z'}] if 'bot-a' in path else []
        route.fulfill(status=200,content_type='application/json',body=json.dumps(data))

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
            page=context.new_page();fx=Fixture();errors=[];failed=[]
            page.on('pageerror',lambda e:errors.append(str(e)))
            page.on('requestfailed',lambda r:failed.append({'url':r.url,'failure':r.failure}))
            page.route('**/app/api/**',fx.route)
            page.goto(f'http://127.0.0.1:{server.server_port}/app/#bot-a',wait_until='networkidle')
            opener=page.get_by_role('button',name='Mở rộng bảng',exact=True)
            dialog=page.get_by_role('dialog',name='Bảng mở rộng',exact=True)
            def open_table():
                expect(opener).to_be_visible();opener.click();expect(dialog).to_be_visible()
                expect(dialog.get_by_role('button',name='Đóng bảng',exact=True)).to_be_focused()
            open_table()
            expect(dialog).to_contain_text('owner-a-0 dòng 39')
            for value in ['125%','150%','175%','200%','100%']:
                dialog.get_by_role('button',name='Phóng to bảng',exact=True).click()
                expect(dialog.get_by_role('button',name='Phóng to bảng',exact=True)).to_have_text(value)
            page.screenshot(path=str(OUT/'table-mobile.png'))
            page.keyboard.press('Escape');expect(dialog).to_have_count(0);expect(opener).to_be_focused()
            check('mobile table opens with modal focus, zoom cycles and Escape restores the opener')
            wrap=page.locator('#messages .tbl');wrap.focus();page.keyboard.press('ArrowRight')
            page.wait_for_function('document.querySelector("#messages .tbl").scrollLeft>0')
            check('wide inline table can be scrolled using the keyboard')
            open_table();stage=dialog.locator('.table-stage');stage.focus();page.keyboard.press('ArrowRight');page.keyboard.press('ArrowDown')
            page.wait_for_function('document.querySelector(".table-stage").scrollLeft>0 && document.querySelector(".table-stage").scrollTop>0')
            page.wait_for_timeout(250)
            stage.evaluate('n=>{n.scrollLeft=0;n.scrollTop=0}')
            cell=dialog.locator('tbody tr:first-child td').first
            box=cell.evaluate('n=>{const range=document.createRange();range.setStart(n.firstChild,0);range.setEnd(n.firstChild,1);const r=range.getBoundingClientRect();return {x:r.x,y:r.y,height:r.height}}');edge=stage.bounding_box()
            page.mouse.move(box['x']+1,box['y']+box['height']/2);page.mouse.down()
            try:
                page.mouse.move(box['x']+50,box['y']+box['height']/2,steps=5)
                page.wait_for_function('getSelection().toString().length>0')
                page.mouse.move(edge['x']+edge['width']+10,box['y']+box['height']/2,steps=12)
                page.wait_for_function('document.querySelector(".table-stage").scrollLeft>20')
                assert page.evaluate('getSelection().toString().length>0')
            finally:page.mouse.up()
            expect(dialog).to_be_visible()
            check('real pointer selection beyond the wide table scrolls horizontally; keyboard scroll works on both axes')
            page.evaluate('getSelection().removeAllRanges()')
            page.locator('#messages').evaluate('n=>{n.scrollTop=210}')
            scroll=page.locator('#messages').evaluate('n=>n.scrollTop')
            dialog.get_by_role('button',name='Đóng bảng',exact=True).click()
            expect(dialog).to_have_count(0);expect(opener).to_be_focused()
            assert abs(page.locator('#messages').evaluate('n=>n.scrollTop')-scroll)<1
            check('closing restores focus without scrolling the conversation to its offscreen opener')
            open_table();dialog.get_by_role('button',name='Phóng to bảng',exact=True).click()
            stage.evaluate('n=>{n.scrollLeft=80;n.scrollTop=120}')
            page.evaluate('window.__AURORA.render()')
            expect(dialog).to_be_visible();expect(dialog.get_by_role('button',name='Phóng to bảng',exact=True)).to_have_text('125%')
            assert stage.evaluate('n=>n.scrollLeft===80 && n.scrollTop===120')
            check('unchanged message rerender retains the open viewer, zoom and scroll position')
            page.evaluate('()=>{window.__AURORA.state().msgs["bot-a"][0].text+="\\n\\nUpdated";window.__AURORA.render()}')
            expect(dialog).to_have_count(0)
            check('replacing the source message dismisses its stale snapshot')
            open_table();page.evaluate('document.querySelector("#messages .table-card").remove()');expect(dialog).to_have_count(0)
            check('removing the source table cleans up the modal')
            page.reload(wait_until='networkidle');open_table()
            page.evaluate('document.querySelector("#backBtn").click()');expect(dialog).to_have_count(0)
            expect(page.locator('#thread')).to_have_class('screen thread off')
            page.locator('#rows .row[data-id="bot-a"] .rowin').click();open_table()
            page.evaluate('document.dispatchEvent(new CustomEvent("aurora-network-open-task",{detail:{botId:"bot-b"}}))')
            expect(page.locator('#threadName')).to_have_text('Chat b');expect(dialog).to_have_count(0)
            check('leaving a conversation or navigating directly to another closes its viewer')
            page.locator('#backBtn').click();page.locator('#rows .row[data-id="bot-a"] .rowin').click();open_table()
            page.evaluate('document.querySelector("#messages .table-actions button:last-of-type").click()')
            expect(dialog).to_have_count(1);expect(dialog).to_be_visible()
            page.mouse.click(2,2);expect(dialog).to_have_count(0);expect(opener).to_be_focused()
            check('repeated activation keeps a single viewer and backdrop dismissal restores focus')
            page.set_viewport_size({'width':1280,'height':900});open_table()
            bounds=dialog.bounding_box();assert bounds['x']>=0 and bounds['x']+bounds['width']<=1280
            assert bounds['y']>=0 and bounds['y']+bounds['height']<=900
            dialog.get_by_role('button',name='Đóng bảng',exact=True).focus();page.keyboard.press('Tab');expect(stage).to_be_focused()
            page.keyboard.press('Shift+Tab');expect(dialog.get_by_role('button',name='Đóng bảng',exact=True)).to_be_focused()
            page.keyboard.press('Shift+Tab');expect(dialog.get_by_role('button',name='Phóng to bảng',exact=True)).to_be_focused()
            assert page.locator('#input').evaluate('n=>n!==document.activeElement')
            page.screenshot(path=str(OUT/'table-desktop.png'))
            page.keyboard.press('Escape');expect(opener).to_be_focused()
            check('desktop viewer stays inside the viewport and keyboard focus moves between its controls')
            open_table()
            fx.scope='owner-b';page.evaluate('window.__AURORA.reload()')
            expect(page.locator('#thread')).to_have_class('screen thread off')
            expect(dialog).to_have_count(0)
            check('workspace change dismisses the old table snapshot')
            assert not fx.posts and not errors,errors
            (OUT/'table-viewer-result.json').write_text(json.dumps({'result':'PASS','scenarios':11,'browser':browser.version,'errors':errors,'failedRequests':failed},indent=2),encoding='utf-8')
        finally:browser.close()
finally:server.shutdown();server.server_close()
