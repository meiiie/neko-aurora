// md.js unit tests in a real browser (no server needed).
import {chromium} from 'playwright-core';import fs from 'node:fs';
const src=fs.readFileSync(new URL('../../native/internal/web/assets/app/md.js',import.meta.url),'utf8');
const b=await chromium.launch({executablePath:'/usr/bin/google-chrome',args:['--no-sandbox']});const p=await b.newPage();await p.setContent('<div id=o></div>');await p.addScriptTag({content:src});
let fail=0;const R=(t,opt)=>p.evaluate(([t,o])=>{const n=document.getElementById('o');AURORA_MD.render(n,t,[{url:'https://a.vn/x'},{url:'https://b.vn'}],o);return n.innerHTML},[t,opt||{}]);
const ok=(c,m,x)=>{console.log(c?'PASS':'FAIL',m);if(!c){fail++;console.log('   ',x)}};
let h;
h=await R('1. Một\n   - con A\n   - con B\n2. Hai');ok(/<ol><li><div class="li-b">Một<ul><li><div class="li-b">con A/.test(h)&&/<\/ul><\/div><\/li><li><div class="li-b">Hai/.test(h),'nested list under ordered',h);
h=await R('- [x] Xong\n- [ ] Chưa');ok(h.includes('tick on')&&h.includes('aria-label="Chưa xong"')&&!h.includes('[x]'),'task list',h);
h=await R('```bash\nnpx cap sync\n  indented\n```\nsau');ok(h.includes('class="code-lang">bash')&&h.includes('npx cap sync\n  indented')&&h.includes('<p>sau</p>'),'fenced code + lang + copy',h);
h=await R('```js\nlet a=1',{streaming:true});ok(h.includes('let a=1')&&h.includes('<pre'),'unclosed fence while streaming',h);
h=await R('Đang **tổng hợp',{streaming:true});ok(!h.includes('*')&&h.includes('Đang tổng hợp'),'dangling bold hidden while streaming',h);
h=await R('| A | B |\n|:--|--:|\n| 1 | 2 |');ok(h.includes('role="region"')&&h.includes('text-align: right')&&h.includes('<td>1</td>'),'aligned scrollable table',h);
h=await R('> trích\n> > lồng\n\nsau');ok(/<blockquote><p>trích<\/p><blockquote><p>lồng/.test(h),'nested blockquote',h);
h=await R('## Tiêu đề\n#### nhỏ\n---\n`a*b*c` ~~bỏ~~ \\*sao\\* [t](https://x.vn) giá [1][2] [9]');ok(h.includes('<h3>Tiêu đề')&&h.includes('<h5>nhỏ')&&h.includes('<hr>')&&h.includes('<code>a*b*c</code>')&&h.includes('<s>bỏ</s>')&&h.includes('*sao*')&&h.includes('href="https://x.vn"')&&(h.match(/class="cite"/g)||[]).length===2&&!h.includes('[9]'),'inline set',h);
h=await R('<img src=x onerror=alert(1)> **b**');ok(!h.includes('<img')&&h.includes('&lt;img'),'no html injection',h);
h=await R('Dòng một\nDòng hai\n\n- a\n\n- b');ok(h.includes('Dòng một<br>Dòng hai')&&h.includes('class="loose"'),'soft breaks + loose list',h);
h=await R('snake_case_name và _nghiêng_');ok(h.includes('snake_case_name')&&h.includes('<em>nghiêng</em>'),'intraword underscore',h);
await b.close();console.log(fail?'FAILS '+fail:'FAILS 0');process.exit(fail?1:0);
