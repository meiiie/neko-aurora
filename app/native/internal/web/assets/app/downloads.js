"use strict";
(() => {
  const pending=new Map(),t=s=>window.__I18N?.t(s)||s;
  async function save({href,workspaceId,active,notice=()=>{}}) {
    if(!active()||!workspaceId)return false;
    let url,bot,name;
    try {
      url=new URL(href,location.href);const match=url.pathname.match(/^\/app\/api\/agents\/([A-Za-z0-9_-]+)\/computer\/download$/);
      if(url.origin!==location.origin||!match||url.searchParams.getAll("path").length!==1)throw Error("download_url");
      bot=match[1];name=url.searchParams.get("path").split(/[\\/]/).pop();if(!name)throw Error("download_name");
      url.searchParams.set("workspaceId",workspaceId);
    } catch {notice(t("Không tải được file. Kiểm tra lại liên kết."));return false}
    for(const [key,job] of pending)if(!job.active()){job.controller.abort();pending.delete(key)}
    const key=workspaceId+"|"+url.href;if(pending.has(key))return false;
    if(pending.size>=2){notice(t("Đang chuẩn bị hai file. Đợi một lượt hoàn tất rồi thử lại."));return false}
    const controller=new AbortController(),job={controller,active};pending.set(key,job);
    const timer=setTimeout(()=>controller.abort(),120000);let response;
    notice(t("Đang chuẩn bị file…"));
    try {
      response=await fetch(url.href,{credentials:"same-origin",cache:"no-store",redirect:"error",signal:controller.signal});
      if(!active())return false;
      if(!response.ok)throw Object.assign(Error("download_http"),{status:response.status});
      const rawSize=response.headers.get("Content-Length"),size=Number(rawSize);
      if(response.headers.get("X-Aurora-Workspace")!==workspaceId||response.headers.get("X-Aurora-Bot")!==bot||response.headers.get("Content-Type")?.split(";")[0]!=="application/octet-stream"||!/^\d+$/.test(rawSize||"")||!Number.isSafeInteger(size))throw Error("download_response");
      const blob=await response.blob();if(!active())return false;if(blob.size!==size)throw Error("download_incomplete");
      const local=URL.createObjectURL(blob),link=document.createElement("a");link.href=local;link.download=name;link.hidden=true;document.body.append(link);
      try{link.click()}finally{link.remove();setTimeout(()=>URL.revokeObjectURL(local),1000)}
      notice(t("Đã chuyển file cho trình duyệt."));return true;
    } catch(e) {
      if(active())notice(t(e.status===409?"File hoặc phiên làm việc đã thay đổi. Tải lại danh sách rồi thử lại.":e.status===413?"File vượt giới hạn dung lượng tải xuống.":e.status===503?"Máy chủ đang bận chuẩn bị file. Hãy thử lại sau.":"Không tải được file. Nội dung có thể chưa đầy đủ; hãy thử lại."));return false;
    } finally {clearTimeout(timer);if(pending.get(key)===job)pending.delete(key);if(response?.body&&!response.bodyUsed)await response.body.cancel().catch(()=>{})}
  }
  window.AURORA_DOWNLOADS={save};
})();
