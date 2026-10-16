# Independent light UI mobile/accessibility supplement, 2026-10-11

The independently audited snapshot is **`2cda67d724e116c9e5678e64f75cd0239137e5d1`** on `ui-light-reference-f6`. **Regression 28/28 PASS; context 28/28 PASS; new light-feature checks 12 PASS / 7 FAIL / 0 BLOCKED**. The seven failed gates identify six finding groups below. These were sent early to the UI and integrator owners; the UI owner acknowledged them and is preparing a separate immutable fix. No working-tree delta is treated as validated.

The previous f6 context report/evidence remain intact. This is a separate behavioral/AX/layout audit of the new light UI. The user rejected the graphite visual direction; passing functionality does not approve this light design or establish reference-image fidelity. **Visual acceptance remains pending.** No production UI/runtime, existing tests/config, Git index, credentials, provider, grant, native window, team browser/mouse or network settings were changed.

## Exact evidence and environment

Source: `C:\Users\Admin\Documents\Codex\2026-10-11\task-3\ui-work-3cb9b6f`. Windows, Node v25.9.0, Chrome/154.0.8037.98, headless, three owned temporary profiles and dynamic loopback ports. UI, three Geist WOFF2 subsets and two provenance/license notices are frozen from the exact Git object. All nine raw-byte hashes match the declared commit. Fixture bots, messages, events and context responses are entirely in memory; no actual context API or Go/Rust/Core runtime is used by these suites.

```powershell
& 'C:\Program Files\nodejs\node.exe' tests/mobile-audit-20261011/light-audit.mjs --source 'C:\Users\Admin\Documents\Codex\2026-10-11\task-3\ui-work-3cb9b6f' --output tests/mobile-audit-20261011/evidence/light-2cda-regression --commit 2cda67d724e116c9e5678e64f75cd0239137e5d1 --committed true --suite regression
& 'C:\Program Files\nodejs\node.exe' tests/mobile-audit-20261011/light-audit.mjs --source 'C:\Users\Admin\Documents\Codex\2026-10-11\task-3\ui-work-3cb9b6f' --output tests/mobile-audit-20261011/evidence/light-2cda-context --commit 2cda67d724e116c9e5678e64f75cd0239137e5d1 --committed true --suite context
& 'C:\Program Files\nodejs\node.exe' tests/mobile-audit-20261011/light-audit.mjs --source 'C:\Users\Admin\Documents\Codex\2026-10-11\task-3\ui-work-3cb9b6f' --output tests/mobile-audit-20261011/evidence/light-2cda-features-verified --commit 2cda67d724e116c9e5678e64f75cd0239137e5d1 --committed true --suite features
```

| Owned evidence directory under `tests/mobile-audit-20261011/evidence/` | UTC completion | Outcome |
|---|---|---|
| `light-2cda-regression/results.json` | 2026-10-11 11:57:42 | 28 pass / 0 fail / 0 blocked; exit 0 |
| `light-2cda-context/results.json` | 2026-10-11 11:58:02 | 28 pass / 0 fail / 0 blocked; exit 0 |
| `light-2cda-features-verified/results.json` | 2026-10-11 11:59:24 | 12 pass / 7 fail / 0 blocked; exit 1 |

Runner SHA-256: `d7dc2c3a6dba7ad55f96dd433a8ac6566a1d38399d42f76da79a1067c744c494`. Context cases: `f3fdffd80f6c189fce66564405a6bc044ce1102ffb08340137914355bcbb253c`. Verified light cases: `f75804aa894e38a6854d6f9f100be62e7e583448f3f9ea45e5794afa321424fa`. The initial light-cases hash imported, but not executed, by the regression/context modes is `b27fa887b381788ad5c6e336d04e3c7167768209d17331547588983edf277486`; exact historical bytes are retained at `light-2cda-regression/initial-light-cases.mjs`.

The initial features run had one false font-readiness failure: CDP already reported custom Geist rendering and both used faces were loaded, but `document.fonts.check` included a declared, unused face. The verified font gate explicitly loads the tested Vietnamese string before checking readiness. This harness correction is not a product font defect. No other assertion changed, and all seven behavioral failures persisted.

## Findings at 2cda67d

Paths are relative to `native/internal/web/assets/`. High/medium/low indicate impact within the simulated browser scope, not physical-phone or backend conclusions.

