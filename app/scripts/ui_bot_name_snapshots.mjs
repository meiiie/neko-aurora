import {execFileSync} from 'node:child_process';
import {fileURLToPath} from 'node:url';
import path from 'node:path';
import vm from 'node:vm';
const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..'),commit=process.argv[2];
if(!/^[a-f0-9]{40}$/.test(commit??''))throw Error('Exact source commit required');
const source=execFileSync('git',['show',commit+':native/internal/web/assets/app.js'],{cwd:root,encoding:'utf8'}),syncSource=source.slice(source.indexOf('async function sync(){'),source.indexOf('async function cancelWork(')),acceptSource=source.slice(source.indexOf('function acceptBotRecord('),source.indexOf('const botNameCommands='));
const original={id:'bot-one',name:'Original',alias:'original',revision:1,color:'#123456',displayRevision:0},updated={...original,name:'Updated',displayRevision:1},added={...original,id:'bot-new',name:'New',alias:'new'};
const checks=[];
async function schedule(name,ack,bots,expected){
 let release;const response=new Promise(r=>release=r),noop=()=>{},renders=[];
 const context=vm.createContext({state:{sequence:100,bots:[{...original}],source:{close:noop}},syncGeneration:0,syncRetry:null,clearTimeout:noop,request:()=>response,setHint:noop,acceptProject:noop,acceptSourceFile:noop,acceptCollaboration:noop,acceptPluginCatalog:noop,acknowledged:noop,render:()=>renders.push(context.state.bots.map(b=>b.name)),EventSource:class{close(){}addEventListener(){}},refreshAccountCatalog:noop,resumeOutbox:async()=>{},reconcileDurableCommands:async()=>{},reconcileUploadIntents:async()=>{}});
 vm.runInContext(acceptSource+syncSource,context);const pending=context.sync();if(ack){context.acceptBotRecord(ack);context.render()}release({sequence:100,runtime:'core_unavailable',capabilities:[],bots,work:[]});let error=null;try{await pending}catch(e){error=String(e)}const ok=expected(context.state.bots,error);checks.push({name,status:ok?'PASS':'FAIL',renders,error,observed:context.state.bots.map(b=>({id:b.id,name:b.name,displayRevision:b.displayRevision}))});
}
await schedule('delayed_snapshot_keeps_newer_rename_ack',updated,[original],(bots,e)=>!e&&bots[0].name==='Updated'&&bots[0].displayRevision===1);
await schedule('delayed_snapshot_keeps_created_bot_ack',added,[original],(bots,e)=>!e&&bots.some(b=>b.id==='bot-new'));
await schedule('newer_snapshot_advances_name',null,[updated],(bots,e)=>!e&&bots[0].name==='Updated'&&bots[0].displayRevision===1);
await schedule('same_counter_conflicting_identity_fails_closed',null,[{...original,name:'Forged'}],(bots,e)=>Boolean(e)&&bots[0].name==='Original');
console.log(JSON.stringify({sourceCommit:commit,scope:'Exact-source Node VM deferred snapshot/ACK schedules, no browser/service/provider.',checks},null,2));if(checks.some(c=>c.status==='FAIL'))process.exitCode=1;
