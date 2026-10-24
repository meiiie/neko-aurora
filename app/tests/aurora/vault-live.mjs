// Vault live check (fake data only: 4242 test card, fake login). Mobile driver,
// screen recording + screenshots: Vault page, save-card form card, bot listing
// the vault without values, approval + fill. env: U, EMAIL, O (dir/), DEL=1.
import {chromium} from 'playwright-core';import fs from 'fs';import {execFileSync} from 'child_process';
const U=process.env.U||'http://127.0.0.1:18791';if(U.includes(':8765'))throw 0;const EMAIL=process.env.EMAIL||'t@example.com',O=process.env.O||'/tmp/vlt/out/';fs.mkdirSync(O,{recursive:true});
const sl=ms=>new Promise(r=>setTimeout(r,ms));const T0=Date.now();const log=[];const L=s=>{log.push(((Date.now()-T0)/1000).toFixed(1)+' '+s);console.log(log.at(-1))};
const b=await chromium.launch({executablePath:'/usr/bin/google-chrome',args:['--no-sandbox']});
const ctx=await b.newContext({viewport:{width:390,height:844},isMobile:true,locale:'vi-VN',hasTouch:true,deviceScaleFactor:2});const p=await ctx.newPage();
p.on('pageerror',e=>L('pageerror '+e.stack));
const shot=async n=>{await sl(400);await p.screenshot({path:O+n+'.png'});L('shot '+n)};
await p.goto(U+'/app/');await p.waitForURL(/signin/);await p.fill('#email',EMAIL);const rp=p.waitForResponse(r=>r.url().endsWith('/app/auth/email'));await p.tap('#sendBtn');const s=await (await rp).json();
await p.waitForSelector('#codeStep:not(.hidden)');await p.fill('#code',s.devCode);await p.waitForURL(/welcome/,{waitUntil:'commit'});await p.fill('#name','Darren');await sl(800);await p.tap('[data-goal=work]');await sl(500);await p.tap('#startBtn',{force:true});await sl(7000);
await p.goto(U+'/app/',{waitUntil:'commit'}).catch(()=>{});await sl(3000);
const F=O+'frames/';fs.mkdirSync(F,{recursive:true});const fr=[];const cdp=await ctx.newCDPSession(p);cdp.on('Page.screencastFrame',async f=>{const n=F+String(fr.length).padStart(5,'0')+'.jpg';fs.writeFileSync(n,Buffer.from(f.data,'base64'));fr.push([n,f.metadata.timestamp]);cdp.send('Page.screencastFrameAck',{sessionId:f.sessionId}).catch(()=>{})});await cdp.send('Page.startScreencast',{format:'jpeg',quality:80,everyNthFrame:1});
const api=(path,body)=>p.evaluate(async([path,body])=>{const r=await fetch('/app/api'+path,body?{method:'POST',headers:{'content-type':'application/json'},body:JSON.stringify(body)}:{});return r.json()},[path,body]);
const idle=async(max=180e3)=>{const t0=Date.now();await sl(4000);for(;;){const st=await api('/state');const busy=st.agents.some(a=>a.status==='working');if(!busy&&Date.now()-t0>6000||Date.now()-t0>max)return;await sl(1500)}};
let out={};
try{
 // 1) Settings → Vault: add a fake login from the page
 await p.tap('#meBtn');await sl(1200);await p.evaluate(()=>document.querySelector('#openVault')?.scrollIntoView({block:'center'}));await sl(400);await shot('01-settings-vault-cell');
 await p.tap('#openVault');await sl(1500);await shot('02-vault-empty');
 await p.tap('[data-vadd=login]');await sl(600);await p.fill('#vaultLabel','Shop thử');await p.fill('#vaultBody input[data-secret=site]','shop.example');await p.fill('#vaultBody input[data-secret=username]','tester@example.com');await p.fill('#vaultBody input[data-secret=password]','fake-Pass-2026');await shot('03-vault-add-login');
 await p.tap('#vaultSave');await sl(1500);await p.tap('#vaultBody .vault-row');await sl(600);await shot('04-vault-item-masked');
 await p.tap('#vaultBody [data-vreveal]');await sl(1200);await shot('05-vault-revealed');await p.tap('#vaultBody [data-vreveal]');await sl(400);
 await p.tap('#closeVault');await sl(800);
 // 2) chat: bot raises the save-card form card
 await p.goto(U+'/app/',{waitUntil:'commit'}).catch(()=>{});await sl(2500);await (await p.$('#rows .row[data-id] .rowin')).tap();await sl(2000);
 const send=async t=>{await p.tap('#input');await p.fill('#input',t);await sl(200);await p.tap('#composer .send');L('SENT '+t)};
 await send('Mình muốn lưu thẻ thanh toán vào Vault để bạn dùng khi cần. Gửi mình thẻ để nhập nhé.');await idle();
 const card=await p.waitForSelector('.vault-card input[data-secret=number]',{timeout:20000}).catch(()=>null);out.formCard=!!card;await shot('06-chat-save-card-form');
 if(card){await p.fill('.vault-card input[data-secret=number]','4242424242424242');await p.fill('.vault-card input[data-secret=name]','Test Owner');await p.fill('.vault-card input[data-secret=exp]','1234');await p.fill('.vault-card input[data-secret=cvc]','123');await p.fill('.vault-card #vaultLabel','Thẻ thử 4242');await shot('07-chat-card-filled');await p.tap('.vault-card #vaultSave');await sl(1500);await shot('08-chat-card-saved');await idle()}
 // 3) bot lists the vault (metadata only)
 await send('Liệt kê các mục trong Vault của mình.');await idle();await shot('09-bot-lists-vault');
 // 4) use the card on the demo checkout: approval, then server-side fill
 await send(`Dùng thẻ thử 4242 trong Vault điền form thanh toán thử ở ${U}/app/vault-demo.html giúp mình.`);await idle();
 const ap=await p.waitForSelector('.approval.ap-vault .btns .go',{timeout:30000}).catch(()=>null);out.approval=!!ap;await p.evaluate(()=>{const m=document.getElementById('messages');m.scrollTop=m.scrollHeight});await shot('10-approval-vault');
 if(ap){await ap.tap();L('APPROVED');await sl(2000);await shot('11-approved-filling');for(let i=0;i<60;i++){const t=await p.evaluate(()=>document.querySelector('.approval.ap-vault .ap-line')?.textContent||'');if(t&&!t.includes('đang dùng'))break;await sl(1500)}await idle();await p.evaluate(()=>{const m=document.getElementById('messages');m.scrollTop=m.scrollHeight});await shot('12-fill-result')}
 // 5) Vault page: use history
 await p.tap('#backBtn').catch(()=>{});await sl(800);await p.goto(U+'/app/',{waitUntil:'commit'}).catch(()=>{});await sl(2500);await p.tap('#meBtn');await sl(1000);await p.tap('#openVault');await sl(1500);await p.evaluate(()=>{const d=document.querySelector('.vault-log');if(d){d.open=true;d.scrollIntoView()}});await sl(500);await shot('13-vault-list-history');
 const st=await api('/state');const msgs=[];for(const a of st.agents){const m=await api('/agents/'+a.id+'/messages');const l=(m.messages||m||[]);if(l.length)msgs.push(a.name+': '+JSON.stringify(l.map(x=>({r:x.role,t:(x.text||'').slice(0,400),ap:x.approval&&{k:x.approval.kind,t:x.approval.title,b:x.approval.body,st:x.approval.state,o:x.approval.outcome},sec:x.secret}))))}
 out.thread=msgs.join('\n');out.vault=await api('/vault');
}catch(e){L('ERR '+e.stack)}
await cdp.send('Page.stopScreencast').catch(()=>{});
let lst='';fr.forEach(([n,t],i)=>{const nx=fr[i+1]?fr[i+1][1]:t+1;lst+=`file '${n}'\nduration ${Math.max(0.01,Math.min(3,nx-t)).toFixed(3)}\n`});if(fr.length){lst+=`file '${fr.at(-1)[0]}'\n`;fs.writeFileSync(O+'frames.txt',lst);execFileSync('ffmpeg',['-y','-loglevel','error','-f','concat','-safe','0','-i',O+'frames.txt','-vf','scale=trunc(iw/2)*2:trunc(ih/2)*2,fps=30','-pix_fmt','yuv420p','-c:v','libx264','-crf','26',O+'recording.mp4'])}fs.rmSync(F,{recursive:true,force:true});fs.rmSync(O+'frames.txt',{force:true});
const raw=JSON.stringify(out);out.leak=['4242424242424242','4242 4242 4242 4242','fake-Pass-2026'].filter(x=>(out.thread||'').includes(x));
fs.writeFileSync(O+'log.txt',log.join('\n')+'\n---\n'+JSON.stringify(out,null,1));
if(process.env.DEL==='1'){await p.evaluate(async()=>{await fetch('/app/api/account/delete',{method:'POST',headers:{'content-type':'application/json'},body:JSON.stringify({confirm:'DELETE'})})});L('deleted')}
await ctx.close();await b.close();console.log('leak',out.leak,'form',out.formCard,'approval',out.approval)
