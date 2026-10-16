import {expect,test} from "bun:test";
import {fixedServiceFetch,serviceTarget,type ServiceNativeDriver} from "../../api/local-service-http";
function driver(run:(send:(event:any)=>void,args:any)=>Promise<void>){
  let send!:(event:any)=>void;const calls:{name:string;args:any}[]=[];
  const native:ServiceNativeDriver={channel:handler=>{send=handler;return "fixture-channel";},invoke:async(name,args)=>{calls.push({name,args});if(name==="neko_wiii_service_fetch")await run(send,args);}};
  return {native,calls};
}
test("fixed endpoint rejects Neko bootstrap, authority routes and encoded traversal",async()=>{
  let called=0;const f=driver(async()=>{called++;});
  for(const path of ["/","/v1/adapters/catalog","/v1/messages","/api/v1/%2e%2e/%2e%2e/","/api/v1/foo%2f..%2f..","/health#cookie"]){await expect(fixedServiceFetch(f.native,"http://127.0.0.1:8765"+path)).rejects.toThrow();}
  for(const url of ["http://user:password@localhost:8000/health","http://example.com/health","https://other.example/health"]){await expect(fixedServiceFetch(f.native,url)).rejects.toThrow();}
  expect(called).toBe(0);
});
test("configured local and exact official remote service routes are preserved",()=>{
  expect(serviceTarget("http://localhost:8080/api/v1/threads?q=hello%20world")).toEqual({port:8080,remote:false,path:"/api/v1/threads?q=hello%20world"});
  expect(serviceTarget("https://wiii.holilihu.online/api/v1/health")).toEqual({port:null,remote:true,path:"/api/v1/health"});
  expect(serviceTarget("https://other.example/api/v1/health")).toBeNull();
});
test("encoded colon and Unicode API IDs preserve their canonical request path",()=>{
  const path="/api/v1/organizations/"+encodeURIComponent("org:đội")+"/settings";
  expect(serviceTarget("http://localhost:8000"+path)?.path).toBe(path);
  expect(serviceTarget("http://localhost:8000/api/v1/organizations/org:đội/settings")?.path).toBe("/api/v1/organizations/org:%C4%91%E1%BB%99i/settings");
});
test("segment decoding rejects traversal, separators, controls and invalid escape/UTF8 before IPC",async()=>{
  const f=driver(async()=>{});
  for(const segment of ["%2E%2e","%252e%252e","a%2Fb","a%5cb","a%3Fb","a%23b","a%00b","a%C2%80b","%FF","%C0%AF","%","%1","..","."]){
    await expect(fixedServiceFetch(f.native,"http://localhost:8000/api/v1/x/"+segment+"/y")).rejects.toThrow("route denied");
  }
  expect(f.calls).toEqual([]);
});
test("JSON body, scoped headers and900s header deadline survive transport",async()=>{
  const f=driver(async(send)=>{send({kind:"headers",status:200,headers:[["content-type","application/json"]]});send({kind:"chunk",bytes:[...new TextEncoder().encode('{"ok":true}')]});send({kind:"done"});});
  const response=await fixedServiceFetch(f.native,"http://localhost:8080/api/v1/chat",{method:"POST",headers:{"Content-Type":"application/json","X-Session-ID":"fixture-session"},body:'{"text":"fixture"}'},900000,()=>"fixture-request-a");
  expect(await response.json()).toEqual({ok:true});const request=f.calls[0].args.request;
  expect(request.port).toBe(8080);expect(request.timeoutMs).toBe(900000);expect(atob(request.bodyBase64)).toBe('{"text":"fixture"}');expect(request.headers).toContainEqual(["x-session-id","fixture-session"]);
});
test("multipart upload retains generated boundary and binary file bytes",async()=>{
  const f=driver(async(send)=>{send({kind:"headers",status:200,headers:[["content-type","application/octet-stream"]]});send({kind:"chunk",bytes:[0,1,254,255]});send({kind:"done"});});
  const form=new FormData();form.append("file",new File([new Uint8Array([0,1,254,255])],"fixture.bin",{type:"application/octet-stream"}));form.append("parser_mode","fast");
  const response=await fixedServiceFetch(f.native,"http://localhost:8000/api/v1/document-context/parse",{method:"POST",body:form});
  const request=f.calls[0].args.request;const ct=request.headers.find(([key]:string[])=>key==="content-type")[1];expect(ct).toContain("multipart/form-data; boundary=");
  const encoded=atob(request.bodyBase64);expect(encoded).toContain("fixture.bin");expect(encoded).toContain(String.fromCharCode(0,1,254,255));expect(encoded).toContain("fast");expect([...new Uint8Array(await response.arrayBuffer())]).toEqual([0,1,254,255]);
});
test("SSE resolves headers before completion and keeps progressive chunks",async()=>{
  let release!:()=>void;const done=new Promise<void>(resolve=>{release=resolve;});
  const f=driver(async(send)=>{send({kind:"headers",status:200,headers:[["content-type","text/event-stream"]]});send({kind:"chunk",bytes:[...new TextEncoder().encode("data: one\n\n")]});await done;send({kind:"chunk",bytes:[...new TextEncoder().encode("data: two\n\n")]});send({kind:"done"});});
  const response=await fixedServiceFetch(f.native,"http://localhost:8000/api/v1/chat/stream/v3",{method:"POST",body:"{}"});const reader=response.body!.getReader();
  expect(new TextDecoder().decode((await reader.read()).value)).toBe("data: one\n\n");release();expect(new TextDecoder().decode((await reader.read()).value)).toBe("data: two\n\n");expect((await reader.read()).done).toBe(true);
});
test("already aborted request emits zero native calls",async()=>{
  const f=driver(async()=>{}),c=new AbortController();c.abort();await expect(fixedServiceFetch(f.native,"http://localhost:8000/health",{signal:c.signal})).rejects.toThrow();expect(f.calls).toEqual([]);
});
test("abort while awaiting headers sends scoped cancel and cannot resolve late headers",async()=>{
  let send!:(event:any)=>void,release!:()=>void;const work=new Promise<void>(r=>{release=r;});const f=driver(async(s)=>{send=s;await work;});const c=new AbortController();
  const response=fixedServiceFetch(f.native,"http://localhost:8000/health",{signal:c.signal},60000,()=>"fixture-cancel-a");await new Promise(r=>setTimeout(r,0));c.abort();await expect(response).rejects.toThrow();
  expect(f.calls.some(c=>c.name==="neko_wiii_service_cancel"&&c.args.requestId==="fixture-cancel-a")).toBe(true);send({kind:"headers",status:200,headers:[]});release();
});
test("stream reader cancellation cancels only its native request",async()=>{
  let release!:()=>void;const work=new Promise<void>(r=>{release=r;});const f=driver(async(send)=>{send({kind:"headers",status:200,headers:[]});await work;send({kind:"done"});});
  const response=await fixedServiceFetch(f.native,"http://localhost:8000/api/v1/chat/stream/v3",{},60000,()=>"fixture-cancel-b");await response.body!.cancel();expect(f.calls.some(c=>c.name==="neko_wiii_service_cancel"&&c.args.requestId==="fixture-cancel-b")).toBe(true);release();
});
test("Cookie, Host and Origin inputs are denied before native send",async()=>{
  const f=driver(async()=>{});for(const key of ["Cookie","Host","Origin"]){await expect(fixedServiceFetch(f.native,"http://localhost:8000/health",{headers:{[key]:"fixture-only"}})).rejects.toThrow();}expect(f.calls).toEqual([]);
});
test("Set-Cookie and redirect response events never reach a Response",async()=>{
  for(const event of [{kind:"headers",status:200,headers:[["set-cookie","fixture-only"]]},{kind:"headers",status:302,headers:[]}]){
    const f=driver(async(send)=>{send(event);});await expect(fixedServiceFetch(f.native,"http://localhost:8000/health")).rejects.toThrow();
  }
});
test("no-content204 semantics and native rejection remain intact",async()=>{
  const f=driver(async(send)=>{send({kind:"headers",status:204,headers:[]});send({kind:"done"});});const response=await fixedServiceFetch(f.native,"http://localhost:8000/api/v1/auth/logout",{method:"POST"});expect(response.status).toBe(204);expect(response.body).toBeNull();
  const denied=driver(async()=>{throw new Error("fixture-native-rejected");});await expect(fixedServiceFetch(denied.native,"http://localhost:8000/health")).rejects.toThrow("fixture-native-rejected");
});
