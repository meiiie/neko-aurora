// Live hosted verification over the public URL with a throwaway account:
// burst messaging, multi-step plan, computer command approved via API,
// duplicate-bubble and status-chip checks, then account deletion.
// U=https://…railway.app EMAIL=x@example.com (must be in AURORA_DEV_EMAILS) node live-verify.mjs
import {chromium} from 'playwright-core';
const U=process.env.U,EMAIL=process.env.EMAIL,O=process.env.OUT||'/tmp/live/';if(!U||U.includes(':8765'))throw new Error('U required, never 8765');
const sl=ms=>new Promise(r=>setTimeout(r,ms));const res={};const ok=(k,v,x)=>{res[k]=!!v;console.log(v?'PASS':'FAIL',k,x===undefined?'':JSON.stringify(x))};
const b=await chromium.launch({executablePath:'/usr/bin/google-chrome',args:['--no-sandbox']});
const ctx=await b.newContext({viewport:{width:390,height:844},deviceScaleFactor:2,isMobile:true,hasTouch:true,locale:'vi-VN',userAgent:'Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1'});
const p=await ctx.newPage();const errs=[];p.on('pageerror',e=>errs.push(String(e)));
const H={'content-type':'application/json',origin:U};
const get=async path=>(await p.request.get(U+'/app/api/'+path)).json();
const postj=async(path,data)=>{const r=await p.request.post(U+'/app/api/'+path,{headers:H,data});return {status:r.status(),j:await r.json().catch(()=>null)}};
try{
 await p.goto(U+'/app/');await p.waitForURL(/signin\.html/);await p.fill('#email',EMAIL);
 const rp=p.waitForResponse(r=>r.url().endsWith('/app/auth/email'));await p.tap('#sendBtn');const sent=await (await rp).json();
 await p.waitForSelector('#codeStep:not(.hidden)');await p.fill('#code',sent.devCode);await p.waitForURL(/welcome\.html/);
 await p.fill('#name','Tester');await p.tap('[data-goal=research]');await p.tap('#startBtn');await p.waitForURL(/\/app\/#bot-/,{timeout:30000});await sl(2500);
 const bot=(await get('state')).agents.find(a=>a.name==='Aurora').id;ok('signed in + onboarded',!!bot);
 const status=async()=>(await get('state')).agents.find(a=>a.id===bot);
 const idle=async(max=300)=>{let w=false;for(let i=0;i<max*2;i++){await sl(500);const a=await status();if(a.status==='working')w=true;if(w&&a.status!=='working'||!w&&i>24)return a}};
 const open=async()=>{await p.evaluate(id=>location.hash=id,bot);await p.reload();await p.waitForFunction(()=>window.__AURORA&&__AURORA.current(),null,{timeout:30000});await sl(600)};
 const type=async t=>{await p.tap('#input');await p.keyboard.type(t,{delay:4});await p.keyboard.press('Enter')};
 const msgs=()=>get(`agents/${bot}/messages`);
 const ONLY=process.env.ONLY;let n0=0,ms=[];
 if(!ONLY||ONLY==='burst'){
 await open();n0=(await msgs()).length;
 await type('Bạn có máy tính không?');await sl(900);
 for(const m of ['máy đó chạy hệ điều hành gì','có python không','liệt kê giúp mình thư mục nhà','trả lời ngắn gọn','cảm ơn nhé']){await type(m);await sl(200)}
 ok('composer keeps focus after send',await p.evaluate(()=>document.activeElement?.id==='input'));
 await p.screenshot({path:O+'1-burst.png'});await idle();await sl(2500);
 ms=(await msgs()).slice(n0);let pend=ms.find(m=>m.approval&&m.approval.state==='pending');
 if(pend){await postj(`approvals/${pend.taskId||pend.id}`,{decision:'approved'});await sl(2000);await idle();await sl(2000);ms=(await msgs()).slice(n0)}
 await open();await p.evaluate(()=>{const m=document.querySelector('#messages');m.scrollTop=m.scrollHeight});await sl(500);await p.screenshot({path:O+'2-burst-final.png'});
 const ag=ms.filter(m=>m.role==='agent'&&(m.text||'').trim());const us=ms.filter(m=>m.role==='user');
 ok('burst: 6 user messages stored once',us.length===6,us.length);
 ok('burst: one merged reply (no preamble/duplicate bubble)',ag.length===1,ag.map(m=>m.text.slice(0,80)));
 ok('burst: user messages shown before the reply',ms.findIndex(m=>m.role==='agent'&&(m.text||'').trim())>ms.findLastIndex(m=>m.role==='user'));
 const chips=await p.evaluate(()=>[...document.querySelectorAll('#messages .sources summary')].map(e=>e.textContent));
 const tools=ag.flatMap(m=>m.tools||[]);ok('chip label matches tool status',!chips.some(c=>/Không lấy được/.test(c))||tools.every(t=>t.status==='failed'),{chips,tools:tools.map(t=>t.name+':'+t.status)});
 }
 if(!ONLY||ONLY==='plan'){
 n0=(await msgs()).length;await postj(`agents/${bot}/messages`,{text:'Làm giúp mình: (1) tra giá vàng thế giới hôm nay (2) tra giá vàng SJC (3) viết nhận định ngắn cho người mua.',clientNonce:'live-plan-'+Date.now()});
 const doing=new Set();for(let i=0;i<600;i++){await sl(500);const a=await status();if(a.doing)doing.add(a.doing);if(i>6&&a.status!=='working')break}
 await sl(1500);ms=(await msgs()).slice(n0);await open();await p.evaluate(()=>{const m=document.querySelector('#messages');m.scrollTop=m.scrollHeight});await sl(400);await p.screenshot({path:O+'3-plan.png'});
 const steps=ms.filter(m=>/:step\d+$/.test(m.id)).length;const fin=ms.filter(m=>m.role==='agent').at(-1);
 ok('plan: 3 milestone updates + final with SJC price',steps===3&&/14\d|SJC/.test(fin?.text||''),{steps,doing:[...doing],fin:(fin?.text||'').slice(0,160)});
 }
 if(ONLY==='sandbox'){
 const shot=async f=>{await open();await p.evaluate(()=>{const m=document.querySelector('#messages');m.scrollTop=m.scrollHeight});await sl(400);await p.screenshot({path:O+f})};
 const cmdTurn=async(text,re,tag)=>{const s0=(await msgs()).length;await postj(`agents/${bot}/messages`,{text,clientNonce:'live-sb-'+tag+Date.now()});
  let ap=null;for(let i=0;i<150&&!ap;i++){await sl(1000);ap=(await msgs()).slice(s0).find(m=>m.approval&&m.approval.state==='pending')}
  ok(tag+': bot asks approval',!!ap,ap?undefined:(await msgs()).slice(s0).map(m=>m.role+':'+(m.text||'').slice(0,160)));if(!ap)return;
  await shot('sb-'+tag+'-approval.png');const r=await postj(`approvals/${ap.taskId||ap.id}`,{decision:'approved'});ok(tag+': approved via API',r.status<300,r.status);
  const t0=Date.now();let hit=false;for(let i=0;i<180&&!hit;i++){await sl(1000);hit=re.test(JSON.stringify((await msgs()).slice(s0)))}
  ok(tag+': command ran, output back',hit,{secs:Math.round((Date.now()-t0)/1000)});await idle(120);await sl(2000);await shot('sb-'+tag+'-done.png');
  const L=(await msgs()).slice(s0);const ag=L.filter(m=>m.role==='agent'&&(m.text||'').trim());
  ok(tag+': output shown once (no duplicate bubble)',ag.filter(m=>re.test(m.text)).length<=1&&ag.filter(m=>/cần bạn duyệt/.test(m.text)).length<=1,L.map(m=>m.role+':'+(m.text||'').slice(0,140)+(m.approval?' AP:'+m.approval.state+' '+(m.approval.outcome||'').slice(0,140):'')))};
 await cmdTurn('Trên máy tính của bạn: tạo /home/bot/sales.csv gồm cột item,qty,price với 3 dòng (tao,3,10000 / cam,5,8000 / le,2,15000), viết /home/bot/stats.py đọc CSV bằng module csv và in "TONG=" + tổng qty*price, rồi chạy python3 /home/bot/stats.py. Báo kết quả.',/TONG=100000/,'csv');
 await cmdTurn('Chạy lại python3 /home/bot/stats.py (đừng tạo lại file) và cho mình biết nó in gì.',/TONG=100000/,'persist');
 const files=await get(`agents/${bot}/computer`).catch(()=>null);console.log('home listing',JSON.stringify(files).slice(0,300));
}
 if(ONLY!=='sandbox'){n0=(await msgs()).length;await postj(`agents/${bot}/messages`,{text:'Trên máy tính của bạn, chạy lệnh: echo xin-chao > /home/bot/hi.txt && cat /home/bot/hi.txt && uname -s',clientNonce:'live-cmd-'+Date.now()});
 let ap=null;for(let i=0;i<120&&!ap;i++){await sl(1000);ap=(await msgs()).slice(n0).find(m=>m.approval&&m.approval.state==='pending')}
 ok('computer: approval card appeared',!!ap,ap?undefined:(await msgs()).slice(n0).map(m=>m.role+':'+(m.text||'').slice(0,120)+' tools='+JSON.stringify(m.tools||[]).slice(0,200)));
 if(ap){await open();await p.evaluate(()=>{const m=document.querySelector('#messages');m.scrollTop=m.scrollHeight});await p.screenshot({path:O+'4-approval.png'});const r=await postj(`approvals/${ap.taskId||ap.id}`,{decision:'approved'});ok('computer: approved via API',r.status<300,r.status);
  let out='';for(let i=0;i<90&&!out;i++){await sl(1000);const L=(await msgs()).slice(n0);const t=L.map(m=>(m.text||'')+JSON.stringify(m.approval||{})).join('\n');if(/xin-chao/.test(t)&&/Linux/.test(t))out=t}
  await idle(60);await sl(1500);await open();await p.evaluate(()=>{const m=document.querySelector('#messages');m.scrollTop=m.scrollHeight});await sl(400);await p.screenshot({path:O+'5-computer-done.png'});
  const L=(await msgs()).slice(n0);ok('computer: one reply after the result (no repeated approval text)',L.filter(m=>m.role==='agent'&&/cần bạn duyệt/.test(m.text||'')).length<=1);ok('computer: real output (xin-chao, Linux) came back',!!out,L.map(m=>m.role+':'+(m.text||'').slice(0,80)+(m.approval?' AP:'+JSON.stringify(m.approval.delivery)+(m.approval.outcome||'').slice(0,100):'')))}
 }
 ok('no page errors',errs.length===0,errs);
}finally{
 const d=await p.request.post(U+'/app/api/account/delete',{headers:H,data:{confirm:'DELETE'}});await sl(1500);const me=await p.request.get(U+'/app/auth/me');ok('test account deleted',d.ok()&&me.status()===401);
 console.log('RESULT',JSON.stringify(res));await b.close()}
