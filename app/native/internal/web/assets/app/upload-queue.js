"use strict";
(() => {
  const MAX_BYTES = 1 << 20, MAX_ITEMS = 12, MAX_TOTAL = 8 << 20;
  function create(factory = globalThis.indexedDB) {
    let opening;
    function open() {
      if (!opening) opening = new Promise((resolve, reject) => {
        if (!factory) return reject(Error("upload_storage_unavailable"));
        const req = factory.open("aurora.image-uploads.v1", 1);
        let blocked = false;
        req.onupgradeneeded = () => req.result.createObjectStore("images", {keyPath:"id"});
        req.onerror = () => reject(req.error);
        req.onblocked = () => { blocked = true; reject(Error("upload_storage_blocked")); };
        req.onsuccess = () => {
          const db = req.result;
          if (blocked) { db.close(); return; }
          db.onversionchange = () => { db.close(); opening = null; };
          resolve(db);
        };
      }).catch(error => { opening = null; throw error; });
      return opening;
    }
    async function transaction(write, action) {
      const db = await open();
      return new Promise((resolve, reject) => {
        const tx = db.transaction("images", write ? "readwrite" : "readonly");
        let value, failure;
        tx.oncomplete = () => resolve(value);
        tx.onabort = () => reject(failure || tx.error || Error("upload_storage_aborted"));
        const fail = error => { failure = error; tx.abort(); };
        try { action(tx.objectStore("images"), result => { value = result; }, fail); }
        catch (error) { fail(error); }
      });
    }
    function add(scope, agent, name, data) {
      if (!scope || !agent || typeof name !== "string" || name.length > 200 || typeof data !== "string" || data.length % 4 !== 0 || !/^[A-Za-z0-9+/]+={0,2}$/.test(data) || data.length > Math.ceil(MAX_BYTES / 3) * 4) return Promise.reject(Error("bad_image"));
      const bytes = data.length * 3 / 4 - (data.endsWith("==") ? 2 : data.endsWith("=") ? 1 : 0);
      if (bytes > MAX_BYTES) return Promise.reject(Error("bad_image"));
      const item = {id:crypto.randomUUID(), scope, agent, name, data, bytes, created:Date.now(), state:"pending", error:"", path:""};
      return transaction(true, (store, done, fail) => {
        const req = store.getAll();
        req.onsuccess = () => {
          try {
          if (req.result.length >= MAX_ITEMS || req.result.reduce((n, row) => n + row.bytes, 0) + bytes > MAX_TOTAL) return fail(Error("upload_queue_full"));
          store.add(item); done(item);
          } catch (error) { fail(error); }
        };
      });
    }
    function list(scope, agent) {
      return transaction(false, (store, done) => {
        const req = store.getAll();
        req.onsuccess = () => done(req.result.filter(x => x.scope === scope && x.agent === agent).sort((a,b) => a.created - b.created));
      });
    }
    function change(item, patch) {
      return transaction(true, (store, done, fail) => {
        const req = store.get(item.id);
        req.onsuccess = () => {
          try {
          const current = req.result;
          if (!current || current.scope !== item.scope || current.agent !== item.agent) return fail(Error("upload_missing"));
          if (patch === null) { store.delete(item.id); done(); }
          else { const next = {...current, state:patch.state, error:patch.error || "", path:patch.path || ""}; store.put(next); done(next); }
          } catch (error) { fail(error); }
        };
      });
    }
    return {add, list, change, remove:item => change(item, null)};
  }
  globalThis.AURORA_UPLOAD_QUEUE = {create, MAX_BYTES, MAX_ITEMS, MAX_TOTAL};
})();
