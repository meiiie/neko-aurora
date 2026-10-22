"use strict";
(()=>{
const $=s=>document.querySelector(s);
const SHELLS=["#d9603b","#e0473a","#e5a428","#5b9e3c","#2f9e8f","#2f8fd6","#3b6fe0","#8b5cf6","#c76bd1","#e86a92"];
const HATS=["none","beanie","cap","top","crown"],EYES=["none","round","shades","visor","mono"],EXTRAS=["none","bow","flower","antenna","scarf"];
const VN={none:"Không",beanie:"Mũ len",cap:"Mũ lưỡi trai",top:"Mũ chóp",crown:"Vương miện",round:"Kính tròn",shades:"Kính râm",visor:"Kính visor",mono:"Kính một tròng",bow:"Nơ",flower:"Hoa",antenna:"Ăng-ten",scarf:"Khăn"};
const shade=(h,k)=>{const n=parseInt(h.slice(1),16),f=c=>Math.max(0,Math.min(255,Math.round(c*k)));return`rgb(${f(n>>16)},${f(n>>8&255)},${f(n&255)})`};
// ---------- pixel art (original, drawn in code on a 16x16 grid) ----------
function draw(cv,a,part){const g=cv.getContext("2d");g.clearRect(0,0,16,16);const p=(x,y,w,h,c)=>{g.fillStyle=c;g.fillRect(x,y,w,h)};
 const c=SHELLS[a.shell]||SHELLS[0],d=shade(c,.75),l=shade(c,1.18);
 // body
 p(4,6,8,7,c);p(3,7,10,5,c);p(4,6,8,1,l);p(3,11,10,1,d);p(4,12,8,1,d);p(4,13,2,1,d);p(10,13,2,1,d);
 p(3,5,2,2,c);p(11,5,2,2,c);p(3,5,1,1,l);p(12,5,1,1,l); // ears
 if(part!=="eyes"||a.eyes==="none"){p(5,8,2,2,"#221d1a");p(9,8,2,2,"#221d1a");p(5,8,1,1,"#fff");p(9,8,1,1,"#fff")}
 p(7,10,2,1,"#f3b49a");
 const H=a.hat,K="#2a2320";
 if(H==="beanie"){p(4,3,8,3,"#3b6fe0");p(3,5,10,1,"#2b55b8");p(7,2,2,1,"#f6f1ea")}
 if(H==="cap"){p(4,3,8,3,"#e0473a");p(10,5,4,1,"#b8352b");p(7,3,1,1,"#fff")}
 if(H==="top"){p(5,1,6,4,K);p(5,4,6,1,"#c2410c");p(3,5,10,1,K)}
 if(H==="crown"){p(4,3,8,2,"#e5a428");p(4,2,1,1,"#e5a428");p(7,1,2,2,"#e5a428");p(11,2,1,1,"#e5a428");p(7,3,2,1,"#e0473a")}
 const E=a.eyes;
 if(E==="round"){g.strokeStyle=K;p(4,7,4,1,K);p(8,7,4,1,K);p(4,10,4,1,K);p(8,10,4,1,K);p(4,7,1,4,K);p(11,7,1,4,K);p(7,8,2,1,K);p(5,8,2,2,"#221d1a");p(9,8,2,2,"#221d1a")}
 if(E==="shades"){p(3,7,10,1,K);p(4,8,3,2,K);p(9,8,3,2,K);p(5,8,1,1,"#6b7280");p(10,8,1,1,"#6b7280")}
 if(E==="visor"){p(3,8,10,2,"#2f9e8f");p(4,8,3,1,"#9fe5da")}
 if(E==="mono"){p(5,8,2,2,"#221d1a");p(8,7,4,4,"#e5a428");p(9,8,2,2,"#221d1a");p(11,11,1,2,"#e5a428")}
 const X=a.extra;
 if(X==="bow"){p(10,4,2,2,"#e86a92");p(13,4,2,2,"#e86a92");p(12,5,1,1,"#c24d75")}
 if(X==="flower"){p(2,3,3,3,"#f6f1ea");p(3,4,1,1,"#e5a428")}
 if(X==="antenna"){p(7,1,1,4,K);p(6,0,3,2,"#e0473a")}
 if(X==="scarf"){p(3,11,10,1,"#e5a428");p(10,12,2,3,"#e5a428")}
}
const thumb=(a)=>{const cv=document.createElement("canvas");cv.width=cv.height=16;draw(cv,a);return cv};
// ---------- state ----------
const KEY="neko-duet-v1";const now=Date.now(),H=3600e3,D=24*H;
const seed=()=>({agents:[
 {id:"a1",name:"Morning Brief",tag:"Mỗi sáng",av:{shell:0,hat:"none",eyes:"round",extra:"none"},t:now-2*H,status:"done",snip:"Đã gửi bản tin 5:30. 2 việc cần bạn.",msgs:[{me:0,t:"Chào buổi sáng. 5 việc đang chạy, 2 việc cần bạn: hợp đồng Northfield và giấy phép mở cửa hàng."}]},
 {id:"a2",name:"Posting Buddy",av:{shell:6,hat:"cap",eyes:"shades",extra:"none"},t:now,status:"drafting",snip:"Đang soạn",msgs:[{me:1,t:"Viết giúp mình một bài giới thiệu Neko Bot."},{me:0,t:"Mình đã soạn bản nháp theo giọng của bạn. Đăng giúp không?"},{approval:1,title:"Đăng lên X",body:"Neko Bot: nhiều đồng nghiệp AI, một luồng việc, bạn duyệt trước khi gửi.",state:"pending"}]},
 {id:"a3",name:"Inbox Zero",av:{shell:3,hat:"beanie",eyes:"none",extra:"antenna"},t:now-5*H,status:"waiting",snip:"Có 3 thư cần bạn trả lời trước 18:00.",msgs:[{me:0,t:"Mình đã lưu trữ 83 bản tin, giữ lại 4 thư quan trọng. 3 thư cần bạn trả lời."},{approval:1,title:"Gửi trả lời cho Elena",body:"Chào Elena,\nĐây là deck cho buổi họp thứ Năm kèm link data room.\nDarren",state:"pending"}]},
 {id:"a4",name:"Research",tag:"Thiết kế",av:{shell:7,hat:"top",eyes:"mono",extra:"none"},t:now-D,status:"done",snip:"Bản tóm tắt thứ Ba đã sẵn sàng. Giữ lại 41/960 nguồn.",msgs:[{me:0,t:"Bản tóm tắt nghiên cứu đã xong: 41 nguồn liên quan trên 960."}]},
 {id:"a5",name:"Shop Bot",av:{shell:9,hat:"none",eyes:"none",extra:"bow"},t:now-3*D,status:"paused",snip:"Tạm dừng làm mới listing qua đêm cho tới khi bạn duyệt.",msgs:[{me:0,t:"Mình tạm dừng làm mới listing qua đêm. Tiếp tục không?"}]},
 {id:"a6",name:"Design Buddy",av:{shell:4,hat:"crown",eyes:"visor",extra:"scarf"},t:now-6*D,status:"done",snip:"Đã dựng lại màn cài đặt theo mẫu của bạn.",msgs:[{me:0,t:"Đã dựng lại màn cài đặt."}]}
],log:[{t:now-2*H,x:"Morning Brief đọc lịch, hộp thư và 6 kênh"},{t:now-H,x:"Inbox Zero lưu trữ 83 bản tin"},{t:now,x:"Posting Buddy soạn bản nháp, chờ bạn duyệt"}]});
let S;try{S=JSON.parse(localStorage.getItem(KEY))}catch{}if(!S||!Array.isArray(S.agents))S=seed();
const save=()=>{try{localStorage.setItem(KEY,JSON.stringify(S))}catch{}};
const waiting=a=>a.msgs.some(m=>m.approval&&m.state==="pending");
const ago=t=>{const s=(Date.now()-t)/1e3;if(s<60)return"bây giờ";if(s<3600)return Math.floor(s/60)+" phút";if(s<86400)return new Date(t).toLocaleTimeString("vi-VN",{hour:"2-digit",minute:"2-digit"});return new Date(t).toLocaleDateString("vi-VN",{day:"numeric",month:"numeric"})};
const el=(t,c,x)=>{const e=document.createElement(t);if(c)e.className=c;if(x!=null)e.textContent=x;return e};
let toastT;const toast=m=>{const t=$("#toast");t.textContent=m;t.classList.add("show");clearTimeout(toastT);toastT=setTimeout(()=>t.classList.remove("show"),1800)};
const log=x=>{S.log.push({t:Date.now(),x});S.log=S.log.slice(-60)};
// ---------- list ----------
let filter="";
function renderList(){
 const ags=[...S.agents].sort((a,b)=>(b.pin|0)-(a.pin|0)||(waiting(b)-waiting(a))||b.t-a.t);const w=ags.filter(waiting).length;
 $("#summary").innerHTML="";$("#summary").append(`${ags.length} agent · `);const b=el("b",0,`${w} đang chờ bạn`);$("#summary").append(b);
 const br=$("#brief");br.innerHTML="";br.append(el("h2",0,"Bản tin sáng"),el("p",0,`Chào Darren. ${ags.filter(a=>a.status==="drafting"||a.status==="working").length+3} việc đang chạy, ${w} việc cần bạn.`));
 const ol=el("ol");ags.filter(waiting).forEach(a=>{const li=el("li"),bt=el("button",0,`${a.name}: ${a.msgs.find(m=>m.approval&&m.state==="pending").title}`);bt.onclick=()=>openThread(a.id);li.append(bt);ol.append(li)});if(!w)ol.append(el("li",0,"Không có gì cần bạn lúc này."));br.append(ol);
 const ul=$("#rows");ul.innerHTML="";
 ags.filter(a=>!filter||a.name.toLowerCase().includes(filter)).forEach(a=>{
  const li=el("li","row");li.dataset.id=a.id;const acts=el("div","actions"),pin=el("button","pin",a.pin?"Bỏ ghim":"Ghim"),ar=el("button","arch","Lưu trữ");acts.append(pin,ar);
  pin.onclick=e=>{e.stopPropagation();a.pin=!a.pin;save();renderList()};
  ar.onclick=e=>{e.stopPropagation();S.agents=S.agents.filter(x=>x!==a);log(`Đã lưu trữ ${a.name}`);save();renderList();toast("Đã lưu trữ "+a.name)};
  const inn=el("div","rowin");inn.setAttribute("role","button");inn.tabIndex=0;inn.append(thumb(a.av));const tx=el("div","rowtxt"),h=el("div","rowh");h.append(el("b",0,a.name));if(a.tag)h.append(el("span","tag",a.tag));
  const tm=el("span","time"+(a.status==="drafting"?" now":""),a.status==="drafting"?"bây giờ":ago(a.t));h.append(tm);
  const sn=el("div","snip"+(waiting(a)?" wait":"")+(a.status==="drafting"||a.status==="working"?" dots":""),a.snip);tx.append(h,sn);inn.append(tx);li.append(acts,inn);ul.append(li);
  inn.onclick=()=>{if(performance.now()<suppress)return;if(li.dataset.open){closeRow(li);return}openThread(a.id)};inn.onkeydown=e=>{if(e.key==="Enter")openThread(a.id)};
  swipeRow(li,inn)});
}
let openRow=null,suppress=0;const closeRow=li=>{li.querySelector(".rowin").style.transform="";delete li.dataset.open;if(openRow===li)openRow=null};
function swipeRow(li,inn){let x0,y0,dx=0,mode=null,base=0;
 li.addEventListener("touchstart",e=>{const t=e.touches[0];if(cur){mode="skip";x0=null;return}x0=t.clientX;y0=t.clientY;dx=0;mode=null;base=li.dataset.open?-160:0;if(openRow&&openRow!==li)closeRow(openRow)},{passive:true});
 li.addEventListener("touchmove",e=>{if(x0==null)return;const t=e.touches[0],ddx=t.clientX-x0,ddy=t.clientY-y0;if(!mode){if(Math.abs(ddx)>8&&Math.abs(ddx)>Math.abs(ddy))mode="x";else if(Math.abs(ddy)>8)mode="y"}if(mode!=="x")return;e.preventDefault();dx=ddx;li.classList.add("drag");inn.style.transform=`translate3d(${Math.max(-200,Math.min(0,base+dx))}px,0,0)`},{passive:false});
 li.addEventListener("touchend",()=>{li.classList.remove("drag");if(mode!=="x")return;const p=base+dx;if(p<-70){inn.style.transform="translate3d(-160px,0,0)";li.dataset.open=1;openRow=li}else closeRow(li);
  if(Math.abs(dx)>10)suppress=performance.now()+400});
 li.addEventListener("touchcancel",()=>{li.classList.remove("drag");closeRow(li)});
}
// ---------- thread ----------
let cur=null,busy=false;
function openThread(id){const a=S.agents.find(x=>x.id===id);if(!a)return;cur=a;draw($("#threadAvatar"),a.av);$("#threadName").textContent=a.name;renderThread();$("#thread").classList.remove("off");$("#list").classList.add("under");const hs=history.state;if(hs&&hs.spent)history.replaceState({t:1},"");else if(!(hs&&hs.t))history.pushState({t:1},"")}
function closeThread(fromPop){if(!cur)return;cur=null;$("#thread").classList.add("off");$("#list").classList.remove("under");$("#thread").style.transform="";$("#list").style.transform="";renderList();if(!fromPop&&history.state&&history.state.t)history.replaceState({spent:1},"")}
const stateTxt={drafting:"Đang soạn…",working:"Đang làm việc…",waiting:"Chờ bạn duyệt",paused:"Tạm dừng",done:"Đã xong"};
function renderThread(){const a=cur;if(!a)return;$("#threadState").textContent=waiting(a)?"Chờ bạn duyệt":stateTxt[a.status]||"";const box=$("#messages");box.innerHTML="";
 a.msgs.forEach((m,i)=>{if(m.approval){const c=el("div","approval"+(m.state!=="pending"?" done":""));c.append(el("small",0,"Cần bạn duyệt · không gửi khi chưa có OK"),el("div",0,m.title),el("pre",0,m.body));
  if(m.state==="pending"){const bs=el("div","btns"),no=el("button",0,"Để sau"),go=el("button","go","Duyệt");no.onclick=()=>decide(a,i,"later");go.onclick=()=>decide(a,i,"ok");bs.append(no,go);c.append(bs)}else c.append(el("small",0,m.state==="ok"?"✓ Bạn đã duyệt (bản demo: không gửi ra ngoài)":"Đã hoãn"));box.append(c)}
  else box.append(el("div","msg"+(m.me?" me":""),m.t))});
 requestAnimationFrame(()=>box.scrollTop=box.scrollHeight)}
function decide(a,i,v){const m=a.msgs[i];if(!m||m.state!=="pending")return;m.state=v;a.t=Date.now();a.status=v==="ok"?"done":"paused";a.snip=v==="ok"?"Đã duyệt: "+m.title:"Đã hoãn: "+m.title;a.msgs.push({me:0,t:v==="ok"?"Xong. Mình đã ghi nhận bạn duyệt.":"Ok, mình giữ lại và sẽ nhắc bạn sau."});log(`${a.name}: ${v==="ok"?"bạn duyệt":"bạn hoãn"} “${m.title}”`);save();renderThread()}
function sendMsg(text){const a=cur;if(!a||!text.trim())return;a.msgs.push({me:1,t:text.trim()});a.status="working";a.snip="Đang làm việc";a.t=Date.now();log(`Bạn giao cho ${a.name}: ${text.trim().slice(0,60)}`);save();renderThread();
 const id=a.id;setTimeout(()=>{const b=S.agents.find(x=>x.id===id);if(!b)return;const risky=/gửi|send|đăng|post|trả|pay|thanh toán/i.test(text);
  b.msgs.push({me:0,t:risky?"Mình đã chuẩn bị xong. Việc này đi ra ngoài nên cần bạn duyệt trước.":"Mình nhận việc rồi. Sẽ báo khi xong hoặc khi cần bạn quyết định."});
  if(risky)b.msgs.push({approval:1,title:"Gửi theo yêu cầu",body:text.trim(),state:"pending"});b.status=risky?"waiting":"working";b.snip=risky?"Cần bạn duyệt: Gửi theo yêu cầu":"Đang làm việc";b.t=Date.now();log(`${b.name}: ${risky?"chờ bạn duyệt":"đang làm"}`);save();if(cur===b)renderThread();else renderList()},700)}
// edge-swipe back (iOS style)
(()=>{const th=$("#thread"),ls=$("#list"),root=$("#app");let x0=null,y0,dx=0,on=false;
 root.addEventListener("touchstart",e=>{const t=e.touches[0];if(!cur||sheetOpen||t.clientX>28){x0=null;return}x0=t.clientX;y0=t.clientY;dx=0;on=false},{passive:true});
 root.addEventListener("touchmove",e=>{if(x0==null)return;const t=e.touches[0];dx=Math.max(0,t.clientX-x0);if(!on&&dx>10&&dx>Math.abs(t.clientY-y0))on=true;if(!on)return;e.preventDefault();th.classList.add("drag");ls.classList.add("drag");th.style.transform=`translate3d(${dx}px,0,0)`;ls.style.transform=`translate3d(${-24+24*dx/innerWidth}%,0,0)`},{passive:false});
 const end=()=>{if(x0==null)return;th.classList.remove("drag");ls.classList.remove("drag");if(on&&dx>innerWidth*.33)closeThread();else{th.style.transform="";ls.style.transform=""}x0=null};
 root.addEventListener("touchend",end);root.addEventListener("touchcancel",end)})();
// ---------- creator sheet ----------
let draft;const newDraft=()=>({shell:5,hat:"cap",eyes:"shades",extra:"none"});
function renderPickers(){const P=$("#pickers");P.innerHTML="";draw($("#preview"),draft);
 const g1=el("div","group"),h1=el("h3");h1.append("Màu vỏ",el("span",0,"#"+(draft.shell+1)));const sw=el("div","swatches");
 SHELLS.forEach((c,i)=>{const b=el("button","sw");b.setAttribute("aria-label","Màu "+(i+1));b.setAttribute("aria-pressed",draft.shell===i);const d=el("i");d.style.background=c;b.append(d);b.onclick=()=>{draft.shell=i;renderPickers()};sw.append(b)});g1.append(h1,sw);P.append(g1);
 [["Mũ","hat",HATS],["Kính","eyes",EYES],["Phụ kiện","extra",EXTRAS]].forEach(([lab,k,list])=>{const g=el("div","group"),h=el("h3");h.append(lab,el("span",0,VN[draft[k]]));const o=el("div","opts");
  list.forEach(v=>{const b=el("button","opt"+(v==="none"?" none":""));b.setAttribute("aria-label",VN[v]);b.setAttribute("aria-pressed",draft[k]===v);if(v==="none")b.textContent="⁄";else{const cv=document.createElement("canvas");cv.width=cv.height=16;draw(cv,{...draft,hat:"none",eyes:"none",extra:"none",[k]:v},k);b.append(cv)}b.onclick=()=>{draft[k]=v;renderPickers()};o.append(b)});g.append(h,o);P.append(g)})}
const sheet=$("#sheet"),scrim=$("#scrim");let sheetOpen=null,openedAt=0,ghost=0;
document.addEventListener("click",e=>{if(performance.now()<ghost&&!(sheetOpen&&sheetOpen.contains(e.target))){e.stopImmediatePropagation();e.preventDefault()}},true);
function openSheet(s){if(sheetOpen)return;openedAt=performance.now();sheetOpen=s;scrim.classList.remove("hidden");s.classList.remove("hidden");s.classList.add("closed");s.offsetHeight;s.classList.remove("closed")}
function closeSheet(){const s=sheetOpen;if(!s||performance.now()-openedAt<350)return;sheetOpen=null;ghost=performance.now()+400;s.style.transform="";s.classList.add("closed");scrim.classList.add("hidden");setTimeout(()=>{if(sheetOpen!==s)s.classList.add("hidden")},330)}
$("#newBtn").onclick=()=>{if(sheetOpen)return;draft=newDraft();$("#agentName").value="Posting Buddy";$("#agentDesc").value="Soạn bài theo giọng tôi. Không bao giờ tự đăng.";renderPickers();$("#sheetBody").scrollTop=0;openSheet(sheet)};
$("#cancelBtn").onclick=closeSheet;scrim.onclick=closeSheet;
$("#surpriseBtn").onclick=()=>{const r=l=>l[Math.random()*l.length|0];draft={shell:Math.random()*SHELLS.length|0,hat:r(HATS),eyes:r(EYES),extra:r(EXTRAS)};renderPickers()};
$("#createBtn").onclick=()=>{if(sheetOpen!==sheet)return;const name=$("#agentName").value.trim()||"Agent mới",desc=$("#agentDesc").value.trim();const a={id:"a"+Date.now().toString(36)+Math.random().toString(36).slice(2,5),name,av:{...draft},t:Date.now(),status:"done",snip:desc||"Sẵn sàng nhận việc.",msgs:[{me:0,t:`Chào, mình là ${name}. ${desc} Nhắn mình việc cần làm nhé.`}]};S.agents.unshift(a);log(`Đã tạo agent ${name}`);save();closeSheet();renderList();toast("Đã tạo "+name)};
document.querySelectorAll(".sheet").forEach(s=>{let y0=null,dy=0;const start=e=>{if(e.target.closest(".scroll")&&e.target.closest(".scroll").scrollTop>0)return;y0=e.touches[0].clientY;dy=0};
 s.addEventListener("touchstart",start,{passive:true});
 s.addEventListener("touchmove",e=>{if(y0==null)return;dy=e.touches[0].clientY-y0;if(dy<=0){s.style.transform="";return}if(e.target.closest(".scroll")&&dy<12)return;e.preventDefault();s.classList.add("drag");s.style.transform=`translate3d(0,${dy}px,0)`},{passive:false});
 const end=()=>{if(y0==null)return;s.classList.remove("drag");if(dy>120)closeSheet();else s.style.transform="";y0=null};s.addEventListener("touchend",end);s.addEventListener("touchcancel",end)});
// ---------- activity ----------
$("#activityBtn").onclick=()=>{if(sheetOpen)return;const L=$("#log");L.innerHTML="";[...S.log].reverse().forEach(x=>{const li=el("li"),t=el("time",0,ago(x.t));li.append(t,x.x);L.append(li)});openSheet($("#activity"))};$("#closeActivity").onclick=closeSheet;
// ---------- misc ----------
$("#backBtn").onclick=()=>closeThread();
addEventListener("popstate",()=>{if(sheetOpen)closeSheet();else if(cur)closeThread(true)});
$("#searchBtn").onclick=()=>{const s=$("#search");s.classList.toggle("hidden");if(!s.classList.contains("hidden"))s.focus();else{filter="";s.value="";renderList()}};
$("#search").oninput=e=>{filter=e.target.value.toLowerCase();renderList()};
const inp=$("#input");inp.oninput=()=>{inp.style.height="auto";inp.style.height=Math.min(140,inp.scrollHeight)+"px"};
$("#composer").onsubmit=e=>{e.preventDefault();if(busy)return;busy=true;sendMsg(inp.value);inp.value="";inp.style.height="";setTimeout(()=>busy=false,150)};
inp.onkeydown=e=>{if(e.key==="Enter"&&!e.shiftKey&&!e.isComposing){e.preventDefault();$("#composer").requestSubmit()}};
draw($("#meAvatar"),{shell:2,hat:"none",eyes:"none",extra:"flower"});
renderList();
if("serviceWorker"in navigator&&location.protocol!=="file:")navigator.serviceWorker.register("/duet/sw.js",{scope:"/duet/"}).catch(()=>{});
window.__DUET={state:()=>S,reset:()=>{S=seed();save();renderList()}};
})();
