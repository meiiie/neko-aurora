# Memory operation ownership and bounded waiting

Code commit `c29b364e443de370691c6f08909533d9c0c4a31c`, based on `eb1a5ba1c63030247f3d992298b45de460671507`. Solo pstack Feature / Prove It Works / Show Me Your Work and webapp-testing. Continues [workspace/revision protection](grokbot-memory-revisions-2026-10-29.md).

Import preview/apply and ask/summary-preview now share the panel's pending-operation state with CRUD. While a request is pending, controls in that view are disabled; the sheet can still close. A pending import cannot overlap another edit/add in the same panel. Completion restores controls only in the originating render, so an older request cannot lock or overwrite a newly opened view. Import timeout retains its text and requires a new explicit preview before another apply.

Memory POST paths have finite waiting limits in supported browsers, including paths with workspace/revision queries: 30 seconds for mutations/import and 95 seconds for ask/summary-preview around the server's 90-second model budget. GET keeps its existing 15-second timeout/read retry. Mutation/model POSTs do not automatically retry. Timeout is an uncertain acknowledgement, not evidence the server did nothing. Existing revision checks and GET-only reconciliation remain. SW cache v73.

## Validation

Windows amd64; Node 25.9.0, Go 1.27.1, Python 3.13.7/Playwright, Chrome 156.0.8078.49. Loopback fixtures only; no real model/provider. The memory browser fixture injects controllable AbortSignal.timeout controllers, verifies the requested 30,000/95,000 millisecond deadlines, then aborts fetch to test error propagation and restoration. It does not wait 95 real seconds or prove operating-system scheduling behavior. Other browser suites use the ordinary browser API.

- RED `memory-browser-015036.log`: the add field was still enabled while import preview was held.
- `memory-browser-015106.log`: **27 memory groups PASS**, including six new groups for pending-operation ownership, duplicate preview suppression, timeout recovery, later-view independence and accepted writes with lost acknowledgement.
- **204 browser scenario groups and 8 Node tests PASS**, plus syntax, embedded assets and application build on one final manifest.
- `schedule-test-browser-015306.log` failed during initial chat loading before routine assertions. The diagnostic run `schedule-diagnostic-015424.log` and subsequent ordinary run passed without source changes. Expected lost-response/abort failures appeared only on the explicitly faulted schedule API in the diagnostic. The original startup flake was not reproduced and its root cause remains unresolved; no production fix is claimed.
- Go implementation, Core and Rust sources did not change. The preceding code checkpoint `a07a27955b65114c8c82c956b5debad580211b11` has full Go **1,949 PASS / 38 SKIP**, vet and build evidence. Full Go/Core/Rust gates were not repeated for these browser-only changes.

| Exact command | Result / log |
|---|---|
| `python -u app/tests/aurora/memory-lifecycle-browser.py` | PASS `memory-browser-015106.log` |
| `python -u app/tests/aurora/adoption-browser.py` | PASS `browser-015125.log` |
| `python -u app/tests/aurora/schedule-timezone-browser.py` | PASS `timezone-browser-015226.log` |
| `python -u app/tests/aurora/schedule-loading-browser.py` | PASS `schedule-loading-015239.log` |
| `python -u app/tests/aurora/schedule-browser.py` | PASS `schedule-browser-015252.log` |
| `python -u app/tests/aurora/schedule-test-browser.py` | PASS `schedule-test-browser-015518.log` |
| `python -u app/tests/aurora/vault-cards-browser.py` | PASS `vault-cards-015536.log` |
| `python -u app/tests/aurora/vault-lifecycle-browser.py` | PASS `vault-browser-015547.log` |
| `python -u app/tests/aurora/dictation-browser.py` | PASS `dictation-015554.log` |
| `python -u app/tests/aurora/composer-browser.py` | PASS `composer-015605.log` |
| `node --test app/tests/aurora/composer-drafts-test.mjs app/tests/aurora/outbox-test.mjs` | PASS `composer-unit-015644.log` |
| `python -u app/tests/aurora/chat-loading-browser.py` | PASS `chat-loading-015646.log` |
| `python -u app/tests/aurora/read-state-browser.py` | PASS `read-state-015701.log` |
| `python -u app/tests/aurora/list-focus-browser.py` | PASS `list-focus-015726.log` |
| `python -u app/tests/aurora/selection-browser.py` | PASS `selection-browser-015734.log` |
| `python -u app/tests/aurora/table-viewer-browser.py` | PASS `table-viewer-015751.log` |
| `python -u app/tests/aurora/table-scroll-browser.py` | PASS `table-scroll-015802.log` |
| `python .audit/2026-10-28-grok/syntax.py` | PASS `syntax-015149.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -count=1 ./internal/web` | PASS `web-assets-015813.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe build -o E:\Sach\Sua\neko-aurora\.audit\2026-10-29-memory-operations\neko-aurora.exe ./cmd/neko-aurora` | PASS `build-015821.log` |

Go commands run in `app/native`; other commands run at repository root. Manifest `071df7bbd8e1cc6ff6b90b382bbce5d454f8e4a51c5ba1489d9400c6c178c8be`; 1,813 source files verified against the code commit, accounting for existing Git LF normalization. `.audit/2026-10-29-memory-operations/validation.jsonl` records cwd, UTC timestamps, environment and unchanged source snapshots. 5 candidate files and 498 newly reachable history blobs had no recognizable secret matches before the code checkpoint; detection is not exhaustive.

## Limits

This governs one UI panel and browser waiting. It does not cancel or roll back a request already executing on the server, add durable idempotency receipts, coordinate separate processes, encrypt memory, synchronize drafts, make the history journal atomic, or guarantee power-loss recovery. Legacy callers omitting workspace/revision retain the compatibility limits documented in the preceding report. Browsers without AbortSignal.timeout do not get this native deadline path. Model output quality, live providers, hosted multi-device, Tauri and physical-device acceptance remain unvalidated. Changelog totals remain 20 partial, one excluded and 799 without feature-specific evidence.
