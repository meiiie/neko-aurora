# Memory workspace, revision and conflict recovery

Code commit `a07a27955b65114c8c82c956b5debad580211b11`, based on `4467c731af0167cb9637948f49530c587ea7531c`. Solo pstack Feature / Prove It Works / Show Me Your Work and webapp-testing. Builds on [persistence acknowledgement](grokbot-memory-persistence-2026-10-29.md) and [panel ownership](grokbot-memory-lifecycle-2026-10-29.md).

Previously, deleting an earlier fact from another client shifted positional indexes: an old edit/delete could mutate a different fact. Eight competing edits with the same original view were all accepted. CRUD and summary apply now check the supplied list revision under the memory mutex, including the disk write. Stale requests return 409 without changing facts/history. Private and team revisions are separate; edits to one do not invalidate the other. Failed persistence preserves the original revision. Malformed positional indexes no longer parse a valid leading number.

The GET response returns workspaceId, private/team lists and their revisions; lists and hashes come from one snapshot. All new memory UI requests, including profile counts, import and model tools, carry their originating workspaceId query field. The server rejects mismatches, repeated/empty fields and malformed query encoding. Memory route responses use no-store. The UI rejects a list response with a different workspace or missing revisions.

Summary previews capture their source list/revision before model dispatch. Applying a proposal uses that exact revision even if memory changed during the model call. Conflicts preserve the draft and offer explicit GET-only reload; after reload a separate copyable comparison draft remains. No write, import or model call is automatically replayed. Shared-memory add/edit/delete use the team revision. SW cache v72.

## Validation

Windows amd64; Go 1.27.1, Node 25.9.0, Python 3.13.7/Playwright, Chrome 156.0.8078.49. Temporary stores, local HTTP/browser fixtures and stub model replies only. The 390x844 conflict-recovery screenshot was visually inspected; this is viewport coverage, not phone acceptance.

- RED `memory-revisions-013428.log`: stale indexed edit/delete returned 200 and changed facts; wrong workspace requests succeeded; eight competing edits succeeded; stale model proposal was applied.
- RED `memory-browser-013652.log`: the old profile/memory read omitted workspace. The scoped fixture rejected it before the panel could load.
- Broad-gate `timezone-browser-014520.log` timed out at the initial profile-button visibility check before schedule assertions. A diagnostic run `timezone-diagnostic-014657.log` and normal rerun `timezone-browser-014728.log` both passed on the identical manifest, without source changes. The initial startup flake is retained and its root cause is unresolved; no production fix is claimed for it.
- **27 new Go test/subtest events PASS; 68 memory/utility events PASS**. Coverage includes private/team add/edit/delete/summary, legacy omissions, malformed query/position, eight concurrent revisions with exactly one winner, storage rollback, empty lists, independent scopes and mutation during a stub model call.
- **Full Go: 1,949 PASS, 38 SKIP, zero FAIL across 25 packages**; vet/build PASS. **198 browser scenario groups and 8 Node tests PASS** on one frozen manifest, including 21 memory groups. This adds seven browser groups to the previous checkpoint. Core/Rust are unchanged, so their full gates were not repeated.

