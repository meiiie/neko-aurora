# Independent continuity draft-clear UI acceptance — 2026-10-13

Scoped result: **PASS, 25 checks**, exit 0, on immutable code commit `66aada94f120a4c98f95585aa043ee0f1c2ab520`. The corrected exact-source baseline at `0188e2b2be3d9bcfe1f5637da7885251d738d70b` retains **5 PASS / 2 expected RED** for the missing draft-clear control and handler. No production/index/Main changes, real note mutation, GUI/browser, provider or grant were performed by independent QA.

## Recipe and observed behavior

This independent recipe reads entire `app.js` and `index.html` Git blobs, then evaluates 41 exact named function source lines and four exact continuity event-registration lines in a bounded VM. It executes the actual `requestRaw` session checks and actual policy retain/run/retire functions. Authored DOM, fetch, localStorage and locks supply their external interfaces. Other rendering, reference-card construction and background UI interfaces are authored stubs; full browser startup is not executed. The receipt records hashes of the selected function and event source as well as the full blobs.

| Family | PASS | Observation |
|---|---:|---|
| Explicit control and actual registration | 2 | UTF-8 `Xóa bản nháp`, `type=button`, default disabled, and the actual click handler exist. |
| Existing late-read boundaries | 5 | Input edits, navigation ABA, replacement editor objects, actual request session epoch changes and foreign response scope prevent a late GET from filling a newer draft. |
| Local clear | 1 | Instructions, decision cards and reference cards clear; loaded project/channel/CAS revisions stay intact; editor/read generations advance; focus moves to instructions. No nonce, GET, POST, storage write or lock is created. |
| Concurrent read/clear | 2 | Outstanding GET leaves Clear disabled and direct dispatch is a no-op. A later validated GET can establish editable state; a subsequent Clear remains empty when an older GET completes. No invalid target was forced valid. |
| Clear refusals | 10 | Project/channel scope, project/channel revision, navigation generation, descriptor revision, capability loss, unloaded state, storage error and closed dialog leave the original draft/generations unchanged and create no side effects. |
| Original pending command | 1 | The actual retain function clones the original body and retains its original nonce. Clear cannot change that body, marker, authored cache value or draft while pending. |
| Focus fallback | 2 | When focused Save or Clear becomes disabled, actual render code moves authored DOM focus to the dialog's Close control with `preventScroll`. |
| Explicit empty Save | 2 | Clear produces zero POSTs; the actual submit/retain/run path subsequently creates one new Save nonce and exactly one empty CAS POST. The marker exists before fetch. A matching acknowledgement retires it; a 409 keeps the exact empty command/nonce and blocks a new Clear/Save. |

The observed empty Save body used the original project/channel scope and CAS values: project revision 2, channel revision 3, continuity revision 5, empty instructions/decisions/references, and one fresh explicit-save nonce. A second submit while that request was pending issued no extra nonce or POST. Network and cache remain authored VM interfaces, so this demonstrates source control flow and retained in-memory command identity rather than real backend durability, native storage or a real note's mutation. Root's separate backend empty-note test remains separate evidence.

The existing 2026-10-13 adapter-repair seal was re-read and remained `10f9720b8be8489db6b31da3f4e14b3566039584541c7a83cd38b2d9a103d292`. Its checks were not repeated or broadened.

## Exact commands and environment

Outer command cwd: `C:/Users/Admin/Documents/Codex/2026-10-11/task/checkpoint-verify-20261011`. Runtime: Node `v25.9.0`, Windows amd64. Each child received a named OS-variable allowlist and its own receipt-only `profile`, `cache`, `temp` and `state` directories below `E:/CodexScratch/ux-independent-20261013`. Git system/global configuration was disabled, optional locks were disabled, and no provider/credential environment was inherited. The child metadata uses explicit named lookups.

```powershell
& 'C:\Program Files\nodejs\node.exe' .\scripts\acceptance_continuity_clear_ui_20261013.mjs --commit 0188e2b2be3d9bcfe1f5637da7885251d738d70b --mode baseline --label baseline-0188-v2
& 'C:\Program Files\nodejs\node.exe' .\scripts\acceptance_continuity_clear_ui_20261013.mjs --commit 66aada94f120a4c98f95585aa043ee0f1c2ab520 --mode acceptance --label immutable-66a-v1
```

The corrected baseline and acceptance use the identical final probe SHA256 `0f566c031d869b6f253e0a34f7ae7071b5f3b6a6dfb021a814600ae39266101c`. Baseline exit 1 records its expected RED result; acceptance exit 0 records 25 PASS / 0 RED / 0 FAIL.

## Raw evidence and source quiescence

Receipt base: `E:/CodexScratch/ux-independent-20261013`.

| Evidence | SHA256 |
|---|---|
| `continuity-clear-ui-baseline-0188-v2/results.json` | `c16d3ca5b7b10fd8b8077182b290adf45b610901a32cd97c96e29ba8ee7ac764` |
| `continuity-clear-ui-immutable-66a-v1/results.json` | `179135d62ae9a27f28f0f214746e9c6c20b73a53a6e6d2d58fa11960e23357e5` |
| `continuity-clear-ui-immutable-66a-v1/command.json` | `6dd5062075be3d19b46ec8b90a49ee5da9c6c95cb6ff8095b62c7fba6c121c31` |
| `continuity-clear-ui-immutable-66a-v1/source-quiescence.json` | `8513df9386ff71350b388501c314ae07ca225774cf03e0f2a2b099246f347b6b` |
| `continuity-clear-ui-immutable-66a-v1/seal.json` | `b4afe90f905d79e55bdddba0c5d06f3d854fd0be15346f9c57dc94dcc14ae17a` |

The seal confirmed clean HEAD `66aada94f120a4c98f95585aa043ee0f1c2ab520` in the isolated source and exactly four changed paths from the baseline. It reverified all four Git object hashes and the two executed source snapshots:

| Exact immutable source | SHA256 |
|---|---|
| `native/internal/web/assets/app.js` | `e73ce59442f34e390e0393a0a4600ed6c5b63f85ad8face397219ee9f4a63152` |
| `native/internal/web/assets/index.html` | `1ef4ec5426e86e4363f512880b5403ba80101c895cc477c08dfc56a182778798` |
| `docs/ux-continuity-clear-20261013.md` | `a4e496ca32a09c805f852de24a4836e8b7852c2fe4ddec0a0509ac5bed5b3d4d` |
| Root `scripts/ui_continuity_clear_20261013.mjs` | `e6d2e02fa2820fd9ef07cef4b26557a312b60a05d766d2d6950816551474f5c5` |

All command, stdout, stderr, executed probe and source-snapshot hashes for each round are also retained in `seal.json`. No working-source execution is used for the final GREEN claim.

## Preserved harness failure and remaining limits

The first independent baseline receipt `continuity-clear-ui-baseline-0188-v1/results.json`, SHA256 `99b703c3101480add07153b12b354846b51bec4c6daba8ec709e5c3b0e4cf10d`, remains immutable with its setup failures. Its authored DOM lookup `$` existed in the harness closure but was omitted from the VM context, so the exact source could not resolve it. The sole correction was adding that context property; production source and assertions were unchanged. That failed round is not credited as a valid baseline acceptance result. The subsequent corrected baseline and GREEN round retain separate command/profile/receipt directories.

Remaining blockers: none for this bounded independent source-fixture scope. Native/browser visual review, physical keyboard and assistive-technology behavior, installed-profile behavior, phone acceptance, real storage/backend note mutation, model evaluation and cloud sync remain unassessed here. Root remains the sole Git mutator and owns final checkpoint scanning/integration.
