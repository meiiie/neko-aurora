import { expect, test } from "bun:test";
import { mkdirSync, mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { loadSavedAccountBridge, prepareSavedAccountCapsule, type SavedAccountBinding } from "../src/adapters/saved-account-bridge.ts";
import { getProvider } from "../src/adapters/providers.ts";
import { redactCredentialJSON } from "../src/shared/credential-output.ts";

const FIXTURE_SECRET="fixture-saved-account-private-material", KEY="6".repeat(64);
function fixture(endpoint="https://example.invalid/v1") {
 const root=mkdtempSync(join(tmpdir(),"saved-key-fixture-")),stateHome=join(root,"task"),privateRoot=join(root,"private");mkdirSync(stateHome);mkdirSync(privateRoot);
 const binding:SavedAccountBinding={fixtureOnly:false,accountId:"account-"+"a".repeat(32),accountRevision:1,credentialGeneration:1,provider:"openai-compatible",endpoint,model:"fixture-model",requestId:"fixture-request",attemptId:"fixture-attempt",epoch:1,leaseFence:1,contextSHA256:"b".repeat(64),consentId:"fixture-explicit-consent",consentRevision:1,expiresAtUnixMS:Date.now()+30000,limits:{maxInputBytes:16000,maxOutputTokens:256,maxRequests:2,maxSteps:2,maxSeconds:30}};
 const capsulePath=join(privateRoot,"saved-account-capsule.json");let current=true;const currentAuthority=()=>current;
 const input={version:1,purpose:"saved_account_task_v1",binding,secret:FIXTURE_SECRET,capsulePath,stateHome,revisionKey:KEY};
 return {binding,capsulePath,stateHome,currentAuthority,input,revoke:()=>current=false};
}
function load(f:ReturnType<typeof fixture>) {const r=prepareSavedAccountCapsule(f.input,f.currentAuthority);return {r,cfg:loadSavedAccountBridge({binding:f.binding,capsuleId:r.capsuleId,capsulePath:f.capsulePath,stateHome:f.stateHome,revisionKey:KEY,currentAuthority:f.currentAuthority})};}

test("saved account capsule is encrypted purpose bound and never grants execution",()=>{
 const f=fixture();const {r,cfg}=load(f);expect(r.version).toBe(1);expect(r.capsuleId).toMatch(/^sa1_[a-f0-9]{32}$/);expect(Object.keys(r).sort()).toEqual(["version","capsuleId","bindingSHA256","expiresAtUnixMS","fixtureOnly"].sort());
 const bytes=readFileSync(f.capsulePath,"utf8")+readFileSync(f.capsulePath+".current","utf8")+JSON.stringify(r)+JSON.stringify(cfg.data);
 for(const secret of [FIXTURE_SECRET,KEY,f.stateHome,f.capsulePath])expect(bytes.includes(secret)).toBe(false);
 expect(cfg.apiKey===FIXTURE_SECRET).toBe(true);expect(cfg.projectTrust).toEqual({state:"none",files:[]});expect(cfg.data.max_retries).toBe(0);expect(cfg.data.offline_retry_seconds).toBe(0);
 expect(redactCredentialJSON({reply:FIXTURE_SECRET},cfg.credentialSecrets)).toEqual({reply:"[redacted credential]"});
});
test("missing authority denies both preparation and loader",()=>{
 const f=fixture();expect(()=>prepareSavedAccountCapsule(f.input)).toThrow("Saved account bridge unavailable");const r=prepareSavedAccountCapsule(f.input,f.currentAuthority);
 expect(()=>loadSavedAccountBridge({binding:f.binding,capsuleId:r.capsuleId,capsulePath:f.capsulePath,stateHome:f.stateHome,revisionKey:KEY})).toThrow("Saved account bridge unavailable");
});
test("rotation and original context attempt consent generation cannot rebind a capsule",()=>{
 for(const field of ["credentialGeneration","accountRevision","contextSHA256","attemptId","epoch","leaseFence","consentRevision","model","endpoint"] as const){const f=fixture();const {r}=load(f);const changed={...f.binding,[field]:typeof f.binding[field]==="number"?Number(f.binding[field])+1:"different-original-binding"};expect(()=>loadSavedAccountBridge({binding:changed,capsuleId:r.capsuleId,capsulePath:f.capsulePath,stateHome:f.stateHome,revisionKey:KEY,currentAuthority:f.currentAuthority})).toThrow("Saved account bridge unavailable");}
});
test("revoke expiry marker swap and cipher tamper close current config",()=>{
 const f=fixture();const {cfg}=load(f);f.revoke();expect(()=>cfg.profileBridge!.assertCurrent(cfg)).toThrow("Saved account bridge unavailable");
 const x=fixture();const pair=load(x);const sealed=JSON.parse(readFileSync(x.capsulePath,"utf8"));sealed.ciphertext=sealed.ciphertext.slice(0,-1)+(sealed.ciphertext.endsWith("a")?"b":"a");writeFileSync(x.capsulePath,JSON.stringify(sealed));expect(()=>pair.cfg.profileBridge!.assertCurrent(pair.cfg)).toThrow("Saved account bridge unavailable");
 const e=fixture();e.binding.expiresAtUnixMS=Date.now()-1;expect(()=>prepareSavedAccountCapsule(e.input,e.currentAuthority)).toThrow("Saved account bridge unavailable");
 const a=fixture(),b=fixture();const aa=load(a);load(b);writeFileSync(a.capsulePath+".current",readFileSync(b.capsulePath+".current"));expect(()=>aa.cfg.profileBridge!.assertCurrent(aa.cfg)).toThrow("Saved account bridge unavailable");
});
test("closed schema no global account fallback and credential header controls",()=>{
 for(const extra of [{extra:true},{secret:"fixture\r\nheader-injection"},{purpose:"configured_env_only"},{revisionKey:"wrong"}]){const f=fixture();expect(()=>prepareSavedAccountCapsule({...f.input,...extra},f.currentAuthority)).toThrow("Saved account bridge unavailable");}
 const f=fixture();const {cfg}=load(f);cfg.data.model="another-model";expect(()=>cfg.profileBridge!.assertCurrent(cfg)).toThrow("Saved account bridge unavailable");
});
test("Core canonical provider uses saved key and 401 403 503 do not automatically replay",async()=>{
 const original=globalThis.fetch;
 try{for(const status of [200,401,403,503]){const f=fixture();const {cfg}=load(f);let calls=0;globalThis.fetch=(async(url,init)=>{calls++;expect(String(url)).toBe("https://example.invalid/v1/chat/completions");expect(new Headers(init?.headers).get("Authorization")).toBe("Bearer "+FIXTURE_SECRET);expect(init?.redirect).toBe("manual");if(status===200)return new Response("data: "+JSON.stringify({choices:[{delta:{content:"fixture saved account result"},finish_reason:null}]})+"\n\ndata: "+JSON.stringify({choices:[{delta:{},finish_reason:"stop"}],usage:{completion_tokens:4}})+"\n\ndata: [DONE]\n\n",{headers:{"Content-Type":"text/event-stream"}});return new Response("fixture response may contain "+FIXTURE_SECRET,{status});}) as typeof fetch;
 const provider=getProvider(cfg);if(status===200)expect((await provider.complete([{role:"user",content:"authored fixture request"}])).content).toBe("fixture saved account result");else{let message="";try{await provider.complete([{role:"user",content:"authored fixture request"}]);}catch(e){message=String(e);}expect(message.includes(FIXTURE_SECRET)).toBe(false);expect(message).toContain(status===401?"unauthorized":status===403?"forbidden":"transient");}expect(calls).toBe(1);
 }}finally{globalThis.fetch=original;}
});

test("fixture authority binds loopback bit and actual Core adapter consumes only authored local key",async()=>{
 let calls=0;const server=Bun.serve({hostname:"127.0.0.1",port:0,fetch:request=>{calls++;expect(request.headers.get("Authorization")).toBe("Bearer "+FIXTURE_SECRET);return new Response("data: "+JSON.stringify({choices:[{delta:{content:"local saved-key Core result"},finish_reason:null}]})+"\n\ndata: "+JSON.stringify({choices:[{delta:{},finish_reason:"stop"}],usage:{completion_tokens:4}})+"\n\ndata: [DONE]\n\n",{headers:{"Content-Type":"text/event-stream"}});}});
 try{const f=fixture(`http://127.0.0.1:${server.port}/v1`);expect(()=>prepareSavedAccountCapsule(f.input,f.currentAuthority)).toThrow("Saved account bridge unavailable");f.binding.fixtureOnly=true;const {r,cfg}=load(f);expect(r.fixtureOnly).toBe(true);expect((await getProvider(cfg).complete([{role:"user",content:"authored private loopback request"}])).content).toBe("local saved-key Core result");expect(calls).toBe(1);const expected={...f.binding,fixtureOnly:false};expect(()=>loadSavedAccountBridge({binding:expected,capsuleId:r.capsuleId,capsulePath:f.capsulePath,stateHome:f.stateHome,revisionKey:KEY,currentAuthority:f.currentAuthority})).toThrow("Saved account bridge unavailable");}finally{await server.stop(true);}
});
test("live current-authority revocation aborts a silent saved-key provider",async()=>{
 let enter!:()=>void,release!:(response:Response)=>void;const entered=new Promise<void>(r=>enter=r),held=new Promise<Response>(r=>release=r);const server=Bun.serve({hostname:"127.0.0.1",port:0,fetch:async()=>{enter();return await held;}});
 try{const f=fixture(`http://127.0.0.1:${server.port}/v1`);f.binding.fixtureOnly=true;const {cfg}=load(f);const pending=getProvider(cfg).complete([{role:"user",content:"authored hung request"}]).then(()=>"unsafe success",()=>"stopped");await entered;f.revoke();expect(await Promise.race([pending,new Promise<string>(r=>setTimeout(()=>r("not stopped"),750))])).toBe("stopped");}finally{release(new Response("fixture closed",{status:503}));await server.stop(true);}
});

test("saved-key config disables updates and outside-root access and guards mutation",()=>{
 const f=fixture();const {cfg}=load(f);for(const key of ["auto_update","auto_update_check","read_outside_root"]){expect(cfg.data[key]).toBe(false);cfg.data[key]=true;expect(()=>cfg.profileBridge!.assertCurrent(cfg)).toThrow("Saved account bridge unavailable");cfg.data[key]=false;}
});
