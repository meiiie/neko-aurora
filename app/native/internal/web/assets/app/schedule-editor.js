"use strict";
(() => {
  function create({schedule, workspaceId, save, reload, cancel}) {
    const el = (tag, text) => { const node = document.createElement(tag); if (text) node.textContent = text; return node; };
    const form = el("form"); form.className = "schedule-editor";
    form.setAttribute("aria-label", "Sửa lịch hẹn");
    const field = (label, input) => { const row = el("label"); row.append(el("span", label), input); form.append(row); return row; };
    const text = el("textarea"); text.value = schedule.text; text.required = true; text.maxLength = 4000; text.rows = 3;
    text.oninput = () => text.setCustomValidity([...text.value].length > 1000 ? "Tối đa 1.000 ký tự." : "");
    field("Nội dung lịch", text);
    const at = el("input"); at.type = "time"; at.required = true; at.value = schedule.at; field("Giờ (Việt Nam)", at);
    const repeat = el("select");
    for (const [value, name] of [["daily","Mỗi ngày"],["weekdays","Thứ 2–6"],["once","Một lần"]]) { const option = el("option",name); option.value = value; repeat.append(option); }
    repeat.value = schedule.repeat; field("Lặp lại", repeat);
    const date = el("input"); date.type = "date"; date.value = schedule.date || ""; const dateRow = field("Ngày hẹn", date);
    const syncDate = () => { const once = repeat.value === "once"; dateRow.hidden = !once; date.disabled = !once; date.required = once; };
    repeat.onchange = syncDate; syncDate();
    const hint = el("p", "Thay đổi áp dụng cho lượt tiếp theo. Nếu giờ mới đã qua hoặc lịch đã chạy hôm nay, lịch lặp bắt đầu từ ngày mai. Lượt đã nhận giữ nội dung cũ."); hint.className = "caption";
    const status = el("p"); status.className = "schedule-edit-status"; status.setAttribute("role","status");
    const actions = el("div"); actions.className = "schedule-edit-actions";
    const submit = el("button","Lưu lịch"), back = el("button","Hủy chỉnh sửa"), refresh = el("button","Tải lại lịch");
    submit.type = "submit"; back.type = refresh.type = "button";
    submit.className = "pill"; back.className = refresh.className = "pill soft"; refresh.hidden = true;
    back.onclick = cancel;
    let busy = false, saved = false;
    const lock = value => { busy = value; form.querySelectorAll("input,textarea,select,button").forEach(x => { x.disabled = value; }); if (!value) { syncDate(); submit.disabled = saved; } };
    refresh.onclick = async () => { lock(true); if (await reload() === false) { lock(false); status.textContent = "Chưa tải lại được lịch. Nội dung chỉnh sửa vẫn còn."; } };
    form.onsubmit = async event => {
      event.preventDefault(); if (busy || saved) return;
      text.oninput(); if (!form.reportValidity()) return;
      const body = {workspaceId, revision:schedule.revision, text:text.value, at:at.value, repeat:repeat.value, date:repeat.value === "once" ? date.value : ""};
      lock(true); status.textContent = "Đang lưu lịch…"; refresh.hidden = true;
      try {
        await save(body);
        saved = true;
        if (await reload() === false) { lock(false); submit.disabled = true; refresh.hidden = false; status.textContent = "Đã lưu lịch. Chưa tải lại được danh sách; bấm Tải lại lịch để kiểm tra."; }
      } catch (error) {
        lock(false);
        refresh.hidden = error.status === 400;
        status.textContent = error.status === 409 ? "Lịch đã thay đổi. Nội dung chỉnh sửa vẫn còn; tải lại để xem phiên bản mới." : error.status === 400 ? "Thông tin lịch chưa hợp lệ. Kiểm tra nội dung, giờ và ngày hẹn trong tương lai." : error.status === 404 ? "Lịch không còn tồn tại. Tải lại danh sách để kiểm tra." : "Chưa nhận được xác nhận. Nội dung vẫn còn; tải lại lịch để kiểm tra trước khi thử lại.";
        if (error.status === 404) submit.disabled = true;
      }
    };
    actions.append(submit,back,refresh); form.append(hint,status,actions);
    return {element:form, focus:() => text.focus()};
  }
  globalThis.AURORA_SCHEDULE_EDITOR = {create};
})();
