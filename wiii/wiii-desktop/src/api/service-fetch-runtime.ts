import { requireQaFixtureUrl } from "../lib/qa-profile";
import { fixedServiceFetch, serviceTarget, type ServiceNativeDriver } from "./local-service-http";

export async function wiiiServiceFetch(url:string,init:RequestInit={},timeoutMs=60000):Promise<Response> {
  requireQaFixtureUrl(url);
  if(typeof window!=="undefined"&&"__TAURI_INTERNALS__" in window&&serviceTarget(url)) {
    const {invoke,Channel}=await import("@tauri-apps/api/core");
    const driver:ServiceNativeDriver={invoke:(name,args)=>invoke(name,args),channel:onmessage=>{const channel=new Channel<Parameters<typeof onmessage>[0]>();channel.onmessage=onmessage;return channel;}};
    return fixedServiceFetch(driver,url,init,timeoutMs);
  }
  return fetch(url,init);
}
