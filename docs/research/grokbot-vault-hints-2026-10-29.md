# Vault usage guidance

Code commit `f6ebaed0d7ba316d40e804021db3b6b413d484a8`, based on `e7b78fa62a0d66715ac79f2ec89e97a695174d1a`. Solo pstack Feature / Prove It Works / Show Me Your Work and webapp-testing. Adapts the owner-authored Hints field from [Grok Bot 0.77.0](https://x.ai/changelog/bot#v0.77.0), Improved entry 18 (source line 56, SHA256 `0e1eebc77d16fab729831496dd69503f0f0c62753f15ffe7b855bd73758b691f`).

Owners can add, replace or clear usage guidance when creating/editing a Vault entry or responding to a credential card in chat. Item details show that guidance without revealing a password. The form explains that bots can read it and every use still needs approval. Literal text rendering preserves content across language changes and displays markup as text. Textareas are cleared on close, workspace change, successful save and chat leave. Failed saves keep edits; a late save response cannot overwrite a newer form. A refreshed copy of the same credential card preserves unfinished guidance and focus, while a different card target/label/type or workspace does not inherit it. SW cache v76.

The owner API accepts optional `useHint`. Omission (including JSON null) preserves the previous hint; an explicit empty string clears it. The storage API accepts at most one optional hint, validates UTF-8, rejects NUL and limits input to 500 Unicode code points before trimming. The browser's maxlength is 500 UTF-16 code units. The existing 8192-byte serialized aggregate limit includes guidance. An internal reserved key stores it in the encrypted field map using the same atomic mutation as the credential fields. The plaintext Meta.Hint format is unchanged. Public Fields, reveal/fill values, filled-field lists and environment field names omit the reserved key. Save responses now consistently include filled field names, as list responses already did.

The bot's vault_list output quotes guidance as owner-authored data. The tool contract says it carries no execution authority, and vault_use still parks a pending approval, including when the guidance asks to ignore approval rules. No credential field is included in model metadata. Owner workspace and read-only checks apply to hint edits.

## Evidence

Windows amd64; Go 1.27.1, Node 25.9.0, Python 3.13.7/Playwright, Chrome 156.0.8078.49. Synthetic credentials and temporary stores, loopback fixtures and a direct tool-contract test; no live model/provider/account call. Mobile 390x844 edit/detail and desktop 1280x900 screenshots visually inspected. English mode still has untranslated pre-existing Vault labels.

- RED `hint-api-022446.log`: the existing API rejected the new field; `hint-browser-022547.log`: no guidance editor existed.
- `hint-browser-022719.log` hit the sheet-close ghost-click guard before opening chat; the fixture now waits 450 ms after close, matching existing UI behavior.
- RED `hint-browser-022927.log` reproduced guidance loss when a credential card's text changed. `hint-browser-023021.log` repeated it after an audit edit helper's exact-match assertion prevented any source write. The final implementation preserves same-card guidance/focus and clears it for a changed target.
- **8 new Go test/subtest events and 9 new browser groups PASS**. Store checks cover encrypted bytes/reopen, legacy preserve/clear, validation, aggregate size, rollback and concurrent hint/credential edits. API checks cover public metadata, credential exclusion, pending approval, workspace mismatch and read-only rejection.
- **Full Go: 1,980 PASS, 38 SKIP, zero FAIL across 25 packages**; 52 Vault, 51 Aurora/Vault and 123 MCP events PASS. **134 browser groups and 8 Node tests PASS**, plus vet, syntax, embedded assets and native build on the same final source manifest.

| Exact command | Result / log |
|---|---|
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -v -count=1 -timeout=120s -run TestItemHints ./internal/vault` | PASS `hint-store-023046.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -v -count=1 -timeout=120s -run TestVaultHints ./internal/aurora` | PASS `hint-api-023048.log` |
| `python -u app/tests/aurora/vault-hints-browser.py` | PASS `hint-browser-023054.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -v -count=1 -timeout=120s -run TestItemMerge ./internal/vault` | PASS `item-merge-023102.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -v -count=1 -timeout=180s -run TestVault|TestItem ./internal/aurora ./internal/vault` | PASS `vault-all-023105.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -v -count=1 ./internal/vault` | PASS `vault-023111.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -v -count=1 -timeout=180s -run TestMCP ./internal/aurora` | PASS `mcp-023114.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -json -p=1 -count=1 -timeout=300s ./...` | PASS `go-serial-023123.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe vet ./...` | PASS `vet-023449.log` |
| `python -u app/tests/aurora/vault-lifecycle-browser.py` | PASS `vault-browser-023452.log` |
| `python -u app/tests/aurora/vault-cards-browser.py` | PASS `vault-cards-023458.log` |
| `python -u app/tests/aurora/adoption-browser.py` | PASS `browser-023509.log` |
| `python -u app/tests/aurora/content-i18n-browser.py` | PASS `content-browser-023533.log` |
| `python -u app/tests/aurora/memory-lifecycle-browser.py` | PASS `memory-browser-023540.log` |
| `python -u app/tests/aurora/chat-loading-browser.py` | PASS `chat-loading-023559.log` |
| `python -u app/tests/aurora/read-state-browser.py` | PASS `read-state-023609.log` |
| `python -u app/tests/aurora/composer-browser.py` | PASS `composer-023629.log` |
| `node --test app/tests/aurora/composer-drafts-test.mjs app/tests/aurora/outbox-test.mjs` | PASS `composer-unit-023703.log` |
| `python .audit/2026-10-28-grok/syntax.py` | PASS `syntax-023704.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -count=1 ./internal/web` | PASS `web-assets-023707.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe build -o E:\Sach\Sua\neko-aurora\.audit\2026-10-29-vault-hints\neko-aurora.exe ./cmd/neko-aurora` | PASS `build-023710.log` |

Go commands run in `app/native`; other commands run at repository root. Manifest `e2996e19d888d155f5ea2064877a7bdf77906bbce9db366e7271fbd49b9857c6`; 1,820 files verified against the code commit with existing Git LF normalization. Local validation.jsonl records exact cwd, UTC times, commands and unchanged before/after source. 9 candidates and 527 newly reachable history blobs had no recognizable secret matches before the code checkpoint; detection is not exhaustive.

## Limits and remaining acceptance

Guidance is intentionally readable metadata for owners and bots. Its Vault copy is encrypted, but published list responses and model/work context can contain it; this is not a promise that every copy stays encrypted or that guidance is a credential-safe input. No real-model selection/adherence evaluation or adversarial prompt-injection evaluation is claimed. Approval parking is enforced by existing code; this feature adds no automatic payment, login, fill, command execution or grant. Bot-authored edits, payment preference, multi-contact fields and password import are separate upstream work.

Same-field updates from different clients remain last-writer-wins. There is no revision/CAS, cross-process lock, durable request receipt or new deadline for Vault mutations. Chat drafts remain in the current view and are intentionally cleared on leaving it. Existing audit append and item persistence are separate; no new crash/power-loss guarantee. Core/Rust were unchanged and their full gates were not repeated. No cloud multi-device, Tauri-runtime, screen-reader application or physical-phone acceptance.

The entry receives **partial-local-validation**. Coverage becomes **22 partial, one excluded and 797 without feature-specific evidence**; no complete upstream parity claim.
