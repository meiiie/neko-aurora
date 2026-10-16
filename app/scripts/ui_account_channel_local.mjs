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
const output=path.resolve(root,options.output??'.build/evidence-account-channel-local');
await mkdir(output,{recursive:true});
const temporary=await mkdtemp(path.join(root,'.build','neko-local-channel-'));
const binary=path.join(root,'.build','neko-control.exe');
const service=spawn(binary,['--listen','127.0.0.1:0','--state',path.join(temporary,'state'),'--host-executable',path.resolve(options.host)],{cwd:root,windowsHide:true,stdio:['ignore','pipe','pipe']});
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
 let serial=0;const pending=new Map();socket.addEventListener('message',event=>{const value=JSON.parse(event.data),entry=pending.get(value.id);if(entry){clearTimeout(entry.timer);pending.delete(value.id);value.error?entry.reject(new Error(value.error.message)):entry.resolve(value.result)}});
 call=(method,params={})=>new Promise((resolve,reject)=>{const id=++serial,timer=setTimeout(()=>{pending.delete(id);reject(new Error('CDP timeout '+method))},10000);pending.set(id,{resolve,reject,timer});socket.send(JSON.stringify({id,method,params}))});
 evaluate=async expression=>{const result=await call('Runtime.evaluate',{expression,returnByValue:true,awaitPromise:true});if(result.exceptionDetails)throw new Error('Browser evaluation failed');return result.result.value};
 const api=async(url,body)=>evaluate('(async()=>{const r=await fetch('+JSON.stringify(url)+','+JSON.stringify(body===undefined?{}:{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)})+');return {status:r.status,data:await r.json()}})()');
 const select=async(id,value)=>evaluate('(()=>{const e=document.getElementById('+JSON.stringify(id)+');e.value='+JSON.stringify(value)+';e.dispatchEvent(new Event("change",{bubbles:true}))})()');
 const send=async text=>{await evaluate('document.getElementById("prompt").value='+JSON.stringify(text)+';document.getElementById("prompt").dispatchEvent(new Event("input"));document.getElementById("composer").requestSubmit()');await until(()=>evaluate('inFlight.size===0'));return (await api('/v1/snapshot')).data.work.find(work=>work.text===text)};
 const shot=async(name,width,height,mobile=false)=>{await call('Emulation.setDeviceMetricsOverride',{width,height,deviceScaleFactor:1,mobile});await evaluate('document.fonts.ready');await delay(250);const value=await call('Page.captureScreenshot',{format:'png'}),bytes=Buffer.from(value.data,'base64'),filename=name+'.png';await writeFile(path.join(output,filename),bytes);shots.push({filename,width,height,sha256:sha(bytes)})};
 await call('Page.enable');await call('Runtime.enable');await call('Page.navigate',{url:base});await until(()=>evaluate('document.getElementById("bot-name")?.textContent==="Meiiie"'));
 const assetHashes={};for(const name of ['index.html','style.css','app.js','connections.js']){const served=await evaluate('fetch('+JSON.stringify(name==='index.html'?'/':'/'+name)+').then(r=>r.text())'),source=await readFile(path.join(root,'native/internal/web/assets',name));assetHashes[name]=sha(source);check('actual_embedded_'+name+'_matches_source',sha(Buffer.from(served,'utf8'))===assetHashes[name])}
 const snapshot=(await api('/v1/snapshot')).data;check('actual_go_channel_account_capabilities',snapshot.capabilities.includes('local_channels_v1')&&snapshot.capabilities.includes('account_lease_fixture_v1'));
 const accountInput=name=>({id:'',expectedRevision:0,clientNonce:randomUUID(),name,provider:'openai-compatible',endpoint:'https://example.invalid/v1',model:'fixture-ui-model',secret:'fixture-only-not-a-key'});
 const a=(await api('/v1/accounts',accountInput('Fixture Account A'))).data.account,b=(await api('/v1/accounts',accountInput('Fixture Account B'))).data.account;
 check('synthetic_accounts_stored_unverified',a?.status==='stored_unverified'&&b?.status==='stored_unverified');
 const alpha=(await api('/v1/channels',{id:'',name:'Nhóm kiểm tra local',expectedRevision:0,botIds:['bot-meiiie','bot-davin'],clientNonce:randomUUID()})).data.channel;
 check('actual_go_channel_created',alpha?.revision===1&&alpha.botIds.length===2);
 await evaluate('sync()');await until(()=>evaluate('state.accounts.length===2&&document.querySelectorAll("#channels button").length===1'));
 await evaluate('selectChannel('+JSON.stringify(alpha.id)+')');await select('recipient-choice','bot-meiiie');await select('account-choice',a.id+'@'+a.revision);
 const work=await send('@davin Kiểm tra cấu hình tài khoản và giữ reply trong nhóm gốc.');
 check('actual_go_server_pins_selected_account',work?.accountPin?.accountId===a.id&&work.accountPin.revision===a.revision);
 check('actual_go_tag_keeps_group_origin',work?.botId==='bot-davin'&&work.sourceBotId==='bot-meiiie'&&work.publication?.replyChannelId===alpha.id&&work.conversationId===alpha.id);
 check('ack_keeps_source_group_view',await evaluate('state.channelId==='+JSON.stringify(alpha.id)+'&&state.botId==="bot-meiiie"&&document.getElementById("prompt").value===""'));
 await select('account-choice',b.id+'@'+b.revision);const unchanged=(await api('/v1/snapshot')).data.work.find(value=>value.requestId===work.requestId);
 check('new_selection_does_not_rebind_saved_work',unchanged.accountPin.accountId===a.id);
 await shot('account-channel-desktop',1280,720);
 check('actual_go_desktop_resize_keeps_compact_composer_52px',await evaluate('document.getElementById("composer").getBoundingClientRect().height===52'));
 const remove=await api('/v1/accounts/remove',{id:a.id,expectedRevision:a.revision});check('actual_go_local_account_removed',remove.status===200);
 await evaluate('refreshAccountCatalog()');await until(()=>evaluate('state.work.get('+JSON.stringify(work.requestId)+')?.status==="suspended"'));
 check('actual_go_revocation_fences_bound_work',await evaluate('state.work.get('+JSON.stringify(work.requestId)+')?.failure==="account_authority_changed"'));
 check('revoked_work_explains_immutable_old_account',await evaluate('document.querySelector(".task-authority").textContent.includes("Quyền tài khoản gốc đã thay đổi")'));
 const second=await send('Yêu cầu mới dùng tài khoản B.');check('new_intake_uses_explicit_current_account',second?.accountPin?.accountId===b.id);
 const cas=snapshot.capabilities.includes('local_channel_intake_cas_v1'),paused=[];let originalBody,originalReceipt;
 if(cas){socket.addEventListener('message',event=>{const value=JSON.parse(event.data);if(value.method==='Fetch.requestPaused')paused.push(value.params)});await evaluate('state.source.close();state.source=null');await call('Fetch.enable',{patterns:[{urlPattern:base+'/v1/messages',requestStage:'Response'}]});await evaluate('document.getElementById("prompt").value="Fixture lost ACK original authority";document.getElementById("composer").requestSubmit()');await until(()=>paused.length);const held=paused.shift();originalBody=JSON.parse(held.request.postData);const bytes=await call('Fetch.getResponseBody',{requestId:held.requestId});originalReceipt=JSON.parse(bytes.base64Encoded?Buffer.from(bytes.body,'base64').toString('utf8'):bytes.body).work;check('actual_cas_ui_freezes_channel_revision',originalBody.expectedChannelRevision===1&&originalBody.accountId===b.id&&originalBody.accountRevision===b.revision);await call('Fetch.fulfillRequest',{requestId:held.requestId,responseCode:503,responseHeaders:[{name:'Content-Type',value:'application/json'}],body:Buffer.from('{"error":"fixture lost ACK"}').toString('base64')});await call('Fetch.disable');await until(()=>evaluate('inFlight.size===0&&outbox.length===1'))}
 const changed=(await api('/v1/channels',{id:alpha.id,name:alpha.name,expectedRevision:alpha.revision,botIds:['bot-meiiie'],clientNonce:randomUUID()})).data.channel;
 if(cas)await evaluate('acceptChannel('+JSON.stringify(changed)+');render()');else await until(()=>evaluate('state.channels.find(c=>c.id==='+JSON.stringify(alpha.id)+')?.revision===2'));
 check('actual_go_acl_revision_updates_ui',changed?.revision===2&&await evaluate('document.getElementById("recipient-choice").options.length===1'));
 check('acl_change_never_rebinds_old_publication',(await api('/v1/snapshot')).data.work.find(value=>value.requestId===second.requestId).publication.audienceRevision===1);
 if(cas){await call('Fetch.enable',{patterns:[{urlPattern:base+'/v1/messages',requestStage:'Request'}]});await evaluate('document.querySelector(".outbox-rejected button:not([hidden])").click()');await until(()=>paused.length);const held=paused.shift(),body=JSON.parse(held.request.postData);check('actual_cas_retry_keeps_full_original_tuple',JSON.stringify(body)===JSON.stringify(originalBody));await call('Fetch.continueRequest',{requestId:held.requestId});await call('Fetch.disable');await until(()=>evaluate('inFlight.size===0&&outbox.length===0'));const retained=(await api('/v1/snapshot')).data.work.filter(value=>value.clientNonce===originalBody.clientNonce);check('actual_cas_lost_ack_returns_original_suspended_work',retained.length===1&&retained[0].requestId===originalReceipt.requestId&&retained[0].status==='suspended'&&retained[0].publication.audienceRevision===1);const stale=await api('/v1/messages',{...originalBody,clientNonce:randomUUID()});check('actual_cas_new_stale_nonce_rejected',stale.status===409);await evaluate('sync()')}
 if(cas)check('actual_cas_retry_ack_clears_only_original_draft',await evaluate('document.getElementById("prompt").value===""'));
 await call('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});await delay(200);await evaluate('document.getElementById("nav-open").click()');await delay(200);
 check('mobile_shared_navigation_focus_trapped',await evaluate('document.getElementById("main").inert&&document.getElementById("sidebar").contains(document.activeElement)'));
 await evaluate('closeNavigation()');await shot('account-channel-mobile',390,844,true);
 check('mobile_select_targets_44px',await evaluate('["account-choice","recipient-choice"].every(id=>{const r=document.getElementById(id).getBoundingClientRect();return r.height>=44&&r.width>=44})'));
 await evaluate('document.getElementById("channel-settings").click()');check('local_acl_form_has_current_revision',await evaluate('channelEditing.revision===2&&document.getElementById("channel-dialog").open'));
 await shot('account-channel-acl-form-mobile',390,844,true);await evaluate('document.getElementById("channel-close").click()');
 await shot('account-channel-short',390,260,true);check('short_viewport_send_is_visible',await evaluate('(()=>{const r=document.getElementById("send").getBoundingClientRect();return r.top>=0&&r.bottom<=innerHeight&&r.width>=44&&r.height>=44&&document.documentElement.scrollWidth<=innerWidth})()'));
 const report={status:'PASS',codeCommit:commit,workingTree:dirty,command:'node scripts/ui_account_channel_local.mjs --host '+options.host+(options.commit?' --commit '+options.commit:''),environment:{platform:process.platform,node:process.version,browser:await call('Browser.getVersion')},scope:'Actual Go control plane and journal; existing Rust DPAPI host; synthetic accounts; no Core, model, provider, cloud, external channel or grant. Screenshots are headless browser pixels, not phone/native acceptance.',casCapability:snapshot.capabilities.includes('local_channel_intake_cas_v1'),visualAcceptance:'approved_light_direction_only_new_controls_need_review',checks,shots,goSha256:sha(await readFile(binary)),hostSha256:sha(await readFile(options.host)),harnessSha256:sha(await readFile(fileURLToPath(import.meta.url))),assetSha256:Object.fromEntries(await Promise.all(['index.html','style.css','app.js','connections.js'].map(async name=>[name,sha(await readFile(path.join(root,'native/internal/web/assets',name)))])))};
 await writeFile(path.join(output,'results.json'),JSON.stringify(report,null,2)+'\n');console.log(JSON.stringify({status:report.status,codeCommit:commit,workingTree:dirty,checks:checks.length,output},null,2));
}catch(error){await writeFile(path.join(output,'failed-run.json'),JSON.stringify({status:'FAIL',codeCommit:commit,workingTree:dirty,checks,error:error.message},null,2)+'\n');throw error}
finally{if(call)await call('Browser.close').catch(()=>{});socket?.close();if(browser?.exitCode===null)browser.kill();if(service.exitCode===null)service.kill();}
