const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const path = require('node:path');
const source = fs.readFileSync(path.resolve(__dirname, '../../native/internal/web/assets/app/aurora/avatar-motion.js'), 'utf8');

function boot({ stored = {}, reduce = false, hidden = false, failStorage = false } = {}) {
  const events = {}, documentEvents = {}, observed = new Set();
  let intersect, mutate, mediaChange, fineChange;
  let now=0, next=0; const frames=new Map(), timers=new Map();
  const media = { matches: reduce, addEventListener: (_, fn) => { mediaChange = fn; } };
  const fine = {matches:true,addEventListener:(_,fn)=>{fineChange=fn}};
  const context = {
    performance:{now:()=>now},
    requestAnimationFrame:fn=>{frames.set(++next,fn);return next},cancelAnimationFrame:id=>frames.delete(id),
    setTimeout:(fn,delay)=>{timers.set(++next,{fn,at:now+delay});return next},clearTimeout:id=>timers.delete(id),
    Date, Event, console,
    document: { hidden, documentElement: {}, addEventListener: (type, fn) => { documentEvents[type] = fn; } },
    localStorage: { getItem: key => stored[key], setItem: (key, value) => { if (failStorage) throw Error('unavailable'); stored[key] = value; } },
    matchMedia: query => query.includes("reduced") ? media : fine,
    IntersectionObserver: class {
      constructor(fn) { intersect = fn; }
      observe(node) { observed.add(node); }
      unobserve(node) { observed.delete(node); }
    },
    MutationObserver: class { constructor(fn) { mutate = fn; } observe() {} },
    addEventListener: (type, fn) => { events[type] = fn; },
    dispatchEvent() {},
  };
  context.window = context;
  vm.runInNewContext(source, context);
  const node = () => {const properties={};return { dataset: {}, properties, isConnected: true, getBoundingClientRect:()=>({x:0,y:0,width:48,height:48}), style: { setProperty:(k,v)=>{properties[k]=v} } }};
  const step=(ms=16)=>{now+=ms;const pending=[...frames.values()];frames.clear();pending.forEach(fn=>fn(now));for(const [id,t] of [...timers])if(t.at<=now){timers.delete(id);t.fn()}};
  const move=(nodes,x=72,y=24,type="mouse")=>{const owner={contains:n=>nodes.includes(n)};documentEvents.pointermove({target:{closest:()=>owner},clientX:x,clientY:y,pointerType:type,isPrimary:true})};
  const visible = (target, value) => intersect([{ target, isIntersecting: value }]);
  return { api: context.AURORA_AVATARS, context, node, visible, observed, mutate, media, mediaChange: () => mediaChange(), events, documentEvents, stored, frames, timers, step, move, fine, fineChange:()=>fineChange() };
}

test('motion only runs when visible and returns to rest offscreen or with a hidden document', () => {
  const b = boot(), node = b.node();
  b.api.attach(node, 'bot-1', 'working');
  assert.equal(node.dataset.motion, 'off');
  b.visible(node, true); assert.equal(node.dataset.motion, 'on');
  b.context.document.hidden = true; b.documentEvents.visibilitychange(); assert.equal(node.dataset.motion, 'off');
  b.context.document.hidden = false; b.documentEvents.visibilitychange(); assert.equal(node.dataset.motion, 'on');
  b.visible(node, false); assert.equal(node.dataset.motion, 'off');
});

test('reduced motion takes priority over automatic animation, including live OS preference changes', () => {
  const b = boot({ reduce: true }), node = b.node();
  b.api.attach(node, 'bot-1', 'working'); b.visible(node, true); assert.equal(node.dataset.motion, 'off');
  b.api.setMode('auto'); assert.equal(node.dataset.motion, 'off');
  b.media.matches = false; b.mediaChange(); assert.equal(node.dataset.motion, 'on');
  b.media.matches = true; b.mediaChange(); assert.equal(node.dataset.motion, 'off');
});

