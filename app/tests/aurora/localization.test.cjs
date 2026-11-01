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
