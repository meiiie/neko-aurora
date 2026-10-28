"use strict";
(() => {
  const el=(tag,text,cls)=>{const n=document.createElement(tag);if(text!=null)n.textContent=text;if(cls)n.className=cls;return n};
  const value=(tag,text)=>{const n=el(tag,text);n.dataset.noi18n="";return n};
  function create({server,workspaceId,api,active}) {
    const root=el("section",null,"mcp-authorization"),open=el("button","Xem thông tin cấp quyền · "+server.name,"pill soft");
    open.type="button";
    const status=el("p",null,"caption");status.setAttribute("role","status");status.setAttribute("aria-live","polite");
    const result=el("div");root.append(open,status,result);let pending=false;
    const alive=()=>root.isConnected&&active();
    async function preview(issuer="") {
      if(pending||!alive())return;
      pending=true;open.disabled=true;result.querySelectorAll("button,select").forEach(n=>n.disabled=true);status.textContent="Đang kiểm tra thông tin cấp quyền…";
      try {
        const data=await api({workspaceId,name:server.name,url:server.url,issuer});
        if(!alive())return;
        if(data.resource!==server.url||!Array.isArray(data.issuers)||!data.issuers.length)throw Error("changed");
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
        status.textContent="Đã đọc thông tin yêu cầu. Tài khoản chưa được cấp quyền.";heading.focus();
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
