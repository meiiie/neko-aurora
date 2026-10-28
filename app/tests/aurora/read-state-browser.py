"""Local read-state interaction fixtures; no live service or device acceptance."""
import functools
import http.server
import json
import os
import threading
from pathlib import Path
from urllib.parse import urlsplit
from playwright.sync_api import expect, sync_playwright

class FixtureHTTPServer(http.server.ThreadingHTTPServer):
    request_queue_size = 64


ROOT = Path(__file__).resolve().parents[3]
OUT = ROOT / '.audit/2026-10-28-read-state'
OUT.mkdir(exist_ok=True)

class Handler(http.server.SimpleHTTPRequestHandler):
    def log_message(self, *_):
        pass

class Fixture:
    def __init__(self):
        self.scope = 'read-owner-a'
        self.gets = 0
    def route(self, route):
        path = urlsplit(route.request.url).path.removeprefix('/app/api')
        if path == '/events':
            route.fulfill(status=200, content_type='text/event-stream', body='')
            return
        data = {}
        if path == '/state':
            data = {'workspaceId': self.scope, 'agents': [{'id':'bot-'+x, 'name':'Chat '+x.upper(), 'status':'idle', 't':'2026-10-28T04:00:00Z', 'av':{'shape':0,'color':4}} for x in ['a','b']], 'pending':[], 'ai':{}, 'secrets':[], 'outbound':{}}
        elif path.endswith('/messages'):
            assert route.request.method == 'GET'
            self.gets += 1
            bot = path.split('/')[2]
            data = [{'id':bot+'-msg', 'agentId':bot, 'role':'agent', 'text':'Message in '+self.scope+' for '+bot, 't':'2026-10-28T04:00:00Z'}]
        route.fulfill(status=200, content_type='application/json', body=json.dumps(data))

def check(label):
    print('PASS '+label, flush=True)

