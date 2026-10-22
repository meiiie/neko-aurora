"use strict";
(()=>{
const $=s=>document.querySelector(s);
const SHELLS=["#e0261f","#e2827c","#eda12c","#f7b551","#96590a","#2c7f52","#74cfc6","#506c62","#8b8496","#e8bd63"]; // avatar shell colours, brand-derived (design tokens live in /app/aurora/aurora.css)
const HATS=["none","beanie","cap","top","crown"],EYES=["none","round","shades","visor","mono"],EXTRAS=["none","bow","flower","antenna","scarf"];
const VN={none:"Không",beanie:"Mũ len",cap:"Mũ lưỡi trai",top:"Mũ chóp",crown:"Vương miện",round:"Kính tròn",shades:"Kính râm",visor:"Kính visor",mono:"Kính một tròng",bow:"Nơ",flower:"Hoa",antenna:"Ăng-ten",scarf:"Khăn"};
const shade=(h,k)=>{const n=parseInt(h.slice(1),16),f=c=>Math.max(0,Math.min(255,Math.round(c*k)));return`rgb(${f(n>>16)},${f(n>>8&255)},${f(n&255)})`};
// ---------- pixel art (original, drawn in code on a 16x16 grid) ----------
function draw(cv,a,part){const g=cv.getContext("2d");g.clearRect(0,0,16,16);const p=(x,y,w,h,c)=>{g.fillStyle=c;g.fillRect(x,y,w,h)};
 const c=SHELLS[a.shell]||SHELLS[0],d=shade(c,.75),l=shade(c,1.18);
 // body
 p(4,6,8,7,c);p(3,7,10,5,c);p(4,6,8,1,l);p(3,11,10,1,d);p(4,12,8,1,d);p(4,13,2,1,d);p(10,13,2,1,d);
 p(3,5,2,2,c);p(11,5,2,2,c);p(3,5,1,1,l);p(12,5,1,1,l); // ears
 if(part!=="eyes"||a.eyes==="none"){p(5,8,2,2,"#17130e");p(9,8,2,2,"#17130e");p(5,8,1,1,"#fff");p(9,8,1,1,"#fff")}
 p(7,10,2,1,"#f3b49a");
 const H=a.hat,K="#2a2320";
 if(H==="beanie"){p(4,3,8,3,"#3b6fe0");p(3,5,10,1,"#2b55b8");p(7,2,2,1,"#f6f1ea")}
 if(H==="cap"){p(4,3,8,3,"#e0473a");p(10,5,4,1,"#b8352b");p(7,3,1,1,"#fff")}
 if(H==="top"){p(5,1,6,4,K);p(5,4,6,1,"#c2410c");p(3,5,10,1,K)}
 if(H==="crown"){p(4,3,8,2,"#e5a428");p(4,2,1,1,"#e5a428");p(7,1,2,2,"#e5a428");p(11,2,1,1,"#e5a428");p(7,3,2,1,"#e0473a")}
 const E=a.eyes;
 if(E==="round"){g.strokeStyle=K;p(4,7,4,1,K);p(8,7,4,1,K);p(4,10,4,1,K);p(8,10,4,1,K);p(4,7,1,4,K);p(11,7,1,4,K);p(7,8,2,1,K);p(5,8,2,2,"#17130e");p(9,8,2,2,"#17130e")}
 if(E==="shades"){p(3,7,10,1,K);p(4,8,3,2,K);p(9,8,3,2,K);p(5,8,1,1,"#6b7280");p(10,8,1,1,"#6b7280")}
 if(E==="visor"){p(3,8,10,2,"#2f9e8f");p(4,8,3,1,"#9fe5da")}
 if(E==="mono"){p(5,8,2,2,"#17130e");p(8,7,4,4,"#e5a428");p(9,8,2,2,"#17130e");p(11,11,1,2,"#e5a428")}
 const X=a.extra;
 if(X==="bow"){p(10,4,2,2,"#e86a92");p(13,4,2,2,"#e86a92");p(12,5,1,1,"#c24d75")}
 if(X==="flower"){p(2,3,3,3,"#f7f5eb");p(3,4,1,1,"#e5a428")}
 if(X==="antenna"){p(7,1,1,4,K);p(6,0,3,2,"#e0473a")}
 if(X==="scarf"){p(3,11,10,1,"#e0261f");p(10,12,2,3,"#e0261f")}
}
const thumb=(a)=>{const cv=document.createElement("canvas");cv.width=cv.height=16;draw(cv,a);return cv};
// ---------- server state (real Go control plane at /app/api) ----------
const S={agents:[],pending:[],brief:null,ai:{},vapid:"",msgs:{},drafts:{}};
const API="/app/api";
async function api(path,body){const r=await fetch(API+path,{method:body===undefined?"GET":"POST",credentials:"same-origin",cache:"no-store",headers:body===undefined?{}:{"Content-Type":"application/json"},body:body===undefined?undefined:JSON.stringify(body)});
 if(!r.ok){const e=new Error(String(r.status));e.status=r.status;throw e}return r.json()}
const T=x=>new Date(x).getTime();
const waiting=a=>S.pending.some(m=>m.agentId===a.id)||a.status==="waiting";
const ago=t=>{t=T(t);const s=(Date.now()-t)/1e3;if(s<60)return"bây giờ";if(s<3600)return Math.floor(s/60)+" phút";if(s<86400&&new Date(t).getDate()===new Date().getDate())return new Date(t).toLocaleTimeString("vi-VN",{hour:"2-digit",minute:"2-digit"});return new Date(t).toLocaleDateString("vi-VN",{day:"numeric",month:"numeric"})};
const el=(t,c,x)=>{const e=document.createElement(t);if(c)e.className=c;if(x!=null)e.textContent=x;return e};
let toastT;const toast=m=>{const t=$("#toast");t.textContent=m;t.classList.add("show");clearTimeout(toastT);toastT=setTimeout(()=>t.classList.remove("show"),2200)};
const upsert=a=>{const i=S.agents.findIndex(x=>x.id===a.id);if(i<0)S.agents.push(a);else S.agents[i]=a};
let listQueued=false;const renderListSoon=()=>{if(listQueued)return;listQueued=true;requestAnimationFrame(()=>{listQueued=false;renderList()})};
async function loadState(){const st=await api("/state");S.agents=st.agents;S.pending=st.pending;S.brief=st.brief;S.ai=st.ai;S.vapid=st.vapidPublicKey;renderList();if(cur){const a=S.agents.find(x=>x.id===cur.id);if(a){cur=a;await loadMsgs(a.id)}}}
async function loadMsgs(id){S.msgs[id]=await api(`/agents/${id}/messages`);if(cur&&cur.id===id)renderThread()}
// ---------- live events (SSE) ----------
let es,esDown=false;
function connect(){es=new EventSource(API+"/events");
 es.onopen=()=>{if(esDown){esDown=false;loadState().catch(()=>{})}};
 es.onerror=()=>{esDown=true};
 es.addEventListener("agent",e=>{const a=JSON.parse(e.data);upsert(a);if(cur&&cur.id===a.id){cur=a;renderThread()}renderListSoon()});
 es.addEventListener("archived",e=>{const {id}=JSON.parse(e.data);S.agents=S.agents.filter(a=>a.id!==id);S.pending=S.pending.filter(m=>m.agentId!==id);renderListSoon()});
 es.addEventListener("message",e=>{const m=JSON.parse(e.data);const L=S.msgs[m.agentId];delete S.drafts[m.id];
  if(L){const i=L.findIndex(x=>x.id===m.id);if(i<0)L.push(m);else L[i]=m}
  S.pending=S.pending.filter(x=>x.id!==m.id);if(m.approval&&m.approval.state==="pending")S.pending.push(m);
  if(cur&&cur.id===m.agentId)renderThread();renderListSoon()});
 es.addEventListener("delta",e=>{const d=JSON.parse(e.data);S.deltaCount=(S.deltaCount||0)+1;const x=S.drafts[d.id]||(S.drafts[d.id]={agentId:d.agentId,text:""});x.text+=d.delta;if(cur&&cur.id===d.agentId)paintDraft(d.id)});
 es.addEventListener("draft_end",e=>{const d=JSON.parse(e.data);if(S.drafts[d.id]){delete S.drafts[d.id];if(cur&&cur.id===d.agentId)renderThread()}});
 es.addEventListener("activity",e=>{if(sheetOpen===$("#activity"))renderLog([JSON.parse(e.data)],true)});
 es.addEventListener("brief",e=>{S.brief=JSON.parse(e.data);renderListSoon()});
 es.addEventListener("notify",e=>{const n=JSON.parse(e.data);if(document.hidden&&!pushOn&&"Notification"in window&&Notification.permission==="granted")navigator.serviceWorker?.ready.then(r=>r.showNotification(n.title,{body:n.body,data:{url:n.url},tag:n.url}))});
}
// ---------- list ----------
let filter="";
function renderList(){
 const ags=[...S.agents].sort((a,b)=>(b.pin|0)-(a.pin|0)||(waiting(b)-waiting(a))||T(b.t)-T(a.t));const w=ags.filter(waiting).length;
 const sum=$("#summary");sum.textContent=`${ags.length} agent · `;sum.append(el("b",0,`${w} đang chờ bạn`));
 if(!S.ai.configured)sum.append(el("span","warn"," · AI chưa cấu hình"));
 const br=$("#brief");br.innerHTML="";const h=el("div","brief-h");h.append(el("h2",0,"Bản tin sáng"));const rf=el("button","text-btn small","Làm mới");rf.setAttribute("aria-label","Tạo lại bản tin sáng");rf.onclick=refreshBrief;h.append(rf);br.append(h);
 const b=S.brief;br.append(el("p",0,b?b.text:"Chưa có bản tin."));if(b)br.append(el("small","meta",`${b.source==="ai"?"AI · "+(S.ai.model||""):"Tự động"} · ${ago(b.generated)}`));
 const ol=el("ol");S.pending.forEach(m=>{const a=S.agents.find(x=>x.id===m.agentId);if(!a)return;const li=el("li"),bt=el("button",0,`${a.name}: ${m.approval.title}`);bt.onclick=()=>openThread(a.id);li.append(bt);ol.append(li)});
 if(b&&b.items)b.items.filter(it=>!S.pending.some(m=>it.includes(m.approval.title))).slice(0,4).forEach(it=>ol.append(el("li","note",it)));
 if(!ol.children.length)ol.append(el("li","note","Không có gì cần bạn lúc này."));br.append(ol);
 const ul=$("#rows");ul.innerHTML="";
 ags.filter(a=>!filter||a.name.toLowerCase().includes(filter)).forEach(a=>{
  const li=el("li","row");li.dataset.id=a.id;const acts=el("div","actions"),pin=el("button","pin",a.pin?"Bỏ ghim":"Ghim"),ar=el("button","arch","Lưu trữ");acts.append(pin,ar);
  pin.onclick=e=>{e.stopPropagation();suppress=performance.now()+600;api(`/agents/${a.id}`,{pin:!a.pin}).then(upsert).then(renderList).catch(()=>toast("Không ghim được"))};
  ar.onclick=e=>{e.stopPropagation();suppress=performance.now()+600;if(ar.disabled)return;ar.disabled=true;api(`/agents/${a.id}`,{archived:true}).then(()=>{S.agents=S.agents.filter(x=>x.id!==a.id);S.pending=S.pending.filter(m=>m.agentId!==a.id);renderList();toast("Đã lưu trữ "+a.name)}).catch(()=>{ar.disabled=false;toast("Không lưu trữ được")})};
  const inn=el("div","rowin");inn.setAttribute("role","button");inn.tabIndex=0;inn.append(thumb(a.av));const tx=el("div","rowtxt"),h=el("div","rowh");h.append(el("b",0,a.name));if(a.tag)h.append(el("span","tag",a.tag));
  const live=a.status==="working";h.append(el("span","time"+(live?" now":""),live?"bây giờ":ago(a.t)));
  tx.append(h,el("div","snip"+(waiting(a)?" wait":"")+(live?" dots":"")+(a.status==="error"?" err":""),a.snip));inn.append(tx);li.append(acts,inn);ul.append(li);
  inn.onclick=()=>{if(performance.now()<suppress)return;if(li.dataset.open){closeRow(li);return}openThread(a.id)};inn.onkeydown=e=>{if(e.key==="Enter")openThread(a.id)};
  swipeRow(li,inn)});
}
let briefBusy=false;async function refreshBrief(){if(briefBusy)return;briefBusy=true;toast("Đang tạo bản tin…");try{S.brief=await api("/brief",{});renderList();toast("Đã cập nhật bản tin")}catch{toast("Không tạo được bản tin")}finally{briefBusy=false}}
let openRow=null,suppress=0;const closeRow=li=>{li.querySelector(".rowin").style.transform="";delete li.dataset.open;if(openRow===li)openRow=null};
function swipeRow(li,inn){let x0,y0,dx=0,mode=null,base=0;
 li.addEventListener("touchstart",e=>{const t=e.touches[0];if(cur){mode="skip";x0=null;return}x0=t.clientX;y0=t.clientY;dx=0;mode=null;base=li.dataset.open?-160:0;if(openRow&&openRow!==li)closeRow(openRow)},{passive:true});
 li.addEventListener("touchmove",e=>{if(x0==null)return;const t=e.touches[0],ddx=t.clientX-x0,ddy=t.clientY-y0;if(!mode){if(Math.abs(ddx)>8&&Math.abs(ddx)>Math.abs(ddy))mode="x";else if(Math.abs(ddy)>8)mode="y"}if(mode!=="x")return;e.preventDefault();dx=ddx;li.classList.add("drag");inn.style.transform=`translate3d(${Math.max(-200,Math.min(0,base+dx))}px,0,0)`},{passive:false});
 li.addEventListener("touchend",()=>{li.classList.remove("drag");if(mode!=="x")return;const p=base+dx;if(p<-70){inn.style.transform="translate3d(-160px,0,0)";li.dataset.open=1;openRow=li}else closeRow(li);
  if(Math.abs(dx)>10)suppress=performance.now()+400});
 li.addEventListener("touchcancel",()=>{li.classList.remove("drag");closeRow(li)});
}
// ---------- thread ----------
let cur=null;
const stateTxt={working:"Đang làm việc…",waiting:"Chờ bạn duyệt",idle:"Sẵn sàng",done:"Đã xong",error:"Có lỗi"};
const stateLine=a=>a.status==="working"&&a.snip&&a.snip.startsWith("Bước")?a.snip:waiting(a)?"Chờ bạn duyệt":stateTxt[a.status]||"";
function openThread(id){const a=S.agents.find(x=>x.id===id);if(!a)return;cur=a;draw($("#threadAvatar"),a.av);$("#threadName").textContent=a.name;$("#threadState").textContent=stateLine(a);renderThread();loadMsgs(id).catch(()=>toast("Không tải được hội thoại"));$("#thread").classList.remove("off");$("#list").classList.add("under");const hs=history.state;if(hs&&hs.spent)history.replaceState({t:1},"","#"+id);else if(!(hs&&hs.t))history.pushState({t:1},"","#"+id)}
function closeThread(fromPop){if(!cur)return;cur=null;stopVoice();$("#thread").classList.add("off");$("#list").classList.remove("under");$("#thread").style.transform="";$("#list").style.transform="";renderList();if(!fromPop&&history.state&&history.state.t)history.replaceState({spent:1},"",location.pathname)}
const deciding=new Set();
const ICON={pending:"○",running:"◐",done:"✓",waiting:"⏸",failed:"✕",cancelled:"–"};
const editing=new Set(),usedChoice=new Set();
function planCard(m){const c=el("div","plan");const st=m.plan.steps,done=st.filter(x=>x.status==="done").length;
 const h=el("div","plan-h");h.append(el("b",0,"Kế hoạch"),el("span",0,`${done}/${st.length}`));c.append(h);
 const bar=el("div","bar");const f=el("i");f.style.width=(100*done/st.length)+"%";bar.append(f);c.append(bar);
 const ol=el("ol");st.forEach((x,i)=>{const li=el("li","step "+x.status);li.append(stepIcon(x.status),el("span",0,x.title));ol.append(li)});c.append(ol);return c}
function approvalCard(m){const ap=m.approval,c=el("div","approval"+(ap.state!=="pending"?" done":""));c.dataset.id=m.id;
 c.append(el("small",0,ap.state==="superseded"?"Đã thay bằng yêu cầu mới":"Cần bạn duyệt · không gửi khi chưa có OK"),el("div","ap-title",ap.title));
 if(ap.to){const meta=el("div","ap-meta");meta.append(el("span",0,"Tới: "),el("b",0,ap.to));c.append(meta)}
 const ed=ap.state==="pending"&&editing.has(m.id);
 if(ap.subject||(ed&&ap.kind==="email")){if(ed){const i=el("input","ap-subject");i.value=ap.subject||"";i.setAttribute("aria-label","Tiêu đề");c.append(i)}else{const meta=el("div","ap-meta");meta.append(el("span",0,"Tiêu đề: "),el("b",0,ap.subject));c.append(meta)}}
 if(ed){const t=el("textarea","ap-body");t.value=ap.body;t.rows=8;t.setAttribute("aria-label","Nội dung");c.append(t)}else c.append(el("pre",0,ap.body));
 if(ap.state==="pending"){const bs=el("div","btns"),no=el("button",0,"Để sau"),e=el("button",0,ed?"Huỷ sửa":"Sửa"),go=el("button","go",ed?"Lưu & duyệt":"Duyệt");
  no.onclick=()=>decide(m,"declined",bs);e.onclick=()=>{ed?editing.delete(m.id):editing.add(m.id);renderThread()};
  go.onclick=()=>{const ex={};if(ed){ex.body=c.querySelector(".ap-body").value;const sj=c.querySelector(".ap-subject");if(sj)ex.subject=sj.value;if(!ex.body.trim()){toast("Nội dung trống");return}}decide(m,"approved",bs,ex)};
  if(deciding.has(m.id))no.disabled=e.disabled=go.disabled=true;bs.append(no,e,go);c.append(bs);
  c.append(el("small","hint","Muốn đổi hướng? Nhắn yêu cầu mới, bản nháp này sẽ được thay."))}
 else if(ap.state!=="superseded"){const dv=ap.delivery,DS={queued:"đang chờ kênh email",sending:"đang gửi…",sent:"đã gửi"+(dv&&dv.from?" từ "+dv.from:""),failed:"gửi thất bại"+(dv&&dv.detail?": "+dv.detail:"")};c.append(el("small",0,ap.state==="approved"?(dv?"✓ Bạn đã duyệt · "+DS[dv.status]:"✓ Bạn đã duyệt · chưa gửi ra ngoài (kênh gửi chưa kết nối)"):"Đã hoãn"))}
 return c}
function renderThread(){const a=cur;if(!a)return;$("#threadState").textContent=stateLine(a);const box=$("#messages");
 const keepEdit=[...box.querySelectorAll(".ap-body")].map(t=>[t.closest(".approval").dataset.id,t.value,t.closest(".approval").querySelector(".ap-subject")?.value]);
 box.innerHTML="";const L=S.msgs[a.id]||[];
 L.forEach((m,idx)=>{if(m.text){const b=el("div","msg"+(m.role==="user"?" mine":m.role==="system"?" sys":""),m.text);if(m.queued){b.append(el("span","chip","Đang chờ lượt"))}box.append(b)}
  if(m.plan)box.append(planCard(m));
  if(m.choices&&m.choices.length){const wrap=el("div","choices");const later=L.slice(idx+1).some(x=>x.role==="user");m.choices.forEach(ch=>{const b=el("button","choice",ch);b.disabled=later||usedChoice.has(m.id);b.onclick=async()=>{if(usedChoice.has(m.id))return;usedChoice.add(m.id);renderThread();if(!(await sendMsg(ch)))usedChoice.delete(m.id)};wrap.append(b)});box.append(wrap)}
  if(m.approval)box.append(approvalCard(m))});
 keepEdit.forEach(([id,v,sj])=>{const c=box.querySelector(`.approval[data-id="${id}"]`);if(c){const t=c.querySelector(".ap-body");if(t)t.value=v;const s2=c.querySelector(".ap-subject");if(s2&&sj!=null)s2.value=sj}});
 Object.entries(S.drafts).forEach(([id,d])=>{if(d.agentId===a.id){const b=el("div","msg draft",visible(d.text));b.id="d_"+id;box.append(b)}});
 if(a.status==="working"&&!Object.values(S.drafts).some(d=>d.agentId===a.id))box.append(el("div","msg typing dots","Đang nghĩ"));
 setSendMode();
 if(!box.contains(document.activeElement)||document.activeElement===box)requestAnimationFrame(()=>box.scrollTop=box.scrollHeight)}
const visible=t=>{const i=t.search(/(^|\n)\s*(APPROVAL|PLAN|CHOICES):/);return(i>=0?t.slice(0,i):t).trim()||"…"};
function paintDraft(id){const n=document.getElementById("d_"+id);if(!n){renderThread();return}n.textContent=visible(S.drafts[id].text);const box=$("#messages");box.scrollTop=box.scrollHeight}
async function decide(m,v,bs,extra){if(deciding.has(m.id))return;deciding.add(m.id);bs.querySelectorAll("button").forEach(b=>b.disabled=true);
 try{await api(`/approvals/${m.id}`,{decision:v,...(extra||{})});buzz(v==="approve"?"confirm":"soft");editing.delete(m.id)}catch(e){if(e.status!==409){deciding.delete(m.id);toast("Không ghi nhận được, thử lại");bs.querySelectorAll("button").forEach(b=>b.disabled=false)}}}
// send ↔ stop in the same spot while the agent works (stopping holds queued follow-ups)
function setSendMode(){const b=$("#composer .send");const run=!!(cur&&cur.status==="working")&&!$("#input").value.trim();b.classList.toggle("stop",run);b.replaceChildren(I(run?"square":"arrow-up"));b.setAttribute("aria-label",run?"Dừng":"Gửi")}
let stopping=false;async function stopAgent(){if(!cur||stopping)return;stopping=true;try{await api(`/agents/${cur.id}/stop`,{});buzz("soft");toast("Đã dừng")}catch{toast("Không dừng được")}finally{stopping=false}}
let sending=false;
async function sendMsg(text){const a=cur;text=text.trim();if(!a||!text||sending)return false;sending=true;
 try{await api(`/agents/${a.id}/messages`,{text});buzz("tap");return true}catch{toast("Không gửi được, thử lại");return false}finally{sending=false}}
// edge-swipe back (iOS style), anywhere on screen while a thread is open
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
let creating=false;
$("#createBtn").onclick=async()=>{if(sheetOpen!==sheet||creating)return;creating=true;const name=$("#agentName").value.trim()||"Agent mới",desc=$("#agentDesc").value.trim();
 try{const a=await api("/agents",{name,desc,av:{...draft}});upsert(a);closeSheet();renderList();toast("Đã tạo "+a.name)}catch{toast("Không tạo được agent")}finally{creating=false}};
document.querySelectorAll(".sheet").forEach(s=>{let y0=null,dy=0;const start=e=>{const sc=e.target.closest(".scroll");if(sc&&sc.scrollTop>0)return;y0=e.touches[0].clientY;dy=0};
 s.addEventListener("touchstart",start,{passive:true});
 s.addEventListener("touchmove",e=>{if(y0==null)return;dy=e.touches[0].clientY-y0;if(dy<=0){s.style.transform="";return}if(e.target.closest(".scroll")&&dy<12)return;e.preventDefault();s.classList.add("drag");s.style.transform=`translate3d(0,${dy}px,0)`},{passive:false});
 const end=()=>{if(y0==null)return;s.classList.remove("drag");if(dy>120)closeSheet();else s.style.transform="";y0=null};s.addEventListener("touchend",end);s.addEventListener("touchcancel",end)});
// ---------- activity ----------
function renderLog(items,prepend){const L=$("#log");if(!prepend)L.innerHTML="";items.forEach(x=>{const li=el("li"),t=el("time",0,ago(x.t));li.append(t,x.text);prepend?L.prepend(li):L.append(li)})}
$("#activityBtn").onclick=()=>{if(sheetOpen)return;renderLog([]);openSheet($("#activity"));api("/activity?limit=80").then(x=>renderLog(x)).catch(()=>toast("Không tải được hoạt động"))};$("#closeActivity").onclick=closeSheet;
// ---------- notifications (Web Push via VAPID; falls back to SW notifications while the app is open) ----------
let pushOn=false;
const u8=b=>{const s=atob((b+"=".repeat((4-b.length%4)%4)).replace(/-/g,"+").replace(/_/g,"/"));return Uint8Array.from(s,c=>c.charCodeAt(0))};
async function enableNotify(){if(!("Notification"in window)||!("serviceWorker"in navigator)){toast("Thiết bị này chưa hỗ trợ thông báo. Trên iPhone: thêm vào Màn hình chính trước.");return}
 const p=await Notification.requestPermission();if(p!=="granted"){toast("Bạn chưa cho phép thông báo");return}
 const reg=await navigator.serviceWorker.ready;
 if("PushManager"in window&&S.vapid){try{const sub=(await reg.pushManager.getSubscription())||await reg.pushManager.subscribe({userVisibleOnly:true,applicationServerKey:u8(S.vapid)});await api("/push/subscribe",sub.toJSON());pushOn=true;toast("Đã bật thông báo đẩy")}catch{toast("Bật thông báo khi app đang mở (push chưa khả dụng)")}}
 else toast("Bật thông báo khi app đang mở");$("#bellBtn").classList.add("on")}
$("#bellBtn").onclick=()=>{enableNotify().catch(()=>toast("Không bật được thông báo"))};
// ---------- voice input (Web Speech API where supported) ----------
const SR=window.SpeechRecognition||window.webkitSpeechRecognition;let rec=null;
function stopVoice(){if(rec){try{rec.stop()}catch{}rec=null;$("#micBtn").classList.remove("on")}}
if(!SR)$("#micBtn").classList.add("hidden");
$("#micBtn").onclick=()=>{if(rec){stopVoice();return}rec=new SR();rec.lang="vi-VN";rec.interimResults=true;rec.continuous=false;const base=inp.value?inp.value+" ":"";
 rec.onresult=e=>{let t="";for(const r of e.results)t+=r[0].transcript;inp.value=base+t;inp.oninput()};rec.onend=()=>{rec=null;$("#micBtn").classList.remove("on")};rec.onerror=e=>{toast(e.error==="not-allowed"?"Chưa cho phép micro":"Không nghe được, thử lại")};
 try{rec.start();$("#micBtn").classList.add("on")}catch{rec=null}};
// ---------- themes ----------
const THEMES=[{id:"brand",name:"Theo hệ thống",sub:"Kem sáng hoặc tím đêm, theo máy",sw:["#f4efe4","#e0261f","#131018"]},{id:"light",name:"Sáng",sub:"Kem, khăn đỏ · luôn sáng",sw:["#f4efe4","#e0261f","#17130e"],tc:"#f4efe4"},{id:"dark",name:"Tối",sub:"Tím đêm, khăn đỏ · luôn tối",sw:["#131018","#e0261f","#f4efe4"],tc:"#131018"},{id:"gold",name:"Đen vàng",sub:"Nền đen, điểm vàng · tối",sw:["#0d0c0a","#d4a72c","#f5ecd7"],tc:"#0d0c0a"},{id:"blue",name:"Trắng xanh",sub:"Nền trắng, điểm xanh · sáng",sw:["#f5f8fc","#1d5fd1","#0f1b2d"],tc:"#f5f8fc"}];
const curTheme=()=>{try{return localStorage.getItem("neko-aurora-theme")||"brand"}catch{return"brand"}};
function applyTheme(id){const t=THEMES.find(x=>x.id===id)||THEMES[0];try{t.id==="brand"?localStorage.removeItem("neko-aurora-theme"):localStorage.setItem("neko-aurora-theme",t.id)}catch{}
 if(t.id==="brand")delete document.documentElement.dataset.theme;else document.documentElement.dataset.theme=t.id;
 const ms=document.querySelectorAll('meta[name="theme-color"]');ms.forEach((m,i)=>m.content=t.tc||(i===0?"#f4efe4":"#131018"));
 const cs=document.querySelector('meta[name="color-scheme"]');if(cs)cs.content=t.id==="gold"||t.id==="dark"?"dark":t.id==="blue"||t.id==="light"?"light":"light dark";renderThemes()}
function renderThemes(){const L=$("#themeList");L.innerHTML="";const c=curTheme();THEMES.forEach(t=>{const b=el("button","theme-opt");b.setAttribute("aria-pressed",c===t.id);b.dataset.theme=t.id;const d=el("span","dots3");t.sw.forEach(x=>{const i=el("i");i.style.background=x;d.append(i)});const tx=el("span");tx.append(el("b",0,t.name),el("small",0,t.sub));b.append(d,tx);b.onclick=()=>applyTheme(t.id);L.append(b)})}
$("#themeBtn").onclick=()=>{if(sheetOpen)return;renderThemes();openSheet($("#themes"))};$("#closeThemes").onclick=closeSheet;
// ---------- misc ----------
$("#backBtn").onclick=()=>closeThread();
addEventListener("popstate",()=>{if(sheetOpen)closeSheet();else if(cur)closeThread(true)});
$("#searchBtn").onclick=()=>{const s=$("#search");s.classList.toggle("hidden");if(!s.classList.contains("hidden"))s.focus();else{filter="";s.value="";renderList()}};
$("#search").oninput=e=>{filter=e.target.value.toLowerCase();renderList()};
const inp=$("#input");inp.oninput=()=>{inp.style.height="auto";inp.style.height=Math.min(140,inp.scrollHeight)+"px";setSendMode()};
$("#composer").onsubmit=async e=>{e.preventDefault();stopVoice();const v=inp.value;if(!v.trim()){if(cur&&cur.status==="working")stopAgent();return}inp.value="";inp.style.height="";if(!(await sendMsg(v)))inp.value=v};
inp.onkeydown=e=>{if(e.key==="Enter"&&!e.shiftKey&&!e.isComposing){e.preventDefault();$("#composer").requestSubmit()}};
document.addEventListener("visibilitychange",()=>{if(!document.hidden&&es)loadState().catch(()=>{})});
draw($("#meAvatar"),{shell:2,hat:"none",eyes:"none",extra:"flower"});
if("serviceWorker"in navigator)navigator.serviceWorker.register("/app/sw.js",{scope:"/app/"}).then(r=>r.pushManager?.getSubscription()).then(s=>{if(s){pushOn=true;$("#bellBtn").classList.add("on")}}).catch(()=>{});
function boot(){return loadState().then(()=>{$("#login").classList.add("hidden");if(!es)connect();const h=location.hash.slice(1);if(h)openThread(h)}).catch(e=>{if(e.status===401){$("#login").classList.remove("hidden")}else toast("Không kết nối được máy chủ")})}
let logging=false;$("#loginForm").onsubmit=async e=>{e.preventDefault();if(logging)return;logging=true;try{const r=await fetch("/app/login",{method:"POST",credentials:"same-origin",headers:{"Content-Type":"application/json"},body:JSON.stringify({code:$("#loginCode").value})});if(!r.ok){toast(r.status===429?"Thử quá nhiều lần, đợi 5 phút":"Mã không đúng");return}$("#loginCode").value="";await boot()}catch{toast("Không kết nối được")}finally{logging=false}};
boot();
// ---------- Aurora icons (Lucide, ISC) + haptics ----------
function I(n,sz){return self.AURORA_ICONS?AURORA_ICONS.icon(n,sz):document.createTextNode("")}
function stepIcon(st){const m={pending:"circle",running:"loader-circle",done:"check",waiting:"pause",failed:"x",cancelled:"minus"};const s=el("span","ic");s.append(I(m[st]||"circle",16));return s}
// Haptics: no guide shipped with the design system, so keep them rare and short; vibrate is a no-op on iOS Safari.
function buzz(k){try{if(matchMedia("(prefers-reduced-motion: reduce)").matches||!navigator.vibrate)return;navigator.vibrate({tap:8,confirm:[10,40,14],soft:16}[k]||8)}catch{}}
[["#themeBtn","sun-moon"],["#bellBtn","bell"],["#searchBtn","search"],["#newBtn","plus"],["#backBtn","chevron-left"],["#activityBtn","list-checks"],["#micBtn","mic"]].forEach(([q,n])=>{const b=$(q);if(b)b.replaceChildren(I(n))});
{const sb=$("#surpriseBtn");if(sb)sb.replaceChildren(I("dices",18),document.createTextNode(" Ngẫu nhiên"))}
setSendMode();
window.__AURORA={state:()=>S,reload:loadState,current:()=>cur&&cur.id};
})();
