"use strict";
(() => {
  function mount({root, context, insert, notice}) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "selection-excerpt pill hidden";
    button.textContent = "Thêm vào chat";
    button.setAttribute("aria-label", "Thêm đoạn đã chọn vào nháp");
    document.body.append(button);
    let captured = null, frame = 0;
    const element = node => node?.nodeType === Node.ELEMENT_NODE ? node : node?.parentElement;
    const scope = () => JSON.stringify(context());
    const hasSelection = () => { const selected = getSelection(); return !!selected && !selected.isCollapsed && root.contains(selected.anchorNode); };
    const interactive = node => element(node)?.closest("button,input,textarea,select,[contenteditable=true]");
    function clear() { captured = null; button.classList.add("hidden"); }
    function reset() {
      clear();
      const selected = getSelection();
      if (selected && (root.contains(selected.anchorNode) || root.contains(selected.focusNode))) selected.removeAllRanges();
    }
    function beforeRender() {
      const selected = getSelection();
      if (!selected || selected.isCollapsed || selected.rangeCount !== 1) return;
      const anchor = selected.anchorNode, focus = selected.focusNode;
      const source = element(anchor)?.closest(".message-item .msg");
      if (!source || !root.contains(source) || !source.contains(focus)) return;
      const a = selected.anchorOffset, f = selected.focusOffset, text = selected.toString(), originalScope = scope();
      return () => {
        if (originalScope !== scope() || !anchor.isConnected || !focus.isConnected || !root.contains(source)) return;
        try {
          const current = getSelection();
          current.setBaseAndExtent(anchor, a, focus, f);
          if (current.toString() !== text) current.removeAllRanges();
        } catch { reset(); }
      };
    }
    function valid() {
      return captured && captured.scope === scope() && captured.source.isConnected && root.contains(captured.source) && captured.range.toString() === captured.text && captured.source.contains(captured.range.commonAncestorContainer);
    }
    function position() {
      if (!valid()) { clear(); return; }
      const r = captured.range.getBoundingClientRect();
      if (!r.width && !r.height) { clear(); return; }
      const bounds = root.getBoundingClientRect();
      if (r.bottom < Math.max(0, bounds.top) || r.top > Math.min(innerHeight, bounds.bottom) || r.right < Math.max(0, bounds.left) || r.left > Math.min(innerWidth, bounds.right)) { button.classList.add("hidden"); return; }
      button.classList.remove("hidden");
      const w = button.offsetWidth, h = button.offsetHeight;
      button.style.left = Math.max(8, Math.min(innerWidth - w - 8, r.left + r.width / 2 - w / 2)) + "px";
      button.style.top = Math.max(8, Math.min(innerHeight - h - 8, r.bottom + 8)) + "px";
    }
    function read() {
      if (!context().agent || root.closest(".screen.off")) { clear(); return; }
      const selected = getSelection();
      if (!selected || selected.rangeCount !== 1 || selected.isCollapsed) {
        if (document.activeElement === button && valid()) position(); else clear();
        return;
      }
      const range = selected.getRangeAt(0), start = element(range.startContainer), end = element(range.endContainer);
      const source = start?.closest(".message-item .msg"), item = source?.closest(".message-item");
      if (!item || !root.contains(item) || source !== end?.closest(".message-item .msg") || interactive(range.startContainer) || interactive(range.endContainer) || range.cloneContents().querySelector("button,input,textarea,select") || !range.toString().trim()) { clear(); return; }
      captured = {scope:scope(), source, range:range.cloneRange(), text:range.toString(), id:item.dataset.messageId};
      position();
    }
    function queue() { if (!frame) frame = requestAnimationFrame(() => { frame = 0; read(); }); }
    button.addEventListener("pointerdown", event => event.preventDefault());
    button.onclick = () => {
      if (!valid()) { clear(); return; }
      const item = captured;
      if ([...item.text].length > 2000) { notice("Chọn tối đa 2.000 ký tự để thêm vào nháp."); return; }
      if (!insert(item.id, item.text)) return;
      clear(); getSelection()?.removeAllRanges();
    };
    document.addEventListener("selectionchange", queue);
    document.addEventListener("pointerup", queue);
    document.addEventListener("pointerdown", event => {
      if (event.target !== button && !root.contains(event.target)) reset();
    });
    document.addEventListener("keydown", event => {
      if (event.key === "Escape" && captured) {
        const target = captured.source.closest(".message-item"), focused = document.activeElement === button;
        clear(); getSelection()?.removeAllRanges(); if (focused) target?.focus({preventScroll:true});
      }
    });
    root.addEventListener("scroll", () => { if (captured) position(); }, true);
    addEventListener("resize", () => { if (captured) position(); });
    return {clear:reset, beforeRender, hasSelection};
  }
  globalThis.AURORA_SELECTION_EXCERPT = {mount};
})();
