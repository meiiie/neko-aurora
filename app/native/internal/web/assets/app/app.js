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
// iOS shell (Capacitor) native plugins, null in the browser
const CAP=window.Capacitor&&window.Capacitor.isNativePlatform&&window.Capacitor.isNativePlatform()?window.Capacitor.Plugins:null;

async function api(path,body,retry){const get=body===undefined;let r;try{r=await fetch(API+path,{method:get?"GET":"POST",credentials:"same-origin",cache:"no-store",headers:get?{}:{"Content-Type":"application/json"},body:get?undefined:JSON.stringify(body),signal:get&&AbortSignal.timeout?AbortSignal.timeout(15000):undefined});
 // a GET stuck on a container that is shutting down (deploy) or a 503 while the workspace opens: retry once on a fresh connection
 if(get&&!retry&&r.status===503){await new Promise(f=>setTimeout(f,1500));return api(path,body,true)}
 if(!r.ok){const e=new Error(String(r.status));e.status=r.status;e.body=await r.json().catch(()=>null);throw e}return get?await r.json():r.json()}catch(e){if(get&&!retry&&!e.status){await new Promise(f=>setTimeout(f,800));return api(path,body,true)}throw e}}
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
 es.addEventListener("agent",e=>{const a=JSON.parse(e.data);const was=S.agents.find(x=>x.id===a.id);if(was&&was.status==="working"&&a.status!=="working"&&S.msgs[a.id])loadMsgs(a.id).catch(()=>{});/* adopt server order once a turn ends (messages folded in mid-turn sit before the answer) */if(seen&&seen[a.id]==null)markSeen(a.id);upsert(a);if(cur&&cur.id===a.id){cur=a;markSeen(a.id);renderThread()}renderListSoon()});
 es.addEventListener("archived",e=>{const {id}=JSON.parse(e.data);S.agents=S.agents.filter(a=>a.id!==id);S.pending=S.pending.filter(m=>m.agentId!==id);renderListSoon()});
 es.addEventListener("message",e=>{const m=JSON.parse(e.data);const L=S.msgs[m.agentId];delete S.drafts[m.id];
  if(L){const i=L.findIndex(x=>x.id===m.id);if(i>=0)L[i]=m;else{const j=/:step\d+$/.test(m.id)?L.findIndex(x=>x.id===m.taskId):-1;if(j>=0)L.splice(j,0,m);else L.push(m)}}
  S.pending=S.pending.filter(x=>x.id!==m.id);if(m.approval&&m.approval.state==="pending")S.pending.push(m);
  if(cur&&cur.id===m.agentId)renderThread();renderListSoon()});
 es.addEventListener("delta",e=>{const d=JSON.parse(e.data);S.deltaCount=(S.deltaCount||0)+1;if(!S.drafts[d.id])setTimeout(()=>{if(S.drafts[d.id]&&cur&&cur.id===d.agentId&&!document.getElementById("d_"+d.id))paintDraft(d.id)},470);const x=S.drafts[d.id]||(S.drafts[d.id]={agentId:d.agentId,from:d.from,text:"",t0:Date.now()});x.text=typeof d.text==="string"?d.text:x.text+d.delta;if(cur&&cur.id===d.agentId)paintDraft(d.id)});
 es.addEventListener("draft_end",e=>{const d=JSON.parse(e.data);if(S.drafts[d.id]){delete S.drafts[d.id];if(cur&&cur.id===d.agentId)renderThread()}});
 es.addEventListener("activity",e=>{if(sheetOpen===$("#activity"))renderLog([JSON.parse(e.data)],true)});
 es.addEventListener("brief",e=>{S.brief=JSON.parse(e.data);renderListSoon()});
 es.addEventListener("notify",e=>{const n=JSON.parse(e.data);if(muted.includes((n.url||"").split("#")[1]))return;if(document.hidden&&!pushOn&&"Notification"in window&&Notification.permission==="granted")navigator.serviceWorker?.ready.then(r=>r.showNotification(n.title,{body:n.body,data:{url:n.url},tag:n.url}))});
}
// ---------- list ----------
function avatarOf(a,sz){if(!isGroup(a))return face(a.av,sz);const w=el("span","stack");w.style.width=w.style.height=sz+"px";(a.members||[]).slice(0,2).forEach((id,i)=>{const b=byId(id);w.append(face(b&&b.av,Math.round(sz*.64),"s"+i))});return w}
// chat header avatar carries the same presence dot as the list (Messenger-style)
function headAvatar(a){const h=$("#threadAvatar");const k=a.status==="working"?"work":waiting(a)?"wait":"";if(h.dataset.k===k&&h.dataset.id===a.id&&h.firstChild)return;h.dataset.k=k;h.dataset.id=a.id;h.classList.add("head-av");h.replaceChildren(avatarOf(a,32));const bd=badge(a);if(bd)h.append(bd)}
function badge(a){const k=a.status==="working"?"work":waiting(a)?"wait":a.status==="error"?"err":"";if(!k)return null;const b=el("i","badge "+k);b.setAttribute("aria-hidden","true");return b}
let filter="";
const statusLine=a=>a.status==="working"?"Đang làm việc…":waiting(a)?"Chờ bạn duyệt":a.snip||a.desc||"";
function renderList(){
 const all=[...S.agents].sort((a,b)=>(waiting(b)-waiting(a))||T(b.t)-T(a.t));
 const q=filter.trim().toLowerCase(),match=a=>!q||a.name.toLowerCase().includes(q)||(a.snip||"").toLowerCase().includes(q);
 const pinned=all.filter(a=>a.pin&&match(a)),rest=all.filter(a=>!a.pin&&match(a));
 const fe=$("#featured");fe.innerHTML="";fe.classList.toggle("hidden",!pinned.length);
 pinned.forEach(a=>{const b=el("button","feat");b.dataset.id=a.id;b.setAttribute("aria-label",a.name);const av=el("span","feat-av");av.append(avatarOf(a,92));const bd=badge(a);if(bd)av.append(bd);const nm=el("span","feat-name");nm.append(el("span",0,a.name));if(unread(a)){nm.append(el("i","dot"));b.setAttribute("aria-label",a.name+", chưa đọc")}b.append(av,nm);b.onclick=()=>{if(performance.now()<suppress)return;openThread(a.id)};longPress(b,a);fe.append(b)});
 renderBrief(q);
 const ul=$("#rows");ul.innerHTML="";
 const hid=new Set(store.get("neko-aurora-hidden",[])),secs=store.get("neko-aurora-sections",{list:[],of:{}});
 const visibleRest=rest.filter(a=>q||!hid.has(a.id)||showHidden),ordered=visibleRest.filter(a=>!secs.of[a.id]||!secs.list.some(x=>x.id===secs.of[a.id]));
 secs.list.forEach(sc=>{const its=visibleRest.filter(a=>secs.of[a.id]===sc.id);if(its.length)ordered.push({hdr:sc},...its)});
 const hiddenN=rest.filter(a=>hid.has(a.id)).length;
 ordered.forEach(a=>{
  if(a.hdr){const h=el("li","sec-h");h.setAttribute("role","heading");h.setAttribute("aria-level","2");h.append(el("span",0,a.hdr.name));ul.append(h);return}
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
  if(hid.has(a.id))li.classList.add("is-hidden");
  if(!isGroup(a))swipeRow(li,inn);longPress(inn,a)});
 if(hiddenN&&!q){const li=el("li","hid-toggle"),b=el("button","text-btn",showHidden?"Ẩn lại "+hiddenN+" bot":"Đã ẩn · "+hiddenN);b.onclick=()=>{showHidden=!showHidden;buzz("tap");renderList()};li.append(b);ul.append(li)}
 const em=$("#empty");em.classList.toggle("hidden",!!(pinned.length||rest.length));em.textContent=q?"Không tìm thấy “"+filter.trim()+"”.":"Chưa có bot nào. Bấm + để tạo.";
 updatePill()}
// ---------- long-press context menu (iOS-style: blurred backdrop, preview, menu + stacked submenu) ----------
let showHidden=false,ctxOpen=null;
function longPress(node,a){let t=null,x0=0,y0=0;const clear=()=>{clearTimeout(t);t=null;node.classList.remove("pressing")};
 node.addEventListener("touchstart",e=>{if(e.touches.length>1)return;const p=e.touches[0];x0=p.clientX;y0=p.clientY;node.classList.add("pressing");t=setTimeout(()=>{t=null;node.classList.remove("pressing");suppress=performance.now()+700;openCtx(a,node)},480)},{passive:true});
 node.addEventListener("touchmove",e=>{if(!t)return;const p=e.touches[0];if(Math.abs(p.clientX-x0)>8||Math.abs(p.clientY-y0)>8)clear()},{passive:true});
 node.addEventListener("touchend",clear);node.addEventListener("touchcancel",clear);
 node.addEventListener("contextmenu",e=>{e.preventDefault();if(!ctxOpen){suppress=performance.now()+700;openCtx(a,node)}})}
const reduceMotion=()=>matchMedia("(prefers-reduced-motion: reduce)").matches;
function ctxItem(icon,label,fn,opt={}){const b=el("button","ctx-i"+(opt.danger?" danger":""));b.setAttribute("role","menuitem");if(opt.id)b.id=opt.id;b.append(I(icon,22),el("span",0,label));if(opt.chev)b.append(el("span","ctx-chev"),I(opt.chev,18));b.onclick=e=>{e.stopPropagation();if(performance.now()<suppress)return;fn(b)};return b}
function closeCtx(){const c=ctxOpen;if(!c)return;ctxOpen=null;c.classList.add("out");document.removeEventListener("keydown",ctxKey);setTimeout(()=>c.remove(),reduceMotion()?0:220);c._from&&c._from.focus&&c._from.focus({preventScroll:true})}
const ctxKey=e=>{if(e.key==="Escape"){e.preventDefault();const sub=ctxOpen&&ctxOpen.querySelector(".ctx-sub");if(sub)sub._close();else closeCtx()}};
function openCtx(a,from){if(ctxOpen||sheetOpen)return;closeMenu();if(openRow)closeRow(openRow);buzz("tap");
 const o=el("div","ctx");o._from=from;o.setAttribute("role","dialog");o.setAttribute("aria-label","Tuỳ chọn cho "+a.name);
 o.addEventListener("click",e=>{if(performance.now()<suppress){e.stopPropagation();e.preventDefault();const f=o.querySelector(".ctx-menu .ctx-i");f&&f.focus({preventScroll:true})}},true); // the lift that ends the long-press is not a tap
 const bg=el("div","ctx-bg");bg.onclick=closeCtx;o.append(bg);
 const card=el("div","ctx-card");const hd=el("div","ctx-h");hd.append(avatarOf(a,28),el("b",0,a.name));const bub=el("div","ctx-bub");
 const lastText=()=>{const L=(S.msgs[a.id]||[]).filter(m=>m.role==="agent"&&m.text);return L.length?L[L.length-1].text:statusLine(a)};
 let place=()=>{};const fill=()=>{bub.replaceChildren();md(bub,lastText().slice(0,600));place()};fill();if(!S.msgs[a.id])loadMsgs(a.id).then(()=>ctxOpen===o&&fill()).catch(()=>{});
 card.append(hd,bub);card.onclick=()=>{closeCtx();openThread(a.id)};
 const menu=el("div","ctx-menu glass");menu.setAttribute("role","menu");
 const un=unread(a),hid=store.get("neko-aurora-hidden",[]),isHid=hid.includes(a.id);
 menu.append(
  ctxItem("message-square-dot",un?"Đánh dấu đã đọc":"Đánh dấu chưa đọc",()=>{if(un)markSeen(a.id);else{if(!seen)seen={};seen[a.id]=T(a.t)-1;store.set("neko-aurora-seen",seen)}closeCtx();renderList();buzz("confirm")},{id:"ctxUnread"}),
  isGroup(a)?null:ctxItem("pin",a.pin?"Bỏ ghim":"Ghim",()=>{closeCtx();api(`/agents/${a.id}`,{pin:!a.pin}).then(x=>{upsert(x);renderList();buzz("confirm");toast(x.pin?"Đã ghim":"Đã bỏ ghim")}).catch(()=>toast("Không ghim được"))},{id:"ctxPin"}),
  ctxItem("folder-plus","Mục mới",b=>newSection(menu,a),{id:"ctxSection"}),
  ctxItem("share","Chia sẻ",()=>{closeCtx();const url=location.origin+"/app/#"+a.id;if(navigator.share)navigator.share({title:a.name,text:a.desc||a.name,url}).catch(()=>{});else navigator.clipboard&&navigator.clipboard.writeText(url).then(()=>toast("Đã sao chép liên kết"))},{id:"ctxShare"}),
  ctxItem("eye-off",isHid?"Bỏ ẩn":"Ẩn",()=>{const h=new Set(store.get("neko-aurora-hidden",[]));isHid?h.delete(a.id):h.add(a.id);store.set("neko-aurora-hidden",[...h]);closeCtx();renderList();buzz("soft");toast(isHid?"Đã bỏ ẩn "+a.name:"Đã ẩn "+a.name)},{id:"ctxHide",danger:!isHid}),
  ctxItem("ellipsis","Thêm",()=>openSub(o,menu,a),{id:"ctxMore",chev:"chevron-right"}));[...menu.childNodes].forEach(n=>{if(!n.tagName)n.remove()});
 o.append(card,menu);document.body.append(o);ctxOpen=o;document.addEventListener("keydown",ctxKey);
 // place: preview hugs the pressed row, the menu sits under it; both stay on screen
 const r=from.getBoundingClientRect(),st=parseFloat(getComputedStyle(document.documentElement).getPropertyValue("--st"))||0;
 place=()=>{const vh=innerHeight,ch=card.offsetHeight,mh=menu.offsetHeight,gap=12,top=Math.max(st+12,Math.min(r.top-8,vh-ch-mh-gap-24));card.style.top=top+"px";menu.style.top=(top+ch+gap)+"px"};place();o.style.setProperty("--ox",(r.left+r.width/2)+"px");
 requestAnimationFrame(()=>o.classList.add("in"));setTimeout(()=>(menu.querySelector(".ctx-i")||menu).focus({preventScroll:true}),60)}
