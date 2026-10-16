import vm from 'node:vm';
import path from 'node:path';
import {readFile, mkdir, writeFile} from 'node:fs/promises';
import {execFileSync} from 'node:child_process';
import {createHash, webcrypto} from 'node:crypto';
import {fileURLToPath} from 'node:url';

const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..');
const opts=Object.fromEntries(process.argv.slice(2).flatMap((v,i,a)=>v.startsWith('--')?[[v.slice(2),a[i+1]]]:[]));
if(!/^[a-f0-9]{40}$/.test(opts.commit??''))throw Error('--commit must be an immutable full Git commit');
const scratch='E:/CodexScratch/ux-independent-20261013';
const output=path.resolve(opts.output??scratch).replaceAll('\\','/');
if(output!==scratch&&!output.startsWith(scratch+'/'))throw Error('Owned independent UX evidence directory required');
const label=opts.label??opts.commit.slice(0,12);
if(!/^[a-zA-Z0-9_-]{1,80}$/.test(label))throw Error('Invalid evidence label');
const sha=bytes=>createHash('sha256').update(bytes).digest('hex');
const readObject=filename=>execFileSync('git',['show',opts.commit+':'+filename],{cwd:root,encoding:'utf8',maxBuffer:8*1024*1024});
const app=readObject('native/internal/web/assets/app.js');
const connections=readObject('native/internal/web/assets/connections.js');
const html=readObject('native/internal/web/assets/index.html');
const result={sourceCommit:opts.commit,sourceMode:'immutable Git objects; working tree never evaluated',sourceSHA256:{app:sha(app),connections:sha(connections),html:sha(html)},harnessSHA256:sha(await readFile(fileURLToPath(import.meta.url))),command:process.argv,environment:{node:process.version,platform:process.platform,arch:process.arch},scope:'Actual source functions and Connections module in bounded Node VM with typed public fixtures and minimal DOM. No backend, Core, provider, grants, browser, screenshots, native Tauri, accessibility/layout, phone or cloud acceptance.',checks:[],requests:[]};
const blocked=message=>{const error=Error(message);error.scopeBlocked=true;throw error};
const check=async(name,fn)=>{try{const evidence=await fn();if(evidence!==true)throw Error(typeof evidence==='string'?evidence:'assertion');result.checks.push({name,status:'PASS'})}catch(error){result.checks.push({name,status:error.scopeBlocked?'BLOCKED':'FAIL',error:error.message});if(!error.scopeBlocked)process.exitCode=1}};
function extract(source,name){
 const lines=source.split('\n'),at=lines.findIndex(line=>new RegExp('^\\s*(?:async )?function '+name+'\\(').test(line));
 if(at<0)throw Error('Missing actual source function '+name);
 if(lines[at].trimEnd().endsWith('}'))return lines[at];
 const indent=lines[at].match(/^\s*/)[0],end=lines.findIndex((line,i)=>i>at&&line.startsWith(indent+'}')&&line.slice(indent.length).match(/^}\s*$/));
 if(end<0)throw Error('Unbounded source extraction '+name);
 return lines.slice(at,end+1).join('\n');
}
function load(ctx,source,names){for(const name of names)vm.runInContext(extract(source,name),ctx,{timeout:1000,filename:name+'.js'})}
const run=(ctx,code)=>vm.runInContext(code,ctx,{timeout:1000});
const settle=async()=>{for(let i=0;i<16;i++)await Promise.resolve()};
function dom(){
 const nodes=new Map();let document;
 function el(tag='div',text=''){
  const listeners=new Map(),attrs=new Map();
  const value={tagName:tag.toUpperCase(),textContent:text??'',value:'',checked:false,disabled:false,hidden:false,open:false,children:[],dataset:{},style:{},options:[],elements:{},inert:false,
   classList:{add(){},remove(){},toggle(){},contains(){return false}},
   append(...items){this.children.push(...items);if(tag==='select')this.options=this.children},replaceChildren(...items){this.children=[...items];this.options=tag==='select'?this.children:this.options;if(tag==='select'&&!items.some(e=>e.value===this.value))this.value=items[0]?.value??''},
   setAttribute(k,v){attrs.set(k,String(v))},getAttribute(k){return attrs.get(k)??null},removeAttribute(k){attrs.delete(k)},querySelectorAll(){return []},querySelector(selector){return selector==='.connection-tabs'?get('connection-tabs'):null},contains(target){return target===this||this.children.some(c=>c.contains?.(target))},after(){},
   addEventListener(type,fn){if(!listeners.has(type))listeners.set(type,[]);listeners.get(type).push(fn)},dispatch(type){for(const fn of listeners.get(type)??[])fn({target:this,key:'',preventDefault(){}})},click(){if(!this.disabled)this.dispatch('click')},
   focus(){document.activeElement=this},showModal(){this.open=true},close(){this.open=false;this.dispatch('close')},reset(){for(const field of Object.values(this.elements))field.value=''}
  };return value;
 }
 const get=id=>{if(!nodes.has(id)){const match=html.match(new RegExp('<([a-z]+)[^>]*\\bid="'+id+'"[^>]*>')),e=el(match?.[1]??'div');e.id=id;e.hidden=/\bhidden(?:\s|>)/.test(match?.[0]??'');e.disabled=/\bdisabled(?:\s|>)/.test(match?.[0]??'');e.value=match?.[0].match(/\bvalue="([^"]*)"/)?.[1]??'';nodes.set(id,e)}return nodes.get(id)};
 document={activeElement:null,body:el('body'),getElementById:get,createElement:el,querySelectorAll(){return []},addEventListener(){}};document.activeElement=document.body;
 get('account-form').elements=Object.fromEntries(['name','provider','endpoint','model','secret'].map(k=>[k,get(k==='secret'?'account-secret':k==='provider'?'account-provider':'form-'+k)]));
 get('core-profile-template').content={cloneNode:()=>el('fragment')};
 return {$:get,node:(tag,className,text)=>el(tag,text),document,nodes};
}
const channel='channel-'+ 'a'.repeat(32),reportId='report-delivery-'+ 'b'.repeat(32),requestId='request-independent-ux';
const bot={id:'bot-meiiie',name:'Meiiie',alias:'meiiie',color:'#567878'};
const publicSource={version:1,id:'installed-core',revision:2,label:'Existing Core profile source',runtimeSha256:'c'.repeat(64),fixtureOnly:false};
function connectionContext({sourceList=[publicSource],owner=true,readOnly=false,capability=true}={}){
 const d=dom(),calls=[],pending=[];
 const state={botId:bot.id,channelId:'',accountChoice:null,readOnly,capabilities:capability?['local_core_profiles_v1','account_save_receipts_v1']:[],bots:[bot],accounts:[]};
 const ctx=vm.createContext({...d,state,window:{},TextEncoder,URL,URLSearchParams,AbortController,crypto:webcrypto,navigationGeneration:0,sessionState:{ready:true,generation:1,metadata:{mode:owner?'local_owner':'fixture',actorId:owner?'local-owner':'fixture-actor'}},sessionOwner:()=>ctx.sessionState.ready&&ctx.sessionState.metadata.mode==='local_owner',fixtureSession:()=>ctx.sessionState.metadata.mode==='fixture',sessionCurrent:epoch=>ctx.sessionState.ready&&epoch===ctx.sessionState.generation,viewKey:()=>ctx.state.channelId||'conversation-'+ctx.state.botId,activeChannel:()=>null,privateConversation:()=>({id:'conversation-'+bot.id,botId:bot.id,title:'Private local conversation',revision:1}),laneKey:()=> 'conversation-'+bot.id,selectedProject:()=>null,sharedChannel:()=>null,botState:()=>({archived:false}),protectWorkControlFocus(){},containFocus(){},closeNavigation(){},setWorkPanel(){},renderPluginCatalog(){},importPluginFile(){},renderRouting(){},cacheKey:k=>k,cacheGet:()=>null,cacheSet(){throw Error('Unexpected cache write during source-only opening')},cacheRemove(){},commandDigest:async v=>sha(JSON.stringify(v))});
 ctx.requestRaw=(url,body)=>{calls.push({url,method:body===undefined?'GET':'POST'});if(body!==undefined)throw Error('Unexpected write during Connections source-only opening');if(url==='/v1/core-profile-sources'){if(ctx.holdSources)return new Promise(resolve=>pending.push(resolve));return Promise.resolve({version:1,sources:sourceList})}if(url==='/v1/core-discovery')return Promise.resolve({version:1,mode:'read_only_discovery',candidates:[],execution:'not_started',credentialAccess:'not_requested'});if(url==='/v1/connections')return Promise.resolve({version:1,revision:0,accounts:[],capabilities:state.capabilities});throw Error('Unexpected GET '+url)};
 load(ctx,app,['strictKeys','journalSensitive','journalCounter','canonicalJSON']);ctx.journalNonce=v=>typeof v==='string'&&/^[a-zA-Z0-9_-]{8,128}$/.test(v);
 vm.runInContext(connections,ctx,{timeout:1000,filename:'immutable-connections.js'});
 return {ctx,d,calls,pending,sourceList};
}
await check('connections_first_open_preloads_source_step_without_consent',async()=>{const c=connectionContext();c.d.$('connections-open').click();await settle();result.requests.push({story:'first_connections',calls:c.calls});return c.calls.filter(v=>v.url==='/v1/core-profile-sources').length===1&&!c.d.$('core-profile-panel').hidden&&c.d.$('core-profile-source').value===publicSource.id||'No single automatic sources GET / source step / sole descriptor selection'});
await check('connections_open_never_reads_profiles_or_creates_consent',async()=>{const c=connectionContext();c.d.$('connections-open').click();await settle();return c.calls.every(v=>v.method==='GET'&&!v.url.startsWith('/v1/core-profiles')&&!v.url.includes('consents'))});
await check('connections_first_multi_source_keeps_choice_explicit',async()=>{const c=connectionContext({sourceList:[publicSource,{...publicSource,id:'second-source',label:'Second Core source'}]});c.d.$('connections-open').click();await settle();return c.calls.some(v=>v.url==='/v1/core-profile-sources')&&c.d.$('core-profile-source').value===''||'Multi-source preload missing or a descriptor was selected automatically'});
await check('connections_fixture_and_readonly_do_not_preload_sources',async()=>{for(const options of [{owner:false},{readOnly:true},{capability:false}]){const c=connectionContext(options);c.d.$('connections-open').click();await settle();if(c.calls.some(v=>v.url==='/v1/core-profile-sources'||v.method==='POST'))return false}return true});
await check('connections_pending_preload_render_and_button_single_get',async()=>{const c=connectionContext();c.ctx.holdSources=true;c.d.$('connections-open').click();await settle();if(!c.pending.length)blocked('Automatic source GET absent; preload dedup story is not exercised');c.ctx.window.nekoConnections.syncView();c.ctx.window.nekoCoreProfiles.syncView();c.d.$('core-use-installed').click();await settle();return c.calls.filter(v=>v.url==='/v1/core-profile-sources').length===1});
await check('connections_late_get_after_navigation_cannot_select_source',async()=>{const c=connectionContext();c.ctx.holdSources=true;c.d.$('connections-open').click();await settle();if(!c.pending.length)blocked('Automatic source GET absent; stale-navigation story is not exercised');c.ctx.navigationGeneration++;c.ctx.state.botId='bot-davin';c.ctx.window.nekoConnections.syncView();for(const resolve of c.pending)resolve({version:1,sources:c.sourceList});await settle();return c.d.$('core-profile-source').value!==publicSource.id&&c.ctx.window.nekoCoreProfiles.selection()===null});
await check('connections_late_get_after_close_cannot_reopen_panel',async()=>{const c=connectionContext();c.ctx.holdSources=true;c.d.$('connections-open').click();await settle();if(!c.pending.length)blocked('Automatic source GET absent; close story is not exercised');c.d.$('connections-dialog').close();for(const resolve of c.pending)resolve({version:1,sources:c.sourceList});await settle();return !c.d.$('connections-dialog').open&&c.d.$('core-profile-source').value!==publicSource.id&&c.ctx.window.nekoCoreProfiles.selection()===null});
await check('connections_late_get_after_session_change_cannot_select_source',async()=>{const c=connectionContext();c.ctx.holdSources=true;c.d.$('connections-open').click();await settle();if(!c.pending.length)blocked('Automatic source GET absent; session-change story is not exercised');c.ctx.sessionState.generation++;c.ctx.window.nekoConnections.sessionChanged();c.ctx.window.nekoCoreProfiles.sessionChanged();for(const resolve of c.pending)resolve({version:1,sources:c.sourceList});await settle();return c.d.$('core-profile-source').value!==publicSource.id&&c.ctx.window.nekoCoreProfiles.selection()===null});
await check('connections_late_get_after_authority_change_cannot_select_source',async()=>{for(const change of [c=>{c.ctx.state.readOnly=true},c=>{c.ctx.state.capabilities=[]},c=>{c.ctx.sessionState.metadata={mode:'fixture',actorId:'fixture-actor'}}]){const c=connectionContext();c.ctx.holdSources=true;c.d.$('connections-open').click();await settle();if(!c.pending.length)blocked('Automatic source GET absent; authority-change story is not exercised');change(c);c.ctx.window.nekoConnections.syncView();c.ctx.window.nekoCoreProfiles.syncView();for(const resolve of c.pending)resolve({version:1,sources:c.sourceList});await settle();if(c.d.$('core-profile-source').value===publicSource.id||c.ctx.window.nekoCoreProfiles.selection()!==null)return false}return true});

function sourceContext(){
 const d=dom();d.$('work-sources-dialog').open=true;
 const work={requestId,botId:bot.id,conversationId:channel,contextPin:{version:5,atSequence:20,publishedReports:[{deliveryId:reportId,sequence:17,destinationChannelId:channel,audienceRevision:2}]}};
 const target={id:requestId,channelId:channel,projectId:'',sequence:20,key:channel,navigation:4};
 const metadata={version:1,requestId,channelId:channel,atSequence:20,usable:true,omitted:0,sources:[{id:reportId,kind:'published_report',reason:'owner_published_result_destination_only',pinnedRevision:17,currentRevision:17,usable:true}]};
 const state={botId:bot.id,channelId:channel,sequence:20,projects:[],channels:[{id:channel,revision:2}],projectChoices:{},conversations:[],files:[],work:new Map([[requestId,work]]),reportDeliveries:new Map([[reportId,{id:reportId,createdSequence:17,active:true,status:'delivered',destinationChannelId:channel,expectedDestinationRevision:2,reply:'Synthetic public report'}]])};
 const ctx=vm.createContext({...d,state,TextEncoder,URLSearchParams,navigationGeneration:4,workSourcesTarget:target,workSourcesData:null,workSourcesReadGeneration:0,botProfiles:new Map(),pluginRecords:new Map(),continuityDescriptors:new Map(),continuityKey:(p,c)=>p+'/'+c,resourcePinAvailable:()=>true,request:async()=>metadata,metadata,target});
 load(ctx,app,['laneKey','projectChoice','viewKey','belongsToLane','belongsToView','sourceAccess','journalSensitive','parseSourceExplanation','sourceExplanationURL','sourceStateSignature','sourceReasonText','workSourcesCurrent','invalidateWorkSources','checkWorkSourcesScope','readWorkSources','fenceReportDeliveries','groupSourceWork']);
 return {ctx,d,state,work,target,metadata};
}
await check('sources_accept_valid_public_destination_and_strip_private_extras',async()=>{const c=sourceContext();c.ctx.metadata={...c.metadata,contextPin:{private:'forbidden-private-fixture'},sources:[{...c.metadata.sources[0],name:'forbidden-private-fixture',reply:'forbidden-private-fixture',source:{conversationId:'private'}}]};const parsed=run(c.ctx,'parseSourceExplanation(metadata,target)');return !!parsed&&!JSON.stringify(parsed).includes('forbidden-private-fixture')});
await check('sources_wrong_destination_and_older_sequence_rejected',async()=>{const c=sourceContext();return run(c.ctx,'parseSourceExplanation({...metadata,channelId:"different-channel"},target)===null&&parseSourceExplanation({...metadata,atSequence:19},target)===null')});
await check('sources_real_typed_get_renders_public_metadata',async()=>{const c=sourceContext();await run(c.ctx,'readWorkSources()');return c.d.$('work-sources-list').children[0]?.children[0]?.textContent==='Báo cáo đã công bố'&&c.ctx.workSourcesData?.sources[0]?.id===reportId});
await check('sources_authority_fence_drops_late_get_even_when_report_reenabled',async()=>{const c=sourceContext();let resolve;c.ctx.request=()=>new Promise(r=>resolve=r);const pending=run(c.ctx,'readWorkSources()');run(c.ctx,'fenceReportDeliveries()');c.state.reportDeliveries.get(reportId).active=true;resolve(c.metadata);await pending;return c.ctx.workSourcesData===null&&c.d.$('work-sources-list').children.length===0});
await check('sources_navigation_drops_late_get_without_retarget',async()=>{const c=sourceContext();let resolve;c.ctx.request=()=>new Promise(r=>resolve=r);const pending=run(c.ctx,'readWorkSources()');c.ctx.navigationGeneration++;c.state.channelId='channel-'+ 'd'.repeat(32);run(c.ctx,'checkWorkSourcesScope()');resolve(c.metadata);await pending;return c.ctx.workSourcesData===null&&c.d.$('work-sources-list').children.length===0&&!c.d.$('work-sources-dialog').open});
await check('sources_report_access_change_clears_presented_rows',async()=>{const c=sourceContext();await run(c.ctx,'readWorkSources()');c.state.reportDeliveries.get(reportId).active=false;run(c.ctx,'checkWorkSourcesScope()');return c.ctx.workSourcesData===null&&c.d.$('work-sources-list').children.length===0});
await check('groups_sources_current_stage_and_scope_only',async()=>{const c=sourceContext();const group={id:'collab-'+ 'e'.repeat(32),channelId:channel,requestIds:[requestId]},w=c.state.work.get(requestId);w.collaboration={id:group.id,stage:1};c.ctx.group=group;if(run(c.ctx,'groupSourceWork(group)?.requestId')!==requestId)return false;group.requestIds=[];if(run(c.ctx,'groupSourceWork(group)')!==null)return false;group.requestIds=[requestId];w.conversationId='different-channel';return run(c.ctx,'groupSourceWork(group)')===null});
await check('groups_goal_rendered_without_private_history_payload',async()=>{const d=dom(),ctx=vm.createContext({...d});load(ctx,app,['makeGroupView']);ctx.group={id:'collab-'+ 'e'.repeat(32),goal:'Shared public task goal'};const view=run(ctx,'makeGroupView(group)');return view.el.children[0]?.children[0]?.textContent==='Shared public task goal'});

function reconnectContext(){
 const d=dom(),input={botId:bot.id,text:'Original awaiting acknowledgement',lane:'foreground',clientNonce:'independent-reconnect-001'};
 const work={...input,requestId:'request-reconnect-fixture',conversationId:'conversation-'+bot.id,status:'waiting_host',createdSequence:1,createdAt:'2026-10-13T00:00:00Z'};
 const state={botId:bot.id,channelId:'',accountChoice:null,projectChoices:{},capabilities:[],channels:[],readOnly:false,pending:input,work:new Map()};d.$('prompt').value=input.text;
 const ctx=vm.createContext({...d,state,window:{},sessionState:{ready:true,generation:1,autoReplay:true},outbox:[input],outboxReplay:false,inFlight:new Set(),views:new Map([['conversation-'+bot.id,{draft:input.text,generation:3,botId:bot.id}]]),navigationGeneration:3,fixtureSession:()=>false,sessionOwner:()=>true,sessionCurrent:epoch=>epoch===ctx.sessionState.generation,render(){},setHint(){},botDefaultMode:()=>false,cacheSet(){},cacheRemove(){},request:async()=>({work}),acceptContextWork:w=>{state.work.set(w.requestId,w)},TextEncoder,URLSearchParams,input,work});
 load(ctx,app,['laneKey','projectChoice','viewKey','inputView','canonicalJSON','strictKeys','validCoreProfileInput','matchingCoreProfileInput','matchingBotDefaultInput','matchingRequestPolicy','messageInput','saveOutbox','acknowledged','rejectInput','resumeOutbox','retryOutboxInput',...['outboxDraftScope','clearAcknowledgedOutbox'].filter(name=>app.includes('function '+name+'('))]);return {ctx,d,state,input,work};
}
await check('reconnect_same_view_confirmed_original_clears_live_and_saved_draft',async()=>{const c=reconnectContext();await run(c.ctx,'resumeOutbox()');return c.ctx.outbox.length===0&&c.d.$('prompt').value===''&&c.ctx.views.get('conversation-'+bot.id).draft===''||'Replay ACK removes outbox but leaves the acknowledged live/saved draft'});
await check('reconnect_confirmed_original_clears_matching_pending',async()=>{const c=reconnectContext();await run(c.ctx,'resumeOutbox()');return c.ctx.outbox.length===0&&c.state.pending===null||'Replay ACK removes outbox but keeps state.pending bound to old nonce'});
await check('reconnect_late_ack_keeps_new_navigation_draft',async()=>{const c=reconnectContext();let resolve;c.ctx.request=()=>new Promise(r=>resolve=r);const pending=run(c.ctx,'resumeOutbox()');c.ctx.navigationGeneration++;c.state.botId='bot-davin';c.d.$('prompt').value='New view draft';resolve({work:c.work});await pending;return c.ctx.outbox.length===0&&c.d.$('prompt').value==='New view draft'&&c.ctx.views.get('conversation-'+bot.id).draft===c.input.text});
await check('reconnect_late_ack_keeps_edited_same_view_draft',async()=>{const c=reconnectContext();let resolve;c.ctx.request=()=>new Promise(r=>resolve=r);const pending=run(c.ctx,'resumeOutbox()');c.d.$('prompt').value='Edited during reconnect';resolve({work:c.work});await pending;return c.ctx.outbox.length===0&&c.d.$('prompt').value==='Edited during reconnect'});
await check('reconnect_ack_does_not_clear_newer_pending_nonce',async()=>{const c=reconnectContext();const newer={...c.input,text:'New pending message',clientNonce:'independent-new-pending-001'};c.state.pending=newer;await run(c.ctx,'resumeOutbox()');return c.ctx.outbox.length===0&&c.state.pending===newer});
await check('reconnect_nonmatching_ack_does_not_clear_draft_or_pending',async()=>{const c=reconnectContext();c.ctx.request=async()=>({work:{...c.work,text:'A different acknowledged message'}});await run(c.ctx,'resumeOutbox()');return c.ctx.outbox.length===1&&c.d.$('prompt').value===c.input.text&&c.state.pending===c.input});
await check('reconnect_late_ack_after_session_generation_change_preserves_draft',async()=>{const c=reconnectContext();let resolve;c.ctx.request=()=>new Promise(r=>resolve=r);const pending=run(c.ctx,'resumeOutbox()');c.ctx.sessionState.generation++;resolve({work:c.work});await pending;return c.d.$('prompt').value===c.input.text&&c.state.pending===c.input});
await check('reconnect_rejected_entry_does_not_block_later_normal_entry',async()=>{const c=reconnectContext();c.ctx.outbox.unshift({...c.input,clientNonce:'independent-rejected-001',rejection:409});await run(c.ctx,'resumeOutbox()');return c.ctx.outbox.length===1&&c.ctx.outbox[0].rejection===409});

result.status=result.checks.some(c=>c.status==='FAIL')?'FAIL':result.checks.some(c=>c.status==='BLOCKED')?'BLOCKED':'PASS';
result.counts=Object.fromEntries(['PASS','FAIL','BLOCKED'].map(status=>[status.toLowerCase(),result.checks.filter(c=>c.status===status).length]));
await mkdir(output,{recursive:true});
const harnessArtifact=path.join(output,'harness-'+result.harnessSHA256+'.mjs');
try{await writeFile(harnessArtifact,await readFile(fileURLToPath(import.meta.url)),{flag:'wx'})}catch(error){if(error.code!=='EEXIST'||sha(await readFile(harnessArtifact))!==result.harnessSHA256)throw error}
result.harnessArtifact=harnessArtifact;
await writeFile(path.join(output,label+'.json'),JSON.stringify(result,null,2)+'\n',{flag:'wx'});
console.log(JSON.stringify(result,null,2));
