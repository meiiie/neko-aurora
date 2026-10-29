# Reload fixture reliability and refreshed chat identity — 2026-10-29

Harness code: `fae9ab7af2237e1e303f29ec2fdadfd3dff4528c`. Chat identity code: `b9a800d4398a13734e9a7fe211ae997c99dba0b1`. Branch `codex/grokbot-adoption`. Work stayed in Neko Aurora with one agent, synthetic data and local services. No real grants, provider accounts, deployment or push. The pre-existing edit in the main adoption document is preserved outside these checkpoints.

## Captured reload failure and fixture repair

The composer regression failed again in `composer-124431.log`. This time diagnostics captured two requests, app.js and mcp-authorization.js, rejected with ERR_CONNECTION_REFUSED. Other assets still returned HTTP 200; document.readyState was complete, no JavaScript exception was reported, and window.__AURORA was undefined. The application had not loaded. The new startup recovery panel remained usable; the retained screenshot is `composer-refused-scripts.png`.

The composer fixture used Python's default listen queue of five. The committed `fixture-http-test.py` fills the pending queue before starting the accept loop, then requires all 32 clients to receive an HTTP response. With the old default, connections 6–32 timed out (`fixture-http-124723.log`); with a queue of 64, all 32 connect and are served (`fixture-http-124757.log`). `fixture_http.py` now supplies that bounded queue to all eight remaining fixtures that directly used the default server: composer, computer recovery, forms, MCP configuration/discovery, schedules, schedule test and uploads. Existing fixtures already using 64 were unchanged.

This repairs a demonstrated local test-server capacity defect and identifies the new reload failure at asset transport. It does not establish that the older untraced `composer-031038.log` had the same cause, or imply that the production Go listener had a five-connection queue. The startup recovery behavior from `1788bc2` remains covered by deterministic API, lost-script and dependency-failure scenarios. No assertion timeout was increased to hide the composer failure.

Three complete composer runs pass on the same final source: composer-124759.log, composer-125020.log, composer-125433.log. Browser tests do not prove phone, real-provider, cloud or OS-specific clipboard acceptance.

## Chat identity fixes

State refresh now updates the visible bot/group name and composer placeholder. The draft and focused input stay intact. A real browser EventSource reconnect, fed by synthetic loopback responses, refreshes the name without reopening the chat. Literal owner names remain literal under English UI and cannot inject markup.

The header avatar cache now includes bot appearance or the first two group members' appearances, so image/member changes render immediately. This does not redesign presence badges or task-list synchronization. The group welcome avatar row participates in layout instead of inheriting absolute positioning; its one/three avatars no longer overlap the title. Long names shrink with ellipsis in the header while preserving both controls, and unbroken welcome titles wrap inside the conversation. Service-worker cache version: 82.

RED evidence: `identity-browser-123441.log` (stale title), `identity-browser-123824.log` (zero-height avatar row), `identity-browser-124120.log` (menu outside mobile viewport), `identity-browser-124332.log` (stale avatar). Final fixture: `python -u app/tests/aurora/chat-identity-browser.py`, seven scenario groups. Mobile/desktop screenshots and the long-name/group cases were inspected. The read-only header diagnostic was used to inspect actual geometry and cached DOM, rather than guessing from a screenshot.

## Final validation

Source manifest `58f964f2abd7a5f153c658db56e8dc28dcccc674c1925e0588e846158948c95e` across 1832 files is verified against the final code commit, allowing existing Git LF normalization. Environment: Windows amd64, PowerShell 7.6, Python 3.13.7, Chrome 156.0.8078.49 headless, Node 25.9.0, Go 1.27.1; allowlisted environment excludes live credentials. Gates ran sequentially on frozen source; earlier intermediate runs remain in the audit trail and are not substituted for final evidence.

181 distinct browser scenario groups pass (repeat composer runs counted once), plus the 32-connection HTTP regression and 8 Node assertions. Syntax, Go embedded-asset validation and native build pass. All eight consumers of the new fixture server were exercised. Full Go evidence remains 2023 PASS / 38 SKIP from startup code `1788bc2`; `browser-only-delta.json` verifies that subsequent changes are limited to browser assets and Python fixtures. Native Go/Core/Rust/tool source files are unchanged; the full Go, Core and Rust suites were not rerun for these later browser-only changes.

| Gate | Result | Seconds | Final log in .audit/2026-10-29-chat-identity |
|---|---|---:|---|
| identity-browser | PASS | 7.69 | identity-browser-125007.log |
| copy-browser | PASS | 5.71 | copy-browser-125015.log |
| composer | PASS | 35.02 | composer-125433.log |
| chat-loading | PASS | 10.27 | chat-loading-125055.log |
| read-state | PASS | 22.16 | read-state-125106.log |
| content-browser | PASS | 8.24 | content-browser-125128.log |
| list-focus | PASS | 5.71 | list-focus-125136.log |
| browser | PASS | 25.93 | browser-125142.log |
| startup-browser | PASS | 19.08 | startup-browser-125208.log |
| recovery-browser | PASS | 12.01 | recovery-browser-125227.log |
| form-browser | PASS | 33.46 | form-browser-125240.log |
| mcp-config-browser | PASS | 30.45 | mcp-config-browser-125313.log |
| mcp-browser | PASS | 14.54 | mcp-browser-125344.log |
| schedule-browser | PASS | 7.81 | schedule-browser-125359.log |
| schedule-test-browser | PASS | 15.72 | schedule-test-browser-125407.log |
| upload-browser | PASS | 10.72 | upload-browser-125423.log |
| fixture-http | PASS | 1.66 | fixture-http-124757.log |
| composer-unit | PASS | 1.29 | composer-unit-125509.log |
| syntax | PASS | 2.49 | syntax-125510.log |
| web-assets | PASS | 3.61 | web-assets-125513.log |
| build | PASS | 4.12 | build-125516.log |

Exact commands, timestamps, environment and source snapshots are in validation.jsonl. Recognizable secret-pattern scans found no matches: 14 candidates/590 history blobs before splitting the first code checkpoint, then 4 candidates/600 history blobs before the second. This is not exhaustive secret detection. No unrelated staged changes were included.

## Remaining scope

The [0.78.1 incremental index](grokbot-adoption-index-2026-10-29.json) now marks Fixed-11 as partial local evidence for visible chat identity after reconnect. Task-list behavior is not implemented or validated by this unit. Across the stored 820+100 source entries, 26 have partial local evidence, one is excluded, and 893 remain unvalidated; these are source bullets, not independent feature counts or complete parity.

The [formatted reply copy report](grokbot-reply-copy-2026-10-29.md) covers the other 0.78.1 addition. Selection-to-Markdown and paste acceptance remain separate, as do broader provider, shopping, subscription, sharing, native update and platform work. The [earlier startup report](grokbot-composer-reload-2026-10-29.md) preserves the original investigation and limits.

Workflow: pstack Feature, Prove It Works and Show Me Your Work, with direct self-review under the user's no-subagent rule. The decisions.tsv keeps failures, investigation pivots and final evidence rather than erasing failed runs.
