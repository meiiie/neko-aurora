# Local bot/file/replica checkpoint — 2026-10-12

Application **8ceb7598ee2c6bc966bf02f1ad84439f251948a7** adds per-bot instructions/history selection, bounded DOCX extraction and scoped text preview, plus a GET-only two-client local replica. Tauri/Rust/Go/Core architecture and accepted light layout remain. Baseline is delivery `fef7758f963e54bdb4f3356b9053564523ed6e3f`; this delivery changes documentation only after the tested application. Exact delivery/source/binary hashes are in [the packet manifest](../.checkpoints/local-bot-file-sync-release/manifest.json).

## Run and use

```powershell
.\scripts\run-prebuilt-local.ps1 -Manifest .checkpoints\local-bot-file-sync-release\launch.json -CheckOnly
.\scripts\run-prebuilt-local.ps1 -Manifest .checkpoints\local-bot-file-sync-release\launch.json
```

Open http://127.0.0.1:8765. Default starts a durable journal/UI, saves work as waiting for runtime and calls no provider. Add `-StateDirectory` for an isolated review state. `-Desktop` uses the unchanged hash-pinned Tauri shell; these new controls have not received native-window acceptance. The old default launcher receipt and all previous binaries/media remain available. Never open a newer journal with an older binary that lacks its event schema; use a compatible state backup for rollback.

Composer menu → “Hướng dẫn & lịch sử của bot” configures the current bot for new intake. Existing work keeps its original profile/context pin. History-off skips completed conversation replies in the new context; it does not erase history, disable all memory or expand file authority. Upload a DOCX into its permitted project/channel and read its text from the Files panel. Full original bytes remain separately downloadable. Preview is textContent only and clears/fences on scope/revoke/close.

## Fresh evidence

| Gate | Result at exact application commit | Scope |
| --- | --- | --- |
| Go `test ./... -count=1 -json`, vet, build | PASS292 top-level +325 subtests, vet/build exit0 | Current native source, including two-cookie local-sync API and restart/fence tests |
| Actual Go/browser | PASS29;7 PNGs inspected | Authenticated HTTP profile/CAS/lost+late ACK, DOCX metadata/text/revoke, long filename and390×260/320×320; response faults via CDP, no API stub |
| Go/Rust/Core | PASS7 | Real pinned processes/controlled loopback model: old live profile pin/binding/lease survives edits, new context v4, text hash vs ZIP hash, deep broker read beyond context excerpt, foreground completes while background lives, authentic stop HANDLE |
| Node metadata source | PASS16 | Actual DTO function in isolated VM; separate from browser/backend acceptance |
| Existing browser regression | PASS42 | Current compiled Go with labelled CDP context/focus fixtures; assertions unchanged, only output/profile paths adapted to E |
| Existing light/pixel suite | PASS23 | Static fixture/geometry; does not substitute for real native/phone |
| Independent review | Detached copy12 RED→GREEN; bounded Go/protocol + metadata consumer RED→GREEN closed | Review on654/0600/47/177; latest preview CSS uses Root's actual screenshot/geometry closure, no borrowed full-suite/native review |

Evidence, exact commands, environment, drivers, hashes and labelled historical failures live in [verification.json](../.checkpoints/local-bot-file-sync-release/verification.json). All test binaries, caches, temporary state and headless profiles were owned E scratch; no production state, provider credentials, grants or public deployment. Native host SHA256 `5beef30a02ac26768d26a90cdd9c4c1ed55ec99edd9687358d08a5ae22ae1423`; pinned Core SHA256 `97ca4c8acff2e0bb6dc0b9577d856b3601d4c6c914579a9c9a1fe3cbb67c47fb`. Go SHA256 `d173b17cb690eb739c0705c1e2e64ff2cc1a7664402832b4e78b8bd4eb3ba1e9`. Rust/Core/Tauri full historical suites were not rerun this slice.

## Historical failures retained

Initial654 returned profile/CAS pointers that could alias stored Work; independent12-case probe failed and0600 detached copies closed it. Actual47 preview failed because browser DTO omitted DOCX extracted-text identity;177 preserved validated metadata and consumer/HTTP checks passed.61 horizontal overflow was fixed, but a stronger short-screen check then failed because long title displaced text;8ceb clamps visual title and the unchanged stronger gate passes.654's immediate close assertion was a harness timing issue: the queued dialog close event was not awaited; the original driver and classification remain separate. No failed receipt was overwritten.

## Remaining gates and next useful work

Local replica is protocol/reconciliation simulation, not cloud transport/auth/device pairing. Real OAuth/key broker/provider adapter/model evaluation, cloud/deploy and external connectors remain separate deferred work. Physical phone/OS keyboard/safe-area/AT speech, new-control visual review and native Tauri UI are unaccepted. Wiii native owner consent remains blocked by the supported Computer Use runtime; no consent was simulated. Formal Go race is blocked by missing C compiler. Prior native timeout attribution and broader simultaneous-crash/power-loss gates remain open.

Useful local follow-up is bot archive/new-conversation boundaries and file retention/export/backup before remote pairing and secure provider onboarding. No final ZIP or release acceptance is claimed.

Current Library preview remains the preserved5fa recording (does not show these new controls): `current-desktop-5fa4c43.png` / libfile_52894225b1b88191bc6f014d53b58e74 / version1; `current-mobile-5fa4c43.png` / libfile_93377cda7f648191b893dd36ab065b28 / version1; `current-local-demo-5fa4c43.mp4` / libfile_cb7e4ad025b88191aa1eae2c8cd65586 / version2,12.32s. Recorded app `5fa4c4337ff390767dd844f24d764ce9d901cea9`; actual Core controlled fixture/headless renderer, not physical phone/model quality. No new upload or media replacement is claimed. New preview PNGs are local packet evidence.
