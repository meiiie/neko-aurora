// Live check of the extras: AI & tools sheet, thinking, providers, skills, MCP, bot create, forms, image fallback, browser, folder, memory ask/summarize.
// U=https://… EMAIL=… O=/out/dir/ node extras.mjs
import {chromium} from 'playwright-core';import fs from 'node:fs';
const U=process.env.U,EMAIL=process.env.EMAIL,O=process.env.O;if(!U||U.includes(':8765'))throw 0;fs.mkdirSync(O,{recursive:true});
const sl=ms=>new Promise(r=>setTimeout(r,ms));
const b=await chromium.launch({executablePath:'/usr/bin/google-chrome',args:['--no-sandbox']});
const ctx=await b.newContext({viewport:{width:390,height:844},deviceScaleFactor:2,isMobile:true,hasTouch:true,locale:'vi-VN'});const p=await ctx.newPage();
const errs=[];p.on('pageerror',e=>errs.push(String(e)));const H={'content-type':'application/json',origin:U};
const get=async x=>{const r=await p.request.get(U+'/app/api/'+x,{timeout:60000});return r.ok()?r.json():null};
const post=async(x,d)=>{const r=await p.request.post(U+'/app/api/'+x,{headers:H,data:d});return {s:r.status(),j:await r.json().catch(()=>null)}};
const openT=async id=>{await p.goto(U+'/app/');await sl(2500);const r=await p.$(`#rows .row[data-id="${id}"] .rowin`)||await p.$(`.feat[data-id="${id}"]`);if(r)await r.tap();await sl(2500)};
const shot=n=>p.screenshot({path:O+n+'.png'});
const idle=async(id,max=240e3)=>{const t0=Date.now();await sl(5000);for(;;){const st=await get('state');const a=st.agents.find(x=>x.id===id);const busy=a.status==='working'||(a.members||[]).some(m=>st.agents.find(x=>x.id===m)?.status==='working');if(!busy&&Date.now()-t0>12000||Date.now()-t0>max)return;await sl(4000)}};
let out={};
const lastAgent=async id=>{const ms=await get(`agents/${id}/messages`);return (ms.messages||ms).filter(m=>m.role==='agent').at(-1)||{}};
const msgs=async id=>{const ms=await get(`agents/${id}/messages`);return ms.messages||ms};
const say=async(id,t)=>{const r=await post(`agents/${id}/messages`,{text:t,clientNonce:'x-'+Date.now()+'-'+(Math.random()*1e6|0)});if(r.s>=300)(out.sayErr=out.sayErr||[]).push(r.s);await idle(id)};
const card=async kind=>{for(let i=0;i<10;i++){const L=await msgs(aur);const m=L.filter(x=>x.approval&&x.approval.kind===kind).at(-1);if(m)return m;await sl(3000)}return null};
let aur;
try{
 await p.goto(U+'/app/');await p.waitForURL(/signin/);await p.fill('#email',EMAIL);const rp=p.waitForResponse(r=>r.url().endsWith('/app/auth/email'));await p.tap('#sendBtn');const s=await (await rp).json();
 await p.waitForSelector('#codeStep:not(.hidden)');await p.fill('#code',s.devCode);await p.waitForURL(/welcome/);await p.fill('#name','Darren');await p.tap('[data-goal=work]');await p.tap('#startBtn');
 await p.waitForURL(/\/app\/(#bot-.*)?$/,{timeout:30000,waitUntil:"commit"}).catch(()=>{});for(let i=0;i<30&&!((await get('state'))?.agents||[]).some(a=>a.name==='Aurora');i++)await sl(2000);
 let st=await get('state');aur=st.agents.find(a=>a.name==='Aurora').id;await idle(aur);
 // A) AI & tools sheet: thinking on, MCP server added through the form
 await openT(aur);await p.tap('#whoBtn');await sl(2000);await p.tap('#prExt');await sl(2500);await shot('01-ai-tools');
 await p.tap('#extThink .switch');await sl(1500);
 await p.fill('#mcpName','deepwiki');await p.fill('#mcpUrl','https://mcp.deepwiki.com/mcp');await p.tap('.mcp-add .pill');await sl(9000);
 await p.evaluate(()=>document.querySelector('#mcpName')?.scrollIntoView({block:'center'}));await sl(400);await shot('02-mcp-added');
 out.ext=await get(`agents/${aur}/ext`);
 await p.tap('#closeExt');await sl(600);
 // B) settings: models/providers and skills
 await p.goto(U+'/app/');await sl(2000);await p.tap('#meBtn');await sl(1200);await p.tap('#openModels');await sl(2000);await shot('03-models');
 if((process.env.PROV==="1"&&process.env.ZAI_API_KEY)){const f='#provForm input';const ins=await p.$$(f);await ins[0].fill('zai2');await ins[1].fill('GLM khoá riêng');await ins[2].fill('https://api.z.ai/api/coding/paas/v4');await ins[3].fill('glm-4.5-air');await ins[4].fill((process.env.PROV==="1"&&process.env.ZAI_API_KEY));await p.tap('#provForm .pill');await sl(2500);await shot('04-provider-saved')}
 out.models=await get('models');
 await p.tap('#closeExt');await sl(600);await p.tap('#meBtn').catch(()=>{});await sl(1000);await p.tap('#openSkills');await sl(1500);
 const sf=await p.$$('#skillForm .ext-in');await sf[0].fill('Viết lời chúc');await sf[1].fill('lời chúc sinh nhật ngắn');await sf[2].fill('Viết đúng 2 câu, ấm áp, có tên người nhận, kết bằng một emoji 🎉.');await p.tap('#skillForm .pill');await sl(1500);await shot('05-skills');
 await p.tap('#closeExt');await sl(600);
 // C) thinking shown folded
 await say(aur,'17 × 23 bằng bao nhiêu? Trả lời một dòng.');out.think=(await lastAgent(aur));
 await openT(aur);await p.evaluate(()=>document.querySelectorAll('details.think').forEach(d=>d.open=true));await sl(500);await shot('06-thinking');
 // D) custom provider model
 if((process.env.PROV==="1"&&process.env.ZAI_API_KEY)){out.setModel=(await post(`agents/${aur}/ext`,{model:'zai2:glm-4.5-air',thinking:false})).s;await say(aur,'Bạn đang chạy mô hình nào? Trả lời đúng 1 câu ngắn.');out.provReply=(await lastAgent(aur)).text;await post(`agents/${aur}/ext`,{model:''})}
 // E) skill use
 await say(aur,'Viết lời chúc sinh nhật cho chị Lan.');out.skill=(await msgs(aur)).at(-1);
 // F) MCP
 await say(aur,'Dùng máy chủ deepwiki hỏi repo facebook/react: React dùng cơ chế gì để lên lịch render? Trả lời 2 câu.');
 let mc=await card('mcp');out.mcpCard=mc&&mc.approval;out.mcpTools=(await msgs(aur)).flatMap(m=>m.tools||[]).filter(t=>t.name==='mcp');
 await openT(aur);await shot('07-mcp');
 if(mc&&mc.approval.state==='pending'){const btn=await p.$(`.approval[data-id="${mc.id}"] button.go`)||await p.$('.approval .pill:not(.soft)');if(btn)await btn.tap();else await post('approvals/'+mc.taskId,{decision:'approved'});await sl(4000);await idle(aur);await openT(aur);await shot('08-mcp-done')}
 out.mcpReply=(await lastAgent(aur)).text;
 // G) bot creation with confirmation card
 const n0=(await get('state')).agents.length;await say(aur,'Tạo giúp mình một bot tên Kế toán chuyên ghi sổ chi tiêu hằng ngày.');const bc=await card('bot');out.botCard=bc&&bc.approval;
 await openT(aur);await shot('09-bot-card');out.botsBefore=(await get('state')).agents.length===n0;
 if(bc){const btn=await p.$(`.approval[data-id="${bc.id}"] button.go`);if(btn)await btn.tap();else await post('approvals/'+bc.taskId,{decision:'approved'});await sl(3000)}
 out.botCreated=(await get('state')).agents.some(a=>a.name.startsWith('Kế toán'));await openT(aur);await shot('10-bot-created');
 // H) form card
 await say(aur,'Mình muốn đặt một cuộc họp. Gửi mình một form để điền: tiêu đề, ngày, giờ, hình thức (online hoặc offline).');out.form=(await msgs(aur)).filter(m=>m.form).at(-1)?.form;
 await openT(aur);await p.evaluate(()=>document.querySelector('.form-card')?.scrollIntoView({block:'center'}));await sl(500);await shot('11-form');
 const fc=await p.$('.form-card');if(fc){const ins=await fc.$$('input[type=text],textarea');const vals=['Họp kế hoạch Q4','9:30','Phòng họp tầng 3'];for(let k=0;k<ins.length;k++)await ins[k].fill(vals[k]||'ok');const nums=await fc.$$('input[type=number]');for(const n of nums)await n.fill('5');const d=await fc.$('input[type=date]');if(d)await d.fill('2026-10-30');await (await fc.$('button[type=submit]')).tap();await sl(1500);await idle(aur);await openT(aur);await shot('12-form-sent');out.formAfter=(await lastAgent(aur)).text}
 // I) image → vision fallback
 if(process.env.IMG){await openT(aur);await p.tap('#plusBtn');await sl(400);const fcp=p.waitForEvent('filechooser');await p.tap('#pmImg');const ch=await fcp;await ch.setFiles(process.env.IMG);await sl(3000);await shot('13-image-attached');await p.tap('#composer .send').catch(()=>{});await sl(2000);await idle(aur);await openT(aur);await shot('14-image-reply');out.img=(await lastAgent(aur)).text;out.imgTools=(await msgs(aur)).flatMap(m=>m.tools||[]).filter(t=>t.name==='image_describe')}
 // J) browser
 await say(aur,'Mở bằng trình duyệt trang https://example.com rồi cho mình biết tiêu đề trang.');out.browser=(await msgs(aur)).flatMap(m=>m.tools||[]).filter(t=>t.name==='browser_open');out.browserReply=(await lastAgent(aur)).text;
 // K) working folder + recall
 await post(`agents/${aur}/ext`,{folder:'du-an'});await say(aur,'Ghi file ghi-chu.txt với nội dung "xin chao" (không cần chạy lệnh).');out.files=(await get(`agents/${aur}/computer`))?.files?.map(f=>f.path);
 await post(`agents/${aur}/memory`,{text:'Darren thích bảng ngắn'});await post(`agents/${aur}/memory`,{text:'Darren thích bảng so sánh ngắn gọn'});await post(`agents/${aur}/memory`,{text:'Darren làm việc buổi sáng'});
 await openT(aur);await p.tap('#whoBtn');await sl(2000);await p.tap('#prMem');await sl(2000);await p.tap('#memSum');await sl(15000);await shot('15-memory-summarize');
 await p.tap('#memAsk');await sl(400);await p.fill('.mem-q input','Mình thích trình bày thế nào?');await p.tap('.mem-q .pill');await sl(12000);await shot('16-memory-ask');out.memAns=await p.evaluate(()=>document.querySelector('.mem-ans')?.textContent);
}catch(e){out.error=String(e);await shot('zz-error').catch(()=>{})}
finally{out.errs=errs;out.tail=aur?(await msgs(aur).catch(()=>[])).slice(-8).map(m=>({role:m.role,text:(m.text||'').slice(0,120),form:!!m.form,ap:m.approval&&m.approval.kind,queued:m.queued})):null;out.state=aur?(await get('state')).agents.find(a=>a.id===aur):null;out.usage=await get('usage?range=1').catch(()=>null);out.deleted=(await post('account/delete',{confirm:'DELETE'})).s;fs.writeFileSync(O+'result.json',JSON.stringify(out,null,1));console.log(JSON.stringify(out).slice(0,300));await b.close()}
