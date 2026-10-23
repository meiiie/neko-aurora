// Live: "Hai bạn ..." in a group → every member replies. U EMAIL O [DEL=1]
import {chromium} from 'playwright-core';import fs from 'node:fs';
const U=process.env.U,EMAIL=process.env.EMAIL,O=process.env.O;if(!U||U.includes(':8765'))throw 0;fs.mkdirSync(O,{recursive:true});
const sl=ms=>new Promise(r=>setTimeout(r,ms));
const b=await chromium.launch({executablePath:'/usr/bin/google-chrome',args:['--no-sandbox']});
const ctx=await b.newContext({viewport:{width:390,height:844},deviceScaleFactor:2,isMobile:true,hasTouch:true,locale:'vi-VN'});const p=await ctx.newPage();
const H={'content-type':'application/json',origin:U};
const get=async x=>{const r=await p.request.get(U+'/app/api/'+x);return r.ok()?r.json():null};
const post=async(x,d)=>{const r=await p.request.post(U+'/app/api/'+x,{headers:H,data:d});return r.json().catch(()=>null)};
await p.goto(U+'/app/');await p.waitForURL(/signin/);await p.fill('#email',EMAIL);const rp=p.waitForResponse(r=>r.url().endsWith('/app/auth/email'));await p.tap('#sendBtn');const s=await (await rp).json();
await p.waitForSelector('#codeStep:not(.hidden)');await p.fill('#code',s.devCode);await sl(4000);
if(p.url().includes('welcome')){await p.fill('#name','Darren');await p.tap('[data-goal=work]');await p.tap('#startBtn')}
for(let i=0;i<30&&!((await get('state'))?.agents||[]).some(a=>a.name==='Phân tích');i++)await sl(2000);
const st=await get('state');const aur=st.agents.find(a=>a.name==='Aurora').id,ana=st.agents.find(a=>a.name==='Phân tích').id;
const g=await post('groups',{name:'Hai bạn',bots:[aur,ana]});
await post(`agents/${g.id}/messages`,{text:'Hai bạn chào mình một câu ngắn nhé.',clientNonce:'g-'+Date.now()});
let ms=[];for(let i=0;i<40;i++){await sl(4000);const r=await get(`agents/${g.id}/messages`);ms=(r.messages||r).filter(m=>m.role==='agent');if(new Set(ms.map(m=>m.from)).size>=2)break}
console.log(JSON.stringify(ms.map(m=>[m.from,(m.text||'').slice(0,80)])));
await p.goto(U+'/app/');await sl(2500);await (await p.$(`#rows .row[data-id="${g.id}"] .rowin`))?.tap();await sl(3000);await p.screenshot({path:O+'group-both-reply.png'});
if(process.env.DEL==='1'){const r=await p.evaluate(async()=>(await fetch('/app/api/account/delete',{method:'POST',headers:{'content-type':'application/json'},body:JSON.stringify({confirm:'DELETE'})})).status);console.log('del',r)}
await b.close();
