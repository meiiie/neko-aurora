# Required group dependencies within the existing context budget

This slice continues the finite two-bot group flow from application commit `e2597a99e444f134c3a665f6f6b7cba874259f2c`. The prior Version 2 policy reserved two slots, but optional reports could still consume the byte budget before required peer results. A near-limit report and bounded proposal stopped stage 2 at `needs_owner/stage_context_unavailable`. A profile with `IncludeHistory=false` also excluded the explicitly requested group's prior result, producing the same stop even for a short reply.

The RED probes use Go production code equivalent to that base commit plus newly authored tests. Receipts include the actual working-tree status and source hashes; they do not claim a clean checkout when other workers' changes are present. The two RED cases both recorded one stage and a missing required stage 2.

## Deterministic Version 3 selection

New groups freeze `PublishedReportContext.Version=3`. They retain Version 2's exact repeated-publication identity, newest-six limit and four admitted report-file identities. Creation measures the encoded `Sources` plus `Reports` JSON envelope, including field names, brackets and escaped strings, against the existing 8,000-byte bound. Every frozen report therefore fits into stage 1. A whole report that cannot fit is counted as omitted before freezing.

Each new group's stage context uses this priority:

1. Complete authorized prior results from this group's exact stage/request map, in canonical stage order.
2. Whole frozen reports, newest first, while the combined eight-slot and 8,000-byte envelope still fits.
3. Eligible completed ambient history in the same owner, space, shared channel and project, only when the pinned profile permits history.

Selections are serialized chronologically and hashed with the existing ContextPin algorithm. No text, reply, artifact descriptor, report or file excerpt is truncated or summarized. New publication pins are not fetched during continuation. Whole optional reports that no longer fit increment the omission count. A required dependency that cannot fit alone still causes the existing owner stop; it is not replaced with a partial source.

`IncludeHistory=false` retains its value and excludes ambient history. Complete results from the requested group are explicit task dependencies. They enter the admitted descriptive context without importing a bot's private conversation, changing the profile, expanding the owner goal or granting a new capability.

## Authority and durable replay

Stage 1 contains all frozen reports. Stages 2 and 3 retain complete earlier stage sources, whose stored contexts recursively preserve original report, file, source, destination and plugin authority. Dropping a whole optional report from a later prompt therefore does not drop its dependency fence. Source access, destination membership or reader-policy revocation blocks claim/continuation; regrant or reenable does not rebind the old pins.

The frozen group plugin selection must remain the same even when a selected report is absent from a later prompt. The exact `validPluginWork` hook reconstructs eligibility using the group's original frozen report pins only for Version 3. Its owned helper rejects missing groups, mismatched owner/channel/tool policy/plugin pins and invalid original source pins. This preserves the original reader revision/schema pin for replay. Actual discovery and `report.read_artifact` continue to require a selected current-context report/file pin: an omitted file is undiscoverable and unreadable. A selected stage-1 file still reads its exact admitted bytes.

Historical group policies 1 and 2 retain their previous capture, omission, ordering, hash and owner-stop behavior. Historical Version 2's near-limit byte stop remains a positive compatibility test. Unsupported policy versions 0 and 4 reject. New Version 3 groups retain identical selection hashes, stage maps and original nonce receipts across three restarts. A new nonce creates an independent group; changed intent with the original nonce conflicts.

Version 3 is another local journal reader boundary. A control-plane binary that supports only group report policies 1 and 2 cannot replay newly created Version 3 records. No journal is rewritten or automatically downgraded by production code. Root owns the exact binary upgrade/downgrade evidence and preserved checkpoints.

## Canonical Core contract

The Core repository was inspected read-only at clean commit `973a3bd93466baa4689e01c4050dfc59f5a7f7c4`, under `C:/Users/Admin/Documents/Codex/2026-10-11/task/core-published-report-context-9f`. The validator at `src/core/context/admission.ts:77` accepts same-scope derived sources and validates their hashes, extents, source count and manifest budget. It treats the profile span as descriptive data; there is no history-preference gate over independently admitted sources. `contextSourceMessages` retains the admitted manifest unchanged. `StructuredContextCompactor` construction and installation retain its authenticated source group and exact binding.

Read source SHA-256:

| Core source | SHA-256 |
| --- | --- |
| `src/core/context/admission.ts` | `159ea9bab612a0d4f28cccbd3a08ac7c7843ac084d17b2343e10dcdab4190952` |
| `src/adapters/context/compactor.ts` | `b887cede986fa757f3577b9f84030813b4889988fc09aee66c870f84aada17a5` |