test('off preference survives reload, syncs across windows, and works when storage fails', () => {
  const b = boot(), node = b.node(); b.api.attach(node, 'bot-1', 'idle'); b.visible(node, true);
  assert.equal(b.api.setMode('off'), true); assert.equal(node.dataset.motion, 'off');
  assert.equal(boot({ stored: b.stored }).api.mode(), 'off');
  b.stored['aurora.avatar-motion.v1'] = 'auto'; b.events.storage({ key: 'aurora.avatar-motion.v1' });
  assert.equal(node.dataset.motion, 'on');
  b.api.setMode('off'); delete b.stored['aurora.avatar-motion.v1']; b.events.storage({ key: null });
  assert.equal(node.dataset.motion, 'on');
  assert.equal(b.api.setMode('always'), false); assert.equal(b.api.mode(), 'auto');
  const blocked = boot({ failStorage: true }); assert.equal(blocked.api.setMode('off'), false); assert.equal(blocked.api.mode(), 'off');
});

test('replaced avatars release their observer and stale intersection callbacks cannot revive them', () => {
  const b = boot(), old = b.node(), next = b.node();
  b.api.attach(old, 'bot-1', 'working'); b.visible(old, true);
  old.isConnected = false; b.api.attach(next, 'bot-1', 'idle'); b.mutate();
  assert.equal(b.observed.has(old), false); assert.equal(b.observed.has(next), true);
  b.visible(old, false); b.visible(next, true); b.api.setMode('off');
  assert.equal(next.dataset.motion, 'off');
  assert.equal(b.observed.size, 1);
});

test('avatar state follows supplied status and never upgrades unknown status to working', () => {
  const b = boot();
  for (const state of ['working', 'waiting', 'error', 'idle', undefined, 'unknown']) {
    const node = b.node(); b.api.attach(node, 'bot-1', state);
    assert.equal(node.dataset.avatarState, ['working', 'waiting', 'error'].includes(state) ? state : 'idle');
  }
});


test('same surface retains the avatar, gaze and blink deadline across status updates',()=>{
 const b=boot();let creates=0;const create=()=>{creates++;return b.node()};
 const n=b.api.reuse('header/bot','shape-1',create,'bot','idle');b.visible(n,true);b.move([n]);for(let i=0;i<80;i++)b.step();
 const x=n.properties['--gaze-x'];assert.equal(x,'5.000px');
 const deadline=[...b.timers.values()][0].at;
 assert.equal(b.api.reuse('header/bot','shape-1',create,'bot','working'),n);
 assert.equal(creates,1);assert.equal(n.properties['--gaze-x'],x);assert.equal([...b.timers.values()][0].at,deadline);
 assert.notEqual(b.api.reuse('list/bot','shape-1',create,'bot','idle'),n);
 assert.notEqual(b.api.reuse('header/bot','shape-2',create,'bot','idle'),n);
});

test('pointer gaze is bounded, local, settles with no RAF, and returns after leaving',()=>{
 const b=boot(),a=b.node(),other=b.node();b.api.attach(a,'a','idle');b.api.attach(other,'b','idle');b.visible(a,true);b.visible(other,true);
 b.move([a],10000,-10000);for(let i=0;i<80;i++)b.step();
 assert.equal(a.properties['--gaze-x'],'5.000px');assert.equal(a.properties['--gaze-y'],'-3.000px');
 assert.equal(other.properties['--gaze-x'],'0px');assert.equal(b.frames.size,0);
 b.documentEvents.pointerleave();for(let i=0;i<100;i++)b.step();
 assert.equal(a.properties['--gaze-x'],'0.000px');assert.equal(b.frames.size,0);
});

test('approval state, touch and reduced motion cannot be overridden by gaze',()=>{
 const b=boot(),n=b.node();b.api.attach(n,'bot','waiting');b.visible(n,true);b.move([n]);for(let i=0;i<10;i++)b.step();assert.equal(n.properties['--gaze-x'],'0.000px');
 b.api.setState(n,'idle');b.move([n],72,24,'touch');b.step();assert.equal(b.frames.size,0);
 b.move([n]);b.step();b.media.matches=true;b.mediaChange();assert.equal(n.properties['--gaze-x'],'0px');assert.equal(b.frames.size,0);assert.equal(b.timers.size,0);
});

