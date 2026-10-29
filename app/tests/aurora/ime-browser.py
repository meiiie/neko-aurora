"""IME event-order fixtures against the real UI; no OS IME/provider claims."""
import functools
import http.server
import json
import os
import threading
from pathlib import Path
from urllib.parse import urlsplit
from playwright.sync_api import expect, sync_playwright
from fixture_http import FixtureHTTPServer

ROOT=Path(__file__).resolve().parents[3]
OUT=ROOT/'.audit/2026-10-29-ime-input';OUT.mkdir(exist_ok=True)

class Handler(http.server.SimpleHTTPRequestHandler):
    def log_message(self,*_):pass

class Fixture:
    def __init__(self,kind):
        self.kind=kind;self.posts=[];self.messages=[]
        if kind!='composer':
            m=dict(id='request',taskId='request-task',agentId='bot-a',role='agent',text='',t='2026-10-29T04:00:00Z')
            if kind=='choice':m['choices']=['Option A','Option B']
            if kind=='form':m['form']=dict(answerVersion=1,title='Fixture form',fields=[dict(name='answer',label='Answer',type='text',required=True)])
            if kind=='question':m['questions']=[dict(id='q1',prompt='Choose one or write an answer',options=['Option A','Option B'],multiple=False)]
            self.messages=[m]
    def route(self,r):
        q=r.request;path=urlsplit(q.url).path.removeprefix('/app/api');data={}
        if path=='/state':data=dict(workspaceId='ime-owner',agents=[dict(id='bot-a',name='IME fixture',status='idle',av=dict(shape=0,color=4))],pending=[],ai={},secrets=[],outbound={})
        elif path.endswith('/messages'):
            if q.method=='GET':data=self.messages
            else:
                b=q.post_data_json;self.posts.append(b);data=dict(id=b['clientNonce']+':u',taskId='answer',agentId='bot-a',role='user',text=b['text'],t='2026-10-29T04:01:00Z')
                for k in ['answerFor','formFor','formValues']:data[k]=b.get(k)
                self.messages.append(data)
        elif path.endswith('/ext'):data=dict(workspaceId='ime-owner',menu=[],allSkills=[],mcp=[],mcpRevision=0)
        r.fulfill(status=202 if q.method=='POST' and path.endswith('/messages') else 200,content_type='application/json',body=json.dumps(data))

