"use strict";
(()=>{
const $=s=>document.querySelector(s),dialog=$("#networkDialog"),entry=$("#networkBtn");
if(!dialog||!entry)return;
const node=(tag,cls,value)=>{const n=document.createElement(tag);if(cls)n.className=cls;if(value!=null)n.textContent=value;return n};
const raw=(tag,cls,value)=>{const n=node(tag,cls,value);n.dataset.noi18n="";return n};
const bytes=s=>new TextEncoder().encode(s).length;
const size=n=>n<1024?`${n} B`:`${Number((n/1024).toFixed(1))} KiB`;
const nonce=()=>"network-"+(crypto.randomUUID?crypto.randomUUID():Array.from(crypto.getRandomValues(new Uint8Array(16)),n=>n.toString(16).padStart(2,"0")).join(""));
const state={sessions:[],messages:[],limits:null};
const directoryNodes=new Map(),messageNodes=new Map(),busy=new Set(),actionNonces=new Map();
let selected="",files=[],lastFocus=null,refreshing=false,sending=false,readingFiles=false,pendingSend=null,selectSignature="",timer=null;
const byId=id=>state.sessions.find(s=>s.id===id);
const sessionName=id=>{const s=byId(id);return s?s.name||s.alias||s.id:id||"Chủ sở hữu"};
const sessionScope=s=>s.conversationId==="conversation-"+s.botId?"Riêng":"Nhóm · "+String(s.conversationId||"").slice(-8);
const selectedSession=()=>byId(selected);
const selectedMessages=()=>state.messages.filter(m=>m.toSessionId===selected||m.fromSessionId===selected);
const wakeMode=()=>$("input[name='networkMode']:checked").value==="wake";
const expired=m=>m.expiresAt&&Date.parse(m.expiresAt)<=Date.now();
const stamp=t=>{const d=new Date(t);return Number.isNaN(d.getTime())?"":d.toLocaleString("vi-VN",{day:"numeric",month:"numeric",hour:"2-digit",minute:"2-digit"})};
const fullStamp=t=>{const d=new Date(t);return Number.isNaN(d.getTime())?"":d.toLocaleString("vi-VN")};
const deliveryNames={queued:"Đang chờ giao",delivered:"Đã giao vào lượt agent",held:"Đang giữ yêu cầu",blocked:"Bị chặn bởi phạm vi phiên",expired:"Hết hạn",failed:"Giao thất bại"};
const taskNames={none:"Chưa có tác vụ",queued:"Đang chờ chạy",accepted:"Đã nhận tác vụ",pending:"Đang chờ chạy",running:"Đang chạy",working:"Đang chạy",waiting:"Chờ bạn duyệt",waiting_host:"Chờ máy thực thi",admission_blocked:"Chưa được phép bắt đầu",done:"Đã hoàn tất",completed:"Đã hoàn tất",failed:"Thất bại",cancelled:"Đã huỷ",stopped:"Đã dừng"};
const sessionStates={idle:"Sẵn sàng",busy:"Đang làm việc",working:"Đang làm việc",running:"Đang chạy",waiting:"Chờ duyệt",error:"Có lỗi",offline:"Ngoại tuyến",active:"Đang hoạt động",closed:"Đã đóng"};
const tierNames={pull:"Nhận ở lượt sau",turn:"Có thể yêu cầu đánh thức"};
const failureNames={network_quota_exceeded:"Đã đạt hạn mức mô hình. Yêu cầu đang chờ, chưa bắt đầu lượt mô hình.",network_forbidden:"Quyền hiện tại chưa cho phép bắt đầu lượt agent.",network_identity_required:"Cần phiên chủ sở hữu hợp lệ để bắt đầu lượt agent."};
const notice=(text,bad=false)=>{const n=$("#networkNotice");n.textContent=text;n.classList.toggle("bad",bad)};
const formError=text=>{const n=$("#networkFormError");n.textContent=text;n.classList.toggle("hidden",!text)};
async function request(path,body){
 const get=body===undefined,options={method:get?"GET":"POST",credentials:"same-origin",cache:"no-store"};
 if(!get){options.headers={"Content-Type":"application/json"};options.body=JSON.stringify(body)}
 if(typeof AbortSignal!=="undefined"&&AbortSignal.timeout)options.signal=AbortSignal.timeout(get?15000:30000);
 const r=await fetch("/app/api"+path,options);
 if(!r.ok){const e=new Error("HTTP "+r.status);e.status=r.status;throw e}
 return r.json();
}
function errorText(e,action){
 if(e.status===401)return"Phiên chủ sở hữu đã hết hạn. Đóng hộp thư và đăng nhập lại.";
 if(e.status===403)return"Bạn không có quyền thực hiện thao tác này trong phiên hiện tại.";
 if(e.status===404)return"Không tìm thấy phiên hoặc tin nhắn. Làm mới danh bạ để kiểm tra.";
 if(e.status===409)return"Trạng thái đã thay đổi hoặc lần gửi này không còn khớp. Làm mới rồi kiểm tra hộp thư.";
 if(e.status===413)return"Nội dung hoặc file vượt giới hạn máy chủ. Giảm dung lượng rồi gửi lại.";
 if(e.status===429)return"Đã đạt giới hạn gửi. Đợi một chút rồi thử lại.";
 if(e.status===400)return"Máy chủ từ chối nội dung. Kiểm tra tên file, dung lượng và phiên nhận.";
 return action+" Kiểm tra kết nối rồi thử lại.";
}
function readLimits(value){
 const limits={};
 for(const key of ["maxFiles","maxFileBytes","maxTotalBytes","maxTextBytes"]){if(!value||!Number.isSafeInteger(value[key])||value[key]<1)throw new Error("Invalid mailbox limits");limits[key]=value[key]}
 return limits;
}
async function refresh(manual=false){
 if(refreshing)return;
 refreshing=true;$("#networkRefresh").disabled=true;
 if(manual)notice("Đang làm mới danh bạ và hộp thư…");
 try{
  const data=await request("/network");
  const limits=readLimits(data.limits);
  if(!Array.isArray(data.sessions)||!Array.isArray(data.messages))throw new Error("Invalid mailbox response");
  const firstLoad=!state.limits;
  state.sessions=data.sessions;state.messages=data.messages;state.limits=limits;
  if(!selectedSession())selected=firstLoad?state.sessions[0]?.id||"":"";
  renderDirectory();renderMailbox();syncComposer();
  if(manual||$("#networkNotice").classList.contains("bad"))notice("Đã cập nhật · hộp thư tự làm mới khi đang mở.");
 }catch(e){notice(errorText(e,"Không tải được giao tiếp agent."),true)}
 finally{refreshing=false;$("#networkRefresh").disabled=false}
}
function putInOrder(parent,children){
 children.forEach((child,i)=>{if(parent.children[i]!==child)parent.insertBefore(child,parent.children[i]||null)});
 const keep=new Set(children);[...parent.children].forEach(child=>{if(!keep.has(child))child.remove()});
}
function renderDirectory(){
 const q=$("#networkSearch").value.trim().toLocaleLowerCase(),matches=state.sessions.filter(s=>!q||[s.name,s.alias,s.id,s.botId,s.deviceId].some(v=>String(v||"").toLocaleLowerCase().includes(q)));
 const list=$("#networkSessions"),children=[];
 for(const s of matches){
  let li=directoryNodes.get(s.id);
  if(!li){
   li=node("li");const b=raw("button","network-session");b.type="button";b.dataset.session=s.id;
   const copy=node("span","network-session-copy");copy.append(raw("span","network-session-name"),raw("span","network-session-meta"));
   b.append(raw("span","network-session-face"),copy,node("span","network-session-unread"));
   b.onclick=()=>chooseSession(s.id);li.append(b);directoryNodes.set(s.id,li);
  }
  const b=li.firstChild,name=s.name||s.alias||s.id,unread=Math.max(0,Number(s.unread)||0);
  b.classList.toggle("selected",s.id===selected);b.setAttribute("aria-pressed",String(s.id===selected));
  b.querySelector(".network-session-name").textContent=name;
  b.querySelector(".network-session-face").textContent=Array.from(name).slice(0,2).join("").toLocaleUpperCase();
  b.querySelector(".network-session-meta").textContent=[s.alias?"@"+s.alias:"",sessionScope(s),sessionStates[s.status]||s.status,tierNames[s.tier]||s.tier].filter(Boolean).join(" · ");
  const badge=b.querySelector(".network-session-unread");badge.textContent=unread>99?"99+":String(unread);badge.hidden=!unread;
  b.setAttribute("aria-label",name+(unread?`, ${unread} tin chưa xem`:""));children.push(li);
 }
 putInOrder(list,children);
 for(const [id] of directoryNodes)if(!state.sessions.some(s=>s.id===id))directoryNodes.delete(id);
 $("#networkDirectoryCount").textContent=q?`${matches.length} / ${state.sessions.length} phiên`:`${state.sessions.length} phiên agent`;
 $("#networkNoSessions").classList.toggle("hidden",matches.length>0);
 $("#networkNoSessions").textContent=q?"Không có phiên phù hợp.":"Chưa có phiên agent trong phạm vi hiện tại.";
 const signature=JSON.stringify(state.sessions.map(s=>[s.id,s.name,s.alias,s.conversationId]));
 if(signature!==selectSignature){
  const select=$("#networkTarget"),options=[node("option",null,"Chọn phiên agent")];options[0].value="";
  state.sessions.forEach(s=>{const o=raw("option",null,(s.name||s.id)+(s.alias?" · @"+s.alias:"")+" · "+sessionScope(s));o.value=s.id;options.push(o)});
  select.replaceChildren(...options);selectSignature=signature;
 }
 $("#networkTarget").value=selected;
 const unread=state.sessions.reduce((sum,s)=>sum+Math.max(0,Number(s.unread)||0),0),badge=$("#networkUnread");
 badge.textContent=unread>99?"99+":String(unread);badge.classList.toggle("hidden",!unread);
 entry.setAttribute("aria-label","Giao tiếp agent"+(unread?`, ${unread} tin chưa xem`:""));
}
function chooseSession(id){
 if(sending)return;
 selected=id;pendingSend=null;formError("");renderDirectory();renderMailbox();syncComposer();
}
function statusTag(label,kind){return node("span","network-status"+(kind?" "+kind:""),label)}
function messageCard(m){
 const li=node("li","network-message"+(m.origin!=="human"?" peer":""));li.dataset.message=m.id;
 const top=node("div","network-message-top"),human=m.origin==="human",sender=human?"Bạn · chủ sở hữu":(m.fromName||sessionName(m.fromSessionId))+" · agent";
 const time=node("time",null,stamp(m.createdAt));if(m.createdAt)time.dateTime=m.createdAt;time.title=fullStamp(m.createdAt);top.append(raw("b",null,sender),time);
 li.append(top,raw("p","network-message-route","Tới "+sessionName(m.toSessionId)),raw("p","network-message-body",m.text||""));
 if(m.attachments?.length){
  const attachments=node("ul","network-message-files");
  m.attachments.forEach(f=>{const item=node("li"),a=raw("a",null,`${f.name} · ${size(f.bytes||0)} ↓`);a.href="/app/api/network/messages/"+encodeURIComponent(m.id)+"/files/"+encodeURIComponent(f.id);a.download=f.name;a.setAttribute("aria-label","Tải file "+f.name);item.append(a);attachments.append(item)});li.append(attachments);
 }
 const statuses=node("div","network-statuses");
 statuses.append(statusTag(m.wake?"Yêu cầu đánh thức":"FYI · không đánh thức"));
 const delivery=expired(m)&&m.deliveryState==="queued"?"expired":m.deliveryState;
 statuses.append(statusTag("Giao tin: "+(deliveryNames[delivery]||delivery||"Chưa rõ"),delivery==="failed"||delivery==="expired"||delivery==="blocked"?"bad":delivery==="held"?"warn":""));
 statuses.append(statusTag("Tác vụ: "+(taskNames[m.taskState]||m.taskState||"Chưa có tác vụ"),m.taskState==="failed"?"bad":m.taskState==="waiting"||m.taskState==="waiting_host"||m.taskState==="admission_blocked"?"warn":""));
 if(m.readAt){const tag=statusTag("Đã xem · "+stamp(m.readAt));tag.title="Chủ sở hữu đã xem · "+fullStamp(m.readAt);statuses.append(tag)}
 if(m.expiresAt){const tag=statusTag("Hết hạn · "+stamp(m.expiresAt));tag.title=fullStamp(m.expiresAt);statuses.append(tag)}
 li.append(statuses);
 if(m.failure)li.append(raw("p","network-form-error",failureNames[m.failure]||"Máy chủ đang giữ yêu cầu: "+m.failure));
 if(m.approval){
  const ap=m.approval,details=node("details","network-approval"),summary=raw("summary",null,(ap.state==="pending"?"Chờ chủ sở hữu duyệt: ":ap.state==="approved"?"Đã duyệt: ":ap.state==="declined"?"Đã hoãn: ":"Yêu cầu duyệt: ")+(ap.title||"Tác vụ"));
  details.append(summary,raw("pre",null,ap.body||""));
  if(ap.decidedAt)details.append(node("p","network-inline-hint","Quyết định · "+stamp(ap.decidedAt)));
  li.append(details);
 }
 const actions=node("div","network-message-actions");
 if(!m.readAt){const ack=node("button",null,"Đánh dấu đã xem");ack.type="button";ack.onclick=()=>messageAction(m,"ack");actions.append(ack)}
 if(human&&!m.wake&&m.deliveryState==="queued"&&!expired(m)){
  const wake=node("button","network-wake-btn","Đánh thức từ tin này");wake.type="button";wake.onclick=()=>messageAction(m,"wake");actions.append(wake);
 }
 if(m.approval?.state==="pending"&&m.approval.workId){
  const s=byId(m.toSessionId);
  if(s?.botId){const task=node("button",null,"Xem tác vụ & duyệt");task.type="button";task.onclick=()=>openTask(s,m.approval.workId);actions.append(task)}
 }
 actions.querySelectorAll("button").forEach(b=>b.disabled=busy.has(m.id));
 li.append(actions);
 if(human&&!m.wake&&m.deliveryState==="queued"&&!expired(m))li.append(node("p","network-inline-hint","Đánh thức sẽ tạo yêu cầu mới từ bạn, có thể bắt đầu lượt mô hình và phát sinh chi phí. Tin FYI gốc được giữ nguyên."));
 return li;
}
function renderMailbox(){
 const s=selectedSession(),list=$("#networkMessages"),messages=s?selectedMessages():[],nearBottom=list.scrollHeight-list.scrollTop-list.clientHeight<80,oldTop=list.scrollTop;
 $("#networkMailboxTitle").textContent=s?s.name||s.alias||s.id:"Hộp thư";
 $("#networkSessionMeta").textContent=s?[s.alias?"@"+s.alias:"",s.conversationId,s.deviceId?"Thiết bị: "+s.deviceId:"",tierNames[s.tier]||s.tier].filter(Boolean).join(" · "):"";
 $("#networkMailCount").textContent=s?`${messages.length} tin`:"";
 const children=messages.map(m=>{
  const sig=JSON.stringify(m)+"|"+busy.has(m.id)+"|"+expired(m)+"|"+sessionName(m.toSessionId)+"|"+sessionName(m.fromSessionId),old=messageNodes.get(m.id);
  // A status refresh leaves a focused action and an expanded approval intact.
  if(old&&(old.sig===sig||old.node.contains(document.activeElement)))return old.node;
  const card=messageCard(m);if(old?.node.querySelector("details[open]"))card.querySelector("details")?.setAttribute("open","");
  messageNodes.set(m.id,{sig,node:card});return card;
 });
 putInOrder(list,children);
 for(const [id] of messageNodes)if(!state.messages.some(m=>m.id===id))messageNodes.delete(id);
 $("#networkNoMessages").classList.toggle("hidden",messages.length>0);
 $("#networkNoMessages").textContent=s?"Chưa có tin nhắn. Gửi FYI để chia sẻ thông tin mà không đánh thức agent.":"Chọn một phiên để xem hộp thư.";
 if(nearBottom)list.scrollTop=list.scrollHeight;else list.scrollTop=oldTop;
}
function actionNonce(id,action){const key=id+":"+action;if(!actionNonces.has(key))actionNonces.set(key,nonce());return actionNonces.get(key)}
async function messageAction(m,action){
 if(busy.has(m.id))return;
 busy.add(m.id);renderMailbox();notice(action==="wake"?"Đang gửi yêu cầu đánh thức…":"Đang ghi nhận đã xem…");
 try{
  const result=await request("/network/messages/"+encodeURIComponent(m.id)+"/"+action,{clientNonce:actionNonce(m.id,action)});
  if(result.message){const i=state.messages.findIndex(x=>x.id===result.message.id);if(i<0)state.messages.push(result.message);else state.messages[i]=result.message}
  notice(action==="wake"?"Đã ghi nhận yêu cầu đánh thức. Trạng thái tác vụ hiển thị theo máy chủ.":"Đã đánh dấu chủ sở hữu đã xem. Trạng thái giao tin và tác vụ được giữ riêng.");
  await refresh();
 }catch(e){notice(errorText(e,action==="wake"?"Chưa xác nhận được yêu cầu đánh thức.":"Chưa ghi nhận được đã xem."),true)}
 finally{busy.delete(m.id);renderMailbox()}
}
function openTask(session,workId){
 close();document.dispatchEvent(new CustomEvent("aurora-network-open-task",{detail:{botId:session.botId,conversationId:session.conversationId,workId}}));
}
function validationError(){
 const l=state.limits,text=$("#networkText").value;
 if(!l)return"Chờ tải giới hạn từ máy chủ trước khi gửi.";
 if(!selectedSession())return"Chọn một phiên nhận trong danh bạ.";
 if(!text.trim())return"Nhập nội dung để agent hiểu thông tin hoặc yêu cầu của bạn.";
 if(/[\x00-\x08\x0b\x0c\x0e-\x1f\x7f-\x9f]/.test(text))return"Nội dung chứa ký tự điều khiển không được hỗ trợ.";
 if(bytes(text)>l.maxTextBytes)return"Nội dung vượt "+size(l.maxTextBytes)+". Hãy chia nhỏ tin nhắn.";
 if(files.length>l.maxFiles)return"Chỉ gửi tối đa "+l.maxFiles+" file mỗi tin.";
 if(files.some(f=>bytes(f.content)>l.maxFileBytes))return"Một file vượt giới hạn "+size(l.maxFileBytes)+".";
 if(files.reduce((sum,f)=>sum+bytes(f.content),0)>l.maxTotalBytes)return"Tổng file vượt "+size(l.maxTotalBytes)+".";
 return"";
}
function syncComposer(){
 const l=state.limits,count=bytes($("#networkText").value),wake=wakeMode(),used=files.reduce((sum,f)=>sum+bytes(f.content),0);
 $("#networkTextCount").textContent=l?`${size(count)} / ${size(l.maxTextBytes)}`:`${size(count)} · chờ giới hạn`;
 $("#networkTextCount").classList.toggle("over",!!l&&count>l.maxTextBytes);
 $("#networkFileLimits").textContent=l?`${files.length}/${l.maxFiles} file · ${size(used)}/${size(l.maxTotalBytes)} · ${size(l.maxFileBytes)}/file`:"";
 $("#networkWakeHint").textContent=wake?"Đánh thức có thể bắt đầu một lượt mô hình và phát sinh chi phí. "+(selectedSession()?.tier==="pull"?"Phiên chưa có runtime đánh thức; yêu cầu sẽ chờ máy chủ.":"Hãy nêu rõ việc cần làm."):"FYI lưu vào hộp thư, không bắt đầu lượt mô hình. Agent có thể nhận thông tin ở lượt sau.";
 $("#networkWakeHint").classList.toggle("waking",wake);
 $("#networkSend").textContent=sending?"Đang gửi…":wake?"Gửi & đánh thức":"Gửi FYI";
 $("#networkSend").disabled=sending||readingFiles||!!validationError();
 $("#networkPickFiles").disabled=sending||readingFiles||!l||files.length>=l.maxFiles;
 $("#networkTarget").disabled=sending;$("#networkText").readOnly=sending;
 $("#networkComposer").querySelectorAll("input[name='networkMode']").forEach(i=>i.disabled=sending);
 $("#networkDraftFiles").querySelectorAll("button").forEach(b=>b.disabled=sending);
}
function renderFiles(){
 $("#networkDraftFiles").replaceChildren(...files.map((f,index)=>{
  const item=node("li"),remove=node("button",null,"×");remove.type="button";remove.setAttribute("aria-label","Bỏ file "+f.name);remove.onclick=()=>{files.splice(index,1);pendingSend=null;formError("");renderFiles();syncComposer();$("#networkPickFiles").focus()};
  item.append(raw("span",null,f.name+" · "+size(bytes(f.content))),remove);return item;
 }));
}
async function pickFiles(){
 const picked=[...$("#networkFiles").files];$("#networkFiles").value="";
 if(!picked.length||!state.limits||sending)return;
 readingFiles=true;syncComposer();formError("");
 try{
  const l=state.limits;
  if(files.length+picked.length>l.maxFiles)throw new Error("Chỉ gửi tối đa "+l.maxFiles+" file mỗi tin.");
  let total=files.reduce((sum,f)=>sum+bytes(f.content),0);const additions=[];
  for(const file of picked){
   if(!file.name||file.name!==file.name.trim()||file.name.startsWith(".")||bytes(file.name)>120||/[\x00-\x1f\x7f-\x9f/\\:]/.test(file.name))throw new Error("Tên file phải ngắn, không ẩn và không chứa đường dẫn: "+file.name);
   if(files.concat(additions).some(f=>f.name.toLowerCase()===file.name.toLowerCase()))throw new Error("Tên file bị trùng: "+file.name);
   if(file.size>l.maxFileBytes)throw new Error(file.name+" vượt "+size(l.maxFileBytes)+".");
   total+=file.size;if(total>l.maxTotalBytes)throw new Error("Tổng file vượt "+size(l.maxTotalBytes)+".");
   let content;try{content=new TextDecoder("utf-8",{fatal:true}).decode(await file.arrayBuffer())}catch{throw new Error(file.name+" không phải văn bản UTF-8.")}
   if(/[\x00-\x08\x0b\x0c\x0e-\x1f\x7f-\x9f]/.test(content))throw new Error(file.name+" chứa dữ liệu nhị phân hoặc ký tự điều khiển.");
   if(bytes(content)>l.maxFileBytes)throw new Error(file.name+" vượt giới hạn sau khi đọc.");
   additions.push({name:file.name,content});
  }
  files.push(...additions);pendingSend=null;renderFiles();
 }catch(e){formError(e.message)}
 finally{readingFiles=false;syncComposer()}
}
$("#networkComposer").addEventListener("submit",async e=>{
 e.preventDefault();if(sending||readingFiles)return;
 const error=validationError();if(error){formError(error);return}
 const payload={toSessionId:selected,text:$("#networkText").value,wake:wakeMode(),attachments:files.map(f=>({name:f.name,content:f.content}))};
 const fingerprint=JSON.stringify(payload);
 if(!pendingSend||pendingSend.fingerprint!==fingerprint)pendingSend={fingerprint,payload:{...payload,clientNonce:nonce()}};
 sending=true;formError("");syncComposer();
 try{
  const result=await request("/network/messages",pendingSend.payload);
  if(!result.message)throw new Error("Missing message receipt");
  const index=state.messages.findIndex(m=>m.id===result.message.id);if(index<0)state.messages.push(result.message);else state.messages[index]=result.message;
  $("#networkText").value="";files=[];pendingSend=null;renderFiles();renderMailbox();
  notice(result.duplicate?"Lần gửi trước đã được ghi nhận; không tạo tin trùng.":payload.wake?"Đã ghi nhận yêu cầu đánh thức. Xem trạng thái giao tin và tác vụ bên trên.":"Đã gửi FYI. Agent chưa được đánh thức.");
  await refresh();
 }catch(e){formError(errorText(e,"Chưa xác nhận được lần gửi. Bấm gửi lại với cùng nội dung để kiểm tra an toàn."))}
 finally{sending=false;syncComposer()}
});
function open(){
 if(dialog.open)return;
 lastFocus=document.activeElement;dialog.showModal();entry.setAttribute("aria-expanded","true");$("#networkClose").focus();refresh(true);
 timer=setInterval(()=>{if(dialog.open&&!document.hidden)refresh()},5000);
}
function close(){if(dialog.open)dialog.close()}
entry.addEventListener("click",open);$("#networkClose").onclick=close;
dialog.addEventListener("close",()=>{clearInterval(timer);timer=null;entry.setAttribute("aria-expanded","false");if(lastFocus?.isConnected)lastFocus.focus()});
dialog.addEventListener("cancel",e=>{e.preventDefault();close()});
dialog.addEventListener("keydown",e=>{
 if(e.key==="Escape"){e.preventDefault();e.stopPropagation();close();return}
 if(e.key!=="Tab")return;
 const focusable=[...dialog.querySelectorAll("button,a[href],input,textarea,select,summary,[tabindex]")].filter(n=>!n.disabled&&n.tabIndex>=0&&n.getClientRects().length);
 const first=focusable[0],last=focusable.at(-1);
 if(e.shiftKey&&document.activeElement===first){e.preventDefault();last?.focus()}else if(!e.shiftKey&&document.activeElement===last){e.preventDefault();first?.focus()}
});
dialog.addEventListener("click",e=>{if(e.target===dialog){const r=dialog.getBoundingClientRect();if(e.clientX<r.left||e.clientX>r.right||e.clientY<r.top||e.clientY>r.bottom)close()}});
$("#networkRefresh").onclick=()=>refresh(true);
$("#networkSearch").addEventListener("input",renderDirectory);
$("#networkTarget").addEventListener("change",e=>chooseSession(e.target.value));
$("#networkText").addEventListener("input",()=>{pendingSend=null;formError("");syncComposer()});
$("#networkComposer").querySelectorAll("input[name='networkMode']").forEach(i=>i.addEventListener("change",()=>{pendingSend=null;syncComposer()}));
$("#networkPickFiles").onclick=()=>$("#networkFiles").click();$("#networkFiles").addEventListener("change",pickFiles);
document.addEventListener("visibilitychange",()=>{if(!document.hidden&&dialog.open)refresh()});
for(const [id,name] of [["networkIcon","message-square-dot"],["networkSearchIcon","search"],["networkFileIcon","folder"]])if(window.AURORA_ICONS)$("#"+id).replaceChildren(AURORA_ICONS.icon(name,18));
window.AURORA_NETWORK={peerMessage(m){
 const card=node("div","network-peer-message"),source=m.origin==="human"?"Giao tiếp · chủ sở hữu":(m.fromName||"Agent")+" · tin từ agent";
 card.append(raw("div","network-peer-source",source),raw("div","network-peer-body",m.text||""));return card;
}};
syncComposer();
})();