function openSub(o,menu,a){if(menu.querySelector(".ctx-sub"))return;buzz("tap");const more=menu.querySelector("#ctxMore");const sub=el("div","ctx-sub glass");sub.setAttribute("role","menu");
 const head=ctxItem("ellipsis","Thêm",()=>sub._close(),{chev:"chevron-down"});head.classList.add("ctx-subh");sub.append(head,el("div","ctx-sep"));
 sub.append(ctxItem("copy","Sao chép ID",()=>{closeCtx();(navigator.clipboard?navigator.clipboard.writeText(a.id):Promise.reject()).then(()=>toast("Đã sao chép ID")).catch(()=>toast(a.id))},{id:"ctxCopy"}));
 if(!isGroup(a))sub.append(ctxItem("copy-plus","Nhân bản",()=>{closeCtx();api("/agents",{name:(a.name+" (bản sao)").slice(0,40),desc:a.desc||"",av:a.av}).then(x=>{upsert(x);markSeen(x.id);renderList();buzz("confirm");toast("Đã nhân bản "+a.name)}).catch(e=>toast(e.status===409?"Đã đạt giới hạn 32 bot":"Không nhân bản được"))},{id:"ctxDup"}));
 const del=ctxItem("trash-2","Lưu trữ",b=>{if(!b.dataset.arm){b.dataset.arm=1;b.querySelector("span").textContent="Chạm lần nữa để lưu trữ";buzz("soft");return}closeCtx();api(`/agents/${a.id}`,{archived:true}).then(()=>{S.agents=S.agents.filter(x=>x.id!==a.id);renderList();toast("Đã lưu trữ "+a.name)}).catch(()=>toast("Không lưu trữ được"))},{id:"ctxArchive",danger:true});
 if(!isGroup(a))sub.append(del);
 sub._close=()=>{menu.classList.remove("dim");sub.classList.add("out");setTimeout(()=>sub.remove(),reduceMotion()?0:200);more.focus({preventScroll:true})};
 const mr=more.getBoundingClientRect(),m0=menu.getBoundingClientRect();sub.style.top=(mr.top-m0.top-8)+"px";menu.append(sub);menu.classList.add("dim");
 // keep the stacked submenu on screen
 const over=m0.top+parseFloat(sub.style.top)+sub.offsetHeight-(innerHeight-24);if(over>0){menu.style.top=(parseFloat(menu.style.top)-over)+"px";const c=o.querySelector(".ctx-card");c.style.top=(parseFloat(c.style.top)-over)+"px"}
 requestAnimationFrame(()=>sub.classList.add("in"));setTimeout(()=>sub.querySelector("#ctxCopy").focus({preventScroll:true}),40)}
function newSection(menu,a){const f=el("form","ctx-form");const i=el("input");i.placeholder="Tên mục, ví dụ Công việc";i.maxLength=24;i.setAttribute("aria-label","Tên mục mới");const go=el("button","pill small","Tạo");go.type="submit";f.append(i,go);
 f.onsubmit=e=>{e.preventDefault();const n=i.value.trim();if(!n)return;const sc=store.get("neko-aurora-sections",{list:[],of:{}});let x=sc.list.find(z=>z.name.toLowerCase()===n.toLowerCase());if(!x){x={id:"s"+Date.now().toString(36),name:n};sc.list.push(x)}sc.of[a.id]=x.id;store.set("neko-aurora-sections",sc);closeCtx();renderList();buzz("confirm");toast("Đã chuyển "+a.name+" vào "+n)};
 const sc=store.get("neko-aurora-sections",{list:[],of:{}});menu.replaceChildren(f);
 sc.list.forEach(x=>menu.append(ctxItem("folder",x.name,()=>{const s2=store.get("neko-aurora-sections",{list:[],of:{}});s2.of[a.id]=x.id;store.set("neko-aurora-sections",s2);closeCtx();renderList();toast("Đã chuyển vào "+x.name)})));
 if(sc.of[a.id])menu.append(ctxItem("x","Bỏ khỏi mục",()=>{const s2=store.get("neko-aurora-sections",{list:[],of:{}});delete s2.of[a.id];s2.list=s2.list.filter(x=>Object.values(s2.of).includes(x.id));store.set("neko-aurora-sections",s2);closeCtx();renderList()}));
 setTimeout(()=>i.focus(),30)}
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
function openThread(id){const a=byId(id);if(!a)return;closeMenu();cur=a;markSeen(id);headAvatar(a);$("#threadName").textContent=a.name;$("#threadState").textContent=stateLine(a);$("#input").placeholder=isGroup(a)?"Hỏi cả nhóm":"Hỏi "+a.name;closeFind();renderThread();loadMsgs(id).catch(()=>toast("Không tải được hội thoại"));$("#thread").classList.remove("off");$("#list").classList.add("under");buzz("tap");const hs=history.state;if(hs&&hs.spent)history.replaceState({t:1},"","#"+id);else if(!(hs&&hs.t))history.pushState({t:1},"","#"+id)}
function closeThread(fromPop){if(!cur)return;if(profId)closeProfile(true);closeFind();markSeen(cur.id);cur=null;stopVoice();$("#thread").classList.add("off");$("#list").classList.remove("under");$("#thread").style.transform="";$("#list").style.transform="";renderList();if(!fromPop&&history.state&&history.state.t)history.replaceState({spent:1},"",location.pathname)}
const deciding=new Set();
const ICON={pending:"○",running:"◐",done:"✓",waiting:"⏸",failed:"✕",cancelled:"–"};
const editing=new Set(),usedChoice=new Set();
function planCard(m){const c=el("div","plan");const st=m.plan.steps,done=st.filter(x=>x.status==="done").length;
 const h=el("div","plan-h");h.append(el("b",0,"Kế hoạch"),el("span",0,`${done}/${st.length}`));c.append(h);
 const bar=el("div","bar");const f=el("i");f.style.width=(100*done/st.length)+"%";bar.append(f);c.append(bar);
 const ol=el("ol");st.forEach((x,i)=>{const li=el("li","step "+x.status);li.append(stepIcon(x.status),el("span",0,x.title));ol.append(li)});c.append(ol);return c}
function approvalCard(m){const ap=m.approval,c=el("div","approval"+(ap.state!=="pending"?" done":""));c.dataset.id=m.id;
 c.append(el("small",0,ap.state==="superseded"?"Đã thay bằng yêu cầu mới":ap.kind==="computer"?"Lệnh trên máy tính của bot · chỉ chạy khi bạn duyệt":ap.kind==="vault"?"Dùng Vault · giá trị thật chỉ chèn trên máy chủ khi bạn duyệt":"Cần bạn duyệt · không gửi khi chưa có OK"),el("div","ap-title",ap.title));
 if(ap.to){const meta=el("div","ap-meta");meta.append(el("span",0,"Tới: "),el("b",0,ap.to));c.append(meta)}
 const ed=ap.state==="pending"&&editing.has(m.id);
 if(ap.subject||(ed&&ap.kind==="email")){if(ed){const i=el("input","ap-subject");i.value=ap.subject||"";i.setAttribute("aria-label","Tiêu đề");c.append(i)}else{const meta=el("div","ap-meta");meta.append(el("span",0,"Tiêu đề: "),el("b",0,ap.subject));c.append(meta)}}
 if(ed){const t=el("textarea","ap-body");t.value=ap.body;t.rows=8;t.setAttribute("aria-label","Nội dung");c.append(t)}else if(ap.kind==="computer"){const cs=AURORA_UX.cmdShown(ap.body),fc=AURORA_UX.foldCmd(cs.cmd),pre=el("pre","ap-cmd",fc.head);pre.title=ap.body;pre.dataset.noi18n="";c.append(pre);if(fc.more){const d=el("details","ap-more"),sm=el("summary",0,"Xem cả lệnh · thêm "+fc.more+" dòng"),rp=el("pre","ap-cmd",fc.rest);rp.dataset.noi18n="";d.append(sm,rp);c.append(d)}}else if(ap.kind==="vault"){c.classList.add("ap-vault");const hd=el("div","ap-vhd");hd.append(I("shield-check",18),el("span",0,"Bot không thấy số thẻ, mật khẩu hay mã"));c.append(hd,el("pre",0,ap.body))}else c.append(el("pre",0,ap.body));
 const os=ap.kind==="computer"&&ap.outcome?AURORA_UX.outSummary(ap.outcome):null;
 if(os&&os.count){if(os.long){const d=el("details","ap-outd"),sm=el("summary",0,`Xem toàn bộ đầu ra · ${os.count} dòng`);const tail=el("pre","ap-out ap-tail",os.tail),full=el("pre","ap-out",os.full);tail.dataset.noi18n=full.dataset.noi18n="";d.append(sm,full);c.append(tail,d)}else{const o=el("pre","ap-out",os.full);o.dataset.noi18n="";c.append(o)}}
 if(ap.state==="pending"){const bs=el("div","btns"),no=el("button",0,"Để sau"),e=el("button",0,ed?"Huỷ sửa":"Sửa"),vk=ap.kind==="vault",go=el("button","go",ed?"Lưu & duyệt":"Duyệt");
  no.onclick=()=>decide(m,"declined",bs);e.onclick=()=>{ed?editing.delete(m.id):editing.add(m.id);renderThread()};
  go.onclick=()=>{const ex={};if(ed){ex.body=c.querySelector(".ap-body").value;const sj=c.querySelector(".ap-subject");if(sj)ex.subject=sj.value;if(!ex.body.trim()){toast("Nội dung trống");return}}decide(m,"approved",bs,ex)};
  if(deciding.has(m.id))no.disabled=e.disabled=go.disabled=true;vk?bs.append(no,go):bs.append(no,e,go);c.append(bs);
  c.append(el("small","hint","Muốn đổi hướng? Nhắn yêu cầu mới, bản nháp này sẽ được thay."))}
 else if(ap.state!=="superseded"){const dv=ap.delivery;if(ap.kind==="vault"&&ap.state==="approved"){c.append(el("small","ap-line"+(dv&&dv.status==="failed"?" bad":""),!dv||dv.status==="sending"?"✓ Đã duyệt · đang dùng Vault…":dv.status==="failed"?"✓ Đã duyệt · không dùng được"+(dv.detail?": "+dv.detail:""):"✓ Đã duyệt · đã dùng, giá trị đã che"));if(ap.outcome&&dv&&dv.status==="sent")c.append(el("p","ap-vout",ap.outcome));return c}const DS={queued:"đang chờ kênh email",sending:"đang gửi…",sent:"đã gửi"+(dv&&dv.from?" từ "+dv.from:""),failed:"gửi thất bại"+(dv&&dv.detail?": "+dv.detail:"")};if(ap.kind==="computer"&&ap.state==="approved"){const sec=os&&os.secs!=null?" · "+String(os.secs).replace(".",",")+"s":"";c.append(el("small","ap-line"+(os&&os.exit?" bad":""),!dv||dv.status==="sending"||dv.status==="queued"?"✓ Đã duyệt · đang chạy…":dv.status==="failed"?"✓ Đã duyệt · không chạy được"+(dv.detail?": "+dv.detail:""):os&&os.exit?`✓ Đã duyệt · lỗi (mã thoát ${os.exit})${os.err?" · "+os.err:""}${sec}`:`✓ Đã duyệt · chạy xong${os&&os.exit===0?" · mã thoát 0":""}${sec}`));return c}
 c.append(el("small",0,ap.state==="approved"?(dv?"✓ Bạn đã duyệt · "+DS[dv.status]:"✓ Bạn đã duyệt · chưa gửi ra ngoài (kênh gửi chưa kết nối)"):"Đã hoãn"))}
 return c}
// tool activity stays unobtrusive: a status line while running, folded into the
// "Nguồn" footer afterwards (states/dedup after CopilotKit/openmuse search-tool-card.tsx, MIT)
const srcHost=u=>{try{return new URL(u).hostname.replace(/^www\./,"")}catch{return u}};
function uniqSources(list){const seen=new Map();(list||[]).forEach((x,i)=>{if(!seen.has(x.url))seen.set(x.url,{...x,n:i+1})});return[...seen.values()]}
// markdown lives in md.js (AURORA_MD); drafts render with streaming:true
const md=(node,text,src,streaming)=>AURORA_MD.render(node,text,src,{streaming});
document.addEventListener("aurora-copied",()=>{buzz("confirm");toast("Đã sao chép")});
function sourceList(m){const w=el("details","sources");const sm=el("summary");const u=uniqSources(m.sources);const ok=u.length>0,T=m.tools||[],bad=T.length>0&&T.every(t=>t.status==="failed");sm.append(el("span",0,ok?`Nguồn · ${u.length}`:bad?"Không lấy được dữ liệu":`Các bước · ${T.length}`),I("chevron-down",14));w.append(sm);
 (m.tools||[]).forEach(t=>{const r=el("p","tool-line"+(t.status==="failed"?" bad":""),(t.summary||t.name)+(t.ms?" · "+(t.ms/1000).toFixed(1).replace(".",",")+"s":""));w.append(r)});const ol=el("ol");
 u.forEach(x=>{const li=el("li");const l=el("a");l.href=x.url;l.target="_blank";l.rel="noopener noreferrer";l.append(el("b",0,"["+x.n+"] "+(x.title||srcHost(x.url))),el("small",0,srcHost(x.url)+(x.at?" · "+new Date(x.at).toLocaleTimeString("vi-VN",{hour:"2-digit",minute:"2-digit"}):"")));li.append(l);ol.append(li)});w.append(ol);return w}
