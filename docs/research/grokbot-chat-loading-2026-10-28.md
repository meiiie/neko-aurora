# Chat history and event callbacks belong to the current workspace

Code commit: `dc61d4b8f054a15892009b5891c7af76130fe78a`, based on `aa65dbb335a78f2d5bb9604aae86b9c87b77835d`. Solo pstack continuation of [read-state resilience](grokbot-read-state-2026-10-28.md).

The browser fixture reproduced an older history request replacing a newer response for the same conversation. Aurora now admits only the latest `/state` response and the latest history request for each bot. History is also bound to the cache object that existed when the request began, so replacing workspace state invalidates that request even after A/B/A. Stale successes and failures are discarded; current failures remain visible. Conversation-open and task-navigation continuations check their original cache/current conversation before showing an error or moving focus.

Changing workspace closes the old EventSource and creates a new one. Every event/open/error callback checks stream identity, as do delayed draft rendering and notification callbacks waiting for service-worker readiness. Old callbacks cannot update current chats, drafts, briefing or notifications. Current stream delivery and notification display remain available. Old conversation context menus close on workspace change. Service worker cache is v62.

## Validation

Windows amd64; Python 3.13.7/Playwright, Chrome 156.0.8078.49, Go 1.27.1, Node 25.9.0. Loopback fixtures and isolated ephemeral browser profiles, no provider account, live grant, paid call, push or deployment. Exact cwd/environment/commands: `.audit/2026-10-28-chat-loading/validation.jsonl`.

| Command | Result / audit log |
|---|---|
| `python -u app/tests/aurora/chat-loading-browser.py` | PASS `chat-loading-221115.log` |
| `python .audit/2026-10-28-grok/syntax.py` | PASS `syntax-220621.log` |
| `python -u app/tests/aurora/read-state-browser.py` | PASS `read-state-220637.log` |
| `python -u app/tests/aurora/adoption-browser.py` | PASS `browser-220702.log` |
| `python -u app/tests/aurora/composer-browser.py` | PASS `composer-220732.log` |
| `python -u app/tests/aurora/mcp-discovery-browser.py` | PASS `mcp-browser-220810.log` |
| `python -u app/tests/aurora/mcp-config-browser.py` | PASS `mcp-config-browser-220824.log` |
| `go.exe test -v -count=1 -timeout=180s -run TestMCPOAuthBrowserLocalCookieFlow ./internal/aurora` | PASS `oauth-browser-220852.log` |
| `go.exe test -count=1 ./internal/web` | PASS `web-assets-220921.log` |
| `go.exe build -o E:\Sach\Sua\neko-aurora\.audit\2026-10-28-chat-loading\neko-aurora.exe ./cmd/neko-aurora` | PASS `build-220930.log` |

The new suite has 12 scenario groups: held responses across workspace round trips, same-workspace response order, stale state and failure handling, old/current event ownership, deferred notifications, context menu closure and current error recovery. Regression suites: 9 read-state, 21 common UI, 23 composer, 7 MCP discovery, 10 MCP configuration and 7 OAuth browser scenarios. The OAuth suite uses the real local Go API/cookie/callback/service worker with fake token endpoints. Syntax, embedded-asset tests and a new Go executable build passed.

The focused suite's EventSource and notification registration are injected test doubles. They validate callback ownership, not real network SSE teardown/reconnect or operating-system notification delivery. The source assignment before an `await` already preserved the old cache object across workspace changes: both cross-workspace history cases passed on the baseline. The failing RED (`chat-loading-220230.log`) is specifically same-workspace out-of-order history. Initial GREEN `220417` passed 11 groups; deferred-notification coverage brought the final count to 12.

The broader gates share manifest `1b67277d6be73d7f4f6d56358e9d20316e927aa07fc0f95b07d13d207c82e087`. Final focused run `chat-loading-221115.log` and code commit use `da0ccbf671e4ff54c4ef823a0dd180c4bb5c147ff34140f3ad60df73893b4e67`; 1796 files verified with existing Git LF normalization. The sole intervening source difference is the test's positive wait for the old error toast to disappear before the final screenshot. Product source is identical across both manifests. Mobile 390×844 and desktop interactions were checked; the final mobile screenshot in the ignored audit directory was inspected. This is not physical-device acceptance.

Candidate scan: 3 files and 381 newly reachable history blobs, zero recognizable matches. Source coverage remains 12 partially validated entries; this follow-up does not claim another changelog item complete.

## Limits

UI fencing does not replace server-side authorization or cancel every outstanding GET. Same-workspace ordering between an HTTP snapshot and an intervening SSE event still needs a server sequence/merge contract. Browser push, APNs, disconnected devices and cross-tab notification synchronization are outside these fixtures. Full Go/Core/Rust suites were not rerun for this frontend-only change.
