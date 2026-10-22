import {notLive} from './guard.mjs';
// Accessibility audit at 390: names, 44px targets, text contrast, reduced motion, across main screens.
import {chromium} from 'playwright-core';
const U=process.env.AURORA_URL||'http://127.0.0.1:8799/app/';const sl=ms=>new Promise(r=>setTimeout(r,ms));
notLive(U);
const b=await chromium.launch({executablePath:'/usr/bin/google-chrome',args:['--no-sandbox']});
const p=await (await b.newContext({viewport:{width:390,height:844},isMobile:true,hasTouch:true,locale:'vi-VN',reducedMotion:'reduce'})).newPage();
await p.addInitScript(t=>{try{localStorage.setItem('neko-aurora-lang','vi');if(t)localStorage.setItem('neko-aurora-theme',t)}catch{}},process.env.THEME||'');
const audit=name=>p.evaluate(name=>{const out=[];const lum=c=>{const m=c.match(/[\d.]+/g);if(!m)return null;let [r,g,b,a=1]=m.map(Number);if(c.startsWith("color(")){r*=255;g*=255;b*=255}const f=v=>{v/=255;return v<=.03928?v/12.92:((v+.055)/1.055)**2.4};return{L:.2126*f(r)+.7152*f(g)+.0722*f(b),a}};
 const bgOf=e=>{for(let x=e;x;x=x.parentElement){const c=getComputedStyle(x).backgroundColor;const l=lum(c);if(l&&l.a>.5)return l}return lum('rgb(255,255,255)')};
 const vis=e=>{const r=e.getBoundingClientRect();const s=getComputedStyle(e);return r.width>0&&r.height>0&&s.visibility!=='hidden'&&s.display!=='none'&&r.bottom>0&&r.top<innerHeight&&!e.closest('.hidden,[aria-hidden=true],.off')};
 for(const e of document.querySelectorAll('button,a[href],input,textarea,select,[role=button],[role=switch],[tabindex]')){if(!vis(e))continue;const nm=(e.getAttribute('aria-label')||e.textContent||e.title||e.placeholder||(e.labels&&e.labels[0]&&e.labels[0].textContent)||'').trim();const id=e.id||e.className||e.tagName;
  if(!nm)out.push(`${name} NONAME ${e.tagName}#${id}`);const r=e.getBoundingClientRect();if((r.width<44||r.height<44)&&!e.closest('.md,.sources')&&e.tagName!=='INPUT'&&e.tagName!=='TEXTAREA')out.push(`${name} SMALL ${Math.round(r.width)}x${Math.round(r.height)} ${e.tagName}#${id} "${nm.slice(0,20)}"`)}
 const seen=new Set();for(const e of document.querySelectorAll('body *')){if(!vis(e))continue;const t=[...e.childNodes].filter(n=>n.nodeType===3&&n.textContent.trim()).map(n=>n.textContent.trim()).join(' ');if(!t)continue;const s=getComputedStyle(e);const fg=lum(s.color),bg=bgOf(e);if(!fg||!bg)continue;const hi=Math.max(fg.L,bg.L),lo=Math.min(fg.L,bg.L);const cr=(hi+.05)/(lo+.05);const big=parseFloat(s.fontSize)>=24||(parseFloat(s.fontSize)>=18.6&&+s.fontWeight>=700);const need=big?3:4.5;const k=s.color+'|'+t.slice(0,15);if(cr<need&&!seen.has(k)&&!e.closest('[disabled],button:disabled')){seen.add(k);out.push(`${name} CONTRAST ${cr.toFixed(2)}<${need} "${t.slice(0,30)}" ${s.color} ${e.tagName}.${e.className}<${e.parentElement.className}`)}}
 return out},name);
let all=[];
await p.goto(U);await p.waitForFunction(()=>window.__AURORA&&__AURORA.state().agents.length>0);await sl(700);all.push(...await audit('list'));
const id=await p.evaluate(()=>__AURORA.state().agents.find(a=>a.kind!=='group').id);await p.evaluate(id=>location.hash=id,id);await p.reload();await p.waitForFunction(()=>window.__AURORA&&__AURORA.current());await sl(800);all.push(...await audit('thread'));
await p.goto(U);await p.waitForFunction(()=>window.__AURORA);await sl(500);await p.locator('#meBtn').click();await sl(600);all.push(...await audit('settings'));
await p.locator('#delAcctBtn').click();await sl(600);all.push(...await audit('delete'));
const rm=await p.evaluate(()=>[...document.styleSheets].some(s=>{try{return [...s.cssRules].some(r=>r.conditionText&&r.conditionText.includes('reduced-motion'))}catch{return false}}));
const anim=await p.evaluate(()=>[...document.querySelectorAll('*')].filter(e=>{const s=getComputedStyle(e);return parseFloat(s.transitionDuration)>0.05||(s.animationName!=='none'&&parseFloat(s.animationDuration)>0.05)}).length);
console.log(all.join('\n'));console.log('REDUCED_MOTION_RULE',rm,'ANIMATED_UNDER_REDUCE',anim,'ISSUES',all.length);await b.close();
