"""Image queue lifecycle against Chromium + loopback fixtures, no provider calls."""
import base64
import functools
import hashlib
import http.server
import json
import os
import threading
import time
from pathlib import Path
from fixture_http import FixtureHTTPServer
from urllib.parse import urlsplit
from playwright.sync_api import expect, sync_playwright

ROOT=Path(__file__).resolve().parents[3]
OUT=ROOT/'.audit/2026-10-28-uploads'
OUT.mkdir(parents=True,exist_ok=True)
PNG=base64.b64decode('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+a9wAAAABJRU5ErkJggg==')
class Handler(http.server.SimpleHTTPRequestHandler):
    def log_message(self,*_):pass
class Fixture:
    def __init__(self):
        self.scope='upload-owner-a';self.posts=[];self.uploads=[];self.received={};self.mode='ok';self.held=[]
    def route(self,route):
        req=route.request;path=urlsplit(req.url).path.removeprefix('/app/api');data={}
        if path=='/events':route.fulfill(status=200,content_type='text/event-stream',body='');return
        if path=='/state':
            data={'workspaceId':self.scope,'agents':[{'id':'bot-a','name':'Chat A','status':'idle','av':{'shape':0,'color':4}}],'pending':[],'ai':{},'secrets':[],'outbound':{}}
        elif path.endswith('/messages'):
            if req.method=='POST':self.posts.append(req.post_data_json)
            data=[]
        elif path.endswith('/computer/upload'):
            body=req.post_data_json;self.uploads.append(body)
            assert body['deduplicate'] is True
            assert body['workspaceId']==self.scope,'dispatch crossed workspace'
            key=(body['workspaceId'],body['name'],body['data'])
            self.received.setdefault(key,'uploads/i-'+hashlib.sha256(base64.b64decode(body['data'])).hexdigest()+'-image.jpg')
            data={'path':self.received[key]}
            if self.mode=='lost':route.abort('connectionreset');return
            if self.mode=='held':self.held.append((route,data));return
            if self.mode=='invalid':data={'path':'uploads/bad]\nINJECT'}
        route.fulfill(status=200,content_type='application/json',body=json.dumps(data))
