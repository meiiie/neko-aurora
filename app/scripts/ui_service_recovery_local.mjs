import {serviceRecoveryCases} from './ui_service_recovery_cases.mjs';
import {spawn,execFileSync} from 'node:child_process';
import {readFile,writeFile,mkdir,mkdtemp} from 'node:fs/promises';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {createHash,randomUUID} from 'node:crypto';

const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..');
const options=Object.fromEntries(process.argv.slice(2).reduce((pairs,value,index,args)=>{if(value.startsWith('--'))pairs.push([value.slice(2),args[index+1]]);return pairs},[]));
if(!options.host)throw new Error('--host must name the existing Rust host executable');
const commit=execFileSync('git',['rev-parse','HEAD'],{cwd:root,encoding:'utf8'}).trim();
const dirty=Boolean(execFileSync('git',['status','--porcelain'],{cwd:root,encoding:'utf8'}).trim());
if(options.commit&&(options.commit!==commit||dirty))throw new Error('Exact full commit and clean tracked source required');
const output=path.resolve(root,options.output??'.build/evidence-service-recovery-ui');
await mkdir(output,{recursive:true});
const temporary=await mkdtemp(path.join(root,'.build','neko-local-channel-'));
const binary=path.join(root,'.build','neko-aurora.exe');
const runtimeEnv=Object.fromEntries(['SystemRoot','WINDIR','TEMP','TMP','USERPROFILE','LOCALAPPDATA','APPDATA'].filter(k=>process.env[k]).map(k=>[k,process.env[k]]));
const service=spawn(binary,['--listen','127.0.0.1:0','--state',path.join(temporary,'state'),'--host-executable',path.resolve(options.host)],{cwd:root,windowsHide:true,stdio:['ignore','pipe','pipe'],env:runtimeEnv});
let startup='',errors='';service.stdout.on('data',data=>startup+=data);service.stderr.on('data',data=>errors+=data);
const delay=ms=>new Promise(resolve=>setTimeout(resolve,ms));
const until=async predicate=>{for(let i=0;i<100;i++){const value=await predicate();if(value)return value;await delay(100)}throw new Error('Local UI gate timed out')};
const checks=[],shots=[];let browser,socket,call,evaluate;
const check=(name,ok)=>{checks.push({name,status:ok?'PASS':'FAIL'});if(!ok)throw new Error(name)};
const sha=bytes=>createHash('sha256').update(bytes).digest('hex');
try{
 const base=await until(()=>startup.match(/^READY (http:\/\/127\.0\.0\.1:\d+)/m)?.[1]);
 const profile=await mkdtemp(path.join(temporary,'browser-'));
 browser=spawn(options.chrome??'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe',['--headless=new','--disable-gpu','--disable-background-networking','--disable-sync','--disable-extensions','--no-first-run','--remote-debugging-address=127.0.0.1','--remote-debugging-port=0','--user-data-dir='+profile,'about:blank'],{windowsHide:true,stdio:'ignore'});
 const active=await until(async()=>{try{return await readFile(path.join(profile,'DevToolsActivePort'),'utf8')}catch{return null}});
 const target=await fetch('http://127.0.0.1:'+Number(active.split('\n')[0])+'/json/new?about:blank',{method:'PUT'}).then(response=>response.json());
 socket=new WebSocket(target.webSocketDebuggerUrl);await new Promise((resolve,reject)=>{socket.addEventListener('open',resolve,{once:true});socket.addEventListener('error',reject,{once:true})});
 let serial=0;const pending=new Map();socket.addEventListener('message',event=>{const value=JSON.parse(event.data),entry=pending.get(value.id);call?.onPaused?.(value);if(entry){clearTimeout(entry.timer);pending.delete(value.id);value.error?entry.reject(new Error(value.error.message)):entry.resolve(value.result)}});
 call=(method,params={})=>new Promise((resolve,reject)=>{const id=++serial,timer=setTimeout(()=>{pending.delete(id);reject(new Error('CDP timeout '+method))},10000);pending.set(id,{resolve,reject,timer});socket.send(JSON.stringify({id,method,params}))});
 evaluate=async expression=>{const result=await call('Runtime.evaluate',{expression,returnByValue:true,awaitPromise:true});if(result.exceptionDetails)throw new Error('Browser evaluation failed');return result.result.value};
 const api=async(url,body)=>evaluate('(async()=>{const r=await fetch('+JSON.stringify(url)+','+JSON.stringify(body===undefined?{}:{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)})+');return {status:r.status,data:await r.json()}})()');
 const select=async(id,value)=>evaluate('(()=>{const e=document.getElementById('+JSON.stringify(id)+');e.value='+JSON.stringify(value)+';e.dispatchEvent(new Event("change",{bubbles:true}))})()');
 const send=async text=>{await evaluate('document.getElementById("prompt").value='+JSON.stringify(text)+';document.getElementById("prompt").dispatchEvent(new Event("input"));document.getElementById("composer").requestSubmit()');await until(()=>evaluate('inFlight.size===0'));return (await api('/v1/snapshot')).data.work.find(work=>work.text===text)};
 const shot=async(name,width,height,mobile=false)=>{await call('Emulation.setDeviceMetricsOverride',{width,height,deviceScaleFactor:1,mobile});await evaluate('document.fonts.ready');await delay(250);const value=await call('Page.captureScreenshot',{format:'png'}),bytes=Buffer.from(value.data,'base64'),filename=name+'.png';await writeFile(path.join(output,filename),bytes);shots.push({filename,width,height,sha256:sha(bytes)})};
 await call('Page.enable');await call('Runtime.enable');await call('Page.navigate',{url:base});await until(()=>evaluate('document.getElementById("bot-name")?.textContent==="Meiiie"'));
 const assetHashes={};for(const name of ['index.html','style.css','app.js','connections.js']){const served=await evaluate('fetch('+JSON.stringify(name==='index.html'?'/':'/'+name)+').then(r=>r.text())'),source=await readFile(path.join(root,'native/internal/web/assets',name));assetHashes[name]=sha(source);check('actual_embedded_'+name+'_matches_source',sha(Buffer.from(served,'utf8'))===assetHashes[name])}
 const snapshot=(await api('/v1/snapshot')).data;const ctx={evaluate,api,send,call,until,delay,check,shot,base,temporary,socket,snapshot};await serviceRecoveryCases(ctx);
 const report={status:'PASS',codeCommit:commit,workingTree:dirty,command:{executable:process.execPath,arguments:process.argv.slice(1),cwd:root},environment:{node:process.version,platform:process.platform,browser:await call('Browser.getVersion')},scope:'Actual owned Go embedded assets/capability/unproved API denial + typed CDP service recovery/receipt fixtures. Native stopped proof and resumed Core execution are not induced by this harness; parent owns native9. No provider/native window/cloud/physical phone.',checks,shots,assetSha256:assetHashes,goSha256:sha(await readFile(binary)),hostSha256:sha(await readFile(options.host))};
 await writeFile(path.join(output,'results.json'),JSON.stringify(report,null,2));console.log(JSON.stringify({status:'PASS',checks:checks.length,codeCommit:commit,workingTree:dirty,output}));
}catch(error){let ui=null;try{ui=await evaluate('({focus:document.activeElement?.id,geometry:[...document.querySelectorAll(".task-controls [data-action=recover]")].map(e=>({hidden:e.hidden,rect:e.getBoundingClientRect().toJSON(),hit:document.elementFromPoint(e.getBoundingClientRect().left+10,e.getBoundingClientRect().top+10)?.outerHTML})),commands:[...taskCommands.values()],work:[...state.work.values()].map(w=>({requestId:w.requestId,status:w.status,failure:w.failure}))})')}catch{}await writeFile(path.join(output,'failed-run.json'),JSON.stringify({ui,status:'FAIL',error:String(error.message),codeCommit:commit,workingTree:dirty,checks,startup,errors,scope:'Owned headless local Go UI gate; no Core/provider'},null,2));throw error}
finally{socket?.close();browser?.kill();service.kill();}