Go fixtures build a valid existing RuntimeManifest Version 1 with history disabled and exactly zero, one and two required prior-result sources at stages 1, 2 and 3. The seven-field launch frame, ContextPin Version 5, manifest schema, authenticated reader envelope and Core loop are unchanged. This is a source audit and local host-contract fixture check, not a newly executed Core/provider/model acceptance run.

## Owned paths and validation

The worker owns `group_context.go`, `group_context_test.go`, `group_context_budget_test.go`, `collaborations.go` and `published_reports.go` in the ledger, the budget runner and this report. Root additionally authorized only the early Version 3 dispatch block in `context.go` and frozen-eligibility call block in `plugins.go`. All other context, permissions, plugin, API, UI, host and Core code remains outside this worker's ownership. Root alone writes Git/index/Main.

The runner is `scripts/e2e_group_context_budget_20261013.py`; despite its filename, it is an isolated local ledger fixture gate. It passes only explicitly named Windows environment fields plus local Go/cache/temp controls. Provider and host configuration are excluded. Environment: Windows amd64, Go `go1.27.2 windows/amd64`, Python 3.13, `GOPROXY=off`, `GOSUMDB=off`, `GOTOOLCHAIN=local`, `GOENV=off`, `CGO_ENABLED=0`. Owned journals/temp directories and cache stay under `E:/CodexScratch/group-context-budget-20261013`; the owned cache is its `gocache` directory.

Common command prefix:

```powershell
python scripts/e2e_group_context_budget_20261013.py --go C:/Users/Admin/Documents/Codex/2026-10-11/task/.tooling/go/bin/go.exe
```

Each run directory retains `results.json`, full `go-test.jsonl` and stderr, with exact base commit, command, working-tree status and Go-source/runner SHA-256.

| Owned output directory suffix | Arguments after the prefix | Outcome |
| --- | --- | --- |
| `red-r1` | `--output E:/CodexScratch/group-context-budget-20261013/red-r1` | Expected RED: both actual stage-2 failures; source set `ed0970bd62136fc874e8005124ad5c8134a0e74db0c9c08a001264de0de69645`. |
| `green-r1` | `--output E:/CodexScratch/group-context-budget-20261013/green-r1` | PASS: two initial cases, before the stronger file/replay gates. |
| `composition-r1` | `--output E:/CodexScratch/group-context-budget-20261013/composition-r1` | Build FAIL: test used nonexistent `copyContext`; corrected to the existing `copyWork(...).Context`. |
| `composition-r2` | `--output E:/CodexScratch/group-context-budget-20261013/composition-r2` | FAIL: final-write fixture omitted its canonical context receipt; file-bearing stage-2 restart exposed recomputed plugin eligibility. |
| `replay-debug-r1` | `--output E:/CodexScratch/group-context-budget-20261013/replay-debug-r1 --run '^(TestGroupContextBudgetNearLimitReportPriorResultsContinue\|TestGroupContextBudgetReplayStableSelectionAndNonce)$'` | Near-limit completion PASS; replay FAIL at accepted stage-2 record, sequence 22. |
| `replay-debug-r2` | `--output E:/CodexScratch/group-context-budget-20261013/replay-debug-r2 --run '^TestGroupContextBudgetReplayStableSelectionAndNonce$'` | Diagnostic FAIL: profile/runtime/context/group/publication guards true, plugin guard false. This justified Root's exact additional plugin hook. |
| `composition-r3` | `--output E:/CodexScratch/group-context-budget-20261013/composition-r3` | PASS: six top-level tests, nine pass events including subtests after the durable eligibility fix. |
| `final-composition-r1` | `--output E:/CodexScratch/group-context-budget-20261013/final-composition-r1` | FAIL: header-boundary fixture was below the intended limit; seven other top-level tests passed. Only its synthetic reply length was calibrated. |
| `final-composition-r2` | `--output E:/CodexScratch/group-context-budget-20261013/final-composition-r2` | PASS: eight top-level tests, eleven pass events, zero failures; source set `88e6c20322c70cef1e030597f9fc159e31c963d483c88bb57f6fc586e11fe6a8`. |
| `related-r1` | `--output E:/CodexScratch/group-context-budget-20261013/related-r1 --run '^(TestGroupContext\|TestPublishedReport\|TestCollaboration\|TestGroupServiceRecovery\|TestIndependentGroup\|TestPlugin)'` | PASS: 82 top-level tests, 174 pass events including subtests, zero failures; the same source set as `final-composition-r2`. |

The near-limit completion records source/report envelope sizes 7,441, 1,055 and 2,171 bytes at stages 1, 2 and 3, with source counts zero, one and two, and report counts one, zero and zero. The report is dropped whole in later stages, with omission count one. Complete prior text/reply/artifact descriptors are checked against stored results, and only the final coordinator writes.

