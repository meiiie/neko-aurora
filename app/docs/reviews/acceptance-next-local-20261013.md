# Independent next-local acceptance - 2026-10-13

Current independent result: **GREEN for bounded ledger paths on immutable `e127a4189ffa39cf055930ed05a30d7360aba928` and bounded UI transport paths on immutable `b2b7bf83d296b29ad6f51c306e644ac3914b279c`**. The original six ledger assertions pass unchanged. Four focused ancestry families, including ten subcases with reviewer history disabled and owned E `USERPROFILE`, pass. Passive transport passes 5/5 and frozen UX/ownership passes 25/25 + 4/4 on b2b. Historical e259 ledger **4 PASS / 2 FAIL** and transport **2 PASS / 3 FAIL** remain recorded below. GREEN is limited to the exact local fixtures and source paths described here, not full-product acceptance.

This package adds only `scripts/acceptance_next_local_20261013.*` and this review. Root owns implementation, Git/index/Main and integration. The previous frozen 25-check and 4-check runners are unchanged. No full suite, repository clone, browser/CUA/ETW, native Core, provider, model, grant, deployment or push was run by this reviewer.

## Isolated actual ledger evidence

Command actually executed:

```text
node scripts/acceptance_next_local_20261013.mjs --commit e2597a99e444f134c3a665f6f6b7cba874259f2c --label baseline-e259-v2
```

The runner exports only `native/go.mod` and nine needed internal packages: ledger, connections, executors, protocol, identity, historyexport, retentionplan, documents and syncstream. The resulting archive contains 172 tracked files / 1,380,288 exact Git-object bytes. The archive is not a clone or checkout. Its Windows CRLF export changes in five text files are recorded, then replaced with exact immutable Git-object bytes; every source file is verified before and after the run. `sourceUnchanged` is true.

Actual Go command: pinned Go executable `C:/Users/Admin/Documents/Codex/2026-10-11/task/.tooling/go/bin/go.exe`, `test -json -count=1 -run ^TestIndependentNextLocal ./internal/ledger`, run under the owned E archive's `source/native` directory. It executes only six authored tests. Existing package tests compile for their typed fixture helpers but do not run. Synthetic store admission bindings do not start Core processes.

Environment: Windows, PowerShell, Node `v25.9.0`, Go `go1.27.2 windows/amd64`, CGO disabled. Process environment is a named whitelist of system path/process variables plus explicitly owned E cache/temp directories; `GOENV/GOWORK/GOPROXY/GOSUMDB=off`, `GOTOOLCHAIN=local`. No provider credentials or arbitrary parent environment are passed. The exact environment names, paths, command, outcome and source manifest are in `results.json`.

Evidence directory: `E:/CodexScratch/ux-independent-20261013/next-local-baseline-e259-v2/`, including `results.json`, `manifest.json`, `go-test.jsonl`, stderr, immutable source archive and the exact injected `probe.go`.

- Archive SHA-256: `4181ffa82bd44b966f049e62604cb8d5233a11684c6e08e21d58f83d72b3d9a6`.
- Probe SHA-256: `88a59725f59290fb6239a44087b4ab9668deeb1cc7cf9dba1b18b20d103eed5b`.
- Runner SHA-256: `c1390d3830d4aac68af98366ccb722d07729d116ba8ea28db2db0adb3c62600b`.
- Go executable SHA-256: `b8cd508a0a7aac883cd750e9e75eb306d219e726bdf1cde29009490729437fbf`.
- Exit: 1, no tool error or signal, four PASS and two FAIL.

The earlier `next-local-baseline-e259-v1` directory is retained as setup evidence. Archive CRLF comparison stopped before Go execution; that setup error is not a product failure.

### NL-01 - required peer result is excluded with history disabled

Both participants have valid profiles with `IncludeHistory=false`. A separate completed shared-channel ambient result exists. The first group stage completes with a nonempty 24-byte result. Advancing the group stops at one stage: `needs_owner / stage_context_unavailable`.

Causal source boundary: `captureContextVersion` in `native/internal/ledger/context.go` breaks the entire prior-source loop for the false history flag. `validCollaborationWork` in `collaborations.go` requires each explicit prior group task in the next stage's `Context.Sources`. A tiny required result therefore becomes unavailable even without byte pressure. Proposed V3 acceptance: keep the profile false and exclude optional ambient history while retaining complete authorized dependencies from this explicitly requested group. This is an availability/selection failure; no permission bypass is alleged.

### NL-02 - optional report bytes displace a required peer source

