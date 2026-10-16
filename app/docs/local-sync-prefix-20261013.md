# Bounded local sync prefixes

Base code: `714c57ddb4f208570dd519bb25466a592a993274`, isolated branch `candidate-sync-prefix-20261013` at `E:/CodexScratch/unified-desktop-20261013/sync-prefix-source`. This is writer implementation evidence. Root owns the subsequent immutable checkpoint, integration, full regression/build and candidate packaging.

## Behavior

The previous incremental path collected up to 128 frames and validated the combined envelope once. Encoded-byte or total-update overflow rejected the whole page even when an earlier contiguous prefix fit. The shared historical projection loop also rejected an oversized work/reply/report after a valid prefix, including an event beyond the first 128 frames.

Incremental reads now project only events after the checked cursor and stop before projecting beyond the 128-frame page. Each candidate prefix passes the existing `syncstream.NewIncrement` validation. A limit encountered while constructing the next frame or sealing the next candidate returns the last valid sealed prefix. A first unrepresentable frame still returns `sync_limit`; the next cursor at a later unrepresentable frame returns the same explicit limit. No event or partial frame is skipped. Filtered events retain empty frames and advance the exact global sequence.

The store lock still captures the current authority binding and one high-water mark for the complete request. The existing publication/current-source ACL, principal/session fences, selected scope and cursor checks remain before or within projection. Prefix selection does not change DTO fields, snapshot projection or quotas: 128 frames, 256 KiB encoded envelope, 512 total updates, 16 KiB object, and the existing 12 KiB work/reply/report text guard. Complete snapshots continue to return explicit limits rather than truncated state. No journal write, grant, resync policy, transport or API change is added.

## Focused acceptance

Receipts are immutable directories under `E:/CodexScratch/unified-desktop-20261013/sync-prefix-validation`. Every run uses the Go executable `C:/Users/Admin/Documents/Codex/2026-10-11/task/.tooling/go/bin/go.exe`, cwd `<isolated-source>/native`, and `test -json -count=1 -run <expression> ./internal/ledger`.

| Receipt | Exact run expression | Result |
|---|---|---|
| `baseline-red-v1` | `^TestLocalSyncPrefix` | Untouched production714: 1 top-level PASS / 5 expected RED; nested4 PASS / 8 RED. Encoded/update overflow, later unrepresentable events, consumed history and report prefix failure reproduced `sync_limit`. First-frame quota controls passed. |
| `working-green-v1` | `^TestLocalSyncPrefix` | Same unchanged new test file: 6 top-level PASS / 12 nested PASS, zero failure/skip. |
| `focused-regression-v1` | `^Test(LocalSyncIdentityReplayAudienceAndFileAccessFences\|CorruptOptionalSyncIdentityDoesNotDisableUnrelatedLocalJournal\|LocalSyncFirstRenameCannotCollideWithBuiltinRevision\|FixtureScopedProjectionSyncAndACLRemoval\|ReportDeliveryLocalSyncRevocationRequiresFreshBinding)$` | Four actual matching cases PASS. The originally intended ACL expression did not match a test name; it was not claimed as run and was checked separately below. |
| `focused-authority-v1` | `^Test(FixtureScopedProjectionCursorAndACLInvalidation\|FixtureMemberExportMatchesSourceRevocationAcrossReplay)$` | Two actual cases PASS: actor/session cursor isolation, ACL removal and current-source revocation across replay. |

The new behavioral cases cover escaped-text encoded overflow using real durable Accept and queued pause/resume events; largest-prefix validation, multiple-page convergence, excluded private events as contiguous empty frames, duplicate-page reconciliation, byte-identical journal/nonmutation and restart identity/projection. A synthetic full-Work/artifact projection fixture isolates the update-count boundary: 64 eight-update frames fit exactly 512 updates, and two pages converge to the complete projection. Synthetic projection events also isolate oversized input/reply, JSON-escaped object and update-count frames at sequences2 and130, already-consumed history, and the published-report text guard. These synthetic descriptors do not prove physical artifact creation, runtime execution or model behavior. First oversized frames and full snapshots still fail explicitly.

All Go child environments are constructed from named OS variables only (`SystemRoot`, `WINDIR`, `COMSPEC`, `PATHEXT`, `PATH`, `PROCESSOR_ARCHITECTURE`, `NUMBER_OF_PROCESSORS`) plus owned home/app-data/temp/module-cache paths and fixed offline settings. `GOENV=off`, `GOWORK=off`, `GOPROXY=off`, `GOSUMDB=off`, `GOTOOLCHAIN=local`, `CGO_ENABLED=0`; Git optional locks and system/global config are disabled. Home/state/temp directories are per receipt. The explicitly authorized reusable offline Go cache is `E:/CodexScratch/core-auth-bridge-20261012/root/gocache`. Environment values were not enumerated or printed; receipts list named keys and owned scratch paths. No provider/proxy/credential variables are passed.

Baseline production SHA-256: `9bf7a2f7d6b164f1729592e3fbd611b50cdea09e29b3718752e37b503f512b39`.

Validated production SHA-256: `1fa99985ea144e4fc76d987b81e5a18789f02fed298e7ea9920fe721edd96990`.

New test SHA-256 (identical RED/GREEN): `24bc244be0d74c2d1a618d3ffb585299669034058fd63a95a10f2180e9bb79d8`.

Each receipt records exact args, source hashes, raw output hashes, pass/fail cases, exit status, named environment and source-quiescence checks. The final separate seal records this document hash, candidate-only secret scan and read-only Git scope. No Git/index/Main mutation was performed by the writer. This slice does not claim independent QA, native GUI, physical phone, production cloud sync, paid provider, credentials, grants, deployment or push acceptance. Prior repair/UI/document-worker seals remain unchanged.
