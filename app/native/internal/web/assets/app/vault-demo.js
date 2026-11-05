// Demo checkout for the Vault fill: checks the fields locally, sends nothing.
document.getElementById("pay").addEventListener("submit",function(e){e.preventDefault();var f=e.target,n=(f.cardnumber.value||"").replace(/\D/g,"");
const I=window.__I18N,out=document.getElementById("out"),present={name:!!f.ccname.value,expiry:!!f.exp.value,cvc:!!f.cvc.value};if(n.length>=12)I.bind(out,"vault.demoResult",()=>({digits:n.slice(-4),...Object.fromEntries(Object.entries(present).map(([k,v])=>[k,I.message(v?"vault.present":"vault.missing")]))}));else I.uiText(out,"Thiếu số thẻ.")});