| ID / priority | Repro and path | Measured outcome / concrete request |
|---|---|---|
| LIGHT-MA-01 high | `app.js:91`, nav-open. At 390x844 open drawer; search `fixture-no-such-bot`; Escape; reopen drawer. | Roster has 0 results, main is inert, but initial focus is BODY outside sidebar. Fall back to search or another visible drawer control when no bot matches. Preserve Tab/Shift+Tab scope and opener return. Gate `light_empty_search_drawer_reopen_initial_focus`. |
| LIGHT-MA-02 medium | `style.css:62`, mobile `.bot-search`. Open drawer at 390x844. | Search label hit area is 295x**36**, below the Neko 44x44 product goal. Search has a correct AX searchbox name; other measured drawer actions reach 44 px. Increase narrow/coarse search hit height. This alone is not a WCAG AA finding. Gate `light_search_touch44_product_goal`. |
| LIGHT-MA-03 high for mention / medium for lane | `style.css:59`, absolute `.mention-picker`/`.composer-menu`. At **390x260**, open `@` picker with 14 fixture bots, then separately open the lane menu. | Mention box top=-33, first option y=-24..20, center hit fails. Lane menu top=-10 clips its heading, although both radio rows are hittable. Measure available viewport space and cap popup height/overflow, including resize while open. Gate `light_mention_short_viewport_option_reachable` and `light_lane_menu_small_viewports_reachable`; pixel `light-mention-short.png`. This is resized layout, not an actual OS-keyboard observation. |
| LIGHT-MA-04 low usability expectation | `app.js:120`, `handleMentionKey`. Type `@`; press Shift+Tab. | Draft changes to `@meiiie ` and focus stays prompt. Reverse Tab should close without selecting and move to the previous composer control. This is a keyboard direction expectation; not a claim of an inescapable keyboard trap. Gate `light_mention_reverse_tab_keeps_navigation_direction`. |
| LIGHT-MA-05 high | `app.js:118`, `markMention`. At 390x844, type `@` with 14 bots; ArrowDown 9 times. | Active descendant is selected at y=956..1000 while listbox is y=551..771 and scrollTop=0. Scroll the selected option into the visible listbox while keeping DOM focus in prompt. Gate `light_mention_keyboard_active_option_visible`. |
| LIGHT-MA-06 medium | `style.css:58`, `#hint:not([data-tone=error])`. Hold the fixture message POST after Enter. | Text becomes “Đang lưu tin nhắn…” but AX reports ignored / role none because normal hints use `display:none`. Preserve a concise status region using a visual-only hiding technique when normal text should not be shown. Gate `light_normal_pending_feedback_exposed_to_at`. AX exposure is not AT speech acceptance. |

## Passing behavior and font boundaries

The reused regression/context suites preserve the f6 safeguards: ordinary drawer/work modal focus and return, named actions, 44 px core touch controls, low-height work-panel content, reduced motion, streaming reading/focus, denied-storage startup, snapshot retry, same-nonce outbox retry/reconnect, late bot/draft ACK fences, context capabilities/revision gates, runtime-attempt binding, stale ACK rejection and context focus fences. Their shared initialization/source checks overlap; suite counts should not be presented as unique end-to-end scenarios.

New checks pass search filtering without changing selected bot/draft/search focus; lane keyboard selection/indicator/FormData and Escape return; mention ArrowDown/Enter selection without submit or bot switching; Escape/Tab behavior and synthetic composition/repeat guards; mobile Copy naming/44 px target/visibility/focus; reduced-motion suppression of composer Web Animations and working-avatar animation.

**Font evidence is explicitly split:**

- Independent fixture: exact committed Latin, Latin Extended and Vietnamese WOFF2 bytes are served by the owned Node server as `font/woff2`. Its CSP permits `font-src 'self'` and inline style updates. The verified gate observes custom **Geist** rendering through CDP, successful tested-string FontFaceSet readiness and HTTP 200 for all three exact paths. These fixture headers/CSP are harness choices, not Go/Tauri behavior. Source notices retain Geist OFL and Neko provenance; no Grok bundle was run or its unlicensed assets copied.
- Static exact-2c Go source: `native/internal/api/server.go:42,47` lacks a font-src allowance and limits asset routing to the four original files; `native/internal/web/embed.go:21` likewise lacks WOFF2 MIME entries. This identifies an integration requirement; the owned fixture cannot prove it fixed.
- The UI owner separately reports integration commit `05e87e4`, `actual-typography.json`, three actual Go HTTP font responses and custom Geist rendering. That is **owner-provided separate integration evidence**, not one of this auditor's executed suites and not Tauri/phone acceptance. Its exact commit/command/environment should remain with the integrator's report. A later integrated Go state does not alter the immutable 2c UI checks above.

## Remaining acceptance and handoff

Actual phone keyboard/IME, safe-area values, touch/OS back, VoiceOver/TalkBack speech, lifecycle/reconnect/handover, Tauri rendering, backend stop/context/archive semantics and authenticated cloud/multi-host replay remain unaccepted by this audit. Current reduced-height and coarse-pointer checks only simulate browser layout/input capabilities.

For the pending fix, rerun all three modes against its immutable commit; do not run the writer's changing checkout. In particular retest empty search reopening, active-option visibility, popup layout before/after resize, Shift+Tab, pending AX status and 44 px search. Keep the user’s visual review separate from those results.

UI task `01a12a58-38b6-77de-a3d4-7f44118a7022` acknowledged the six findings and is preparing a UI-only delta. Runtime/integrator task `01a1299d-c1f3-72db-9e36-eafba745401c` received the same repro/evidence and font distinction. Only the new light report, owned runners/cases/readme and selected evidence are handed off; no Git checkpoint/index write is made.
