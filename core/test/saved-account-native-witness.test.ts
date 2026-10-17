import { afterEach, expect, test } from "bun:test";
import { createHash, createHmac } from "node:crypto";
import { chmodSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { openNativeSavedAccountWitness } from "../src/adapters/saved-account-native-witness.ts";
import { isJsonObject, type JsonValue } from "../src/shared/wire.ts";
import type { SavedAccountBinding } from "../src/adapters/saved-account-bridge.ts";
const roots:string[]=[];
afterEach(()=>{for(const root of roots.splice(0))rmSync(root,{recursive:true,force:true});});
function canonical(v:JsonValue):string {if(Array.isArray(v))return "["+v.map(canonical).join(",")+"]";if(isJsonObject(v))return "{"+Object.keys(v).sort().map(k=>JSON.stringify(k)+":"+canonical(v[k]!)).join(",")+"}";return JSON.stringify(v);}
function fixture() {
 const root=mkdtempSync(join(tmpdir(),"native-witness-"));roots.push(root);const witnessPath=join(root,"witness"),executable=join(root,"helper"),revisionKey="9".repeat(64),now=Date.now();
 const binding:SavedAccountBinding={fixtureOnly:true,accountId:"account-"+"a".repeat(32),accountRevision:1,credentialGeneration:1,provider:"openai-compatible",endpoint:"http://127.0.0.1:49152/v1",model:"fixture",requestId:"fixture-request",attemptId:"fixture-attempt",epoch:1,leaseFence:1,contextSHA256:"b".repeat(64),consentId:"fixture-consent",consentRevision:1,expiresAtUnixMS:now+30000,limits:{maxInputBytes:16000,maxOutputTokens:256,maxRequests:2,maxSteps:2,maxSeconds:30}};
 const pins={binding,witnessPath,serviceGeneration:"executor-boot-"+"c".repeat(32),authoritySHA256:"d".repeat(64),revisionKey};
 const write=(revision=1,state="active")=>{const issuedAtUnixMS=Date.now(),body={version:1,purpose:"saved_account_authority_v1",bindingSHA256:createHash("sha256").update(canonical(binding)).digest("hex"),serviceGeneration:pins.serviceGeneration,authoritySHA256:pins.authoritySHA256,revision,state,issuedAtUnixMS,validUntilUnixMS:issuedAtUnixMS+5000};const key=createHmac("sha256",Buffer.from(revisionKey,"hex")).update("neko-saved-key-v1/authority-witness").digest();writeFileSync(witnessPath,canonical({...body,mac:createHmac("sha256",key).update(canonical(body)).digest("hex")}));};write();writeFileSync(witnessPath+".mode","ok");
 const script=`#!${process.execPath}\nimport {createInterface} from "node:readline";import {readFileSync,writeFileSync} from "node:fs";
 let path,sequence=0;for await(const line of createInterface({input:process.stdin})){const frame=JSON.parse(line);if(!path){if(process.argv[2]!=="--saved-witness-reader"||Object.keys(frame).sort().join()!=="path,version")process.exit(3);path=frame.path;writeFileSync(path+".pid",String(process.pid));continue;}if(frame.version!==1||frame.sequence!==++sequence)process.exit(4);const mode=readFileSync(path+".mode","utf8");if(mode==="silent")continue;if(mode==="exit")process.exit(1);if(mode==="late")await new Promise(r=>setTimeout(r,150));if(mode==="oversize"){process.stdout.write("x".repeat(24577));continue;}let witness=readFileSync(path,"utf8");if(mode==="large-witness")witness="x".repeat(4097);const reply=JSON.stringify({version:1,sequence:mode==="wrong-sequence"?sequence+1:sequence,witness});process.stdout.write(mode==="malformed"?"oops\\n":reply+"\\n"+(mode==="double"?reply+"\\n":""));}
 `;writeFileSync(executable,script);chmodSync(executable,0o700);
 return {pins,write,options:{...pins,executable,executableSHA256:createHash("sha256").update(script).digest("hex")},mode:(m:string)=>writeFileSync(witnessPath+".mode",m),pid:()=>Number(readFileSync(witnessPath+".pid","utf8"))};
}
const posix=test.skipIf(process.platform==="win32");
posix("native session awaits verified proof, renews, and close poisons authority and reaps helper",async()=>{
 const f=fixture(),session=await openNativeSavedAccountWitness(f.options);const pid=f.pid();
 try{expect(session.authority(f.pins.binding)).toBe(true);f.write(3);await Bun.sleep(70);expect(session.authority(f.pins.binding)).toBe(true);}finally{await session.close();}
 expect(session.authority(f.pins.binding)).toBe(false);expect(()=>process.kill(pid,0)).toThrow();await session.close();
});
posix("native reader requires matching executable checksum and signed authority",async()=>{
 const f=fixture();await expect(openNativeSavedAccountWitness({...f.options,executableSHA256:"0".repeat(64)})).rejects.toThrow("Saved account authority unavailable");
 await expect(openNativeSavedAccountWitness({...f.options,revisionKey:"8".repeat(64)})).rejects.toThrow("Saved account authority unavailable");expect(()=>process.kill(f.pid(),0)).toThrow();
});
posix("invalid frames EOF oversized witness and late responses fail closed without helper leaks",async()=>{
 for(const mode of ["malformed","wrong-sequence","double","oversize","large-witness","exit","silent","late"]){const f=fixture();f.mode(mode);await expect(openNativeSavedAccountWitness(f.options)).rejects.toThrow("Saved account authority unavailable");expect(()=>process.kill(f.pid(),0)).toThrow();}
});
posix("retirement and missed observations permanently poison an established native session",async()=>{
 for(const kind of ["retired","silent","exit"]){const f=fixture(),session=await openNativeSavedAccountWitness(f.options);try{if(kind==="retired")f.write(2,"retired");else f.mode(kind);await Bun.sleep(170);expect(session.authority(f.pins.binding)).toBe(false);f.mode("ok");f.write(3);await Bun.sleep(60);expect(session.authority(f.pins.binding)).toBe(false);}finally{await session.close();}expect(()=>process.kill(f.pid(),0)).toThrow();}
});
posix("event-loop suspension cannot serve a stale cached observation",async()=>{
 const f=fixture(),session=await openNativeSavedAccountWitness(f.options);try{const end=performance.now()+110;while(performance.now()<end){/* actual paused reader */}expect(session.authority(f.pins.binding)).toBe(false);}finally{await session.close();}
});
