# Memory panel request and view ownership

Code commit `d9f2d8b31a0554d1edb916427140a11216e7d7b7`, based on `bc59d7abda61aa56dc54b98d17b8843dbd3075ff`. Solo pstack Feature / Prove It Works / Show Me Your Work and webapp-testing. This supports the broader memory/reliability adoption backlog without claiming another changelog entry complete.

Previously, an older memory response could replace a newer panel, row callbacks used a mutable bot identifier, and a late save/delete acknowledgement could reopen memory over another draft. The panel now owns an immutable bot/workspace view, request sequence and render identity. Leaving the sheet clears its hidden fields/content and revokes pending UI continuations. The import, ask and summarize tools mount only after the current read succeeds; their callbacks cannot update a later view. Same-bot reopen and reused bot IDs after workspace changes are covered.

Add/edit/delete and applying a summary disable controls while awaiting acknowledgement. Accepted mutation plus failed list reload retains the loaded content and offers GET-only retry, without replaying the mutation. Read errors appear inside the panel. Error feedback retains the edit text and describes the missing acknowledgement. Keyboard users can open a memory row with Enter/Space and focus retry. User facts, answers, proposals and history content bypass the UI translator. Added UI copy is translated; the full legacy memory UI is not claimed bilingual. SW cache v71.

## Validation

Windows amd64; Go 1.27.1, Node 25.9.0, Python 3.13.7/Playwright, Chrome 156.0.8078.49. Local HTTP fixtures with held responses; ask/summarize use stub replies, not a model. Desktop 1280x900 and mobile 390x844 screenshots were inspected.

- RED `memory-browser-010734` reproduced the older read replacing the latest fact after close/reopen.
- First implementation `010923` passed four groups before a test locator matched both add and edit status elements. The selector was narrowed to the editing row. `011009` passed six groups before the test reused an exact delete-button name after the confirmation label changed; a stable row-scoped selector fixed the harness. These failed logs are retained.
- `011058` passed 12 groups. Two review additions cover a pending edit across switching bots and a late import acknowledgement over a newer draft. Final suite `011224` passes **14 groups**.
- **191 browser scenario groups and 8 Node tests PASS**, with syntax, embedded asset tests and application build on the same final manifest. This includes the prior 177 browser groups. Go implementation, Rust and Core are unchanged, so their full gates were not repeated.

| Exact command | Result / log |
|---|---|
| `python -u app/tests/aurora/memory-lifecycle-browser.py` | PASS `memory-browser-011224.log` |
| `python -u app/tests/aurora/adoption-browser.py` | PASS `browser-011239.log` |
| `python -u app/tests/aurora/schedule-timezone-browser.py` | PASS `timezone-browser-011304.log` |
| `python -u app/tests/aurora/schedule-loading-browser.py` | PASS `schedule-loading-011313.log` |
| `python -u app/tests/aurora/schedule-browser.py` | PASS `schedule-browser-011320.log` |
| `python -u app/tests/aurora/schedule-test-browser.py` | PASS `schedule-test-browser-011327.log` |
| `python -u app/tests/aurora/vault-cards-browser.py` | PASS `vault-cards-011343.log` |
| `python -u app/tests/aurora/vault-lifecycle-browser.py` | PASS `vault-browser-011353.log` |
| `python -u app/tests/aurora/dictation-browser.py` | PASS `dictation-011400.log` |
| `python -u app/tests/aurora/composer-browser.py` | PASS `composer-011406.log` |
| `node --test app/tests/aurora/composer-drafts-test.mjs app/tests/aurora/outbox-test.mjs` | PASS `composer-unit-011440.log` |
| `python -u app/tests/aurora/chat-loading-browser.py` | PASS `chat-loading-011442.log` |
| `python -u app/tests/aurora/read-state-browser.py` | PASS `read-state-011455.log` |
| `python -u app/tests/aurora/list-focus-browser.py` | PASS `list-focus-011514.log` |
| `python -u app/tests/aurora/selection-browser.py` | PASS `selection-browser-011519.log` |
| `python -u app/tests/aurora/table-viewer-browser.py` | PASS `table-viewer-011530.log` |
| `python -u app/tests/aurora/table-scroll-browser.py` | PASS `table-scroll-011538.log` |
| `python .audit/2026-10-28-grok/syntax.py` | PASS `syntax-011545.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -count=1 ./internal/web` | PASS `web-assets-011548.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe build -o E:\Sach\Sua\neko-aurora\.audit\2026-10-29-memory-lifecycle\neko-aurora.exe ./cmd/neko-aurora` | PASS `build-011552.log` |

Source manifest `11431c5f50e392f840510b5ab96e032da968c667a23db230e4f21b5dd659ed2f`; `code-checkpoint.json` verifies 1,810 source files against the code commit, accounting for existing Git LF normalization. `.audit/2026-10-29-memory-lifecycle/validation.jsonl` records cwd, UTC timestamps, environment and before/after manifests. Five candidate files and 471 newly reachable history blobs had no recognizable secret matches before the code checkpoint; this is not exhaustive detection.

## Remaining server contracts

The scope checks above govern UI dispatch and response application. They do not cancel already-sent server requests, provide server-side workspace/revision fencing, establish durable mutation receipts, prevent concurrent positional edits from another client, or guarantee storage success. Existing import revision checks remain; broader CRUD/summarize contracts need separate work. A failed acknowledgement is not proof that the server did nothing. Pending UI data may remain in closures until requests finish; clearing hidden DOM is not heap zeroization. Memory drafts remain temporary and are deliberately cleared on leaving the panel. No cloud sync, real-model quality, provider, Tauri or physical-device acceptance is claimed.

Server review identified independent persistence issues: several memory mutations update RAM/history before confirming the main file, and some owner API paths discard write failures. These remain queued for the next persistence unit. Coverage stays at 19 partially validated changelog entries, one excluded behavior and 800 entries without feature-specific evidence.
