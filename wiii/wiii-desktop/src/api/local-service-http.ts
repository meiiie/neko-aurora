export interface ServiceNativeRequest { requestId: string; port: number | null; remote: boolean; path: string; method: string;
  headers: [string,string][]; bodyBase64: string | null; timeoutMs: number }
type Event = {kind:"headers";status:number;headers:[string,string][]}|{kind:"chunk";bytes:number[]}|{kind:"done"};
export interface ServiceNativeDriver { channel(onmessage:(event:Event)=>void):unknown; invoke(command:string,args:Record<string,unknown>):Promise<unknown> }
const MAX_BYTES=64*1024*1024;
const fail=(message:string):never=>{throw new Error(`Wiii local service: ${message}`);};
function decodedPath(path:string):string {
  if(path!=="/health"&&!path.startsWith("/api/v1/"))fail("route denied");
  return path.split("/").slice(1).map(segment=>{
    let value:string;try{value=decodeURIComponent(segment);}catch{return fail("route denied");}
    if(!value||value==="."||value===".."||/[\p{Cc}\u2028\u2029/\\%?#]/u.test(value))fail("route denied");
    return value;
  }).join("/");
}
export function serviceTarget(url:string):{port:number|null;remote:boolean;path:string}|null {
  const parsed=new URL(url);
  if(parsed.username||parsed.password||parsed.hash)fail("invalid endpoint");
  const local=parsed.protocol==="http:"&&["localhost","127.0.0.1"].includes(parsed.hostname);
  const remote=parsed.origin==="https://wiii.holilihu.online";
  if(!local&&!remote)return null;
  const path=parsed.pathname;
  const original=url.match(/^[a-z][a-z0-9+.-]*:\/\/[^/?#]*(\/[^?#]*)?/i)?.[1]??"/";
  if(decodedPath(original)!==decodedPath(path))fail("route denied");
  return {port:local?Number(parsed.port||80):null,remote,path:path+parsed.search};
}
function encode(bytes:Uint8Array):string {
  let binary="";for(let i=0;i<bytes.length;i+=16384)binary+=String.fromCharCode(...bytes.subarray(i,i+16384));return btoa(binary);
}
export async function fixedServiceFetch(driver:ServiceNativeDriver,url:string,init:RequestInit={},timeoutMs=60000,makeId:()=>string=()=>crypto.randomUUID()):Promise<Response> {
  const target=serviceTarget(url);if(!target)fail("unsupported endpoint");
  if(init.signal?.aborted)throw new DOMException("Aborted","AbortError");
  if(!Number.isInteger(timeoutMs)||timeoutMs<1||timeoutMs>900000)fail("invalid timeout");
  const request=new Request(url,init);const headers=[...request.headers.entries()] as [string,string][];
  for(const [name] of headers)if(!["authorization","x-api-key","x-user-id","x-organization-id","x-session-id","x-request-id","content-type","accept","accept-language"].includes(name.toLowerCase()))fail("header denied");
  const bytes=init.body===undefined||init.body===null?null:new Uint8Array(await request.arrayBuffer());
  if(bytes&&bytes.length>MAX_BYTES)fail("body oversized");
  if(init.signal?.aborted)throw new DOMException("Aborted","AbortError");
  const requestId=makeId();let settled=false,ended=false,receivedHeaders=false,total=0;
  let controller!:ReadableStreamDefaultController<Uint8Array>;
  const stream=new ReadableStream<Uint8Array>({start(c){controller=c;},cancel(){ended=true;void driver.invoke("neko_wiii_service_cancel",{requestId}).catch(()=>{});}});
  let resolve!:(response:Response)=>void,reject!:(error:unknown)=>void;
  const pending=new Promise<Response>((yes,no)=>{resolve=yes;reject=no;});
  const finishError=(error:unknown)=>{if(ended)return;ended=true;if(!settled){settled=true;reject(error);}controller.error(error);};
  const abort=()=>{finishError(new DOMException("Aborted","AbortError"));void driver.invoke("neko_wiii_service_cancel",{requestId}).catch(()=>{});};
  init.signal?.addEventListener("abort",abort,{once:true});
  const channel=driver.channel(event=>{
    if(ended)return;
    try {
      if(event.kind==="headers") {
        if(receivedHeaders||!Number.isInteger(event.status)||event.status<200||event.status>599||event.status>=300&&event.status<400)fail("invalid response");
        const output=new Headers();for(const [key,value] of event.headers){if(!["content-type","content-length","content-disposition","cache-control","etag","x-request-id","x-trace-id"].includes(key.toLowerCase()))fail("unexpected response header");output.append(key,value);}
        receivedHeaders=true;settled=true;resolve(new Response([204,205,304].includes(event.status)?null:stream,{status:event.status,headers:output}));
      } else if(event.kind==="chunk") {
        if(!receivedHeaders||event.bytes.length>16384||event.bytes.some(n=>!Number.isInteger(n)||n<0||n>255)||(total+=event.bytes.length)>MAX_BYTES)fail("invalid response chunk");
        controller.enqueue(Uint8Array.from(event.bytes));
      } else if(event.kind==="done") {if(!receivedHeaders)fail("missing response");ended=true;controller.close();init.signal?.removeEventListener("abort",abort);}
      else fail("invalid response event");
    } catch(error){finishError(error);void driver.invoke("neko_wiii_service_cancel",{requestId}).catch(()=>{});}
  });
  const input:ServiceNativeRequest={requestId,...target!,method:request.method,headers,bodyBase64:bytes?encode(bytes):null,timeoutMs};
  void driver.invoke("neko_wiii_service_fetch",{request:input,onEvent:channel}).catch(finishError).finally(()=>{if(ended)init.signal?.removeEventListener("abort",abort);});
  return pending;
}
