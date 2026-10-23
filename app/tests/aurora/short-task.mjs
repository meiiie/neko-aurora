// Short live check: sandbox commands need no approvals; presence dot on avatars.
import {chromium} from 'playwright-core';import fs from 'node:fs';
const U=process.env.U,EMAIL=process.env.EMAIL,O=process.env.O;if(!U||U.includes(':8765'))throw 0;fs.mkdirSync(O,{recursive:true});
const sl=ms=>new Promise(r=>setTimeout(r,ms));
const b=await chromium.launch({executablePath:'/usr/bin/google-chrome',args:['--no-sandbox']});
const ctx=await b.newContext({viewport:{width:390,height:844},deviceScaleFactor:2,isMobile:true,hasTouch:true,locale:'vi-VN'});const p=await ctx.newPage();
const errs=[];p.on('pageerror',e=>errs.push(String(e)));const H={'content-type':'application/json',origin:U};
const get=async x=>{const r=await p.request.get(U+'/app/api/'+x,{timeout:60000});return r.ok()?r.json():null};
const post=async(x,d)=>(await p.request.post(U+'/app/api/'+x,{headers:H,data:d})).status();
let bot,out={};
try{
 await p.goto(U+'/app/');await p.waitForURL(/signin/);await p.fill('#email',EMAIL);const rp=p.waitForResponse(r=>r.url().endsWith('/app/auth/email'));await p.tap('#sendBtn');const s=await (await rp).json();
 await p.waitForSelector('#codeStep:not(.hidden)');await p.fill('#code',s.devCode);await p.waitForURL(/welcome/);await p.fill('#name','Darren');await p.tap('[data-goal=research]');await p.tap('#startBtn');
 await p.waitForURL(/\/app\/(#bot-.*)?$/,{timeout:30000});for(let i=0;i<30&&!((await get('state'))?.agents||[]).some(a=>a.name==='Aurora');i++)await sl(2000);
 bot=(await get('state')).agents.find(a=>a.name==='Aurora').id;await sl(8000);
 await post(`agents/${bot}/messages`,{text:'Dùng máy tính của bạn: viết script python (imageio) tạo video mp4 dài 5 giây, nền trắng, chữ "Xin chào" màu xanh, chạy nó rồi gửi mình link tải.',clientNonce:'short-'+Date.now()});
 await sl(6000);await p.goto(U+'/app/');await sl(2500);await p.screenshot({path:O+'list-working-dot.png'});
 await p.evaluate(id=>location.hash=id,bot);await p.reload();await sl(3000);await p.screenshot({path:O+'chat-working.png'});
 const t0=Date.now();let st;
 for(;;){await sl(8000);st=(await get('state')).agents.find(a=>a.id===bot);const pend=(await get('state')).pending||[];if(pend.length)out.pendingSeen=(out.pendingSeen||0)+1;if(st.status!=='working'&&Date.now()-t0>30000||Date.now()-t0>9*60e3)break}
 await p.reload();await sl(3000);await p.screenshot({path:O+'chat-done.png',fullPage:false});
 await p.evaluate(()=>{const m=document.getElementById('messages');m.scrollTop=0});await sl(800);await p.screenshot({path:O+'chat-top.png'});
 await p.goto(U+'/app/');await sl(2500);await p.screenshot({path:O+'list-idle.png'});
 const ms=await get(`agents/${bot}/messages`);fs.writeFileSync(O+'messages.json',JSON.stringify(ms,null,1));
 const L=ms.messages||ms;out.cards=L.filter(m=>m.approval).length;out.elapsed=((Date.now()-t0)/60e3).toFixed(1);
 const pc=await get(`agents/${bot}/computer`);out.files=(pc.files||[]).filter(f=>!f.dir).map(f=>f.path+':'+f.size);out.cmds=(pc.log||[]).map(l=>l.cmd.slice(0,80)+' → '+l.exitCode);
 for(const f of (pc.files||[]).filter(f=>!f.dir&&/\.(mp4|py)$/.test(f.path))){const r=await p.request.get(U+`/app/api/agents/${bot}/computer/download?path=${encodeURIComponent(f.path)}`);fs.writeFileSync(O+f.path.replace(/\//g,'_'),await r.body())}
 out.lastReply=(L.filter(m=>m.role==='agent').at(-1)||{}).text;
}finally{out.errs=errs;out.deleted=await post('account/delete',{confirm:'DELETE'});console.log(JSON.stringify(out,null,1));fs.writeFileSync(O+'result.json',JSON.stringify(out,null,1));await b.close()}
