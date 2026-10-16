import fs from 'node:fs';
import path from 'node:path';
import vm from 'node:vm';
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {execFileSync} from 'node:child_process';
import {fileURLToPath} from 'node:url';

const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..');
const args=Object.fromEntries(process.argv.slice(2).flatMap((value,index,list)=>value.startsWith('--')?[[value.slice(2),list[index+1]]]:[]));
const output=path.resolve(args.output??'E:/CodexScratch/reconnect-mobile-20261013/green');
const owned=path.resolve('E:/CodexScratch/reconnect-mobile-20261013');
if(output!==owned&&!output.startsWith(owned+path.sep))throw Error('Owned reconnect evidence directory required');
fs.mkdirSync(output,{recursive:true});
const sourceCommit=args['source-commit']??execFileSync('git',['rev-parse','HEAD'],{cwd:root,encoding:'utf8'}).trim();
if(!/^[a-f0-9]{40}$/.test(sourceCommit))throw Error('Exact source commit required');
const source=args['source-commit']?execFileSync('git',['show',sourceCommit+':native/internal/web/assets/app.js'],{cwd:root,encoding:'utf8'}):fs.readFileSync(path.join(root,'native/internal/web/assets/app.js'),'utf8');
const sha=value=>createHash('sha256').update(value).digest('hex');
function fn(name){
 const start=source.search(new RegExp('^(?:async )?function '+name+'\\(','m'));
 if(start<0)throw Error('Missing actual function: '+name);
 const rest=source.slice(start),end=rest.slice(1).search(/\n(?:async )?function |\n(?:const |let |\$\(|window\.|document\.|for\()/);
 return end<0?rest:rest.slice(0,end+1);
}
const names=['cacheKey','cacheSet','cacheRemove','sessionCurrent','requireSession','fixtureSession','sessionOwner','laneKey','projectChoice','viewKey','inputView','strictKeys','canonicalJSON','validCoreProfileInput','matchingCoreProfileInput','messageInput','botDefaultMode','intakeTargetBot','matchingBotDefaultInput','matchingRequestPolicy','saveOutbox','acknowledged','rejectInput','sourceAccess','acceptContextWork','requestRaw','request','retryOutboxInput','resumeOutbox','renderOutbox','reconcile'];
for(const name of ['outboxDraftScope','clearAcknowledgedOutbox'])if(new RegExp('^function '+name+'\\(','m').test(source))names.push(name);

function fixture(){
 let context;
 const doc={activeElement:null,dialog:null,getElementById:id=>elements[id],createElement:tag=>new Element(tag),querySelector:selector=>selector==='dialog[open]'?doc.dialog:null};
 class Element{
  constructor(tag){this.tagName=tag;this.children=[];this.parentElement=null;this.hidden=false;this.disabled=false;this.inert=false;this.tabIndex=0;this.textContent='';this.value='';this.attributes=new Map();this.listeners=new Map();this.dataset={};this.focusOptions=[];}
  get firstChild(){return this.children[0]??null;}
  get nextSibling(){return this.parentElement?.children[this.parentElement.children.indexOf(this)+1]??null;}
  contains(other){return other===this||this.children.some(child=>child.contains(other));}
  append(...children){for(const child of children)this.insertBefore(child,null);}
  insertBefore(child,before){if(child.parentElement){const old=child.parentElement.children;old.splice(old.indexOf(child),1);}child.parentElement=this;const index=before===null?this.children.length:this.children.indexOf(before);this.children.splice(index,0,child);}
  remove(){if(this.contains(doc.activeElement))doc.activeElement=doc.body;const children=this.parentElement?.children;if(children)children.splice(children.indexOf(this),1);this.parentElement=null;}
  setAttribute(key,value){this.attributes.set(key,String(value));}
  getAttribute(key){return this.attributes.get(key)??null;}
  addEventListener(kind,callback){this.listeners.set(kind,callback);}
  focus(options){doc.activeElement=this;this.focusOptions.push(options);if(context)vm.runInContext('focusEpoch++',context);}
  click(){if(this.disabled||this.hidden)return Promise.resolve();return Promise.resolve(this.listeners.get('click')?.());}
 }
 const elements={};for(const id of ['outbox-notice','prompt','main','hint','other'])elements[id]=new Element(id);
 doc.body=new Element('body');doc.activeElement=doc.body;doc.body.append(elements.main);elements.main.append(elements['outbox-notice'],elements.prompt,elements.other);
 const posts=[],storage=new Map();
 const fetch=async(url,options)=>new Promise((resolve,reject)=>posts.push({url,body:options.body?JSON.parse(options.body):undefined,headers:options.headers,resolve:(status,value)=>resolve({status,ok:status>=200&&status<300,headers:{get:()=>options.headers?.['X-Neko-Session-Binding']??null},json:async()=>value}),reject}));
 context=vm.createContext({TextEncoder,JSON,Date,Number,Object,Array,Map,Set,AbortController,AbortSignal,document:doc,fetch,queueMicrotask,localStorage:{setItem:(key,value)=>storage.set(key,value),removeItem:key=>storage.delete(key)},window:{nekoCoreProfiles:{selection:()=>null}},$:(id)=>elements[id],node:(tag,className,text)=>{const el=doc.createElement(tag);el.className=className;el.textContent=text??'';return el;}});
 vm.runInContext(names.map(fn).join('\n')+`
 const sessionState={ready:true,generation:1,autoReplay:true,controllers:new Set(),metadata:{mode:'local_owner',actorId:'local-owner',sessionBinding:'local-owner',cacheNamespace:'local-owner'}};
 const state={botId:'bot-meiiie',channelId:'channel-original',accountChoice:{id:'account-original',revision:4},projectChoices:{'channel-original':{id:'project-original',revision:3}},capabilities:['account_lease_fixture_v1'],readOnly:false,bots:[{id:'bot-meiiie',alias:'meiiie'}],channels:[{id:'channel-original',selfRole:'writer'}],work:new Map([['req-old',{requestId:'req-old',status:'running'}]])};
 const outboxKey='neko.local-message-outbox.v1',outboxNodes=new Map(),inFlight=new Set(),views=new Map(),hints=[];let outbox=[],outboxReplay=false,navigationGeneration=1,focusEpoch=0;
 function setHint(text,error=false){hints.push({text,error})}function render(){renderOutbox()}function invalidateSession(){sessionState.ready=false;sessionState.generation++}function sync(){return Promise.resolve()}function retrySync(){}
 `,context);
 const run=expression=>vm.runInContext(expression,context);
 const original={botId:'bot-meiiie',text:'Original pending message',lane:'background',clientNonce:'fixture-reconnect-original-0001',channelId:'channel-original',expectedChannelRevision:7,accountId:'account-original',accountRevision:4,projectId:'project-original',projectRevision:3,fileIds:['file-original'],expectedBotProfileRevision:2,pluginSelection:[{id:'neko-project-sources',expectedRevision:5}],expectedContinuityRevision:6};
 function add(input=original){context.input=structuredClone(input);run('outbox.push(input);saveOutbox();renderOutbox()');return run('outboxNodes.get(input.clientNonce)');}
 function receipt(input=original){return {...structuredClone(input),requestId:'req-retried',status:'waiting_host',epoch:1,publication:{replyChannelId:input.channelId,audienceRevision:input.expectedChannelRevision},accountPin:input.accountId?{accountId:input.accountId,revision:input.accountRevision}:undefined,resources:{project:input.projectId?{id:input.projectId,revision:input.projectRevision}:undefined,requestedFileIds:[...(input.fileIds??[])].sort(),continuity:{revision:input.expectedContinuityRevision??0}},coreProfilePin:input.coreProfileSelection?{consentId:input.coreProfileSelection.consentId,consentRevision:input.coreProfileSelection.expectedConsentRevision}:undefined};}
 return {run,context,doc,elements,posts,storage,original,add,receipt};
}

const checks=[];
async function check(name,body){try{await body();checks.push({name,status:'PASS'});}catch(error){checks.push({name,status:'FAIL',reason:error.message.split('\n\n')[0]});process.exitCode=1;}}
const settle=async()=>{for(let i=0;i<8;i++)await Promise.resolve();};

await check('idle_original_message_remains_visible',async()=>{const f=fixture(),view=f.add();assert.equal(f.elements['outbox-notice'].hidden,false);assert.equal(view.preview.textContent,f.original.text);assert.equal(view.retry.disabled,false);});
await check('retry_keeps_visible_busy_row_without_composer_focus',async()=>{
 const f=fixture(),view=f.add();view.retry.focus();const pending=view.retry.click();
 assert.equal(f.posts.length,1);assert.equal(f.elements['outbox-notice'].hidden,false,'Pending row must remain visible during transport');assert.ok(view.row.parentElement===f.elements['outbox-notice'],'Original row must remain attached');assert.equal(view.row.getAttribute('aria-busy'),'true');assert.equal(view.retry.disabled,true);assert.equal(view.edit.disabled,true);assert.equal(view.discard.disabled,true);assert.ok(f.doc.activeElement===view.row,'Retry must focus status row, preserving mobile keyboard state');assert.equal(view.row.focusOptions.at(-1)?.preventScroll,true);assert.match(view.description.textContent,/Đang chờ xác nhận/);
 f.posts[0].reject(Error('offline'));await pending;await settle();
});
await check('failed_retry_restores_same_retry_control',async()=>{
 const f=fixture(),view=f.add();view.retry.focus();const pending=view.retry.click();f.posts[0].reject(Error('offline'));await pending;await settle();
 assert.equal(f.run('outbox.length'),1);assert.equal(view.row.getAttribute('aria-busy'),'false');assert.equal(view.retry.disabled,false);assert.equal(f.doc.activeElement,view.retry);assert.equal(f.elements.prompt.value,'');assert.equal(f.posts.length,1);
});
await check('retry_preserves_all_original_pins_and_single_nonce',async()=>{
 const f=fixture(),view=f.add();view.retry.focus();const pending=view.retry.click();await f.run('retryOutboxInput(input)');assert.equal(f.posts.length,1);assert.deepEqual(f.posts[0].body,f.original);f.posts[0].resolve(201,{work:f.receipt()});await pending;await settle();assert.equal(f.run('outbox.length'),0);assert.equal(f.run('state.work.get("req-old").status'),'running');
});
await check('conflict_keeps_reviewable_original_and_does_not_resubmit',async()=>{
 const f=fixture(),view=f.add();const pending=view.retry.click();f.posts[0].resolve(409,{});await pending;await settle();assert.equal(f.run('outbox[0].rejection'),409);assert.equal(view.retry.hidden,true);assert.equal(view.edit.hidden,false);assert.equal(view.discard.disabled,false);await f.run('retryOutboxInput(input)');assert.equal(f.posts.length,1);
});
await check('failed_retry_does_not_steal_other_focus',async()=>{const f=fixture(),view=f.add();const pending=view.retry.click();f.elements.other.focus();f.posts[0].reject(Error('offline'));await pending;await settle();assert.equal(f.doc.activeElement,f.elements.other);});
await check('failed_retry_does_not_steal_focus_after_navigation',async()=>{const f=fixture(),view=f.add();const pending=view.retry.click();f.run('navigationGeneration++;state.channelId="channel-other";renderOutbox()');f.elements.other.focus();f.posts[0].reject(Error('offline'));await pending;await settle();assert.equal(f.doc.activeElement,f.elements.other);assert.equal(f.run('outbox.length'),1);});
await check('mobile_inert_main_prevents_retry_focus_return',async()=>{const f=fixture(),view=f.add();const pending=view.retry.click();f.elements.main.inert=true;f.posts[0].reject(Error('offline'));await pending;await settle();assert.equal(f.doc.activeElement,view.row);});
await check('open_dialog_prevents_retry_focus_return',async()=>{const f=fixture(),view=f.add();const pending=view.retry.click();f.doc.dialog={open:true};f.posts[0].reject(Error('offline'));await pending;await settle();assert.equal(f.doc.activeElement,view.row);});
await check('late_ack_does_not_clear_draft_after_scope_change',async()=>{const f=fixture(),view=f.add();f.elements.prompt.value=f.original.text;const pending=view.retry.click();f.run('navigationGeneration++;state.botId="bot-davin";state.accountChoice={id:"account-new",revision:1};state.projectChoices["channel-original"]={id:"project-new",revision:1}');f.elements.prompt.value='Keep new scoped draft';f.elements.other.focus();f.posts[0].resolve(201,{work:f.receipt()});await pending;await settle();assert.equal(f.elements.prompt.value,'Keep new scoped draft');assert.equal(f.doc.activeElement,f.elements.other);assert.deepEqual(f.posts[0].body,f.original);});
await check('session_change_fences_late_ack',async()=>{const f=fixture(),view=f.add();const pending=view.retry.click();f.run('sessionState.generation++;sessionState.ready=false;outbox=[];outboxNodes.clear();inFlight.clear()');f.elements.other.focus();f.posts[0].resolve(201,{work:f.receipt()});await pending;await settle();assert.equal(f.run('state.work.has("req-retried")'),false);assert.equal(f.doc.activeElement,f.elements.other);});
await check('read_only_pending_retry_never_reaches_transport',async()=>{const f=fixture();f.run('state.readOnly=true');const view=f.add();assert.equal(view.retry.disabled,true);await f.run('retryOutboxInput(input)');assert.equal(f.posts.length,0);assert.equal(f.run('outbox.length'),1);});
await check('fixture_reader_cannot_retry_shared_message',async()=>{const f=fixture();f.run('sessionState.metadata.mode="fixture";sessionState.metadata.actorId="fixture-reader";state.channels[0].selfRole="reader"');const view=f.add();assert.equal(view.retry.disabled,true);await f.run('retryOutboxInput(input)');assert.equal(f.posts.length,0);});
await check('core_profile_retry_keeps_consent_pin_and_no_account',async()=>{const f=fixture(),input={...f.original,coreProfileSelection:{consentId:'core-consent-'+'a'.repeat(32),expectedConsentRevision:8}};delete input.accountId;delete input.accountRevision;const view=f.add(input),pending=view.retry.click();assert.deepEqual(f.posts[0].body,input);f.posts[0].resolve(201,{work:f.receipt(input)});await pending;await settle();assert.equal(f.run('outbox.length'),0);});
await check('core_profile_pending_is_never_automatically_replayed',async()=>{const f=fixture(),input={...f.original,coreProfileSelection:{consentId:'core-consent-'+'a'.repeat(32),expectedConsentRevision:8}};delete input.accountId;delete input.accountRevision;f.add(input);await f.run('resumeOutbox()');assert.equal(f.posts.length,0);assert.equal(f.run('outbox.length'),1);});
await check('mismatched_ack_does_not_retire_original',async()=>{const f=fixture(),view=f.add(),pending=view.retry.click(),work=f.receipt();work.publication.audienceRevision=8;f.posts[0].resolve(201,{work});await pending;await settle();assert.equal(f.run('outbox.length'),1);assert.equal(view.retry.disabled,false);});
await check('reconnect_ack_clears_only_original_live_and_saved_draft',async()=>{const f=fixture();f.add();f.run('state.pending=input;views.set(viewKey(),{draft:input.text,generation:navigationGeneration})');f.elements.prompt.value=f.original.text;f.elements.other.focus();const pending=f.run('resumeOutbox()');f.posts[0].resolve(201,{work:f.receipt()});await pending;assert.equal(f.run('outbox.length'),0);assert.equal(f.elements.prompt.value,'');assert.equal(f.run('views.get(viewKey()).draft'),'');assert.ok(f.doc.activeElement===f.elements.other,'Automatic replay must not take focus');});
await check('reconnect_ack_clears_exact_original_pending',async()=>{const f=fixture();f.add();f.run('state.pending=input');const pending=f.run('resumeOutbox()');f.posts[0].resolve(201,{work:f.receipt()});await pending;assert.equal(f.run('state.pending'),null);});
await check('reconnect_ack_preserves_new_live_and_saved_draft',async()=>{const f=fixture();f.add();f.elements.prompt.value=f.original.text;f.run('views.set(viewKey(),{draft:input.text,generation:navigationGeneration})');const pending=f.run('resumeOutbox()');f.elements.prompt.value='Edited while reconnecting';f.run('views.set(viewKey(),{draft:"Edited while reconnecting",generation:navigationGeneration});state.pending={...input,text:"New pending message",clientNonce:"fixture-new-pending-0002"}');f.posts[0].resolve(201,{work:f.receipt()});await pending;assert.equal(f.elements.prompt.value,'Edited while reconnecting');assert.equal(f.run('views.get(viewKey()).draft'),'Edited while reconnecting');assert.equal(f.run('state.pending.clientNonce'),'fixture-new-pending-0002');});
await check('reconnect_late_navigation_preserves_original_saved_and_new_live_draft',async()=>{const f=fixture();f.add();f.elements.prompt.value=f.original.text;f.run('views.set(viewKey(),{draft:input.text,generation:navigationGeneration})');const key=f.run('viewKey()'),pending=f.run('resumeOutbox()');f.run('navigationGeneration++;state.botId="bot-davin";state.channelId="channel-other"');f.elements.prompt.value='New navigation draft';f.elements.other.focus();f.posts[0].resolve(201,{work:f.receipt()});await pending;assert.equal(f.elements.prompt.value,'New navigation draft');assert.equal(f.run('views.get('+JSON.stringify(key)+').draft'),f.original.text);assert.ok(f.doc.activeElement===f.elements.other);});
await check('mismatched_reconnect_ack_keeps_draft_and_pending',async()=>{const f=fixture();f.add();f.elements.prompt.value=f.original.text;f.run('state.pending=input;views.set(viewKey(),{draft:input.text,generation:navigationGeneration})');const pending=f.run('resumeOutbox()'),work=f.receipt();work.resources.project.revision++;f.posts[0].resolve(201,{work});await pending;assert.equal(f.run('outbox.length'),1);assert.equal(f.elements.prompt.value,f.original.text);assert.equal(f.run('state.pending.clientNonce'),f.original.clientNonce);});
await check('manual_mismatched_ack_keeps_original_draft',async()=>{const f=fixture(),view=f.add();f.elements.prompt.value=f.original.text;f.run('views.set(viewKey(),{draft:input.text,generation:navigationGeneration})');const pending=view.retry.click(),work=f.receipt();work.accountPin.revision++;f.posts[0].resolve(201,{work});await pending;await settle();assert.equal(f.elements.prompt.value,f.original.text);assert.equal(f.run('views.get(viewKey()).draft'),f.original.text);});
await check('same_nonce_different_pending_scope_is_preserved',async()=>{const f=fixture();f.add();f.run('state.pending={...input,projectRevision:9}');const pending=f.run('resumeOutbox()');f.posts[0].resolve(201,{work:f.receipt()});await pending;assert.equal(f.run('state.pending.projectRevision'),9);});
await check('account_choice_change_without_navigation_preserves_draft',async()=>{const f=fixture();f.add();f.elements.prompt.value=f.original.text;const pending=f.run('resumeOutbox()');f.run('state.accountChoice={id:"account-new",revision:1}');f.posts[0].resolve(201,{work:f.receipt()});await pending;assert.equal(f.elements.prompt.value,f.original.text);});
await check('new_matching_text_after_replay_started_is_preserved',async()=>{const f=fixture();f.add();f.elements.prompt.value='Different draft at replay start';const pending=f.run('resumeOutbox()');f.elements.prompt.value=f.original.text;f.posts[0].resolve(201,{work:f.receipt()});await pending;assert.equal(f.elements.prompt.value,f.original.text);});
await check('session_change_stops_old_automatic_queue',async()=>{const f=fixture();f.add();f.run('outbox.push({...input,clientNonce:"fixture-second-original-0002"})');const pending=f.run('resumeOutbox()');f.run('sessionState.generation++;outbox=[];outboxNodes.clear();inFlight.clear()');f.elements.other.focus();f.posts[0].resolve(201,{work:f.receipt()});await pending;assert.equal(f.posts.length,1);assert.equal(f.run('state.work.has("req-retried")'),false);assert.ok(f.doc.activeElement===f.elements.other);});
await check('same_text_new_pending_before_replay_preserves_live_and_saved_draft',async()=>{const f=fixture();f.add();f.elements.prompt.value=f.original.text;f.run('views.set(viewKey(),{draft:input.text,generation:navigationGeneration});state.pending={...input,clientNonce:"fixture-new-same-text-0002"}');const pending=f.run('resumeOutbox()');f.posts[0].resolve(201,{work:f.receipt()});await pending;assert.equal(f.run('outbox.length'),0);assert.equal(f.run('state.pending.clientNonce'),'fixture-new-same-text-0002');assert.equal(f.elements.prompt.value,f.original.text);assert.equal(f.run('views.get(viewKey()).draft'),f.original.text);});
await check('same_text_new_pending_during_replay_preserves_live_and_saved_draft',async()=>{const f=fixture();f.add();f.elements.prompt.value=f.original.text;f.run('views.set(viewKey(),{draft:input.text,generation:navigationGeneration});state.pending=input');const pending=f.run('resumeOutbox()');f.run('state.pending={...input,clientNonce:"fixture-new-same-text-0002"}');f.posts[0].resolve(201,{work:f.receipt()});await pending;assert.equal(f.run('outbox.length'),0);assert.equal(f.run('state.pending.clientNonce'),'fixture-new-same-text-0002');assert.equal(f.elements.prompt.value,f.original.text);assert.equal(f.run('views.get(viewKey()).draft'),f.original.text);});

const result={status:checks.every(value=>value.status==='PASS')?'PASS':'FAIL',sourceCommit,sourceSelection:args['source-commit']?'immutable Git blob':'working source at recorded HEAD plus application hash',command:process.argv.join(' '),environment:{platform:process.platform,node:process.version,execution:'Node VM with minimal DOM adapter and synthetic transport; no browser, native window, physical phone, provider or cloud'},applicationSHA256:sha(source),harnessSHA256:sha(fs.readFileSync(fileURLToPath(import.meta.url))),checks};
fs.writeFileSync(path.join(output,'result.json'),JSON.stringify(result,null,2)+'\n');
console.log(JSON.stringify(result,null,2));
