"""Real Chrome composer lifecycle with injected recognition events; no live microphone."""
import functools,http.server,json,os,threading
from pathlib import Path
from urllib.parse import urlsplit
from playwright.sync_api import expect,sync_playwright

ROOT=Path(__file__).resolve().parents[3]
OUT=ROOT/'.audit/2026-10-28-dictation';OUT.mkdir(exist_ok=True)
PNG=__import__('base64').b64decode('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jH1cAAAAASUVORK5CYII=')
class Server(http.server.ThreadingHTTPServer):request_queue_size=64
class Handler(http.server.SimpleHTTPRequestHandler):
    def log_message(self,*_):pass
class Fixture:
    def __init__(self):self.scope='voice-owner-a';self.posts=[]
    def route(self,r):
        req=r.request;path=urlsplit(req.url).path.removeprefix('/app/api');data={}
        if path=='/state':data={'workspaceId':self.scope,'agents':[{'id':'bot-'+i,'name':'Chat '+i.upper(),'status':'idle','t':'2026-10-28T04:00:00Z','av':{'shape':0,'color':4}} for i in ['a','b']],'pending':[],'ai':{},'secrets':[],'outbound':{}}
        elif path.endswith('/messages'):
            if req.method=='GET':data=[]
            else:
                body=req.post_data_json;self.posts.append(body)
                data={'id':body['clientNonce']+':u','agentId':path.split('/')[2],'role':'user','text':body['text'],'t':'2026-10-28T04:01:00Z'}
        elif path.endswith('/computer/upload'):data={'path':'uploads/voice-fixture.png'}
        r.fulfill(status=200,content_type='application/json',body=json.dumps(data))

