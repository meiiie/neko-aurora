import {test} from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import vm from 'node:vm';

const context=vm.createContext({});
vm.runInContext(readFileSync(new URL('../../native/internal/web/assets/app/composer-drafts.js',import.meta.url),'utf8'),context);
const {create}=context.AURORA_COMPOSER_DRAFTS;
const storage=()=>{const values=new Map();return {values,getItem:k=>values.get(k)??null,setItem:(k,v)=>values.set(k,v),removeItem:k=>values.delete(k)}};

test('draft text and reference survive reload separately for each chat and workspace',()=>{
 const disk=storage(),s=create(disk);
 s.put('owner-a','bot-1',{text:' Bản nháp\n[Ảnh: /home/a.jpg]',ref:{id:'bot-2',name:'Nghiên cứu'}});
 s.put('owner-a','bot-2',{text:'Nháp khác'});s.put('owner-b','bot-1',{text:'Tài khoản khác'});
 const reloaded=create(disk);
 assert.equal(reloaded.get('owner-a','bot-1').text,' Bản nháp\n[Ảnh: /home/a.jpg]');
 assert.equal(reloaded.get('owner-a','bot-1').ref.name,'Nghiên cứu');
 assert.equal(reloaded.get('owner-a','bot-2').text,'Nháp khác');
 assert.equal(reloaded.get('owner-b','bot-1').text,'Tài khoản khác');
 s.put('owner-a','bot-1',{text:''});assert.equal(reloaded.get('owner-a','bot-1').text,'');
 assert.equal(reloaded.get('owner-b','bot-1').text,'Tài khoản khác');
});
test('storage failure keeps edited draft in memory without claiming durable save',()=>{
 const disk=storage(),s=create(disk);s.put('a','b',{text:'Old'});
 const write=disk.setItem;disk.setItem=()=>{throw Error('QuotaExceededError')};
 assert.equal(s.put('a','b',{text:'New',ref:{id:'c',name:'C'}}),false);
 assert.equal(s.get('a','b').text,'New');assert.equal(create(disk).get('a','b').text,'Old');
 disk.setItem=write;assert.equal(s.put('a','b',s.get('a','b')),true);
 assert.equal(create(disk).get('a','b').text,'New');
});
test('malformed data and mutable callers cannot poison another draft',()=>{
 const disk=storage(),s=create(disk);const draft={text:'Keep',ref:{id:'c',name:'C'}};
 s.put('a.b','c',draft);s.put('a','b.c',{text:'Other'});draft.ref.name='changed';
 const result=s.get('a.b','c');result.ref.name='mutated';assert.equal(s.get('a.b','c').ref.name,'C');
 assert.equal(s.get('a','b.c').text,'Other');
 const [key]=disk.values.keys();disk.values.set(key,'broken JSON');
 assert.equal(create(disk).get('a.b','c').text,'');assert.equal(create(disk).get('a','b.c').text,'Other');
 assert.equal(s.put('','c',{text:'No scope'}),false);
});

test('a quote-only draft survives reload and does not share mutable source data',()=>{
 const disk=storage(),s=create(disk),reply={id:'message-1',author:'Bot',text:'Original'};
 s.put('owner','chat',{text:'',reply});reply.text='changed';
 const loaded=create(disk).get('owner','chat');assert.equal(loaded.reply.text,'Original');
 loaded.reply.text='mutated';assert.equal(s.get('owner','chat').reply.text,'Original');
 assert.equal(s.get('owner','other').reply,null);
 s.put('owner','chat',{text:'',reply:null});assert.equal(disk.values.size,0);
});
