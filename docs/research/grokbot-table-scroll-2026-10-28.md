# Wide-table scrolling and localized controls

Code commit: `24059a1ca8a7b2e92c62f58a2dc3bd3f29eeabf6`, based on `e3ca7558ad8822b9ae394036e3ac5d32a58aac24`. Solo pstack Feature / Prove It Works / Show Me Your Work and webapp-testing. Source entry: [Grok Bot 0.77.0](https://x.ai/changelog/bot#v0.77.0), Improved 17.

Overflowing inline tables now expose left/right buttons when the table card is hovered or contains keyboard focus. Touch-style media keeps the controls visible in their own row so they do not obscure cells. Each button names and controls its own scroll region, moves roughly one viewport step, honors reduced motion, and remains focusable with aria-disabled/no-op behavior at an edge. Tables that fit hide both controls. Scroll, mount, resize, hover/focus and font-readiness updates recompute overflow; the implementation does not retain per-table observers or global node registries. The old right-edge mask is removed at the end.

Authored table controls, status messages and the expanded viewer now use explicit Vietnamese/English translations. Markdown content remains excluded from automatic translation. Table cells, exact source Markdown and copied TSV are unchanged. Service-worker cache is v65.

## Validation

Windows amd64; Python 3.13.7/Playwright, Chrome 156.0.8078.49, Go 1.27.1 and Node 25.9.0. Loopback state fixtures and ephemeral browser profiles; Chrome clipboard APIs use fixture text. No provider accounts, live OAuth grants, paid calls, external messages, push or deployment. Exact cwd/environment/commands: `.audit/2026-10-28-table-scroll/validation.jsonl`.

| Command | Result / audit log |
|---|---|
| `python -u app/tests/aurora/table-scroll-browser.py` | PASS `table-scroll-230946.log` |
| `python -u app/tests/aurora/table-viewer-browser.py` | PASS `table-viewer-230952.log` |
| `python -u app/tests/aurora/selection-browser.py` | PASS `selection-browser-230958.log` |
| `python -u app/tests/aurora/list-focus-browser.py` | PASS `list-focus-231008.log` |
| `python -u app/tests/aurora/chat-loading-browser.py` | PASS `chat-loading-231013.log` |
| `python -u app/tests/aurora/read-state-browser.py` | PASS `read-state-231023.log` |
| `python -u app/tests/aurora/adoption-browser.py` | PASS `browser-231041.log` |
| `python -u app/tests/aurora/composer-browser.py` | PASS `composer-231106.log` |
| `python .audit/2026-10-28-grok/syntax.py` | PASS `syntax-231140.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -count=1 ./internal/web` | PASS `web-assets-231143.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe build -o E:\Sach\Sua\neko-aurora\.audit\2026-10-28-table-scroll\neko-aurora.exe ./cmd/neko-aurora` | PASS `build-231147.log` |

106 browser scenario groups passed: 8 new table-scroll, 11 table-viewer, 10 selected-excerpt, 12 list-focus, 12 chat-loading, 9 read-state, 21 common UI and 23 composer. New coverage includes actual pointer and Enter activation, independent horizontal/vertical scroll, edge no-ops with retained focus, hover/focus visibility, narrow tables, viewport resize without a hover trigger, exact clipboard payloads, translated clipboard failure, unchanged Vietnamese cells under English controls, viewer focus/zoom and touch emulation. Touch-control bounds are asserted not to overlap the table region. Desktop and touch-emulated screenshots were inspected; emulation is not physical-device acceptance.

All final gates share manifest `4a63303c002c8eccd60287a4b68dfc10382ac63bda1ef6004f44a332ad1db390`; 1,800 files verified against the code commit with existing Git LF normalization. Candidate scan covered 5 files and 409 newly reachable history blobs with zero recognizable secret-pattern matches. Full Go/Core/Rust suites were not rerun for this frontend change.

## Retained failures and limits

RED `table-scroll-230005.log` failed because the requested controls did not exist. First implementation `230224` and diagnostic `230338` failed pointer activation: hovering the toolbar did not activate the frame-only hover rule, leaving opacity zero and pointer-events none. Hover/focus handling now covers the whole table card; `230513` passed. Expanded `230547` used a 45-digit fixture that already overflowed on desktop, contrary to its intended fit-then-resize condition; a 40-digit fixture verified both widths at `230630`. Screenshot review then moved touch controls into a separate row and translated the copied status. `230753`/`230759` passed those changes; final `230946` added clipboard-refusal coverage and the complete gate above passed without source edits during execution.

No RTL scrolling, screen-reader, native Tauri/webview, physical touch device or all-browser acceptance is claimed. This does not implement Slack rich-text export, document tables, every layout-width transition, or every upstream scrolling/zoom behavior. Controls translate only between the app's existing Vietnamese and English modes. Index coverage is now 14 partially validated source entries, 1 intentionally unadopted behavior and 805 entries without feature-specific validation.
