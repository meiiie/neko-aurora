# Form receipt replay after display-cache eviction

Code commit: `9c49a07883773ff26fe212822e82ba82b6052ad5`, based on `b3421631e68c607c3e97ee5e2993e5331fac4cbb`. Local pstack Feature / Prove It Works checkpoint, one implementer, no subagents.

The side store retains only 300 task display entries, including forms and thinking text. Previously, a form answer that had already been journaled could return HTTP 400 on retry after its form definition was evicted. The new path recognizes an actual owner-message receipt for the original thread and exact form nonce. It then passes the retry through the existing ledger payload comparison, preserving the original response and recipient set. A changed answer remains a conflict. Workspace, thread, reference and reply checks remain in force.

The form completion endpoint also recognizes that receipt before consulting the display cache. An unacknowledged evicted form remains unavailable. A legacy work whose nonce merely resembles a form answer cannot bypass the missing-definition check. Existing cached-form/legacy completion behavior is retained; this patch does not invent an evicted form schema or restore its rendered fields.

## Validation

Windows amd64, PowerShell 7.6, Go 1.27.1; allowlisted environment with real provider/account variables removed. The HTTP tests use loopback and real temporary ledger/side-store files. They evict through the actual bounded-cache routine, persist it, close/reopen the ledger and construct a new service. Both a direct chat and a group recover the original response without adding work. Negative cases cover changed payload, wrong workspace/thread, unanswered forms and legacy nonce-only work.

| Command in `app/native` | Result / local log |
|---|---|
| `go test -v -count=1 -timeout=120s -run 'Test(FormReceipt\|FormReplay\|ProviderModelThinkingAndForm)' ./internal/aurora` with new regression before the fix | FAIL: accepted form retry returned 400 after eviction/restart; `form-replay-155330.log` |
| `go test -json -count=1 -timeout=180s ./internal/aurora` | PASS: 186 test/subtest events, 24 existing skips, 0 failures; `aurora-155631.log` |
| Focused command above on final source | PASS: 4 events, 1 existing computers-off skip; `form-replay-155824.log` |
| `go vet ./...` | PASS; `vet-155830.log` |
| `go build -o <audit>/neko-aurora.exe ./cmd/neko-aurora` | PASS; `build-155743.log` |

Full package/build source manifest: `22eb529af97e2b6d582a012a0ab6e8d3f1ea24ea159a2c58e05ff05a8ef4b4da`. Final focused/vet manifest: **`bdd6d179f629b8140577ed3f1debcbe96efa6ce8ed0ddcf20a58a5ad26c7af2d`**. The only difference is explicit cancellation cleanup in the new test fixture: vet initially reported that the reassigned cancel function was not used on all paths (`vet-155740.log`). Runtime code did not change after the full package/build checks. No source changed within a validation run. Exact commands, timestamps and manifests are in `.audit/2026-10-28-form-replay/validation.jsonl` and `code-checkpoint.json`.

The candidate secret scan covered four files and 180 newly reachable history blobs, with zero recognizable matches and no printed values. The complete monorepo, browser and model suites were not rerun for this server-only fix. No cloud sync, physical-phone or live-provider acceptance is claimed. This extends partial evidence for `0.77.0-Fixed-6`; adoption totals are unchanged.
