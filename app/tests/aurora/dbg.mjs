import {chromium} from 'playwright-core';
const U='http://127.0.0.1:18791';const sl=ms=>new Promise(r=>setTimeout(r,ms));
const b=await chromium.launch({executablePath:'/usr/bin/google-chrome',args:['--no-sandbox']});
const ctx=await b.newContext({viewport:{width:390,height:844},isMobile:true,hasTouch:true});const p=await ctx.newPage();
p.on('console',m=>console.log('console',m.type(),m.text()));p.on('pageerror',e=>console.log('pageerror',e));
await p.goto(U+'/app/');await p.waitForURL(/signin/);await p.fill('#email','t@example.com');const rp=p.waitForResponse(r=>r.url().endsWith('/app/auth/email'));await p.tap('#sendBtn');const s=await (await rp).json();
await p.waitForSelector('#codeStep:not(.hidden)');await p.fill('#code',s.devCode);await p.waitForURL(/welcome|\/app\/(#.*)?$/);if(p.url().includes('welcome')){await p.fill('#name','D');await p.tap('[data-goal=work]');await p.tap('#startBtn')}
await p.waitForURL(/\/app\/(#bot-.*)?$/);await sl(4000);
const st=await (await p.request.get(U+'/app/api/state')).json();const id=st.agents[0].id;console.log('agents',st.agents.map(a=>a.id+':'+a.name).join(','));
await p.goto(U+'/app/#'+id);await sl(3000);await p.tap('#whoBtn');await sl(2500);
console.log(await p.evaluate(()=>[document.querySelector('#profile').className,document.querySelector('#profBody').innerHTML.slice(0,300),location.hash]));
await p.request.post(U+'/app/api/account/delete',{headers:{'content-type':'application/json',origin:U},data:{confirm:'DELETE'}});await b.close();
