import fs from 'node:fs';
import path from 'node:path';
import vm from 'node:vm';
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
const repo=process.argv[2],out=process.argv[3];
assert.ok(path.resolve(repo).startsWith(path.resolve('E:/CodexScratch/continuity-clear-ui-20261013')+path.sep));
assert.ok(path.resolve(out).startsWith(path.resolve('E:/CodexScratch/continuity-clear-ui-20261013')+path.sep));
fs.mkdirSync(out,{recursive:false});
const source=fs.readFileSync(repo+'/native/internal/web/assets/app.js','utf8'),html=fs.readFileSync(repo+'/native/internal/web/assets/index.html','utf8');
const lines=source.split(/\r?\n/),sha=b=>createHash('sha256').update(b).digest('hex'),checks=[];
async function check(name,fn){try{await fn();checks.push({name,status:'PASS'});}catch(e){checks.push({name,status:'FAIL',reason:String(e.message)});process.exitCode=1;}}
function fresh(){
 const elements=new Map(),document={activeElement:null},calls={nonce:0,post:[],retain:[],request:0};
 const $=id=>{if(!elements.has(id))elements.set(id,{id,disabled:false,hidden:false,open:id==='continuity-dialog',value:'old instructions',textContent:'',children:['old'],listeners:new Map(),replaceChildren(...children){this.children=children;},focus(){document.activeElement=this;},addEventListener(type,fn){this.listeners.set(type,fn);}});return elements.get(id);};
 let replyResolve;const held=new Promise(r=>replyResolve=r);
 const ctx=vm.createContext({$,document,Map,Boolean,Date,Error,JSON,encodeURIComponent,state:{capabilities:['local_project_continuity_v1']},navigationGeneration:7,continuityGeneration:9,continuityReadGeneration:4,continuityTarget:{projectId:'project-a',channelId:'channel-a',expectedProjectRevision:2,expectedChannelRevision:3,expectedRevision:1,valid:true},continuityReturn:{key:'view-a',generation:7},continuityDecisionCards:new Map([['d',{text:{value:'old decision'}}]]),continuityReferenceCards:new Map([['r',{title:{value:'old reference'}}]]),continuityDescriptors:new Map([['project-a/channel-a',{revision:1}]]),policyStorageError:false,marker:null,key:'view-a',project:{id:'project-a',revision:2},channel:{id:'channel-a',revision:3},viewKey(){return ctx.key;},selectedProject(){return ctx.project;},activeChannel(){return ctx.channel;},continuityKey:(p,c)=>p+'/'+c,pendingPolicy:()=>ctx.marker,reconcile(){},policyNotice:()=>({}),crypto:{randomUUID(){calls.nonce++;return 'explicit-save-nonce-'+calls.nonce;}},request:()=>{calls.request++;return held;},acceptContinuityDescriptor:()=>true,fillContinuityEditor(c){$('continuity-instructions').value=c.instructions;},continuityURL:(p,c)=>'/v1/projects/'+p+'/continuity?channelId='+c,retainPolicyCommand:async(kind,id,body,url)=>{calls.retain.push({kind,id,body,url});return {body,url,message:'saved'};},runPolicyCommand:async command=>calls.post.push(command.body),readContinuityDraft:()=>({instructions:$('continuity-instructions').value,decisions:[...ctx.continuityDecisionCards.values()],references:[...ctx.continuityReferenceCards.values()]})});
 for(const name of ['clearContinuityEditor','continuityEdited','continuityEditorCurrent','continuityDraftEditable','renderContinuityCommand','clearContinuityDraft','readContinuity']){
  const line=lines.find(l=>l.startsWith('function '+name+'(')||l.startsWith('async function '+name+'('));if(line)vm.runInContext(line,ctx);
 }
 const submit=lines.find(l=>l.startsWith('$("continuity-form").addEventListener("submit"'));vm.runInContext(submit,ctx);
 return {ctx,$,calls,document,replyResolve};
}
await check('explicit_clear_draft_button_and_actual_handler',()=>{assert.ok(/<button[^>]*id="continuity-clear-draft"[^>]*type="button"[^>]*disabled[^>]*>Xóa bản nháp/.test(html),'missing explicit clear-draft control');assert.equal(typeof fresh().ctx.clearContinuityDraft,'function');});
if(typeof fresh().ctx.clearContinuityDraft==='function'){
 await check('clear_all_fields_keeps_loaded_scope_revision_and_creates_no_command',()=>{
  const f=fresh(),old={...f.ctx.continuityTarget};f.ctx.renderContinuityCommand();f.ctx.clearContinuityDraft();
  assert.equal(f.$('continuity-instructions').value,'');assert.equal(f.ctx.continuityDecisionCards.size,0);assert.equal(f.ctx.continuityReferenceCards.size,0);
  for(const k of ['projectId','channelId','expectedRevision','expectedProjectRevision','expectedChannelRevision','valid'])assert.equal(f.ctx.continuityTarget[k],old[k]);
  assert.equal(f.calls.nonce,0);assert.equal(f.calls.post.length,0);assert.equal(f.calls.retain.length,0);assert.equal(f.calls.request,0);assert.equal(f.document.activeElement,f.$('continuity-instructions'));assert.equal(f.ctx.continuityGeneration,10);assert.equal(f.ctx.continuityReadGeneration,5);
 });
 await check('explicit_save_after_clear_uses_same_scope_CAS_and_one_new_save_nonce',async()=>{
  const f=fresh();f.ctx.renderContinuityCommand();f.ctx.clearContinuityDraft();await f.$('continuity-form').listeners.get('submit')({preventDefault(){}});
  assert.equal(f.calls.post.length,1);assert.equal(f.calls.nonce,1);assert.equal(f.calls.retain[0].id,'project-a');
  assert.deepEqual(JSON.parse(JSON.stringify(f.calls.post[0])),{channelId:'channel-a',expectedProjectRevision:2,expectedChannelRevision:3,expectedRevision:1,clientNonce:'explicit-save-nonce-1',instructions:'',decisions:[],references:[]});
 });
 for(const [name,change] of [['pending_original',f=>f.ctx.marker={clientNonce:'original-command',body:{instructions:'immutable old body'}}],['storage_denied',f=>f.ctx.policyStorageError=true],['stale_descriptor',f=>f.ctx.continuityDescriptors.set('project-a/channel-a',{revision:2})],['unloaded',f=>f.ctx.continuityTarget.valid=false],['capability_removed',f=>f.ctx.state.capabilities=[]],['wrong_project_revision',f=>f.ctx.project.revision=3],['navigation_changed',f=>f.ctx.navigationGeneration=8],['closed_dialog',f=>f.$('continuity-dialog').open=false]]){
  await check(name+'_does_not_clear_or_send',()=>{const f=fresh();change(f);const before=JSON.stringify(f.ctx.marker);f.ctx.clearContinuityDraft();assert.equal(f.$('continuity-instructions').value,'old instructions');assert.equal(f.ctx.continuityDecisionCards.size,1);assert.equal(f.calls.nonce,0);assert.equal(f.calls.post.length,0);assert.equal(JSON.stringify(f.ctx.marker),before);assert.equal(f.document.activeElement,null);});
 }
 await check('pending_GET_disables_clear_and_cannot_replace_an_edited_draft',async()=>{
  const f=fresh();const p=f.ctx.readContinuity();assert.equal(f.$('continuity-clear-draft').disabled,true);f.ctx.clearContinuityDraft();assert.equal(f.$('continuity-instructions').value,'old instructions');
  f.$('continuity-instructions').value='new local draft';f.ctx.continuityEdited();f.replyResolve({continuity:{projectId:'project-a',channelId:'channel-a',version:1,revision:1,instructions:'late old content'}});await p;assert.equal(f.$('continuity-instructions').value,'new local draft');assert.equal(f.calls.post.length,0);
 });
 await check('clear_fences_old_read_token_and_disable_focus_falls_back_inside_dialog',()=>{
  const f=fresh();const token=f.ctx.continuityReadGeneration;f.ctx.clearContinuityDraft();assert.ok(f.ctx.continuityReadGeneration>token);f.$('continuity-clear-draft').focus();f.ctx.marker={};f.ctx.renderContinuityCommand();assert.equal(f.document.activeElement,f.$('continuity-close'));assert.equal(f.$('continuity-clear-draft').disabled,true);
 });
}
const result={status:checks.every(c=>c.status==='PASS')?'PASS':'FAIL',checks,sourceSHA256:sha(source),indexSHA256:sha(html),command:process.argv,node:process.version,scope:'Actual app.js clear/render/read/submit function source evaluated in an authored DOM/storage/network VM. No browser/native/phone/AT/model or real continuity mutation. Save retention/POST interfaces are stubs; existing durable-command behavior is separate.'};
fs.writeFileSync(out+'/results.json',JSON.stringify(result,null,2)+'\n');console.log(JSON.stringify({status:result.status,checks:checks.length,failures:checks.filter(c=>c.status==='FAIL'),out}));
