// Hard real task as a user: 6-min Vietnamese pptx on Hg→Au transmutation.
// Acts only as the user (approve, pick choices, nudge). Saves everything.
import {chromium} from 'playwright-core';import fs from 'node:fs';import {execSync} from 'node:child_process';
const U=process.env.U,EMAIL=process.env.EMAIL,O=process.env.O||'/workspace/neko-bot-shots/aurora/pptx-task/';if(!U||U.includes(':8765'))throw 0;
const sl=ms=>new Promise(r=>setTimeout(r,ms));const log=(...a)=>{const s=a.map(x=>typeof x==='string'?x:JSON.stringify(x)).join(' ');console.log(s);fs.appendFileSync(O+'transcript.log',s+'\n')};
const credit=()=>{const m=execSync('railway usage 2>/dev/null',{cwd:'/workspace/neko-computer'}).toString().match(/Current usage:\s+\$([\d.]+)/);return m?2.44-+m[1]:9};
const b=await chromium.launch({executablePath:'/usr/bin/google-chrome',args:['--no-sandbox']});
const ctx=await b.newContext({viewport:{width:390,height:844},deviceScaleFactor:2,isMobile:true,hasTouch:true,locale:'vi-VN',acceptDownloads:true});
const p=await ctx.newPage();const H={'content-type':'application/json',origin:U};
const get=async x=>{const r=await p.request.get(U+'/app/api/'+x);return r.ok()?r.json():null};
const post=async(x,d)=>(await p.request.post(U+'/app/api/'+x,{headers:H,data:d})).status();
let bot,shot=0;const msgs=async()=>{const r=await get(`agents/${bot}/messages`);return Array.isArray(r)?r:[]};
const snap=async n=>{await p.evaluate(id=>location.hash=id,bot);await p.reload();await sl(1500);await p.evaluate(()=>{const m=document.querySelector('#messages');if(m)m.scrollTop=m.scrollHeight});await sl(500);await p.screenshot({path:O+`${String(++shot).padStart(2,'0')}-${n}.png`})};
const idle=async(max=600)=>{let w=false;for(let i=0;i<max/2;i++){await sl(2000);const a=(await get('state'))?.agents.find(x=>x.id===bot);if(!a)continue;if(a.status==='working')w=true;if(i%15==0)log('  …',a.status,a.doing||'');if(w&&a.status!=='working'||!w&&i>10)return a}};
const PROMPT=`Mình cần một bài thuyết trình 6 phút bằng tiếng Việt về chủ đề "Biến thuỷ ngân thành vàng" (chuyển hoá hạt nhân Hg→Au): từ giả kim thuật lịch sử, thí nghiệm 1941 của Sherr, Bainbridge và Anderson, đến đề xuất của Marathon Fusion năm 2025.
Yêu cầu:
1. File .pptx thật, chỉnh sửa được (mỗi thành phần là shape/text riêng, không dán ảnh), phong cách trắng – xanh dương.
2. Sơ đồ kiến trúc/quy trình (vd. Hg-198 + neutron → Hg-197 → Au-197) dựng thủ công bằng shape và có animation; không dùng ảnh AI kiểu sách cổ, cái cân.
3. Kịch bản nói tự nhiên bằng tiếng Việt theo phong cách các nhà vô địch hùng biện thế giới, chia theo từng slide, tổng ~6 phút.
4. Nghiên cứu có nguồn trích dẫn.
5. Có thể tham khảo: https://github.com/meiiie/meiiie_skillls/tree/main/motion-pptx-studio , https://skills.wheresryan.sh/anatomy , https://manimgx.academa.ai/
6. Cuối cùng quay một video xem trước để mình duyệt.
Dùng máy tính của bạn để làm file (python-pptx), xong gửi link tải file cho mình.`;
try{
 await p.goto(U+'/app/');await p.waitForURL(/signin/);await p.fill('#email',EMAIL);const rp=p.waitForResponse(r=>r.url().endsWith('/app/auth/email'));await p.tap('#sendBtn');const s=await (await rp).json();
 await p.waitForSelector('#codeStep:not(.hidden)');await p.fill('#code',s.devCode);await p.waitForURL(/welcome/);await p.fill('#name','Darren');await p.tap('[data-goal=research]');await p.tap('#startBtn');await p.waitForURL(/#bot-/,{timeout:30000});
 bot=(await get('state')).agents.find(a=>a.name==='Aurora').id;await idle(120);
 let seen=(await msgs()).length;const t0=Date.now();
 await post(`agents/${bot}/messages`,{text:PROMPT,clientNonce:'pptx-'+Date.now()});log('USER:',PROMPT.slice(0,80));
 const nudges=['Tiếp tục làm file .pptx trên máy tính của bạn nhé (cài python-pptx nếu cần), xong gửi link tải.','Giờ dựng video xem trước (vd. render slide ra ảnh rồi ghép mp4 bằng ffmpeg nếu có) và gửi link tải.','Gửi mình link tải file pptx, kịch bản và video nhé.'];let ni=0;
 for(let round=0;round<(+process.env.ROUNDS||8);round++){
  if(credit()<1.2){log('STOP: credit < $1.2');break}
  await idle(600);const L=await msgs();const nw=L.slice(seen);seen=L.length;
  for(const m of nw)log(`[r${round}] ${m.role}:`,(m.text||'').slice(0,600).replace(/\n/g,' ⏎ '),m.approval?`APPROVAL(${m.approval.state}): ${(m.approval.body||m.approval.title||'').slice(0,300)}`:'',(m.tools||[]).map(t=>t.name+':'+t.status).join(','),(m.sources||[]).length?`sources=${m.sources.length}`:'');
  await snap('round'+round);
  const ap=L.find(m=>m.approval&&m.approval.state==='pending');
  if(ap){log('USER: approve');await post(`approvals/${ap.taskId||ap.id}`,{decision:'approved'});continue}
  const ch=nw.filter(m=>m.choices&&m.choices.length).at(-1);
  if(ch){log('USER choice:',ch.choices[0]);await post(`agents/${bot}/messages`,{text:ch.choices[0],clientNonce:'pc-'+Date.now()});continue}
  const pc=await get(`agents/${bot}/computer`);const files=(pc?.files||[]).map(f=>f.path);log('files:',files);
  const havePptx=files.some(f=>/\.pptx$/.test(f)),haveVid=files.some(f=>/\.(mp4|webm|gif)$/.test(f));
  if(havePptx&&haveVid)break;
  if(ni>=nudges.length)break;
  const n=havePptx?nudges[1]:nudges[ni===1?2:0];ni++;log('USER nudge:',n);await post(`agents/${bot}/messages`,{text:n,clientNonce:'pn-'+Date.now()});
 }
 log('elapsed min',((Date.now()-t0)/60000).toFixed(1));
 const pc=await get(`agents/${bot}/computer`);log('final files:',(pc?.files||[]).map(f=>f.path+':'+f.size));log('cmd log:',(pc?.log||[]).map(x=>x.cmd.slice(0,120)+' → '+x.exitCode));
 for(const f of (pc?.files||[]).filter(f=>!f.dir&&/\.(pptx|mp4|webm|gif|md|txt|py|pdf|png)$/.test(f.path))){const r=await p.request.get(U+`/app/api/agents/${bot}/computer/download?path=${encodeURIComponent(f.path)}`);if(r.ok()){fs.writeFileSync(O+'out-'+f.path.replace(/\//g,'_'),await r.body());log('saved',f.path,r.status())}else log('download failed',f.path,r.status())}
 const all=await msgs();fs.writeFileSync(O+'messages.json',JSON.stringify(all,null,1));
}finally{const d=await post('account/delete',{confirm:'DELETE'});log('account deleted',d);await b.close()}
