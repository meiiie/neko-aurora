// Live check of the DS Bot-inspired features (no sandbox commands, low cost).
// U=https://… EMAIL=… O=/out/dir/ node features.mjs
import {chromium} from 'playwright-core';import fs from 'node:fs';
const U=process.env.U,EMAIL=process.env.EMAIL,O=process.env.O;if(!U||U.includes(':8765'))throw 0;fs.mkdirSync(O,{recursive:true});
const sl=ms=>new Promise(r=>setTimeout(r,ms));
const b=await chromium.launch({executablePath:'/usr/bin/google-chrome',args:['--no-sandbox']});
const ctx=await b.newContext({viewport:{width:390,height:844},deviceScaleFactor:2,isMobile:true,hasTouch:true,locale:'vi-VN'});const p=await ctx.newPage();
const errs=[];p.on('pageerror',e=>errs.push(String(e)));const H={'content-type':'application/json',origin:U};
const get=async x=>{const r=await p.request.get(U+'/app/api/'+x,{timeout:60000});return r.ok()?r.json():null};
const post=async(x,d)=>{const r=await p.request.post(U+'/app/api/'+x,{headers:H,data:d});return {s:r.status(),j:await r.json().catch(()=>null)}};
const openT=async id=>{await p.goto(U+'/app/');await sl(2500);const r=await p.$(`#rows .row[data-id="${id}"] .rowin`)||await p.$(`.feat[data-id="${id}"]`);if(r)await r.tap();await sl(2500)};
const shot=n=>p.screenshot({path:O+n+'.png'});
const idle=async(id,max=240e3)=>{const t0=Date.now();await sl(5000);for(;;){const st=await get('state');const a=st.agents.find(x=>x.id===id);const busy=a.status==='working'||(a.members||[]).some(m=>st.agents.find(x=>x.id===m)?.status==='working');if(!busy&&Date.now()-t0>12000||Date.now()-t0>max)return;await sl(4000)}};
let out={};
try{
 await p.goto(U+'/app/');await p.waitForURL(/signin/);await p.fill('#email',EMAIL);const rp=p.waitForResponse(r=>r.url().endsWith('/app/auth/email'));await p.tap('#sendBtn');const s=await (await rp).json();
 await p.waitForSelector('#codeStep:not(.hidden)');await p.fill('#code',s.devCode);await p.waitForURL(/welcome/);await p.fill('#name','Darren');await p.tap('[data-goal=work]');await p.tap('#startBtn');
 await p.waitForURL(/\/app\/(#bot-.*)?$/,{timeout:30000});for(let i=0;i<30&&!((await get('state'))?.agents||[]).some(a=>a.name==='Aurora');i++)await sl(2000);
 let st=await get('state');const aur=st.agents.find(a=>a.name==='Aurora').id;const ana=st.agents.find(a=>a.name==='Phân tích')?.id;await idle(aur);
 // 1) memory review + team scope
 await post(`agents/${aur}/messages`,{text:'Từ giờ mọi bot gọi mình là anh Darren và trả lời thật ngắn nhé. Thủ đô của Pháp là gì?',clientNonce:'f1-'+Date.now()});await idle(aur);await sl(8000);
 out.memory1=await get(`agents/${aur}/memory`);
 // 2) owner adds + edits memory through the API the panel uses
 await post(`agents/${aur}/memory`,{text:'Darren thích bảng so sánh ngắn'});out.memEdit=(await post(`agents/${aur}/memory/0`,{text:'Darren thích bảng so sánh ngắn, có nguồn'})).s;
 // UI: profile → memory panel
 await openT(aur);await p.tap('#whoBtn');await sl(2500);out.prof=await p.evaluate(()=>({n:document.querySelector('#profBody').innerHTML.length,cells:[...document.querySelectorAll('#profBody .cell span')].map(x=>x.textContent).slice(0,12)}));await shot('01-profile-memory-presets');
 await p.evaluate(()=>document.querySelector('#presetBox')?.scrollIntoView({block:'center'}));await sl(500);await shot('02-presets');
 await p.tap('#prMem');await sl(1800);await shot('03-memory-panel');
 const it=await p.$('#memBody .mem-it');if(it){await it.tap();await sl(500);await shot('04-memory-edit');await p.tap('#memBody .mem-bar .pill.soft');await sl(1200)}
 await p.evaluate(()=>document.querySelector('.mem-log')?.setAttribute('open',''));await p.evaluate(()=>document.querySelector('.mem-log')?.scrollIntoView());await sl(400);await shot('05-memory-log');
 await p.tap('#closeMem');await sl(600);
 // 3) presets via UI: tap Chỉ đọc then back to Máy riêng
 await p.tap('#presetBox [data-preset=readonly]');await sl(1500);out.presetRO=(await get(`agents/${aur}/computer`)).policy;await shot('06-preset-readonly');
 await p.tap('#presetBox [data-preset=workspace]');await sl(1500);out.presetWS=(await get(`agents/${aur}/computer`)).policy;
 // 4) team memory reaches another bot
 if(ana){await post(`agents/${ana}/messages`,{text:'Chào bạn, bạn gọi mình là gì?',clientNonce:'f2-'+Date.now()});await idle(ana);const ms=await get(`agents/${ana}/messages`);out.anaReply=((ms.messages||ms).filter(m=>m.role==='agent').at(-1)||{}).text}
 // 5) group routing + handoff
 const g=(await post('groups',{name:'Đội việc',bots:[aur,ana]})).j;out.group=g&&g.id;
 await post(`agents/${g.id}/messages`,{text:'Mình có bảng chi tiêu tuần: ăn 700k, xăng 300k, cafe 250k. Ai phân tích giúp tỷ lệ từng khoản? Aurora giao cho người hợp nhất.',clientNonce:'f3-'+Date.now()});
 await idle(g.id,300e3);await sl(6000);await idle(g.id,200e3);
 const gm=await get(`agents/${g.id}/messages`);out.groupMsgs=(gm.messages||gm).map(m=>({role:m.role,from:m.from,text:(m.text||'').slice(0,220)}));
 await openT(g.id);await shot('07-group-handoff');await p.evaluate(()=>{const m=document.getElementById('messages');m.scrollTop=0});await sl(500);await shot('08-group-top');
 // 6) search
 await p.goto(U+'/app/');await sl(2500);await p.fill('#search','thu do');await sl(2500);await shot('09-search');out.search=(await get('search?q=thu%20do')).hits.length;
 // 7) usage page
 await p.fill('#search','');await p.tap('#meBtn');await sl(1200);await p.tap('#openUsage');await sl(2000);await shot('10-usage');out.usage=await get('usage?range=1');
}catch(e){out.error=String(e);await shot('zz-error').catch(()=>{})}
finally{out.errs=errs;out.deleted=(await post('account/delete',{confirm:'DELETE'})).s;console.log(JSON.stringify(out,null,1));fs.writeFileSync(O+'result.json',JSON.stringify(out,null,1));await b.close()}
