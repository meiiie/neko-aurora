// Actual task-control, acknowledgement, retirement and retry handlers with DOM/storage/transport fixtures.
// Run: node app/scripts/ui_task_confirmed_refresh.mjs (no browser/native/provider/network).
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {randomUUID} from 'node:crypto';
import vm from 'node:vm';
const source=readFileSync(new URL('../native/internal/web/assets/app.js',import.meta.url),'utf8');
const extract=name=>{let start=source.indexOf('function '+name+'(');assert.ok(start>=0);if(source.slice(start-6,start)==='async ')start-=6;if(name==='controlTask')return source.slice(start,source.indexOf('\n}',start)+2);const next=source.slice(start+1).search(/\n(?:async )?function /);return source.slice(start,next<0?source.length:start+1+next)};
const flush=async()=>{for(let i=0;i<30;i++)await Promise.resolve()};
const results=[];
async function test(name,fn){try{await fn();results.push({name,status:'PASS'})}catch(e){results.push({name,status:'FAIL',error:e.message});process.exitCode=1}}
function fixture(){
 const nodes=new Map(),calls=[],refreshes=[];let envelope={records:[],maxObservedAt:0};
 const $=id=>{if(!nodes.has(id))nodes.set(id,{value:'',hidden:true,disabled:false,open:false,textContent:'',listeners:{},addEventListener(k,fn){this.listeners[k]=fn},close(){this.open=false},focus(){ctx.document.activeElement=this}});return nodes.get(id)};
 const previous={requestId:'work-a',botId:'bot-a',conversationId:'channel-a',text:'Original task',epoch:1,taskRevision:0};
 const ctx=vm.createContext({$,TextEncoder,crypto:{randomUUID},state:{work:new Map([[previous.requestId,previous]]),capabilities:['local_task_controls_v1','local_service_crash_recovery_v1'],channels:[{id:'channel-a',revision:1,botIds:['bot-a']}]},document:{activeElement:null},mobile:{matches:false},taskNodes:new Map(),taskCommands:new Map(),navigationGeneration:0,panelGeneration:0,focusEpoch:0,key:'channel-a',viewKey:()=>ctx.key,render(){},controlLabels:{},canControl:()=>true,sourceAccess:()=>({granted:true,revision:0}),resourcePinAvailable:()=>true,groupProfileRetryIssue:()=>'',journalWorkMatches:(w,s)=>w?.botId===s.botId&&w?.conversationId===s.channelId,
  durableRecords:new Map(),durableIntents:new Map(),durableMaxObserved:0,durableClockBlocked:false,durableCorruptCount:0,durableStorageError:false,
  retainDurableCommand:async(kind,command)=>{if(command.record)return;const record={kind,input:command.body,scope:{viewKey:'channel-a',channelId:'channel-a',channelRevision:1,botId:'bot-a',filePins:[]},createdAt:Date.now(),expiresAt:Date.now()+300000,sha256:command.body.clientNonce};command.record=record;ctx.durableRecords.set(record.input.clientNonce,record);ctx.durableIntents.set(record.input.clientNonce,command);envelope.records.push(record)},
  journalLock:async fn=>fn(),readCommandEnvelope:()=>envelope,writeCommandEnvelope:records=>{if(ctx.failStorage)throw Error('storage');envelope={...envelope,records}},
  canonicalJSON:JSON.stringify,recoveryResourceIdentity:()=>null,
  request:(path,body)=>new Promise((resolve,reject)=>calls.push({path,body,resolve,reject})),sync:()=>new Promise((resolve,reject)=>refreshes.push({resolve,reject})),retrySync:()=>{ctx.reconnects++},reconnects:0,
  taskDialogTarget:null,taskDialogGeneration:0
 });
 for(const name of ['controlTask','acceptContextWork','recoveredOriginal','retireDurableCommand','durableRetryIssue','prepareDurableRetry','durableFailure'])vm.runInContext(extract(name),ctx);
 const submit=source.split('\n').find(line=>line.startsWith('$("task-control-form").addEventListener("submit"'));vm.runInContext(submit,ctx);
 return {ctx,$,calls,refreshes,previous,send:(action='supplement')=>ctx.controlTask('work-a',action,'Additional work'),ack(index=0){const b=calls[index].body;const work={...previous,epoch:b.action==='recover'?2:1,taskRevision:1};if(b.action==='recover'){previous.core={sessionId:'same',contextAdmission:{version:1}};work.core=previous.core}calls[index].resolve({work,...(['supplement','replace'].includes(b.action)?{related:{...previous,requestId:'work-related',text:b.text,taskLink:{parentRequestId:'work-a',kind:b.action,commandNonce:b.clientNonce}}}:{})})},command:()=>ctx.taskCommands.get('work-a')};
}
for(const action of ['supplement','replace','pause','resume','cancel','recover'])await test(`${action}: confirmed ACK survives failed refresh without retry trap`,async()=>{
 const h=fixture();const sending=h.send(action);await flush();
 // Concurrent newer work exercises the refresh branch for non-related acknowledgements.
 if(!['supplement','replace'].includes(action))h.ctx.state.work.set('work-a',{...h.previous,epoch:3,taskRevision:2});
 h.ack();await flush();h.refreshes[0].reject(Error('offline'));await flush();
 // Old handler enters a second refresh inside its failure path; release it to observe the wrong result.
 h.refreshes[1]?.reject(Error('offline'));
 assert.equal(await sending,true,'validated POST must stay successful');assert.equal(h.command().retry,false);assert.equal(h.command().pending,false);assert.equal(h.ctx.durableRecords.size,0);assert.equal(h.ctx.reconnects,1);
 if(['supplement','replace'].includes(action))assert.equal(h.ctx.state.work.get('work-related').text,'Additional work');
});
await test('hanging refresh cannot hold confirmed command pending',async()=>{const h=fixture();let done=false;const sending=h.send().then(v=>{done=v});await flush();h.ack();await flush();assert.equal(done,true);assert.equal(h.command().pending,false);assert.equal(h.command().retry,false);h.refreshes[0].resolve();await sending});
await test('true POST failure retries with identical original nonce',async()=>{const h=fixture();const sending=h.send();await flush();h.calls[0].reject(Error('503'));assert.equal(await sending,false);assert.equal(h.command().retry,true);assert.equal(h.ctx.durableRecords.size,1);const retry=h.ctx.controlTask('work-a','supplement','Additional work',h.command());await flush();assert.equal(h.calls[1].body.clientNonce,h.calls[0].body.clientNonce);h.ack(1);await flush();h.refreshes[0].resolve();assert.equal(await retry,true)});
await test('pending duplicate controls cannot mint another command',async()=>{const h=fixture();const sending=h.send();await flush();await h.send();await h.ctx.controlTask('work-a','supplement','Additional work',h.command());assert.equal(h.calls.length,1);h.ack();await flush();assert.equal(await sending,true);h.refreshes[0].resolve()});
await test('invalid acknowledgement remains unresolved with original durable command',async()=>{const h=fixture();const sending=h.send();await flush();h.calls[0].resolve({work:{...h.previous,taskRevision:1}});await flush();h.refreshes[0].resolve();assert.equal(await sending,false);assert.equal(h.command().retry,true);assert.equal(h.ctx.durableRecords.size,1);assert.equal(h.ctx.state.work.get('work-a').taskRevision,0)});
await test('storage retirement failure preserves blocked reconciliation and warning',async()=>{const h=fixture();h.ctx.failStorage=true;const sending=h.send();await flush();h.ack();await flush();h.refreshes[0]?.resolve();assert.equal(await sending,true);assert.equal(h.ctx.durableRecords.size,1);assert.equal(h.command().blocked,true);assert.match(h.command().message,/chưa xoá/);assert.equal(await h.ctx.prepareDurableRetry(h.command()),false);assert.equal(h.calls.length,1)});
for(const newer of [false,true])await test(`dialog acknowledgement owns only its original generation (${newer?'newer dialog':'same dialog'})`,async()=>{const h=fixture();h.ctx.taskDialogTarget={requestId:'work-a',action:'supplement',epoch:1,revision:0};h.$('task-control-text').value='Additional work';h.$('task-control-dialog').open=true;const sending=h.$('task-control-form').listeners.submit({preventDefault(){}});await flush();if(newer){h.ctx.taskDialogGeneration++;h.$('task-control-text').value='New draft';h.$('task-control-status').textContent='New status'}h.ack();await flush();assert.equal(h.$('task-control-dialog').open,newer);if(newer){assert.equal(h.$('task-control-text').value,'New draft');assert.equal(h.$('task-control-status').textContent,'New status')}h.refreshes[0].resolve();await sending});
console.log(JSON.stringify({scope:'Node VM actual handlers; fixture transport/DOM/storage, not browser acceptance',results},null,2));
