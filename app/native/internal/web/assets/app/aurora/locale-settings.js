(() => {
  "use strict";
  const i18n = window.__I18N;
  const make = (tag, className, key) => {
    const node = document.createElement(tag);
    if (className) node.className = className;
    if (key) i18n.bind(node, key);
    return node;
  };
  function create() {
    const section = make("section", "card-group locale-settings");
    section.id = "localeSettings";
    section.append(make("h3", "", "settings.locale.title"));
    const card = make("div", "card");
    const field = (id, title, options, description) => {
      const row = make("div", "locale-field");
      const label = make("label", "", title);
      label.htmlFor = id;
      const select = make("select", "");
      select.id = id;
      select.setAttribute("aria-describedby", description);
      for (const [value, text, key] of options) {
        const option = make("option", "", key);
        option.value = value;
        if (!key) { option.dataset.noi18n = ""; option.textContent = text; }
        select.append(option);
      }
      row.append(label, select);
      card.append(row);
      return select;
    };
    const language = field("interfaceLanguage", "settings.locale.language", [["auto", "", "settings.locale.auto"], ...Object.entries(i18n.labels)], "localeActive localeNote");
    const active = make("p", "locale-active");
    active.id = "localeActive";
    active.dataset.noi18n = "";
    card.append(active);
    const region = field("formatRegion", "settings.locale.region", [["auto", "", "settings.locale.follow"], ...i18n.regions.map(code => [code, "", "region." + code])], "localePreview localeNote");
    const preview = make("div", "locale-preview");
    preview.append(make("span", "", "settings.locale.preview"));
    const example = make("output", "");
    example.id = "localePreview";
    example.dataset.noi18n = "";
    preview.append(example);
    card.append(preview);
    const note = make("p", "locale-note", "settings.locale.note");
    note.id = "localeNote";
    const status = make("p", "locale-save-status");
    status.setAttribute("role", "status");
    status.setAttribute("aria-live", "polite");
    status.dataset.noi18n = "";
    section.append(card, note, status);
    let lastPersisted = null;
    function refresh() {
      language.value = i18n.pref();
      region.value = i18n.region();
      active.textContent = i18n.message("settings.locale.active", { language: i18n.labels[i18n.lang] });
      example.textContent = i18n.date(new Date(2026, 10, 23, 14, 5), { dateStyle: "medium", timeStyle: "short" }) + " · " + i18n.number(12345.67);
      if (lastPersisted !== null) status.textContent = i18n.message(lastPersisted ? "settings.locale.saved" : "settings.locale.temporary");
    }
    language.onchange = () => { lastPersisted = i18n.set(language.value); refresh(); };
    region.onchange = () => { lastPersisted = i18n.setRegion(region.value); refresh(); };
    if (create.listener) window.removeEventListener("aurora:localechange", create.listener);
    create.listener = refresh;
    window.addEventListener("aurora:localechange", refresh);
    refresh();
    return section;
  }
  window.AURORA_LOCALE_SETTINGS = { create };
})();
