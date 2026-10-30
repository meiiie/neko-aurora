"""Selected message excerpts use the real UI with loopback state fixtures."""
import functools,http.server,json,os,threading
from pathlib import Path
from urllib.parse import urlsplit
from playwright.sync_api import expect,sync_playwright
ROOT=Path(__file__).resolve().parents[3]
OUT=ROOT/'.audit/2026-10-28-selection';OUT.mkdir(parents=True,exist_ok=True)
class Server(http.server.ThreadingHTTPServer):request_queue_size=64
class Handler(http.server.SimpleHTTPRequestHandler):
 def log_message(self,*_):pass
class Fixture:
 def __init__(self):self.scope='excerpt-owner-a';self.posts=[]
 def route(self,route):
  req=route.request;p=urlsplit(req.url).path.removeprefix('/app/api');data={}
  if p=='/events':route.fulfill(status=200,content_type='text/event-stream',body='');return
  if p=='/state':data={'workspaceId':self.scope,'agents':[{'id':'bot-'+x,'name':'Chat '+x.upper(),'status':'idle','av':{'shape':0,'color':4}} for x in ['a','b']],'pending':[],'ai':{},'secrets':[],'outbound':{}}
  elif p.endswith('/messages'):
   if req.method=='POST':self.posts.append(req.post_data_json)
   data=[{'id':'m1','taskId':'t1','agentId':'bot-a','role':'agent','text':'Đây là **đoạn cần chọn** với tiếng Việt và 👩🏽‍💻.\n\nDòng hai giữ nguyên.','t':'2026-10-28T04:00:00Z'},{'id':'m2','taskId':'t2','agentId':'bot-a','role':'user','text':'Tin của tôi','t':'2026-10-28T04:01:00Z'},{'id':'long','taskId':'long','agentId':'bot-a','role':'agent','text':'x'*2100,'t':'2026-10-28T04:02:00Z'}] if 'bot-a' in p else []
  route.fulfill(status=200,content_type='application/json',body=json.dumps(data))
