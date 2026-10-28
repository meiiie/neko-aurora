# Vault view lifetime and workspace-bound requests

Code commit: da64ddfa3e676335b915b982cb14caef895378d6, based on `5a1bd71980caf9c801531c63517f8525eee9c4d3`. Solo pstack Feature / Prove It Works / Show Me Your Work and webapp-testing continuation of the Vault backlog.

Revealed values and entered secrets previously remained in hidden DOM after Escape/scrim closure, and delayed list/reveal/save/delete continuations could update a later Vault view. The common close/swap path now revokes the view, clears inputs, removes Vault DOM and cached values, and cancels the hide timer. Workspace changes also clear message-node caches so handlers captured under an earlier workspace are not reused. Leaving a chat clears Vault-form inputs and evicts those cards from the node cache; reopening rebuilds their current chat ownership.

Each Vault view and async action is bound to its originating workspace, view generation and connected element. Late responses/errors cannot replace a newer edit or expose old values. Reveal/delete/save buttons prevent duplicate dispatch while pending. Revealed values still auto-hide after 30 seconds, and collapsing a row or entering edit clears them. Editing an existing login continues to show an empty masked password field; values are fetched only by explicit reveal. No secret is copied to the model by these UI changes.

New UI requests name workspaceId for list/save/reveal/delete. The server rejects a supplied mismatch before reading, mutating or logging Vault use. List/reveal responses echo their workspace and use Cache-Control:no-store, which the UI validates before displaying them. Legacy callers omitting workspaceId remain supported (including an empty reveal/delete body); this is not a mandatory fence for every legacy client. Reveal/delete now parse bounded JSON, reject unknown fields, trailing JSON, malformed/type-invalid/oversized bodies, and reject ambiguous workspace query parameters. Save uses the same bounded strict decoder. Delete also honors a read-only ledger, matching save. The service-worker cache is v67.

## Validation

Windows amd64; Go 1.27.1, Node 25.9.0, Python 3.13.7/Playwright and Chrome 156.0.8078.49. Allowlisted test environment excludes provider credentials; temporary encrypted Vaults and loopback/injected state responses contain only fixture values. No real credential, provider, payment, grant or deployment. No JS heap-zeroization, live account or physical-phone claim.

- RED `vault-lifecycle-234539`: wrong-workspace list/reveal/delete returned 200; save rejected the previously unknown workspace field with 400; responses lacked binding; read-only delete returned 200. Server repair passes in `234635`.
- RED `vault-browser-234937`: after an explicit reveal, Escape left the fixture secret in the hidden Vault DOM. The first UI repair passes nine scenarios, then `235147` fails the timer fixture: the virtual clock was installed after the real hide timer was registered. Moving clock installation before navigation makes that case pass.
- `235337` passes ten scenarios, then the fixture clicks a chat during the existing 400 ms post-sheet-close ghost-click guard; the ignored click leaves the thread offscreen. The fixture now waits for that defined guard before navigating. Review also found cached Vault cards retained an earlier chat generation; leaving the chat now evicts them, and workspace changes clear all cached message nodes. Twelve browser groups pass in `235529` and on the final source.
- Final focused API tests: 21 PASS test/subtest events. Focused Vault/Item selection: 32 PASS, 2 SKIP (Chrome form-fill discovery and command-use fixture without a sandbox). These skips do not negate the separate real-Chrome UI tests, but no form-filling/command-injection acceptance is claimed here.
- **Full Go: 1868 PASS, 38 SKIP, 0 FAIL across 25 passing packages**; vet/build pass. **129 browser scenario groups** pass (12 Vault + the previous 117 dictation/composer/adoption/loading/read/focus/selection/table groups), along with 8 Node draft/outbox tests, JS syntax and embedded assets. Checks began on 28 October and continued after midnight on 29 October in Asia/Saigon; UTC timestamps remain explicit in the audit.

| Command | Result / log |
|---|---|
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -v -count=1 -timeout=120s -run TestVault(WorkspaceFence|ScopedResponsesAndLegacyCaller|DeleteHonorsReadOnlyLedger|ActionBodies|LegacyEmptyActionAndAmbiguousQuery) ./internal/aurora` | PASS `vault-lifecycle-235653.log` |
| `python -u app/tests/aurora/vault-lifecycle-browser.py` | PASS `vault-browser-235701.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -v -count=1 -timeout=180s -run TestVault|TestItem ./internal/aurora ./internal/vault` | PASS `vault-all-235707.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -json -p=1 -count=1 -timeout=300s ./...` | PASS `go-serial-235713.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe vet ./...` | PASS `vet-000059.log` |
| `python -u app/tests/aurora/dictation-browser.py` | PASS `dictation-000111.log` |
| `python -u app/tests/aurora/composer-browser.py` | PASS `composer-000118.log` |
| `node --test app/tests/aurora/composer-drafts-test.mjs app/tests/aurora/outbox-test.mjs` | PASS `composer-unit-000152.log` |
| `python -u app/tests/aurora/adoption-browser.py` | PASS `browser-000153.log` |
| `python -u app/tests/aurora/chat-loading-browser.py` | PASS `chat-loading-000217.log` |
| `python -u app/tests/aurora/read-state-browser.py` | PASS `read-state-000228.log` |
| `python -u app/tests/aurora/list-focus-browser.py` | PASS `list-focus-000247.log` |
| `python -u app/tests/aurora/selection-browser.py` | PASS `selection-browser-000252.log` |
| `python -u app/tests/aurora/table-viewer-browser.py` | PASS `table-viewer-000303.log` |
| `python -u app/tests/aurora/table-scroll-browser.py` | PASS `table-scroll-000309.log` |
| `python .audit/2026-10-28-grok/syntax.py` | PASS `syntax-000316.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -count=1 ./internal/web` | PASS `web-assets-000318.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe build -o E:\Sach\Sua\neko-aurora\.audit\2026-10-28-vault-lifecycle\neko-aurora.exe ./cmd/neko-aurora` | PASS `build-000322.log` |


Source manifest: `667dcfcce5570d47fa83f03152685ec2a22e54d44ea8a71104f1eeca45da9132`. Exact commands, cwd, timestamps, base, before/after manifests and outcomes are in `.audit/2026-10-28-vault-lifecycle/validation.jsonl`. `code-checkpoint.json` verifies 1805 source files against `da64ddfa3e676335b915b982cb14caef895378d6` (existing Git LF normalization accounted for). Before checkpoint, 5 candidate files and 434 newly reachable history blobs were scanned with zero recognizable secret matches; this is not exhaustive secret detection.

## Limits

Closing a view does not cancel a mutation already dispatched to the server; it may complete, and reopening reads current state. There is no new durable save/delete receipt, item revision fence, cross-process transaction or remote revocation. Legacy unscoped API callers do not gain the new workspace fence. UI cleanup removes accessible DOM values and timers; JavaScript and browser/network buffers cannot promise secret-memory erasure. Vault-card completion markers still use the older local storage mechanism and need separate receipt/storage-failure work. This unit does not add payments, address suggestions, purpose hints, provider credential fill, team sharing, private certificate authorities or Windows Core credential sealing. Rust/Core full gates are not repeated because their code is unchanged.

This records Vault foundation work without crediting another changelog bullet. Coverage remains 15 partially validated entries, one excluded behavior and 804 entries without feature-specific evidence.