| Exact command | Result / log |
|---|---|
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -v -count=1 -timeout=120s -run TestMemoryRevision ./internal/aurora` | PASS `memory-revisions-014009.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -v -count=1 -timeout=180s -run Test(Memory|Mem|TeamMemory|UtilityQuota) ./internal/aurora ./internal/hub` | PASS `memory-all-014015.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -json -p=1 -count=1 -timeout=300s ./...` | PASS `go-serial-014022.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe vet ./...` | PASS `vet-014431.log` |
| `python -u app/tests/aurora/memory-lifecycle-browser.py` | PASS `memory-browser-014434.log` |
| `python -u app/tests/aurora/adoption-browser.py` | PASS `browser-014453.log` |
| `python -u app/tests/aurora/schedule-timezone-browser.py` | PASS `timezone-browser-014728.log` |
| `python -u app/tests/aurora/schedule-loading-browser.py` | PASS `schedule-loading-014737.log` |
| `python -u app/tests/aurora/schedule-browser.py` | PASS `schedule-browser-014744.log` |
| `python -u app/tests/aurora/schedule-test-browser.py` | PASS `schedule-test-browser-014750.log` |
| `python -u app/tests/aurora/vault-cards-browser.py` | PASS `vault-cards-014806.log` |
| `python -u app/tests/aurora/vault-lifecycle-browser.py` | PASS `vault-browser-014817.log` |
| `python -u app/tests/aurora/dictation-browser.py` | PASS `dictation-014823.log` |
| `python -u app/tests/aurora/composer-browser.py` | PASS `composer-014829.log` |
| `node --test app/tests/aurora/composer-drafts-test.mjs app/tests/aurora/outbox-test.mjs` | PASS `composer-unit-014904.log` |
| `python -u app/tests/aurora/chat-loading-browser.py` | PASS `chat-loading-014905.log` |
| `python -u app/tests/aurora/read-state-browser.py` | PASS `read-state-014915.log` |
| `python -u app/tests/aurora/list-focus-browser.py` | PASS `list-focus-014933.log` |
| `python -u app/tests/aurora/selection-browser.py` | PASS `selection-browser-014937.log` |
| `python -u app/tests/aurora/table-viewer-browser.py` | PASS `table-viewer-014949.log` |
| `python -u app/tests/aurora/table-scroll-browser.py` | PASS `table-scroll-014956.log` |
| `python .audit/2026-10-28-grok/syntax.py` | PASS `syntax-015003.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -count=1 ./internal/web` | PASS `web-assets-015005.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe build -o E:\Sach\Sua\neko-aurora\.audit\2026-10-29-memory-revisions\neko-aurora.exe ./cmd/neko-aurora` | PASS `build-015009.log` |

Go commands run in `app/native`; browser/Node/syntax commands run at repository root. Manifest `3f48c487a03bf4701f09d054eb6af46d9efe5035fcac76dff7e93eb9911f77ab`; `code-checkpoint.json` verifies 1,813 source files against the code commit, accounting for existing Git LF normalization. `.audit/2026-10-29-memory-revisions/validation.jsonl` records cwd, UTC timestamps, environment and unchanged before/after manifests. 10 candidate files and 484 newly reachable history blobs had no recognizable secret matches before the code checkpoint; detection is not exhaustive.

## Compatibility and limits

Workspace/revision fields are optional for legacy callers; those callers do not receive the new fence. New UI requires the new server response contract. A revision is a content hash of the raw stored list, retaining import's distinction between nil and empty lists; it is not a monotonic generation, authorization grant or durable mutation receipt. An add that is already a duplicate can remain a no-op. The private/team snapshot does not make the separately read journal an atomic snapshot. Separate-process writers, fsync/power-loss, atomic facts/history, cloud synchronization and general exactly-once remain unproven.

The UI has not canceled already-sent requests. Generic memory POST deadlines and overlapping import/other in-panel operations remain a separate next unit. Comparison drafts are temporary and cleared when leaving the panel. No live provider/model quality, hosted multi-device, Tauri or physical-device acceptance is claimed.

## Changelog mapping

The [official changelog](https://x.ai/changelog/bot) was rechecked on 2026-10-29 local time and still lists 0.77.0, dated October 26, as latest. These persistence/revision tests add partial evidence for `0.59.0-Fixed-2` (source line 806, SHA256 `92f929977e3e3c47478a1ff792c0ea3c69e6005a5bae0a23d479d2d3f78ec150`): reliable save/forget behavior only. Reply latency with large memory/skill sets is not validated. Import's existing partial entry remains partial. Totals become **20 partial, one excluded, 799 without feature-specific evidence**, not 820 independent features or full upstream parity.
