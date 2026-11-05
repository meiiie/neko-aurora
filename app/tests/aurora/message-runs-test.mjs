import {test} from 'node:test';
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
const {sameMessageRun}=createRequire(import.meta.url)('../../native/internal/web/assets/app/ux.js');
const first={role:'agent',agentId:'group',from:'meiiie',text:'Đã đọc bản nháp.',t:'2026-11-01T10:00:00Z'};
const next={...first,text:'Mình đang kiểm tra phần còn lại.',t:'2026-11-01T10:00:30Z'};
test('adjacent updates group only for the same author and a short valid time gap',()=>{
 assert.equal(sameMessageRun(first,next),true);
 for(const change of [{from:'davin'},{role:'user'},{t:'invalid'},{t:'2026-11-01T09:59:59Z'},{t:'2026-11-01T10:05:00Z'}])assert.equal(sameMessageRun(first,{...next,...change}),false);
 assert.equal(sameMessageRun(null,next),false);
});
test('quotes, interruptions, queues and actionable cards retain their visual boundary',()=>{
 for(const key of ['quote','interrupted','queued','approval','plan','form','questions','choices','secret','recovery']){
  assert.equal(sameMessageRun({...first,[key]:true},next),false,key);
  assert.equal(sameMessageRun(first,{...next,[key]:true}),false,key);
 }
 assert.equal(sameMessageRun(first,{...next,text:''}),false);
 assert.equal(sameMessageRun({...first,role:'system'},{...next,role:'system'}),false);
});
