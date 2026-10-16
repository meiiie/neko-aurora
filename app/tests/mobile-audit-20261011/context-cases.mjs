export async function contextCases({ run, fixture, clone, push, evaluate, call, viewport, reload, click, key, ax, screenshot, touch, until, delay }) {
  const caps = ['context_access_v1', 'context_refresh_v1'];
  const baseWork = (overrides = {}) => ({ requestId: 'ctx-source', botId: 'bot-meiiie', sourceBotId: 'bot-meiiie', sourceMessageId: 'ctx-message', clientNonce: 'fixture-context-nonce', ownerUserId: 'fixture-only', conversationId: 'bot-meiiie', spaceId: 'fixture-only', text: 'Fixture source request', lane: 'background', status: 'completed', desiredState: 'running', epoch: 12, createdSequence: 1, reply: 'Fixture original reply', contextRevision: 2, contextAccess: { revision: 3, granted: true }, ...overrides });
  const stopped = (overrides = {}) => baseWork({ status: 'suspended', desiredState: 'suspended', epoch: 17, runtimeStopped: true, attemptId: 'fixture-attempt', core: { nativeProcessId: 4517, sessionId: 'fixture-session' }, runtimeStopReceipt: { version: 1, nativeProcessId: 4517, sessionId: 'fixture-session' }, operations: [], contextAttempts: [], ...overrides });
  const access = '.task[data-request-id="ctx-source"] .context-access';
  const refresh = '.task[data-request-id="ctx-source"] .context-refresh';
  const task = '.task[data-request-id="ctx-source"]';
  const reset = async (work = baseWork(), capabilities = caps) => {
    for (const held of fixture.contextHeld.splice(0)) held.json({ error: 'fixture-reset' }, 503);
    fixture.work = [clone(work)]; fixture.capabilities = [...capabilities]; fixture.sequence = 1; fixture.events = [];
    fixture.contextRequests = []; fixture.contextMode = 'hold'; fixture.snapshotMode = 'normal';
    const snapshotBefore = fixture.snapshotRequests;
    await evaluate(`localStorage.setItem('neko.selected-bot','bot-meiiie')`); await reload();
    if (!await until(() => fixture.snapshotRequests > snapshotBefore)) throw new Error('New fixture snapshot not requested after reload');
    await viewport(390, 844);
    if (!await until(() => evaluate(`state.work.has('ctx-source')`))) throw new Error('Context fixture reset not ready');
    await click('#work-open');
  };
  const visible = async selector => evaluate(`(()=>{const e=document.querySelector(${JSON.stringify(selector)});return !!e&&!e.hidden&&!!e.getBoundingClientRect().height&&!e.closest('[hidden]')})()`);
  const live = async () => evaluate(`(()=>{const w=state.work.get('ctx-source'),e=document.querySelector('.task[data-request-id="ctx-source"] .context-feedback');return {epoch:w.epoch,status:w.status,desiredState:w.desiredState,contextRevision:w.contextRevision,access:w.contextAccess,reply:w.reply,feedback:e.textContent,focus:document.activeElement.className||document.activeElement.id||document.activeElement.tagName,focusInside:document.getElementById('work-panel').contains(document.activeElement)}})()`);
  const held = async () => { if (!await until(() => fixture.contextHeld.length)) throw new Error('Expected held fixture request; ' + JSON.stringify(await live())); return fixture.contextHeld.shift(); };
  const respond = async (item, work, status = 200) => { item.json(status === 200 ? (item.kind === 'access' ? { work } : work) : { error: 'fixture-context-response' }, status); await delay(180); };
  const sendEvent = async work => { fixture.work = [clone(work)]; const event = push(work); if (!await until(() => evaluate(`state.sequence === ${event.sequence}`))) throw new Error('Fixture SSE not consumed'); };

  await run('context_capabilities_absent_hides_controls', async () => {
    await reset(stopped(), []); const values = { access: await visible(access), refresh: await visible(refresh), calls: fixture.contextRequests.length };
    return { ok: !values.access && !values.refresh && values.calls === 0, evidence: values };
  });
  await run('context_access_default_and_ax_toggle', async () => {
    const work = baseWork(); delete work.contextAccess; await reset(work, ['context_access_v1']);
    const node = await ax(access); const values = await evaluate(`({pressed:document.querySelector(${JSON.stringify(access)}).getAttribute('aria-pressed'),text:document.querySelector(${JSON.stringify(access)}).textContent,note:document.querySelector('.context-note').textContent})`);
    return { ok: node.role === 'button' && !node.ignored && node.name.includes('Fixture source request') && values.pressed === 'true' && !await visible(refresh), evidence: { ax: node, ...values } };
  });
  await run('context_mobile_layout_and_touch_targets', async () => {
    await reset(stopped({ text: 'Fixture very long request ' + 'W'.repeat(480) })); const values = [];
    for (const [width, height] of [[320, 640], [390, 844], [700, 700], [390, 260], [320, 320]]) {
      await viewport(width, height); const controls = await touch([access, refresh]);
      const geometry = await evaluate(`({width:innerWidth,height:innerHeight,documentWidth:document.documentElement.scrollWidth,panelWidth:document.getElementById('work-panel').scrollWidth,panelClient:document.getElementById('work-panel').clientWidth})`);
      await evaluate(`document.querySelector(${JSON.stringify(refresh)}).scrollIntoView({block:'nearest'})`);
      const bounds = await evaluate(`(()=>{const r=document.querySelector(${JSON.stringify(refresh)}).getBoundingClientRect(),p=document.getElementById('tasks-section');return {top:r.top,bottom:r.bottom,left:r.left,right:r.right,scrollportHeight:p.clientHeight,scrollHeight:p.scrollHeight,scrollTop:p.scrollTop}})()`);
      const reachable = bounds.top >= -1 && bounds.bottom <= height + 1 && bounds.left >= -1 && bounds.right <= width + 1 && bounds.scrollportHeight >= bounds.bottom - bounds.top;
      if (!reachable) await screenshot('context-short-' + width + 'x' + height);
      values.push({ ...geometry, controls, reachable, bounds });
    }
    await viewport(390, 844); await screenshot('context-mobile-390');
    return { ok: values.every(v => v.documentWidth <= v.width + 1 && v.panelWidth <= v.panelClient + 1 && v.reachable && v.controls.every(c => c.width >= 43.99 && c.height >= 43.99)), evidence: { values, keyboardScope: 'Resized layout only, no OS keyboard or safe-area hardware values' } };
  });
  await run('context_coarse_pointer_landscape44', async () => {
    await reset(stopped()); await call('Emulation.setTouchEmulationEnabled', { enabled: true, maxTouchPoints: 5 }); await viewport(844, 390, true); await click('#work-open');
    const controls = await touch([access, refresh]); const media = await evaluate(`({coarse:matchMedia('(pointer:coarse)').matches,touchPoints:navigator.maxTouchPoints})`);
    const reachable = [];
    for (const selector of [access, refresh]) {
      await evaluate(`document.querySelector(${JSON.stringify(selector)}).scrollIntoView({block:'nearest'})`);
      reachable.push(await evaluate(`(()=>{const e=document.querySelector(${JSON.stringify(selector)}),r=e.getBoundingClientRect(),hit=document.elementFromPoint(r.x+r.width/2,r.y+r.height/2);return {selector:${JSON.stringify(selector)},hit:e===hit||e.contains(hit),top:r.top,bottom:r.bottom,scrollportHeight:document.getElementById('tasks-section').clientHeight}})()`));
    }
    await screenshot('context-coarse-landscape'); await call('Emulation.setTouchEmulationEnabled', { enabled: false });
    return { ok: media.coarse && controls.length === 2 && controls.every(c => c.width >= 43.99 && c.height >= 43.99) && reachable.every(v => v.hit && v.top >= 0 && v.bottom <= 390), evidence: { media, controls, reachable } };
  });
  await run('context_keyboard_tab_escape_return', async () => {
    await reset(stopped()); await evaluate(`document.querySelector(${JSON.stringify(access)}).focus()`); let escaped = false;
    for (let i = 0; i < 10; i++) { await key('Tab', i % 2 === 1); if (!await evaluate(`document.getElementById('work-panel').contains(document.activeElement)`)) escaped = true; }
    await key('Escape'); const values = await evaluate(`({closed:document.getElementById('work-panel').hidden,focus:document.activeElement.id,mainInert:document.getElementById('main').inert})`);
    return { ok: !escaped && values.closed && values.focus === 'work-open' && !values.mainInert, evidence: { escaped, ...values } };
  });
  await run('context_refresh_runtime_proof_matrix', async () => {
    const variants = [
      ['native_matches', stopped(), true],
      ['never_launched', stopped({ core: undefined, attemptId: undefined, runtimeStopped: undefined, runtimeStopReceipt: undefined }), true],
      ['pid_mismatch', stopped({ runtimeStopReceipt: { version: 1, nativeProcessId: 4518, sessionId: 'fixture-session' } }), false],
      ['session_mismatch', stopped({ runtimeStopReceipt: { version: 1, nativeProcessId: 4517, sessionId: 'other-session' } }), false],
      ['receipt_absent', stopped({ runtimeStopReceipt: undefined }), false],
      ['not_stopped', stopped({ runtimeStopped: false }), false],
      ['receipt_wrong_version', stopped({ runtimeStopReceipt: { version: 2, nativeProcessId: 4517, sessionId: 'fixture-session' } }), false],
      ['unknown_wsl', stopped({ core: { provider: 'wsl', sessionId: 'fixture-session' }, runtimeStopReceipt: undefined }), false],
      ['operations_pending', stopped({ operations: [{ id: 'fixture-operation' }] }), false],
      ['attempt_limit', stopped({ contextAttempts: [{}, {}, {}, {}] }), false],
      ['own_access_revoked', stopped({ contextAccess: { revision: 3, granted: false } }), false],
      ['suspending', stopped({ status: 'suspending' }), false],
      ['desired_running', stopped({ desiredState: 'running' }), false],
      ['unsafe_epoch', stopped({ epoch: 9007199254740992 }), false],
      ['capability_absent', stopped(), false, ['context_access_v1']],
    ];
    const values = [];
    for (const [name, work, expected, capabilities] of variants) { await reset(work, capabilities ?? caps); values.push({ name, expected, actual: await visible(refresh), calls: fixture.contextRequests.length }); }
    return { ok: values.every(v => v.actual === v.expected && v.calls === 0), evidence: { values, scope: 'UI guard only; fixture proof is neither native stop proof nor backend acceptance' } };
  });
  await run('context_access_pending_no_optimistic_duplicate', async () => {
    await reset(); await evaluate(`document.querySelector(${JSON.stringify(access)}).focus()`); await key('Enter'); const item = await held();
    await evaluate(`document.querySelector(${JSON.stringify(access)}).click();document.querySelector(${JSON.stringify(access)}).click()`);
    const before = await live(), status = await ax('.context-feedback'); const body = clone(item.input);
    await respond(item, baseWork({ contextAccess: { revision: 4, granted: false } })); const after = await live();
    return { ok: fixture.contextRequests.length === 1 && before.access.granted && !after.access.granted && body.expectedRevision === 3 && body.granted === false && !!body.clientNonce && status.role === 'status', evidence: { before, after, body, calls: fixture.contextRequests.length, status } };
  });
  await run('context_refresh_runtime_attempt_binding_matrix', async () => {
    const binding = { nativeProcessId: 4517, sessionId: 'fixture-session' };
    const variants = [
      ['binding_only_match', stopped({ core: undefined, runtimeAttemptBinding: binding }), true],
      ['binding_only_pid_mismatch', stopped({ core: undefined, runtimeAttemptBinding: { ...binding, nativeProcessId: 4518 } }), false],
      ['binding_only_session_mismatch', stopped({ core: undefined, runtimeAttemptBinding: { ...binding, sessionId: 'other-session' } }), false],
      ['binding_missing_session', stopped({ core: undefined, runtimeAttemptBinding: { nativeProcessId: 4517 } }), false],
      ['binding_pid_zero', stopped({ core: undefined, runtimeAttemptBinding: { ...binding, nativeProcessId: 0 } }), false],
      ['binding_pid_string', stopped({ core: undefined, runtimeAttemptBinding: { ...binding, nativeProcessId: '4517' } }), false],
      ['binding_blocks_never_launched_fallback', stopped({ core: undefined, attemptId: undefined, runtimeStopped: undefined, runtimeStopReceipt: undefined, runtimeAttemptBinding: binding }), false],
      ['binding_mismatch_core_matches', stopped({ runtimeAttemptBinding: { ...binding, sessionId: 'other-session' } }), false],
      ['current_binding_precedes_old_core', stopped({ core: { nativeProcessId: 9921, sessionId: 'old-core-session' }, runtimeAttemptBinding: binding }), true],
      ['binding_no_receipt', stopped({ core: undefined, runtimeAttemptBinding: binding, runtimeStopReceipt: undefined }), false],
      ['binding_not_stopped', stopped({ core: undefined, runtimeAttemptBinding: binding, runtimeStopped: false }), false],
    ];
    const values = [];
    for (const [name, work, expected] of variants) { await reset(work); values.push({ name, expected, actual: await visible(refresh), calls: fixture.contextRequests.length }); }
    return { ok: values.every(v => v.actual === v.expected && v.calls === 0), evidence: { values, scope: 'Fixture runtimeAttemptBinding UI gate/precedence only; not actual attempt/process-stop proof' } };
  });
  await run('context_access_retry_preserves_nonce_body', async () => {
    await reset(); await click(access); const first = await held(); await respond(first, null, 503); const uncertain = await live();
    await click(access); const second = await held(); const same = JSON.stringify(first.input) === JSON.stringify(second.input); await respond(second, baseWork({ contextAccess: { revision: 4, granted: false } }));
    return { ok: same && uncertain.access.granted && fixture.contextRequests.length === 2, evidence: { same, uncertain, first: first.input, second: second.input } };
  });
  await run('context_retry_accessible_name_matches_visible_action', async () => {
    await reset(); await click(access); await respond(await held(), null, 503);
    const node = await ax(access), text = await evaluate(`document.querySelector(${JSON.stringify(access)}).textContent`);
    return { ok: node.name.includes(text), evidence: { visibleText: text, accessibleName: node.name, scope: 'Chromium AX label/action consistency; no speech or voice-control device tested' } };
  });
  await run('context_access_conflict_resync_without_auto_retry', async () => {
    await reset(); await click(access); const item = await held(), before = fixture.snapshotRequests;
    fixture.work = [baseWork({ epoch: 19, status: 'suspended', desiredState: 'suspended', contextAccess: { revision: 6, granted: false } })]; await respond(item, null, 409); const values = await live();
    return { ok: values.epoch === 19 && values.access.revision === 6 && !values.access.granted && fixture.snapshotRequests > before && fixture.contextRequests.length === 1 && values.feedback.includes('Đã tải lại'), evidence: { values, additionalSnapshots: fixture.snapshotRequests - before, calls: fixture.contextRequests.length } };
  });
  await run('context_late_ack_preserves_other_bot_draft_focus', async () => {
    await reset(); await click(access); const item = await held(); await key('Escape'); await click('#nav-open'); await click('#bots button[data-bot-id="bot-davin"]');
    await evaluate(`document.getElementById('prompt').value='Fixture untouched Davin draft';document.getElementById('prompt').focus()`); await respond(item, baseWork({ contextAccess: { revision: 4, granted: false } }));
    const values = await evaluate(`({bot:document.getElementById('bot-name').textContent,draft:document.getElementById('prompt').value,focus:document.activeElement.id,sourceAccess:state.work.get('ctx-source').contextAccess})`);
    return { ok: values.bot === 'Davin' && values.draft === 'Fixture untouched Davin draft' && values.focus === 'prompt' && !values.sourceAccess.granted, evidence: values };
  });
  await run('context_newer_sse_rejects_late_ack_equal_counters', async () => {
    await reset(); await click(access); const item = await held(); const newer = baseWork({ epoch: 30, status: 'suspended', desiredState: 'suspended', reply: 'Fixture SSE newest reply', contextRevision: 8, contextAccess: { revision: 4, granted: false } });
    await sendEvent(newer); await respond(item, baseWork({ epoch: 12, contextRevision: 2, contextAccess: { revision: 4, granted: false } })); const values = await live();
    return { ok: values.epoch === 30 && values.reply === newer.reply && values.contextRevision === 8, evidence: values };
  });
  await run('context_newer_sse_not_rolled_back_by_mixed_revision_ack', async () => {
    await reset(); await click(access); const item = await held();
    const newer = baseWork({ epoch: 30, status: 'suspended', desiredState: 'suspended', reply: 'Fixture SSE newest reply', contextRevision: 8, contextAccess: { revision: 3, granted: true } });
    await sendEvent(newer); const before = await live();
    await respond(item, baseWork({ epoch: 12, status: 'completed', desiredState: 'running', reply: 'Fixture original reply', contextRevision: 2, contextAccess: { revision: 4, granted: false } })); const after = await live();
    return { ok: after.epoch >= before.epoch && after.contextRevision >= before.contextRevision && after.status === 'suspended' && after.reply === newer.reply, evidence: { before, after, premise: 'ACK has newer access revision but older epoch/context revision; preserving backend-newer work must not depend on a single counter OR comparison', fixtureTransport: 'Actual loopback SSE then held loopback HTTP ACK' } };
  });
  await run('context_regrant_never_auto_refresh', async () => {
    await reset(stopped({ contextAccess: { revision: 4, granted: false } })); await click(access); const item = await held(); await respond(item, stopped({ contextAccess: { revision: 5, granted: true } })); const values = await live();
    return { ok: values.status === 'suspended' && values.access.granted && fixture.contextRequests.length === 1 && fixture.contextRequests[0].kind === 'access' && await visible(refresh), evidence: { values, calls: fixture.contextRequests } };
  });
  await run('context_refresh_explicit_payload_and_pending_guard', async () => {
    await reset(stopped()); await click(refresh); const item = await held(); await evaluate(`document.querySelector(${JSON.stringify(refresh)}).click()`); const before = await live();
    await respond(item, stopped({ epoch: 18, status: 'waiting_host', desiredState: 'running', reply: '', runtimeStopped: false, runtimeStopReceipt: undefined, core: undefined, attemptId: undefined, contextRevision: 3 })); const after = await live();
    return { ok: fixture.contextRequests.length === 1 && item.input.expectedEpoch === 17 && Object.keys(item.input).length === 2 && before.status === 'suspended' && after.status === 'waiting_host' && after.epoch === 18 && !await visible(refresh), evidence: { body: item.input, before, after, calls: fixture.contextRequests.length } };
  });
  await run('context_refresh_hidden_control_restores_logical_focus', async () => {
    await reset(stopped()); await evaluate(`document.querySelector(${JSON.stringify(refresh)}).focus()`); await key('Enter'); const item = await held(); const pending = await live();
    await respond(item, stopped({ epoch: 18, status: 'waiting_host', desiredState: 'running', reply: '', runtimeStopped: false, runtimeStopReceipt: undefined, core: undefined, attemptId: undefined, contextRevision: 3 })); const after = await live();
    const focusUsable = await evaluate(`(()=>{const e=document.activeElement,r=e.getBoundingClientRect();return document.getElementById('work-panel').contains(e)&&r.width>0&&r.height>0&&!e.disabled&&!e.hidden})()`);
    return { ok: focusUsable, evidence: { pending, after, expectation: 'Keyboard-initiated refresh should finish at a visible enabled control in the still-open mobile work dialog; do not steal focus after the user leaves' } };
  });
  await run('context_pending_ack_does_not_steal_moved_dialog_focus', async () => {
    await reset(stopped()); await evaluate(`document.querySelector(${JSON.stringify(refresh)}).focus()`); await key('Enter'); const item = await held();
    await evaluate(`document.getElementById('tasks-tab').focus()`);
    await respond(item, stopped({ epoch: 18, status: 'waiting_host', desiredState: 'running', reply: '', runtimeStopped: false, runtimeStopReceipt: undefined, core: undefined, attemptId: undefined, contextRevision: 3 }));
    const values = await evaluate(`({focus:document.activeElement.id,panelClosed:document.getElementById('work-panel').hidden})`);
    return { ok: values.focus === 'tasks-tab' && !values.panelClosed, evidence: values };
  });
  await run('context_old_ack_does_not_steal_reopened_panel_focus', async () => {
    await reset(stopped()); await evaluate(`document.querySelector(${JSON.stringify(refresh)}).focus()`); await key('Enter'); const item = await held();
    await key('Escape'); await click('#work-open'); await evaluate(`document.getElementById('work-close').focus()`);
    await respond(item, stopped({ epoch: 18, status: 'waiting_host', desiredState: 'running', reply: '', runtimeStopped: false, runtimeStopReceipt: undefined, core: undefined, attemptId: undefined, contextRevision: 3 }));
    const values = await evaluate(`({focus:document.activeElement.id,panelClosed:document.getElementById('work-panel').hidden})`);
    return { ok: values.focus === 'work-close' && !values.panelClosed, evidence: values };
  });
  await run('context_refresh_uncertain_ack_snapshot_no_auto_replay', async () => {
    await reset(stopped()); await click(refresh); const item = await held(), before = fixture.snapshotRequests;
    fixture.work = [stopped({ epoch: 18, status: 'waiting_host', desiredState: 'running', reply: '', core: undefined, attemptId: undefined, runtimeStopped: false, runtimeStopReceipt: undefined })]; await respond(item, null, 503); await delay(300); const values = await live();
    return { ok: values.epoch === 18 && values.status === 'waiting_host' && fixture.snapshotRequests > before && fixture.contextRequests.length === 1, evidence: { values, additionalSnapshots: fixture.snapshotRequests - before, calls: fixture.contextRequests.length } };
  });
  await run('context_capability_removed_pending_ack_no_hidden_dispatch', async () => {
    await reset(stopped()); await click(access); const item = await held(); fixture.capabilities = []; await evaluate('sync()');
    await respond(item, stopped({ contextAccess: { revision: 4, granted: false } })); const values = { accessVisible: await visible(access), refreshVisible: await visible(refresh), calls: fixture.contextRequests.length, current: await live() };
    await evaluate(`document.querySelector(${JSON.stringify(access)}).click();document.querySelector(${JSON.stringify(refresh)}).click()`);
    return { ok: !values.accessVisible && !values.refreshVisible && fixture.contextRequests.length === 1, evidence: values };
  });
  await run('context_late_refresh_ack_preserves_closed_panel_draft_focus', async () => {
    await reset(stopped()); await click(refresh); const item = await held(); await key('Escape'); await evaluate(`document.getElementById('prompt').value='Fixture continuing draft';document.getElementById('prompt').focus()`);
    await respond(item, stopped({ epoch: 18, status: 'waiting_host', desiredState: 'running', reply: '', core: undefined, attemptId: undefined, runtimeStopped: false, runtimeStopReceipt: undefined }));
    const values = await evaluate(`({closed:document.getElementById('work-panel').hidden,draft:document.getElementById('prompt').value,focus:document.activeElement.id})`);
    return { ok: values.closed && values.draft === 'Fixture continuing draft' && values.focus === 'prompt', evidence: values };
  });
  await run('context_refresh_newer_sse_not_rolled_back_by_context_revision_ack', async () => {
    await reset(stopped()); await click(refresh); const item = await held();
    const newer = stopped({ epoch: 21, status: 'cancelled', desiredState: 'cancelled', reply: 'Fixture cancellation already observed', contextRevision: 2, contextAccess: { revision: 5, granted: false } });
    await sendEvent(newer); const before = await live(); await respond(item, stopped({ epoch: 18, status: 'waiting_host', desiredState: 'running', reply: '', contextRevision: 3, contextAccess: { revision: 3, granted: true } })); const after = await live();
    return { ok: after.epoch >= before.epoch && after.access.revision >= before.access.revision && after.status === 'cancelled', evidence: { before, after, premise: 'Refresh ACK has context revision +1 but older epoch/access revision than already observed cancellation event' } };
  });
}
