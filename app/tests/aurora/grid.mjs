// 4pt-grid audit: padding/margin/gap/height values of visible UI that are not multiples of 4.
import {chromium} from 'playwright-core';const sl=ms=>new Promise(r=>setTimeout(r,ms));
const b=await chromium.launch({executablePath:'/usr/bin/google-chrome',args:['--no-sandbox']});const p=await (await b.newContext({viewport:{width:390,height:844},isMobile:true,hasTouch:true,locale:'vi-VN'})).newPage();
await p.goto(process.env.U||'http://127.0.0.1:8812/app/');await p.waitForFunction(()=>window.__AURORA&&__AURORA.state().agents.length>2);await sl(600);
const run=n=>p.evaluate(n=>{const out={};for(const e of document.querySelectorAll('body *')){const r=e.getBoundingClientRect();if(!r.width||e.closest('.hidden,.off,svg,.md'))continue;const s=getComputedStyle(e);
 for(const k of ['paddingTop','paddingBottom','paddingLeft','paddingRight','marginTop','marginBottom','rowGap','columnGap']){const v=parseFloat(s[k]);if(v&&v%4&&v>1){const key=(e.id?'#'+e.id:'.'+String(e.className).split(' ')[0])+' '+k+'='+v;out[key]=1}}}
 return Object.keys(out).map(k=>n+' '+k)},n);
let all=[...await run('list')];const id=await p.evaluate(()=>__AURORA.state().agents.find(a=>a.name==='Ra mắt iOS').id);await p.evaluate(id=>location.hash=id,id);await p.reload();await p.waitForFunction(()=>window.__AURORA&&__AURORA.current());await sl(800);all.push(...await run('thread'));
console.log(all.join('\n'));console.log('OFFGRID',all.length);await b.close();
