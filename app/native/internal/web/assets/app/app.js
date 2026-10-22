"use strict";
(()=>{
const $=s=>document.querySelector(s);
const VERSION="2026.10";
// ---------- flat bot faces (original shapes drawn in code: solid body + two capsule eyes) ----------
const COLORS=["#e0261f","#f26b2b","#eda12c","#2c9f5e","#1fa7a0","#3b7be0","#5b5bd6","#9b5de5","#e8478c","#96590a","#6b7280","#2a2320"];
const poly=(n,r,rot=-90,r2)=>{const p=[];const k=r2?n*2:n;for(let i=0;i<k;i++){const a=(rot+i*360/k)*Math.PI/180,rr=r2&&i%2?r2:r;p.push((50+rr*Math.cos(a)).toFixed(1)+","+(50+rr*Math.sin(a)).toFixed(1))}return p.join(" ")};
const rp=(pts,w)=>`<polygon points="${pts}" stroke="C" stroke-width="${w}" stroke-linejoin="round"/>`;
const SHAPES=[ // [markup, eye centre y]
 ['<circle cx="50" cy="50" r="44"/>',48],
 ['<rect x="8" y="8" width="84" height="84" rx="30"/>',48],
 [rp(poly(6,40,-90),12),48],
 [rp("50,10 90,50 50,90 10,50",14),48],
 ['<circle cx="50" cy="27" r="23"/><circle cx="73" cy="50" r="23"/><circle cx="50" cy="73" r="23"/><circle cx="27" cy="50" r="23"/><circle cx="50" cy="50" r="26"/>',48],
 [rp(poly(5,44,-90,24),12),54],
 ['<circle cx="34" cy="58" r="24"/><circle cx="58" cy="42" r="30"/><circle cx="72" cy="64" r="20"/><rect x="12" y="58" width="78" height="26" rx="13"/>',54],
 ['<path d="M50 6C62 24 86 44 86 62a36 36 0 0 1-72 0C14 44 38 24 50 6Z"/>',60],
 ['<rect x="20" y="6" width="60" height="88" rx="30"/>',46],
 [rp("50,14 88,82 12,82",14),60],
 [rp(poly(5,42,-90),12),52],
 [rp(poly(8,46,-90,36),6),48],
 ['<path d="M50 8C78 8 92 34 90 56C88 80 70 94 48 92C24 90 8 74 10 50C12 26 26 8 50 8Z"/>',48]];
const SVGNS="http://www.w3.org/2000/svg";
function face(f,sz,cls){f=f||{};const s=SHAPES[f.shape|0]||SHAPES[0],c=COLORS[f.color|0]||COLORS[0],y=s[1];
 const v=document.createElementNS(SVGNS,"svg");v.setAttribute("viewBox","0 0 100 100");v.setAttribute("width",sz);v.setAttribute("height",sz);v.setAttribute("aria-hidden","true");v.setAttribute("class","face"+(cls?" "+cls:""));
 v.innerHTML=`<g fill="${c}">${s[0].replace(/"C"/g,`"${c}"`)}</g><rect x="35" y="${y-11}" width="10" height="22" rx="5" fill="#fff"/><rect x="55" y="${y-11}" width="10" height="22" rx="5" fill="#fff"/>`;return v}
// ---------- server state (real Go control plane at /app/api) ----------
const S={agents:[],pending:[],brief:null,ai:{},vapid:"",outbound:[],msgs:{},drafts:{}};
const API="/app/api";
async function api(path,body){const r=await fetch(API+path,{method:body===undefined?"GET":"POST",credentials:"same-origin",cache:"no-store",headers:body===undefined?{}:{"Content-Type":"application/json"},body:body===undefined?undefined:JSON.stringify(body)});
 if(!r.ok){const e=new Error(String(r.status));e.status=r.status;throw e}return r.json()}
const T=x=>new Date(x).getTime();
const isGroup=a=>a&&a.kind==="group";
const bots=()=>S.agents.filter(a=>!isGroup(a));
const byId=id=>S.agents.find(x=>x.id===id);
const waiting=a=>S.pending.some(m=>m.agentId===a.id)||a.status==="waiting";
const ago=t=>{t=T(t);if(!(t>0))return"";const s=(Date.now()-t)/1e3;if(s<60)return"bây giờ";if(s<3600)return Math.floor(s/60)+" phút";if(s<86400&&new Date(t).getDate()===new Date().getDate())return new Date(t).toLocaleTimeString("vi-VN",{hour:"2-digit",minute:"2-digit"});return new Date(t).toLocaleDateString("vi-VN",{day:"numeric",month:"numeric"})};
const el=(t,c,x)=>{const e=document.createElement(t);if(c)e.className=c;if(x!=null)e.textContent=x;return e};
let toastT;const toast=m=>{const t=$("#toast");t.textContent=m;t.classList.add("show");clearTimeout(toastT);toastT=setTimeout(()=>t.classList.remove("show"),2200)};
const upsert=a=>{const i=S.agents.findIndex(x=>x.id===a.id);if(i<0)S.agents.push(a);else S.agents[i]=a};
let listQueued=false;const renderListSoon=()=>{if(listQueued)return;listQueued=true;requestAnimationFrame(()=>{listQueued=false;renderList()})};
async function loadState(){const st=await api("/state");S.agents=st.agents;S.pending=st.pending;S.brief=st.brief;S.ai=st.ai;S.vapid=st.vapidPublicKey;S.outbound=st.outbound||[];S.pushSubs=st.pushSubscriptions|0;seedSeen();renderList();if(cur){const a=byId(cur.id);if(a){cur=a;await loadMsgs(a.id)}}}
async function loadMsgs(id){S.msgs[id]=await api(`/agents/${id}/messages`);if(cur&&cur.id===id)renderThread()}
// ---------- unread (device-local: last time each conversation was seen) ----------
const store={get(k,d){try{const v=localStorage.getItem(k);return v==null?d:JSON.parse(v)}catch{return d}},set(k,v){try{localStorage.setItem(k,JSON.stringify(v))}catch{}}};
let seen=store.get("neko-aurora-seen",null);
function seedSeen(){if(seen)return;seen={};const now=Date.now();S.agents.forEach(a=>seen[a.id]=now);store.set("neko-aurora-seen",seen)}
const markSeen=id=>{if(!seen)seen={};seen[id]=Date.now();store.set("neko-aurora-seen",seen)};
const unread=a=>!(cur&&cur.id===a.id)&&a.status!=="working"&&T(a.t)>(seen&&seen[a.id]!=null?seen[a.id]:0);
// ---------- live events (SSE) ----------
let es,esDown=false;
function connect(){es=new EventSource(API+"/events");
 es.onopen=()=>{if(esDown){esDown=false;loadState().catch(()=>{})}};
 es.onerror=()=>{esDown=true};
 es.addEventListener("agent",e=>{const a=JSON.parse(e.data);if(seen&&seen[a.id]==null)markSeen(a.id);upsert(a);if(cur&&cur.id===a.id){cur=a;markSeen(a.id);renderThread()}renderListSoon()});
 es.addEventListener("archived",e=>{const {id}=JSON.parse(e.data);S.agents=S.agents.filter(a=>a.id!==id);S.pending=S.pending.filter(m=>m.agentId!==id);renderListSoon()});
 es.addEventListener("message",e=>{const m=JSON.parse(e.data);const L=S.msgs[m.agentId];delete S.drafts[m.id];
  if(L){const i=L.findIndex(x=>x.id===m.id);if(i<0)L.push(m);else L[i]=m}
  S.pending=S.pending.filter(x=>x.id!==m.id);if(m.approval&&m.approval.state==="pending")S.pending.push(m);
  if(cur&&cur.id===m.agentId)renderThread();renderListSoon()});
 es.addEventListener("delta",e=>{const d=JSON.parse(e.data);S.deltaCount=(S.deltaCount||0)+1;const x=S.drafts[d.id]||(S.drafts[d.id]={agentId:d.agentId,from:d.from,text:""});x.text+=d.delta;if(cur&&cur.id===d.agentId)paintDraft(d.id)});
 es.addEventListener("draft_end",e=>{const d=JSON.parse(e.data);if(S.drafts[d.id]){delete S.drafts[d.id];if(cur&&cur.id===d.agentId)renderThread()}});
 es.addEventListener("activity",e=>{if(sheetOpen===$("#activity"))renderLog([JSON.parse(e.data)],true)});
 es.addEventListener("brief",e=>{S.brief=JSON.parse(e.data);renderListSoon()});
 es.addEventListener("notify",e=>{const n=JSON.parse(e.data);if(muted.includes((n.url||"").split("#")[1]))return;if(document.hidden&&!pushOn&&"Notification"in window&&Notification.permission==="granted")navigator.serviceWorker?.ready.then(r=>r.showNotification(n.title,{body:n.body,data:{url:n.url},tag:n.url}))});
}
// ---------- list ----------
function avatarOf(a,sz){if(!isGroup(a))return face(a.av,sz);const w=el("span","stack");w.style.width=w.style.height=sz+"px";(a.members||[]).slice(0,2).forEach((id,i)=>{const b=byId(id);w.append(face(b&&b.av,Math.round(sz*.64),"s"+i))});return w}
function badge(a){const k=a.status==="working"?"work":waiting(a)?"wait":a.status==="error"?"err":"";if(!k)return null;const b=el("i","badge "+k);b.setAttribute("aria-hidden","true");return b}
let filter="";
const statusLine=a=>a.status==="working"?"Đang làm việc…":waiting(a)?"Chờ bạn duyệt":a.snip||a.desc||"";
function renderList(){
 const all=[...S.agents].sort((a,b)=>(waiting(b)-waiting(a))||T(b.t)-T(a.t));
 const q=filter.trim().toLowerCase(),match=a=>!q||a.name.toLowerCase().includes(q)||(a.snip||"").toLowerCase().includes(q);
 const pinned=all.filter(a=>a.pin&&match(a)),rest=all.filter(a=>!a.pin&&match(a));
 const fe=$("#featured");fe.innerHTML="";fe.classList.toggle("hidden",!pinned.length);
 pinned.forEach(a=>{const b=el("button","feat");b.dataset.id=a.id;b.setAttribute("aria-label",a.name);const av=el("span","feat-av");av.append(avatarOf(a,72));const bd=badge(a);if(bd)av.append(bd);if(unread(a))av.append(el("i","dot"));b.append(av,el("span","feat-name",a.name));b.onclick=()=>openThread(a.id);fe.append(b)});
 renderBrief(q);
 const ul=$("#rows");ul.innerHTML="";
 rest.forEach(a=>{
  const li=el("li","row");li.dataset.id=a.id;
  if(!isGroup(a)){const acts=el("div","actions"),pin=el("button","pin",a.pin?"Bỏ ghim":"Ghim"),ar=el("button","arch","Lưu trữ");acts.append(pin,ar);li.append(acts);
   pin.onclick=e=>{e.stopPropagation();suppress=performance.now()+600;api(`/agents/${a.id}`,{pin:!a.pin}).then(upsert).then(()=>{buzz("tap");renderList()}).catch(()=>toast("Không ghim được"))};
   ar.onclick=e=>{e.stopPropagation();suppress=performance.now()+600;if(ar.disabled)return;ar.disabled=true;api(`/agents/${a.id}`,{archived:true}).then(()=>{S.agents=S.agents.filter(x=>x.id!==a.id);S.pending=S.pending.filter(m=>m.agentId!==a.id);renderList();toast("Đã lưu trữ "+a.name)}).catch(()=>{ar.disabled=false;toast("Không lưu trữ được")})}}
  const inn=el("div","rowin");inn.setAttribute("role","button");inn.tabIndex=0;const av=el("span","row-av");av.append(avatarOf(a,52));const bd=badge(a);if(bd)av.append(bd);inn.append(av);
  const tx=el("div","rowtxt"),h=el("div","rowh");h.append(el("b",0,a.name));if(isGroup(a))h.append(el("span","tag",(a.members||[]).length+" bot"));
  const live=a.status==="working",un=unread(a);h.append(el("span","time"+(live?" now":"")+(un?" un":""),live?"bây giờ":ago(a.t)));
  const sn=el("div","snip"+(waiting(a)?" wait":"")+(live?" dots":"")+(a.status==="error"?" err":"")+(un?" un":""),statusLine(a));
  tx.append(h,sn);inn.append(tx);if(un)inn.append(el("i","dot"));li.append(inn);ul.append(li);
  inn.onclick=()=>{if(performance.now()<suppress)return;if(li.dataset.open){closeRow(li);return}openThread(a.id)};inn.onkeydown=e=>{if(e.key==="Enter")openThread(a.id)};
  if(!isGroup(a))swipeRow(li,inn)});
 const em=$("#empty");em.classList.toggle("hidden",!!(pinned.length||rest.length));em.textContent=q?"Không tìm thấy “"+filter.trim()+"”.":"Chưa có bot nào. Bấm + để tạo.";
 updatePill()}
function renderBrief(q){const br=$("#brief");br.innerHTML="";br.classList.toggle("hidden",!!q);if(q)return;
 const h=el("div","brief-h");h.append(el("h2",0,"Bản tin sáng"));const rf=el("button","text-btn small","Làm mới");rf.setAttribute("aria-label","Tạo lại bản tin sáng");rf.onclick=refreshBrief;h.append(rf);br.append(h);
 const b=S.brief;br.append(el("p",0,b?b.text:"Chưa có bản tin."));if(b)br.append(el("small","meta",`${b.source==="ai"?"AI · "+(S.ai.model||""):"Tự động"} · ${ago(b.generated)}`+(S.ai.configured?"":" · AI chưa cấu hình")));
 const ol=el("ol");S.pending.forEach(m=>{const a=byId(m.agentId);if(!a)return;const li=el("li"),bt=el("button","brief-item");bt.append(el("span","bi-dot"),el("span",0,`${a.name}: ${m.approval.title}`));bt.onclick=()=>openThread(a.id);li.append(bt);ol.append(li)});
 if(ol.children.length)br.append(ol)}
// floating "N chưa đọc" pill: shows when unread rows sit below the fold
function updatePill(){const sc=$("#listScroll"),pill=$("#unreadPill");const items=[...document.querySelectorAll("#rows .row")].filter(r=>r.querySelector(".dot"));
 const fold=sc.getBoundingClientRect().bottom-80,below=items.filter(r=>r.getBoundingClientRect().top>fold);
 pill.classList.toggle("hidden",!below.length);if(below.length){pill.replaceChildren(I("arrow-up",16),document.createTextNode(` ${below.length} chưa đọc`));pill.firstChild.style.transform="rotate(180deg)";pill.onclick=()=>{below[0].scrollIntoView({block:"center",behavior:matchMedia("(prefers-reduced-motion: reduce)").matches?"auto":"smooth"});buzz("tap")}}}
$("#listScroll").addEventListener("scroll",()=>{if(!listQueued)requestAnimationFrame(updatePill)},{passive:true});
let briefBusy=false;async function refreshBrief(){if(briefBusy)return;briefBusy=true;toast("Đang tạo bản tin…");try{S.brief=await api("/brief",{});renderList();toast("Đã cập nhật bản tin")}catch{toast("Không tạo được bản tin")}finally{briefBusy=false}}
let openRow=null,suppress=0;const closeRow=li=>{li.querySelector(".rowin").style.transform="";delete li.dataset.open;if(openRow===li)openRow=null};
function swipeRow(li,inn){let x0,y0,dx=0,mode=null,base=0;
 li.addEventListener("touchstart",e=>{const t=e.touches[0];if(cur){mode="skip";x0=null;return}x0=t.clientX;y0=t.clientY;dx=0;mode=null;base=li.dataset.open?-160:0;if(openRow&&openRow!==li)closeRow(openRow)},{passive:true});
 li.addEventListener("touchmove",e=>{if(x0==null)return;const t=e.touches[0],ddx=t.clientX-x0,ddy=t.clientY-y0;if(!mode){if(Math.abs(ddx)>8&&Math.abs(ddx)>Math.abs(ddy))mode="x";else if(Math.abs(ddy)>8)mode="y"}if(mode!=="x")return;e.preventDefault();dx=ddx;li.classList.add("drag");inn.style.transform=`translate3d(${Math.max(-200,Math.min(0,base+dx))}px,0,0)`},{passive:false});
 li.addEventListener("touchend",()=>{li.classList.remove("drag");if(mode!=="x")return;const p=base+dx;if(p<-70){inn.style.transform="translate3d(-160px,0,0)";li.dataset.open=1;openRow=li;buzz("tap")}else closeRow(li);
  if(Math.abs(dx)>10)suppress=performance.now()+400});
 li.addEventListener("touchcancel",()=>{li.classList.remove("drag");closeRow(li)});
}
// ---------- thread ----------
let cur=null;
const stateTxt={working:"Đang làm việc…",waiting:"Chờ bạn duyệt",idle:"Sẵn sàng",done:"Đã xong",error:"Có lỗi"};
const stateLine=a=>isGroup(a)&&a.status!=="working"&&!waiting(a)?(a.members||[]).map(id=>byId(id)?.name).filter(Boolean).join(", "):a.status==="working"&&a.snip&&a.snip.startsWith("Bước")?a.snip:waiting(a)?"Chờ bạn duyệt":stateTxt[a.status]||"";
function openThread(id){const a=byId(id);if(!a)return;closeMenu();cur=a;markSeen(id);$("#threadAvatar").replaceChildren(avatarOf(a,32));$("#threadName").textContent=a.name;$("#threadState").textContent=stateLine(a);$("#input").placeholder=isGroup(a)?"Hỏi cả nhóm":"Hỏi "+a.name;closeFind();renderThread();loadMsgs(id).catch(()=>toast("Không tải được hội thoại"));$("#thread").classList.remove("off");$("#list").classList.add("under");buzz("tap");const hs=history.state;if(hs&&hs.spent)history.replaceState({t:1},"","#"+id);else if(!(hs&&hs.t))history.pushState({t:1},"","#"+id)}
function closeThread(fromPop){if(!cur)return;if(profId)closeProfile(true);closeFind();markSeen(cur.id);cur=null;stopVoice();$("#thread").classList.add("off");$("#list").classList.remove("under");$("#thread").style.transform="";$("#list").style.transform="";renderList();if(!fromPop&&history.state&&history.state.t)history.replaceState({spent:1},"",location.pathname)}
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
function speaker(box,m,last){if(!isGroup(cur)||m.role!=="agent"||!m.from)return;if(last.v===m.from)return;last.v=m.from;const b=byId(m.from);const h=el("div","speaker");h.append(face(b&&b.av,20),el("span",0,b?b.name:"Bot"));box.append(h)}
function renderThread(){const a=cur;if(!a)return;$("#threadState").textContent=stateLine(a);const box=$("#messages");
 const keepEdit=[...box.querySelectorAll(".ap-body")].map(t=>[t.closest(".approval").dataset.id,t.value,t.closest(".approval").querySelector(".ap-subject")?.value]);
 box.innerHTML="";const L=S.msgs[a.id]||[];const last={v:null};
 L.forEach((m,idx)=>{if(m.role!=="agent")last.v=null;if(m.text){speaker(box,m,last);const b=el("div","msg"+(m.role==="user"?" mine":m.role==="system"?" sys":""),m.text);if(m.queued){b.append(el("span","chip","Đang chờ lượt"))}box.append(b)}
  if(m.plan)box.append(planCard(m));
  if(m.choices&&m.choices.length){const wrap=el("div","choices");const later=L.slice(idx+1).some(x=>x.role==="user");m.choices.forEach(ch=>{const b=el("button","choice",ch);b.disabled=later||usedChoice.has(m.id);b.onclick=async()=>{if(usedChoice.has(m.id))return;usedChoice.add(m.id);renderThread();if(!(await sendMsg(ch)))usedChoice.delete(m.id)};wrap.append(b)});box.append(wrap)}
  if(m.approval)box.append(approvalCard(m))});
 keepEdit.forEach(([id,v,sj])=>{const c=box.querySelector(`.approval[data-id="${id}"]`);if(c){const t=c.querySelector(".ap-body");if(t)t.value=v;const s2=c.querySelector(".ap-subject");if(s2&&sj!=null)s2.value=sj}});
 Object.entries(S.drafts).forEach(([id,d])=>{if(d.agentId===a.id){speaker(box,{role:"agent",from:d.from},last);const b=el("div","msg draft",visible(d.text));b.id="d_"+id;box.append(b)}});
 if(a.status==="working"&&!Object.values(S.drafts).some(d=>d.agentId===a.id))box.append(el("div","msg typing dots","Đang nghĩ"));
 setSendMode();
 if(!box.contains(document.activeElement)||document.activeElement===box)requestAnimationFrame(()=>box.scrollTop=box.scrollHeight)}
const visible=t=>{const i=t.search(/(^|\n)\s*(APPROVAL|PLAN|CHOICES):/);return(i>=0?t.slice(0,i):t).trim()||"…"};
function paintDraft(id){const n=document.getElementById("d_"+id);if(!n){renderThread();return}n.textContent=visible(S.drafts[id].text);const box=$("#messages");box.scrollTop=box.scrollHeight}
async function decide(m,v,bs,extra){if(deciding.has(m.id))return;deciding.add(m.id);bs.querySelectorAll("button").forEach(b=>b.disabled=true);
 try{await api(`/approvals/${m.id}`,{decision:v,...(extra||{})});buzz(v==="approved"?"confirm":"soft");editing.delete(m.id)}catch(e){if(e.status!==409){deciding.delete(m.id);toast("Không ghi nhận được, thử lại");bs.querySelectorAll("button").forEach(b=>b.disabled=false)}}}
// send ↔ stop in the same spot while the agent works (stopping holds queued follow-ups)
function setSendMode(){const b=$("#composer .send");const empty=!$("#input").value.trim(),run=!!(cur&&cur.status==="working")&&empty,voice=empty&&!run;b.classList.toggle("stop",run);b.classList.toggle("voice",voice);b.replaceChildren(I(run?"square":voice?"audio-lines":"arrow-up"));b.setAttribute("aria-label",run?"Dừng":voice?"Nói":"Gửi")}
let stopping=false;async function stopAgent(){if(!cur||stopping)return;stopping=true;try{await api(`/agents/${cur.id}/stop`,{});buzz("soft");toast("Đã dừng")}catch{toast("Không dừng được")}finally{stopping=false}}
let sending=false;
async function sendMsg(text){const a=cur;text=text.trim();if(!a||!text||sending)return false;sending=true;
 try{await api(`/agents/${a.id}/messages`,{text});buzz("tap");return true}catch{toast("Không gửi được, thử lại");return false}finally{sending=false}}
// edge-swipe back (iOS style), anywhere on screen while a thread is open
(()=>{const th=$("#thread"),ls=$("#list"),root=$("#app");let x0=null,y0,dx=0,on=false;
 root.addEventListener("touchstart",e=>{const t=e.touches[0];if(!cur||sheetOpen||profId||t.clientX>28){x0=null;return}x0=t.clientX;y0=t.clientY;dx=0;on=false},{passive:true});
 root.addEventListener("touchmove",e=>{if(x0==null)return;const t=e.touches[0];dx=Math.max(0,t.clientX-x0);if(!on&&dx>10&&dx>Math.abs(t.clientY-y0))on=true;if(!on)return;e.preventDefault();th.classList.add("drag");ls.classList.add("drag");th.style.transform=`translate3d(${dx}px,0,0)`;ls.style.transform=`translate3d(${-24+24*dx/innerWidth}%,0,0)`},{passive:false});
 const end=()=>{if(x0==null)return;th.classList.remove("drag");ls.classList.remove("drag");if(on&&dx>innerWidth*.33)closeThread();else{th.style.transform="";ls.style.transform=""}x0=null};
 root.addEventListener("touchend",end);root.addEventListener("touchcancel",end)})();
// ---------- sheets (spring-ish open, drag-to-dismiss, scrim) ----------
const scrim=$("#scrim");let sheetOpen=null,openedAt=0,ghost=0;
document.addEventListener("click",e=>{if(performance.now()<ghost&&!(sheetOpen&&sheetOpen.contains(e.target))){e.stopImmediatePropagation();e.preventDefault()}},true);
function openSheet(s){if(sheetOpen)return;closeMenu();openedAt=performance.now();sheetOpen=s;scrim.classList.remove("hidden");s.classList.remove("hidden");s.classList.add("closed");s.offsetHeight;s.classList.remove("closed");buzz("tap")}
function closeSheet(force){const s=sheetOpen;if(!s||(!force&&performance.now()-openedAt<350))return;sheetOpen=null;ghost=performance.now()+400;s.style.transform="";s.classList.add("closed");scrim.classList.add("hidden");document.activeElement?.blur?.();setTimeout(()=>{if(sheetOpen!==s)s.classList.add("hidden")},330)}
// swap one sheet for another (settings → connectors / themes)
function swapSheet(next){const s=sheetOpen;if(s){sheetOpen=null;s.classList.add("closed");setTimeout(()=>{if(sheetOpen!==s)s.classList.add("hidden")},330)}openedAt=0;openSheet(next)}
scrim.onclick=()=>closeSheet();
document.querySelectorAll(".sheet").forEach(s=>{let y0=null,dy=0;const start=e=>{const sc=e.target.closest(".scroll");if(sc&&sc.scrollTop>0)return;if(e.target.closest("input,textarea"))return;y0=e.touches[0].clientY;dy=0};
 s.addEventListener("touchstart",start,{passive:true});
 s.addEventListener("touchmove",e=>{if(y0==null)return;dy=e.touches[0].clientY-y0;if(dy<=0){s.style.transform="";return}if(e.target.closest(".scroll")&&dy<12)return;e.preventDefault();s.classList.add("drag");s.style.transform=`translate3d(0,${dy}px,0)`},{passive:false});
 const end=()=>{if(y0==null)return;s.classList.remove("drag");if(dy>120)closeSheet(true);else s.style.transform="";y0=null};s.addEventListener("touchend",end);s.addEventListener("touchcancel",end)});
// ---------- "+" popover ----------
const MENUS=["#newMenu","#threadMenu","#profMenu","#plusMenu"].map(q=>$(q));let menuFor=null;
function closeMenu(){MENUS.forEach(m=>m.classList.add("hidden"));if(menuFor)menuFor.setAttribute("aria-expanded","false");menuFor=null}
function toggleMenu(m,btn){const was=!m.classList.contains("hidden");closeMenu();if(was||sheetOpen)return;const r=btn.getBoundingClientRect();m.classList.remove("hidden");
 if(m.classList.contains("up")){m.style.top="auto";m.style.bottom=(innerHeight-r.top+8)+"px";m.style.left=Math.max(12,r.left)+"px";m.style.right="auto";m.style.transformOrigin="bottom left"}
 else{m.style.top=(r.bottom+8)+"px";m.style.right=Math.max(12,innerWidth-r.right)+"px";m.style.left="auto";m.style.bottom="auto"}
 menuFor=btn;btn.setAttribute("aria-expanded","true");buzz("tap")}
$("#newBtn").onclick=e=>{e.stopPropagation();toggleMenu($("#newMenu"),$("#newBtn"))};
document.addEventListener("pointerdown",e=>{if(menuFor&&!MENUS.some(m=>m.contains(e.target))&&!menuFor.contains(e.target))closeMenu()},true);
document.addEventListener("keydown",e=>{if(e.key==="Escape"){closeMenu();if(sheetOpen)closeSheet(true)}});
// ---------- create bot ----------
let draft={shape:0,color:0};
function renderCreator(){const p=$("#preview");p.replaceChildren(face(draft,132));p.style.setProperty("--tint",COLORS[draft.color]);
 const sh=$("#shapes");sh.innerHTML="";SHAPES.forEach((_,i)=>{const b=el("button","shape");b.setAttribute("aria-label","Hình "+(i+1));b.setAttribute("aria-pressed",draft.shape===i);b.append(face({shape:i,color:draft.color},40));b.onclick=()=>{draft.shape=i;buzz("tap");renderCreator()};sh.append(b)});
 const co=$("#colors");co.innerHTML="";COLORS.forEach((c,i)=>{const b=el("button","color");b.setAttribute("aria-label","Màu "+(i+1));b.setAttribute("aria-pressed",draft.color===i);const d=el("i");d.style.background=c;b.append(d);b.onclick=()=>{draft.color=i;buzz("tap");renderCreator()};co.append(b)});
 syncCreate()}
const syncCreate=()=>{$("#createBtn").disabled=!$("#agentName").value.trim()};
$("#agentName").oninput=syncCreate;
let editing_bot=null;
function openCreator(edit){if(sheetOpen)return;editing_bot=edit||null;draft=edit?{shape:edit.av?.shape|0,color:edit.av?.color|0}:{shape:Math.random()*SHAPES.length|0,color:Math.random()*COLORS.length|0};$("#agentName").value=edit?edit.name:"";$("#agentDesc").value=edit?edit.desc||"":"";$("#sheetTitle").textContent=edit?"Sửa bot":"Bot mới";$("#createBtn").textContent=edit?"Lưu":"Tạo bot";renderCreator();$("#sheetBody").scrollTop=0;openSheet($("#sheet"))}
$("#menuBot").onclick=()=>{closeMenu();openCreator()};
$("#cancelBtn").onclick=()=>closeSheet(true);
let creating=false;
$("#createBtn").onclick=async()=>{if(sheetOpen!==$("#sheet")||creating)return;const name=$("#agentName").value.trim(),desc=$("#agentDesc").value.trim();if(!name)return;creating=true;$("#createBtn").disabled=true;
 try{const ed=editing_bot;const a=await api(ed?`/agents/${ed.id}`:"/agents",{name,desc,av:{...draft}});upsert(a);markSeen(a.id);closeSheet(true);renderList();if(ed){if(cur&&cur.id===a.id){cur=a;$("#threadName").textContent=a.name;$("#threadAvatar").replaceChildren(avatarOf(a,32))}if(profId===a.id)renderProfile()}buzz("confirm");toast((ed?"Đã lưu ":"Đã tạo ")+a.name)}catch{toast(editing_bot?"Không lưu được":"Không tạo được bot");syncCreate()}finally{creating=false}};
$("#agentName").onkeydown=e=>{if(e.key==="Enter"){e.preventDefault();$("#createBtn").click()}};
// ---------- new group chat ----------
let picked=[];
function renderGroup(){const q=$("#groupSearch").value.trim().toLowerCase();const ch=$("#groupChips");ch.innerHTML="";
 picked.forEach(id=>{const b=byId(id);if(!b)return;const c=el("button","gchip");c.append(face(b.av,20),el("span",0,b.name));c.setAttribute("aria-label","Bỏ "+b.name);c.onclick=()=>{picked=picked.filter(x=>x!==id);renderGroup()};ch.append(c)});
 const ul=$("#groupList");ul.innerHTML="";const list=bots().filter(b=>!q||b.name.toLowerCase().includes(q)).sort((a,b)=>a.name.localeCompare(b.name,"vi"));
 if(!list.length)ul.append(el("li","note",bots().length<2?"Cần ít nhất 2 bot để tạo nhóm.":"Không tìm thấy bot."));
 list.forEach(b=>{const li=el("li"),bt=el("button","pick-row");const on=picked.includes(b.id);bt.setAttribute("aria-pressed",on);bt.append(face(b.av,40));const t=el("span","pt");t.append(el("b",0,b.name),el("small",0,b.desc||""));bt.append(t);const ck=el("span","ck");if(on)ck.append(I("check",16));bt.append(ck);
  bt.onclick=()=>{if(on)picked=picked.filter(x=>x!==b.id);else if(picked.length<8)picked.push(b.id);else toast("Tối đa 8 bot");buzz("tap");renderGroup()};li.append(bt);ul.append(li)});
 $("#groupNext").disabled=picked.length<2}
$("#menuGroup").onclick=()=>{closeMenu();if(sheetOpen)return;picked=[];$("#groupSearch").value="";$("#groupName").value="";renderGroup();openSheet($("#groupSheet"))};
$("#groupSearch").oninput=renderGroup;$("#closeGroup").onclick=()=>closeSheet(true);
let grouping=false;
$("#groupNext").onclick=async()=>{if(grouping||picked.length<2)return;grouping=true;$("#groupNext").disabled=true;const name=$("#groupName").value.trim()||picked.map(id=>byId(id)?.name).filter(Boolean).join(", ").slice(0,40);
 try{const g=await api("/groups",{name,bots:picked});upsert(g);markSeen(g.id);closeSheet(true);renderList();buzz("confirm");setTimeout(()=>openThread(g.id),340)}catch{toast("Không tạo được nhóm");$("#groupNext").disabled=false}finally{grouping=false}};
// ---------- settings (grouped inset cards) ----------
let haptics=store.get("neko-aurora-haptics",true);
function cell(icon,label,opt={}){const b=el(opt.toggle!=null||opt.static?"div":"button","cell"+(opt.danger?" danger":""));if(opt.id)b.id=opt.id;const ic=el("span","cell-ic"+(opt.tone?" "+opt.tone:""));ic.append(I(icon,18));b.append(ic);const t=el("span","cell-t");t.append(el("span",0,label));if(opt.sub)t.append(el("small",0,opt.sub));b.append(t);
 if(opt.value)b.append(el("span","cell-v",opt.value));
 if(opt.toggle!=null){const sw=el("button","switch");sw.setAttribute("role","switch");sw.setAttribute("aria-checked",!!opt.toggle);sw.setAttribute("aria-label",label);if(opt.locked)sw.disabled=true;sw.onclick=()=>opt.onToggle&&opt.onToggle(sw);b.append(sw)}
 else if(!opt.static&&!opt.danger){const c=el("span","chev");c.append(I(opt.ext?"arrow-up-right":"chevron-right",16));b.append(c)}
 if(opt.onClick)b.onclick=opt.onClick;return b}
const card=(title,...cells)=>{const w=el("section","card-group");if(title)w.append(el("h3",0,title));const c=el("div","card");cells.forEach(x=>c.append(x));w.append(c);return w};
function connectedCount(){return S.outbound.filter(o=>o.configured&&(o.id!=="push"||pushOn)).length}
function renderSettings(){const B=$("#settingsBody");B.innerHTML="";
 const acc=el("div","account");const m=el("img");m.src="/app/icon.svg";m.alt="";m.width=m.height=52;const at=el("div");at.append(el("b",0,"Chủ sở hữu"),el("small",0,location.host));acc.append(m,at);B.append(card(null,acc));
 B.append(card("Mức dùng",cell("sparkles","Mô hình AI",{static:1,value:S.ai.configured?(S.ai.model||"đã cấu hình"):"chưa cấu hình"}),cell("activity","Lõi chạy",{static:1,value:S.ai.runtime||"Neko Core"}),cell("users","Bot & nhóm",{static:1,value:`${bots().length} bot · ${S.agents.length-bots().length} nhóm`})));
 B.append(card("Ứng dụng",cell("layout-grid","Kết nối ứng dụng",{id:"openConnectors",value:connectedCount()+" đã kết nối",onClick:()=>{renderConnectors();swapSheet($("#connectors"))}})));
 B.append(card("Bot",cell("check","Luôn duyệt trước khi gửi",{toggle:true,locked:true,sub:"Không gì rời máy chủ khi bạn chưa bấm Duyệt"}),
  cell("bell","Thông báo",{id:"notifyCell",toggle:pushOn||notifOn,sub:pushOn?"Thông báo đẩy đang bật":notifOn?"Khi app đang mở":"Báo khi bot cần bạn",onToggle:sw=>{if(sw.getAttribute("aria-checked")==="true"){toast("Tắt trong Cài đặt của máy");return}enableNotify().then(renderSettings).catch(()=>toast("Không bật được thông báo"))}}),
  cell("smartphone","Phản hồi rung",{id:"hapticsCell",toggle:haptics,sub:"Rung nhẹ khi chạm (Android)",onToggle:sw=>{haptics=!haptics;store.set("neko-aurora-haptics",haptics);sw.setAttribute("aria-checked",haptics);buzz("tap")}}),
  cell("sun-moon","Giao diện",{id:"openThemes",value:(THEMES.find(t=>t.id===curTheme())||THEMES[0]).name,onClick:()=>{renderThemes();swapSheet($("#themes"))}})));
 const link=(icon,label,href)=>cell(icon,label,{ext:1,onClick:()=>window.open(href,"_blank","noopener")});
 B.append(card("Trợ giúp",link("message-circle","Trung tâm trợ giúp","https://github.com/meiiie/neko-aurora#readme"),cell("check","Quyền riêng tư",{onClick:()=>toast("Dữ liệu nằm trên máy chủ của bạn; không gửi gì ra ngoài khi chưa duyệt.")}),link("git-branch","Giấy phép mã nguồn mở","/app/aurora/lucide-LICENSE.txt")));
 B.append(card(null,cell("log-out","Đăng xuất",{id:"logoutBtn",danger:1,onClick:logout})));
 const ft=el("footer","about");const lg=el("img");lg.src="/app/icon.svg";lg.alt="";lg.width=lg.height=36;ft.append(lg,el("b",0,"Neko Aurora"),el("small",0,"Phiên bản "+VERSION));B.append(ft)}
async function logout(){try{await fetch("/app/logout",{method:"POST",credentials:"same-origin",headers:{"Content-Type":"application/json"},body:"{}"})}catch{}closeSheet(true);if(es){es.close();es=null}const r=await fetch(API+"/state",{credentials:"same-origin",cache:"no-store"}).catch(()=>null);if(r&&r.status===401)$("#login").classList.remove("hidden");else toast("Máy chủ cục bộ: phiên luôn mở trên máy này")}
$("#meBtn").onclick=()=>{if(sheetOpen)return;renderSettings();openSheet($("#settings"))};$("#closeSettings").onclick=()=>closeSheet(true);
// ---------- connectors ----------
const CATS=[["all","Tất cả"],["send","Gửi đi"],["notify","Thông báo"],["work","Lịch & việc"],["chat","Nhắn tin"],["code","Mã nguồn"]];
const CONNS=[{id:"email",name:"Email (SMTP)",desc:"Gửi email đã duyệt từ địa chỉ của bạn",icon:"mail",cat:"send",real:1},{id:"push",name:"Thông báo đẩy",desc:"Báo trên máy khi bot cần bạn duyệt",icon:"bell",cat:"notify",real:1},
 {id:"telegram",name:"Telegram",desc:"Nhắn bot qua Telegram",icon:"send",cat:"chat"},{id:"zalo",name:"Zalo",desc:"Nhận và trả lời tin Zalo",icon:"message-circle",cat:"chat"},{id:"slack",name:"Slack",desc:"Đăng tin vào kênh làm việc",icon:"hash",cat:"chat"},
 {id:"calendar",name:"Lịch",desc:"Đọc lịch, đề xuất giờ họp",icon:"calendar",cat:"work"},{id:"x",name:"X",desc:"Soạn bài đăng để bạn duyệt",icon:"at-sign",cat:"send"},{id:"github",name:"GitHub",desc:"Theo dõi issue và pull request",icon:"git-branch",cat:"code"}];
let connCat="all";
function connState(c){if(!c.real)return{label:"Sắp có",soon:1};const o=S.outbound.find(x=>x.id===c.id)||{};
 if(c.id==="email")return o.configured?{label:"Đã kết nối",on:1,sub:o.from?"Gửi từ "+o.from:""}:{label:"Thiết lập",act:()=>toast("Đặt AURORA_SMTP_HOST, _USER, _PASSWORD, _FROM trên máy chủ rồi khởi động lại")};
 if(c.id==="push")return pushOn?{label:"Đã bật",on:1}:o.configured?{label:"Kết nối",act:()=>enableNotify().then(renderConnectors)}:{label:"Thiết lập",act:()=>toast("Máy chủ chưa có khoá VAPID")};return{label:"Sắp có",soon:1}}
function renderConnectors(){const q=$("#connSearch").value.trim().toLowerCase();$("#connCount").textContent=connectedCount()+" đã kết nối";
 const cs=$("#connCats");cs.innerHTML="";CATS.forEach(([id,n])=>{const b=el("button","cat",n);b.setAttribute("aria-pressed",connCat===id);b.onclick=()=>{connCat=id;buzz("tap");renderConnectors()};cs.append(b)});
 const ul=$("#connList");ul.innerHTML="";CONNS.filter(c=>(connCat==="all"||c.cat===connCat)&&(!q||c.name.toLowerCase().includes(q)||c.desc.toLowerCase().includes(q))).forEach(c=>{const st=connState(c);const li=el("li","conn");li.dataset.id=c.id;
  const ic=el("span","conn-ic");ic.append(I(c.icon,22));const t=el("span","pt");t.append(el("b",0,c.name),el("small",0,st.sub||c.desc));
  const b=el("button","conn-btn"+(st.on?" on":"")+(st.soon?" soon":""),st.label);if(st.soon||st.on)b.disabled=true;if(st.act)b.onclick=()=>{buzz("tap");st.act()};li.append(ic,t,b);ul.append(li)});
 if(!ul.children.length)ul.append(el("li","note","Không có kết nối phù hợp."))}
$("#connSearch").oninput=renderConnectors;$("#closeConnectors").onclick=()=>closeSheet(true);
$("#appsBtn").onclick=()=>{if(sheetOpen)return;renderConnectors();openSheet($("#connectors"))};
// ---------- activity ----------
function renderLog(items,prepend){const L=$("#log");if(!prepend)L.innerHTML="";items.forEach(x=>{const li=el("li"),t=el("time",0,ago(x.t));li.append(t,x.text);prepend?L.prepend(li):L.append(li)})}
$("#activityBtn").onclick=()=>{closeMenu();if(sheetOpen)return;renderLog([]);openSheet($("#activity"));api("/activity?limit=80").then(x=>renderLog(x)).catch(()=>toast("Không tải được hoạt động"))};$("#closeActivity").onclick=()=>closeSheet(true);
// ---------- notifications (Web Push via VAPID; falls back to SW notifications while the app is open) ----------
let pushOn=false,notifOn=false;
const u8=b=>{const s=atob((b+"=".repeat((4-b.length%4)%4)).replace(/-/g,"+").replace(/_/g,"/"));return Uint8Array.from(s,c=>c.charCodeAt(0))};
async function enableNotify(){if(!("Notification"in window)||!("serviceWorker"in navigator)){toast("Thiết bị này chưa hỗ trợ thông báo. Trên iPhone: thêm vào Màn hình chính trước.");return}
 const p=await Notification.requestPermission();if(p!=="granted"){toast("Bạn chưa cho phép thông báo");return}
 const reg=await navigator.serviceWorker.ready;
 if("PushManager"in window&&S.vapid){try{const sub=(await reg.pushManager.getSubscription())||await reg.pushManager.subscribe({userVisibleOnly:true,applicationServerKey:u8(S.vapid)});await api("/push/subscribe",sub.toJSON());pushOn=true;toast("Đã bật thông báo đẩy")}catch{toast("Bật thông báo khi app đang mở (push chưa khả dụng)")}}
 else toast("Bật thông báo khi app đang mở");notifOn=true}
// ---------- voice input (Web Speech API where supported) ----------
const SR=window.SpeechRecognition||window.webkitSpeechRecognition;let rec=null;
function stopVoice(){if(rec){try{rec.stop()}catch{}rec=null;$("#micBtn").classList.remove("on")}}
if(!SR)$("#micBtn").classList.add("hidden");
$("#micBtn").onclick=()=>{if(rec){stopVoice();return}rec=new SR();rec.lang="vi-VN";rec.interimResults=true;rec.continuous=false;const base=inp.value?inp.value+" ":"";
 rec.onresult=e=>{let t="";for(const r of e.results)t+=r[0].transcript;inp.value=base+t;inp.oninput()};rec.onend=()=>{rec=null;$("#micBtn").classList.remove("on")};rec.onerror=e=>{toast(e.error==="not-allowed"?"Chưa cho phép micro":"Không nghe được, thử lại")};
 try{rec.start();$("#micBtn").classList.add("on")}catch{rec=null}};
// ---------- themes ----------
const THEMES=[{id:"light",name:"Sáng",sub:"Xám trung tính, nút đen · mặc định",sw:["#f7f7f7","#ffffff","#0d0d0d"],tc:"#f7f7f7"},{id:"dark",name:"Tối",sub:"Nền đen, chữ trắng",sw:["#000000","#1c1c1e","#f5f5f5"],tc:"#000000"},{id:"cream",name:"Kem Aurora",sub:"Kem sáng, khăn đỏ",sw:["#f4efe4","#e0261f","#17130e"],tc:"#f4efe4"},{id:"gold",name:"Đen vàng",sub:"Nền đen, điểm vàng · tối",sw:["#0d0c0a","#d4a72c","#f5ecd7"],tc:"#0d0c0a"},{id:"blue",name:"Trắng xanh",sub:"Nền trắng, điểm xanh · sáng",sw:["#f5f8fc","#1d5fd1","#0f1b2d"],tc:"#f5f8fc"}];
const curTheme=()=>{try{const t=localStorage.getItem("neko-aurora-theme");return THEMES.some(x=>x.id===t)?t:"light"}catch{return"light"}};
function applyTheme(id){const t=THEMES.find(x=>x.id===id)||THEMES[0];try{t.id==="light"?localStorage.removeItem("neko-aurora-theme"):localStorage.setItem("neko-aurora-theme",t.id)}catch{}
 if(t.id==="light")delete document.documentElement.dataset.theme;else document.documentElement.dataset.theme=t.id;
 document.querySelectorAll('meta[name="theme-color"]').forEach(m=>m.content=t.tc);
 const cs=document.querySelector('meta[name="color-scheme"]');if(cs)cs.content=t.id==="gold"||t.id==="dark"?"dark":"light";renderThemes()}
function renderThemes(){const L=$("#themeList");L.innerHTML="";const c=curTheme();THEMES.forEach(t=>{const b=el("button","theme-opt");b.setAttribute("aria-pressed",c===t.id);b.dataset.theme=t.id;const d=el("span","dots3");t.sw.forEach(x=>{const i=el("i");i.style.background=x;d.append(i)});const tx=el("span");tx.append(el("b",0,t.name),el("small",0,t.sub));b.append(d,tx);b.onclick=()=>applyTheme(t.id);L.append(b)})}
$("#closeThemes").onclick=()=>closeSheet(true);
// ---------- bot profile ----------
let profId=null,skipPop=false,muted=store.get("neko-aurora-muted",[]);
const shareLink=async a=>{const url=location.origin+"/app/#"+a.id;try{if(navigator.share){await navigator.share({title:a.name,text:a.name+" · Neko Aurora",url});return}}catch(e){if(e&&e.name==="AbortError")return}
 try{await navigator.clipboard.writeText(url);toast("Đã sao chép liên kết")}catch{toast(url)}};
function openProfile(id){const a=byId(id);if(!a)return;closeMenu();profId=id;renderProfile();if(!S.msgs[id])loadMsgs(id).then(()=>profId===id&&renderProfile()).catch(()=>{});$("#profile").classList.remove("off");$("#profBody").scrollTop=0;buzz("tap");history.pushState({p:1},"",location.hash||"#"+id)}
function closeProfile(fromPop){if(!profId)return;profId=null;closeMenu();$("#profile").classList.add("off");if(!fromPop&&history.state&&history.state.p){skipPop=true;history.back()}}
function renderProfile(){const a=byId(profId);const B=$("#profBody");if(!a){B.innerHTML="";return}B.innerHTML="";
 const hero=el("div","hero");const av=el("span","hero-av");av.append(avatarOf(a,112));const bd=a.pin?el("i","badge pin"):badge(a);if(a.pin)bd.append(I("star",12));if(bd)av.append(bd);hero.append(av,el("h2",0,a.name));if(a.desc)hero.append(el("p",0,a.desc));
 if(!isGroup(a)){const ed=el("button","pill soft","Sửa");ed.id="profEdit";ed.onclick=()=>openCreator(a);hero.append(ed)}B.append(hero);
 const L=S.msgs[a.id]||[],pend=S.pending.filter(m=>m.agentId===a.id).length;
 const toThread=()=>{closeProfile();if(!cur||cur.id!==a.id)setTimeout(()=>openThread(a.id),cur?0:0)};
 const rows=[cell("message-circle","Hội thoại",{id:"prChat",value:L.length?L.length+" tin":"",onClick:toThread}),cell("clock","Chờ duyệt",{id:"prPend",value:pend+" chờ",onClick:toThread})];
 if(isGroup(a))rows.push(cell("users","Thành viên",{static:1,value:(a.members||[]).map(id=>byId(id)?.name).filter(Boolean).join(", ")}));
 else rows.push(cell("users","Nhóm",{static:1,value:S.agents.filter(g=>isGroup(g)&&(g.members||[]).includes(a.id)).length+" nhóm"}));
 rows.push(cell("folder","Hoạt động",{id:"prLog",onClick:()=>{renderLog([]);openSheet($("#activity"));api("/activity?limit=80").then(x=>renderLog(x)).catch(()=>toast("Không tải được hoạt động"))}}));
 rows.push(cell("monitor","Máy chạy",{id:"prRun",value:a.status==="working"?"Đang làm":waiting(a)?"Chờ bạn":"Rảnh",onClick:()=>toast((S.ai.runtime||"Neko Core")+" · "+(S.ai.model||"AI chưa cấu hình"))}));
 B.append(card(null,...rows));
 const on=!muted.includes(a.id);const nc=card(null,cell("bell","Thông báo",{id:"prNotify",toggle:on,onToggle:sw=>{const now=sw.getAttribute("aria-checked")!=="true";muted=now?muted.filter(x=>x!==a.id):[...muted,a.id];store.set("neko-aurora-muted",muted);sw.setAttribute("aria-checked",now);buzz("tap");if(now&&!pushOn&&!notifOn)enableNotify().catch(()=>{})}}));
 B.append(nc,el("p","caption","Báo khi bot này xong việc hoặc đang chờ bạn (trên máy này)."));
 $("#pmPinT").textContent=a.pin?"Bỏ ghim":"Ghim";$("#pmPin").classList.toggle("hidden",isGroup(a))}
$("#profBack").onclick=()=>closeProfile();
$("#profShare").onclick=()=>{const a=byId(profId);if(a)shareLink(a)};
$("#profMore").onclick=e=>{e.stopPropagation();toggleMenu($("#profMenu"),$("#profMore"))};
$("#pmCopy").onclick=async()=>{closeMenu();const id=profId;try{await navigator.clipboard.writeText(id);toast("Đã sao chép ID")}catch{toast("ID: "+id)}};
$("#pmPin").onclick=()=>{closeMenu();const a=byId(profId);if(!a)return;api(`/agents/${a.id}`,{pin:!a.pin}).then(x=>{upsert(x);renderList();renderProfile();toast(x.pin?"Đã ghim":"Đã bỏ ghim")}).catch(()=>toast("Không ghim được"))};
// ---------- thread header menu, find, share, + menu ----------
$("#threadMore").onclick=e=>{e.stopPropagation();toggleMenu($("#threadMenu"),$("#threadMore"))};
$("#whoBtn").onclick=()=>cur&&openProfile(cur.id);
$("#tmProfile").onclick=()=>cur&&openProfile(cur.id);
$("#tmShare").onclick=()=>{closeMenu();if(cur)shareLink(cur)};
$("#plusBtn").onclick=e=>{e.stopPropagation();toggleMenu($("#plusMenu"),$("#plusBtn"))};
$("#pmGroup").onclick=()=>{closeMenu();if(sheetOpen)return;picked=cur&&!isGroup(cur)?[cur.id]:[];$("#groupSearch").value="";$("#groupName").value="";renderGroup();openSheet($("#groupSheet"))};
function closeFind(){$("#findBar").classList.add("hidden");$("#findInput").value="";document.querySelectorAll("#messages .hit").forEach(x=>x.classList.remove("hit"))}
function runFind(){const q=$("#findInput").value.trim().toLowerCase();const ms=[...document.querySelectorAll("#messages .msg")];ms.forEach(x=>x.classList.remove("hit"));if(!q){$("#findCount").textContent="";return}
 const hits=ms.filter(x=>x.textContent.toLowerCase().includes(q));hits.forEach(x=>x.classList.add("hit"));$("#findCount").textContent=hits.length?hits.length+" kết quả":"0";if(hits.length)hits.at(-1).scrollIntoView({block:"center"})}
$("#tmFind").onclick=()=>{closeMenu();$("#findBar").classList.remove("hidden");$("#findInput").focus()};
$("#findInput").oninput=runFind;$("#findClose").onclick=closeFind;
// ---------- misc ----------
$("#backBtn").onclick=()=>closeThread();
addEventListener("popstate",()=>{if(skipPop){skipPop=false;return}closeMenu();if(sheetOpen)closeSheet(true);else if(profId)closeProfile(true);else if(cur)closeThread(true)});
$("#search").oninput=e=>{filter=e.target.value;renderList()};
$("#search").onkeydown=e=>{if(e.key==="Enter")e.target.blur()};
const inp=$("#input");inp.oninput=()=>{inp.style.height="auto";inp.style.height=Math.min(140,inp.scrollHeight)+"px";setSendMode()};
$("#composer").onsubmit=async e=>{e.preventDefault();stopVoice();const v=inp.value;if(!v.trim()){if(cur&&cur.status==="working")stopAgent();else if(SR)$("#micBtn").click();else inp.focus();return}inp.value="";inp.style.height="";if(!(await sendMsg(v)))inp.value=v};
inp.onkeydown=e=>{if(e.key==="Enter"&&!e.shiftKey&&!e.isComposing){e.preventDefault();$("#composer").requestSubmit()}};
document.addEventListener("visibilitychange",()=>{if(!document.hidden&&es)loadState().catch(()=>{})});
if("serviceWorker"in navigator)navigator.serviceWorker.register("/app/sw.js",{scope:"/app/"}).then(r=>r.pushManager?.getSubscription()).then(s=>{if(s)pushOn=true}).catch(()=>{});
function boot(){return loadState().then(()=>{$("#login").classList.add("hidden");if(!es)connect();const h=location.hash.slice(1);if(h)openThread(h)}).catch(e=>{if(e.status===401){$("#login").classList.remove("hidden")}else toast("Không kết nối được máy chủ")})}
let logging=false;$("#loginForm").onsubmit=async e=>{e.preventDefault();if(logging)return;logging=true;try{const r=await fetch("/app/login",{method:"POST",credentials:"same-origin",headers:{"Content-Type":"application/json"},body:JSON.stringify({code:$("#loginCode").value})});if(!r.ok){toast(r.status===429?"Thử quá nhiều lần, đợi 5 phút":"Mã không đúng");return}$("#loginCode").value="";await boot()}catch{toast("Không kết nối được")}finally{logging=false}};
boot();
// ---------- Aurora icons (Lucide, ISC) + haptics ----------
function I(n,sz){return self.AURORA_ICONS?AURORA_ICONS.icon(n,sz):document.createTextNode("")}
function stepIcon(st){const m={pending:"circle",running:"loader-circle",done:"check",waiting:"pause",failed:"x",cancelled:"minus"};const s=el("span","ic");s.append(I(m[st]||"circle",16));return s}
// Haptics: rare and short; owner can turn them off; vibrate is a no-op on iOS Safari.
function buzz(k){try{if(!haptics||matchMedia("(prefers-reduced-motion: reduce)").matches||!navigator.vibrate)return;navigator.vibrate({tap:8,confirm:[10,40,14],soft:16}[k]||8)}catch{}}
[["#newBtn","plus"],["#threadMore","ellipsis"],["#plusBtn","plus"],["#profBack","chevron-left"],["#profShare","share"],["#profMore","ellipsis"],["#appsBtn","layout-grid"],["#searchIc","search"],["#backBtn","chevron-left"],["#micBtn","mic"],["#cancelBtn","x"],["#closeGroup","x"],["#closeSettings","x"],["#closeConnectors","x"]].forEach(([q,n])=>{const b=$(q);if(b)b.replaceChildren(I(n))});
document.querySelectorAll(".mi[data-i]").forEach(s=>s.replaceChildren(I(s.dataset.i,18)));
setSendMode();
window.__AURORA={state:()=>S,reload:loadState,current:()=>cur&&cur.id};
})();