A frozen published report fits the original selection budget. A later completed proposal encodes as a 2,181-byte `Sources` array; the proposal and optional report together encode as 9,360 bytes. Advancing stops with `needs_owner / stage_context_unavailable`, one stage, although the required proposal by itself is below 8,000 bytes.

Causal boundary: v2 freezes public reports before peer source selection; `captureContextVersion` then refuses the combined candidate. Reserving two count slots does not reserve encoded bytes. Proposed V3 acceptance: prioritize complete required peer results, then include only whole frozen reports that fit, followed by optional ambient history. Required sources alone exceeding the existing encoded bound must still stop at the owner gate. Old v1/v2 journal selection must retain its historical policy rather than silently replaying as v3. Root and the group owner received exact fixtures and byte counts.

### Passing ledger controls

- A 1,415-byte reply containing control characters encodes as an 8,845-byte source array. The next stage remains blocked at `needs_owner / stage_context_unavailable`; raw byte length is not used to bypass the encoded 8 KB limit.
- A report published after group intake is not added to the task's frozen report pins. Reading its artifact through the task-scoped reader returns `ErrNotFound` and no bytes, while the selected original artifact remains readable. Revoking the unselected report source does not suspend or retarget the original group.
- A late channel revision/removal suspends the group with `authority_changed`, retains its existing stage/frozen pins and refuses the original queued task claim.
- Revoking a selected uploaded source file suspends the group, preserves the original resource pin, refuses the queued claim and refuses reading the revoked file. These are local store fixtures, not remote identity, native stop or browser acceptance.

## Independent passive transport source evidence

Command actually executed:

```text
node scripts/acceptance_next_local_20261013.transport.mjs --commit e2597a99e444f134c3a665f6f6b7cba874259f2c --label next-local-e259-transport-v1
```

The runner extracts actual `sync()` and its actual SSE callback along with existing acknowledgement/draft ownership functions from immutable Git objects. Requests and EventSource are intercepted in a bounded Node VM. Typed local-owner snapshots model a passive reader receiving already committed work; no HTTP service is launched. Layout, unrelated projection rendering and unrelated journals are support stubs. This is one emulated reader, not genuine two-client HTTP, browser or cloud acceptance. The UI owner's reported c273 two-cookie local service check is separate supporting evidence.

Evidence: `E:/CodexScratch/ux-independent-20261013/next-local-e259-transport-v1.json`; exit 1, **2 PASS / 3 FAIL / 0 HARNESS_ERROR**. App Git-object SHA-256 `fac676e1edb78bfc5c4138c767f9a54aa4d7a38c54543d96f2ac834497ae0ad1`; harness SHA-256 `79d6371ae98d097f79ff9425a773806c99c35999c92f9c05ac537fe8a52772a9`. Environment: Node `v25.9.0`, `win32`, `x64`.

### NL-03 - passive exact ACK retires the outbox without clearing its owned draft

The passive snapshot case and the SSE authority-event/resnapshot case remove the exact original outbox item but leave its original live draft, saved-view draft and pending nonce. Both cases make zero POSTs. A held owned retry followed by passive acknowledgement and a lost attempt response also leaves those fields, with exactly one original POST; no second POST is made by the probe.

Causal boundary: `sync()` and stream projection call `acknowledged` directly, whereas the earlier retry/resume correction invokes `clearAcknowledgedOutbox` around attempt-owned acknowledgements. Passive receipt transport therefore misses the same owned draft cleanup. A newer same-text pending nonce remains protected in this baseline, and a nonmatching snapshot retains the original outbox and draft. Root and the UI owner received the exact source reproduction before independent retest of the proposed passive helper.

## Retest dependencies and remaining limits

Root will supply a quiescent immutable target for the group v3 selection and passive UI acknowledgement changes. Retest the same authored ledger and transport assertions against that exact source; retain the baseline RED results. Additional v3 acceptance should verify that a whole report omitted at a later stage cannot obtain task reader authority, while ancestral file/source/channel/plugin revocation still fences the task through its required peer dependencies. Genuine old-version replay and binary/service checks are separate Root evidence unless independently run here.

There is no claim of browser, accessibility/layout, actual Tauri interaction, phone, cloud sync, paid-provider/model evaluation, native Core process stop/resume or broad concurrency acceptance. No screenshots are claimed, and the blocked CUA/Chrome surfaces were not retried. The earlier 25+4 GREEN result remains historical evidence for its exact534 paths; it does not cover these newly reproduced gaps or establish full-product completion.

## Immutable retests and final bounded result

