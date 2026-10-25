"use strict";
(()=>{
const $=s=>document.querySelector(s),dialog=$("#cloudDialog"),entry=$("#cloudBtn"),zoom=$("#cloudZoom");
if(!dialog||!entry)return;
const node=(tag,text)=>{const n=document.createElement(tag);if(text!=null)n.textContent=text;return n};
const pendingKey="aurora-cloud-pending-v1",pending=new Map();
let agents=[],vaultItems=[],selected="",state={},observation=null,frame=null,point=null;
let busy=false,reading=false,generation=0,revision=0,lastFocus=null,timer=null,readController=null;
let targetSignature="",vaultSignature="",imageRevision=0,reviewed=false,connectionFailed=false,zoomFocus=null;
try{
 const saved=JSON.parse(sessionStorage.getItem(pendingKey)||"{}");
 for(const [id,items] of Object.entries(saved))if(/^bot-[A-Za-z0-9_-]+$/.test(id)&&Array.isArray(items)){
  const safe=items.filter(p=>p&&typeof p.nonce==="string"&&/^cloud-[A-Za-z0-9-]{8,100}$/.test(p.nonce)&&Number.isFinite(p.at)).slice(-12).map(p=>({nonce:p.nonce,at:p.at,status:"unknown"}));
  if(safe.length)pending.set(id,safe);
 }
}catch{}
function persistPending(){
 try{sessionStorage.setItem(pendingKey,JSON.stringify(Object.fromEntries([...pending].map(([id,items])=>[id,items.map(p=>({nonce:p.nonce,at:p.at}))]))))}catch{}
}
function pendingFor(id=selected){return pending.get(id)||[]}
function uncertain(){return pendingFor().some(p=>p.status==="unknown")}
function removePending(id,nonce){const items=pendingFor(id).filter(p=>p.nonce!==nonce);items.length?pending.set(id,items):pending.delete(id);persistPending()}
const nonce=()=>"cloud-"+(crypto.randomUUID?crypto.randomUUID():Array.from(crypto.getRandomValues(new Uint8Array(16)),n=>n.toString(16).padStart(2,"0")).join(""));
function notice(text,bad=false){$("#cloudNotice").textContent=text;$("#cloudNotice").classList.toggle("bad",bad)}
const statusNames={stopped:"Trình duyệt đã đóng",ready:"Trình duyệt sẵn sàng",starting:"Đang khởi động",recovery_required:"Cần khôi phục · đang tạm dừng",unavailable:"Cloud chưa khả dụng"};
const authNames={login_required:"Cần bạn đăng nhập",reauth_required:"Cần bạn đăng nhập lại",authenticated:"Trang đã xác nhận đăng nhập",unknown:"Đăng nhập chưa xác minh"};
const controlNames={owner:"Bạn đang giữ điều khiển",agent:"Agent đang giữ điều khiển · bạn đang quan sát",paused:"Agent đang tạm dừng"};
const errors={
 cloud_browser_not_configured:"Cloud chưa được cấu hình trên máy chủ. Chưa có trình duyệt cloud để kết nối.",
 cloud_browser_invalid_configuration:"Cấu hình worker cloud chưa hợp lệ. Cần kiểm tra kết nối trên máy chủ.",
 cloud_browser_unavailable:"Chưa kết nối được worker cloud. Hồ sơ trình duyệt không được mở trên máy của bạn.",
 cloud_browser_owner_identity_required:"Cần đăng nhập bằng phiên chủ sở hữu để mở máy cloud.",
 cloud_browser_invalid_request:"Máy chủ từ chối thao tác. Kiểm tra trang và phần tử đã chọn.",
 cloud_browser_invalid_response:"Worker chưa trả về trạng thái hợp lệ. Giữ agent tạm dừng và quan sát mới.",
 cloud_browser_conflict:"Trạng thái hoặc quyền điều khiển đã đổi. Quan sát mới rồi kiểm tra trang.",
 cloud_browser_unknown_outcome:"Chưa xác định được thao tác trước đã diễn ra hay chưa. Không tự gửi lại.",
 store_unavailable:"Kho trạng thái chưa khả dụng. Chưa thực hiện thao tác cloud.",
 fixture_session_invalid:"Phiên chủ sở hữu không còn hợp lệ. Đăng nhập lại trước khi mở máy cloud.",
 principal_forbidden:"Phiên hiện tại không có quyền thực hiện thao tác này.",
 contract_unsupported:"Máy chủ hiện tại chưa hỗ trợ thao tác cloud này.",
 nonce_or_cursor_conflict:"Mã yêu cầu hoặc trạng thái đã đổi. Quan sát mới và kiểm tra trang.",
 not_found:"Không tìm thấy bot hoặc mục được yêu cầu trong phạm vi của bạn.",
 not_configured:"Cloud chưa được cấu hình trên máy chủ. Chưa có trình duyệt cloud để kết nối.",
 cloud_not_configured:"Cloud chưa được cấu hình trên máy chủ. Chưa có trình duyệt cloud để kết nối.",
 cloud_unavailable:"Chưa kết nối được worker cloud. Hồ sơ trình duyệt không được mở trên máy của bạn.",
 cloud_identity_required:"Cần đăng nhập bằng phiên chủ sở hữu để mở máy cloud.",
 cloud_forbidden:"Phiên hiện tại không có quyền điều khiển máy cloud này.",
 cloud_group_unsupported:"Nhóm không có hồ sơ trình duyệt riêng. Hãy chọn một bot.",
 cloud_not_found:"Không tìm thấy bot hoặc hồ sơ cloud trong phạm vi của bạn.",
 cloud_stale_epoch:"Máy cloud đã khởi động lại hoặc đổi quyền điều khiển. Hãy quan sát mới.",
 stale_epoch:"Máy cloud đã khởi động lại hoặc đổi quyền điều khiển. Hãy quan sát mới.",
 stale_observation:"Trang đã thay đổi. Quan sát mới trước khi thao tác.",
 stale_frame:"Ảnh màn hình đã cũ. Chọn lại điểm trên ảnh mới.",
 stale_target:"Phần tử đã thay đổi. Quan sát mới rồi chọn lại.",
 control_required:"Bạn cần nhận điều khiển trước khi nhập hoặc bấm trên trang.",
 recovery_required:"Worker cần khôi phục trạng thái. Giữ agent tạm dừng và kiểm tra trang.",
 unknown_outcome:"Chưa xác định được thao tác trước đã diễn ra hay chưa. Không tự gửi lại.",
 cloud_unknown_outcome:"Chưa xác định được thao tác trước đã diễn ra hay chưa. Không tự gửi lại.",
 vault_origin_mismatch:"Trang hiện tại không trùng trang của mục Vault. Chưa điền dữ liệu.",
 cloud_vault_origin_mismatch:"Trang hiện tại không trùng trang của mục Vault. Chưa điền dữ liệu.",
 vault_item_changed:"Mục Vault đã thay đổi. Tải lại danh sách và duyệt lại lần dùng này.",
 protected_login:"Trang đang ở bước đăng nhập cần chủ sở hữu xử lý.",
 owner_control_required:"Bạn cần nhận điều khiển và quan sát mới trước khi cho phép agent dùng hồ sơ.",
 browser_stopped:"Trình duyệt chưa sẵn sàng. Mở lại và quan sát trước khi cấp quyền cho agent.",
 egress_denied:"Địa chỉ này nằm ngoài phạm vi kết nối được phép của worker.",
 capacity_exceeded:"Worker đã đạt số trình duyệt đang mở. Đóng trình duyệt cloud khác rồi kiểm tra lại."
};
function errorText(e){
 const value=e.body?.error,code=e.body?.code||(value&&typeof value==="object"?value.code:value);
 if(typeof code==="string"&&errors[code])return errors[code];
 if(e.status===401)return"Phiên chủ sở hữu đã hết hạn. Đóng cửa sổ và đăng nhập lại.";
 if(e.status===403)return"Bạn không có quyền thực hiện thao tác cloud này.";
 if(e.status===404)return"Không tìm thấy bot hoặc tính năng cloud trên máy chủ hiện tại.";
 if(e.status===409)return"Trạng thái hoặc quyền điều khiển đã đổi. Quan sát mới rồi kiểm tra trang.";
 if(e.status===400)return"Máy chủ từ chối thao tác. Kiểm tra trang và phần tử đã chọn.";
 if(e.status===413)return"Nội dung vượt giới hạn máy chủ. Hãy nhập ít hơn.";
 if(e.status===429)return"Worker đang giới hạn yêu cầu. Đợi một chút rồi quan sát mới.";
 if(e.status===423)return"Kho trạng thái đang ở chế độ chỉ đọc. Chưa thể điều khiển cloud.";
 return"Chưa nhận được xác nhận từ máy chủ. Kiểm tra kết nối và quan sát mới.";
}
async function request(path,body,signal){
 const get=body===undefined,options={method:get?"GET":"POST",credentials:"same-origin",cache:"no-store",signal};
 if(!get){options.headers={"Content-Type":"application/json"};options.body=JSON.stringify(body)}
 if(!signal&&typeof AbortSignal!=="undefined"&&AbortSignal.timeout)options.signal=AbortSignal.timeout(get?15000:30000);
 const response=await fetch("/app/api"+path,options);
 if(!response.ok){const e=new Error("Cloud request failed");e.status=response.status;e.body=await response.json().catch(()=>null);throw e}
 return response.json();
}
function path(id=selected){return"/agents/"+encodeURIComponent(id)+"/cloud"}
function wire(data){return data?.state&&typeof data.state==="object"?{...data.state,...(typeof data.configured==="boolean"?{configured:data.configured}:{})}:data}
function sameView(id,version,operation){return dialog.open&&selected===id&&generation===version&&revision===operation}
function clearInputs(){
 $("#cloudText").value="";$("#cloudVaultConfirm").checked=false;$("#cloudX").value="";$("#cloudY").value="";point=null;
}
function clearFrame(message){
 closeZoom(false);
 imageRevision++;frame=null;point=null;$("#cloudFrame").removeAttribute("src");$("#cloudFramePick").classList.add("hidden");$("#cloudFrameEmpty").classList.remove("hidden");
 if(message)$("#cloudFrameMessage").textContent=message;
}
function closeZoom(restore=true){
 const focus=zoomFocus;zoomFocus=null;
 if(zoom.open)zoom.close();
 $("#cloudZoomFrame").removeAttribute("src");zoom.classList.remove("native");$("#cloudZoomNative").setAttribute("aria-pressed","false");$("#cloudExpand").setAttribute("aria-expanded","false");
 if(restore&&dialog.open){if(focus?.isConnected&&!focus.disabled)focus.focus();else $("#cloudDismiss").focus()}
}
function openZoom(){
 if(!dialog.open||!frame||zoom.open)return;
 const source=$("#cloudFrame").getAttribute("src");if(!source)return;
 zoomFocus=document.activeElement;$("#cloudZoomFrame").src=source;zoom.showModal();$("#cloudExpand").setAttribute("aria-expanded","true");$("#cloudZoomClose").focus();
}
function invalidate(){observation=null;point=null;reviewed=false;$("#cloudVaultConfirm").checked=false}
function applyState(data){
 const value=wire(data);if(!value||typeof value!=="object")throw new Error("Invalid cloud response");
 const previous=state;
 state={...state};
 if(typeof value.configured==="boolean")state.configured=value.configured;
 if(typeof value.localDocker==="boolean")state.localDocker=value.localDocker;
 state.protected=value.protected!==false;
 if(state.protected&&observation)observation.ownerObserved=false;
 for(const key of ["status","control","bootId","lastSavedAt"])if(typeof value[key]==="string")state[key]=value[key];
 state.origin=typeof value.origin==="string"?value.origin:"";state.title=typeof value.title==="string"?value.title:"";state.authState=authNames[value.authState]?value.authState:"unknown";
 if(Number.isSafeInteger(value.epoch)&&value.epoch>=0)state.epoch=value.epoch;
 if((previous.epoch&&state.epoch!==previous.epoch)||(previous.bootId&&state.bootId!==previous.bootId)||(previous.origin&&state.origin!==previous.origin)){
  invalidate();clearFrame("Phiên hoặc trang đã thay đổi. Đang lấy ảnh mới.");
 }
 if(observation&&value.observationId!==observation.id)invalidate();
 if(state.control!=="owner")point=null;
 if(state.configured===false||state.status!=="ready"){
  invalidate();clearFrame(state.configured===false?"Chưa cấu hình worker cloud. Aurora không dùng trình duyệt trên máy của bạn.":state.status==="recovery_required"?"Worker vừa khôi phục. Agent đang tạm dừng; hãy kiểm tra phiên cloud.":"Trình duyệt cloud chưa sẵn sàng để hiển thị.");
 }
 return value;
}
function applyObservation(value){
 if(typeof value.observationId!=="string"||!value.observationId||!Number.isSafeInteger(state.epoch))throw new Error("Missing cloud observation");
 const targets=Array.isArray(value.targets)?value.targets.slice(0,256).filter(t=>t&&typeof t.id==="string"&&typeof t.role==="string"&&typeof t.name==="string").map(t=>({id:t.id,role:t.role,name:t.name,type:typeof t.type==="string"?t.type:"",protected:t.protected===true})):[];
 if(observation?.id!==value.observationId)$("#cloudVaultConfirm").checked=false;
 observation={id:value.observationId,epoch:state.epoch,botId:selected,ownerObserved:state.control==="owner",protected:value.protected!==false,origin:state.origin||"",targets,text:typeof value.text==="string"?value.text.slice(0,16384):""};
 $("#cloudObservation").textContent=observation.text||"Quan sát không có nội dung văn bản. Xem màn hình và các phần tử hiện tại.";
}
function setFrame(value,id,version,operation){
 const f=value.frame;
 if(!f)return;
 if(!f.id||typeof f.id!=="string"||typeof value.observationId!=="string"||!value.observationId||!["image/png","image/jpeg"].includes(f.mime)||typeof f.data!=="string"||!f.data.length||f.data.length>1398104||f.data.length%4!==0||!/^[A-Za-z0-9+/]+={0,2}$/.test(f.data)||!Number.isInteger(f.width)||!Number.isInteger(f.height)||f.width<1||f.height<1||f.width>8192||f.height>8192)throw new Error("Invalid cloud frame");
 if(frame?.id===f.id&&frame.epoch===state.epoch)return;
 const image=$("#cloudFrame"),ticket=++imageRevision,candidate={id:f.id,width:f.width,height:f.height,epoch:state.epoch,observationId:value.observationId};
 frame=null;
 image.onload=()=>{
  if(ticket!==imageRevision||!sameView(id,version,operation)||document.hidden)return;
  if(image.naturalWidth!==candidate.width||image.naturalHeight!==candidate.height){clearFrame("Kích thước ảnh không khớp trạng thái worker. Quan sát mới trước khi chọn điểm.");render();return}
  frame=candidate;$("#cloudFramePick").classList.remove("hidden");$("#cloudFrameEmpty").classList.add("hidden");if(zoom.open)$("#cloudZoomFrame").src=image.src;render();
 };
 image.onerror=()=>{if(ticket===imageRevision&&sameView(id,version,operation)){clearFrame("Không đọc được ảnh màn hình. Quan sát mới để kết nối lại.");render()}};
 image.src="data:"+f.mime+";base64,"+f.data;
}
function renderTargets(){
 const select=$("#cloudTarget"),targets=observation?.targets||[],signature=JSON.stringify(targets);
 if(signature===targetSignature)return;
 const current=select.value,options=[node("option",targets.length?"Chọn phần tử cần thao tác":"Chưa có phần tử · quan sát mới")];options[0].value="";
 for(const target of targets){const option=node("option",[target.role,target.name||"Không có tên",target.protected?"mục nhập bảo vệ":""].filter(Boolean).join(" · "));option.value=target.id;option.dataset.noi18n="";options.push(option)}
 select.replaceChildren(...options);select.value=targets.some(t=>t.id===current)?current:"";targetSignature=signature;
}
function canonicalOrigin(value){
 try{const url=new URL(value);return url.protocol==="https:"&&!url.username&&!url.password?url.origin:""}catch{return""}
}
function renderVault(){
 const select=$("#cloudVaultItem"),signature=JSON.stringify(vaultItems.map(item=>[item.id,item.label,item.hint?.site]));
 if(signature!==vaultSignature){
  const current=select.value,options=[node("option",vaultItems.length?"Chọn mục đăng nhập":"Chưa có mục đăng nhập trong Vault")];options[0].value="";
  for(const item of vaultItems){const option=node("option",item.label||item.hint?.site||"Mục đăng nhập");option.value=item.id;option.dataset.noi18n="";options.push(option)}
  select.replaceChildren(...options);select.value=vaultItems.some(item=>item.id===current)?current:"";vaultSignature=signature;$("#cloudVaultConfirm").checked=false;
 }
 const item=vaultItems.find(item=>item.id===select.value),site=item?.hint?.site||"",current=observation?.origin||state.origin||"";
 $("#cloudVaultSite").textContent=site||"—";$("#cloudVaultOrigin").textContent=current||"—";
 const matches=!!item&&!!canonicalOrigin(site)&&canonicalOrigin(site)===canonicalOrigin(current);
 const allowed=canInput()&&!!observation&&matches;
 select.disabled=busy||!vaultItems.length;$("#cloudVaultConfirm").disabled=!allowed;$("#cloudVaultFill").disabled=!allowed||!$("#cloudVaultConfirm").checked;
 $("#cloudVaultHint").textContent=item&&!matches?"Trang đã lưu phải trùng origin HTTPS của trang đang quan sát. Chưa điền dữ liệu.":"Chỉ tải tên và trang của mục đăng nhập. Giá trị thật được điền trên cloud, không tự gửi form.";
}
function canInput(){return!!selected&&!connectionFailed&&state.configured===true&&state.status==="ready"&&state.control==="owner"&&Number.isSafeInteger(state.epoch)&&state.epoch>0&&!busy&&!uncertain()}
function canGrantProfile(){return canInput()&&observation?.botId===selected&&observation.epoch===state.epoch&&observation.ownerObserved&&observation.protected===false&&state.protected===false&&!["login_required","reauth_required"].includes(state.authState)}
function render(){
 renderTargets();
 $("#cloudIntro").textContent=state.localDocker?"Docker tạm trên máy này · hồ sơ riêng của bot. Máy này cần bật; chuyển worker lên máy chủ riêng để chạy độc lập.":"Trình duyệt riêng của bot chạy trên cloud. Bạn xem và can thiệp tại đây.";
 const ready=state.configured===true&&state.status==="ready",epoch=Number.isSafeInteger(state.epoch)&&state.epoch>0,owned=canInput(),observed=owned&&observation?.epoch===state.epoch,target=observed&&observation.targets.some(t=>t.id===$("#cloudTarget").value);
 const status=$("#cloudStatus");status.textContent=!selected?"Chưa chọn bot":connectionFailed?"Chưa kết nối được cloud":state.configured===false?"Chưa cấu hình cloud":statusNames[state.status]||"Chưa tải trạng thái";status.classList.toggle("warn",state.configured===false||state.status==="recovery_required");status.classList.toggle("bad",connectionFailed||state.status==="unavailable");
 $("#cloudAuth").textContent=connectionFailed?authNames.unknown:authNames[state.authState]||authNames.unknown;$("#cloudAuth").classList.toggle("warn",connectionFailed||state.authState!=="authenticated");
 $("#cloudPage").textContent=[state.title,state.origin].filter(Boolean).join(" · ")||"Chưa mở trình duyệt";
 $("#cloudControlState").textContent=busy?"Đang chờ xác nhận thao tác…":controlNames[state.control]||"Chỉ chủ sở hữu có thể mở và điều khiển máy cloud.";
 $("#cloudBot").disabled=busy;$("#cloudRefresh").disabled=!selected||busy||reading;
 $("#cloudRefresh").textContent=state.status==="recovery_required"?"Kiểm tra trạng thái":"Quan sát mới";$("#cloudOpen").textContent=state.status==="recovery_required"?"Mở lại trình duyệt":"Mở trình duyệt";
 $("#cloudOpen").disabled=busy||uncertain()||connectionFailed||state.configured!==true||!["stopped","unavailable","recovery_required"].includes(state.status);
 $("#cloudTake").disabled=busy||!ready||!epoch||state.control==="owner";
 $("#cloudPause").disabled=busy||!ready||!epoch||state.control==="paused";
 $("#cloudResume").disabled=!canGrantProfile();
 $("#cloudGrantHint").textContent=state.protected!==false||["login_required","reauth_required"].includes(state.authState)?"Trang đăng nhập, QR hoặc bước bảo vệ cần bạn xử lý. Chỉ cho phép agent sau một quan sát mới không còn bước bảo vệ; quyền này không xác minh danh tính Zalo.":"Cho phép agent dùng hồ sơ đang mở của bot này là quyết định của bạn. Quyền này không xác minh danh tính Zalo và không xác nhận đăng nhập thành công.";
 $("#cloudStop").disabled=busy||state.configured!==true||!epoch||!["ready","starting","recovery_required"].includes(state.status);
 $("#cloudUrl").disabled=!owned;$("#cloudGo").disabled=!owned;
 $("#cloudTarget").disabled=!observed;$("#cloudClick").disabled=!target;$("#cloudText").disabled=!target;$("#cloudFill").disabled=!target||!$("#cloudText").value;
 $("#cloudKey").disabled=!target;$("#cloudPress").disabled=!target;
 $("#cloudFramePick").disabled=!owned||!frame;$("#cloudX").disabled=!owned||!frame;$("#cloudY").disabled=!owned||!frame;
 $("#cloudExpand").disabled=!frame;
 $("#cloudX").max=frame?String(frame.width-1):"0";$("#cloudY").max=frame?String(frame.height-1):"0";
 const validPoint=owned&&frame&&point&&point.frameId===frame.id&&point.epoch===state.epoch;
 $("#cloudPoint").disabled=!validPoint;$("#cloudPointState").textContent=validPoint?`Điểm ${point.x}, ${point.y} · gắn với ảnh hiện tại.`:point?"Ảnh đã đổi. Chọn lại điểm hoặc sửa tọa độ trên ảnh mới.":"Chưa chọn điểm.";
 $("#cloudScrollUp").disabled=!observed;$("#cloudScrollDown").disabled=!observed;
 const recovery=state.status==="recovery_required";
 $("#cloudUncertain").classList.toggle("hidden",!uncertain());$("#cloudUncertainText").textContent=recovery?"Worker cần khôi phục; thao tác cũ có thể đã diễn ra. Kiểm tra trạng thái rồi xác nhận bạn ghi nhận kết quả chưa rõ. Sau đó bạn có thể mở lại trình duyệt với hồ sơ cũ. Aurora không chạy lại thao tác cũ.":"Có thao tác chưa rõ kết quả. Aurora giữ mã yêu cầu và không gửi lại. Quan sát mới, kiểm tra trang và trạng thái trước khi cho phép thao tác tiếp theo.";
 $("#cloudPendingIds").textContent=pendingFor().filter(p=>p.status==="unknown").map(p=>p.nonce+" · kết quả chưa rõ").join("\n");$("#cloudResolve").textContent=recovery?"Tôi ghi nhận kết quả chưa rõ · cho phép mở lại":"Tôi đã kiểm tra trang · cho phép thao tác mới";$("#cloudResolve").disabled=busy||connectionFailed||!reviewed;
 renderVault();
}
async function refresh(observe=false,manual=false){
 if(!selected||busy||reading||!dialog.open||document.hidden)return;
 reading=true;const id=selected,version=generation,operation=revision,controller=new AbortController();readController=controller;render();
 const timeout=setTimeout(()=>controller.abort(),20000),signal=controller.signal;
 if(manual)notice("Đang lấy trạng thái và quan sát mới từ cloud…");
 try{
  const data=await request(path(id),undefined,signal);if(!sameView(id,version,operation))return;
  applyState(data);
  if(state.configured===true&&state.status==="ready"){
   if(observe){const result=await request(path(id)+"/observe",{},signal);if(!sameView(id,version,operation))return;applyObservation(applyState(result));reviewed=true}
   if(!document.hidden){const result=await request(path(id)+"/frame",{},signal);if(!sameView(id,version,operation)||document.hidden)return;setFrame(applyState(result),id,version,operation)}
  }else if(observe&&state.configured===true&&["stopped","unavailable","recovery_required"].includes(state.status))reviewed=true;
  connectionFailed=false;
  if(manual)notice(state.configured===false?errors.not_configured:uncertain()?"Đã lấy trạng thái mới. Kiểm tra trang; yêu cầu chưa rõ kết quả vẫn chưa được gửi lại.":"Đã cập nhật từ cloud. Trạng thái đăng nhập chỉ dựa trên bằng chứng từ trang.");
 }catch(e){if(sameView(id,version,operation)&&!(signal.aborted&&document.hidden)){connectionFailed=true;reviewed=false;clearFrame("Chưa nhận được ảnh mới từ cloud. Kiểm tra kết nối và quan sát mới.");notice(errorText(e),true)}}
 finally{clearTimeout(timeout);if(generation===version&&readController===controller){reading=false;readController=null;render()}}
}
async function mutate(suffix,body,label){
 if(busy||!selected||state.configured!==true)return;
 const safety=suffix==="/control"&&["paused","owner"].includes(body.mode)||suffix==="/close";
 if(uncertain()&&!safety){notice("Kiểm tra thao tác chưa rõ kết quả trước khi tạo thao tác mới.",true);return}
 const id=selected,version=generation,marker={nonce:nonce(),at:Date.now(),status:"started"};
 pending.set(id,[...pendingFor(id),marker]);persistPending();busy=true;revision++;readController?.abort();reading=false;reviewed=false;
 const payload={...body,clientNonce:marker.nonce};invalidate();clearFrame("Đang chờ thao tác và ảnh mới từ cloud…");render();notice(label);
 try{
  const result=await request(path(id)+suffix,payload),value=wire(result),receipt=value?.receipt||result?.receipt;
  if(receipt?.nonce===marker.nonce&&["dispatched","rejected"].includes(receipt.status)){
   removePending(id,marker.nonce);
   if(dialog.open&&generation===version&&selected===id){applyState(result);notice(receipt.status==="dispatched"?"Đã chuyển thao tác tới trình duyệt. Chưa xác nhận đăng nhập, gửi form hoặc gửi tin thành công.":errors[receipt.code]||"Worker đã từ chối thao tác. Quan sát mới để kiểm tra trang.",receipt.status==="rejected")}
  }else{marker.status="unknown";persistPending();if(dialog.open&&generation===version&&selected===id){if(receipt?.nonce===marker.nonce&&receipt.status==="unknown")applyState(result);notice(errors.unknown_outcome,true)}}
 }catch(e){marker.status="unknown";persistPending();if(dialog.open&&generation===version)notice(errorText(e)+" Mã yêu cầu được giữ lại; Aurora không tự gửi lại.",true)}
 finally{
  busy=false;
  if(dialog.open){render();if(selected)await refresh(true,false)}
 }
}
function targetAction(kind,extra={}){
 if(!canInput()||!observation)return;
 const target=observation.targets.find(t=>t.id===$("#cloudTarget").value);if(!target){notice("Chọn phần tử từ quan sát hiện tại.",true);return}
 return mutate("/action",{kind,epoch:state.epoch,observationId:observation.id,targetId:target.id,...extra},"Đang chuyển thao tác trực tiếp tới trình duyệt…");
}
function bindPoint(x,y){
 if(!canInput()||!frame||!Number.isInteger(x)||!Number.isInteger(y)||x<0||y<0||x>=frame.width||y>=frame.height){point=null;render();return}
 point={frameId:frame.id,epoch:frame.epoch,x,y};$("#cloudX").value=String(x);$("#cloudY").value=String(y);render();
}
async function loadVault(){
 $("#cloudVaultRefresh").disabled=true;
 try{const data=await request("/vault");vaultItems=Array.isArray(data.items)?data.items.filter(item=>item&&item.type==="login"&&typeof item.id==="string"):[];if(dialog.open)renderVault()}
 catch(e){if(dialog.open){$("#cloudVaultHint").textContent="Không tải được danh sách Vault. "+errorText(e)}}
 finally{$("#cloudVaultRefresh").disabled=false}
}
function chooseBot(id){
 if(busy||!agents.some(agent=>agent.id===id))return;
 generation++;revision++;readController?.abort();reading=false;selected=id;state={};connectionFailed=false;invalidate();clearInputs();clearFrame("Đang lấy trạng thái cloud của bot…");$("#cloudObservation").textContent="Chưa có quan sát.";$("#cloudBot").value=id;notice("");render();refresh(true,true);
}
async function open(detail={}){
 if(dialog.open){if(detail.botId&&detail.botId!==selected)chooseBot(detail.botId);return}
 lastFocus=document.activeElement;dialog.showModal();entry.setAttribute("aria-expanded","true");$("#cloudDismiss").focus();render();
 const version=++generation;
 notice("Đang tải danh sách bot…");
 try{
  const data=await request("/state");if(!dialog.open||generation!==version)return;
  agents=Array.isArray(data.agents)?data.agents.filter(agent=>agent&&agent.kind!=="group"&&typeof agent.id==="string"&&/^bot-[A-Za-z0-9_-]+$/.test(agent.id)):[];
  const select=$("#cloudBot"),options=[node("option",agents.length?"Chọn bot":"Chưa có bot")];options[0].value="";
  for(const agent of agents){const option=node("option",agent.name||agent.id);option.value=agent.id;option.dataset.noi18n="";options.push(option)}select.replaceChildren(...options);
  const active=agents.find(agent=>agent.id===location.hash.slice(1))?.id;
  const desired=detail.botId||active||selected||agents[0]?.id||"";
  if(busy){select.value=agents.some(agent=>agent.id===selected)?selected:"";notice("Đang chờ thao tác đã gửi. Mã yêu cầu được giữ lại; chưa tạo thao tác mới.");render()}
  else if(agents.some(agent=>agent.id===desired))chooseBot(desired);
  else{selected="";state={};clearFrame("Chưa có bot phù hợp để mở máy cloud.");notice(detail.botId?"Bot này không nằm trong phạm vi chủ sở hữu hiện tại.":"Tạo một bot trước khi mở máy cloud.",true);render()}
 }catch(e){if(dialog.open&&generation===version)notice(errorText(e),true)}
 if(!dialog.open)return;
 loadVault();timer=setInterval(()=>{if(dialog.open&&!document.hidden)refresh()},5000);
}
function close(){if(dialog.open)dialog.close()}
entry.addEventListener("click",()=>open());$("#cloudDismiss").onclick=close;
document.addEventListener("aurora:cloud-open",event=>open(event.detail||{}));
dialog.addEventListener("close",()=>{
 clearInterval(timer);timer=null;generation++;readController?.abort();reading=false;clearInputs();invalidate();clearFrame("Cửa sổ máy cloud đã đóng.");$("#cloudObservation").textContent="Chưa có quan sát.";entry.setAttribute("aria-expanded","false");if(lastFocus?.isConnected)lastFocus.focus();
});
dialog.addEventListener("cancel",event=>{event.preventDefault();zoom.open?closeZoom():close()});
dialog.addEventListener("keydown",event=>{
 if(event.key==="Escape"){event.preventDefault();event.stopPropagation();zoom.open?closeZoom():close();return}
 if(zoom.open)return;
 if(event.key!=="Tab")return;
 const controls=[...dialog.querySelectorAll("button,input,select,summary,[tabindex]")].filter(n=>!n.disabled&&n.tabIndex>=0&&n.getClientRects().length),first=controls[0],last=controls.at(-1);
 if(event.shiftKey&&document.activeElement===first){event.preventDefault();last?.focus()}else if(!event.shiftKey&&document.activeElement===last){event.preventDefault();first?.focus()}
});
$("#cloudExpand").onclick=openZoom;$("#cloudZoomClose").onclick=()=>closeZoom();
$("#cloudZoomNative").onclick=()=>{const native=zoom.classList.toggle("native");$("#cloudZoomNative").setAttribute("aria-pressed",String(native))};
zoom.addEventListener("cancel",event=>{event.preventDefault();event.stopPropagation();closeZoom()});
zoom.addEventListener("keydown",event=>{
 if(event.key==="Escape"){event.preventDefault();event.stopPropagation();closeZoom();return}
 if(event.key!=="Tab")return;
 const first=$("#cloudZoomNative"),last=$("#cloudZoomStage");
 if(event.shiftKey&&document.activeElement===first){event.preventDefault();last.focus()}else if(!event.shiftKey&&document.activeElement===last){event.preventDefault();first.focus()}
 event.stopPropagation();
});
$("#cloudBot").onchange=event=>chooseBot(event.target.value);
$("#cloudRefresh").onclick=()=>refresh(true,true);
$("#cloudResolve").onclick=()=>{if(!reviewed||busy)return;for(const marker of pendingFor())if(marker.status==="unknown")removePending(selected,marker.nonce);reviewed=false;notice("Đã ghi nhận bạn kiểm tra trạng thái. Thao tác cũ không được chạy lại; bạn có thể chủ động tạo thao tác mới.");render()};
$("#cloudOpen").onclick=()=>mutate("/open",{},"Đang mở trình duyệt riêng trên cloud…");
for(const [id,mode,label] of [["cloudTake","owner","Đang dừng input của agent và nhận điều khiển…"],["cloudPause","paused","Đang tạm dừng quyền điều khiển…"],["cloudResume","agent","Đang cho phép agent dùng hồ sơ này…"]])$("#"+id).onclick=()=>{if(mode==="agent"&&!canGrantProfile()){notice("Cần bạn giữ điều khiển và có quan sát mới không còn bước đăng nhập hoặc bảo vệ trước khi cấp quyền cho agent.",true);return}mutate("/control",{mode,epoch:state.epoch},label)};
$("#cloudStop").onclick=()=>mutate("/close",{epoch:state.epoch},"Đang đóng trình duyệt cloud và giữ hồ sơ…");
$("#cloudNavigate").onsubmit=event=>{event.preventDefault();if(canInput())mutate("/action",{kind:"navigate",epoch:state.epoch,url:$("#cloudUrl").value.trim(),...(observation?{observationId:observation.id}:{})},"Đang mở địa chỉ trên cloud…")};
$("#cloudTarget").onchange=()=>{ $("#cloudText").value="";render()};
$("#cloudText").oninput=render;$("#cloudClick").onclick=()=>targetAction("click");
$("#cloudTargetForm").onsubmit=event=>{event.preventDefault();const text=$("#cloudText").value;if(!text||!canInput())return;$("#cloudText").value="";targetAction("fill",{text})};
$("#cloudPress").onclick=()=>targetAction("press",{key:$("#cloudKey").value});
$("#cloudFramePick").onclick=event=>{
 if(!canInput()||!frame)return;
 const rect=$("#cloudFrame").getBoundingClientRect(),scale=Math.min(rect.width/frame.width,rect.height/frame.height),width=frame.width*scale,height=frame.height*scale,left=rect.left+(rect.width-width)/2,top=rect.top+(rect.height-height)/2;
 const x=event.detail===0?width/2:event.clientX-left,y=event.detail===0?height/2:event.clientY-top;
 if(x<0||y<0||x>=width||y>=height||!scale)return;
 bindPoint(Math.min(frame.width-1,Math.floor(x/scale)),Math.min(frame.height-1,Math.floor(y/scale)));
};
for(const id of ["cloudX","cloudY"])$("#"+id).oninput=()=>{if(!$("#cloudX").value||!$("#cloudY").value){point=null;render();return}bindPoint(Number($("#cloudX").value),Number($("#cloudY").value))};
$("#cloudPointForm").onsubmit=event=>{event.preventDefault();if(!canInput()||!frame||!point||point.frameId!==frame.id)return;mutate("/action",{kind:"click",epoch:state.epoch,observationId:frame.observationId,frameId:point.frameId,x:point.x,y:point.y},"Đang bấm tại điểm đã chọn trên ảnh…")};
for(const [id,deltaY] of [["cloudScrollUp",-480],["cloudScrollDown",480]])$("#"+id).onclick=()=>{if(canInput()&&observation)mutate("/action",{kind:"scroll",epoch:state.epoch,observationId:observation.id,deltaY},"Đang cuộn trang trên cloud…")};
$("#cloudVaultRefresh").onclick=loadVault;$("#cloudVaultItem").onchange=()=>{$("#cloudVaultConfirm").checked=false;renderVault()};$("#cloudVaultConfirm").onchange=renderVault;
$("#cloudVaultForm").onsubmit=event=>{
 event.preventDefault();if(!canInput()||!observation||!$("#cloudVaultConfirm").checked)return;
 const item=vaultItems.find(item=>item.id===$("#cloudVaultItem").value),origin=canonicalOrigin(observation.origin);
 if(!item||!origin||canonicalOrigin(item.hint?.site)!==origin){notice("Trang hiện tại không khớp mục Vault. Quan sát mới rồi kiểm tra lại.",true);return}
 $("#cloudVaultConfirm").checked=false;
 mutate("/vault",{itemId:item.id,epoch:state.epoch,observationId:observation.id,expectedOrigin:origin,confirmed:true},"Đang yêu cầu điền Vault cho đúng trang đã duyệt…");
};
document.addEventListener("visibilitychange",()=>{if(document.hidden){readController?.abort();clearFrame("Màn hình được ẩn khi bạn rời ứng dụng.");render()}else if(dialog.open)refresh()});
if(window.AURORA_ICONS)$("#cloudIcon").replaceChildren(AURORA_ICONS.icon("monitor",19));
render();
})();
