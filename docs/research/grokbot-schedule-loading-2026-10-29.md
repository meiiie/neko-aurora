# Schedule load recovery, reconnect refresh and draft ownership

Code commit `b8d62a5bb9b03bb1fa2f3da633b7155c17078d44`, based on `d0bb114a76f93c35a72fa0770925ae35cd13667c`. Solo pstack Feature / Prove It Works / Show Me Your Work and webapp-testing.

The [official Grok Bot changelog](https://x.ai/changelog/bot) was rechecked on 29 October Asia/Saigon; latest remains 0.77.0 dated 26 October. This unit adapts the inline routine-load error, reconnect refresh and retained-content retry ideas represented by index IDs 0.71.2-Fixed-28, 0.71.2-Fixed-36 and the routines portion of 0.70.2-Fixed-11. It does not copy reference implementation or assets.

Previously, schedule fetch failures produced only a transient toast, an older successful read could overwrite a newer list, and reconnect refreshed state/messages without refreshing schedules. The profile now always distinguishes loading, error and a successful empty list. Failure retains the last list and provides a local retry button. A request number, workspace and profile generation fence both successful and failed responses, including close/reopen of the same bot. Keyboard retry returns focus to the loaded section without scrolling another view.

Successful state refresh starts schedule loading independently of chat history. Reconnect and visibility/state refresh can therefore update schedules even when chat history fails. Automatic refresh preserves an open schedule editor, including one opened while the read is pending; save or cancel can load the current server list. Late profile-history callbacks also preserve the editor and are bound to their originating profile/workspace. Acknowledgements from an older mutation cannot reload a newer profile over its draft. Accepted pause/delete followed by a failed GET remain accepted; retry reads the list without replaying the mutation. Existing revision checks, schedule-test receipts and explicit edit-conflict recovery remain in use. Added UI text has Vietnamese/English translations; SW cache v69.

## Validation

Windows amd64; Go 1.27.1, Node 25.9.0, Python 3.13.7/Playwright, Chrome 156.0.8078.49. Local HTTP fixtures and injected EventSource reconnection; no provider, account, live scheduled turn, grant or deployment. Desktop 1280x900 and mobile 390x844 screenshots were inspected with the affected section scrolled into view.

- RED `schedule-loading-002702`: initial schedule error had no inline status/retry section. First repair `002744` passed 12 groups; existing editor and test-run suites also passed.
- Review added two behavioral regressions. `003154` passed 12 groups but failed when chat history returned 500: schedule refresh was still downstream of that await. Moving schedule loading before the independent history await passed this case. `003243` then passed 13 groups but lost the active editor when deferred profile history completed. Profile/workspace generation checks and the editor-preservation guard repair it; final suite has 14 groups.
- Broad gate `table-viewer-003138` failed its pointer-selection precondition: horizontal scrolling occurred but no text was selected. The fixture now allows keyboard scrolling to settle, starts the real pointer drag at a measured text glyph rather than cell padding, and asserts a real selection before dragging outside the table. No table product code changed. The final suite checks selection and scrolling together; the failed log remains.
- **171 browser scenario groups, 8 Node tests, syntax, embedded web assets and application build PASS** on one final manifest: 14 new groups, 18 existing schedule edit/test groups, plus the prior 139 browser groups. Slow final runs (schedule-test 117.09 seconds, vault-cards 144.73 seconds) still completed successfully; no test result was inferred from elapsed time. No overlapping heavy checks were started by this task.

| Command | Result / log |
|---|---|
| `python -u app/tests/aurora/schedule-loading-browser.py` | PASS `schedule-loading-003316.log` |
| `python -u app/tests/aurora/schedule-browser.py` | PASS `schedule-browser-003323.log` |
| `python -u app/tests/aurora/schedule-test-browser.py` | PASS `schedule-test-browser-003329.log` |
| `python -u app/tests/aurora/vault-cards-browser.py` | PASS `vault-cards-003528.log` |
| `python -u app/tests/aurora/vault-lifecycle-browser.py` | PASS `vault-browser-003753.log` |
| `python -u app/tests/aurora/dictation-browser.py` | PASS `dictation-003806.log` |
| `python -u app/tests/aurora/composer-browser.py` | PASS `composer-003814.log` |
| `node --test app/tests/aurora/composer-drafts-test.mjs app/tests/aurora/outbox-test.mjs` | PASS `composer-unit-003850.log` |
| `python -u app/tests/aurora/adoption-browser.py` | PASS `browser-003851.log` |
| `python -u app/tests/aurora/chat-loading-browser.py` | PASS `chat-loading-003916.log` |
| `python -u app/tests/aurora/read-state-browser.py` | PASS `read-state-003926.log` |
| `python -u app/tests/aurora/list-focus-browser.py` | PASS `list-focus-003944.log` |
| `python -u app/tests/aurora/selection-browser.py` | PASS `selection-browser-003949.log` |
| `python -u app/tests/aurora/table-viewer-browser.py` | PASS `table-viewer-004002.log` |
| `python -u app/tests/aurora/table-scroll-browser.py` | PASS `table-scroll-004011.log` |
| `python .audit/2026-10-28-grok/syntax.py` | PASS `syntax-004021.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -count=1 ./internal/web` | PASS `web-assets-004024.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe build -o E:\Sach\Sua\neko-aurora\.audit\2026-10-29-schedule-loading\neko-aurora.exe ./cmd/neko-aurora` | PASS `build-004033.log` |

Source manifest `75914eec727a559e8087406d5f846c82e9b37501ce4a5878c8103b459b53cd60`. `code-checkpoint.json` verifies 1807 source files against `b8d62a5bb9b03bb1fa2f3da633b7155c17078d44`, with existing Git LF normalization accounted for. `.audit/2026-10-29-schedule-loading/validation.jsonl` records exact commands, cwd, UTC timestamps, environment and before/after manifests. Five candidate files and 448 newly reachable history blobs had no recognizable secret matches before the code checkpoint; this is not exhaustive secret detection.

## Limits and coverage

This validates the browser's schedule read/mutation-continuation lifecycle against injected HTTP/SSE events. It does not establish real reconnect delivery, multi-device convergence, server request cancellation, webhook/routine execution, timezone handling, provider connectors or physical-device acceptance. The list remains memory-only across a page reload. Automatic refresh intentionally defers while an editor is open; it does not merge remote edits into a local draft. Other profile sections and the broader chats/pages/skills/plugins retry bullet remain separate work. Full Go/Rust/Core gates are not repeated because their implementation is unchanged.

The three referenced index entries gain partial local validation, bringing recorded coverage to 18 partial entries, one excluded behavior and 801 entries without feature-specific evidence. No full Grok Bot parity is claimed.
