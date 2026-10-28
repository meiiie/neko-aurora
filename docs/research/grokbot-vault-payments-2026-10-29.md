# Vault preferred payment card

Code commit `92d9658a9acd2c5c4ff9765bcb08d23fa8b61cde`, based on `2d76761998a7e010427568a5e64b5c2fb61c3bab`. Solo pstack Feature / Prove It Works / Show Me Your Work and webapp-testing. Adapts the Payments view and owner-selected default from [Grok Bot 0.77.0](https://x.ai/changelog/bot#v0.77.0), New entry 5, source line 22, SHA256 `e807c1d35ad5f6a80f8df6e31898aed7faf3b554066e81efa802ff6fa5e6ffc8`.

Vault now has All and Payments tabs. Payments filters saved cards, explains that use still needs approval and offers adding a card. Owners can mark one card as preferred, replace that choice or explicitly clear it. No first-card or deletion fallback is chosen automatically. The badge appears in both views and the preferred card sorts first. Owner labels and masked summaries remain literal across language changes. New controls and the visible payment-view chrome have Vietnamese/English strings. Tabs implement selected state, roving focus and Arrow/Home/End keys; successful preference saves restore button focus when the user has not focused another control. SW cache v78.

Storage records a backward-compatible optional preferredPayment flag in existing public card metadata. This non-secret flag is plaintext alongside labels and masked hints; card credentials stay in the sealed payload. One Vault mutex covers validating the card, replacing all preference flags and one persistence write, with in-memory rollback on a write error. Clear removes the preference; deleting the selected item removes it with that item. Credential, label and guidance edits preserve the preference. It is excluded from credential fields, reveal/fill maps and environment names. Older stores without flags have no default; older app versions that rewrite metadata may discard this new optional flag.

POST `/app/api/vault/payments` accepts an explicit workspaceId and a required preferredItem string (empty means clear). Missing/empty workspace, absent/null item, malformed/oversized/unknown/trailing JSON and non-card targets are rejected. A different workspace and read-only ledger return 409; missing item returns 404. The response is no-store and includes workspaceId/preferredItem. The operation updates selection metadata and best-effort activity, without executing a credential use or creating a grant.

The bot's Vault metadata marks the preferred card and the prompt recommends proposing it when suitable. Preference is selection guidance, not execution authority. vault_use still parks the existing approval, and changing preference does not retarget a pending approval to another card. The UI applies only acknowledgements matching the original workspace, view and requested preference. Duplicate activation within the same mounted control is blocked. Failures preserve displayed state and offer explicit retry; old acknowledgements cannot rewrite a reopened/new-workspace view. A successful acknowledgement updates the current metadata view without revealing or resending credential values.

## Evidence

Windows amd64; Go 1.27.1, Node 25.9.0, Python 3.13.7/Playwright, Chrome 156.0.8078.49. Temporary stores, synthetic test card numbers, loopback metadata fixtures and direct tool-contract tests. No real card, payment, model/provider/account or grant. Mobile 390x844 Vietnamese and desktop 1280x900 English screenshots visually inspected.

- RED `payment-api-025048.log`: the preference endpoint did not exist (404). RED `payment-browser-025121.log`: no Payments tabs existed. RED `payment-browser-025555.log`: the new save action lost keyboard focus; final code restores owned focus after acknowledgement.
- **17 new Go test/subtest events and 10 new browser groups PASS**. Storage tests cover persist/reopen, replacement/clear, delete without fallback, edit preservation, sealed credentials, failed-write rollback, concurrent choices and interleaving with an item edit. API tests cover strict scope/body/type/read-only checks, metadata without card numbers, pending approval and unchanged approval target.
- **Full Go: 2,022 PASS / 38 SKIP, zero FAIL across 25 packages**; 55 Vault, 68 Aurora/Vault and 123 MCP events PASS. **131 browser groups and 8 Node tests PASS**, plus vet, syntax, embedded assets and native build on one unchanged final source manifest.

| Exact command | Result / log |
|---|---|
| `python -u app/tests/aurora/vault-payments-browser.py` | PASS `payment-browser-025615.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -v -count=1 -timeout=120s -run TestItemPaymentPreference ./internal/vault` | PASS `payment-store-025625.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -v -count=1 -timeout=120s -run TestVaultPaymentPreference ./internal/aurora` | PASS `payment-api-025627.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -v -count=1 -timeout=120s -run TestSecret.*Lifecycle ./internal/aurora` | PASS `secret-api-025632.log` |
| `python -u app/tests/aurora/secret-lifecycle-browser.py` | PASS `secret-browser-025638.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -v -count=1 -timeout=120s -run TestItemHints ./internal/vault` | PASS `hint-store-025656.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -v -count=1 -timeout=120s -run TestVaultHints ./internal/aurora` | PASS `hint-api-025659.log` |
| `python -u app/tests/aurora/vault-hints-browser.py` | PASS `hint-browser-025704.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -v -count=1 -timeout=120s -run TestItemMerge ./internal/vault` | PASS `item-merge-025712.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -v -count=1 -timeout=180s -run TestVault|TestItem ./internal/aurora ./internal/vault` | PASS `vault-all-025715.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -v -count=1 ./internal/vault` | PASS `vault-025721.log` |
| `python -u app/tests/aurora/vault-lifecycle-browser.py` | PASS `vault-browser-025724.log` |
| `python -u app/tests/aurora/vault-cards-browser.py` | PASS `vault-cards-025731.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -v -count=1 -timeout=180s -run TestMCP ./internal/aurora` | PASS `mcp-025741.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -json -p=1 -count=1 -timeout=300s ./...` | PASS `go-serial-025748.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe vet ./...` | PASS `vet-030104.log` |
| `python -u app/tests/aurora/adoption-browser.py` | PASS `browser-030107.log` |
| `python -u app/tests/aurora/content-i18n-browser.py` | PASS `content-browser-030131.log` |
| `python -u app/tests/aurora/chat-loading-browser.py` | PASS `chat-loading-030138.log` |
| `python -u app/tests/aurora/read-state-browser.py` | PASS `read-state-030148.log` |
| `python -u app/tests/aurora/composer-browser.py` | PASS `composer-030207.log` |
| `node --test app/tests/aurora/composer-drafts-test.mjs app/tests/aurora/outbox-test.mjs` | PASS `composer-unit-030242.log` |
| `python .audit/2026-10-28-grok/syntax.py` | PASS `syntax-030243.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -count=1 ./internal/web` | PASS `web-assets-030245.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe build -o E:\Sach\Sua\neko-aurora\.audit\2026-10-29-vault-payments\neko-aurora.exe ./cmd/neko-aurora` | PASS `build-030248.log` |

Go commands run in app/native, others at repository root. Manifest `24dc9e0e911121a48b5dc200dbb2458ef92e341734b6ab3173046eb6f4fa1c69`; 1,825 files verified against the code commit with existing Git LF normalization. Local validation.jsonl records exact commands/cwd/UTC times and unchanged before/after sources. 10 candidate files and 547 newly reachable history blobs had no recognizable secret matches before the code checkpoint; scanning is not exhaustive.

## Limits and remaining acceptance

This is a preference among Aurora's existing saved card items. It adds no payment processor, Link account, purchase transaction, automatic charge or integration with an external password manager. Existing Vault browser fill can submit a form after approval, but no real checkout was exercised here. No claim of model preference adherence, merchant acceptance, billing accuracy or real payment safety follows from these fixture tests.

Concurrent owner clients remain last-writer-wins; the current tab can show an older acknowledged snapshot until Vault is reopened. A request already sent may finish after the view closes. No request cancellation, durable receipt, revision/CAS, cross-process lock, transaction with the activity journal or new deadline is added. No crash/power-loss or Go race-detector claim. Core/Rust are unchanged; their full gates were not repeated. No cloud multi-device, Tauri-runtime, screen-reader application or physical-phone acceptance. Other Vault editing screens still have pre-existing untranslated labels.

This source entry receives **partial-local-validation**. Coverage becomes **23 partial, one excluded and 796 without feature-specific evidence** out of 820 source bullets; these bullets are not a count of independent product features, and complete upstream parity is not claimed.
