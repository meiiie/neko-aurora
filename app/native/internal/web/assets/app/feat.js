"use strict";
// Neko Aurora: memory panel, permission presets, usage page, chat search.
// Ideas from DS Bot (Apache-2.0, github.com/Fei2411/DeepSeek-Bot); no code copied.
(()=>{
const esc=s=>String(s??"");
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
window.openMemory=async function(id,keep){memBot=id;const a=byId(id);const B=$("#memBody");if(!keep){B.innerHTML="";B.append(el("p","caption","Đang tải…"));if(sheetOpen)swapSheet($("#memSheet"));else openSheet($("#memSheet"))}
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
 PRESETS.forEach(([k,n])=>{const b=el("button","seg-b",n);b.dataset.preset=k;b.setAttribute("role","radio");b.setAttribute("aria-checked",k===cur);b.onclick=()=>api(`/agents/${id}/computer/policy`,{preset:k,askFirst:k==="readonly"?true:!!p.askFirst}).then(np=>{buzz("tap");toast("Quyền: "+n);presetCard(id,box)}).catch(()=>toast("Không lưu được"));seg.append(b)});
 w.append(seg,el("p","caption",PRESETS.find(x=>x[0]===cur)[2]));const old=box.querySelector("#presetBox");old?old.replaceWith(w):box.prepend(w)}).catch(()=>{})}
// ---------- profile hooks ----------
const _rp=renderProfile;window.renderProfile=renderProfile=function(){_rp();const a=byId(profId);if(!a||isGroup(a))return;const B=$("#profBody");
 const c=card("Ghi nhớ",cell("sparkles","Bộ nhớ",{id:"prMem",value:"",sub:"Xem, sửa, xoá điều bot nhớ về bạn",onClick:()=>openMemory(a.id)}));const pc=$("#pcBox");pc?B.insertBefore(c,pc):B.append(c);
 api(`/agents/${a.id}/memory`).then(m=>{const v=c.querySelector(".cell-v")||el("span","cell-v");v.textContent=((m.items||[]).length+(m.team||[]).length)+" mục";const ch=c.querySelector(".chev");ch?ch.before(v):c.querySelector(".cell").append(v)}).catch(()=>{});
 if(pc)presetCard(a.id,pc)};
// ---------- usage page ----------
let usageRange="7";
const fmtTok=n=>n>=1e6?(n/1e6).toFixed(1)+"M":n>=1e3?(n/1e3).toFixed(1)+"K":String(n);
const KIND={chat:"Trò chuyện",checkpoint:"Tóm tắt ngữ cảnh",memory:"Ghi nhớ",budget:"Quyết định khi hết giờ",factcheck:"Kiểm tra số liệu",review:"Rà soát file",rewrite:"Sửa tiếng Việt",brief:"Bản tin"};
window.openUsage=async function(keep){const B=$("#usageBody");if(!keep){B.innerHTML="";swapSheet($("#usageSheet"))}
 let u;try{u=await api("/usage?range="+usageRange)}catch{B.innerHTML="";B.append(el("p","caption","Không tải được"));return}
 B.innerHTML="";const seg=el("div","seg");[["1","24 giờ"],["7","7 ngày"],["30","30 ngày"],["365","12 tháng"]].forEach(([k,n])=>{const b=el("button","seg-b",n);b.setAttribute("aria-checked",k===usageRange);b.onclick=()=>{usageRange=k;openUsage(true)};seg.append(b)});
 const hero=el("div","use-hero");hero.append(el("b",0,"≈ "+fmtTok(u.total.tokens)+" token"),el("small",0,u.total.calls+" lượt gọi mô hình · "+(S.ai.model||"")));B.append(seg,hero);
 const days=u.days||[],mx=Math.max(1,...days.map(d=>d.tokens));if(days.length>1){const ch=el("div","use-bars");days.slice(-31).forEach(d=>{const c=el("i");c.style.height=Math.max(3,Math.round(d.tokens/mx*72))+"px";c.title=d.key+": "+fmtTok(d.tokens);ch.append(c)});B.append(ch)}
 B.append(card("Theo bot",...(u.bots||[]).map(r=>cell("users",u.names[r.key]||r.key,{static:1,value:fmtTok(r.tokens),sub:r.calls+" lượt"}))));
 B.append(card("Theo loại",...(u.kinds||[]).map(r=>cell("activity",KIND[r.key]||r.key,{static:1,value:fmtTok(r.tokens),sub:r.calls+" lượt"}))));
 B.append(el("p","caption","Số token là ước tính từ độ dài văn bản gửi và nhận."))};
$("#closeUsage").onclick=()=>closeSheet(true);
const _rs=renderSettings;window.renderSettings=renderSettings=function(){_rs();const B=$("#settingsBody");const c=card("Sử dụng",cell("activity","Mức dùng AI",{id:"openUsage",sub:"Token theo ngày, theo bot và theo loại",onClick:()=>openUsage()}));const last=B.querySelectorAll(".card-group");last.length>1?B.insertBefore(c,last[last.length-1]):B.append(c)};
// ---------- chat search ----------
let hits=[],hitQ="",tmr=0;
const _rl=renderList;window.renderList=renderList=function(){_rl();const q=filter.trim();if(q.length<2||hitQ!==q||!hits.length)return;const ul=$("#rows");const h=el("li","sec-h");h.setAttribute("role","heading");h.append(el("span",0,"Tin nhắn"));ul.append(h);
 hits.forEach(x=>{const li=el("li","hit-row");const b=el("button","hit-b");const a=byId(x.agentId);const av=el("span","row-av");if(a)av.append(avatarOf(a,36));const t=el("span","hit-t");t.append(el("b",0,x.name+(x.role==="user"?" · bạn":"")),el("span",0,x.snippet));b.append(av,t);b.onclick=()=>{openThread(x.agentId);setTimeout(()=>{const f=$("#findInput");if(f){$("#findBar").classList.remove("hidden");f.value=q;runFind()}},700)};li.append(b);ul.append(li)})};
$("#search").addEventListener("input",()=>{clearTimeout(tmr);const q=filter.trim();if(q.length<2){hits=[];hitQ="";return}tmr=setTimeout(()=>api("/search?q="+encodeURIComponent(q)).then(r=>{if(filter.trim()!==q)return;hits=r.hits||[];hitQ=q;renderList()}).catch(()=>{}),250)});
})();
