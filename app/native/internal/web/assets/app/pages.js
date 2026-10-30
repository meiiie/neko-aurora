"use strict";
(() => {
  const MAX = 65536, encode = new TextEncoder(), drafts = new Map();
  const node = (tag, text, cls) => { const n = document.createElement(tag); if (text != null) n.textContent = text; if (cls) n.className = cls; return n; };
  const literal = (tag, text, cls) => { const n = node(tag, text, cls); n.dataset.noi18n = ""; return n; };
  const button = (text, action) => { const b = node("button", text); b.type = "button"; b.onclick = action; return b; };
  const size = text => encode.encode(text).length;
  const validID = id => /^page-[a-f0-9]{32}$/.test(id);
  const validRevision = n => Number.isSafeInteger(n) && n > 0;
  const validPage = p => p && validID(p.id) && validRevision(p.revision) && typeof p.title === "string" && typeof p.text === "string" && size(p.text) <= MAX && typeof p.archived === "boolean" && /^[a-f0-9]{64}$/.test(p.sha256);
  const sameInput = (p, input) => validPage(p) && (!input.id || p.id === input.id) && p.revision === input.baseRevision + 1 && p.title === input.title && p.text === input.text && p.archived === input.archived;
  const key = (scope, id) => "aurora.pages.v1." + JSON.stringify([scope, id || "new"]);
  let dialog, body, current, getScope, returnFocus;
  function close() {
    if (current) current.controller.abort();
    current = null;
    if (dialog) { dialog.close(); dialog.replaceChildren(); dialog.remove(); dialog = null; }
    if (returnFocus?.isConnected) returnFocus.focus();
  }
  function view() {
    current?.controller.abort();
    const v = { scope: getScope(), controller: new AbortController(), busy: false };
    current = v; body.replaceChildren(); return v;
  }
  const active = v => current === v && dialog?.open && getScope() === v.scope && !v.controller.signal.aborted;
  async function request(v, path, payload, raw = false) {
    if (!active(v)) throw new Error("stale");
    const url = "/app/api" + path + (payload ? "" : (path.includes("?") ? "&" : "?") + new URLSearchParams({ workspaceId: v.scope }));
    const response = await fetch(url, { method: payload ? "POST" : "GET", credentials: "same-origin", cache: "no-store", redirect: "error", headers: payload ? { "Content-Type": "application/json" } : {}, body: payload ? JSON.stringify(payload) : undefined, signal: AbortSignal.any([v.controller.signal, AbortSignal.timeout(30000)]) });
    if (!active(v)) throw new Error("stale");
    if (!response.ok) { const error = new Error("request"); error.status = response.status; throw error; }
    if (raw) return response;
    const data = await response.json();
    if (!active(v) || data.workspaceId !== v.scope) throw new Error("scope");
    return data;
  }
  function remember(v) {
    const name = key(v.scope, v.draft.id), text = JSON.stringify(v.draft);
    drafts.set(name, text);
    try { localStorage.setItem(name, text); if (localStorage.getItem(name) !== text) throw new Error("storage"); v.storageError = false; return true; }
    catch { v.storageError = true; return false; }
  }
  function readDraft(scope, id) {
    const name = key(scope, id), raw = drafts.get(name) ?? localStorage.getItem(name);
    if (!raw) return null;
    const d = JSON.parse(raw);
    if (d.workspaceId !== scope || d.id !== id || !Number.isSafeInteger(d.baseRevision) || d.baseRevision < 0 || typeof d.title !== "string" || typeof d.text !== "string" || typeof d.archived !== "boolean") throw new Error("draft");
    if (d.pending && (d.pending.workspaceId !== scope || d.pending.id !== id || d.pending.baseRevision !== d.baseRevision || d.pending.title !== d.title.trim() || d.pending.text !== d.text || d.pending.archived !== d.archived || !/^[a-zA-Z0-9_-]{8,128}$/.test(d.pending.clientNonce))) throw new Error("pending");
    return d;
  }
  function forget(scope, id) {
    drafts.delete(key(scope, id));
    try { localStorage.removeItem(key(scope, id)); return true; } catch { return false; }
  }
  function status(v, text) {
    const warning = "Không lưu được bản nháp trên thiết bị. Hãy tải bản nháp trước khi đóng hoặc tải lại.";
    if (active(v)) v.status.textContent = text + (v.storageError && text !== warning ? " " + warning : "");
  }
  function download(text, name) {
    const url = URL.createObjectURL(new Blob([text], { type: "text/markdown;charset=utf-8" })), a = node("a");
    a.href = url; a.download = name; document.body.append(a); a.click(); a.remove(); setTimeout(() => URL.revokeObjectURL(url), 10000);
  }
  function loading(v) { const n = node("p", "Đang tải…", "pages-note"); n.setAttribute("role", "status"); body.append(n); }
  function loadError(v, action) {
    if (!active(v)) return;
    body.replaceChildren(node("p", "Chưa tải được trang tài liệu.", "pages-note"), button("Tải lại", action), button("Về thư viện", list));
  }
  async function list() {
    const v = view(); loading(v);
    try {
      const data = await request(v, "/pages");
      if (!Array.isArray(data.pages)) throw new Error("pages");
      body.replaceChildren();
      const top = node("div", null, "pages-actions"); top.append(button("Trang tài liệu mới", () => edit("")), button("Làm mới", list)); body.append(top);
      const filter = node("input"); filter.type = "search"; filter.placeholder = "Tìm trang tài liệu"; filter.setAttribute("aria-label", "Tìm trang tài liệu");
      const archive = node("input"); archive.type = "checkbox"; const label = node("label", null, "pages-check"); label.append(archive, node("span", "Hiện trang đã lưu trữ"));
      const rows = node("div", null, "pages-list"); body.append(filter, label, rows);
      function render() {
        rows.replaceChildren(); const q = filter.value.toLocaleLowerCase();
        const found = data.pages.filter(p => validID(p.id) && typeof p.title === "string" && (archive.checked || !p.archived) && p.title.toLocaleLowerCase().includes(q));
        if (!found.length) rows.append(node("p", data.pages.length ? "Không có trang phù hợp." : "Chưa có trang tài liệu. Tạo trang đầu tiên để lưu ghi chú và các phiên bản.", "pages-note"));
        for (const p of found) { const b = button("", () => edit(p.id)); b.className = "pages-row"; b.append(literal("b", p.title), node("small", (p.archived ? "Đã lưu trữ · " : "") + "Phiên bản " + p.revision)); rows.append(b); }
      }
      filter.oninput = archive.onchange = render; render(); filter.focus();
    } catch { loadError(v, list); }
  }
  async function edit(id) {
    const v = view(); loading(v);
    try {
      v.latest = id ? (await request(v, "/pages/" + id)).page : null;
      if (id && (!validPage(v.latest) || v.latest.id !== id)) throw new Error("page");
      v.draft = readDraft(v.scope, id) || { workspaceId: v.scope, id, baseRevision: v.latest?.revision || 0, title: v.latest?.title || "", text: v.latest?.text || "", archived: v.latest?.archived || false, pending: null };
      editor(v);
      if (v.draft.pending) void receipt(v);
    } catch { loadError(v, () => edit(id)); }
  }
  function editor(v) {
    if (!active(v)) return;
    body.replaceChildren();
    const d = v.draft, top = node("div", null, "pages-actions");
    const back = button("Về thư viện", list), history = button("Lịch sử phiên bản", () => historyView(v)); history.disabled = !d.id;
    top.append(back, history, button("Tải bản nháp", () => download(d.text, "aurora-draft.md"))); body.append(top);
    v.status = node("p", "", "pages-notice"); v.status.setAttribute("role", "status"); body.append(v.status);
    v.conflict = node("div", null, "pages-conflict"); body.append(v.conflict);
    const titleLabel = node("label", "Tiêu đề trang"), title = node("input"); title.id = "pageTitleInput"; titleLabel.htmlFor = title.id; title.value = d.title; title.autocomplete = "off";
    const textLabel = node("label", "Nội dung Markdown"), text = node("textarea"); text.id = "pageTextInput"; textLabel.htmlFor = text.id; text.value = d.text; text.spellcheck = false;
    const archive = node("input"); archive.type = "checkbox"; archive.checked = d.archived; const label = node("label", null, "pages-check"); label.append(archive, node("span", "Lưu trữ trang"));
    v.count = literal("small", "", "pages-note");
    const save = button("Lưu phiên bản", () => savePage(v)), check = button("Kiểm tra lần lưu", () => receipt(v)), retry = button("Gửi lại lần lưu", () => submit(v));
    const actions = node("div", null, "pages-actions"); actions.append(save, check, retry);
    const preview = node("details", null, "pages-preview"), summary = node("summary", "Xem trước Markdown"), rendered = literal("div", "", "msg bot"); preview.append(summary, rendered);
    const previewText = () => { if (preview.open) { rendered.replaceChildren(); AURORA_MD.render(rendered, d.text, []); } };
    preview.ontoggle = previewText;
    v.extra = node("section", null, "pages-history");
    body.append(titleLabel, title, textLabel, text, v.count, label, actions, preview, v.extra);
    v.controls = () => {
      const pending = !!d.pending, invalid = !d.title.trim() || size(d.title.trim()) > 512 || /[\u0000-\u001f\u007f-\u009f]/.test(d.title) || size(d.text) > MAX || d.text.includes("\0");
      title.disabled = text.disabled = archive.disabled = v.busy || pending; archive.disabled ||= !d.id;
      save.disabled = v.busy || pending || invalid || !!v.conflicting;
      check.hidden = retry.hidden = !pending; check.disabled = retry.disabled = v.busy;
      history.disabled = !d.id || v.busy;
      v.count.textContent = size(d.text).toLocaleString() + " / 65,536 bytes";
      v.count.classList.toggle("pages-error", size(d.text) > MAX);
    };
    const changed = () => { d.title = title.value; d.text = text.value; d.archived = archive.checked; const saved = remember(v); status(v, saved ? "Bản nháp đã lưu trên thiết bị này." : "Không lưu được bản nháp trên thiết bị. Hãy tải bản nháp trước khi đóng hoặc tải lại."); v.controls(); previewText(); };
    title.oninput = text.oninput = archive.onchange = changed;
    if (v.latest && d.baseRevision !== v.latest.revision && !d.pending) conflict(v);
    v.controls(); title.focus();
  }
  function conflict(v) {
    v.conflicting = true; v.conflict.replaceChildren(node("p", "Trang đã có phiên bản mới. Bản nháp của bạn vẫn được giữ."));
    const newer = node("details"), pre = literal("pre", v.latest.text); newer.append(node("summary", "Xem bản mới trên máy chủ"), literal("b", v.latest.title), pre);
    const use = button("Tiếp tục với bản nháp", () => { if (v.busy || v.draft.pending) return; v.draft.baseRevision = v.latest.revision; v.conflicting = false; remember(v); v.conflict.replaceChildren(); v.controls(); status(v, "Đã chọn bản nháp. Bấm Lưu phiên bản để tạo bản mới."); });
    v.conflict.append(newer, use);
  }
  async function conflictRefresh(v) {
    try { const data = await request(v, "/pages/" + v.draft.id); if (!validPage(data.page) || data.page.id !== v.draft.id) throw new Error("page"); v.latest = data.page; conflict(v); }
    catch { if (active(v)) { v.conflicting = true; v.conflict.replaceChildren(node("p", "Chưa tải được bản mới. Bản nháp vẫn được giữ."), button("Tải bản mới", () => conflictRefresh(v).then(() => active(v) && v.controls()))); } }
  }
  function savePage(v) {
    if (!active(v) || v.busy || v.draft.pending || v.conflicting) return;
    const d = v.draft;
    d.title = d.title.trim();
    let nonce;
    try { nonce = [...crypto.getRandomValues(new Uint8Array(16))].map(n => n.toString(16).padStart(2, "0")).join(""); }
    catch { status(v, "Không tạo được mã biên nhận. Chưa gửi lần lưu; hãy tải bản nháp."); return; }
    d.pending = { workspaceId: v.scope, id: d.id, baseRevision: d.baseRevision, title: d.title, text: d.text, archived: d.archived, clientNonce: nonce };
    if (!remember(v)) { d.pending = null; remember(v); status(v, "Không thể ghi biên nhận trên thiết bị. Chưa gửi lần lưu; hãy tải bản nháp."); v.controls(); return; }
    void submit(v);
  }
  async function accepted(v, page, input) {
    if (!active(v) || !sameInput(page, input)) throw new Error("ack");
    const d = v.draft;
    // Keep the immutable receipt if cleanup fails; reopening resolves it by GET.
    if (!forget(v.scope, d.id)) { status(v, "Đã lưu nhưng chưa xoá được biên nhận trên thiết bị. Hãy kiểm tra lại lần lưu."); return; }
    v.draft = { workspaceId: v.scope, id: page.id, baseRevision: page.revision, title: page.title, text: page.text, archived: page.archived, pending: null };
    v.latest = page; v.conflicting = false; editor(v); status(v, "Đã lưu phiên bản " + page.revision + ".");
    // A recovered acknowledgement can predate another writer's latest version.
    await conflictRefreshIfNewer(v, page);
  }
  async function conflictRefreshIfNewer(v, page) {
    try { const data = await request(v, "/pages/" + page.id); if (!validPage(data.page) || data.page.id !== page.id) throw new Error("page"); if (data.page.revision !== page.revision) { v.latest = data.page; conflict(v); v.controls(); } }
    catch { if (active(v)) status(v, "Đã lưu phiên bản. Chưa kiểm tra được bản mới nhất; lần lưu tiếp theo sẽ kiểm tra xung đột."); }
  }
  async function submit(v) {
    if (!active(v) || v.busy || !v.draft.pending) return;
    if (!remember(v)) { status(v, "Không thể ghi biên nhận trên thiết bị. Chưa gửi lần lưu; hãy tải bản nháp."); return; }
    const input = v.draft.pending; v.busy = true; v.controls(); status(v, "Đang lưu phiên bản…");
    try { const data = await request(v, "/pages/save", input); await accepted(v, data.page, input); }
    catch (e) {
      if (!active(v)) return;
      if ([400, 404, 409, 423, 507].includes(e.status)) {
        v.draft.pending = null; remember(v);
        if (e.status === 409 && v.draft.id) { status(v, "Lần lưu bị xung đột."); await conflictRefresh(v); }
        else status(v, e.status === 507 ? "Đã đạt giới hạn 64 trang, kể cả trang lưu trữ." : "Máy chủ từ chối lần lưu. Bản nháp vẫn được giữ.");
      } else status(v, "Chưa xác nhận được lần lưu. Kiểm tra biên nhận trước khi gửi lại; bản nháp vẫn được giữ.");
    } finally { if (active(v)) { v.busy = false; v.controls(); } }
  }
  async function receipt(v) {
    if (!active(v) || v.busy || !v.draft.pending) return;
    const input = v.draft.pending; v.busy = true; v.controls(); status(v, "Đang kiểm tra lần lưu…");
    try { const data = await request(v, "/page-receipts/" + encodeURIComponent(input.clientNonce)); if (data.found === true) await accepted(v, data.page, input); else if (data.found === false) status(v, "Chưa có biên nhận. Bạn có thể gửi lại đúng lần lưu này."); else throw new Error("receipt"); }
    catch { status(v, "Chưa kiểm tra được lần lưu. Biên nhận và bản nháp vẫn được giữ."); }
    finally { if (active(v)) { v.busy = false; v.controls(); } }
  }
  async function historyView(v, before) {
    if (!active(v) || v.busy) return;
    v.busy = true; v.controls();
    try {
      const data = await request(v, "/pages/" + v.draft.id + "/history" + (before ? "?before=" + before : ""));
      if (!Array.isArray(data.versions)) throw new Error("history");
      if (!before) v.extra.replaceChildren(node("h3", "Lịch sử phiên bản")); else v.extra.querySelector(".pages-more")?.remove();
      for (const p of data.versions) { if (!validRevision(p.revision) || p.id !== v.draft.id) throw new Error("history"); const b = button("", () => revisionView(v, p.revision)); b.className = "pages-row"; b.append(node("span", "Phiên bản " + p.revision), literal("small", p.title)); v.extra.append(b); }
      if (data.versions.length === 50) { const b = button("Xem thêm phiên bản", () => historyView(v, data.versions.at(-1).revision)); b.className = "pages-more"; v.extra.append(b); }
      v.extra.querySelector("button")?.focus();
    } catch { status(v, "Chưa tải được lịch sử phiên bản. Hãy thử lại."); }
    finally { if (active(v)) { v.busy = false; v.controls(); } }
  }
  async function revisionView(v, revision) {
    if (!active(v) || v.busy) return;
    v.busy = true; v.controls();
    try {
      const p = (await request(v, "/pages/" + v.draft.id + "?revision=" + revision)).page;
      if (!validPage(p) || p.id !== v.draft.id || p.revision !== revision) throw new Error("revision");
      const rendered = literal("div", "", "msg bot"); AURORA_MD.render(rendered, p.text, []);
      const restore = button("Dùng phiên bản này làm bản nháp", () => { if (v.busy || v.draft.pending) return; v.draft.title = p.title; v.draft.text = p.text; v.draft.archived = p.archived; remember(v); v.extra.replaceChildren(); editor(v); status(v, "Bản nháp đã được thay bằng phiên bản đã chọn. Bấm Lưu phiên bản để ghi lại."); }); restore.disabled = !!v.draft.pending;
      const actions = node("div", null, "pages-actions"); actions.append(button("Lịch sử phiên bản", () => historyView(v)), button("Tải Markdown phiên bản này", () => exportRevision(v, p)), restore);
      v.extra.replaceChildren(node("h3", "Phiên bản " + revision), literal("h4", p.title), actions, rendered); actions.querySelector("button").focus();
    } catch { status(v, "Chưa tải được phiên bản. Hãy thử lại."); }
    finally { if (active(v)) { v.busy = false; v.controls(); } }
  }
  async function exportRevision(v, p) {
    if (!active(v) || v.busy) return;
    v.busy = true; v.controls();
    try {
      const r = await request(v, "/pages/" + p.id + "/export?revision=" + p.revision, undefined, true);
      if (r.headers.get("X-Aurora-Workspace") !== v.scope || r.headers.get("X-Aurora-Page") !== p.id || r.headers.get("X-Aurora-Revision") !== String(p.revision) || r.headers.get("X-Aurora-SHA256") !== p.sha256 || r.headers.get("Content-Length") !== String(size(p.text))) throw new Error("export");
      const text = await r.text();
      if (!active(v) || text !== p.text) throw new Error("export");
      download(text, p.id + "-v" + p.revision + ".md"); status(v, "Đã chuẩn bị tệp Markdown đúng phiên bản.");
    } catch { status(v, "Không tải được đúng phiên bản. Chưa tạo tệp tải xuống."); }
    finally { if (active(v)) { v.busy = false; v.controls(); } }
  }
  function open(options) {
    close(); getScope = options.getScope;
    if (!getScope()) return;
    returnFocus = options.returnFocus || document.activeElement;
    dialog = node("dialog", null, "pages-dialog"); dialog.id = "pagesDialog"; dialog.setAttribute("aria-labelledby", "pagesHeading");
    const heading = node("h2", "Thư viện trang"); heading.id = "pagesHeading";
    const top = node("header", null, "pages-top"); top.append(heading, button("Đóng thư viện", close));
    body = node("div", null, "pages-body"); dialog.append(top, body); document.body.append(dialog);
    dialog.addEventListener("keydown", e => {
      if (e.key !== "Tab" || e.target.closest("dialog") !== dialog) return;
      const stops = [...dialog.querySelectorAll("button,input,textarea,summary,a[href],[tabindex]")].filter(n => !n.disabled && n.tabIndex >= 0 && n.getClientRects().length);
      const first = stops[0], last = stops.at(-1);
      if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last?.focus(); }
      else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first?.focus(); }
    });
    dialog.addEventListener("cancel", e => { e.preventDefault(); close(); }); dialog.showModal();
    if (options.create) void edit(""); else void list();
  }
  self.AURORA_PAGES = { open, close };
})();
