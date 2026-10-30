import {createRequire} from 'node:module';
import {test} from 'node:test';
import assert from 'node:assert/strict';
const U=createRequire(import.meta.url)('../../native/internal/web/assets/app/ux.js');
const working={status:'working'},generic=U.phase(working,[]);
test('unconfirmed work uses neutral text for greetings and substantive requests',()=>{
  for(const text of ['Chào bạn','Cảm ơn nhé','So sánh hai báo cáo']){
    assert.deepEqual(U.phase(working,[{role:'user',text}]),{k:'think',t:'Đang xử lý',generic:true});
  }
});
test('generic text waits for the full 1500 ms and keeps its original phase',()=>{
  let time=0;const show=U.statusGate(()=>time);
  assert.equal(show('a/u1',generic).t,'');time=1499;assert.equal(show('a/u1',generic).t,'');
  time=1500;assert.deepEqual(show('a/u1',generic),generic);time=30000;assert.deepEqual(show('a/u1',generic),generic);
});
test('new turns chats and workspaces restart the delay',()=>{
  let time=0;const show=U.statusGate(()=>time);
  for(const key of ['workspace-a/chat-a/u1','workspace-a/chat-a/u2','workspace-a/chat-b/u1','workspace-b/chat-b/u1']){
    assert.equal(show(key,generic).t,'');time+=1500;assert.equal(show(key,generic).t,generic.t);
  }
});
test('specific work bypasses a pending generic phase and a later fallback waits again',()=>{
  let time=0;const show=U.statusGate(()=>time),specific=U.phase(working,[{role:'agent',tools:[{name:'web_search',status:'running'}]}]);
  show('a',generic);time=50;assert.equal(show('a',specific).t,'Đang tìm trên web');time=2000;assert.equal(show('a',generic).t,'');
});
test('hidden or completed feedback clears the previous delay',()=>{
  let time=0;const show=U.statusGate(()=>time);show('a',generic);time=2000;assert.equal(show('a',generic).t,generic.t);
  assert.equal(show('a',null),null);assert.equal(show('a',generic).t,'');assert.equal(U.phase({status:'idle'},[]),null);
});
test('approval and plan details remain specific',()=>{
  assert.equal(U.phase(working,[{approval:{state:'pending'}}]).k,'wait');
  assert.equal(U.phase({...working,snip:'Bước 2/4 · Kiểm tra'},[]).k,'step');
});
test('known tools retain their concrete activity and an unknown tool can supply a summary',()=>{
  assert.equal(U.phase(working,[{role:'agent',tools:[{name:'computer_run',status:'running',arg:'make report.xlsx'}]}]).t,'Đang làm bảng tính');
  assert.deepEqual(U.phase(working,[{role:'agent',tools:[{name:'custom',status:'running',summary:'Đọc số liệu'}]}]),{k:'run',t:'Đọc số liệu',generic:false});
  assert.equal(U.phase(working,[{role:'agent',tools:[{name:'custom',status:'running'}]}]).generic,true);
});
test('a completed tool does not fabricate a summarizing phase',()=>{
  const p=U.phase(working,[{role:'agent',tools:[{name:'web_search',status:'done'}]}]);
  assert.equal(p.t,'Đang xử lý');assert.equal(p.generic,true);
});