server=FixtureHTTPServer(('127.0.0.1',0),functools.partial(Handler,directory=str(ROOT/'app/native/internal/web/assets')))
threading.Thread(target=server.serve_forever,daemon=True).start()
errors=[]
try:
    with sync_playwright() as p:
        browser=p.chromium.launch(headless=True,executable_path=os.environ.get('AURORA_TEST_CHROME') or 'C:/Program Files/Google/Chrome/Application/chrome.exe')
        def setup(kind):
            ctx=browser.new_context(viewport=dict(width=390,height=844),locale='vi-VN',service_workers='block',reduced_motion='reduce')
            ctx.add_init_script('window.EventSource=class{addEventListener(){}close(){}}')
            page=ctx.new_page();fx=Fixture(kind);page.on('pageerror',lambda e:errors.append(str(e)));page.route('**/app/api/**',fx.route)
            page.goto(f'http://127.0.0.1:{server.server_port}/app/#bot-a',wait_until='networkidle');expect(page.locator('#whoBtn')).to_contain_text('IME fixture')
            selector={'choice':'.choice-custom input','form':'.form-card input','composer':'#input','question':'.question-card'}[kind]
            inp=page.locator(selector);expect(inp).to_be_visible();return ctx,page,fx,inp
        def composition(inp,phase):inp.dispatch_event('composition'+phase,dict(data='日本語'))
        def count_posts(page,fx,n):
            page.wait_for_timeout(100);assert len(fx.posts)==n,(n,fx.posts)
        def form(inp):return inp.locator('xpath=ancestor::form[1]')
        def done(page,fx,text):
            expect(page.locator('.message-item.mine .msg').last).to_contain_text(text)
            count_posts(page,fx,1);assert fx.posts[0]['text'].endswith(text)
        try:
            for kind in ['choice','form','composer']:
                ctx,page,fx,inp=setup(kind);text='日本語 · Tiếng Việt';inp.fill(text);composition(inp,'start')
                # Synthetic composition does not activate the OS IME. A trusted textarea Enter would insert a newline.
                if kind=='composer':inp.dispatch_event('keydown',dict(key='Enter',code='Enter'))
                else:inp.press('Enter')
                count_posts(page,fx,0);expect(inp).to_have_value(text);expect(inp).to_be_focused()
                form(inp).evaluate('n=>n.requestSubmit()');count_posts(page,fx,0)
                composition(inp,'end');inp.press('Enter');done(page,fx,text)
                if kind=='choice':assert fx.posts[0]['answerFor']=='request-task'
                if kind=='form':assert fx.posts[0]['formValues']==[text]
                print('PASS '+kind+' composition state preserves draft and ordinary Enter submits once');ctx.close()
            for kind in ['choice','form','composer']:
                for marker in ['isComposing','keyCode']:
                    ctx,page,fx,inp=setup(kind);text='韓国語 と Việt';inp.fill(text)
                    # The marker catches boundary events even when compositionstart/end arrive out of order.
                    composition(inp,'start');composition(inp,'end')
                    prevented=inp.evaluate('''(n,marker)=>{const e=new KeyboardEvent('keydown',{key:'Enter',code:'Enter',bubbles:true,cancelable:true,...(marker==='isComposing'?{isComposing:true}:{keyCode:229})});n.dispatchEvent(e);n.form.requestSubmit();return e.defaultPrevented}''',marker)
                    count_posts(page,fx,0);expect(inp).to_have_value(text);assert not prevented,'IME keydown must keep its native text-commit action'
                    inp.dispatch_event('keyup',dict(key='Enter',code='Enter'));inp.press('Enter');done(page,fx,text);ctx.close()
                print('PASS '+kind+' composing flags and keyCode229 suppress only the composition submission')
            ctx,page,fx,inp=setup('composer');inp.fill('First');inp.press('Shift+Enter');inp.type('Second');expect(inp).to_have_value('First\nSecond');count_posts(page,fx,0)
            composition(inp,'start');page.locator('#whoBtn').focus();form(inp).evaluate('n=>n.requestSubmit()');done(page,fx,'First\nSecond')
            print('PASS Shift+Enter keeps newlines and leaving the field does not leave submission locked');ctx.close()
            ctx,page,fx,inp=setup('composer');inp.fill('Nháp 日本語');composition(inp,'start');page.reload(wait_until='networkidle');expect(inp).to_have_value('Nháp 日本語');inp.press('Enter');done(page,fx,'Nháp 日本語')
            print('PASS reload restores composer text without persisting an obsolete composition lock');ctx.close()
            ctx,page,fx,card=setup('question');card.get_by_role('button',name='Câu trả lời riêng',exact=True).click();inp=card.locator('textarea');inp.fill('Tự chọn 日本語');composition(inp,'start');inp.dispatch_event('keydown',dict(key='Enter',code='Enter',isComposing=True));count_posts(page,fx,0);expect(inp).to_have_value('Tự chọn 日本語');composition(inp,'end');inp.press('Enter');inp.type('Dòng hai')
            page.reload(wait_until='networkidle');expect(card.locator('textarea')).to_have_value('Tự chọn 日本語\nDòng hai');card.get_by_role('button',name='Xem lại',exact=True).click();expect(card).to_contain_text('Tự chọn 日本語');card.get_by_role('button',name='Quay lại',exact=True).click();expect(card.locator('textarea')).to_have_value('Tự chọn 日本語\nDòng hai')
            card.get_by_role('button',name='Xem lại',exact=True).click();page.screenshot(path=str(OUT/'own-answer-mobile.png'));card.get_by_role('button',name='Gửi câu trả lời',exact=True).click();count_posts(page,fx,1)
            assert fx.posts[0]['questionAnswers']==[dict(id='q1',selected=[],text='Tự chọn 日本語\nDòng hai')]
            print('PASS single-choice own answers keep Unicode newlines review and reload before explicit send');ctx.close()
            ctx,page,fx,inp=setup('choice');inp.fill('Keyboard button');composition(inp,'start');composition(inp,'end');inp.press('Tab');page.keyboard.press('Enter');done(page,fx,'Keyboard button')
            print('PASS keyboard activation of the submit button still works after composition');ctx.close()
            assert not errors,errors
        finally:browser.close()
finally:server.shutdown();server.server_close()
