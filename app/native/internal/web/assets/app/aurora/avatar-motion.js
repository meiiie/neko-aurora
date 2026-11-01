(() => {
  "use strict";
  const key = "aurora.avatar-motion.v1", tracked = new Map();
  const reduced = matchMedia("(prefers-reduced-motion: reduce)");
  let mode = "auto";
  try { if (localStorage.getItem(key) === "off") mode = "off"; } catch {}
  const allowed = () => mode !== "off" && !reduced.matches && !document.hidden;
  const update = node => { node.dataset.motion = allowed() && tracked.get(node) ? "on" : "off"; };
  function refresh() {
    for (const node of tracked.keys()) update(node);
    window.dispatchEvent(new Event("aurora:motionchange"));
  }
  const observer = new IntersectionObserver(entries => {
    for (const { target, isIntersecting } of entries) {
      if (!tracked.has(target)) continue;
      tracked.set(target, isIntersecting); update(target);
    }
  });
  function attach(node, id, state) {
    let seed = 0;
    for (const char of String(id)) seed = (seed * 31 + char.codePointAt(0)) >>> 0;
    node.dataset.avatarState = state === "working" ? "working" : state === "waiting" ? "waiting" : state === "error" ? "error" : "idle";
    node.style.setProperty("--avatar-phase", -((Date.now() + seed) % 6400) / 1000 + "s");
    node.style.setProperty("--avatar-work-phase", -((Date.now() + seed) % 3200) / 1000 + "s");
    tracked.set(node, false); update(node); observer.observe(node);
    return node;
  }
  function setMode(value) {
    if (!["auto", "off"].includes(value)) return false;
    mode = value;
    let saved = true;
    try { localStorage.setItem(key, value); } catch { saved = false; }
    refresh(); return saved;
  }
  new MutationObserver(() => {
    for (const node of tracked.keys()) if (!node.isConnected) {
      observer.unobserve(node); tracked.delete(node);
    }
  }).observe(document.documentElement, { childList: true, subtree: true });
  document.addEventListener("visibilitychange", refresh);
  reduced.addEventListener("change", refresh);
  window.addEventListener("storage", event => {
    if (event.key !== key && event.key !== null) return;
    try { mode = localStorage.getItem(key) === "off" ? "off" : "auto"; } catch {}
    refresh();
  });
  function createSettings() {
    const I = window.__I18N, section = document.createElement("section");
    section.className = "card-group locale-settings";
    const heading = I.bind(document.createElement("h3"), "motion.title");
    const card = document.createElement("div"); card.className = "card";
    const row = document.createElement("div"); row.className = "locale-field";
    const label = I.bind(document.createElement("label"), "motion.label"); label.htmlFor = "avatarMotion";
    const select = document.createElement("select"); select.id = "avatarMotion"; select.setAttribute("aria-describedby", "avatarMotionNote");
    for (const value of ["auto", "off"]) {
      const option = I.bind(document.createElement("option"), "motion." + value); option.value = value; select.append(option);
    }
    const note = I.bind(document.createElement("p"), "motion.note"); note.id = "avatarMotionNote"; note.className = "locale-note";
    const status = document.createElement("p"); status.className = "locale-save-status"; status.setAttribute("role", "status");
    select.onchange = () => I.bind(status, setMode(select.value) ? "settings.locale.saved" : "settings.locale.temporary");
    if (createSettings.listener) window.removeEventListener("aurora:motionchange", createSettings.listener);
    createSettings.listener = () => { select.value = mode; };
    window.addEventListener("aurora:motionchange", createSettings.listener); select.value = mode;
    row.append(label, select); card.append(row); section.append(heading, card, note, status);
    return section;
  }
  window.AURORA_AVATARS = { attach, createSettings, setMode, mode: () => mode };
})();
