# Chat list focus and fixed viewport

Code commit: `8a0c5fd2edcbddc8266d675d3ab075249ab87e35`, based on `fc5de0c3bb3b5bcec800f9c221e82f6703e0b698`. Solo pstack Feature / Prove It Works / Show Me Your Work and native Playwright continuation of the [table-viewer gate](grokbot-table-viewer-2026-10-28.md).

A focused row lost focus to BODY whenever a state refresh rebuilt the list. The new focused regression reproduced it deterministically. List rebuilds now preserve the same bot and control (row, pin, archive or featured button) within the same workspace, without scrolling it into view. Pin/unpin follows the same bot between list and featured locations. Removing the focused bot moves focus to search. Existing search selection, context-menu focus and composer focus remain untouched; a new workspace does not inherit focus from a repeated bot ID.

Chat rows now use native buttons. Enter/Space and featured-button keyboard activation open the intended chat and focus its composer with `preventScroll`; Space activates on release. Pointer opening keeps its existing focus behavior. The fixed outer app shell uses `overflow: clip` so focus/programmatic scrolling cannot shift the entire app sideways; ordinary chat, list and table scroll containers remain scrollable. `overflow: hidden` is retained as a fallback for engines lacking clip support. The semantics follow [MDN's overflow reference](https://developer.mozilla.org/en-US/docs/Web/CSS/Reference/Properties/overflow): hidden can scroll programmatically, whereas clip is not a scroll container. Service-worker cache is v64.

## Validation

Windows amd64; Python 3.13.7/Playwright, Chrome 156.0.8078.49, Go 1.27.1, Node 25.9.0. Loopback fixture state and isolated browser contexts, provider credentials excluded. No live account, paid model, grant, external message, push or deployment. Exact cwd/environment/commands and retained failures: `.audit/2026-10-28-list-focus/validation.jsonl`.

| Command | Result / audit log |
|---|---|
| `python -u app/tests/aurora/list-focus-browser.py` | PASS `list-focus-225418.log` |
| `python -u app/tests/aurora/read-state-browser.py` | PASS `read-state-225425.log`, `read-state-225610.log` |
| `python -u app/tests/aurora/table-viewer-browser.py` | PASS `table-viewer-225443.log` |
| `python -u app/tests/aurora/selection-browser.py` | PASS `selection-browser-225450.log` |
| `python -u app/tests/aurora/chat-loading-browser.py` | PASS `chat-loading-225501.log` |
| `python -u app/tests/aurora/adoption-browser.py` | PASS `browser-225511.log` |
| `python -u app/tests/aurora/composer-browser.py` | PASS `composer-225535.log` |
| `python .audit/2026-10-28-grok/syntax.py` | PASS `syntax-225628.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -count=1 ./internal/web` | PASS `web-assets-225630.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe build -o E:\Sach\Sua\neko-aurora\.audit\2026-10-28-list-focus\neko-aurora.exe ./cmd/neko-aurora` | PASS `build-225633.log` |

98 distinct browser scenario groups passed: 12 focus, 9 read-state, 11 table-viewer, 10 selected-excerpt, 12 chat-loading, 21 common UI and 23 composer. The final read-state gate ran twice to exercise the previously intermittent keyboard path. New cases cover real Enter and Space, same-workspace refresh/reorder, scheduled agent-event rendering, pin/unpin, removed controls, unchanged search/caret/menu/composer, workspace identity, desktop/mobile-sized layout and a fixed-shell scroll invariant. EventSource in the focus suite is a callback fixture, not transport acceptance. Screenshots at 1280×900 and 390×844 were inspected.

All final gates share manifest `cbcf5d5fcbf66dcda22f04a9b10fa517e4bd603bc9aec79b1eafd6e6cdd1286b`; 1,799 files verified against the code commit with existing Git LF normalization. Candidate scan: 5 files and 400 newly reachable history blobs, zero recognizable secret-pattern matches. Full Go/Core/Rust suites were not rerun for this frontend-only change.

## Retained failures and boundaries

- RED `list-focus-224110.log` proved loss of the focused row on reload; GREEN `224209` passed the initial case and `224314` passed 10 expanded cases.
- The additional composer-focus assertion failed at `224738`: preserving row focus alone left it behind after keyboard navigation. Explicit keyboard-to-composer focus fixed it. Native button activation replaced the temporary custom key handler.
- Selected-excerpt runs `224403` and diagnostic `224533` timed out at the initial chat-position assertion. The diagnostic captured a thread at x=-154 with all five stylesheets loaded and window scrollX=0. The same source path later passed at `224822`; the failure is not treated as a missing asset. A separate deterministic RED at `224939` proved that setting the outer shell's scrollLeft shifted the layout. Clip fixes that reproduced invariant; the original transition/focus timing was not fully traced. Layout/request diagnostics remain in the selection fixture.
- Native focus/shell changes passed focused runs at `224959` and `225004`, followed by the final full UI gate above. Fixture connection-aborted writes while browser contexts close can appear in logs; they are not application failures or proof of a transport fix.

This is not general accessibility certification, screen-reader testing, touch-device acceptance, a keyboard-navigation redesign or a fix for every focus control. Old engines using the hidden fallback retain the old shell-scrolling limitation. List focus is device-local UI state; no cloud synchronization or server read-state protocol was added. The index remains 13 partial entries, 1 intentionally unadopted behavior and 806 without feature-specific validation.
