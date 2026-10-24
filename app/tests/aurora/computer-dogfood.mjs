// Bot computer dogfood (live GLM) against a separate test server.
// AURORA_URL=http://127.0.0.1:8793/app/ OUT=/tmp/pcdog node computer-dogfood.mjs
// A bot writes a CSV and a Python script in its sandbox, asks to run it,
// the owner approves, and the bot reports the result. Screenshots: chat,
// approval card, profile "Máy tính" section.
import {chromium} from 'playwright-core';
const U=process.env.AURORA_URL||'http://127.0.0.1:8793/app/',OUT=process.env.OUT||'/tmp';const sl=ms=>new Promise(r=>setTimeout(r,ms));
const b=await chromium.launch({executablePath:'/usr/bin/google-chrome',args:['--no-sandbox']});
const p=await (await b.newContext({viewport:{width:390,height:844},deviceScaleFactor:2,isMobile:true,hasTouch:true})).newPage();await p.goto(U);await p.waitForFunction(()=>window.__AURORA);
const api=(path,body)=>p.evaluate(async([path,body])=>{const r=await fetch('/app/api/'+path,{method:body?'POST':'GET',headers:{'content-type':'application/json'},body:body?JSON.stringify(body):undefined});const t=await r.text();try{return JSON.parse(t)}catch{return t}},[path,body]);
const t0=Date.now(),T=()=>((Date.now()-t0)/1000).toFixed(1)+'s';
const bot=await api('agents',{name:'Phân tích',desc:'Nhà phân tích dữ liệu, viết và chạy script Python trên máy tính riêng',clientNonce:'pc-'+Date.now()});
const id=bot.id||bot.agent?.id;console.log(T(),'bot',id);
const msgs=()=>api(`agents/${id}/messages`);
const status=async()=>(await api('state')).agents.find(a=>a.id===id);
const idle=async(max=240)=>{let w=false;for(let i=0;i<max*2;i++){await sl(500);const a=await status();if(a.status==='working')w=true;if(w&&a.status!=='working'||!w&&i>20)return a}return null};
const csv='thang,san_pham,so_luong,don_gia\\n2026-07,Cà phê,120,45000\\n2026-07,Trà sữa,80,38000\\n2026-08,Cà phê,150,45000\\n2026-08,Trà sữa,95,38000\\n2026-08,Bánh mì,60,25000\\n2026-09,Cà phê,170,47000\\n2026-09,Trà sữa,90,39000\\n2026-09,Bánh mì,75,25000';
await api(`agents/${id}/messages`,{text:`Trên máy tính của bạn, tạo file sales.csv với nội dung sau (giữ nguyên các dòng):\n${csv.replaceAll('\\n','\n')}\nRồi viết script Python analyze.py (chỉ dùng thư viện chuẩn) tính doanh thu (so_luong*don_gia) theo tháng và theo sản phẩm, tháng tăng trưởng mạnh nhất, rồi chạy nó và báo kết quả cho mình bằng bảng.`,clientNonce:'pc'+Date.now()});
const approved=new Set();
for(let round=0;round<6;round++){
  await idle(300);await sl(1500);
  const pend=(await msgs()).filter(m=>m.approval&&m.approval.state==='pending'&&!approved.has(m.id));
  if(!pend.length)break;
  for(const m of pend){console.log(T(),'approve',JSON.stringify(m.approval.body));
    if(round===0){await p.goto(U+'#'+id);await sl(2500);await p.screenshot({path:OUT+'/pc-1-approval.png'})}
    approved.add(m.id);await api(`approvals/${m.id}`,{decision:'approved'})}
  await sl(3000);
}
await idle(300);await sl(2000);
const all=await msgs();for(const m of all.slice(-6))console.log('---',m.role,(m.text||'').slice(0,900),m.approval?JSON.stringify({kind:m.approval.kind,state:m.approval.state,outcome:m.approval.outcome}):'',(m.tools||[]).map(t=>t.name+':'+t.status).join(' '));
const pc=await api(`agents/${id}/computer`);console.log('computer',JSON.stringify({files:pc.files.map(f=>f.path),log:pc.log.map(r=>[r.cmd,r.exitCode])}));
await p.goto(U+'#'+id);await sl(3000);await p.evaluate(()=>{const s=document.querySelector('#messages')?.closest('.scroll')||document.querySelector('#messages');if(s)s.scrollTop=1e9});await sl(800);await p.screenshot({path:OUT+'/pc-2-result.png'});
await p.click('#whoBtn');await p.waitForSelector('#pcBox .card',{timeout:15000});await p.evaluate(()=>{const b=document.querySelector('#profBody');b.scrollTop=document.querySelector('#pcBox').offsetTop-60});await sl(800);
await p.click('.pc-run summary').catch(()=>{});await sl(600);await p.screenshot({path:OUT+'/pc-3-profile.png'});
await p.evaluate(()=>{const b=document.querySelector('#profBody');b.scrollTop=1e9});await sl(600);await p.screenshot({path:OUT+'/pc-4-files.png'});
await b.close();
