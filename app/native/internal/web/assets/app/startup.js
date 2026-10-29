"use strict";
(() => {
  let phase = "loading", blocked = false;
  function paint() {
    const panel = document.getElementById("startup");
    if (!panel) return;
    panel.classList.toggle("hidden", phase === "ready");
    document.getElementById("startupStatus").textContent = phase === "failed"
      ? "Chưa mở được Neko Aurora. Hãy tải lại để thử kết nối."
      : "Đang mở Neko Aurora…";
    document.getElementById("startupRetry").onclick = () => location.reload();
  }
  function failed() { if (phase !== "ready") { phase = "failed"; paint(); } }
  window.AURORA_STARTUP = {
    get blocked() { return blocked; },
    ready() { if (!blocked) phase = "ready"; paint(); },
    failed,
  };
  addEventListener("error", event => {
    if (phase !== "ready" && (event.target?.tagName === "SCRIPT" || event.filename)) { blocked = true; failed(); }
  }, true);
  addEventListener("load", () => { if (!window.__AURORA) failed(); });
  document.addEventListener("DOMContentLoaded", paint, {once:true});
})();
