// Neko Aurora markdown → DOM (never innerHTML). A small CommonMark-flavoured
// block parser written for chat: fenced code with copy, nested and task lists,
// aligned scrollable tables, nested blockquotes, headings, rules, links,
// inline code, emphasis, strikethrough and [n] citations. Streaming-safe:
// an unclosed fence renders as code so far and dangling ** / ` are hidden.
// Block structure follows the CommonMark spec (CC-BY-SA 4.0 text, rules only;
// no code copied from markdown-it/micromark).
(()=>{
const el=(t,c,x)=>{const e=document.createElement(t);if(c)e.className=c;if(x!=null)e.textContent=x;return e};
const host=u=>{try{return new URL(u).hostname.replace(/^www\./,"")}catch{return u}};
const INL=/(\\[\\`*_~\[\]()#+\-.!>|]|`+[^`\n]*?`+|\*\*(?=\S)[^\n]*?\S\*\*|__(?=\S)[^\n]*?\S__|~~(?=\S)[^\n]*?\S~~|\[[^\]\n]+\]\((?:https?:\/\/|mailto:|\/app\/api\/agents\/[\w-]+\/computer\/download\?path=)[^\s)]+\)|https?:\/\/[^\s<>()]+[^\s<>().,;:!?'"]|\[\d{1,2}\](?:\s*,?\s*\[\d{1,2}\])*|(?<![\w*])\*(?=\S)[^*\n]*?\S\*(?![\w*])|(?<![\p{L}\p{N}_])_(?=\S)[^_\n]*?\S_(?![\p{L}\p{N}_]))/gu;
function link(t,u){const a=el("a","lnk",t);a.href=u;a.target="_blank";a.rel="noopener noreferrer";return a}
function inline(node,text,src){let last=0,m;INL.lastIndex=0;const re=new RegExp(INL.source,"gu");
 while((m=re.exec(text))){if(m.index>last)node.append(text.slice(last,m.index));const t=m[0];
  if(t[0]==="\\")node.append(t[1]);
  else if(t[0]==="`"){const n=t.match(/^`+/)[0].length;node.append(el("code",0,t.slice(n,-n).replace(/^ (.+) $/,"$1")))}
  else if(t.startsWith("**")||t.startsWith("__")){const b=el("strong");inline(b,t.slice(2,-2),src);node.append(b)}
  else if(t.startsWith("~~")){const s=el("s");inline(s,t.slice(2,-2),src);node.append(s)}
  else if(/^\[\d/.test(t)&&!t.includes("](")){t.match(/\d{1,2}/g).forEach(n=>{const x=src&&src[+n-1];if(!x)return;const c=link(n,x.url);c.className="cite";c.setAttribute("aria-label","Nguồn "+n+": "+host(x.url));node.append(c)})}
  else if(t[0]==="["){const k=t.indexOf("](");const a=link("",t.slice(k+2,-1));inline(a,t.slice(1,k),src);node.append(a)}
  else if(t.startsWith("http"))node.append(link(host(t)+(new URL(t).pathname.length>1?"/…":""),t))
  else{const e=el("em");inline(e,t.slice(1,-1),src);node.append(e)}
  last=m.index+t.length}
 if(last<text.length)node.append(text.slice(last))}
// streaming: hide a dangling opener on the last line so "**Đang" never shows asterisks
function settle(t){const ls=t.split("\n");let l=ls[ls.length-1];if(/^\s*(```|~~~)/.test(l))return t;
 if(((l.match(/\*\*/g)||[]).length)%2)l=l.replace(/\*\*(?!.*\*\*)/,"");if(((l.match(/`/g)||[]).length)%2)l=l.replace(/`(?!.*`)/,"");ls[ls.length-1]=l;return ls.join("\n")}
const FENCE=/^(\s{0,3})(`{3,}|~{3,})\s*([\w+#.-]*)/,HEAD=/^\s{0,3}(#{1,6})\s+(.*?)\s*#*\s*$/,HR=/^\s{0,3}([-*_])(\s*\1){2,}\s*$/,
 ITEM=/^(\s*)([-*+•]|\d{1,3}[.)])\s+(.*)$/,QUOTE=/^\s{0,3}>\s?/,ROW=/^\s*\|?.*\|.*$/,DELIM=/^\s*\|?\s*:?-{2,}:?\s*(\|\s*:?-{2,}:?\s*)*\|?\s*$/;
const cells=l=>{let s=l.trim();if(s.startsWith("|"))s=s.slice(1);if(s.endsWith("|")&&!s.endsWith("\\|"))s=s.slice(0,-1);return s.split(/(?<!\\)\|/).map(c=>c.trim().replace(/\\\|/g,"|"))};
const startsBlock=(l,n)=>FENCE.test(l)||HEAD.test(l)||HR.test(l)||QUOTE.test(l)||ITEM.test(l)||(ROW.test(l)&&n!=null&&DELIM.test(n));
const indentOf=l=>l.match(/^\s*/)[0].replace(/\t/g,"    ").length;
function code(node,lang,body){const w=el("div","code");const hd=el("div","code-h");hd.append(el("span","code-lang",lang||"code"));
 const cp=el("button","code-copy","Sao chép");cp.type="button";cp.setAttribute("aria-label","Sao chép mã");cp.onclick=()=>{(navigator.clipboard?navigator.clipboard.writeText(body):Promise.reject()).then(()=>{cp.textContent="Đã chép";setTimeout(()=>cp.textContent="Sao chép",1400);document.dispatchEvent(new CustomEvent("aurora-copied"))}).catch(()=>{})};
 hd.append(cp);const pre=el("pre");pre.tabIndex=0;pre.append(el("code",lang?"lang-"+lang:0,body));w.append(hd,pre);node.append(w)}
function tableCard(node,wrap,table,markdown){
 const card=el("div","table-card"),bar=el("div","table-actions"),status=el("span","table-status");status.setAttribute("role","status");
 const copy=(title,body)=>{const b=el("button","pill soft",title);b.type="button";b.onclick=async()=>{try{await navigator.clipboard.writeText(body());status.textContent="Đã sao chép"}catch{status.textContent="Không sao chép được. Hãy chọn văn bản trong bảng."}};return b};
 bar.append(copy("Sao chép bảng",()=>Array.from(table.rows,row=>Array.from(row.cells,c=>c.textContent.replace(/\t|\n/g," ")).join("\t")).join("\n")),copy("Sao chép Markdown",()=>markdown));
 const expand=el("button","pill soft","Mở rộng bảng");expand.type="button";expand.onclick=()=>{
  const dialog=el("dialog","table-viewer"),head=el("div","table-actions"),body=el("div","table-stage md"),clone=table.cloneNode(true);
  dialog.setAttribute("aria-label","Bảng mở rộng");body.tabIndex=0;body.setAttribute("role","region");body.setAttribute("aria-label","Nội dung bảng, cuộn để xem");
  const zoom=el("button","pill soft","100%"),close=el("button","pill","Đóng bảng");zoom.type=close.type="button";zoom.setAttribute("aria-label","Phóng to bảng");
  let scale=1;zoom.onclick=()=>{scale=scale>=2?1:scale+.25;body.style.setProperty("--table-scale",scale);zoom.textContent=Math.round(scale*100)+"%"};
  close.onclick=()=>dialog.close();dialog.onclose=()=>{dialog.remove();if(expand.isConnected)expand.focus()};dialog.onclick=e=>{if(e.target===dialog)dialog.close()};
  head.append(el("strong",0,"Bảng"),zoom,close);body.append(clone);dialog.append(head,body);document.body.append(dialog);dialog.showModal();close.focus();
 };
 bar.append(expand,status);card.append(bar,wrap);node.append(card);
}
function blocks(node,lines,src){let i=0;
 while(i<lines.length){const l=lines[i];
  if(!l.trim()){i++;continue}
  let m=l.match(FENCE);if(m){const f=m[2],body=[];i++;while(i<lines.length&&!new RegExp("^\\s{0,3}"+f[0]+"{"+f.length+",}\\s*$").test(lines[i]))body.push(lines[i++].replace(new RegExp("^ {0,"+m[1].length+"}"),""));i++;code(node,m[3],body.join("\n"));continue}
  if((m=l.match(HEAD))){const lv=m[1].length,e=el(lv<=2?"h3":lv===3?"h4":"h5");inline(e,m[2],src);node.append(e);i++;continue}
  if(HR.test(l)){node.append(el("hr"));i++;continue}
  if(ROW.test(l)&&i+1<lines.length&&DELIM.test(lines[i+1])){const start=i,al=cells(lines[i+1]).map(c=>c.startsWith(":")&&c.endsWith(":")?"center":c.endsWith(":")?"right":"");
   const wrap=el("div","tbl"),t=el("table"),th=el("thead"),tr=el("tr");wrap.tabIndex=0;wrap.setAttribute("role","region");wrap.setAttribute("aria-label","Bảng, vuốt ngang để xem");
   const cell=(tag,c,k)=>{const x=el(tag);if(al[k])x.style.textAlign=al[k];inline(x,c,src);return x};
   cells(l).forEach((c,k)=>tr.append(cell("th",c,k)));th.append(tr);t.append(th);const tb=el("tbody");i+=2;
   while(i<lines.length&&lines[i].trim()&&ROW.test(lines[i])){const r=el("tr");cells(lines[i]).forEach((c,k)=>r.append(cell("td",c,k)));tb.append(r);i++}t.append(tb);wrap.append(t);tableCard(node,wrap,t,lines.slice(start,i).join("\n"));continue}
  if(QUOTE.test(l)){const buf=[];while(i<lines.length&&lines[i].trim()&&(QUOTE.test(lines[i])||(buf.length&&!startsBlock(lines[i],lines[i+1]))))buf.push(lines[i++].replace(QUOTE,""));const q=el("blockquote");blocks(q,buf,src);node.append(q);continue}
  if((m=l.match(ITEM))){const base=indentOf(l),ol=/\d/.test(m[2]),list=el(ol?"ol":"ul");if(ol&&parseInt(m[2])!==1)list.start=parseInt(m[2]);let loose=false;
   while(i<lines.length&&(m=lines[i].match(ITEM))&&indentOf(lines[i])<=base+1&&/\d/.test(m[2])===ol){const off=indentOf(lines[i])+m[2].length+1,body=[m[3]];i++;
    while(i<lines.length){const n=lines[i];if(!n.trim()){if(i+1<lines.length&&lines[i+1].trim()&&indentOf(lines[i+1])>base&&!(ITEM.test(lines[i+1])&&indentOf(lines[i+1])<=base+1)){body.push("");i++;loose=true;continue}break}
     if(indentOf(n)>base&&!(ITEM.test(n)&&indentOf(n)<=base+1)){body.push(n.replace(new RegExp("^\\s{0,"+Math.max(off,base+2)+"}"),""));i++;continue}
     if(!startsBlock(n,lines[i+1])&&indentOf(n)<=base&&body.length&&body[body.length-1].trim()&&!ITEM.test(n)){body.push(n.trim());i++;continue}break}
    if(i<lines.length&&!lines[i].trim()&&i+1<lines.length&&(m=lines[i+1].match(ITEM))&&indentOf(lines[i+1])<=base+1){loose=true;i++}
    const it=el("li");const tk=body[0].match(/^\[([ xX])\]\s+(.*)$/);if(tk){it.className="task";const b=el("span","tick"+(tk[1]===" "?"":" on"));b.setAttribute("role","img");b.setAttribute("aria-label",tk[1]===" "?"Chưa xong":"Đã xong");it.append(b);body[0]=tk[2]}
    const inner=el("div","li-b");blocks(inner,body,src);if(!loose&&inner.firstChild&&inner.firstChild.tagName==="P"){const p=inner.firstChild;while(p.firstChild)inner.insertBefore(p.firstChild,p);p.remove()}it.append(inner);list.append(it)}
   if(loose)list.classList.add("loose");node.append(list);continue}
  const p=el("p"),buf=[l.trim()];i++;while(i<lines.length&&lines[i].trim()&&!startsBlock(lines[i],lines[i+1]))buf.push(lines[i++].trim());
  buf.forEach((b,k)=>{if(k)p.append(el("br"));inline(p,b.replace(/\s{2,}$/,""),src)});node.append(p)}}
function render(node,text,src,opt){node.classList.add("md");node.replaceChildren();text=String(text||"").replace(/\r\n?/g,"\n");if(opt&&opt.streaming)text=settle(text);blocks(node,text.split("\n"),src)}
self.AURORA_MD={render,inline};
})();
