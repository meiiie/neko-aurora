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
const S={agents:[],pending:[],brief:null,ai:{},vapid:"",outbound:{},secrets:[],msgs:{},drafts:{}};
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
async function loadState(){const st=await api("/state");S.agents=st.agents;S.pending=st.pending;S.brief=st.brief;S.ai=st.ai;S.vapid=st.vapidPublicKey;S.outbound=st.outbound||{};S.secrets=st.secrets||[];S.pushSubs=st.pushSubscriptions|0;seedSeen();renderList();if(cur){const a=byId(cur.id);if(a){cur=a;await loadMsgs(a.id)}}}
async function loadMsgs(id){S.msgs[id]=await api(`/agents/${id}/messages`);if(cur&&cur.id===id)renderThread()}
// ---------- unread (device-local: last time each conversation was seen) ----------
const store={get(k,d){try{const v=localStorage.getItem(k);return v==null?d:JSON.parse(v)}catch{return d}},set(k,v){try{localStorage.setItem(k,JSON.stringify(v))}catch{}}};
let seen=store.get("neko-aurora-seen",null);
function seedSeen(){if(seen)return;seen={};const now=Date.now();S.agents.forEach(a=>seen[a.id]=now);store.set("neko-aurora-seen",seen)}
const markSeen=id=>{if(!seen)seen={};seen[id]=Date.now();store.set("neko-aurora-seen",seen)};
const unread=a=>!(cur&&cur.id===a.id)&&a.status!=="working"&&T(a.t)>(seen&&seen[a.id]!=null?seen[a.id]:0);
// ---------- live events (SSE) ----------
let es,esDown=false;
// offline state: one quiet pill; EventSource reconnects on its own and state reloads on reopen
let netT=null;function net(){const off=!navigator.onLine||(esDown&&es&&es.readyState!==1);const b=$("#netBanner");if(!b)return;b.textContent=!navigator.onLine?"Ngoại tuyến · tin nhắn sẽ chưa gửi được":"Mất kết nối · đang thử lại…";b.classList.toggle("hidden",!off)}
addEventListener("online",()=>{net();if(es&&es.readyState===2)connect()});addEventListener("offline",net);
function connect(){es=new EventSource(API+"/events");
 es.onopen=()=>{if(esDown){esDown=false;loadState().catch(()=>{})}net()};
 es.onerror=()=>{esDown=true;setTimeout(net,2500)};
 es.addEventListener("agent",e=>{const a=JSON.parse(e.data);if(seen&&seen[a.id]==null)markSeen(a.id);upsert(a);if(cur&&cur.id===a.id){cur=a;markSeen(a.id);renderThread()}renderListSoon()});
 es.addEventListener("archived",e=>{const {id}=JSON.parse(e.data);S.agents=S.agents.filter(a=>a.id!==id);S.pending=S.pending.filter(m=>m.agentId!==id);renderListSoon()});
 es.addEventListener("message",e=>{const m=JSON.parse(e.data);const L=S.msgs[m.agentId];delete S.drafts[m.id];
  if(L){const i=L.findIndex(x=>x.id===m.id);if(i<0)L.push(m);else L[i]=m}
  S.pending=S.pending.filter(x=>x.id!==m.id);if(m.approval&&m.approval.state==="pending")S.pending.push(m);
  if(cur&&cur.id===m.agentId)renderThread();renderListSoon()});
 es.addEventListener("delta",e=>{const d=JSON.parse(e.data);S.deltaCount=(S.deltaCount||0)+1;const x=S.drafts[d.id]||(S.drafts[d.id]={agentId:d.agentId,from:d.from,text:""});x.text=typeof d.text==="string"?d.text:x.text+d.delta;if(cur&&cur.id===d.agentId)paintDraft(d.id)});
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
 pinned.forEach(a=>{const b=el("button","feat");b.dataset.id=a.id;b.setAttribute("aria-label",a.name);const av=el("span","feat-av");av.append(avatarOf(a,92));const bd=badge(a);if(bd)av.append(bd);if(unread(a))av.append(el("i","dot"));b.append(av,el("span","feat-name",a.name));b.onclick=()=>openThread(a.id);fe.append(b)});
 renderBrief(q);
 const ul=$("#rows");ul.innerHTML="";
 rest.forEach(a=>{
  const li=el("li","row");li.dataset.id=a.id;
  if(!isGroup(a)){const acts=el("div","actions"),pin=el("button","pin",a.pin?"Bỏ ghim":"Ghim"),ar=el("button","arch","Lưu trữ");acts.append(pin,ar);li.append(acts);
   pin.onclick=e=>{e.stopPropagation();suppress=performance.now()+600;api(`/agents/${a.id}`,{pin:!a.pin}).then(upsert).then(()=>{buzz("tap");renderList()}).catch(()=>toast("Không ghim được"))};
   ar.onclick=e=>{e.stopPropagation();suppress=performance.now()+600;if(ar.disabled)return;ar.disabled=true;api(`/agents/${a.id}`,{archived:true}).then(()=>{S.agents=S.agents.filter(x=>x.id!==a.id);S.pending=S.pending.filter(m=>m.agentId!==a.id);renderList();toast("Đã lưu trữ "+a.name)}).catch(()=>{ar.disabled=false;toast("Không lưu trữ được")})}}
  const inn=el("div","rowin");inn.setAttribute("role","button");inn.tabIndex=0;const av=el("span","row-av");av.append(avatarOf(a,48));const bd=badge(a);if(bd)av.append(bd);inn.append(av);
  const tx=el("div","rowtxt"),h=el("div","rowh");h.append(el("b",0,a.name));if(isGroup(a))h.append(el("span","tag",(a.members||[]).length+" bot"));
  const live=a.status==="working",un=unread(a);h.append(el("span","time"+(live?" now":"")+(un?" un":""),live?"bây giờ":ago(a.t)));
  const sn=el("div","snip"+(waiting(a)?" wait":"")+(live?" dots":"")+(a.status==="error"?" err":"")+(un?" un":""),statusLine(a));
  tx.append(h,sn);inn.append(tx);if(un)inn.append(el("i","dot"));li.append(inn);ul.append(li);
  inn.onclick=()=>{if(performance.now()<suppress)return;if(li.dataset.open){closeRow(li);return}openThread(a.id)};inn.onkeydown=e=>{if(e.key==="Enter")openThread(a.id)};
  if(!isGroup(a))swipeRow(li,inn)});
 const em=$("#empty");em.classList.toggle("hidden",!!(pinned.length||rest.length));em.textContent=q?"Không tìm thấy “"+filter.trim()+"”.":"Chưa có bot nào. Bấm + để tạo.";
 updatePill()}
