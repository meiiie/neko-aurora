import { spawn } from "node:child_process";
import { createHash } from "node:crypto";
import { createReadStream, lstatSync, realpathSync } from "node:fs";
import { isAbsolute, normalize } from "node:path";
import { isJsonObject, isText, type JsonValue } from "../shared/wire.ts";
import { parseSavedAccountBinding, type SavedAccountAuthority } from "./saved-account-bridge.ts";
import { createSavedAccountWitnessVerifier, type SavedAccountWitnessOptions } from "./saved-account-witness.ts";

// Trusted launcher must establish a protected installation first. A hash/stat
// check cannot prevent a writable pathname/ancestor being swapped before spawn.
interface NativeOptions extends SavedAccountWitnessOptions {executable:string;executableSHA256:string}
export interface NativeSavedAccountWitness {authority:SavedAccountAuthority;close():Promise<void>}
function unavailable():Error {return new Error("Saved account authority unavailable");}
async function verifyTrustedExecutable(path:string,expected:string):Promise<void> {
 if(!isAbsolute(path)||normalize(path)!==path||/[\x00-\x1f\x7f]/.test(path)||!/^([a-f0-9]{64})$/.test(expected)||realpathSync.native(path)!==path)throw unavailable();
 const before=lstatSync(path);if(!before.isFile()||before.isSymbolicLink()||before.nlink!==1)throw unavailable();
 const hash=createHash("sha256");for await(const bytes of createReadStream(path))hash.update(bytes);
 const after=lstatSync(path);
 if(hash.digest("hex")!==expected||!after.isFile()||after.isSymbolicLink()||after.nlink!==1||before.dev!==after.dev||before.ino!==after.ino||before.size!==after.size||before.mtimeMs!==after.mtimeMs||before.ctimeMs!==after.ctimeMs)throw unavailable();
}

/** Explicit trusted-launcher seam, NOT wired into CLI or ACP. Windows activation
 * remains gated on native acceptance. Freshness is bounded observation, not a
 * synchronous per-call filesystem check: samples every 50ms, max age 100ms. */
export async function openNativeSavedAccountWitness(options:NativeOptions):Promise<NativeSavedAccountWitness> {
 try {
  const binding=parseSavedAccountBinding(options.binding),path=options.witnessPath;
  if(!isAbsolute(path)||normalize(path)!==path||Buffer.byteLength(path)>4096||/[\x00-\x1f\x7f]/.test(path))throw unavailable();
  const init=JSON.stringify({version:1,path})+"\n";if(Buffer.byteLength(init)>8192)throw unavailable();
  await verifyTrustedExecutable(options.executable,options.executableSHA256);
  const child=spawn(options.executable,["--saved-witness-reader"],{windowsHide:true,stdio:["pipe","pipe","pipe"],env:process.platform==="win32"?{SystemRoot:process.env.SystemRoot??"C:\\Windows"}:{}});
  const exited=new Promise<void>(resolve=>child.once("close",()=>resolve()));
  let terminal=false,sequence=0,buffer=Buffer.alloc(0),latest="",observedAt=-Infinity;
  let verifier:SavedAccountAuthority|undefined,sample:ReturnType<typeof setTimeout>|undefined,observationExpiry:ReturnType<typeof setTimeout>|undefined;
  let pending:{sequence:number;sent:number;resolve:()=>void;reject:(error:Error)=>void;timer:ReturnType<typeof setTimeout>}|undefined;
  const poison=()=>{
   if(terminal)return;terminal=true;clearTimeout(sample);clearTimeout(observationExpiry);buffer.fill(0);buffer=Buffer.alloc(0);latest="";
   if(pending){const wait=pending;pending=undefined;clearTimeout(wait.timer);wait.reject(unavailable());}
   verifier?.(binding);child.stdin.destroy();child.kill("SIGKILL");
  };
  const read=()=>{if(terminal||performance.now()-observedAt>=100||performance.now()<observedAt){poison();throw unavailable();}return latest;};
  const authority:SavedAccountAuthority=candidate=>{if(terminal||!verifier)return false;const current=verifier(candidate);if(!current)poison();return current;};
  child.once("error",poison);child.once("exit",poison);child.stdout.once("end",poison);
  child.stdin.on("error",poison);child.stdout.on("error",poison);child.stderr.on("error",poison);
  // No helper diagnostics are forwarded or retained, including private paths.
  child.stderr.on("data",poison);
  child.stdout.on("data",(chunk:Buffer)=>{
   if(terminal)return;
   try {
    if(!pending||buffer.length+chunk.length>24576)throw unavailable();
    buffer=Buffer.concat([buffer,chunk]);const end=buffer.indexOf(10);if(end<0)return;
    if(end!==buffer.length-1)throw unavailable();
    const line=new TextDecoder("utf-8",{fatal:true,ignoreBOM:true}).decode(buffer.subarray(0,end));buffer.fill(0);buffer=Buffer.alloc(0);
    // SAFETY: JSON syntax enters the JSON domain; the exact frame is validated here.
    const frame=JSON.parse(line) as JsonValue;
    if(!isJsonObject(frame)||Object.keys(frame).sort().join()!=="sequence,version,witness"||JSON.stringify(frame)!==line||frame.version!==1||frame.sequence!==pending.sequence||!isText(frame.witness)||Buffer.byteLength(frame.witness)>4096||performance.now()-pending.sent>=100)throw unavailable();
    if(latest&&performance.now()-observedAt>=100)throw unavailable();
    latest=frame.witness;observedAt=pending.sent;
    clearTimeout(observationExpiry);observationExpiry=setTimeout(poison,Math.max(0,100-(performance.now()-observedAt)));
    const wait=pending;pending=undefined;clearTimeout(wait.timer);wait.resolve();
   }catch{poison();}
  });
  const request=async()=>{
   if(terminal||pending||sequence>=Number.MAX_SAFE_INTEGER)throw unavailable();
   await new Promise<void>((resolve,reject)=>{
    const sent=performance.now();pending={sequence:++sequence,sent,resolve,reject,timer:setTimeout(poison,100)};
    child.stdin.write(JSON.stringify({version:1,sequence})+"\n");
   });
  };
  const poll=()=>{sample=setTimeout(()=>{void request().then(()=>{if(!authority(binding))return;poll();},poison);},Math.max(0,50-(performance.now()-observedAt)));};
  const close=async()=>{poison();await exited;};
  try {
   child.stdin.write(init);await request();
   verifier=createSavedAccountWitnessVerifier(options,read);if(!authority(binding))throw unavailable();poll();
   return {authority,close};
  }catch{await close();throw unavailable();}
 }catch{throw unavailable();}
}
