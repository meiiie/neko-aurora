import {test} from 'node:test';
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
const require=createRequire(import.meta.url);
const {acceptMessage}=require('../../native/internal/web/assets/app/ux.js');

test('an interrupted draft becomes a separate message before its interjection',()=>{
 const state={msgs:{bot:[{id:'run:u'},{id:'next:u'}]},drafts:{run:{text:'First visible words'},other:{text:'Unrelated'}}};
 const partial={id:'run:partial1',taskId:'run',agentId:'bot',interrupted:true,beforeRequestId:'next',text:'First visible words'};
 acceptMessage(state,partial);
 assert.deepEqual(state.msgs.bot.map(x=>x.id),['run:u','run:partial1','next:u']);
 assert.equal(state.drafts.run,undefined);
 assert.equal(state.drafts.other.text,'Unrelated');
 state.drafts.run={text:'New answer still streaming'};
 acceptMessage(state,{...partial,sources:[{url:'https://example.com'}]});
 assert.equal(state.drafts.run.text,'New answer still streaming');
 assert.equal(state.msgs.bot.filter(x=>x.interrupted).length,1);
 acceptMessage(state,{id:'run',taskId:'run',agentId:'bot',text:'Final answer'});
 assert.equal(state.drafts.run,undefined);
 assert.equal(state.msgs.bot[1].text,'First visible words');
 assert.equal(state.msgs.bot.at(-1).text,'Final answer');
 const reload=JSON.parse(JSON.stringify(state.msgs.bot));
 assert.deepEqual(reload.map(x=>x.id),['run:u','run:partial1','next:u','run']);
});

test('late delivery and a second interjection do not overwrite an earlier segment',()=>{
 const state={msgs:{bot:[{id:'run:u'},{id:'run',text:'Final'}]},drafts:{}};
 acceptMessage(state,{id:'run:partial1',taskId:'run',agentId:'bot',interrupted:true,beforeRequestId:'next',text:'First'});
 acceptMessage(state,{id:'run:partial2',taskId:'run',agentId:'bot',interrupted:true,beforeRequestId:'last',text:'Second'});
 assert.deepEqual(state.msgs.bot.map(x=>x.id),['run:u','run:partial1','run:partial2','run']);
});


test('completed reply stays before its queued follow-up on updates and delayed events',()=>{
 const state={msgs:{bot:[{id:'run:u',taskId:'run',role:'user'},{id:'next:u',taskId:'next',role:'user',queued:true}]},drafts:{run:{text:'Original joke'}}};
 const final={id:'run',taskId:'run',agentId:'bot',role:'agent',text:'Original joke, complete.',beforeRequestId:'next'};
 acceptMessage(state,final);
 assert.equal(state.drafts.run,undefined);
 assert.deepEqual(state.msgs.bot.map(x=>x.id),['run:u','run','next:u']);
 acceptMessage(state,{...final,sources:[]});
 assert.deepEqual(state.msgs.bot.map(x=>x.id),['run:u','run','next:u']);
 const late={msgs:{bot:[{id:'run:u',taskId:'run',role:'user'}]},drafts:{}};
 acceptMessage(late,final);
 acceptMessage(late,{id:'next:u',taskId:'next',agentId:'bot',role:'user',text:'Email please'});
 assert.deepEqual(late.msgs.bot.map(x=>x.id),['run:u','run','next:u']);
});

test('tool metadata and an unchanged prior step never erase an active draft',()=>{
 const state={msgs:{bot:[]},drafts:{run:{text:'Still answering'}}};
 acceptMessage(state,{id:'run',taskId:'run',agentId:'bot',text:'',tools:[{status:'done'}]});
 assert.equal(state.drafts.run.text,'Still answering');
 acceptMessage(state,{id:'run',taskId:'run',agentId:'bot',text:'First answer'});
 state.drafts.run={text:'Continuing this task'};
 acceptMessage(state,{id:'run',taskId:'run',agentId:'bot',text:'First answer',sources:[]});
 assert.equal(state.drafts.run.text,'Continuing this task');
});
