# Group context saturation fix, 2026-10-13

The existing two leading tags select two members of the open shared channel and create one finite collaboration. Its coordinator proposes, the second bot reviews, and the coordinator concludes. Every stage requires all completed prior stages in its pinned context. Neko Core remains the only agent loop; the Go ledger orchestrates the existing three-stage, five-minute contract.

At source commit `195a0ec990db0478d9f052bc0e79a83b12dc9d78`, publishing the same private completed report eight times and starting a group reproduced a blocker: stage 1 completed, then stage 2 became `needs_owner` with `stage_context_unavailable`. Report pins consumed all eight combined context slots, so the required proposal was omitted. Eight independent short reports have the same slot-budget conflict.

New group creation now freezes `PublishedReportContext.Version=2`. It selects at most six report pins, leaving two of the existing eight slots for prior stages. Exact repeated public results keep their newest delivery pin and increment `omitted`. Identity includes publisher, destination/audience, result, source request/message/conversation, epoch, access/conversation/bot-state revisions, result hash, selected artifact descriptors and admitted file excerpts. Only delivery-specific ID, message ID, publication sequence and the derived pin hash are excluded from comparison. Equal reply bytes from independent sources remain independent, as do different artifact selections from the same source.

Single-bot report intake keeps Version 1. Historical groups with report-context Version 1 reconstruct their original selection, ordering, hashes and omission count; retries retain original pins and the current stage map. Missing historical report context retains its existing path. Unknown policy versions reject during journal replay. ContextPin stays Version 5, with the existing 8,000-byte combined source/report bound, four admitted file identities, launch limits and Core manifest/schema unchanged. New publications after group creation do not enter a continuation.

This introduces a local journal compatibility boundary: the old control-plane implementation expects report-context Version 1 and cannot replay newly created Version 2 group records. Keep the original journal/binary checkpoint for rollback; reopening a journal containing new Version 2 records requires this updated reader. Existing journals are not rewritten or migrated by this change. An old saturated Version 1 group still retains its old frozen selection.

The slot reservation does not guarantee that unusually large report pins, large peer replies, concurrent newer shared history, or a profile excluding history can fit required stage context. The existing byte limit and refusal remain effective; oversized peer output still produces `stage_context_unavailable`. This slice does not invent summaries, truncate evidence, change profile permissions, expand sources, change participant count, or modify model semantics.

## Owned changes

- `native/internal/ledger/group_context.go`: exact repeated-publication identity and new group capture policy entry point.
- `native/internal/ledger/published_reports.go`: shared capture implementation preserves Version 1 and applies Version 2 group limits.
- `native/internal/ledger/collaborations.go`: select Version 2 for new groups and reconstruct the declared supported policy on replay.
- `native/internal/ledger/group_context_test.go`: real local ledger/journal/API-method fixtures.
- `scripts/e2e_group_context_20261013.py`: isolated Go test runner with explicit toolchain, environment whitelist, source hashes, full commands and retained receipts. Despite its `e2e` filename, this runner is a local ledger fixture gate.

Root alone integrates and writes Git checkpoints. This worker does not modify API routes, frontend, runtime/host/Core, Main product state or existing archives.

## Validation evidence

All runs use Windows amd64, Go `go1.27.2 windows/amd64`, Python 3.13. Environment contains only the named Windows process fields plus local Go/cache/temp controls; provider/host configuration and credentials are excluded. Go runs with `GOPROXY=off`, `GOSUMDB=off`, `GOTOOLCHAIN=local`, `GOENV=off`, `CGO_ENABLED=0`; owned temp journals live under each run's `tmp` directory. Shared content cache is `E:/CodexScratch/core-auth-bridge-20261012/root/gocache`. Every receipt identifies the unchanged base commit plus the actual dirty working tree and SHA-256 of the Go source set and runner. Root's later exact combined checkpoint supersedes the working-tree identity for integration acceptance.

Common command prefix:

```powershell
python scripts/e2e_group_context_20261013.py --go C:/Users/Admin/Documents/Codex/2026-10-11/task/.tooling/go/bin/go.exe
```

Run-specific arguments and outcomes:

