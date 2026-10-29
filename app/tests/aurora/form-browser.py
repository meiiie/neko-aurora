"""Form draft and structured submission fixtures; no provider or model calls."""
import functools
import http.server
import json
import os
import threading
from pathlib import Path
from fixture_http import FixtureHTTPServer
from urllib.parse import urlsplit
from playwright.sync_api import expect, sync_playwright

ROOT = Path(__file__).resolve().parents[3]
OUT = ROOT / '.audit/2026-10-28-form-drafts'
OUT.mkdir(parents=True, exist_ok=True)

class Handler(http.server.SimpleHTTPRequestHandler):
    def log_message(self, *_): pass

class Fixture:
    def __init__(self):
        self.scope = 'form-owner-a'
        self.form = dict(answerVersion=1, title='Thông tin dự án', fields=[
            dict(name='name', label='Tên dự án', type='text', required=True),
            dict(name='amount', label='Số lượng', type='number'),
            dict(name='date', label='Ngày', type='date'),
            dict(name='format', label='Dạng', type='select', options=['Bảng', 'Biểu đồ'])])
        self.posts, self.receipts, self.held = [], {}, []
        self.mode = 'ok'

    def route(self, route):
        req = route.request
        path = urlsplit(req.url).path.removeprefix('/app/api')
        data, status = {}, 200
        if path == '/events':
            route.fulfill(status=200, content_type='text/event-stream', body=''); return
        if path == '/state':
            data = dict(workspaceId=self.scope, agents=[dict(id='bot-a', name='Aurora', status='idle', av=dict(shape=0, color=4))], pending=[], ai={}, secrets=[], outbound={})
        elif path.endswith('/messages'):
            key = self.scope
            if req.method == 'GET':
                data = [dict(id='form-message', taskId='req-form', agentId='bot-a', role='agent', text='', form=self.form, t='2026-10-28T04:00:00Z'), *self.receipts.get(key, {}).values()]
            else:
                body = req.post_data_json; self.posts.append(body)
                if self.mode == 'offline': route.abort('internetdisconnected'); return
                if self.mode == 'hold': self.held.append(route); return
                if self.mode == 'reject': status, data = 400, dict(error='invalid')
                else:
                    receipt = dict(id=body['clientNonce']+':u', taskId='answer', agentId='bot-a', role='user', text=body['text'], formFor=body['formFor'], t='2026-10-28T04:01:00Z')
                    if body.get('formAnswerVersion')==1: receipt['formValues']=body['formValues']
                    data = self.receipts.setdefault(key, {}).setdefault(body['clientNonce'], receipt)
                    if self.mode == 'lost': self.mode = 'ok'; route.abort('connectionreset'); return
        route.fulfill(status=status, content_type='application/json', body=json.dumps(data))

def check(text): print('PASS '+text, flush=True)

