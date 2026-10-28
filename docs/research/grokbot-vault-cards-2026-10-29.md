# Vault card completion markers and expired-session cleanup

Code commit: `d527f566b914e93adea11eb710a46e43dab737ec`, based on `c55c044f6ba7cdd9fa2d32965b4a57258714b33e`. Solo pstack Feature / Prove It Works / Show Me Your Work and webapp-testing continuation.

After a successful server save, a blocked or full localStorage previously threw into the save-error path. The card displayed an error and allowed another save even though the first was accepted. The new device-local completion marker is remembered in memory before best-effort persistence; storage failure keeps the card completed and explains that this device could not remember its state. Blocked marker reads no longer break rendering. Keys bind workspace, bot, work, message and Vault type; bounded versioned JSON stores only the item ID, with no saved label/hint/secret. Unscoped legacy keys remain untouched and are not imported because their owner is unknown. Corrupt, oversized, wrong-version and malformed item-ID records are ignored. Completed cards offer View Vault. UI text has Vietnamese/English dictionary entries; SW cache v68.

A current /state HTTP401 now clears Vault values, timers and secret inputs, closes the active thread/sheet and revokes pending history loads before choosing the existing local or hosted sign-in path. The auth-config result is checked against the latest state request; an older 401/config response cannot replace a newer authenticated view. An older successful boot cannot open a chat after a newer state expired. A successful state load hides the local login view. This is state-refresh cleanup, not a universal handler for every endpoint.

## Validation

Windows amd64; Go 1.27.1, Node 25.9.0, Python 3.13.7/Playwright, Chrome 156.0.8078.49. Allowlisted environment excludes provider credentials. All values and HTTP responses are loopback fixtures. No live account, provider, grant or deployment.

- RED `vault-cards-001224`: accepted save plus a rejected localStorage write left the form with a save-error alert.
- `001827` reached the correct saved card but the fixture incorrectly required an exact CSS class list and rejected its transient `enter` class. The assertion now checks the saved class token. `001909` passes eight initial groups; added cold-boot ordering and actual failed-save retry bring the final suite to ten.
- First broad gate `selection-browser-002209` failed before chat initialization: fixture asset md.js returned ERR_CONNECTION_REFUSED. The selection fixture now uses a 64-connection accept queue (matching newer browser fixtures) instead of Python's smaller default. Queue exhaustion is a plausible cause, not a proven kernel-level diagnosis. The full gate was repeated on the final source, without suppressing the failed log or retrying product mutations.
- **139 browser scenario groups, 8 Node tests, syntax, embedded web assets and Go application build PASS** on one frozen source manifest. Ten new Vault-card groups cover storage failures, marker ownership/reload/corruption, latest and stale expiry/config responses, cold-boot ordering and failed-server-save retry. Desktop 1280x720 and mobile 390x844 screenshots were inspected. This is browser viewport coverage, not native/phone acceptance.

| Command | Result / log |
|---|---|
| `python -u app/tests/aurora/vault-cards-browser.py` | PASS `vault-cards-002246.log` |
| `python -u app/tests/aurora/vault-lifecycle-browser.py` | PASS `vault-browser-002300.log` |
| `python -u app/tests/aurora/dictation-browser.py` | PASS `dictation-002308.log` |
| `python -u app/tests/aurora/composer-browser.py` | PASS `composer-002316.log` |
| `node --test app/tests/aurora/composer-drafts-test.mjs app/tests/aurora/outbox-test.mjs` | PASS `composer-unit-002349.log` |
| `python -u app/tests/aurora/adoption-browser.py` | PASS `browser-002351.log` |
| `python -u app/tests/aurora/chat-loading-browser.py` | PASS `chat-loading-002417.log` |
| `python -u app/tests/aurora/read-state-browser.py` | PASS `read-state-002428.log` |
| `python -u app/tests/aurora/list-focus-browser.py` | PASS `list-focus-002459.log` |
| `python -u app/tests/aurora/selection-browser.py` | PASS `selection-browser-002505.log` |
| `python -u app/tests/aurora/table-viewer-browser.py` | PASS `table-viewer-002517.log` |
| `python -u app/tests/aurora/table-scroll-browser.py` | PASS `table-scroll-002524.log` |
| `python .audit/2026-10-28-grok/syntax.py` | PASS `syntax-002529.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -count=1 ./internal/web` | PASS `web-assets-002532.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe build -o E:\Sach\Sua\neko-aurora\.audit\2026-10-29-vault-cards\neko-aurora.exe ./cmd/neko-aurora` | PASS `build-002536.log` |

Source manifest `ec1aca56bc00530bcc7cd7eca089c2c7e03a41c926e7c82b0a232ea7c880bace`; `code-checkpoint.json` verifies 1806 source files against `d527f566b914e93adea11eb710a46e43dab737ec` with existing LF normalization accounted for. Exact cwd, command, UTC timestamp, environment, before/after manifests and results are in `.audit/2026-10-29-vault-cards/validation.jsonl`. Before checkpoint, five candidate files and 441 newly reachable history blobs had zero recognizable-secret matches; the pattern scan is not exhaustive.

## Limits

These are local completion markers, not durable server receipts or exactly-once saves. If storage is unavailable, reload loses the in-memory completion. Old unscoped markers are not migrated, and a save whose response arrives after leaving the card is outside this marker guarantee. A marker describes an accepted save, not the continued existence of the Vault item. No server idempotency, revision transaction, cross-device synchronization, heap erasure or hosted sign-in acceptance is claimed. Authentication cleanup is triggered by the current state response; other credential/request lifetimes require separate review. Full Go/Rust/Core gates are not repeated because their implementation is unchanged.

This unit adds no new changelog credit: 15 partially validated entries, one excluded behavior, 804 without feature-specific evidence remain.
