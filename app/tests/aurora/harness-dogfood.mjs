// Harness dogfood (multi-round, live GLM) against a separate test server.
// AURORA_URL=http://127.0.0.1:8831/app/ node harness-dogfood.mjs
// Checks: parallel lookups, memory directive + recall, mid-turn steering,
// long chat compaction, a step plan with milestones, and Stop.
import {chromium} from 'playwright-core';
const U=process.env.AURORA_URL||'http://127.0.0.1:8831/app/';const sl=ms=>new Promise(r=>setTimeout(r,ms));
const b=await chromium.launch({executablePath:'/usr/bin/google-chrome',args:['--no-sandbox']});
const p=await (await b.newContext()).newPage();await p.goto(U);await p.waitForFunction(()=>window.__AURORA);
const api=(path,body,method)=>p.evaluate(async([path,body,method])=>{const r=await fetch('/app/api/'+path,{method:method||(body?'POST':'GET'),headers:{'content-type':'application/json'},body:body?JSON.stringify(body):undefined});const t=await r.text();try{return JSON.parse(t)}catch{return t}},[path,body,method]);
const t0=Date.now();const T=()=>((Date.now()-t0)/1000).toFixed(1)+'s';const res={};
const bot=await api('agents',{name:'Harness test',desc:'Trợ lý nghiên cứu, so sánh giá, viết báo cáo ngắn',clientNonce:'hd-'+Date.now()});
const id=bot.id||bot.agent?.id;console.log(T(),'bot',id);
const msgs=()=>api(`agents/${id}/messages`);
let k=0;const send=async t=>{const r=await api(`agents/${id}/messages`,{text:t,clientNonce:'hd'+Date.now()+'n'+(k++)});if(typeof r==='string')console.log('send failed',r);return r};
const status=async()=>(await api('state')).agents.find(a=>a.id===id);
const idle=async(max=240)=>{let w=false;for(let i=0;i<max*2;i++){await sl(500);const a=await status();if(a.status==='working')w=true;if(w&&a.status!=='working'||!w&&i>20)return a}return null};
const last=async()=>(await msgs()).filter(m=>m.role!=='user').at(-1);
const ONLY=process.env.ONLY;const on=n=>!ONLY||ONLY===n;
if(on('memory')){
// 1 memory
await send('Ghi nhớ giúp mình: mình luôn muốn câu trả lời có một bảng tóm tắt ngắn ở cuối, và mình ở Hải Phòng.');await idle();
res.memory=await api(`agents/${id}/memory`);console.log(T(),'memory',JSON.stringify(res.memory));
}
if(on('parallel')){
await send('Tra giúp mình giá vàng SJC hôm nay, tỷ giá USD/VND và giá iPhone 17 Pro tại Việt Nam, rồi so sánh ngắn.');
let doing=new Set();for(let i=0;i<480;i++){await sl(500);const a=await status();if(a.doing)doing.add(a.doing);if(a.status!=='working'&&i>2)break}
var m=await last();res.parallel={tools:(m.tools||[]).map(t=>t.name+':'+t.status+':'+t.ms),sources:(m.sources||[]).length,doing:[...doing],table:/\|.*\|/.test(m.text),text:m.text.slice(0,700)};console.log(T(),'parallel',JSON.stringify(res.parallel));
}
if(on('steer')){
await send('Tìm 3 tin tức công nghệ nổi bật tuần này ở Việt Nam và tóm tắt.');await sl(+(process.env.STEER_MS||4000));
await send('À đổi ý: chỉ lấy tin về AI thôi nhé.');await idle();await sl(1500);await idle();
const all=await msgs();res.steer=all.slice(-4).map(x=>x.role+': '+x.text.slice(0,300));console.log(T(),'steer',JSON.stringify(res.steer,null,1));
}
if(on('plan')){
await send('Làm giúp mình: (1) tra giá vàng thế giới hôm nay (2) tra giá vàng SJC (3) viết nhận định ngắn cho người mua ở Hải Phòng.');
let ds=new Set();for(let i=0;i<900;i++){await sl(500);const a=await status();if(a.doing)ds.add(a.doing);if(a.status!=='working'&&i>4)break}
var m=await last();res.plan={doing:[...ds],steps:(m.plan?.steps||[]).map(s=>s.status),text:m.text.slice(0,500)};console.log(T(),'plan',JSON.stringify(res.plan));
}
if(on('recall')){
await send('Mình ở đâu nhỉ, và bạn nhớ mình thích trình bày thế nào?');await idle();var m=await last();res.recall=m.text.slice(0,400);console.log(T(),'recall',res.recall);
}
if(on('stop')){
await send('Viết một báo cáo rất dài 2000 chữ về lịch sử thị trường vàng Việt Nam từ 1990.');await sl(6000);
const s1=await api(`agents/${id}/stop`,{});await idle(60);res.stop={resp:s1,status:(await status()).status};console.log(T(),'stop',JSON.stringify(res.stop));
}
console.log('RESULT',JSON.stringify(res));await b.close();
