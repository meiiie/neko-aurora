# Core source onboarding convenience, 2026-10-13

Opening Connections now reads the existing safe Core source descriptors for an eligible local owner and reveals the existing source step when a valid nonempty catalog arrives. A sole source is selected by the existing select control. Multiple sources require an explicit choice when no valid previous source remains; a still-present previous selection is retained. This removes the extra initial "Dùng profile có sẵn" click.

Eligibility remains `local_core_profiles_v1`, local owner, non-fixture session and writable state. The installed Core, its profiles and the bundled runtime are unchanged. Discovery copy still describes an unconnected installation; descriptor availability does not establish profile configuration, credential readiness or provider usability.

## Authority and asynchronous behavior

The automatic path uses only `GET /v1/core-profile-sources` plus read-only reconciliation of existing pending operation receipts. Original ordering remains durable-record load, source descriptor GET, then receipt GETs. Receipt reconciliation uses `allowActivate=false`. It cannot select an execution consent or replay a pending command.

Metadata inspection remains the explicit “Cho phép xem profile” action. Execution remains a separate explicit approval. Opening Connections never calls `/v1/core-profiles`, posts inspection/execution consent, reads credentials or calls a provider. The existing `not_checked` profile copy and manual configuration path remain in place.

Concurrent opening/explicit-button reads in the same session and navigation reuse one pending discovery. Re-rendering does not start a source GET. Closing the dialog or profile panel, switching to plugins, session rotation and changed navigation prevent a late response from revealing the panel. Current owner, capability and read-only checks also precede applying descriptors. An old request cannot clear a newer pending discovery. Empty, malformed or failed catalogs leave the initial panel collapsed and the explicit action available for another read.

## Source identity and validation

This uncommitted slice starts from application `195a0ec990db0478d9f052bc0e79a83b12dc9d78`. Root owns the eventual checkpoint and integration. Owned files are `native/internal/web/assets/connections.js`, `scripts/ui_core_source_onboarding_local.mjs` and this note. No app/index/style, backend, Core, Git index or live state is changed by this worker.

The baseline Connections SHA256 is `1c2e341c0d17ba6e8b83ae12b12716d755816ea6748238ea55d9ed081bd46770`. The initial changed Connections SHA256 is `c1ebff4b197138c93acd1cb4ce155888b3b272608aad5ff46660f4e9b06672ec`. Initial harness SHA256 is `4a13ffb7bbc93884796207fc4317ddb7e363faad276bf74449022c9c2e59b383`. The harness also records the exact app-helper hash and working-tree flag rather than presenting modified source as a clean checkpoint. The explicit multiple-source correction below has its own identities and evidence.

Environment: Windows `win32`, Node `v25.9.0`, selected checkout `C:/Users/Admin/Documents/Codex/2026-10-11/task/checkpoint-verify-20261011`. Evidence is confined to the owned `E:/CodexScratch/core-source-onboarding-20261013/` directory.

| Command | Result | Scope |
|---|---|---|
| `node scripts/ui_core_source_onboarding_local.mjs --output E:/CodexScratch/core-source-onboarding-20261013/red-final` | Expected RED: 9 FAIL / 12 PASS | Actual UI closure/validators, baseline Connections source; missing automatic read/reveal |
| `node scripts/ui_core_source_onboarding_local.mjs --output E:/CodexScratch/core-source-onboarding-20261013/green-initial` | PASS: 21 checks | Actual UI closure/validators with minimal DOM and authored safe source/receipt GETs |
| `node --check native/internal/web/assets/connections.js` | PASS | JavaScript syntax |
| `node --check scripts/ui_core_source_onboarding_local.mjs` | PASS | Harness syntax |
| `node scripts/ui_connections_secure_local.mjs` | PASS: 13 checks | Existing manual-account Node VM boundary |

Checks cover automatic safe discovery, sole and multiple choices, no implicit profile GET/POST, request coalescing, invalid/empty/failed catalogs, closed/session/navigation/actor/read-only/capability fences, fresh reopening, and pending receipt order without activation/replay.

Browser and native startup gates are blocked in the parent environment and were not retried or worked around. This worker ran no browser, Go service, Core process, broker, profile catalog inspection, provider or consent mutation. Node VM probes do not establish browser layout/focus, native WebView2, phone acceptance, real credential usability, bridge interoperability or model behavior. Root must bind any further validation to its exact integrated checkpoint.

## Explicit choice with multiple sources

Independent immutable review of `104bdf2e34b65f345d2b1ffb6a6014d1a7629b5c` produced 24 PASS / 1 FAIL in the frozen 25-check suite. The failure was `connections_first_multi_source_keeps_choice_explicit`: an initially empty native select chose the first descriptor when two sources arrived. The initial owned suite checked a subsequent choice change but did not assert the blank initial selection. That earlier 21-check PASS remains historical and does not establish the explicit-choice requirement.

The correction adds a disabled `Chọn nguồn Core` option with empty value only when there are multiple sources and no valid previous choice. After replacing options, it explicitly sets the value to empty; inspection stays disabled until a source is selected. This explicit assignment matters because a native single-select otherwise defaults to its first enabled option. A sole source stays selected automatically. A valid previous choice is restored, and a removed previous choice returns an ambiguous catalog to the placeholder. No new endpoint or authority operation is added.

Current Connections SHA256 is `00057bd40d0b7bda8eff04a563332cff8770db0c8fce561fb2c36a425e184d33`; current owned harness SHA256 is `0473e6cdd518628cb7c56e577f3fc169ee1cc496c9b4d1e19b7b8ef05b830d1e`. The harness now accepts an exact `--source-commit` to read immutable Git objects without altering the checkout. Its minimal select adapter defaults to the first enabled option and permits an explicit assignment to the disabled empty placeholder.

| Command | Result | Source scope |
|---|---|---|
| `node scripts/ui_core_source_onboarding_local.mjs --source-commit 104bdf2e34b65f345d2b1ffb6a6014d1a7629b5c --output E:/CodexScratch/core-source-onboarding-20261013/multiple-choice-frozen-red-104` | Expected RED: 22 PASS / 2 FAIL | Immutable104; initial ambiguous and removed-choice-to-ambiguous cases fail |
| `node scripts/ui_core_source_onboarding_local.mjs --output E:/CodexScratch/core-source-onboarding-20261013/multiple-choice-final-green-104` | PASS: 24 checks | Modified working Connections source, exact hash above; same strengthened harness |

The frozen independent 25-check harness is unchanged. Root owns its post-checkpoint retest and any fresh backend checks. This correction is still Node VM evidence, with the same browser/native and authority limits described above.
