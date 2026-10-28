"""Owner guidance editing in real browser UI with isolated Vault fixtures."""
import functools,http.server,json,os,threading,time
from pathlib import Path
from urllib.parse import urlsplit
from playwright.sync_api import expect,sync_playwright
ROOT=Path(__file__).resolve().parents[3];OUT=ROOT/'.audit/2026-10-29-vault-hints';OUT.mkdir(exist_ok=True)
class Server(http.server.ThreadingHTTPServer):request_queue_size=64
class Handler(http.server.SimpleHTTPRequestHandler):
    def log_message(self,*_):pass
class Fixture:
    def __init__(self):self.scope='hint-owner-a';self.hint='Chỉ dùng cho công tác';self.fail=False;self.hold=False;self.pending=[];self.saves=[]
    def route(self,r):
        path=urlsplit(r.request.url).path.removeprefix('/app/api');data={};status=200
        item=lambda:dict(id='aabbccdd',type='login',label='Travel',hint={'site':'example.test'},filled=['password'],useHint=self.hint)
        if path=='/state':data=dict(workspaceId=self.scope,agents=[dict(id='bot-a',name='Chat',status='idle',av={})],pending=[],ai={},secrets=[],outbound={})
        elif path.endswith('/messages'):data=[dict(id='hint-card',taskId='hint-work',agentId='bot-a',role='agent',text='Save login',secret={'vault':'login','label':'Card fixture'},t='2026-10-29T00:00:00Z')]
        elif path=='/vault':
            if r.request.method=='POST':
                body=r.request.post_data_json;self.saves.append(body);status=500 if self.fail else 200
                if not self.fail:self.hint=body.get('useHint',self.hint)
                data=item()
                if self.hold:self.pending.append((r,json.dumps(data)));return
            else:data=dict(workspaceId=self.scope,items=[item()],uses=[])
        elif path.endswith('/reveal'):raise AssertionError('editing guidance must not reveal credentials')
        r.fulfill(status=status,content_type='application/json',body=json.dumps(data))