// morning brief: rendered as the first list row; tap to expand (full text, approvals waiting, refresh)
let briefOpen=false;
function renderBrief(q){const br=$("#brief");br.innerHTML="";br.classList.toggle("hidden",!!q);if(q)return;const b=S.brief;
 const head=el("button","rowin brief-head");head.setAttribute("aria-expanded",briefOpen);const av=el("span","row-av brief-av");const lg=el("img");lg.src="/app/icon.svg";lg.alt="";av.append(lg);if(S.pending.length){const n=el("i","badge wait");av.append(n)}
 const tx=el("div","rowtxt"),h=el("div","rowh");h.append(el("b",0,"Bản tin sáng"),el("span","time",b?ago(b.generated):""));tx.append(h,el("div","snip",S.pending.length?`${S.pending.length} việc chờ bạn duyệt`:(b?b.text:"Chưa có bản tin.")));head.append(av,tx);head.onclick=()=>{briefOpen=!briefOpen;buzz("tap");renderBrief("")};br.append(head);
 if(!briefOpen)return;const body=el("div","brief-body");body.append(el("p",0,b?b.text:"Chưa có bản tin."));if(b)body.append(el("small","meta",`${b.source==="ai"?"AI · "+(S.ai.model||""):"Tự động"}`+(S.ai.configured?"":" · AI chưa cấu hình")));
 const ol=el("ol");S.pending.forEach(m=>{const a=byId(m.agentId);if(!a)return;const li=el("li"),bt=el("button","brief-item");bt.append(el("span","bi-dot"),el("span",0,`${a.name}: ${m.approval.title}`));bt.onclick=()=>openThread(a.id);li.append(bt);ol.append(li)});if(ol.children.length)body.append(ol);
 const rf=el("button","pill soft small","Làm mới");rf.setAttribute("aria-label","Tạo lại bản tin sáng");rf.onclick=refreshBrief;body.append(rf);br.append(body)}
// floating "N chưa đọc" pill: shows when unread rows sit below the fold
function updatePill(){const sc=$("#listScroll"),pill=$("#unreadPill");const items=[...document.querySelectorAll("#rows .row")].filter(r=>r.querySelector(".dot"));
 const fold=sc.getBoundingClientRect().bottom-80,below=items.filter(r=>r.getBoundingClientRect().top>fold);
 pill.classList.toggle("hidden",!below.length);if(below.length){pill.replaceChildren(I("chevron-down",18),document.createTextNode(`${below.length} chưa đọc`));pill.onclick=()=>{below[0].scrollIntoView({block:"center",behavior:matchMedia("(prefers-reduced-motion: reduce)").matches?"auto":"smooth"});buzz("tap")}}}
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
// tool activity stays unobtrusive: a status line while running, folded into the
// "Nguồn" footer afterwards (states/dedup after CopilotKit/openmuse search-tool-card.tsx, MIT)
const srcHost=u=>{try{return new URL(u).hostname.replace(/^www\./,"")}catch{return u}};
function uniqSources(list){const seen=new Map();(list||[]).forEach((x,i)=>{if(!seen.has(x.url))seen.set(x.url,{...x,n:i+1})});return[...seen.values()]}
// ---------- markdown (safe: builds DOM nodes, never innerHTML) ----------
// headings, **bold**, *italic*, `code`, lists, tables, quotes, links, [n] citations
function mdInline(node,text,src){const re=/(\*\*[^*\n]+\*\*|__[^_\n]+__|`[^`\n]+`|\[[^\]\n]+\]\(https?:\/\/[^\s)]+\)|https?:\/\/[^\s<>()]+[^\s<>().,;:!?]|\[\d{1,2}\](?:\s*,?\s*\[\d{1,2}\])*|(?<![\w*])\*[^*\n]+\*(?![\w*])|(?<![\w_])_[^_\n]+_(?![\w_]))/g;let last=0,m;
 const link=(t,u)=>{const a=el("a","lnk",t);a.href=u;a.target="_blank";a.rel="noopener noreferrer";return a};
 while((m=re.exec(text))){if(m.index>last)node.append(text.slice(last,m.index));const t=m[0];
  if(t.startsWith("**")||t.startsWith("__")){const b=el("strong");mdInline(b,t.slice(2,-2),src);node.append(b)}
  else if(t[0]==="`")node.append(el("code",0,t.slice(1,-1)))
  else if(/^\[\d/.test(t)&&!t.includes("](")){t.match(/\d{1,2}/g).forEach(n=>{const x=src&&src[+n-1];if(!x){node.append("["+n+"]");return}const c=link(n,x.url);c.className="cite";c.setAttribute("aria-label","Nguồn "+n+": "+srcHost(x.url));node.append(c)})}
  else if(t[0]==="["){const k=t.indexOf("](");node.append(link(t.slice(1,k),t.slice(k+2,-1)))}
  else if(t.startsWith("http"))node.append(link(srcHost(t),t))
  else{const e=el("em");mdInline(e,t.slice(1,-1),src);node.append(e)}
  last=m.index+t.length}
 if(last<text.length)node.append(text.slice(last))}
