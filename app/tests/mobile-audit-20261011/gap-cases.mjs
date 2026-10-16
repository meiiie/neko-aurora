export async function gapCases({run,fixture,bots,clone,push,evaluate,call,viewport,click,key,screenshot,until,delay}) {
  const originalBots=clone(bots);
  const cohort=Array.from({length:12},(_,i)=>({id:'gap-bot-'+i,name:'Gap colleague '+i,alias:'gap'+i,color:'#536563',revision:1}));
  const reset=async(extra=[],work=[])=>{
    for(const h of fixture.cancelHeld.splice(0))h.json({error:'fixture-reset'},503);
    bots.splice(0,bots.length,...clone(originalBots),...clone(extra));
    fixture.work=clone(work);fixture.sequence=0;fixture.events=[];fixture.capabilities=[];
    fixture.cancelMode='normal';fixture.cancelRequests=[];fixture.messageMode='normal';fixture.requests=[];
    await call('Emulation.setTouchEmulationEnabled',{enabled:false});
    await viewport(390,844);await evaluate('localStorage.clear()');
    const n=fixture.snapshotRequests;await call('Page.reload');
    if(!await until(()=>fixture.snapshotRequests>n&&evaluate(`state.bots.length===${bots.length}&&state.work.size===${work.length}`)))throw new Error('Gap reset failed');
    await delay(150);
  };
  const input=async text=>evaluate(`(()=>{const p=document.getElementById('prompt');p.value=${JSON.stringify(text)};p.focus();p.dispatchEvent(new Event('input',{bubbles:true}))})()`);
  const focus=async()=>evaluate(`(()=>{const e=document.activeElement,r=e.getBoundingClientRect();return {id:e.id,tag:e.tagName,text:e.textContent.slice(0,60),inert:!!e.closest('[inert]'),disabled:!!e.disabled,visible:r.width>0&&r.height>0&&getComputedStyle(e).visibility!=='hidden'&&!e.closest('[hidden]'),insideWork:document.getElementById('work-panel').contains(e),insideNav:document.getElementById('sidebar').contains(e),mainInert:document.getElementById('main').inert,panelClosed:document.getElementById('work-panel').hidden}})()`);
  const usable=f=>f.tag!=='BODY'&&f.visible&&!f.inert&&!f.disabled;
  const mention=async()=>evaluate(`(()=>{const p=document.getElementById('mention-picker'),r=p.getBoundingClientRect(),prompt=document.getElementById('prompt'),id=prompt.getAttribute('aria-activedescendant'),e=document.getElementById(id),s=e?.getBoundingClientRect();return {width:innerWidth,height:innerHeight,visualHeight:visualViewport.height,offsetTop:visualViewport.offsetTop,hidden:p.hidden,top:r.top,bottom:r.bottom,left:r.left,right:r.right,scroll:p.scrollTop,selected:id,selectedTop:s?.top,selectedBottom:s?.bottom,selectedHit:!!e&&e.contains(document.elementFromPoint(s.x+s.width/2,s.y+s.height/2)),focus:document.activeElement.id,draft:prompt.value,documentWidth:document.documentElement.scrollWidth}})()`);
  const menu=async()=>evaluate(`(()=>{const p=document.getElementById('composer-menu'),r=p.getBoundingClientRect(),e=p.querySelector('input:checked'),l=e.closest('label'),s=l.getBoundingClientRect();return {width:innerWidth,height:innerHeight,hidden:p.hidden,top:r.top,bottom:r.bottom,left:r.left,right:r.right,scroll:p.scrollTop,selected:e.value,selectedTop:s.top,selectedBottom:s.bottom,selectedHit:l.contains(document.elementFromPoint(s.x+s.width/2,s.y+s.height/2)),focusLane:document.activeElement.value,documentWidth:document.documentElement.scrollWidth}})()`);
  const work=()=>({requestId:'gap-work',botId:'bot-meiiie',sourceBotId:'bot-meiiie',sourceMessageId:'gap-message',clientNonce:'gap-nonce',text:'Gap cancellation request',lane:'background',status:'running',desiredState:'running',epoch:1,createdSequence:1,createdAt:'2026-10-11T00:00:00Z'});
  const event=async w=>{fixture.work=[clone(w)];const sent=push(w);if(!await until(()=>evaluate(`state.sequence===${sent.sequence}`)))throw new Error('Gap SSE not consumed');};
  const startCancel=async()=>{
    const w=work();await reset([],[w]);fixture.cancelMode='hold';await click('#work-open');
    await evaluate(`document.querySelector('.task[data-request-id="gap-work"] footer button').focus()`);
    await key('Enter');if(!await until(()=>fixture.cancelHeld.length===1))throw new Error('Held cancel missing');
    return {w,held:fixture.cancelHeld.shift(),pending:await focus()};
  };
  const release=async(held,w,status=200)=>{held.json(status===200?w:{error:'gap-unavailable'},status);await delay(220);};

  await run('gap_mention_open_orientation_keeps_selected_option',async()=>{
    await reset(cohort);await input('@');for(let i=0;i<9;i++)await key('ArrowDown');
    const initial=await mention(),values=[];
    for(const [w,h]of [[320,320],[390,260],[844,390],[390,844]]){
      await call('Emulation.setTouchEmulationEnabled',{enabled:true,maxTouchPoints:5});await viewport(w,h,true);values.push(await mention());
    }
    await screenshot('gap-mention-restored');
    return {ok:values.every(v=>!v.hidden&&v.top>=0&&v.bottom<=v.height&&v.left>=0&&v.right<=v.width&&v.documentWidth<=v.width&&v.selected===initial.selected&&v.selectedTop>=v.top&&v.selectedBottom<=v.bottom&&v.selectedHit&&v.focus==='prompt'&&v.draft==='@')&&fixture.requests.length===0,evidence:{initial,values,scope:'Layout and visual viewport resize plus coarse pointer; no physical keyboard or OS orientation event'}};
  });
  await run('gap_lane_open_orientation_keeps_radio_reachable',async()=>{
    await reset();await input('Gap retained lane draft');await click('#composer-menu-open');const values=[];
    for(const [w,h]of [[390,260],[320,320],[844,390],[390,844]]){
      await call('Emulation.setTouchEmulationEnabled',{enabled:true,maxTouchPoints:5});await viewport(w,h,true);values.push(await menu());
    }
    await key('Escape');const restored=await focus();
    return {ok:values.every(v=>!v.hidden&&v.top>=0&&v.bottom<=v.height&&v.left>=0&&v.right<=v.width&&v.documentWidth<=v.width&&v.selectedHit&&v.focusLane==='foreground')&&restored.id==='composer-menu-open',evidence:{values,restored,draft:await evaluate('document.getElementById("prompt").value')}};
  });
  await run('gap_focused_mention_keyboard_shrink_and_restore',async()=>{
    await reset(cohort);await input('@');for(let i=0;i<13;i++)await key('ArrowDown');
    await viewport(390,260,true);const shrunk=await mention();await screenshot('gap-mention-shrunk');
    await viewport(390,844,true);const restored=await mention();await key('Enter');
    const selected=await evaluate(`({draft:document.getElementById('prompt').value,focus:document.activeElement.id,hidden:document.getElementById('mention-picker').hidden})`);
    return {ok:shrunk.selectedHit&&shrunk.top>=0&&restored.selected===shrunk.selected&&restored.selectedHit&&selected.hidden&&selected.focus==='prompt'&&selected.draft==='@gap11 '&&fixture.requests.length===0,evidence:{shrunk,restored,selected,scope:'390x844 to 390x260 CDP metrics simulation only; actual keyboard overlay/panning is not covered'}};
  });
  await run('gap_empty_drawer_same_breakpoint_resize_focus',async()=>{
    await reset();await click('#nav-open');await evaluate(`(()=>{const s=document.getElementById('bot-search');s.value='gap-no-results';s.focus();s.dispatchEvent(new Event('input',{bubbles:true}))})()`);
    const values=[];
    for(const [w,h]of [[320,320],[390,260],[390,844]]){await viewport(w,h,true);values.push({...await focus(),geometry:await evaluate(`(()=>{const s=document.getElementById('bot-search'),r=s.getBoundingClientRect();return {top:r.top,bottom:r.bottom,width:innerWidth,height:innerHeight,count:document.querySelectorAll('#bots button').length}})()`)});}
    await key('Tab',true);const reverse=await focus();await key('Escape');const closed=await focus();
    return {ok:values.every(v=>usable(v)&&v.id==='bot-search'&&v.insideNav&&v.mainInert&&v.geometry.count===0&&v.geometry.top>=0&&v.geometry.bottom<=v.geometry.height)&&reverse.insideNav&&usable(reverse)&&closed.id==='nav-open',evidence:{values,reverse,closed}};
  });
  await run('gap_empty_drawer_breakpoint_crossing_focus_and_reopen',async()=>{
    await reset();await click('#nav-open');await evaluate(`(()=>{const s=document.getElementById('bot-search');s.value='gap-no-results';s.focus();s.dispatchEvent(new Event('input',{bubbles:true}))})()`);
    await viewport(844,390,true);const landscape=await focus();await viewport(390,844,true);const portrait=await focus();await screenshot('gap-drawer-return');
    await click('#nav-open');const reopened=await focus();const count=await evaluate(`document.querySelectorAll('#bots button').length`);
    return {ok:usable(landscape)&&usable(portrait)&&reopened.insideNav&&usable(reopened)&&count===0,evidence:{landscape,portrait,reopened,count,expectation:'Crossing the mobile breakpoint may close the drawer; focus must remain on a visible non-inert control. Reopen must focus search when roster is empty.'}};
  });
  await run('gap_keyboard_cancel_pending_and_terminal_focus',async()=>{
    const {w,held,pending}=await startCancel();const cancelled={...w,status:'cancelled',desiredState:'cancelled'};
    await event(cancelled);const streamed=await focus();await release(held,cancelled);const acknowledged=await focus();await screenshot('gap-cancel-terminal');
    return {ok:[pending,streamed,acknowledged].every(f=>usable(f)&&f.insideWork),evidence:{pending,streamed,acknowledged,calls:fixture.cancelRequests.length,expectation:'Keyboard cancel disables then removes its own control; visible enabled focus should remain in the open modal.'}};
  });
  await run('gap_late_cancel_after_leaving_preserves_new_draft_focus',async()=>{
    const {w,held}=await startCancel();await key('Escape');await input('Gap newer draft after cancel');
    await evaluate(`document.getElementById('prompt').setSelectionRange(4,9)`);
    const cancelled={...w,status:'cancelled',desiredState:'cancelled'};await event(cancelled);await release(held,cancelled);
    const values=await evaluate(`({focus:document.activeElement.id,draft:document.getElementById('prompt').value,selection:[document.getElementById('prompt').selectionStart,document.getElementById('prompt').selectionEnd],closed:document.getElementById('work-panel').hidden,mainInert:document.getElementById('main').inert})`);
    return {ok:values.focus==='prompt'&&values.draft==='Gap newer draft after cancel'&&values.selection.join()==='4,9'&&values.closed&&!values.mainInert,evidence:values};
  });
  await run('gap_late_events_preserve_open_popup_and_active_focus',async()=>{
    const w=work();await reset(cohort,[w]);await input('@');for(let i=0;i<7;i++)await key('ArrowDown');const initial=await mention();
    await event({...w,status:'completed',reply:'Gap late completed response'});const updated=await mention();
    await key('Escape');await click('#composer-menu-open');await event({...w,status:'cancelled'});const lane=await menu();
    return {ok:updated.focus==='prompt'&&!updated.hidden&&updated.selected===initial.selected&&updated.selectedHit&&updated.draft==='@'&&!lane.hidden&&lane.focusLane==='foreground'&&lane.selectedHit,evidence:{initial,updated,lane,scope:'Fixture SSE terminal events; no model or real task cancellation semantics validated'}};
  });
}
