# Schedule editing with occurrence and revision guards

Code commit: `b80ecfa2dc99259f1fc9e74374ceee4da609e6a3`, based on `f6aba1f04ea4e3061ef4d23863e76e3772e10fa6`. Local pstack Feature / Prove It Works checkpoint, one implementer, no subagents.

The profile now lets the owner edit a schedule's prompt, time and daily/weekday/one-time pattern. The strict endpoint requires the current workspace and revision. It preserves the bot, identity and paused state; failed persistence restores the old row. A one-time edit requires an explicit future Vietnam date/time. Recurring edits whose new time has passed, or whose occurrence was already admitted today, start tomorrow. The actual ledger receipt is checked even when the schedule display cache lost its Last field. Already admitted work retains its original prompt. The persisted StartDate is an admission fence, not a fabricated execution receipt.

The editor retains fields after conflict, invalid input, timeout or lost response. Reload is explicit; an acknowledged save whose list refresh fails remains saved and cannot be submitted again, including after another failed refresh. Scope checks suppress late reloads across account changes. The editor has labels, keyboard actions, focus restoration and a 390px layout; the script is embedded and registered in PWA cache v54.

## Validation

Windows amd64, PowerShell 7.6, Go 1.27.1, Node 25.9.0, Python 3.13.7, Chrome 156.0.8078.48. The runner strips real provider/account variables. Server tests use temporary real ledger/schedule files and HTTP handlers; browser tests use loopback fixtures.

| Command (Go commands in `app/native`) | Result / local log |
|---|---|
| `go test -v -count=1 -timeout=120s -run 'TestSchedule(Edit\|Control\|Due)' ./internal/aurora` before implementation | FAIL: positive edit routes returned 404; `schedule-edit-161105.log` |
| Same focused command after implementation | PASS; `schedule-edit-162429.log` |
| `go test -json -count=1 -timeout=180s ./internal/aurora` | PASS: 190 test/subtest events, 24 existing skips, 0 failures; `aurora-162737.log` |
| `go vet ./...` | PASS; `vet-162819.log` |
| `go build -o <audit>/neko-aurora.exe ./cmd/neko-aurora` | PASS; `build-162823.log` |
| `go test -count=1 ./internal/web` | PASS; `web-assets-162826.log` |
| `python -u app/tests/aurora/schedule-browser.py` | PASS: 9 scenarios; `schedule-browser-162720.log` |
| `python -u app/tests/aurora/adoption-browser.py` | PASS: 21 scenarios; `browser-162837.log` |
| `python -u app/tests/aurora/composer-browser.py` | PASS: 23 scenarios; `composer-162901.log` |
| `python -u app/tests/aurora/upload-browser.py` | PASS: 11 scenarios; `upload-browser-162936.log` |
| `python -u app/tests/aurora/selection-browser.py` | PASS: 10 scenarios; `selection-browser-162945.log` |
| `node --test app/tests/aurora/composer-drafts-test.mjs app/tests/aurora/outbox-test.mjs` | PASS: 8 tests; `composer-unit-162829.log` |

The final full-package, browser, asset, vet and build source manifest is **`73e21bfd4c4079b368339b5ef7b0c77a84bd0efe54889995102ce723da8dd9a2`**. The earlier focused run differs only in the browser test. The source bytes were verified against the code commit, allowing repository CRLF normalization. Exact commands, environment, timestamps and manifests are in `.audit/2026-10-28-schedule-edit/validation.jsonl` and `code-checkpoint.json`.

The first browser run exposed the repeated-reload bug that re-enabled Save after a successful edit. Later fixture failures selected the morning-brief row instead of the bot row; the selector was corrected without raising deadlines. A diagnostic Python print also failed under the Windows console encoding. These failed runs are retained, including one failed diagnostic run whose fixture changed before it ended. No source changed during the final successful gates. The screenshot was visually inspected. This is Chromium viewport/keyboard evidence, not physical-phone or native desktop acceptance.

The secret scan covered 10 candidate files and 187 newly reachable history blobs, finding no recognizable secrets and printing no matched values. No provider calls, grants, push or deployment occurred. This adds partial local evidence for source entry `0.68.0-Improved-3` (line 581): Pause/Resume/Edit/Delete are present; live Test-run and right-click routine controls remain outside this checkpoint. Timezone expansion, webhook identity and durable trigger handoff also remain. The index now has 11 partially validated entries, one intentionally unadopted behavior and 808 entries without feature-specific validation; source bullets are not independent features.
