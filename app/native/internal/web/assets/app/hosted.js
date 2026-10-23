"use strict";
// Hosted sign-in (email code / magic link / Apple / Google) and one-button onboarding.
(()=>{
const $=s=>document.querySelector(s),t=s=>window.__I18N?window.__I18N.t(s):s;
const msg=(s,err)=>{const m=$("#msg");m.textContent=t(s);m.classList.toggle("err",!!err)};
const post=(u,b)=>fetch(u,{method:"POST",credentials:"same-origin",headers:{"Content-Type":"application/json"},body:JSON.stringify(b||{})}).then(async r=>({ok:r.ok,status:r.status,j:await r.json().catch(()=>({}))}));
const busy=(b,on)=>{b.disabled=on;b.setAttribute("aria-busy",on)};
const H={expired:"Liên kết đã hết hạn, gửi mã mới nhé.",failed:"Đăng nhập không thành công, thử lại.",full:"Máy chủ đã đủ người dùng."};
if($("#emailStep")){
 fetch("/app/auth/config").then(r=>r.json()).then(c=>{if(c.deployUrl)$("#deployBtn").href=c.deployUrl;if(c.apple||c.google)$("#oauth").classList.remove("hidden");if(c.apple)$("#apple").classList.remove("hidden");if(c.google)$("#google").classList.remove("hidden");if(!c.email)msg("Đăng nhập bằng email chưa được cấu hình trên máy chủ này.",1)}).catch(()=>{});
 fetch("/app/auth/me").then(r=>{if(r.ok)location.replace("/app/")});
 const h=location.hash.slice(1);if(H[h])msg(H[h],1);
 let email="";
 const send=async()=>{const b=$("#sendBtn");email=$("#email").value.trim();if(!/^\S+@\S+\.\S+$/.test(email)){msg("Email không hợp lệ",1);$("#email").focus();return}
  busy(b,true);const r=await post("/app/auth/email",{email}).catch(()=>null);busy(b,false);
  if(!r){msg("Không kết nối được máy chủ",1);return}
  if(r.status===429){msg("Bạn vừa yêu cầu mã, đợi 30 giây rồi thử lại.",1);return}
  if(!r.ok){msg(r.j.error==="email_signin_not_configured"?"Đăng nhập bằng email chưa được cấu hình trên máy chủ này.":"Không gửi được mã, thử lại.",1);return}
  $("#emailStep").classList.add("hidden");$("#codeStep").classList.remove("hidden");$("#sentTo").textContent=t("Mã đã gửi tới")+" "+email;msg("");$("#code").focus()};
 const verify=async()=>{const b=$("#verifyBtn"),code=$("#code").value.replace(/\D/g,"");if(code.length!==6){msg("Nhập đủ 6 số",1);return}
  busy(b,true);const r=await post("/app/auth/verify",{email,code}).catch(()=>null);busy(b,false);
  if(r&&r.ok){location.replace(r.j.next||"/app/");return}
  msg(r&&r.status===403?"Máy chủ đã đủ người dùng.":"Mã không đúng hoặc đã hết hạn",1);$("#code").select()};
 $("#sendBtn").onclick=send;$("#email").onkeydown=e=>{if(e.key==="Enter")send()};
 $("#verifyBtn").onclick=verify;$("#code").oninput=e=>{e.target.value=e.target.value.replace(/\D/g,"").slice(0,6);if(e.target.value.length===6)verify()};
 $("#backBtn").onclick=()=>{$("#codeStep").classList.add("hidden");$("#emailStep").classList.remove("hidden");msg("");$("#email").focus()};
}
if($("#startBtn")){
 let goal="work";
 fetch("/app/auth/me").then(r=>r.ok?r.json():Promise.reject(r)).then(me=>{if(me.onboarded){location.replace("/app/");return}if(me.name)$("#name").value=me.name;$("#plan").textContent=t("Gói")+" "+me.plan+" · "+me.quota.daily+" "+t("tin/ngày")}).catch(()=>location.replace("/app/signin.html"));
 document.querySelectorAll(".goal").forEach(g=>g.onclick=()=>{goal=g.dataset.goal;document.querySelectorAll(".goal").forEach(x=>x.setAttribute("aria-pressed",x===g))});
 $("#startBtn").onclick=async()=>{const b=$("#startBtn");busy(b,true);msg("Aurora đang thiết lập các bot cho bạn…");
  const r=await post("/app/api/onboard",{name:$("#name").value.trim(),goal}).catch(()=>null);
  if(r&&r.ok){location.replace("/app/#bot-meiiie");return}busy(b,false);msg("Chưa thiết lập được, thử lại.",1)};
}
})();
