// E2E: account deletion through the UI on a throwaway server.
// Usage: AURORA_BIN=/tmp/neko-aurora.new node account-delete.mjs
import {chromium} from 'playwright-core';import {spawn} from 'node:child_process';import fs from 'node:fs';import os from 'node:os';import path from 'node:path';
const st=fs.mkdtempSync(path.join(os.tmpdir(),'aurora-del-'));const port=8813,U=`http://127.0.0.1:${port}/app/`;const sl=ms=>new Promise(r=>setTimeout(r,ms));
const srv=spawn(process.env.AURORA_BIN||'/tmp/neko-aurora.new',['-state',st,'-listen','127.0.0.1:'+port],{env:{...process.env,AURORA_BACKUP_EVERY:'off',AURORA_BRIEF_LOOP:'off',ZAI_API_KEY:'fake-test-key',AURORA_AI_BASE_URL:'http://127.0.0.1:1'},stdio:['ignore','pipe','pipe']});
let out='';srv.stdout.on('data',d=>out+=d);srv.stderr.on('data',d=>out+=d);
const up=async()=>{for(let i=0;i<100;i++){try{if((await fetch(U)).ok)return}catch{}await sl(100)}console.log(out);throw new Error('server not up')};
let fail=0;const ok=(c,m)=>{console.log(c?'PASS':'FAIL',m);if(!c)fail++};
await up();const b=await chromium.launch({executablePath:'/usr/bin/google-chrome',args:['--no-sandbox']});
const p=await (await b.newContext({viewport:{width:390,height:844},isMobile:true,hasTouch:true,locale:'vi-VN'})).newPage();
await p.goto(U);await p.waitForFunction('window.__AURORA');
await p.evaluate(async()=>{const A=(u,x)=>fetch('/app/api'+u,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(x)});await A('/agents',{name:'Bot xoá thử'});await A('/secrets',{name:'DEMO_KEY',value:'abc123'})});
await p.reload();await p.waitForFunction('window.__AURORA&&__AURORA.state().agents.length>0');
const n0=await p.evaluate(()=>__AURORA.state().agents.length);ok(n0>0,'seeded '+n0+' agents');
ok(fs.existsSync(path.join(st,'aurora','vault.key')),'vault key exists before');
await p.locator('#meBtn').click();await sl(600);
await p.locator('#delAcctBtn').click();await sl(500);
ok(await p.locator('#delAcctGo').isDisabled(),'delete disabled before typing');
await p.locator('#delAcctInput').fill('xoa');ok(await p.locator('#delAcctGo').isDisabled(),'wrong word keeps it disabled');
await p.locator('#delAcctInput').fill('XOÁ');ok(!(await p.locator('#delAcctGo').isDisabled()),'XOÁ enables delete');
await p.locator('#delAcctGo').click();await sl(1200);
await up();await sl(500);
await p.goto(U);await p.waitForFunction('window.__AURORA');const st2=await p.evaluate(()=>fetch('/app/api/state').then(r=>r.json()));
const userBots=st2.agents.filter(a=>a.name==='Bot xoá thử').length;ok(userBots===0,'created bot gone after delete');
ok(!(st2.secrets||[]).includes('DEMO_KEY'),'secret gone');
ok(!fs.existsSync(path.join(st,'.delete-account')),'wipe marker cleared');
ok(out.includes('state wiped'),'server reported wipe and restarted');
await b.close();srv.kill();
fs.rmSync(st,{recursive:true,force:true});console.log(fail?`FAILS ${fail}`:'FAILS 0');process.exit(fail?1:0);
