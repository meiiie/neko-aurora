import { afterEach, expect, test } from "bun:test";
import { createHash, createHmac } from "node:crypto";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { isJsonObject, type JsonValue } from "../src/shared/wire.ts";
const roots:string[]=[];
afterEach(()=>{for(const root of roots.splice(0))rmSync(root,{recursive:true,force:true});});
function canonical(v:JsonValue):string {if(Array.isArray(v))return "["+v.map(canonical).join(",")+"]";if(isJsonObject(v))return "{"+Object.keys(v).sort().map(k=>JSON.stringify(k)+":"+canonical(v[k]!)).join(",")+"}";return JSON.stringify(v);}
function fixture(endpoint="http://127.0.0.1:49152/v1") {
 const root=mkdtempSync(join(tmpdir(),"saved-cli-"));roots.push(root);const stateHome=join(root,"state"),home=join(root,"home");mkdirSync(stateHome);mkdirSync(home);
 mkdirSync(join(home,".neko-core"));writeFileSync(join(home,".neko-core/config.json"),"not-json-HOME-SECRET");
 const now=Date.now(),revisionKey="9".repeat(64),secret="synthetic-cli-private-material";
 const binding={fixtureOnly:true,accountId:"account-"+"a".repeat(32),accountRevision:1,credentialGeneration:1,provider:"openai-compatible",endpoint,model:"fixture-model",requestId:"fixture-request",attemptId:"fixture-attempt",epoch:1,leaseFence:1,contextSHA256:"b".repeat(64),consentId:"fixture-consent",consentRevision:1,expiresAtUnixMS:now+30000,limits:{maxInputBytes:16000,maxOutputTokens:256,maxRequests:2,maxSteps:2,maxSeconds:30}};
 const input={version:1,purpose:"saved_account_task_v1",binding,secret,capsulePath:join(root,"capsule"),stateHome,revisionKey,witnessPath:join(root,"witness"),serviceGeneration:"executor-boot-"+"c".repeat(32),authoritySHA256:"d".repeat(64)};
 const body={version:1,purpose:"saved_account_authority_v1",bindingSHA256:createHash("sha256").update(canonical(binding)).digest("hex"),serviceGeneration:input.serviceGeneration,authoritySHA256:input.authoritySHA256,revision:1,state:"active",issuedAtUnixMS:now,validUntilUnixMS:now+5000};
 const key=createHmac("sha256",Buffer.from(revisionKey,"hex")).update("neko-saved-key-v1/authority-witness").digest();writeFileSync(input.witnessPath,canonical({...body,mac:createHmac("sha256",key).update(canonical(body)).digest("hex")}));
 const run=(data=canonical(input),extra:string[]=[],override:NodeJS.ProcessEnv={})=>{
  const env:NodeJS.ProcessEnv={...process.env,HOME:home,USERPROFILE:home,NEKO_HOME:home};for(const name of Object.keys(env))if(name.startsWith("NEKO_PROFILE")||name==="NEKO_CREDENTIAL_HOME")delete env[name];
  const child=Bun.spawnSync([process.execPath,"bin/neko.ts","saved-account","prepare","--json","--broker-stdin",...extra],{env:{...env,...override},stdin:Buffer.from(data),stdout:"pipe",stderr:"pipe",timeout:5000});
  const stdout=child.stdout.toString(),stderr=child.stderr.toString();for(const value of [secret,revisionKey,input.capsulePath,input.witnessPath,"HOME-SECRET"])expect(stdout+stderr).not.toContain(value);
  expect(readdirSync(stateHome)).toEqual([]);expect(readFileSync(join(home,".neko-core/config.json"),"utf8")).toBe("not-json-HOME-SECRET");return {code:child.exitCode,stdout,stderr};
 };
 return {input,run};
}
const posix=test.skipIf(process.platform==="win32");
posix("private CLI prepares only a public receipt without home discovery or execution",async()=>{
 let calls=0;const server=Bun.serve({hostname:"127.0.0.1",port:0,fetch:()=>{calls++;return new Response("unexpected",{status:500});}});
 try {const f=fixture(`http://127.0.0.1:${server.port}/v1`),r=f.run();expect(r.code).toBe(0);expect(r.stderr).toBe("");const receipt=JSON.parse(r.stdout);expect(Object.keys(receipt).sort()).toEqual(["bindingSHA256","capsuleId","expiresAtUnixMS","fixtureOnly","version"]);expect(receipt.capsuleId).toMatch(/^sa1_[a-f0-9]{32}$/);expect(r.stdout.length).toBeLessThan(512);expect(existsSync(f.input.capsulePath)).toBe(true);expect(existsSync(f.input.capsulePath+".current")).toBe(true);expect(readFileSync(f.input.capsulePath,"utf8")).not.toContain(f.input.secret);await Bun.sleep(20);expect(calls).toBe(0);
 }finally{await server.stop(true);}
});
function denied(f:ReturnType<typeof fixture>,data?:string,flags?:string[],env?:NodeJS.ProcessEnv) {const r=f.run(data,flags,env);expect(r.code).toBe(1);expect(r.stdout).toBe("");expect(r.stderr).toBe("Saved account preparation unavailable\n");expect(existsSync(f.input.capsulePath)).toBe(false);}
test("private CLI rejects malformed oversized duplicate and unknown-field input",()=>{for(const kind of ["malformed","oversize","duplicate","extra"]){const f=fixture(),raw=canonical(f.input);denied(f,kind==="malformed"?"{synthetic-cli-private-material":kind==="oversize"?" ".repeat(16385):kind==="duplicate"?raw.replace("{",'{"version":1,'):canonical({...f.input,extra:true}));}});
posix("private CLI requires the current witness and never falls back to home",()=>{for(const kind of ["missing","tampered","wrong-key"]){const f=fixture();if(kind==="missing")rmSync(f.input.witnessPath);if(kind==="tampered")writeFileSync(f.input.witnessPath,"{}");if(kind==="wrong-key")f.input.revisionKey="8".repeat(64);denied(f);}});
test("private CLI rejects every extra flag and profile or credential-home environment",()=>{
 for(const flags of [["--profile","fixture"],["--credential-home","/private"],["--profile-v2"],["--host-profile","nekobot"],["--help"],["--json"],["--secret","synthetic-cli-private-material"]])denied(fixture(),undefined,flags);
 for(const env of [{NEKO_PROFILE:"fixture"},{NEKO_PROFILE_CAPSULE_PATH:"/private"},{NEKO_CREDENTIAL_HOME:"/private"},{NEKO_PROFILE:""}])denied(fixture(),undefined,[],env);
});
test.skipIf(process.platform!=="win32")("private preparation remains unavailable on Windows",()=>denied(fixture()));

