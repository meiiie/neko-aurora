# Independent UX source acceptance — 2026-10-13

Status: immutable baseline **RED**; worker/integration retest pending. This review executes actual JavaScript functions and the complete Connections module in a bounded Node VM with typed public fixtures and a minimal DOM. It is not browser, native Tauri, phone, cloud sync, backend, provider or model acceptance.

Only `scripts/ux_acceptance_20261013*.mjs` and this review are owned by the independent reviewer. No implementation, backend, Core, Git, Main, service state or other worker files were changed. Root owns checkpoints and integration. Evidence is appended under `E:/CodexScratch/ux-independent-20261013`; existing results are never overwritten.

## Baseline evidence

- Code commit: `195a0ec990db0478d9f052bc0e79a83b12dc9d78`.
- Source mode: `git show <full commit>:<path>`; the mutable working tree is never evaluated.
- Source SHA-256: app `ca4dc7f7c88995c8d0a5d2e940f5bf5850d86fba651bf5e20d113c7ef25fb1a9`; Connections `1c2e341c0d17ba6e8b83ae12b12716d755816ea6748238ea55d9ed081bd46770`; HTML `499d86507dc91a33e07c8b1cd803e82f2a0ed4d5b33118e775f04d46299d88d3`.
- Environment: Windows, Node `v25.9.0`, `win32`, `x64`; PowerShell; no network surface or external endpoint supplied to the VM.
- Command: `node scripts/ux_acceptance_20261013.mjs --commit 195a0ec990db0478d9f052bc0e79a83b12dc9d78 --output E:/CodexScratch/ux-independent-20261013 --label baseline-195-v4`.
- Result: exit 1, **16 PASS / 4 FAIL / 5 BLOCKED**. Four failed assertions represent two gaps, not four independent product defects.
- Raw evidence: `E:/CodexScratch/ux-independent-20261013/baseline-195-v4.json`. It records full command, environment, exact source hashes and harness hash `ee7997886cfcbf6bd86234ad4129ca9e2a0b70dc9552c35fa0d296b89d5ff576`; the exact runner is retained as `harness-ee7997886cfcbf6bd86234ad4129ca9e2a0b70dc9552c35fa0d296b89d5ff576.mjs` in the same directory.

`baseline-195-v1.json` is retained as harness-development evidence: two missing minimal-DOM methods produced harness errors. Those errors are not product findings. `baseline-195-v2.json` corrected the DOM methods; v3 additionally marks dependent missing-preload cases BLOCKED and adds session/authority fences; v4 adds newer-pending, mismatched-ACK and session-change safeguards plus immutable runner snapshots. No historical evidence was rewritten. The 17 old missing-session/fixture failures in `ui_cross_channel_report_local.mjs` are excluded from this review.

## Reproduced gaps

### UX-01 — first Connections does not preload the source step

The verified local owner has `local_core_profiles_v1`, is not read-only, and opens Connections. The actual module issues GET `/v1/core-discovery` and GET `/v1/connections`, but issues no GET `/v1/core-profile-sources`; the source panel stays closed and the sole typed safe descriptor remains unselected. A two-source fixture likewise never reaches the explicit source choice step.

Baseline locations: `connections.js:40` opens Connections and selects the account tab; `:73` contains source discovery, reached by the separate explicit installed-profile action at `:80`. The required change is source-descriptor onboarding only. Automatic profile reads, inspection/execution consent, provider calls and readiness claims remain outside the authorized automatic step.

The absence of the automatic request blocks meaningful tests for preload dedup, late navigation, close, session and authority changes. The baseline run records those five cases as BLOCKED rather than claiming they are safe.

### UX-02 — automatic reconnect ACK leaves the acknowledged draft and pending nonce

The typed input has bot `bot-meiiie`, text `Original awaiting acknowledgement`, foreground lane and nonce `independent-reconnect-001`. The request response echoes the input with a task ID, exact private conversation ID, `waiting_host` status and valid sequence/time. The input is the current `state.pending`, the current outbox item, the live composer text, and the saved current-view draft at navigation generation 3.

Actual `resumeOutbox()` removes the acknowledged item from the outbox, but leaves the original live composer text, the saved draft and `state.pending` unchanged. The user can therefore still see an already acknowledged message as an unsent draft. Baseline locations: `app.js:66` reconciles an acknowledgement by removing the matching outbox item; `:72` automatic replay calls it without current-draft/pending cleanup.

