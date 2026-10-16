import { createCipheriv, createDecipheriv, createHash, createHmac, randomBytes, timingSafeEqual } from "node:crypto";
import { closeSync, existsSync, fstatSync, fsyncSync, linkSync, lstatSync, openSync, readFileSync, realpathSync, unlinkSync, writeFileSync } from "node:fs";
import { dirname, isAbsolute, join, relative, resolve, sep } from "node:path";
import { DEFAULTS, NekoConfig, type Profile } from "./config.ts";
import { ProfileBridgeBudget, type ProfileBridgeLimits } from "./profile-bridge-budget.ts";
import { isJsonNumber, isJsonObject, isText, type JsonObject, type JsonValue } from "../shared/wire.ts";
import { containsCredentialJSON, CredentialOutputFilter, redactCredentialJSON } from "../shared/credential-output.ts";
import type { Provider, ProviderResponse } from "../core/ports.ts";

export interface SavedAccountBinding {
 fixtureOnly:boolean; accountId:string; accountRevision:number; credentialGeneration:number; provider:"openai-compatible"|"anthropic"; endpoint:string; model:string;
 requestId:string; attemptId:string; epoch:number; leaseFence:number; contextSHA256:string; consentId:string; consentRevision:number; expiresAtUnixMS:number; limits:ProfileBridgeLimits;
}
export interface SavedAccountReceipt {version:1;capsuleId:string;bindingSHA256:string;expiresAtUnixMS:number;fixtureOnly:boolean}
export type SavedAccountAuthority=(binding:Readonly<SavedAccountBinding>)=>boolean;
interface LoadOptions {binding:SavedAccountBinding;capsuleId:string;capsulePath:string;stateHome:string;revisionKey:string;currentAuthority?:SavedAccountAuthority}
const PURPOSE="saved_account_task_v1",HEX=/^[a-f0-9]{64}$/,ID=/^[A-Za-z0-9_-]{3,128}$/,CAPSULE=/^sa1_[a-f0-9]{32}$/;
const bindingKeys=["fixtureOnly","accountId","accountRevision","credentialGeneration","provider","endpoint","model","requestId","attemptId","epoch","leaseFence","contextSHA256","consentId","consentRevision","expiresAtUnixMS","limits"];
function fail():never{throw new Error("Saved account bridge unavailable");}
function exact(v:unknown,keys:string[]):v is JsonObject{return isJsonObject(v)&&Object.keys(v).sort().join()===keys.slice().sort().join();}
function canonical(value:unknown):string{if(Array.isArray(value))return "["+value.map(canonical).join(",")+"]";if(value!==null&&typeof value==="object"){const o=value as Record<string,unknown>;return "{"+Object.keys(o).sort().map(k=>JSON.stringify(k)+":"+canonical(o[k])).join(",")+"}";}return JSON.stringify(value);}
function text(v:JsonValue|undefined,bound:number):string{if(!isText(v)||!v||Buffer.byteLength(v)>bound||/[\u0000-\u001f\u007f\ufffd]/.test(v))fail();return v;}
function number(v:JsonValue|undefined,max=Number.MAX_SAFE_INTEGER):number{if(!isJsonNumber(v)||!Number.isSafeInteger(v)||v<1||v>max)fail();return v;}
export function parseSavedAccountBinding(value:unknown):SavedAccountBinding{
 if(!exact(value,bindingKeys))fail();const raw=value.limits,keys=["maxInputBytes","maxOutputTokens","maxRequests","maxSteps","maxSeconds"];if(!raw||!exact(raw,keys))fail();
 const limits={maxInputBytes:number(raw.maxInputBytes,64000),maxOutputTokens:number(raw.maxOutputTokens,4096),maxRequests:number(raw.maxRequests,8),maxSteps:number(raw.maxSteps,6),maxSeconds:number(raw.maxSeconds,600)};
 const accountId=text(value.accountId,40),provider=text(value.provider,32),endpoint=text(value.endpoint,512),model=text(value.model,120),requestId=text(value.requestId,128),attemptId=text(value.attemptId,128),consentId=text(value.consentId,128),contextSHA256=text(value.contextSHA256,64);
 let url:URL;try{url=new URL(endpoint);}catch{return fail();}
 if(!/^account-[a-f0-9]{32}$/.test(accountId)||!["openai-compatible","anthropic"].includes(provider)||(typeof value.fixtureOnly!=="boolean" || (value.fixtureOnly ? url.protocol!=="http:"||url.hostname!=="127.0.0.1"||!url.port||Number(url.port)<1 : url.protocol!=="https:"))||!url.hostname||url.username||url.password||url.search||url.hash||url.href!==endpoint||![requestId,attemptId,consentId].every(v=>ID.test(v))||!HEX.test(contextSHA256))fail();
 return Object.freeze({fixtureOnly:value.fixtureOnly as boolean,accountId,accountRevision:number(value.accountRevision),credentialGeneration:number(value.credentialGeneration),provider:provider as SavedAccountBinding["provider"],endpoint,model,requestId,attemptId,epoch:number(value.epoch),leaseFence:number(value.leaseFence),contextSHA256,consentId,consentRevision:number(value.consentRevision),expiresAtUnixMS:number(value.expiresAtUnixMS),limits:Object.freeze(limits)});
}
function pinHash(binding:SavedAccountBinding):string{return createHash("sha256").update(canonical(binding)).digest("hex");}
function current(binding:SavedAccountBinding,authority?:SavedAccountAuthority):void{try{if(!authority||authority(binding)!==true||Date.now()>=binding.expiresAtUnixMS)fail();}catch{return fail();}}
function keyOf(raw:string,purpose:string):Buffer{if(!HEX.test(raw))fail();return createHmac("sha256",Buffer.from(raw,"hex")).update("neko-saved-key-v1/"+purpose).digest();}
function root(path:string):string{
 if(!isAbsolute(path)||/[\x00-\x1f]/.test(path))fail();let cursor=resolve(path);for(;;){if(existsSync(cursor)&&lstatSync(cursor).isSymbolicLink())fail();if(dirname(cursor)===cursor)break;cursor=dirname(cursor);}if(!existsSync(path)||!lstatSync(path).isDirectory())fail();return realpathSync.native(path);
}
function safePath(path:string,state:string):string{
 if(!isAbsolute(path)||/[\x00-\x1f]/.test(path))fail();const parent=root(dirname(path)),target=join(parent,resolve(path).slice(resolve(path).lastIndexOf(sep)+1));const rel=relative(state,target);if(rel===""||rel!==".."&&!rel.startsWith(".."+sep)&&!isAbsolute(rel))fail();return target;
}
function bytesAt(path:string):Buffer{
 const before=lstatSync(path);if(!before.isFile()||before.isSymbolicLink()||before.nlink!==1||before.size>65536)fail();const fd=openSync(path,"r");try{const stat=fstatSync(fd);if(!stat.isFile()||stat.nlink!==1||stat.ino!==before.ino||stat.dev!==before.dev||stat.size>65536)fail();const bytes=readFileSync(fd),after=fstatSync(fd),named=lstatSync(path);if(bytes.length>65536||stat.size!==after.size||stat.mtimeMs!==after.mtimeMs||stat.ino!==named.ino||stat.dev!==named.dev||named.isSymbolicLink())fail();return bytes;}finally{closeSync(fd);}
}
function jsonAt(path:string):JsonValue{const bytes=bytesAt(path);try{const data=JSON.parse(bytes.toString("utf8")) as JsonValue;if(bytes.toString("utf8").trim()!==canonical(data))fail();return data;}finally{bytes.fill(0);}}
function createAtomic(path:string,value:JsonValue):void{
 if(existsSync(path))fail();const temporary=path+".tmp-"+randomBytes(16).toString("hex"),fd=openSync(temporary,"wx",0o600);try{writeFileSync(fd,canonical(value)+"\n");fsyncSync(fd);}finally{closeSync(fd);}try{linkSync(temporary,path);}finally{unlinkSync(temporary);}
}
function b64(v:JsonValue|undefined,length?:number):Buffer{if(!isText(v)||!/^[A-Za-z0-9_-]+$/.test(v))fail();const b=Buffer.from(v,"base64url");if(b.toString("base64url")!==v||length!==undefined&&b.length!==length)fail();return b;}
function secret(v:JsonValue|undefined):string{if(!isText(v)||v.length<8||Buffer.byteLength(v)>4096||/[^\x21-\x7e]/.test(v))fail();return v;}
export function prepareSavedAccountCapsule(input:unknown,currentAuthority?:SavedAccountAuthority):SavedAccountReceipt{
 try{
  if(!exact(input,["version","purpose","binding","secret","capsulePath","stateHome","revisionKey"])||input.version!==1||input.purpose!==PURPOSE||!input.binding)fail();const binding=parseSavedAccountBinding(input.binding),material=secret(input.secret),state=root(text(input.stateHome,4096)),path=safePath(text(input.capsulePath,4096),state),rawKey=text(input.revisionKey,64);
  current(binding,currentAuthority);const now=Date.now();if(binding.expiresAtUnixMS-now>binding.limits.maxSeconds*1000||canonical(binding).includes(material))fail();
  const capsuleId="sa1_"+randomBytes(16).toString("hex"),bindingSHA256=pinHash(binding),header:JsonObject={version:1,purpose:PURPOSE,capsuleId,bindingSHA256,createdAtUnixMS:now,expiresAtUnixMS:binding.expiresAtUnixMS,fixtureOnly:binding.fixtureOnly};
  const plain=Buffer.from(canonical({version:1,secret:material})),nonce=randomBytes(12),key=keyOf(rawKey,"encryption"),cipher=createCipheriv("aes-256-gcm",key,nonce);cipher.setAAD(Buffer.from(canonical(header)));let encrypted:Buffer;try{encrypted=Buffer.concat([cipher.update(plain),cipher.final()]);}finally{plain.fill(0);key.fill(0);}
  const sealed:JsonObject={...header,nonce:nonce.toString("base64url"),tag:cipher.getAuthTag().toString("base64url"),ciphertext:encrypted.toString("base64url")};const marker:JsonObject={...header,sealedSHA256:createHash("sha256").update(canonical(sealed)).digest("hex")};const markerKey=keyOf(rawKey,"marker");marker.mac=createHmac("sha256",markerKey).update(canonical(marker)).digest("hex");markerKey.fill(0);current(binding,currentAuthority);createAtomic(path,sealed);createAtomic(path+".current",marker);current(binding,currentAuthority);return {version:1,capsuleId,bindingSHA256,expiresAtUnixMS:binding.expiresAtUnixMS,fixtureOnly:binding.fixtureOnly};
 }catch{return fail();}
}
function materialAt(options:LoadOptions,binding:SavedAccountBinding):string{
 try{
  const state=root(options.stateHome),path=safePath(options.capsulePath,state),sealed=jsonAt(path),marker=jsonAt(path+".current"),common=["version","purpose","capsuleId","bindingSHA256","createdAtUnixMS","expiresAtUnixMS","fixtureOnly"];
  if(!exact(sealed,[...common,"nonce","tag","ciphertext"])||!exact(marker,[...common,"sealedSHA256","mac"])||sealed.version!==1||sealed.purpose!==PURPOSE||sealed.capsuleId!==options.capsuleId||!CAPSULE.test(options.capsuleId)||sealed.bindingSHA256!==pinHash(binding)||common.some(k=>sealed[k]!==marker[k])||sealed.expiresAtUnixMS!==binding.expiresAtUnixMS)fail();
  const created=number(sealed.createdAtUnixMS),expiry=number(sealed.expiresAtUnixMS);if(Date.now()<created||Date.now()>=expiry||expiry-created>binding.limits.maxSeconds*1000||marker.sealedSHA256!==createHash("sha256").update(canonical(sealed)).digest("hex"))fail();
  const {mac,...body}=marker;if(!isText(mac)||!HEX.test(mac))fail();const mk=keyOf(options.revisionKey,"marker"),expected=createHmac("sha256",mk).update(canonical(body)).digest();mk.fill(0);if(!timingSafeEqual(Buffer.from(mac,"hex"),expected))fail();
  const header:JsonObject={};for(const k of common)header[k]=sealed[k]!;const key=keyOf(options.revisionKey,"encryption"),decipher=createDecipheriv("aes-256-gcm",key,b64(sealed.nonce,12));decipher.setAAD(Buffer.from(canonical(header)));decipher.setAuthTag(b64(sealed.tag,16));let plain:Buffer;try{plain=Buffer.concat([decipher.update(b64(sealed.ciphertext)),decipher.final()]);}finally{key.fill(0);}try{if(plain.length>8192)fail();const data=JSON.parse(plain.toString("utf8")) as JsonValue;if(!exact(data,["version","secret"])||data.version!==1)fail();return secret(data.secret);}finally{plain.fill(0);}
 }catch{return fail();}
}
export function loadSavedAccountBridge(options:LoadOptions):NekoConfig{
 try{
  const binding=parseSavedAccountBinding(options.binding as unknown as JsonValue);current(binding,options.currentAuthority);const material=materialAt(options,binding),state=root(options.stateHome),hash=pinHash(binding),profileId="saved_"+hash.slice(0,16),origin=new URL(binding.endpoint).origin;
  const profile:Profile={provider:binding.provider==="anthropic"?"anthropic":"openai_compat",model:binding.model,base_url:binding.endpoint,auth:"api_key"};const data={...DEFAULTS,...profile,profiles:{[profileId]:profile},active_profile:profileId,max_tokens:binding.limits.maxOutputTokens,max_steps:binding.limits.maxSteps,timeout_seconds:binding.limits.maxSeconds,mode:"default",approval:"prompt",auto_update:false,auto_update_check:false,read_outside_root:false,max_retries:0,offline_retry_seconds:0,retry_base_delay_seconds:0,retry_max_delay_seconds:0,mcp_servers:{}};
  const cfg=new NekoConfig(data,profileId,{[profileId]:profile},material,null,[],{state:"none",files:[]},state),started=performance.now();let observed=Date.now();const initial=observed;
  const validate=(config:NekoConfig)=>{const now=Date.now();if(now<observed||now>=binding.expiresAtUnixMS||performance.now()-started>=binding.expiresAtUnixMS-initial)fail();observed=now;current(binding,options.currentAuthority);if(config.provider!==profile.provider||config.model!==binding.model||config.baseUrl!==binding.endpoint.replace(/\/+$/,"")||config.profile!==profileId||config.apiKey!==material||config.data.max_retries!==0||config.data.offline_retry_seconds!==0||config.data.auto_update!==false||config.data.auto_update_check!==false||config.data.read_outside_root!==false||materialAt(options,binding)!==material)fail();};
  const budget=new ProfileBridgeBudget(state,"sa1_"+hash,binding.limits,origin,binding.expiresAtUnixMS);
  cfg.profileBridge={assertCurrent:validate,credentialSecrets:Object.freeze([material]),authPolicy:"static_key",complete:async(provider:Provider,config:NekoConfig,args:Parameters<Provider["complete"]>)=>{
   validate(config);let authorityStopped=false;const controller=new AbortController(),watchdog=setInterval(()=>{try{validate(config);}catch{authorityStopped=true;controller.abort();}},50);const next:Parameters<Provider["complete"]>=[...args];next[3]=args[3]?AbortSignal.any([args[3],controller.signal]):controller.signal;const filter=new CredentialOutputFilter([material]),onDelta=args[2];if(onDelta)next[2]=(value,kind)=>{validate(config);if(kind==="tool")return;for(const chunk of filter.push(value,kind==="reasoning"?"reasoning":"content"))onDelta(chunk.text,chunk.kind);};
   if(args[4]?.onToolCallReady){const ready=args[4].onToolCallReady;next[4]={...args[4],onToolCallReady:call=>{validate(config);if(containsCredentialJSON(call as unknown as JsonValue,[material]))fail();ready(call);}};}
   try{const result=await budget.complete(provider,()=>validate(config),n=>{config.data.max_tokens=n;},next);validate(config);if(containsCredentialJSON(result.tool_calls as unknown as JsonValue,[material]))fail();for(const chunk of filter.flush())onDelta?.(chunk.text,chunk.kind);return redactCredentialJSON(result as unknown as JsonValue,[material]) as unknown as ProviderResponse;}
   catch(error){if(authorityStopped)fail();if(error instanceof Error&&error.message==="Saved account bridge unavailable")throw error;const message=error instanceof Error?error.message:""; const status=Number(/\bHTTP\s+(\d{3})\b/.exec(message)?.[1]);const code=status===401?"unauthorized":status===403?"forbidden":status===429||status>=500&&status<=599?"transient":"unknown";throw new Error("Saved account request "+code);}finally{clearInterval(watchdog);}
  }};Object.freeze(cfg.profileBridge);validate(cfg);return cfg;
 }catch{return fail();}
}
