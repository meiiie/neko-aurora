# Native finite collaboration acceptance — 2026-10-12

The genuine Windows Go → Rust trusted host → Neko Core path passed **22/22 gates** in `native-group-r3`. One shared-channel goal ran three finite stages with two bot identities: Meiiie proposed, Davin reviewed, and Meiiie returned the final file. This tests the typed collaboration API that the two-leading-alias UI uses; it does not test the UI parser, keyboard or rendering.

## Exact execution identity

| Item | Identity |
| --- | --- |
| Runtime Go source | `4a5383af1184a01d71c432e83ec703deae73d0b4` |
| Go build metadata | Go 1.27.2, Windows amd64, CGO disabled; embedded `vcs.revision` matches and `vcs.modified=false` |
| Go executable SHA256 | `d5e619db82bc3bca3c08dca32de1e5d04ea8f53da1f8b9a50b891b90e5337633` |
| Trusted host SHA256 | `5beef30a02ac26768d26a90cdd9c4c1ed55ec99edd9687358d08a5ae22ae1423` |
| Core SHA256 | `97ca4c8acff2e0bb6dc0b9577d856b3601d4c6c914579a9c9a1fe3cbb67c47fb` |
| Runner SHA256 | `d0b15947222a6d39c3cdee315cf82dd35706554d87ba7c793f83b30b5023eac5` |
| Python / OS | Python 3.13.7; Windows 11 build 26300 |

The source checkout was concurrently at `3ffe66ff4cb16edab4e9d0a161ead0f3d5a65765`, with the new runner untracked. This is not a claim that the whole working tree was clean. The executed Go binary came from the separate clean `4a5383a` build receipt and embedded metadata above. No Go, Rust, Core or UI source was changed by this work. Four fixture/client dependencies were hashed before and after the run, and remained identical.

## Observed results

All 22 named assertions are in the public `results.json` receipt. They cover:

- Three completed stages with distinct stable bot IDs and the first selected bot as final coordinator.
- Exact shared-channel source/reply destination and audience revision, with identical account/project/file pins across stages.
- Genuine Core process bindings and typed context admission; both prior results appear in each subsequent stage's context.
- Unrelated target-private and other-channel canaries excluded from context.
- Deliberate early-stage file-write attempts denied before effects; only the final coordinator creates one effect and one file.
- Exact generated final-file bytes and SHA256, with 404 when requested through another shared channel.
- Shared project history exports one owner goal and the final reply/file, without publishing generated stage instructions as separate owner requests.
- Identical group nonce returns the original terminal map; changed goal with that nonce returns 409.
- A new foreground message completes while the held background group's retained authentic Core handle is still live. The single warm-fixture measurement was **1.981 seconds**; this is not a general latency benchmark.
- Scoped group cancellation waits for that retained handle to signal exit, keeps unrelated completed work, and permits no late reply, artifact or second stage.
- Three service restarts preserve completed/cancelled group maps and the original final artifact.
- Removing a participant changes audience authority, stops the held native Core, preserves the original pin and prevents further stages/effects without rebinding.
- A synthetic account value is excluded from the task journal. The account remains `stored_unverified`; its sealed synthetic value is not provider-authenticated or consumed by Core.

Final artifact SHA256: `b208ac1a6690816b1ee39a172fe068954b0e4c8000ae16ee761fc4b9a20d55e1`. The full run took 19.259 seconds. Both retained handles were signalled; the owned Go service and fixture process exited.

## Reproduction and evidence

Run from the source checkout:

```powershell
python scripts/e2e_tag_collaboration_daily.py `
  --commit 4a5383af1184a01d71c432e83ec703deae73d0b4 `
  --go E:/CodexScratch/local-daily-use-20261012/daily-build-4a5383a-r1/neko-control.exe `
  --host E:/Sach/Sua/NekoBot/.build/neko-host.exe `
  --core E:/Sach/Sua/NekoBot/.build/core-context-admission-1565417/delivery-9f2303f/neko-core-9f2303f-candidate.exe `
  --go-tool C:/Users/Admin/Documents/Codex/2026-10-11/task/.tooling/go/bin/go.exe `
  --build-receipt E:/CodexScratch/local-daily-use-20261012/daily-build-4a5383a-r1/receipt.json `
  --output E:/CodexScratch/local-daily-use-20261012/native-tag-worker/native-group-r3
```

The output directory must be fresh; use a new suffix for a rerun. The canonical public evidence is `E:/CodexScratch/local-daily-use-20261012/native-tag-worker/native-group-r3/results.json` and `build-info.txt`. A whitelist manifest seals those receipts and this runner/report. Raw journals, sealed account files, private fixture config and logs stay under the private fixture directory and are excluded from the delivery whitelist.

Two failed rounds are retained:

| Round | Result | Cause |
| --- | --- | --- |
| r1 | FAIL after 2 passing setup gates | Harness selected an account with no sealed value. Existing `connections.WithSelection` requires `hasSecret` and positive credential generation, so intake correctly returned 409 before a stage. Setup was corrected with a locally sealed synthetic fixture value. |
| r2 | FAIL after 10 passing gates | The wrong-channel download oracle expected 409. `readArtifactLocked` skips mismatched channels and returns `ErrNotFound`, so the correct refusal is 404. Only that expected status changed. |
| r3 | PASS 22/22 | Complete final run, exit 0; owned clients/processes closed. |

Neither failed round established a product defect, and neither was discarded or relabelled PASS.

## Remaining acceptance

This is local process integration with a controlled loopback provider, not model-quality evaluation. The provider deterministically attempts writes and emits stage results. The user-facing leading-alias flow needs its separate UI gate. Cross-channel delivery commands, real account broker/provider authentication, cloud/device sync, native Tauri UI, physical phone/keyboard, accessibility speech and the actual five-minute expiry boundary were not tested here. No deployment, external grant, paid provider, real credential or production-state mutation occurred.