function speaker(box,m,last){if(!isGroup(cur)||m.role!=="agent"||!m.from)return;if(last.v===m.from)return;last.v=m.from;const b=byId(m.from);const h=el("div","speaker");h.append(face(b&&b.av,20),el("span",0,b?b.name:"Bot"));box.append(h)}
function renderThread(){const a=cur;if(!a)return;$("#threadState").textContent=stateLine(a);headAvatar(a);const box=$("#messages");
 const keepSec=[...box.querySelectorAll(".secret input[data-secret]")].map(i=>[i.closest(".secret").dataset.id,i.dataset.secret,i.value]);
 const keepForm=[...box.querySelectorAll(".form-card [name]")].map(i=>[i.closest(".form-card").dataset.id,i.name,i.value]);
 const keepEdit=[...box.querySelectorAll(".ap-body")].map(t=>[t.closest(".approval").dataset.id,t.value,t.closest(".approval").querySelector(".ap-subject")?.value]);
 const sEl=statusEl();[...box.childNodes].forEach(n=>n!==sEl&&n.remove());if(sEl.parentNode!==box)box.append(sEl);const all=S.msgs[a.id]||[];const W=win[a.id]||(win[a.id]=PAGE);const L=all.length>W?all.slice(-W):all;const last={v:null};
 const first=stickFor!==a.id;if(first){stickFor=a.id;stick=true;nodeCache.clear()}const frag=document.createDocumentFragment();const mark=(n0,isNew)=>{if(isNew)[...frag.childNodes].slice(n0).forEach(n=>{if(!n.classList)return;n.classList.add("enter");n.addEventListener("animationend",()=>n.classList.remove("enter"),{once:true})})};
 if(all.length>W){const o=el("button","older","Xem tin nhắn cũ hơn");o.type="button";o.onclick=loadOlder;frag.append(o)}if(justSent)stick=true;let prevT=0;
 L.forEach((m,idx)=>{
  // keyed reuse: a message whose data and context did not change keeps its
  // DOM (no markdown re-parse, open folds and inputs stay as they are)
  const later=m.choices&&m.choices.length?L.slice(idx+1).some(x=>x.role==="user"):false;
  const sig=JSON.stringify(m)+"|"+(idx===L.length-1&&!m.text?a.status:"")+"|"+later+"|"+showThink+"|"+usedChoice.has(m.id)+"|"+last.v+"|"+prevT;
  const hit=nodeCache.get(m.id);if(hit&&hit.sig===sig){hit.nodes.forEach(n=>frag.append(n));last.v=hit.lastV;prevT=hit.prevT;return}
  const n0=frag.childNodes.length,isNew=!first&&!shownIds.has(m.id)&&!(m.role==="user"&&sentRecently(m.text));if(!first&&!shownIds.has(m.id))seenAt.set(m.id,Date.now());shownIds.add(m.id);if(m.role!=="agent")last.v=null;const t=T(m.t);if(m.role==="user"&&t>0&&(!prevT||t-prevT>15*60e3))frag.append(el("div","daystamp",dayStamp(t)));if(t>0)prevT=t;
  const cp=m.role==="agent"&&AURORA_UX.isCheckpoint(m.text),body=cp?m.text.trim().slice(0,-AURORA_UX.CHECKPOINT.length).trim():m.text;
  if(m.thinking&&m.role==="agent"&&showThink)frag.append(thinkCard(m));
  if(body&&!AURORA_UX.isNoise(m)){speaker(frag,m,last);const b=el("div","msg"+(m.role==="user"?" mine":m.role==="system"?" sys":" bot"));if(m.role==="agent"){if(isNew&&!reduceMotion())typeIn(b,body,m.sources,a.id);else{md(b,body,m.sources);chipify(b,a.id)}}else b.textContent=body;frag.append(b)}
  if(m.role==="user"&&m.ref)frag.append(el("div","note-line ref-line","Kèm ngữ cảnh: chat “"+m.ref+"”"));
  if(cp)frag.append(el("div","note-line","↻ Đang làm tiếp ở lượt sau"));
  if(m.plan)frag.append(planCard(m));
  if(m.choices&&m.choices.length){const wrap=el("div","choices");const later=L.slice(idx+1).some(x=>x.role==="user");m.choices.forEach(ch=>{const b=el("button","choice",ch);b.disabled=later||usedChoice.has(m.id);b.onclick=async()=>{if(usedChoice.has(m.id))return;usedChoice.add(m.id);renderThread();if(!(await sendMsg(ch)))usedChoice.delete(m.id)};wrap.append(b)});frag.append(wrap)}
  if(m.form)frag.append(formCard(m));
  if(m.tools&&m.tools.length&&(m.text||m.approval)&&!(a.status==="working"&&idx===L.length-1&&!m.text))frag.append(sourceList(m));
  if(m.approval)frag.append(approvalCard(m));if(m.secret)frag.append(secretCard(m));mark(n0,isNew);nodeCache.set(m.id,{sig,nodes:[...frag.childNodes].slice(n0),lastV:last.v,prevT})});
 box.insertBefore(frag,sEl);
 keepForm.forEach(([id,n,v])=>{const i=box.querySelector(`.form-card[data-id="${id}"] [name="${n}"]`);if(i&&v)i.value=v});
 keepSec.forEach(([id,n,v])=>{const i=box.querySelector(`.secret[data-id="${id}"] input[data-secret="${n}"]`);if(i&&!i.value)i.value=v});
 keepEdit.forEach(([id,v,sj])=>{const c=box.querySelector(`.approval[data-id="${id}"]`);if(c){const t=c.querySelector(".ap-body");if(t)t.value=v;const s2=c.querySelector(".ap-subject");if(s2&&sj!=null)s2.value=sj}});
 outbox.filter(o=>o.agentId===a.id).forEach(o=>{last.v=null;const b=el("div","msg mine pending"+(o.shown?"":" enter"),o.text);o.shown=1;if(o.wait)b.append(el("span","chip","đang gửi…"));frag.append(b)});
 Object.entries(S.drafts).forEach(([id,d])=>{if(d.agentId===a.id&&shown(d)){speaker(frag,{role:"agent",from:d.from},last);const b=el("div","msg bot draft"+(shownIds.has(id)?"":" enter"));shownIds.add(id);b.id="d_"+id;frag.append(b);if(reduceMotion())d.sl=1e9;drawDraft(id,b);requestAnimationFrame(()=>paintDraft(id))}});
 // an answer just landed (first seen < 2.5 s ago, nothing running): no status line while the bot state catches up
 const lm=L.at(-1),age=lm&&seenAt.has(lm.id)?Date.now()-seenAt.get(lm.id):1e9;
 const landed=lm&&lm.role==="agent"&&lm.text&&age<2500&&!(lm.tools||[]).some(t=>t.status==="running")&&!(lm.approval&&lm.approval.state==="pending");
 if(landed)setTimeout(()=>{if(cur&&cur.id===a.id)renderThread()},2600-age);
 if(!L.length&&!outbox.some(o=>o.agentId===a.id)&&!Object.values(S.drafts).some(d=>d.agentId===a.id))frag.append(emptyThread(a));
 box.insertBefore(frag,sEl);setStatus(a,L,!landed&&!Object.values(S.drafts).some(d=>d.agentId===a.id&&shown(d)));
 setSendMode();
 if(stick)requestAnimationFrame(()=>{box.scrollTop=box.scrollHeight;justSent=false;syncJump()})}
// long chats: the last PAGE messages render; older ones load on demand (button
// or scrolling to the top) with the scroll position kept
const PAGE=60,win={},nodeCache=new Map(),seenAt=new Map();
function loadOlder(){if(!cur)return;const box=$("#messages"),h=box.scrollHeight,top=box.scrollTop;win[cur.id]=(win[cur.id]||PAGE)+PAGE;stick=false;renderThread();box.scrollTop=top+box.scrollHeight-h}
// a finished answer that arrived whole (no stream) types in like a stream
function typeIn(b,body,src,id){let k=0;b.classList.add("typing");const step=()=>{if(!b.isConnected&&k>0){md(b,body,src);chipify(b,id);return}const gap=body.length-k;k+=Math.max(3,Math.ceil(gap/14));while(k<body.length&&!/\s/.test(body[k]))k++;
 if(k>=body.length){b.classList.remove("typing");md(b,body,src);chipify(b,id);if(stick){const x=$("#messages");x.scrollTop=x.scrollHeight}return}
 md(b,body.slice(0,k),src,true);if(stick){const x=$("#messages");x.scrollTop=x.scrollHeight}requestAnimationFrame(step)};md(b,body.slice(0,Math.min(body.length,24)),src,true);requestAnimationFrame(step)}
// PWA install: Android/Chrome gives a prompt event; iOS Safari only has the
// Share menu, so the cell explains it. Hidden inside the app shell or once installed.
let installEv=null;addEventListener("beforeinstallprompt",e=>{e.preventDefault();installEv=e});addEventListener("appinstalled",()=>{installEv=null;toast("Đã cài Neko Aurora")});
const installable=()=>!(CAP||matchMedia("(display-mode: standalone)").matches||navigator.standalone);
async function installApp(){if(installEv){installEv.prompt();try{await installEv.userChoice}catch{}installEv=null;return}toast("Trên iPhone: bấm nút Chia sẻ ở Safari, chọn “Thêm vào MH chính”")}
// first open of a bot with no messages: who it is and three starters
function emptyThread(a){const w=el("div","thread-empty enter");const f=el("div","te-face"),g=isGroup(a);
 const mem=g?(a.members||[]).map(byId).filter(Boolean):[];if(g&&mem.length){f.classList.add("stack");mem.slice(0,3).forEach(b=>f.append(face(b.av||{},48)))}else f.append(face(a.av||{},64));
 const names=mem.map(b=>b.name);const who=names.length>1?names.slice(0,-1).join(", ")+" và "+names.at(-1):names[0]||"";
 w.append(f,el("b",0,a.name),el("p",0,g?(who?"Nhóm gồm "+who+". Nhắn một lần, các bot cùng làm.":"Nhóm chat"):a.desc||"Sẵn sàng nhận việc."));
 const c=el("div","choices");(g?["Mỗi bạn giới thiệu ngắn một câu nhé","Chia việc giúp mình: lên kế hoạch tuần này","Cả nhóm cùng góp ý một ý tưởng"]:["Bạn giúp được mình những gì?","Gợi ý 3 việc mình nên giao cho bạn","Bắt đầu với một việc nhỏ hôm nay nhé"]).forEach(t=>{const b=el("button","choice",t);b.type="button";b.onclick=()=>sendMsg(t);c.append(b)});w.append(c);return w}
// Working status (one persistent node, so the mascot and shine never restart
// on a re-render): the bot's own mascot moves per phase (bob/scan/hop…, eyes
// blink), the phase text shines and crossfades when it changes, the stop
// button sits on the line. Hidden at once when the answer starts.
let stEl=null;const shownIds=new Set();const sent=new Map();const sentRecently=t=>{const x=sent.get(t);return x&&Date.now()-x<120e3};
function statusEl(){if(!stEl){stEl=el("div","status off");stEl.setAttribute("role","status");stEl.setAttribute("aria-live","polite");const x=el("button","st-stop");x.type="button";x.setAttribute("aria-label","Dừng");x.append(I("square",12));x.onclick=()=>stopAgent();stEl.append(el("span","st-mascot"),el("span","st-t"),x)}return stEl}
function phaseOf(a,L){const p=AURORA_UX.phase(a,L);if(!p)return null;if(p.k==="think"){const u=[...L].reverse().find(m=>m.role==="user");const s=u?(Date.now()-T(u.t))/1e3:0;p.t=s>20?"Đang hoàn thiện câu trả lời":s>8?"Đang sắp xếp ý":"Đang suy nghĩ"}return p}
function setStatus(a,L,on){const st=statusEl(),p=on&&phaseOf(a,L);st.classList.toggle("off",!p);if(!p)return;
 const key=a.id+JSON.stringify(a.av||{});if(st.dataset.av!==key){st.dataset.av=key;st.firstChild.replaceChildren(face(a.av||{},28))}
 st.dataset.k=p.k;st.lastChild.hidden=a.status!=="working";const t=st.children[1],old=t.querySelector("span:not(.out)");if(old&&old.textContent===p.t)return;
 if(old){old.classList.add("out");setTimeout(()=>old.remove(),260)}t.append(el("span",old?"in":"",p.t))}
