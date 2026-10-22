import puppeteer from 'puppeteer-core'; // npm i puppeteer-core; CHROME=/path/to/chrome
const U=process.env.DUET_URL||'http://127.0.0.1:8765/duet/',O=process.env.OUT||'./duet-shots/';
const sleep=ms=>new Promise(r=>setTimeout(r,ms));
const b=await puppeteer.launch({executablePath:process.env.CHROME||'/usr/bin/google-chrome',args:['--no-sandbox','--hide-scrollbars']});
const results=[];let fail=0;
const ok=(w,name,cond,extra='')=>{results.push(`${cond?'PASS':'FAIL'} ${w} ${name} ${extra}`);if(!cond)fail++};
for(const w of [360,390,430]){try{
 const ctx=await b.createBrowserContext();const p=await ctx.newPage();globalThis.P=p;const errs=[];p.on('pageerror',e=>errs.push(String(e)));
 await p.setViewport({width:w,height:Math.round(w*2.165),deviceScaleFactor:2,isMobile:true,hasTouch:true});
 await p.goto(U,{waitUntil:'domcontentloaded'});await p.waitForFunction('window.__DUET');await p.evaluate(()=>{localStorage.clear();__DUET.reset()});await sleep(300);
 const T=p.touchscreen;
 const swipe=async(x1,y1,x2,y2,steps=6,dt=8)=>{await T.touchStart(x1,y1);for(let i=1;i<=steps;i++){await T.touchMove(x1+(x2-x1)*i/steps,y1+(y2-y1)*i/steps);await sleep(dt)}await T.touchEnd()};
 const box=async s=>{const r=await p.$eval(s,e=>{const b=e.getBoundingClientRect();return[b.x+b.width/2,b.y+b.height/2]});return r};
 const tap=async s=>{const [x,y]=await box(s);await T.tap(x,y)};
 const overflow=()=>p.evaluate(()=>document.documentElement.scrollWidth>innerWidth||document.body.scrollWidth>innerWidth);
 const small=()=>p.evaluate(()=>[...document.querySelectorAll('button,input,textarea,[role=button]')].filter(e=>{const r=e.getBoundingClientRect();const s=getComputedStyle(e);return r.width&&r.height&&s.visibility!=='hidden'&&r.bottom>0&&r.top<innerHeight&&r.right>0&&r.left<innerWidth&&(r.width<43.5||r.height<43.5)}).map(e=>(e.className||e.tagName)+':'+e.getBoundingClientRect().width.toFixed(2)+'x'+e.getBoundingClientRect().height.toFixed(2)));
 ok(w,'no-h-overflow list',!(await overflow()));let sm=await small();ok(w,'touch>=44 list',!sm.length,sm.join(','));
 await p.screenshot({path:`${O}${w}-1-list.png`});
 // fast repeated taps on +
 for(let i=0;i<6;i++){await tap('#newBtn');await sleep(15)}
 await sleep(400);ok(w,'sheet opens once after 6 fast taps',await p.$eval('#sheet',e=>!e.classList.contains('closed')&&!e.classList.contains('hidden')));
 for(let i=0;i<12;i++){await tap('#surpriseBtn');await sleep(10)}
 await p.$eval('#sheetBody',e=>e.scrollTop=0);await sleep(100);
 // pick explicit options like screenshot
 const pick=async(sel)=>{await p.$eval(sel,e=>e.scrollIntoView({block:'center'}));await sleep(60);await tap(sel)};
 await pick('.swatches .sw:nth-child(6)');await pick('.group:nth-child(2) .opt:nth-child(3)');await pick('.group:nth-child(3) .opt:nth-child(3)');await pick('.group:nth-child(4) .opt:nth-child(1)');
 await p.$eval('#sheetBody',e=>e.scrollTop=0);await sleep(150);
 ok(w,'no-h-overflow sheet',!(await overflow()));sm=await small();ok(w,'touch>=44 sheet',!sm.length,sm.join(','));
 await p.screenshot({path:`${O}${w}-2-creator.png`});
 // drag-to-dismiss via grip
 const [gx,gy]=await box('#grip');await swipe(gx,gy,gx,gy+260,8,10);await sleep(450);
 ok(w,'drag grip down dismisses sheet',await p.$eval('#sheet',e=>e.classList.contains('hidden')));
 // create
 const n0=await p.evaluate(()=>__DUET.state().agents.length);
 await tap('#newBtn');await sleep(400);await p.$eval('#agentName',e=>e.value='Neko Tester');for(let i=0;i<5;i++){await tap('#createBtn');await sleep(10)}await sleep(450);
 const n1=await p.evaluate(()=>__DUET.state().agents.length);ok(w,'5 fast taps on Create make exactly 1 agent',n1===n0+1,`${n0}->${n1}`);
 ok(w,'pre-swipe on list',await p.$eval('#thread',e=>e.classList.contains('off')));await p.screenshot({path:O+w+'-pre.png'});// swipe row left
 const [rx,ry]=await box('.row:nth-child(2) .rowin');await swipe(rx+100,ry,rx-120,ry,6,8);await sleep(350);
 ok(w,'row swipe reveals actions',await p.$eval('.row:nth-child(2)',e=>!!e.dataset.open));await p.screenshot({path:`${O}${w}-3-rowswipe.png`});
 await T.tap(rx-80,ry);await sleep(300);ok(w,'tap closes open row (no nav)',await p.$eval('#thread',e=>e.classList.contains('off')));
 // vertical fast flicks in list shouldn't open rows
 for(let i=0;i<4;i++){await swipe(w/2,600,w/2+6,200,4,5);await swipe(w/2,200,w/2-6,600,4,5)}await sleep(300);
 ok(w,'vertical flicks do not open rows',await p.$$eval('.row',r=>r.every(e=>!e.dataset.open)));
 await p.$eval('#listScroll',e=>e.scrollTop=0);
 // open thread Posting Buddy
 await p.evaluate(()=>{const r=[...document.querySelectorAll('.row')].find(e=>e.textContent.includes('Posting Buddy'));r.scrollIntoView({block:'center'})});await sleep(100);
 const pb=await p.evaluate(()=>{const r=[...document.querySelectorAll('.row')].find(e=>e.textContent.includes('Posting Buddy')).querySelector('.rowin').getBoundingClientRect();return[r.x+r.width/2,r.y+r.height/2]});
 await T.tap(pb[0],pb[1]);await sleep(400);ok(w,'tap row opens thread',await p.$eval('#thread',e=>!e.classList.contains('off')));
 ok(w,'no-h-overflow thread',!(await overflow()));sm=await small();ok(w,'touch>=44 thread',!sm.length,sm.join(','));
 await p.screenshot({path:`${O}${w}-4-thread.png`});
 // fast approve taps
 for(let i=0;i<5;i++){const g=await p.$('.approval .go');if(g){await tap('.approval .go')}await sleep(8)}await sleep(200);
 const dec=await p.evaluate(()=>__DUET.state().agents.find(a=>a.name==='Posting Buddy').msgs.filter(m=>m.t==='Xong. Mình đã ghi nhận bạn duyệt.').length);ok(w,'5 fast taps on Approve -> 1 decision',dec===1,'n='+dec);
 // send risky message
 await tap('#input');await p.keyboard.type('Gửi bản nháp cho Elena');await p.keyboard.press('Enter');await p.keyboard.press('Enter');await sleep(1000);
 const ap=await p.$$eval('.approval:not(.done)',e=>e.length);ok(w,'outbound request gates on approval',ap===1,'pending='+ap);
 await p.screenshot({path:`${O}${w}-5-approval.png`});
 // edge swipe back fast
 await swipe(5,400,w*0.8,410,5,6);await sleep(450);ok(w,'edge swipe back closes thread',await p.$eval('#thread',e=>e.classList.contains('off')));
 // stress: 10 rapid open/back cycles mixing tap-back and edge swipe; measure frames
 await p.evaluate(()=>{window.__f=[];let l=performance.now();const f=t=>{__f.push(t-l);l=t;if(__f.length<4000)requestAnimationFrame(f)};requestAnimationFrame(f)});
 for(let i=0;i<10;i++){const [x,y]=await box('.row:nth-child(3) .rowin');await T.tap(x,y);await sleep(i%2?340:60);const o1=await p.$eval('#thread',e=>e.classList.contains('off'));if(i%2)await tap('#backBtn');else await swipe(4,400,w*.9,400,4,5);await sleep(340);}
 await sleep(500);
 const st=await p.evaluate(()=>({off:document.querySelector('#thread').classList.contains('off'),lt:document.querySelector('#list').style.transform,f:__f.slice(5)}));
 const avg=st.f.reduce((a,b)=>a+b,0)/st.f.length,long=st.f.filter(x=>x>50).length;
 ok(w,'10 rapid open/back cycles end on list, consistent',st.off&&!st.lt);ok(w,'frame avg',avg<25,`avg=${avg.toFixed(1)}ms long>50ms=${long}`);
 ok(w,'no page errors',!errs.length,errs.join('|'));
 await p.screenshot({path:`${O}${w}-6-after-stress.png`});
}catch(e){ok(w,"EXC",false,e.message.slice(0,120));await P.screenshot({path:O+w+"-exc.png"});console.log(P.url())} 
}
// manifest check
const p=await b.newPage();await p.goto(U);const man=await p.evaluate(async()=>{const r=await fetch('/duet/manifest.webmanifest');return r.json()});ok('all','manifest standalone',man.display==='standalone'&&man.start_url==='/duet/');
await b.close();console.log(results.join('\n'));console.log('FAILS',fail);
