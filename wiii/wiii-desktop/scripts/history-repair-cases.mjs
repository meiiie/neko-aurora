import assert from 'node:assert/strict';

export async function historyRepairCases(check, {NekoBotLocalBridge}, repair, native) {
 const source={sourceApp:'wiii',sourceSessionId:'wiii-source-test',sourceThreadId:'thread-test',channelId:'channel-test',expectedChannelRevision:2,ownerUserId:'local-owner',spaceId:'local-space'};
 const session={id:'adapter-test',version:1,sourceApp:'wiii',sourceSessionId:source.sourceSessionId,sourceThreadId:source.sourceThreadId,channelId:source.channelId,channelRevision:2,ownerUserId:source.ownerUserId,spaceId:source.spaceId,returnRoute:'/v1/adapters/sessions/adapter-test/replies'};
 const row=n=>({replyId:'reply-request-'+n,requestId:'request-'+n,sourceMessageId:'message-'+n,botId:'bot-test',role:'assistant',text:'Synthetic reply '+n,status:'completed',sequence:n+1,artifacts:[],availability:'available'});
 const accepted=n=>({input:{sourceMessageId:'message-'+n,botId:'bot-test',text:'Synthetic original '+n,lane:'foreground',clientNonce:'original-nonce-'+n,accountId:'account-test',accountRevision:7,projectId:'project-test',projectRevision:9,fileIds:['file-test']},requestId:'request-'+n,state:'accepted',errorCode:null});
 const saved=count=>({version:1,source,registrationNonce:'registration-test',adapterSession:session,cursor:count,outbox:Array.from({length:count},(_,i)=>accepted(i)),replies:Array.from({length:count},(_,i)=>{const {availability,...reply}=row(i);return reply;}),blocked:false});
 const delta=(cursor,replies=[])=>({...repair.sessionScope(session),cursor,replies});
 const token=offset=>{const bytes=Buffer.alloc(161);bytes[0]=1;bytes.writeUInt32BE(offset,1);return bytes.toString('base64url');};
 const reorder=value=>Array.isArray(value)?value.map(reorder):value&&typeof value==='object'?Object.fromEntries(Object.keys(value).sort().reverse().map(key=>[key,reorder(value[key])])):value;
 function fixture(count=1, cap=true, reorderStorage=false) {
  const map=new Map(),calls=[],writes=[];let original=saved(count),handler=async()=>({status:200,body:JSON.stringify(delta(count))}),failActive=false,nonceCounter=0;
  const storage={read:async()=>structuredClone(original),write:async value=>{original=structuredClone(value);},history:{read:async key=>structuredClone(map.get(key)??null),write:async(key,value)=>{writes.push(key);if(key==='active'&&failActive)throw Error('synthetic active save failure');map.set(key,reorderStorage?reorder(value):structuredClone(value));}}};
  const transport={...(cap?{capabilities:[repair.HISTORY_CAPABILITY]}:{}),request:async(method,path,body)=>{calls.push({method,path,body});return handler(method,path,body);}};
  const bridge=new NekoBotLocalBridge(source,transport,storage,()=> 'new-pending-nonce-'+(++nonceCounter));
  return {bridge,map,calls,writes,storage,transport,original:()=>structuredClone(original),handle:fn=>handler=fn,failActive:()=>failActive=true,reload:()=>new NekoBotLocalBridge(source,transport,storage,()=> 'reload-nonce')};
 }
 async function history(f,count=513, options={}) {
  const rows=Array.from({length:count},(_,i)=>row(i)),scope=repair.sessionScope(session),authorityDigest='a'.repeat(64),snapshotCursor=2000;
  if(options.mask)rows[0]={...rows[0],availability:'source_unavailable',text:'',artifacts:[]};
  const witness=await repair.historyWitness(scope,snapshotCursor,authorityDigest,rows),base={...scope,version:1,snapshotCursor,authorityDigest,...witness,totalRows:rows.length};
  f.handle(async(method,path)=>{
   assert.equal(method,'GET');assert.ok(path.startsWith('/v1/adapters/sessions/adapter-test/history'));
   const cursor=path.includes('?cursor=')?path.split('?cursor=')[1]:null;
   if(cursor===token(99999)) {
    assert.ok(f.writes.some(key=>key.startsWith('generation/')),'Candidate persisted before confirmation');
    assert.equal((options.bridge??f.bridge).visibleReplies.length,0,'No provisional rows visible');
    if(options.beforeConfirm)await options.beforeConfirm();
    if(options.confirmError)return {status:409,body:JSON.stringify({error:'adapter_history_authority_changed'})};
    return {status:200,body:JSON.stringify({...base,offset:rows.length,rows:[],complete:true,nextCursor:null,confirmationCursor:token(99999)})};
   }
   const offset=cursor?Buffer.from(cursor,'base64url').readUInt32BE(1):0;
   if(options.pageError&&offset>0)return {status:409,body:JSON.stringify({error:'adapter_history_authority_changed'})};
   const chunk=rows.slice(offset,offset+128),complete=offset+chunk.length===rows.length;
   const value={...base,offset,rows:chunk,complete,nextCursor:complete?null:token(offset+chunk.length),confirmationCursor:complete?token(99999):null};
   if(options.mismatch&&offset>0)value.sourceSessionId='other-session';
   if(options.wrongDigest)value.historyDigest='b'.repeat(64);
   return {status:200,body:JSON.stringify(value)};
  }); return rows;
 }
 await check('513_rows_atomic_confirmed_repair_retains_original_receipts',async()=>{
  const f=fixture(100);await f.bridge.hydrate();const originals=f.original().outbox;await history(f);await f.bridge.repairHistory();
  assert.equal(f.bridge.visibleReplies.length,513);assert.equal(f.bridge.snapshot.outbox.length,0);assert.equal(f.bridge.snapshot.replies.length,0);
  assert.deepEqual(f.bridge.acceptedReceipts,originals);assert.equal(f.calls.length,6);assert.ok(f.calls.every(c=>c.method==='GET'));
  assert.ok(!f.bridge.acceptedReceipts.some(r=>r.input.sourceMessageId==='message-500'),'Observed rows never invent original inputs');
  const pending=await f.bridge.enqueue({sourceMessageId:'fresh-message',botId:'bot-test',text:'New synthetic draft',lane:'foreground'});assert.equal(pending.state,'queued');assert.equal(f.bridge.acceptedReceipts.length,100);assert.equal(f.calls.length,6);
 });
 await check('100_accepted_roll_into_shards_before_new_pending_without_post',async()=>{
  const f=fixture(100);await f.bridge.hydrate();const before=f.original().outbox;await f.bridge.enqueue({sourceMessageId:'fresh-message',botId:'bot-test',text:'New draft',lane:'foreground'});
  assert.equal(f.bridge.snapshot.outbox.length,1);assert.deepEqual(f.bridge.acceptedReceipts,before);assert.equal(f.calls.length,0);
  await assert.rejects(f.bridge.discard('message-0'),e=>e.code==='accepted_message_is_history');
 });
 for(const [name,options] of [['midpage_authority_change',{pageError:true}],['final_authority_change',{confirmError:true}],['midpage_binding_change',{mismatch:true}],['digest_mismatch',{wrongDigest:true}]]) await check(name+'_holds_without_partial_activation',async()=>{
  const f=fixture();await f.bridge.hydrate();await f.bridge.refresh();const originals=f.original();await history(f,513,options);await assert.rejects(f.bridge.repairHistory());
  assert.equal(f.bridge.visibleReplies.length,0);assert.deepEqual(f.original(),originals);assert.equal(f.map.has('active'),false);assert.equal(f.bridge.acceptedReceipts[0].input.clientNonce,originals.outbox[0].input.clientNonce);
 });
 await check('final_storage_failure_and_reload_ignore_inactive_candidate',async()=>{
  const f=fixture();await f.bridge.hydrate();await history(f);f.failActive();await assert.rejects(f.bridge.repairHistory());
  assert.equal(f.bridge.visibleReplies.length,0);assert.equal(f.map.has('active'),false);const reload=f.reload();await reload.hydrate();assert.equal(reload.visibleReplies.length,0);assert.equal(reload.acceptedReceipts[0].input.clientNonce,'original-nonce-0');
 });
 await check('late_navigation_cancels_activation_preserving_pending',async()=>{
  const f=fixture();await f.bridge.hydrate();let current=true;await f.bridge.enqueue({sourceMessageId:'unsent-message',botId:'bot-test',text:'Untouched draft',lane:'foreground'});
  await history(f,513,{beforeConfirm:()=>{current=false;}});await assert.rejects(f.bridge.repairHistory(()=>current),e=>e.code==='history_navigation_changed');
  assert.equal(f.map.has('active'),false);assert.equal(f.bridge.snapshot.outbox.find(i=>i.input.sourceMessageId==='unsent-message').input.text,'Untouched draft');assert.equal(f.bridge.visibleReplies.length,0);
 });
 await check('masked_row_replaces_same_sequence_body_and_reload_is_held',async()=>{
  const f=fixture();await f.bridge.hydrate();await history(f,513,{mask:true});await f.bridge.repairHistory();assert.equal(f.bridge.visibleReplies[0].text,'');const reload=f.reload();await reload.hydrate();assert.equal(reload.visibleReplies.length,0);assert.equal(reload.acceptedReceipts.length,1);
 });
 await check('serde_reordered_object_keys_reload_semantic_history_and_original_receipts',async()=>{
  const f=fixture(100,true,true);await f.bridge.hydrate();const originals=f.original().outbox;await history(f);await f.bridge.repairHistory();const reload=f.reload();await reload.hydrate();assert.equal(reload.visibleReplies.length,0);assert.deepEqual(reload.acceptedReceipts,originals);
 });
 await check('unsupported_peer_uses_only_explicit_capability_get',async()=>{
  const f=fixture(1,false);await f.bridge.hydrate();f.handle(async()=>({status:404,body:'{}'}));await assert.rejects(f.bridge.repairHistory(),e=>e.code==='history_repair_unsupported');assert.equal(f.calls.length,1);assert.equal(f.calls[0].path,'/v1/adapters/capabilities');assert.equal(f.bridge.visibleReplies.length,0);
 });
 await check('native_generation_loss_hides_previously_visible_cache',async()=>{
  const f=fixture();await f.bridge.hydrate();await f.bridge.refresh();assert.equal(f.bridge.visibleReplies.length,1);
  const client=new native.NekoBotNativeClient(async()=>{throw 'neko_bot_local:connection_changed';});f.handle((method,path,body)=>client.transport({leaseId:'fixture-lease',source,projectId:'project-test',channelName:'Synthetic',bots:[]}).request(method,path,body));
  await assert.rejects(f.bridge.refresh(),e=>e.code==='connection_changed');assert.equal(f.bridge.visibleReplies.length,0);assert.equal(f.bridge.acceptedReceipts.length,1);
 });
 await check('mixed_clear_then_unrecorded_delta_hides_prior_body_without_partial_commit',async()=>{
  const f=fixture();await f.bridge.hydrate();await f.bridge.refresh();const original=f.original(),{availability:_,...clear}=row(0),{availability:__,...unknown}=row(999);clear.text='';clear.sequence=11;unknown.sequence=12;
  f.handle(async()=>({status:200,body:JSON.stringify(delta(12,[clear,unknown]))}));await assert.rejects(f.bridge.refresh(),e=>e.code==='reply_message_mismatch');assert.equal(f.bridge.visibleReplies.length,0);assert.deepEqual(f.original(),original);
 });
 await check('strict_rows_reject_masked_content_unpaired_unicode_and_extra_fields',()=>{
  assert.throws(()=>repair.parseHistoryRow({...row(0),availability:'source_unavailable'}));assert.throws(()=>repair.parseHistoryRow({...row(0),text:String.fromCharCode(0xd800)}));assert.throws(()=>repair.parseHistoryRow({...row(0),clientNonce:'forbidden'}));
 });
 await check('history_observation_cannot_fabricate_pending_intake_ack',async()=>{
  const f=fixture(0);await f.bridge.hydrate();const input={sourceMessageId:'message-0',botId:'bot-test',text:'Different local body and nonce',lane:'foreground'};await f.bridge.enqueue(input);const before=structuredClone(f.bridge.snapshot.outbox[0]);
  await history(f,1);await f.bridge.repairHistory();assert.deepEqual(f.bridge.snapshot.outbox[0],before);assert.equal(f.bridge.acceptedReceipts.length,0);assert.equal(f.bridge.visibleReplies.length,1);assert.ok(f.calls.every(c=>c.method==='GET'));
 });
 await check('history_cursor_requires_canonical_161byte_v1_215char_frame',()=>{
  assert.equal(repair.historyCursor(token(0)),token(0));for(const value of ['short-token',token(0)+'A','A'.repeat(215),token(0).slice(0,-1)+'B'])assert.throws(()=>repair.historyCursor(value));
 });
 await check('incremental_observation_preserves_unproven_pending_body_nonce',async()=>{
  const f=fixture(0);await f.bridge.hydrate();await f.bridge.enqueue({sourceMessageId:'message-0',botId:'bot-test',text:'Different local pending body',lane:'foreground'});const before=structuredClone(f.bridge.snapshot.outbox[0]);const {availability,...reply}=row(0);
  f.handle(async()=>({status:200,body:JSON.stringify(delta(1,[reply]))}));await f.bridge.refresh();assert.deepEqual(f.bridge.snapshot.outbox[0],before);assert.equal(f.bridge.acceptedReceipts.length,0);assert.equal(f.bridge.replyWindow(0).length,1);assert.ok(f.calls.every(c=>c.method==='GET'));
 });
 await check('bounded_history_window_retains_all_rows_without_full_projection_clone',async()=>{
  const f=fixture();await f.bridge.hydrate();await history(f);await f.bridge.repairHistory();assert.equal(f.bridge.replyCount,513);assert.equal(f.bridge.replyWindow(0).length,100);assert.equal(f.bridge.replyWindow(500).length,13);assert.equal(f.bridge.replyWindow(500)[0].requestId,'request-500');assert.throws(()=>f.bridge.replyWindow(0,101));
 });
 await check('native_client_generation_fences_late_response_after_disconnect',async()=>{
  let release,entered;const started=new Promise(resolve=>entered=resolve);const client=new native.NekoBotNativeClient(async command=>{if(command==='neko_bot_local_disconnect')return;entered();return new Promise(resolve=>release=resolve);});
  const transport=client.transport({leaseId:'fixture-lease',source,projectId:'project-test',channelName:'Synthetic',bots:[]}),request=transport.request('GET',session.returnRoute+'?after=0');await started;await client.disconnect();release({status:200,body:'{}'});await assert.rejects(request,e=>e.code==='connection_changed');
 });
 await check('100_maximum_body_pending_inputs_fit_separate_bounded_shards',async()=>{
  const f=fixture(0);await f.bridge.hydrate();for(let i=0;i<100;i++)await f.bridge.enqueue({sourceMessageId:'pending-only-'+i,botId:'bot-test',text:'x'.repeat(32768),lane:'foreground'});const before=structuredClone(f.bridge.snapshot.outbox);
  await history(f,1);await f.bridge.repairHistory();assert.deepEqual(f.bridge.snapshot.outbox,before);const reload=f.reload();await reload.hydrate();assert.deepEqual(reload.snapshot.outbox,before);assert.equal(reload.visibleReplies.length,0);
 });
 await check('reloaded_v2_empty_delta_cannot_release_recovered_bodies',async()=>{
  const f=fixture();await f.bridge.hydrate();await history(f);await f.bridge.repairHistory();const reload=f.reload();await reload.hydrate();f.handle(async()=>({status:200,body:JSON.stringify(delta(2001))}));await reload.refresh();assert.equal(reload.replyCount,0);assert.equal(reload.visibleReplies.length,0);assert.equal(reload.error,'history_repair_required');
  await history(f,513,{bridge:reload});await reload.repairHistory();assert.equal(reload.replyCount,513);assert.equal(reload.error,null);assert.equal(reload.acceptedReceipts.length,1);
 });
 await check('new_history_artifacts_8_and_filename80_utf8_bounds_preserve_legacy_policy',()=>{
  const files=Array.from({length:8},(_,i)=>({id:'file-'+i,filename:'猫'.repeat(26)+'aa',sha256:'a'.repeat(64),bytes:1}));
  assert.equal(repair.parseHistoryRow({...row(0),artifacts:files}).artifacts.length,8);
  assert.throws(()=>repair.parseHistoryRow({...row(0),artifacts:[...files,{...files[0],id:'ninth-file'}]}));
  assert.throws(()=>repair.parseHistoryRow({...row(0),artifacts:[{...files[0],filename:'猫'.repeat(27)}]}));
 });
}