test('blink scheduler varies by identity and stops when all avatars are hidden or removed',()=>{
 const deadline=id=>{const b=boot(),n=b.node();b.api.attach(n,id,'idle');b.visible(n,true);return [...b.timers.values()][0].at};
 assert.equal(deadline('bot-1'),deadline('bot-1'));assert.ok(Math.abs(deadline('bot-1')-deadline('bot-2'))>100);
 const b=boot(),a=b.node(),other=b.node();b.api.attach(a,'a','idle');b.api.attach(other,'b','idle');b.visible(a,true);b.visible(other,true);
 assert.equal(b.timers.size,1);b.step(11000);assert.equal(a.dataset.blink,'on');b.step(180);assert.equal(a.dataset.blink,'off');
 b.visible(a,false);b.visible(other,false);assert.equal(b.timers.size,0);
 b.visible(a,true);assert.equal(b.timers.size,1);a.isConnected=false;b.mutate();assert.equal(b.timers.size,0);assert.equal(b.observed.has(a),false);
});

test('idle gestures have quiet gaps and only one ambient performance runs at a time',()=>{
 const b=boot(),nodes=Array.from({length:12},(_,i)=>{const n=b.node();b.api.attach(n,'coworker-'+i,'idle');b.visible(n,true);return n});
 let performances=0,rests=0;
 for(let i=0;i<1200;i++){
  b.step(50);
  const performing=nodes.filter(n=>['glance','settle'].includes(n.dataset.gesture));
  assert.ok(performing.length<=1,'sidebar must not perform in chorus');
  if(performing.length)performances++;else rests++;
  assert.equal(b.frames.size,0,'idle motion uses bounded CSS clips, no continuous JS RAF');
  assert.equal(b.timers.size,1);
 }
 assert.ok(performances>0);assert.ok(rests>performances);
});

test('hover greets once, turns the whole face and returns to rest after a scroll',()=>{
 const b=boot(),n=b.node();b.api.attach(n,'bot','idle');b.visible(n,true);b.move([n]);
 assert.equal(n.dataset.gesture,'greet');b.step(600);assert.equal(n.dataset.gesture,'rest');
 b.move([n]);assert.equal(n.dataset.gesture,'rest','repeated input cannot restart the greeting');
 for(let i=0;i<80;i++)b.step();assert.equal(n.properties['--head-turn'],'2.750deg');
 b.documentEvents.scroll();for(let i=0;i<100;i++)b.step();
 assert.equal(n.properties['--head-turn'],'0.000deg');assert.equal(b.frames.size,0);
});

test('keyboard focus greets, while an actual busy or approval state takes priority',()=>{
 const b=boot(),n=b.node();b.api.attach(n,'bot','idle');b.visible(n,true);
 const event={target:{closest:()=>({contains:x=>x===n,matches:()=>true})}};
 b.documentEvents.focusin(event);assert.equal(n.dataset.gesture,'greet');
 for(const state of ['working','waiting','error']){
  b.api.setState(n,state);b.documentEvents.focusin(event);b.move([n]);
  assert.equal(n.dataset.gesture,'rest');
  for(let i=0;i<40;i++)b.step(1000);
  assert.equal(n.dataset.gesture,'rest');assert.equal(n.dataset.avatarState,state);
 }
});

test('reduced motion cancels gestures and turning immediately without replaying hidden activity',()=>{
 const b=boot(),n=b.node();b.api.attach(n,'bot','idle');b.visible(n,true);b.move([n]);b.step();
 b.media.matches=true;b.mediaChange();
 assert.equal(n.dataset.gesture,'rest');assert.equal(n.properties['--head-turn'],'0deg');
 assert.equal(b.frames.size,0);assert.equal(b.timers.size,0);
 b.step(90000);b.media.matches=false;b.mediaChange();b.step(100);
 assert.equal(n.dataset.gesture,'rest');assert.equal(b.frames.size,0);
});