checks=[]
def check(s):checks.append(s);print('PASS '+s,flush=True)
server=Server(('127.0.0.1',0),functools.partial(Handler,directory=str(ROOT/'app/native/internal/web/assets')));threading.Thread(target=server.serve_forever,daemon=True).start()
try:
 with sync_playwright() as p:
    browser=p.chromium.launch(headless=True,executable_path=os.environ.get('AURORA_TEST_CHROME') or 'C:/Program Files/Google/Chrome/Application/chrome.exe')
    try:
        context=browser.new_context(viewport={'width':390,'height':844},locale='vi-VN',service_workers='block',reduced_motion='reduce');context.add_init_script('window.fixtureStreams=[];window.EventSource=class{constructor(){this.listeners={};fixtureStreams.push(this)}addEventListener(t,f){(this.listeners[t]??=[]).push(f)}close(){}emit(t,d){for(const f of this.listeners[t]||[])f({data:JSON.stringify(d)})}}')
        page=context.new_page();fx=Fixture();errors=[];page.on('pageerror',lambda e:errors.append(str(e)));page.route('**/app/api/**',fx.route);page.goto(f'http://127.0.0.1:{server.server_port}/app/',wait_until='networkidle')
        body=page.locator('#vaultBody');hint=body.locator('textarea[name="useHint"]')
        def open_vault():page.locator('#meBtn').click();page.locator('#openVault').click();expect(body.locator('.vault-row')).to_be_visible()
        def edit():body.locator('.vault-row').click();body.get_by_role('button',name='Sửa',exact=True).click();expect(hint).to_be_visible()
        def close():page.keyboard.press('Escape');page.wait_for_timeout(450)
        open_vault();edit();expect(hint).to_have_value(fx.hint);expect(hint).to_have_attribute('maxlength','500');expect(body).to_contain_text('Bot đọc gợi ý này')
        hint.scroll_into_view_if_needed();page.screenshot(path=str(OUT/'vault-hint-edit-mobile.png'))
        hint.fill('Dùng cho chuyến đi trong nước');body.locator('#vaultSave').click();expect(body.locator('.vault-row')).to_be_visible();assert fx.saves[-1]['useHint']=='Dùng cho chuyến đi trong nước' and fx.saves[-1]['fields']=={}
        body.locator('.vault-row').click();expect(body.locator('.vault-use-hint')).to_have_text('Dùng cho chuyến đi trong nước')
        check('guidance can be edited and read without revealing or resubmitting a credential')
        close();page.reload(wait_until='networkidle');open_vault();edit();expect(hint).to_have_value('Dùng cho chuyến đi trong nước');hint.fill('');body.locator('#vaultSave').click();expect(body.locator('.vault-row')).to_be_visible();body.locator('.vault-row').click();expect(body.locator('.vault-use-hint')).to_have_count(0);assert fx.saves[-1]['useHint']==''
        check('server-backed guidance survives reload and explicit empty clears it')
        body.get_by_role('button',name='Sửa',exact=True).click();hint.fill('Keep failed edit');fx.fail=True;body.locator('#vaultSave').click();expect(body.locator('.sec-err')).to_contain_text('Không lưu được');expect(hint).to_have_value('Keep failed edit');expect(body.locator('#vaultSave')).to_be_enabled();fx.fail=False
        page.evaluate('window.oldHint=document.querySelector("#vaultBody textarea[name=useHint]")');close();assert page.evaluate('oldHint.value')=='';expect(body).to_have_text('')
        check('failed saves preserve edits and closing scrubs detached hint fields')
        open_vault();edit();hint.fill('Earlier save');fx.hold=True;body.locator('#vaultSave').click()
        end=time.monotonic()+3
        while not fx.pending and time.monotonic()<end:page.wait_for_timeout(20)
        assert fx.pending;held=fx.pending.pop();close();fx.hold=False;open_vault();edit();hint.fill('Current edit');held[0].fulfill(status=200,content_type='application/json',body=held[1]);page.wait_for_timeout(80);expect(hint).to_have_value('Current edit')
        check('a late save acknowledgement leaves a newer edit intact')
        page.evaluate('window.oldHint=document.querySelector("#vaultBody textarea[name=useHint]")');fx.scope='hint-owner-b';page.evaluate('window.__AURORA.reload()');expect(body).to_have_text('');assert page.evaluate('oldHint.value')==''
        check('workspace changes scrub hint input and close the prior Vault view')
        page.wait_for_timeout(450)
        page.locator('#rows .rowin').click();card=page.locator('#messages .vault-form');expect(card.locator('textarea[name="useHint"]')).to_be_visible();card.locator('textarea[name="useHint"]').fill('Unsubmitted chat guidance')
        page.evaluate('fixtureStreams.at(-1).emit("message",{id:"hint-card",taskId:"hint-work",agentId:"bot-a",role:"agent",text:"Updated prompt",secret:{vault:"login",label:"Card fixture"},t:"2026-10-29T00:00:00Z"})')
        expect(card.locator('textarea[name="useHint"]')).to_have_value('Unsubmitted chat guidance')
        expect(card.locator('textarea[name="useHint"]')).to_be_focused()
        check('same credential-card updates preserve unfinished guidance')
        page.evaluate('fixtureStreams.at(-1).emit("message",{id:"hint-card",taskId:"hint-work",agentId:"bot-a",role:"agent",text:"Changed target",secret:{vault:"login",label:"Card fixture",site:"different.example.test"},t:"2026-10-29T00:00:00Z"})')
        expect(card.locator('textarea[name="useHint"]')).to_have_value('');card.locator('textarea[name="useHint"]').fill('New target guidance')
        check('a changed credential-card target does not inherit previous guidance')
        page.locator('#backBtn').click();expect(card.locator('textarea[name="useHint"]')).to_have_value('')
        check('chat credential cards offer guidance and clear it when leaving the conversation')
        fx.hint='Phiên bản của tôi <img src=x onerror=window.hintExecuted=1>'
        with page.expect_navigation(wait_until='networkidle'):page.evaluate('window.__I18N.set("en")')
        open_vault();body.locator('.vault-row').click();expect(body.locator('.vault-use-hint')).to_have_text(fx.hint);expect(body.locator('.vault-use-hint img')).to_have_count(0);assert page.evaluate('window.hintExecuted||0')==0
        page.screenshot(path=str(OUT/'vault-hint-mobile-en.png'));page.set_viewport_size({'width':1280,'height':900});page.screenshot(path=str(OUT/'vault-hint-desktop-en.png'))
        check('English UI preserves literal guidance and renders markup as text')
        assert all(x['workspaceId']=='hint-owner-a' for x in fx.saves);assert not errors,errors
        (OUT/'hint-browser-result.json').write_bytes(json.dumps(dict(result='PASS',scenarios=len(checks),browser=browser.version,errors=errors)).encode())
    finally:browser.close()
finally:server.shutdown();server.server_close()
