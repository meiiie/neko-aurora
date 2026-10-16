import vm from 'node:vm';
import {readFile,writeFile,mkdir} from 'node:fs/promises';
import {execFileSync} from 'node:child_process';
import {createHash,webcrypto} from 'node:crypto';
import {fileURLToPath} from 'node:url';
import path from 'node:path';

const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..');
const options=Object.fromEntries(process.argv.slice(2).flatMap((v,i,a)=>v.startsWith('--')?[[v.slice(2),a[i+1]]]:[]));
const output=options.output?path.resolve(options.output):null;
if(output&&!output.replaceAll('\\','/').startsWith('E:/CodexScratch/core-source-onboarding-20261013/'))throw Error('Owned E evidence path required');
const sourceCommit=options['source-commit'];
if(sourceCommit&&!/^[a-f0-9]{40}$/.test(sourceCommit))throw Error('Exact immutable source commit required');
const readSource=async name=>sourceCommit?execFileSync('git',['show',sourceCommit+':'+name],{cwd:root,encoding:'utf8'}):readFile(path.join(root,name),'utf8');
const source=await readSource('native/internal/web/assets/connections.js');
const app=await readSource('native/internal/web/assets/app.js');
const sha=v=>createHash('sha256').update(v).digest('hex');
const result={sourceCommit:sourceCommit??execFileSync('git',['rev-parse','HEAD'],{cwd:root,encoding:'utf8'}).trim(),sourceMode:sourceCommit?'immutable Git objects':'working files',workingTree:Boolean(execFileSync('git',['status','--porcelain'],{cwd:root,encoding:'utf8'}).trim()),sourceSHA256:sha(source),appSHA256:sha(app),harnessSHA256:sha(await readFile(fileURLToPath(import.meta.url))),command:process.argv,environment:{node:process.version,platform:process.platform},scope:'Actual Core profile UI closure and validators in isolated Node VM with authored safe source/receipt GETs and minimal DOM. No browser, Go, Core, profile inspection, consent POST, credential read or provider.',checks:[]};
const extract=(text,name)=>{let a=text.indexOf('function '+name+'(');if(a<0)throw Error('Missing function '+name);if(text.slice(a-6,a)==='async ')a-=6;const b=text.slice(a+1).search(/\n(?:async )?function /);return text.slice(a,b<0?text.length:a+1+b)};
const start=source.indexOf('(()=>{\n const $=id=>document.getElementById(id),dialog=$("connections-dialog"),panel='),end=source.indexOf('\n})();',start);
if(start<0||end<0)throw Error('Missing actual Core profile UI closure');
const helpers=source.slice(source.indexOf('function coreProfileText('),start),closure=source.slice(start,end+6);
const descriptor={version:1,id:'authored-local-core',revision:1,label:'Compatible bundled Core source',runtimeSha256:'a'.repeat(64),fixtureOnly:false};
const tick=()=>new Promise(resolve=>setTimeout(resolve,0));
const settle=async()=>{for(let i=0;i<12;i++)await tick()};
const check=async(name,fn)=>{try{if(!await fn())throw Error('assertion');result.checks.push({name,status:'PASS'})}catch(e){result.checks.push({name,status:'FAIL',error:e.message});process.exitCode=1}};

