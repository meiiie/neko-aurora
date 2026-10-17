// Run: node app/scripts/ui_group_confirmed_refresh.mjs
// Actual composer/create-group handlers; deferred transport and minimal DOM/storage boundaries.
// No browser, service, native runtime, provider, credentials, or network.
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {randomUUID} from 'node:crypto';
import vm from 'node:vm';
const source=readFileSync(new URL('../native/internal/web/assets/app.js',import.meta.url),'utf8');
const results=[];
const extract=name=>{
 let start=source.indexOf('function '+name+'(');
 assert.ok(start>=0, name+' source exists');
 if(source.slice(start-6,start)==='async ')start-=6;
 const next=source.slice(start+1).search(/\n(?:async )?function /);
 return source.slice(start,next<0?source.length:start+1+next);
};
const flush=async()=>{for(let i=0;i<20;i++)await Promise.resolve()};
async function scenario(name,test){
 const nodes=new Map(),calls=[],refreshes=[],accepted=[],retired=[],hints=[];
 const body={goal:'Shared goal',channelId:'channel-fixture',expectedChannelRevision:1,coordinatorBotId:'bot-a',participants:['bot-a','bot-b'],maxStages:3};
 const text='@a @b Shared goal';
 const $=id=>{if(!nodes.has(id))nodes.set(id,{value:'',disabled:false,open:false,listeners:{},addEventListener(event,fn){this.listeners[event]=fn},close(){this.open=false}});return nodes.get(id)};
 const context=vm.createContext({$,crypto:{randomUUID},window:{},state:{botId:'bot-a'},navigationGeneration:0,key:'view-a',views:new Map(),groupCommands:new Map(),groupDialogGeneration:0,groupDialogCommand:null,groupDialogReturn:null,
  viewKey:()=>context.key,render(){},setHint:t=>hints.push(t),canonicalJSON:JSON.stringify,
  sessionState:{generation:0},sessionCurrent:()=>true,tagCollaborationIntent:()=>({body}),
  loadDurableCommands:async()=>{},restoreDurableMaps(){},
  retainDurableCommand:async(kind,command)=>{command.record??={input:command.body}},
  prepareDurableRetry:async()=>true,journalGroupMatches:()=>true,durableFailure:()=>false,
  retireDurableCommand:async command=>{retired.push(command.body.clientNonce);return true},
  acceptCollaboration:g=>accepted.push(g),
  request:(path,input)=>new Promise((resolve,reject)=>calls.push({path,body:input,resolve,reject})),
  sync:()=>new Promise((resolve,reject)=>refreshes.push({resolve,reject})),retrySync:()=>{context.reconnects++},reconnects:0
 });
 for(const name of ['submitTaggedCollaboration','createGroup'])vm.runInContext(extract(name),context);
 const start=source.indexOf('$("composer").addEventListener("submit",async event=>{'),end=source.indexOf('\n});',start);
 assert.ok(start>=0&&end>start);vm.runInContext(source.slice(start,end+4),context);
 $('prompt').value=text;context.views.set('view-a',{draft:text,generation:0});
 const h={context,$,calls,refreshes,accepted,retired,hints,text,body,
  send:()=>$('composer').listeners.submit({preventDefault(){}}),
  ack:index=>{const call=calls[index];call.resolve({collaboration:{...call.body,id:'collab-fixture'}})},
  fail:index=>calls[index].reject(Error('network')),
  retry:()=>context.createGroup([...context.groupCommands.values()][0]),flush};
 try{await test(h);results.push({name,status:'PASS'})}catch(error){results.push({name,status:'FAIL',error:error.message})}
}
await scenario('confirmed POST survives failed refresh and second Send does not duplicate group',async h=>{
 const sending=h.send();await h.flush();h.ack(0);await h.flush();
 h.refreshes[0].reject(Error('snapshot unavailable'));await sending;await h.flush();
 assert.equal(h.$('prompt').value,'','confirmed submission clears owned composer');
 assert.equal(h.context.views.get('view-a').draft,'');
 assert.equal(h.context.groupCommands.size,0);assert.equal(h.accepted.length,1);assert.equal(h.retired.length,1);
 assert.equal(h.context.reconnects,1,'failed refresh uses reconnect path');
 await h.send();assert.equal(h.calls.length,1,'empty confirmed composer must not send another group');
});
await scenario('confirmed result and draft cleanup do not wait for a hanging snapshot',async h=>{
 let done=false;const sending=h.send().then(()=>{done=true});await h.flush();h.ack(0);await h.flush();
 assert.equal(done,true,'intake acknowledgement completes independently of snapshot');
 assert.equal(h.$('prompt').value,'');await h.send();assert.equal(h.calls.length,1);
 h.refreshes[0].resolve();await sending;
});
await scenario('true POST failure retains original command and nonce for retry',async h=>{
 const sending=h.send();await h.flush();h.fail(0);await sending;
 assert.equal(h.$('prompt').value,h.text);assert.equal(h.context.groupCommands.size,1);
 assert.equal(h.retired.length,0);assert.equal(h.refreshes.length,0);
 const command=[...h.context.groupCommands.values()][0];assert.equal(command.retry,true);
 await h.send();assert.equal(h.calls.length,1,'Send must not mint a nonce for unresolved intake');
 const retry=h.retry();await h.flush();assert.equal(h.calls.length,2);
 assert.equal(h.calls[1].body.clientNonce,h.calls[0].body.clientNonce);h.ack(1);await h.flush();h.refreshes[0].resolve();await retry;
});
await scenario('duplicate Send and retry while POST pending produce one mutation',async h=>{
 const sending=h.send();await h.flush();await h.send();await h.retry();assert.equal(h.calls.length,1);
 h.ack(0);await h.flush();h.refreshes[0].reject(Error('snapshot unavailable'));await sending;
 assert.equal(h.$('prompt').value,'');assert.equal(h.calls.length,1);
});
await scenario('newer composer and saved draft survive old acknowledgement and failed refresh',async h=>{
 const sending=h.send();await h.flush();h.$('prompt').value='Newer draft';h.context.views.get('view-a').draft='Newer draft';
 h.ack(0);await h.flush();h.refreshes[0].reject(Error('snapshot unavailable'));await sending;
 assert.equal(h.$('prompt').value,'Newer draft');assert.equal(h.context.views.get('view-a').draft,'Newer draft');
 assert.equal(h.accepted.length,1);
});
await scenario('navigation preserves destination draft while clearing only owned origin draft',async h=>{
 const sending=h.send();await h.flush();h.context.key='view-b';h.context.navigationGeneration++;h.context.state.botId='bot-b';h.$('prompt').value='Other conversation';h.context.views.set('view-b',{draft:'Other conversation',generation:1});
 h.ack(0);await h.flush();h.refreshes[0].reject(Error('snapshot unavailable'));await sending;
 assert.equal(h.$('prompt').value,'Other conversation');assert.equal(h.context.views.get('view-b').draft,'Other conversation');
 assert.equal(h.context.views.get('view-a').draft,'','confirmed origin draft is retired');
});
await scenario('dialog duplicate button remains blocked and confirmed result closes owned dialog despite refresh failure',async h=>{
 const command={body:{...h.body,clientNonce:randomUUID()},key:'view-a'};
 h.context.groupCommands.set(command.body.clientNonce,command);h.context.groupDialogCommand=command;
 h.context.groupDialogReturn={key:'view-a',generation:0,draft:h.body.goal};h.$('prompt').value=h.body.goal;h.$('collaboration-dialog').open=true;
 const sending=h.context.createGroup(command,0);await h.flush();assert.equal(h.$('collaboration-save').disabled,true);
 await h.context.createGroup(command,0);assert.equal(h.calls.length,1);
 h.ack(0);await h.flush();h.refreshes[0].reject(Error('snapshot unavailable'));const result=await sending;await h.flush();
 assert.equal(result?.clientNonce,command.body.clientNonce);assert.equal(h.$('collaboration-dialog').open,false);assert.equal(h.$('prompt').value,'');
 assert.equal(command.retry,false);assert.equal(command.pending,false);assert.equal(h.$('collaboration-save').disabled,false);
});
console.log(JSON.stringify({scope:'Actual composer, submitTaggedCollaboration and createGroup in Node VM; boundary fixtures only; no browser/native/provider acceptance',results},null,2));
if(results.some(r=>r.status==='FAIL'))process.exitCode=1;
