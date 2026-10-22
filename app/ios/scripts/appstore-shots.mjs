// App Store screenshots: seeds realistic vi content on a throwaway server, captures 6.9" and 6.5".
import {chromium} from 'playwright-core';
const U=process.env.AURORA_URL||'http://127.0.0.1:8812/app/',O=process.env.OUT;const sl=ms=>new Promise(r=>setTimeout(r,ms));
const b=await chromium.launch({executablePath:'/usr/bin/google-chrome',args:['--no-sandbox','--hide-scrollbars']});
const seed=process.env.SEED==='1';
for(const [tag,w,h] of [['6.9',440,956],['6.5',428,926]]){
 const ctx=await b.newContext({viewport:{width:w,height:h},deviceScaleFactor:3,isMobile:true,hasTouch:true,locale:'vi-VN'});
 const p=await ctx.newPage();const errs=[];p.on('pageerror',e=>errs.push(String(e)));
 await p.goto(U);await p.waitForFunction('window.__AURORA');await sl(500);
 if(seed&&tag==='6.9'){await p.evaluate(async()=>{const A=(path,body)=>fetch('/app/api'+path,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)}).then(r=>r.json());
  const bots=[['Phân tích vàng','Theo dõi giá vàng, dẫn nguồn số liệu','Nhận định thị trường vàng tuần này'],['Thư ký Neko','Soạn và trả lời email, chờ bạn duyệt','Trả lời email anh Minh về lịch họp thứ Năm'],['Cùng học tiếng Anh','Luyện phát âm và từ vựng mỗi ngày','Hôm nay luyện phát âm giúp mình'],['Lên kế hoạch','Lịch trình, du lịch, việc cần làm','Lập kế hoạch du lịch Đà Lạt 3 ngày']];
  for(const [name,desc,q] of bots){const a=await A('/agents',{name,desc});await A('/agents/'+a.id+'/messages',{text:q});await new Promise(r=>setTimeout(r,2500))}
  await A('/brief',{})});await sl(6000);await p.reload();await p.waitForFunction('window.__AURORA&&__AURORA.state().agents.length>0');await sl(800)}
 const S=n=>p.screenshot({path:`${O}${tag}-${n}.png`});
 await S('1-home');
 const open=async n=>{const id=await p.evaluate(n=>__AURORA.state().agents.find(a=>a.name===n).id,n);await p.evaluate(id=>location.hash=id,id);await p.reload();await p.waitForFunction('window.__AURORA&&__AURORA.current()');await sl(1200);await p.evaluate(()=>{const m=document.querySelector('#messages');m.scrollTop=m.scrollHeight});await sl(300)};
 await open('Phân tích vàng');await S('2-research');
 await open('Thư ký Neko');await S('3-approval');
 await open('Cùng học tiếng Anh');await S('4-learn');
 await open('Lên kế hoạch');await S('5-plan');
 console.log(tag,'errs',errs);await ctx.close()}
await b.close();
