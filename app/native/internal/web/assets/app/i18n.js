/* Local resources only. UI translations never alter conversation or form data. */
(() => {
  "use strict";
  const resources = window.AURORA_LOCALES;
  const labels = { vi: "Tiếng Việt", en: "English", "zh-Hans": "简体中文", "zh-Hant": "繁體中文" };
  const defaults = { vi: "vi-VN", en: "en-US", "zh-Hans": "zh-CN", "zh-Hant": "zh-TW" };
  const regions = ["vi-VN", "en-US", "en-GB", "zh-CN", "zh-TW", "zh-HK"];
  const languageKey = "neko-aurora-lang", regionKey = "neko-aurora-region";
  const read = key => { try { return localStorage.getItem(key) || "auto"; } catch { return "auto"; } };
  const validPreference = value => value === "auto" || Object.hasOwn(labels, value);
  function matchLanguage(tag) {
    try {
      const locale = new Intl.Locale(tag);
      if (locale.language === "vi" || locale.language === "en") return locale.language;
      if (locale.language === "zh") {
        const script = locale.maximize().script;
        return script === "Hant" ? "zh-Hant" : script === "Hans" ? "zh-Hans" : null;
      }
    } catch {}
    return null;
  }
  function resolve(preference, languages = navigator.languages || [navigator.language]) {
    if (preference !== "auto" && validPreference(preference)) return preference;
    for (const tag of languages || []) { const match = matchLanguage(tag); if (match) return match; }
    return "en";
  }
  let preference = validPreference(read(languageKey)) ? read(languageKey) : "auto";
  let region = regions.includes(read(regionKey)) ? read(regionKey) : "auto";
  let language = resolve(preference);
  const locale = () => {
    if (region !== "auto") return region;
    if (preference === "auto") {
      for (const tag of navigator.languages || [navigator.language]) {
        if (matchLanguage(tag) !== language) continue;
        try { const detected = new Intl.Locale(tag).maximize(); return detected.language + "-" + detected.region; } catch {}
      }
    }
    return defaults[language];
  };
  const number = (value, options = {}) => new Intl.NumberFormat(locale(), options).format(value);
  const date = (value, options = {}) => {
    const parsed = value instanceof Date ? value : new Date(value);
    return Number.isNaN(parsed.getTime()) ? "" : new Intl.DateTimeFormat(locale(), options).format(parsed);
  };
  const relative = (value, unit, options = {}) => new Intl.RelativeTimeFormat(language, { numeric: "auto", ...options }).format(value, unit);
  const engine = window.i18next.createInstance();
  engine.init({ lng: language, fallbackLng: "en", supportedLngs: Object.keys(labels), load: "currentOnly", resources,
    ns: ["ui", "legacy"], defaultNS: "ui", keySeparator: false, nsSeparator: false, initAsync: false,
    interpolation: { escapeValue: false }, returnEmptyString: false });
  engine.services.formatter.add("number", value => number(value));
  function message(key, options = {}) { return engine.t(key, { ...options, ns: "ui" }); }
  const patterns = [
    [/^Bước (\d+)\/(\d+): (.+)$/, "agent.step", ["current", "total", "title"]],
    [/^Đã lưu trữ (.+)$/, "misc.archived", ["name"]],
    [/^Đã bỏ ẩn (.+)$/, "misc.unhidden", ["name"]],
    [/^Đã ẩn (?!·)(.+)$/, "misc.hidden", ["name"]],
    [/^Đã nhân bản (.+)$/, "misc.duplicated", ["name"]],
    [/^Đã chuyển vào (.+)$/, "misc.movedTo", ["folder"]],
    [/^Đã chuyển (.+) vào (.+)$/, "misc.moved", ["name", "folder"]],
    [/^Đã lưu (?!phiên bản)(.+)$/, "misc.saved", ["name"]],
    [/^Tuỳ chọn cho (.+)$/, "misc.options", ["name"]],
    [/^Không tìm thấy “(.+)”\.$/, "misc.noMatch", ["query"]],
    [/^Ẩn lại (\d+) bot$/, "misc.hideAgain", ["count"]],
    [/^Đã ẩn · (\d+)$/, "misc.hiddenCount", ["count"]],
    [/^Xem cả lệnh · thêm (\d+) dòng$/, "misc.fullCommand", ["count"]],
    [/^Hình (\d+)$/, "misc.shape", ["count"]],
    [/^Màu (\d+)$/, "misc.color", ["count"]],
    [/^Bỏ (.+)$/, "misc.remove", ["name"]],
    [/^Máy này · (\d+) thiết bị$/, "misc.devices", ["count"]],
    [/^Đã kết nối · gửi từ (.+)$/, "misc.connectedFrom", ["email"]],
    [/^(\d+) tin$/, "network.mailCount", ["count"]],
    [/^(\d+) chờ$/, "misc.waitCount", ["count"]],
    [/^(\d+) nhóm$/, "misc.groups", ["count"]],
    [/^(\d+) kết quả$/, "misc.matches", ["count"]],
    [/^Bộ nhớ · (.+)$/, "misc.memoryFor", ["name"]],
    [/^Riêng (.+)$/, "misc.privateFor", ["name"]],
    [/^Lịch sử thay đổi \((\d+)\)$/, "misc.changeHistory", ["count"]],
    [/^(\d+) mục$/, "misc.items", ["count"]],
    [/^(\d+) lượt gọi mô hình$/, "misc.calls", ["count"]],
    [/^(\d+) lượt$/, "misc.turns", ["count"]],
    [/^Kèm chat: (.+)$/, "misc.reference", ["name"]],
    [/^AI & công cụ · (.+)$/, "misc.toolsFor", ["name"]],
    [/^Kết nối lại · (.+)$/, "misc.reconnect", ["name"]],
    [/^Có thể thử lại sau (.+)$/, "misc.retryAt", ["time"]],
    [/^Đã nhập (\d+) mục$/, "misc.imported", ["count"]],
    [/^Đề xuất \((\d+) → (\d+) mục\)$/, "misc.proposal", ["before", "after"]],
    [/^Lịch sử dùng \((\d+)\)$/, "misc.usageHistory", ["count"]],
    [/^✓ Đã duyệt · không dùng được(.*)$/, "chat.vaultFailure", ["detail"]],
    [/^Đã lưu phiên bản (\d+)\.$/, "page.saved", ["version"]],
    [/^Đã lưu trữ · Phiên bản (\d+)$/, "page.archived", ["version"]],
    [/^Phiên bản (.+)$/, "app.version", ["version"]],
    [/^Mỗi (\d+) phút · Đã tạm dừng$/, "routine.paused", ["count"]],
    [/^Mỗi (\d+) phút$/, "routine.every", ["count"]],
    [/^Hôm nay (.+)$/, "time.today", ["time"]],
    [/^Hôm qua (.+)$/, "time.yesterday", ["time"]],
    [/^(\d+) bot · (\d+) nhóm$/, "count.workspace", ["bots", "groups"]],
    [/^(\d+) đã kết nối$/, "count.connected", ["count"]],
    [/^(\d+) chưa đọc$/, "count.unread", ["count"]],
    [/^(\d+) bot$/, "count.bots", ["count"]],
    [/^(\d+) tài khoản$/, "count.accounts", ["count"]],
    [/^(\d+) phút$/, "count.minutes", ["count"]],
    [/^(\d+) giờ$/, "count.hours", ["count"]],
    [/^Nguồn · (\d+)$/, "trace.sources", ["count"]],
    [/^Các bước · (\d+)$/, "trace.steps", ["count"]],
    [/^Hỏi (.+)$/, "chat.ask", ["name"]],
    [/^Đang tra (.+)$/, "trace.lookup", ["name"]],
    [/^Linux riêng · (.+)$/, "computer.private", ["status"]],
    [/^lỗi (\d+)$/, "computer.error", ["code"]],
  ];
  const has = key => Object.hasOwn(resources.en.legacy, key);
  const sourceKeys = window.AURORA_LOCALE_SOURCE_KEYS || {};
  function translate(value, options) {
    if (typeof value !== "string") return value;
    if (Object.hasOwn(resources.en.ui, value) || Object.hasOwn(resources.en.ui, value + "_other")) return message(value, options);
    const key = value.trim();
    let result;
    if (has(key)) result = engine.t(key, { ns: "legacy" });
    else if (Object.hasOwn(sourceKeys, key)) result = message(sourceKeys[key]);
    else for (const [pattern, target, names] of patterns) {
      const match = key.match(pattern);
      if (!match) continue;
      const values = {};
      names.forEach((name, index) => { values[name] = ["count", "bots", "groups", "before", "after", "current", "total"].includes(name) ? Number(match[index + 1]) : name === "status" ? translate(match[index + 1]) : match[index + 1]; });
      result = message(target, values);
      break;
    }
    return result === undefined ? value : value.replace(key, () => result);
  }
  // Transitional adapter for legacy views. New views bind semantic catalogue keys.
  const originals = new WeakMap(), attributes = new WeakMap(), boundAttributes = new WeakMap(), boundText = new WeakMap();
  const protectedContent = ".md,.msg.mine,.ap-body,.ap-subject,.ap-title,.brief-body,[data-noi18n],script,style,code,pre";
  const textSkip = protectedContent + ",textarea,input";
  const attributeNames = ["aria-label", "placeholder", "title", "alt"];
  const reverse = new Map();
  for (const catalogue of Object.values(resources)) for (const [source, target] of Object.entries(catalogue.legacy)) {
    if (!has(target) && !reverse.has(target)) reverse.set(target, source);
  }
  for (const [source, key] of Object.entries(sourceKeys)) for (const catalogue of Object.values(resources)) {
    const target = catalogue.ui[key];
    if (target && !reverse.has(target)) reverse.set(target, source);
  }
  const sourceOf = value => {
    const trimmed = value.trim();
    return reverse.has(trimmed) ? value.replace(trimmed, () => reverse.get(trimmed)) : value;
  };
  function updateValue(value, previous, set) {
    const source = previous && previous.rendered === value ? previous.source : sourceOf(value);
    const rendered = translate(source);
    if (value !== rendered) set(rendered);
    return { source, rendered };
  }
  function walk(node) {
    if (node.nodeType === 3) {
      if (!node.parentElement || node.parentElement.closest(textSkip) || node.parentElement.closest("[data-i18n]")) return;
      originals.set(node, updateValue(node.nodeValue, originals.get(node), value => { node.nodeValue = value; }));
      return;
    }
    if (node.nodeType !== 1) return;
    const bindings = boundAttributes.get(node);
    if (bindings) for (const [name, binding] of bindings) node.setAttribute(name, message(binding.key, typeof binding.options === "function" ? binding.options() : binding.options));
    if (node.closest(protectedContent)) return;
    if (node.dataset.i18n) renderBinding(node);
    const previous = attributes.get(node) || {};
    for (const attr of attributeNames) {
      if (bindings?.has(attr)) continue;
      const value = node.getAttribute(attr);
      if (value) previous[attr] = updateValue(value, previous[attr], next => node.setAttribute(attr, next));
    }
    attributes.set(node, previous);
    for (const child of node.childNodes) walk(child);
  }
  let observer;
  const observe = () => observer?.observe(document.body, { subtree: true, childList: true, characterData: true, attributes: true, attributeFilter: attributeNames });
  function apply() {
    observer?.disconnect();
    document.documentElement.lang = language;
    document.documentElement.dir = "ltr";
    if (document.body) walk(document.body);
    document.querySelectorAll?.("[data-i18n]").forEach(renderBinding);
    if (document.title) document.title = translate(sourceOf(document.title));
    observe();
  }
  function changed(persisted = true) {
    language = resolve(preference);
    engine.changeLanguage(language);
    apply();
    window.dispatchEvent(new CustomEvent("aurora:localechange", { detail: { language, locale: locale(), persisted } }));
  }
  function save(key, value) { try { localStorage.setItem(key, value); return true; } catch { return false; } }
  function set(value) {
    if (!validPreference(value)) return false;
    preference = value;
    const persisted = save(languageKey, value);
    changed(persisted);
    return persisted;
  }
  function setRegion(value) {
    if (value !== "auto" && !regions.includes(value)) return false;
    region = value;
    const persisted = save(regionKey, value);
    changed(persisted);
    return persisted;
  }
  function renderBinding(node) {
    const binding = boundText.get(node), options = binding?.options || {};
    if (binding?.rendered !== undefined && node.textContent !== binding.rendered) {
      boundText.delete(node); delete node.dataset.i18n; return;
    }
    const text = message(node.dataset.i18n, typeof options === "function" ? options() : options);
    if (binding) binding.rendered = text;
    if (node.textContent !== text) node.textContent = text;
  }
  function bind(node, key, options = {}) {
    node.dataset.i18n = key; boundText.set(node, { options }); renderBinding(node); return node;
  }
  function literal(node, value) {
    boundText.delete(node); delete node.dataset.i18n; node.dataset.noi18n = ""; node.textContent = value; return node;
  }
  function uiText(node, source) {
    const key = sourceKeys[source];
    if (key) return bind(node, key);
    boundText.delete(node); delete node.dataset.i18n; node.textContent = translate(source); return node;
  }
  function bindAttribute(node, attribute, key, options = {}) {
    if (!attributeNames.includes(attribute)) throw new Error("Unsupported localized attribute");
    const bindings = boundAttributes.get(node) || new Map();
    bindings.set(attribute, { key, options });
    boundAttributes.set(node, bindings);
    node.setAttribute(attribute, message(key, typeof options === "function" ? options() : options));
  }
  window.__I18N = {
    get lang() { return language; }, get locale() { return locale(); }, get speechLocale() { return defaults[language]; },
    pref: () => preference, region: () => region, labels: Object.freeze(labels), regions: Object.freeze(regions),
    set, setRegion, t: translate, message, bind, bindAttribute, literal, uiText, apply, number, date, relative,
    compare: (a, b) => new Intl.Collator(locale(), { sensitivity: "base", numeric: true }).compare(a, b),
    resolve, size: Object.keys(resources.en.legacy).length,
  };
  document.documentElement.lang = language;
  document.documentElement.dir = "ltr";
  document.addEventListener("DOMContentLoaded", () => {
    observer = new MutationObserver(records => {
      observer.disconnect();
      for (const record of records) {
        if (record.type === "childList") record.addedNodes.forEach(walk);
        else walk(record.target);
      }
      observe();
    });
    apply();
  });
  window.addEventListener("languagechange", () => { if (preference === "auto") changed(); });
  window.addEventListener("storage", event => {
    if (event.key !== languageKey && event.key !== regionKey && event.key !== null) return;
    preference = validPreference(read(languageKey)) ? read(languageKey) : "auto";
    region = regions.includes(read(regionKey)) ? read(regionKey) : "auto";
    changed();
  });
})();
