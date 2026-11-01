import {test} from 'node:test';
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
const {conversationTarget}=createRequire(import.meta.url)('../../native/internal/web/assets/app/ux.js');
const agents=[
 {id:'meiiie',t:'2026-11-01T08:00:00Z'},
 {id:'team',kind:'group',t:'2026-11-01T09:00:00Z'},
 {id:'hidden',hidden:true,t:'2026-11-01T10:00:00Z'},
];
test('a valid conversation link takes precedence over the remembered conversation',()=>{
 assert.equal(conversationTarget(agents,'meiiie','team'),'meiiie');
 assert.equal(conversationTarget(agents,'hidden','team'),'hidden');
});
test('reopening prefers the last conversation over a more recent update elsewhere',()=>{
 assert.equal(conversationTarget(agents,'','meiiie'),'meiiie');
 assert.equal(conversationTarget(agents,'archived','meiiie'),'meiiie');
});
test('stale, hidden and cross-workspace selections fall back to the most recent visible conversation',()=>{
 for(const remembered of ['archived','hidden','other-workspace',''])assert.equal(conversationTarget(agents,'missing',remembered),'team');
});
test('empty lists and invalid times are deterministic without un-hiding conversations',()=>{
 assert.equal(conversationTarget([],'missing','missing'),null);
 assert.equal(conversationTarget([agents[2]],'','hidden'),null);
 const fresh=[{id:'one',t:'invalid'},{id:'two'},{id:'three',t:0}];
 assert.equal(conversationTarget(fresh,'',''),'one');
 assert.equal(conversationTarget([...fresh,{id:'four',t:1000}],'',''),'four');
 assert.deepEqual(fresh.map(a=>a.id),['one','two','three']);
});
