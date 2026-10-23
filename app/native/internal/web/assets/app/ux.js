// Pure view helpers for computer work in the chat (mobile UX review, run 9):
// command cards show only the bot's command, long output folds to a tail
// summary, downloads become file chips, hidden follow-up turns keep a status.
(function(g){
const CHECKPOINT="Việc còn dài — mình đã lưu tiến độ và làm tiếp ngay ở lượt sau.";
const NOISE=[/^Mình cần bạn duyệt lệnh này trước khi chạy trên máy tính của mình\.?$/,/^Đã chạy lệnh trên máy tính của mình\.?$/];
const PRE=[/^\s*\(set -o pipefail\) 2>\/dev\/null && set -o pipefail; set -e\n/,/^\s*cd (\/home\/bot|~|\$HOME)\/?\s*(&&|;)\s*/,/^\s*set -e(o pipefail| -o pipefail)?\s*(&&|;)\s*/,/^\s*set -o pipefail\s*(&&|;)\s*/,/^\s*set -eu?o pipefail\s*(&&|;)\s*/];
function cmdShown(body){let s=String(body||""),hid=false;for(let again=true;again;){again=false;for(const r of PRE){const t=s.replace(r,"");if(t!==s){s=t;hid=again=true}}}return{cmd:s.trim(),hidden:hid}}
function outSummary(text){const raw=String(text||"").replace(/^```\n?|\n?```$/g,"");let lines=raw.split("\n");
 const m=raw.match(/\[mã thoát (\d+), ([\d.]+)s\]/);const exit=m?+m[1]:null,secs=m?+m[2]:null;
 if(lines[0]&&lines[0].startsWith("$ "))lines=lines.slice(1);lines=lines.filter(l=>!/^\[mã thoát \d+/.test(l));while(lines.length&&!lines.at(-1).trim())lines.pop();
 let err="";if(exit)for(let i=lines.length-1;i>=0;i--){if(/^\s*[\w.]*(Error|Exception|error)\b.*:|^\s*(bash|sh): /.test(lines[i])){err=lines[i].trim();break}}
 return{exit,secs,err,count:lines.length,long:lines.length>8,tail:lines.slice(-4).join("\n"),full:lines.join("\n")}}
const TYPES={pptx:"Bản trình chiếu",ppt:"Bản trình chiếu",mp4:"Video",webm:"Video",mov:"Video",gif:"Ảnh động",png:"Ảnh",jpg:"Ảnh",jpeg:"Ảnh",webp:"Ảnh",pdf:"PDF",docx:"Tài liệu",xlsx:"Bảng tính",csv:"Bảng CSV",md:"Văn bản",txt:"Văn bản",py:"Mã Python",zip:"Tệp nén",json:"JSON",html:"Trang HTML"};
function fileInfo(href,sizes){const q=String(href).split("?")[1]||"";const p=new URLSearchParams(q).get("path")||"";const name=p.split("/").pop()||p;const ext=(name.match(/\.([a-z0-9]+)$/i)||[,""])[1].toLowerCase();
 const size=sizes&&sizes[p];return{path:p,name,ext,label:TYPES[ext]||"Tệp",size:size==null?"":kb(size)}}
function kb(n){return n<1024?n+" B":n<1<<20?Math.round(n/1024)+" KB":(n/(1<<20)).toFixed(1).replace(".",",")+" MB"}
const isDownload=href=>/\/computer\/download\?/.test(String(href||""));
const isCheckpoint=t=>String(t||"").trim().endsWith(CHECKPOINT);
const isNoise=(m)=>!!(m&&m.approval&&m.approval.kind==="computer"&&NOISE.some(r=>r.test(String(m.text||"").trim())));
// status for the open thread: running command, plan step, hidden follow-up
function statusText(a,L){L=L||[];if(L.some(m=>m.approval&&m.approval.state==="pending"))return"Chờ bạn duyệt";const sending=L.some(m=>m.approval&&m.approval.kind==="computer"&&m.approval.delivery&&m.approval.delivery.status==="sending");
 if(sending)return"Đang chạy lệnh…";if(!a||a.status!=="working")return"";
 if(a.snip&&/^Bước \d+\/\d+/.test(a.snip))return a.snip;
 const lastW=[...L].reverse().find(m=>m.role==="agent"&&m.tools&&m.tools.length);const run=lastW&&lastW.tools.find(t=>t.status==="running");
 if(run)return run.name==="computer_run"?"Đang chạy lệnh…":(run.summary||"Đang làm…");
 const lastU=[...L].reverse().find(m=>m.role==="user"),last=L.at(-1);
 if(last&&last.role!=="user"&&(last.approval&&last.approval.kind==="computer"&&last.approval.state==="approved"||isCheckpoint(last.text)))return"Đang làm tiếp…";
 const T=x=>x&&x.t?new Date(x.t).getTime():0;
 return lastW&&T(lastW)>=T(lastU)?"Đang tổng hợp":"Đang suy nghĩ"}
// foldCmd: a long command (inline heredoc) shows its first n lines; the rest folds
function foldCmd(cmd,n=6){const l=String(cmd||"").split("\n");return l.length<=n+2?{head:l.join("\n"),rest:"",more:0}:{head:l.slice(0,n).join("\n"),rest:l.slice(n).join("\n"),more:l.length-n}}
// mergeStacked: owner messages sent back to back with no reply between (a tapped
// choice, then a nudge) render as one bubble instead of a stack
function mergeStacked(L){const out=[];for(const m of L||[]){const p=out.at(-1);if(m.role==="user"&&p&&p.role==="user"&&!p.approval&&!m.approval){out[out.length-1]={...p,text:(p.text||"")+"\n"+(m.text||""),merged:(p.merged||1)+1,id:m.id,t:m.t}}else out.push(m)}return out}
const X={CHECKPOINT,cmdShown,foldCmd,mergeStacked,outSummary,fileInfo,kb,isDownload,isCheckpoint,isNoise,statusText};
g.AURORA_UX=X;if(typeof module!=="undefined")module.exports=X})(typeof window!=="undefined"?window:globalThis);
