// Run: node app/scripts/ui_bot_creation_races.mjs
// Executes the actual create-bot handler with deferred requests and minimal DOM boundaries.
// No browser, service, native runtime, provider, credentials, or network are used.
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import vm from 'node:vm';
import {randomUUID} from 'node:crypto';
const source=readFileSync(new URL('../native/internal/web/assets/app.js',import.meta.url),'utf8');
const start=source.indexOf('const botForm=$("bot-form");'),end=source.indexOf('function retrySync()',start);
assert.ok(start>=0&&end>start,'create-bot source boundaries');
const results=[];
async function scenario(name,run){
 const elements=new Map(),calls=[],accepted=[],selected=[];
 function $(id){
  if(!elements.has(id))elements.set(id,{disabled:false,hidden:true,open:false,textContent:'',listeners:{},
   addEventListener(type,handler){this.listeners[type]=handler},
   showModal(){this.open=true},close(){this.open=false;this.listeners.close?.()},
   reset(){for(const field of Object.values(this.elements))field.value=''}
  });
  return elements.get(id);
 }
 $('bot-form').elements=Object.fromEntries(['name','alias','description'].map(key=>[key,{value:''}]));
 const context=vm.createContext({$,crypto:{randomUUID},navigationGeneration:0,
  FormData:class{constructor(form){this.form=form}get(key){return this.form.elements[key].value}},
  closeNavigation(){},setWorkPanel(){},render(){},
  acceptBotRecord(bot){accepted.push(bot)},selectBot(id){selected.push(id);context.navigationGeneration++},
  request(path,body){return new Promise((resolve,reject)=>calls.push({path,body,resolve,reject}))}
 });
 vm.runInContext(source.slice(start,end),context);
 const edit=(field,value)=>{$('bot-form').elements[field].value=value;$('bot-form').listeners.input?.()};
 const h={
  open:()=>$('create-bot').listeners.click(),close:()=>$('bot-close').listeners.click(),
  fill:(name,alias=name.toLowerCase())=>{edit('name',name);edit('alias',alias)},edit,
  value:field=>$('bot-form').elements[field].value,
  // Do not await the async submit: each deferred transport is settled explicitly below.
  submit:()=>{void $('bot-form').listeners.submit({preventDefault(){}})},
  settle:async(index,error)=>{const call=calls[index];if(error)call.reject(Error(error));else call.resolve({bot:{id:'bot-'+call.body.alias,...call.body}});await Promise.resolve();await Promise.resolve()},
  navigate:id=>{context.navigationGeneration++;selected.push(id)},
  read:()=>({open:$('bot-dialog').open,name:$('bot-form').elements.name.value,alias:$('bot-form').elements.alias.value,disabled:$('bot-save').disabled,status:$('bot-status').textContent,selected:[...selected],accepted:[...accepted],calls:calls.map(c=>({path:c.path,body:c.body}))})
 };
 try{await h.open();await run(h);results.push({name,status:'PASS'})}
 catch(error){results.push({name,status:'FAIL',error:error.message})}
}
await scenario('normal success resets/closes/selects; duplicate submit stays blocked',async h=>{
  await h.fill('Original');await h.submit();assert.equal((await h.read()).disabled,true);
  await h.submit();assert.equal((await h.read()).calls.length,1);
  await h.settle(0);const s=await h.read();assert.equal(s.open,false);assert.equal(s.name,'');assert.deepEqual(s.selected,['bot-original']);assert.equal(s.disabled,false);
 });
 for(const field of ['name','alias','description'])await scenario(`edit ${field} keeps newer draft after old success`,async h=>{
  await h.fill('Original');await h.submit();await h.edit(field,'newerdraft');
  await h.settle(0);const s=await h.read();assert.equal(s.open,true,'older success must not close edited dialog');assert.equal(await h.value(field),'newerdraft');assert.deepEqual(s.selected,[]);assert.equal(s.accepted.length,1);assert.equal(s.disabled,false);
 });
 await scenario('stale error and finally leave newer pending request intact',async h=>{
  await h.fill('Original');await h.submit();await h.fill('Newer');assert.equal((await h.read()).disabled,false,'edit allows new command');await h.submit();
  await h.settle(0,'409');const s=await h.read();assert.equal(s.disabled,true);assert.equal(s.status,'Đang tạo bot…');await h.settle(1);assert.deepEqual((await h.read()).selected,['bot-newer']);
 });
 await scenario('close/reopen newer request retains nonce after stale success and lost ack',async h=>{
  await h.fill('Original');await h.submit();await h.close();await h.open();await h.fill('Newer');await h.submit();
  await h.settle(0);let s=await h.read();assert.equal(s.disabled,true);assert.equal(s.name,'Newer');assert.equal(s.open,true);assert.deepEqual(s.selected,[]);
  await h.settle(1,'network');s=await h.read();assert.equal(s.disabled,false);assert.match(s.status,/Thử lại/);await h.submit();s=await h.read();assert.equal(s.calls[2].body.clientNonce,s.calls[1].body.clientNonce,'lost-ack retry must retain newer command nonce');await h.settle(2);
 });
 await scenario('close/reopen same draft retries original request',async h=>{
  await h.fill('Original');await h.submit();await h.close();await h.open();await h.submit();let s=await h.read();assert.equal(s.calls[0].body.clientNonce,s.calls[1].body.clientNonce);
  await h.settle(0,'network');s=await h.read();assert.equal(s.disabled,true);assert.equal(s.status,'Đang tạo bot…');await h.settle(1);assert.deepEqual((await h.read()).selected,['bot-original']);
 });
 await scenario('ordinary network retry retains nonce and pending button',async h=>{
  await h.fill('Original');await h.submit();await h.settle(0,'network');assert.equal((await h.read()).disabled,false);await h.submit();let s=await h.read();assert.equal(s.disabled,true);assert.deepEqual(s.calls[0].body,s.calls[1].body);await h.settle(1);assert.equal((await h.read()).open,false);
 });
 await scenario('navigation during request is not stolen by success',async h=>{
  await h.fill('Original');await h.submit();await h.navigate('bot-other');await h.settle(0);const s=await h.read();assert.deepEqual(s.selected,['bot-other']);assert.equal(s.accepted.length,1);assert.equal(s.open,true,'navigation change must preserve dialog');assert.equal(s.name,'Original');assert.equal(s.alias,'original');assert.equal(s.disabled,false);
 });
 await scenario('closed dialog ignores delayed error',async h=>{
  await h.fill('Original');await h.submit();await h.close();const before=await h.read();await h.settle(0,'409');const after=await h.read();assert.equal(after.status,before.status);assert.equal(after.open,false);assert.equal(after.disabled,false);
 });

console.log(JSON.stringify({scope:'Actual create-bot handler in Node VM; minimal DOM/request boundaries; no browser/native/service/provider claim',results},null,2));
if(results.some(r=>r.status==='FAIL'))process.exitCode=1;