SPEECH='''window.fixtureRecs=[];window.SpeechRecognition=class{constructor(){if(window.voiceConstructorThrows)throw Error('fixture constructor failed');fixtureRecs.push(this);this.stops=0}start(){if(window.voiceStartThrows)throw Error('fixture start failed')}stop(){this.stops++;if(window.syncFinal)this.onresult?.({results:[[{transcript:window.syncFinal}]]});this.onend?.()}emit(words){this.onresult?.({results:words.map(([text,final])=>Object.assign([{transcript:text}],{isFinal:final}))})}}'''
EVENTS='window.EventSource=class{constructor(){this.readyState=1}addEventListener(){}close(){}}'
def check(label):print('PASS '+label,flush=True)
server=Server(('127.0.0.1',0),functools.partial(Handler,directory=str(ROOT/'app/native/internal/web/assets')))
threading.Thread(target=server.serve_forever,daemon=True).start()
try:
    with sync_playwright() as p:
        chrome=os.environ.get('AURORA_TEST_CHROME')
        if not chrome and Path('C:/Program Files/Google/Chrome/Application/chrome.exe').exists():chrome='C:/Program Files/Google/Chrome/Application/chrome.exe'
        browser=p.chromium.launch(headless=True,executable_path=chrome)
        try:
            context=browser.new_context(viewport={'width':1280,'height':900},locale='vi-VN',service_workers='block',reduced_motion='reduce')
            context.add_init_script(EVENTS+';'+SPEECH)
            page=context.new_page();fx=Fixture();errors=[]
            page.on('pageerror',lambda e:errors.append(str(e)));page.route('**/app/api/**',fx.route)
            page.goto(f'http://127.0.0.1:{server.server_port}/app/#bot-a',wait_until='networkidle')
            inp=page.locator('#input');mic=page.locator('#micBtn')
            expect(page.locator('#threadName')).to_have_text('Chat A')
            inp.fill('Bản nháp');mic.click()
            assert page.evaluate('fixtureRecs.at(-1).lang')=='vi-VN'
            page.evaluate('fixtureRecs.at(-1).emit([["xin chào",false]])');expect(inp).to_have_value('Bản nháp xin chào')
            page.evaluate('fixtureRecs.at(-1).emit([["xin chào ",true],["Aurora",false]])');expect(inp).to_have_value('Bản nháp xin chào Aurora')
            page.evaluate('fixtureRecs.at(-1).emit([["xin chào Aurora",true]])');expect(inp).to_have_value('Bản nháp xin chào Aurora')
            assert not fx.posts
            check('interim and final results replace their own segment without duplicate prefix or implicit send')
            inp.fill('Nội dung tôi tự sửa')
            page.evaluate('fixtureRecs.at(-1).emit([["cũ đến muộn",true]])');expect(inp).to_have_value('Nội dung tôi tự sửa')
            expect(mic).to_have_attribute('aria-pressed','false');assert page.evaluate('fixtureRecs.at(-1).stops')==1
            check('manual edits end dictation and survive late recognition results')
            mic.click();page.evaluate('fixtureRecs.at(-1).emit([["giọng nói",false]])')
            page.locator('#imgPick').set_input_files({'name':'voice.png','mimeType':'image/png','buffer':PNG})
            expect(inp).to_have_value('Nội dung tôi tự sửa giọng nói\n[Ảnh: uploads/voice-fixture.png]')
            before=inp.input_value();page.evaluate('fixtureRecs.at(-1).emit([["ghi đè ảnh",true]])')
            expect(inp).to_have_value(before);expect(mic).to_have_attribute('aria-pressed','false')
            check('programmatic attachment edits survive an in-flight recognition callback')
            inp.fill('Chỉ gửi nội dung này');mic.click();page.evaluate('window.syncFinal="không được chen vào"')
            page.locator('#composer .send').click()
            expect(inp).to_have_value('');page.wait_for_function('document.querySelector("#messages").textContent.includes("Chỉ gửi nội dung này")')
            assert fx.posts[-1]['text']=='Chỉ gửi nội dung này'
            page.evaluate('window.syncFinal="";fixtureRecs.at(-1).emit([["sau khi gửi",true]])');expect(inp).to_have_value('')
            check('send revokes recognition before synchronous stop callbacks and keeps the accepted text exact')
            inp.fill('Giữ');mic.click();page.evaluate('window.oldVoice=fixtureRecs.at(-1)');mic.click();mic.click()
            page.evaluate('oldVoice.onend();oldVoice.onerror({error:"not-allowed"});oldVoice.emit([["cũ",true]])')
            expect(mic).to_have_attribute('aria-pressed','true');expect(inp).to_have_value('Giữ')
            page.evaluate('fixtureRecs.at(-1).emit([["mới",false]])');expect(inp).to_have_value('Giữ mới')
            expect(mic).to_have_attribute('aria-label','Dừng nhập giọng nói')
            check('stop and restart isolate old result, end and error callbacks from the new session')
            page.evaluate('fixtureRecs.at(-1).onerror({error:"not-allowed"})')
            expect(mic).to_have_attribute('aria-pressed','false');expect(inp).to_have_value('Giữ mới')
            expect(page.locator('#toast')).to_contain_text('Chưa cho phép micro')
            page.evaluate('window.voiceStartThrows=true');mic.click();expect(mic).to_have_attribute('aria-pressed','false')
            expect(page.locator('#toast')).to_contain_text('Không nghe được, thử lại');expect(inp).to_have_value('Giữ mới')
            page.evaluate('window.voiceStartThrows=false;window.voiceConstructorThrows=true');mic.click()
            expect(mic).to_have_attribute('aria-pressed','false');expect(inp).to_have_value('Giữ mới')
            page.evaluate('window.voiceConstructorThrows=false');mic.press('Enter');expect(mic).to_have_attribute('aria-pressed','true')
            check('permission and start failures clear recording state, retain text, and allow an explicit retry')
            page.locator('#backBtn').click();page.locator('#rows .row[data-id="bot-b"] .rowin').click();inp.fill('Nháp B')
            page.evaluate('fixtureRecs.at(-1).emit([["từ chat A",true]])');expect(inp).to_have_value('Nháp B')
            mic.click();fx.scope='voice-owner-b';page.evaluate('window.__AURORA.reload()')
            page.locator('#rows .row[data-id="bot-b"] .rowin').click();expect(inp).to_have_value('')
            page.evaluate('fixtureRecs.at(-1).emit([["từ workspace cũ",true]])');expect(inp).to_have_value('')
            check('chat and workspace transitions reject old speech even when bot identifiers are reused')
            inp.fill('Dòng một\n');mic.click();page.evaluate('fixtureRecs.at(-1).emit([["Dòng hai",false]])')
            expect(inp).to_have_value('Dòng một\nDòng hai');page.evaluate('fixtureRecs.at(-1).onend()')
            expect(mic).to_have_attribute('aria-pressed','false');page.reload();expect(inp).to_have_value('Dòng một\nDòng hai')
            check('existing whitespace is preserved and visible dictated text survives draft reload')
            page.evaluate('localStorage.setItem("neko-aurora-lang","en")');page.reload();mic.click()
            assert page.evaluate('fixtureRecs.at(-1).lang')=='en-US'
            expect(mic).to_have_attribute('aria-label','Stop dictation')
            page.evaluate('fixtureRecs.at(-1).onend()');expect(mic).to_have_attribute('aria-label','Speak to type')
            check('recognition language and active/inactive control labels follow English UI preference')
            page.set_viewport_size({'width':390,'height':844});mic.click()
            page.evaluate('fixtureRecs.at(-1).emit([["mobile fixture",false]])');expect(inp).to_have_value('Dòng một\nDòng hai mobile fixture')
            expect(mic).to_have_attribute('aria-pressed','true');page.screenshot(path=str(OUT/'dictation-mobile.png'))
            mic.click();page.set_viewport_size({'width':1280,'height':900});page.screenshot(path=str(OUT/'dictation-desktop.png'))
            check('mobile-sized and desktop composers retain working dictation controls')
            unsupported=browser.new_context(locale='vi-VN',service_workers='block')
            unsupported.add_init_script(EVENTS+';window.SpeechRecognition=undefined;window.webkitSpeechRecognition=undefined')
            other=unsupported.new_page();other.route('**/app/api/**',Fixture().route);other.goto(f'http://127.0.0.1:{server.server_port}/app/#bot-a')
            expect(other.locator('#micBtn')).to_be_hidden();other.locator('#composer .send').click();expect(other.locator('#input')).to_be_focused()
            unsupported.close();check('unsupported browsers retain text input without offering an unavailable microphone')
            assert not errors,errors
            (OUT/'dictation-result.json').write_text(json.dumps({'result':'PASS','scenarios':11,'browser':browser.version,'errors':errors,'speech':'injected fixture, no microphone'}),encoding='utf-8')
        finally:browser.close()
finally:server.shutdown();server.server_close()
