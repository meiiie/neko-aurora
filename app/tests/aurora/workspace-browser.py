"""Workspace interactions against isolated HTTP fixtures, not provider/device acceptance."""
import functools
import http.server
import json
import os
import threading
from pathlib import Path
from urllib.parse import parse_qs, urlsplit

from playwright.sync_api import expect, sync_playwright
from fixture_http import FixtureHTTPServer

ROOT = Path(__file__).resolve().parents[3]
OUT = Path(os.environ.get('AURORA_BROWSER_AUDIT_DIR', ROOT / '.audit/workspace-browser'))
OUT.mkdir(parents=True, exist_ok=True)


class Static(http.server.SimpleHTTPRequestHandler):
    def log_message(self, *_):
        pass


class Fixture:
    def __init__(self):
        self.scope = 'workspace-design-fixture'
        self.agents = [
            {'id': 'research', 'name': 'An · Nghiên cứu', 'desc': 'Tìm hiểu, đối chiếu và ghi lại điều quan trọng.', 'status': 'idle', 'snip': 'Bản nghiên cứu đã sẵn sàng.', 'av': {'shape': 0, 'color': 4}},
            {'id': 'writer', 'name': 'Minh · Biên tập', 'desc': 'Biến ý tưởng thành nội dung rõ ràng.', 'status': 'working', 'snip': 'Đang soạn bản nháp', 'av': {'shape': 2, 'color': 1}},
            {'id': 'planner', 'name': 'Linh · Kế hoạch', 'desc': 'Sắp xếp công việc và nhắc đúng lúc.', 'status': 'waiting', 'snip': 'Cần bạn xem lại kế hoạch.', 'av': {'shape': 3, 'color': 3}},
            {'id': 'hidden', 'name': 'Bot đã ẩn', 'desc': 'Không hiện trên trang tổng quan', 'status': 'idle', 'av': {'shape': 1, 'color': 2}},
        ]
        for index, agent in enumerate(self.agents):
            agent['t'] = f'2026-10-30T09:0{index}:00Z'
        self.schedules = [
            {'id': 'daily', 'bot': 'research', 'text': 'Tổng hợp những tin đáng chú ý trong ngày', 'at': '08:00', 'timezone': 'Asia/Ho_Chi_Minh', 'repeat': 'daily', 'revision': 1, 'paused': False},
            {'id': 'interval', 'bot': 'planner', 'text': 'Kiểm tra tiến độ kế hoạch', 'repeat': 'interval', 'intervalMinutes': 60, 'revision': 2, 'paused': True},
        ]
        self.fail_schedule = False
        self.foreign_schedule = False
        self.hold_schedule = False
        self.held = []
        self.posts = []
        self.schedule_queries = []

    def route(self, route):
        request = route.request
        parsed = urlsplit(request.url)
        path = parsed.path.removeprefix('/app/api')
        data, status = {}, 200
        if request.method != 'GET':
            self.posts.append((path, request.post_data_json))
        if path == '/state':
            data = {'workspaceId': self.scope, 'agents': self.agents, 'pending': [], 'ai': {'configured': True, 'model': 'Fixture model', 'runtime': 'Neko Core'}, 'secrets': [], 'outbound': {}}
        elif path == '/events':
            route.fulfill(content_type='text/event-stream', body='')
            return
        elif path.endswith('/messages'):
            bot = path.split('/')[2]
            data = [
                {'id': bot + '-u', 'agentId': bot, 'role': 'user', 'text': 'Giúp mình chuẩn bị kế hoạch cho tuần tới.', 't': '2026-10-30T09:10:00Z'},
                {'id': bot + '-a', 'agentId': bot, 'role': 'agent', 'text': '## Bắt đầu từ những việc quan trọng\n\nMình đã chia công việc thành ba phần để bạn dễ theo dõi.\n\n1. **Nghiên cứu** — đối chiếu thông tin và giữ lại nguồn.\n2. **Thực hiện** — hoàn thiện từng phần với kết quả cụ thể.\n3. **Kiểm tra** — xem lại trước khi chia sẻ.\n\nBạn có thể tiếp tục trao đổi ở đây, hoặc mở thư viện trang để ghi lại ý tưởng.', 't': '2026-10-30T09:11:00Z'},
            ]
        elif path == '/schedules':
            query = parse_qs(parsed.query)
            self.schedule_queries.append(query)
            if 'workspaceId' in query and self.hold_schedule:
                self.held.append((route, self.scope))
                return
            if self.fail_schedule:
                status, data = 503, {'error': 'storage'}
            else:
                rows = [row for row in self.schedules if not query.get('bot') or row['bot'] == query['bot'][0]]
                data = {'workspaceId': 'foreign' if self.foreign_schedule else self.scope, 'schedules': rows}
        elif path.endswith('/computer'):
            data = {'available': False, 'files': [], 'log': [], 'usedBytes': 0, 'quotaBytes': 1048576}
        elif path.endswith('/memory'):
            data = {'workspaceId': self.scope, 'revision': 0, 'items': []}
        elif path.endswith('/ext'):
            data = {'workspaceId': self.scope, 'mcpRevision': 0, 'menu': [], 'allSkills': [], 'mcp': []}
        elif path == '/pages':
            data = {'workspaceId': self.scope, 'pages': []}
        elif path in ('/activity', '/search'):
            data = [] if path == '/activity' else {'hits': []}
        elif path == '/vault':
            data = {'workspaceId': self.scope, 'items': []}
        route.fulfill(status=status, content_type='application/json', body=json.dumps(data))


