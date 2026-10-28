"use strict";
(() => {
  let query=location.search,fragment=location.hash;
  history.replaceState(null,"",location.pathname);
  let input=null;
  try {
    if(query.length>8192||fragment)throw Error();
    const params=new URLSearchParams(query),seen=new Set();
    for(const [key] of params){if(seen.has(key))throw Error();seen.add(key)}
    const state=params.get("state"),code=params.get("code"),error=params.get("error"),issuer=params.get("iss");
    if(!/^[a-f0-9]{64}\.[A-Za-z0-9_-]{43}$/.test(state||"")||Boolean(code)===Boolean(error)||(code&&code.length>4096)||(error&&error.length>128)||(issuer&&issuer.length>2048))throw Error();
    input={state};if(code)input.code=code;if(error)input.error=error;if(issuer!==null)input.iss=issuer;
  }catch{}
  query=fragment="";
  document.addEventListener("DOMContentLoaded",async()=>{
    const status=document.getElementById("oauth-result");
    if(!input){status.textContent="Liên kết đăng nhập không hợp lệ hoặc đã được mở trước đó. Hãy quay lại cài đặt MCP để kiểm tra trạng thái.";return}
    try{
      const body=JSON.stringify(input);input=null;
      const response=await fetch("/app/api/mcp/oauth/callback",{method:"POST",credentials:"same-origin",cache:"no-store",redirect:"error",headers:{"Content-Type":"application/json"},body});
      if(response.status===401){status.textContent="Phiên Aurora chưa đăng nhập. Hãy quay về Aurora, đăng nhập rồi bắt đầu kết nối MCP mới.";return}
      if(!response.ok)throw Error();
      const result=await response.json();
      status.textContent=result.phase==="connected"?"Đã kết nối MCP. Quay về cài đặt và tải lại trạng thái để sử dụng.":result.phase==="denied"?"Bạn đã từ chối cấp quyền. Tài khoản chưa được kết nối.":"Cần kiểm tra trạng thái kết nối trong cài đặt MCP.";
    }catch{status.textContent="Chưa xác nhận được kết quả đăng nhập. Kiểm tra trạng thái trong cài đặt MCP; không gửi lại mã đăng nhập từ trang này."}
  },{once:true});
})();