server = FixtureHTTPServer(('127.0.0.1',0),functools.partial(Handler,directory=str(ROOT/'app/native/internal/web/assets')))
threading.Thread(target=server.serve_forever,daemon=True).start()
url=f'http://127.0.0.1:{server.server_port}/app/'
try:
    with sync_playwright() as p:
        chrome=os.environ.get('AURORA_TEST_CHROME')
        if not chrome and Path('C:/Program Files/Google/Chrome/Application/chrome.exe').exists():
            chrome='C:/Program Files/Google/Chrome/Application/chrome.exe'
        browser=p.chromium.launch(headless=True,executable_path=chrome)
        print('Chrome '+browser.version,flush=True)
        try:
            for value in [True,'broken',42,[],{'bot-a':'never','bot-b':{'bad':True}},{'bot-a':1e300,'bot-b':-1},None]:
                context=browser.new_context(viewport={'width':1280,'height':900},locale='vi-VN',service_workers='block')
                raw=json.dumps(value)
                context.add_init_script('localStorage.setItem("neko-aurora-seen",'+json.dumps(raw)+');localStorage.setItem("aurora.seen.v1.read-owner-a",'+json.dumps(raw)+');')
                page=context.new_page()
                fixture,errors=Fixture(),[]
                page.on('pageerror',lambda e:errors.append(str(e)))
                page.route('**/app/api/**',fixture.route)
                page.goto(url+'#bot-a')
                expect(page.locator('#thread')).not_to_have_class('screen thread off')
                expect(page.locator('#messages')).to_contain_text('Message in read-owner-a for bot-a')
                assert fixture.gets >= 1
                assert page.evaluate('localStorage.getItem("neko-aurora-seen")') == raw
                if isinstance(value,dict):
                    stored=page.evaluate('JSON.parse(localStorage.getItem("aurora.seen.v1.read-owner-a"))')
                    assert 'bot-b' not in stored,stored
                page.locator('#backBtn').click()
                expect(page.locator('#thread')).to_have_class('screen thread off')
                assert not errors,errors
                context.close()
            check('malformed legacy and scoped records cannot block chat open, message load or back navigation')

            blocked=browser.new_context(viewport={'width':1280,'height':900},locale='vi-VN',service_workers='block')
            blocked.add_init_script('''{const get=Storage.prototype.getItem,set=Storage.prototype.setItem;for(const [name,original] of [['getItem',get],['setItem',set]])Storage.prototype[name]=function(k,...args){if(k.startsWith('aurora.seen.')||k==='neko-aurora-seen')throw new DOMException('blocked','SecurityError');return original.call(this,k,...args)}}''')
            page=blocked.new_page();fixture,errors=Fixture(),[]
            page.on('pageerror',lambda e:errors.append(str(e)))
            page.route('**/app/api/**',fixture.route)
            page.goto(url+'#bot-a')
            expect(page.locator('#thread')).not_to_have_class('screen thread off')
            expect(page.locator('#messages')).to_contain_text('Message in read-owner-a for bot-a')
            page.locator('#backBtn').click()
            expect(page.locator('#thread')).to_have_class('screen thread off')
            assert not errors,errors
            blocked.close()
            check('storage denied before startup still opens and closes the requested chat')


            context=browser.new_context(viewport={'width':1280,'height':900},locale='vi-VN',service_workers='block')
            page=context.new_page()
            fixture,errors=Fixture(),[]
            page.on('pageerror',lambda e:errors.append(str(e)))
            page.route('**/app/api/**',fixture.route)
            page.goto(url)
            row=lambda bot:page.locator(f'#rows .row[data-id="bot-{bot}"] .rowin')
            dot=lambda bot:row(bot).locator('i.dot')
            expect(row('a')).to_be_visible()
            def toggle(bot,label):
                row(bot).click(button='right')
                item=page.locator('#ctxUnread')
                expect(item).to_contain_text(label)
                page.wait_for_timeout(720) # Existing long-press ghost-click fence.
                item.click()
                expect(page.locator('.ctx')).to_have_count(0)
            toggle('b','Đánh dấu chưa đọc')
            expect(dot('b')).to_have_count(1)
            page.reload()
            expect(row('b')).to_be_visible()
            expect(dot('b')).to_have_count(1)
            check('mark-unread persists after reload')

            fixture.scope='read-owner-b'
            page.evaluate('window.__AURORA.reload()')
            expect(dot('b')).to_have_count(0)
            fixture.scope='read-owner-a'
            page.evaluate('window.__AURORA.reload()')
            expect(dot('b')).to_have_count(1)
            check('same bot ID has independent read markers across workspace A/B/A')
            row('b').press('Enter')
            expect(page.locator('#messages')).to_contain_text('Message in read-owner-a for bot-b')
            page.locator('#backBtn').click()
            expect(dot('b')).to_have_count(0)
            page.reload()
            expect(row('b')).to_be_visible()
            expect(dot('b')).to_have_count(0)
            check('keyboard opening marks the original workspace read and survives reload')

            page.evaluate("""()=>{const get=Storage.prototype.getItem,set=Storage.prototype.setItem;window.fixtureRestoreStorage=()=>{Storage.prototype.getItem=get;Storage.prototype.setItem=set};Storage.prototype.getItem=function(k){if(k.startsWith('aurora.seen.')||k==='neko-aurora-seen')throw new DOMException('blocked','SecurityError');return get.call(this,k)};Storage.prototype.setItem=function(k,v){if(k.startsWith('aurora.seen.')||k==='neko-aurora-seen')throw new DOMException('quota','QuotaExceededError');return set.call(this,k,v)}}""")
            toggle('b','Đánh dấu chưa đọc')
            expect(dot('b')).to_have_count(1)
            fixture.scope='read-owner-b'
            page.evaluate('window.__AURORA.reload()')
            expect(dot('b')).to_have_count(0)
            fixture.scope='read-owner-a'
            page.evaluate('window.__AURORA.reload()')
            expect(dot('b')).to_have_count(1)
            check('storage failures keep independent in-window markers across workspace switches')
            row('b').click()
            expect(page.locator('#messages')).to_contain_text('Message in read-owner-a for bot-b')
            page.locator('#backBtn').click()
            expect(dot('b')).to_have_count(0)
            check('blocked storage does not prevent opening, reading or closing a conversation')
            page.evaluate('window.fixtureRestoreStorage()')
            toggle('b','Đánh dấu chưa đọc')
            toggle('b','Đánh dấu đã đọc')
            page.reload()
            expect(row('b')).to_be_visible()
            expect(dot('b')).to_have_count(0)
            check('context mark-read persists after storage becomes available again')

            page.set_viewport_size({'width':390,'height':844})
            toggle('a','Đánh dấu chưa đọc')
            expect(dot('a')).to_have_count(1)
            page.screenshot(path=str(OUT/'read-mobile-list.png'),animations='disabled')
            row('a').click()
            expect(page.locator('#messages')).to_contain_text('Message in read-owner-a for bot-a')
            page.wait_for_function("()=>{const r=document.querySelector('#thread').getBoundingClientRect();return r.left>=0&&r.right<=innerWidth+.5}")
            page.screenshot(path=str(OUT/'read-mobile-chat.png'),animations='disabled')
            page.set_viewport_size({'width':1280,'height':900})
            page.screenshot(path=str(OUT/'read-desktop-chat.png'),animations='disabled')
            assert not errors,errors
            check('mobile list, context action and conversation remain usable without overflow')
            context.close()
        except Exception:
            print('READ DIAGNOSTICS',json.dumps({'errors':errors,'page':page.evaluate('''()=>({url:location.href,body:document.body.innerText.slice(-1000),rows:document.querySelectorAll('#rows .row').length,state:window.__AURORA?.state()})''')},ensure_ascii=True),flush=True)
            page.screenshot(path=str(OUT/'read-failure.png'))
            raise
        finally:
            browser.close()
finally:
    server.shutdown()
    server.server_close()
