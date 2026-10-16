import fs from 'node:fs';import path from 'node:path';import assert from 'node:assert/strict';import {createHash} from 'node:crypto';import {stripTypeScriptTypes} from 'node:module';import {pathToFileURL} from 'node:url';

export async function historyStorageCases(check, modules) {
 const file='E:/Sach/Sua/NekoBot/.build/wiii-frontend-ab6923b/wiii-desktop/src/lib/storage.ts',raw=fs.readFileSync(file,'utf8'),sourceSHA256=createHash('sha256').update(raw).digest('hex');
 const imports=raw.match(/await import\("@tauri-apps\/plugin-store"\)/g);assert.equal(imports.length,1);
 const transformed=stripTypeScriptTypes(raw,{mode:'transform'}).replace('await import("@tauri-apps/plugin-store")','await globalThis.__historyStoreFixture.importStore()');
 const target=path.join(modules,'actual-strict-storage.mjs');fs.writeFileSync(target,transformed);
 const stores=new Map();
 class Store {
  staged=new Map();durable=new Map();failures=0;saves=0;
  static async load(name){if(!stores.has(name))stores.set(name,new Store());return stores.get(name);}
  async get(key){return structuredClone(this.staged.get(key));}async set(key,value){this.staged.set(key,structuredClone(value));}async delete(key){this.staged.delete(key);}
  async save(){this.saves++;if(this.failures>0){this.failures--;throw Error('Synthetic plugin-store save failure');}this.durable=structuredClone(this.staged);}
 }
 const priorWindow=globalThis.window,priorFixture=globalThis.__historyStoreFixture;
 globalThis.window={__TAURI_INTERNALS__:{}};globalThis.__historyStoreFixture={importStore:async()=>({Store})};
 try {
  const actual=await import(pathToFileURL(target).href);
  await check('actual_strict_plugin_store_failed_active_save_restores_staging_and_durable_pointer',async()=>{
   await actual.saveStoreStrict('fixture-active.json','source',{version:2,generation:'old'});const store=await Store.load('fixture-active.json');store.failures=1;
   await assert.rejects(actual.saveStoreStrict('fixture-active.json','source',{version:2,generation:'candidate'}));
   assert.deepEqual(store.staged.get('source'),{version:2,generation:'old'});assert.deepEqual(store.durable.get('source'),{version:2,generation:'old'});
   await store.save();assert.equal((await actual.loadStoreStrict('fixture-active.json','source',null)).generation,'old');
  });
  await check('actual_strict_plugin_store_failed_first_write_removes_candidate_from_staging',async()=>{
   const store=await Store.load('fixture-new-active.json');store.failures=1;await assert.rejects(actual.saveStoreStrict('fixture-new-active.json','source',{generation:'candidate'}));
   assert.equal(store.staged.has('source'),false);assert.equal(store.durable.has('source'),false);await store.save();assert.equal(await actual.loadStoreStrict('fixture-new-active.json','source',null),null);
  });
  await check('actual_strict_plugin_store_compensation_failure_is_explicit',async()=>{
   await actual.saveStoreStrict('fixture-dual-failure.json','source',{generation:'old'});const store=await Store.load('fixture-dual-failure.json');store.failures=2;
   await assert.rejects(actual.saveStoreStrict('fixture-dual-failure.json','source',{generation:'candidate'}),error=>error instanceof AggregateError&&error.errors.length===2);
   assert.equal(store.staged.get('source').generation,'old');assert.equal(store.durable.get('source').generation,'old');
  });
 }finally{if(priorWindow===undefined)delete globalThis.window;else globalThis.window=priorWindow;if(priorFixture===undefined)delete globalThis.__historyStoreFixture;else globalThis.__historyStoreFixture=priorFixture;}
 return {path:file,sourceSHA256,scope:'Actual verified historical strict-store implementation, authored Store plugin interface fixture; logical save/compensation only, no native runtime or physical power-loss claim.'};
}
