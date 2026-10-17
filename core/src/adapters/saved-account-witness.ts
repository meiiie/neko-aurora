import { createHash, createHmac, timingSafeEqual } from "node:crypto";
import { closeSync, constants, fstatSync, lstatSync, openSync, readSync, type Stats } from "node:fs";
import { dirname, isAbsolute, normalize } from "node:path";
import { isJsonObject, isText, type JsonValue } from "../shared/wire.ts";
import { parseSavedAccountBinding, type SavedAccountAuthority, type SavedAccountBinding } from "./saved-account-bridge.ts";

export interface SavedAccountWitnessOptions {
 binding:SavedAccountBinding; witnessPath:string; serviceGeneration:string; authoritySHA256:string; revisionKey:string;
}
const HEX=/^[a-f0-9]{64}$/;
const KEYS=["version","purpose","bindingSHA256","serviceGeneration","authoritySHA256","revision","state","issuedAtUnixMS","validUntilUnixMS","mac"].sort().join();
function fail():never {throw new Error("Saved account authority unavailable");}
function canonical(v:JsonValue):string {
 if(Array.isArray(v))return "["+v.map(canonical).join(",")+"]";
 if(isJsonObject(v))return "{"+Object.keys(v).sort().map(k=>JSON.stringify(k)+":"+canonical(v[k]!)).join(",")+"}";
 return JSON.stringify(v);
}
function digest(binding:SavedAccountBinding):string {return createHash("sha256").update(canonical(binding)).digest("hex");}
function integer(v:JsonValue|undefined,min:number):v is number {return typeof v==="number"&&Number.isSafeInteger(v)&&v>=min;}
function sameFile(a:Stats,b:Stats):boolean {return a.dev===b.dev&&a.ino===b.ino;}
function regular(s:Stats):boolean {return s.isFile()&&!s.isSymbolicLink()&&s.nlink===1&&s.size<=4096;}
function directories(path:string):Array<[string,Stats]> {
 const result:Array<[string,Stats]>=[];
 for(let cursor=dirname(path);;cursor=dirname(cursor)) {
  const stat=lstatSync(cursor);if(!stat.isDirectory()||stat.isSymbolicLink())fail();result.push([cursor,stat]);
  if(dirname(cursor)===cursor)return result;
 }
}
// Go renews by atomic rename: identity is stable within a read, not across revisions.
function readWitness(path:string):string {
 const parents=directories(path),before=lstatSync(path);if(!regular(before))fail();
 const fd=openSync(path,constants.O_RDONLY|constants.O_NOFOLLOW|constants.O_NONBLOCK),buffer=Buffer.alloc(4097);
 try {
  const opened=fstatSync(fd);if(!regular(opened)||!sameFile(before,opened))fail();
  let used=0;
  while(used<buffer.length){const count=readSync(fd,buffer,used,buffer.length-used,null);if(count===0)break;used+=count;}
  const after=fstatSync(fd),named=lstatSync(path);
  if(used>4096||!regular(after)||!regular(named)||!sameFile(opened,after)||!sameFile(opened,named)
   ||opened.size!==after.size||opened.size!==used||opened.mtimeMs!==after.mtimeMs||opened.ctimeMs!==after.ctimeMs
   ||after.size!==named.size||after.mtimeMs!==named.mtimeMs||after.ctimeMs!==named.ctimeMs)fail();
  for(const [parent,prior] of parents){const next=lstatSync(parent);if(!next.isDirectory()||next.isSymbolicLink()||!sameFile(prior,next))fail();}
  return new TextDecoder("utf-8",{fatal:true,ignoreBOM:true}).decode(buffer.subarray(0,used));
 }finally{buffer.fill(0);closeSync(fd);}
}

/** Private preparation consumer; ordinary CLI/config and runtime execution stay off.
 * One live reader per task. No whole-state rollback or restart/tombstone guarantee.
 * Windows is deliberately denied until native handle/reparse identity is verified.
 */
