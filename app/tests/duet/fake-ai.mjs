import http from 'node:http';
http.createServer((q,r)=>{let b='';q.on('data',c=>b+=c);q.on('end',()=>{
 if(q.headers.authorization!=='Bearer fake-test-key'){r.writeHead(401);return r.end('{}')}
 const j=JSON.parse(b||'{}');const last=(j.messages||[]).at(-1)?.content||'';
 let reply;if(j.messages?.[0]?.content.includes('bản tin sáng'))reply='{"text":"Chào Darren, hôm nay có vài việc cần bạn.","items":["Duyệt bài đăng của Posting Buddy","Xem thư của Inbox Zero"]}';
 else if(/Thực hiện bước/.test(last))reply=/2\/2/.test(last)?'Bước cuối xong. Bạn muốn giọng nào?\nCHOICES: ["Thân mật","Trang trọng"]':'Đã xong bước này.';
 else if(/chậm/.test(last))reply=Array(200).fill('chữ').join(' ');
 else if(/kế hoạch/i.test(last))reply='Mình sẽ làm theo 2 bước.\nPLAN: ["Gom ý chính","Viết bản nháp"]';
 else if(/gửi|đăng/i.test(last))reply='Mình đã soạn xong bản nháp theo yêu cầu.\nAPPROVAL: {"title":"Gửi thư cho Elena","kind":"send","body":"Chào Elena, đây là deck cho thứ Năm."}';
 else reply='Mình nhận việc rồi, đây là câu trả lời thử nghiệm được stream từng phần.';
 r.writeHead(200,{'content-type':'text/event-stream'});const parts=reply.split(/(?<= )/);let i=0;
 const t=setInterval(()=>{if(r.destroyed){clearInterval(t);return}if(i<parts.length){r.write(`data: ${JSON.stringify({choices:[{delta:{content:parts[i++]}}]})}\n\n`)}else{r.write('data: [DONE]\n\n');r.end();clearInterval(t)}},30)})}).listen(8901,()=>console.log('fake ai on 8901'));