function fixture({mode='local_owner',actor='local-owner',readOnly=false,capability=true,sources=[descriptor],deferred=false,reject=false}={}){
 const requests=[],events=[],nodes=new Map(),cache=new Map();let resolveSources;
 const document={activeElement:null,createElement:tag=>make(tag)};
 function make(tag='div'){
  const handlers=new Map(),attrs=new Map();
  return {tag,children:[],dataset:{},hidden:false,disabled:false,open:false,textContent:'',className:'',_value:'',
   get value(){return this._value},set value(v){this._value=String(v)},get options(){return this.children},
   classList:{toggle(){}},setAttribute(k,v){attrs.set(k,String(v))},getAttribute:k=>attrs.get(k),removeAttribute:k=>attrs.delete(k),
   addEventListener(k,fn){if(!handlers.has(k))handlers.set(k,[]);handlers.get(k).push(fn)},dispatch(k){for(const fn of handlers.get(k)??[])fn({preventDefault(){},target:this})},
   append(...xs){this.children.push(...xs)},replaceChildren(...xs){this.children=xs;if(this.tag==='select')this._value=xs.find(e=>!e.disabled)?.value??''},
   contains(v){return v===this||this.children.some(c=>c.contains?.(v))},focus(){document.activeElement=this},after(){},cloneNode(){return make(tag)},
   close(){this.open=false;this.dispatch('close')}
  };
 }
 const get=id=>{if(!nodes.has(id))nodes.set(id,make(['core-profile-source','core-profile-choice'].includes(id)?'select':'div'));return nodes.get(id)};
 document.getElementById=get;get('core-profile-template').content=make();
 for(const step of ['source','profile','scope']){const e=make();e.dataset.step=step;get('core-profile-steps').append(e)}
 for(const [key,value] of Object.entries({maxInputBytes:64000,maxOutputTokens:1024,maxRequests:1,maxSteps:1,maxSeconds:120,ttlSeconds:300}))get('core-profile-'+key).value=value;
 const channel='channel-'+'b'.repeat(32),state={readOnly,capabilities:capability?['local_core_profiles_v1']:[],botId:'bot-meiiie',accountChoice:null,bots:[{id:'bot-meiiie',name:'Meiiie'}]};
 const sessionState={ready:true,generation:1,metadata:{mode,actorId:actor,cacheNamespace:'authored-owner'}};
 const context=vm.createContext({document,window:{},state,sessionState,TextEncoder,URL,URLSearchParams,crypto:webcrypto,navigationGeneration:1,view:channel,
  viewKey:()=>context.view,activeChannel:()=>({id:channel,revision:2,name:'Local group',botIds:['bot-meiiie']}),privateConversation:()=>null,laneKey:()=>channel,selectedProject:()=>null,sharedChannel:()=>({id:channel,revision:2}),botState:()=>({archived:false}),renderRouting(){},
  localStorage:{getItem:key=>{events.push('cache-read');return cache.get(key)??null},setItem:(key,value)=>{events.push('cache-write');cache.set(key,value)},removeItem:key=>cache.delete(key)},
  requestRaw:async(route,body)=>{requests.push({route,method:body===undefined?'GET':'POST'});if(body!==undefined||!['/v1/core-profile-sources','/v1/core-profile-receipts'].some(p=>route===p||route.startsWith(p+'?')))throw Error('Unauthorized authored route');
   if(route==='/v1/core-profile-sources'){events.push('sources-get');if(reject)throw Error('authored-source-failure');if(deferred)return new Promise(resolve=>resolveSources=resolve);return {version:1,sources:structuredClone(sources)}}
   events.push('receipt-get');const q=new URLSearchParams(route.split('?')[1]);return {version:1,found:false,active:false,kind:q.get('kind'),clientNonce:q.get('clientNonce')}
  }
 });
 for(const name of ['cacheKey','cacheGet','cacheSet','cacheRemove','sessionOwner','fixtureSession','sessionCurrent','canonicalJSON','strictKeys','journalNonce','journalSensitive','commandDigest'])vm.runInContext(extract(app,name),context);
 vm.runInContext(helpers+closure,context);
 const run=s=>vm.runInContext(s,context),open=()=>{get('connections-dialog').open=true;get('connections-open').dispatch('click')};
 return {context,state,sessionState,get,run,requests,events,cache,open,get pendingResolver(){return resolveSources},resolve:()=>resolveSources?.({version:1,sources:structuredClone(sources)})};
}