setInterval(()=>{if(cur&&stEl&&!stEl.classList.contains("off"))setStatus(cur,S.msgs[cur.id]||[],true)},1000);
// download links in a reply become file chips (type, name, size; 48px tap target)
const pcSizes={};
function chipify(b,id){const links=[...b.querySelectorAll("a[href]")].filter(x=>AURORA_UX.isDownload(x.getAttribute("href")));if(!links.length)return;
 const paint=()=>links.forEach(x=>{const f=AURORA_UX.fileInfo(x.getAttribute("href"),pcSizes[id]);x.className="file-chip";x.removeAttribute("target");x.setAttribute("download",f.name);x.setAttribute("aria-label",`Tải ${f.name}`);x.replaceChildren(el("span","fc-ext",(f.ext||"file").toUpperCase().slice(0,4)),Object.assign(el("span","fc-txt"),{}));const t=x.lastChild;t.append(el("b",0,f.name),el("small",0,f.label+(f.size?" · "+f.size:"")))});
 paint();if(!pcSizes[id]){pcSizes[id]={};api(`/agents/${id}/computer`).then(st=>{(st.files||[]).forEach(f=>{if(!f.dir)pcSizes[id][f.path]=f.size});paint()}).catch(()=>{})}}
const srcOf=id=>{const m=(S.msgs[cur&&cur.id]||[]).find(x=>x.id===id);return m&&m.sources};
const atBottom=b=>b.scrollHeight-b.scrollTop-b.clientHeight<120;let justSent=false;
// stick: follow new text; a user scroll up releases it, scrolling back down or sending re-arms it
let stick=true,stickFor=null;
const dayStamp=t=>{const d=new Date(t),n=new Date(),y=new Date(n-864e5);const hm=d.toLocaleTimeString("vi-VN",{hour:"2-digit",minute:"2-digit"});return(d.toDateString()===n.toDateString()?"Hôm nay ":d.toDateString()===y.toDateString()?"Hôm qua ":d.toLocaleDateString("vi-VN",{day:"numeric",month:"numeric"})+" ")+hm};
function syncJump(){const b=$("#messages");$("#jumpBtn").classList.toggle("hidden",atBottom(b))}
const visible=t=>{const i=t.search(/(^|\n)\s*(APPROVAL|PLAN|CHOICES|TOOL|SECRET):|(APPROVAL|PLAN|CHOICES|TOOL|SECRET):\s*[\[{]/);return(i>=0?t.slice(0,i):t).trim()||"…"};
// A draft is painted only once it has a letter or digit: a lone "-" or "**"
// at the start of a streamed step must not flash as its own bubble.
function shown(d){const t=visible(d.text||"");return Date.now()-(d.t0||0)>=450&&/[\p{L}\p{N}]/u.test(t)&&/\s/.test(t.trim())} // a whole first word, never a lone "Đ"
// Streaming reveal: the draft shows words up to d.sl, which a frame loop
// walks toward the received text (faster the further behind), so a chunked
// stream reads as a steady typewriter. Reduced motion shows text as it lands.
const reveal={};
function paintDraft(id){const d=S.drafts[id];if(!d)return;const n=document.getElementById("d_"+id);if(!n){if(shown(d))renderThread();return}
 if(reduceMotion()){d.sl=1e9;drawDraft(id,n);return}
 if(reveal[id])return;reveal[id]=requestAnimationFrame(function step(){const d=S.drafts[id],n=document.getElementById("d_"+id);if(!d||!n){delete reveal[id];return}
  const full=visible(d.text).length,gap=full-(d.sl||0);if(gap<=0){delete reveal[id];return}d.sl=(d.sl||0)+Math.max(2,Math.ceil(gap/12));drawDraft(id,n);reveal[id]=requestAnimationFrame(step)})}
function drawDraft(id,n){const d=S.drafts[id],t=visible(d.text);let k=Math.min(t.length,d.sl||0);while(k<t.length&&!/\s/.test(t[k]))k++;if(k===t.length){const w=t.trimEnd().search(/\s\S*$/);if(w>0)k=w} // the word still arriving waits for its end
 n.replaceChildren();md(n,t.slice(0,k),srcOf(id),true);const box=$("#messages");if(stick)box.scrollTop=box.scrollHeight;syncJump()}
async function decide(m,v,bs,extra){if(deciding.has(m.id))return;deciding.add(m.id);bs.querySelectorAll("button").forEach(b=>b.disabled=true);
 try{await api(`/approvals/${m.id}`,{decision:v,...(extra||{})});buzz(v==="approved"?"confirm":"soft");editing.delete(m.id)}catch(e){if(e.status!==409){deciding.delete(m.id);toast("Không ghi nhận được, thử lại");bs.querySelectorAll("button").forEach(b=>b.disabled=false)}}}
// the composer never locks or turns into stop: voice when empty, send with text (stop lives on the status line)
function setSendMode(){const b=$("#composer .send");const voice=!$("#input").value.trim();b.classList.toggle("voice",voice);b.replaceChildren(I(voice?"audio-lines":"arrow-up"));b.setAttribute("aria-label",voice?"Nói":"Gửi")}
let stopping=false;async function stopAgent(){if(!cur||stopping)return;stopping=true;try{await api(`/agents/${cur.id}/stop`,{});buzz("soft");toast("Đã dừng")}catch{toast("Không dừng được")}finally{stopping=false}}

// Outbox: the composer never blocks. Every message gets a client nonce, shows
// at once, and is delivered in order; retries (offline, reconnect, 429/5xx)
// reuse the nonce so the server stores it exactly once. The running bot folds
// new messages into its current turn (server inbox), so nothing waits.
const outbox=[];let pumping=false;const sleep=ms=>new Promise(r=>setTimeout(r,ms));
const MAX_MSG=10000;
function sendMsg(text){const a=cur;text=text.trim();if(!a||!text)return false;
 if([...text].length>MAX_MSG){toast(`Tin quá dài (tối đa ${MAX_MSG.toLocaleString("vi-VN")} ký tự). Chia nhỏ hoặc gửi dạng file nhé.`);return false}
 const ref=pendingRef&&pendingRef.for===a.id?pendingRef.id:undefined;setRef(null);
 sent.set(text,Date.now());outbox.push({agentId:a.id,text,ref,n:"c"+Date.now().toString(36)+Math.random().toString(36).slice(2,10),t:Date.now()});buzz("tap");renderThread();pump();return true}
async function pump(){if(pumping)return;pumping=true;try{while(outbox.length){const it=outbox[0];
  try{const m=await api(`/agents/${it.agentId}/messages`,{text:it.text,clientNonce:it.n,ref:it.ref});outbox.shift();
   if(m&&m.id){const L=S.msgs[it.agentId]||(S.msgs[it.agentId]=[]);if(!L.some(x=>x.id===m.id))L.push(m)}if(cur&&cur.id===it.agentId)renderThread()}
  catch(e){const quota=e.status===429&&e.body&&e.body.error==="quota_exceeded";
   if(quota||e.status&&e.status>=400&&e.status<500&&e.status!==429){
    // final refusal: never retry, and give the text back so nothing is lost
    const drop=quota?outbox.filter(x=>x.agentId===it.agentId):[outbox[0]];for(const x of drop)outbox.splice(outbox.indexOf(x),1);
    const inp=$("#input");if(cur&&cur.id===it.agentId&&!inp.value.trim()){inp.value=drop.map(x=>x.text).join("\n");inp.dispatchEvent(new Event("input"))}
    toast(quota?`Hết lượt hôm nay (${e.body.daily} tin/ngày). Mai dùng tiếp nhé.`:e.status===413?"Tin quá dài, chia nhỏ giúp mình nhé":"Không gửi được, thử lại");if(cur&&cur.id===it.agentId)renderThread();continue}
   it.wait=true;if(cur&&cur.id===it.agentId)renderThread();await sleep(e.status===429?4000:!navigator.onLine?1500:2000)}}}finally{pumping=false}}
addEventListener("online",pump);
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
 fetch("/app/auth/me").then(r=>r.ok?r.json():null).then(me=>{if(!me)return;const q=me.quota||{};at.replaceChildren(el("b",0,me.name||"Bạn"),el("small",0,[me.email,q.daily?`Gói ${me.plan||"free"} · ${q.used||0}/${q.daily} tin hôm nay`:""].filter(Boolean).join(" · ")))}).catch(()=>{});
 B.append(card("Mức dùng",cell("sparkles","Mô hình AI",{static:1,value:S.ai.configured?(S.ai.model||"đã cấu hình"):"chưa cấu hình"}),cell("activity","Lõi chạy",{static:1,value:S.ai.runtime||"Neko Core"}),cell("users","Bot & nhóm",{static:1,value:`${bots().length} bot · ${S.agents.length-bots().length} nhóm`})));
 B.append(card("Ứng dụng",cell("layout-grid","Kết nối ứng dụng",{id:"openConnectors",value:connectedCount()+" đã kết nối",onClick:()=>{renderConnectors();swapSheet($("#connectors"))}})));
 B.append(card("Bot",cell("check","Luôn duyệt trước khi gửi",{toggle:true,locked:true,sub:"Không gì rời máy chủ khi bạn chưa bấm Duyệt"}),
  cell("bell","Thông báo",{id:"notifyCell",toggle:pushOn||notifOn,sub:pushOn?"Thông báo đẩy đang bật":notifOn?"Khi app đang mở":"Báo khi bot cần bạn",onToggle:sw=>{if(sw.getAttribute("aria-checked")==="true"){toast("Tắt trong Cài đặt của máy");return}enableNotify().then(renderSettings).catch(()=>toast("Không bật được thông báo"))}}),
  ...(installable()?[cell("smartphone","Cài app lên màn hình chính",{id:"installCell",sub:installEv?"Mở nhanh như ứng dụng, toàn màn hình":"Safari: bấm Chia sẻ rồi “Thêm vào MH chính”",onClick:installApp})]:[]),
  cell("smartphone","Phản hồi rung",{id:"hapticsCell",toggle:haptics,sub:CAP?"Rung nhẹ khi chạm":"Rung nhẹ khi chạm (Android)",onToggle:sw=>{haptics=!haptics;store.set("neko-aurora-haptics",haptics);sw.setAttribute("aria-checked",haptics);buzz("tap")}}),
  cell("message-circle","Ngôn ngữ",{id:"langCell",value:{auto:"Theo thiết bị",vi:"Tiếng Việt",en:"English"}[window.__I18N?__I18N.pref():"auto"],onClick:()=>{if(!window.__I18N)return;const order=["auto","vi","en"],p=__I18N.pref();__I18N.set(order[(order.indexOf(p)+1)%3])}}),
  cell("sun-moon","Giao diện",{id:"openThemes",value:(THEMES.find(t=>t.id===curTheme())||THEMES[0]).name,onClick:()=>{renderThemes();swapSheet($("#themes"))}})));
 const link=(icon,label,href)=>cell(icon,label,{ext:1,onClick:()=>window.open(href,"_blank","noopener")});
 B.append(card("Trợ giúp",link("message-circle","Trung tâm trợ giúp","https://github.com/meiiie/neko-aurora#readme"),link("check","Quyền riêng tư","/app/privacy.html"),link("list-checks","Điều khoản sử dụng","/app/terms.html"),link("git-branch","Giấy phép mã nguồn mở","/app/aurora/lucide-LICENSE.txt")));
 B.append(card(null,cell("log-out","Đăng xuất",{id:"logoutBtn",danger:1,onClick:logout}),cell("x","Xoá tài khoản & dữ liệu",{id:"delAcctBtn",danger:1,onClick:()=>{renderDelAcct();swapSheet($("#delAcct"))}})));
 const ft=el("footer","about");const lg=el("img");lg.src="/app/icon.svg";lg.alt="";lg.width=lg.height=36;ft.append(lg,el("b",0,"Neko Aurora"),el("small",0,"Phiên bản "+VERSION));B.append(ft)}
function renderDelAcct(){const B=$("#delAcctBody");B.innerHTML="";
 B.append(el("p","del-warn","Thao tác này xoá vĩnh viễn mọi bot, nhóm, tin nhắn, bản nháp chờ duyệt, khoá trong kho bí mật, kết nối và bản sao lưu trên máy chủ này. Không thể hoàn tác."));
 const f=field("Gõ XOÁ để xác nhận",{autocomplete:"off",autocapitalize:"characters",spellcheck:false});f.i.id="delAcctInput";
 const go=el("button","pill wide danger","Xoá vĩnh viễn");go.id="delAcctGo";go.disabled=true;
 f.i.oninput=()=>{go.disabled=!/^(xoá|xóa|delete)$/i.test(f.i.value.trim())};
 go.onclick=async()=>{go.disabled=true;go.textContent="Đang xoá…";try{await api("/account/delete",{confirm:f.i.value.trim().normalize("NFC").replace(/xóa/i,"XOÁ")});buzz("confirm");try{localStorage.clear()}catch{}if(es){es.close();es=null}closeSheet(true);toast("Đã xoá tài khoản và dữ liệu");setTimeout(()=>location.reload(),2500)}catch{go.disabled=false;go.textContent="Xoá vĩnh viễn";toast("Không xoá được, thử lại")}};
 B.append(f.w,go)}
$("#closeDelAcct").onclick=()=>closeSheet(true);
async function logout(){if(await fetch("/app/auth/config").then(r=>r.ok).catch(()=>false)){await fetch("/app/logout",{method:"POST",credentials:"same-origin",headers:{"Content-Type":"application/json"},body:"{}"}).catch(()=>{});location.replace("/app/signin.html");return}try{await fetch("/app/logout",{method:"POST",credentials:"same-origin",headers:{"Content-Type":"application/json"},body:"{}"})}catch{}closeSheet(true);if(es){es.close();es=null}const r=await fetch(API+"/state",{credentials:"same-origin",cache:"no-store"}).catch(()=>null);if(r&&r.status===401)$("#login").classList.remove("hidden");else toast("Máy chủ cục bộ: phiên luôn mở trên máy này")}
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
function secretCard(m){if(m.secret.vault)return vaultCard(m);const x=m.secret,saved=S.secrets.includes(x.name),c=el("div","secret"+(saved?" saved":""));c.dataset.id=m.id;c.dataset.name=x.name;
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
const THEMES=[{id:"auto",name:"Theo thiết bị",sub:"Sáng hoặc tối theo cài đặt máy · mặc định",sw:["#f7f7f7","#000000","#f5f5f5"],tc:""},{id:"light",name:"Sáng",sub:"Xám trung tính, nút đen",sw:["#f7f7f7","#ffffff","#0d0d0d"],tc:"#f7f7f7"},{id:"dark",name:"Tối",sub:"Nền đen, chữ trắng",sw:["#000000","#1c1c1e","#f5f5f5"],tc:"#000000"},{id:"cream",name:"Kem Aurora",sub:"Kem sáng, khăn đỏ",sw:["#f4efe4","#e0261f","#17130e"],tc:"#f4efe4"},{id:"gold",name:"Đen vàng",sub:"Nền đen, điểm vàng · tối",sw:["#0d0c0a","#d4a72c","#f5ecd7"],tc:"#0d0c0a"},{id:"blue",name:"Trắng xanh",sub:"Nền trắng, điểm xanh · sáng",sw:["#f5f8fc","#1d5fd1","#0f1b2d"],tc:"#f5f8fc"}];
const curTheme=()=>{try{const t=localStorage.getItem("neko-aurora-theme");return THEMES.some(x=>x.id===t)?t:"auto"}catch{return"auto"}};
const sysDark=matchMedia("(prefers-color-scheme: dark)");sysDark.addEventListener?.("change",()=>{if(curTheme()==="auto")applyTheme("auto")});
function applyTheme(id){const pick=THEMES.find(x=>x.id===id)||THEMES[0];try{pick.id==="auto"?localStorage.removeItem("neko-aurora-theme"):localStorage.setItem("neko-aurora-theme",pick.id)}catch{}
 const t=pick.id==="auto"?THEMES.find(x=>x.id===(sysDark.matches?"dark":"light")):pick;
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
 if(!isGroup(a)){const pc=el("div");pc.id="pcBox";B.append(pc);renderComputer(a.id,pc)}
 $("#pmPinT").textContent=a.pin?"Bỏ ghim":"Ghim";$("#pmPin").classList.toggle("hidden",isGroup(a));$("#pmArch").classList.toggle("hidden",isGroup(a))}
// bot computer: sandboxed home, recent commands, files, reset
const kb=n=>n<1024?n+" B":n<1048576?(n/1024).toFixed(1)+" KB":(n/1048576).toFixed(1)+" MB";
function renderComputer(id,box){api(`/agents/${id}/computer`).then(st=>{if(profId!==id)return;box.innerHTML="";
 const rows=[cell("monitor","Trạng thái",{static:1,value:st.available?"Sẵn sàng":"Đang tắt",sub:st.available?"Linux riêng · "+kb(st.usedBytes)+" / "+kb(st.quotaBytes):st.error||""})];
 box.append(card("Máy tính",...rows));
 if(st.log.length){const w=el("section","card-group");w.append(el("h3",0,"Lệnh gần đây"));const c=el("div","card pc-log");st.log.slice(-5).reverse().forEach(r=>{const d=el("details","pc-run");const s=el("summary");s.append(el("code",0,"$ "+r.cmd),el("span","pc-exit"+(r.exitCode?" bad":""),r.timedOut?"hết giờ":r.exitCode?"lỗi "+r.exitCode:"xong"));d.append(s,el("pre",0,r.output||"(không có đầu ra)"));c.append(d)});w.append(c);box.append(w)}
 const files=st.files.filter(f=>!f.dir).slice(0,12);
 if(files.length)box.append(card("File",...files.map(f=>cell("folder",f.path,{value:kb(f.size),onClick:()=>/\.(pptx?|docx?|xlsx?|pdf|png|jpe?g|gif|webp|mp4|webm|mov|zip|gz|tar)$/i.test(f.path)?(location.href=API+`/agents/${id}/computer/download?path=${encodeURIComponent(f.path)}`):api(`/agents/${id}/computer/file?path=${encodeURIComponent(f.path)}`).then(x=>{const old=box.querySelector(".pc-view");if(old)old.remove();const p=el("pre","pc-view",x.text+(x.truncated?"\n…":""));box.append(p);p.scrollIntoView({behavior:"smooth",block:"nearest"})}).catch(()=>toast("Không mở được file"))}))));
 box.append(card(null,cell("x","Làm mới máy tính",{danger:1,id:"pcReset",onClick:()=>{if(!confirm("Xoá mọi file và lịch sử lệnh trên máy tính của bot này?"))return;api(`/agents/${id}/computer/reset`,{}).then(()=>{toast("Đã làm mới");renderComputer(id,box)}).catch(()=>toast("Không làm mới được"))}})),el("p","caption","Mỗi bot có một máy Linux riêng, cách ly với máy chủ, mặc định không có mạng. Bot tự chạy lệnh trên máy này; việc gửi ra ngoài, tiêu tiền hay dùng khoá của bạn luôn chờ bạn duyệt."))
}).catch(()=>{box.innerHTML=""})}
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
// Grok-style composer: one line inline with mic/send; once text wraps, the field takes the full width and the buttons drop to a row below.
const inp=$("#input"),ask=inp.parentElement;inp.oninput=()=>{const was=ask.classList.contains("multi");if(!inp.value)ask.classList.remove("multi");inp.style.height="auto";if(!was&&inp.value&&inp.scrollHeight>52)ask.classList.add("multi");inp.style.height="auto";inp.style.height=Math.min(160,inp.scrollHeight)+"px";setSendMode()};
$("#composer").onsubmit=async e=>{e.preventDefault();stopVoice();const v=inp.value;if(!v.trim()){if(SR)$("#micBtn").click();else inp.focus();return}inp.value="";inp.style.height="";ask.classList.remove("multi");justSent=true;setSendMode();if(!sendMsg(v)){inp.value=v;inp.oninput()}};
inp.onkeydown=e=>{if(e.key==="Enter"&&!e.shiftKey&&!e.isComposing){e.preventDefault();$("#composer").requestSubmit()}};
document.addEventListener("visibilitychange",()=>{if(!document.hidden&&es)loadState().catch(()=>{})});
if("serviceWorker"in navigator)navigator.serviceWorker.register("/app/sw.js",{scope:"/app/"}).then(r=>r.pushManager?.getSubscription()).then(s=>{if(s)pushOn=true}).catch(()=>{});
function boot(){return loadState().then(()=>{$("#login").classList.add("hidden");if(!es)connect();const h=location.hash.slice(1);if(h)openThread(h)}).catch(async e=>{if(e.status===401){const c=await fetch("/app/auth/config").then(r=>r.ok?r.json():null).catch(()=>null);if(c&&c.hosted){location.replace("/app/signin.html");return}$("#login").classList.remove("hidden")}else toast("Không kết nối được máy chủ")})}
let logging=false;$("#loginForm").onsubmit=async e=>{e.preventDefault();if(logging)return;logging=true;try{const r=await fetch("/app/login",{method:"POST",credentials:"same-origin",headers:{"Content-Type":"application/json"},body:JSON.stringify({code:$("#loginCode").value})});if(!r.ok){toast(r.status===429?"Thử quá nhiều lần, đợi 5 phút":"Mã không đúng");return}$("#loginCode").value="";await boot()}catch{toast("Không kết nối được")}finally{logging=false}};
boot();
// ---------- Aurora icons (Lucide, ISC) + haptics ----------
function I(n,sz){return self.AURORA_ICONS?AURORA_ICONS.icon(n,sz):document.createTextNode("")}
function stepIcon(st){const m={pending:"circle",running:"loader-circle",done:"check",waiting:"pause",failed:"x",cancelled:"minus"};const s=el("span","ic");s.append(I(m[st]||"circle",16));return s}
// Haptics: rare and short; owner can turn them off; vibrate is a no-op on iOS Safari.
// iOS shell (Capacitor): Safari has no navigator.vibrate, so use native Taptic haptics there.
function buzz(k){try{if(CAP&&CAP.Haptics){if(!haptics||matchMedia("(prefers-reduced-motion: reduce)").matches)return;k==="confirm"?CAP.Haptics.notification({type:"SUCCESS"}):CAP.Haptics.impact({style:k==="soft"?"MEDIUM":"LIGHT"});return}if(!haptics||matchMedia("(prefers-reduced-motion: reduce)").matches||!navigator.vibrate)return;navigator.vibrate({tap:8,confirm:[10,40,14],soft:16}[k]||8)}catch{}}
[["#newBtn","plus"],["#threadMore","ellipsis"],["#plusBtn","plus"],["#profBack","chevron-left"],["#profShare","share"],["#profMore","ellipsis"],["#appsBtn","layout-grid"],["#searchIc","search"],["#backBtn","chevron-left"],["#jumpBtn","chevron-down"],["#micBtn","mic"],["#cancelBtn","x"],["#closeGroup","x"],["#closeSetup","chevron-left"],["#closeSettings","x"],["#closeConnectors","x"]].forEach(([q,n])=>{const b=$(q);if(b)b.replaceChildren(I(n))});
document.querySelectorAll(".mi[data-i]").forEach(s=>s.replaceChildren(I(s.dataset.i,18)));
setSendMode();
$("#messages").addEventListener("scroll",()=>{const b=$("#messages");stick=atBottom(b);syncJump();if(b.scrollTop<80&&cur&&(S.msgs[cur.id]||[]).length>(win[cur.id]||PAGE)&&!olderBusy){olderBusy=true;requestAnimationFrame(()=>{loadOlder();olderBusy=false})}},{passive:true});let olderBusy=false;
if(window.visualViewport)visualViewport.addEventListener("resize",()=>{if(cur&&stick){const b=$("#messages");b.scrollTop=b.scrollHeight}});if(window.ResizeObserver)new ResizeObserver(syncJump).observe($("#messages"));$("#jumpBtn").onclick=()=>{const b=$("#messages");b.scrollTo({top:b.scrollHeight,behavior:"smooth"});buzz("tap")};
window.__AURORA={state:()=>S,reload:loadState,current:()=>cur&&cur.id,render:()=>renderThread()};
// iOS shell: APNs registration (permission asked once, only after login; push is optional for the app to work)
if(CAP&&CAP.PushNotifications){const P=CAP.PushNotifications;P.addListener("registration",t=>{api("/push/apns",{token:t.value}).catch(()=>{})});P.addListener("pushNotificationActionPerformed",a=>{const u=a.notification&&a.notification.data&&a.notification.data.url;if(u&&u.startsWith("/app/"))location.href=u});P.checkPermissions().then(r=>r.receive==="granted"?P.register():r.receive==="prompt"?P.requestPermissions().then(x=>x.receive==="granted"&&P.register()):0).catch(()=>{})}
// ---------- memory panel, presets, usage, search (DS Bot ideas; Apache-2.0 credit in THIRD_PARTY_NOTICES) ----------

// ---------- memory panel ----------
let memBot=null;
const BY={bot:"bot tự ghi",review:"tự nhận ra",owner:"bạn"},OP={add:"Thêm",edit:"Sửa",delete:"Xoá"};
function memRow(it,i,team){const li=el("li","mem-it");const t=el("p","mem-t",it.text);const meta=el("small","mem-m",new Date(it.at).toLocaleDateString("vi-VN"));li.append(t,meta);
 li.onclick=()=>{if(li.classList.contains("editing"))return;li.classList.add("editing");li.innerHTML="";const ta=el("textarea","mem-edit");ta.value=it.text;ta.maxLength=300;ta.rows=3;const bar=el("div","mem-bar");
  const del=el("button","pill danger","Xoá"),cancel=el("button","pill soft","Huỷ"),save=el("button","pill","Lưu");bar.append(del,cancel,save);li.append(ta,bar);ta.focus();
  cancel.onclick=e=>{e.stopPropagation();openMemory(memBot,true)};
  save.onclick=e=>{e.stopPropagation();const v=ta.value.trim();if(!v)return;api(`/agents/${memBot}/memory/${i}`,{text:v,team}).then(()=>{toast("Đã lưu");openMemory(memBot,true)}).catch(()=>toast("Không lưu được (không ghi mật khẩu/khoá)"))};
  del.onclick=async e=>{e.stopPropagation();if(del.dataset.armed!=="1"){del.dataset.armed="1";del.textContent="Bấm lần nữa để xoá";return}
   const r=await fetch(API+`/agents/${memBot}/memory/${i}${team?"?scope=team":""}`,{method:"DELETE",credentials:"same-origin"}).catch(()=>null);if(r&&r.ok){toast("Đã xoá");openMemory(memBot,true)}else toast("Không xoá được")}};
 return li}
function memSection(title,items,team,note){const w=el("section","card-group");w.append(el("h3",0,title+" · "+items.length));const ul=el("ul","card mem-list");
 if(!items.length)ul.append(el("li","mem-empty",note));items.forEach((it,i)=>ul.append(memRow(it,i,team)));w.append(ul);return w}
async function openMemory(id,keep){memBot=id;const a=byId(id);const B=$("#memBody");if(!keep){B.innerHTML="";B.append(el("p","caption","Đang tải…"));if(sheetOpen)swapSheet($("#memSheet"));else openSheet($("#memSheet"))}
 let m;try{m=await api(`/agents/${id}/memory`)}catch{B.innerHTML="";B.append(el("p","caption","Không tải được bộ nhớ"));return}
 B.innerHTML="";$("#memTitle").textContent="Bộ nhớ · "+(a?a.name:"");
 const add=el("form","mem-add");const inp=el("input");inp.placeholder="Thêm điều bot nên nhớ…";inp.maxLength=300;inp.id="memInput";const sc=el("label","mem-scope");const cb=el("input");cb.type="checkbox";cb.id="memTeam";sc.append(cb,el("span",0,"Cho mọi bot"));const go=el("button","pill","Thêm");go.type="submit";add.append(inp,sc,go);
 add.onsubmit=e=>{e.preventDefault();const v=inp.value.trim();if(!v)return;api(`/agents/${id}/memory`,{text:v,team:cb.checked}).then(()=>{toast("Đã ghi nhớ");openMemory(id,true)}).catch(()=>toast("Không ghi được (không ghi mật khẩu/khoá)"))};
 B.append(add,memSection("Riêng "+(a?a.name:"bot"),m.items||[],false,"Chưa có gì. Bot tự ghi khi bạn nêu sở thích lâu dài."),memSection("Chung cho mọi bot",m.team||[],true,"Tên, cách xưng hô, ngôn ngữ… áp dụng cho mọi bot."));
 const j=m.journal||[];if(j.length){const d=el("details","mem-log");d.append(el("summary",0,"Lịch sử thay đổi ("+j.length+")"));const ol=el("ol");j.forEach(c=>{const li=el("li");li.append(el("b",0,(OP[c.op]||c.op)+(c.bot==="_team"?" · chung":"")),el("span",0," "+(c.after||c.before||"")),el("small",0," — "+(BY[c.by]||c.by)+", "+ago(c.at)));if(c.op==="edit"&&c.before)li.append(el("s",0,c.before));ol.append(li)});d.append(ol);B.append(d)}
 B.append(el("p","caption","Bộ nhớ là dữ liệu, không phải lệnh. Không bao giờ lưu mật khẩu hay khoá."))};
$("#closeMem").onclick=()=>closeSheet(true);
// ---------- permission presets ----------
const PRESETS=[["readonly","Chỉ đọc","Chỉ xem, đọc file; mọi thay đổi chờ bạn duyệt"],["workspace","Máy riêng","Tự làm trên máy riêng của bot, không mạng"],["full","Toàn quyền","Có mạng; gửi ra ngoài vẫn hỏi bạn"]];
function presetCard(id,box){api(`/agents/${id}/computer`).then(st=>{const p=st.policy||{};const cur=p.preset||(p.network?"full":"workspace");const w=el("section","card-group");w.id="presetBox";w.append(el("h3",0,"Quyền của bot"));const seg=el("div","seg");seg.setAttribute("role","radiogroup");
 PRESETS.forEach(([k,n])=>{const b=el("button","seg-b",n);b.dataset.preset=k;b.setAttribute("role","radio");b.setAttribute("aria-checked",k===cur);b.onclick=()=>api(`/agents/${id}/computer/policy`,{preset:k}).then(np=>{buzz("tap");toast("Quyền: "+n);presetCard(id,box)}).catch(()=>toast("Không lưu được"));seg.append(b)});
 w.append(seg,el("p","caption",PRESETS.find(x=>x[0]===cur)[2]));const old=box.querySelector("#presetBox");old?old.replaceWith(w):box.prepend(w)}).catch(()=>{})}
// ---------- profile hooks ----------
const _rp=renderProfile;renderProfile=function(){_rp();const a=byId(profId);if(!a)return;const B=$("#profBody");if(isGroup(a)){B.append(card("Ghi nhớ",cell("sparkles","Bộ nhớ nhóm",{id:"prMem",sub:"Điều cả nhóm nhớ, chỉ dùng trong nhóm này",onClick:()=>openMemory(a.id)})));return}schedCard(a.id,B);
 const c=card("Ghi nhớ",cell("sparkles","Bộ nhớ",{id:"prMem",value:"",sub:"Xem, sửa, xoá điều bot nhớ về bạn",onClick:()=>openMemory(a.id)}));const pc=$("#pcBox");pc?B.insertBefore(c,pc):B.append(c);
 api(`/agents/${a.id}/memory`).then(m=>{const v=c.querySelector(".cell-v")||el("span","cell-v");v.textContent=((m.items||[]).length+(m.team||[]).length)+" mục";const ch=c.querySelector(".chev");ch?ch.before(v):c.querySelector(".cell").append(v)}).catch(()=>{});
 if(pc)presetCard(a.id,pc)};
// ---------- usage page ----------
let usageRange="7";
const fmtTok=n=>n>=1e6?(n/1e6).toFixed(1)+"M":n>=1e3?(n/1e3).toFixed(1)+"K":String(n);
const KIND={chat:"Trò chuyện",checkpoint:"Tóm tắt ngữ cảnh",memory:"Ghi nhớ",budget:"Quyết định khi hết giờ",factcheck:"Kiểm tra số liệu",review:"Rà soát file",rewrite:"Sửa tiếng Việt",brief:"Bản tin"};
async function openUsage(keep){const B=$("#usageBody");if(!keep){B.innerHTML="";swapSheet($("#usageSheet"))}
 let u;try{u=await api("/usage?range="+usageRange)}catch{B.innerHTML="";B.append(el("p","caption","Không tải được"));return}
 B.innerHTML="";const seg=el("div","seg");[["1","24 giờ"],["7","7 ngày"],["30","30 ngày"],["365","12 tháng"]].forEach(([k,n])=>{const b=el("button","seg-b",n);b.setAttribute("aria-checked",k===usageRange);b.onclick=()=>{usageRange=k;openUsage(true)};seg.append(b)});
 const hero=el("div","use-hero");hero.append(el("b",0,"≈ "+fmtTok(u.total.tokens)+" token"),el("small",0,u.total.calls+" lượt gọi mô hình · "+(S.ai.model||"")));B.append(seg,hero);
 const days=u.days||[],mx=Math.max(1,...days.map(d=>d.tokens));if(days.length>1){const ch=el("div","use-bars");days.slice(-31).forEach(d=>{const c=el("i");c.style.height=Math.max(3,Math.round(d.tokens/mx*72))+"px";c.title=d.key+": "+fmtTok(d.tokens);ch.append(c)});B.append(ch)}
 B.append(card("Theo bot",...(u.bots||[]).map(r=>cell("users",u.names[r.key]||r.key,{static:1,value:fmtTok(r.tokens),sub:r.calls+" lượt"}))));
 B.append(card("Theo loại",...(u.kinds||[]).map(r=>cell("activity",KIND[r.key]||r.key,{static:1,value:fmtTok(r.tokens),sub:r.calls+" lượt"}))));
 B.append(el("p","caption","Số token là ước tính từ độ dài văn bản gửi và nhận."))};
$("#closeUsage").onclick=()=>closeSheet(true);
const _rs=renderSettings;renderSettings=function(){_rs();const B=$("#settingsBody");const c=card("Sử dụng",cell("activity","Mức dùng AI",{id:"openUsage",sub:"Token theo ngày, theo bot và theo loại",onClick:()=>openUsage()}));const last=B.querySelectorAll(".card-group");last.length>1?B.insertBefore(c,last[last.length-1]):B.append(c)};
// ---------- chat search ----------
let hits=[],hitQ="",tmr=0;
// search snippets are plain text: markdown marks, citation brackets and directives are dropped
const plainSnip=t=>String(t||"").replace(/\*\*|__|`+|^#+\s*|\[(\d+)\]|!?\[([^\]]*)\]\([^)]*\)/gm,(m,n,l)=>l||"").replace(/\s+/g," ").trim();
const _rl=renderList;renderList=function(){_rl();const q=filter.trim();if(q.length<2||hitQ!==q||!hits.length)return;$("#empty").classList.add("hidden");const ul=$("#rows");const h=el("li","sec-h");h.setAttribute("role","heading");h.append(el("span",0,"Tin nhắn"));ul.append(h);
 hits.forEach(x=>{const li=el("li","hit-row");const b=el("button","hit-b");const a=byId(x.agentId);const av=el("span","row-av");if(a)av.append(avatarOf(a,36));const t=el("span","hit-t");t.append(el("b",0,x.name+(x.role==="user"?" · bạn":"")),el("span",0,plainSnip(x.snippet)));b.append(av,t);b.onclick=()=>{openThread(x.agentId);setTimeout(()=>{const f=$("#findInput");if(f){$("#findBar").classList.remove("hidden");f.value=q;runFind()}},700)};li.append(b);ul.append(li)})};
$("#search").addEventListener("input",()=>{clearTimeout(tmr);const q=filter.trim();if(q.length<2){hits=[];hitQ="";return}tmr=setTimeout(()=>api("/search?q="+encodeURIComponent(q)).then(r=>{if(filter.trim()!==q)return;hits=r.hits||[];hitQ=q;renderList()}).catch(()=>{}),250)});
// ---------- reference a chat (DS Bot "Reference a chat") ----------
let pendingRef=null;
function setRef(r){pendingRef=r;const box=$("#refChip");if(box)box.remove();if(!r)return;const c=el("div","ref-chip");c.id="refChip";c.append(el("span",0,"Kèm chat: "+r.name));const x=el("button","text-btn","Bỏ");x.onclick=()=>setRef(null);c.append(x);const comp=$("#composer")||$("#msgInput")?.closest("form")||$("#msgInput")?.parentElement;comp&&comp.parentElement.insertBefore(c,comp)}
$("#pmRef").onclick=()=>{closeMenu();if(!cur)return;const B=$("#refBody");B.innerHTML="";const L=S.agents.filter(x=>x.id!==cur.id);
 B.append(card(null,...L.map(x=>cell(isGroup(x)?"users":"message-circle",x.name,{sub:x.snip||"",onClick:()=>{setRef({id:x.id,name:x.name,for:cur.id});closeSheet(true);toast("Sẽ kèm chat “"+x.name+"”")}}))),el("p","caption","Tin nhắn gần nhất của chat đó được gửi kèm làm ngữ cảnh cho tin tiếp theo của bạn."));openSheet($("#refSheet"))};
$("#closeRef").onclick=()=>closeSheet(true);
// ---------- schedules on the profile ----------
const RPT={daily:"Mỗi ngày",weekdays:"Thứ 2–6",once:"Một lần"};
function schedCard(id,B){api("/schedules?bot="+encodeURIComponent(id)).then(r=>{const L=r.schedules||[];const old=B.querySelector("#schedBox");if(old)old.remove();if(!L.length&&!old)return;
 const w=card("Lịch hẹn",...(L.length?L.map(x=>cell("clock",x.at+" · "+(RPT[x.repeat]||x.repeat)+(x.date?" "+x.date.split("-").reverse().join("/"):""),{sub:x.text,value:"Xoá",onClick:()=>api(`/schedules/${x.id}/delete`,{}).then(()=>{toast("Đã xoá lịch");schedCard(id,B)}).catch(()=>toast("Không xoá được"))})):[cell("clock","Chưa có lịch hẹn",{static:1})]));w.id="schedBox";const pc=$("#pcBox");pc?B.insertBefore(w,pc):B.append(w)}).catch(()=>{})}

// ---------- extras: model menu, providers, thinking, skills, MCP, folder, forms, images (DS Bot ideas, Apache-2.0 credit) ----------
let showThink=localStorage.getItem("aurora.think")!=="0";
function thinkCard(m){const d=el("details","think");d.append(el("summary",0,"Suy nghĩ của bot"));const p=el("pre",0,m.thinking.slice(0,6000));d.append(p);return d}
function formCard(m){const f=m.form;const w=el("form","card form-card");w.dataset.id=m.taskId;w.append(el("h4",0,f.title));const ins=[];
 f.fields.forEach((x,i)=>{const l=el("label","fm-row");l.append(el("span",0,x.label+(x.required?" *":"")));let inp;
  if(x.type==="select"){inp=el("select");(x.options||[]).forEach(o=>{const op=el("option",0,o);op.value=o;inp.append(op)})}else if(x.type==="textarea"){inp=el("textarea");inp.rows=3}else{inp=el("input");inp.type=x.type==="number"?"number":x.type==="date"?"date":"text"}
  inp.name="f"+i;if(x.required)inp.required=true;if(f.done)inp.disabled=true;l.append(inp);ins.push([x,inp]);w.append(l)});
 const go=el("button","pill",f.done?"Đã gửi":"Gửi");go.type="submit";go.disabled=!!f.done;w.append(go);
 w.onsubmit=async e=>{e.preventDefault();const txt="📋 "+f.title+"\n"+ins.map(([x,i])=>"- "+x.label+": "+(i.value||"(bỏ trống)")).join("\n");go.disabled=true;if(await sendMsg(txt)){api(`/forms/${m.taskId}/done`,{}).catch(()=>{});f.done=true}else go.disabled=false};return w}
const CAPS=[["web","Tìm & đọc web"],["computer","Máy tính"],["browser","Trình duyệt"],["mcp","Máy chủ MCP"],["bots","Tạo/sửa bot"],["skills","Kỹ năng"]];
function sel(opts,cur){const s=el("select","ext-sel");opts.forEach(([v,n])=>{const o=el("option",0,n);o.value=v;if(v===cur)o.selected=true;s.append(o)});return s}
function field(label,node,hint){const l=el("label","fm-row");l.append(el("span",0,label),node);if(hint)l.append(el("small","caption",hint));return l}
async function openExt(id,keep){const B=$("#extBody");const a=byId(id);if(!keep){B.innerHTML="";B.append(el("p","caption","Đang tải…"));$("#extTitle").textContent="AI & công cụ · "+(a?a.name:"");sheetOpen?swapSheet($("#extSheet")):openSheet($("#extSheet"))}
 let x;try{x=await api(`/agents/${id}/ext`)}catch{B.innerHTML="";B.append(el("p","caption","Không tải được"));return}
 B.innerHTML="";const save=(p,msg)=>api(`/agents/${id}/ext`,p).then(()=>{buzz("tap");toast(msg||"Đã lưu");openExt(id,true)}).catch(()=>toast("Không lưu được"));
 const ms=sel([["","Mặc định máy chủ"],...x.menu.map(m=>[m.id,m.label+" · "+m.provider])],x.model||"");ms.id="extModel";ms.onchange=()=>save({model:ms.value},"Đã đổi mô hình");
 const g1=el("section","card-group");g1.append(el("h3",0,"Mô hình"));const c1=el("div","card pad");c1.append(field("Mô hình trả lời",ms,"Thêm nhà cung cấp và khoá riêng ở Cài đặt › Mô hình."));g1.append(c1);
 const th=card(null,cell("sparkles","Hiện suy nghĩ",{id:"extThink",sub:"Bot suy nghĩ trước khi trả lời (chậm hơn); phần suy nghĩ gập lại dưới tin nhắn",toggle:!!x.thinking,onToggle:sw=>save({thinking:sw.getAttribute("aria-checked")!=="true"})}));g1.append(th);
 const fi=el("input","ext-in");fi.id="extFolder";fi.placeholder="vd. du-an/bao-cao";fi.value=x.folder||"";const fb=el("button","pill","Lưu");fb.type="button";fb.onclick=()=>save({folder:fi.value},"Đã đặt thư mục làm việc");const fr=el("div","row-in");fr.append(fi,fb);
 const g2=el("section","card-group");g2.append(el("h3",0,"Thư mục làm việc"));const c2=el("div","card pad");c2.append(field("Trong /home/bot",fr,"Lệnh và file của bot mặc định nằm trong thư mục này."));g2.append(c2);
 const off=x.off||{};const g3=card("Khả năng",...CAPS.map(([k,n])=>cell("check",n,{id:"cap_"+k,toggle:!off[k],onToggle:sw=>{const o={...off};if(sw.getAttribute("aria-checked")==="true")o[k]=true;else delete o[k];save({off:o})}})));
 const on=new Set(x.skills||[]);const g4=card("Kỹ năng"+(on.size?"":" · tất cả"),...(x.allSkills||[]).map(k=>cell("sparkles",k.name,{sub:k.desc,toggle:!on.size||on.has(k.id),onToggle:sw=>{let l=on.size?[...on]:(x.allSkills||[]).map(z=>z.id);l=sw.getAttribute("aria-checked")==="true"?l.filter(z=>z!==k.id):[...l,k.id];l.length?save({skills:l.length===(x.allSkills||[]).length?[]:l}):save({off:{...off,skills:true}})}})));
 const mcp=x.mcp||[];const strip=()=>mcp.map(m=>({name:m.name,url:m.url,header:m.header||undefined,secret:m.secret||undefined}));
 const g5=card("Máy chủ MCP",...mcp.map((m,i)=>cell("link",m.name,{sub:m.error?("Lỗi: "+m.error):(m.tools||[]).map(t=>t.name).join(", ")||"chưa có công cụ",value:"Gỡ",onClick:()=>{const l=strip();l.splice(i,1);save({mcp:l},"Đã gỡ máy chủ")}})));
 const mf=el("form","mcp-add");const mn=el("input","ext-in");mn.placeholder="Tên (vd. deepwiki)";mn.id="mcpName";const mu=el("input","ext-in");mu.placeholder="https://…/mcp hoặc …/sse";mu.id="mcpUrl";mu.inputMode="url";const mh=el("input","ext-in");mh.placeholder="Header xác thực (tuỳ chọn, vd. Authorization)";const msx=el("input","ext-in");msx.placeholder="Tên bí mật trong kho (vd. MCP_TOKEN)";const mb=el("button","pill","Thêm máy chủ");mb.type="submit";
 mf.append(mn,mu,mh,msx,mb);mf.onsubmit=e=>{e.preventDefault();if(!mn.value.trim()||!mu.value.trim())return;save({mcp:[...strip(),{name:mn.value.trim(),url:mu.value.trim(),header:mh.value.trim()||undefined,secret:msx.value.trim()||undefined}]},"Đã thêm máy chủ MCP")};
 g5.append(mf,el("p","caption","Bot gọi thẳng công cụ của máy chủ đã thêm. Khoá đặt trong kho bí mật, không bao giờ hiện ra."));
 B.append(g1,g2,g3,g4,g5)}
$("#closeExt").onclick=()=>closeSheet(true);
async function openModels(keep){const B=$("#extBody");if(!keep){B.innerHTML="";$("#extTitle").textContent="Mô hình & nhà cung cấp";sheetOpen?swapSheet($("#extSheet")):openSheet($("#extSheet"))}
 let x;try{x=await api("/models")}catch{B.innerHTML="";B.append(el("p","caption","Không tải được"));return}B.innerHTML="";
 B.append(card("Nhà cung cấp",cell("sparkles","GLM (mặc định)",{static:1,sub:"Có sẵn trên máy chủ",value:"✓"}),...(x.providers||[]).map(p=>cell("link",p.name,{sub:p.base+" · "+p.models.join(", ")+(p.hasKey?" · có khoá":" · chưa có khoá"),value:"Xoá",onClick:()=>api(`/providers/${p.id}/delete`,{}).then(()=>{toast("Đã xoá");openModels(true)})}))));
 const f=el("form","mcp-add");f.id="provForm";const ins=[["id","Mã (vd. openai)"],["name","Tên hiển thị"],["base","Địa chỉ API (vd. https://api.openai.com/v1)"],["models","Các mô hình, cách nhau dấu phẩy"]].map(([k,p])=>{const i=el("input","ext-in");i.name=k;i.placeholder=p;f.append(i);return i});
 const key=el("input","ext-in");key.type="password";key.autocomplete="off";key.name="key";key.placeholder="Khoá API (lưu vào kho bí mật)";f.append(key);const b=el("button","pill","Lưu nhà cung cấp");b.type="submit";f.append(b);
 f.onsubmit=e=>{e.preventDefault();const [id,name,base,models]=ins.map(i=>i.value.trim());api("/providers",{id,name,base,models:models.split(",").map(s=>s.trim()).filter(Boolean),key:key.value||undefined}).then(()=>{key.value="";toast("Đã lưu nhà cung cấp");openModels(true)}).catch(()=>toast("Không lưu được (kiểm tra mã và địa chỉ https)"))};
 const g=el("section","card-group");g.append(el("h3",0,"Thêm nhà cung cấp tương thích OpenAI"));const c=el("div","card pad");c.append(f,el("p","caption","Khoá chỉ được ghi vào kho bí mật mã hoá; không bao giờ hiện lại hay gửi cho mô hình."));g.append(c);B.append(g);
 const vs=sel(x.models.map(m=>[m.id,m.label+" · "+m.provider]).concat(x.models.some(m=>m.id===x.vision)?[]:[[x.vision,x.vision]]),x.vision);vs.id="visionSel";vs.onchange=()=>api("/vision",{model:vs.value}).then(()=>toast("Đã đổi mô hình đọc ảnh")).catch(()=>toast("Không lưu được"));
 const g2=el("section","card-group");g2.append(el("h3",0,"Mô hình đọc ảnh"));const c2=el("div","card pad");c2.append(field("Khi bạn gửi ảnh",vs,"Mô hình trả lời chỉ đọc chữ; ảnh được mô hình này mô tả lại."));g2.append(c2);B.append(g2);
 const tg=card("Hiển thị",cell("sparkles","Hiện phần suy nghĩ",{id:"thinkShow",sub:"Với bot bật “Hiện suy nghĩ”",toggle:showThink,onToggle:sw=>{showThink=sw.getAttribute("aria-checked")!=="true";localStorage.setItem("aurora.think",showThink?"1":"0");sw.setAttribute("aria-checked",showThink)}}),cell("activity","Trình duyệt của bot",{static:1,value:x.browser?"Có":"Đọc trang thường",sub:x.browser?"Trình duyệt không giao diện, chỉ đọc":"Máy chủ này không có Chrome; dùng trình đọc trang"}));B.append(tg)}
async function openSkills(keep){const B=$("#extBody");if(!keep){B.innerHTML="";$("#extTitle").textContent="Kỹ năng";sheetOpen?swapSheet($("#extSheet")):openSheet($("#extSheet"))}
 let x;try{x=await api("/skills")}catch{return}B.innerHTML="";
 B.append(card("Kỹ năng",...x.skills.map(k=>cell("sparkles",k.name,{sub:k.desc,value:k.id.startsWith("sk-")?"Xoá":"",onClick:()=>{if(k.id.startsWith("sk-"))api(`/skills/${k.id}/delete`,{}).then(()=>openSkills(true))}}))));
 const f=el("form","mcp-add");f.id="skillForm";const n=el("input","ext-in");n.placeholder="Tên kỹ năng";const d=el("input","ext-in");d.placeholder="Dùng khi nào (một dòng)";const b=el("textarea","ext-in");b.rows=4;b.placeholder="Hướng dẫn từng bước cho bot";const go=el("button","pill","Thêm kỹ năng");go.type="submit";f.append(n,d,b,go);
 f.onsubmit=e=>{e.preventDefault();api("/skills",{name:n.value.trim(),desc:d.value.trim(),body:b.value.trim()}).then(()=>{toast("Đã thêm kỹ năng");openSkills(true)}).catch(()=>toast("Cần tên và hướng dẫn"))};
 const g=el("section","card-group");g.append(el("h3",0,"Kỹ năng mới"));const c=el("div","card pad");c.append(f);g.append(c,el("p","caption","Bot chỉ thấy tên và mô tả; khi việc khớp, bot đọc hướng dẫn đầy đủ."));B.append(g)}
const _rp2=renderProfile;renderProfile=function(){_rp2();const a=byId(profId);if(!a||isGroup(a))return;const B=$("#profBody");const c=card("AI & công cụ",cell("sparkles","Mô hình, suy nghĩ, kỹ năng, MCP",{id:"prExt",sub:"Thư mục làm việc và các khả năng của bot",onClick:()=>openExt(a.id)}));const m=$("#prMem");const g=m&&m.closest(".card-group");g?g.after(c):B.append(c)};
const _rs2=renderSettings;renderSettings=function(){_rs2();const B=$("#settingsBody");const c=card("AI",cell("sparkles","Mô hình & nhà cung cấp",{id:"openModels",sub:"GLM mặc định, thêm khoá riêng, mô hình đọc ảnh",onClick:()=>openModels()}),cell("list-checks","Kỹ năng",{id:"openSkills",sub:"Hướng dẫn bot dùng lại cho việc lặp lại",onClick:()=>openSkills()}));const u=$("#openUsage");const g=u&&u.closest(".card-group");g?g.before(c):B.append(c)};
// memory: ask + summarize
const _om=openMemory;openMemory=async function(id,keep){await _om(id,keep);const B=$("#memBody");if(!B.querySelector(".mem-add"))return;const bar=el("div","mem-tools");const ask=el("button","pill soft","Hỏi bộ nhớ");ask.id="memAsk";const sum=el("button","pill soft","Gộp trùng lặp");sum.id="memSum";bar.append(ask,sum);B.querySelector(".mem-add").after(bar);
 ask.onclick=()=>{if(B.querySelector(".mem-q"))return;const f=el("form","mem-add mem-q");const i=el("input");i.placeholder="vd. Bot biết gì về lịch làm việc của mình?";const g=el("button","pill","Hỏi");f.append(i,g);bar.after(f);i.focus();f.onsubmit=e=>{e.preventDefault();if(!i.value.trim())return;g.disabled=true;api(`/agents/${id}/memory/ask`,{q:i.value.trim()}).then(r=>{const p=el("p","mem-ans",r.answer);f.after(p);g.disabled=false}).catch(()=>{g.disabled=false;toast("Không hỏi được")})}};
 sum.onclick=()=>{sum.disabled=true;sum.textContent="Đang gộp…";api(`/agents/${id}/memory/summarize`,{}).then(r=>{sum.disabled=false;sum.textContent="Gộp trùng lặp";const L=r.proposal||[];if(!L.length){toast("Không có gì để gộp");return}const w=el("div","card mem-prop");w.append(el("b",0,"Đề xuất ("+r.before+" → "+L.length+" mục)"));const ul=el("ul");L.forEach(t=>ul.append(el("li",0,t)));const ok=el("button","pill","Áp dụng"),no=el("button","pill soft","Bỏ");const br=el("div","mem-bar");br.append(no,ok);w.append(ul,br);bar.after(w);no.onclick=()=>w.remove();ok.onclick=()=>api(`/agents/${id}/memory/summarize`,{apply:L}).then(()=>{toast("Đã gộp bộ nhớ");openMemory(id,true)}).catch(()=>toast("Không áp dụng được"))}).catch(()=>{sum.disabled=false;sum.textContent="Gộp trùng lặp";toast("Không gộp được")})}};
// images: downscale on the phone, upload into the bot's computer, describe via the vision model
function shrink(file){return new Promise((ok,bad)=>{const img=new Image();img.onload=()=>{const k=Math.min(1,1600/Math.max(img.width,img.height));const c=document.createElement("canvas");c.width=img.width*k|0;c.height=img.height*k|0;c.getContext("2d").drawImage(img,0,0,c.width,c.height);c.toBlob(b=>b?ok(b):bad(),"image/jpeg",.82)};img.onerror=bad;const fr=new FileReader();fr.onload=()=>{img.src=fr.result};fr.onerror=bad;fr.readAsDataURL(file)})} // data: URL, the CSP allows no blob: images
$("#pmImg").onclick=()=>{closeMenu();if(!cur||isGroup(cur)){toast("Gửi ảnh trong chat với một bot");return}$("#imgPick").click()};
$("#imgPick").onchange=async e=>{const f=e.target.files&&e.target.files[0];e.target.value="";if(!f||!cur)return;try{const b=await shrink(f);const nm=(f.name||"anh").replace(/\.[^.]+$/,"").normalize("NFC").slice(0,50)+".jpg";const data=await new Promise((ok,bad)=>{const fr=new FileReader();fr.onload=()=>ok(String(fr.result).split(",")[1]);fr.onerror=bad;fr.readAsDataURL(b)});const j=await api(`/agents/${cur.id}/computer/upload`,{name:nm,data});const inp=$("#input");inp.value=(inp.value.trim()?inp.value.trim()+"\n":"Bạn xem ảnh này giúp mình.\n")+"[Ảnh: "+j.path+"]";inp.dispatchEvent(new Event("input"));inp.focus();toast("Đã tải ảnh lên")}catch{toast("Không tải được ảnh (tối đa 1 MB sau khi nén)")}};
// ---------- Vault (cards, logins, personal details) ----------
// Values live only in the encrypted vault on the server. The list shows
// metadata; a value is fetched (and logged) only when the owner taps to reveal.
const VF={card:[["number","Số thẻ",1,"numeric"],["name","Tên trên thẻ"],["exp","Hết hạn (MM/YY)",0,"numeric"],["cvc","CVC",1,"numeric"],["zip","Mã bưu chính"]],
 login:[["site","Trang web"],["username","Tên đăng nhập / email"],["password","Mật khẩu",1],["totp","Khoá 2 lớp (TOTP, tuỳ chọn)",1],["notes","Ghi chú"]],
 identity:[["name","Họ tên"],["email","Email",0,"email"],["phone","Điện thoại",0,"tel"],["address","Địa chỉ"],["city","Thành phố"],["postal","Mã bưu chính"],["country","Quốc gia"]]};
function vfield(label,attrs){const w=el("label","fld");w.append(el("span",0,label));const i=el("input");Object.assign(i,attrs);i.setAttribute("aria-label",label);w.append(i);return{w,i}}
const VT={card:["credit-card","Thẻ thanh toán","Thẻ"],login:["lock","Đăng nhập","Đăng nhập"],identity:["user-round","Thông tin cá nhân","Thông tin"]};
const VA={fill:"Điền form",env:"Dùng trong lệnh",reveal:"Bạn xem",save:"Đã lưu",delete:"Đã xoá"};
let vaultData=null,vaultOpen=null,vaultEdit=null,vaultShown={};
const vSub=it=>it.type==="card"?(it.hint.brand||"Thẻ")+" •••• "+(it.hint.last4||""):it.type==="login"?[it.hint.site,it.hint.user].filter(Boolean).join(" · ")+(it.hint.totp?" · 2 lớp":""):(it.hint.name||"")+" · "+(it.filled||[]).length+" mục";
function vaultForm(type,item,onSaved,work){const f=el("form","vault-form");const ins={};
 (VF[type]||[]).forEach(([k,label,sec,mode])=>{let w,i;if(sec){const sf=secretField(k,label,item?"Đã lưu · để trống nếu giữ nguyên":label);w=el("label","fld");w.append(el("span",0,label),sf.w);i=sf.i}else{const x=vfield(label,{autocomplete:"off",autocapitalize:"off",spellcheck:false});w=x.w;i=x.i;i.dataset.secret=k;if(item)i.placeholder="Đã lưu · để trống nếu giữ nguyên";if(k==="site"&&work&&work.site)i.value=work.site}
  if(mode)i.inputMode=mode;if(k==="number"){i.maxLength=23;i.oninput=()=>{const d=i.value.replace(/\D/g,"").slice(0,19);i.value=d.replace(/(.{4})/g,"$1 ").trim()}}if(k==="exp"){i.maxLength=5;i.oninput=()=>{const d=i.value.replace(/\D/g,"").slice(0,4);i.value=d.length>2?d.slice(0,2)+"/"+d.slice(2):d}}
  i.setAttribute("data-1p-ignore","");i.setAttribute("data-lpignore","true");ins[k]=i;f.append(w)});
 const lb=vfield("Tên gợi nhớ (tuỳ chọn)",{autocomplete:"off",value:item?item.label:(work&&work.label)||""});lb.i.id="vaultLabel";f.prepend(lb.w);
 const err=el("p","sec-err");err.setAttribute("role","alert");const go=el("button","pill wide",item?"Lưu thay đổi":"Lưu vào Vault");go.type="submit";go.id="vaultSave";
 f.onsubmit=async e=>{e.preventDefault();err.textContent="";const fields={};Object.entries(ins).forEach(([k,i])=>{if(i.value.trim())fields[k]=i.value.trim()});if(!item&&!Object.keys(fields).length){err.textContent="Chưa nhập gì.";return}
  go.disabled=true;go.textContent="Đang lưu…";try{const it=await api("/vault",{id:item?item.id:"",type,label:lb.i.value.trim(),fields,work:work&&work.task||""});Object.values(ins).forEach(i=>i.value="");buzz("confirm");toast("Đã lưu vào Vault");onSaved&&onSaved(it)}
  catch(e){err.textContent=e.status===400?(type==="card"?"Số thẻ không hợp lệ.":type==="login"?"Cần mật khẩu; khoá 2 lớp phải là mã base32.":"Kiểm tra lại thông tin."):"Không lưu được, thử lại.";go.disabled=false;go.textContent=item?"Lưu thay đổi":"Lưu vào Vault"}};
 f.append(err,go,el("small","hint","Mã hoá trên máy chủ theo tài khoản của bạn. Bot chỉ thấy nhãn và 4 số cuối, không bao giờ thấy giá trị."));return f}
async function openVault(keep){const B=$("#vaultBody");if(!keep){B.innerHTML="";B.append(el("p","caption","Đang tải…"));if(sheetOpen)swapSheet($("#vaultSheet"));else openSheet($("#vaultSheet"))}
 try{vaultData=await api("/vault")}catch{B.innerHTML="";B.append(el("p","caption","Không tải được Vault"));return}renderVault()}
function renderVault(){const B=$("#vaultBody"),d=vaultData||{items:[],uses:[]};B.innerHTML="";
 const hero=el("div","vault-hero");hero.append(I("shield-check",22),el("div",0));hero.lastChild.append(el("b",0,"Vault của bạn"),el("small",0,"Thẻ, đăng nhập và thông tin cá nhân, mã hoá trên máy chủ. Mỗi lần bot dùng đều hỏi bạn duyệt."));B.append(hero);
 if(vaultEdit&&vaultEdit.type){const it=vaultEdit.id?d.items.find(x=>x.id===vaultEdit.id):null;const h=el("div","vault-fhd");const back=el("button","text-btn","Huỷ");back.onclick=()=>{vaultEdit=null;renderVault()};h.append(el("b",0,(it?"Sửa ":"Thêm ")+VT[vaultEdit.type][2].toLowerCase()),back);B.append(h,vaultForm(vaultEdit.type,it,()=>{vaultEdit=null;openVault(true)}));return}
 const add=el("div","vault-add");Object.keys(VT).forEach(t=>{const b=el("button","pill ghost");b.dataset.vadd=t;b.append(I("plus",16),el("span",0,VT[t][2]));b.onclick=()=>{vaultEdit={type:t};renderVault()};add.append(b)});B.append(add);
 if(!d.items.length)B.append(el("p","caption","Chưa có mục nào. Thêm ở trên, hoặc khi bot cần, nó sẽ gửi thẻ để bạn nhập ngay trong chat."));
 Object.keys(VT).forEach(t=>{const list=d.items.filter(x=>x.type===t);if(!list.length)return;const g=el("div","vault-group");g.append(el("h3",0,VT[t][1]));
  list.forEach(it=>{const row=el("div","vault-it"+(vaultOpen===it.id?" open":""));row.dataset.id=it.id;const hd=el("button","vault-row");const ic=el("span","cell-ic");ic.append(I(VT[t][0],18));const tx=el("span","cell-t");tx.append(el("span",0,it.label),el("small",0,vSub(it)));hd.append(ic,tx,I(vaultOpen===it.id?"chevron-down":"chevron-right",16));
   hd.onclick=()=>{vaultOpen=vaultOpen===it.id?null:it.id;renderVault()};row.append(hd);if(vaultOpen===it.id)row.append(vaultDetail(it));g.append(row)});B.append(g)});
 if(d.uses&&d.uses.length){const det=el("details","mem-log vault-log");det.append(el("summary",0,"Lịch sử dùng ("+d.uses.length+")"));const ol=el("ol");d.uses.forEach(u=>{const li=el("li");li.append(el("b",0,VA[u.action]||u.action),el("span",0," "+u.label+(u.target?" · "+u.target:"")),el("small",0," — "+(u.bot?(byId(u.bot)?.name||"bot")+", ":"")+ago(u.at)));if(u.result)li.append(el("small","vault-res",u.result));ol.append(li)});det.append(ol);B.append(det)}}
function vaultDetail(it){const box=el("div","vault-det");const sh=vaultShown[it.id];const VFm=Object.fromEntries((VF[it.type]||[]).map(x=>[x[0],x[1]]));
 (it.filled||[]).forEach(k=>{const r=el("div","vault-f");r.append(el("span",0,VFm[k]||k));const v=sh?sh.fields[k]:"";const val=el("code",sh?"shown":0,sh?(k==="number"?v.replace(/(.{4})/g,"$1 ").trim():v):"••••••••");val.dataset.noi18n="";r.append(val);
  if(sh){const cp=el("button","icon-btn sm");cp.setAttribute("aria-label","Sao chép");cp.append(I("copy",15));cp.onclick=()=>{navigator.clipboard?.writeText(v).then(()=>toast("Đã sao chép")).catch(()=>{})};r.append(cp)}box.append(r)});
 if(sh&&sh.code){const r=el("div","vault-f");r.append(el("span",0,"Mã 2 lớp hiện tại"),el("code","shown",sh.code));box.append(r)}
 const bar=el("div","mem-bar");const eye=el("button","pill ghost");eye.dataset.vreveal=it.id;eye.append(I(sh?"eye-off":"eye",16),el("span",0,sh?"Ẩn":"Hiện giá trị"));
 eye.onclick=async()=>{if(vaultShown[it.id]){delete vaultShown[it.id];renderVault();return}try{const r=await api(`/vault/${it.id}/reveal`,{});vaultShown[it.id]=r;renderVault();setTimeout(()=>{if(vaultShown[it.id]===r){delete vaultShown[it.id];if(sheetOpen===$("#vaultSheet"))renderVault()}},30000)}catch{toast("Không mở được")}};
 const ed=el("button","pill ghost");ed.append(I("pencil",16),el("span",0,"Sửa"));ed.onclick=()=>{delete vaultShown[it.id];vaultEdit={type:it.type,id:it.id};renderVault()};
 const del=el("button","pill ghost danger");del.append(I("trash-2",16),el("span",0,"Xoá"));del.onclick=async()=>{if(!confirm("Xoá “"+it.label+"” khỏi Vault?"))return;try{await api(`/vault/${it.id}/delete`,{});delete vaultShown[it.id];vaultOpen=null;toast("Đã xoá");openVault(true)}catch{toast("Không xoá được")}};
 bar.append(eye,ed,del);box.append(bar);if(sh)box.append(el("small","hint","Tự ẩn sau 30 giây. Lần xem này được ghi vào lịch sử."));return box}
$("#closeVault").onclick=()=>{vaultShown={};vaultEdit=null;closeSheet(true)};
// in-chat Vault form card: the bot asked to save a card/login/details; values go straight to the vault
function vaultCard(m){const x=m.secret,t=x.vault,key="vault-saved:"+m.id,saved=localStorage.getItem(key),c=el("div","secret vault-card"+(saved?" saved":""));c.dataset.id=m.id;c.dataset.name=x.name;
 const top=el("div","sec-top");top.append(I(VT[t]?VT[t][0]:"lock",18),el("b",0,x.label||VT[t][1]),el("code",0,"Vault"));c.append(top);if(x.desc)c.append(el("p","sec-desc",x.desc));
 if(saved){const d=el("div","sec-done");d.append(I("check",16),el("span",0,"Đã lưu vào Vault · "+saved));c.append(d);return c}
 c.append(vaultForm(t,null,it=>{localStorage.setItem(key,vSub(it)||it.label);nodeCache.delete(m.id);renderThread()},{task:m.taskId,label:x.label,site:x.site}));return c}
{const _rs3=renderSettings;renderSettings=function(){_rs3();const B=$("#settingsBody");const c=card("Bảo mật",cell("shield-check","Vault",{id:"openVault",sub:"Thẻ, đăng nhập, thông tin cá nhân · mã hoá",onClick:()=>openVault()}));const u=$("#openUsage");const g=u&&u.closest(".card-group");g?g.after(c):B.append(c)}}
window.__AURORA=Object.assign(window.__AURORA||{},{openVault});
})();
