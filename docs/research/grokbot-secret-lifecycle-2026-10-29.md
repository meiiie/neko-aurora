# Credential-card ownership and generic secret API

Code commit `73e92ebd948e717f112dcd984435550ce60089d5`, based on `a708a8518868cd1fb0a221e78af62d9ae20bff06`. Solo pstack Feature / Prove It Works / Show Me Your Work and webapp-testing. This is follow-up reliability work for Aurora credential entry; it does not count as another completed Grok Bot feature.

Generic secret cards now bind writes and acknowledgements to their originating workspace, chat visit and mounted card. Leaving a chat, switching workspace or expiring the owner session clears entered values. Reopened cards have fresh handlers. Old success/failure responses cannot mutate a newer card, publish a completion notice or trigger a state refresh. A successful current save invalidates the card cache and shows completion; rejected saves retain the current draft for explicit retry. Server secret-name changes also invalidate cached generic cards.

Credential draft restoration now matches workspace, agent, work, message, type/name and label, plus the site for typed Vault cards. Same-context refreshes retain edits, including cleared or changed prefilled fields. A changed target/type/label cannot inherit old values. Old DOM fields are scrubbed before rerender, then compatible fields are restored from the temporary in-memory snapshot. Guidance uses the existing context key. No browser storage is introduced. SW cache v77.

Generic `/secrets` list/save/delete responses are no-store. GET validates the query with ParseQuery: supplied workspace must occur once and be nonempty, malformed encoding/separators return 400, mismatched workspace returns 409, and successful responses include workspaceId. Save accepts workspaceId and uses the existing bounded 16 KiB decoder with unknown-field and trailing-JSON rejection. Delete applies that decoder and a read-only guard. Failed validation/scope/read-only checks leave credentials intact. Legacy omitted workspace on GET and omitted/empty workspace on writes remain accepted; empty delete bodies (and the existing null-object behavior) remain compatible. Legacy unscoped callers therefore do not receive workspace fencing.

## Evidence

Windows amd64; Go 1.27.1, Node 25.9.0, Python 3.13.7/Playwright, Chrome 156.0.8078.49. Synthetic values, temporary stores and loopback HTTP/SSE fixtures only. Desktop 1280x900 entry and mobile 390x844 completion screenshots inspected. No real credentials, model/provider/account or external execution.

- RED `secret-api-024104.log` reproduced wrong-workspace access, read-only deletion, cacheable responses, malformed query acceptance and trailing-JSON mutation. RED `secret-browser-024116.log` reproduced retained inputs and late response ownership failures; session expiry already scrubbed generic values.
- `secret-browser-024234.log` exposed lost edits to a prefilled site. The fixture then explicitly targeted the password for changed-target cases and separately tested edited/cleared site preservation. `secret-browser-024324.log` retained the site regression. `secret-browser-024404.log` also reproduced stale completion state after a server refresh. Final source fixes both and all 14 new browser groups pass.
- **25 new Go test/subtest events and 14 new browser groups PASS**. **Full Go: 2,005 PASS / 38 SKIP, zero FAIL across 25 packages**. **121 browser groups and 8 Node tests PASS**; vet, syntax, embedded assets and native build also pass on the same final manifest.

| Exact command | Result / log |
|---|---|
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -v -count=1 -timeout=120s -run TestSecret.*Lifecycle ./internal/aurora` | PASS `secret-api-024438.log` |
| `python -u app/tests/aurora/secret-lifecycle-browser.py` | PASS `secret-browser-024443.log` |
| `python -u app/tests/aurora/vault-hints-browser.py` | PASS `hint-browser-024501.log` |
| `python -u app/tests/aurora/vault-lifecycle-browser.py` | PASS `vault-browser-024508.log` |
| `python -u app/tests/aurora/vault-cards-browser.py` | PASS `vault-cards-024515.log` |
| `python -u app/tests/aurora/chat-loading-browser.py` | PASS `chat-loading-024525.log` |
| `python -u app/tests/aurora/content-i18n-browser.py` | PASS `content-browser-024535.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -json -p=1 -count=1 -timeout=300s ./...` | PASS `go-serial-024546.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe vet ./...` | PASS `vet-024901.log` |
| `python -u app/tests/aurora/adoption-browser.py` | PASS `browser-024904.log` |
| `python -u app/tests/aurora/read-state-browser.py` | PASS `read-state-024927.log` |
| `python -u app/tests/aurora/composer-browser.py` | PASS `composer-024945.log` |
| `node --test app/tests/aurora/composer-drafts-test.mjs app/tests/aurora/outbox-test.mjs` | PASS `composer-unit-025019.log` |
| `python .audit/2026-10-28-grok/syntax.py` | PASS `syntax-025021.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -count=1 ./internal/web` | PASS `web-assets-025024.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe build -o E:\Sach\Sua\neko-aurora\.audit\2026-10-29-secret-lifecycle\neko-aurora.exe ./cmd/neko-aurora` | PASS `build-025026.log` |

Go commands run in app/native, others at repository root. Manifest `d00813742f4006b8bb055b99ecb81023b2974ce7d2500adb023c85105373722c`; 1,822 files verified against the code commit with existing Git LF normalization. Local validation.jsonl records commands, cwd, UTC timestamps and unchanged before/after sources. Before the code checkpoint, 5 candidates and 540 newly reachable history blobs had no recognizable secret matches; the scanner is not exhaustive.

## Limits

DOM scrubbing is not cryptographic JavaScript memory erasure. Requests already sent may still complete on the server after a view closes; no cancellation, idempotency receipt, revision/CAS, cross-process lock or new request deadline is added. An explicit retry or a new view can still overwrite the same named secret. Item persistence and activity logging remain separate. Scope checks do not add authentication; existing owner route/session protection remains responsible for that. Connector configuration screens and Gmail/SMTP mutations are separate from the generic credential-card flow and were not redesigned here.

Core/Rust unchanged; their full gates were not repeated. No cloud, Tauri-runtime, screen-reader application or physical-phone acceptance. The source inventory remains 22 partially validated bullets, one excluded and 797 without feature-specific evidence; this repair earns no additional upstream coverage.
