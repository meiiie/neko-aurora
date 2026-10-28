"""Held-response and event ownership fixtures against the local Aurora UI.
Fake EventSource checks callback ownership, not SSE transport reliability.
"""
import functools
import http.server
import json
import os
import threading
import time
from pathlib import Path
from urllib.parse import urlsplit
from playwright.sync_api import expect, sync_playwright

ROOT=Path(__file__).resolve().parents[3]
OUT=ROOT/'.audit/2026-10-28-chat-loading'
OUT.mkdir(exist_ok=True)
class Server(http.server.ThreadingHTTPServer):
    request_queue_size=64
class Handler(http.server.SimpleHTTPRequestHandler):
    def log_message(self,*_): pass
class Fixture:
    def __init__(self):
        self.scope='owner-a';self.version=0;self.hold=None;self.held=[];self.states=0
    def route(self,route):
        path=urlsplit(route.request.url).path.removeprefix('/app/api')
        data={};kind=None
        if path=='/state':
            self.states+=1;kind='state'
            data={'workspaceId':self.scope,'agents':[{'id':'bot-a','name':'Chat '+self.scope,'status':'idle','t':'2026-10-28T04:00:00Z','av':{'shape':0,'color':4}}],'pending':[],'ai':{},'secrets':[],'outbound':{}}
        elif path.endswith('/messages'):
            kind='messages'
            data=[{'id':'msg-a','agentId':'bot-a','role':'agent','text':self.scope+'/'+str(self.version),'t':'2026-10-28T04:00:00Z'}]
        if kind and self.hold==kind:
            self.hold=None;self.held.append((route,data));return
        route.fulfill(status=200,content_type='application/json',body=json.dumps(data))