server = FixtureHTTPServer(('127.0.0.1', 0), functools.partial(Static, directory=str(ROOT / 'app/native/internal/web/assets')))
threading.Thread(target=server.serve_forever, daemon=True).start()
fixture = Fixture()
checks = []


def passed(label):
    checks.append(label)
    print('PASS ' + label, flush=True)


with sync_playwright() as playwright:
    browser = playwright.chromium.launch(executable_path='C:/Program Files/Google/Chrome/Application/chrome.exe', headless=True)
    context = browser.new_context(viewport={'width': 1440, 'height': 1000}, reduced_motion='reduce', service_workers='block')
    context.add_init_script("""if(!localStorage.getItem('neko-aurora-lang'))localStorage.setItem('neko-aurora-lang','vi');
        localStorage.setItem('neko-aurora-hidden',JSON.stringify(['hidden']));
        window.EventSource=class extends EventTarget {constructor(){super();this.readyState=1}close(){this.readyState=2}};""")
    page = context.new_page()
    errors = []
    page.on('pageerror', lambda error: errors.append(str(error)))
    page.route('**/app/api/**', fixture.route)
    try:
        page.goto(f'http://127.0.0.1:{server.server_port}/app/')
        page.wait_for_load_state('networkidle')
        expect(page.locator('#workspaceHome')).to_be_visible()
        expect(page.locator('#workspaceBotCount')).to_have_text('3')
        expect(page.locator('#workspaceWorkingCount')).to_have_text('1')
        expect(page.locator('#workspaceWaitingCount')).to_have_text('1')
        assert page.locator('#list').bounding_box()['width'] == 288
        assert page.locator('#workspaceHome').bounding_box()['width'] > 900
        page.screenshot(path=str(OUT / 'workspace-home-light.png'))
        passed('desktop uses the full workspace and real state counts')
        expect(page.locator('#workspaceRecent')).not_to_contain_text('Bot đã ẩn')
        page.locator('[data-workspace-action="pages"]').click()
        expect(page.locator('#pagesDialog')).to_be_visible()
        page.get_by_role('button', name='Đóng thư viện', exact=True).click()
        expect(page.locator('[data-workspace-action="pages"]')).to_be_focused()
        page.locator('[data-workspace-action="apps"]').click()
        expect(page.locator('#connectors')).to_be_visible()
        page.locator('#closeConnectors').click()
        expect(page.locator('#connectors')).not_to_be_visible()
        page.wait_for_timeout(150)
        page.locator('[data-workspace-action="settings"]').click()
        expect(page.locator('#settings')).to_be_visible()
        page.locator('#closeSettings').click()
        expect(page.locator('#settings')).not_to_be_visible()
        page.wait_for_timeout(150)
        passed('rail actions open the existing library, connectors and settings')

        page.locator('[data-workspace-filter="working"]').click()
        expect(page.locator('#rows .row')).to_have_count(1)
        expect(page.locator('#rows .row')).to_have_attribute('data-id', 'writer')
        page.locator('[data-workspace-filter="waiting"]').click()
        expect(page.locator('#rows .row')).to_have_attribute('data-id', 'planner')
        page.locator('[data-workspace-filter="all"]').click()
        passed('activity filters operate on the actual bot list')

        row = lambda bot: page.locator(f'#rows [data-id="{bot}"] .rowin')
        pin = page.locator('#rows [data-id="research"] .pin')
        pin.focus()
        expect(pin).to_be_focused()
        assert pin.evaluate('n=>{const b=n.getBoundingClientRect();return document.elementFromPoint(b.x+b.width/2,b.y+b.height/2)?.closest(".pin")===n}')
        row('research').focus()
        row('research').click()
        expect(page.locator('#thread')).to_be_visible()
        expect(page.locator('#list')).not_to_have_attribute('inert', '')
        page.locator('#input').fill('Bản nháp cần được giữ khi chuyển bot')
        row('writer').click()
        expect(page.locator('#input')).to_have_value('')
        row('research').click()
        expect(page.locator('#input')).to_have_value('Bản nháp cần được giữ khi chuyển bot')
        expect(row('research')).to_have_attribute('aria-current', 'true')
        assert not fixture.posts
        page.screenshot(path=str(OUT / 'workspace-chat-light.png'))
        passed('persistent sidebar switches bots while retaining drafts without sending')

        page.locator('#input').focus()
        page.keyboard.press('Control+k')
        expect(page.locator('#workspacePalette')).to_be_visible()
        page.locator('#workspaceQuery').fill('nghien cuu')
        expect(page.locator('#workspaceResults [role=option]')).to_have_count(1)
        page.locator('#workspaceQuery').dispatch_event('keydown', {'key': 'Enter', 'isComposing': True})
        expect(page.locator('#workspacePalette')).to_be_visible()
        page.keyboard.press('Escape')
        expect(page.locator('#input')).to_be_focused()
        page.keyboard.press('Control+k')
        page.locator('#workspaceQuery').fill('ke hoach')
        page.keyboard.press('Enter')
        expect(row('planner')).to_have_attribute('aria-current', 'true')
        passed('command palette handles Vietnamese search, IME, Enter and return focus')

        page.locator('#whoBtn').click()
        expect(page.locator('#profile')).not_to_have_class('screen profile off')
        assert page.locator('#profile').bounding_box()['width'] == 360
        assert page.locator('#thread').bounding_box()['width'] >= 700
        page.screenshot(path=str(OUT / 'workspace-context-light.png'))
        row('research').click()
        expect(page.locator('#profile')).to_have_class('screen profile off')
        expect(page.locator('#input')).to_have_value('Bản nháp cần được giữ khi chuyển bot')
        row('planner').click()
        passed('bot context opens alongside chat on a wide desktop')

        page.locator('#workspaceSidebarToggle').click()
        expect(page.locator('#list')).to_have_attribute('inert', '')
        page.reload()
        expect(page.locator('#app')).to_have_attribute('data-sidebar-collapsed', 'true')
        page.locator('#workspaceSidebarToggle').click()
        expect(row('planner')).to_be_visible()
        passed('sidebar density survives reload without changing the conversation')

        page.locator('[data-workspace-action="routines"]').click()
        expect(page.locator('.workspace-routine')).to_have_count(2)
        assert fixture.schedule_queries[-1]['workspaceId'] == [fixture.scope]
        page.locator('#workspaceRoutineFilter').select_option('planner')
        expect(page.locator('.workspace-routine')).to_have_count(1)
        expect(page.locator('.workspace-routine')).to_contain_text('Mỗi 60 phút')
        page.locator('#workspaceRoutineFilter').select_option('')
        page.screenshot(path=str(OUT / 'workspace-routines-light.png'))
        fixture.fail_schedule = True
        page.locator('#workspaceRoutineReload').click()
        expect(page.locator('#workspaceRoutineStatus')).to_have_text('Chưa tải được lịch chạy. Hãy thử lại.')
        expect(page.locator('.workspace-routine')).to_have_count(0)
        fixture.fail_schedule = False
        fixture.foreign_schedule = True
        page.locator('#workspaceRoutineReload').click()
        expect(page.locator('#workspaceRoutineStatus')).to_have_text('Chưa tải được lịch chạy. Hãy thử lại.')
        expect(page.locator('.workspace-routine')).to_have_count(0)
        fixture.foreign_schedule = False
        page.locator('#workspaceRoutineReload').click()
        expect(page.locator('.workspace-routine')).to_have_count(2)
        page.locator('.workspace-routine').first.get_by_role('button').click()
        expect(page.locator('#threadName')).to_have_text('An · Nghiên cứu')
        expect(page.locator('#schedBox')).to_be_visible()
        page.locator('#profBack').click()
        passed('routines overview filters, recovers failures, rejects wrong scope and opens the owning bot')

        fixture.hold_schedule = True
        page.locator('[data-workspace-action="routines"]').click()
        expect(page.locator('#workspaceRoutineReload')).to_be_disabled()
        row('research').click()
        expect(page.locator('#threadName')).to_have_text('An · Nghiên cứu')
        page.locator('#backBtn').click()
        expect(page.locator('#workspaceHome')).to_be_visible()
        for route, _ in fixture.held:
            try:
                route.abort()
            except Exception:
                pass
        fixture.held.clear()
        fixture.hold_schedule = False
        page.locator('[data-workspace-action="routines"]').click()
        expect(page.locator('.workspace-routine')).to_have_count(2)
        expect(page.locator('#workspaceRoutineReload')).to_be_enabled()
        passed('opening a sidebar chat cancels a pending overview and returning can load it again')

        page.locator('[data-workspace-action="routines"]').click()
        expect(page.locator('.workspace-routine')).to_have_count(2)
        fixture.hold_schedule = True
        page.locator('[data-workspace-filter="working"]').click()
        page.locator('#workspaceRoutineReload').click()
        page.wait_for_function("document.querySelector('#workspaceRoutineReload').disabled")
        fixture.scope = 'different-workspace'
        fixture.agents = [dict(fixture.agents[0], name='Workspace mới')]
        page.evaluate('window.__AURORA.reload()')
        expect(page.locator('#workspaceHome')).to_be_visible()
        for route, old_scope in fixture.held:
            try:
                route.fulfill(content_type='application/json', body=json.dumps({'workspaceId': old_scope, 'schedules': fixture.schedules}))
            except Exception:
                pass
        fixture.held.clear()
        fixture.hold_schedule = False
        expect(page.locator('#workspaceRoutineRows')).to_be_empty()
        expect(page.locator('#workspaceRecent')).not_to_contain_text('Kế hoạch')
        expect(row('research')).to_be_visible()
        expect(page.locator('[data-workspace-filter="all"]')).to_have_attribute('aria-pressed', 'true')
        page.locator('#workspaceRecent .workspace-agent').first.focus()
        fixture.scope = 'third-workspace'
        page.evaluate('window.__AURORA.reload()')
        expect(page.locator('#workspaceRecent .workspace-agent').first).not_to_be_focused()
        passed('workspace changes clear old results and cancel pending reads')

        page.set_viewport_size({'width': 390, 'height': 844})
        expect(page.locator('#workspaceRail')).not_to_be_visible()
        expect(page.locator('#list')).to_be_visible()
        row('research').click()
        expect(page.locator('#list')).to_have_attribute('inert', '')
        expect(page.locator('#input')).to_have_value('')
        page.screenshot(path=str(OUT / 'workspace-chat-mobile.png'))
        page.locator('#backBtn').click()
        page.locator('#list [data-workspace-go="routines"]').click()
        expect(page.locator('#workspaceRoutines')).to_be_visible()
        expect(page.locator('#workspaceRoutinesBack')).to_be_visible()
        page.screenshot(path=str(OUT / 'workspace-routines-mobile.png'))
        page.locator('#workspaceRoutinesBack').click()
        expect(page.locator('#search')).to_be_focused()
        assert page.evaluate('document.documentElement.scrollWidth <= innerWidth')
        passed('phone retains single-pane navigation, separate drafts and a working routines back button')

        page.set_viewport_size({'width': 1440, 'height': 1000})
        page.evaluate("localStorage.setItem('neko-aurora-theme','dark')")
        page.reload()
        expect(page.locator('html')).to_have_attribute('data-theme', 'dark')
        expect(page.locator('#workspaceHome')).to_be_visible()
        page.screenshot(path=str(OUT / 'workspace-home-dark.png'))
        page.keyboard.press('Control+k')
        page.locator('#workspaceQuery').fill('zzzz no result')
        expect(page.locator('#workspacePaletteEmpty')).to_be_visible()
        page.keyboard.press('Enter')
        expect(page.locator('#workspacePalette')).to_be_visible()
        for _ in range(4):
            page.keyboard.press('Tab')
            assert page.evaluate("document.querySelector('#workspacePalette').contains(document.activeElement)")
        page.keyboard.press('Escape')
        passed('dark theme, empty palette results and dialog keyboard containment')
        fixture.agents[0]['name'] = 'Cài đặt'
        page.evaluate("localStorage.setItem('neko-aurora-lang','en')")
        page.reload()
        expect(page.locator('html')).to_have_attribute('lang', 'en')
        expect(page.locator('#workspaceHomeTitle')).to_have_text('Good work starts here.')
        expect(page.locator('#workspaceRecent')).to_contain_text('Cài đặt')
        page.screenshot(path=str(OUT / 'workspace-home-english-dark.png'))
        passed('English chrome leaves owner-authored names unchanged')
        for width in (320, 768, 900, 1200, 1440):
            page.set_viewport_size({'width': width, 'height': 900})
            row('research').click()
            expect(page.locator('#input')).to_be_visible()
            box = page.locator('#composer').bounding_box()
            assert box['x'] >= 0 and box['x'] + box['width'] <= width + 1, (width, box)
            assert page.evaluate('document.documentElement.scrollWidth <= innerWidth')
            page.locator('#backBtn').click()
        passed('composer and shell fit phone, tablet, compact desktop and wide desktop')
        assert not errors, errors
        assert not fixture.posts, fixture.posts
        passed('navigation causes no mutation or browser exception')
        (OUT / 'result.json').write_text(json.dumps({'checks': checks, 'errors': errors}, indent=2), encoding='utf-8')
    except Exception:
        page.screenshot(path=str(OUT / 'workspace-failure.png'))
        print(json.dumps({'errors': errors, 'body': page.locator('body').inner_text()[-2500:]}), flush=True)
        raise
    finally:
        browser.close()
        server.shutdown()
        server.server_close()
