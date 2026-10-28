# Computer outcome recovery

Code commit: `9db4ee4f997c26b0b94ddf0ea8d51a094768468f`, based on `573a94975301fc4762252853c652282836d47463`. Local pstack Feature / Prove It Works / Show Me Your Work, one implementer. No Railway account or paid provider was contacted.

## Behavior

A dropped response, missing/negative exit code, timeout or failed download used to leave no durable barrier against a later command overwriting the VM home. Successful commands also depended on a stdout change marker to trigger downloads. The host now writes a bounded pending record beside the home before dispatch, containing only version, VM ID, phase, local-home hash, command hash and time. New commands, warm-up and LRU eviction refuse pending homes, including after process-memory state is discarded. Every confirmed command retrieves the home; stdout content no longer decides whether file synchronization is necessary.

For a confirmed command with an unconfirmed download, the owner can explicitly retry **file retrieval only**. It never replays the command and refuses to overwrite changed local files. Unknown execution remains fenced. Reset now waits for confirmed remote destruction before deleting local files. Provisioning is serialized; per-home locks protect active or recovering VMs from eviction. An unconfirmed eviction retains the VM identity and prevents creating a replacement.

The scoped recovery/reset API requires `workspaceId`; stale scopes and unknown fields are rejected. Existing reset clients must include that field. The UI labels pending recovery, distinguishes unavailable confirmation from success, warns before deleting unretrieved remote files, and offers a read-only status reload after lost responses. Late responses cannot mutate a different account's profile. PWA cache is v55.

The [Railway sandbox documentation](https://docs.railway.com/sandboxes), checked on 2026-10-28, distinguishes command exit from HTTP success and documents that HTTP execution has no durable session handle or client cancellation. This patch fences uncertainty in the existing HTTP integration. A WebSocket session transport remains separate.

## Validation

Windows amd64, PowerShell 7.6, Go 1.27.1, Node 25.9.0, Python 3.13.7, Chrome 156.0.8078.48. Provider/account variables were excluded by the audit runner. The existing shell fixture uses Git Bash in temporary local directories; new faults use loopback HTTP. Exact commands, base, source hashes and results are in `.audit/2026-10-28-computer-recovery/validation.jsonl`.

| Command (Go commands in `app/native`) | Result |
|---|---|
| `go test -v -count=1 -timeout=120s -run 'Test(RailwayRecovery\|ComputerRecovery\|RailwayReset)' ./internal/computer` on new tests before implementation | Expected FAIL; `recovery-164854.log` |
| Full Computer suite during implementation | FAIL: cached-home shortcut missed an expired VM; `computer-165159.log`. Fixed with a pre-dispatch VM probe; `computer-165326.log` PASS |
| `go test -v -count=1 -timeout=120s -run TestComputerRecoveryAPI ./internal/aurora` | RED 404 before route; GREEN after implementation; `recovery-api-165637.log`, `recovery-api-165805.log` |
| `go test -json -p=1 -count=1 -timeout=300s ./...` | PASS: 1,650 test/subtest events, 37 existing skips, 0 failures, 25 packages; 241.46 s; `go-serial-170620.log` |
| Focused recovery command after final negative-exit guard | PASS; `recovery-171115.log` |
| `go vet ./...`; `go build -o <audit>/neko-aurora.exe ./cmd/neko-aurora`; `go test -count=1 ./internal/web` | PASS; `vet-171128.log`, `build-171133.log`, `web-assets-171136.log` |
| `python -u app/tests/aurora/computer-recovery-browser.py` | PASS: 7 scenarios; `recovery-browser-171139.log`; mobile screenshot inspected at 390 × 844 |
| `python -u app/tests/aurora/{adoption,composer,upload,selection,schedule}-browser.py` (run separately) | PASS: 74 scenarios combined. Selection initially failed during language-switch reload; fixed the test to wait for navigation, then PASS in `selection-browser-171349.log` |
| `node --test app/tests/aurora/composer-drafts-test.mjs app/tests/aurora/outbox-test.mjs` | PASS: 8 tests; `composer-unit-171147.log` |

Full-Go source manifest: `26db6e85bd111dbcc67305e00b723035beda71f68c0b23d2a0614a5885e75949`. Final runtime source and focused/build/browser gates: `6ac77af9e22e74ecc7dabd67ccd176e276545ff4ccf1ff82f3a7cd770bb64175`. Final selection-test-only adjustment: `685423f6d635b83c40d0f108e60af251357f849dbc420b8fd61081a18ea8f694`. No source changed during any reported check. No second full-Go run is implied after the negative-exit guard; that branch was exercised by the focused regression. Candidate scan: 12 files and 213 newly reachable history blobs, zero recognizable secret matches, no matched values printed.

## Limits

These are local fixtures, not production Railway acceptance, model evaluation, physical-device or Tauri acceptance. No Go race run was possible with the installed C toolchain. The home swap and pending file are not an OS-power-loss transaction. A failed swap cleanup may require manual reconciliation, and unknown remote execution cannot be resumed automatically. Process-memory registry loss is tested; cloud account deletion, VM-create response loss and all provider lifecycle states are not covered. Provisioning serialization can increase latency. Local file lists and editable verification files are not remote completion receipts. No additional changelog bullet is declared complete from this unit alone.
