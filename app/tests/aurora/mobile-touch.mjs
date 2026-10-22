// Real-touch mobile test for /app/ against a running neko-aurora.
// npm i puppeteer-core ; AURORA_URL=http://127.0.0.1:8765/app/ OUT=./aurora-shots/ node mobile-touch.mjs
import puppeteer from 'puppeteer-core';
const U=process.env.AURORA_URL||'http://127.0.0.1:8765/app/',O=process.env.OUT||'./aurora-shots/';
const REAL=!!process.env.REAL_AI; // stricter timeouts off for a real model
const sleep=ms=>new Promise(r=>setTimeout(r,ms));
const b=await puppeteer.launch({executablePath:process.env.CHROME||'/usr/bin/google-chrome',args:['--no-sandbox','--hide-scrollbars']});
const results=[];let fail=0;
const ok=(w,name,cond,extra='')=>{results.push(`${cond?'PASS':'FAIL'} ${w} ${name} ${extra}`);if(!cond)fail++};
for(const w of (process.env.WIDTHS||'360,390,430').split(',').map(Number)){let p;try{
 const ctx=await b.createBrowserContext();await ctx.overridePermissions(new URL(U).origin,['notifications']);p=await ctx.newPage();const errs=[];p.on('pageerror',e=>errs.push(String(e)));
 await p.setViewport({width:w,height:Math.round(w*2.165),deviceScaleFactor:2,isMobile:true,hasTouch:true});
 await p.goto(U,{waitUntil:'domcontentloaded'});await p.waitForFunction('window.__AURORA&&__AURORA.state().agents.length>0',{timeout:10000});await sleep(300);
 const T=p.touchscreen;
 const swipe=async(x1,y1,x2,y2,steps=6,dt=8)=>{await T.touchStart(x1,y1);for(let i=1;i<=steps;i++){await T.touchMove(x1+(x2-x1)*i/steps,y1+(y2-y1)*i/steps);await sleep(dt)}await T.touchEnd()};
 const box=async s=>p.$eval(s,e=>{const b=e.getBoundingClientRect();return[b.x+b.width/2,b.y+b.height/2]});
 const tap=async s=>{const [x,y]=await box(s);await T.tap(x,y)};
 const srv=path=>p.evaluate(async q=>(await fetch('/app/api'+q,{cache:'no-store'})).json(),path);
 const overflow=()=>p.evaluate(()=>document.documentElement.scrollWidth>innerWidth||document.body.scrollWidth>innerWidth);
 const small=()=>p.evaluate(()=>[...document.querySelectorAll('button,input,textarea,[role=button]')].filter(e=>{const r=e.getBoundingClientRect();const s=getComputedStyle(e);return r.width&&r.height&&s.visibility!=='hidden'&&r.bottom>0&&r.top<innerHeight&&r.right>0&&r.left<innerWidth&&(r.width<43.5||r.height<43.5)}).map(e=>(e.id||e.className||e.tagName)+':'+e.getBoundingClientRect().width.toFixed(1)+'x'+e.getBoundingClientRect().height.toFixed(1)));
 const rowOf=name=>p.evaluate(n=>{const r=[...document.querySelectorAll('.row')].find(e=>e.querySelector('.rowh b').textContent===n);if(!r)return null;r.scrollIntoView({block:'center'});const b=r.querySelector('.rowin').getBoundingClientRect();return[b.x+b.width/2,b.y+b.height/2]},name);
 ok(w,'loaded from server',(await srv('/state')).agents.length>0);
 ok(w,'no-h-overflow list',!(await overflow()));let sm=await small();ok(w,'touch>=44 list',!sm.length,sm.join(','));
 await p.screenshot({path:`${O}${w}-1-list.png`});
 // notifications: permission granted, bell → subscribe or in-app fallback
 await tap('#bellBtn');await sleep(1500);const bell=await p.$eval('#bellBtn',e=>e.classList.contains('on'));const toastTxt=await p.$eval('#toast',e=>e.textContent);
 ok(w,'bell enables notifications (push or fallback)',bell,toastTxt);
 // creator
 for(let i=0;i<6;i++){await tap('#newBtn');await sleep(15)}
 await sleep(400);ok(w,'sheet opens once after 6 fast taps',await p.$eval('#sheet',e=>!e.classList.contains('closed')&&!e.classList.contains('hidden')));
 for(let i=0;i<12;i++){await tap('#surpriseBtn');await sleep(10)}
 const pick=async(sel)=>{await p.$eval(sel,e=>e.scrollIntoView({block:'center'}));await sleep(60);await tap(sel)};
 await pick('.swatches .sw:nth-child(6)');await pick('.group:nth-child(2) .opt:nth-child(3)');await pick('.group:nth-child(3) .opt:nth-child(3)');await pick('.group:nth-child(4) .opt:nth-child(1)');
 await p.$eval('#sheetBody',e=>e.scrollTop=0);await sleep(150);
 ok(w,'no-h-overflow sheet',!(await overflow()));sm=await small();ok(w,'touch>=44 sheet',!sm.length,sm.join(','));
 await p.screenshot({path:`${O}${w}-2-creator.png`});
 const [gx,gy]=await box('#grip');await swipe(gx,gy,gx,gy+260,8,10);await sleep(450);
 ok(w,'drag grip down dismisses sheet',await p.$eval('#sheet',e=>e.classList.contains('hidden')));
 const name='Tester '+w;const n0=(await srv('/state')).agents.length;
 await tap('#newBtn');await sleep(400);await p.$eval('#agentName',(e,n)=>e.value=n,name);for(let i=0;i<5;i++){await tap('#createBtn');await sleep(10)}await sleep(800);
 const st1=await srv('/state');ok(w,'5 fast taps on Create → exactly 1 agent on server',st1.agents.length===n0+1&&st1.agents.filter(a=>a.name===name).length===1,`${n0}->${st1.agents.length}`);
 // row swipe + archive a throwaway agent
 await p.evaluate(async n=>{await fetch('/app/api/agents',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({name:n,desc:'x',av:{shell:1,hat:'none',eyes:'none',extra:'none'}})})},'Trash '+w);await sleep(500);
 let rp=await rowOf('Trash '+w);await sleep(100);rp=await rowOf('Trash '+w);
 await swipe(rp[0]+100,rp[1],rp[0]-120,rp[1],6,8);await sleep(350);
 const opened=await p.evaluate(n=>!![...document.querySelectorAll('.row')].find(e=>e.querySelector('.rowh b').textContent===n)?.dataset.open,'Trash '+w);
 ok(w,'row swipe reveals actions',opened);await p.screenshot({path:`${O}${w}-3-rowswipe.png`});
 const arch=await p.evaluate(n=>{const r=[...document.querySelectorAll('.row')].find(e=>e.querySelector('.rowh b').textContent===n);const b=r.querySelector('.arch').getBoundingClientRect();return[b.x+b.width/2,b.y+b.height/2]},'Trash '+w);
 for(let i=0;i<3;i++){await T.tap(arch[0],arch[1]);await sleep(10)}await sleep(700);
 ok(w,'archive via swipe persists server-side',!(await srv('/state')).agents.some(a=>a.name==='Trash '+w));
 ok(w,'thread not opened by swipe/archive',await p.$eval('#thread',e=>e.classList.contains('off')));
 for(let i=0;i<4;i++){await swipe(w/2,600,w/2+6,200,4,5);await swipe(w/2,200,w/2-6,600,4,5)}await sleep(300);
 ok(w,'vertical flicks do not open rows',await p.$$eval('.row',r=>r.every(e=>!e.dataset.open)));
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
  const isStop=await p.$eval('#composer .send',e=>e.classList.contains('stop'));ok(w,'send turns into stop while working',isStop);
  await tap('#composer .send');await sleep(900);const ag=await p.evaluate(()=>__AURORA.state().agents.find(x=>x.id===__AURORA.current()));ok(w,'stop halts the running turn',ag&&ag.status==='idle',ag&&ag.status);
  ok(w,'send button back after stop',await p.$eval('#composer .send',e=>!e.classList.contains('stop')))}
 // activity sheet
 await tap('#activityBtn');await sleep(900);const logN=await p.$$eval('#log li',l=>l.length);ok(w,'activity log loads from server',logN>3,'items='+logN);
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
