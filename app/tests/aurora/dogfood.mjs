import {notLive} from './guard.mjs';
// Dogfood: real user flow at 390 on the live server, video + per-stage screenshots.
import {chromium} from 'playwright-core';
const U=process.env.AURORA_URL||'http://127.0.0.1:8798/app/',O=process.env.OUT||'/workspace/neko-bot-shots/aurora/dogfood/',TAG=process.env.TAG||'run1';
notLive(U);
const BOT=process.env.BOT||'Phân tích vàng',Q=process.env.Q||'Phân tích giá vàng hôm nay (thế giới XAU/USD và SJC Việt Nam), xu hướng tuần qua và nhận định ngắn hạn';
const b=await chromium.launch({executablePath:'/usr/bin/google-chrome',args:['--no-sandbox']});
const ctx=await b.newContext({viewport:{width:390,height:844},deviceScaleFactor:2,isMobile:true,hasTouch:true,locale:'vi-VN',recordVideo:{dir:O+'raw/',size:{width:390,height:844}}});
const p=await ctx.newPage();const t0=Date.now();const T=()=>((Date.now()-t0)/1000).toFixed(1)+'s';const log=[];const L=s=>{log.push(T()+' '+s);console.log(T(),s)};
const shot=async n=>p.screenshot({path:`${O}${TAG}-${n}.png`});
p.on('pageerror',e=>L('PAGEERROR '+e));
await p.goto(U);await p.waitForFunction('window.__AURORA&&__AURORA.state().agents.length>0');await p.waitForTimeout(600);await shot('0-list');
let id=await p.evaluate(n=>__AURORA.state().agents.find(a=>a.name===n)?.id,BOT);
if(!id){await p.tap('#newBtn');await p.waitForTimeout(350);await p.tap('#menuBot');await p.waitForTimeout(500);await p.tap('#agentName');await p.keyboard.type(BOT,{delay:30});
 await p.tap('#agentDesc');await p.keyboard.type(process.env.DESC||'Phân tích thị trường vàng, dẫn nguồn số liệu',{delay:15});await shot('1-create');await p.tap('#createBtn');await p.waitForTimeout(900);
 id=await p.evaluate(n=>__AURORA.state().agents.find(a=>a.name===n)?.id,BOT);L('created bot')}
await p.evaluate(id=>{const r=[...document.querySelectorAll('.row')].find(e=>e.dataset.id===id);if(r)r.scrollIntoView({block:'center'})},id);
await p.evaluate(id=>location.hash=id,id);await p.reload();await p.waitForFunction('window.__AURORA&&__AURORA.current()');await p.waitForTimeout(500);
const n0=(await p.evaluate(id=>(__AURORA.state().msgs[id]||[]).length,id));
await p.tap('#input');await p.keyboard.type(Q,{delay:12});await shot('2-typed');await p.keyboard.press('Enter');L('sent');await p.waitForTimeout(400);await shot('3-sending');
let firstDelta=null,plan=false,stream=false,lastShot=0,steps='';
for(let i=0;i<2400;i++){await p.waitForTimeout(250);
 const s=await p.evaluate(id=>{const a=__AURORA.state().agents.find(x=>x.id===id);return{st:a.status,draft:!!document.querySelector('.msg.draft'),plan:[...document.querySelectorAll('.plan .step')].map(e=>e.className.replace('step ','')).join(','),tool:(document.querySelector('#messages .status')?.textContent||''),ack:!!document.querySelector('#messages [data-id$=":ack"], #messages .msg.bot'),ap:!!document.querySelector('.approval:not(.done)')}},id);
 if(s.draft&&!firstDelta){firstDelta=T();L('first delta')}
 if(s.draft&&!stream){stream=true;await shot('4-streaming')}
 if(s.plan&&s.plan!==steps){steps=s.plan;L('plan '+steps);if(!plan){plan=true;await shot('5-plan')}else if(Date.now()-lastShot>8000){lastShot=Date.now();await shot('5-plan-'+i)}}
 if(s.tool&&s.tool!==globalThis.lastTool){globalThis.lastTool=s.tool;L('status '+s.tool);await shot('3b-status-'+(s.tool.includes('tra')?'tool':'compose'))}
 if(s.ap){L('approval');await shot('6-approval')}
 if(i>8&&s.st!=='working'){L('done status='+s.st);break}}
await p.waitForTimeout(800);
const ms=await p.evaluate(id=>__AURORA.state().msgs[id],id);const fin=ms.slice(n0).filter(m=>m.role!=='user');
await p.evaluate(()=>{const m=document.querySelector('#messages');m.scrollTop=m.scrollHeight});await shot('7-final');
// tall screenshot of whole answer
const sum=p.locator('#messages .sources summary').last();if(await sum.count()){await sum.scrollIntoViewIfNeeded();await sum.tap();await p.waitForTimeout(400);await p.evaluate(()=>{const m=document.querySelector('#messages');m.scrollTop=m.scrollHeight});await shot('8-sources');L('sources opened')}
const cite=await p.locator('#messages .cite').count();L('citations rendered '+cite);
console.log('=== ANSWER ===');for(const m of fin)console.log('['+m.role+']',m.text,m.plan?JSON.stringify(m.plan.steps.map(s=>s.title+':'+s.status)):'',m.tools?JSON.stringify(m.tools):'',m.sources?JSON.stringify(m.sources.map(x=>x.url)):'');
console.log('=== LOG ===\n'+log.join('\n'));
const v=p.video();await ctx.close();const path=await v.path();console.log('VIDEO',path);await b.close();
