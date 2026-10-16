export async function lightCases({run,fixture,bots,clone,admit,evaluate,call,viewport,click,key,ax,screenshot,touch,until,delay,base,files}) {
  const originalBots=clone(bots);
  const reset=async(extra=[])=>{
    for(const h of fixture.held.splice(0)) h.json({error:'fixture-reset'},503);
    bots.splice(0,bots.length,...clone(originalBots),...clone(extra)); fixture.work=[];fixture.sequence=0;fixture.events=[];fixture.capabilities=[];fixture.messageMode='normal';fixture.requests=[];
    await evaluate(`localStorage.clear()`);const n=fixture.snapshotRequests;await call('Page.reload');
    if(!await until(()=>fixture.snapshotRequests>n&&evaluate(`state.bots.length===${bots.length}`)))throw new Error('Light fixture reset failed');
    await viewport(390,844);await delay(150);
  };
  const input=async(text)=>evaluate(`(()=>{const p=document.getElementById('prompt');p.value=${JSON.stringify(text)};p.focus();p.dispatchEvent(new Event('input',{bubbles:true}))})()`);
  const visible=async(selector)=>evaluate(`(()=>{const e=document.querySelector(${JSON.stringify(selector)}),r=e?.getBoundingClientRect();return !!r?.width&&!!r?.height&&!e.closest('[hidden]')})()`);
  const cohort=Array.from({length:12},(_,i)=>({id:'fixture-bot-'+i,name:'Fixture colleague '+i,alias:'fixture'+i,color:'#536563',revision:1}));

  await run('light_fixture_committed_fonts_loaded',async()=>{
    await evaluate(`document.fonts.load('16px Geist','Đồng nghiệp tiếng Việt').then(()=>document.fonts.ready)`);await call('DOM.enable');await call('CSS.enable');
    const doc=await call('DOM.getDocument'),node=await call('DOM.querySelector',{nodeId:doc.root.nodeId,selector:'#bot-name'});
    const fonts=await call('CSS.getPlatformFontsForNode',{nodeId:node.nodeId});const responses=[];
    for(const name of [...files.keys()].filter(x=>x.endsWith('.woff2'))){const r=await fetch(base+'/'+name);responses.push({name,status:r.status,type:r.headers.get('content-type'),bytes:(await r.arrayBuffer()).byteLength});}
    const status=await evaluate(`({loaded:document.fonts.check('16px Geist','Đồng nghiệp tiếng Việt'),family:getComputedStyle(document.body).fontFamily,faces:[...document.fonts].map(f=>({family:f.family,status:f.status}))})`);
    return {ok:status.loaded&&fonts.fonts.some(f=>f.isCustomFont)&&responses.length===3&&responses.every(r=>r.status===200&&r.type==='font/woff2'),evidence:{status,platformFonts:fonts.fonts,responses,scope:'Owned Node fixture routes/CSP only, not actual Go or Tauri font loading'}};
  });
  await run('light_drawer_search_preserves_bot_draft_and_focus',async()=>{
    await reset();await input('Fixture draft before search');await click('#nav-open');
    await evaluate(`document.getElementById('bot-search').value='DAv';document.getElementById('bot-search').focus();document.getElementById('bot-search').dispatchEvent(new Event('input',{bubbles:true}))`);
    const values=await evaluate(`({bot:document.getElementById('bot-name').textContent,draft:document.getElementById('prompt').value,focus:document.activeElement.id,results:[...document.querySelectorAll('#bots button')].map(b=>b.dataset.botId)})`);
    await key('Escape');return {ok:values.bot==='Meiiie'&&values.draft==='Fixture draft before search'&&values.focus==='bot-search'&&values.results.join()==='bot-davin',evidence:values};
  });
  await run('light_empty_search_drawer_reopen_initial_focus',async()=>{
    await reset();await click('#nav-open');await evaluate(`document.getElementById('bot-search').value='fixture-no-such-bot';document.getElementById('bot-search').focus();document.getElementById('bot-search').dispatchEvent(new Event('input',{bubbles:true}))`);
    await key('Escape');await click('#nav-open');const values=await evaluate(`({results:document.querySelectorAll('#bots button').length,mainInert:document.getElementById('main').inert,focus:document.activeElement.id,inside:document.getElementById('sidebar').contains(document.activeElement)})`);
    return {ok:values.results===0&&values.mainInert&&values.inside,evidence:values};
  });
  await run('light_search_touch44_product_goal',async()=>{
    await reset();await click('#nav-open');const values=await touch(['.bot-search','#create-bot','#mobile-create-bot','#runtime-open']);const name=await ax('#bot-search');
    return {ok:values.every(v=>v.width>=43.99&&v.height>=43.99)&&name.name.length>0,evidence:{values,ax:name,scope:'44x44 Neko goal; not by itself WCAG AA failure'}};
  });
  await run('light_lane_menu_keyboard_select_escape_focus',async()=>{
    await reset();await click('#composer-menu-open');const before=await evaluate(`({expanded:document.getElementById('composer-menu-open').getAttribute('aria-expanded'),focusLane:document.activeElement.value})`);
    await key('ArrowDown');const selected=await evaluate(`({lane:new FormData(document.getElementById('composer')).get('lane'),focus:document.activeElement.id,hidden:document.getElementById('composer-menu').hidden,indicator:document.getElementById('composer-menu-open').dataset.lane})`);
    await click('#composer-menu-open');await key('Escape');const closed=await evaluate(`({hidden:document.getElementById('composer-menu').hidden,focus:document.activeElement.id})`);
    return {ok:before.expanded==='true'&&before.focusLane==='foreground'&&selected.lane==='background'&&selected.indicator==='background'&&selected.hidden&&selected.focus==='prompt'&&closed.hidden&&closed.focus==='composer-menu-open',evidence:{before,selected,closed}};
  });
  await run('light_lane_menu_small_viewports_reachable',async()=>{
    await reset();const values=[];
    for(const [w,h]of [[320,320],[390,260],[390,844],[844,390]]){
      await viewport(w,h,w<=700);await click('#composer-menu-open');
      values.push(await evaluate(`(()=>{const e=document.getElementById('composer-menu'),r=e.getBoundingClientRect();return {width:innerWidth,height:innerHeight,left:r.left,right:r.right,top:r.top,bottom:r.bottom,documentWidth:document.documentElement.scrollWidth,targets:[...e.querySelectorAll('label')].map(x=>{const b=x.getBoundingClientRect();return {height:b.height,width:b.width,centerHit:x.contains(document.elementFromPoint(b.x+b.width/2,b.y+b.height/2))}})}})()`));
      await key('Escape');
    }
    return {ok:values.every(v=>v.top>=0&&v.bottom<=v.height&&v.left>=0&&v.right<=v.width&&v.documentWidth<=v.width&&v.targets.every(t=>t.centerHit)),evidence:values};
  });
  await run('light_mention_arrow_enter_select_without_submit',async()=>{
    await reset();await input('@');const list=await ax('#mention-picker');await key('ArrowDown');const selected=await evaluate(`document.getElementById('prompt').getAttribute('aria-activedescendant')`);await key('Enter');
    const values=await evaluate(`({draft:document.getElementById('prompt').value,focus:document.activeElement.id,hidden:document.getElementById('mention-picker').hidden,bot:document.getElementById('bot-name').textContent,active:document.getElementById('prompt').getAttribute('aria-activedescendant')})`);
    return {ok:list.role==='listbox'&&selected==='mention-bot-davin'&&values.draft==='@davin '&&values.hidden&&values.focus==='prompt'&&values.bot==='Meiiie'&&!values.active&&fixture.requests.length===0,evidence:{list,selected,values,calls:fixture.requests.length}};
  });
  await run('light_mention_escape_tab_composition_repeat',async()=>{
    await reset();await input('@d');await evaluate(`document.getElementById('prompt').dispatchEvent(new KeyboardEvent('keydown',{key:'Enter',bubbles:true,isComposing:true}));document.getElementById('prompt').dispatchEvent(new KeyboardEvent('keydown',{key:'Enter',bubbles:true,repeat:true}))`);
    const unchanged=await evaluate(`document.getElementById('prompt').value`);await key('Escape');const escape=await evaluate(`({draft:document.getElementById('prompt').value,hidden:document.getElementById('mention-picker').hidden})`);
    await input('@d');await key('Tab');const tab=await evaluate(`({draft:document.getElementById('prompt').value,focus:document.activeElement.id})`);
    return {ok:unchanged==='@d'&&escape.hidden&&escape.draft==='@d'&&tab.draft==='@davin '&&tab.focus==='prompt'&&fixture.requests.length===0,evidence:{unchanged,escape,tab,calls:fixture.requests.length,scope:'Synthetic composing/repeat guard only, no real phone IME'}};
  });
  await run('light_mention_reverse_tab_keeps_navigation_direction',async()=>{
    await reset();await input('@');await key('Tab',true);const values=await evaluate(`({draft:document.getElementById('prompt').value,focus:document.activeElement.id,hidden:document.getElementById('mention-picker').hidden})`);
    return {ok:values.focus==='composer-menu-open'&&values.draft==='@'&&values.hidden,evidence:{...values,expectation:'Reverse Tab should return to previous composer control without silently selecting alias'}};
  });
  await run('light_mention_keyboard_active_option_visible',async()=>{
    await reset(cohort);await input('@');for(let i=0;i<9;i++)await key('ArrowDown');
    const values=await evaluate(`(()=>{const p=document.getElementById('mention-picker'),pr=p.getBoundingClientRect(),id=document.getElementById('prompt').getAttribute('aria-activedescendant'),e=document.getElementById(id),r=e.getBoundingClientRect();return {id,selected:e.getAttribute('aria-selected'),top:r.top,bottom:r.bottom,pickerTop:pr.top,pickerBottom:pr.bottom,scroll:p.scrollTop,focus:document.activeElement.id}})()`);
    return {ok:values.top>=values.pickerTop&&values.bottom<=values.pickerBottom&&values.focus==='prompt',evidence:values};
  });
  await run('light_mention_short_viewport_option_reachable',async()=>{
    await reset(cohort);await viewport(390,260);await input('@');await screenshot('light-mention-short');
    const values=await evaluate(`(()=>{const p=document.getElementById('mention-picker'),r=p.getBoundingClientRect(),e=p.firstElementChild,b=e.getBoundingClientRect();return {top:r.top,bottom:r.bottom,firstTop:b.top,firstBottom:b.bottom,centerHit:e.contains(document.elementFromPoint(b.x+b.width/2,b.y+b.height/2)),height:innerHeight}})()`);
    return {ok:values.top>=0&&values.bottom<=values.height&&values.centerHit,evidence:values};
  });
  await run('light_normal_pending_feedback_exposed_to_at',async()=>{
    await reset();fixture.messageMode='hold';await input('Fixture pending status');await key('Enter');if(!await until(()=>fixture.held.length))throw new Error('Expected pending fixture POST');
    const status=await ax('#hint'),text=await evaluate(`document.getElementById('hint').textContent`);const item=fixture.held.shift();fixture.messageMode='normal';item.json(admit(item.input));await delay(200);
    return {ok:status.role==='status'&&!status.ignored,evidence:{ax:status,text,scope:'AX exposure; no actual AT speech'}};
  });
  await run('light_mobile_reply_action_visible_keyboard_focus',async()=>{
    await reset();const w={requestId:'fixture-reply',botId:'bot-meiiie',sourceBotId:'bot-meiiie',sourceMessageId:'fixture-message',clientNonce:'fixture-reply-nonce',text:'Fixture completed question',reply:'Fixture Vietnamese answer: Đồng nghiệp cùng làm.',lane:'foreground',status:'completed',desiredState:'running',epoch:1,createdSequence:1,createdAt:'2026-10-11T00:00:00Z'};fixture.work=[w];await evaluate('sync()');
    const values=await touch(['.reply-actions .text-button']);await evaluate(`document.querySelector('.reply-actions .text-button').focus()`);const style=await evaluate(`({opacity:getComputedStyle(document.querySelector('.reply-actions')).opacity,focus:document.activeElement.textContent})`);const name=await ax('.reply-actions .text-button');
    return {ok:values.length===1&&values[0].height>=44&&values[0].width>=44&&style.opacity==='1'&&name.name==='Sao chép',evidence:{values,style,ax:name}};
  });
  await run('light_reduced_motion_composer_and_working_avatar',async()=>{
    await reset();await call('Emulation.setEmulatedMedia',{features:[{name:'prefers-reduced-motion',value:'reduce'}]});
    const w={requestId:'fixture-motion',botId:'bot-meiiie',text:'Fixture working',lane:'background',status:'running',epoch:1,createdSequence:1,createdAt:'2026-10-11T00:00:00Z'};fixture.work=[w];await evaluate('sync()');await input('Fixture many lines\nline two\nline three\nline four');
    const values=await evaluate(`({reduce:matchMedia('(prefers-reduced-motion:reduce)').matches,avatar:getComputedStyle(document.querySelector('#bots .avatar')).animationName,animations:document.getAnimations().map(a=>a.effect?.getTiming().duration),composerTransition:getComputedStyle(document.getElementById('composer')).transitionDuration})`);
    await call('Emulation.setEmulatedMedia',{features:[]});return {ok:values.reduce&&values.avatar==='none'&&values.animations.length===0,evidence:values};
  });
  await reset();await viewport(1280,720);await screenshot('light-empty-desktop');await viewport(390,844);await screenshot('light-empty-mobile');
}
