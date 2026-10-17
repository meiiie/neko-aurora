import { realpathSync } from "node:fs";
import { isAbsolute } from "node:path";
import { type Readable, type Writable } from "node:stream";
import { isJsonObject, isText, type JsonValue } from "../shared/wire.ts";
import { loadSavedAccountBridge, parseSavedAccountBinding } from "./saved-account-bridge.ts";
import { openSavedAccountWitness, parseNativeWitnessDescriptor, type NativeWitnessDescriptor, type SavedAccountWitnessOptions } from "./saved-account-witness.ts";
import { runAcpServer } from "./acp.ts";
import { NEKOBOT_HOST_PROFILE } from "./host-profile.ts";

interface Bootstrap extends SavedAccountWitnessOptions {
 version:1;purpose:"saved_account_acp_v1";capsuleId:string;capsulePath:string;stateHome:string;root:string;nativeWitness:NativeWitnessDescriptor;
}
const KEYS=["version","purpose","binding","capsuleId","capsulePath","stateHome","revisionKey","witnessPath","serviceGeneration","authoritySHA256","root","nativeWitness"].sort().join();
function unavailable():Error {return new Error("Saved account launch unavailable");}
function canonical(v:JsonValue):string {
 if(Array.isArray(v))return "["+v.map(canonical).join(",")+"]";
 if(isJsonObject(v))return "{"+Object.keys(v).sort().map(k=>JSON.stringify(k)+":"+canonical(v[k]!)).join(",")+"}";
 return JSON.stringify(v);
}
function path(value:JsonValue|undefined):string {
 if(!isText(value)||!isAbsolute(value)||Buffer.byteLength(value)>4096||/[\x00-\x1f\x7f]/.test(value))throw unavailable();return value;
}
// Consume only the private first line. ACP may already share the same pipe chunk;
// put its untouched tail back before handing the stream to the existing server.
async function firstLine(input:Readable):Promise<string> {
 let buffer=Buffer.alloc(0);
 try {
  return await new Promise<string>((resolve,reject)=>{
   const stop=()=>{clearTimeout(timer);input.off("data",data);input.off("end",failed);input.off("error",failed);input.pause();};
   const failed=()=>{stop();input.destroy();reject(unavailable());};
   const data=(chunk:Buffer)=>{
    try {
     const end=chunk.indexOf(10),length=end<0?chunk.length:end;
     if(buffer.length+length+1>16384){failed();return;}
     buffer=Buffer.concat([buffer,chunk.subarray(0,length)]);
     if(end<0)return;
     const line=new TextDecoder("utf-8",{fatal:true,ignoreBOM:true}).decode(buffer);stop();
     if(end+1<chunk.length)input.unshift(Buffer.from(chunk.subarray(end+1)));
     resolve(line);
    }catch{failed();}
   };
   const timer=setTimeout(failed,2000);
   input.on("data",data);input.once("end",failed);input.once("error",failed);input.resume();
  });
 }finally{buffer.fill(0);}
}

/** Exact future private transport contract. No current CLI route invokes it. */
export async function readSavedAccountAcpBootstrap(input:Readable,argv:readonly string[]):Promise<Bootstrap> {
 try {
  if(argv.join("\0")!==["saved-account","acp","--broker-stdin"].join("\0")
   ||Object.keys(process.env).some(name=>name==="NEKO_CREDENTIAL_HOME"||name.startsWith("NEKO_PROFILE")))throw unavailable();
  const raw=await firstLine(input);
  // SAFETY: bounded JSON syntax enters the JSON domain; exact canonical schema follows.
  const value=JSON.parse(raw) as JsonValue;
  if(!isJsonObject(value)||Object.keys(value).sort().join()!==KEYS||canonical(value)!==raw||value.version!==1||value.purpose!=="saved_account_acp_v1"
   ||!value.binding||!value.nativeWitness||!isText(value.capsuleId)||!/^sa1_[a-f0-9]{32}$/.test(value.capsuleId)
   ||!isText(value.revisionKey)||!isText(value.serviceGeneration)||!isText(value.authoritySHA256))throw unavailable();
  return {version:1,purpose:"saved_account_acp_v1",binding:parseSavedAccountBinding(value.binding),capsuleId:value.capsuleId,
   capsulePath:path(value.capsulePath),stateHome:path(value.stateHome),root:path(value.root),witnessPath:path(value.witnessPath),
   revisionKey:value.revisionKey,serviceGeneration:value.serviceGeneration,authoritySHA256:value.authoritySHA256,nativeWitness:parseNativeWitnessDescriptor(value.nativeWitness)};
 }catch{throw unavailable();}
}

/** Internal trusted-host startup only, deliberately unwired until native acceptance.
 * Rust retains install guards and waits for durable admitted/prompt_ready ACKs before
 * sending session/prompt. Core does not interpret a capsule as that host admission. */
export async function runSavedAccountAcp(argv:readonly string[],transport:{input:Readable;output:Writable}):Promise<void> {
 try {
  const bootstrap=await readSavedAccountAcpBootstrap(transport.input,argv),root=realpathSync.native(bootstrap.root);
  const witness=await openSavedAccountWitness(bootstrap);
  try {
   const config=loadSavedAccountBridge({...bootstrap,currentAuthority:witness.authority});
   config.data.context_mode="structured";
   await runAcpServer({hostProfile:NEKOBOT_HOST_PROFILE,contextProtocol:1,signal:witness.signal,
    savedAccountLaunch:{root,contextSHA256:bootstrap.binding.contextSHA256},
    configForRoot:(requested,profile)=>{
     if(realpathSync.native(requested)!==root||profile&&profile!==config.profile)throw unavailable();
     config.profileBridge!.assertCurrent(config);return config;
    }},transport);
   if(witness.signal.aborted)throw unavailable();
  }finally{await witness.close();}
 }catch{throw unavailable();}
}