The same fixture with a newer edited composer draft preserves the edit. A held response followed by navigation to another bot also preserves the new-view draft. Any fix must retain both protections. This source-level finding does not assert an actual duplicate backend task or provider execution.

Both reproductions and typed fixtures were sent directly to Root and the responsible implementation owners before any fix. The independent reviewer did not edit implementation code.

### UX-03 — supplemental working fix clears a newer same-text draft

Root requested a separate ownership probe before checkpoint. The frozen 25-check runner remains unchanged. `scripts/ux_acceptance_20261013_draft_ownership.mjs` evaluates actual outbox functions from working `app.js`, pinned to quiescent SHA-256 `79ee96db25e365f9a5684f56c74569bbe5bdbef1f400db818a898366038cee59`. It checks the full file hash both before and after execution, then retains the exact source and harness under their hashes in the owned E evidence directory. HEAD `195a0ec…` is a reference only, not the tested working code revision.

Command: `node scripts/ux_acceptance_20261013_draft_ownership.mjs --working-sha 79ee96db25e365f9a5684f56c74569bbe5bdbef1f400db818a898366038cee59 --label working-79ee-draft-ownership-v1`. Environment is Node `v25.9.0`, `win32`, `x64`. Result: exit 1, **2 PASS / 2 FAIL / 0 HARNESS_ERROR**. Evidence: `E:/CodexScratch/ux-independent-20261013/working-79ee-draft-ownership-v1.json`; harness SHA-256 `665b82f07fc02dd36c1e7dc328bc3a8850839562d5688773836b54e8f672c571`.

A newer `state.pending` nonce has the same text, bot and lane as an older retained outbox input. Current view, account and navigation generation still match. An exact old ACK correctly leaves the newer pending nonce alone, but the new `clearAcknowledgedOutbox` helper continues past that condition and clears both the live composer and saved-view draft. The probe reproduces this with the newer pending input present before replay and with it installed while the old response is held. An unchanged original ACK clears its owned draft correctly; a different-text newer pending draft is preserved. Root and the reconnect owner received the exact typed reproduction. This is a source-state ownership invariant check, not a claim that a specific browser route creates this state or that any backend task is duplicated.

The supplemental script also records edit-away-then-back before ACK: both drafts are cleared when the final text again equals the original. This is labeled an observation, not an additional product failure. The current input handler has no draft mutation generation; a requirement to preserve renewed draft ownership after any intervening user edit needs to be explicit. That observation does not change the definite newer-nonce finding.

## Passing source boundaries

- Opening Connections creates no inspection/execution consent and does not GET `/v1/core-profiles`. Fixture actor, read-only and missing-capability contexts do not preload sources.
- A valid published-report Sources response is parsed for the exact task, channel and sequence. Extra private context, reply and source labels are not retained. Wrong destination and older sequence are refused.
- The actual Sources reader renders the public report title. Authority fencing clears the open view and invalidates a held response even if the report later becomes active again. Navigation drops a held response without retargeting. A report access change clears already presented source rows.
- Group Sources resolves only the exact current stage and group/channel/project scope; zero-stage and wrong-channel fixtures yield no source work. The literal shared public goal is placed in the intended DOM structure. This does not test backend report-context saturation or scoped actor projection.
- A rejected outbox entry does not block a later valid normal entry during automatic replay. Late ACKs preserve edited or newly navigated drafts.

## Remaining acceptance gates

Retest the same runner against Root's immutable integration checkpoint and record exact source/harness hashes and any new helper dependencies. Worker-local tests or working-tree hashes do not replace that retest.

Native CUA is blocked by kernel error `0x80070003`; Chrome did not establish the supplied isolated user-data directory. Neither surface was retried or bypassed in this review, and no screenshot is claimed. Browser layout, focus behavior, accessibility, actual native interaction and phone acceptance remain untested. Group saturation is a separate backend fixture validation owned by the group worker. No service, model, grant, provider, deployment or push was launched here.

## Immutable integration retests - final source result

Latest independent result: **GREEN on code commit `53495231a06aac97c1092029fda35971bdbc08c5` for the bounded source acceptance paths: 25/25 UX checks and 4/4 supplemental ownership checks, both exit 0.** Earlier RED and pending wording above describes historical baseline/working evidence. The assertions and harness hashes were unchanged for both immutable integration retests below; no result was rewritten or relaxed.