await check('eligible_open_loads_and_reveals_sole_safe_source',async()=>{const f=fixture();f.open();await settle();return f.requests.filter(r=>r.route==='/v1/core-profile-sources').length===1&&!f.get('core-profile-panel').hidden&&f.get('core-profile-source').value===descriptor.id&&!f.get('core-profile-inspect').disabled});
await check('opening_has_only_safe_descriptor_get_without_inspection_or_post',async()=>{const f=fixture();f.open();await settle();return f.requests.length===1&&f.requests.every(r=>r.method==='GET'&&r.route==='/v1/core-profile-sources')&&f.get('core-profile-metadata').hidden&&f.get('core-profile-approve').disabled});
await check('source_step_and_copy_do_not_claim_connected_credentials',async()=>{const f=fixture();f.open();await settle();return f.get('core-profile-panel').dataset.phase==='source'&&f.get('core-profile-steps').children[0].getAttribute('aria-current')==='step'&&f.get('core-discovery-note').textContent.includes('cho phép xem metadata')&&f.get('core-profile-note').textContent===''});
await check('explicit_button_and_rerenders_coalesce_inflight_source_get',async()=>{const f=fixture({deferred:true});f.open();await settle();const preloaded=f.requests.length===1;f.get('core-use-installed').dispatch('click');for(let i=0;i<3;i++)f.run('window.nekoCoreProfiles.syncView()');await settle();const once=f.requests.length===1;f.resolve();await settle();return preloaded&&once&&!f.get('core-profile-panel').hidden&&!f.get('core-profile-inspect').disabled});
await check('closing_dialog_discards_deferred_source_response',async()=>{const f=fixture({deferred:true});f.open();await settle();f.get('connections-dialog').close();f.resolve();await settle();return f.get('core-profile-source').children.length===0&&f.get('core-profile-panel').hidden});
await check('closing_profile_panel_prevents_deferred_auto_reopen',async()=>{const f=fixture({deferred:true});f.open();await settle();f.get('core-profile-close').dispatch('click');f.resolve();await settle();return f.get('core-profile-panel').hidden&&f.get('core-profile-source').children.length===0});
await check('session_rotation_discards_deferred_source_response',async()=>{const f=fixture({deferred:true});f.open();await settle();f.sessionState.generation++;f.run('window.nekoCoreProfiles.sessionChanged()');f.resolve();await settle();return f.get('core-profile-panel').hidden&&f.get('core-profile-source').children.length===0});
await check('navigation_change_discards_deferred_auto_reveal',async()=>{const f=fixture({deferred:true});f.open();await settle();f.context.navigationGeneration++;f.context.view='another-view';f.resolve();await settle();return f.get('core-profile-panel').hidden&&f.get('core-profile-source').children.length===0&&!f.get('core-use-installed').disabled});
for(const [name,opts] of [['fixture_actor',{mode:'fixture',actor:'fixture-writer'}],['nonowner',{mode:'fixture',actor:'fixture-reader'}],['readonly',{readOnly:true}],['missing_capability',{capability:false}]])await check(name+'_never_preloads',async()=>{const f=fixture(opts);f.open();await settle();return f.requests.length===0&&f.get('core-profile-panel').hidden&&f.get('core-use-installed').disabled});
for(const [name,change] of [['readonly',f=>f.state.readOnly=true],['capability',f=>f.state.capabilities=[]],['actor',f=>Object.assign(f.sessionState.metadata,{mode:'fixture',actorId:'fixture-writer'})]])await check(name+'_change_fences_deferred_auto_reveal',async()=>{const f=fixture({deferred:true});f.open();await settle();change(f);f.resolve();await settle();f.run('window.nekoCoreProfiles.syncView()');return f.get('core-profile-panel').hidden&&f.get('core-profile-source').children.length===0&&f.get('core-use-installed').disabled});
await check('empty_catalog_does_not_auto_reveal',async()=>{const f=fixture({sources:[]});f.open();await settle();return f.requests.length===1&&f.get('core-profile-panel').hidden&&f.get('core-profile-inspect').disabled});
await check('invalid_source_descriptor_does_not_auto_reveal',async()=>{const f=fixture({sources:[{...descriptor,configuration:'unapproved-field'}]});f.open();await settle();return f.requests.length===1&&f.get('core-profile-panel').hidden&&f.get('core-profile-source').children.length===0});
await check('failed_catalog_preserves_explicit_retry_without_inspection',async()=>{const f=fixture({reject:true});f.open();await settle();return f.requests.length===1&&f.get('core-profile-panel').hidden&&!f.get('core-use-installed').disabled&&f.get('core-profile-inspect').disabled});
await check('multiple_sources_start_blank_with_inspection_disabled',async()=>{const other={...descriptor,id:'other-compatible-core',label:'Second source'},f=fixture({sources:[descriptor,other]});f.open();await settle();return !f.get('core-profile-panel').hidden&&f.get('core-profile-source').value===''&&f.get('core-profile-source').children[0]?.value===''&&f.get('core-profile-source').children[0]?.disabled&&f.get('core-profile-inspect').disabled&&f.get('core-profile-approve').disabled&&f.requests.length===1});
await check('multiple_source_choices_enable_inspection_after_explicit_change',async()=>{const other={...descriptor,id:'other-compatible-core',label:'Second source'},f=fixture({sources:[descriptor,other]});f.open();await settle();f.get('core-profile-source').value=other.id;f.get('core-profile-source').dispatch('change');return !f.get('core-profile-panel').hidden&&f.get('core-profile-source').children.filter(e=>e.value).length===2&&f.get('core-profile-source').value===other.id&&!f.get('core-profile-inspect').disabled&&f.get('core-profile-approve').disabled&&f.requests.length===1});
await check('valid_previous_multi_source_choice_survives_reopening',async()=>{const other={...descriptor,id:'other-compatible-core',label:'Second source'},f=fixture({sources:[descriptor,other]});f.open();await settle();f.get('core-profile-source').value=other.id;f.get('core-profile-source').dispatch('change');f.get('connections-dialog').close();f.open();await settle();return f.get('core-profile-source').value===other.id&&!f.get('core-profile-inspect').disabled&&f.get('core-profile-source').children.filter(e=>e.value).length===2&&f.requests.length===2});
await check('removed_previous_choice_returns_multi_source_to_blank',async()=>{const other={...descriptor,id:'other-compatible-core',label:'Second source'},sources=[descriptor,other],f=fixture({sources});f.open();await settle();f.get('core-profile-source').value=other.id;f.get('core-profile-source').dispatch('change');f.get('connections-dialog').close();sources.splice(1,1,{...descriptor,id:'replacement-compatible-core',label:'Replacement source'});f.open();await settle();return f.get('core-profile-source').value===''&&f.get('core-profile-inspect').disabled&&f.get('core-profile-source').children[0]?.disabled&&f.requests.length===2});
await check('pending_receipts_follow_catalog_without_activation_or_replay',async()=>{const f=fixture(),input={sourceId:descriptor.id,expectedSourceRevision:1,approveInspection:true,ttlSeconds:300,clientNonce:'authored-original-inspection'},createdAt=Date.now();f.context.material={kind:'inspection',input,createdAt};const digest=await f.run('commandDigest(material)');f.cache.set('neko.core-profile-operations.v1',JSON.stringify({version:1,records:[{kind:'inspection',input,createdAt,sha256:digest}]}));f.open();await settle();return f.events.indexOf('cache-read')<f.events.indexOf('sources-get')&&f.events.indexOf('sources-get')<f.events.indexOf('receipt-get')&&f.requests.length===2&&f.requests.every(r=>r.method==='GET')&&f.run('window.nekoCoreProfiles.selection()')===null&&f.get('core-profile-metadata').hidden});
await check('reopening_fresh_dialog_does_not_accept_old_response',async()=>{const f=fixture({deferred:true});f.open();await settle();const old=f.pendingResolver;f.get('connections-dialog').close();f.open();await settle();old?.({version:1,sources:[descriptor]});await settle();const ignored=f.get('core-profile-panel').hidden&&f.get('core-profile-source').children.length===0;f.resolve();await settle();return ignored&&f.requests.filter(r=>r.route==='/v1/core-profile-sources').length===2&&!f.get('core-profile-panel').hidden&&f.get('core-profile-source').value===descriptor.id});

result.status=result.checks.every(c=>c.status==='PASS')?'PASS':'FAIL';
if(output){await mkdir(output,{recursive:true});await writeFile(path.join(output,'results.json'),JSON.stringify(result,null,2)+'\n')}
console.log(JSON.stringify(result,null,2));
