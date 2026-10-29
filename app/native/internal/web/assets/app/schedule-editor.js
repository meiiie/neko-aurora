"use strict";
(() => {
  function create({schedule, workspaceId, save, reload, cancel}) {
    const el = (tag, text) => { const node = document.createElement(tag); if (text) node.textContent = text; return node; };
    const form = el("form"); form.className = "schedule-editor";
    form.setAttribute("aria-label", "Sửa lịch hẹn");
    const field = (label, input) => { const row = el("label"); input.setAttribute("aria-label",label); row.append(el("span", label), input); form.append(row); return row; };
    const text = el("textarea"); text.value = schedule.text; text.required = true; text.maxLength = 4000; text.rows = 3;
    text.oninput = () => text.setCustomValidity([...text.value].length > 1000 ? "Tối đa 1.000 ký tự." : "");
    field("Nội dung lịch", text);
    const at = el("input"); at.type = "time"; at.required = true; at.value = schedule.at; const atRow = field("Giờ hẹn", at);
    const timezone = el("select"), defaultZone = "Asia/Ho_Chi_Minh", zones = new Set([defaultZone,"UTC","Europe/Kyiv","America/New_York","Europe/London","Asia/Tokyo","Asia/Singapore","Australia/Sydney",schedule.timezone||defaultZone]);
    try { for (const zone of Intl.supportedValuesOf("timeZone")) zones.add(zone); } catch {}
    for (const zone of [...zones].sort()) { const option = el("option",zone); option.value = zone; timezone.append(option); }
    timezone.value = schedule.timezone||defaultZone; const timezoneRow = field("Múi giờ",timezone);
    const repeat = el("select");
    for (const [value, name] of [["daily","Mỗi ngày"],["weekdays","Thứ 2–6"],["once","Một lần"],["interval","Theo khoảng thời gian"]]) { const option = el("option",name); option.value = value; repeat.append(option); }
    repeat.value = schedule.repeat; field("Lặp lại", repeat);
    const date = el("input"); date.type = "date"; date.value = schedule.date || ""; const dateRow = field("Ngày hẹn", date);
    const interval = el("input"); interval.type = "number"; interval.min = "15"; interval.max = "10080"; interval.step = "1"; interval.value = schedule.intervalMinutes || 60;
    const intervalRow = field("Khoảng lặp (phút)",interval);
    const syncDate = () => { const once = repeat.value === "once", timed = repeat.value === "interval"; dateRow.hidden = !once; date.disabled = !once; date.required = once; intervalRow.hidden = !timed; interval.disabled = !timed; interval.required = timed; atRow.hidden = timezoneRow.hidden = timed; at.disabled = timezone.disabled = timed; at.required = !timed; if (typeof hint !== "undefined") hint.textContent = timed ? intervalHint : calendarHint; };
    repeat.onchange = syncDate;
    const calendarHint = "Giờ hẹn theo múi giờ đã chọn. Khi đổi giờ mùa hè, giờ bị lặp chỉ chạy một lần; giờ bị bỏ qua sẽ chạy sau đó. Thay đổi áp dụng cho lượt tiếp theo. Nếu giờ mới đã qua hoặc lịch đã chạy hôm nay, lịch lặp bắt đầu từ ngày mai. Lượt đã nhận giữ nội dung cũ.";
    const intervalHint = "Từ 15 đến 10.080 phút. Lượt đầu sau một khoảng từ lúc tạo hoặc đổi khoảng lặp. Chỉ sửa nội dung hay tạm dừng không đổi mốc. Khi máy hoạt động lại, chỉ nhận một lượt đến hạn gần nhất; không chạy dồn.";
    const hint = el("p"); hint.className = "caption"; syncDate();
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
      const body = {workspaceId, revision:schedule.revision, text:text.value, at:repeat.value === "interval" ? "" : at.value, timezone:timezone.value, repeat:repeat.value, date:repeat.value === "once" ? date.value : ""};
      if (repeat.value === "interval") body.intervalMinutes = Number(interval.value);
      lock(true); status.textContent = "Đang lưu lịch…"; refresh.hidden = true;
      try {
        await save(body);
        saved = true;
        if (await reload() === false) { lock(false); submit.disabled = true; refresh.hidden = false; status.textContent = "Đã lưu lịch. Chưa tải lại được danh sách; bấm Tải lại lịch để kiểm tra."; }
      } catch (error) {
        lock(false);
        refresh.hidden = error.status === 400;
        status.textContent = error.status === 409 ? "Lịch đã thay đổi. Nội dung chỉnh sửa vẫn còn; tải lại để xem phiên bản mới." : error.status === 400 ? "Thông tin lịch chưa hợp lệ. Kiểm tra nội dung, múi giờ, giờ và ngày hẹn trong tương lai." : error.status === 404 ? "Lịch không còn tồn tại. Tải lại danh sách để kiểm tra." : "Chưa nhận được xác nhận. Nội dung vẫn còn; tải lại lịch để kiểm tra trước khi thử lại.";
        if (error.status === 404) submit.disabled = true;
      }
    };
    actions.append(submit,back,refresh); form.append(hint,status,actions);
    return {element:form, focus:() => text.focus()};
  }
  globalThis.AURORA_SCHEDULE_EDITOR = {create};
})();
