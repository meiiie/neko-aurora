import {test} from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import vm from 'node:vm';

const context=vm.createContext({});
vm.runInContext(readFileSync(new URL('../../native/internal/web/assets/app/outbox.js',import.meta.url),'utf8'),context);
const {create,MAX_AGE}=context.AURORA_OUTBOX;
const storage=()=>{const values=new Map();return {get length(){return values.size},key:i=>[...values.keys()][i],getItem:k=>values.get(k)??null,setItem:(k,v)=>values.set(k,v),removeItem:k=>values.delete(k)}};
const item=(n='fixture-1',scope='owner-a')=>({n,workspaceId:scope,agentId:'bot-1',text:'Tin tiếng Việt',t:1000});

test('reload preserves independent messages written by two tabs',()=>{
 const disk=storage(),a=create(disk,()=>1000),b=create(disk,()=>1000);
 a.put(item());b.put(item('fixture-2'));
 assert.equal(create(disk,()=>1001).list('owner-a').length,2);
 a.remove('owner-a','fixture-1');assert.equal(b.list('owner-a')[0].n,'fixture-2');
});
test('account scope never exposes or removes another account queue',()=>{
 const queue=create(storage(),()=>1000);queue.put(item());queue.put(item('fixture-b','owner-b'));
 assert.equal(queue.list('owner-b')[0].n,'fixture-b');queue.clear('owner-b');assert.equal(queue.list('owner-a').length,1);
 assert.equal(queue.list('').length,0);
});
test('expired and future messages remain readable but cannot auto-send',()=>{
 const disk=storage();create(disk,()=>1000).put(item());
 assert.ok(create(disk,()=>1000+MAX_AGE+1).list('owner-a')[0].error);
 assert.equal(create(disk,()=>1000+MAX_AGE+1).list('owner-a')[0].text,item().text);
});
test('full or failed storage does not acknowledge enqueue',()=>{
 const disk=storage(),queue=create(disk,()=>1000);for(let i=0;i<128;i++)queue.put(item('n'+i));
 assert.throws(()=>queue.put(item('overflow')),/outbox_full/);
 disk.setItem=()=>{throw Error('QuotaExceededError')};assert.throws(()=>queue.put(item('n0')),/QuotaExceededError/);
});