def check(label): print('PASS '+label,flush=True)
server=Server(('127.0.0.1',0),functools.partial(Handler,directory=str(ROOT/'app/native/internal/web/assets')))
threading.Thread(target=server.serve_forever,daemon=True).start()
url=f'http://127.0.0.1:{server.server_port}/app/'
try:
    with sync_playwright() as p:
        chrome=os.environ.get('AURORA_TEST_CHROME')
        if not chrome and Path('C:/Program Files/Google/Chrome/Application/chrome.exe').exists():chrome='C:/Program Files/Google/Chrome/Application/chrome.exe'
        browser=p.chromium.launch(headless=True,executable_path=chrome)
        print('Chrome '+browser.version,flush=True)
        try:
            context=browser.new_context(viewport={'width':1280,'height':900},locale='vi-VN',service_workers='block')
            context.add_init_script('''window.fixtureStreams=[];window.EventSource=class{constructor(){this.listeners={};this.readyState=1;this.closed=false;fixtureStreams.push(this);queueMicrotask(()=>this.onopen?.({}))}addEventListener(type,fn){(this.listeners[type]??=[]).push(fn)}close(){this.closed=true;this.readyState=2}emit(type,data){for(const fn of this.listeners[type]||[])fn({data:JSON.stringify(data)})}}''')
            page=context.new_page();fx=Fixture();errors=[]
            page.on('pageerror',lambda e:errors.append(str(e)))
            page.route('**/app/api/**',fx.route)
            page.goto(url+'#bot-a')
            expect(page.locator('#messages')).to_contain_text('owner-a/0')
            def start(name):page.evaluate('()=>{window.'+name+'=window.__AURORA.reload()}')
            def held(count=1):
                deadline=time.monotonic()+8
                while len(fx.held)<count and time.monotonic()<deadline:page.wait_for_timeout(20)
                assert len(fx.held)==count,'held request missing'
            def release(index=0,status=200):
                route,data=fx.held.pop(index)
                route.fulfill(status=status,content_type='application/json',body=json.dumps(data if status==200 else {'error':'fixture failure'}))
            def open_chat():
                page.locator('#rows .row[data-id="bot-a"] .rowin').click()
                expect(page.locator('#thread')).not_to_have_class('screen thread off')
            def switch(scope):
                fx.scope=scope;page.evaluate('window.__AURORA.reload()')
                expect(page.locator('#thread')).to_have_class('screen thread off')
            def message(text):
                expect(page.locator('#messages')).to_contain_text(text)
                assert page.evaluate('window.__AURORA.state().msgs["bot-a"][0].text')==text

            fx.hold='messages';start('oldOwnerLoad');held()
            switch('owner-b');open_chat();message('owner-b/0')
            release();page.evaluate('window.oldOwnerLoad');message('owner-b/0')
            check('late owner A history cannot replace owner B history for the same bot ID')

            fx.hold='messages';start('oldRoundTrip');held()
            switch('owner-a');switch('owner-b');fx.version=1;open_chat();message('owner-b/1')
            release();page.evaluate('window.oldRoundTrip');message('owner-b/1')
            check('workspace generation rejects an old response after B/A/B')

            fx.hold='messages';start('firstLoad');held()
            fx.version=2;fx.hold='messages';start('secondLoad');held(2)
            release(1);page.evaluate('window.secondLoad');message('owner-b/2')
            release();page.evaluate('window.firstLoad');message('owner-b/2')
            check('same-workspace history requests cannot complete out of order over newer data')

            fx.hold='state';start('oldState');held()
            switch('owner-a');open_chat();message('owner-a/2')
            release();page.evaluate('window.oldState')
            assert page.evaluate('window.__AURORA.state().workspaceId')=='owner-a'
            message('owner-a/2')
            check('an older state response cannot switch the UI back to the previous workspace')

            fx.hold='messages';start('oldFailure');held()
            switch('owner-b');open_chat();message('owner-b/2')
            release(status=500);page.evaluate('window.oldFailure');message('owner-b/2')
            check('late history failure from an old workspace is discarded')

            fx.hold='state';start('oldStateFailure');held()
            fx.version=3;page.evaluate('window.__AURORA.reload()');message('owner-b/3')
            release(status=500);page.evaluate('window.oldStateFailure');message('owner-b/3')
            check('superseded state failures cannot undo a successful reload')

            assert page.evaluate('fixtureStreams.length')>=5
            assert page.evaluate('fixtureStreams.slice(0,-1).every(x=>x.closed)')
            before=page.evaluate('JSON.stringify(window.__AURORA.state())')
            page.evaluate('''()=>{const old=fixtureStreams[0];old.emit('agent',{id:'bot-a',name:'wrong owner',status:'idle'});old.emit('message',{id:'wrong',agentId:'bot-a',role:'agent',text:'private old workspace'});old.emit('archived',{id:'bot-a'});old.emit('delta',{id:'wrong-draft',agentId:'bot-a',delta:'private old draft'});old.emit('brief',{text:'private old brief'});old.onopen?.({});old.onerror?.({})}''')
            assert page.evaluate('JSON.stringify(window.__AURORA.state())')==before
            message('owner-b/3')
            check('workspace switches close the old event stream and ignore its queued callbacks')
            page.evaluate('''()=>{const live=fixtureStreams.at(-1);live.emit('delta',{id:'draft-current',agentId:'bot-a',delta:'current draft'});fixtureStreams[0].emit('draft_end',{id:'draft-current'})}''')
            assert page.evaluate('window.__AURORA.state().drafts["draft-current"].text')=='current draft'
            page.evaluate('''()=>{const live=fixtureStreams.at(-1);live.emit('message',{id:'live-message',agentId:'bot-a',role:'agent',text:'Current live message',t:'2026-10-28T04:01:00Z'});live.emit('draft_end',{id:'draft-current'})}''')
            expect(page.locator('#messages')).to_contain_text('Current live message')
            assert page.evaluate('window.__AURORA.state().drafts["draft-current"]===undefined')
            check('current stream messages and draft completion still work after replacing the stream')

            page.evaluate('''()=>{window.fixtureNotices=[];window.fixtureHidden=true;Object.defineProperty(document,'hidden',{get:()=>fixtureHidden,configurable:true});window.Notification=class{static permission='granted'};Object.defineProperty(navigator.serviceWorker,'ready',{value:new Promise(r=>{window.fixtureWorkerReady=()=>r({showNotification:(title)=>fixtureNotices.push(title)})}),configurable:true});fixtureStreams.at(-1).emit('notify',{title:'Old owner notice',body:'Old content',url:'/app/#bot-a'})}''')
            switch('owner-a')
            page.evaluate('fixtureWorkerReady()')
            assert page.evaluate('fixtureNotices')==[]
            page.evaluate("fixtureStreams.at(-1).emit('notify',{title:'Current owner notice',url:'/app/#bot-a'})")
            assert page.evaluate('fixtureNotices')==['Current owner notice']
            page.evaluate('()=>{window.fixtureHidden=false}')
            check('deferred notification from an old stream is suppressed while current notifications work')
            open_chat();message('owner-a/3')

            page.locator('#backBtn').click()
            page.locator('#rows .rowin').click(button='right')
            expect(page.locator('.ctx')).to_have_count(1)
            switch('owner-b')
            expect(page.locator('.ctx')).to_have_count(0)
            check('workspace change dismisses an old conversation context menu')
            page.wait_for_timeout(720)
            fx.hold='messages';open_chat();held();release(status=500)
            expect(page.locator('#toast')).to_contain_text('Không tải được hội thoại')
            check('a current history failure still reports the error to the user')
            fx.version=4;page.evaluate('window.__AURORA.reload()');message('owner-b/4')
            page.set_viewport_size({'width':390,'height':844})
            page.wait_for_function("()=>{const r=document.querySelector('#thread').getBoundingClientRect();return r.left>=0&&r.right<=innerWidth+.5}")
            expect(page.locator('#toast')).to_have_class('toast')
            page.screenshot(path=str(OUT/'chat-loading-mobile.png'),animations='disabled')
            assert not errors,errors
            check('current workspace can reload successfully after errors with mobile layout intact')
        except Exception:
            print('CHAT DIAGNOSTICS',json.dumps({'errors':errors,'state':page.evaluate('window.__AURORA?.state()')},ensure_ascii=True),flush=True)
            page.screenshot(path=str(OUT/'chat-loading-failure.png'))
            raise
        finally:browser.close()
finally:
    server.shutdown();server.server_close()
