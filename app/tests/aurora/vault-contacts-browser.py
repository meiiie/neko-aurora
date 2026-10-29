"""Contact editor interactions with masked metadata and synthetic owner requests."""
import copy
import functools
import http.server
import json
import os
import threading
import time
from pathlib import Path
from urllib.parse import urlsplit
from playwright.sync_api import expect, sync_playwright
from fixture_http import FixtureHTTPServer

ROOT=Path(__file__).resolve().parents[3]
OUT=ROOT/'.audit/2026-10-29-vault-contacts'
OUT.mkdir(exist_ok=True)


class Handler(http.server.SimpleHTTPRequestHandler):
    def log_message(self,*_):pass


class Fixture:
    def __init__(self):
        self.scope='contact-owner-a';self.posts=[];self.hold='';self.pending=[];self.status=200;self.reveals=0;self.version=0
        self.values={'emails':[{'id':'legacy-email','value':'first@example.test'}],'phones':[{'id':'legacy-phone','value':'+84 900 111 222'}]}
    def item(self):
        contacts={'revision':str(self.version)}
        for key,entries in self.values.items():contacts[key]=[{'id':e['id'],'hint':('•••• '+e['value'][-4:]) if key=='phones' else e['value'][:2]+'•••@example.test'} for e in entries]
        return dict(id='aabbccdd',type='identity',label='Fixture contact',hint={'name':'Fi•••'},filled=['name','email','phone'],useHint='Travel only',contacts=contacts)
    def route(self,r):
        path=urlsplit(r.request.url).path.removeprefix('/app/api');data={};status=200;kind=''
        if path=='/state':data=dict(workspaceId=self.scope,agents=[],pending=[],ai={},secrets=[],outbound={})
        elif path=='/vault':
            if r.request.method=='GET':data=dict(workspaceId=self.scope,items=[self.item()],uses=[])
            else:kind='save';data=self.item();status=self.status
        elif path.endswith('/contacts'):
            kind='contact';body=r.request.post_data_json;status=self.status
            if body.get('workspaceId')!=self.scope or body.get('revision')!=str(self.version):status=409
            if status==200:
                key='emails' if body['kind']=='email' else 'phones';entries=self.values[key];index=next((i for i,e in enumerate(entries) if e['id']==body.get('entryId')),-1)
                if body['action']=='add':entries.append(dict(id='contact-'+str(self.version),value=body['value']))
                elif body['action']=='primary':entries.insert(0,entries.pop(index))
                elif body['action']=='replace':entries[index]['value']=body['value']
                elif body['action']=='remove':entries.pop(index)
                self.version+=1
            data=dict(workspaceId=self.scope,item=self.item())
        elif path.endswith('/reveal'):
            self.reveals+=1;data=dict(workspaceId=self.scope,fields={'name':'Fixture Owner','email':self.values['emails'][0]['value'],'phone':self.values['phones'][0]['value']},contacts=copy.deepcopy(self.values))
        elif path.endswith('/delete'):kind='delete'
        if kind:
            self.posts.append(dict(kind=kind,body=r.request.post_data_json))
            if self.hold==kind:self.pending.append((r,status,copy.deepcopy(data)));return
        r.fulfill(status=status,content_type='application/json',body=json.dumps(data))


