# Passive two-client reconnect acknowledgements — 2026-10-13

The current UI could confirm a saved message through a reconnect snapshot or SSE update, remove its exact outbox entry, and still leave the old live draft, saved draft and pending pointer. The previous reconnect fix covered direct transport ACKs and automatic replay; snapshot/SSE confirmation bypassed that ownership cleanup. This made an already accepted obligation look like a draft still needing a new send.

`acknowledgeSyncedWork` now captures ownership for idle outbox entries, applies the existing complete acknowledgement matcher, and clears only retired, ACK-owned data. The snapshot and SSE paths use it. An in-flight request keeps its earlier ownership snapshot, protecting text entered after that attempt began. If its transport response is lost after an exact passive acknowledgement retired the input, the request's error path reconciles that confirmed result without posting again. A newer pending nonce/body, including identical text, remains intact. Nonce/account/channel/project/profile/file/plugin/continuity pin matching, session/navigation fences and the light layout remain unchanged.

This is a bounded fix for REQ-PB-010 / SCN-PB-014 and SCN-PB-015. The direction was read from the preserved original `E:\Sach\Sua\NekoBot\reference\handoff\neko-development-direction.md`; the source-copy acceptance map and local sync, source/history and file contracts were also read. No cloud/device pairing, new instance rebinding, backend API/schema, transport identity or file grant was added.

## Owned files

- `native/internal/web/assets/app.js`: passive acknowledgement helper, snapshot/SSE integration and confirmed-result handling after a lost transport response; five added/four removed lines against the base checkpoint.
- `scripts/ui_two_client_reconnect_20261013.mjs`: actual extracted sync/SSE/outbox/file-preview functions in a minimal Node VM; optional genuine pinned Go receipt bridge.
- This document.

The worker did not edit Connections, ledger/API, index, stylesheet/branding, Rust/Core, Main, production state or Git. No Core/host configuration, provider call, credential inspection, account/config operation, actual grant, deployment or push occurred. Local owned state contains one synthetic waiting-host obligation per genuine backend run. Earlier receipts remain intact.

## Exact identities

Execution checkout: `C:\Users\Admin\Documents\Codex\2026-10-11\task\checkpoint-verify-20261011`.

Base HEAD: `e2597a99e444f134c3a665f6f6b7cba874259f2c`. The worker has no Git write role. Working-source GREEN binds that HEAD reference to the delivered app.js SHA; Root must checkpoint and rerun against the new immutable source commit.

| Item | Exact identity |
| --- | --- |
| Base immutable app.js (Git LF) | SHA-256 `fac676e1edb78bfc5c4138c767f9a54aa4d7a38c54543d96f2ac834497ae0ad1` |
| Base working app.js (CRLF) | SHA-256 `edf112e37347433dbc13b5e979af0bad27d0f7792ef05730acf178f36dcb863d` |
| Delivered working app.js | SHA-256 `ab0451bf4df5a3d481336651860735c7e3bdfd5722b51fd1c7b0b047bffbf74a` |
| Initial 17-check harness (historical isolation limitation below) | SHA-256 `b52516d5eb1e368c32e2dca2f2f0514daff16ffdb3298cbf5388c04bc4a39bd7` |
| Named-environment corrected 17-check harness | SHA-256 `9ad5f9a8384a03cf469969124c9b989dbe8e6f782a1ccd1f8f8496bb1a535225` |
| Final harness with held-HTTP availability probe | SHA-256 `fbc5ca8c6937bd6ece332d8d988614ff128cff53b75951edf5e9cabff97118db` |
| Pinned Go binary | `E:\CodexScratch\parallel-product-20261013\candidate-5349523\.build\release\neko-control.exe` |
| Go binary SHA-256 | `c2732194124fb989dc84b301fb06e910f64579e94f79e930bf57b1b91ffa337d` |
| Go embedded VCS | `53495231a06aac97c1092029fda35971bdbc08c5`, `vcs.modified=false` |

No binary was built from concurrent working Go source. The harness checks the binary hash and embedded VCS before starting it. Go is started with only `--listen 127.0.0.1:0 --state <new owned scratch state>`, with no host/runtime configuration. Node runs two independent cookie clients representing the same local owner. One client saves a synthetic background obligation; the other reads the genuinely saved snapshot. That receipt feeds the actual current UI source functions in isolated VM consumers. A restart confirms the same request and journal sequence. Initial sequence is 0; accepted and restarted sequence are 1. Both owned processes stop, and stderr is empty.

The backend embedded UI is not the delivered app.js. This bridge intentionally pairs the pinned Go snapshot contract with the newly edited UI source in a VM. It is not an integrated browser or native release test.

## Commands, environment and outcomes

Environment: Windows `win32`, PowerShell, Node `v25.9.0` (`C:\Program Files\nodejs\node.exe`). Binary metadata is read with the explicit local Go tool at `C:\Users\Admin\Documents\Codex\2026-10-11\task\.tooling\go\bin\go.exe`. New evidence/service state/temp files use `E:\CodexScratch\two-client-reconnect-20261013`; `TEMP` and `TMP` point to its `tmp` child. No browser/CUA/CDP workaround was attempted.

