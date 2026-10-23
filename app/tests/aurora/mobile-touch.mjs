// Real-touch mobile test for /app/ against a running neko-aurora.
// npm i puppeteer-core ; AURORA_URL=http://127.0.0.1:8765/app/ OUT=./aurora-shots/ node mobile-touch.mjs
import puppeteer from 'puppeteer-core';
const U=process.env.AURORA_URL||'http://127.0.0.1:8765/app/',O=process.env.OUT||'./aurora-shots/';
const REAL=!!process.env.REAL_AI;
const SMTP=process.env.FAKE_SMTP||''; // e.g. http://127.0.0.1:8926 (fake-smtp.mjs); enables the Gmail delivery flow // stricter timeouts off for a real model
const sleep=ms=>new Promise(r=>setTimeout(r,ms));
const b=await puppeteer.launch({executablePath:process.env.CHROME||'/usr/bin/google-chrome',args:['--no-sandbox','--hide-scrollbars']});
const results=[];let fail=0;
const ok=(w,name,cond,extra='')=>{results.push(`${cond?'PASS':'FAIL'} ${w} ${name} ${extra}`);if(!cond)fail++};
for(const w of (process.env.WIDTHS||'360,390,430').split(',').map(Number)){let p;try{
 const ctx=await b.createBrowserContext();await ctx.overridePermissions(new URL(U).origin,['notifications']);p=await ctx.newPage();await p.evaluateOnNewDocument(()=>{try{if(!localStorage.getItem('neko-aurora-lang'))localStorage.setItem('neko-aurora-lang','vi')}catch{}});const errs=[];p.on('pageerror',e=>errs.push(String(e)));
 await p.setViewport({width:w,height:Math.round(w*2.165),deviceScaleFactor:2,isMobile:true,hasTouch:true});
 await p.goto(U,{waitUntil:'domcontentloaded'});await p.waitForFunction('window.__AURORA&&__AURORA.state().agents.length>0',{timeout:10000});await sleep(300);
 const T=p.touchscreen;
 const swipe=async(x1,y1,x2,y2,steps=6,dt=8)=>{await T.touchStart(x1,y1);for(let i=1;i<=steps;i++){await T.touchMove(x1+(x2-x1)*i/steps,y1+(y2-y1)*i/steps);await sleep(dt)}await T.touchEnd()};
 const box=async s=>p.$eval(s,e=>{const b=e.getBoundingClientRect();return[b.x+b.width/2,b.y+b.height/2]});
 const tap=async s=>{const [x,y]=await box(s);await T.tap(x,y)};
 const srv=path=>p.evaluate(async q=>(await fetch('/app/api'+q,{cache:'no-store'})).json(),path);
 const overflow=()=>p.evaluate(()=>document.documentElement.scrollWidth>innerWidth||document.body.scrollWidth>innerWidth);
 const small=()=>p.evaluate(()=>[...document.querySelectorAll('button,input,textarea,[role=button]')].filter(e=>{const r=e.getBoundingClientRect();const s=getComputedStyle(e);return r.width&&r.height&&s.visibility!=='hidden'&&r.bottom>0&&r.top<innerHeight&&r.right>0&&r.left<innerWidth&&(r.width<43.5||r.height<43.5)}).map(e=>(e.id||e.className||e.tagName)+':'+e.getBoundingClientRect().width.toFixed(1)+'x'+e.getBoundingClientRect().height.toFixed(1)));
 const rowOf=name=>p.evaluate(n=>{const r=[...document.querySelectorAll('.row')].find(e=>e.querySelector('.rowh b').textContent===n);const f=[...document.querySelectorAll('.feat')].find(e=>e.getAttribute('aria-label')===n);if(!r&&!f)return null;(r||f).scrollIntoView({block:'center'});const b=(r?r.querySelector('.rowin'):f).getBoundingClientRect();return[b.x+b.width/2,b.y+b.height/2]},name);
 ok(w,'loaded from server',(await srv('/state')).agents.length>0);
 ok(w,'no-h-overflow list',!(await overflow()));let sm=await small();ok(w,'touch>=44 list',!sm.length,sm.join(','));
 await p.screenshot({path:`${O}${w}-1-list.png`});
 // settings sheet: notifications toggle (push or in-app fallback), haptics toggle, connectors
 await tap('#meBtn');await sleep(500);ok(w,'avatar opens settings sheet',await p.$eval('#settings',e=>!e.classList.contains('hidden')&&!e.classList.contains('closed')));
 ok(w,'no-h-overflow settings',!(await overflow()));sm=await small();ok(w,'touch>=44 settings',!sm.length,sm.join(','));
 await p.screenshot({path:`${O}${w}-2a-settings.png`});
 await tap('#notifyCell .switch');await sleep(1500);await p.$eval('#notifyCell',e=>e.scrollIntoView({block:'center'}));const toastTxt=await p.$eval('#toast',e=>e.textContent);
 ok(w,'notifications toggle enables (push or fallback)',await p.$eval('#notifyCell .switch',e=>e.getAttribute('aria-checked')==='true'),toastTxt);
 const h0=await p.$eval('#hapticsCell .switch',e=>e.getAttribute('aria-checked'));await tap('#hapticsCell .switch');await sleep(100);const h1=await p.$eval('#hapticsCell .switch',e=>e.getAttribute('aria-checked'));await tap('#hapticsCell .switch');await sleep(100);
 ok(w,'haptics toggle flips and persists',h0!==h1&&await p.evaluate(()=>localStorage.getItem('neko-aurora-haptics'))==='true');
 await p.$eval('#openConnectors',e=>e.scrollIntoView({block:'center'}));await sleep(80);await tap('#openConnectors');await sleep(700);
 ok(w,'settings → connectors sheet',await p.$eval('#connectors',e=>!e.classList.contains('hidden')&&!e.classList.contains('closed'))&&await p.$eval('#settings',e=>e.classList.contains('hidden')));
 const conn=await p.$$eval('.conn',l=>l.map(x=>[x.dataset.id,x.querySelector('.conn-btn').textContent,x.querySelector('.conn-btn').disabled]));
 const REALC=['gmail','push','smtp'];
 ok(w,'connectors: Gmail+Push+SMTP real, others "Sắp có" disabled',REALC.every(id=>conn.find(c=>c[0]===id)&&conn.find(c=>c[0]===id)[1]!=='Sắp có')&&conn.filter(c=>!REALC.includes(c[0])).every(c=>c[1]==='Sắp có'&&c[2]),conn.map(c=>c[0]+':'+c[1]).join(','));
 const look2=await p.evaluate(()=>{const s=document.querySelector('#connectors').getBoundingClientRect();const h=[...document.querySelectorAll('.conn-h span')].map(e=>e.textContent);const ic=document.querySelector('.conn[data-id=gmail] .conn-ic').getBoundingClientRect();const d=document.querySelector('.conn[data-id=smtp] .pt small');const lh=parseFloat(getComputedStyle(d).lineHeight);return{top:s.top,h,ic:[ic.width,ic.height,getComputedStyle(document.querySelector('.conn-ic')).borderRadius],lines:Math.round(d.getBoundingClientRect().height/lh),pill:document.querySelector('#connCount').textContent,brand:!!document.querySelector('.conn[data-id=gmail] .conn-ic path'),close:getComputedStyle(document.querySelector('#closeConnectors')).borderRadius}});
 ok(w,'connectors sheet: full height, sections Nổi bật/Năng suất, round X, "N đã kết nối" pill',look2.top<=30&&look2.h[0]==='Nổi bật'&&look2.h[1]==='Năng suất'&&/đã kết nối$/.test(look2.pill)&&look2.close==='50%',JSON.stringify(look2));
 ok(w,'rows: rounded-square brand tile + 2-line gray description',look2.brand&&look2.ic[0]===44&&look2.ic[2]==='12px'&&look2.lines===2,JSON.stringify(look2.ic)+' lines='+look2.lines);
 sm=await small();ok(w,'touch>=44 connectors',!sm.length,sm.join(','));
 await p.screenshot({path:`${O}${w}-2b-connectors.png`});
 const nRows=(await p.$$('.conn')).length;await p.$eval('.conn-sec[data-id=chat] .conn-h',e=>e.scrollIntoView({block:'center'}));await sleep(80);await tap('.conn-sec[data-id=chat] .conn-h');await sleep(150);ok(w,'chevron header collapses a section',(await p.$$('.conn')).length<nRows&&await p.$eval('.conn-sec[data-id=chat] .conn-h',e=>e.getAttribute('aria-expanded')==='false'));await tap('.conn-sec[data-id=chat] .conn-h');await sleep(100);
 await tap('#closeConnectors');await sleep(450);
 // "+" popover → Create Bot sheet
 await tap('#newBtn');await sleep(300);ok(w,'+ opens frosted popover',await p.$eval('#newMenu',e=>!e.classList.contains('hidden')&&getComputedStyle(e).backdropFilter.includes('blur')));
 await p.screenshot({path:`${O}${w}-2c-popover.png`});
 for(let i=0;i<6;i++){try{await tap('#menuBot')}catch{}await sleep(15)}
 await sleep(400);ok(w,'Bot mới opens creator once after 6 fast taps',await p.$eval('#sheet',e=>!e.classList.contains('closed')&&!e.classList.contains('hidden'))&&await p.$eval('#newMenu',e=>e.classList.contains('hidden')));
 ok(w,'13 shapes + 12 colours, Create disabled until named',(await p.$$('.shape')).length===13&&(await p.$$('.color')).length===12&&await p.$eval('#createBtn',e=>e.disabled));
 const pick=async(sel)=>{await p.$eval(sel,e=>e.scrollIntoView({block:'center'}));await sleep(60);await tap(sel)};
 await pick('.shape:nth-child(6)');await pick('.color:nth-child(4)');
 ok(w,'shape/colour picks update preview',await p.$eval('.shape:nth-child(6)',e=>e.getAttribute('aria-pressed')==='true')&&await p.$eval('.color:nth-child(4)',e=>e.getAttribute('aria-pressed')==='true'));
 await p.$eval('#sheetBody',e=>e.scrollTop=0);await sleep(150);
 ok(w,'no-h-overflow sheet',!(await overflow()));sm=await small();ok(w,'touch>=44 sheet',!sm.length,sm.join(','));
 await p.screenshot({path:`${O}${w}-2-creator.png`});
 const [gx,gy]=await box('#grip');await swipe(gx,gy,gx,gy+260,8,10);await sleep(450);
 ok(w,'drag grip down dismisses sheet',await p.$eval('#sheet',e=>e.classList.contains('hidden')));
 const RUN=String(Date.now()%100000);const name='Tester '+w+' '+RUN;const n0=(await srv('/state')).agents.length;
 await tap('#newBtn');await sleep(250);await tap('#menuBot');await sleep(450);await pick('.shape:nth-child(6)');await pick('.color:nth-child(4)');await p.$eval('#sheetBody',e=>e.scrollTop=0);await sleep(100);await tap('#agentName');await p.keyboard.type(name);await sleep(50);
 ok(w,'Create enabled once named',await p.$eval('#createBtn',e=>!e.disabled));
 for(let i=0;i<5;i++){await tap('#createBtn');await sleep(10)}await sleep(800);
 const st1=await srv('/state');ok(w,'5 fast taps on Create → exactly 1 agent on server',st1.agents.length===n0+1&&st1.agents.filter(a=>a.name===name).length===1,`${n0}->${st1.agents.length}`);
 const mine=st1.agents.find(a=>a.name===name);ok(w,'flat face persisted (shape 5, colour 3)',mine&&mine.av&&mine.av.shape===5&&mine.av.color===3,JSON.stringify(mine&&mine.av));
 // row swipe + archive a throwaway agent
 await p.evaluate(async n=>{await fetch('/app/api/agents',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({name:n,desc:'x',av:{shape:1,color:2}})})},'Trash '+w+' '+RUN);await sleep(500);
 let rp=null;for(let i=0;i<30&&!rp;i++){rp=await rowOf('Trash '+w+' '+RUN);if(!rp)await sleep(100)}await sleep(100);rp=await rowOf('Trash '+w+' '+RUN);
 await swipe(rp[0]+100,rp[1],rp[0]-120,rp[1],6,8);await sleep(350);
 const opened=await p.evaluate(n=>!![...document.querySelectorAll('.row')].find(e=>e.querySelector('.rowh b').textContent===n)?.dataset.open,'Trash '+w+' '+RUN);
 ok(w,'row swipe reveals actions',opened);await p.screenshot({path:`${O}${w}-3-rowswipe.png`});
 const arch=await p.evaluate(n=>{const r=[...document.querySelectorAll('.row')].find(e=>e.querySelector('.rowh b').textContent===n);const b=r.querySelector('.arch').getBoundingClientRect();return[b.x+b.width/2,b.y+b.height/2]},'Trash '+w+' '+RUN);
 for(let i=0;i<3;i++){await T.tap(arch[0],arch[1]);await sleep(10)}await sleep(700);
 ok(w,'archive via swipe persists server-side',!(await srv('/state')).agents.some(a=>a.name==='Trash '+w+' '+RUN));
 ok(w,'thread not opened by swipe/archive',await p.$eval('#thread',e=>e.classList.contains('off')));
 for(let i=0;i<4;i++){await swipe(w/2,600,w/2+6,200,4,5);await swipe(w/2,200,w/2-6,600,4,5)}await sleep(300);
 ok(w,'vertical flicks do not open rows',await p.$$eval('.row',r=>r.every(e=>!e.dataset.open)));
 // pin via swipe → featured row
 rp=await rowOf(name);await sleep(80);rp=await rowOf(name);await swipe(rp[0]+100,rp[1],rp[0]-120,rp[1],6,8);await sleep(350);
 const pinB=await p.evaluate(n=>{const r=[...document.querySelectorAll('.row')].find(e=>e.querySelector('.rowh b').textContent===n);const b=r.querySelector('.pin').getBoundingClientRect();return[b.x+b.width/2,b.y+b.height/2]},name);
 await T.tap(pinB[0],pinB[1]);await sleep(600);await p.$eval('#listScroll',e=>e.scrollTop=0);await sleep(100);
 ok(w,'pin moves bot to featured row',await p.$$eval('.feat .feat-name',(l,n)=>l.some(x=>x.textContent===n),name)&&(await srv('/state')).agents.find(a=>a.name===name).pin);
 // unread: another bot replies while its thread is closed → dot + bold row
 const other=(await srv('/state')).agents.find(a=>a.kind!=='group'&&a.name!==name&&!a.pin);
 if(other){await p.evaluate(async id=>{await fetch('/app/api/agents/'+id+'/messages',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({text:'Chào nhé'})})},other.id);
  let un=false;for(let i=0;i<(REAL?600:80);i++){un=await p.evaluate(id=>!!document.querySelector(`.row[data-id="${id}"] .dot`),other.id);if(un)break;await sleep(100)}
  ok(w,'reply in a closed thread shows unread dot',un);await p.screenshot({path:`${O}${w}-1b-unread.png`})}
 // bottom search bar filters
 await tap('#search');await p.keyboard.type(name);await sleep(200);
 ok(w,'bottom search filters rows',await p.$$eval('.row .rowh b,.feat .feat-name',(l,n)=>l.length>=1&&l.every(x=>x.textContent===n),name));
 await p.$eval('#search',e=>{e.value='';e.dispatchEvent(new Event('input'))});await p.evaluate(()=>document.activeElement.blur());await sleep(200);
 // new group chat: "Tới:" search, multi-select, Next → real group thread with fan-out
 await tap('#newBtn');await sleep(250);await tap('#menuGroup');await sleep(500);
 ok(w,'Tiếp disabled until 2 bots picked',await p.$eval('#groupNext',e=>e.disabled));
 await tap('#groupSearch');await p.keyboard.type(name);await sleep(150);await tap('.pick-row');await sleep(80);
 await p.$eval('#groupSearch',e=>{e.value='';e.dispatchEvent(new Event('input'))});await sleep(100);
 await p.evaluate(n=>{const r=[...document.querySelectorAll('.pick-row')].find(x=>x.querySelector('b').textContent!==n&&x.getAttribute('aria-pressed')!=='true');r.scrollIntoView({block:'center'})},name);await sleep(80);
 const sec=await p.evaluate(n=>{const r=[...document.querySelectorAll('.pick-row')].find(x=>x.querySelector('b').textContent!==n&&x.getAttribute('aria-pressed')!=='true');const b=r.getBoundingClientRect();return[b.x+b.width/2,b.y+b.height/2]},name);await T.tap(sec[0],sec[1]);await sleep(150);
 ok(w,'2 chips in "Tới:" and Tiếp enabled',(await p.$$('.gchip')).length===2&&await p.$eval('#groupNext',e=>!e.disabled));
 sm=await small();ok(w,'touch>=44 group sheet',!sm.length,sm.join(','));await p.screenshot({path:`${O}${w}-2d-group.png`});
 const g0=(await srv('/state')).agents.filter(a=>a.kind==='group').length;for(let i=0;i<4;i++){try{await tap('#groupNext')}catch{}await sleep(10)}await sleep(1200);
 const gs=(await srv('/state')).agents.filter(a=>a.kind==='group');ok(w,'4 fast taps on Tiếp → exactly 1 group',gs.length===g0+1,`${g0}->${gs.length}`);
 ok(w,'group thread opens',await p.$eval('#thread',e=>!e.classList.contains('off'))&&await p.evaluate(()=>(__AURORA.current()||'').startsWith('channel-')));
 await tap('#input');await p.keyboard.type('Chào cả nhóm, mỗi bạn giới thiệu một câu');await p.keyboard.press('Enter');
 let gok=false;for(let i=0;i<(REAL?900:120);i++){gok=await p.evaluate(()=>{const L=__AURORA.state().msgs[__AURORA.current()]||[];return new Set(L.filter(m=>m.role==='agent'&&m.from&&m.text).map(m=>m.from)).size>=2});if(gok)break;await sleep(100)}
 const spk=await p.$$eval('.speaker span',l=>l.map(x=>x.textContent));
 ok(w,'both group members reply with speaker labels',gok&&spk.length>=2,spk.join(','));await sleep(300);await p.screenshot({path:`${O}${w}-2e-group-thread.png`});
 await tap('#backBtn');await sleep(450);
 // thread + streaming AI reply + approval gate
 rp=await rowOf(name);await sleep(80);rp=await rowOf(name);await T.tap(rp[0],rp[1]);await sleep(450);
 ok(w,'tap row opens thread',await p.$eval('#thread',e=>!e.classList.contains('off')));
 ok(w,'no-h-overflow thread',!(await overflow()));sm=await small();ok(w,'touch>=44 thread',!sm.length,sm.join(','));
 const mic=await p.$eval('#micBtn',e=>!e.classList.contains('hidden'));results.push(`INFO ${w} voice button ${mic?'shown (Web Speech available)':'hidden (unsupported)'}`);
 const d0=await p.evaluate(()=>__AURORA.state().deltaCount||0);await tap('#input');await p.keyboard.type('Soạn giúp mình 2 câu giới thiệu Neko Aurora');await p.keyboard.press('Enter');await p.keyboard.press('Enter');
 let sawDraft=false;for(let i=0;i<(REAL?600:100);i++){if(await p.$('.msg.draft'))sawDraft=true;const done=await p.evaluate(n=>{const a=__AURORA.state().agents.find(x=>x.name===n);const L=(__AURORA.state().msgs[a.id]||[]);return a&&a.status!=='working'&&L.filter(m=>m.role==='agent').length>=2&&!document.querySelector('.msg.draft')&&!document.querySelector('.msg.typing')},name);if(done&&i>3)break;await sleep(100)}
 const deltas=(await p.evaluate(()=>__AURORA.state().deltaCount||0))-d0;sawDraft=sawDraft||deltas>1;const ms1=await srv('/agents/'+(await p.evaluate(()=>__AURORA.current()))+'/messages');
 ok(w,'user msg persisted once (double Enter)',ms1.filter(m=>m.role==='user').length===1);
 const reply=ms1.filter(m=>m.role==='agent').at(-1);ok(w,'AI reply streamed and persisted',sawDraft&&reply&&reply.text.length>10,`deltas=${deltas} `+(reply?.text||'').slice(0,70).replace(/\n/g,' '));
 await p.screenshot({path:`${O}${w}-4-thread-reply.png`});
 // reply flow: centered day stamp above the user bubble; markdown answer (heading, list, table, bold, link)
 ok(w,'centered "Hôm nay hh:mm" stamp above user message',await p.$eval('.daystamp',e=>/^Hôm nay \d{2}:\d{2}$/.test(e.textContent)&&getComputedStyle(e).textAlign==='center'));
 await tap('#input');await p.keyboard.type('Cho mình xem bảng markdown mẫu');await p.keyboard.press('Enter');
 let stOn=false;for(let i=0;i<40&&!stOn;i++){stOn=!!(await p.$("#messages .status"));if(!stOn)await sleep(50)}ok(w,"single status line (face + text, no dots) while working",stOn&&!(await p.$('#messages .dots')));
 ok(w,'composer collapses + keyboard dismissed after send',await p.evaluate(()=>document.activeElement!==document.querySelector('#input')&&document.querySelector('#input').value===''));
 let mdOk=null;for(let i=0;i<(REAL?600:100)&&!mdOk;i++){mdOk=await p.evaluate(()=>{const b=[...document.querySelectorAll('.msg.bot.md')].at(-1);return b&&b.querySelector('h3')&&b.querySelectorAll('ul li').length>=2&&b.querySelector('table td')&&b.querySelector('strong')&&b.querySelector('a.lnk[target=_blank][rel*=noopener]')?{lh:getComputedStyle(b).lineHeight,fs:getComputedStyle(b).fontSize}:null});if(!mdOk)await sleep(100)}
 ok(w,'markdown renders heading, bullets, table, bold, safe link',!!mdOk&&!(await p.$('#messages .status')),JSON.stringify(mdOk));
 ok(w,'no-h-overflow with table',!(await overflow()));await p.screenshot({path:`${O}${w}-4a-markdown.png`});
 await p.$eval('#messages',e=>{e.style.maxHeight='300px';e.scrollTop=0});await sleep(250);{const j=await p.evaluate(()=>{const m=document.querySelector('#messages');return{h:document.querySelector('#jumpBtn').classList.contains('hidden'),st:m.scrollTop,sh:m.scrollHeight,ch:m.clientHeight}});ok(w,'scrolled up → round jump-to-bottom button',!j.h,JSON.stringify(j))}await tap('#jumpBtn');await sleep(700);ok(w,'jump button returns to bottom',await p.$eval('#jumpBtn',e=>e.classList.contains('hidden')));await p.$eval('#messages',e=>e.style.maxHeight='');
 await p.setOfflineMode(true);await sleep(300);ok(w,'offline → quiet status pill shown',await p.$eval('#netBanner',e=>!e.classList.contains('hidden')&&/Ngoại tuyến/.test(e.textContent)));
 await p.setOfflineMode(false);let back=false;for(let i=0;i<60&&!back;i++){back=await p.$eval('#netBanner',e=>e.classList.contains('hidden'));if(!back)await sleep(100)}ok(w,'back online → pill hides',back);
 // Grok-like look: neutral default theme, system font, black user bubble, plain assistant text
 const look=await p.evaluate(()=>{const cs=getComputedStyle(document.body);const u=document.querySelector('.msg.mine'),g=document.querySelector('.msg.bot');return{theme:document.documentElement.dataset.theme||'',bg:cs.backgroundColor,font:cs.fontFamily,ub:u&&getComputedStyle(u).backgroundColor,uc:u&&getComputedStyle(u).color,gb:g&&getComputedStyle(g).backgroundColor}});
 ok(w,'default neutral theme + system font stack',!look.theme&&look.bg==='rgb(247, 247, 247)'&&/^-apple-system/.test(look.font),look.bg+' '+look.font.slice(0,40));
 ok(w,'black user bubble, white text; assistant light-gray bubble',/^rgb\((0|13), \1, \1\)$/.test(look.ub)&&look.uc==='rgb(247, 247, 247)'&&look.gb==='rgb(237, 237, 238)',JSON.stringify([look.ub,look.uc,look.gb]));
 ok(w,'composer: + button, "Hỏi <bot>" pill, black voice button',await p.$eval('#input',(e,n)=>e.placeholder==='Hỏi '+n,name)&&await p.$eval('#composer .send',e=>e.classList.contains('voice')&&/^rgb\((0|13), \1, \1\)$/.test(getComputedStyle(e).backgroundColor))&&!!(await p.$('#plusBtn')));
 // header popover → find in chat
 await tap('#threadMore');await sleep(350);ok(w,'thread ⋯ opens frosted popover (Xem hồ sơ / Tìm / Chia sẻ)',await p.$eval('#threadMenu',e=>!e.classList.contains('hidden')&&getComputedStyle(e).backdropFilter.includes('blur')&&e.querySelectorAll('button').length===3));
 sm=await small();ok(w,'touch>=44 thread popover',!sm.length,sm.join(','));
 await p.screenshot({path:`${O}${w}-4b-chat-popover.png`});
 await tap('#tmFind');await sleep(250);await p.keyboard.type('Neko');await sleep(250);ok(w,'Tìm trong chat highlights matches',(await p.$$('.msg.hit')).length>=1,await p.$eval('#findCount',e=>e.textContent));await tap('#findClose');await sleep(150);
 // profile page
 await tap('#threadMore');await sleep(300);await tap('#tmProfile');await sleep(500);
 ok(w,'Xem hồ sơ opens bot profile',await p.$eval('#profile',e=>!e.classList.contains('off'))&&await p.$eval('#profBody h2',(e,n)=>e.textContent===n,name));
 ok(w,'profile rows carry real values',await p.$eval('#prChat .cell-v',e=>/\d+ tin/.test(e.textContent))&&await p.$eval('#prPend .cell-v',e=>/\d+ chờ/.test(e.textContent))&&!!(await p.$('#prRun .cell-v')));
 ok(w,'no-h-overflow profile',!(await overflow()));sm=await small();ok(w,'touch>=44 profile',!sm.length,sm.join(','));
 await p.screenshot({path:`${O}${w}-4c-profile.png`});
 const pid=await p.evaluate(()=>__AURORA.current());await tap('#prNotify .switch');await sleep(150);
 ok(w,'profile notifications toggle mutes this bot on device',await p.$eval('#prNotify .switch',e=>e.getAttribute('aria-checked')==='false')&&await p.evaluate(id=>JSON.parse(localStorage.getItem('neko-aurora-muted')||'[]').includes(id),pid));
 await tap('#prNotify .switch');await sleep(150);
 await tap('#profMore');await sleep(300);ok(w,'profile ⋯ popover (Sao chép ID, Ghim)',await p.$eval('#profMenu',e=>!e.classList.contains('hidden')));
 await p.screenshot({path:`${O}${w}-4d-profile-popover.png`});
 await tap('#pmCopy');await sleep(300);ok(w,'Sao chép ID gives feedback',/ID/.test(await p.$eval('#toast',e=>e.textContent)));
 await tap('#profEdit');await sleep(500);ok(w,'Sửa opens editor prefilled',await p.$eval('#sheetTitle',e=>e.textContent==='Sửa bot')&&await p.$eval('#agentName',(e,n)=>e.value===n,name));
 const desc='Soạn bài và lịch '+w;await p.$eval('#agentDesc',e=>e.value='');await tap('#agentDesc');await p.keyboard.type(desc);for(let i=0;i<3;i++){await tap('#createBtn');await sleep(10)}await sleep(700);
 ok(w,'Lưu patches the bot on server',(await srv('/state')).agents.find(a=>a.id===pid)?.desc===desc&&await p.$eval('#profBody .hero p',(e,d)=>e.textContent===d,desc));
 await tap('#profBack');await sleep(450);ok(w,'profile back returns to the chat',await p.$eval('#profile',e=>e.classList.contains('off'))&&await p.$eval('#thread',e=>!e.classList.contains('off')));
 await tap('#input');await p.keyboard.type('Gửi email cho elena@example.com kèm deck cho buổi họp thứ Năm');await p.keyboard.press('Enter');
 let ap=null;for(let i=0;i<(REAL?600:100);i++){ap=await p.$('.approval:not(.done) .go');if(ap)break;await sleep(100)}
 ok(w,'outbound request gates on approval card',!!ap);await sleep(200);
 await p.screenshot({path:`${O}${w}-5-approval.png`});
 if(ap){for(let i=0;i<5;i++){try{await tap('.approval:not(.done) .go')}catch{}await sleep(8)}await sleep(900);
  const ms2=await srv('/agents/'+(await p.evaluate(()=>__AURORA.current()))+'/messages');const apps=ms2.filter(m=>m.approval);
  ok(w,'5 fast taps on Duyệt → 1 server decision',apps.length>=1&&apps.at(-1).approval.state==='approved'&&ms2.filter(m=>m.text.startsWith('Đã ghi nhận bạn duyệt')).length===1);
  ok(w,'approved item not sent externally (honest follow-up)',ms2.some(m=>m.text.includes('chưa gửi')));
  await p.screenshot({path:`${O}${w}-6-approved.png`})}
 // generative UI: multi-step plan card + quick choices + stop
 await tap('#input');await p.keyboard.type('Lập kế hoạch viết bài giới thiệu');await p.keyboard.press('Enter');
 let planOk=false;for(let i=0;i<(REAL?900:150);i++){planOk=await p.evaluate(()=>{const s=[...document.querySelectorAll('.plan .step')];const c=x=>x.classList;const allDone=s.length>=2&&s.every(x=>c(x).contains('done')||c(x).contains('waiting'));const paused=s.length>=2&&s.some(x=>c(x).contains('waiting'))&&s.every(x=>!c(x).contains('running')&&!c(x).contains('pending'))&&!!document.querySelector('.choices button,.approval');return allDone||paused});if(planOk)break;await sleep(100)}
 ok(w,'plan card steps run live (done or paused for owner)',planOk,await p.$$eval('.plan .step',l=>l.map(x=>x.className.replace('step ','')).join(',')));
 const hasChoice=await p.$('.choice:not([disabled])');if(!REAL)ok(w,'quick-choice buttons rendered',!!hasChoice);
 await p.screenshot({path:`${O}${w}-6b-plan.png`});
 if(hasChoice){for(let i=0;i<4;i++){try{await tap('.choice')}catch{}await sleep(10)}await sleep(600);const n=(await srv('/agents/'+(await p.evaluate(()=>__AURORA.current()))+'/messages')).filter(m=>m.role==='user'&&(m.text==='Thân mật'||m.text==='Trang trọng')).length;if(!REAL)ok(w,'4 fast taps on a choice send it once',n===1,'n='+n)}
 if(!REAL){for(let i=0;i<60;i++){if(await p.evaluate(()=>{const a=__AURORA.state().agents.find(x=>x.id===__AURORA.current());return a&&a.status!=='working'}))break;await sleep(100)}
  await tap('#input');await p.keyboard.type('Viết thật chậm');await p.keyboard.press('Enter');await sleep(600);
  ok(w,'composer stays idle while working (voice button, not stop)',await p.$eval('#composer .send',e=>e.classList.contains('voice')));
  ok(w,'status line shows spinner + stop',!!(await p.$('#messages .status .st-spin'))&&!!(await p.$('#messages .status .st-stop')));
  await tap('#messages .status .st-stop');await sleep(900);const ag=await p.evaluate(()=>__AURORA.state().agents.find(x=>x.id===__AURORA.current()));ok(w,'stop halts the running turn',ag&&ag.status==='idle',ag&&ag.status);
  ok(w,'status line gone after stop',!(await p.$('#messages .status .st-stop')))}
 // activity sheet
 await tap('#plusBtn');await sleep(300);await tap('#activityBtn');await sleep(900);const logN=await p.$$eval('#log li',l=>l.length);ok(w,'activity log loads from server',logN>3,'items='+logN);
 await p.screenshot({path:`${O}${w}-7-activity.png`});await tap('#closeActivity');await sleep(450);
 // edge swipe back
 await swipe(5,400,w*0.8,410,5,6);await sleep(450);ok(w,'edge swipe back closes thread',await p.$eval('#thread',e=>e.classList.contains('off')));
 // stress
 await p.evaluate(()=>{window.__f=[];let l=performance.now();const f=t=>{__f.push(t-l);l=t;if(__f.length<4000)requestAnimationFrame(f)};requestAnimationFrame(f)});
 for(let i=0;i<10;i++){const [x,y]=await box('.row:nth-child(3) .rowin');await T.tap(x,y);await sleep(i%2?340:60);if(i%2)await tap('#backBtn');else await swipe(4,400,w*.9,400,4,5);await sleep(340)}
 await sleep(500);
 const st=await p.evaluate(()=>({off:document.querySelector('#thread').classList.contains('off'),lt:document.querySelector('#list').style.transform,url:location.pathname,f:__f.slice(5)}));
 const avg=st.f.reduce((a,b)=>a+b,0)/st.f.length,long=st.f.filter(x=>x>50).length;
 ok(w,'10 rapid open/back cycles end on list, still in app',st.off&&!st.lt&&st.url==='/app/');ok(w,'frame avg',avg<25,`avg=${avg.toFixed(1)}ms long>50ms=${long}`);
 await p.screenshot({path:`${O}${w}-8-after-stress.png`});
 // SSE reconnect: reload keeps server state
 await p.reload({waitUntil:'domcontentloaded'});await p.waitForFunction('window.__AURORA&&__AURORA.state().agents.length>0');ok(w,'state survives reload (server-side)',await p.evaluate(n=>__AURORA.state().agents.some(a=>a.name===n),name));
 // Gmail setup → vault (masked) → "Gửi thử" → approval → fake SMTP captures delivery
 if(SMTP){const post=(q,b)=>p.evaluate(async([q,b])=>(await fetch('/app/api'+q,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(b)})).status,[q,b]);
  await post('/connectors/gmail/disconnect',{});await fetch(SMTP+'/reset');await p.evaluate(()=>__AURORA.reload());
  await tap('#appsBtn');await sleep(600);await p.$eval('.conn[data-id=gmail]',e=>e.scrollIntoView({block:'center'}));
  ok(w,'Gmail row offers Kết nối',await p.$eval('.conn[data-id=gmail] .conn-btn',e=>e.textContent==='Kết nối'));
  await tap('.conn[data-id=gmail] .conn-btn');await sleep(700);
  ok(w,'Kết nối opens Gmail setup sheet',await p.$eval('#connSetup',e=>!e.classList.contains('hidden')&&!e.classList.contains('closed'))&&await p.$eval('#connectors',e=>e.classList.contains('hidden')));
  ok(w,'setup: masked app-password field + Google app-passwords link',await p.$eval('#gmailPass',e=>e.type==='password'&&e.autocomplete==='off'&&e.hasAttribute('data-1p-ignore'))&&await p.$eval('.secret a',e=>e.href==='https://myaccount.google.com/apppasswords'&&e.rel.includes('noopener')));
  sm=await small();ok(w,'touch>=44 Gmail setup',!sm.length,sm.join(','));ok(w,'no-h-overflow Gmail setup',!(await overflow()));
  const addr=`darren.${w}@gmail.com`,PW='abcd efgh ijkl mnop'; // test-only fake app password
  await tap('#gmailAddr');await p.keyboard.type(addr);await tap('#gmailPass');await p.keyboard.type(PW);
  await p.screenshot({path:`${O}${w}-9a-gmail-setup.png`});
  await tap('#gmailSave');let tb=null;for(let i=0;i<40&&!tb;i++){tb=await p.$('#gmailTest');await sleep(100)}
  const sec=await p.evaluate(async()=>await (await fetch('/app/api/secrets')).text());const st2=await p.evaluate(async()=>await (await fetch('/app/api/state')).text());
  ok(w,'Lưu stores Gmail in vault; API lists names only',!!tb&&sec.includes('GMAIL_APP_PASSWORD')&&!/abcd/.test(sec)&&!/abcd/.test(st2)&&await p.$eval('#gmailPass',e=>e.value===''));
  await p.screenshot({path:`${O}${w}-9b-gmail-connected.png`});
  const to=`friend.${w}.${RUN}@example.org`;await p.$eval('#gmailTestTo',e=>{e.value='';e.scrollIntoView({block:'center'})});await sleep(80);await tap('#gmailTestTo');await p.keyboard.type(to);await p.$eval('#gmailTest',e=>e.scrollIntoView({block:'center'}));await sleep(80);await tap('#gmailTest');
  let ga=null;for(let i=0;i<(REAL?900:120);i++){ga=await p.$('.approval:not(.done) .go');if(ga&&await p.evaluate(t=>[...document.querySelectorAll('.approval:not(.done) .ap-meta b')].some(b=>b.textContent===t),to))break;ga=null;await sleep(100)}
  ok(w,'Gửi thử → bot drafts test email behind approval card',!!ga);await sleep(200);await p.screenshot({path:`${O}${w}-9c-gmail-test-approval.png`});
  const before=await (await fetch(SMTP+'/mails')).json();ok(w,'nothing delivered before Duyệt',before.length===0);
  if(ga){await tap('.approval:not(.done) .go');let m=[];for(let i=0;i<80&&!m.length;i++){await sleep(100);m=await (await fetch(SMTP+'/mails')).json()}
   const {createHash}=await import('node:crypto');const want=createHash('sha256').update(PW.replace(/ /g,'')).digest('hex');
   ok(w,'after Duyệt: fake SMTP got 1 mail from Gmail address via STARTTLS+AUTH with vault password',m.length===1&&m[0].from===addr&&m[0].to===to&&m[0].user===addr&&m[0].pwSha===want&&/Subject: /.test(m[0].data),m.length?`from=${m[0].from} to=${m[0].to}`:'none');
   let sent=false;for(let i=0;i<40&&!sent;i++){sent=await p.evaluate(()=>[...document.querySelectorAll('.approval.done small')].some(s=>/đã gửi từ/.test(s.textContent)));if(!sent)await sleep(100)}
   ok(w,'card shows "đã gửi từ <gmail>"',sent);await p.screenshot({path:`${O}${w}-9d-gmail-sent.png`})}
  // in-chat secret request card
  await tap('#input');await p.keyboard.type('Bạn cần khoá API gì thì xin mình nhé');await p.keyboard.press('Enter');
  let si=null;for(let i=0;i<(REAL?600:100)&&!si;i++){si=await p.$('#messages .secret:not(.saved) input[data-secret]');if(!si)await sleep(100)}
  ok(w,'bot SECRET request renders masked card (label, name, Lưu)',!!si&&await p.$eval('#messages .secret:not(.saved) input[data-secret]',e=>e.type==='password')&&await p.$eval('#messages .secret:not(.saved) code',e=>/^[A-Z][A-Z0-9_]+$/.test(e.textContent)));
  sm=await small();ok(w,'touch>=44 secret card',!sm.length,sm.join(','));
  await p.$eval('#messages .secret:not(.saved)',e=>e.scrollIntoView({block:'center'}));await p.screenshot({path:`${O}${w}-9e-secret-card.png`});
  for(let i=0;i<50;i++){if(await p.evaluate(()=>{const a=__AURORA.state().agents.find(x=>x.id===__AURORA.current());return a&&a.status!=='working'}))break;await sleep(100)}await sleep(300);
  if(si){await p.$eval('#messages .secret:not(.saved) input[data-secret]',e=>e.scrollIntoView({block:'center'}));await sleep(100);const nm=await p.$eval('#messages .secret:not(.saved)',e=>e.dataset.name);const SV='tok-'+RUN+'-zzq';await tap('#messages .secret:not(.saved) input[data-secret]');await p.keyboard.type(SV);await tap('#messages .secret:not(.saved) .sec-save');
   let done=false;for(let i=0;i<30&&!done;i++){done=!!(await p.$(`#messages .secret.saved[data-name="${nm}"]`));if(!done)await sleep(100)}
   const msgs=JSON.stringify(await srv('/agents/'+(await p.evaluate(()=>__AURORA.current()))+'/messages'))+JSON.stringify(await srv('/activity?limit=200'));
   const listed=(await p.evaluate(async()=>await (await fetch('/app/api/secrets')).text())).includes(nm);
   ok(w,'Lưu → card shows "Đã lưu"; value absent from messages/activity',done&&listed&&!msgs.includes(SV)&&(await p.evaluate(async()=>await (await fetch('/app/api/secrets')).text())).includes(nm),`done=${done} listed=${listed} toast=${await p.$eval('#toast',e=>e.textContent)}`);
   await p.screenshot({path:`${O}${w}-9f-secret-saved.png`});await post('/secrets/'+nm+'/delete',{})}
  await p.goto(U,{waitUntil:'domcontentloaded'});await p.waitForFunction('window.__AURORA&&__AURORA.state().agents.length>0');await sleep(300);
  await tap('#appsBtn');await sleep(600);ok(w,'Gmail row: disabled-look "Đã kết nối" + 1 tài khoản',await p.$eval('.conn[data-id=gmail]',e=>e.querySelector('.conn-btn').textContent==='Đã kết nối'&&e.querySelector('.conn-btn').classList.contains('on')&&/1 tài khoản/.test(e.querySelector('small').textContent)));
  await p.screenshot({path:`${O}${w}-9g-connectors-connected.png`});await tap('#closeConnectors');await sleep(400)}
 // i18n: English UI on demand (and when the device language is English); chat content untouched
 await p.evaluate(()=>localStorage.setItem('neko-aurora-lang','en'));await p.reload();await p.waitForFunction('window.__AURORA&&__AURORA.state().agents.length>0');await sleep(500);
 const en=await p.evaluate(()=>({lang:document.documentElement.lang,me:document.querySelector('#meBtn').getAttribute('aria-label'),ph:document.querySelector('#input').getAttribute('placeholder')}));
 await tap('#meBtn');await sleep(500);const ens=await p.evaluate(()=>({t:document.querySelector('#settings .sheet-top b').textContent,body:document.querySelector('#settingsBody').innerText}));
 ok(w,'English UI: lang=en, labels + settings translated',en.lang==='en'&&en.me==='Settings'&&ens.t==='Settings'&&/Language/.test(ens.body)&&/Log out|Sign out/.test(ens.body)&&!/Đăng xuất|Ngôn ngữ/.test(ens.body),JSON.stringify([en,ens.t]));
 await p.screenshot({path:`${O}${w}-9-english-settings.png`});
 await p.evaluate(()=>localStorage.setItem('neko-aurora-lang','auto'));await p.reload();await p.waitForFunction('window.__AURORA&&__AURORA.state().agents.length>0');await sleep(300);
 ok(w,'auto follows device language (test browser is en-US)',await p.evaluate(()=>navigator.language.startsWith('en')&&document.documentElement.lang==='en'));
 await p.evaluate(()=>localStorage.setItem('neko-aurora-lang','vi'));await p.reload();await p.waitForFunction('window.__AURORA&&__AURORA.state().agents.length>0');await sleep(300);
 ok(w,'back to Vietnamese',await p.evaluate(()=>document.documentElement.lang==='vi'&&document.querySelector('#meBtn').getAttribute('aria-label')==='Cài đặt'));
 ok(w,'no page errors',!errs.length,errs.join('|').slice(0,200));
 await ctx.close();
}catch(e){ok(w,'EXC',false,e.message.slice(0,160));try{await p.screenshot({path:O+w+'-exc.png'})}catch{}}}
// brief
{const p=await b.newPage();await p.goto(U,{waitUntil:'domcontentloaded'});await p.waitForFunction('window.__AURORA');
 const br=await p.evaluate(async()=>(await fetch('/app/api/brief',{method:'POST',headers:{'Content-Type':'application/json'},body:'{}'})).json());
 ok('all','morning brief generated',!!br.text,`source=${br.source} "${(br.text||'').slice(0,80)}"`);
 const man=await p.evaluate(async()=>(await fetch('/app/manifest.webmanifest')).json());ok('all','manifest standalone',man.display==='standalone'&&man.start_url==='/app/');
 const sw=await p.evaluate(async()=>{const r=await navigator.serviceWorker.getRegistration('/app/');return !!r});ok('all','service worker registered',sw);
 const leak=await p.evaluate(async()=>{const t=await (await fetch('/app/api/state')).text();return /api[_-]?key|bearer/i.test(t)});ok('all','no API key material in client payloads',!leak);
 await p.close()}
await b.close();console.log(results.join('\n'));console.log('FAILS',fail);process.exit(fail?1:0);
