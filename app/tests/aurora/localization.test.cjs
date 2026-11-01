const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const assets = path.resolve(__dirname, '../../native/internal/web/assets/app');

function boot({ stored = {}, languages = ['vi-VN'], failStorage = false } = {}) {
  const events = new Map(), calls = [];
  const context = {
    Intl, Date, console, setTimeout, clearTimeout,
    navigator: { languages, language: languages[0] },
    document: { documentElement: {}, body: null, addEventListener() {} },
    localStorage: { getItem: key => stored[key], setItem: (key, value) => { if (failStorage) throw Error('full'); stored[key] = value; } },
    CustomEvent: class { constructor(type, options) { this.type = type; this.detail = options.detail; } },
    addEventListener: (name, fn) => events.set(name, fn),
    dispatchEvent: event => calls.push(event),
    location: { reload() { throw Error('Language change must not reload the page'); } },
  };
  context.window = context;
  vm.createContext(context);
  for (const file of ['aurora/vendor/i18next.min.js', 'aurora/locales.js', 'i18n.js']) vm.runInContext(fs.readFileSync(path.join(assets, file), 'utf8'), context, { filename: file });
  return { api: context.__I18N, context, stored, calls, events };
}

test('ordered device language negotiation, scripts and legacy preferences', () => {
  for (const [languages, expected] of [
    [['fr-FR', 'vi-VN', 'en'], 'vi'], [['zh-TW'], 'zh-Hant'], [['zh-HK'], 'zh-Hant'],
    [['zh-Hans-HK'], 'zh-Hans'], [['zh-Hant-CN'], 'zh-Hant'], [['zh-SG'], 'zh-Hans'],
    [['zh-Latn', 'vi'], 'vi'],
    [['zh'], 'zh-Hans'], [['invalid_tag', 'en-GB'], 'en'], [['fr'], 'en'],
  ]) assert.equal(boot({ languages }).api.lang, expected);
  assert.equal(boot({ stored: { 'neko-aurora-lang': 'vi' }, languages: ['en-US'] }).api.lang, 'vi');
  assert.equal(boot({ stored: { 'neko-aurora-lang': 'unknown' }, languages: ['zh-TW'] }).api.lang, 'zh-Hant');
});

test('all four settings, core AI and legacy messages use the selected catalogue', () => {
  const { api, context, calls } = boot();
  const expected = { vi: ['Ngôn ngữ giao diện', 'Chọn mô hình'], en: ['Interface language', 'Choose a model'], 'zh-Hans': ['界面语言', '选择模型'], 'zh-Hant': ['介面語言', '選擇模型'] };
  for (const [language, [title, model]] of Object.entries(expected)) {
    assert.equal(api.set(language), true);
    assert.equal(api.message('settings.locale.language'), title);
    assert.equal(api.t('Chọn model'), model);
    assert.equal(context.document.documentElement.lang, language);
  }
  assert.equal(calls.length, 4);
  assert.equal(api.set('de'), false);
  assert.equal(api.lang, 'zh-Hant');
});

test('plural messages, literal interpolation and whitespace remain correct', () => {
  const { api } = boot({ stored: { 'neko-aurora-lang': 'en' } });
  assert.equal(api.t('1 bot'), '1 bot');
  assert.equal(api.t('2 bot'), '2 bots');
  assert.equal(api.t(' Mỗi 1 phút '), ' Every 1 minute ');
  assert.equal(api.t('Mỗi 2 phút'), 'Every 2 minutes');
  assert.equal(api.t('Hỏi Phiên bản của tôi'), 'Ask Phiên bản của tôi');
  const name = '<img src=x onerror=alert(1)> $& {{count}}';
  assert.equal(api.message('chat.ask', { name }), 'Ask ' + name);
  assert.equal(api.t('unknown literal'), 'unknown literal');
});

test('formatting region is independent of UI language; auto respects device region', () => {
  const { api } = boot({ languages: ['en-GB'] });
  const stamp = new Date(2026, 10, 23, 14, 5);
  assert.equal(api.date(stamp, { dateStyle: 'short' }), new Intl.DateTimeFormat('en-GB', { dateStyle: 'short' }).format(stamp));
  api.set('vi');
  assert.equal(api.number(12345.67), '12.345,67');
  api.setRegion('en-US');
  assert.equal(api.lang, 'vi');
  assert.equal(api.number(12345.67), '12,345.67');
  assert.equal(api.message('count.bots', { count: 1234 }), '1,234 bot');
  api.set('zh-Hant');
  assert.equal(api.speechLocale, 'zh-TW');
  api.setRegion('auto');
  assert.equal(api.locale, 'zh-TW');
  assert.equal(api.date('not a date'), '');
  assert.equal(api.setRegion('bad'), false);
});

test('storage failures keep current selection in memory without reloading', () => {
  const { api, calls } = boot({ failStorage: true });
  assert.equal(api.set('zh-Hans'), false);
  assert.equal(api.lang, 'zh-Hans');
  assert.equal(api.pref(), 'zh-Hans');
  assert.equal(calls[0].detail.persisted, false);
});

test('cross-tab storage and device changes update only valid preferences', () => {
  const { api, events, stored, context } = boot();
  stored['neko-aurora-lang'] = 'en';
  events.get('storage')({ key: 'unrelated' });
  assert.equal(api.lang, 'vi');
  events.get('storage')({ key: 'neko-aurora-lang' });
  assert.equal(api.lang, 'en');
  api.set('auto');
  context.navigator.languages = ['zh-HK'];
  events.get('languagechange')();
  assert.equal(api.lang, 'zh-Hant');
});

