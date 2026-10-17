import { expect, test } from "bun:test";
import { PassThrough } from "node:stream";
import { createHash, createHmac } from "node:crypto";
import { chmodSync, existsSync, mkdtempSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { readSavedAccountAcpBootstrap, runSavedAccountAcp } from "../src/adapters/saved-account-acp.ts";
import { prepareSavedAccountEnvelope } from "../src/adapters/saved-account-prepare.ts";
import { isJsonObject, type JsonValue } from "../src/shared/wire.ts";
const argv=["saved-account","acp","--broker-stdin"];
function canonical(v:JsonValue):string {if(Array.isArray(v))return "["+v.map(canonical).join(",")+"]";if(isJsonObject(v))return "{"+Object.keys(v).sort().map(k=>JSON.stringify(k)+":"+canonical(v[k]!)).join(",")+"}";return JSON.stringify(v);}
function fixture(){const base=mkdtempSync(join(tmpdir(),"saved-bootstrap-")),root=join(base,"root"),stateHome=join(base,"state"),witnessPath=join(base,"witness"),executable=join(base,"helper"),revisionKey="9".repeat(64);mkdirSync(root);mkdirSync(stateHome);const now=Date.now();
 const binding={fixtureOnly:true,accountId:"account-"+"a".repeat(32),accountRevision:1,credentialGeneration:1,provider:"openai-compatible" as const,endpoint:"http://127.0.0.1:49152/v1",model:"fixture",requestId:"fixture-request",attemptId:"fixture-attempt",epoch:1,leaseFence:1,contextSHA256:"b".repeat(64),consentId:"fixture-consent",consentRevision:1,expiresAtUnixMS:now+30000,limits:{maxInputBytes:16000,maxOutputTokens:256,maxRequests:2,maxSteps:2,maxSeconds:30}};
 const boot="executor-boot-"+"c".repeat(32),authoritySHA256="d".repeat(64),body={version:1,purpose:"saved_account_authority_v1",bindingSHA256:createHash("sha256").update(canonical(binding)).digest("hex"),serviceGeneration:boot,authoritySHA256,revision:1,state:"active",issuedAtUnixMS:now,validUntilUnixMS:now+5000};const key=createHmac("sha256",Buffer.from(revisionKey,"hex")).update("neko-saved-key-v1/authority-witness").digest();writeFileSync(witnessPath,canonical({...body,mac:createHmac("sha256",key).update(canonical(body)).digest("hex")}));
 const helper=`#!${process.execPath}\nimport{createInterface}from"node:readline";import{readFileSync,writeFileSync}from"node:fs";let path;for await(const line of createInterface({input:process.stdin})){const v=JSON.parse(line);if(!path){path=v.path;writeFileSync(path+".pid",String(process.pid));continue;}process.stdout.write(JSON.stringify({version:1,sequence:v.sequence,witness:readFileSync(path,"utf8")})+"\\n");}`;writeFileSync(executable,helper);chmodSync(executable,0o700);
 const nativeWitness={executable,executableSHA256:createHash("sha256").update(helper).digest("hex")},shared={binding,capsulePath:join(base,"capsule"),stateHome,revisionKey,witnessPath,serviceGeneration:boot,authoritySHA256,nativeWitness};
 const bootstrap={version:1 as const,purpose:"saved_account_acp_v1" as const,...shared,capsuleId:"sa1_"+"e".repeat(32),root};
 return {base,bootstrap,prepare:{version:1,purpose:"saved_account_task_v1",...shared,secret:"synthetic-bootstrap-secret"},pid:()=>Number(readFileSync(witnessPath+".pid","utf8")),close:()=>rmSync(base,{recursive:true,force:true})};
}
test("bootstrap consumes exactly one bounded line and preserves coalesced ACP bytes",async()=>{const f=fixture(),input=new PassThrough();try{const pending=readSavedAccountAcpBootstrap(input,argv),tail='{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1}}\n';input.write(canonical(f.bootstrap)+"\n"+tail);expect(await pending).toEqual(f.bootstrap);expect(input.read()?.toString()).toBe(tail);}finally{input.destroy();f.close();}});
test("bootstrap denies duplicate oversized malformed and mixed-mode envelopes",async()=>{const f=fixture();try{const raw=canonical(f.bootstrap);for(const bytes of [raw.replace("{",'{"version":1,'),"x".repeat(16385),"not-json",canonical({...f.bootstrap,secret:"never-forward-this"})]){const input=new PassThrough(),pending=readSavedAccountAcpBootstrap(input,argv);input.end(bytes+"\n");await expect(pending).rejects.toThrow("Saved account launch unavailable");}const input=new PassThrough();await expect(readSavedAccountAcpBootstrap(input,[...argv,"--profile","saved"])).rejects.toThrow("Saved account launch unavailable");input.destroy();}finally{f.close();}});
test("bootstrap without its first newline times out without requiring ACP EOF",async()=>{const input=new PassThrough();try{const pending=readSavedAccountAcpBootstrap(input,argv);input.write("{}");await expect(pending).rejects.toThrow("Saved account launch unavailable");}finally{input.destroy();}},4000);
test.skipIf(process.platform==="win32")("internal native prepare and startup share witness proof and cleanup without public bootstrap",async()=>{
 const f=fixture(),input=new PassThrough(),output=new PassThrough();let wire="";output.on("data",chunk=>wire+=chunk.toString());try{const receipt=await prepareSavedAccountEnvelope(f.prepare);expect(()=>process.kill(f.pid(),0)).toThrow();f.bootstrap.capsuleId=receipt.capsuleId;
 const running=runSavedAccountAcp(argv,{input,output});input.write(canonical(f.bootstrap)+'\n'+JSON.stringify({jsonrpc:"2.0",id:1,method:"initialize",params:{protocolVersion:1,clientCapabilities:{}}})+'\n');
 const until=Date.now()+2000;while(!wire.includes('"id":1')&&Date.now()<until)await Bun.sleep(10);expect(wire).toContain('"id":1');for(const value of [f.prepare.secret,f.prepare.revisionKey,f.bootstrap.capsulePath,"saved_account_acp_v1"])expect(wire).not.toContain(value);
 const pid=f.pid();rmSync(f.bootstrap.witnessPath);await expect(running).rejects.toThrow("Saved account launch unavailable");expect(()=>process.kill(pid,0)).toThrow();
 }finally{input.destroy();output.destroy();f.close();}
});

test("current CLI exposes neither saved-account ACP nor native preparation",()=>{
 const f=fixture();try{for(const [args,input] of [[argv,f.bootstrap],[["saved-account","prepare","--json","--broker-stdin"],f.prepare]] as const){const child=Bun.spawnSync([process.execPath,"bin/neko.ts",...args],{stdin:Buffer.from(canonical(input)+"\n"),stdout:"pipe",stderr:"pipe"});expect(child.exitCode).toBe(1);expect(child.stdout.toString()).toBe("");expect(child.stderr.toString()).toBe("Saved account preparation unavailable\n");expect(existsSync(f.bootstrap.witnessPath+".pid")).toBe(false);}}
 finally{f.close();}
});
