"""Owner payment preference UI with synthetic metadata, never a payment provider."""
import functools,http.server,json,os,threading,time
from pathlib import Path
from urllib.parse import urlsplit
from playwright.sync_api import expect,sync_playwright
ROOT=Path(__file__).resolve().parents[3];OUT=ROOT/'.audit/2026-10-29-vault-payments';OUT.mkdir(exist_ok=True)
class Server(http.server.ThreadingHTTPServer):request_queue_size=64
class Handler(http.server.SimpleHTTPRequestHandler):
    def log_message(self,*_):pass
class Fixture:
    def __init__(self):self.scope='payment-owner-a';self.preferred='';self.status=200;self.hold=False;self.pending=[];self.posts=[];self.empty=False;self.wrong_scope=False
    def route(self,r):
        path=urlsplit(r.request.url).path.removeprefix('/app/api');data={};status=200
        if path=='/state':data=dict(workspaceId=self.scope,agents=[],pending=[],ai={},secrets=[],outbound={})
        elif path=='/vault':
            items=[dict(id='aabbccdd',type='card',label='Phiên bản của tôi',hint={'brand':'Visa','last4':'1111'},filled=['number'],preferredPayment=self.preferred=='aabbccdd'),dict(id='11223344',type='card',label='Backup card',hint={'brand':'Mastercard','last4':'4444'},filled=['number'],preferredPayment=self.preferred=='11223344'),dict(id='55667788',type='login',label='Fixture login',hint={'site':'example.test'},filled=['password'])]
            data=dict(workspaceId=self.scope,items=[] if self.empty else items,uses=[])
        elif path=='/vault/payments':
            assert r.request.method=='POST';body=r.request.post_data_json;self.posts.append(body);status=self.status
            if status==200:self.preferred=body['preferredItem']
            data=dict(workspaceId='wrong' if self.wrong_scope else self.scope,preferredItem=self.preferred)
            if self.hold:self.pending.append((r,status,data));return
        elif path.endswith('/reveal'):raise AssertionError('choosing a preferred payment must not reveal credentials')
        elif path.endswith('/delete'):
            if self.preferred==path.split('/')[2]:self.preferred=''
        r.fulfill(status=status,content_type='application/json',body=json.dumps(data))

