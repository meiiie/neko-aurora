"""Formatted reply copying with real ClipboardItem payloads and a synthetic sink."""
import functools
import http.server
import json
import os
import re
import threading
from pathlib import Path
from urllib.parse import urlsplit
from playwright.sync_api import expect, sync_playwright

ROOT=Path(__file__).resolve().parents[3]
OUT=ROOT/'.audit/2026-10-29-reply-copy'
OUT.mkdir(parents=True,exist_ok=True)
TEXT='# Kế hoạch **Aurora**\n\n- Giữ tiếng Việt 👩🏽‍💻\n- [Tài liệu](https://example.org/guide?q=1&lang=vi)\n\n3. Bước ba\n4. Bước bốn\n\n> Một trích dẫn\n\n```js\nconst safe = "<img onerror=alert(1)>";\n```\n\n| Tên | Giá |\n|---|---:|\n| A | 12 |\n\n<script>alert(1)</script>\n\n[Không chạy](javascript:alert(1))'


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
            context=browser.new_context(viewport={'width':390,'height':844},locale='vi-VN',service_workers='block',reduced_motion='reduce')
            context.add_init_script('''
              window.clip={calls:0,items:[],mode:'ok',resolve:null,done:false};
              window.originalClipboardItem=window.ClipboardItem;
              Object.defineProperty(navigator,'clipboard',{value:{
                async write(items){clip.calls++;clip.done=false;if(clip.mode==='deny')throw new DOMException('fixture','NotAllowedError');
                  if(clip.mode==='hold')await new Promise(resolve=>clip.resolve=resolve);
                  const record={};for(const type of items[0].types)record[type]=await (await items[0].getType(type)).text();clip.items.push(record);clip.done=true},
                async writeText(text){clip.calls++;clip.items.push({'text/plain':text});clip.done=true}
              },configurable:true});
            ''')
            page=context.new_page();scope='copy-owner-a';errors=[];posts=[]
            page.on('pageerror',lambda e:errors.append(str(e)))

            def api(route):
                path=urlsplit(route.request.url).path
                if route.request.method!='GET':posts.append(path)
                if path.endswith('/events'):route.fulfill(content_type='text/event-stream',body='');return
                if path.endswith('/state'):data={'workspaceId':scope,'agents':[{'id':'bot-'+x,'name':'Chat '+x,'status':'idle','av':{}} for x in ['a','b']],'pending':[],'ai':{},'secrets':[]}
                elif path.endswith('/messages'):data=[{'id':'reply','taskId':'reply','agentId':'bot-a','role':'agent','text':TEXT,'t':'2026-10-29T04:00:00Z'}] if 'bot-a' in path else []
                else:data={}
                route.fulfill(json=data)

            page.route('**/app/api/**',api)
            page.goto(f'http://127.0.0.1:{server.server_port}/app/#bot-a',wait_until='networkidle')
            button=page.get_by_role('button',name='Sao chép câu trả lời',exact=True)
            expect(button).to_be_visible()
            page.locator('#input').fill('Nháp không được đổi')
            button.click()
            page.wait_for_function('clip.items.length===1')
            copied=page.evaluate('clip.items[0]')
            assert copied['text/plain']==TEXT
            assert page.evaluate('''html=>{const d=new DOMParser().parseFromString(html,'text/html');return d.querySelector('h3 strong')?.textContent==='Aurora' && d.querySelectorAll('ul li').length===2 && d.querySelector('ol')?.getAttribute('start')==='3' && d.querySelector('blockquote') && d.querySelector('pre code') && d.querySelectorAll('table tbody tr').length===1 && d.querySelector('a')?.getAttribute('href')==='https://example.org/guide?q=1&lang=vi'}''',copied['text/html'])
            expect(page.locator('#input')).to_have_value('Nháp không được đổi')
            assert not posts
            print('PASS rich copy preserves markdown, headings, lists, links, quotes, code and tables')
            assert page.evaluate('''html=>{const d=new DOMParser().parseFromString(html,'text/html');return !d.querySelector('script,img,button,input,style,svg,iframe') && [...d.querySelectorAll('*')].every(e=>[...e.attributes].every(a=>!a.name.startsWith('on')&&!['class','id','style'].includes(a.name))) && [...d.links].every(a=>['https:','http:','mailto:'].includes(a.protocol))}''',copied['text/html'])
            assert 'Sao chép bảng' not in copied['text/html'] and 'Sao chép mã' not in copied['text/html']
            assert '&lt;script&gt;' in copied['text/html']
            print('PASS clipboard HTML excludes UI controls and executable markup')
            button.focus();page.keyboard.press('Enter')
            page.wait_for_function('clip.items.length===2')
            expect(button).to_be_focused()
            page.screenshot(path=str(OUT/'reply-copy-mobile.png'))
            print('PASS keyboard copying keeps focus and works in the mobile viewport')
            page.evaluate('window.ClipboardItem=undefined')
            button.click();page.wait_for_function('clip.items.length===3')
            assert page.evaluate('clip.items[2]')=={'text/plain':TEXT}
            page.evaluate('()=>{window.ClipboardItem=originalClipboardItem}')
            print('PASS browsers without rich clipboard support receive exact markdown')
            page.evaluate('clip.mode="deny"')
            button.click()
            expect(page.locator('#toast')).to_contain_text('Không sao chép được')
            expect(button).to_be_enabled()
            assert page.evaluate('clip.items.length')==3
            print('PASS permission denial reports failure without a second clipboard write')
            page.evaluate('clip.mode="hold";document.querySelector("#toast").classList.remove("show")')
            before=page.evaluate('clip.calls')
            button.click();expect(button).to_be_disabled()
            button.evaluate('b=>b.click()')
            assert page.evaluate('clip.calls')==before+1
            page.locator('#backBtn').click();page.locator('#rows .row[data-id="bot-b"] .rowin').click()
            page.evaluate('clip.resolve()');page.wait_for_function('clip.done')
            expect(page.locator('#toast')).not_to_have_class(re.compile(r'\bshow\b'))
            expect(page.locator('#input')).to_have_value('')
            print('PASS pending copy is single-flight and cannot notify a different chat')
            page.locator('#backBtn').click();page.locator('#rows .row[data-id="bot-a"] .rowin').click()
            expect(button).to_be_enabled();expect(page.locator('#input')).to_have_value('Nháp không được đổi')
            button.click();expect(button).to_be_disabled()
            scope='copy-owner-b';page.evaluate('window.__AURORA.reload()')
            page.evaluate('clip.resolve()');page.wait_for_function('clip.done')
            expect(page.locator('#toast')).not_to_have_class(re.compile(r'\bshow\b'))
            print('PASS late completion cannot notify the next workspace')
            page.locator('#rows .row[data-id="bot-a"] .rowin').click()
            with page.expect_navigation(wait_until='load'):page.evaluate('window.__I18N.set("en")')
            page.set_viewport_size({'width':1280,'height':900})
            english=page.get_by_role('button',name='Copy reply',exact=True)
            expect(english).to_be_visible();english.click();page.wait_for_function('clip.items.length===1')
            expect(page.locator('#toast')).to_contain_text('Copied')
            page.screenshot(path=str(OUT/'reply-copy-desktop-en.png'))
            assert not posts and not errors,errors
            print('PASS English desktop copy uses the same literal content without browser errors')
        except Exception:
            print('COPY DIAGNOSTICS',json.dumps({'errors':errors,'posts':posts}))
            page.screenshot(path=str(OUT/'reply-copy-failure.png'))
            raise
        finally:browser.close()
finally:
    server.shutdown();server.server_close()