test("private input without EOF terminates within its input deadline",async()=>{
 const f=fixture(),env={...process.env};for(const name of Object.keys(env))if(name.startsWith("NEKO_PROFILE")||name==="NEKO_CREDENTIAL_HOME")delete env[name];
 const child=Bun.spawn([process.execPath,"bin/neko.ts","saved-account","prepare","--json","--broker-stdin"],{env,stdin:"pipe",stdout:"pipe",stderr:"pipe"});
 child.stdin.write(canonical(f.input));child.stdin.flush();
 const code=await child.exited;expect(code).toBe(1);expect(await new Response(child.stdout).text()).toBe("");expect(await new Response(child.stderr).text()).toBe("Saved account preparation unavailable\n");expect(existsSync(f.input.capsulePath)).toBe(false);
},5000);

test("prefix flags cannot route private preparation through help or ordinary errors",()=>{
 for(const prefix of [["--help"],["--profile","fixture"],["--credential-home","--help"],["--json"]])for(const tail of [["--json","--broker-stdin"],[]]){
  const r=Bun.spawnSync([process.execPath,"bin/neko.ts",...prefix,"saved-account","prepare",...tail],{stdin:Buffer.from("synthetic-cli-private-material"),stdout:"pipe",stderr:"pipe"});
  expect(r.exitCode).toBe(1);expect(r.stdout.toString()).toBe("");expect(r.stderr.toString()).toBe("Saved account preparation unavailable\n");
 }
});
test("ordinary prompts and option values named saved-account retain ordinary help",()=>{
 for(const argv of [["run","saved-account","--help"],["--profile","saved-account","--help"],["--prompt","saved-account","--help"],["--resume","saved-account","--help"]]){
  const r=Bun.spawnSync([process.execPath,"bin/neko.ts",...argv],{stdin:Buffer.from(""),stdout:"pipe",stderr:"pipe"});expect(r.exitCode).toBe(0);expect(r.stdout.toString()).toContain("Usage:");expect(r.stderr.toString()).toBe("");
 }
});
