"use strict";
(() => {
  const el=(tag,text,cls)=>{const n=document.createElement(tag);if(text!=null)n.textContent=text;if(cls)n.className=cls;return n};
  const value=(tag,text)=>{const n=el(tag,text);n.dataset.noi18n="";return n};
  function create({server,workspaceId,api,oauth,active}) {
    const root=el("section",null,"mcp-authorization"),open=el("button","Xem thông tin cấp quyền · "+server.name,"pill soft");
    open.type="button";
    const status=el("p",null,"caption");status.setAttribute("role","status");status.setAttribute("aria-live","polite");
    const result=el("div"),management=el("div");root.append(open,status,result,management);let pending=false,authorization=null,account=null,busy=false;
    const alive=()=>root.isConnected&&active();
    const accountStatus=el("p",null,"caption");accountStatus.setAttribute("role","status");accountStatus.tabIndex=-1;
    const controls=el("div"),reload=el("button","Tải trạng thái đăng nhập","pill soft");reload.type="button";
    if(oauth)management.append(reload,accountStatus,controls);
    const identity=()=>({workspaceId,name:server.name,authorizationGeneration:account?.generation||"",authorizationRevision:account?.revision||0});
    const labels={disconnected:"Chưa kết nối tài khoản.",pending:"Đang chờ bạn đăng nhập tại nhà cung cấp.",connected:"Tài khoản đã kết nối.",expired:"Phiên đăng nhập đã hết hạn.",denied:"Bạn đã từ chối cấp quyền.",configuration_changed:"Cấu hình đã thay đổi. Cần đăng nhập lại.",outcome_unknown:"Chưa biết kết quả lần đổi token trước. Cần bắt đầu đăng nhập mới."};
    function renderAccount(){
      controls.replaceChildren();if(!account)return;
      accountStatus.textContent=labels[account.phase];
      if(account.scopes?.length)controls.append(value("p","Quyền của phiên: "+account.scopes.join(", ")));
      if(account.expiresAt)controls.append(el("p","Hết hạn: "+new Date(account.expiresAt*1000).toLocaleString("vi-VN"),"caption"));
      if(account.authorizationUrl){const link=el("a","Tiếp tục đăng nhập tại nhà cung cấp","pill");const u=new URL(account.authorizationUrl);if(u.protocol==="https:"&&!u.username&&!u.password){link.href=u.href;link.target="_blank";link.rel="noopener noreferrer";controls.append(link)}}
      const button=(text,action)=>{const b=el("button",text,"pill soft");b.type="button";b.onclick=action;controls.append(b)};
      if(account.canRefresh)button("Làm mới phiên đăng nhập",()=>operate("refresh",{nonce:crypto.randomUUID()}));
      if(account.phase!=="disconnected"){
        controls.append(el("p","Ngắt kết nối xóa token trên Aurora. Thu hồi quyền tại nhà cung cấp trong trang quản lý tài khoản của họ.","caption"));
        button("Ngắt kết nối trên Aurora",()=>operate("disconnect"));
      }
      if(["connected","pending"].includes(account.phase))return;
      if(server.header||server.secret){controls.append(el("p","Xóa cấu hình header và bí mật thủ công trước khi đăng nhập OAuth.","caption"));return}
      if(!authorization?.authorization?.pkceS256){controls.append(el("p","Xem thông tin cấp quyền và chọn máy chủ hỗ trợ đăng nhập trước khi tiếp tục.","caption"));return}
      if(!account.redirectUri){controls.append(el("p","Địa chỉ Aurora hiện tại chưa được cấu hình cho đăng nhập MCP.","caption"));return}
      controls.append(el("p","Dùng Client ID công khai đã đăng ký tại máy chủ cấp quyền với địa chỉ quay về sau:","caption"),value("p",account.redirectUri));
      const label=el("label","Client ID"),client=el("input");client.className="ext-in";client.maxLength=2048;client.autocomplete="off";client.spellcheck=false;label.append(client);controls.append(label);
      button("Bắt đầu đăng nhập",()=>{if(client.value.trim())operate("start",{clientId:client.value.trim(),issuer:authorization.authorization.issuer,scopes:authorization.scopes||[]})});
    }
    async function operate(action,extra={}){
      if(busy||pending||!alive())return;busy=true;reload.disabled=true;open.disabled=true;controls.querySelectorAll("button,input").forEach(n=>n.disabled=true);accountStatus.textContent="Đang xử lý…";
      try{
        const data=await oauth(action,{...identity(),...extra});if(!alive())return;
        if(!Object.hasOwn(labels,data.phase)||!Number.isSafeInteger(data.revision)||data.revision<0)throw Error();
        account=data;renderAccount();
      }catch(error){
        if(alive()){account=null;controls.replaceChildren();const messages={mcp_oauth_disabled:"Địa chỉ Aurora hiện tại chưa được cấu hình cho đăng nhập MCP.",mcp_oauth_changed:"Cấu hình hoặc phiên đăng nhập đã thay đổi. Tải lại cấu hình MCP trước khi tiếp tục.",mcp_oauth_invalid:"Thông tin đăng nhập không hợp lệ. Kiểm tra Client ID và thông tin cấp quyền."};accountStatus.textContent=messages[error.body?.error]||"Chưa xác nhận được thay đổi. Tải trạng thái đăng nhập để kiểm tra trước khi tiếp tục."}
      }finally{busy=false;if(alive()){reload.disabled=false;open.disabled=false;accountStatus.focus({preventScroll:true});accountStatus.scrollIntoView({block:"nearest"})}}
    }
    reload.onclick=()=>operate("status");
    async function preview(issuer="") {
      if(pending||busy||!alive())return;
      authorization=null;renderAccount();
      pending=true;open.disabled=true;result.querySelectorAll("button,select").forEach(n=>n.disabled=true);status.textContent="Đang kiểm tra thông tin cấp quyền…";
      try {
        const data=await api({workspaceId,name:server.name,url:server.url,issuer});
        if(!alive())return;
        if(data.resource!==server.url||!Array.isArray(data.issuers)||!data.issuers.length)throw Error("changed");
        authorization=data;renderAccount();
        result.replaceChildren();
        const heading=el("h4","Thông tin cấp quyền");heading.tabIndex=-1;result.append(heading);
        const list=el("dl");list.append(el("dt","Dịch vụ"),value("dd",data.resource));
        const scopes=Array.isArray(data.scopes)?data.scopes:[];
        list.append(el("dt","Quyền được yêu cầu"),scopes.length?value("dd",scopes.join(", ")):el("dd","Máy chủ chưa nêu quyền cụ thể."));result.append(list);
        if(data.authorization){
          list.append(el("dt","Máy chủ cấp quyền"),value("dd",data.authorization.issuer));
          result.append(el("p",data.authorization.pkceS256?"Máy chủ hỗ trợ bảo vệ phiên đăng nhập.":"Máy chủ chưa công bố phương thức đăng nhập được hỗ trợ.","caption"));
        }else{
          const label=el("label","Chọn máy chủ cấp quyền"),select=el("select");select.setAttribute("aria-label","Chọn máy chủ cấp quyền");
          for(const issuer of data.issuers){const option=value("option",issuer);option.value=issuer;select.append(option)}label.append(select);result.append(label);
          const choose=el("button","Xem máy chủ đã chọn","pill soft");choose.type="button";choose.onclick=()=>preview(select.value);result.append(choose);
        }
        status.textContent="Đã đọc thông tin yêu cầu. Chỉ xem thông tin chưa cấp thêm quyền.";heading.focus();
      }catch{
        if(alive()){result.replaceChildren();status.textContent="Chưa xác minh được thông tin cấp quyền. Kiểm tra địa chỉ HTTPS công khai của máy chủ rồi thử lại."}
      }finally{
        pending=false;if(alive()){open.disabled=false;result.querySelectorAll("button,select").forEach(n=>n.disabled=false)}
      }
    }
    open.onclick=()=>preview();return root;
  }
  globalThis.AURORA_MCP_AUTHORIZATION={create};
})();
