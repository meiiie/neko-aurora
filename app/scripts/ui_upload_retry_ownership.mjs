// Portable actual-handler regression. DOM and transport are fixtures, not browser acceptance.
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {readFileSync} from 'node:fs';
import {execFileSync} from 'node:child_process';
import {createHash, webcrypto} from 'node:crypto';
const source = readFileSync(new URL('../native/internal/web/assets/app.js', import.meta.url), 'utf8');
const line = prefix => { const value = source.split('\n').find(v => v.startsWith(prefix)); assert.ok(value, prefix); return value; };
const deferred = () => { let resolve; const promise = new Promise(r => resolve = r); return {promise, resolve}; };
function fixture() {
  const nodes = new Map();
  const $ = id => { if (!nodes.has(id)) nodes.set(id, {disabled:false, textContent:'', files:[], open:false, listeners:{}, addEventListener(type, fn){this.listeners[type]=fn;}, focus(){}, reset(){$('upload-file').files=[];}, showModal(){this.open=true;}, close(){this.open=false;this.listeners.close?.();}}); return nodes.get(id); };
  const project = {id:'project-original', revision:1, name:'Original project'};
  const bytes = Buffer.from('original file');
  const file = {name:'original.txt', size:bytes.length, arrayBuffer:async()=>bytes};
  const original = {body:{projectId:project.id, projectRevision:1, channelId:'channel-original', filename:file.name, clientNonce:'original-nonce'}, bytes:bytes.length, digest:createHash('sha256').update(bytes).digest('hex'), marker:{}, lookupMissing:true, restored:true};
  const gate = deferred(), entered = deferred(), sent = [];
  const ctx = vm.createContext({$, TextEncoder, Uint8Array, crypto:webcrypto, btoa:v=>Buffer.from(v,'binary').toString('base64'), document:{activeElement:null}, state:{readOnly:false, capabilities:['local_uploads_v1'], channelId:'channel-original'}, selectedProject:()=>project, activeChannel:()=>({revision:1}), projectIntakeArchived:()=>false, uploadRetryIssue:()=>'', containFocus(){}, sessionState:{ready:true,generation:1}, sessionCurrent:epoch=>ctx.sessionState.ready&&epoch===ctx.sessionState.generation, uploadCommands:new Map([[original.body.clientNonce,original]]), original, file,
    checkUploadReceipt:async command=>{entered.resolve();const result=await gate.promise;command.lookupMissing=result==='missing';if(result==='found'){command.accepted=true;ctx.uploadCommands.delete(command.body.clientNonce);}return result==='found';},
    runUpload:async(command,token)=>{sent.push({command,token,body:{...command.body}});}
  });
  vm.runInContext('let uploadGeneration=0,uploadTarget=null,uploadActive=null,uploadReselect=null;',ctx);
  for(const prefix of ['function disableUploadSave(', 'function openUpload()', 'function openUploadRetry(', '$('+'"upload-open").addEventListener(', '$('+'"upload-form").addEventListener("submit"']) vm.runInContext(line(prefix),ctx);
  const run = code=>vm.runInContext(code,ctx);
  run('openUploadRetry(original)'); $('upload-file').files=[file];
  const submit = ()=>$('upload-form').listeners.submit({preventDefault(){}});
  return {$,ctx,run,original,file,gate,entered,sent,submit};
}
const results=[];
async function test(name,fn){try{await fn();results.push({name,status:'PASS'});}catch(error){results.push({name,status:'FAIL',error:error.message});process.exitCode=1;}}
for(const result of ['missing','found','error']) for(const interruption of ['close','reopen','new-file','new-active','session','reselected-owner','removed-command','replaced-command']) {
  await test(`${interruption} during delayed ${result} preserves current owner`,async()=>{
    const f=fixture(),pending=f.submit();await f.entered.promise;
    if(interruption==='close')f.$('upload-dialog').close();
    if(['reopen','new-active'].includes(interruption)){f.$('upload-dialog').close();f.run('openUpload()');}
    if(interruption==='new-file'){f.$('upload-file').files=[{...f.file,name:'new.txt'}];f.$('upload-file').listeners.change();}
    if(interruption==='new-active')f.run('uploadActive={body:{clientNonce:"newer-nonce",contentBase64:"newer bytes"}}');
    if(interruption==='session')f.ctx.sessionState.generation++;
    if(interruption==='reselected-owner')f.run('uploadReselect={...original}');
    if(interruption==='replaced-command')f.ctx.uploadCommands.set(f.original.body.clientNonce,{...f.original});
    if(interruption==='removed-command')f.ctx.uploadCommands.delete(f.original.body.clientNonce);
    // A newer pending owner must never be enabled by an older found/error completion.
    if(!['removed-command','replaced-command'].includes(interruption)){f.$('upload-save').disabled=true;f.$('upload-status').textContent='newer status';}
    const active=f.run('uploadActive'),open=f.$('upload-dialog').open;
    f.gate.resolve(result);await pending;
    assert.equal(f.sent.length,0,'interrupted handler must not send');
    assert.equal(f.original.body.contentBase64,undefined,'interrupted handler must not reattach file bytes');
    assert.equal(f.run('uploadActive'),active,'newer active command must stay owned');
    if(!['removed-command','replaced-command'].includes(interruption)){assert.equal(f.$('upload-dialog').open,open);assert.equal(f.$('upload-save').disabled,true,'old completion must not enable newer save');assert.equal(f.$('upload-status').textContent,'newer status');}
  });
}
for(const result of ['missing','found','error'])await test(`ordinary ${result} receipt keeps retry behavior`,async()=>{
  const f=fixture(),pending=f.submit();await f.entered.promise;f.gate.resolve(result);await pending;
  assert.equal(f.sent.length,result==='missing'?1:0);
  if(result==='missing'){assert.equal(f.sent[0].command,f.original);assert.equal(f.sent[0].body.clientNonce,'original-nonce');assert.equal(f.sent[0].body.contentBase64,Buffer.from('original file').toString('base64'));}
  else {assert.equal(f.$('upload-dialog').open,result!=='found');assert.equal(f.$('upload-save').disabled,false);}
});
console.log(JSON.stringify({commit:execFileSync('git',['rev-parse','HEAD'],{cwd:new URL('../..',import.meta.url),encoding:'utf8'}).trim(),sourceSHA256:createHash('sha256').update(source).digest('hex'),environment:{node:process.version,platform:process.platform},scope:'Actual upload submit/open/close/change handlers in Node VM; deferred receipt and upload boundary fixtures. No browser or backend acceptance.',results},null,2));
