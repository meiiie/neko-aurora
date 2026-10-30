# Form drafts and validated structured answers

Code commit: `b08cee56b3c1a52650037fbcc81366323c85df8a`, based on `36895f7da661b4733998d8c0e1e246ee3c526130`. Solo pstack Feature / Prove It Works / Show Me Your Work. Extends the existing partial [Grok Bot form evidence](https://x.ai/changelog/bot); no reference code/assets copied.

## Behavior

New form definitions use answerVersion 1. The server validates exact field count, required values, finite decimal/scientific numbers, real ISO dates and allowed select options against the saved ledger schema. Each value is bounded to 2,000 Unicode code points, and the resulting text to 10,000 code points / 32,000 bytes. It derives the message from validated fields instead of trusting the client's preview. Version 0 forms remain compatible; older outbox attempts retain their original wire payload, avoiding changed hosted reservation digests.

Owner receipts persist the original field array, tied to source task, conversation, author and fixed nonce. Live admission and journal replay validate the source, reject metadata downgrades and deep-copy values. Different fields cannot replay an existing receipt even if their multiline summaries happen to be identical. Confirmed history renders each field under its original label; old text-only receipts retain a text fallback. Render caching updates when typed receipt data arrives later.

Unsent drafts persist per workspace, chat and form with an exact schema fingerprint, independently of the outbox. Reload never submits them. Corrupt or changed-schema records remain available for review until explicitly discarded; observed cross-window changes block overwrite. Save failures preserve current-window values, prevent unsaved dispatch and permit explicit clearing. Storage admits at most 128 drafts per workspace; bounded memory evicts only copies already saved, and tells the user to copy current input when all 128 memory slots hold unsaved drafts. Accepted receipts remove the draft. PWA cache v57 includes the new form renderer.

## Validation

Windows amd64, PowerShell 7.6, Go 1.27.1, Node 25.9.0, Python 3.13.7, Chrome 156.0.8078.48. Runner excludes provider/account environment variables; only local fixtures. Exact commands, source snapshots and results are in `.audit/2026-10-28-form-drafts/validation.jsonl`.

| Command (Go commands in app/native) | Result |
|---|---|
| `go test -v -count=1 -timeout=120s -run 'Test(FormValues\|FormHistory\|FormDefinition\|FormReceipt\|FormReplay)' ./internal/aurora` | RED: invalid fields accepted and forged preview trusted (`form-values-175858.log`); RED: distinct raw values replayed after restart (`form-values-181757.log`). Final PASS `form-values-182252.log` |
| `go test -v -count=1 -timeout=120s -run 'Test(OwnerForm\|OwnerMessage\|DirectForm)' ./internal/ledger` | PASS `form-ledger-182245.log`: deep copies, source/shape/nonce validation, exact retry after reopen, legacy nonce rejection, rehashed corrupt metadata rejection |
| `go test -json -p=1 -count=1 -timeout=300s ./...` | PASS: 1,710 test/subtest events, 37 existing skips, 0 failures, 25 packages; 190.52 s; `go-serial-182415.log` |
| `go vet ./...` | PASS `vet-182726.log` |
| `python -u app/tests/aurora/form-browser.py` | RED: draft lost on reload (`form-browser-180107.log`); RED: receipt hydration retained stale text fallback (`form-browser-183140.log`). Final PASS: 16 scenarios, `form-browser-183226.log` |
| `python -u app/tests/aurora/adoption-browser.py` | Final PASS: 21 scenarios, `browser-183301.log` |
| `go test -count=1 ./internal/web`; `go build -o <audit>/neko-aurora.exe ./cmd/neko-aurora` | Final PASS `web-assets-183325.log`, `build-183328.log` |
| Composer/upload/selection/schedule/schedule-test/recovery browser scripts; `node --test app/tests/aurora/composer-drafts-test.mjs app/tests/aurora/outbox-test.mjs` | Earlier PASS: 69 further browser scenarios and 8 Node tests, log timestamps 181344–181507; these preceded typed receipt display and final cache/memory changes |

Final backend/full-Go/vet source manifest: `8d8ea33c559dd1f74704a6200d4c4dd77a46b11846625b19afb3652977d8d3f2`. Final 37 browser scenarios, asset and embedded build manifest: `3b8a2c361ca5552697dc7c5e75dc7e7d0ac65ed9805108c221b20d6d8dbde1c6`; only frontend source/tests changed after full Go. Earlier broader UI manifest: `599c7c029993ed3e3e7603697d0cf46036ba363916ef9b7bd8e218b22ce54408`. No source changed during checks. The total 106 browser scenarios were exercised across these revisions, not all on the final manifest. Reviewed 390 × 844 draft and confirmed-value screenshots.

Fixture failures were corrected separately: function assignments accidentally evaluated as callbacks (180256), a held request prevented networkidle (180507), and a test invoked a closure-private function (183048); the final hydration regression uses the actual visibilitychange event. Final candidate scan: 20 files and 260 newly reachable history blobs, zero recognizable secret matches, no matched values printed.

## Limits

Browser storage is device-local, synchronous and not a cross-window transaction or cloud sync. Closing a window loses values that could not be persisted. Memory-only failures and capacity limits are visible. The DOM input maximum uses UTF-16 units, stricter than the server's Unicode limit for some characters. Existing journals remain readable, but rollback to older binaries with new journal fields is not promised. Hosted ordinary-message admission reserves quota before downstream field validation; failed admission may retain that reservation. Model understanding of canonical multiline text is not evaluated here. No power-loss, Go race, native Tauri or physical-device acceptance, actual account calls or deployment. The changelog entry remains partial.