export function createSavedAccountWitnessAuthority(options:SavedAccountWitnessOptions):SavedAccountAuthority {
 try {
  const path=options.witnessPath;
  if(process.platform==="win32"||!isAbsolute(path)||normalize(path)!==path||Buffer.byteLength(path)>4096||/[\x00-\x1f\x7f]/.test(path))fail();
  return createSavedAccountWitnessVerifier(options,()=>readWitness(path));
 }catch{return fail();}
}

/** Shared signed-record verifier. Only trusted readers supply original bounded bytes;
 * callbacks do not select or broaden the binding, boot, key or proof deadlines. */
export function createSavedAccountWitnessVerifier(options:SavedAccountWitnessOptions,read:()=>string):SavedAccountAuthority {
 try {
  const binding=parseSavedAccountBinding(options.binding),bindingHash=digest(binding);
  const {serviceGeneration:boot,authoritySHA256:authority,revisionKey}=options;
  if(!/^executor-boot-[a-f0-9]{32}$/.test(boot)||!HEX.test(authority)||!HEX.test(revisionKey))fail();
  const material=Buffer.from(revisionKey,"hex");
  const key=createHmac("sha256",material).update("neko-saved-key-v1/authority-witness").digest();material.fill(0);
  const openedWall=Date.now(),openedMono=performance.now(),taskDeadline=openedMono+binding.expiresAtUnixMS-openedWall;
  let observed=openedWall,lastRevision=0,lastBytes="",proofDeadline=Infinity,terminal=false;
  const clock=()=>{
   const wall=Date.now(),mono=performance.now();
   if(wall<observed||mono<openedMono||wall>=binding.expiresAtUnixMS||mono>=taskDeadline)fail();
   observed=wall;return {wall,mono};
  };
  const check:SavedAccountAuthority=(candidate)=>{
   if(terminal)return false;
   try {
    if(digest(parseSavedAccountBinding(candidate))!==bindingHash)fail();clock();
    const raw=read();
    if(Buffer.byteLength(raw)>4096)fail();
    // SAFETY: JSON parsing yields the JSON domain; exact schema and field types follow.
    const record=JSON.parse(raw) as JsonValue;
    if(!isJsonObject(record)||Object.keys(record).sort().join()!==KEYS||canonical(record)!==raw
     ||record.version!==1||record.purpose!=="saved_account_authority_v1"||record.bindingSHA256!==bindingHash
     ||record.serviceGeneration!==boot||record.authoritySHA256!==authority||!integer(record.revision,1)
     ||!integer(record.issuedAtUnixMS,0)||!integer(record.validUntilUnixMS,0)||!isText(record.mac)||!HEX.test(record.mac))fail();
    const {mac,...body}=record,expected=createHmac("sha256",key).update(canonical(body)).digest();
    if(!timingSafeEqual(Buffer.from(mac,"hex"),expected)||record.state!=="active"
     ||record.validUntilUnixMS<=record.issuedAtUnixMS||record.validUntilUnixMS-record.issuedAtUnixMS>5000
     ||record.validUntilUnixMS>binding.expiresAtUnixMS||record.revision<lastRevision
     ||record.revision===lastRevision&&raw!==lastBytes)fail();
    const {wall,mono}=clock();
    if(wall<record.issuedAtUnixMS||wall>=record.validUntilUnixMS)fail();
    if(record.revision!==lastRevision) {
     // Original clock mapping also prevents a newer revision with unchanged expiry
     // from renewing its lifetime while the wall clock is frozen.
     proofDeadline=Math.min(mono+record.validUntilUnixMS-wall,openedMono+record.validUntilUnixMS-openedWall,taskDeadline);
     lastRevision=record.revision;lastBytes=raw;
    }
    if(clock().mono>=proofDeadline)fail();return true;
   }catch{terminal=true;key.fill(0);return false;}
  };
  if(binding.expiresAtUnixMS-openedWall>binding.limits.maxSeconds*1000||!check(binding)){key.fill(0);fail();}
  return check;
 }catch{return fail();}
}