The historical retest dependencies above were subsequently met for these exact paths. Root granted the write slot for this review after the source checkpoints and the independent outputs. No implementation, Git/index or Main changes were made by this reviewer.

| Immutable code | Command/evidence label | Result |
| --- | --- | --- |
| `b2b7bf83d296b29ad6f51c306e644ac3914b279c` | `integration-b2b-transport-v1` | Exit 0; 5 PASS / 0 FAIL / 0 HARNESS_ERROR |
| `b2b7bf83d296b29ad6f51c306e644ac3914b279c` | `integration-b2b-ux-v1` | Exit 0; 25 PASS / 0 FAIL / 0 BLOCKED |
| `b2b7bf83d296b29ad6f51c306e644ac3914b279c` | `integration-b2b-draft-ownership-v1` | Exit 0; 4 PASS / 0 FAIL / 0 HARNESS_ERROR |
| `e127a4189ffa39cf055930ed05a30d7360aba928` | `integration-e127-ledger-v1` | Exit 0; unchanged six ledger tests all PASS; source manifest unchanged |
| `e127a4189ffa39cf055930ed05a30d7360aba928` | `integration-e127-ancestry-v1` | Exit 0; four families/eight subcases all PASS; source manifest unchanged |
| `e127a4189ffa39cf055930ed05a30d7360aba928` | `integration-e127-ancestry-v2` | Exit 0; four families/ten subcases all PASS; reviewer history-off/plugin additions and owned E USERPROFILE; source manifest unchanged |

The ancestry v2 run was required by the new plugin/history-off guards and explicit profile-directory environment, not by the documentation update. Existing ancestry v1 assertions were retained; only the additional guards and two plugin subcases were added. Four parent family outcomes and ten nested subcase outcomes appear in JSON; those aggregate family outcomes are not fourteen independent product scenarios.

Actual command lines:

```text
node scripts/acceptance_next_local_20261013.transport.mjs --commit b2b7bf83d296b29ad6f51c306e644ac3914b279c --label integration-b2b-transport-v1
node scripts/ux_acceptance_20261013.mjs --commit b2b7bf83d296b29ad6f51c306e644ac3914b279c --output E:/CodexScratch/ux-independent-20261013 --label integration-b2b-ux-v1
node scripts/ux_acceptance_20261013_draft_ownership.mjs --commit b2b7bf83d296b29ad6f51c306e644ac3914b279c --label integration-b2b-draft-ownership-v1
node scripts/acceptance_next_local_20261013.mjs --commit e127a4189ffa39cf055930ed05a30d7360aba928 --label integration-e127-ledger-v1
node scripts/acceptance_next_local_20261013.ancestry.mjs --commit e127a4189ffa39cf055930ed05a30d7360aba928 --label integration-e127-ancestry-v1
node scripts/acceptance_next_local_20261013.ancestry.mjs --commit e127a4189ffa39cf055930ed05a30d7360aba928 --label integration-e127-ancestry-v2
```

The exact Go commands, environment names/values for owned paths, commands, source hashes and per-test outcomes remain in each result JSON. The ancestry runner invokes only `go test -json -count=1 -run ^TestIndependentNextAncestry ./internal/ledger`; the unchanged original runner uses `^TestIndependentNextLocal`. Node is `v25.9.0`, Go is `go1.27.2 windows/amd64`, platform is `win32/x64` and CGO is disabled.

### Evidence identity and isolation

All evidence below is under `E:/CodexScratch/ux-independent-20261013/`:

