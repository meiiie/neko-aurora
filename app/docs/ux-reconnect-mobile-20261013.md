# Reconnect pending-message journey — 2026-10-13

An uncertain message retry now keeps its original row visible while waiting for the transport. The row exposes `aria-busy`, retains the message preview, and disables retry/edit/discard until the attempt finishes. An explicit retry moves focus to that row with `preventScroll`; a failed attempt restores its retry control only if the same session, navigation and row still own focus, with no open dialog or inert main pane. Removing an acknowledged row also respects an open dialog before returning focus to the composer.

The independent UX reviewer also reproduced an automatic-reconnect gap: a matching ACK removed the outbox entry while leaving the original live/saved draft and `state.pending`. Automatic and manual retries now share an ownership snapshot and exact-ACK cleanup. Cleanup requires an input retained before the attempt and retired by the existing full acknowledgement matcher. Only a pending input with the same nonce and serialized request body is cleared. A different current pending nonce or body stops live/saved draft cleanup, including when the newer pending message has identical text. The pending ownership decision is made before clearing an ACK-owned pointer. Draft cleanup additionally requires the captured/current session, view, bot, account/default/Core choice, navigation and original text to match. A draft edited during reconnect, a new pending input, or a different view stays intact. Accepted work keeps running; cleanup neither navigates nor focuses the composer.

The nonce, channel/account/project/profile revisions, requested files, plugin choices and continuity pins still pass through `messageInput` unchanged. Core-profile entries remain excluded from automatic replay. Read-only restores and fixture readers cannot submit pending messages. The automatic queue stops if its captured session changes.

## Owned delivery

- `native/internal/web/assets/app.js`: outbox row lifecycle and guarded acknowledgement cleanup.
- `scripts/ui_reconnect_mobile_20261013.mjs`: actual extracted UI/transport functions in Node VM, with a minimal DOM adapter and synthetic transport. An optional exact `--source-commit` reads immutable Git objects without resetting the shared checkout.
- This document.

No index, stylesheet, branding, Go API, Rust host, Core runtime, real grant, secret restore, service state, provider call, cloud operation, deployment, push or Git mutation was performed by this worker. Other workers' concurrent diffs are outside this delivery.

## Exact validation identity

Worker execution checkout: `C:\Users\Admin\Documents\Codex\2026-10-11\task\checkpoint-verify-20261011`.

Recorded HEAD: `195a0ec990db0478d9f052bc0e79a83b12dc9d78` (`docs: record published-source inspection evidence and renderer blockers`). The worker has no Git write role; GREEN evaluates the working source at that HEAD plus the source SHA below. The integrator must issue the new checkpoint and rerun against its immutable commit.

| Item | SHA-256 |
| --- | --- |
| Immutable baseline app.js Git blob (LF) | `ca4dc7f7c88995c8d0a5d2e940f5bf5850d86fba651bf5e20d113c7ef25fb1a9` |
| Original baseline working app.js (CRLF) | `cd1f26fe9a10a8bfb48ac101d4d54744c806bbe446e0236f827f747b71964159` |
| Initial 26-check handoff working app.js | `79ee96db25e365f9a5684f56c74569bbe5bdbef1f400db818a898366038cee59` |
| Initial 26-check reconnect harness | `613f888dc20c274342f89b7a9c5039e09c864c98fa188d16395c80ee0945ecfa` |
| Delivered working app.js with pending ownership guard | `edf112e37347433dbc13b5e979af0bad27d0f7792ef05730acf178f36dcb863d` |
| Delivered 28-check reconnect harness | `c3977b67ac33b1412f0bb902d567a7fc1e50176514f538a545c5cc2b5d5efc66` |

Environment: Windows `win32`, PowerShell, Node `v25.9.0` at `C:\Program Files\nodejs\node.exe`. `TEMP` and `TMP` were set to `E:\CodexScratch\reconnect-mobile-20261013\tmp` for evidence-producing runs. All worker evidence is under `E:\CodexScratch\reconnect-mobile-20261013`.

## Commands and outcomes

