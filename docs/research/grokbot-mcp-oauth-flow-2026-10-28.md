# Aurora MCP OAuth lifecycle

Code commit: 339d1b7fdde8f18d02176a5c3398c255cf4445ab, based on `03bd4951b9c405279304447844bfc9b5f58f1ee9`. Solo pstack continuation; no subagents, live provider accounts, grants, deployment or push.

The Aurora MCP settings now support an explicit public-client OAuth flow: preview issuer/scopes, enter a pre-registered Client ID, start or resume browser login, read status, refresh, and forget credentials locally. Callback origins come from the actual literal-loopback listener or explicitly configured HTTPS public/hosted origin, never request Host or forwarding headers. OAuth is disabled for unconfigured origins. Manual header/secret configuration must be removed before OAuth start. Native callback ports must match the provider registration.

A private encrypted Vault record binds workspace, bot, the full MCP configuration and its revision, issuer, client ID, callback URI, endpoints and scopes. Each authorization has a generation and revision, and mutations reject stale callers. Fresh discovery must still match the previewed scope set and advertise S256. Callback validates state, expiry and issuer (including denials); exact completed callbacks return a receipt. Start persists before returning the browser link. Pending status can recover a lost start acknowledgement without creating a new grant.

Exchange and refresh persist a consuming marker before their single token POST. Storage failure before that marker sends nothing; response loss, malformed response or failure saving tokens leaves an unknown outcome across restart and cannot resend the old code/refresh token. Refresh receipts bind a caller nonce and prior revision. Successful refresh replaces stored tokens; an omitted refresh token does not retain the old one. Disconnect empties private secrets, retains a nonsecret tombstone to fence stale mutations, then closes the cached connection. It does not revoke permissions at the provider.

OAuth HTTP uses bounded, duplicate-key-checked UTF-8 JSON, public HTTPS with SafeClient dial checks, no cookies or redirects and no replayable token request body. Issuer comparison is exact. Every MCP connection lookup checks current credential phase/configuration/expiry before returning a cached handle; invalid credentials close that handle. OAuth MCP traffic uses the public-network client in native mode too. 401 remains an explicit reconnect failure and never automatically refreshes or replays a tool.

The public callback HTML/JS has no-store, no ETag, no-referrer and a restrictive CSP. It removes query/fragment before submitting to the authenticated same-origin API and never bootstraps an owner session or writes browser storage. Reload and duplicate URL parameters cannot resubmit a code. The callback files remain outside the service-worker cache. Settings carry workspace/configuration and authorization generation/revision, discard late responses after view/workspace changes and require status lookup after an uncertain mutation. Service worker v60 publishes the new controls.

## Validation

Windows amd64, PowerShell 7.6, Go 1.27.1, Node 25.9.0, Python 3.13.7, Chrome 156.0.8078.49. The audit runner passes an allowlisted environment excluding live/provider credentials. Commands, source hashes, timestamps and results are in `.audit/2026-10-28-mcp-oauth-flow/validation.jsonl`.

| Command (app/native unless noted) | Result |
|---|---|
| `go test -v -count=1 -timeout=180s -run TestMCP ./internal/aurora` | 123 pass, one opt-in browser skip; `mcp-210520.log` |
| `go test -v -count=1 -timeout=120s -run TestMCPOAuthCallback ./internal/api ./internal/hub ./internal/web` | Five tests/subtests pass; `oauth-auth-210541.log` |
| `AURORA_TEST_MCP_BROWSER=<repo>/app/tests/aurora/mcp-oauth-browser.py go test -v -count=1 -timeout=180s -run TestMCPOAuthBrowserLocalCookieFlow ./internal/aurora` | PASS, seven browser scenarios; `oauth-browser-210556.log`. Includes lost refresh acknowledgement and actual service-worker behavior |
| `python -u app/tests/aurora/mcp-discovery-browser.py` (repo root) | Seven scenarios pass; `mcp-browser-210730.log` |
| `python -u app/tests/aurora/adoption-browser.py`; `python -u app/tests/aurora/composer-browser.py` (repo root) | 21 and 23 scenarios pass; `browser-210746.log`, `composer-210811.log` |
| Node syntax checks; `go test -count=1 ./internal/web` | PASS `syntax-210741.log`, `web-assets-210743.log` |
| `go test -json -p=1 -count=1 -timeout=300s ./...` | 1,847 pass, 38 skip, zero fail across 25 packages; `go-serial-210845.log`, 228.84 seconds. The opt-in browser test was separately run above |
| `go vet ./...`; `go build -o <audit>/neko-aurora.exe ./cmd/neko-aurora` | PASS `vet-211234.log`, `build-211239.log` |

Backend fixtures use the real encrypted Vault, reopen services and inject precise write failures and fake HTTPS transports. Browser OAuth uses the real local Go API, existing Strict owner cookie and the actual service worker; the external login page and token endpoint are fixtures. API tests additionally verify local/public authentication gates and unauthenticated hosted callback behavior. These are not real public/hosted account acceptance or phone tests.

The first token parser test rejected quoted expires_in only after switching from json.Number to raw JSON/int64 validation (oauth-state-203205.log RED, 203258 GREEN). Browser harness failures are preserved: 205857 incorrectly assumed all storage was empty despite existing Aurora preferences; 210049 tried eval forbidden by the real CSP. The final harness checks callback storage writes/credential absence and inspects the actual service-worker cache. Discovery regression 210622 lost its selected chat after a workspace reset; a new document at the explicit chat URL repaired the harness without weakening product assertions (210730 GREEN).

Final source manifest `65db5e23749d5785370646783c0027f7f9b2af8c1fd342ab89bd634fce765a45` covers the discovery/composer/adoption browser suites, syntax/assets, full Go, vet and build. The earlier focused MCP/auth/OAuth-browser checks use `9771cfe8814222b7686af573c94bdbaf4024bb88f23858950c5d16ee3b5a6823`; the only subsequent source change was the discovery browser harness document reset described above. No source changed during checks. All 1,794 manifest files match the code commit, allowing existing Git LF normalization. Desktop/mobile screenshots were inspected. Candidate secret scan: 25 files and 339 newly reachable history blobs, zero recognizable key/token matches; no matched values printed.

## Limits and remaining work

This is pre-registered public-client support, not automatic DCR/CIMD, confidential-client registration or full OAuth provider certification. Token refresh rotation is a provider requirement; fixtures prove local replacement and unknown-outcome fencing, not remote implementation compliance. Any MCP list revision conservatively requires reauthorization of existing OAuth records; per-server configuration generations remain a possible refinement. Removing or renaming a configured server can leave an inert encrypted record; configuration edits do not promise credential garbage collection. Automatic step-up/scope union is not implemented: owner starts a new explicit flow, and tools are not automatically replayed. An already dispatched remote action may have succeeded.

Single-process/restart tests do not establish cross-process arbitration, power-loss durability, cloud synchronization or remote revocation. Core's separate SDK refresh recovery and credential storage remain separate backlog work. Local/public middleware and local Chrome are covered; successful hosted-account browser login, native/mobile shells, provider-specific contracts and release/install acceptance remain unvalidated. Grok adoption coverage remains partial.

References consulted on 2026-10-28: [MCP authorization](https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization) and [authorization security considerations](https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization/security-considerations). They inform issuer/resource/PKCE and token handling; the implementation and tests are original repository code.
