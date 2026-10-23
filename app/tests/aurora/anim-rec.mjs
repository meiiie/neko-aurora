// Live animation check: mobile screen recording, one task then 3 rapid follow-ups; logs status line / drafts / composer state every 150ms.
// env: U, EMAIL, O (dir ending /), WAIT ms, DEL=1 deletes the account. Never points at :8765.
import {chromium} from 'playwright-core';import fs from 'fs';import {execFileSync} from 'child_process';
const U=process.env.U||'http://127.0.0.1:18791';if(U.includes(':8765'))throw 0;const _u=U,EMAIL=process.env.EMAIL||'t@example.com',O=process.env.O||'/tmp/anim/out/';fs.mkdirSync(O,{recursive:true});
const sl=ms=>new Promise(r=>setTimeout(r,ms));
const b=await chromium.launch({executablePath:'/usr/bin/google-chrome',args:['--no-sandbox']});
const ctx=await b.newContext({viewport:{width:390,height:844},isMobile:true,locale:'vi-VN',hasTouch:true,deviceScaleFactor:2});const p=await ctx.newPage();
const log=[];const T0=Date.now();const L=s=>{log.push(((Date.now()-T0)/1000).toFixed(1)+' '+s);};
p.on('pageerror',e=>L('pageerror '+e));
await p.goto(U+'/app/');await p.waitForURL(/signin/);await p.fill('#email',EMAIL);const rp=p.waitForResponse(r=>r.url().endsWith('/app/auth/email'));await p.tap('#sendBtn');const s=await (await rp).json();
await p.waitForSelector('#codeStep:not(.hidden)');await p.fill('#code',s.devCode);await p.waitForURL(/welcome/,{waitUntil:'commit'});await p.fill('#name','Darren');await sl(800);await p.tap('[data-goal=work]');await sl(500);await p.tap('#startBtn',{force:true});await sl(7000);
await p.goto(U+'/app/',{waitUntil:'commit'}).catch(()=>{});await sl(3000);const r=await p.$('#rows .row[data-id] .rowin');await r.tap();await sl(2500);
const F=O+'frames/';fs.mkdirSync(F,{recursive:true});const fr=[];const cdp=await ctx.newCDPSession(p);cdp.on('Page.screencastFrame',async f=>{const n=F+String(fr.length).padStart(5,'0')+'.jpg';fs.writeFileSync(n,Buffer.from(f.data,'base64'));fr.push([n,f.metadata.timestamp]);cdp.send('Page.screencastFrameAck',{sessionId:f.sessionId}).catch(()=>{})});await cdp.send('Page.startScreencast',{format:'jpeg',quality:80,everyNthFrame:1});
const snap=async()=>p.evaluate(()=>{const q=s=>document.querySelector(s);const st=q('#messages .status');return{status:st?st.textContent:null,drafts:document.querySelectorAll('#messages .draft').length,mine:document.querySelectorAll('#messages .msg.mine').length,bots:document.querySelectorAll('#messages .msg.bot').length,send:q('#composer .send')?.getAttribute('aria-label'),dis:q('#input').disabled}});
let stop=false;(async()=>{let prev='';while(!stop){const x=JSON.stringify(await snap().catch(()=>({})));if(x!==prev){L(x);prev=x}await sl(150)}})();
const send=async t=>{await p.tap('#input');await p.fill('#input',t);await sl(150);await p.tap('#composer .send');L('SENT '+t)};
await send(process.env.M1||'Tìm giúp mình 3 tin tức công nghệ mới nhất hôm nay, mỗi tin một dòng có nguồn.');
await sl(2500);await send('À, chỉ lấy tin về AI thôi nhé.');await sl(1200);await send('Thêm một câu tóm tắt cuối cùng nữa.');await sl(1000);await send('Viết bằng tiếng Việt.');
await sl(+process.env.WAIT||90000);stop=true;await sl(300);
await cdp.send('Page.stopScreencast').catch(()=>{});await p.screenshot({path:O+'end.png'});
let lst='';fr.forEach(([n,t],i)=>{const nx=fr[i+1]?fr[i+1][1]:t+1;lst+=`file '${n}'\nduration ${Math.max(0.01,nx-t).toFixed(3)}\n`});lst+=`file '${fr.at(-1)[0]}'\n`;fs.writeFileSync(O+'frames.txt',lst);execFileSync('ffmpeg',['-y','-loglevel','error','-f','concat','-safe','0','-i',O+'frames.txt','-vf','scale=trunc(iw/2)*2:trunc(ih/2)*2,fps=30','-pix_fmt','yuv420p','-c:v','libx264','-crf','26',O+'recording.mp4']);fs.rmSync(F,{recursive:true});fs.rmSync(O+'frames.txt');
const msgs=await p.evaluate(async()=>{const st=await (await fetch('/app/api/state')).json();const out=[];for(const a of st.agents){const m=await (await fetch('/app/api/agents/'+a.id+'/messages')).json();out.push(a.name+': '+JSON.stringify((m.messages||m||[]).map?.(x=>({r:x.role,t:(x.text||'').slice(0,160),q:x.queued}))||m).slice(0,2500))}return document.querySelector('#messages').innerText.slice(-1500)+'\n===\n'+out.join('\n')});
fs.writeFileSync(O+'log.txt',log.join('\n')+'\n---\n'+msgs);
if(process.env.DEL){await p.evaluate(async()=>{await fetch('/app/api/account/delete',{method:'POST',headers:{'content-type':'application/json'},body:JSON.stringify({confirm:'DELETE'})})})}
await ctx.close();await b.close();console.log('ok')
