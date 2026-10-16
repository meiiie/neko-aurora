import fs from 'node:fs';
import path from 'node:path';
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {stripTypeScriptTypes} from 'node:module';
import {fileURLToPath,pathToFileURL} from 'node:url';
import {historyRepairCases} from './history-repair-cases.mjs';
import {historyStorageCases} from './history-storage-cases.mjs';

const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..');
const output=path.resolve(process.argv[2]??'E:/CodexScratch/wiii-current-repair-20261013/history-tests');
if(!output.startsWith('E:'+path.sep+'CodexScratch'+path.sep+'wiii-current-repair-20261013'+path.sep))throw Error('Owned evidence path required');
fs.mkdirSync(output,{recursive:true});
const modules=fs.mkdtempSync(path.join(output,'source-modules-')),sources={};
for(const name of ['nekobot-local-protocol','history-repair','history-cache','nekobot-local-bridge','nekobot-native']) {
 const file=path.join(root,'src/neko-chill',name+'.ts');if(!fs.existsSync(file))continue;
 const raw=fs.readFileSync(file,'utf8');sources[name]=createHash('sha256').update(raw).digest('hex');
 fs.writeFileSync(path.join(modules,name+'.mjs'),stripTypeScriptTypes(raw,{mode:'transform'}).replace(/from "\.\/(.*?)"/g,'from "./$1.mjs"'));
}
const bridgeModule=await import(pathToFileURL(path.join(modules,'nekobot-local-bridge.mjs')).href);
const checks=[];
async function check(name,operation){try{await operation();checks.push({name,status:'PASS'});}catch(error){checks.push({name,status:'FAIL',reason:String(error.message)});process.exitCode=1;}}
await check('explicit_existing_session_history_repair_entrypoint',()=>assert.equal(typeof bridgeModule.NekoBotLocalBridge.prototype.repairHistory,'function'));
if(fs.existsSync(path.join(modules,'history-repair.mjs'))) {
 const repair=await import(pathToFileURL(path.join(modules,'history-repair.mjs')).href);
 const vectors=JSON.parse(fs.readFileSync(process.argv[3]??'C:/Users/Admin/Documents/Codex/2026-10-11/task/checkpoint-verify-20261011/scripts/adapter_history_repair_vectors_20261013.json','utf8'));
 for(const vector of vectors.cases) await check('portable_digest_'+vector.name,async()=>{
  assert.equal(repair.scopeBytes(vector.scope).length,vector.expected.scopeBytes);
  assert.equal(await repair.sha256(repair.scopeBytes(vector.scope)),vector.expected.scopeSHA256);
  const result=await repair.historyWitness(vector.scope,vector.snapshotCursor,vector.expected.authorityDigest,vector.rows);
  assert.equal(result.totalBytes,vector.expected.totalBytes);assert.equal(result.historyDigest,vector.expected.historyDigest);assert.equal(result.snapshotId,vector.expected.snapshotId);
 });
 const native=await import(pathToFileURL(path.join(modules,'nekobot-native.mjs')).href);
 await historyRepairCases(check,bridgeModule,repair,native);
}
const storage=await historyStorageCases(check,modules);
const result={status:checks.every(x=>x.status==='PASS')?'PASS':'FAIL',checks,sources,storage,node:process.version,command:process.argv,scope:'Actual current source TypeScript modules, synthetic storage/transport only; no browser/native/provider/user state.'};
fs.writeFileSync(path.join(output,'result.json'),JSON.stringify(result,null,2)+'\n');console.log(JSON.stringify(result,null,2));
