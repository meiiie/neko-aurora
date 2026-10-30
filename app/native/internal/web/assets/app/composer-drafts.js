"use strict";
(() => {
  const prefix = "aurora.composer.v1.";
  const empty = () => ({text:"", ref:null, reply:null});
  function copy(value) {
    if (!value || typeof value.text !== "string") return empty();
    const ref = value.ref;
    const reply=value.reply;
    return {text:value.text, ref:ref && typeof ref.id === "string" && ref.id && typeof ref.name === "string" ? {id:ref.id, name:ref.name} : null, reply:reply&&typeof reply.id==="string"&&reply.id&&typeof reply.author==="string"&&typeof reply.text==="string"?{id:reply.id,author:reply.author,text:reply.text}:null};
  }
  function create(storage) {
    const memory = new Map(), pending = new Set();
    const key = (scope, agent) => prefix + encodeURIComponent(JSON.stringify([scope, agent]));
    function get(scope, agent) {
      if (!scope || !agent) return empty();
      const k = key(scope, agent);
      if (!pending.has(k)) {
        try { memory.set(k, copy(JSON.parse(storage.getItem(k)))); } catch {}
      }
      return copy(memory.get(k));
    }
    function put(scope, agent, value) {
      if (!scope || !agent) return false;
      const k = key(scope, agent), draft = copy(value);
      memory.set(k, draft);
      pending.add(k);
      try {
        if (!draft.text && !draft.ref && !draft.reply) {
          storage.removeItem(k);
          if (storage.getItem(k) !== null) throw Error("draft_not_removed");
        } else {
          const text = JSON.stringify(draft);
          storage.setItem(k, text);
          if (storage.getItem(k) !== text) throw Error("draft_not_saved");
        }
        pending.delete(k);
        return true;
      } catch { return false; }
    }
    return {get, put};
  }
  globalThis.AURORA_COMPOSER_DRAFTS = {create};
})();