def check(s):print('PASS '+s,flush=True)
server=FixtureHTTPServer(('127.0.0.1',0),functools.partial(Handler,directory=str(ROOT/'app/native/internal/web/assets')))
threading.Thread(target=server.serve_forever,daemon=True).start()
url=f'http://127.0.0.1:{server.server_port}/app/#bot-a'
try:
 with sync_playwright() as p:
    chrome=os.environ.get('AURORA_TEST_CHROME')
    if not chrome and Path('C:/Program Files/Google/Chrome/Application/chrome.exe').exists():chrome='C:/Program Files/Google/Chrome/Application/chrome.exe'
    browser=p.chromium.launch(headless=True,executable_path=chrome)
    try:
        context=browser.new_context(viewport={'width':390,'height':844},locale='vi-VN',service_workers='block')
        fx=Fixture();errors=[]
        def new_page():
            pg=context.new_page();pg.on('pageerror',lambda e:errors.append(str(e)));pg.route('**/app/api/**',fx.route);pg.goto(url);expect(pg.locator('#whoBtn')).to_contain_text('Chat A');return pg
        page=new_page()
        def choose():page.locator('#imgPick').set_input_files({'name':'same.png','mimeType':'image/png','buffer':PNG})
        def wait_upload(count):
            end=time.monotonic()+10
            while len(fx.uploads)<count and time.monotonic()<end:page.wait_for_timeout(25)
            assert len(fx.uploads)==count
        page.locator('#input').fill('Giữ nháp')
        fx.mode='lost';choose();wait_upload(1)
        expect(page.locator('#uploadQueue')).to_contain_text('Chưa nhận được xác nhận')
        first=fx.uploads[0]
        page.close();page=new_page()
        expect(page.locator('#uploadQueue')).to_contain_text('Thử lại')
        expect(page.locator('#input')).to_have_value('Giữ nháp')
        assert len(fx.uploads)==1 and not fx.posts
        check('lost response preserves bytes across page close without automatic dispatch')
        fx.mode='ok';page.locator('#uploadQueue').get_by_role('button',name='Thử lại',exact=True).focus();page.keyboard.press('Enter')
        wait_upload(2);expect(page.locator('#uploadQueue')).to_be_hidden()
        assert fx.uploads[1]==first and len(fx.received)==1
        assert page.locator('#input').input_value().count('[Ảnh: ')==1
        check('keyboard retry reuses exact body and server receipt, appending once')
        page.locator('#input').fill('Pending before reload');fx.mode='held';choose();wait_upload(3)
        expect(page.locator('#uploadQueue')).to_contain_text('Đang tải ảnh')
        page.close();page=new_page()
        expect(page.locator('#uploadQueue')).to_contain_text('Ảnh chờ trên thiết bị')
        assert len(fx.uploads)==3
        check('in-flight upload is recoverable after window closes')
        fx.scope='upload-owner-b';page.reload();expect(page.locator('#uploadQueue')).to_be_hidden();expect(page.locator('#input')).to_have_value('')
        assert len(fx.uploads)==3
        fx.scope='upload-owner-a';page.reload();expect(page.locator('#uploadQueue')).to_contain_text('Thử lại')
        fx.mode='ok';page.locator('#uploadQueue').get_by_role('button',name='Thử lại',exact=True).click();wait_upload(4);expect(page.locator('#uploadQueue')).to_be_hidden()
        check('pending image bytes remain isolated across account changes')
        page.locator('#input').fill('Durable draft before quota')
        page.evaluate('''()=>{const set=Storage.prototype.setItem;Storage.prototype.setItem=function(k,v){if(k.startsWith('aurora.composer.'))throw new DOMException('full','QuotaExceededError');return set.call(this,k,v)}}''')
        choose();wait_upload(5)
        expect(page.locator('#uploadQueue')).to_contain_text('chờ chèn vào nháp')
        expect(page.locator('#toast')).to_contain_text('Nháp chưa được lưu')
        page.reload();expect(page.locator('#input')).to_have_value('Durable draft before quota');expect(page.locator('#uploadQueue')).to_contain_text('Chèn vào nháp')
        assert len(fx.uploads)==5
        page.locator('#uploadQueue').get_by_role('button',name='Chèn vào nháp',exact=True).click();expect(page.locator('#uploadQueue')).to_be_hidden()
        assert page.locator('#input').input_value().count('[Ảnh: ')==1
        check('ready receipt survives draft storage failure and needs no re-upload')
        page.evaluate('''()=>{window.fixtureRemove=IDBObjectStore.prototype.delete;IDBObjectStore.prototype.delete=function(k){if(this.name==='images'){this.transaction.abort();return}return window.fixtureRemove.call(this,k)}}''')
        choose();wait_upload(6);expect(page.locator('#uploadQueue')).to_contain_text('chờ chèn vào nháp')
        page.reload();expect(page.locator('#uploadQueue')).to_contain_text('Chèn vào nháp')
        assert page.locator('#input').input_value().count('[Ảnh: ')==1
        page.locator('#uploadQueue').get_by_role('button',name='Chèn vào nháp',exact=True).click();expect(page.locator('#uploadQueue')).to_be_hidden()
        assert page.locator('#input').input_value().count('[Ảnh: ')==1
        check('interrupted queue deletion cannot duplicate an existing draft reference')
        page.evaluate('''()=>{const add=IDBObjectStore.prototype.add;IDBObjectStore.prototype.add=function(v){const r=add.call(this,v);if(this.name==='images')r.addEventListener('success',()=>this.transaction.abort());return r}}''')
        choose();expect(page.locator('#toast')).to_contain_text('Chưa lưu được ảnh')
        assert len(fx.uploads)==6
        page.reload();expect(page.locator('#uploadQueue')).to_be_hidden()
        check('aborted storage transaction never dispatches or claims image persistence')
        result=page.evaluate('''async()=>{const q=AURORA_UPLOAD_QUEUE.create();const rows=[];for(let i=0;i<12;i++)rows.push(await q.add('limits','bot-a','tiny.jpg','YQ=='));let full=false;try{await q.add('limits','bot-a','tiny.jpg','YQ==')}catch(e){full=e.message==='upload_queue_full'}for(const x of rows)await q.remove(x);return full}''')
        assert result
        result=page.evaluate('''async()=>{const q=AURORA_UPLOAD_QUEUE.create(),data='AAAA'.repeat(349525)+'AA==',rows=[];for(let i=0;i<8;i++)rows.push(await q.add('limits','bot-a','big.jpg',data));let full=false;try{await q.add('limits','bot-a','tiny.jpg','YQ==')}catch(e){full=e.message==='upload_queue_full'}for(const x of rows)await q.remove(x);return full}''')
        assert result
        check('item and aggregate byte budgets bound local queue growth')
        fx.mode='invalid';choose();wait_upload(7);expect(page.locator('#uploadQueue')).to_contain_text('Chưa nhận được xác nhận')
        assert 'INJECT' not in page.locator('#input').input_value()
        expect(page.locator('#uploadQueue').get_by_role('button',name='Bỏ ảnh khỏi hàng đợi')).to_be_enabled()
        page.screenshot(path=str(OUT/'uploads-mobile.png'))
        overflow=page.evaluate('document.documentElement.scrollWidth>innerWidth')
        assert not overflow
        page.locator('#uploadQueue').get_by_role('button',name='Bỏ ảnh khỏi hàng đợi').click();expect(page.locator('#uploadQueue')).to_be_hidden()
        page.reload();expect(page.locator('#uploadQueue')).to_be_hidden()
        assert not fx.posts and not errors,errors
        check('invalid server path rejected; mobile queue can be removed without chat dispatch or browser errors')
        before=page.locator('#input').input_value()
        page.evaluate('''()=>{const put=IDBObjectStore.prototype.put;IDBObjectStore.prototype.put=function(v){const r=put.call(this,v);if(this.name==='images'&&v.state==='ready')r.addEventListener('success',()=>this.transaction.abort());return r}}''')
        fx.mode='ok';choose();wait_upload(8);expect(page.locator('#uploadQueue')).to_contain_text('Chưa nhận được xác nhận')
        expect(page.locator('#input')).to_have_value(before)
        page.reload();page.locator('#uploadQueue').get_by_role('button',name='Thử lại',exact=True).click();wait_upload(9);expect(page.locator('#uploadQueue')).to_be_hidden()
        check('ready-receipt transaction must commit before draft insertion')
        page.evaluate('const realTimeout=AbortSignal.timeout.bind(AbortSignal);AbortSignal.timeout=ms=>realTimeout(ms===30000?100:ms)')
        fx.mode='held';choose();wait_upload(10);expect(page.locator('#uploadQueue')).to_contain_text('Chưa nhận được xác nhận')
        page.reload();expect(page.locator('#uploadQueue')).to_contain_text('Thử lại')
        fx.mode='ok';page.locator('#uploadQueue').get_by_role('button',name='Thử lại',exact=True).click();wait_upload(11);expect(page.locator('#uploadQueue')).to_be_hidden()
        check('timeout after server acceptance retains retryable bytes and receipt identity')
        assert not fx.posts and not errors,errors
        (OUT/'upload-browser-result.json').write_text(json.dumps({'result':'PASS','scenarios':11,'browser':browser.version,'viewport':[390,844],'uploads':len(fx.uploads),'serverReceipts':len(fx.received),'chatPosts':len(fx.posts),'errors':errors},indent=2),encoding='utf-8')
    finally:browser.close()
finally:server.shutdown();server.server_close()
