# Historical local integration checkpoint532, 2026-10-11

Current primary, newer candidates and requirement/scenario gates are indexed in [the current local product status](product-local-current-20261011.md). This page retains the named532 evidence.

Tested code: **532b47fee0424be7ffde2f074678281a43dda0a6**, clean `product-local-84f7db7` in `C:/Users/Admin/Documents/Codex/2026-10-11/task/checkpoint-verify-20261011`. Subsequent documentation checkpoints do not change tested application source. The primary `E:/Sach/Sua/NekoBot` source was restored baseline **84f7db770f44137cbc6942344a5f83fa2549bf0c** at validation time; local promotion is recorded in a separate hash-bound receipt. The candidate is also saved under a separate checkpoint directory. Original ZIPs, credentials and production state are preserved. No cloud deployment, provider call, grant, public push or user-state migration occurred.

## Implemented behavior

The local account/channel frontend selects explicit audience and account revisions for new requests and freezes nonce/scope while retrying. It retains independent channel/project drafts and cannot pull navigation or clear a new draft on a late ACK. Private bot history stays outside shared-channel context.

Project/source intake supports bounded local uploads, text availability versus quarantine, scoped download, project/file permission revisions and frozen excerpts. Runtime chooses related ready-text files within the selected channel/project. The UI shows public source metadata and keeps upload bytes in its in-memory retry queue. It does not persist or render pinned source excerpts. Authority removal fences execution/publication; regrant does not silently rebuild old source pins.

Versioned task controls and finite collaboration are implemented in the API/journal/native runtime. Queued pause/resume preserves identity and context; launched semantic pause returns 501. Supplement creates a linked obligation; replacement fences the old work and waits for exact stop/effect proof. Finite groups run at most three lead/reviewer/lead-final stages within five minutes; only the final stage can write the result file. Dedicated group cancellation owns the group. Their frontend controls remain the next UI slice; these backend capabilities do not imply that current UI exposes every action.

Fresh native tasks use pinned Core **2360aa6b6ee015d3e9262a9f425011dfec8f46e7**. Historical writer-v1 and writer-v2 provenance stays retained; cross-digest admission/replay is denied. The separate explicit Core metadata migration command was tested on owned copies and was never applied to user state.

Wiii remains a separate ADE. Frozen bridge **1682909182cfe5c5d11629dd093cbd704a6f2c37** preserves source-message/workspace/account/channel authority through an owner-injected local transport. Its default hydration does not register or contact the runtime. The native permission/bootstrap boundary is not enabled by the headless transport fixture.

## Exact validation

| Gate | Result and exact scope |
| --- | --- |
| Go532 | **101 top-level / 59 subtests PASS**, vet/build PASS. VCS metadata records commit532 and modified=false. |
| Project/file on Go532 | **32 PASS** on actual compiled Go, journal, DPAPI and Chrome154; upload lost-ACK retry/CAS/source/public metadata gates. |
| Wiii168 → Go532/Rust/Core | **14 PASS**, actual native Core and owned synthetic loopback provider; 9,275-byte primary UTF-8 input, receipt/artifact/checksum, restart and audience fence. |
| Tauri current UI/Core | **45 PASS**, actual owned WebView2/Go532/Rust/native Core; foreground reply while background is running, durable file download/checksum and reload. |
| Project/file on Tauri | **32 PASS**, same assertions as the Chrome suite, actual owned WebView2; no Core/model execution in this suite. Mobile sizes are viewport emulation. |
| Independent provenance fix4b | **7 top-level / 12 subtests PASS**. Legacy journals and proof-free same-runtime continuation survive three restarts; buggy-ceba-produced digest rewrites are rejected. Host/pin files remain identical. |
| Wiii native compile168 | `cargo check --offline --locked` **PASS**, exit0 in214.48s with Cargo/Rust1.97.1 and existing MSVC14.43. Target/temp on E; no dependency download. 918 non-generated inputs and525 frontend-dist files unchanged. |
| Prior exact native integrationceba | Rust11, adoption3, project8, group6, selected-group6, isolated-controls5 and isolated-writer6 PASS. Those receipts remain named ceba607; host/protocol/runner source identity is checked against532. They are not represented as fresh execution of every suite on Go532. |

Tauri harness derivations change only owned renderer/port/path/output plumbing; assertions remain unchanged. Both original and derived hashes are recorded. The current native screenshot pixels were inspected. New-control visual approval, physical phone, AT speech, OS keyboard/lifecycle and nonzero safe-area acceptance remain pending.

Wiii's build regenerated four Tauri schema outputs. Before/after bytes and hashes are retained, including the semantic addition of the already-declared `allow-neko-create-quick-chat-workspace` permission to the generated index. Authoritative Rust/config/capability inputs stayed unchanged. Compilation is not native package, bootstrap, GUI or permission-flow acceptance. The conservative wrapper's raw FAIL for generated-file identity and successful compiler exit0 are both retained and classified separately.

## Preserved evidence and recovery

`E:/Sach/Sua/NekoBot/.checkpoints/neko-product-532b47f/` contains the full-history Git bundle, source manifest, four exact binaries, current and historical validation and independent review. Initial preservation verifies112 artifacts. Bundle SHA256: `314d57b6993510b3819d646e59a1ccb2805b7baa4221a9579d2c2f24589741d9`; initial receipt SHA256: `7648c6382da546fdd188326943524bbfa74a68900e02736f8274ecb191d5a3b6`.

Current Go `.build/neko-control.exe` SHA256: `6f0d7b14041dea8064f01bb7638a79f8f07e92f9bcf2fc9474f93977e0ede0cc`; Rust host `4393c5f40d765e2c4bdd9c1fc93801f24feca356a43e86003d9108f4ed7eb00f`; Core `fa48d43f59257dcd171441f09a8ac53d3702485ddf7f80ac13a0796d53ac6310`. Desktop wrapper `fe55188eea870cede9801b7d3c8b5ced969399f61a013ab63f1671e422b11f89`, with unchanged restored wrapper source.

Local evidence roots are `.build/product-ui-532b47f`, `.build/product-native-ui-532b47f`, `.build/product-native-project-532b47f`, `.build/product-gap-review` and `.build/core-adoption-ceba607`. Wiii native receipt is `E:/Sach/Sua/NekoBot/.build/wiii-native-check-1682909/evidence/native-compile-classified.json`, SHA256 `837705d68bc892da145b690a1329105f9b529ffc5de79652e5fc70641c68e12a`; its15-artifact manifest is `cac6b790a6dd1a2e9a027f4c87a4b046435791aedd75df3c494e303e84058c61`.

Secret screening before the code checkpoint scanned291 text files with zero findings and checked reviewed binary hashes. Original sandbox socket failures, native suite concurrency timeouts and the thin-Wiii execution failure are retained alongside corrected isolated runs. A historical `e2e_core.py` receipt's generic scope sentence calls forced writer-v1 kill blocked even on its writer-v2 passing path; read its explicit checks and pinned digest. This prose discrepancy does not change the successful writer-v2 assertions and is not silently rewritten in frozen evidence.

## Next gate

Consume the UI owner's frozen controls/group patch, rebuild embedded assets, test the combined local workflow and then promote a separately named local candidate while preserving baseline and user changes. The short390×260 transcript currently clips the `Tin mới` affordance when following=false; route/Send44px hit tests pass, and the next UI slice owns its menu alternative. Formal Go race detector remains blocked by missing GCC. Real provider/key consumption, connector grants, multi-host/cloud sync and physical devices are separately deferred acceptance gates.