| Run / owned evidence directory | Arguments after prefix | Result |
| --- | --- | --- |
| `E:/CodexScratch/group-context-20261013/red-r1` | `--output E:/CodexScratch/group-context-20261013/red-r1 --run '^TestGroupContextRepeatedPublicationKeepsPeerResult$'` | Expected RED: stage 2 missing, `needs_owner/stage_context_unavailable`; source set `73b125278a8337286a6e9d558c02ffc7e4b96257d84b3d291a6ae47091c29e23`. |
| `E:/CodexScratch/group-context-20261013/green-r1` | `--output E:/CodexScratch/group-context-20261013/green-r1` | FAIL: independent-report fixture omitted the existing required private-source phrase; three other tests passed. Product code was not changed for this fixture correction. |
| `E:/CodexScratch/group-context-20261013/green-r2` | `--output E:/CodexScratch/group-context-20261013/green-r2` | PASS: four scoped tests; source set `bb44b080d9ec74d02e9301d6d7b10be98f57b8a82072dbe0bfcfe2e6594f2456`. |
| `E:/CodexScratch/group-context-20261013/related-r1` | `--output E:/CodexScratch/group-context-20261013/related-r1 --run '^(TestGroupContext\|TestPublishedReport\|TestCollaboration\|TestGroupServiceRecovery\|TestIndependentGroup)'` | PASS: 60 top-level tests, 149 pass events including subtests, no failures; same source set as `green-r2`. |
| `E:/CodexScratch/group-context-20261013/final-scoped-r1` | `--output E:/CodexScratch/group-context-20261013/final-scoped-r1` | PASS: six scoped tests including divergent file selections and byte-limit refusal; source set `a298287ae0a1f344bdb8e85a58cdb3c688d089199d28c6da080f38a503555793`. |
| `E:/CodexScratch/group-context-20261013/final-scoped-r2` | `--output E:/CodexScratch/group-context-20261013/final-scoped-r2` | PASS: six scoped tests, with historical replay additionally rejecting unsupported policy versions 0 and 3; source set `54629522a9074ab4f00121c76bde6c610a838cdaaee86478bc4aa5dbe7ffc335`. |
| `E:/CodexScratch/group-context-20261013/byte-review-r1` | `--output E:/CodexScratch/group-context-20261013/byte-review-r1 --run '^TestGroupContextNearLimitReportRetainsByteBudgetOwnerGate$'` | PASS: retained byte-budget owner gate; source set `cd2faa8e6839e6b9cb07fe7513aff00235043a2648860fdb6f01037750a052f4`. |
| `E:/CodexScratch/group-context-20261013/final-scoped-r3` | `--output E:/CodexScratch/group-context-20261013/final-scoped-r3` | PASS: all seven final scoped fixtures, no failures; source set `cd2faa8e6839e6b9cb07fe7513aff00235043a2648860fdb6f01037750a052f4`. |

Each directory retains `results.json`, `go-test.jsonl` and `go-test.stderr.txt`. Related tests cover frozen context, exact reader authority, source/destination/plugin/project-file revocation, nonce isolation, owner cancellation, service recovery, stop receipts, legacy groups and stage writer constraints. The source set recorded for the related run predates two additional test cases; production files were unchanged between those runs.

The seven new fixtures cover repeated eight-publication completion, independent equal-byte source authority with newest-six selection and subsequent publication exclusion, prior results at stages 2 and 3, final-only writes, bounded prompt/source bytes, new nonce retry/restart, frozen source/file revocation, exact historical Version 1 replay with unsupported-version rejection, divergent artifact selections and oversized peer refusal. Local format and scoped whitespace checks pass.

Root's requested near-byte-limit composition probe records 7,416 bytes of frozen report JSON and a 570-byte proposal. The required source/report JSON would be 8,437 bytes, so stage 2 still stops at `needs_owner/stage_context_unavailable` with one completed stage. This confirms that the fix addresses report slot exhaustion when enough byte budget remains; it does not reserve byte headroom or promise universal group completion. Root owns any later policy decision around smaller report selection, explicit owner choice or canonical Core compaction.

This is synthetic local ledger/journal acceptance. No real provider/model, Neko Core process, Rust host, Tauri/native window, browser, physical phone, cloud sync, recipient authenticity or multi-owner acceptance is claimed. Formal Go `-race` is unrun because a C compiler is unavailable in this environment. Native CUA and isolated Chrome acceptance remain separately blocked as reported by Root. No deploy, paid provider, new grant, real credential, push or Main state mutation occurred.
