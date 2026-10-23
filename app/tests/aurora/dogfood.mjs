// Realistic hosted dogfooding on a phone viewport with real GLM: onboarding
// variant, English research with sources, multi-round context, markdown
// table/code, refresh mid-turn, offline blip, very long message, rate burst.
// U=https://… EMAIL=x@example.com (in AURORA_DEV_EMAILS) OUT=dir node dogfood.mjs
import {chromium} from 'playwright-core';
const U=process.env.U,EMAIL=process.env.EMAIL,O=process.env.OUT||'/tmp/dog/';if(!U||U.includes(':8765'))throw new Error('U required, never 8765');
const sl=ms=>new Promise(r=>setTimeout(r,ms));const res={};const ok=(k,v,x)=>{res[k]=!!v;console.log(v?'PASS':'FAIL',k,x===undefined?'':JSON.stringify(x).slice(0,700))};
const b=await chromium.launch({executablePath:'/usr/bin/google-chrome',args:['--no-sandbox']});
const ctx=await b.newContext({viewport:{width:390,height:844},deviceScaleFactor:2,isMobile:true,hasTouch:true,locale:'en-US',userAgent:'Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1'});
const p=await ctx.newPage();const errs=[];p.on('pageerror',e=>errs.push(String(e)));
const H={'content-type':'application/json',origin:U};
const get=async path=>(await p.request.get(U+'/app/api/'+path)).json();
const postj=async(path,data)=>{const r=await p.request.post(U+'/app/api/'+path,{headers:H,data});return {status:r.status(),j:await r.json().catch(()=>null)}};
let bot;const status=async()=>(await get('state')).agents.find(a=>a.id===bot);
const idle=async(max=240)=>{let w=false;for(let i=0;i<max*2;i++){await sl(500);const a=await status();if(a.status==='working')w=true;if(w&&a.status!=='working'||!w&&i>24)return a}};
const open=async()=>{await p.evaluate(id=>location.hash=id,bot);await p.reload();await p.waitForFunction(()=>window.__AURORA&&__AURORA.current(),null,{timeout:30000});await sl(600)};
const bottom=async f=>{await p.evaluate(()=>{const m=document.querySelector('#messages');m.scrollTop=m.scrollHeight});await sl(400);await p.screenshot({path:O+f})};
const type=async t=>{await p.tap('#input');await p.keyboard.type(t,{delay:2});await p.keyboard.press('Enter')};
const msgs=()=>get(`agents/${bot}/messages`);
const turn=async(t,f)=>{const n0=(await msgs()).length;await type(t);await sl(1500);await idle();await sl(2000);const L=(await msgs()).slice(n0);if(f)await bottom(f);return L};
const agents=L=>L.filter(m=>m.role==='agent'&&(m.text||'').trim());
try{
 await p.goto(U+'/app/');await p.waitForURL(/signin\.html/);await p.fill('#email',EMAIL);
 const rp=p.waitForResponse(r=>r.url().endsWith('/app/auth/email'));await p.tap('#sendBtn');const sent=await (await rp).json();
 await p.waitForSelector('#codeStep:not(.hidden)');await p.fill('#code',sent.devCode);await p.waitForURL(/welcome\.html/);await p.screenshot({path:O+'0-welcome-en.png'});
 await p.fill('#name','Sam');await p.tap('[data-goal=finance]');await p.tap('#startBtn');await p.waitForURL(/\/app\/#bot-/,{timeout:30000});await sl(2500);
 const st=await get('state');bot=st.agents.find(a=>a.name==='Aurora')?.id||st.agents[0].id;ok('onboarding (finance, English UI)',!!bot,st.agents.map(a=>a.name));
 await open();
 // 1 English research with sources
 let L=await turn('What is the latest stable Python release and when was it released? Cite sources.','1-research-en.png');
 let A=agents(L);ok('research: one reply',A.length===1,A.map(m=>m.text.slice(0,100)));
 ok('research: has sources + citations',A.some(m=>(m.sources||[]).length>0&&/\[\d\]/.test(m.text)),A.map(m=>({s:(m.sources||[]).length,t:m.text.slice(0,200)})));
 ok('research: replies in English',A.some(m=>/\b(the|released|Python)\b/i.test(m.text)&&!/\b(của|được|phiên bản)\b/.test(m.text)),A.map(m=>m.text.slice(0,160)));
 // 2 multi-round context
 L=await turn('And what is the main new feature in it? One sentence.');A=agents(L);
 ok('multi-round: follow-up uses context',A.length===1&&/python|3\.\d+/i.test(A[0].text),A.map(m=>m.text.slice(0,200)));
 // 3 markdown table + code (Vietnamese)
 L=await turn('Lập bảng markdown so sánh 3 loại tiết kiệm (cột: loại, lãi suất ước tính, rủi ro) rồi cho một đoạn code python tính lãi kép 10 năm.','3-table-code.png');
 const dom=await p.evaluate(()=>{const ms=[...document.querySelectorAll('#messages .msg')];const m=ms[ms.length-1];return {table:!!document.querySelector('#messages table'),code:!!document.querySelector('#messages pre code'),over:[...document.querySelectorAll('#messages table, #messages pre')].some(e=>e.getBoundingClientRect().right>window.innerWidth+1&&getComputedStyle(e.parentElement).overflowX!=='auto')}});
 ok('markdown: table + code rendered',dom.table&&dom.code,dom);ok('markdown: nothing overflows the phone screen',!dom.over,dom);
 // 4 refresh mid-turn
 let n0=(await msgs()).length;await type('Tóm tắt ngắn 5 ý về cách lập ngân sách 50/30/20.');await sl(2500);await p.reload();await p.waitForFunction(()=>window.__AURORA&&__AURORA.current(),null,{timeout:30000});
 await idle();await sl(2500);L=(await msgs()).slice(n0);A=agents(L);await bottom('4-refresh-mid-turn.png');
 const shown=await p.evaluate(()=>[...document.querySelectorAll('#messages .msg.agent')].length);
 ok('refresh mid-turn: exactly one reply, user msg once',A.length===1&&L.filter(m=>m.role==='user').length===1,L.map(m=>m.role+':'+(m.text||'').slice(0,60)));
 // 5 offline blip while sending
 n0=(await msgs()).length;await ctx.setOffline(true);await type('Tin nhắn lúc mất mạng: 1+1 bằng mấy?');await sl(800);
 const pend=await p.evaluate(()=>!!document.querySelector('#messages .msg.pending'));await p.screenshot({path:O+'5-offline-pending.png'});
 await sl(3000);await ctx.setOffline(false);await sl(2000);await idle();await sl(2000);L=(await msgs()).slice(n0);
 ok('offline blip: shown pending, delivered once after reconnect',pend&&L.filter(m=>m.role==='user').length===1&&agents(L).length===1,{pend,L:L.map(m=>m.role+':'+(m.text||'').slice(0,60))});
 // 6 very long message
 const long='Đoạn văn dài. '.repeat(900);await p.fill('#input',long);await p.keyboard.press('Enter');await sl(1500);
 const toast=await p.evaluate(()=>document.querySelector('.toast,#toast')?.textContent||'');const kept=(await p.inputValue('#input')).length;
 ok('too long (>10k chars): refused with a clear message, text kept',/quá dài|too long/i.test(toast)&&kept>10000,{toast,kept});await p.fill('#input','');
 n0=(await msgs()).length;L=await turn(('Ghi chú họp: '+'khách hàng muốn giảm chi phí vận hành, ').repeat(60)+' Tóm tắt 3 ý.');
 ok('long but valid (~3k chars): stored whole, answered once',L.find(m=>m.role==='user')?.text.length>2500&&agents(L).length===1,{len:L.find(m=>m.role==='user')?.text.length,a:agents(L).length});
 // 7 rate burst via API (25 fast sends): 429s must not create duplicates
 n0=(await msgs()).length;const codes=[];for(let i=0;i<25;i++){const r=await postj(`agents/${bot}/messages`,{text:'ping '+i,clientNonce:'dog-rate-'+i+'-'+Date.now()});codes.push(r.status)}
 await idle(300);await sl(2000);L=(await msgs()).slice(n0);const acc=codes.filter(c=>c<300).length;
 ok('rate burst: 429 after limit, accepted ones stored once, one merged reply',codes.includes(429)&&L.filter(m=>m.role==='user').length===acc&&agents(L).length<=3,{codes,users:L.filter(m=>m.role==='user').length,agents:agents(L).length});
 await open();await bottom('7-after-burst.png');
 ok('no page errors',errs.length===0,errs);
}finally{
 const d=await p.request.post(U+'/app/api/account/delete',{headers:H,data:{confirm:'DELETE'}});await sl(1500);const me=await p.request.get(U+'/app/auth/me');ok('test account deleted',d.ok()&&me.status()===401);
 console.log('RESULT',JSON.stringify(res));await b.close()}
