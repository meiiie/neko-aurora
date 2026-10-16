# Group availability with a selected Core profile, 2026-10-13

The group menu now disables its existing two-bot action when a Core profile is selected and displays the reason: “Phối hợp · Chưa hỗ trợ profile Core”. Its title and accessible label explain that the selection and current permission remain held. A stale direct invocation also displays that explanation without opening a group dialog. The user can explicitly change selection through the existing profile controls.

## Source and scope

Base: `714c57ddb4f208570dd519bb25466a592a993274`. Owned branch: `candidate-group-availability-20261013`, isolated checkout `E:/CodexScratch/unified-desktop-20261013/group-ui-source`. The production delta changes only `renderCollaborationMenu` and `openCollaboration` in `native/internal/web/assets/app.js` (lines 477 and 479). The other two deliverables are the focused VM runner and this record. Root owns the secret scan, Git checkpoint, Main integration and final candidate packaging.

At the base, the menu action could remain enabled for a chosen profile while `openCollaboration` immediately returned. The existing two-tag path already refused that mode (line 122). This is an availability/explanation correction; it adds no group profile execution. `CollaborationInput` has no profile selection, and `pinCoreProfile` / `coreProfileAllowed` explicitly reject collaboration (`ledger/core_profiles.go:773,825`). The Connections UI currently requests one bot; backend consent validation separately permits one through eight current bot IDs (`:487`). These facts must not be conflated.

The change preserves the chosen object, consent, prompt, pending commands, routing/default/capability/member guards and pending-group limit. Rendering and opening create no POST, nonce or grant. If the newly disabled action held focus in an open composer menu, the menu closes and its opener receives focus; inert-main and other-modal checks prevent focus stealing. Existing light appearance, layout and ordinary group dialog/close behavior stay on their current paths. No CSS, HTML, backend, runtime, Host or Core source changes are included.

## Bounded validation

The runner loads the actual source render/open/tag/close functions and actual group bindings into a minimal Node VM. It checks selection/reason, stale invocation, retained objects/draft/pending command, focus guards, explicit unselection, ordinary open/close, blocked/default/hidden/archive guards, pending limit and the existing two-tag refusal. This is source interaction evidence; it is not integrated browser, accessibility-technology, native desktop, layout, phone, provider or model-quality acceptance.

| Receipt under `E:/CodexScratch/unified-desktop-20261013/` | Source | Result |
| --- | --- | --- |
| `group-availability-red-714c57d/results.json` | Exact immutable base | Expected RED: 11 PASS / 4 FAIL |
| `group-availability-green-working-r1/results.json` | Owned working delta | 14 PASS / 1 FAIL; harness used a case-sensitive predicate against “Chưa” |
| `group-availability-red-normalized-r2/results.json` | Exact immutable base; corrected predicate | Expected RED: 11 PASS / 4 FAIL |
| `group-availability-green-normalized-r2/results.json` | Same runner, owned working delta | GREEN: 15 PASS / 0 FAIL |

Only the test predicate changed from case-sensitive to case-insensitive between rounds. The product delta did not change for that harness failure. All earlier receipts remain preserved.

Exact final commands (absolute launcher prefix `C:/Python313/python.exe -B C:/Users/Admin/Documents/Codex/2026-10-11/task/office-worker-authoring/run_group_vm.py`):

```text
--commit 714c57ddb4f208570dd519bb25466a592a993274 --output E:/CodexScratch/unified-desktop-20261013/group-availability-red-normalized-r2
--working --output E:/CodexScratch/unified-desktop-20261013/group-availability-green-normalized-r2
```

The launcher invokes absolute Node `C:/Program Files/nodejs/node.exe` v25.9.0 on Windows x64, cwd the isolated checkout. Its named environment consists of `SystemRoot`, `WINDIR`, `COMSPEC` plus private E `TEMP`, `TMP`, `USERPROFILE`, `HOME`. The runner's read-only Git subprocess uses named `SystemRoot`, `WINDIR`, `COMSPEC` plus an owned output `USERPROFILE`/`HOME`, with `--no-optional-locks`. No environment enumeration or real profile/credential is an input. The complete argv, source and runner hashes, individual assertions and environment identity are in each receipt; the final raw three-path hashes are in the separate quiescent handoff.

The final immutable checkpoint and five-pin desktop candidate acceptance remain Root gates. The older 714 build is historical; these working-source passes do not certify a newly packaged renderer. Actual asset GET, Tauri verification and any later authorized GUI/model journey must identify their own candidate and environment.
