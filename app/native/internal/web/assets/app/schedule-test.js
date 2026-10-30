"use strict";
(() => {
  const prefix = "aurora.schedule-test.v1.";
  const key = (scope, bot, id) => prefix + encodeURIComponent(JSON.stringify([scope, bot, id]));
  function valid(record, scope, bot) {
    return record && record.workspaceId === scope && record.botId === bot && typeof record.id === "string" && record.id.length > 0 && record.id.length <= 128 &&
      Number.isSafeInteger(record.revision) && record.revision > 0 && /^[A-Za-z0-9_-]{8,100}$/.test(record.clientNonce) && ["pending", "received", "rejected"].includes(record.state) &&
      (record.state !== "received" || typeof record.requestId === "string" && record.requestId.length > 0 && record.requestId.length <= 128);
  }
  function create(storage) {
    function get(scope, bot, id) {
      const raw = storage.getItem(key(scope, bot, id));
      if (raw === null) return null;
      if (raw.length > 4096) throw Error("invalid_test_record");
      const record = JSON.parse(raw);
      if (!valid(record, scope, bot) || record.id !== id) throw Error("invalid_test_record");
      return record;
    }
    function list(scope, bot) {
      const records = [];
      for (let i = 0; i < storage.length; i++) {
        const k = storage.key(i);
        if (!k?.startsWith(prefix)) continue;
        let parts; try { parts = JSON.parse(decodeURIComponent(k.slice(prefix.length))); } catch { continue; }
        if (Array.isArray(parts) && parts.length === 3 && parts[0] === scope && parts[1] === bot) { const record = get(scope, bot, parts[2]); if (record) records.push(record); }
      }
      return records;
    }
    function put(record) {
      if (!valid(record, record.workspaceId, record.botId)) throw Error("invalid_test_record");
      const k = key(record.workspaceId, record.botId, record.id), raw = JSON.stringify(record);
      if (storage.getItem(k) === null && list(record.workspaceId, record.botId).length >= 128) throw Error("test_record_limit");
      storage.setItem(k, raw);
      if (storage.getItem(k) !== raw) throw Error("test_record_not_saved");
    }
    return {get, list, put};
  }
  function mount({schedule, id, botId, workspaceId, store, api, isCurrent, openResult}) {
    const el = (tag, text) => { const n = document.createElement(tag); if (text) n.textContent = text; return n; };
    const root = el("div"); root.className = "schedule-test"; root.setAttribute("role", "group"); root.setAttribute("aria-label", "Chạy thử lịch");
    const actions = el("div"); actions.className = "schedule-controls";
    const run = el("button", "Chạy thử"), check = el("button", "Kiểm tra lượt thử"), view = el("button", "Mở lượt thử");
    for (const b of [run, check, view]) { b.type = "button"; b.className = "pill soft"; actions.append(b); }
    const status = el("p"); status.className = "caption"; status.setAttribute("role", "status"); root.append(actions, status);
    let record = null, busy = false, broken = false;
    const current = () => root.isConnected && isCurrent();
    try { record = store.get(workspaceId, botId, id); } catch { broken = true; status.textContent = "Chưa đọc được lượt thử đã lưu. Không gửi thêm để tránh chạy lặp."; }
    function render() {
      run.disabled = broken || busy || !schedule && record?.state !== "pending";
      run.hidden = !schedule && record?.state !== "pending";
      run.textContent = record?.state === "pending" ? "Thử lại lượt này" : record ? "Chạy thử mới" : "Chạy thử";
      check.hidden = !record; check.disabled = busy || broken;
      view.hidden = record?.state !== "received"; view.disabled = busy;
    }
    function receive(out, attempted) {
      if (!out?.found || typeof out.requestId !== "string" || !out.requestId || out.botId !== botId) throw Error("invalid_test_receipt");
      const saved = store.get(workspaceId, botId, id);
      if (saved?.clientNonce !== attempted.clientNonce) return;
      record = {...attempted, state:"received", requestId:out.requestId};
      let persisted = true; try { store.put(record); } catch { persisted = false; }
      if (current()) status.textContent = "Đã nhận lượt thử. Kết quả sẽ xuất hiện trong chat." + (persisted ? "" : " Chưa lưu được biên nhận trên thiết bị; kiểm tra lại sau khi tải trang.");
    }
    function failure(error) {
      if (!current()) return;
      status.textContent = error.status === 429 ? "Hết hạn mức hoặc gửi quá nhanh. Chưa nhận lượt thử mới; có thể kiểm tra biên nhận." : error.status === 409 ? "Lịch hoặc tài khoản đã thay đổi. Kiểm tra biên nhận và tải lại lịch trước khi tạo lượt mới." : error.status === 404 ? "Lịch đã bị xóa. Vẫn có thể kiểm tra biên nhận của lượt cũ." : "Chưa nhận được xác nhận. Kiểm tra biên nhận hoặc thử lại cùng lượt; không tự chạy thêm.";
    }
    check.onclick = async () => {
      if (busy || broken || !record || !current()) return;
      busy = true; render(); const attempted = record;
      const query = new URLSearchParams({workspaceId, revision:String(attempted.revision), clientNonce:attempted.clientNonce});
      try {
        const out = await api(`/schedules/${encodeURIComponent(id)}/test?${query}`);
        if (out.found) receive(out, attempted);
        else if (current()) status.textContent = "Chưa có biên nhận cho lượt này. Việc kiểm tra không chạy công việc.";
      } catch (error) { failure(error); }
      finally { busy = false; if (current()) render(); }
    };
    run.onclick = async () => {
      if (busy || broken || !current()) return;
      try {
        const saved = store.get(workspaceId, botId, id);
        if (saved?.clientNonce !== record?.clientNonce || saved?.state !== record?.state) { record = saved; render(); status.textContent = "Lượt thử đã thay đổi trên thiết bị. Kiểm tra lại trước khi gửi."; return; }
      } catch { broken = true; render(); status.textContent = "Chưa đọc được lượt thử đã lưu. Chưa gửi công việc."; return; }
      if (record?.state !== "pending") {
        if (!schedule || !confirm("Chạy một lượt ngay với nội dung lịch này? Lượt thử dùng hạn mức và quyền duyệt như công việc thông thường. Lịch định kỳ và trạng thái tạm dừng được giữ nguyên.")) return;
        const next = {workspaceId, botId, id, revision:schedule.revision, clientNonce:globalThis.AURORA_OUTBOX.nonce(), state:"pending"};
        try { store.put(next); } catch { status.textContent = "Chưa lưu được mã lượt thử trên thiết bị. Chưa gửi công việc."; return; }
        record = next;
      }
      busy = true; render(); status.textContent = "Đang gửi lượt thử…"; const attempted = record;
      try {
        const out = await api(`/schedules/${encodeURIComponent(id)}/test`, {workspaceId, revision:attempted.revision, clientNonce:attempted.clientNonce});
        receive(out, attempted);
      } catch (error) {
        if ([400,404,409,429].includes(error.status)) {
          try { const saved = store.get(workspaceId, botId, id); if (saved?.clientNonce === attempted.clientNonce && saved.state === "pending") { record = {...attempted, state:"rejected"}; store.put(record); } } catch {}
        }
        failure(error);
      } finally { busy = false; if (current()) render(); }
    };
    view.onclick = () => { if (current() && record?.state === "received") openResult(record.requestId); };
    if (record?.state === "received") status.textContent = "Đã nhận lượt thử. Mở chat để xem tiến độ hoặc kết quả.";
    else if (record) status.textContent = "Lượt thử trước chưa được xác nhận trong màn hình này. Kiểm tra biên nhận trước khi gửi lại.";
    render(); return root;
  }
  globalThis.AURORA_SCHEDULE_TEST = {create, mount};
})();