test('remaining surfaces use complete messages in all four languages', () => {
  const { api, context } = boot();
  for (const lang of ['vi', 'en', 'zh-Hans', 'zh-Hant']) {
    api.set(lang);
    for (const key of ['cloud.point', 'network.fileLimits', 'network.approval.pending', 'legal.storage', 'legal.warranty', 'agent.step', 'schedule.receivedUnsaved']) {
      const translated = api.message(key, { x:1,y:2,count:2,max:3,used:'2 KiB',total:'8 KiB',each:'4 KiB',title:'Cài đặt',current:1 });
      assert.notEqual(translated, key);
      assert.ok(!translated.includes('{{'), key);
    }
    assert.equal(api.t('Trình duyệt sẵn sàng'), context.AURORA_LOCALES[lang].ui['cloud.browser_ready']);
    assert.equal(api.t('Đã lưu phiên bản 3.'), api.message('page.saved', { version:'3' }));
    assert.equal(api.t('Đã ẩn · 2'), api.message('misc.hiddenCount', { count:2 }));
  }
  api.set('en');
  assert.equal(api.message('network.directory', {count:1}), '1 assistant session');
  assert.equal(api.message('network.directory', {count:2}), '2 assistant sessions');
});

test('bound dynamic text changes language and releases ownership when the view writes a new state', () => {
  const { api, context } = boot();
  const node = { dataset:{},textContent:'' };
  context.document.querySelectorAll = () => node.dataset.i18n ? [node] : [];
  api.bind(node, 'network.approval.pending', () => ({ title:'Cài đặt / 設定 <script>' }));
  for (const lang of ['en', 'zh-Hans', 'zh-Hant', 'vi']) {
    api.set(lang);
    assert.equal(node.textContent, api.message('network.approval.pending', {title:'Cài đặt / 設定 <script>'}));
  }
  node.textContent = 'new state written by the view';
  api.set('en');
  assert.equal(node.textContent, 'new state written by the view');
  assert.equal(node.dataset.i18n, undefined);
  api.bind(node, 'network.approval.pending', {title:'old'});
  api.literal(node, 'Cài đặt');
  api.set('zh-Hant');
  assert.equal(node.textContent, 'Cài đặt');
  assert.equal(node.dataset.i18n, undefined);
});

test('source phrase mappings resolve only static semantic messages', () => {
  const { context } = boot();
  for (const [source, key] of Object.entries(context.AURORA_LOCALE_SOURCE_KEYS)) {
    assert.ok(source.trim());
    for (const catalog of Object.values(context.AURORA_LOCALES)) {
      assert.ok(catalog.ui[key], key);
      assert.ok(!catalog.ui[key].includes('{{'), key);
    }
  }
});

test('mailbox limit validation returns localized errors after server limits change', () => {
  const { api } = boot();
  const source = fs.readFileSync(path.join(assets, 'network.js'), 'utf8');
  const start = source.indexOf('function validationError(){');
  const end = source.indexOf('\nfunction syncComposer()', start);
  assert.ok(start >= 0 && end > start);
  const context = {
    state: { limits: { maxFiles: 2, maxFileBytes: 8, maxTotalBytes: 10, maxTextBytes: 20 } },
    files: [], selectedSession: () => ({ id: 'fixture' }),
    $: () => ({ value: 'Local test' }),
    bytes: value => Buffer.byteLength(value), size: value => `${value} B`,
    m: (key, values) => api.message(key, values),
  };
  vm.createContext(context);
  vm.runInContext(source.slice(start, end), context);
  for (const lang of ['vi', 'en', 'zh-Hans', 'zh-Hant']) {
    api.set(lang);
    for (const [files, key, options] of [
      [[{ content: '1' }, { content: '2' }, { content: '3' }], 'network.fileCountLimit', { count: 2 }],
      [[{ content: '123456789' }], 'network.fileLimit', { size: '8 B' }],
      [[{ content: '123456' }, { content: '123456' }], 'network.totalLimit', { size: '10 B' }],
    ]) {
      context.files = files;
      assert.equal(context.validationError(), api.message(key, options));
    }
    context.files = [{ content: '123' }];
    assert.equal(context.validationError(), '');
  }
});

test('retrying an image upload after a language change preserves the original draft', async () => {
  const { api } = boot();
  const source = fs.readFileSync(path.join(assets, 'app.js'), 'utf8');
  const start = source.indexOf('async function insertUpload(item){');
  const end = source.indexOf('\nasync function transferUpload(', start);
  assert.ok(start >= 0 && end > start);
  let draft = { text: '' }, removed = 0;
  const context = {
    cur: null, S: { workspaceId: 'fixture' }, l10n: api,
    composerDrafts: { get: () => ({ ...draft }) },
    storeComposerDraft: (_scope, _agent, value) => { draft = value; return true; },
    uploadQueue: { remove: async () => { removed++; } }, toast() {},
  };
  vm.createContext(context);
  vm.runInContext(source.slice(start, end), context);
  const item = { scope: 'fixture', agent: 'test', path: 'uploads/Dự-án.jpg' };
  for (const original of ['vi', 'en', 'zh-Hans', 'zh-Hant']) {
    api.set(original);
    draft = { text: '' };
    await context.insertUpload(item);
    const originalText = draft.text;
    assert.ok(originalText.includes(api.message('chat.imageAttachment', { name: item.path })));
    for (const next of ['vi', 'en', 'zh-Hans', 'zh-Hant']) {
      api.set(next);
      await context.insertUpload(item);
      assert.equal(draft.text, originalText);
    }
  }
  assert.equal(removed, 20);
});