checks=[]
def check(s):checks.append(s);print('PASS '+s,flush=True)
server=Server(('127.0.0.1',0),functools.partial(Handler,directory=str(ROOT/'app/native/internal/web/assets')));threading.Thread(target=server.serve_forever,daemon=True).start()
try:
 with sync_playwright() as p:
    browser=p.chromium.launch(headless=True,executable_path=os.environ.get('AURORA_TEST_CHROME') or 'C:/Program Files/Google/Chrome/Application/chrome.exe')
    try:
        ctx=browser.new_context(viewport={'width':390,'height':844},locale='vi-VN',service_workers='block',reduced_motion='reduce');ctx.add_init_script('window.EventSource=class{addEventListener(){}close(){}}')
        page=ctx.new_page();fx=Fixture();errors=[];page.on('pageerror',lambda e:errors.append(str(e)));page.route('**/app/api/**',fx.route);page.goto(f'http://127.0.0.1:{server.server_port}/app/',wait_until='networkidle')
        body=page.locator('#vaultBody');tabs=body.get_by_role('tab');payments=body.get_by_role('tab',name='Thanh toán',exact=True)
        def open_vault():page.locator('#meBtn').click();page.locator('#openVault').click();expect(body.locator('.vault-row')).to_have_count(3)
        def select_card(id):body.locator('.vault-it[data-id="'+id+'"] .vault-row').click();expect(body.locator('[data-vpayment]')).to_be_visible()
        def close():page.keyboard.press('Escape');page.wait_for_timeout(450)
        def held():
            end=time.monotonic()+3
            while not fx.pending and time.monotonic()<end:page.wait_for_timeout(20)
            assert fx.pending;return fx.pending.pop(0)
        open_vault();expect(tabs).to_have_count(2);payments.click();expect(payments).to_have_attribute('aria-selected','true');expect(body.get_by_role('tabpanel')).to_be_visible();expect(body.locator('.vault-row')).to_have_count(2);expect(body).to_contain_text('Bạn vẫn duyệt mỗi lần dùng');assert len(fx.posts)==0
        check('Payments tab filters saved cards without choosing or revealing one')
        tabs.first.focus();page.keyboard.press('ArrowRight');expect(payments).to_be_focused();page.keyboard.press('Home');expect(tabs.first).to_be_focused();expect(body.locator('.vault-row')).to_have_count(3);page.keyboard.press('End');expect(payments).to_be_focused();expect(body.locator('.vault-row')).to_have_count(2)
        check('keyboard arrows Home and End switch focus and selected tab')
        select_card('aabbccdd');body.locator('[data-vpayment]').click();expect(body.locator('.vault-preferred')).to_have_count(1);expect(body.locator('.vault-it[data-id="aabbccdd"] .vault-preferred')).to_be_visible();expect(body.locator('[data-vpayment]')).to_have_text('Bỏ ưu tiên thanh toán');assert fx.posts[-1]==dict(workspaceId='payment-owner-a',preferredItem='aabbccdd')
        expect(body.locator('[data-vpayment]')).to_be_focused()
        page.wait_for_function('!document.querySelector("#toast").classList.contains("show")');body.locator('[data-vpayment]').scroll_into_view_if_needed();page.screenshot(path=str(OUT/'payments-mobile.png'))
        close();open_vault();payments.click();expect(body.locator('.vault-preferred')).to_have_count(1)
        select_card('11223344');body.locator('[data-vpayment]').click();expect(body.locator('.vault-preferred')).to_have_count(1);expect(body.locator('.vault-it[data-id="11223344"] .vault-preferred')).to_be_visible()
        check('selection persists across reopen and replacing it leaves exactly one preferred card')
        body.locator('[data-vpayment]').click();expect(body.locator('.vault-preferred')).to_have_count(0);assert fx.posts[-1]['preferredItem']==''
        check('explicit clear leaves no automatically selected fallback')
        fx.status=500;body.locator('[data-vpayment]').click();expect(body.locator('[data-vpayment-error]')).to_be_visible();expect(body.locator('[data-vpayment]')).to_be_enabled();expect(body.locator('.vault-preferred')).to_have_count(0);posts=len(fx.posts);page.wait_for_timeout(100);assert len(fx.posts)==posts
        fx.status=200;body.locator('[data-vpayment]').click();expect(body.locator('.vault-preferred')).to_have_count(1)
        check('a failed preference keeps prior state and needs an explicit retry')
        fx.hold=True;body.locator('[data-vpayment]').click();r,status,data=held();assert len(fx.posts)==posts+2;expect(body.locator('[data-vpayment]')).to_be_disabled();body.locator('[data-vpayment]').press('Enter');assert len(fx.posts)==posts+2
        close();fx.hold=False;fx.preferred='aabbccdd';open_vault();payments.click();select_card('aabbccdd');r.fulfill(status=status,content_type='application/json',body=json.dumps(data));page.wait_for_timeout(100);expect(body.locator('.vault-it[data-id="aabbccdd"] .vault-preferred')).to_be_visible();expect(body.locator('[data-vpayment]')).to_have_text('Bỏ ưu tiên thanh toán')
        check('duplicate activation is blocked and a late acknowledgement cannot replace a reopened view')
        fx.hold=True;body.locator('[data-vpayment]').click();r,status,data=held();fx.scope='payment-owner-b';fx.preferred='11223344';page.evaluate('window.__AURORA.reload()');expect(body).to_have_text('');page.wait_for_timeout(450);fx.hold=False;open_vault();payments.click();r.fulfill(status=status,content_type='application/json',body=json.dumps(data));page.wait_for_timeout(100);expect(body.locator('.vault-it[data-id="11223344"] .vault-preferred')).to_be_visible()
        check('workspace changes close the old view and reject its late response')
        select_card('11223344');fx.wrong_scope=True;body.locator('[data-vpayment]').click();expect(body.locator('[data-vpayment-error]')).to_be_visible();expect(body.locator('.vault-it[data-id="11223344"] .vault-preferred')).to_be_visible();fx.wrong_scope=False
        check('a response for a different workspace is not applied')
        close();fx.preferred='aabbccdd'
        with page.expect_navigation(wait_until='networkidle'):page.evaluate('window.__I18N.set("en")')
        page.locator('#meBtn').click();page.locator('#openVault').click();body.get_by_role('tab',name='Payments',exact=True).click();expect(body.get_by_role('tab',name='All',exact=True)).to_be_visible();expect(body).to_contain_text('Your Vault');expect(body.locator('.vault-it[data-id="aabbccdd"]')).to_contain_text('Phiên bản của tôi');select_card('aabbccdd');expect(body.locator('[data-vpayment]')).to_have_text('Clear payment preference');page.set_viewport_size({'width':1280,'height':900});page.screenshot(path=str(OUT/'payments-desktop-en.png'))
        check('English controls preserve literal owner card labels')
        close();fx.empty=True;page.locator('#meBtn').click();page.locator('#openVault').click();body.get_by_role('tab',name='Payments',exact=True).click();expect(body.locator('.vault-row')).to_have_count(0);expect(body).to_contain_text('No payment cards yet');expect(body.locator('[data-vadd="card"]')).to_be_visible()
        check('empty Payments view offers adding a card and claims no default')
        assert not errors,errors
        (OUT/'payment-browser-result.json').write_bytes(json.dumps(dict(result='PASS',scenarios=len(checks),browser=browser.version,errors=errors)).encode())
    finally:browser.close()
finally:server.shutdown();server.server_close()
