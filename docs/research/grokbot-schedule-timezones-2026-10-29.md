# Explicit routine timezones

Code commit `31e6040dd1fdcd62ebeab4dea8a6c8211f613332`, based on `1b5db3eb064d80cc399fad5887ccfb866069c3e1`. Solo pstack Feature / Prove It Works / Show Me Your Work and webapp-testing.

The [official Grok Bot changelog](https://x.ai/changelog/bot), rechecked on 29 October Asia/Saigon, remains at 0.77.0 dated 26 October. This unit adds local evidence for the Kyiv timezone-picker portion of index entry `0.77.0-Fixed-52`. Implementation is original. Named-zone validation follows [Go time.LoadLocation](https://pkg.go.dev/time#LoadLocation); the application main imports [time/tzdata](https://pkg.go.dev/time/tzdata) to package the fallback zone database for Windows.

Each routine can now retain an explicit IANA timezone. Existing routines without one continue to use Vietnam time. The editor displays the saved zone, includes Europe/Kyiv and UTC, and sends it with the existing workspace/revision check. Older edit callers that omit the field preserve the saved zone; an explicit empty or invalid zone is rejected. Directives default to Vietnam and are instructed to select another zone only on an explicit user request. This prompt change is not a model-quality evaluation.

Due dates, weekdays, one-time validation and edit start dates use the selected calendar. Repeated DST hours retain one calendar-day occurrence identity, including after reopening schedule storage. A repeating schedule in a skipped hour becomes due on the first later tick that day; nonexistent one-time local times are rejected. Invalid stored zones block admission instead of silently using a fallback. Zone edits retain pause state, prior admitted work and revision conflicts; failed persistence rolls back the edit. Duplicate detection distinguishes different named zones while treating omitted and explicit Vietnam as equivalent. The UI lists the zone and explains the local-time rules. SW cache v70.

Screenshot review caught an additional UI error: the English translator changed a Vietnamese user prompt in the schedule list while the textarea remained correct. A regression reproduced it; the list subtitle now carries the existing user-content translation exclusion. The new labels and guidance translate, while stored prompt text remains unchanged.

## Validation

Windows amd64; Go 1.27.1, Node 25.9.0, Python 3.13.7/Playwright and Chrome 156.0.8078.49. Allowlisted environment excludes live/provider credentials. All schedule admission, HTTP and browser tests use temporary stores or loopback fixtures; no real provider routine ran. Desktop 1280x900 and mobile 390x844 screenshots were inspected.

- RED `schedule-timezones-004301`: existing calendar/admission and edit routes did not honor the selected zone. Focused tests passed after implementation; final focused run `005156` has 21 passing test/subtest events.
- RED `timezone-browser-004700`: no zone field. `004751` exposed an accessible-name problem from including select options in the implicit label; explicit field labels fixed it. Six final browser groups cover legacy defaults, Kyiv/UTC choices, revision-bound keyboard save, reload/cancel, retained error/conflict drafts and English user-content preservation.
- **Full Go: 1,889 PASS, 38 SKIP, 0 FAIL across 25 packages; vet PASS.** Broad browser gate: **177 scenario groups plus 8 Node tests PASS**. Syntax, embedded assets and build PASS. These share manifest `ed5c59c650964176d576cdc3eed8845d9911b0a93fb0afc162ac4e291d03963b`.
- Later RED `timezone-browser-010343` reproduced the translated list subtitle. The final delta is only one scoped `data-noi18n` attribute and the corresponding browser assertion. All 38 schedule browser groups, syntax, embedded assets and build passed again on final manifest `0e06cc1895dc240e1eaa9d086555ab556f2c0460ddb1e978719fbae53e84f911`. `final-ui-delta.json` proves that all Go/Core/Rust source hashes are unchanged from the broad gate; the full Go suite was not repeated for this UI-only delta.

| Command | Result / log |
|---|---|
| `go test -v -count=1 -timeout=120s -run TestScheduleTimezone ./internal/aurora` | PASS `schedule-timezones-005156.log` |
| `go test -v -count=1 -timeout=120s -run TestSchedule ./internal/aurora ./internal/hub` | PASS `schedule-test-005208.log` |
| `go test -json -p=1 -count=1 -timeout=300s ./...` | PASS `go-serial-005217.log`, 472.98 seconds |
| `go vet ./...` | PASS `vet-010010.log` |
| `python -u app/tests/aurora/schedule-timezone-browser.py` | PASS `timezone-browser-010414.log` |
| `python -u app/tests/aurora/schedule-loading-browser.py` | PASS `schedule-loading-010424.log` |
| `python -u app/tests/aurora/schedule-browser.py` | PASS `schedule-browser-010433.log` |
| `python -u app/tests/aurora/schedule-test-browser.py` | PASS `schedule-test-browser-010441.log` |
| `python -u app/tests/aurora/vault-cards-browser.py` | PASS `vault-cards-010100.log` |
| `python -u app/tests/aurora/vault-lifecycle-browser.py` | PASS `vault-browser-010111.log` |
| `python -u app/tests/aurora/dictation-browser.py` | PASS `dictation-010117.log` |
| `python -u app/tests/aurora/composer-browser.py` | PASS `composer-010123.log` |
| `node --test app/tests/aurora/composer-drafts-test.mjs app/tests/aurora/outbox-test.mjs` | PASS `composer-unit-010157.log` |
| `python -u app/tests/aurora/adoption-browser.py` | PASS `browser-010159.log` |
| `python -u app/tests/aurora/chat-loading-browser.py` | PASS `chat-loading-010224.log` |
| `python -u app/tests/aurora/read-state-browser.py` | PASS `read-state-010234.log` |
| `python -u app/tests/aurora/list-focus-browser.py` | PASS `list-focus-010255.log` |
| `python -u app/tests/aurora/selection-browser.py` | PASS `selection-browser-010300.log` |
| `python -u app/tests/aurora/table-viewer-browser.py` | PASS `table-viewer-010311.log` |
| `python -u app/tests/aurora/table-scroll-browser.py` | PASS `table-scroll-010317.log` |
| `python .audit/2026-10-28-grok/syntax.py` | PASS `syntax-010458.log` |
| `go test -count=1 ./internal/web` | PASS `web-assets-010502.log` |
| `go build -o E:\Sach\Sua\neko-aurora\.audit\2026-10-29-schedule-timezones\neko-aurora.exe ./cmd/neko-aurora` | PASS `build-010509.log` |

Go commands above use `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe` from `app/native`; browser/Node commands run from the repository root. `.audit/2026-10-29-schedule-timezones/validation.jsonl` records exact argv, cwd, UTC timestamps, environment and before/after manifests. `code-checkpoint.json` verifies 1,809 source files against the code commit, accounting for existing Git LF normalization. Ten candidate files and 457 newly reachable history blobs had no recognizable secret matches before the code checkpoint; this scan is not exhaustive.

## Limits and coverage

This is per-routine timezone support. Automatic message/browser timezone changes, an account-wide preference, VM timezone changes, calendar connectors, webhook execution, distributed coordination, cross-process locking and physical-device acceptance remain separate work. Browser and server zone database versions can differ; the server validates submitted names. DST fall-back ambiguous one-time hours still use Go's chosen offset, with no first/second occurrence selector. Existing tick/catch-up timing is retained; no exact-minute or global exactly-once claim is made. Calendar-day identities are retained across zone edits, not replaced with a new timezone epoch. The embedded database is included by import and the application builds; this does not claim a separate packaged-installer runtime test with every external timezone source removed.

Only the Kyiv picker entry gains partial local validation. Coverage becomes 19 partial entries, one excluded behavior and 800 entries without feature-specific evidence. No complete Grok Bot parity is claimed.
