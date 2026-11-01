(function () {
  "use strict";
  const $ = selector => document.querySelector(selector);
  const text = value => window.__I18N?.t(value) || value;
  const make = (tag, className, value) => {
    const node = document.createElement(tag);
    if (className) node.className = className;
    if (value !== undefined) node.textContent = text(value);
    return node;
  };
  const literal = (tag, className, value) => {
    const node = make(tag, className);
    node.dataset.noi18n = "";
    node.textContent = value || "";
    return node;
  };
  const icon = name => window.AURORA_ICONS.icon(name, 20);
  const normalize = value => String(value).toLowerCase().normalize("NFD").replace(/\p{Diacritic}/gu, "").replace(/đ/g, "d");
  const desktop = matchMedia("(min-width: 900px)");
  const filters = { all: "Tất cả", unread: "Chưa đọc", working: "Đang làm", waiting: "Cần bạn" };
  let bridge, scope, filter = "all", mode = "home", queued = false, collapsed = false;
  let routineRequest = null, routineGeneration = 0, routineRows = [], routineError = false;
  let paletteEntries = [], paletteIndex = 0, paletteReturn = null;
  let reading = null;
  const actions = [
    ["home", "Tổng quan", "layout-grid"],
    ["routines", "Lịch chạy", "calendar"],
    ["pages", "Thư viện trang", "folder"],
    ["apps", "Kết nối ứng dụng", "layout-grid"],
    ["vault", "Vault", "shield-check"],
    ["network", "Giao tiếp agent", "users"],
    ["settings", "Cài đặt", "sun-moon"],
  ];
  const current = () => bridge?.view() || { agents: [], ready: false };
  function button(label, name, run, className = "workspace-button") {
    const node = make("button", className);
    node.type = "button";
    node.setAttribute("aria-label", text(label));
    if (name) node.append(icon(name));
    node.append(make("span", "", label));
    node.onclick = run;
    return node;
  }
  function matches(agent, flags) {
    if (scope !== bridge.workspaceId()) return true;
    return filter === "all" || filter === "working" && agent.status === "working" || filter === "waiting" && flags.waiting || filter === "unread" && flags.unread;
  }
  function cancelRoutines() {
    routineGeneration++;
    routineRequest?.abort();
    routineRequest = null;
  }
  function activate(id) {
    if (!current().ready) return;
    if (id === "home" || id === "routines") {
      mode = id;
      bridge.home();
      cancelRoutines();
      refresh();
      if (id === "routines") void loadRoutines();
      else if (desktop.matches) $("#workspaceHomeTitle").focus({ preventScroll: true });
      else $("#search").focus({ preventScroll: true });
      return;
    }
    bridge.action(id);
  }
  function openBot(id) {
    mode = "home";
    cancelRoutines();
    bridge.open(id);
    refresh();
  }
  function chooseFilter(value) {
    filter = value;
    bridge.renderList();
    refresh();
  }
  function layout() {
    if (!bridge) return;
    const view = current(), hasThread = !!view.current, profile = !$("#profile").classList.contains("off");
    const routines = mode === "routines" && !hasThread && view.ready;
    $("#app").dataset.workspaceView = hasThread ? "chat" : routines ? "routines" : "home";
    $("#app").dataset.sidebarCollapsed = String(collapsed);
    $("#workspaceHome").hidden = hasThread || routines || !view.ready;
    $("#workspaceRoutines").hidden = !routines;
    $("#workspaceRoutines").inert = !routines || profile && innerWidth < 1280;
    $("#thread").inert = !hasThread || !view.ready || profile && innerWidth < 1280;
    $("#profile").inert = !profile || !view.ready;
    $("#list").inert = !$("#login").classList.contains("hidden") || view.ready && (desktop.matches ? collapsed : hasThread || profile || routines);
    $("#workspaceHome").inert = !desktop.matches || !view.ready || profile && innerWidth < 1280;
    $("#workspaceRail").inert = !view.ready;
    const toggle = $("#workspaceSidebarToggle");
    toggle.setAttribute("aria-expanded", String(!collapsed));
    toggle.setAttribute("aria-label", text(collapsed ? "Hiện danh sách bot" : "Thu gọn danh sách bot"));
    toggle.title = toggle.getAttribute("aria-label");
    document.querySelectorAll("[data-workspace-action]").forEach(node => {
      const active = node.dataset.workspaceAction === (routines ? "routines" : "home");
      if (active) node.setAttribute("aria-current", "page");
      else node.removeAttribute("aria-current");
    });
  }
  function agentButton(agent) {
    const node = make("button", "workspace-agent");
    node.type = "button";
    node.dataset.agent = agent.id;
    node.append(bridge.avatar(agent, 40));
    const copy = make("span", "workspace-agent-copy");
    copy.append(literal("b", "", agent.name), literal("small", "", agent.desc || (agent.snipKey ? window.__I18N.message(agent.snipKey, agent.snipArgs) : agent.snip) || ""));
    node.append(copy);
    const state = agent.waiting ? "Cần bạn" : agent.status === "working" ? "Đang làm" : "Mở hội thoại";
    node.append(make("span", "workspace-agent-status", state));
    node.onclick = () => openBot(agent.id);
    return node;
  }
  function refresh() {
    if (!bridge) return;
    const view = current();
    const sameScope = scope === view.workspaceId;
    if (!sameScope || !view.ready) {
      scope = view.workspaceId;
      filter = "all";
      mode = "home";
      cancelRoutines();
      routineRows = [];
      $("#workspaceRoutineFilter").value = "";
      $("#workspaceRoutineRows").replaceChildren();
      $("#workspacePalette").close();
      $("#workspaceQuery").value = "";
      paletteEntries = [];
      $("#workspaceResults").replaceChildren();
    }
    if (view.current && mode === "routines") {
      mode = "home";
      cancelRoutines();
    }
    layout();
    const agents = view.ready ? view.agents.filter(agent => !agent.hidden) : [];
    const counts = { all: agents.length, unread: agents.filter(a => a.unread).length, working: agents.filter(a => a.status === "working").length, waiting: agents.filter(a => a.waiting).length };
    for (const [key, label] of Object.entries(filters)) {
      const node = $(`[data-workspace-filter="${key}"]`);
      node.setAttribute("aria-pressed", String(filter === key));
      node.setAttribute("aria-label", `${text(label)} (${counts[key]})`);
      node.querySelector("small").textContent = counts[key];
    }
    $("#workspaceBotCount").textContent = agents.length;
    $("#workspaceWorkingCount").textContent = counts.working;
    $("#workspaceWaitingCount").textContent = counts.waiting;
    $("#workspaceCount").textContent = agents.length;
    const recentFocus = sameScope && document.activeElement?.closest(".workspace-agent")?.dataset.agent;
    $("#workspaceRecent").replaceChildren(...agents.slice().sort((a, b) => Number(b.waiting) - Number(a.waiting) || Date.parse(b.t || 0) - Date.parse(a.t || 0)).slice(0, 5).map(agentButton));
    if (recentFocus && document.activeElement === document.body) [...$("#workspaceRecent").children].find(node => node.dataset.agent === recentFocus)?.focus({ preventScroll: true });
    $("#workspaceRecentEmpty").hidden = agents.length > 0;
    $("#workspaceHomeTitle").textContent = text(agents.length ? "Cùng làm việc, từ đây." : "Một nơi cho mọi việc.");
    const active = view.agents.find(a => a.id === view.current);
    $("#workspaceThreadTools").hidden = !active || active.kind === "group";
    for (const node of document.querySelectorAll("#rows [data-id], #featured [data-id]")) {
      const selected = node.dataset.id === view.current;
      node.classList.toggle("workspace-selected", selected);
      const target = node.matches("button") ? node : node.querySelector(".rowin");
      if (selected) target?.setAttribute("aria-current", "true");
      else target?.removeAttribute("aria-current");
    }
    if ($("#workspacePalette").open) renderPalette();
    rememberReading();
  }
  function readingSize(box) {
    return `${box.clientWidth}:${box.clientHeight}:${box.getBoundingClientRect().top}`;
  }
  function rememberReading(force = false) {
    const view = current(), box = $("#messages");
    if (!view.ready || !view.current) { reading = null; return; }
    const owner = `${view.workspaceId}:${view.current}`, size = readingSize(box);
    if (!force && reading?.owner === owner && reading.size !== size) return;
    const top = box.getBoundingClientRect().top;
    const node = [...box.querySelectorAll("[data-message-id]")].find(item => item.getBoundingClientRect().bottom > top + 5);
    const offset = node ? node.getBoundingClientRect().top - top : 0;
    const stableOffset = reading?.owner === owner && reading.node === node && Math.abs(reading.offset - offset) < 1 ? reading.offset : offset;
    reading = { owner, size, node, offset: stableOffset, bottom: box.scrollHeight - box.clientHeight - box.scrollTop < 3 };
  }
  function restoreReading() {
    const view = current(), box = $("#messages");
    if (reading?.owner === `${view.workspaceId}:${view.current}` && reading.size !== readingSize(box)) {
      if (reading.bottom && getSelection()?.isCollapsed !== false) box.scrollTop = box.scrollHeight;
      else if (reading.node?.isConnected && box.contains(reading.node)) {
        box.scrollTop += reading.node.getBoundingClientRect().top - box.getBoundingClientRect().top - reading.offset;
      }
    }
    rememberReading(true);
  }
  function notify() {
    layout();
    if (queued) return;
    queued = true;
    queueMicrotask(() => { queued = false; refresh(); });
  }
  function routineLabel(row) {
    if (row.repeat === "interval") return window.__I18N.message("routine.every", { count: row.intervalMinutes });
    const repeat = { daily: "Mỗi ngày", weekdays: "Thứ 2–6", once: "Một lần" }[row.repeat] || row.repeat;
    return [row.date, row.at, text(repeat), row.timezone || "Asia/Ho_Chi_Minh"].filter(Boolean).join(" · ");
  }
  function renderRoutines() {
    const selected = $("#workspaceRoutineFilter").value;
    const agents = current().agents;
    const rows = selected ? routineRows.filter(row => row.bot === selected) : routineRows;
    const list = $("#workspaceRoutineRows");
    list.replaceChildren();
    for (const row of rows) {
      const agent = agents.find(a => a.id === row.bot);
      if (!agent) continue;
      const card = make("article", "workspace-routine");
      const mark = make("span", "workspace-routine-icon");
      mark.append(icon("calendar"));
      const content = make("div", "workspace-routine-copy");
      content.append(literal("h3", "", row.text), literal("p", "", agent.name));
      const timing = make("p", "workspace-routine-timing");
      timing.append(literal("span", "", routineLabel(row)), make("span", "workspace-state" + (row.paused ? " paused" : ""), row.paused ? "Đã tạm dừng" : row.blocked ? "Cần kiểm tra" : "Đang bật"));
      content.append(timing);
      const manage = button("Quản lý lịch", "chevron-right", () => {
        openBot(agent.id);
        bridge.profile(agent.id, "schedule");
      });
      card.append(mark, content, manage);
      list.append(card);
    }
    $("#workspaceRoutineEmpty").hidden = rows.length > 0 || routineError;
  }
  async function loadRoutines() {
    cancelRoutines();
    const view = current(), generation = routineGeneration;
    if (!view.ready || mode !== "routines") return;
    const controller = new AbortController();
    routineRequest = controller;
    const active = () => routineGeneration === generation && current().workspaceId === view.workspaceId && current().ready && mode === "routines" && !current().current;
    const status = $("#workspaceRoutineStatus"), reload = $("#workspaceRoutineReload");
    routineError = false;
    routineRows = [];
    $("#workspaceRoutineRows").replaceChildren();
    $("#workspaceRoutineEmpty").hidden = true;
    status.textContent = text("Đang tải lịch chạy…");
    reload.disabled = true;
    const select = $("#workspaceRoutineFilter"), selected = select.value;
    select.replaceChildren(make("option", "", "Tất cả bot"));
    select.firstChild.value = "";
    for (const agent of view.agents.filter(a => a.kind !== "group" && !a.hidden)) {
      const option = literal("option", "", agent.name);
      option.value = agent.id;
      select.append(option);
    }
    select.value = view.agents.some(a => a.id === selected) ? selected : "";
    const timer = setTimeout(() => controller.abort(), 15000);
    try {
      const response = await fetch(`/app/api/schedules?workspaceId=${encodeURIComponent(view.workspaceId)}`, { credentials: "same-origin", cache: "no-store", signal: controller.signal });
      if (!response.ok) throw Error("schedules");
      const data = await response.json();
      if (!active()) return;
      if (data.workspaceId !== view.workspaceId || !Array.isArray(data.schedules)) throw Error("scope");
      routineRows = data.schedules.filter(row => view.agents.some(agent => agent.id === row.bot && !agent.hidden));
      status.textContent = text("Chọn một lịch để quản lý trong hồ sơ bot.");
      renderRoutines();
    } catch {
      if (!active()) return;
      routineError = true;
      status.textContent = text("Chưa tải được lịch chạy. Hãy thử lại.");
    } finally {
      clearTimeout(timer);
      if (active()) reload.disabled = false;
      if (routineRequest === controller) routineRequest = null;
    }
  }
  function renderPalette() {
    const view = current(), q = normalize($("#workspaceQuery").value.trim());
    paletteEntries = [
      ...view.agents.filter(agent => !agent.hidden).map(agent => ({ label: agent.name, description: agent.kind === "group" ? "Nhóm" : "Bot", agent, run: () => openBot(agent.id) })),
      ...actions.map(([id, label, name]) => ({ label: text(label), description: "Đi đến", icon: name, run: () => activate(id) })),
      { label: text("Tạo bot mới"), description: "Thao tác", icon: "plus", run: () => bridge.action("create") },
    ].filter(entry => !q || normalize(entry.label).includes(q));
    paletteIndex = Math.min(paletteIndex, Math.max(0, paletteEntries.length - 1));
    const results = $("#workspaceResults");
    results.replaceChildren();
    paletteEntries.forEach((entry, index) => {
      const node = make("button", "workspace-result");
      node.type = "button";
      node.id = `workspace-result-${index}`;
      node.tabIndex = -1;
      node.setAttribute("role", "option");
      node.setAttribute("aria-selected", String(index === paletteIndex));
      node.append(entry.agent ? bridge.avatar(entry.agent, 28) : icon(entry.icon));
      node.append(literal("span", "", entry.label), make("small", "", entry.description));
      node.onclick = () => { closePalette(false); entry.run(); };
      results.append(node);
    });
    $("#workspacePaletteEmpty").hidden = paletteEntries.length !== 0;
    const input = $("#workspaceQuery");
    if (paletteEntries.length) input.setAttribute("aria-activedescendant", `workspace-result-${paletteIndex}`);
    else input.removeAttribute("aria-activedescendant");
    results.children[paletteIndex]?.scrollIntoView({ block: "nearest" });
  }
  function closePalette(restore = true) {
    $("#workspacePalette").close();
    if (restore && paletteReturn?.isConnected && !paletteReturn.closest("[inert]")) paletteReturn.focus({ preventScroll: true });
  }
  function openPalette() {
    if (!current().ready || bridge.busy() || document.querySelector("dialog[open]")) return;
    paletteReturn = document.activeElement;
    paletteIndex = 0;
    $("#workspaceQuery").value = "";
    renderPalette();
    $("#workspacePalette").showModal();
    $("#workspaceQuery").focus();
  }
  function mount(api) {
    bridge = api;
    try { collapsed = localStorage.getItem("aurora.workspace.sidebar.v1") === "collapsed"; } catch {}
    const rail = $("#workspaceNav");
    for (const [id, label, name] of actions) {
      const node = button(label, name, () => activate(id), "workspace-nav-button");
      node.dataset.workspaceAction = id;
      node.title = text(label);
      rail.append(node);
    }
    const tabs = $("#workspaceFilters");
    for (const [id, label] of Object.entries(filters)) {
      const node = button(label, null, () => chooseFilter(id), "workspace-filter");
      node.dataset.workspaceFilter = id;
      node.append(make("small", "", "0"));
      tabs.append(node);
    }
    $("#workspacePaletteButton").append(icon("search"));
    $("#workspacePaletteButton").onclick = openPalette;
    $("#workspaceSidebarToggle").append(icon("list-checks"));
    $("#workspaceSidebarToggle").onclick = () => {
      collapsed = !collapsed;
      try { localStorage.setItem("aurora.workspace.sidebar.v1", collapsed ? "collapsed" : "expanded"); } catch {}
      layout();
    };
    $("#workspaceHomeButton").onclick = () => activate("home");
    document.querySelectorAll("[data-workspace-go]").forEach(node => { node.onclick = () => activate(node.dataset.workspaceGo); });
    $("#workspaceCreateBot").onclick = () => bridge.action("create");
    $("#workspaceNewBot").onclick = () => bridge.action("create");
    $("#workspaceQuickSearch").onclick = openPalette;
    $("#workspaceSkip").onclick = () => {
      const target = current().current ? $("#input") : mode === "routines" ? $("#workspaceRoutineFilter") : desktop.matches ? $("#workspaceHomeTitle") : $("#search");
      target.focus({ preventScroll: true });
    };
    $("#workspaceRoutinesBack").onclick = () => activate("home");
    $("#workspaceRoutineReload").onclick = () => void loadRoutines();
    $("#workspaceRoutineFilter").onchange = renderRoutines;
    const toolbar = $("#workspaceThreadTools");
    for (const [key, label, name] of [["tools", "AI & công cụ", "sparkles"], ["memory", "Bộ nhớ", "folder"], ["computer", "Máy tính", "monitor"]]) {
      toolbar.append(button(label, name, () => { const id = current().current; if (id) bridge.botAction(key, id); }, "workspace-tool"));
    }
    $("#workspacePaletteClose").onclick = () => closePalette();
    $("#workspacePalette").addEventListener("cancel", event => { event.preventDefault(); closePalette(); });
    $("#workspacePalette").addEventListener("keydown", event => {
      if (event.key !== "Tab") return;
      const nodes = [...event.currentTarget.querySelectorAll('button:not([disabled]):not([tabindex="-1"]), input:not([disabled])')];
      const index = nodes.indexOf(document.activeElement);
      event.preventDefault();
      nodes[(index + (event.shiftKey ? -1 : 1) + nodes.length) % nodes.length]?.focus();
    });
    $("#workspacePalette").addEventListener("click", event => { if (event.target === event.currentTarget) closePalette(); });
    $("#workspaceQuery").oninput = () => { paletteIndex = 0; renderPalette(); };
    $("#workspaceQuery").onkeydown = event => {
      if (event.isComposing || event.keyCode === 229) return;
      if (event.key === "ArrowDown" || event.key === "ArrowUp") {
        event.preventDefault();
        const size = paletteEntries.length;
        if (size) paletteIndex = (paletteIndex + (event.key === "ArrowDown" ? 1 : -1) + size) % size;
        renderPalette();
      } else if (event.key === "Enter") {
        event.preventDefault();
        const entry = paletteEntries[paletteIndex];
        if (entry) { closePalette(false); entry.run(); }
      }
    };
    document.addEventListener("keydown", event => {
      if (event.isComposing || event.keyCode === 229 || event.shiftKey || event.altKey || !(event.ctrlKey || event.metaKey) || event.key.toLowerCase() !== "k") return;
      event.preventDefault();
      if ($("#workspacePalette").open) closePalette(); else openPalette();
    });
    window.addEventListener("aurora:localechange", () => { refresh(); if (mode === "routines") renderRoutines(); });
    desktop.addEventListener("change", layout);
    matchMedia("(min-width: 1280px)").addEventListener("change", layout);
    $("#messages").addEventListener("scroll", () => rememberReading(), { passive: true });
    if (window.ResizeObserver) new ResizeObserver(restoreReading).observe($("#messages"));
    new MutationObserver(layout).observe($("#profile"), { attributes: true, attributeFilter: ["class"] });
    new MutationObserver(notify).observe($("#login"), { attributes: true, attributeFilter: ["class"] });
    window.addEventListener("popstate", () => { if (mode === "routines") { mode = "home"; cancelRoutines(); refresh(); } });
    refresh();
  }
  window.AURORA_WORKSPACE = { mount, notify, matches, filtered: () => filter !== "all" };
})();
