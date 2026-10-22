// Long multi-step task at 390: milestones stream in, the app is closed mid-run, reopened, final result checked.
import {chromium} from 'playwright-core';
const U=process.env.AURORA_URL||'http://127.0.0.1:8765/app/',O=process.env.OUT||'/workspace/neko-bot-shots/aurora/dogfood/',TAG=process.env.TAG||'long';
const BOT=process.env.BOT||'Trợ lý chuyến đi',Q=process.env.Q;const sl=ms=>new Promise(r=>setTimeout(r,ms));
const b=await chromium.launch({executablePath:'/usr/bin/google-chrome',args:['--no-sandbox']});
const ctx=await b.newContext({viewport:{width:390,height:844},deviceScaleFactor:2,isMobile:true,hasTouch:true,locale:'vi-VN',recordVideo:{dir:O+'raw/',size:{width:390,height:844}}});
const p=await ctx.newPage();const t0=Date.now();const T=()=>((Date.now()-t0)/1000).toFixed(1)+'s';const L=s=>console.log(T(),s);const errs=[];p.on('pageerror',e=>errs.push(String(e)));
const shot=n=>p.screenshot({path:`${O}${TAG}-${n}.png`});
await p.goto(U);await p.waitForFunction(()=>window.__AURORA&&__AURORA.state().agents.length>0);await sl(600);
let id=await p.evaluate(n=>__AURORA.state().agents.find(a=>a.name===n)?.id,BOT);
if(!id){await p.tap('#newBtn');await sl(350);await p.tap('#menuBot');await sl(500);await p.tap('#agentName');await p.keyboard.type(BOT,{delay:30});await p.tap('#agentDesc');await p.keyboard.type('Trợ lý đa năng: nghiên cứu, lên kế hoạch, viết báo cáo',{delay:10});await p.tap('#createBtn');await sl(900);id=await p.evaluate(n=>__AURORA.state().agents.find(a=>a.name===n)?.id,BOT)}
const open=async()=>{await p.goto(U);await p.waitForFunction(()=>window.__AURORA);await p.evaluate(id=>location.hash=id,id);await p.reload();await p.waitForFunction(()=>window.__AURORA&&__AURORA.current());await sl(400)};
await open();await p.tap('#input');await p.keyboard.type(Q,{delay:8});await p.keyboard.press('Enter');L('sent');
const steps=()=>p.evaluate(id=>(__AURORA.state().msgs[id]||[]).filter(m=>/:step\d+$/.test(m.id)).length,id);
const scroll=()=>p.evaluate(()=>{const m=document.querySelector('#messages');m.scrollTop=m.scrollHeight});
let seen=0;for(let i=0;i<600&&seen<1;i++){await sl(500);const n=await steps();if(n>seen){seen=n;L('milestone '+n);await scroll();await sl(300);await shot('m'+n)}}
await p.goto(U);await p.waitForFunction(()=>window.__AURORA);await sl(1500);L('list subtitle: '+await p.evaluate(id=>{const r=document.querySelector(`[data-id="${id}"]`);return r?r.innerText.replace(/\n/g,' | '):'(pinned/none)'},id));await shot('home-working');L('closing app mid-task');await p.goto('about:blank');await sl(20000);L('reopening');await open();await scroll();await shot('reopen');
let done=false;for(let i=0;i<900;i++){await sl(500);const s=await p.evaluate(id=>__AURORA.state().agents.find(a=>a.id===id).status,id);const n=await steps();if(n>seen){seen=n;L('milestone '+n);await scroll();await sl(300);await shot('m'+n)}if(s!=='working'){done=true;L('done status='+s);break}}
await sl(1000);await scroll();await sl(400);await shot('final');
const ms=await p.evaluate(id=>__AURORA.state().msgs[id],id);const last=ms.at(-1);
console.log('=== MESSAGES ===');for(const m of ms.slice(-8))console.log('['+m.id.split(':').pop()+']',m.text.slice(0,600).replace(/\n+/g,' ⏎ '));
console.log('SOURCES',(last.sources||[]).length,'unique',new Set((last.sources||[]).map(x=>x.url)).size,'errs',errs.length);
const v=p.video();await ctx.close();console.log('VIDEO',await v.path());await b.close();
