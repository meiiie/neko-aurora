"""Chat identity refresh with real EventSource reconnects and synthetic API state."""
import functools
import http.server
import json
import os
import threading
from pathlib import Path
from urllib.parse import urlsplit
from playwright.sync_api import expect, sync_playwright

ROOT=Path(__file__).resolve().parents[3]
OUT=ROOT/'.audit/2026-10-29-chat-identity'
OUT.mkdir(parents=True,exist_ok=True)


class Server(http.server.ThreadingHTTPServer):
    request_queue_size=64


class Handler(http.server.SimpleHTTPRequestHandler):
    def log_message(self,*_):pass


server=Server(('127.0.0.1',0),functools.partial(Handler,directory=str(ROOT/'app/native/internal/web/assets')))
threading.Thread(target=server.serve_forever,daemon=True).start()
try:
    with sync_playwright() as p:
        browser=p.chromium.launch(headless=True,executable_path=os.environ.get('AURORA_TEST_CHROME','C:/Program Files/Google/Chrome/Application/chrome.exe'))
        try:
            page=browser.new_page(viewport={'width':390,'height':844},locale='vi-VN',service_workers='block',reduced_motion='reduce')
            errors,posts=[],[];events=0;scope='identity-owner-a'
            agents=[{'id':'bot-a','name':'Tên cũ','status':'idle','av':{}},{'id':'group-a','name':'Nhóm cũ','kind':'group','members':['bot-a'],'status':'idle'}]
            page.on('pageerror',lambda e:errors.append(str(e)))

            def api(route):
                global events
                path=urlsplit(route.request.url).path
                if route.request.method!='GET':posts.append(path)
                if path.endswith('/events'):
                    events+=1
                    route.fulfill(content_type='text/event-stream',body='')
                    return
                if path.endswith('/state'):data={'workspaceId':scope,'agents':agents,'pending':[],'ai':{},'secrets':[]}
                elif path.endswith('/messages'):data=[]
                else:data={}
                route.fulfill(json=data)

            page.route('**/app/api/**',api)
            page.goto(f'http://127.0.0.1:{server.server_port}/app/#bot-a',wait_until='networkidle')
            header=page.locator('#threadName');draft=page.locator('#input')
            expect(header).to_have_text('Tên cũ');draft.fill('Nháp đang soạn')
            agents[0]['name']='Tên đã đổi'
            agents[0]['av']={'shape':4,'color':5}
            page.evaluate('window.__AURORA.reload()')
            expect(header).to_have_text('Tên đã đổi')
            expect(page.locator('#threadAvatar svg g')).to_have_attribute('fill','#3b7be0')
            expect(draft).to_have_attribute('placeholder','Hỏi Tên đã đổi')
            expect(draft).to_have_value('Nháp đang soạn');expect(draft).to_be_focused()
            print('PASS state refresh updates header and composer identity without losing draft or focus')

            before=events;agents[0]['name']='Sau kết nối lại'
            expect(header).to_have_text('Sau kết nối lại',timeout=10000)
            assert events>before
            expect(draft).to_have_attribute('placeholder','Hỏi Sau kết nối lại')
            expect(draft).to_have_value('Nháp đang soạn')
            page.screenshot(path=str(OUT/'chat-identity-mobile.png'))
            print('PASS real EventSource reconnect refreshes the visible conversation name')

            with page.expect_navigation(wait_until='load'):page.evaluate('window.__I18N.set("en")')
            expect(draft).to_have_value('Nháp đang soạn')
            agents[0]['name']='Sáng';page.evaluate('window.__AURORA.reload()')
            expect(header).to_have_text('Sáng')
            expect(draft).to_have_attribute('placeholder','Ask Sáng')
            assert page.locator('#threadName img').count()==0
            print('PASS English chrome preserves the renamed bot literal')

            page.locator('#backBtn').click();page.locator('#rows .row[data-id="group-a"] .rowin').click()
            expect(header).to_have_text('Nhóm cũ');draft.fill('Nháp nhóm')
            agents[1]['name']='Nhóm <img src=x onerror=alert(1)>'
            page.evaluate('window.__AURORA.reload()')
            expect(header).to_have_text(agents[1]['name'])
            expect(draft).to_have_attribute('placeholder','Ask the group')
            expect(draft).to_have_value('Nháp nhóm')
            assert page.locator('#threadName img').count()==0
            page.set_viewport_size({'width':1280,'height':900})
            page.screenshot(path=str(OUT/'chat-identity-desktop-en.png'))
            print('PASS renamed group stays literal and keeps its group composer prompt')
            def check_faces(count):
                expect(page.locator('.thread-empty .te-face svg')).to_have_count(count)
                bounds=page.locator('.thread-empty').evaluate("n=>({frame:n.querySelector('.te-face').getBoundingClientRect().toJSON(),title:n.querySelector('b').getBoundingClientRect().toJSON(),faces:[...n.querySelectorAll('.te-face svg')].map(e=>e.getBoundingClientRect().toJSON())})")
                assert bounds['frame']['height']>=48,bounds
                assert all(b['bottom']<=bounds['title']['top'] and b['left']>=bounds['frame']['left']-.5 and b['right']<=bounds['frame']['right']+.5 for b in bounds['faces']),bounds
            check_faces(1)
            agents.extend([{'id':'bot-'+x,'name':'Member '+x,'status':'idle','av':{'shape':n+1,'color':n+2}} for n,x in enumerate(['b','c'])])
            agents[1]['members']=['bot-a','bot-b','bot-c']
            page.evaluate('window.__AURORA.reload()')
            check_faces(3)
            expect(page.locator('#threadAvatar svg')).to_have_count(2)
            page.screenshot(path=str(OUT/'chat-identity-desktop-en.png'))
            page.set_viewport_size({'width':390,'height':844})
            check_faces(3)
            page.screenshot(path=str(OUT/'group-identity-mobile.png'))
            print('PASS one and three group avatars occupy their own row above the title')
            def check_header():
                bounds=page.evaluate("()=>({width:innerWidth,back:document.querySelector('#backBtn').getBoundingClientRect().toJSON(),who:document.querySelector('#whoBtn').getBoundingClientRect().toJSON(),more:document.querySelector('#threadMore').getBoundingClientRect().toJSON()})")
                assert bounds['back']['left']>=0 and bounds['more']['right']<=bounds['width']+.5,bounds
                assert bounds['who']['left']>=bounds['back']['right']-.5 and bounds['who']['right']<=bounds['more']['left']+.5,bounds
            check_header()
            agents[1]['name']='W'*40
            page.evaluate('window.__AURORA.reload()')
            expect(header).to_have_text('W'*40)
            check_header();check_faces(3)
            assert page.locator('#messages').evaluate('e=>e.scrollWidth<=e.clientWidth+1')
            page.screenshot(path=str(OUT/'group-identity-long-mobile.png'))
            print('PASS long names keep both header controls visible and wrap within the conversation')



            scope='identity-owner-b';agents[0]['name']='Workspace B'
            page.evaluate('window.__AURORA.reload()')
            expect(page.locator('#thread')).to_have_class('screen thread off')
            page.locator('#rows .row[data-id="bot-a"] .rowin').click()
            expect(header).to_have_text('Workspace B');expect(draft).to_have_value('')
            assert not posts and not errors,errors
            print('PASS workspace replacement closes the previous chat and isolates its draft')
        except Exception:
            print('IDENTITY DIAGNOSTICS',json.dumps({'errors':errors,'events':events,'posts':posts}))
            page.screenshot(path=str(OUT/'chat-identity-failure.png'))
            raise
        finally:browser.close()
finally:
    server.shutdown();server.server_close()
