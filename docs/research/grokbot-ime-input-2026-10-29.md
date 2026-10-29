# IME submission and own-answer validation — 2026-10-29

Code commit: `2107c7e513a71cc4e7a14fcb8d45f7baea672d1e` on `codex/grokbot-adoption`. This covers local behavior corresponding to [Grok Bot 0.78.1](https://x.ai/changelog/bot#v0.78.1), Fixed-7 and Fixed-8. Implementation and self-review follow pstack Feature / Prove It Works / Show Me Your Work and webapp-testing, with no subagents as directly requested. No model, provider, real account, grant, deployment or push was used.

## Change

Enter while an input method is composing no longer enqueues a legacy custom-choice answer, submits a form or sends the chat composer. The same guard protects Vault contact inputs and their enclosing form. It observes compositionstart/compositionend, isComposing and keyCode229, retaining the composition-key marker through implicit submit. It does not cancel the IME key's native text-commit action. Key release, focus change, pointer interaction and a subsequent ordinary key release temporary suppression. State lives only on the form DOM; reload does not restore a composition lock. Drafts and receipt/outbox semantics remain unchanged. Service-worker cache: 85.

The existing single-choice question card already offered its own-answer textarea, review and navigation. It was validated rather than replaced: Unicode text and newlines survive review, Back and reload, and an explicit final send supplies the structured answer once. Normal Enter, Shift+Enter and keyboard activation of a submit button remain supported.

The event-boundary handling follows the documented [keydown IME behavior](https://developer.mozilla.org/en-US/docs/Web/API/Element/keydown_event#keydown_events_with_ime): composition lifecycle order can make isComposing false for an IME key, so keyCode229 is also checked. No source code or assets from Grok Bot were copied.

## Evidence and limits

`ime-browser-134054.log` is the RED baseline: a trusted Enter while the legacy custom-answer input had active composition state generated an unexpected POST. The initial fixture also used trusted Enter in a textarea after a synthetic compositionstart; Chrome inserted a newline because that event does not start an OS input method. `ime-browser-134134.log` retains this fixture failure. The textarea case now dispatches its composition key, verifies no submission/default cancellation, and separately exercises trusted ordinary Enter. Production code was not changed to cancel native text commitment for that test.

The new fixture passes ten scenario groups covering three submission surfaces, both key markers, composition state and programmatic/implicit form submit, regular Enter, Shift+Enter, focus exit, reload, Unicode own answers and keyboard submit controls. The existing Vault contact fixture additionally checks empty composing inputs cannot implicitly submit the enclosing general form. The mobile own-answer review screenshot was inspected. Browser events and loopback APIs are synthetic; no acceptance claim is made for Windows Japanese/Chinese/Korean IMEs, Unikey/Telex, mobile keyboards, WebKit, Firefox or Tauri WebView. Real input-method testing remains open.

## Validation

Final source manifest `0ab90824bfa65926f2d04eb5de83eab755d298318e5888ac8f0d2afd105ae9c2`, 1839 files verified against the code commit with existing LF normalization. Windows amd64; PowerShell 7.6; Python 3.13.7; Chrome 156.0.8078.49; Node 25.9.0; Go 1.27.1. Test subprocesses use an allowlisted environment without live credentials.

168 browser scenario groups, 19 UX assertions and eight composer/outbox assertions pass. Syntax, embedded assets and native build pass. All gates below share the final source manifest. Full Go evidence remains **2048 PASS / 38 SKIP across 25 packages**, recorded with [Vault contact implementation](grokbot-vault-contacts-2026-10-29.md). Source comparison confirms only browser assets and fixtures changed since that Go run. No Go business logic, Core or Rust changes were made, so their full suites were not repeated for this frontend unit.

| Gate | Result | Seconds | Log under .audit/2026-10-29-ime-input |
|---|---|---:|---|
| ime-browser | PASS | 21.05 | ime-browser-134324.log |
| contacts-browser | PASS | 12.17 | contacts-browser-134345.log |
| composer | PASS | 34.74 | composer-134357.log |
| form-browser | PASS | 27.52 | form-browser-134432.log |
| browser | PASS | 24.92 | browser-134500.log |
| startup-browser | PASS | 16.17 | startup-browser-134525.log |
| editor-browser | PASS | 15.36 | editor-browser-134541.log |
| hint-browser | PASS | 8.93 | hint-browser-134557.log |
| payment-browser | PASS | 10.8 | payment-browser-134606.log |
| secret-browser | PASS | 21.26 | secret-browser-134617.log |
| vault-browser | PASS | 7.54 | vault-browser-134638.log |
| vault-cards | PASS | 12.53 | vault-cards-134646.log |
| content-browser | PASS | 7.45 | content-browser-134659.log |
| composer-unit | PASS | 1.11 | composer-unit-134706.log |
| ux-unit | PASS | 0.82 | ux-unit-134740.log |
| syntax | PASS | 3.39 | syntax-134708.log |
| web-assets | PASS | 3.86 | web-assets-134711.log |
| build | PASS | 4.3 | build-134715.log |

Exact commands and snapshots are in validation.jsonl. The dedicated fixture runs with `python -u app/tests/aurora/ime-browser.py`; the remaining browser fixtures use the same invocation pattern. Node checks: `node app/tests/aurora/ux-test.mjs` and `node --test app/tests/aurora/composer-drafts-test.mjs app/tests/aurora/outbox-test.mjs`. Embedded assets use `go test -count=1 ./internal/web` from app/native; build uses `go build -o <audit>/neko-aurora.exe ./cmd/neko-aurora`. Go resolved to E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe. Documentation uses `python tools/check_docs.py`, with local report links checked separately.

Before the code checkpoint, recognizable secret-pattern scans covered 7 candidate files and 630 newly reachable history blobs with zero matches; this is not exhaustive detection. The pre-existing unrelated adoption-document edit is preserved outside the checkpoint. Decisions and the self-review trail remain in decisions.tsv.

## Remaining scope

The [incremental index](grokbot-adoption-index-2026-10-29.json) now records local evidence for both bullets. Across both stored indexes: 30 partial, one excluded and 889 unvalidated source bullets (920 total). These counts are not independent feature counts and do not claim Grok Bot parity. Native IME acceptance and the broader backlog remain open.
