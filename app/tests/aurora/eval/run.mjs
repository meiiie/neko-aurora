// Scored eval for the Aurora harness against a live GLM test server.
// AURORA_URL=http://127.0.0.1:8832/app/ node run.mjs   (never 8765)
import {chromium} from 'playwright-core';import fs from 'fs';
const U=process.env.AURORA_URL||'http://127.0.0.1:8832/app/';if(U.includes(':8765'))throw new Error('refusing to run against the live server');
const tasks=JSON.parse(fs.readFileSync(new URL('./tasks.json',import.meta.url)));const only=process.env.ONLY?.split(',');
const sl=ms=>new Promise(r=>setTimeout(r,ms));
const b=await chromium.launch({executablePath:'/usr/bin/google-chrome',args:['--no-sandbox']});
const p=await (await b.newContext()).newPage();await p.goto(U);await p.waitForFunction(()=>window.__AURORA);
const api=(path,body)=>p.evaluate(async([path,body])=>{const r=await fetch('/app/api/'+path,{method:body?'POST':'GET',headers:{'content-type':'application/json'},body:body?JSON.stringify(body):undefined});const t=await r.text();try{return JSON.parse(t)}catch{return t}},[path,body]);
let k=0;const bots={};
const botFor=async t=>{const key=t.after||t.id;if(!bots[key]){const r=await api('agents',{name:'Eval '+key.slice(0,20),desc:'Trợ lý nghiên cứu, so sánh giá, soạn thư, viết báo cáo',clientNonce:'ev'+Date.now()+'n'+(k++)});bots[key]=r.id||r.agent?.id}return bots[key]};
const status=async id=>(await api('state')).agents.find(a=>a.id===id);
const idle=async id=>{let w=false;for(let i=0;i<900;i++){await sl(500);const a=await status(id);if(a.status==='working')w=true;if(w&&a.status!=='working'||!w&&i>20)return}};
const INTERNAL=/web_search|web_fetch|gold_price|TOOL:|PLAN:|MEMORY:/,HAN=/[\u3400-\u9fff]/,THIRD=/\bDarren\b/;
const rows=[];
for(const t of tasks){if(only&&!only.includes(t.id))continue;
 const id=await botFor(t);const n0=(await api(`agents/${id}/messages`)).length;const t0=Date.now();
 await api(`agents/${id}/messages`,{text:t.q,clientNonce:'ev'+Date.now()+'n'+(k++)});
 if(t.steer){await sl(1500);await api(`agents/${id}/messages`,{text:t.steer,clientNonce:'ev'+Date.now()+'n'+(k++)})}
 await idle(id);if(t.steer){await sl(1500);await idle(id)}
 const ms=(await api(`agents/${id}/messages`)).slice(n0);const agent=ms.filter(m=>m.role==='agent');
 const text=agent.map(m=>m.text||'').join('\n');const tools=[...new Set(agent.flatMap(m=>(m.tools||[]).map(x=>x.name)))];
 const mem=t.memory?(await api(`agents/${id}/memory`)).items||[]:[];
 const checks={
  answered:text.trim().length>0,
  vietnamese:!HAN.test(text),
  noInternal:!INTERNAL.test(text),
  addressesBan:!THIRD.test(text),
  tools:t.tools===null||t.tools.every(x=>tools.includes(x))&&(t.tools.length>0||tools.length===0),
  cites:!t.cite||/\[\d+\]/.test(text),
  must:(t.must||[]).every(x=>text.toLowerCase().includes(x.toLowerCase())),
  approval:!t.approval||agent.some(m=>m.approval&&m.approval.state==='pending'),
  secret:!t.secret||agent.some(m=>m.secret),
  plan:!t.plan||agent.some(m=>m.plan),
  memory:!t.memory||mem.length>0,
  steerOrder:!t.steer||(()=>{const i=ms.findIndex(m=>m.role==='user'&&m.text===t.steer);const fin=ms.findIndex(m=>m.role==='agent'&&/\bAI\b/.test(m.text||'')&&(m.sources||[]).length);return i>=0&&fin>i})()
 };
 const pass=Object.values(checks).filter(Boolean).length,total=Object.keys(checks).length;
 const row={id:t.id,score:+(pass/total).toFixed(2),secs:+((Date.now()-t0)/1000).toFixed(1),failed:Object.entries(checks).filter(([,v])=>!v).map(([k])=>k),tools};rows.push(row);console.log(JSON.stringify(row));
}
const avg=rows.reduce((a,r)=>a+r.score,0)/rows.length;const full=rows.filter(r=>r.score===1).length;
console.log(`EVAL ${full}/${rows.length} tasks fully passed · mean score ${avg.toFixed(3)} · mean ${(rows.reduce((a,r)=>a+r.secs,0)/rows.length).toFixed(1)}s`);
fs.writeFileSync(process.env.OUT||'/tmp/aurora-eval.json',JSON.stringify({at:new Date().toISOString(),rows,avg},null,1));await b.close();