def check(s):print('PASS '+s,flush=True)
server=Server(('127.0.0.1',0),functools.partial(Handler,directory=str(ROOT/'app/native/internal/web/assets')))
threading.Thread(target=server.serve_forever,daemon=True).start()
try:
 with sync_playwright() as p:
  chrome=os.environ.get('AURORA_TEST_CHROME')
  if not chrome and Path('C:/Program Files/Google/Chrome/Application/chrome.exe').exists():chrome='C:/Program Files/Google/Chrome/Application/chrome.exe'
  browser=p.chromium.launch(headless=True,executable_path=chrome)
  try:
   context=browser.new_context(viewport={'width':390,'height':844},locale='vi-VN',service_workers='block');page=context.new_page();fx=Fixture();errors=[]
   page.route('**/app/api/**',fx.route);page.on('pageerror',lambda e:errors.append(str(e)))
   page.on('requestfailed',lambda r:print('REQUEST FAILED',r.url,r.failure,flush=True))
   page.goto(f'http://127.0.0.1:{server.server_port}/app/#bot-a',wait_until='networkidle');expect(page.locator('#whoBtn')).to_contain_text('Chat A')
   pill=page.locator('.selection-excerpt');expect(pill).to_be_hidden()
   def select(selector):
    try:page.wait_for_function('Math.abs(document.querySelector("#thread").getBoundingClientRect().left)<1')
    except Exception:
     print('LAYOUT DIAGNOSTICS',json.dumps(page.evaluate('''()=>({thread:document.querySelector('#thread').getBoundingClientRect().toJSON(),appScroll:document.querySelector('#app').scrollLeft,width:innerWidth,scrollX,styles:[...document.styleSheets].map(s=>s.href),className:document.querySelector('#thread').className})''')),flush=True)
     page.screenshot(path=str(OUT/'selection-layout-failure.png'));raise
    node=page.locator(selector);node.scroll_into_view_if_needed();node.evaluate('''node=>{const range=document.createRange();range.selectNodeContents(node);const sel=getSelection();sel.removeAllRanges();sel.addRange(range)}''')
    try:expect(pill).to_be_visible()
    except AssertionError:
     print(json.dumps(page.evaluate("()=>({selection:getSelection().toString(),source:document.querySelector('[data-message-id=m1] .msg').getBoundingClientRect().toJSON(),root:document.querySelector('#messages').getBoundingClientRect().toJSON(),scroll:document.querySelector('#messages').scrollTop})")));raise
   page.locator('#input').fill('Nháp có sẵn')
   page.locator('.reply-action[data-id="m2"]').click();expect(page.locator('#replyChip')).to_contain_text('Tin của tôi')
   select('[data-message-id="m1"] .msg strong')
   page.evaluate('window.__AURORA.render()');expect(pill).to_be_visible();assert page.evaluate('getSelection().toString()')=='đoạn cần chọn'
   check('unchanged message rerender preserves the actual selected range')
   page.screenshot(path=str(OUT/'selection-mobile.png'))
   box=pill.bounding_box();assert box['x']>=0 and box['x']+box['width']<=390
   pill.click();expect(page.locator('#input')).to_have_value('Nháp có sẵn\n\nTrích từ Chat A:\n> đoạn cần chọn\n\n');expect(page.locator('#replyChip')).to_contain_text('Tin của tôi');expect(page.locator('#input')).to_be_focused()
   assert not fx.posts
   page.reload();expect(page.locator('#input')).to_have_value('Nháp có sẵn\n\nTrích từ Chat A:\n> đoạn cần chọn\n\n')
   check('partial rendered text appends to durable draft without replacing an existing quote or sending')
   select('[data-message-id="m1"] .msg p:nth-child(2)');pill.focus();page.keyboard.press('Enter')
   assert '> Dòng hai giữ nguyên.' in page.locator('#input').input_value()
   check('keyboard activation inserts the selected paragraph')
   select('[data-message-id="m1"] .msg strong');page.locator('#messages').evaluate('n=>n.scrollTop=n.scrollHeight');expect(pill).to_be_hidden();page.locator('#messages').evaluate('n=>n.scrollTop=0');expect(pill).to_be_visible()
   check('selection control follows visible source and hides while it is scrolled away')
   before=page.locator('#input').input_value();select('[data-message-id="m2"] .msg');pill.focus();page.keyboard.press('Escape');expect(pill).to_be_hidden();expect(page.locator('[data-message-id="m2"]')).to_be_focused();expect(page.locator('#input')).to_have_value(before)
   check('Escape cancels and returns focus to the source message')
   select('[data-message-id="long"] .msg');pill.click();expect(page.locator('#toast')).to_contain_text('2.000');expect(page.locator('#input')).to_have_value(before)
   page.locator('#input').fill('d'*9990);select('[data-message-id="m2"] .msg');pill.click();expect(page.locator('#toast')).to_contain_text('10.000');expect(page.locator('#input')).to_have_value('d'*9990)
   check('selection and total draft limits reject without truncating or changing content')
   page.locator('#input').fill('A only');select('[data-message-id="m1"] .msg strong');page.locator('#backBtn').click();expect(pill).to_be_hidden();page.locator('#rows .row[data-id="bot-b"] .rowin').click();expect(pill).to_be_hidden();expect(page.locator('#input')).to_have_value('')
   page.locator('#backBtn').click();page.locator('#rows .row[data-id="bot-a"] .rowin').click();expect(page.locator('#input')).to_have_value('A only')
   select('[data-message-id="m1"] .msg strong');fx.scope='excerpt-owner-b';page.evaluate('window.__AURORA.reload()');expect(pill).to_be_hidden();page.locator('#rows .row[data-id="bot-a"] .rowin').click();expect(page.locator('#input')).to_have_value('');expect(pill).to_be_hidden()
   check('chat and account changes invalidate captured ranges even with repeated message IDs')
   page.locator('[data-message-id="m1"] .msg').evaluate('''node=>{const range=document.createRange();range.setStart(node,0);range.setEnd(document.querySelector('[data-message-id="m2"] .msg'),1);getSelection().removeAllRanges();getSelection().addRange(range)}''');expect(pill).to_be_hidden()
   page.locator('#threadName').evaluate('''node=>{const r=document.createRange();r.selectNodeContents(node);getSelection().removeAllRanges();getSelection().addRange(r)}''');expect(pill).to_be_hidden()
   check('cross-message and non-message selections do not offer insertion')
   select('[data-message-id="m1"] .msg strong');page.locator('[data-message-id="m1"] .msg strong').evaluate('n=>n.textContent="changed while selected"');pill.click();expect(page.locator('#input')).to_have_value('');expect(pill).to_be_hidden()
   check('changed source range cannot insert a stale capture')
   page.reload();page.wait_for_load_state('networkidle')
   with page.expect_navigation(wait_until="networkidle"):page.evaluate('window.__I18N.set("en")')
   select('[data-message-id="m1"] .msg strong');expect(pill).to_have_text('Add to chat');expect(pill).to_have_attribute('aria-label','Add selected text to draft');pill.click();assert '> đoạn cần chọn' in page.locator('#input').input_value()
   with page.expect_navigation(wait_until="networkidle"):page.evaluate('window.__I18N.set("vi")')
   select('[data-message-id="m1"] .msg p:first-child');pill.click();assert '👩🏽‍💻' in page.locator('#input').input_value()
   assert not fx.posts and not errors,errors
   check('translated control leaves Vietnamese and Unicode selected content unchanged')
   (OUT/'selection-browser-result.json').write_text(json.dumps({'result':'PASS','scenarios':10,'browser':browser.version,'viewport':[390,844],'chatPosts':len(fx.posts),'errors':errors},indent=2),encoding='utf-8')
  finally:browser.close()
finally:server.shutdown();server.server_close()
