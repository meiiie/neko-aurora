// Run: node app/scripts/ui_discovery_loading_races.mjs
// Actual Connections IIFE, deferred read-only requests, minimal DOM boundaries.
// No browser rendering, service, native runtime, credentials, or provider claim.
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import vm from 'node:vm';
const source=readFileSync(new URL('../native/internal/web/assets/connections.js',import.meta.url),'utf8');
const start=source.indexOf('(()=>{'),end=source.indexOf('\n})();',start);
assert.ok(start>=0&&end>start,'Connections source boundaries');
const results=[];
const report=version=>({version:1,mode:'read_only_discovery',candidates:[{origin:'installed',pinnedVersion:version}],execution:'not_started',credentialAccess:'not_requested'});
async function scenario(name,run){
 const elements=new Map(),calls=[];
 const document={activeElement:null,getElementById:$,createElement:()=>make()};
 function make(){return {disabled:false,hidden:true,open:false,value:'',textContent:'',listeners:{},dataset:{},
  addEventListener(type,handler){(this.listeners[type]??=[]).push(handler)},
  emit(type){for(const handler of this.listeners[type]??[])handler({preventDefault(){}})},
  showModal(){this.open=true},close(){this.open=false;this.emit('close')},
  reset(){for(const field of Object.values(this.elements))field.value=''},
  focus(){document.activeElement=this},contains(el){return this===el},scrollIntoView(){},
  replaceChildren(...children){this.children=children},append(){},setAttribute(){},querySelector(){return $('tabs')}
 }}
 function $(id){if(!elements.has(id))elements.set(id,make());return elements.get(id)}
 $('account-form').elements=Object.fromEntries(['name','provider','endpoint','model'].map(key=>[key,make()]));
 const sessionState={ready:true,generation:1};
 const context=vm.createContext({document,window:{},sessionState,
  sessionOwner:()=>true,sessionCurrent:epoch=>epoch===sessionState.generation,fixtureSession:()=>false,
  state:{readOnly:false,accountChoice:null,accountCatalogRevision:0},navigationGeneration:0,
  canonicalJSON:JSON.stringify,viewKey:()=> 'view',closeNavigation(){},setWorkPanel(){},
  acceptAccountCatalog(){},policyEnabled:()=>false,
  requestRaw(path){if(path==='/v1/connections')return Promise.resolve({revision:0,accounts:[],plugins:[]});assert.equal(path,'/v1/core-discovery');return new Promise((resolve,reject)=>calls.push({resolve,reject}))}
 });
 vm.runInContext(source.slice(start,end+'\n})();'.length),context);
 const flush=async()=>{for(let i=0;i<8;i++)await Promise.resolve()};
 const h={click:id=>$(id).emit('click'),$,calls,flush,
  settle:async(index,value=report('test'))=>{if(value instanceof Error)calls[index].reject(value);else calls[index].resolve(value);await flush()},
  sessionChange:()=>{sessionState.generation++;context.window.nekoConnections.sessionChanged()},
  loading:()=>{assert.equal($('core-discovery-retry').disabled,true);assert.equal($('core-discovery-status').textContent,'Đang kiểm tra bản cài đặt…')},
  idle:()=>{assert.equal($('core-discovery-retry').disabled,false,'settled discovery must enable retry');assert.notEqual($('core-discovery-status').textContent,'Đang kiểm tra bản cài đặt…')}
 };
 try{await run(h);results.push({name,status:'PASS'})}catch(error){results.push({name,status:'FAIL',error:error.message})}
}
for(const back of [false,true])for(const outcome of [report('old'),new Error('network')])await scenario(`editor${back?' then back':''} releases settled discovery (${outcome instanceof Error?'failure':'success'})`,async h=>{
 h.click('connections-open');await h.flush();h.loading();h.click('account-add');if(back)h.click('account-back');await h.settle(0,outcome);h.idle();
 h.click('core-discovery-retry');h.loading();await h.settle(1,report('retry'));h.idle();assert.match(h.$('core-discovery-status').textContent,/retry/);
});
await scenario('close/reopen ignores old result and finally while new discovery is pending',async h=>{
 h.click('connections-open');h.click('connections-close');h.click('connections-open');h.loading();await h.settle(0,report('old'));h.loading();await h.settle(1,report('new'));h.idle();assert.match(h.$('core-discovery-status').textContent,/new/);
});
await scenario('editor retry is not cleared by older request',async h=>{
 h.click('connections-open');h.click('account-add');h.click('core-discovery-retry');await h.settle(0);h.loading();await h.settle(1);h.idle();
});
await scenario('same-view overlapping discovery keeps latest response and loading owner',async h=>{
 h.click('connections-open');h.click('core-discovery-retry');await h.settle(0);h.loading();await h.settle(1,report('new'));h.idle();assert.match(h.$('core-discovery-status').textContent,/new/);
});
await scenario('late old response cannot overwrite newer completed discovery',async h=>{
 h.click('connections-open');h.click('core-discovery-retry');await h.settle(1,report('new'));await h.settle(0,report('old'));h.idle();assert.match(h.$('core-discovery-status').textContent,/new/);
});
await scenario('session reset clears loading and ignores old response',async h=>{
 h.click('connections-open');h.sessionChange();h.idle();await h.settle(0);h.idle();assert.equal(h.$('core-discovery-status').textContent,'Chưa kiểm tra được Core.');
});
console.log(JSON.stringify({scope:'Actual Connections handlers in Node VM; minimal DOM/request boundaries, no browser acceptance',results},null,2));
if(results.some(r=>r.status==='FAIL'))process.exitCode=1;
