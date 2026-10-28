"use strict";
(() => {
  const el = (tag, text, cls) => { const e = document.createElement(tag); if (text) e.textContent = text; if (cls) e.className = cls; return e; };
  function mount(container, bot, api, saved) {
    const open = el("button", "Nhập bộ nhớ", "pill soft");
    open.type = "button"; open.id = "memImport";
    container.append(open);
    open.onclick = () => {
      if (container.parentNode.querySelector(".memory-import")) return;
      const form = el("form", "", "memory-import card pad");
      const label = el("label", "Dán bộ nhớ từ trợ lý khác · mỗi dòng một điều");
      const input = el("textarea", "", "ext-in"); input.rows = 6; input.maxLength = 32768;
      input.setAttribute("aria-label", "Nội dung bộ nhớ cần nhập"); label.append(input);
      const note = el("p", "Chỉ nhập sở thích và thông tin bạn muốn bot này nhớ. Không nhập mật khẩu, mã hay khóa. Bạn được xem và chọn trước khi lưu.", "caption");
      const status = el("p"); status.setAttribute("role", "status");
      const review = el("div");
      const preview = el("button", "Xem trước", "pill"); preview.type = "submit";
      const cancel = el("button", "Hủy", "pill soft"); cancel.type = "button";
      cancel.onclick = () => { form.remove(); open.focus(); };
      form.append(label, note, status, review, preview, cancel); container.after(form); input.focus();
      input.oninput = () => { review.replaceChildren(); preview.hidden = false; };
      form.onsubmit = async event => {
        event.preventDefault(); if (!input.value.trim()) return;
        const requested = input.value;
        preview.disabled = true; status.textContent = "Đang kiểm tra…";
        try {
          const result = await api(`/agents/${bot}/memory/import/preview`, {text:requested});
          if (!form.isConnected || input.value !== requested) { status.textContent = "Nội dung đã đổi. Bấm Xem trước để kiểm tra lại."; return; }
          review.replaceChildren();
          status.textContent = `${result.items.length} mục có thể nhập · ${result.duplicates} trùng · ${result.rejected} không phù hợp · còn ${result.remaining} chỗ.`;
          const picks = result.items.map((text, i) => {
            const row = el("label", "", "memory-import-pick"), box = el("input");
            box.type = "checkbox"; box.checked = i < result.remaining; const fact = el("span", text); fact.dataset.noi18n = ""; row.append(box, fact); review.append(row);
            return {box,text};
          });
          if (picks.length) {
            const apply = el("button", "Lưu các mục đã chọn", "pill"); apply.type = "button";
            const update = () => { const count = picks.filter(x => x.box.checked).length; apply.disabled = !count || count > result.remaining; };
            picks.forEach(x => x.box.onchange = update); update(); review.append(apply);
            apply.onclick = async () => {
              apply.disabled = true;
              try {
                const value = await api(`/agents/${bot}/memory/import`, {revision:result.revision,items:picks.filter(x => x.box.checked).map(x => x.text)});
                if (form.isConnected) { input.value = ""; form.remove(); saved(value.added); }
              } catch (error) {
                if (!form.isConnected) return;
                status.textContent = error.status === 409 ? "Bộ nhớ đã thay đổi. Bấm Xem trước để kiểm tra lại." : "Chưa nhận được xác nhận. Bấm Xem trước để kiểm tra bộ nhớ trước khi lưu lại.";
                review.replaceChildren(); preview.hidden = false;
              }
            };
          }
          preview.hidden = true;
        } catch { status.textContent = "Chưa đọc được nội dung. Dùng tối đa 100 dòng, mỗi điều tối đa 300 ký tự."; }
        finally { preview.disabled = false; }
      };
    };
  }
  window.AURORA_MEMORY_IMPORT = {mount};
})();
