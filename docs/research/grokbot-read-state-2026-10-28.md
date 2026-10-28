# Chat read markers survive invalid or unavailable browser storage

Code commit: `c684bb995de97b59c4ce26c4ab27bbd0f8a0f26d`, based on `eca0a5081acaf8a5500da7ac783489fbeb5e4230`. Solo pstack Feature / Prove It Works / Show Me Your Work continuation. Narrow local adoption of Grok Bot 0.77.0 Fixed #7, [official changelog](https://x.ai/changelog/bot#v0.77.0).

A valid JSON primitive in the old read-state record made property assignment throw during `openThread`, leaving the conversation hidden. Read markers were also keyed by bot alone across workspaces. Aurora now validates stored maps and finite nonnegative timestamps in the JavaScript Date range, copies entries into prototype-free maps, and uses a versioned key scoped to the server workspace ID. Read/unread context actions use the same setter. Per-workspace in-window maps retain the owner's last action when storage is blocked or full, including A/B/A switches with identical bot IDs. Service worker cache is v61.

The old unscoped key is preserved and intentionally not imported into an arbitrary workspace. On first use of a scoped key, existing conversations are seeded as read, matching the previous initial-load behavior. This can reset old unread dots once; it does not delete chats or the legacy record. Changes made during storage failure remain in the current window and may be lost on reload. There is no cross-tab conflict resolution, server-side read API or multi-device synchronization in this unit.

## Validation

Windows amd64; Python 3.13.7 / Playwright, Chrome 156.0.8078.49, Go 1.27.1, Node 25.9.0. Loopback static assets plus injected API fixtures, isolated ephemeral browser profiles, no real provider/account or paid call. Final source manifest `0b34044ce749f74a35a9b8df84eec830b82bf355fff88aedda34310731b103a7`; 1795 files verified against the code commit with existing Git LF normalization. Exact cwd/environment/commands: `.audit/2026-10-28-read-state/validation.jsonl`.

| Command | Result / audit log |
|---|---|
| `python -u app/tests/aurora/read-state-browser.py` | PASS `read-state-215656.log` |
| `python .audit/2026-10-28-grok/syntax.py` | PASS `syntax-215720.log` |
| `go.exe test -count=1 ./internal/web` | PASS `web-assets-215725.log` |
| `python -u app/tests/aurora/adoption-browser.py` | PASS `browser-215732.log` |
| `python -u app/tests/aurora/composer-browser.py` | PASS `composer-215804.log` |
| `node --test app/tests/aurora/composer-drafts-test.mjs app/tests/aurora/outbox-test.mjs` | PASS `composer-unit-215843.log` |
| `go.exe build -o E:\Sach\Sua\neko-aurora\.audit\2026-10-28-read-state\neko-aurora.exe ./cmd/neko-aurora` | PASS `build-215859.log` |

Nine new browser scenario groups include seven malformed storage variants, denial before startup, context actions, reload, keyboard opening, workspace switches and memory fallback. The common suite has 21 behavior checks (plus its final summary line); composer has 23 checks, and draft/outbox unit tests have 8 passes. Embedded assets, syntax and rebuilt Go executable passed. Desktop 1280×900 and mobile viewport 390×844 screenshots were inspected in the ignored audit directory (`read-desktop-chat.png`, `read-mobile-list.png`, `read-mobile-chat.png`); these are not physical-phone acceptance. No Go runtime or Rust change; their full suites were not rerun for this frontend unit.

## Failed checks retained

- `read-red-214654` and `read-state-214721`: Chrome refused launch under the copied Core harness's overridden USERPROFILE. The browser harness now uses its existing allowlisted Windows environment and Playwright's ephemeral profile. These are infrastructure failures, not application RED.
- Application RED `read-red-214805`: the loopback server served the exact pre-change `app.js` from `eca0a5081acaf8a5500da7ac783489fbeb5e4230`; the malformed-record case left the chat hidden. The baseline file is retained under the ignored audit directory, not substituted in the working tree.
- `read-state-214826`: mobile interaction raced reload readiness. Added a positive visible-row wait before assertions that no unread dot exists; this also prevents an empty DOM from passing those assertions. Expanded-test `215106` failed on a Python indentation error, corrected before rerun.
- `browser-215229` and diagnostic `browser-diagnostic-215500`: local static-script requests failed; the diagnostic observed `ERR_CONNECTION_REFUSED`. The Python fixture server's backlog was 5. Increasing it to 64 in the common/read-state fixtures passed the final gates without weakening product assertions. Common-suite failure diagnostics now retain request failures.

Candidate secret scan: 4 files and 373 newly reachable history blobs, zero recognizable matches. No push or deployment. This closes the documented local read-marker failure paths, while broader workspace response/SSE ordering is a separate follow-up. Changelog coverage stays partial.
