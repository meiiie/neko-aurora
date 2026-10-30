# Routine tests and hosted admission

Code commit: `af6234ca08d91e3158faf0cf07a3c6f35041fcf6`, based on `db2648bd77b86a00578fa48f72d2e173d11fa117`. Solo pstack Feature / Prove It Works / Show Me Your Work. The [official Grok changelog](https://x.ai/changelog/bot), captured on 2026-10-28, describes routine controls in 0.68.0 Improved bullet 3. This implements a local subset, not full routine parity.

## Behavior

Test runs one ordinary queued turn from the saved prompt and exact revision. It keeps pause, recurrence and Last unchanged. The owner confirms that this consumes normal quota and uses normal approval rules. The ledger atomically stores the owner receipt and work. Repeating the same schedule/nonce/revision returns that receipt after edit, deletion or ledger restart; a changed revision conflicts. Receipt lookup is a separate read-only GET. Ordinary chat cannot use the reserved routine nonce namespaces.

The browser saves a nonce before dispatch. Reload/reconnect never automatically executes a test. Explicit retry keeps the same body; receipt lookup can recover a lost acknowledgement, including after the schedule disappears. Corrupt or unavailable storage blocks dispatch. Late responses cannot change a different workspace's UI or replace a newer attempt. Open result navigates to the admitted owner message. PWA cache is v56.

Hosted tests and automatic routines use the existing durable message reservation gate. Invalid/stale tests do not reserve quota. Failed admission retains its reservation; retry does not charge again. Quota exhaustion leaves a one-time occurrence pending with a sanitized blocked state. An already admitted occurrence remains recognized if schedule Last was lost. Pause/resume/delete clients now also require exact `workspaceId`.

## Validation

Windows amd64; PowerShell 7.6; Go 1.27.1; Node 25.9.0; Python 3.13.7; Chrome 156.0.8078.48. The runner excludes live provider/account environment variables. Exact commands, cwd, base, source hashes and results are in `.audit/2026-10-28-schedule-test/validation.jsonl`.

| Command (Go in `app/native`) | Result |
|---|---|
| `go test -v -count=1 -timeout=120s -run TestSchedule ./internal/aurora ./internal/hub` | Initial RED 404, `schedule-test-171844.log`; expanded and final normalization regression PASS, `schedule-test-173805.log` |
| `go test -json -p=1 -count=1 -timeout=300s ./...` | PASS: 1,657 test/subtest events, 37 existing skips, 0 failures, 25 packages; 219.46 s; `go-serial-173336.log` |
| `go vet ./...` | PASS, `vet-173813.log` |
| `go build -o <audit>/neko-aurora.exe ./cmd/neko-aurora`; `go test -count=1 ./internal/web` | Final PASS, `build-174212.log`, `web-assets-174209.log` |
| `python -u app/tests/aurora/schedule-test-browser.py` | PASS: 9 scenarios, `schedule-test-browser-174134.log`; 390 × 844 screenshot inspected |
| `python -u app/tests/aurora/schedule-browser.py`; `python -u app/tests/aurora/selection-browser.py` | PASS: 9 + 10 scenarios, `schedule-browser-174151.log`, `selection-browser-174157.log` |
| `python -u app/tests/aurora/{computer-recovery,adoption,composer,upload}-browser.py` (separate commands) | PASS: 62 scenarios combined; logs 173848, 173854, 173919, 173952 |
| `node --test app/tests/aurora/composer-drafts-test.mjs app/tests/aurora/outbox-test.mjs` | PASS: 8 tests; `composer-unit-173822.log` |

Full-Go manifest: `c595522f62946279f433d330d20b912fedd616ef2c5b83ff7f0857fd07f7ccee`. Final backend normalization guard and focused/vet gate: `ecfbff77338071ab991182ac5b0c438a8bec3930eae86b4bfc2ccd6d28e93dab`. Final CSS and selection fixture adjustment: `f8591525f3bd4a2fe36eee1f5b79da6fa6d69d890cc40b6a05b4fd774d5a026f`. No source changed during reported checks. The full-Go run preceded that normalization guard; focused tests cover it. Browser total: 90 scenarios across these manifests, not a single final-manifest acceptance run.

Regressions caught a reserved-nonce check initially placed in the wrong route, hash-only navigation that did not reload the test page, a timeout fixture that did not invoke its override, and empty live-region CSS that removed accessibility exposure. All were corrected. Selection initially ran during screen transition; the fixture now waits for the thread and target to settle. The final targeted runs above passed. Candidate scan: 17 files and 227 newly reachable history blobs, zero recognizable secret matches; no matched values printed.

## Limits

Local ledger and loopback/browser fixtures are not live provider, model, physical-device or Tauri acceptance. No Go race run with the installed C toolchain. The browser retains one latest attempt per schedule and bounds stored history to 128; it is not cloud sync and has no cross-tab atomic lock. Distinct nonces intentionally admit distinct turns. Reservations retain the existing eight-day cleanup policy. External tools have no exactly-once guarantee. Right-click menus, arbitrary timezones, webhook identity and distributed scheduling remain open. This adds evidence to an existing partial index entry; it does not mark another changelog bullet complete.