```powershell
$env:TEMP = 'E:\CodexScratch\two-client-reconnect-20261013\tmp'
$env:TMP = $env:TEMP
node scripts/ui_two_client_reconnect_20261013.mjs --commit e2597a99e444f134c3a665f6f6b7cba874259f2c --control E:\CodexScratch\parallel-product-20261013\candidate-5349523\.build\release\neko-control.exe --output E:\CodexScratch\two-client-reconnect-20261013\immutable-genuine-red
node scripts/ui_two_client_reconnect_20261013.mjs --control E:\CodexScratch\parallel-product-20261013\candidate-5349523\.build\release\neko-control.exe --output E:\CodexScratch\two-client-reconnect-20261013\final-genuine-green
node --check native/internal/web/assets/app.js
node --check scripts/ui_two_client_reconnect_20261013.mjs
node scripts/ui_reconnect_mobile_20261013.mjs --output E:\CodexScratch\reconnect-mobile-20261013\two-client-regression
node scripts/ui_published_sources_local.mjs
node scripts/file_text_metadata_test.mjs
node scripts/ui_sources_history_local.mjs
```

| Check | Result | Evidence/scope |
| --- | --- | --- |
| Initial authored-source RED before production edit | Expected FAIL: 8 PASS / 4 FAIL | `red/result.json`, base working app hash; actual sync/SSE and attempt functions |
| Final unchanged harness against immutable base and genuine Go receipts | Expected FAIL: 11 PASS / 6 FAIL, exit 1 | `immutable-genuine-red/result.json`; exact baseline Git blob, service-stopped true |
| Delivered source plus genuine Go receipts | PASS: 17/17, exit 0 | `final-genuine-green/result.json`; service-stopped true, no stderr |
| Existing reconnect harness | PASS: 28/28, exit 0 | Preserved separate `E:\CodexScratch\reconnect-mobile-20261013\two-client-regression\result.json` |
| Published-source regression | PASS: 16/16, exit 0 | Actual source-function VM, delivered app hash |
| File metadata regression | PASS: 16/16, exit 0 | Actual DTO functions with authored metadata; no physical file readiness claim |
| Source/history regression | PASS: 19/19, exit 0 | Actual scope/export functions; stale project cannot broaden export |
| Both syntax checks and selective `git diff --check` | PASS: exit 0 | Current owned files |
| Browser/native/phone | BLOCKED; no attempt | Inherited CUA `0x80070003` and browser profile-isolation gates |
| New immutable checkpoint/full integrated suites | Pending Root | Sole integrator/Git writer; not inferred from worker checks |

Initial `genuine-red/result.json` recorded the expected five behavior failures but labelled cleanup false because the first harness checked only `exitCode` after Windows SIGTERM. The stop awaited the exit event. The corrected harness accepts `signalCode` as termination evidence and clears its timeout; both final RED and GREEN confirm owned-process cleanup. That earlier receipt was retained.

The final checks cover passive snapshot ACK, authority-triggered SSE resnapshot, append-only SSE ACK, duplicate/late old-stream/session guards, mismatched receipt, another bot's draft, newer same-text pending, and passive proof during manual/automatic in-flight attempts with a lost response. The file-revoke case uses authored file metadata and the actual snapshot/SSE/preview functions; it does not revoke a real grant. The genuine Go cases prove local saved state and restart with two same-owner HTTP clients. They do not establish two people/devices, cloud transport, remote tombstones, physical files, Core/model quality, painted layout, mobile keyboard, assistive technology, native Tauri or phone acceptance. Root's separate real local-sync v1/v2 cursor probes keep their own identities and scope.

## Root reception corrections and availability

Root found that the first genuine driver enumerated and regex-filtered inherited process environment, retaining the real USERPROFILE. Those receipts remain historical and establish only limited environment isolation; they do not prove absence of secret/home access. The owner corrected the runner to explicit lookups of SYSTEMROOT/WINDIR/PATH/PATHEXT/COMSPEC/PROCESSOR_ARCHITECTURE/NUMBER_OF_PROCESSORS, plus TEMP/TMP/USERPROFILE all directed to its freshly owned E fixture. No environment enumeration remains. Fresh corrected receipts `allowlist-immutable-genuine-red/result.json` (11 PASS/6 expected FAIL) and `allowlist-final-genuine-green/result.json` (17 PASS) record safeEnvKeys/profileHome and stopped owned processes. Production app.js did not change for this correction.

Root also requested a held-HTTP availability check before any extra implementation. `held-http-availability-v2/result.json` records14/14 VM checks using actual renderRouting/renderSessionUI. After exact passive intake confirmation retires the outbox row, a fresh draft enables Send even while the original HTTP response is unresolved (`inFlight=1`, old pending pointer present, one POST only). The send gate derives pending work from retained outbox entries, so no additional availability fix was needed. A fresh pending nonce/draft survives the eventual old-response failure. The initial availability harness lacked botState in its adapter; that setup failure is retained and was corrected before interpreting behavior. Final harness adds this case to the original13 VM checks; a fresh genuine bridge run therefore expects18 checks. No new browser/backend claim follows from the availability-only14-check run.
