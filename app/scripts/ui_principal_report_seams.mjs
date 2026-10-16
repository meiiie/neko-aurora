import fs from 'node:fs';
import vm from 'node:vm';
import crypto from 'node:crypto';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..');
const output=process.argv[2];if(!output?.replaceAll('\\','/').startsWith('E:/CodexScratch/principal-device-boundary-20261012/ui/'))throw Error('Owned output required');
fs.mkdirSync(output,{recursive:true});
const source=fs.readFileSync(path.join(root,'native/internal/web/assets/app.js'),'utf8');
function fn(name){const start=source.search(new RegExp('^(?:async )?function '+name+'\\(','m'));if(start<0)throw Error('missing '+name);const rest=source.slice(start),end=rest.slice(1).search(/\n(?:async )?function |\n(?:const |let |\$\()/);return end<0?rest:rest.slice(0,end+1)}
const names=['strictKeys','journalID','journalCounter','journalNonce','canonicalCommandInput','reportInput','validReportArtifact','validReportDelivery','acceptReportDelivery','canonicalJSON','submitReportDelivery','runReportDelivery','sessionCurrent','requireSession'];
const context=vm.createContext({TextEncoder,JSON,Date,Number,Object,Array,Map,Set,crypto:{randomUUID:()=> 'fixture-report-race-nonce'}});
vm.runInContext(names.map(fn).join('\n')+`
const sessionState={generation:1,ready:true,metadata:{actorId:'local-owner'}};
const state={botId:'bot-meiiie',channelId:'channel-'+ 'a'.repeat(32),readOnly:false,reportDeliveries:new Map(),channels:[{id:'channel-'+'a'.repeat(32),selfRole:'writer'}]};
function fixtureSession(){return true}function sessionOwner(){return sessionState.metadata.actorId==='local-owner'}
const reportCommands=new Map(),views=new Map();let navigationGeneration=1;
function viewKey(){return state.channelId}function setHint(){}function render(){}function restoreDurableMaps(){}
let resumePreparation;function loadDurableCommands(){return new Promise(resolve=>resumePreparation=resolve)}
const posts=[];async function retainDurableCommand(){}async function request(url,body){posts.push({url,body});return {delivery:body}}function reportMatches(){return true}async function sync(){}async function retireDurableCommand(){return true}function durableFailure(){return false}function $(){return {value:''}}
const report={version:1,id:'report-delivery-'+'b'.repeat(32),sourceMessageId:'msg-fixture',ownerUserId:'local-owner',spaceId:'local-space',botId:'bot-meiiie',destinationChannelId:state.channelId,expectedDestinationRevision:1,clientNonce:'fixture-report-nonce-0001',text:'@meiiie send the report here',status:'delivered',createdAt:new Date().toISOString(),createdSequence:1,reply:'Published result',artifacts:[],active:true,source:{requestId:'req-private',conversationId:'conversation-bot-meiiie',messageId:'msg-private',epoch:1,accessRevision:0,conversationRevision:0,botStateRevision:0,resultSha256:'f'.repeat(64)}};
`,context);
const checks=[];function check(name,expression){const pass=Boolean(vm.runInContext(expression,context));checks.push({name,status:pass?'PASS':'FAIL'});if(!pass)process.exitCode=1}
check('owner_requires_exact_private_source_receipt',`validReportDelivery(report)&&!validReportDelivery({...report,source:undefined})`);
vm.runInContext(`sessionState.metadata.actorId='fixture-bob';const projected={...report};delete projected.source;`,context);
check('member_accepts_public_report_without_private_source',`acceptReportDelivery(projected)&&state.reportDeliveries.size===1&&!Object.hasOwn(state.reportDeliveries.get(projected.id),'source')`);
check('member_rejects_source_details_instead_of_rendering_them',`!validReportDelivery(report)`);
vm.runInContext(`state.channels=[];`,context);check('member_requires_current_visible_destination',`!validReportDelivery(projected)`);
vm.runInContext(`sessionState.metadata.actorId='local-owner';const body={botId:'bot-meiiie',destinationChannelId:state.channelId,expectedDestinationRevision:1,text:'Send the report here'};globalThis.preparation=submitReportDelivery(body,body.text).then(()=>null).catch(e=>e.message);sessionState.generation=2;navigationGeneration=2;resumePreparation();`,context);
const failure=await context.preparation;
check('old_report_preparation_cannot_acquire_new_session_authority',`posts.length===0&&reportCommands.size===0`);
checks.push({name:'stale_preparation_stops_with_session_changed',status:failure==='session_changed'?'PASS':'FAIL'});if(failure!=='session_changed')process.exitCode=1;
const receipt={applicationSHA256:crypto.createHash('sha256').update(source).digest('hex'),harnessSHA256:crypto.createHash('sha256').update(fs.readFileSync(fileURLToPath(import.meta.url))).digest('hex'),scope:'Actual extracted validators/preparation functions in isolated Node VM, authored authority/transport; no GUI/real authority/provider',checks};
fs.writeFileSync(path.join(output,'result.json'),JSON.stringify(receipt,null,2)+'\n');console.log(JSON.stringify(receipt));
