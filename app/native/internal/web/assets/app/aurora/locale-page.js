(() => {
  "use strict";
  document.addEventListener("DOMContentLoaded", () => {
    const i18n = window.__I18N, main = document.querySelector("main");
    if (!main || !i18n) return;
    const row = document.createElement("div"); row.className = "page-language";
    const label = i18n.bind(document.createElement("label"), "settings.locale.language");
    const select = document.createElement("select"); select.id = "pageLanguage"; label.htmlFor = select.id;
    const auto = i18n.bind(document.createElement("option"), "settings.locale.auto"); auto.value = "auto"; select.append(auto);
    for (const [value, name] of Object.entries(i18n.labels)) {
      const option = document.createElement("option"); option.value = value; i18n.literal(option, name); select.append(option);
    }
    const status = document.createElement("span"); status.setAttribute("role", "status");
    select.onchange = () => { const saved = i18n.set(select.value); i18n.bind(status, saved ? "settings.locale.saved" : "settings.locale.temporary"); };
    const refresh = () => { select.value = i18n.pref(); };
    window.addEventListener("aurora:localechange", refresh);
    row.append(label, select, status); main.prepend(row); refresh();
    document.querySelectorAll("[data-legal-updated]").forEach(node => i18n.bind(node, "legal.updated", () => ({ date: i18n.date(new Date(2026, 9, 23), { dateStyle: "long" }) })));
  });
})();
