# Vault contact channels — 2026-10-29

Code commit: `a01e3c8db8e6fdb81d45717b95576d40347119cc` on `codex/grokbot-adoption`. Local adaptation of [Grok Bot](https://x.ai/changelog/bot) 0.78.1 New-10 and 0.77.0 Improved-19. One agent implemented and reviewed under the user's no-subagent rule, pstack Feature / Prove It Works / Show Me Your Work and webapp-testing. Reference material was read only. No provider calls, accounts, grants, push or deployment.

## Behavior

An existing Vault identity can hold up to ten emails and ten phone numbers. Its editor adds, replaces, removes and chooses the primary value. Each contact operation saves immediately; a caption distinguishes these actions from Save changes for the general label/address/use-hint form. Masked rows never fetch plaintext for editing. General drafts survive contact updates, errors and explicit refresh. Busy operations exclude concurrent Save/Delete; an unfinished contact draft blocks general Save. Stale workspace/view acknowledgements cannot alter a reopened editor.

Contacts remain in the existing encrypted item, with a random revision checked while holding the Vault mutex through persistence. Old single-value items project compatible metadata without rewriting storage. Editing an old primary rotates the revision even before the first explicit contact operation. Regular label/address edits preserve secondary values. Failure to persist rolls back values, primary choice and revision. The first entry supplies the existing email/phone fill fields; secondary values join redaction but are not added to model-facing fill/environment fields. Owner reveal includes all channels, retains scoped copy buttons and scrubs visible values on close. Service-worker cache: 84.

Validation rejects duplicate values, malformed email/phone syntax, missing/stale revisions, unsupported actions, unexpected JSON fields, workspace mismatch and read-only ledger mutation. Syntax validation does not prove deliverability or normalize equivalent phone numbers/email addresses. Creation retains the original single email/phone inputs; editing adds more. No contacts import, remote sync, automatic sign-in or permission grant was added.

## Evidence and limits

The missing endpoint reproduced as HTTP 404 in contacts-api-131939.log. Two product defects found during review have RED/PASS evidence: short phone hints disclosed the entire short value (contacts-store-132757.log), and a legacy field edit retained revision zero, allowing a stale contact editor to remove the new value (contacts-store-133106.log). A further browser regression removed existing reveal-copy controls; contacts-browser-133626.log failed before the controls were restored. Incorrect fixture field-count and expanded-row assumptions were corrected separately; all original failures remain in validation.jsonl and decisions.tsv.

The contact browser fixture passes 11 scenario groups in real Chrome with synthetic loopback APIs: masked editing, both channel types, primary/replacement/removal, revision conflict/reload, draft and focus retention, IME suppression, pending-operation exclusion, explicit reveal/copy/scrubbing, late success/failure, workspace changes and English labels. Clipboard writes are intercepted by a fixture stub; no OS clipboard acceptance is claimed. Mobile 390px and English desktop 1280px screenshots were inspected. These are viewport checks, not physical-phone or Tauri WebView acceptance.

## Validation

Final source manifest: `129b789111d9bb6fc94c00a5c6a3a6b4ab88204783e55c310de96c672fdaf861`; 1838 source files checked against the Git tree with existing LF normalization accounted for. Windows amd64, PowerShell 7.6, Python 3.13.7, Chrome 156.0.8078.49, Node 25.9.0, Go 1.27.1. Subprocesses use an allowlisted environment excluding live credentials.

Full Go: **2048 PASS / 38 SKIP, 25 packages**, no failed tests. The full Go and vet manifests differ from the final source only in app.js and the contact browser test, where reveal-copy compatibility was restored. Those frontend paths, Vault editor/lifecycle, composer, localization, syntax, embedded assets and build were rerun. validation-source-equivalence.json records the exact differences for retained earlier checks. Core and Rust were unchanged and were not rerun for this unit.

| Gate | Result | Seconds | Log under .audit/2026-10-29-vault-contacts |
|---|---|---:|---|
| contacts-store | PASS | 5.1 | contacts-store-133117.log |
| contacts-api | PASS | 10.16 | contacts-api-133122.log |
| contacts-browser | PASS | 12.8 | contacts-browser-133641.log |
| editor-browser | PASS | 15.61 | editor-browser-133654.log |
| hint-browser | PASS | 9.51 | hint-browser-132858.log |
| payment-browser | PASS | 11.36 | payment-browser-132908.log |
| secret-browser | PASS | 20.1 | secret-browser-132920.log |
| vault-browser | PASS | 7.34 | vault-browser-133710.log |
| vault-cards | PASS | 13.04 | vault-cards-132948.log |
| content-browser | PASS | 7.99 | content-browser-133717.log |
| composer | PASS | 35.28 | composer-133726.log |
| composer-unit | PASS | 1.28 | composer-unit-133044.log |
| syntax | PASS | 2.79 | syntax-133801.log |
| web-assets | PASS | 5.63 | web-assets-133804.log |
| go-serial | PASS | 255.57 | go-serial-133146.log |
| vet | PASS | 4.47 | vet-133602.log |
| build | PASS | 5.43 | build-133810.log |

Exact commands, timestamps and snapshots are retained in validation.jsonl. From app/native, the full gate used `go test -json -p=1 -count=1 -timeout=300s ./...`, then `go vet ./...` and `go build -o <audit>/neko-aurora.exe ./cmd/neko-aurora`; Go resolved to E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe. Contact storage/API gates used `go test -v -count=1 -timeout=120s -run TestItemContacts ./internal/vault` and `-run TestVaultContacts ./internal/aurora`. Browser gates use `python -u app/tests/aurora/<fixture>.py`; Node composer/outbox uses `node --test app/tests/aurora/composer-drafts-test.mjs app/tests/aurora/outbox-test.mjs`. Documentation is checked with `python tools/check_docs.py` and the report's local links are resolved separately.

Before the code checkpoint, recognizable secret-pattern scans covered 14 candidate files and 613 newly reachable history blobs, with zero matches. This is not exhaustive secret detection. The unrelated pre-existing adoption-document whitespace edit remains outside both checkpoints.

## Backlog status

The two source bullets now carry partial local evidence in the [original index](grokbot-adoption-index-2026-10-28.json) and [incremental index](grokbot-adoption-index-2026-10-29.json): 28 partial, 1 excluded and 891 unvalidated across 920 stored bullets. These are source bullets, not independent features or a completion percentage. The broader backlog remains open. [Reload/composer evidence](grokbot-chat-identity-reload-2026-10-29.md) retains its separate fixture-versus-product limits.
