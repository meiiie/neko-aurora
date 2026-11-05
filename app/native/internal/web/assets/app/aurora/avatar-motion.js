(() => {
  "use strict";
  const key = "aurora.avatar-motion.v1", tracked = new Map(), pool = new Map(), moving = new Set();
  const reduced = matchMedia("(prefers-reduced-motion: reduce)");
  const fine = matchMedia("(hover: hover) and (pointer: fine)");
  let blinkTimer = null, frame = null, previousFrame = 0, pointer = null, ambientUntil = 0;
  let mode = "auto";
  try { if (localStorage.getItem(key) === "off") mode = "off"; } catch {}
  const allowed = () => mode !== "off" && !reduced.matches && !document.hidden;
  const active = record => allowed() && record.visible;
  function resetGaze(record) {
    record.x = record.y = record.tx = record.ty = 0;
    record.node.style.setProperty("--gaze-x", "0px");
    record.node.style.setProperty("--gaze-y", "0px");
    record.node.style.setProperty("--head-turn", "0deg");
    moving.delete(record);
  }
  function update(node) {
    const record = tracked.get(node); if (!record) return;
    node.dataset.motion = active(record) ? "on" : "off";
    if (!active(record)) {
      resetGaze(record); node.dataset.blink = "off"; record.blinkEnd = 0; record.nextBlink = 0;
      node.dataset.gesture = "rest"; record.gestureEnd = 0; record.nextGesture = 0;
    }
  }
  function random(record) {
    record.seed = (Math.imul(record.seed, 1664525) + 1013904223) >>> 0;
    return record.seed / 4294967296;
  }
  function gesture(record, kind, now) {
    record.node.dataset.gesture = kind;
    record.gestureEnd = now + (kind === "greet" ? 560 : 1800);
    record.nextGesture = record.gestureEnd + 14000 + random(record) * 16000;
  }
  function greet(owner) {
    const now = performance.now();
    for (const record of tracked.values()) {
      if (!active(record) || !owner.contains(record.node) || record.node.dataset.avatarState !== "idle" || now < record.greetAfter) continue;
      gesture(record, "greet", now); record.greetAfter = now + 3000;
    }
    scheduleBlink();
  }
  function scheduleBlink() {
    if (blinkTimer !== null) clearTimeout(blinkTimer);
    blinkTimer = null;
    const now = performance.now(); let next = Infinity;
    for (const record of tracked.values()) {
      if (!active(record)) continue;
      if (record.blinkEnd && now >= record.blinkEnd) {
        record.node.dataset.blink = "off"; record.blinkEnd = 0; record.nextBlink = 0;
      }
      if (!record.nextBlink) record.nextBlink = now + 4500 + random(record) * 6500;
      if (!record.blinkEnd && now >= record.nextBlink) {
        record.node.dataset.blink = "on"; record.blinkEnd = now + 180;
      }
      next = Math.min(next, record.blinkEnd || record.nextBlink);
      if (record.gestureEnd && now >= record.gestureEnd) {
        record.node.dataset.gesture = "rest"; record.gestureEnd = 0;
      }
      if (record.node.dataset.avatarState === "idle") {
        if (!record.nextGesture) record.nextGesture = now + 8000 + random(record) * 12000;
        if (!record.gestureEnd && now >= record.nextGesture) {
          const engaged = pointer?.owner?.contains(record.node) || record.node.closest?.(owners)?.matches(":focus-visible");
          if (now < ambientUntil || engaged) record.nextGesture = now + 3000 + random(record) * 5000;
          else {
            gesture(record, random(record) < .6 ? "glance" : "settle", now);
            ambientUntil = record.gestureEnd + 1500;
          }
        }
        next = Math.min(next, record.gestureEnd || record.nextGesture);
      }
    }
    if (next !== Infinity) blinkTimer = setTimeout(scheduleBlink, Math.max(16, next - now));
  }
  function requestFrame() { if (frame === null && moving.size) frame = requestAnimationFrame(tick); }
  function tick(now) {
    frame = null;
    const dt = previousFrame ? Math.min(50, now - previousFrame) : 16; previousFrame = now;
    for (const record of moving) {
      if (!active(record) || !record.node.isConnected) { resetGaze(record); continue; }
      const engaged = pointer?.owner?.contains(record.node) && !["waiting", "error"].includes(record.node.dataset.avatarState);
      record.tx = record.ty = 0;
      if (engaged) {
        if (!record.rect || now - record.rectAt > 200) { record.rect = record.node.getBoundingClientRect(); record.rectAt = now; }
        const rect = record.rect, strength = record.node.dataset.avatarState === "working" ? .45 : 1;
        const direction = value => Math.abs(value) < .08 ? 0 : Math.max(-1, Math.min(1, value));
        record.tx = direction((pointer.x - rect.x - rect.width / 2) / Math.max(24, rect.width)) * 5 * strength;
        record.ty = direction((pointer.y - rect.y - rect.height / 2) / Math.max(24, rect.height)) * 3 * strength;
      }
      const blend = 1 - Math.exp(-dt / (engaged ? 85 : 140));
      record.x += (record.tx - record.x) * blend; record.y += (record.ty - record.y) * blend;
      if (Math.abs(record.x - record.tx) + Math.abs(record.y - record.ty) < .025) {
        record.x = record.tx; record.y = record.ty; moving.delete(record);
      }
      record.node.style.setProperty("--gaze-x", record.x.toFixed(3) + "px");
      record.node.style.setProperty("--gaze-y", record.y.toFixed(3) + "px");
      record.node.style.setProperty("--head-turn", (record.x * .55).toFixed(3) + "deg");
    }
    if (moving.size) requestFrame(); else previousFrame = 0;
  }
  function releasePointer() {
    pointer = null;
    for (const record of tracked.values()) if (record.x || record.y) moving.add(record);
    requestFrame();
  }
  function refresh() {
    for (const node of tracked.keys()) update(node);
    if (!allowed() || !fine.matches) releasePointer();
    if (!moving.size && frame !== null) { cancelAnimationFrame(frame); frame = null; previousFrame = 0; }
    scheduleBlink();
    window.dispatchEvent(new Event("aurora:motionchange"));
  }
  const observer = new IntersectionObserver(entries => {
    for (const { target, isIntersecting } of entries) {
      if (!tracked.has(target)) continue;
      tracked.get(target).visible = isIntersecting; update(target);
    }
    scheduleBlink();
  });
  function setState(node, state) {
    if (!node) return;
    node.dataset.avatarState = ["working", "waiting", "error"].includes(state) ? state : "idle";
    const record = tracked.get(node);
    if (record && node.dataset.avatarState !== "idle") {
      node.dataset.gesture = "rest"; record.gestureEnd = 0; record.nextGesture = 0;
    }
    if (record && (record.x || record.y)) { moving.add(record); requestFrame(); }
  }
  function attach(node, id, state) {
    let seed = 0;
    for (const char of String(id)) seed = (seed * 31 + char.codePointAt(0)) >>> 0;
    for (let i = 0; i < 4; i++) seed = (Math.imul(seed, 1664525) + 1013904223) >>> 0;
    setState(node, state);
    node.style.setProperty("--avatar-work-phase", -((Date.now() + seed) % 3200) / 1000 + "s");
    node.style.setProperty("--avatar-direction", seed % 2 ? "1" : "-1");
    tracked.set(node, { node, seed, visible: false, x: 0, y: 0, tx: 0, ty: 0, nextBlink: 0, blinkEnd: 0, nextGesture: 0, gestureEnd: 0, greetAfter: 0 });
    update(node); observer.observe(node);
    return node;
  }
  function reuse(key, signature, create, id, state) {
    let entry = pool.get(key);
    if (!entry || entry.signature !== signature) {
      entry = { node: attach(create(), id, state), signature }; pool.set(key, entry);
    } else setState(entry.node, state);
    return entry.node;
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
      for (const record of moving) if (record.node === node) moving.delete(record);
    }
    for (const [key, entry] of pool) if (!entry.node.isConnected) pool.delete(key);
    if (!moving.size && frame !== null) { cancelAnimationFrame(frame); frame = null; previousFrame = 0; }
    scheduleBlink();
  }).observe(document.documentElement, { childList: true, subtree: true });
  const owners = "#whoBtn,.rowin,.feat,.workspace-agent,.workspace-result";
  document.addEventListener("pointermove", event => {
    if (!allowed() || !fine.matches || event.pointerType === "touch" || event.isPrimary === false) return;
    const owner = event.target.closest?.(owners);
    if (!owner) { if (pointer) releasePointer(); return; }
    if (pointer?.owner !== owner) greet(owner);
    pointer = { owner, x: event.clientX, y: event.clientY };
    for (const record of tracked.values()) if (active(record) && (owner.contains(record.node) || record.x || record.y)) moving.add(record);
    requestFrame();
  }, { passive: true });
  document.addEventListener("pointerleave", releasePointer);
  document.addEventListener("pointercancel", releasePointer);
  document.addEventListener("focusin", event => {
    const owner = event.target.closest?.(owners);
    if (owner?.matches(":focus-visible") && allowed()) greet(owner);
  });
  document.addEventListener("scroll", () => {
    for (const record of tracked.values()) record.rect = null;
    releasePointer();
  }, { passive: true, capture: true });
  window.addEventListener("blur", releasePointer);
  window.addEventListener("resize", () => { for (const record of tracked.values()) record.rect = null; releasePointer(); });
  document.addEventListener("visibilitychange", refresh);
  reduced.addEventListener("change", refresh);
  fine.addEventListener("change", refresh);
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
  window.AURORA_AVATARS = { attach, reuse, setState, createSettings, setMode, mode: () => mode };
})();