server = FixtureHTTPServer(('127.0.0.1', 0), functools.partial(Handler, directory=str(ROOT/'app/native/internal/web/assets')))
threading.Thread(target=server.serve_forever, daemon=True).start()
try:
    with sync_playwright() as p:
        chrome = os.environ.get('AURORA_TEST_CHROME')
        if not chrome and Path('C:/Program Files/Google/Chrome/Application/chrome.exe').exists(): chrome = 'C:/Program Files/Google/Chrome/Application/chrome.exe'
        browser = p.chromium.launch(headless=True, executable_path=chrome)
        context = browser.new_context(viewport=dict(width=390, height=844), locale='vi-VN', service_workers='block')
        page = context.new_page(); fx = Fixture(); errors = []
        page.on('pageerror', lambda e: errors.append(str(e)))
        page.route('**/app/api/**', lambda r: fx.route(r))
        url = f'http://127.0.0.1:{server.server_port}/app/#bot-a'
        def load():
            if page.url.startswith(url.split('#')[0]):
                page.evaluate('(u)=>history.replaceState(null,"",u)', url)
                page.reload(wait_until='networkidle')
            else: page.goto(url, wait_until='networkidle')
            expect(page.locator('.form-card')).to_be_visible()
        def reset():
            global fx
            fx = Fixture(); page.evaluate('localStorage.clear()'); load()
        def field(name): return page.locator('.form-card').get_by_label(name, exact=False)
        def submit(): page.locator('.form-card').get_by_role('button', name='Gửi', exact=True).click()
        def draft_key(): return page.evaluate('Object.keys(localStorage).find(k=>k.startsWith("aurora.forms.v1."))')

        load()
        field('Tên dự án').fill('Aurora chưa gửi')
        field('Số lượng').fill('12.5'); field('Ngày').fill('2026-10-28'); field('Dạng').select_option('Biểu đồ')
        load()
        expect(field('Tên dự án')).to_have_value('Aurora chưa gửi')
        expect(field('Số lượng')).to_have_value('12.5'); expect(field('Dạng')).to_have_value('Biểu đồ')
        assert not fx.posts
        page.locator('.form-card').scroll_into_view_if_needed()
        page.screenshot(path=str(OUT/'form-draft-mobile.png'))
        check('unsent fields survive reload without dispatch')

        fx.scope = 'form-owner-b'; load(); expect(field('Tên dự án')).to_have_value('')
        field('Tên dự án').fill('Account B')
        fx.scope = 'form-owner-a'; load(); expect(field('Tên dự án')).to_have_value('Aurora chưa gửi')
        check('drafts stay in original workspace')

        page.get_by_role('button',name='Xóa bản nháp',exact=True).click()
        expect(field('Tên dự án')).to_have_value(''); load(); expect(field('Tên dự án')).to_have_value('')
        fx.scope='form-owner-b';load();expect(field('Tên dự án')).to_have_value('Account B')
        fx.scope='form-owner-a';load();field('Tên dự án').fill('Aurora chưa gửi')
        check('explicit draft deletion only clears the current form and workspace')

        key = draft_key(); old_raw = page.evaluate('(k)=>localStorage.getItem(k)', key)
        fx.form['fields'][0]['label'] = 'Tên đã đổi'; load()
        expect(page.locator('.form-card').get_by_role('button', name='Gửi', exact=True)).to_be_disabled()
        assert page.evaluate('(k)=>localStorage.getItem(k)', key) == old_raw
        page.get_by_role('button', name='Bỏ bản nháp cũ', exact=True).click()
        expect(field('Tên đã đổi')).to_have_value('')
        check('schema change preserves old draft until explicit discard')

        reset(); page.evaluate('() => {globalThis.savedSet=Storage.prototype.setItem; Storage.prototype.setItem=function(){throw Error("full")}}')
        field('Tên dự án').fill('Keep current value'); submit()
        expect(field('Tên dự án')).to_have_value('Keep current value'); assert not fx.posts
        expect(page.locator('.form-card [role="status"]')).to_contain_text('Chưa lưu')
        page.evaluate('() => {Storage.prototype.setItem=globalThis.savedSet}')
        check('storage failure keeps input and blocks unsaved dispatch')

        reset(); fx.mode = 'offline'; field('Tên dự án').fill('Offline values'); field('Số lượng').fill('-12.5'); submit()
        expect(page.locator('.form-card button[type="submit"]')).to_have_text('Đang chờ xác nhận…')
        load(); expect(field('Tên dự án')).to_have_value('Offline values')
        fx.mode = 'lost'
        expect(page.locator('.form-card button[type="submit"]')).to_have_text('Đã gửi', timeout=12000)
        assert len(fx.posts) >= 2 and all(x == fx.posts[0] for x in fx.posts)
        assert fx.posts[0]['formAnswerVersion'] == 1 and fx.posts[0]['formValues'][1] == '-12.5'
        assert not page.evaluate('Object.keys(localStorage).some(k=>k.startsWith("aurora.forms.v1."))')
        check('offline and lost acknowledgement preserve structured retry and clear accepted draft')

        reset(); field('Tên dự án').fill('recover'); key=draft_key()
        page.evaluate('(k)=>localStorage.setItem(k,"{broken")', key); load()
        expect(page.locator('.form-card button[type="submit"]')).to_be_disabled()
        assert page.evaluate('(k)=>localStorage.getItem(k)', key) == '{broken'
        page.get_by_role('button', name='Bỏ bản nháp cũ', exact=True).click()
        expect(page.locator('.form-card button[type="submit"]')).to_be_enabled()
        check('corrupt draft is not silently replaced')

        reset(); field('Tên dự án').fill('My edit'); key=draft_key()
        page.evaluate('(k)=>{const s=JSON.parse(localStorage.getItem(k));s.values[0]="Other window";localStorage.setItem(k,JSON.stringify(s))}',key)
        field('Tên dự án').fill('Conflicting edit')
        expect(page.locator('.form-card button[type="submit"]')).to_be_disabled()
        assert json.loads(page.evaluate('(k)=>localStorage.getItem(k)',key))['values'][0]=='Other window'
        check('observed cross-window conflict does not overwrite newer draft')

        reset(); page.evaluate('() => {for(let i=0;i<128;i++)localStorage.setItem("aurora.forms.v1.form-owner-a.fixture-"+i,"{}") }')
        field('Tên dự án').fill('Full draft store'); submit(); assert not fx.posts
        expect(page.locator('.form-card [role="status"]')).to_contain_text('Chưa lưu')
        check('draft capacity refuses new admission without deleting older values')

        reset(); fx.form.pop('answerVersion')
        page.evaluate('() => localStorage.setItem("aurora.outbox.v1.form-owner-a.form-req-form",JSON.stringify({workspaceId:"form-owner-a",agentId:"bot-a",n:"form-req-form",text:"Legacy payload",formFor:"req-form",formValues:["Legacy payload","","","Bảng"],t:Date.now()}))')
        load()
        page.wait_for_function('document.querySelector(".form-card button[type=submit]").disabled')
        assert fx.posts and 'formValues' not in fx.posts[0] and 'formAnswerVersion' not in fx.posts[0]
        expect(page.locator('.form-card button[type="submit"]')).to_have_text('Đã gửi', timeout=10000)
        check('previous-version queued attempt keeps original wire payload shape')

        reset(); fx.mode='reject'; field('Tên dự án').fill('Rejected values'); submit()
        expect(page.locator('.form-card button[type="submit"]')).to_have_text('Chưa được nhận')
        load(); expect(field('Tên dự án')).to_have_value('Rejected values')
        assert len(fx.posts)==1
        check('definitive refusal retains values and stops retry')

        reset();fx.form['fields'][0]['type']='textarea';load()
        multiline='Một dòng\n- Số lượng: nội dung riêng'
        field('Tên dự án').fill(multiline);field('Số lượng').fill('3');submit()
        expect(page.locator('.form-card button[type="submit"]')).to_have_text('Đã gửi');load()
        expect(page.locator('.form-result dd').nth(0)).to_have_text(multiline)
        expect(page.locator('.form-result dd').nth(1)).to_have_text('3')
        check('confirmed multiline fields remain assigned to their original labels')

        receipt=next(iter(fx.receipts[fx.scope].values()))
        accepted_values=receipt.pop('formValues');load()
        expect(page.locator('.form-result')).to_have_js_property('tagName','PRE')
        receipt['formValues']=accepted_values
        page.evaluate('() => document.dispatchEvent(new Event("visibilitychange"))')
        expect(page.locator('.form-result dd').nth(0)).to_have_text(multiline)
        check('receipt hydration replaces cached text fallback with typed fields without reload')

        for persisted in (True, False):
            reset()
            result=page.evaluate("""(persisted) => {
              const records=new Map();let fail=!persisted;
              const storage={getItem:k=>records.get(k)??null,setItem:(k,v)=>{if(fail)throw Error('full');records.set(k,v)},removeItem:k=>records.delete(k),get length(){return records.size},key:i=>[...records.keys()][i]};
              const host=document.createElement('div');document.body.append(host);
              const options=i=>({workspaceId:'memory-'+i,storage,active:()=>true,send:()=>{throw Error('unexpected submit')},message:{agentId:'fixture',taskId:'memory-'+i,form:{answerVersion:1,title:'Draft',fields:[{name:'value',label:'Value',type:'text'}]}}});
              const fill=(node,value)=>{const input=node.querySelector('input');input.value=value;input.dispatchEvent(new Event('input'))};
              for(let i=0;i<128;i++){const node=AURORA_FORMS.create(options(i));host.replaceChildren(node);fill(node,'saved-'+i)}
              fail=true;let node=AURORA_FORMS.create(options(128));host.replaceChildren(node);fill(node,'keep-me');
              const warning=node.querySelector('[role=status]').textContent;
              if(persisted){node=AURORA_FORMS.create(options(128));host.replaceChildren(node)}
              const value=node.querySelector('input').value,clear=[...node.querySelectorAll('button')].find(x=>x.textContent==='Xóa bản nháp');
              const clearVisible=clear&&!clear.hidden;clear?.click();const cleared=node.querySelector('input').value==='';host.remove();
              return {value,warning,clearVisible,cleared,records:records.size};
            }""",persisted)
            assert result['value']=='keep-me' and result['clearVisible'] and result['cleared'],result
            if persisted: assert result['records']==128,result
            else: assert 'Sao chép' in result['warning'],result
            check('bounded draft memory '+('evicts only persisted copies and restores failed save' if persisted else 'warns to copy when all slots contain unsaved values'))

        reset(); submit(); assert not fx.posts
        field('Tên dự án').fill('Bàn phím'); field('Số lượng').fill('0.25')
        page.locator('.form-card button[type="submit"]').focus(); page.keyboard.press('Enter')
        expect(page.locator('.form-card button[type="submit"]')).to_have_text('Đã gửi')
        load()
        expect(page.locator('.form-result')).to_contain_text('Bàn phím')
        expect(page.locator('.form-result')).to_contain_text('0.25')
        assert page.locator('.form-card input, .form-card select, .form-card textarea').count()==0
        page.locator('.form-card').scroll_into_view_if_needed()
        page.screenshot(path=str(OUT/'form-mobile.png'))
        assert page.evaluate('document.documentElement.scrollWidth<=innerWidth') and not errors, errors
        check('required field, decimal input, keyboard, confirmed summary after reload and mobile width')
        context.close(); browser.close()
finally:
    server.shutdown(); server.server_close()
