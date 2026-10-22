// UX before/after shots at 390 on a seeded throwaway server (8812).
import {chromium} from 'playwright-core';
const U='http://127.0.0.1:8812/app/',O=process.env.OUT,PH=process.env.PH||'before';const sl=ms=>new Promise(r=>setTimeout(r,ms));
const b=await chromium.launch({executablePath:'/usr/bin/google-chrome',args:['--no-sandbox','--hide-scrollbars']});
const ctx=await b.newContext({viewport:{width:390,height:844},deviceScaleFactor:2,isMobile:true,hasTouch:true,locale:'vi-VN',...(process.env.VIDEO?{recordVideo:{dir:O+'raw/',size:{width:390,height:844}}}:{})});
const p=await ctx.newPage();const errs=[];p.on('pageerror',e=>errs.push(String(e)));
await p.addInitScript(()=>{try{localStorage.setItem('neko-aurora-lang','vi')}catch{}});
await p.goto(U);await p.waitForFunction(()=>window.__AURORA);await sl(500);
if(process.env.SEED){await p.evaluate(async()=>{const A=(u,x)=>fetch('/app/api'+u,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(x)}).then(r=>r.json());
 const bots=[['Phân tích vàng','Giá vàng hôm nay thế nào?',1],['Thư ký Neko','Trả lời email anh Minh về lịch họp thứ Năm',1],['Cùng học tiếng Anh','Hôm nay luyện phát âm giúp mình',1],['Lên kế hoạch','Lập kế hoạch du lịch Đà Lạt 3 ngày',1],['Ra mắt iOS','Kế hoạch ra mắt bản iOS',0],['Sổ sách','Tóm tắt chi tiêu tháng này',0],['Quản gia','Nhắc mình uống nước',0],['Viết bài','Viết bài giới thiệu sản phẩm',0]];
 for(const [name,q,pin] of bots){const a=await A('/agents',{name,desc:''});if(pin)await A('/agents/'+a.id,{pin:true});await A('/agents/'+a.id+'/messages',{text:q});await new Promise(r=>setTimeout(r,1800))}});await sl(4000)}
await p.reload();await p.waitForFunction(()=>window.__AURORA&&__AURORA.state().agents.length>3);await sl(900);
const S=n=>p.screenshot({path:`${O}${PH}-${n}.png`});await S('1-home');
const id=await p.evaluate(()=>__AURORA.state().agents.find(a=>a.name==='Ra mắt iOS').id);
if(PH!=='before'){const bb=await p.locator(`.row[data-id="${id}"]`).boundingBox();const c=await p.context().newCDPSession(p);const x=bb.x+bb.width/2,y=bb.y+bb.height/2;
 await c.send('Input.dispatchTouchEvent',{type:'touchStart',touchPoints:[{x,y}]});await sl(700);await c.send('Input.dispatchTouchEvent',{type:'touchEnd',touchPoints:[]});await sl(500);await S('4-context');
 if(await p.locator('#ctxMore').count()){await p.locator('#ctxMore').tap();await sl(450);await S('5-submenu')}
 await p.keyboard.press('Escape');await sl(400)}
await p.evaluate(id=>location.hash=id,id);await p.reload();await p.waitForFunction(()=>window.__AURORA&&__AURORA.current());await sl(1000);
await p.evaluate(()=>{const m=document.querySelector('#messages');m.scrollTop=0});await sl(200);await S('2-markdown-top');
await p.evaluate(()=>{const m=document.querySelector('#messages');m.scrollTop=m.scrollHeight});await sl(200);await S('3-markdown-bottom');
console.log('errs',errs);if(process.env.VIDEO){const v=p.video();await ctx.close();console.log('VIDEO',await v.path())}await b.close();
