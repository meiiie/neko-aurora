"""Native reply selections become Markdown; the clipboard stays inside the fixture."""
import functools
import http.server
import json
import os
import re
import threading
from pathlib import Path
from urllib.parse import urlsplit
from playwright.sync_api import expect, sync_playwright
from fixture_http import FixtureHTTPServer

ROOT=Path(__file__).resolve().parents[3]
OUT=ROOT/'.audit/2026-10-29-selection-copy'
OUT.mkdir(parents=True,exist_ok=True)
TEXT='''# Kế hoạch **Aurora**

Giữ **tiếng Việt 👩🏽‍💻** và *nhẹ nhàng*, ~~bỏ qua~~ [tài liệu](https://example.org/guide?q=1&lang=vi).

- Cha
  - Con **đậm**
- [x] Đã xong
- [ ] Còn lại

3. Bước ba
4. Bước bốn
5. Bước năm

> Một **trích dẫn**
>
> Dòng sau

````js
const text = "```";
  giữ khoảng trắng



cuối
````

Mã ``a`b`` và `<script>`.

| Tên | Giá |
|---|---:|
| A | 12 |
| B | 23 |

\\*nguyên văn\\* và <img onerror=alert(1)>.

1\\. không thành danh sách &copy;
'''


class Handler(http.server.SimpleHTTPRequestHandler):
    def log_message(self,*_):pass


