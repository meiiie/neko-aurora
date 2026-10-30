# Table viewer source ownership and selection

Code commit: `68942674b99ded286b65f045d13664a2c3ea0a05`, based on `cea95c382d8795e0f36f915ed47a529394e0bb19`. Solo pstack Feature / Prove It Works / Show Me Your Work and webapp-testing continuation.

Two failures were reproduced in the real browser: an expanded table kept showing a previous workspace after a switch, and dragging a text selection beyond the scroll region dismissed the dialog on release. The latter already scrolled horizontally through native Chrome selection; the erroneous backdrop handler then closed it.

Aurora now owns one active table viewer. Leaving a conversation or workspace closes it synchronously; removing or replacing the source closes it through a temporary MutationObserver. An unchanged keyed message render keeps the viewer, zoom and scroll state. All close paths disconnect the observer, remove the dialog and clear active ownership. Explicit dismissal restores focus with `preventScroll`; navigation/source invalidation does not explicitly refocus the old opener. Backdrop dismissal requires both pointer down and click outside the dialog bounds, so a selection started inside cannot dismiss it. Native modal, keyboard scroll and selection behavior are preserved. Service-worker cache is v63.

## Validation

Windows amd64, Python 3.13.7/Playwright, Chrome 156.0.8078.49, Go 1.27.1 and Node 25.9.0. Loopback fixtures, ephemeral browser profiles and an allowlisted environment without provider credentials. No live account, grant, provider call, external message, push or deployment. Exact cwd/commands/environment: `.audit/2026-10-28-table-viewer/validation.jsonl`.

| Command | Result / audit log |
|---|---|
| `python -u app/tests/aurora/table-viewer-browser.py` | PASS `table-viewer-223547.log` |
| `python -u app/tests/aurora/selection-browser.py` | PASS `selection-browser-223552.log` |
| `python -u app/tests/aurora/chat-loading-browser.py` | PASS `chat-loading-223603.log` |
| `python -u app/tests/aurora/read-state-browser.py` | PASS `read-state-223659.log` |
| `python -u app/tests/aurora/adoption-browser.py` | PASS `browser-223716.log` |
| `python -u app/tests/aurora/composer-browser.py` | PASS `composer-223739.log` |
| `python .audit/2026-10-28-grok/syntax.py` | PASS `syntax-223813.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -count=1 ./internal/web` | PASS `web-assets-223815.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe build -o E:\Sach\Sua\neko-aurora\.audit\2026-10-28-table-viewer\neko-aurora.exe ./cmd/neko-aurora` | PASS `build-223819.log` |

86 browser scenario groups passed: 11 new table-viewer, 10 selected-excerpt, 12 chat-loading, 9 read-state, 21 common UI and 23 composer. New cases exercise actual mouse dragging beyond the wide table and keyboard scrolling, zoom, focus return without changing background scroll, unchanged/replaced/removed source nodes, direct chat navigation, workspace switching, repeated activation, backdrop and Escape. Desktop 1280×900 and mobile-sized 390×844 screenshots were inspected. This is not touch-device acceptance.

All final gates share manifest `38f46e324f88f15c06b32a14c986c85b526ee29c582dcf990ba50c9c798b5563`; 1,798 files were verified against the commit with existing Git LF normalization. Candidate scan covered four files and 392 newly reachable history blobs with zero recognizable secret-pattern matches. Full Go/Core/Rust suites were not rerun for this frontend change.

## Retained failures and limits

- Initial `223133` failed on an incorrect expected CSS class in the test. Corrected RED `223154` proved that the old workspace's dialog remained. Initial GREEN `223240` passed those two groups.
- Expanded `223337` exposed the dialog disappearing after selection; the explicit visibility assertion in RED `223437` confirmed that failure. After fixing backdrop handling, `223454` reached a test assumption about Tab wrapping directly from the final control to the first. Native browser chrome can receive focus there; the test now checks explicit Tab/Shift-Tab movement among dialog controls, without replacing the native focus model. All 11 groups passed at `223532` and the final `223547` gate.
- Read-state `223612` failed to open a chat with Enter after workspace round trips, with no page exception. The same source passed at `223659`. The separate ignored `focus-probe.py` deterministically shows focus moving from a chat row to BODY on a same-workspace state reload. That list-focus bug remains a follow-up at this checkpoint; it is not fixed by table changes. A fixture socket write also reported WinError 10053 in the failed run; it does not establish the cause of the keyboard failure.

This provides partial evidence for source entry 0.77.0 Fixed 4, not every table/link scrolling or zoom condition. It does not implement hover arrows, Slack rich-text export, every browser/native platform, realtime streaming selection guarantees or general focus preservation throughout Aurora. No new full-parity claim is made.

Follow-up: the deterministic list-focus bug noted above is addressed at `8a0c5fd2edcbddc8266d675d3ab075249ab87e35` with a separate [focus and viewport report](grokbot-list-focus-2026-10-28.md). The original table gate and its failed read-state run remain unchanged.
