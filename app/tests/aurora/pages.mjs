// Every screen/state at 390 for before/after review. PH=before|after OUT=dir URL=server
import {chromium} from 'playwright-core';
const U=process.env.URL||'http://127.0.0.1:8812/app/',O=process.env.OUT||'/workspace/neko-bot-shots/aurora/pro/',PH=process.env.PH||'before';const sl=ms=>new Promise(r=>setTimeout(r,ms));
const b=await chromium.launch({executablePath:'/usr/bin/google-chrome',args:['--no-sandbox','--hide-scrollbars']});
const ctx=await b.newContext({viewport:{width:390,height:844},deviceScaleFactor:2,isMobile:true,hasTouch:true,locale:'vi-VN'});
const p=await ctx.newPage();const errs=[];p.on('pageerror',e=>errs.push(String(e)));
await p.addInitScript(()=>{try{localStorage.setItem('neko-aurora-lang','vi')}catch{}});
const S=async n=>{await sl(450);await p.screenshot({path:`${O}${PH}-${n}.png`})};
const tap=async s=>{const l=p.locator(s).first();if(await l.count()&&await l.isVisible()){await l.tap({force:true});await sl(250);return true}console.log('miss',s);return false};
const esc=async()=>{await p.keyboard.press('Escape');await sl(350)};
await p.goto(U);await p.waitForFunction(()=>window.__AURORA);await sl(900);await S('01-home');
await p.evaluate(()=>document.querySelector('#listScroll').scrollTop=9999);await S('02-home-bottom');
await p.evaluate(()=>document.querySelector('#listScroll').scrollTop=0);
await tap('#search');await p.keyboard.type('zzz');await S('03-search-empty');await p.fill('#search','');await p.locator('#search').blur();
await tap('#newBtn');await S('04-new-menu');if(await tap('#menuBot'))await S('05-new-bot-sheet');await esc();
await sl(500);await tap('#newBtn');await sl(500);if(await tap('#menuGroup'))await S('06-group-sheet');await esc();
await tap('#appsBtn');await S('07-connectors');await esc();
await tap('#meBtn');await S('08-settings');await p.evaluate(()=>{const s=document.querySelector('#settings .sheet-body,#settingsBody');s&&(s.scrollTop=9999)});await S('09-settings-bottom');await esc();
const id=await p.evaluate(()=>__AURORA.state().agents.find(a=>a.name==='Phân tích vàng'||a.name==='Thư ký Neko')?.id||__AURORA.state().agents[0].id);
await p.reload();await p.waitForFunction(()=>window.__AURORA);await sl(900);await p.locator(`[data-id="${id}"]`).first().tap();await sl(1500);await S('10-chat');
await p.evaluate(()=>{const m=document.querySelector('#messages');m.scrollTop=0});await S('11-chat-top');
await tap('#threadMore');await S('12-thread-menu');await esc();
await tap('#plusBtn');await S('13-plus-menu');await esc();
await p.fill('#input','Xin chào');await S('14-composer-typed');await p.fill('#input','');
await tap('#whoBtn');await S('15-profile');await esc();
await p.context().setOffline(true);await p.evaluate(()=>dispatchEvent(new Event('offline')));await S('16-offline');await p.context().setOffline(false);await p.evaluate(()=>dispatchEvent(new Event('online')));
for(const h of ['privacy','terms']){await p.goto(U+h+'.html');await S('17-'+h)}
console.log('errs',errs);await b.close();
