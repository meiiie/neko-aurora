import http from 'node:http';
const R=[
[/vàng/i,'## Giá vàng sáng 23/10\n\nVàng thế giới **tăng 0,6%** lên quanh 2.735 USD/ounce nhờ đồng USD yếu đi [1]. Trong nước, SJC giữ chênh lệch cao so với thế giới [2].\n\n| Loại | Mua (tr/lượng) | Bán (tr/lượng) |\n|---|---|---|\n| SJC miếng | 139,3 | 142,3 |\n| Nhẫn 9999 | 136,3 | 140,3 |\n\n**Nhận định ngắn hạn:** xu hướng tăng vẫn giữ, nhưng nên chờ nhịp điều chỉnh trước khi mua thêm.'],
[/email|thư/i,'Mình đã soạn thư trả lời anh Minh về lịch họp thứ Năm, bạn xem rồi duyệt nhé.\nAPPROVAL: {"title":"Trả lời anh Minh","kind":"email","to":"minh@congty.vn","subject":"Re: Lịch họp thứ Năm","body":"Chào anh Minh,\\n\\nEm xác nhận họp lúc 14h thứ Năm tại phòng 3. Em sẽ gửi tài liệu trước buổi họp.\\n\\nCảm ơn anh."}'],
[/tiếng anh|phát âm/i,'Tuyệt! Hôm nay mình luyện âm **/θ/** nhé.\n\n1. Đặt đầu lưỡi giữa hai hàm răng.\n2. Thổi hơi nhẹ, không rung dây thanh.\n3. Thử với: *think, three, thank you*.\n\nBạn đọc to câu: **"I think three thin thieves thought."** rồi gửi mình bản ghi âm nhé.'],
[/kế hoạch|du lịch/i,'Mình lên lịch trình Đà Lạt 3 ngày cho bạn.\nPLAN: ["Chọn chỗ ở gần hồ Xuân Hương","Lên lịch tham quan từng ngày","Ước tính chi phí"]'],
[/bản tin sáng/,'{"text":"Chào buổi sáng Meiiie! Hôm nay có 3 việc đang chờ bạn.","items":["Duyệt thư trả lời anh Minh","Xem phân tích giá vàng mới","Lịch trình Đà Lạt đã sẵn sàng"]}'],
];
http.createServer((q,r)=>{let b='';q.on('data',c=>b+=c);q.on('end',()=>{const j=JSON.parse(b||'{}');const all=(j.messages||[]).map(m=>m.content).join('\n');const last=(j.messages||[]).at(-1)?.content||'';
 let reply=(R.find(([re])=>re.test(all.includes('bản tin sáng')?all:last))||[0,'Mình nhận rồi, đang xử lý cho bạn nhé.'])[1];
 if(/Thực hiện bước/.test(all)){const n=+([...all.matchAll(/Thực hiện bước (\d)/g)].at(-1)[1]);reply=['','Chọn **Dalat Wonder Resort**, cách hồ 5 phút đi bộ, khoảng 1,2 triệu/đêm.','Ngày 1: hồ Xuân Hương, chợ đêm. Ngày 2: Langbiang, đồi chè Cầu Đất. Ngày 3: Thiền viện Trúc Lâm.','Tổng chi phí ước tính **5,8 triệu/người** gồm xe, phòng và ăn uống.'][n]||'Đã xong bước này.'}
 r.writeHead(200,{'content-type':'text/event-stream'});for(const p of reply.split(/(?<= )/))r.write(`data: ${JSON.stringify({choices:[{delta:{content:p}}]})}\n\n`);r.end('data: [DONE]\n\n')})}).listen(8902);
