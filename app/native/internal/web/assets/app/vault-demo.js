// Demo checkout for the Vault fill: checks the fields locally, sends nothing.
document.getElementById("pay").addEventListener("submit",function(e){e.preventDefault();var f=e.target,n=(f.cardnumber.value||"").replace(/\D/g,"");
document.getElementById("out").textContent=n.length>=12?"Đã nhận (demo, không trừ tiền): thẻ đuôi "+n.slice(-4)+", tên "+(f.ccname.value?"có":"thiếu")+", hạn "+(f.exp.value?"có":"thiếu")+", CVC "+(f.cvc.value?"có":"thiếu")+".":"Thiếu số thẻ."});