Commands executed from the worker checkout at the source/harness versions recorded above. RED runs without `--source-commit` used the pre-edit working source; their receipts retain that exact SHA. Rerunning those commands after the fix should pass.

```powershell
$env:TEMP = 'E:\CodexScratch\reconnect-mobile-20261013\tmp'
$env:TMP = $env:TEMP
node scripts/ui_reconnect_mobile_20261013.mjs --source-commit 195a0ec990db0478d9f052bc0e79a83b12dc9d78 --output E:\CodexScratch\reconnect-mobile-20261013\final-baseline-red
node scripts/ui_reconnect_mobile_20261013.mjs --output E:\CodexScratch\reconnect-mobile-20261013\final-green
node scripts/ui_reconnect_mobile_20261013.mjs --output E:\CodexScratch\reconnect-mobile-20261013\same-text-red
node scripts/ui_reconnect_mobile_20261013.mjs --output E:\CodexScratch\reconnect-mobile-20261013\same-text-green
node --check native/internal/web/assets/app.js
node --check scripts/ui_reconnect_mobile_20261013.mjs
node scripts/ui_published_sources_local.mjs
node scripts/file_text_metadata_test.mjs
node scripts/ui_metadata_intents_local.mjs
node scripts/readonly_restore_ui_test.mjs
```

| Command | Outcome | Supporting evidence |
| --- | --- | --- |
| Reconnect immutable baseline | Expected RED: exit 1, 19 PASS / 7 FAIL | `final-baseline-red/result.json` |
| Initial reconnect handoff source | GREEN: exit 0, 26/26 PASS | Preserved `final-green/result.json` at initial source/harness hashes |
| Same-text/new-pending ownership before guard | Expected RED: exit 1, 26 PASS / 2 FAIL | `same-text-red/result.json` at initial app.js hash and delivered 28-check harness |
| Delivered source with pending ownership guard | GREEN: exit 0, 28/28 PASS | `same-text-green/result.json` |
| Both `node --check` commands | PASS: each exit 0 | Worker tool results, same source/harness hashes |
| Published-source VM regression | PASS: exit 0, 16/16 | Actual source-function VM output at delivered app hash |
| File metadata VM regression | PASS: exit 0, 16/16 | Actual source-function VM output at delivered app hash |
| Metadata intent VM regression | PASS: exit 0, 17/17 | Actual source-function VM output at delivered app hash |
| Legacy read-only extraction harness | FAIL: exit 1 before UI assertions | `readonly_restore_ui_test.mjs:8`: fixture state omits `capabilities`, while extracted `request()` reads `state.capabilities.includes`; `request()` was not changed in this slice |
| Native/browser/phone acceptance | BLOCKED; no attempt in this slice | Inherited CUA `0x80070003` and Chrome profile-isolation gates; no CDP/native workaround |
| Full Go/Rust suites and immutable new-checkpoint verification | Pending integrator | Worker scope excludes Go/host/runtime work and Git mutation |

The initial row-lifecycle RED was observed before the production edit. The separately added replay-cleanup RED then observed two expected failures before the ACK cleanup edit. The 26-check harness was rerun unchanged against both the immutable baseline and the initial handoff source: the seven baseline failures cover row visibility/focus, original draft/pending cleanup, and rejection of mismatched manual ACK draft cleanup.

An independent supplemental probe then found that a newer pending nonce with the same text survived while its live/saved draft was cleared by the old ACK. Two additional cases were added without removing the original 26. Both failed before the current pending ownership guard and pass afterwards, whether the newer pending input is installed before replay or while the old response is held. Earlier 26-check receipts remain untouched. No generic edit-generation semantics were added: editing away and returning to the original text without a new pending nonce retains the existing text-based ownership behavior.

These results establish behavior of actual extracted functions under authored VM data. They do not establish rendered geometry, touch targets, browser accessibility, mobile keyboard behavior, native Tauri operation, physical-phone acceptance, backend persistence, cloud sync or model quality. The light layout is preserved in source; it was not visually accepted here.
