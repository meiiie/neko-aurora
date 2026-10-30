"""Reload recovery using local assets, synthetic APIs and controlled read failures."""
import functools
import http.server
import json
import os
import threading
from pathlib import Path
from urllib.parse import urlsplit
from playwright.sync_api import expect, sync_playwright

ROOT = Path(__file__).resolve().parents[3]
OUT = ROOT / '.audit/2026-10-29-composer-reload'
OUT.mkdir(parents=True, exist_ok=True)


class Handler(http.server.SimpleHTTPRequestHandler):
    def log_message(self, *_):
        pass


class Server(http.server.ThreadingHTTPServer):
    request_queue_size = 64


server = Server(('127.0.0.1', 0), functools.partial(Handler, directory=str(ROOT / 'app/native/internal/web/assets')))
threading.Thread(target=server.serve_forever, daemon=True).start()
try:
    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True, executable_path=os.environ.get('AURORA_TEST_CHROME', 'C:/Program Files/Google/Chrome/Application/chrome.exe'))
        try:
            context = browser.new_context(viewport={'width': 390, 'height': 844}, locale='vi-VN', service_workers='block')
            page = context.new_page()
            errors, failures, posts, reads, held, auth = [], [], [], [], [], []
            page.on('pageerror', lambda e: errors.append(str(e)))
            page.on('requestfailed', lambda q: failures.append({'url':q.url, 'failure':q.failure}))
            mode = 'ok'
            state = {'workspaceId':'startup-fixture', 'agents':[{'id':'bot-a', 'name':'Reload bot', 'status':'idle', 'av':{}}], 'pending':[], 'ai':{}, 'secrets':[]}

            def api(route):
                req = route.request
                path = urlsplit(req.url).path
                if req.method != 'GET':
                    posts.append(path)
                if path.endswith('/state'):
                    reads.append(path)
                    if mode == 'held':
                        held.append(route)
                        return
                    route.fulfill(status=500 if mode == 'failed' else 401 if mode == 'expired' else 200, json=state)
                elif path.endswith('/messages'):
                    route.fulfill(json=[])
                elif path.endswith('/events'):
                    route.fulfill(content_type='text/event-stream', body='')
                else:
                    route.fulfill(json={})

            page.route('**/app/api/**', api)
            page.route('**/app/auth/config', lambda r:auth.append(r))
            url = f'http://127.0.0.1:{server.server_port}/app/#bot-a'
            page.goto(url)
            expect(page.locator('#whoBtn')).to_contain_text('Reload bot')
            page.locator('#input').fill('Nháp reload · giữ nguyên')
            mode = 'failed'
            page.reload()
            expect(page.locator('#startup')).to_be_visible()
            expect(page.locator('#startupStatus')).to_contain_text('Chưa mở được Neko Aurora')
            print('PASS failed initial state remains actionable after reload')
            page.screenshot(path=str(OUT/'startup-failed-mobile.png'))
            mode = 'ok'
            with page.expect_navigation(wait_until='load'):
                page.locator('#startupRetry').click()
            expect(page.locator('#input')).to_have_value('Nháp reload · giữ nguyên')
            expect(page.locator('#startup')).to_be_hidden()
            assert not posts
            print('PASS retry restores the same chat and draft without sending a message')

            page.route('**/app/app.js', lambda r:r.abort('connectionreset'))
            page.reload()
            expect(page.locator('#startupStatus')).to_contain_text('Chưa mở được Neko Aurora')
            expect(page.locator('#startupRetry')).to_be_enabled()
            assert page.evaluate('typeof window.__AURORA') == 'undefined'
            assert failures[-1]['url'].endswith('/app/app.js')
            print('PASS missing application script exposes a usable reload control')
            page.unroute('**/app/app.js')
            with page.expect_navigation(wait_until='load'):
                page.locator('#startupRetry').click()
            expect(page.locator('#input')).to_have_value('Nháp reload · giữ nguyên')
            print('PASS script recovery preserves draft and deep link')
            mode = 'held'
            page.route('**/app/composer-drafts.js', lambda r:r.abort('connectionreset'))
            page.reload()
            expect(page.locator('#startupStatus')).to_contain_text('Chưa mở được Neko Aurora')
            assert errors and all("create" in e for e in errors),errors
            errors.clear()
            assert len(held)==1
            held.pop().fulfill(json=state)
            page.wait_for_timeout(250)
            expect(page.locator('#startupStatus')).to_contain_text('Chưa mở được Neko Aurora')
            expect(page.locator('#thread')).to_have_class('screen thread off')
            assert page.evaluate('window.__AURORA.state().workspaceId') is None
            page.unroute('**/app/composer-drafts.js');mode='ok'
            with page.expect_navigation(wait_until='load'):
                page.locator('#startupRetry').click()
            expect(page.locator('#input')).to_have_value('Nháp reload · giữ nguyên')
            print('PASS late state cannot conceal fatal dependency initialization failure')


            mode = 'held'
            page.reload()
            expect(page.locator('#startupStatus')).to_contain_text('Đang mở Neko Aurora')
            expect(page.locator('#startupRetry')).to_be_enabled()
            page.wait_for_timeout(5500)
            expect(page.locator('#startup')).to_be_visible()
            assert len(held) == 1 and not posts
            held.pop().fulfill(json=state)
            expect(page.locator('#input')).to_have_value('Nháp reload · giữ nguyên')
            expect(page.locator('#startup')).to_be_hidden()
            mode = 'ok'
            print('PASS slow state read remains visible and restores draft once received')

            for i in range(20):
                language = 'en' if i % 2 == 0 else 'vi'
                with page.expect_navigation(wait_until='load'):
                    page.evaluate('(language)=>window.__I18N.set(language)', language)
                expect(page.locator('#input')).to_have_value('Nháp reload · giữ nguyên')
                expect(page.locator('#startup')).to_be_hidden()
                assert page.evaluate('window.__I18N.lang') == language
            print('PASS twenty language reloads preserve the draft and current conversation')
            with page.expect_navigation(wait_until='load'):
                page.evaluate('window.__I18N.set("en")')
            expect(page.locator('#input')).to_have_value('Nháp reload · giữ nguyên')
            mode = 'failed'
            page.set_viewport_size({'width':1280, 'height':900})
            page.reload()
            expect(page.locator('#startupStatus')).to_contain_text('Could not open Neko Aurora')
            expect(page.get_by_role('button', name='Reload', exact=True)).to_be_visible()
            page.locator('#startupRetry').focus()
            expect(page.locator('#startupRetry')).to_be_focused()
            page.screenshot(path=str(OUT/'startup-failed-desktop-en.png'))
            mode = 'ok'
            with page.expect_navigation(wait_until='load'):
                page.keyboard.press('Enter')
            expect(page.locator('#input')).to_have_value('Nháp reload · giữ nguyên')
            print('PASS recovery is translated and keyboard accessible on desktop')
            mode = 'expired'
            page.reload()
            expect(page.locator('#startup')).to_be_visible()
            expect(page.locator('#startupStatus')).to_contain_text('Opening Neko Aurora')
            page.wait_for_function('()=>window.__AURORA !== undefined')
            while not auth: page.wait_for_timeout(20)
            auth.pop().fulfill(json={'hosted':False})
            expect(page.locator('#login')).to_be_visible()
            expect(page.locator('#startup')).to_be_hidden()
            assert not posts and not errors, {'posts':posts, 'errors':errors}
            print('PASS expired session uses sign-in instead of a reload error')
        except Exception:
            print('STARTUP FAILURE', json.dumps({'errors':errors, 'requests':failures}))
            page.screenshot(path=str(OUT/'startup-failure.png'))
            raise
        finally:
            browser.close()
finally:
    server.shutdown()
    server.server_close()
