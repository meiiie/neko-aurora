"use strict";
(() => {
  const MAX_AGE = 7 * 24 * 60 * 60 * 1000;
  const prefix = scope => "aurora.outbox.v1." + encodeURIComponent(scope) + ".";
  function nonce() {
    const bytes = new Uint8Array(16);
    globalThis.crypto.getRandomValues(bytes);
    return "c" + Array.from(bytes, b => b.toString(16).padStart(2,"0")).join("");
  }
  function create(storage, now = Date.now) {
    function list(scope) {
      if (!scope) return [];
      const items = [], start = prefix(scope);
      for (let i = 0; i < storage.length; i++) {
        const key = storage.key(i);
        if (!key?.startsWith(start)) continue;
        let value;
        try { value = JSON.parse(storage.getItem(key)); } catch { continue; }
        if (!value || value.workspaceId !== scope || typeof value.n !== "string" || key !== start + value.n ||
            typeof value.agentId !== "string" || typeof value.text !== "string" || value.text.length > 32000 || !Number.isFinite(value.t)) continue;
        if (now() - value.t > MAX_AGE || value.t > now() + 60000) value.error = "Tin đã cũ. Sao chép nội dung để xem lại trước khi gửi mới.";
        items.push(value);
      }
      return items.sort((a,b) => a.t-b.t);
    }
    function put(value) {
      if (!value.workspaceId) throw new Error("workspace_required");
      const key = prefix(value.workspaceId) + value.n;
      if (!storage.getItem(key) && list(value.workspaceId).length >= 128) throw new Error("outbox_full");
      const text = JSON.stringify(value);
      storage.setItem(key, text);
      if (storage.getItem(key) !== text) throw new Error("outbox_not_saved");
    }
    function remove(scope, nonce) { storage.removeItem(prefix(scope) + nonce); }
    function clear(scope) { for (const item of list(scope)) remove(scope, item.n); }
    return {list,put,remove,clear};
  }
  globalThis.AURORA_OUTBOX = {create,nonce,MAX_AGE};
})();
