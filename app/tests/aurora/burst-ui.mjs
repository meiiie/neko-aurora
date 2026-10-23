// Composer burst on a phone viewport: 1 question, then 5 quick messages while
// the bot is working (no waiting). Expect one merged reply, no duplicates,
// keyboard focus kept. AURORA_URL must not be the live 8765 server.
import {chromium} from 'playwright-core';
const U=process.env.AURORA_URL||'http://127.0.0.1:8841/app/';if(U.includes(':8765'))throw new Error('not on 8765');
const O=process.env.OUT||'/workspace/neko-bot-shots/aurora/burst/';const sl=ms=>new Promise(r=>setTimeout(r,ms));
const b=await chromium.launch({executablePath:'/usr/bin/google-chrome',args:['--no-sandbox']});
const ctx=await b.newContext({viewport:{width:390,height:844},deviceScaleFactor:2,isMobile:true,hasTouch:true,locale:'vi-VN',storageState:process.env.STATE||undefined});
const p=await ctx.newPage();const errs=[];p.on('pageerror',e=>errs.push(String(e)));
await p.goto(U);await p.waitForFunction(()=>window.__AURORA&&__AURORA.state().agents.length>0,null,{timeout:60000});
const id=await p.evaluate(()=>__AURORA.state().agents[0].id);
await p.evaluate(id=>location.hash=id,id);await p.reload();await p.waitForFunction(()=>window.__AURORA&&__AURORA.current());await sl(500);
const n0=await p.evaluate(id=>(__AURORA.state().msgs[id]||[]).length,id);
const type=async t=>{await p.tap('#input');await p.keyboard.type(t,{delay:5});await p.keyboard.press('Enter')};
const t0=Date.now();
await type(process.env.Q||'Giá vàng SJC hôm nay bao nhiêu?');
await sl(+(process.env.GAP||1200));
const burst=(process.env.BURST||'à mình ở Hải Phòng|so với tuần trước thế nào|nên mua vàng nhẫn hay miếng|trả lời ngắn thôi nhé|thêm một bảng tóm tắt').split('|');
for(const m of burst){await type(m);await sl(250)}
const focused=await p.evaluate(()=>document.activeElement&&document.activeElement.id);
await p.screenshot({path:O+'1-burst-sent.png'});
let st;for(let i=0;i<480;i++){await sl(500);st=await p.evaluate(id=>__AURORA.state().agents.find(a=>a.id===id).status,id);if(i>6&&st!=='working')break;if(i===10)await p.screenshot({path:O+'2-working.png'})}
await sl(1500);await p.evaluate(()=>{const m=document.querySelector('#messages');m.scrollTop=m.scrollHeight});await sl(300);await p.screenshot({path:O+'3-final.png'});
const ms=await p.evaluate(id=>__AURORA.state().msgs[id],id);const neu=ms.slice(n0);
const bubbles=await p.evaluate(()=>[...document.querySelectorAll('#messages .msg')].map(e=>(e.classList.contains('mine')?'U: ':'B: ')+e.textContent.slice(0,90)));
const chips=await p.evaluate(()=>[...document.querySelectorAll('#messages .sources summary')].map(e=>e.textContent));
console.log(JSON.stringify({secs:((Date.now()-t0)/1000).toFixed(1),focusedAfterSend:focused,users:neu.filter(m=>m.role==='user').length,agents:neu.filter(m=>m.role==='agent').length,system:neu.filter(m=>m.role==='system').length,chips,errs,bubbles:bubbles.slice(-9)},null,1));
await b.close();
