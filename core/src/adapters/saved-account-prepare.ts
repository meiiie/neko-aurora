import { isJsonObject, isText, type JsonValue } from "../shared/wire.ts";
import { parseSavedAccountBinding, prepareSavedAccountCapsule, type SavedAccountReceipt } from "./saved-account-bridge.ts";
import { createSavedAccountWitnessAuthority } from "./saved-account-witness.ts";

const KEYS=["version","purpose","binding","secret","capsulePath","stateHome","revisionKey","witnessPath","serviceGeneration","authoritySHA256"].sort().join();
function fail():never {throw new Error("Saved account preparation unavailable");}
function canonical(v:JsonValue):string {
 if(Array.isArray(v))return "["+v.map(canonical).join(",")+"]";
 if(isJsonObject(v))return "{"+Object.keys(v).sort().map(k=>JSON.stringify(k)+":"+canonical(v[k]!)).join(",")+"}";
 return JSON.stringify(v);
}
// A host must close its private pipe. Neither an unlimited stream nor a missing EOF
// may keep a secret-bearing preparation process alive indefinitely.
async function privateInput():Promise<string> {
 const chunks:Buffer[]=[];let length=0;
 try {
  await new Promise<void>((resolve,reject)=>{
   const finish=(error?:Error)=>{clearTimeout(timer);process.stdin.off("data",data);process.stdin.off("end",end);process.stdin.off("error",failed);process.stdin.pause();if(error){process.stdin.destroy();reject(error);}else resolve();};
   const data=(chunk:Buffer)=>{const bytes=Buffer.from(chunk);length+=bytes.length;if(length>16384){bytes.fill(0);finish(new Error("Invalid input"));}else chunks.push(bytes);};
   const end=()=>finish(),failed=()=>finish(new Error("Invalid input"));
   const timer=setTimeout(failed,2000);
   process.stdin.on("data",data);process.stdin.once("end",end);process.stdin.once("error",failed);process.stdin.resume();
  });
  const bytes=Buffer.concat(chunks);try{return new TextDecoder("utf-8",{fatal:true,ignoreBOM:true}).decode(bytes);}finally{bytes.fill(0);}
 }finally{for(const chunk of chunks)chunk.fill(0);}
}

/** Private host-only preparation transport. A receipt is not execution authority. */
export async function prepareSavedAccountFromStdin(argv:string[]):Promise<SavedAccountReceipt> {
 try {
  if(argv.join("\0")!==["saved-account","prepare","--json","--broker-stdin"].join("\0")
   ||Object.keys(process.env).some(name=>name==="NEKO_CREDENTIAL_HOME"||name.startsWith("NEKO_PROFILE")))fail();
  const raw=await privateInput(),body=raw.endsWith("\n")?raw.slice(0,-1):raw;
  // SAFETY: JSON syntax enters the JSON domain; exact canonical schema follows.
  const input=JSON.parse(body) as JsonValue;
  if(!isJsonObject(input)||Object.keys(input).sort().join()!==KEYS||canonical(input)!==body
   ||!input.binding||!isText(input.witnessPath)||!isText(input.serviceGeneration)||!isText(input.authoritySHA256)||!isText(input.revisionKey))fail();
  const {witnessPath,serviceGeneration,authoritySHA256,...capsule}=input;
  const binding=parseSavedAccountBinding(input.binding);
  const authority=createSavedAccountWitnessAuthority({binding,witnessPath,serviceGeneration,authoritySHA256,revisionKey:input.revisionKey});
  return prepareSavedAccountCapsule(capsule,authority);
 }catch{return fail();}
}
