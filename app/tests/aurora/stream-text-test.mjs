import {test} from 'node:test';
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
const {streamText,readableDraft,acceptMessage}=createRequire(import.meta.url)('../../native/internal/web/assets/app/ux.js');

test('all four languages and single-word answers appear without waiting for whitespace',()=>{
 for(const text of ['Chào!','Hello!','我正在查看这个问题。','我正在查看這個問題。','👩🏽‍💻','a\u0306\u0301']){
  assert.equal(readableDraft(text),true);assert.equal(streamText(text),text);
 }
});
test('received characters are not split by a synthetic typewriter or withheld at the end',()=>{
 for(const text of ['Mình đã làm xong','完成','a\u0306\u0301','👩🏽‍💻','**Kết quả**\n\n```js\nconst x = 1;\n```'])assert.equal(streamText(text),text);
 assert.equal(streamText('Chào \ud83d'),'Chào');
 assert.equal(streamText('Chào \ud83d\ude42'),'Chào 🙂');
});
test('protocol payloads and bare markdown markers never become an empty answer bubble',()=>{
 for(const text of ['', '  ', '**', '-', '```', '`', 'TOOL: {"secret":"not displayed"}'])assert.equal(readableDraft(text),false);
 assert.equal(streamText('Đã đọc xong.\nTOOL: {"name":"test"}'),'Đã đọc xong.');
});
test('follow-up and durable reply preserve every received segment and order',()=>{
 const text='我正在查看這個問題。',state={msgs:{bot:[{id:'run:u',taskId:'run',role:'user'},{id:'next:u',taskId:'next',role:'user',queued:true}]},drafts:{run:{text}}};
 assert.equal(streamText(state.drafts.run.text),text);
 acceptMessage(state,{id:'run',taskId:'run',agentId:'bot',role:'agent',text,beforeRequestId:'next'});
 assert.equal(state.drafts.run,undefined);
 assert.deepEqual(state.msgs.bot.map(x=>x.id),['run:u','run','next:u']);
 assert.equal(state.msgs.bot[1].text,text);
});
