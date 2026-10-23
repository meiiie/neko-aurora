// Hosted multi-user sign-up + onboarding (mobile, iPhone size).
// Server: AURORA_MODE=hosted AURORA_DEV_MAIL=1 ... -state DIR -listen 127.0.0.1:PORT
// U=http://127.0.0.1:8797 STATE=/tmp/hostdog/state OUT=/tmp/hostdog node hosted-onboarding.mjs
import {chromium} from 'playwright-core';import fs from 'node:fs';
const U=process.env.U||'http://127.0.0.1:8797',ST=process.env.STATE,OUT=process.env.OUT||'/tmp',sl=ms=>new Promise(r=>setTimeout(r,ms));
const b=await chromium.launch({executablePath:'/usr/bin/google-chrome',args:['--no-sandbox']});
const res={};const ok=(k,v)=>{res[k]=!!v;console.log(v?'PASS':'FAIL',k)};
for(const lang of ['vi','en']){
 const ctx=await b.newContext({viewport:{width:390,height:844},deviceScaleFactor:3,isMobile:true,hasTouch:true,locale:lang==='vi'?'vi-VN':'en-US',userAgent:'Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1'});
 const p=await ctx.newPage();await p.goto(U+'/app/');await p.waitForURL(/signin\.html/);await sl(800);
 await p.screenshot({path:`${OUT}/hosted-1-signin-${lang}.png`});
 const db=await p.$eval('#deployBtn',e=>{const r=e.getBoundingClientRect();return{h:r.height,w:r.width,href:e.href,target:e.target}});
 ok(lang+' deploy button ≥44pt tall, full width, opens Railway in new tab',db.h>=44&&db.w>=300&&/railway\.com/.test(db.href)&&db.target==='_blank');
 await p.evaluate(()=>document.querySelector('#deployBtn').scrollIntoView({block:'center'}));await sl(300);await p.screenshot({path:`${OUT}/hosted-2-deploy-${lang}.png`});
 const email=`tester-${lang}-${Date.now()}@example.com`;
 await p.fill('#email',email);await p.tap('#sendBtn');await p.waitForSelector('#codeStep:not(.hidden)');
 const f=`${ST}/hub/dev-mail/${email.replace('@','_at_')}.txt`;const code=fs.readFileSync(f,'utf8').match(/: (\d{6})\n/)[1];
 await p.screenshot({path:`${OUT}/hosted-3-code-${lang}.png`});
 await p.fill('#code','000000');await sl(600);ok(lang+' wrong code shows error',/không đúng|Wrong/.test(await p.textContent('#msg')));
 await p.fill('#code',code);await p.waitForURL(/welcome\.html/);await sl(800);
 await p.fill('#name',lang==='vi'?'Minh':'Alex');await p.tap('[data-goal=research]');await sl(200);
 await p.screenshot({path:`${OUT}/hosted-4-welcome-${lang}.png`});
 await p.tap('#startBtn');await p.waitForURL(/\/app\/#bot-meiiie/,{timeout:20000});await sl(3500);
 await p.screenshot({path:`${OUT}/hosted-5-aurora-${lang}.png`});
 const names=(await (await p.request.get(U+'/app/api/state')).json()).agents.map(a=>a.name);
 ok(lang+' onboarding created Aurora + helpers, demo bots gone',names.includes('Aurora')&&names.includes('Nghiên cứu')&&!names.includes('Davin')&&!names.includes('Meiiie'));
 await p.goto(U+'/app/');await sl(2500);await p.screenshot({path:`${OUT}/hosted-6-list-${lang}.png`});
 await ctx.close();
}
console.log('RESULT',JSON.stringify(res));await b.close();process.exit(Object.values(res).every(Boolean)?0:1);
