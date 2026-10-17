import { expect, test } from "bun:test";
import * as acp from "@agentclientprotocol/sdk";
import { createHash } from "node:crypto";
import { mkdirSync, mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { createNekoAcpAgent } from "../src/adapters/acp.ts";
import { loadConfig } from "../src/adapters/config.ts";
import { buildAgentRuntime } from "../src/adapters/agent-runtime.ts";
import { NEKOBOT_HOST_PROFILE } from "../src/adapters/host-profile.ts";
import { contextManifestDigest, type ContextManifest } from "../src/core/context/admission.ts";
import { z } from "zod";
const text="Answer only this admitted request.";
function manifest(primary=text):ContextManifest {const text=primary,hash=createHash("sha256").update(text).digest("hex");const scope={ownerUserId:"owner",spaceId:"space",botId:"bot",projectId:null,channelId:"channel",audienceRevision:1};return {version:1,contextPinSHA256:"a".repeat(64),scope,primaryMessage:{id:"message",senderId:"owner",text},sources:[{id:"message",kind:"original-user",version:1,sha256:hash,bytes:Buffer.byteLength(text),scope,accessRevision:0,observedAt:null,effectiveFrom:null,effectiveTo:null,entities:[],span:{offset:0,end:Buffer.byteLength(text),sha256:hash,text}}],factCandidates:[]};}
const meta={"neko.taskProtocol":{version:1,mode:"fixed-active-task"},"neko.executionProtocol":{version:1},"neko.hostEffectProtocol":{version:1},"neko.contextProtocol":{version:1},"neko.taskLabel":"Bound saved request","neko.contextManifest":manifest()};
const servers:Array<acp.McpServerAcp & {type:"acp"}>=[{type:"acp",name:"nekobot",serverId:"saved-host"}];
const identity=z.object({method:z.string().optional()}).passthrough();
function host(){return acp.client({name:"saved-host"}).onRequest("mcp/connect",identity,()=>({connectionId:"saved-connection"})).onRequest("mcp/disconnect",identity,()=>({})).onNotification("mcp/message",identity,()=>{}).onRequest("mcp/message",identity,({params})=>{if(params.method==="initialize")return {protocolVersion:"2025-11-25",capabilities:{tools:{}},serverInfo:{name:"fixture",version:"1"}};if(params.method==="tools/list")return {tools:NEKOBOT_HOST_PROFILE.tools.map(tool=>({name:tool.name,inputSchema:{type:"object",properties:{},additionalProperties:true}}))};throw new Error("No host effects in fixture");});}
function fixture(signal?:AbortSignal, beforeBuild?:()=>Promise<void>, primary=text){const base=mkdtempSync(join(tmpdir(),"saved-acp-")),root=join(base,"root"),home=join(base,"home"),other=join(base,"other");for(const p of [root,home,other])mkdirSync(p);const config=loadConfig({cwd:root,home});config.data.context_mode="structured";const cleanup:Promise<void>[]=[];let calls=0,builds=0,closed=0;
 const app=createNekoAcpAgent({config,signal,hostProfile:NEKOBOT_HOST_PROFILE,contextProtocol:1,...{savedAccountLaunch:{root,contextSHA256:contextManifestDigest(manifest(primary))}},trackCleanup:p=>cleanup.push(p),buildRuntime:async(cfg,options)=>{builds++;const runtime=await buildAgentRuntime(cfg,options);const close=runtime.close;runtime.close=async()=>{closed++;await close();};runtime.agent.setProvider({complete:async()=>{calls++;return {content:"Synthetic admitted result.",tool_calls:[]};}});await beforeBuild?.();return runtime;}});
 return {root,other,app,calls:()=>calls,builds:()=>builds,closed:()=>closed,close:async()=>{await Promise.allSettled(cleanup);rmSync(base,{recursive:true,force:true});}};
}
test("saved launch rejects another root or manifest before runtime creation",async()=>{
 for(const wrong of ["root","manifest"]){const f=fixture();try{await host().connectWith(f.app,async ctx=>{const changed=manifest();changed.contextPinSHA256="b".repeat(64);await expect(ctx.request(acp.methods.agent.session.new,{cwd:wrong==="root"?f.other:f.root,mcpServers:servers,_meta:{...meta,"neko.contextManifest":wrong==="manifest"?changed:manifest()}})).rejects.toThrow("Saved account");expect(f.builds()).toBe(0);expect(f.calls()).toBe(0);});}finally{await f.close();}}
});
test("saved launch admits one session and one exact prompt without pre-prompt provider work",async()=>{
 const f=fixture();try{await host().connectWith(f.app,async ctx=>{const created=await ctx.request(acp.methods.agent.session.new,{cwd:f.root,mcpServers:servers,_meta:meta});expect(f.calls()).toBe(0);await expect(ctx.request(acp.methods.agent.session.new,{cwd:f.root,mcpServers:servers,_meta:meta})).rejects.toThrow("Saved account");
 const echo={"neko.task":created._meta?.["neko.task"],"neko.execution":created._meta?.["neko.execution"],"neko.contextAdmission":created._meta?.["neko.contextAdmission"]};
 await expect(ctx.request(acp.methods.agent.session.prompt,{sessionId:created.sessionId,prompt:[{type:"text",text:"substituted text"}],_meta:echo})).rejects.toThrow("Saved account");expect(f.calls()).toBe(0);
 await ctx.request(acp.methods.agent.session.prompt,{sessionId:created.sessionId,prompt:[{type:"text",text}],_meta:echo});expect(f.calls()).toBeGreaterThan(0);const before=f.calls();await expect(ctx.request(acp.methods.agent.session.prompt,{sessionId:created.sessionId,prompt:[{type:"text",text}],_meta:echo})).rejects.toThrow("Saved account");expect(f.calls()).toBe(before);
 await expect(ctx.request(acp.methods.agent.session.setMode,{sessionId:created.sessionId,modeId:"auto"})).rejects.toThrow("Saved account");
 await ctx.request(acp.methods.agent.session.close,{sessionId:created.sessionId,_meta:echo});await expect(ctx.request(acp.methods.agent.session.new,{cwd:f.root,mcpServers:servers,_meta:meta})).rejects.toThrow("Saved account");
 });}finally{await f.close();}
});
test("saved launch denies persisted-session load or resume",async()=>{const f=fixture();try{await host().connectWith(f.app,async ctx=>{for(const method of [acp.methods.agent.session.load,acp.methods.agent.session.resume])await expect(ctx.request(method,{sessionId:"a".repeat(32),cwd:f.root,mcpServers:servers,_meta:meta})).rejects.toThrow("Saved account");expect(f.builds()).toBe(0);});}finally{await f.close();}});

test("launcher revocation closes the admitted runtime without a provider call",async()=>{
 const lifetime=new AbortController(),f=fixture(lifetime.signal);try{await expect(host().connectWith(f.app,async ctx=>{await ctx.request(acp.methods.agent.session.new,{cwd:f.root,mcpServers:servers,_meta:meta});expect(f.builds()).toBe(1);expect(f.calls()).toBe(0);lifetime.abort();})).rejects.toThrow("ACP connection closed");await f.close();expect(f.closed()).toBe(1);expect(f.calls()).toBe(0);}finally{await f.close();}
});

test("revocation awaits in-flight admission and closes its late runtime",async()=>{
 let release!:()=>void,entered!:()=>void;
 const blocked=new Promise<void>(resolve=>{release=resolve;}),building=new Promise<void>(resolve=>{entered=resolve;});
 const lifetime=new AbortController(),f=fixture(lifetime.signal,async()=>{entered();await blocked;});
 try {
  const connection=host().connectWith(f.app,async ctx=>{
   const admission=ctx.request(acp.methods.agent.session.new,{cwd:f.root,mcpServers:servers,_meta:meta});
   void admission.catch(()=>{});
   await building;lifetime.abort();release();
  });
  await expect(connection).rejects.toThrow("ACP connection closed");
  await f.close();expect(f.closed()).toBe(1);expect(f.calls()).toBe(0);
 }finally{release();await f.close();}
});

test("saved launch rejects whitespace-only and padded command prompts",async()=>{
 for(const primary of [" /task list","   "]){const f=fixture(undefined,undefined,primary);try{await host().connectWith(f.app,async ctx=>{
  const created=await ctx.request(acp.methods.agent.session.new,{cwd:f.root,mcpServers:servers,_meta:{...meta,"neko.contextManifest":manifest(primary)}});
  const echo={"neko.task":created._meta?.["neko.task"],"neko.execution":created._meta?.["neko.execution"],"neko.contextAdmission":created._meta?.["neko.contextAdmission"]};
  await expect(ctx.request(acp.methods.agent.session.prompt,{sessionId:created.sessionId,prompt:[{type:"text",text:primary}],_meta:echo})).rejects.toThrow("Saved account");expect(f.calls()).toBe(0);
 });}finally{await f.close();}}
});
