import vm from 'node:vm';
import path from 'node:path';
import {readFile,writeFile,mkdir} from 'node:fs/promises';
import {execFileSync} from 'node:child_process';
import {createHash} from 'node:crypto';
import {fileURLToPath} from 'node:url';

const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..'),opts=Object.fromEntries(process.argv.slice(2).flatMap((v,i,a)=>v.startsWith('--')?[[v.slice(2),a[i+1]]]:[]));
const sha=v=>createHash('sha256').update(v).digest('hex'),asset='native/internal/web/assets/app.js';
if(Boolean(opts.commit)===Boolean(opts['working-sha']))throw Error('Provide exactly one immutable --commit or quiescent --working-sha');
if(opts.commit&&!/^[a-f0-9]{40}$/.test(opts.commit)||opts['working-sha']&&!/^[a-f0-9]{64}$/.test(opts['working-sha']))throw Error('Full immutable commit or exact working SHA-256 required');
const bytes=opts.commit?Buffer.from(execFileSync('git',['show',opts.commit+':'+asset],{cwd:root,maxBuffer:8*1024*1024})):await readFile(path.join(root,asset));
if(opts['working-sha']&&sha(bytes)!==opts['working-sha'])throw Error('Working source differs from authorized quiescent hash');
const source=bytes.toString('utf8'),output='E:/CodexScratch/ux-independent-20261013',label=opts.label;
if(!/^[a-zA-Z0-9_-]{1,80}$/.test(label??''))throw Error('Unique evidence --label required');
const result={sourceCommit:opts.commit??execFileSync('git',['rev-parse','HEAD'],{cwd:root,encoding:'utf8'}).trim(),sourceMode:opts.commit?'immutable Git object':'working source pinned to authorized quiescent SHA-256; HEAD is reference only',sourceSHA256:sha(bytes),harnessSHA256:sha(await readFile(fileURLToPath(import.meta.url))),command:process.argv,environment:{node:process.version,platform:process.platform,arch:process.arch},scope:'Actual outbox source functions in bounded Node VM with typed public fixtures. No browser/backend/provider/Core/grant/model/device acceptance. Frozen 25-check runner is unchanged.',checks:[],observations:[]};
function extract(name){const lines=source.split('\n'),i=lines.findIndex(line=>new RegExp('^function '+name+'\\(|^async function '+name+'\\(').test(line));if(i<0)throw Error('Missing actual function '+name);if(lines[i].trimEnd().endsWith('}'))return lines[i];const end=lines.findIndex((line,n)=>n>i&&/^}\s*$/.test(line));if(end<0)throw Error('Unbounded function '+name);return lines.slice(i,end+1).join('\n')}
const run=(ctx,code)=>vm.runInContext(code,ctx,{timeout:1000});
function context(){
 const field={value:'Repeated text',listeners:[],addEventListener(type,fn){if(type==='input')this.listeners.push(fn)},dispatchInput(){for(const fn of this.listeners)fn({target:this})}};
 const input={botId:'bot-meiiie',text:field.value,lane:'foreground',clientNonce:'independent-older-001'};
 const work={...input,requestId:'request-older-fixture',conversationId:'conversation-bot-meiiie',status:'waiting_host',createdSequence:1,createdAt:'2026-10-13T00:00:00Z'};
 const state={botId:input.botId,channelId:'',accountChoice:null,projectChoices:{},capabilities:[],channels:[],readOnly:false,pending:input,work:new Map()};
 const ctx=vm.createContext({state,window:{},$:(id)=>{if(id!=='prompt')throw Error('Unmodeled DOM id '+id);return field},sessionState:{ready:true,generation:1,autoReplay:true},outbox:[input],outboxReplay:false,inFlight:new Set(),views:new Map([['conversation-bot-meiiie',{draft:input.text,generation:3,botId:input.botId}]]),navigationGeneration:3,fixtureSession:()=>false,sessionCurrent:epoch=>epoch===ctx.sessionState.generation,render(){},setHint(){},botDefaultMode:()=>false,cacheSet(){},cacheRemove(){},request:async()=>({work}),acceptContextWork:w=>state.work.set(w.requestId,w),resizePrompt(){},renderMentions(){},renderRouting(){},renderCollaborationMenu(){},input,work});
 for(const name of ['laneKey','projectChoice','viewKey','inputView','canonicalJSON','strictKeys','validCoreProfileInput','matchingCoreProfileInput','matchingBotDefaultInput','matchingRequestPolicy','messageInput','saveOutbox','acknowledged','rejectInput','outboxDraftScope','clearAcknowledgedOutbox','resumeOutbox','retryOutboxInput'])run(ctx,extract(name));
 const handler=source.match(/^\$\("prompt"\)\.addEventListener\("input",.*$/m)?.[0]?.split(';$("prompt").addEventListener("keydown"')[0];
 if(handler)run(ctx,handler);
 return {ctx,field,state,input,work};
}
async function check(name,fn){try{const details=await fn();result.checks.push({name,status:details.pass?'PASS':'FAIL',details});if(!details.pass)process.exitCode=1}catch(error){result.checks.push({name,status:'HARNESS_ERROR',error:error.message});process.exitCode=1}}
const drafts=c=>({live:c.field.value,saved:c.ctx.views.get('conversation-bot-meiiie').draft,pendingNonce:c.state.pending?.clientNonce??null,outboxNonces:c.ctx.outbox.map(v=>v.clientNonce)});
await check('newer_pending_nonce_with_identical_text_preserves_live_and_saved_drafts',async()=>{const c=context(),newer={...c.input,clientNonce:'independent-newer-001'};c.state.pending=newer;await run(c.ctx,'resumeOutbox()');return {pass:c.state.pending===newer&&c.field.value===newer.text&&c.ctx.views.get('conversation-bot-meiiie').draft===newer.text,...drafts(c)}});
await check('newer_pending_same_text_started_during_older_ack_preserves_both_drafts',async()=>{const c=context();let resolve;c.ctx.request=()=>new Promise(r=>resolve=r);const pending=run(c.ctx,'resumeOutbox()'),newer={...c.input,clientNonce:'independent-newer-002'};c.state.pending=newer;resolve({work:c.work});await pending;return {pass:c.state.pending===newer&&c.field.value===newer.text&&c.ctx.views.get('conversation-bot-meiiie').draft===newer.text,...drafts(c)}});
await check('unchanged_original_matching_ack_still_clears_owned_draft',async()=>{const c=context();await run(c.ctx,'resumeOutbox()');return {pass:c.state.pending===null&&c.field.value===''&&c.ctx.views.get('conversation-bot-meiiie').draft==='',...drafts(c)}});
await check('newer_pending_different_text_keeps_live_edited_draft',async()=>{const c=context(),newer={...c.input,text:'A distinct new draft',clientNonce:'independent-newer-003'};c.state.pending=newer;c.field.value=newer.text;c.ctx.views.get('conversation-bot-meiiie').draft=newer.text;await run(c.ctx,'resumeOutbox()');return {pass:c.state.pending===newer&&c.field.value===newer.text&&c.ctx.views.get('conversation-bot-meiiie').draft===newer.text,...drafts(c)}});
{
 const c=context();let resolve;c.ctx.request=()=>new Promise(r=>resolve=r);const pending=run(c.ctx,'resumeOutbox()');c.field.value='Edited away from original';c.field.dispatchInput();c.field.value=c.input.text;c.field.dispatchInput();resolve({work:c.work});await pending;
 result.observations.push({name:'edit_away_then_back_to_identical_text_before_original_ack',...drafts(c),policy:'Observation only: current input handler has no edit-generation ownership marker. A stronger preserve-after-any-user-edit requirement needs explicit product intent; equal current text alone cannot distinguish renewed draft ownership.'});
}
if(opts['working-sha']&&sha(await readFile(path.join(root,asset)))!==result.sourceSHA256)throw Error('Working source drifted during probe; evidence not written');
result.status=result.checks.some(v=>v.status!=='PASS')?'FAIL':'PASS';result.counts=Object.fromEntries(['PASS','FAIL','HARNESS_ERROR'].map(status=>[status.toLowerCase(),result.checks.filter(c=>c.status===status).length]));
await mkdir(output,{recursive:true});
for(const [filename,data]of [['harness-'+result.harnessSHA256+'.mjs',await readFile(fileURLToPath(import.meta.url))],['app-'+result.sourceSHA256+'.js',bytes]]){try{await writeFile(path.join(output,filename),data,{flag:'wx'})}catch(error){if(error.code!=='EEXIST'||sha(await readFile(path.join(output,filename)))!==sha(data))throw error}}
await writeFile(path.join(output,label+'.json'),JSON.stringify(result,null,2)+'\n',{flag:'wx'});console.log(JSON.stringify(result,null,2));