server=FixtureHTTPServer(('127.0.0.1',0),functools.partial(Handler,directory=str(ROOT/'app/native/internal/web/assets')))
threading.Thread(target=server.serve_forever,daemon=True).start()
try:
    with sync_playwright() as p:
        browser=p.chromium.launch(headless=True,executable_path=os.environ.get('AURORA_TEST_CHROME','C:/Program Files/Google/Chrome/Application/chrome.exe'))
        try:
            context=browser.new_context(viewport={'width':390,'height':844},locale='vi-VN',service_workers='block',reduced_motion='reduce')
            page=context.new_page();scope='selection-copy-a';errors=[];posts=[]
            page.on('pageerror',lambda e:errors.append(str(e)))
            def api(route):
                path=urlsplit(route.request.url).path
                if route.request.method!='GET':posts.append(path)
                if path.endswith('/events'):route.fulfill(content_type='text/event-stream',body='');return
                if path.endswith('/state'):data={'workspaceId':scope,'agents':[{'id':'bot-'+x,'name':'Chat '+x,'status':'idle','av':{}} for x in ['a','b']],'pending':[],'ai':{},'secrets':[]}
                elif path.endswith('/messages'):data=[{'id':'reply','taskId':'reply','agentId':'bot-a','role':'agent','text':TEXT,'t':'2026-10-29T06:00:00Z'},{'id':'owner','taskId':'owner','agentId':'bot-a','role':'user','text':'Owner **literal**','t':'2026-10-29T06:01:00Z'}] if 'bot-a' in path else []
                else:data={}
                route.fulfill(json=data)
            page.route('**/app/api/**',api)
            page.goto(f'http://127.0.0.1:{server.server_port}/app/#bot-a',wait_until='networkidle')
            body=page.locator('[data-message-id=reply] .msg')
            expect(body).to_be_visible()
            page.locator('#input').fill('Nháp không đổi')
            def select(selector, start=None, end=None):
                node=body.locator(selector) if selector else body
                node.evaluate('''(n,o)=>{document.activeElement?.blur();const r=document.createRange();
                  if(o.start===null)r.selectNodeContents(n);else{r.setStart(n.firstChild,o.start);r.setEnd(n.firstChild,o.end)}
                  getSelection().removeAllRanges();getSelection().addRange(r)}''',{'start':start,'end':end})
            def copy(target='body'):
                return page.locator(target).evaluate('''n=>{const data=new DataTransfer();const e=new ClipboardEvent('copy',{clipboardData:data,bubbles:true,cancelable:true});n.dispatchEvent(e);return {handled:e.defaultPrevented,text:data.getData('text/plain'),types:[...data.types]}}''')
            select(':scope > p strong',0,5)
            result=copy()
            assert result=={'handled':True,'text':'**tiếng**','types':['text/plain']},result
            print('PASS partial bold selection copies only selected text as Markdown')
            select('h3')
            assert copy()['text']=='# Kế hoạch **Aurora**'
            select(':scope > p:first-of-type')
            assert copy()['text']=='Giữ **tiếng Việt 👩🏽‍💻** và *nhẹ nhàng*, ~~bỏ qua~~ [tài liệu](https://example.org/guide?q=1&lang=vi).'
            print('PASS headings links emphasis and Unicode keep their rendered meaning')
            select(':scope > ul:first-of-type')
            assert copy()['text']=='- Cha\n  - Con **đậm**\n- [x] Đã xong\n- [ ] Còn lại',copy()
            select('ol li:nth-child(2)')
            assert copy()['text']=='4. Bước bốn',copy()
            select('blockquote')
            assert copy()['text']=='> Một **trích dẫn**\n>\n> Dòng sau',copy()
            print('PASS nested task lists ordered subsets and quotes retain structure')
            select('pre code')
            assert copy()['text']=='````js\nconst text = "```";\n  giữ khoảng trắng\n\n\n\ncuối\n````',copy()
            select('p code:first-child')
            assert copy()['text']=='``a`b``',copy()
            print('PASS code fences and spans preserve literal backticks and indentation')
            select('.table-card')
            assert copy()['text']=='| Tên | Giá |\n| --- | --- |\n| A | 12 |\n| B | 23 |',copy()
            select('tbody tr:first-child td:last-child')
            assert copy()['text']=='12',copy()
            print('PASS full tables copy as Markdown and partial cells exclude unselected cells')
            select(':scope > p:nth-last-child(2)')
            assert copy()['text']=='\\*nguyên văn\\* và \\<img onerror=alert(1)\\>.',copy()
            select(':scope > p:last-child')
            assert copy()['text']=='1\\. không thành danh sách \\&copy;',copy()
            select('.code-h')
            assert copy()['handled'] is False
            select('.table-actions')
            assert copy()['handled'] is False
            print('PASS literal markup stays escaped and controls are not copied as reply content')
            select(':scope > p strong')
            page.evaluate('''window.keyboardCopies=[];document.addEventListener('copy',e=>{if(e.isTrusted){keyboardCopies.push({handled:e.defaultPrevented,text:e.clipboardData.getData('text/plain')});e.preventDefault()}})''')
            page.keyboard.press('Control+c')
            assert page.evaluate('keyboardCopies')==[{'handled':True,'text':'**tiếng Việt 👩🏽‍💻**'}]
            page.screenshot(path=str(OUT/'selection-copy-mobile.png'))
            page.locator('#input').focus();page.locator('#input').evaluate('n=>n.select()')
            page.keyboard.press('Control+c')
            assert page.evaluate('keyboardCopies.at(-1).handled') is False
            expect(page.locator('#input')).to_have_value('Nháp không đổi')
            print('PASS trusted keyboard copy uses the same handler without intercepting composer copying')
            body.locator('h3 strong').dblclick()
            assert page.evaluate('getSelection().toString()')=='Aurora'
            page.keyboard.press('Control+c')
            assert page.evaluate('keyboardCopies.at(-1)')=={'handled':True,'text':'# **Aurora**'}
            print('PASS mouse-selected content reaches native keyboard Copy with heading context')
            page.locator('[data-message-id=owner] .msg').evaluate('''n=>{const r=document.createRange();r.selectNodeContents(n);getSelection().removeAllRanges();getSelection().addRange(r)}''')
            assert copy()['handled'] is False
            body.evaluate('''n=>{const r=document.createRange();r.setStart(n,0);r.setEnd(document.querySelector('[data-message-id=owner] .msg'),1);getSelection().removeAllRanges();getSelection().addRange(r)}''')
            assert copy()['handled'] is False
            page.locator('#threadName').evaluate('''n=>{const r=document.createRange();r.selectNodeContents(n);getSelection().removeAllRanges();getSelection().addRange(r)}''')
            assert copy()['handled'] is False
            page.evaluate('getSelection().removeAllRanges()')
            assert copy()['handled'] is False
            print('PASS owner cross-message header and empty selections retain browser defaults')
            select('h3');page.evaluate('window.oldReply=document.querySelector("[data-message-id=reply] .msg")')
            page.locator('#backBtn').click();page.locator('#rows .row[data-id=bot-b] .rowin').click()
            assert copy()['handled'] is False
            page.locator('#backBtn').click();page.locator('#rows .row[data-id=bot-a] .rowin').click()
            select('h3');scope='selection-copy-b';page.evaluate('window.__AURORA.reload()')
            expect(page.locator('#thread')).to_have_class(re.compile(r'\boff\b'))
            assert copy()['handled'] is False
            page.locator('#rows .row[data-id=bot-a] .rowin').click()
            expect(page.locator('#input')).to_have_value('')
            print('PASS chat and workspace changes cannot copy a retained old range')
            with page.expect_navigation(wait_until='load'):page.evaluate('window.__I18N.set("en")')
            page.set_viewport_size({'width':1280,'height':900})
            expect(body).to_be_visible();select('h3');assert copy()['text']=='# Kế hoạch **Aurora**'
            page.screenshot(path=str(OUT/'selection-copy-desktop-en.png'))
            assert not posts and not errors,errors
            print('PASS English desktop leaves owner content literal with no API mutations or page errors')
        except Exception:
            print('SELECTION COPY DIAGNOSTICS',json.dumps({'errors':errors,'posts':posts}))
            page.screenshot(path=str(OUT/'selection-copy-failure.png'))
            raise
        finally:browser.close()
finally:server.shutdown();server.server_close()