A 1,500-byte reply containing HTML-sensitive characters encodes to a 9,457-byte required-source envelope and retains `needs_owner/stage_context_unavailable`. A 7,996-byte raw report array becomes an 8,021-byte envelope; it is omitted before group freezing. These tests protect encoded byte and header accounting without increasing the budget.

Related regression coverage includes historical group capture and Version 2's byte stop, frozen source/project/file and reader pins, publication revocation, owner cancellation, service recovery and nonce/stop-receipt fences. Formatting and whitespace checks pass. The source secret scan reports 670 text files and zero findings; Root must scan the actual candidate files again before its checkpoint.

Remaining limits: required prior results that exceed the budget still need owner action or a separately accepted canonical Core compaction path. Real model quality, actual Core execution for this slice, native Tauri/browser/phone acceptance, cloud sync, multi-owner principals and external recipients are outside these fixture claims. Formal Go `-race` is unrun because this environment lacks a C compiler. No paid provider, real credential, grant, deployment, push or Main state mutation occurred.

## Actual native compatibility gate at e127

The later actual Windows Go/Rust/Core gate supplements the earlier ledger-only scope. Application: `e127a4189ffa39cf055930ed05a30d7360aba928`; Core: `973a3bd93466baa4689e01c4050dfc59f5a7f7c4`. Fresh owned state/home/temp, named environment fields and a deterministic loopback provider were used; no installed Core home, account, consent or grant was accessed.

```powershell
python scripts/e2e_group_context_budget_native_20261013.py --go-tool C:/Users/Admin/Documents/Codex/2026-10-11/task/.tooling/go/bin/go.exe --output E:/CodexScratch/group-context-budget-20261013/native-e127/r1
```

**33 checks PASS, zero failures**, 24.79 seconds. Evidence: `E:/CodexScratch/group-context-budget-20261013/native-e127/r1/{results.json,native-seal.json,seal-audit.py}`. Private journal/config/task homes/logs remain excluded from portable evidence. No native or Go rerun was performed for sealing.

Go was built from selective `git archive` of e127's `native/contracts/scripts`, tree `5d67fd62e85858052d202f44245649905933c79d`, using `go build -buildvcs=false -o r1/neko-control-e127.exe ./cmd/neko-control` in `r1/immutable-source/native`; the receipt contains exact absolute arguments. All 379 exported files match the executed source hashes: 359 exact Git byte matches and 20 LF/CRLF-only conversions in other text/data/assets/PowerShell files. All `.go` files and the pinned supervisor are exact Git bytes. No embedded VCS identity is claimed.

Role paths came from the pinned candidate launch manifest. Actual SHA-256:

| Role | SHA-256 |
| --- | --- |
| Built Go | `283a33e6bdf3adcded7f901793494137f92307d8f5d8d9ff696f2238325ee833` |
| Pinned Rust host | `a1693c4f3309c86e29b348ffc913c888cae3f8fd2bdf04bcac418e107a6654d2` |
| Pinned Core | `356247616082e613abd99b97838fd533f5e14ab64b07f480d78ae799d10648bf` |

Actual Core admits complete prior-source counts **0/1/2** with both profiles' history disabled. The near-report group admits report counts **1/0/0**, keeps its reviewer's history disabled and retains complete peers after whole-report omission. Encoded envelopes are `7412/1109/2319` bytes. Private prompt/history canaries stay excluded. Both groups allow only the final coordinator to create one operation/file; each 65-byte output downloads with its exact hash. A completed-service restart retains nonce receipts, pins/maps and artifacts. The earlier ledger fixture separately checks three restarts.

All eight owned tasks have retained trusted Rust-host stop receipts with matching admitted Core PIDs and `runtimeStopped=true`. The owned service and provider thread closed; no external OS HANDLE measurement or broad process enumeration/kill is claimed. On midstage failure, service closure alone would not prove Core stopped; preserve the private journal and report unknown state. This run's retained receipt audit has zero unknown tasks.

Executed runner SHA-256: `ecc6b9b4955ada053ecf244fb5527235e3386e2210c10826fe5ec7f269858098`; original result SHA-256: `a698e22a0069fe839d09b4ba2702558b1f255405fd287af9fa13d828fc19b258`. Archive/source/role/helper/file hashes and exact policy/admission observations are sealed in the evidence.

This proves local protocol/context/tool compatibility with a synthetic provider. Real model/provider quality, native Tauri/browser/phone/AT, cloud and multi-owner acceptance remain outside scope; native CUA/Chrome gates remain independently blocked. No UI workaround, paid provider, real credential, deployment, push or Main state mutation occurred.
