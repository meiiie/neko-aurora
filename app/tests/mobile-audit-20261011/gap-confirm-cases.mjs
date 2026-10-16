export async function gapCases({run,fixture,clone,push,evaluate,call,viewport,click,key,until,delay}) {
  const reset=async work=>{
    for(const h of fixture.cancelHeld.splice(0))h.json({error:'confirmation-reset'},503);
    fixture.work=clone(work);fixture.sequence=0;fixture.events=[];fixture.capabilities=[];fixture.cancelMode='normal';fixture.cancelRequests=[];
    await call('Emulation.setTouchEmulationEnabled',{enabled:true,maxTouchPoints:5});await viewport(390,844,true);
    await evaluate('localStorage.clear()');const n=fixture.snapshotRequests;await call('Page.reload');
    if(!await until(()=>fixture.snapshotRequests>n&&evaluate(`state.work.size===${work.length}`)))throw new Error('Confirmation reset failed');
    await delay(150);
  };
  const focus=async()=>evaluate(`(()=>{const e=document.activeElement,r=e.getBoundingClientRect();return {id:e.id,tag:e.tagName,inside:document.getElementById('work-panel').contains(e),usable:e.tagName!=='BODY'&&r.width>0&&r.height>0&&!e.disabled&&!e.closest('[hidden],[inert]'),panelClosed:document.getElementById('work-panel').hidden,mainInert:document.getElementById('main').inert}})()`);
  const seed=()=>({requestId:'confirm-work',botId:'bot-meiiie',sourceBotId:'bot-meiiie',clientNonce:'confirm-nonce',text:'Confirmed keyboard cancellation',lane:'background',status:'running',desiredState:'running',epoch:1,createdSequence:1,createdAt:'2026-10-11T00:00:00Z'});
  await run('confirm_orientation_focus_loss_with_empty_and_nonempty_roster',async()=>{
    const values=[];
    for(const empty of [true,false]){
      await reset([]);await click('#nav-open');await evaluate(`(()=>{const s=document.getElementById('bot-search');s.value=${empty?'"no-confirmation-results"':'""'};s.focus();s.dispatchEvent(new Event('input',{bubbles:true}))})()`);
      await viewport(844,390,true);const landscape=await focus();await viewport(390,844,true);const portrait=await focus();
      await key('Tab');const recovered=await focus();values.push({empty,landscape,portrait,recovered});
    }
    return {ok:values.every(v=>v.portrait.usable),evidence:{values,scope:'Two independently reloaded scenarios; Tab recovery recorded to distinguish lost focus from permanent keyboard trapping'}};
  });
  await run('confirm_cancel_focus_loss_pending_and_unsolicited_terminal_event',async()=>{
    const w=seed();await reset([w]);fixture.cancelMode='hold';await click('#work-open');
    await evaluate(`document.querySelector('.task footer button').focus()`);await key('Enter');if(!await until(()=>fixture.cancelHeld.length))throw new Error('Confirmation held cancel missing');
    const pending=await focus();await key('Tab');const recovered=await focus();const held=fixture.cancelHeld.shift();held.json({error:'confirmed-unavailable'},503);await delay(200);const rejected=await focus();await key('Escape');const escaped=await focus();
    await reset([w]);await click('#work-open');await evaluate(`document.querySelector('.task footer button').focus()`);const before=await focus();
    const completed={...w,status:'completed',reply:'Independent late event'};fixture.work=[completed];const sent=push(completed);if(!await until(()=>evaluate(`state.sequence===${sent.sequence}`)))throw new Error('Confirmation SSE missing');const terminal=await focus();
    await key('Tab');const terminalRecovered=await focus();
    return {ok:pending.usable&&pending.inside&&terminal.usable&&terminal.inside,evidence:{pending,recovered,rejected,escaped,before,terminal,terminalRecovered,cancellationRequestsInSecondScenario:fixture.cancelRequests.length,scope:'Unsolicited terminal SSE hides an already-focused cancel control even with no cancellation request; recovery via Tab works'}};
  });
}