Both runs used PowerShell, Node `v25.9.0`, `win32`, `x64`, reading source through immutable Git objects. Harness SHA-256 remains `ee7997886cfcbf6bd86234ad4129ca9e2a0b70dc9552c35fa0d296b89d5ff576` for the frozen 25-check runner and `665b82f07fc02dd36c1e7dc328bc3a8850839562d5688773836b54e8f672c571` for the separate ownership probe. Exact hash-named harness snapshots remain in the E evidence directory.

| Exact code commit | Frozen UX runner | Supplemental ownership | Evidence labels |
| --- | --- | --- | --- |
| `104bdf2e34b65f345d2b1ffb6a6014d1a7629b5c` | Exit 1; 24 PASS / 1 FAIL / 0 BLOCKED | Exit 0; 4 PASS / 0 FAIL / 0 HARNESS_ERROR | `integration-104-ux-v1`, `integration-104-draft-ownership-v1` |
| `53495231a06aac97c1092029fda35971bdbc08c5` | Exit 0; 25 PASS / 0 FAIL / 0 BLOCKED | Exit 0; 4 PASS / 0 FAIL / 0 HARNESS_ERROR | `integration-534-ux-v1`, `integration-534-draft-ownership-v1` |

Commands actually executed:

```text
node scripts/ux_acceptance_20261013.mjs --commit 104bdf2e34b65f345d2b1ffb6a6014d1a7629b5c --output E:/CodexScratch/ux-independent-20261013 --label integration-104-ux-v1
node scripts/ux_acceptance_20261013_draft_ownership.mjs --commit 104bdf2e34b65f345d2b1ffb6a6014d1a7629b5c --label integration-104-draft-ownership-v1
node scripts/ux_acceptance_20261013.mjs --commit 53495231a06aac97c1092029fda35971bdbc08c5 --output E:/CodexScratch/ux-independent-20261013 --label integration-534-ux-v1
node scripts/ux_acceptance_20261013_draft_ownership.mjs --commit 53495231a06aac97c1092029fda35971bdbc08c5 --label integration-534-draft-ownership-v1
```

All raw JSON files above and the `integration-104-review.md` / `integration-534-review.md` receipts are retained under `E:/CodexScratch/ux-independent-20261013/`.

Git-object source SHA-256:

- App, both commits: `fac676e1edb78bfc5c4138c767f9a54aa4d7a38c54543d96f2ac834497ae0ad1`.
- Connections at 104: `c1ebff4b197138c93acd1cb4ce155888b3b272608aad5ff46660f4e9b06672ec`.
- Connections at 534: `00057bd40d0b7bda8eff04a563332cff8770db0c8fce561fb2c36a425e184d33`.
- HTML, both commits: `499d86507dc91a33e07c8b1cd803e82f2a0ed4d5b33118e775f04d46299d88d3`.

The Git-object app hash records LF-normalized bytes. Root separately recorded working app SHA `edf112e37347433dbc13b5e979af0bad27d0f7792ef05730acf178f36dcb863d`; it is not substituted for the immutable source actually tested.

Immutable104 resolved first Connections preload and the reconnect draft/pending findings, including newer-nonce/same-text live and saved draft ownership. Its remaining failure was explicit multi-source choice: replacing a single-select with two descriptor options selected the first descriptor automatically. Root and the Core owner received that reproducible finding. Immutable534 adds the empty source choice and explicit empty value when multiple sources have no valid previous choice. The exact frozen assertion that failed104 passes534. Single-source convenience, no consent/profile GET, preload dedup and late navigation/close/session/authority fences all pass without assertion changes.

The public Sources metadata/authority fence and current-stage/group-goal source boundaries also pass. Backend group saturation, runtime admission, service/API builds and owner/manual harnesses are separate Root/worker evidence; this independent review does not turn those into browser or model evaluation. Edit-away-then-back to identical original text remains the uncounted product-intent observation described above. No additional requirement or dirty-generation behavior is claimed to be implemented.

The final GREEN result is limited to Node VM functions, module behavior and minimal-DOM contracts. Native CUA, Chrome isolation, browser layout/focus/accessibility, actual Tauri interaction, phone, cloud, paid-provider and model acceptance remain open. No screenshot, profile read, inspection/execution grant, service, provider, deployment or push was performed by this reviewer. The source review was appended only after Root released its clean-check write slot; no tests, implementation files or Git operations were repeated for this documentation update.