server=FixtureHTTPServer(('127.0.0.1',0),functools.partial(Handler,directory=str(ROOT/'app/native/internal/web/assets')))
threading.Thread(target=server.serve_forever,daemon=True).start()
errors=[]
try:
    with sync_playwright() as p:
        browser=p.chromium.launch(headless=True,executable_path=os.environ.get('AURORA_TEST_CHROME') or 'C:/Program Files/Google/Chrome/Application/chrome.exe')
        def setup():
            ctx=browser.new_context(viewport={'width':390,'height':844},locale='vi-VN',service_workers='block',reduced_motion='reduce');ctx.add_init_script("window.EventSource=class{addEventListener(){}close(){}};window.copiedContacts=[];Object.defineProperty(navigator,'clipboard',{value:{writeText:async v=>window.copiedContacts.push(v)}})")
            page=ctx.new_page();fx=Fixture();page.on('pageerror',lambda e:errors.append(str(e)));page.route('**/app/api/**',fx.route);page.goto(f'http://127.0.0.1:{server.server_port}/app/',wait_until='networkidle');open_editor(page);return ctx,page,fx
        def open_editor(page):
            page.evaluate('window.__AURORA.openVault()');page.locator('.vault-row').click();page.locator('#vaultBody').get_by_role('button',name='Sửa',exact=True).click();expect(page.locator('.vault-contacts')).to_be_visible()
        def held(page,fx):
            end=time.monotonic()+4
            while not fx.pending and time.monotonic()<end:page.wait_for_timeout(20)
            assert fx.pending;return fx.pending.pop()
        def group(page,kind):return page.locator('[data-contact-kind='+kind+']')
        def save(page,kind):group(page,kind).locator('[data-contact-save]').click()
        try:
            ctx,page,fx=setup();email=group(page,'email');phone=group(page,'phone')
            assert fx.reveals==0 and not fx.posts
            expect(email.locator('.contact-row')).to_have_count(1);expect(email.locator('[data-contact-primary]')).to_be_disabled();expect(email.locator('input')).to_have_value('')
            assert 'first@example.test' not in page.locator('#vaultBody').inner_text()
            print('PASS existing contacts remain masked and editing does not reveal values')
            page.locator('#vaultLabel').fill('Unsaved label');email.locator('input').fill('second@example.test');save(page,'email')
            expect(email.locator('.contact-row')).to_have_count(2);expect(email.locator('input')).to_have_value('');expect(email.locator('input')).to_be_focused();expect(page.locator('#vaultLabel')).to_have_value('Unsaved label')
            phone.locator('input').fill('+84 900 333 444');phone.locator('input').press('Enter');expect(phone.locator('.contact-row')).to_have_count(2)
            assert [x['body']['revision'] for x in fx.posts]==['0','1']
            print('PASS adding both channels uses successive revisions and preserves the general draft')
            row=email.locator('.contact-row').nth(1);row.locator('[data-contact-primary]').focus();page.keyboard.press('Enter')
            expect(email.locator('.contact-row').first).to_have_attribute('data-contact-id','contact-0');expect(page.locator('[data-contact-reload]')).to_be_focused()
            email.locator('.contact-row').first.get_by_role('button',name='Sửa',exact=True).click();email.locator('input').fill('replacement@example.test');save(page,'email');expect(email.locator('input')).to_have_value('')
            assert fx.values['emails'][0]['value']=='replacement@example.test'
            count=len(fx.posts);page.once('dialog',lambda d:d.dismiss());email.locator('.contact-row').first.locator('[data-contact-remove]').click();assert len(fx.posts)==count
            page.once('dialog',lambda d:d.accept());email.locator('.contact-row').first.locator('[data-contact-remove]').click();expect(email.locator('.contact-row')).to_have_count(1);assert fx.values['emails'][0]['value']=='first@example.test'
            print('PASS primary selection replacement and confirmed removal preserve channel identity')
            email.locator('input').fill('conflict@example.test');fx.version+=1;save(page,'email');expect(page.locator('.vault-contacts [role=alert]')).to_contain_text('Liên hệ đã đổi')
            expect(email.locator('input')).to_have_value('conflict@example.test');page.locator('[data-contact-reload]').click();expect(page.locator('.vault-contacts [role=status]')).to_contain_text('Đã cập nhật')
            expect(email.locator('input')).to_have_value('conflict@example.test');expect(page.locator('#vaultLabel')).to_have_value('Unsaved label');save(page,'email');expect(email.locator('.contact-row')).to_have_count(2)
            print('PASS conflict refresh preserves both drafts and never retries a mutation automatically')
            email.locator('input').fill('ime@example.test');count=len(fx.posts);email.locator('input').evaluate("n=>n.dispatchEvent(new KeyboardEvent('keydown',{key:'Enter',isComposing:true,bubbles:true,cancelable:true}))");assert len(fx.posts)==count
            page.locator('#vaultSave').click();expect(page.locator('.vault-form > [role=alert]')).to_contain_text('Lưu hoặc hủy');assert len(fx.posts)==count
            email.get_by_role('button',name='Huỷ nhập',exact=True).click()
            print('PASS composing Enter and unrelated Save cannot submit or discard a contact draft')
            fx.status=500;email.locator('input').fill('retry@example.test');save(page,'email');expect(page.locator('.vault-contacts [role=alert]')).to_contain_text('Chưa xác nhận');expect(email.locator('input')).to_have_value('retry@example.test');expect(page.locator('#vaultSave')).to_be_enabled()
            fx.status=200;fx.hold='contact';save(page,'email');r,status,data=held(page,fx);expect(page.locator('#vaultSave')).to_be_disabled();expect(page.locator('.vault-form > [data-vdelete]')).to_be_disabled();count=len(fx.posts);page.locator('.vault-form').evaluate('n=>n.requestSubmit()');assert len(fx.posts)==count
            r.fulfill(status=status,content_type='application/json',body=json.dumps(data));expect(email.locator('input')).to_have_value('');expect(page.locator('#vaultSave')).to_be_enabled();fx.hold=''
            assert fx.reveals==0
            assert page.evaluate("Object.keys(localStorage).every(k=>!String(localStorage.getItem(k)).includes('retry@example.test'))")
            print('PASS failures preserve entries and pending contact changes exclude save and deletion')
            page.locator('.vault-contacts').scroll_into_view_if_needed();page.screenshot(path=str(OUT/'contacts-mobile.png'))
            assert page.locator('.vault-contacts').evaluate('n=>n.scrollWidth<=n.clientWidth+1')
            page.locator('#vaultBody').get_by_role('button',name='Huỷ',exact=True).click();page.locator('[data-vreveal]').click();expect(page.locator('#vaultBody')).to_contain_text('retry@example.test');assert fx.reveals==1
            contact_row=page.locator('.vault-f').filter(has=page.locator('code',has_text='retry@example.test'))
            expect(contact_row.get_by_role('button',name='Sao chép',exact=True)).to_be_visible(timeout=1500)
            contact_row.get_by_role('button',name='Sao chép',exact=True).click();assert page.evaluate('window.copiedContacts')==['retry@example.test']
            stale_copy=contact_row.get_by_role('button',name='Sao chép',exact=True).element_handle()
            page.keyboard.press('Escape');expect(page.locator('#vaultBody')).not_to_contain_text('retry@example.test')
            stale_copy.evaluate('n=>n.click()');assert page.evaluate('window.copiedContacts')==['retry@example.test']
            print('PASS explicit reveal copies secondary values and closing scrubs them and rejects stale copy');ctx.close()
            for code in [200,500]:
                ctx,page,fx=setup();fx.hold='contact';fx.status=code;group(page,'email').locator('input').fill('late@example.test');save(page,'email');r,status,data=held(page,fx)
                page.keyboard.press('Escape');page.wait_for_timeout(450);fx.hold='';fx.status=200;open_editor(page);page.locator('#vaultLabel').fill('New editor');r.fulfill(status=status,content_type='application/json',body=json.dumps(data));page.wait_for_timeout(100)
                expect(page.locator('#vaultLabel')).to_have_value('New editor');expect(page.locator('#vaultSave')).to_be_enabled();expect(page.locator('.vault-contacts [role=status]')).to_have_text('')
                print(f'PASS late HTTP{code} cannot change a reopened editor');ctx.close()
            ctx,page,fx=setup();fx.hold='contact';group(page,'email').locator('input').fill('old-owner@example.test');save(page,'email');r,status,data=held(page,fx);fx.scope='contact-owner-b';page.evaluate('window.__AURORA.reload()');expect(page.locator('#vaultBody')).to_have_text('');page.wait_for_timeout(450);fx.hold='';open_editor(page);page.locator('#vaultLabel').fill('New workspace');r.fulfill(status=status,content_type='application/json',body=json.dumps(data));page.wait_for_timeout(100);expect(page.locator('#vaultLabel')).to_have_value('New workspace');expect(page.locator('.vault-contacts [role=status]')).to_have_text('')
            print('PASS old workspace acknowledgements cannot overwrite current state');ctx.close()
            ctx,page,fx=setup();fx.hold='save';page.locator('#vaultSave').click();r,status,data=held(page,fx);expect(group(page,'email').locator('input')).to_be_disabled();expect(group(page,'email').locator('[data-contact-save]')).to_be_disabled();r.fulfill(status=500,content_type='application/json',body='{}');expect(group(page,'email').locator('input')).to_be_enabled()
            page.keyboard.press('Escape');page.wait_for_timeout(450)
            with page.expect_navigation(wait_until='load'):page.evaluate('window.__I18N.set("en")')
            page.evaluate('window.__AURORA.openVault()');page.locator('.vault-row').click();page.locator('#vaultBody').get_by_role('button',name='Edit',exact=True).click();expect(group(page,'email').get_by_role('button',name='Add email',exact=True)).to_be_visible();expect(group(page,'phone').get_by_role('button',name='Add phone',exact=True)).to_be_visible();page.set_viewport_size({'width':1280,'height':900});page.locator('.vault-contacts').scroll_into_view_if_needed();page.screenshot(path=str(OUT/'contacts-desktop-en.png'))
            print('PASS pending general save locks contact actions and English desktop controls are available');ctx.close()
            assert not errors,errors
        finally:browser.close()
finally:server.shutdown();server.server_close()
