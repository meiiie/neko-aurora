# Vault removal inside the editor

Code commit `43aed1963cae8b7978becd5db1520222217d22b3`, based on `854dd87bb2356a0aea5b1c7efc96fd56036eb982`. Solo pstack Feature / Prove It Works / Show Me Your Work and webapp-testing. Adapts masked saved-password editing and Remove on the form from [Grok Bot 0.77.0](https://x.ai/changelog/bot#v0.77.0), Improved entry 21, source line 59, SHA256 `21972dad0a9696444bf3371cbda7f7b190d992a376f976b109078a3b91b501af`. The same upstream bullet's bot-email ordering is not covered.

Existing-item forms now offer Remove alongside Save changes. Detail and editor share one deletion action with explicit confirmation, origin workspace and current-view checks. A pending save disables removal; pending removal disables saving, including keyboard and programmatic submit attempts. Cancel sends no request. Failed deletion preserves typed credentials/guidance and restores the controls. Successful deletion clears the owned form's inputs and reloads only its still-active view. Old success/failure responses cannot replace a reopened editor or a different workspace. New-item forms have no Remove control.

The saved password stays absent from the browser: an empty password input displays a masked preserve-existing placeholder. Editing does not request reveal, and the unchanged save contract preserves omitted/empty credential fields. Confirmation text preserves the literal item label and follows the UI language. The new mask, deletion feedback and relevant login-editor labels have Vietnamese/English text. The secondary removal control has aligned icon/text and readable foreground/background in light and dark themes. SW cache v79.

## Evidence

Windows amd64; Node 25.9.0, Python 3.13.7/Playwright, Chrome 156.0.8078.49; Go 1.27.1 for embedded assets/build. Synthetic values and loopback API fixtures only. Mobile 390x844 and desktop 1280x900 screenshots inspected; computed text contrast and icon/text alignment are checked in light/dark mode. These are browser checks, not physical-device or screen-reader application acceptance.

- RED `editor-browser-030309.log`: the saved-password mask/editor removal contract was absent.
- The first functional pass exposed a visual defect during screenshot review: existing danger styles produced red text on a red background and a misaligned icon. RED `editor-browser-030743.log` then reproduced the contrast/alignment defect before the final scoped style correction. Exact red/green command records are retained in the local validation.jsonl.
- `composer-031038.log` failed after switching UI language back to Vietnamese: the initial shell remained visible and the question card was absent. Instrumented command `python -u .audit/2026-10-29-vault-editor/composer-diagnostic.py` passed in `composer-diagnostic-031203.log` with no page errors; its request failures were the fixture's intentionally injected offline/response-loss POST failures. The original test passed in `composer-031239.log` on unchanged source. The startup/reload failure was not reproduced and its root cause remains unresolved; it is not claimed fixed.
- **12 new browser groups PASS**: masked editing without reveal, cancellation, failed deletion, deletion/save mutual exclusion, input scrubbing, no delete on creation, late HTTP200/500, workspace ownership, shared detail removal, English and keyboard operation. **170 total browser groups and 8 Node tests PASS**, plus syntax, embedded web tests and native build on one unchanged final source manifest.

| Exact command | Result / log |
|---|---|
| `python -u app/tests/aurora/vault-editor-browser.py` | PASS `editor-browser-030757.log` |
| `python -u app/tests/aurora/vault-payments-browser.py` | PASS `payment-browser-030810.log` |
| `python -u app/tests/aurora/vault-hints-browser.py` | PASS `hint-browser-030821.log` |
| `python -u app/tests/aurora/secret-lifecycle-browser.py` | PASS `secret-browser-030829.log` |
| `python -u app/tests/aurora/vault-lifecycle-browser.py` | PASS `vault-browser-030847.log` |
| `python -u app/tests/aurora/vault-cards-browser.py` | PASS `vault-cards-030855.log` |
| `python -u app/tests/aurora/adoption-browser.py` | PASS `browser-030908.log` |
| `python -u app/tests/aurora/content-i18n-browser.py` | PASS `content-browser-030935.log` |
| `python -u app/tests/aurora/memory-lifecycle-browser.py` | PASS `memory-browser-030945.log` |
| `python -u app/tests/aurora/chat-loading-browser.py` | PASS `chat-loading-031006.log` |
| `python -u app/tests/aurora/read-state-browser.py` | PASS `read-state-031017.log` |
| `python -u app/tests/aurora/composer-browser.py` | PASS `composer-031239.log` |
| `node --test app/tests/aurora/composer-drafts-test.mjs app/tests/aurora/outbox-test.mjs` | PASS `composer-unit-031316.log` |
| `python .audit/2026-10-28-grok/syntax.py` | PASS `syntax-031318.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -count=1 ./internal/web` | PASS `web-assets-031321.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe build -o E:\Sach\Sua\neko-aurora\.audit\2026-10-29-vault-editor\neko-aurora.exe ./cmd/neko-aurora` | PASS `build-031325.log` |

Manifest `ac4b6a9ab367b6b5ac04e5a055049235ffa015552f0b11bb405d983321811965`; 1,826 files verified against the code commit with existing Git LF normalization. Local validation.jsonl records exact cwd, UTC times, command and unchanged source before/after each check. 5 candidates and 561 newly reachable history blobs had no recognizable secret matches before the code checkpoint; scanning is not exhaustive.

browser-only-delta.json confirms exactly five changed source files (app.js, i18n.js, style.css, sw.js and the new browser test) from payment code `92d9658a9acd2c5c4ff9765bcb08d23fa8b61cde`. Native Go/Core/Rust/tooling match its source manifest. The prior **2,022 Go PASS / 38 SKIP** result is carried forward from that exact code and command `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -json -p=1 -count=1 -timeout=300s ./...`; the full Go/Core/Rust suites were not rerun for this browser-only change. Go embedded-asset tests and the native build were run for the new assets.

## Limits

This reuses the existing owner-scoped Vault delete endpoint; no new deletion authority or automatic operation is added. Requests already sent may finish after closing; view ownership does not cancel them. Mutual exclusion is local to one mounted form, not a cross-client lock or durable receipt. Existing backend read-only, workspace and persistence rollback tests remain the separate server evidence. No new CAS/idempotency, journal transaction, crash guarantee or JavaScript memory-erasure claim. Some unrelated Vault text remains untranslated. No real credential, provider/account, model, cloud multi-device, Tauri runtime or physical-phone testing.

The isolated composer reload failure above remains a validation limitation despite the diagnostic and original reruns passing. The final PASS totals do not erase that earlier failure.

The source entry receives partial-local-validation only for the editor behavior. Coverage becomes **24 partial, one excluded and 795 without feature-specific evidence** out of 820 source bullets; full Grok Bot parity is not claimed.