function md(node,text,src){node.classList.add("md");const lines=text.replace(/\r/g,"").split("\n");let i=0;
 const isRow=l=>/^\s*\|.*\|\s*$/.test(l),cells=l=>l.trim().replace(/^\||\|$/g,"").split("|").map(c=>c.trim());
 while(i<lines.length){const l=lines[i];
  if(!l.trim()){i++;continue}
  let h=l.match(/^\s*(#{1,4})\s+(.*)$/);if(h){const e=el(h[1].length<=2?"h3":"h4");mdInline(e,h[2].replace(/\s*#+\s*$/,""),src);node.append(e);i++;continue}
  if(/^\s*([-*_])\s*\1\s*\1[\s\-*_]*$/.test(l)){node.append(el("hr"));i++;continue}
  if(isRow(l)&&i+1<lines.length&&/^\s*\|?\s*:?-{2,}/.test(lines[i+1])){const wrap=el("div","tbl"),t=el("table"),th=el("thead"),tr=el("tr");cells(l).forEach(c=>{const x=el("th");mdInline(x,c,src);tr.append(x)});th.append(tr);t.append(th);const tb=el("tbody");i+=2;
   while(i<lines.length&&isRow(lines[i])){const r=el("tr");cells(lines[i]).forEach(c=>{const x=el("td");mdInline(x,c,src);r.append(x)});tb.append(r);i++}t.append(tb);wrap.append(t);node.append(wrap);continue}
  if(/^\s*>\s?/.test(l)){const q=el("blockquote");const buf=[];while(i<lines.length&&/^\s*>\s?/.test(lines[i]))buf.push(lines[i++].replace(/^\s*>\s?/,""));mdInline(q,buf.join(" "),src);node.append(q);continue}
  const li=/^\s*(?:[-*•+]|(\d{1,2})[.)])\s+(.*)$/;let m=l.match(li);
  if(m){const ol=!!m[1],list=el(ol?"ol":"ul");if(ol&&+m[1]>1)list.start=+m[1];while(i<lines.length&&(m=lines[i].match(li))&&!!m[1]===ol){const it=el("li");mdInline(it,m[2],src);
     while(i+1<lines.length&&/^\s{2,}\S/.test(lines[i+1])&&!li.test(lines[i+1])){it.append(" ");mdInline(it,lines[++i].trim(),src)}list.append(it);i++}node.append(list);continue}
  const p=el("p"),buf=[l.trim()];i++;while(i<lines.length&&lines[i].trim()&&!li.test(lines[i])&&!/^\s*(#{1,4}\s|>|\|)/.test(lines[i]))buf.push(lines[i++].trim());
  buf.forEach((b,k)=>{if(k)p.append(el("br"));mdInline(p,b,src)});node.append(p)}}
function sourceList(m){const w=el("details","sources");const sm=el("summary");const u=uniqSources(m.sources);const ok=u.length>0;sm.append(el("span",0,ok?`Nguồn · ${u.length}`:"Không lấy được dữ liệu"),I("chevron-down",14));w.append(sm);
 (m.tools||[]).forEach(t=>{const r=el("p","tool-line"+(t.status==="failed"?" bad":""),(t.summary||t.name)+(t.ms?" · "+(t.ms/1000).toFixed(1).replace(".",",")+"s":""));w.append(r)});const ol=el("ol");
 u.forEach(x=>{const li=el("li");const l=el("a");l.href=x.url;l.target="_blank";l.rel="noopener noreferrer";l.append(el("b",0,"["+x.n+"] "+(x.title||srcHost(x.url))),el("small",0,srcHost(x.url)+(x.at?" · "+new Date(x.at).toLocaleTimeString("vi-VN",{hour:"2-digit",minute:"2-digit"}):"")));li.append(l);ol.append(li)});w.append(ol);return w}
function speaker(box,m,last){if(!isGroup(cur)||m.role!=="agent"||!m.from)return;if(last.v===m.from)return;last.v=m.from;const b=byId(m.from);const h=el("div","speaker");h.append(face(b&&b.av,20),el("span",0,b?b.name:"Bot"));box.append(h)}
function renderThread(){const a=cur;if(!a)return;$("#threadState").textContent=stateLine(a);const box=$("#messages");
 const keepSec=[...box.querySelectorAll(".secret input[data-secret]")].map(i=>[i.closest(".secret").dataset.id,i.value]);
 const keepEdit=[...box.querySelectorAll(".ap-body")].map(t=>[t.closest(".approval").dataset.id,t.value,t.closest(".approval").querySelector(".ap-subject")?.value]);
 box.innerHTML="";const L=S.msgs[a.id]||[];const last={v:null};
 const pin=atBottom(box);let prevT=0;
 L.forEach((m,idx)=>{if(m.role!=="agent")last.v=null;const t=T(m.t);if(m.role==="user"&&t>0&&(!prevT||t-prevT>15*60e3))box.append(el("div","daystamp",dayStamp(t)));if(t>0)prevT=t;
  if(m.text){speaker(box,m,last);const b=el("div","msg"+(m.role==="user"?" mine":m.role==="system"?" sys":" bot"));if(m.role==="agent")md(b,m.text,m.sources);else b.textContent=m.text;if(m.queued){b.append(el("span","chip","Đang chờ lượt"))}box.append(b)}
  if(m.plan)box.append(planCard(m));
  if(m.choices&&m.choices.length){const wrap=el("div","choices");const later=L.slice(idx+1).some(x=>x.role==="user");m.choices.forEach(ch=>{const b=el("button","choice",ch);b.disabled=later||usedChoice.has(m.id);b.onclick=async()=>{if(usedChoice.has(m.id))return;usedChoice.add(m.id);renderThread();if(!(await sendMsg(ch)))usedChoice.delete(m.id)};wrap.append(b)});box.append(wrap)}
  if(m.tools&&m.tools.length&&m.text&&!(a.status==="working"&&idx===L.length-1&&!m.text))box.append(sourceList(m));
  if(m.approval)box.append(approvalCard(m));if(m.secret)box.append(secretCard(m))});
 keepSec.forEach(([id,v])=>{const i=box.querySelector(`.secret[data-id="${id}"] input[data-secret]`);if(i)i.value=v});
 keepEdit.forEach(([id,v,sj])=>{const c=box.querySelector(`.approval[data-id="${id}"]`);if(c){const t=c.querySelector(".ap-body");if(t)t.value=v;const s2=c.querySelector(".ap-subject");if(s2&&sj!=null)s2.value=sj}});
 Object.entries(S.drafts).forEach(([id,d])=>{if(d.agentId===a.id){speaker(box,{role:"agent",from:d.from},last);const b=el("div","msg bot draft");md(b,visible(d.text),srcOf(id));b.id="d_"+id;box.append(b)}});
 if(a.status==="working"&&!Object.values(S.drafts).some(d=>d.agentId===a.id))box.append(turnStatus(a,L));
 setSendMode();
 if(pin||justSent)requestAnimationFrame(()=>{box.scrollTop=box.scrollHeight;justSent=false;syncJump()})}
// single status line (bot face + milestone text), replaced by the streamed answer
function turnStatus(a,L){const lastW=[...L].reverse().find(m=>m.role==="agent"&&m.tools&&m.tools.length);const run=lastW&&lastW.tools.find(t=>t.status==="running");
 const txt=run?(run.summary||"Đang tra cứu"):lastW&&T(lastW.t)>=T((L.filter(m=>m.role==="user").at(-1)||{}).t)?"Đang tổng hợp":"Đang suy nghĩ";
 const d=el("div","status");d.setAttribute("role","status");const f=el("span","st-face");f.append(face(a.av||{},22));d.append(f,el("span","st-t",txt));return d}
const srcOf=id=>{const m=(S.msgs[cur&&cur.id]||[]).find(x=>x.id===id);return m&&m.sources};
const atBottom=b=>b.scrollHeight-b.scrollTop-b.clientHeight<120;let justSent=false;
const dayStamp=t=>{const d=new Date(t),n=new Date(),y=new Date(n-864e5);const hm=d.toLocaleTimeString("vi-VN",{hour:"2-digit",minute:"2-digit"});return(d.toDateString()===n.toDateString()?"Hôm nay ":d.toDateString()===y.toDateString()?"Hôm qua ":d.toLocaleDateString("vi-VN",{day:"numeric",month:"numeric"})+" ")+hm};
function syncJump(){const b=$("#messages");$("#jumpBtn").classList.toggle("hidden",atBottom(b))}
const visible=t=>{const i=t.search(/(^|\n)\s*(APPROVAL|PLAN|CHOICES):/);return(i>=0?t.slice(0,i):t).trim()||"…"};
function paintDraft(id){const n=document.getElementById("d_"+id);if(!n){renderThread();return}const box=$("#messages"),pin=atBottom(box);n.replaceChildren();md(n,visible(S.drafts[id].text),srcOf(id));if(pin)box.scrollTop=box.scrollHeight;syncJump()}
async function decide(m,v,bs,extra){if(deciding.has(m.id))return;deciding.add(m.id);bs.querySelectorAll("button").forEach(b=>b.disabled=true);
 try{await api(`/approvals/${m.id}`,{decision:v,...(extra||{})});buzz(v==="approved"?"confirm":"soft");editing.delete(m.id)}catch(e){if(e.status!==409){deciding.delete(m.id);toast("Không ghi nhận được, thử lại");bs.querySelectorAll("button").forEach(b=>b.disabled=false)}}}
// send ↔ stop in the same spot while the agent works (stopping holds queued follow-ups)
function setSendMode(){const b=$("#composer .send");const empty=!$("#input").value.trim(),run=!!(cur&&cur.status==="working")&&empty,voice=empty&&!run;b.classList.toggle("stop",run);b.classList.toggle("voice",voice);b.replaceChildren(I(run?"square":voice?"audio-lines":"arrow-up"));b.setAttribute("aria-label",run?"Dừng":voice?"Nói":"Gửi")}
let stopping=false;async function stopAgent(){if(!cur||stopping)return;stopping=true;try{await api(`/agents/${cur.id}/stop`,{});buzz("soft");toast("Đã dừng")}catch{toast("Không dừng được")}finally{stopping=false}}
let sending=false;
async function sendMsg(text){const a=cur;text=text.trim();if(!a||!text||sending)return false;sending=true;
 try{await api(`/agents/${a.id}/messages`,{text});buzz("tap");return true}catch(e){toast(e.status===429?"Bạn gửi nhanh quá, đợi vài giây rồi gửi lại nhé":!navigator.onLine?"Đang ngoại tuyến, tin chưa gửi":"Không gửi được, thử lại");return false}finally{sending=false}}
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
 try{const ed=editing_bot;const a=await api(ed?`/agents/${ed.id}`:"/agents",{name,desc,av:{...draft}});upsert(a);markSeen(a.id);closeSheet(true);renderList();if(ed){if(cur&&cur.id===a.id){cur=a;$("#threadName").textContent=a.name;$("#threadAvatar").replaceChildren(avatarOf(a,32))}if(profId===a.id)renderProfile()}buzz("confirm");toast((ed?"Đã lưu ":"Đã tạo ")+a.name)}catch(e){toast(editing_bot?"Không lưu được":e.status===409&&!editing_bot?"Đã đạt giới hạn 32 bot (kể cả bot đã lưu trữ)":"Không tạo được bot");syncCreate()}finally{creating=false}};
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
function renderSettings(){const B=$("#settingsBody");B.innerHTML="";
 const acc=el("div","account");const m=el("img");m.src="/app/icon.svg";m.alt="";m.width=m.height=52;const at=el("div");at.append(el("b",0,"Chủ sở hữu"),el("small",0,location.host));acc.append(m,at);B.append(card(null,acc));
 B.append(card("Mức dùng",cell("sparkles","Mô hình AI",{static:1,value:S.ai.configured?(S.ai.model||"đã cấu hình"):"chưa cấu hình"}),cell("activity","Lõi chạy",{static:1,value:S.ai.runtime||"Neko Core"}),cell("users","Bot & nhóm",{static:1,value:`${bots().length} bot · ${S.agents.length-bots().length} nhóm`})));
 B.append(card("Ứng dụng",cell("layout-grid","Kết nối ứng dụng",{id:"openConnectors",value:connectedCount()+" đã kết nối",onClick:()=>{renderConnectors();swapSheet($("#connectors"))}})));
 B.append(card("Bot",cell("check","Luôn duyệt trước khi gửi",{toggle:true,locked:true,sub:"Không gì rời máy chủ khi bạn chưa bấm Duyệt"}),
  cell("bell","Thông báo",{id:"notifyCell",toggle:pushOn||notifOn,sub:pushOn?"Thông báo đẩy đang bật":notifOn?"Khi app đang mở":"Báo khi bot cần bạn",onToggle:sw=>{if(sw.getAttribute("aria-checked")==="true"){toast("Tắt trong Cài đặt của máy");return}enableNotify().then(renderSettings).catch(()=>toast("Không bật được thông báo"))}}),
  cell("smartphone","Phản hồi rung",{id:"hapticsCell",toggle:haptics,sub:CAP?"Rung nhẹ khi chạm":"Rung nhẹ khi chạm (Android)",onToggle:sw=>{haptics=!haptics;store.set("neko-aurora-haptics",haptics);sw.setAttribute("aria-checked",haptics);buzz("tap")}}),
  cell("sun-moon","Giao diện",{id:"openThemes",value:(THEMES.find(t=>t.id===curTheme())||THEMES[0]).name,onClick:()=>{renderThemes();swapSheet($("#themes"))}})));
 const link=(icon,label,href)=>cell(icon,label,{ext:1,onClick:()=>window.open(href,"_blank","noopener")});
 B.append(card("Trợ giúp",link("message-circle","Trung tâm trợ giúp","https://github.com/meiiie/neko-aurora#readme"),link("check","Quyền riêng tư","/app/privacy.html"),link("list-checks","Điều khoản sử dụng","/app/terms.html"),link("git-branch","Giấy phép mã nguồn mở","/app/aurora/lucide-LICENSE.txt")));
 B.append(card(null,cell("log-out","Đăng xuất",{id:"logoutBtn",danger:1,onClick:logout})));
 const ft=el("footer","about");const lg=el("img");lg.src="/app/icon.svg";lg.alt="";lg.width=lg.height=36;ft.append(lg,el("b",0,"Neko Aurora"),el("small",0,"Phiên bản "+VERSION));B.append(ft)}
async function logout(){try{await fetch("/app/logout",{method:"POST",credentials:"same-origin",headers:{"Content-Type":"application/json"},body:"{}"})}catch{}closeSheet(true);if(es){es.close();es=null}const r=await fetch(API+"/state",{credentials:"same-origin",cache:"no-store"}).catch(()=>null);if(r&&r.status===401)$("#login").classList.remove("hidden");else toast("Máy chủ cục bộ: phiên luôn mở trên máy này")}
$("#meBtn").onclick=()=>{if(sheetOpen)return;renderSettings();openSheet($("#settings"))};$("#closeSettings").onclick=()=>closeSheet(true);
// ---------- connectors (sheet + setup; credentials live in the server vault) ----------
// Layout follows the iOS connectors sheet pattern; brand marks: Simple Icons (CC0), else Lucide glyphs.
const OB=()=>S.outbound||{};
const CONN_GROUPS=[
 {id:"featured",name:"Nổi bật",items:[
  {id:"gmail",name:"Gmail",brand:"gmail",desc:"Gửi thư đã duyệt từ địa chỉ Gmail của bạn bằng mật khẩu ứng dụng."},
  {id:"push",name:"Thông báo đẩy",icon:"bell",desc:"Báo trên máy này khi bot cần bạn duyệt hoặc đã xong việc."},
  {id:"smtp",name:"Email (SMTP)",icon:"server",desc:"Gửi thư qua máy chủ SMTP bất kỳ: tên miền riêng, Zoho, Fastmail…"}]},
 {id:"work",name:"Năng suất",items:[
  {id:"gcal",name:"Google Calendar",brand:"googlecalendar",desc:"Đọc lịch, đề xuất giờ họp để bạn chốt."},
  {id:"gdrive",name:"Google Drive",brand:"googledrive",desc:"Tìm và tóm tắt tài liệu trong Drive."},
  {id:"notion",name:"Notion",brand:"notion",desc:"Ghi chú và cập nhật trang làm việc."},
  {id:"github",name:"GitHub",brand:"github",desc:"Theo dõi issue và pull request."}]},
 {id:"chat",name:"Nhắn tin",items:[
  {id:"telegram",name:"Telegram",brand:"telegram",desc:"Nhắn bot qua Telegram."},
  {id:"zalo",name:"Zalo",brand:"zalo",desc:"Nhận và trả lời tin Zalo."},
  {id:"discord",name:"Discord",brand:"discord",desc:"Đăng tin vào kênh máy chủ Discord."},
  {id:"slack",name:"Slack",icon:"hash",desc:"Đăng tin vào kênh làm việc."}]},
 {id:"social",name:"Mạng xã hội",items:[{id:"x",name:"X",brand:"x",desc:"Soạn bài đăng để bạn duyệt trước khi đăng."}]}];
const connCollapsed=new Set(store.get("neko-aurora-conn-collapsed",[]));
function connState(c){const o=OB(),g=o.gmail||{},sm=o.smtp||{},pu=o.push||{};
 if(c.id==="gmail")return g.connected?{on:1,sub:"1 tài khoản"+(g.from?" · "+g.from:""),act:()=>openSetup("gmail")}:{act:()=>openSetup("gmail")};
 if(c.id==="smtp")return sm.connected?{on:1,sub:(sm.accounts||1)+" tài khoản"+(sm.from?" · "+sm.from:""),act:()=>openSetup("smtp")}:{act:()=>openSetup("smtp")};
 if(c.id==="push")return pushOn?{on:1,sub:"Máy này · "+Math.max(1,pu.accounts|0)+" thiết bị"}:pu.connected?{act:()=>enableNotify().then(()=>{renderConnectors();renderSettings()}).catch(()=>toast("Không bật được thông báo"))}:{soon:1,label:"Chưa có khoá"};
 return{soon:1}}
function connectedCount(){let n=0;CONN_GROUPS.forEach(g=>g.items.forEach(c=>{if(connState(c).on)n++}));return n}
function connTile(c,sz){const t=el("span","conn-ic");const b=c.brand&&self.AURORA_BRANDS&&AURORA_BRANDS[c.brand];
 if(b){const NS="http://www.w3.org/2000/svg",sv=document.createElementNS(NS,"svg"),pa=document.createElementNS(NS,"path");sv.setAttribute("viewBox","0 0 24 24");sv.setAttribute("width",sz||22);sv.setAttribute("height",sz||22);sv.setAttribute("aria-hidden","true");pa.setAttribute("d",b.d);pa.setAttribute("fill",b.hex==="#000000"||b.hex==="#181717"?"currentColor":b.hex);sv.append(pa);t.append(sv)}else t.append(I(c.icon,sz||22));return t}
function renderConnectors(){$("#connCount").textContent=connectedCount()+" đã kết nối";const B=$("#connBody");B.innerHTML="";
 CONN_GROUPS.forEach(g=>{const sec=el("section","conn-sec");sec.dataset.id=g.id;const shut=connCollapsed.has(g.id);
  const h=el("button","conn-h");h.setAttribute("aria-expanded",!shut);h.append(el("span",0,g.name),I("chevron-right",18));
  h.onclick=()=>{shut?connCollapsed.delete(g.id):connCollapsed.add(g.id);store.set("neko-aurora-conn-collapsed",[...connCollapsed]);buzz("tap");renderConnectors()};sec.append(h);
  if(!shut){const ul=el("ul","conns");g.items.forEach(c=>{const st=connState(c);const li=el("li","conn");li.dataset.id=c.id;
   const t=el("span","pt");t.append(el("b",0,c.name),el("small",0,st.sub||c.desc));
   const b=el("button","conn-btn"+(st.on?" on":"")+(st.soon?" soon":""),st.label||(st.soon?"Sắp có":st.on?"Đã kết nối":"Kết nối"));
   if(st.soon)b.disabled=true;else{if(st.on)b.setAttribute("aria-disabled","true");b.onclick=()=>{buzz("tap");st.act()}}
   if(st.act&&!st.soon){li.classList.add("tap");t.onclick=()=>{buzz("tap");st.act()}}
   li.append(connTile(c),t,b);ul.append(li)});sec.append(ul)}
  B.append(sec)});
 B.append(el("p","conn-foot","Mật khẩu và khoá được mã hoá trong kho bí mật trên máy chủ của bạn; bot không bao giờ đọc được. Mọi thư vẫn phải qua nút Duyệt."))}
// masked secret input (shared by the chat card and connector setup); the value goes only to the vault
function secretField(name,label,ph){const w=el("label","sec-field");const i=el("input");i.type="password";i.autocomplete="off";i.setAttribute("data-1p-ignore","");i.setAttribute("data-lpignore","true");i.setAttribute("data-bwignore","");i.setAttribute("enterkeyhint","done");i.spellcheck=false;i.autocapitalize="off";i.setAttribute("autocorrect","off");i.placeholder=ph||"Dán vào đây";i.setAttribute("aria-label",label);i.dataset.secret=name;
 const eye=el("button","sec-eye");eye.type="button";eye.setAttribute("aria-label","Hiện/ẩn");eye.append(I("eye",18));eye.onclick=e=>{e.preventDefault();const show=i.type==="password";i.type=show?"text":"password";eye.replaceChildren(I(show?"eye-off":"eye",18))};w.append(i,eye);return{w,i}}
let setupKind=null;
function openSetup(kind){setupKind=kind;renderSetup();if(sheetOpen===$("#connSetup"))return;swapSheet($("#connSetup"))}
$("#closeSetup").onclick=()=>{setupKind=null;renderConnectors();swapSheet($("#connectors"))};
function field(label,attrs){const w=el("label","fld");w.append(el("span",0,label));const i=el("input");Object.assign(i,attrs);i.setAttribute("aria-label",label);w.append(i);return{w,i}}
function renderSetup(){const B=$("#setupBody");B.innerHTML="";const o=OB();
 if(setupKind==="gmail"){const g=o.gmail||{};$("#setupTitle").textContent="Gmail";const c=CONN_GROUPS[0].items[0];
  const hd=el("div","setup-hd");hd.append(connTile(c,30),el("b",0,"Gmail"),el("small",0,g.connected?"Đã kết nối · gửi từ "+g.from:"Gửi qua smtp.gmail.com:587 (STARTTLS) bằng mật khẩu ứng dụng"));B.append(hd);
  const em=field("Địa chỉ Gmail",{type:"email",id:"gmailAddr",autocomplete:"email",placeholder:"ban@gmail.com",value:g.from||""});em.i.inputMode="email";
  const card=el("div","secret");card.dataset.name="GMAIL_APP_PASSWORD";const top=el("div","sec-top");top.append(I("key-round",18),el("b",0,"Mật khẩu ứng dụng"),el("code",0,"GMAIL_APP_PASSWORD"));
  const sf=secretField("GMAIL_APP_PASSWORD","Mật khẩu ứng dụng Gmail",g.connected?"Đã lưu · để trống nếu giữ nguyên":"16 ký tự, ví dụ abcd efgh ijkl mnop");sf.i.id="gmailPass";
  const how=el("p","sec-desc");how.append("Bật xác minh 2 bước, rồi tạo mật khẩu ứng dụng tại ");const a=el("a",0,"myaccount.google.com/apppasswords");a.href="https://myaccount.google.com/apppasswords";a.target="_blank";a.rel="noopener noreferrer";how.append(a,". Không dùng mật khẩu Gmail chính.");
  card.append(top,how,sf.w);const save=el("button","pill wide","Lưu");save.id="gmailSave";
  save.onclick=async()=>{const address=em.i.value.trim(),password=sf.i.value;if(!address){toast("Nhập địa chỉ Gmail");return}if(!password&&!g.connected){toast("Nhập mật khẩu ứng dụng");return}save.disabled=true;
   try{S.outbound=await api("/connectors/gmail",{address,password});sf.i.value="";buzz("confirm");toast("Đã lưu vào kho bí mật");renderSetup()}catch(e){sf.i.value="";toast(e.status===400?"Kiểm tra lại địa chỉ hoặc mật khẩu 16 ký tự":"Không lưu được")}finally{save.disabled=false}};
  B.append(em.w,card,save);
  if(g.connected){const t=el("div","setup-test");const to=field("Gửi thử tới",{type:"email",id:"gmailTestTo",value:g.from,autocomplete:"email"});const go=el("button","pill wide ghost","Gửi thử");go.id="gmailTest";
   go.onclick=()=>gmailTest(to.i.value.trim());t.append(el("small",0,"Thư thử đi qua thẻ Duyệt như mọi thư khác."),to.w,go);B.append(t);
   const off=el("button","text-btn danger","Ngắt kết nối Gmail");off.id="gmailOff";off.onclick=async()=>{try{S.outbound=await api("/connectors/gmail/disconnect",{});S.secrets=S.secrets.filter(n=>!n.startsWith("GMAIL_"));toast("Đã xoá khỏi kho");renderSetup()}catch{toast("Không ngắt được")}};B.append(off)}}
 else if(setupKind==="smtp"){const sm=o.smtp||{};$("#setupTitle").textContent="Email (SMTP)";const hd=el("div","setup-hd");hd.append(connTile(CONN_GROUPS[0].items[2],30),el("b",0,"Email (SMTP)"),el("small",0,sm.connected?(sm.env&&!sm.accounts?"Đang dùng cấu hình AURORA_SMTP_* của máy chủ":"Đã kết nối"+(sm.from?" · "+sm.from:"")):"STARTTLS cổng 587 hoặc TLS cổng 465"));B.append(hd);
  const h=field("Máy chủ",{id:"smtpHost",placeholder:"smtp.example.com",autocapitalize:"off"}),pt=field("Cổng",{id:"smtpPort",value:"587",inputMode:"numeric"}),u=field("Tài khoản",{id:"smtpUser",autocomplete:"username",autocapitalize:"off"}),f=field("Gửi từ",{id:"smtpFrom",type:"email",placeholder:"ban@tenmien.vn"});
  const card=el("div","secret");const top=el("div","sec-top");top.append(I("key-round",18),el("b",0,"Mật khẩu SMTP"),el("code",0,"SMTP_PASSWORD"));const sf=secretField("SMTP_PASSWORD","Mật khẩu SMTP");sf.i.id="smtpPass";card.append(top,sf.w);
  const save=el("button","pill wide","Lưu");save.onclick=async()=>{save.disabled=true;try{S.outbound=await api("/connectors/smtp",{host:h.i.value,port:pt.i.value,user:u.i.value,from:f.i.value,password:sf.i.value});sf.i.value="";toast("Đã lưu vào kho bí mật");renderSetup()}catch{sf.i.value="";toast("Kiểm tra lại máy chủ, cổng (587/465) và địa chỉ")}finally{save.disabled=false}};
  B.append(h.w,pt.w,u.w,f.w,card,save);if(sm.accounts){const off=el("button","text-btn danger","Ngắt kết nối SMTP");off.onclick=async()=>{S.outbound=await api("/connectors/smtp/disconnect",{});renderSetup()};B.append(off)}}}
// "Gửi thử": asks the first bot to draft a test email; delivery still waits for the owner's Duyệt
async function gmailTest(to){if(!/^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(to)){toast("Địa chỉ không hợp lệ");return}const b=bots()[0];if(!b){toast("Cần ít nhất 1 bot");return}
 try{await api(`/agents/${b.id}/messages`,{text:`Soạn email thử tới ${to}, tiêu đề "Neko Aurora: thư thử Gmail", nội dung ngắn xác nhận Gmail đã kết nối. Chờ mình duyệt.`});buzz("confirm");closeSheet(true);setTimeout(()=>openThread(b.id),340)}catch{toast("Không gửi được yêu cầu")}}
$("#closeConnectors").onclick=()=>closeSheet(true);
$("#appsBtn").onclick=()=>{if(sheetOpen)return;renderConnectors();openSheet($("#connectors"))};
// in-chat secret request card: masked input → vault; message/journal hold only the name
function secretCard(m){const x=m.secret,saved=S.secrets.includes(x.name),c=el("div","secret"+(saved?" saved":""));c.dataset.id=m.id;c.dataset.name=x.name;
 const top=el("div","sec-top");top.append(I("key-round",18),el("b",0,x.label||x.name),el("code",0,x.name));c.append(top);if(x.desc)c.append(el("p","sec-desc",x.desc));
 if(saved){const d=el("div","sec-done");d.append(I("check",16),el("span",0,"Đã lưu vào kho bí mật"));c.append(d);return c}
 const sf=secretField(x.name,x.label||x.name);const row=el("div","sec-row");const go=el("button","pill","Lưu");go.className="pill sec-save";
 const err=el("p","sec-err");err.setAttribute("role","alert");
 go.onclick=async()=>{const value=sf.i.value;err.textContent="";if(!value){err.textContent="Chưa nhập giá trị.";return}go.disabled=true;go.textContent="Đang lưu…";
  try{await api("/secrets",{name:x.name,value,label:x.label||"",work:m.taskId});sf.i.value="";if(!S.secrets.includes(x.name))S.secrets.push(x.name);buzz("confirm");toast("Đã lưu "+x.name);loadState().catch(()=>{});renderThread()}catch(e){err.textContent=e.status===400?"Giá trị không hợp lệ (tối đa 8 KB).":"Không lưu được, thử lại.";go.disabled=false;go.textContent="Lưu"}};
 sf.i.onkeydown=e=>{if(e.key==="Enter"){e.preventDefault();go.click()}};row.append(sf.w,go);c.append(row,err,el("small","hint","Giá trị chỉ vào kho mã hoá trên máy chủ, không nằm trong tin nhắn."));return c}
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
 $("#pmPinT").textContent=a.pin?"Bỏ ghim":"Ghim";$("#pmPin").classList.toggle("hidden",isGroup(a));$("#pmArch").classList.toggle("hidden",isGroup(a))}
$("#profBack").onclick=()=>closeProfile();
$("#profShare").onclick=()=>{const a=byId(profId);if(a)shareLink(a)};
$("#profMore").onclick=e=>{e.stopPropagation();toggleMenu($("#profMenu"),$("#profMore"))};
$("#pmCopy").onclick=async()=>{closeMenu();const id=profId;try{await navigator.clipboard.writeText(id);toast("Đã sao chép ID")}catch{toast("ID: "+id)}};
// non-drag alternative to swipe-to-archive (WCAG 2.5.7)
$("#pmArch").onclick=()=>{closeMenu();const a=byId(profId);if(!a)return;api(`/agents/${a.id}`,{archived:true}).then(()=>{S.agents=S.agents.filter(x=>x.id!==a.id);renderList();history.back();toast("Đã lưu trữ")}).catch(()=>toast("Không lưu trữ được"))};
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
$("#composer").onsubmit=async e=>{e.preventDefault();stopVoice();const v=inp.value;if(!v.trim()){if(cur&&cur.status==="working")stopAgent();else if(SR)$("#micBtn").click();else inp.focus();return}inp.value="";inp.style.height="";inp.blur();justSent=true;setSendMode();if(!(await sendMsg(v))){inp.value=v;inp.oninput()}};
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
// iOS shell (Capacitor): Safari has no navigator.vibrate, so use native Taptic haptics there.
const CAP=window.Capacitor&&window.Capacitor.isNativePlatform&&window.Capacitor.isNativePlatform()?window.Capacitor.Plugins:null;
function buzz(k){try{if(CAP&&CAP.Haptics){if(!haptics||matchMedia("(prefers-reduced-motion: reduce)").matches)return;k==="confirm"?CAP.Haptics.notification({type:"SUCCESS"}):CAP.Haptics.impact({style:k==="soft"?"MEDIUM":"LIGHT"});return}if(!haptics||matchMedia("(prefers-reduced-motion: reduce)").matches||!navigator.vibrate)return;navigator.vibrate({tap:8,confirm:[10,40,14],soft:16}[k]||8)}catch{}}
[["#newBtn","plus"],["#threadMore","ellipsis"],["#plusBtn","plus"],["#profBack","chevron-left"],["#profShare","share"],["#profMore","ellipsis"],["#appsBtn","layout-grid"],["#searchIc","search"],["#backBtn","chevron-left"],["#jumpBtn","chevron-down"],["#micBtn","mic"],["#cancelBtn","x"],["#closeGroup","x"],["#closeSetup","chevron-left"],["#closeSettings","x"],["#closeConnectors","x"]].forEach(([q,n])=>{const b=$(q);if(b)b.replaceChildren(I(n))});
document.querySelectorAll(".mi[data-i]").forEach(s=>s.replaceChildren(I(s.dataset.i,18)));
setSendMode();
$("#messages").addEventListener("scroll",syncJump,{passive:true});if(window.ResizeObserver)new ResizeObserver(syncJump).observe($("#messages"));$("#jumpBtn").onclick=()=>{const b=$("#messages");b.scrollTo({top:b.scrollHeight,behavior:"smooth"});buzz("tap")};
window.__AURORA={state:()=>S,reload:loadState,current:()=>cur&&cur.id};
})();

// iOS shell: APNs registration (permission asked once, only after login; push is optional for the app to work)
if(CAP&&CAP.PushNotifications){const P=CAP.PushNotifications;P.addListener("registration",t=>{api("/push/apns",{token:t.value}).catch(()=>{})});P.addListener("pushNotificationActionPerformed",a=>{const u=a.notification&&a.notification.data&&a.notification.data.url;if(u&&u.startsWith("/app/"))location.href=u});P.checkPermissions().then(r=>r.receive==="granted"?P.register():r.receive==="prompt"?P.requestPermissions().then(x=>x.receive==="granted"&&P.register()):0).catch(()=>{})}