- b2b JSONs: `integration-b2b-transport-v1.json`, `integration-b2b-ux-v1.json`, `integration-b2b-draft-ownership-v1.json`; receipt `integration-b2b-review.md`.
- Unchanged e127 ledger: `next-local-integration-e127-ledger-v1/results.json`, raw `go-test.jsonl`, manifest and exact injected probe. Original probe hash remains `88a59725f59290fb6239a44087b4ab9668deeb1cc7cf9dba1b18b20d103eed5b`; runner remains `c1390d3830d4aac68af98366ccb722d07729d116ba8ea28db2db0adb3c62600b`.
- Ancestry v1: `next-ancestry-integration-e127-ancestry-v1/results.json`; probe `cd29e94bf2462b3c796e3b7c69ad46d0e79f2ad2ac7d0dd395145531a8f27718`, harness `0a50afcffc60c91ffe7a34c333e693c4e8c72f8ada8ce8520681c1e3d8557338`.
- Ancestry v2: `next-ancestry-integration-e127-ancestry-v2/results.json`; probe `25bb96771dd1f32ac3a931295c1a1f3bb29d8ebdcd90903b77384328fe379968`, harness `bafac18592a9a84b5485a91d408dec81ccebcaef0764a6425c41427deb1b0676`. Exact probe and harness files are retained alongside each result.
- b2b app Git-object SHA-256: `ddcc310493181337adc244876127833a1ec80e415d904d76b1b6231f2a093955`; Connections `00057bd40d0b7bda8eff04a563332cff8770db0c8fce561fb2c36a425e184d33`; HTML `499d86507dc91a33e07c8b1cd803e82f2a0ed4d5b33118e775f04d46299d88d3`.
- Transport/frozen UX/ownership harness hashes remain, respectively, `79d6371ae98d097f79ff9425a773806c99c35999c92f9c05ac537fe8a52772a9`, `ee7997886cfcbf6bd86234ad4129ca9e2a0b70dc9552c35fa0d296b89d5ff576` and `665b82f07fc02dd36c1e7dc328bc3a8850839562d5688773836b54e8f672c571`.

Each e127 ledger archive contains the same 173 immutable tracked files / 1,408,162 exact Git-object bytes; archive SHA-256 `10580770c3c2dc380440ca05b803ddac2f7636c646d94a81b4680570b0d1a596`. Five CRLF export normalizations are recorded and replaced with exact Git-object bytes before compilation. Source bytes match the manifest after execution. The uncommitted working `native/internal/ledger/adapters.go` was not used. Pinned Go executable hash remains `b8cd508a0a7aac883cd750e9e75eb306d219e726bdf1cde29009490729437fbf`.

The unchanged six-test runner and ancestry v1 pass only the named system-variable whitelist plus owned E cache/temp variables; they do **not** specify a USERPROFILE override and are not evidence of complete home-directory isolation. Ancestry v2 additionally sets Go's `USERPROFILE` to its own E `profile` directory and records it in `environment.isolatedPaths`. All these Go runs use `GOENV/GOWORK/GOPROXY/GOSUMDB=off`, `GOTOOLCHAIN=local`, `CGO_ENABLED=0`; no arbitrary parent application environment or provider credential is forwarded. Node/Git orchestration is read-only with respect to the source repository; the profile override claim applies to the spawned Go process, not every host tool.

### What the retests establish

The original 24-byte history-off peer result and the 2,181-byte required-source budget case now advance to the reviewer on e127 without changing the baseline assertions. Encoded control characters still close the owner gate at 8,845 JSON bytes, and unselected later-report/file/channel guards remain intact. Whole optional reports are omitted rather than truncating required peer results.

The additive fixture selects a report grounded in an original private uploaded file. The first group stage has its selected public output reader. The reviewer wholly omits the report, retains only the required peer source, keeps `IncludeHistory=false` and cannot read the omitted report directly, including after replay and an exact original-nonce retry. Runtime prompts do not import the original private request. Selection and context hashes remain stable across restart.

After report omission, the queued reviewer and completed dependent public reply retain fences through their peer ancestor. Four separate causal changes are checked: original task source-access revocation; original report authority invalidation by source-bot state revision; original grounding-file access revocation; and selected report-reader plugin disable. Queued claims remain refused, Sources metadata reports unusability, suspended group/frozen pin/stage identities survive restart, and completed dependent reply/context/artifact projections remain masked. These are actual local ledger fixtures with synthetic admission bindings; they are not native process-stop or provider evaluation.

The V1/V2 tests rewrite only the final synthetic group-creation record to the exact typed legacy capture policy and recompute its digest. The new store accepts those typed historical records and preserves their original version, frozen pin selection, nonce and historical owner gate; it does not silently upgrade them to V3. This is **typed replay compatibility**, distinct from Root's genuine earlier-binary journal generation/reopen evidence. The independent reviewer did not rerun or claim that separate binary proof.

On b2b, the unchanged passive snapshot/SSE source cases now clear the acknowledged original drafts/pending nonce without a new POST. A newer same-text pending owner and nonmatching received work remain preserved. Passive ACK during a held retry followed by a lost response reconciles with one original POST and no second POST. This remains one emulated reader using actual source callbacks and typed writer snapshots; genuine two-cookie service evidence belongs to the UI owner/Root.

All previous RED/setup evidence remains immutable. Native/browser/layout/focus/accessibility, actual Core lifecycle, real phone, cloud, provider/model and broad concurrency gates remain outside this review. Neither the historical 25+4 nor the current local fixture GREEN results establish full-product completion.
