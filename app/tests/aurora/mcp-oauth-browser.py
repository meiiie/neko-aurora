"""Run through the real local Go API; all authorization/token traffic is fixture-only."""
import html
import os
from pathlib import Path
import sys
from urllib.parse import parse_qs, urlencode, urlsplit
from playwright.sync_api import expect, sync_playwright

ROOT=Path(__file__).resolve().parents[3]
OUT=ROOT/'.audit/2026-10-28-mcp-oauth-flow'
BASE,BOT=sys.argv[1:3]
CALLBACK='/app/oauth/mcp/callback.html'

with sync_playwright() as p:
    chrome=os.environ.get('AURORA_TEST_CHROME') or 'C:/Program Files/Google/Chrome/Application/chrome.exe'
    browser=p.chromium.launch(headless=True,executable_path=chrome)
    context=browser.new_context(viewport=dict(width=390,height=844),locale='vi-VN')
    errors=[]; callback_requests=[]; authorization=[]; delivery={"drop_refresh":False}
    context.add_init_script("{const original=Storage.prototype.setItem;window.__oauthStorageWrites=0;Storage.prototype.setItem=function(...args){window.__oauthStorageWrites++;return original.apply(this,args)}}")
    context.on('page',lambda page:page.on('pageerror',lambda e:errors.append(str(e))))
    def route(r):
        u=urlsplit(r.request.url)
        if u.netloc==urlsplit(BASE).netloc:
            if u.path=='/app/api/mcp/oauth/callback':callback_requests.append(r.request)
            if u.path.endswith('/mcp/oauth/refresh') and delivery['drop_refresh']:
                r.fetch();r.abort();return
            r.continue_();return
        if u.hostname=='auth.example.com' and u.path=='/authorize':
            q=parse_qs(u.query);authorization.append(q)
            assert q['client_id']==['fixture-client'] and q['code_challenge_method']==['S256']
            assert q['resource']==['https://mcp.example.com/team/mcp'] and 'code_verifier' not in q
            target=q['redirect_uri'][0]+'?'+urlencode(dict(state=q['state'][0],code='fixture-code',iss='https://auth.example.com/tenant'))
            r.fulfill(content_type='text/html',body='<a href="'+html.escape(target,quote=True)+'">Return to Aurora</a>');return
        r.abort();raise AssertionError('unexpected external request')
    context.route('**/*',route)
    page=context.new_page()
    page.goto(BASE+'/app/#'+BOT,wait_until='domcontentloaded')
    page.locator('#whoBtn').click();page.locator('#prExt').click()
    panel=page.locator('.mcp-authorization')
    panel.get_by_role('button',name='Xem thông tin cấp quyền · fixture').click()
    expect(panel).to_contain_text('files:read')
    panel.get_by_role('button',name='Tải trạng thái đăng nhập').click()
    expect(panel).to_contain_text(BASE+CALLBACK)
    panel.get_by_label('Client ID',exact=True).fill('fixture-client')
    panel.get_by_role('button',name='Bắt đầu đăng nhập').click()
    link=panel.get_by_role('link',name='Tiếp tục đăng nhập tại nhà cung cấp')
    expect(link).to_be_visible();assert not authorization
    print('PASS explicit scoped login starts without navigating or granting automatically',flush=True)
    with context.expect_page() as opened:link.click()
    popup=opened.value;popup.get_by_role('link',name='Return to Aurora').click()
    expect(popup.locator('#oauth-result')).to_contain_text('Đã kết nối MCP')
    assert popup.url==BASE+CALLBACK
    assert len(callback_requests)==1 and callback_requests[0].headers.get('origin')==BASE
    assert 'neko_local=' in callback_requests[0].all_headers().get('cookie','')
    assert popup.evaluate('window.__oauthStorageWrites===0')
    assert popup.evaluate("state=>Object.values(localStorage).concat(Object.values(sessionStorage)).every(v=>![state,'fixture-code','fixture-access','fixture-refresh'].some(secret=>v.includes(secret)))",authorization[0]['state'][0])
    headers=popup.request.get(BASE+CALLBACK).headers
    assert headers['cache-control']=='no-store' and headers['referrer-policy']=='no-referrer' and 'etag' not in headers
    print('PASS cross-site callback uses existing Strict cookie, removes URL code and stores no browser token',flush=True)
    saved=authorization[0]
    callback_url=BASE+CALLBACK+'?'+urlencode(dict(state=saved['state'][0],code='fixture-code',iss='https://auth.example.com/tenant'))
    popup.goto(callback_url);expect(popup.locator('#oauth-result')).to_contain_text('Đã kết nối MCP')
    print('PASS repeated completed callback reads receipt without another token exchange',flush=True)
    panel.get_by_role('button',name='Tải trạng thái đăng nhập').click()
    expect(panel).to_contain_text('Tài khoản đã kết nối')
    delivery['drop_refresh']=True
    panel.get_by_role('button',name='Làm mới phiên đăng nhập').click()
    expect(panel).to_contain_text('Chưa xác nhận được thay đổi')
    expect(panel.get_by_role('button',name='Làm mới phiên đăng nhập')).to_have_count(0)
    panel.get_by_role('button',name='Tải trạng thái đăng nhập').click()
    expect(panel).to_contain_text('Tài khoản đã kết nối')
    expect(page.locator('#toast')).not_to_have_class('toast show')
    panel.screenshot(path=str(OUT/'oauth-mobile.png'))
    assert page.evaluate('document.documentElement.scrollWidth<=innerWidth')
    page.set_viewport_size(dict(width=1440,height=1000));panel.screenshot(path=str(OUT/'oauth-desktop.png'))
    panel.get_by_role('button',name='Ngắt kết nối trên Aurora').click()
    expect(panel).to_contain_text('Chưa kết nối tài khoản')
    print('PASS lost refresh acknowledgement requires status lookup without replay, then local disconnect clears token',flush=True)
    count=len(callback_requests)
    popup.goto(callback_url+'&code=duplicate');expect(popup.locator('#oauth-result')).to_contain_text('không hợp lệ')
    assert len(callback_requests)==count and popup.url==BASE+CALLBACK
    popup.reload();expect(popup.locator('#oauth-result')).to_contain_text('không hợp lệ');assert len(callback_requests)==count
    print('PASS duplicate URL parameters and page reload cannot resubmit a code',flush=True)
    context.clear_cookies()
    popup.goto(callback_url);expect(popup.locator('#oauth-result')).to_contain_text('Phiên Aurora chưa đăng nhập')
    assert not context.cookies(BASE)
    print('PASS callback does not bootstrap a missing owner session',flush=True)
    worker=page.evaluate("async()=>{const registration=await Promise.race([navigator.serviceWorker.ready,new Promise((_,reject)=>setTimeout(()=>reject(Error('worker timeout')),10000))]);return registration.active?.state}")
    assert worker=='activated' and popup.evaluate('navigator.serviceWorker.controller!==null')
    cached=page.evaluate("async()=>{const paths=[];for(const key of await caches.keys()){const cache=await caches.open(key);for(const request of await cache.keys())paths.push(new URL(request.url).pathname)}return paths}")
    assert '/app/' in cached and not any(path.startswith('/app/oauth/mcp/callback.') for path in cached)
    assert not errors,errors
    print('PASS actual service worker caches app assets and leaves callback URLs uncached; no page errors',flush=True)
    print('BROWSER '+browser.version,flush=True)
    context.close();browser.close()
