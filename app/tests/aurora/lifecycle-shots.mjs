// Production UX audit tour at 390: hosted sign-in → onboarding → list → thread → sheets → group → offline, light + dark.
// env: U (hosted server), EMAIL, O (dir ending /), THEME=dark, DEL=1. Never :8765.
import {chromium} from 'playwright-core';import fs from 'fs';
const U=process.env.U||'http://127.0.0.1:18791',EMAIL=process.env.EMAIL||'t@example.com',O=process.env.O||'/tmp/lc/';if(U.includes(':8765'))throw 0;fs.mkdirSync(O,{recursive:true});
const sl=ms=>new Promise(r=>setTimeout(r,ms));
const b=await chromium.launch({executablePath:'/usr/bin/google-chrome',args:['--no-sandbox','--hide-scrollbars']});
const ctx=await b.newContext({viewport:{width:390,height:844},deviceScaleFactor:2,isMobile:true,hasTouch:true,locale:'vi-VN',colorScheme:process.env.THEME==='dark'?'dark':'light'});
const p=await ctx.newPage();const errs=[];p.on('pageerror',e=>errs.push(String(e)));p.on('console',m=>{if(m.type()==='error')errs.push('console '+m.text().slice(0,160))});
let n=0;const shot=async name=>{await sl(450);await p.screenshot({path:`${O}${String(++n).padStart(2,'0')}-${name}.png`})};
const tap=async s=>{await p.tap(s,{timeout:8000}).catch(e=>errs.push('tap '+s))};
await p.goto(U+'/app/');await p.waitForURL(/signin/);await shot('signin');
await p.fill('#email','not-an-email');await tap('#sendBtn');await shot('signin-invalid');
await p.fill('#email',EMAIL);const rp=p.waitForResponse(r=>r.url().endsWith('/app/auth/email'));await tap('#sendBtn');const s=await (await rp).json();
await p.waitForSelector('#codeStep:not(.hidden)');await shot('code');await p.fill('#code','000000');await p.keyboard.press('Enter').catch(()=>{});await sl(800);await shot('code-wrong');
await p.fill('#code',s.devCode);await p.waitForURL(/welcome/,{waitUntil:'commit'});await sl(800);await shot('welcome');
await p.fill('#name','Darren');await sl(400);await tap('[data-goal=work]');await shot('welcome-filled');await tap('#startBtn');await sl(1500);await shot('setup-progress');await sl(7000);
await p.goto(U+'/app/',{waitUntil:'commit'}).catch(()=>{});await sl(3500);await shot('list');
await tap('#newBtn');await shot('new-menu');await tap('#menuBot');await shot('new-bot');await p.keyboard.press('Escape');await sl(300);await tap('#closeSetup').catch(()=>{});
await p.goto(U+'/app/',{waitUntil:'commit'}).catch(()=>{});await sl(2500);
const rows=await p.$$('#rows .row[data-id] .rowin');if(rows[0])await rows[0].tap();await sl(2500);await shot('thread-aurora');
await tap('#plusBtn');await shot('plus-menu');await p.mouse.click(200,300);await sl(300);
await p.tap('#input');await sl(300);await shot('keyboard-focus');
await tap('#threadMore');await shot('thread-menu');await p.mouse.click(200,400);await sl(300);
await tap('#whoBtn');await sl(800);await shot('profile');await tap('#profBack');await sl(500);
await tap('#backBtn');await sl(700);
await tap('#meBtn');await sl(900);await shot('settings');
const sb=await p.$('#settingsBody');if(sb){await sb.evaluate(e=>e.scrollTop=e.scrollHeight);await shot('settings-bottom')}
await p.getByText('Mức dùng AI').first().tap().catch(()=>errs.push('usage'));await sl(1200);await shot('usage');await tap('#closeUsage');await sl(500);await tap('#closeSettings').catch(()=>{});await sl(500);
{const rr=await p.$$('#rows .row[data-id] .rowin');if(rr[1]){await rr[1].tap();await sl(1500);await tap('#whoBtn');await sl(800);await p.getByText('Bộ nhớ').first().tap().catch(()=>errs.push('mem'));await sl(1000);await shot('memory');await tap('#closeMem');await sl(400);await tap('#profBack');await sl(400);await tap('#backBtn');await sl(600)}}
await tap('#search');await p.fill('#search','vàng').catch(()=>{});await sl(900);await shot('search');await p.fill('#search','').catch(()=>{});
await tap('#newBtn');await tap('#menuGroup');await sl(700);await shot('group-new');{for(const nm of ['Phân tích','Thư ký']){await p.locator('#groupList').getByText(nm,{exact:true}).first().tap().catch(()=>errs.push('pick '+nm));await sl(350)}await sl(300);await shot('group-picked');await tap('#groupNext');await sl(1200);await shot('group-after-next');await p.fill('#groupName','Nhóm thử').catch(()=>{});await tap('#groupNext');await sl(2000);await shot('group-thread');if(await p.$('#thread:not(.hidden) #input')){await p.fill('#input','Hai bạn chào mình một câu ngắn nhé');await tap('#composer .send');await sl(25000);await shot('group-reply')}await p.keyboard.press('Escape').catch(()=>{})}await tap('#closeGroup').catch(()=>{});await sl(400);
await ctx.setOffline(true);await sl(1500);await shot('offline');await ctx.setOffline(false);await sl(3000);await shot('reconnected');
await p.addStyleTag({content:'html{font-size:130%!important}'}).catch(()=>{});
fs.writeFileSync(O+'errs.json',JSON.stringify(errs,null,1));
if(process.env.DEL)await p.evaluate(()=>fetch('/app/api/account/delete',{method:'POST',headers:{'content-type':'application/json'},body:JSON.stringify({confirm:'DELETE'})}));
await b.close();console.log('shots',n,'errs',errs.length)
