// ux.js unit tests (node, no server): node ux-test.mjs
import {createRequire} from 'node:module';const require=createRequire(import.meta.url);
const U=require('../../native/internal/web/assets/app/ux.js');let fail=0;
const ok=(c,m,x)=>{console.log(c?'PASS':'FAIL',m);if(!c){fail++;console.log('   ',JSON.stringify(x))}};
let r=U.cmdShown("cd /home/bot && set -e -o pipefail && mkdir -p proj/out && python3 proj/build_deck.py && ls -la proj/out");
ok(r.cmd==="mkdir -p proj/out && python3 proj/build_deck.py && ls -la proj/out"&&r.hidden,'boilerplate hidden',r);
r=U.cmdShown("(set -o pipefail) 2>/dev/null && set -o pipefail; set -e\npython3 a.py");ok(r.cmd==="python3 a.py",'strict prefix hidden',r);
ok(U.cmdShown("python3 a.py").hidden===false,'plain command untouched');
const tb="```\n$ python3 make_video.py\nTraceback (most recent call last):\n  File \"/home/bot/make_video.py\", line 80, in <module>\n    frame = slide_image(slide)\n  File \"x\", line 59\n    font = fb(sz)\n  File \"x\", line 14, in fb\n    return ImageFont.truetype(FONTB, sz)\n  File \"PIL\", line 1\nOSError: cannot open resource\n\n[mã thoát 1, 3.3s]\n```";
r=U.outSummary(tb);ok(r.exit===1&&r.secs===3.3&&r.err==="OSError: cannot open resource"&&r.long&&r.tail.split("\n").length===4&&!r.full.includes("$ python3"),'traceback folded to tail + error line',r);
r=U.outSummary("```\n$ ls\na\nb\n\n[mã thoát 0, 0.1s]\n```");ok(r.exit===0&&!r.long&&r.full==="a\nb"&&r.err==="",'short output shown whole',r);
r=U.fileInfo("/app/api/agents/b/computer/download?path=proj/out/Hg_to_Au.pptx",{"proj/out/Hg_to_Au.pptx":51802});
ok(r.name==="Hg_to_Au.pptx"&&r.ext==="pptx"&&r.label==="Bản trình chiếu"&&r.size==="51 KB",'file chip info',r);
ok(U.fileInfo("/x/computer/download?path=out/preview.mp4",{"out/preview.mp4":3.4*(1<<20)}).size==="3,4 MB",'MB size');
ok(U.isDownload("/app/api/agents/b/computer/download?path=a")&&!U.isDownload("https://x.vn"),'download link detection');
const ap=(st,dv)=>({role:"agent",text:"Đã chạy lệnh trên máy tính của mình.",approval:{kind:"computer",state:st,delivery:dv}});
ok(U.isNoise(ap("approved"))&&!U.isNoise({role:"agent",text:"Đã chạy lệnh trên máy tính của mình."}),'post-approval duplicate line hidden only on cards');
const t=new Date().toISOString(),w={status:"working"};
ok(U.statusText({status:"idle"},[ap("approved",{status:"sending"})])==="Đang chạy lệnh…",'running command shows status even when idle');
ok(U.statusText(w,[{role:"user",t},ap("approved",{status:"sent"})])==="Đang làm tiếp…",'hidden follow-up turn shows Đang làm tiếp');
ok(U.statusText(w,[{role:"user",t},{role:"agent",text:"Xong phần 1.\n\n"+U.CHECKPOINT}])==="Đang làm tiếp…",'checkpoint continuation status');
ok(U.statusText({status:"working",snip:"Bước 2/4 · Tra giá"},[])==="Bước 2/4 · Tra giá",'plan step');
ok(U.statusText({status:"idle"},[{role:"user",t}])==="",'idle: no status');
ok(U.isCheckpoint("abc "+U.CHECKPOINT)&&!U.isCheckpoint("abc"),'checkpoint detection');
ok(U.statusText(w,[{role:"user",t},{role:"agent",approval:{kind:"email",state:"pending"}}])==="Chờ bạn duyệt",'pending card: Chờ bạn duyệt');
ok(U.statusText({status:"idle"},[{role:"agent",approval:{kind:"computer",state:"pending"}},ap("approved",{status:"sending"})])==="Chờ bạn duyệt",'waiting beats running');
{const long=Array.from({length:30},(_, i)=>"line"+i).join("\n");const f=U.foldCmd(long);ok(f.head.split("\n").length===6&&f.more===24&&f.rest.startsWith("line6"),'long command folds after 6 lines',f);
 ok(U.foldCmd("a\nb\nc").more===0,'short command whole');}
{const m=U.mergeStacked([{role:"user",text:"Làm tiếp",id:1},{role:"user",text:"nhanh nhé",id:2},{role:"agent",text:"ok"},{role:"user",text:"x"}]);ok(m.length===3&&m[0].text==="Làm tiếp\nnhanh nhé"&&m[0].merged===2&&m[0].id===2,'stacked owner bubbles merge',m)}
process.exit(fail?1:0);
