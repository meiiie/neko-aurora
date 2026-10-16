import { afterEach, expect, spyOn, test } from "bun:test";
import { spawnSync } from "node:child_process";
import { createHash, createHmac } from "node:crypto";
import * as fs from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { createSavedAccountWitnessAuthority } from "../src/adapters/saved-account-witness.ts";
import { loadSavedAccountBridge, prepareSavedAccountCapsule, type SavedAccountBinding } from "../src/adapters/saved-account-bridge.ts";
import { getProvider } from "../src/adapters/providers.ts";
import { isJsonObject, isJsonNumber, type JsonValue } from "../src/shared/wire.ts";
const KEY="6".repeat(64), BOOT="executor-boot-"+"c".repeat(32), AUTH="d".repeat(64),roots:string[]=[];
afterEach(()=>{for(const root of roots.splice(0))fs.rmSync(root,{recursive:true,force:true});});
function canonical(v:JsonValue):string {if(Array.isArray(v))return "["+v.map(canonical).join(",")+"]";if(isJsonObject(v))return "{"+Object.keys(v).sort().map(k=>JSON.stringify(k)+":"+canonical(v[k]!)).join(",")+"}";return JSON.stringify(v);}
function fixture(now=Date.now()) {
 const root=fs.mkdtempSync(join(tmpdir(),"saved-witness-"));roots.push(root);const witnessPath=join(root,"authority.json"),stateHome=join(root,"state");fs.mkdirSync(stateHome);
 const binding:SavedAccountBinding={fixtureOnly:true,accountId:"account-"+"a".repeat(32),accountRevision:1,credentialGeneration:1,provider:"openai-compatible",endpoint:"http://127.0.0.1:49152/v1",model:"fixture-model",requestId:"fixture-request",attemptId:"fixture-attempt",epoch:1,leaseFence:1,contextSHA256:"b".repeat(64),consentId:"fixture-consent",consentRevision:1,expiresAtUnixMS:now+30000,limits:{maxInputBytes:16000,maxOutputTokens:256,maxRequests:2,maxSteps:2,maxSeconds:30}};
 const options={binding,witnessPath,serviceGeneration:BOOT,authoritySHA256:AUTH,revisionKey:KEY};
 function write(revision=1,state="active",issuedAtUnixMS=Date.now(),validUntilUnixMS=issuedAtUnixMS+5000) {
  const body={version:1,purpose:"saved_account_authority_v1",bindingSHA256:createHash("sha256").update(canonical(binding)).digest("hex"),serviceGeneration:BOOT,authoritySHA256:AUTH,revision,state,issuedAtUnixMS,validUntilUnixMS};
  const key=createHmac("sha256",Buffer.from(KEY,"hex")).update("neko-saved-key-v1/authority-witness").digest();const bytes=canonical({...body,mac:createHmac("sha256",key).update(canonical(body)).digest("hex")});
  fs.writeFileSync(witnessPath+".next",bytes);fs.renameSync(witnessPath+".next",witnessPath);return bytes;
 }
 return {root,stateHome,binding,options,write};
}
function interceptRead(effect:()=>void) {
 const original=fs.readSync;
 return spyOn(fs,"readSync").mockImplementation((fd:number,buffer:NodeJS.ArrayBufferView,offsetOrOptions?:number|fs.ReadOptions,length?:number,position?:fs.ReadPosition|null)=>{
  const count=isJsonNumber(offsetOrOptions)?original(fd,buffer,offsetOrOptions,length!,position??null):original(fd,buffer,offsetOrOptions);
  effect();return count;
 });
}
const posix=test.skipIf(process.platform==="win32");
posix("literal Go vector binds the complete task boot authority and key",()=>{
 const now=2000000000000,f=fixture(now),clock=spyOn(Date,"now").mockReturnValue(now);
 try {
  fs.writeFileSync(f.options.witnessPath,'{"authoritySHA256":"dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd","bindingSHA256":"27867e36eb37e8a769ebcbfc19294290da63aa13c7b108ebcf994c1575b6e0c3","issuedAtUnixMS":2000000000000,"mac":"1d1a9eff90e41bff8a6364052504efcf2ee3c9fb486952c678bc8ba5493f1148","purpose":"saved_account_authority_v1","revision":1,"serviceGeneration":"executor-boot-cccccccccccccccccccccccccccccccc","state":"active","validUntilUnixMS":2000000005000,"version":1}');
  const authority=createSavedAccountWitnessAuthority(f.options);expect(authority(f.binding)).toBe(true);
  for(const change of [{serviceGeneration:"executor-boot-"+"e".repeat(32)},{authoritySHA256:"e".repeat(64)},{revisionKey:"7".repeat(64)},{binding:{...f.binding,attemptId:"different-attempt"}}])expect(()=>createSavedAccountWitnessAuthority({...f.options,...change})).toThrow("Saved account authority unavailable");
  expect(authority({...f.binding,limits:{...f.binding.limits,maxRequests:3}})).toBe(false);expect(authority(f.binding)).toBe(false);
 }finally{clock.mockRestore();}
});
posix("atomic renewal permits skipped revisions but rollback retirement and deletion poison",()=>{
 for(const kind of ["rollback","retired","same-revision-change","delete"]){const f=fixture(),now=Date.now(),one=f.write(1,"active",now),authority=createSavedAccountWitnessAuthority(f.options);f.write(3,"active",now);expect(authority(f.binding)).toBe(true);
 if(kind==="rollback")fs.writeFileSync(f.options.witnessPath,one);if(kind==="retired")f.write(4,"retired",Date.now(),Date.now());if(kind==="same-revision-change")f.write(3,"active",now-1);if(kind==="delete")fs.unlinkSync(f.options.witnessPath);
 expect(authority(f.binding)).toBe(false);f.write(5);expect(authority(f.binding)).toBe(false);}
});
posix("frozen wall time and newer revisions cannot extend unchanged proof expiry",()=>{
 const now=Date.now(),f=fixture(now),wall=spyOn(Date,"now").mockReturnValue(now),mono=spyOn(performance,"now").mockReturnValue(100);
 try{f.write(1,"active",now);const authority=createSavedAccountWitnessAuthority(f.options);mono.mockReturnValue(4100);expect(authority(f.binding)).toBe(true);f.write(2,"active",now);expect(authority(f.binding)).toBe(true);mono.mockReturnValue(5100);expect(authority(f.binding)).toBe(false);f.write(3,"active",now);expect(authority(f.binding)).toBe(false);}finally{wall.mockRestore();mono.mockRestore();}
});
posix("expired oversized noncanonical and aliased files are refused",()=>{
 for(const kind of ["expired","long-lived","duplicate","newline","hardlink","symlink","ancestor","oversized","bom"]){const f=fixture(),now=Date.now(),bytes=f.write();
 if(kind==="bom")fs.writeFileSync(f.options.witnessPath,"\ufeff"+bytes);if(kind==="expired")f.write(1,"active",now-5000,now);if(kind==="long-lived")f.write(1,"active",now,now+5001);if(kind==="duplicate")fs.writeFileSync(f.options.witnessPath,bytes.replace('{','{"version":1,'));if(kind==="newline")fs.appendFileSync(f.options.witnessPath,"\n");if(kind==="oversized")fs.writeFileSync(f.options.witnessPath," ".repeat(4097));if(kind==="hardlink")fs.linkSync(f.options.witnessPath,join(f.root,"alias"));
 if(kind==="symlink"){fs.renameSync(f.options.witnessPath,join(f.root,"target"));fs.symlinkSync(join(f.root,"target"),f.options.witnessPath);}if(kind==="ancestor"){fs.symlinkSync(f.root,join(f.root,"alias"));f.options.witnessPath=join(f.root,"alias","authority.json");}
 expect(()=>createSavedAccountWitnessAuthority(f.options)).toThrow("Saved account authority unavailable");}
});
posix("replacement inside a read rejects even valid signed replacement bytes",()=>{
 const f=fixture();f.write();let swapped=false;
 const read=interceptRead(()=>{if(!swapped){swapped=true;f.write();}});
 try{expect(()=>createSavedAccountWitnessAuthority(f.options)).toThrow("Saved account authority unavailable");expect(swapped).toBe(true);}finally{read.mockRestore();}
});
posix("bridge reload and witness renewal retain the original consumed request budget",async()=>{
 let calls=0;const server=Bun.serve({hostname:"127.0.0.1",port:0,fetch:()=>{calls++;return new Response('data: {"choices":[{"delta":{"content":"ok"},"finish_reason":null}]}\n\ndata: {"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"completion_tokens":1}}\n\ndata: [DONE]\n\n',{headers:{"Content-Type":"text/event-stream"}});}});
 try{const f=fixture();f.binding.endpoint=`http://127.0.0.1:${server.port}/v1`;f.write();const currentAuthority=createSavedAccountWitnessAuthority(f.options),capsulePath=join(f.root,"capsule.json");const r=prepareSavedAccountCapsule({version:1,purpose:"saved_account_task_v1",binding:f.binding,secret:"synthetic-private-key",capsulePath,stateHome:f.stateHome,revisionKey:KEY},currentAuthority);const load=()=>loadSavedAccountBridge({binding:f.binding,capsulePath,capsuleId:r.capsuleId,stateHome:f.stateHome,revisionKey:KEY,currentAuthority});
 await getProvider(load()).complete([{role:"user",content:"one"}]);f.write(2);await getProvider(load()).complete([{role:"user",content:"two"}]);f.write(3);await expect(getProvider(load()).complete([{role:"user",content:"three"}])).rejects.toThrow();expect(calls).toBe(2);f.write(4,"retired",Date.now(),Date.now());expect(load).toThrow("Saved account bridge unavailable");
 }finally{await server.stop(true);}
});
posix("expiry reached during filesystem verification denies the current read",()=>{
 const now=Date.now(),f=fixture(now),clock=spyOn(Date,"now").mockReturnValue(now);f.write(1,"active",now);const authority=createSavedAccountWitnessAuthority(f.options);
 const read=interceptRead(()=>clock.mockReturnValue(now+5000));
 try{expect(authority(f.binding)).toBe(false);clock.mockReturnValue(now);expect(authority(f.binding)).toBe(false);}finally{read.mockRestore();clock.mockRestore();}
});
posix("witness retirement aborts an actual silent bridge request without another network call",async()=>{
 let calls=0,enter!:()=>void,release!:(response:Response)=>void;const entered=new Promise<void>(r=>enter=r),held=new Promise<Response>(r=>release=r);
 const server=Bun.serve({hostname:"127.0.0.1",port:0,fetch:async()=>{calls++;enter();return held;}});
 try{const f=fixture();f.binding.endpoint=`http://127.0.0.1:${server.port}/v1`;f.write();const currentAuthority=createSavedAccountWitnessAuthority(f.options),capsulePath=join(f.root,"capsule.json");const r=prepareSavedAccountCapsule({version:1,purpose:"saved_account_task_v1",binding:f.binding,secret:"synthetic-private-key",capsulePath,stateHome:f.stateHome,revisionKey:KEY},currentAuthority);const cfg=loadSavedAccountBridge({binding:f.binding,capsulePath,capsuleId:r.capsuleId,stateHome:f.stateHome,revisionKey:KEY,currentAuthority});
 const pending=getProvider(cfg).complete([{role:"user",content:"fixture held request"}]).then(()=>"unsafe success",()=>"stopped");await entered;const now=Date.now();f.write(2,"retired",now,now);expect(await Promise.race([pending,new Promise<string>(r=>setTimeout(()=>r("not stopped"),750))])).toBe("stopped");expect(calls).toBe(1);f.write(3);expect(()=>cfg.profileBridge!.assertCurrent(cfg)).toThrow("Saved account bridge unavailable");
 }finally{release(new Response("fixture closed",{status:503}));await server.stop(true);}
});
test.skipIf(process.platform!=="win32")("Windows witness consumption remains denied until native file identity is verified",()=>{
 const f=fixture();f.write();expect(()=>createSavedAccountWitnessAuthority(f.options)).toThrow("Saved account authority unavailable");
});
posix("a reader pause may skip expired proofs when the service has issued a current renewal",()=>{
 const now=Date.now(),f=fixture(now),wall=spyOn(Date,"now").mockReturnValue(now),mono=spyOn(performance,"now").mockReturnValue(100);
 try{f.write(1,"active",now);const authority=createSavedAccountWitnessAuthority(f.options);wall.mockReturnValue(now+6000);mono.mockReturnValue(6100);f.write(4,"active",now+6000);expect(authority(f.binding)).toBe(true);
 wall.mockReturnValue(now+11000);mono.mockReturnValue(11100);expect(authority(f.binding)).toBe(false);f.write(5,"active",now+11000);expect(authority(f.binding)).toBe(false);
 }finally{wall.mockRestore();mono.mockRestore();}
});
posix("a FIFO substituted between stat and open cannot hang the reader",()=>{
 const f=fixture();f.write();
 const script=`import {spyOn} from "bun:test"; import * as fs from "node:fs"; import {execFileSync} from "node:child_process"; import {createSavedAccountWitnessAuthority} from ${JSON.stringify(join(import.meta.dir,"../src/adapters/saved-account-witness.ts"))};
 const options=${JSON.stringify(f.options)},original=fs.openSync; let replaced=false;
 spyOn(fs,"openSync").mockImplementation((...args)=>{if(args[0]===options.witnessPath&&!replaced){replaced=true;fs.unlinkSync(options.witnessPath);execFileSync("mkfifo",[options.witnessPath]);}return original(...args);});
 try{createSavedAccountWitnessAuthority(options);process.exit(2);}catch{process.exit(replaced?0:3);}`;
 const child=spawnSync(process.execPath,["-e",script],{timeout:2000,stdio:"pipe"});
 expect(child.error).toBeUndefined();expect(child.status).toBe(0);
});
