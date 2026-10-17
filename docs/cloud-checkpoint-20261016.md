# Cloud checkpoint — 2026-10-16

Development checkpoint on `wip/cloud-continuation`, based on main
`bc3def7007adf78b7c44549239b25d655a8e3d05`. This is not a release or full product
acceptance. The following results apply to the source changes in this checkpoint
on Linux x86-64, not to historical screenshots or Windows binaries.

## Changes with practical outcomes

- Recovered source from the public repository and verified remote checkpoints.
- Kept the saved-account witness default-off while repairing canonical binding
  vectors, endpoint parity and expiry checks, including a stalled wall clock.
- Replaced unsafe Core bridge casts with typed JSON boundaries and verified that
  credentials remain filtered from streaming, final responses and tool callbacks.
- Correctly scoped Windows backup tests. Linux now explicitly checks unsupported
  protection fails closed without modifying source files.
- Isolated UI tests from the developer's real home and update service. Fixture
  homes disable update checks; a fetch/preconnect guard reports unexpected remote
  calls and refuses automatic redirect following. This is not an OS network sandbox.
- Preserved safety assertions when testing archived upstream workflows and a Core
  checkout nested in the Neko Bot source snapshot. No upstream workflows activated.

## Recorded checks

| Check | Result | Scope |
|---|---|---|
| Rust `cargo test --locked --manifest-path app/host/Cargo.toml` | PASS, 36 tests | Rust 1.99.1, portable host tests; saved-key Windows suite has zero runnable tests here |
| Go `go test -count=1 ./...` and `go vet ./...` in `app/native` | PASS | Go 1.27.2; final rerun includes monotonic witness expiry fix |
| Go control-plane build | PASS | Linux development binary; no installer |
| Witness race checks and independent delayed/frozen-clock regressions | PASS | Local synthetic authority only |
| Go Windows test cross-compilation | PASS | Compilation only, not native execution |
| Core `bun run typecheck` and `bun run lint` | PASS | Bun 1.4.0, frozen lockfile |
| Core `bun test` | 2,160 PASS, 58 SKIP, 0 FAIL | 2,218 tests / 222 files, 27,551 assertions; optional/native/prebuilt gates remain skipped |
| Z.AI config and provider redirect regressions | 89 PASS | Local fixtures, not authenticated GLM inference |
| Frontend JavaScript syntax checks | PASS | Not rendered UI or physical-phone acceptance |
| Documentation links and whitespace | PASS | No product-runtime claim |

The three optional Go fixtures absent here are a local DOCX reference, an isolated
browser report seed and an owned historical journal. Core skips include native
Windows SRT/migration/helper cases, prebuilt artifacts and opt-in evaluations.
Skipped tests are not successes and are not removed from the acceptance plan.

The initial full Core attempt exposed unwritable default-home state and unintended
GitHub update discovery; it was stopped by the environment. A guarded intermediate
run exposed additional fixture-home and imported-repository assumptions. The final
run above completed after correcting those fixtures; production updater behavior
and Git-root verification were not relaxed.

## Remaining product work

Saved-account consumption/bootstrap is still unwired. Exclusive witness ownership,
retirement/restart behavior and Windows file identity/DPAPI need integration and
native evidence. See the [witness contract](../app/docs/saved-account-witness-contract.md).
No real credentials or paid provider calls were used. Network reachability to Z.AI
does not prove account/model access. Cloud sync, actual mobile acceptance, packaging
and signing remain open in the [roadmap](ROADMAP.md).

Do not rerun this entire baseline without a source change or a new risk. Subsequent
slices should prove their user-visible outcome with the smallest relevant regression
and integration checks, then save and verify a source checkpoint.

## Subsequent saved-account slices

- `d6f45ef382813f633778207fd94a3245f028aaf9`: Core read-only witness consumption,
  including nonblocking FIFO substitution rejection. Focused checks: 22 passed,
  one Windows-only skip; Core typecheck/lint passed.
- `f770240d5dd3cca95111ecb761ff951de49399b5`: bounded private
  `saved-account prepare --json --broker-stdin` entry point, with a public receipt
  and no provider call. Final CLI checks: seven passed, one Windows-only skip;
  adjacent checks: 43 passed, two skipped; typecheck/lint passed.
- `8748743844f82b919c6c1c0c74453464a7ba840a`: optional loopback terminal helper.
  Its presence is not evidence of a working owner handoff, authenticated provider,
  encrypted vault or permanent secret storage. No key was entered in this cloud run.
- `e60e8cf8ecb33a9243b32558bb3623f20ecfccf8`: policy-bound saved-account ledger
  admission. The original Send request supplies the consent identity; no extra
  user approval dialog is introduced. Account revision, context and executor
  authority are checked live; fixture leases cannot be promoted. The ledger
  package passed on Linux, and the combined diff received independent review.
  This commit was local-only when recorded; publication was interrupted.

The last slice is deliberately not an enabled execution path. The existing v1
request/step/time bounds describe its narrow development contract, not the final
long-running assistant experience. Runtime lifecycle, trusted-host launch and
native Windows consumption must be connected before claiming saved-account
execution. Real GLM inference remains untested; the owner postponed key entry.

### Runtime and native-reader continuation

`c0de4f7` adds runtime-owned witness lifetime and rejects saved-account attempts
before fixture dispatch. A review reproduced two owners from simultaneous starts;
an atomic retained owner-directory reservation now rejects the loser without
retiring another session. This is cooperative same-path ownership, not protection
against an adversarial filesystem owner. Runtime package race checks and vet passed
on Go 1.27.2/Linux; the concurrent-start regression passed ten race-enabled runs.
The UI now explains this unavailable runtime path without asking the owner to
re-enter their key (six source-function checks, not rendered-device acceptance).

The subsequent Rust/Core native-reader slice uses one private helper session, not
one process per polling interval. Rust bounds frames and checks Windows file and
ancestor identity; Core verifies the original signed bytes and retains proof TTLs.
Observation freshness is explicitly bounded to 100 ms from request send, sampled
at 50 ms; stale/failed transport permanently closes authority. This changes native
observation to bounded sampling, not an instantaneous filesystem revocation claim.

Rust Linux protocol/command checks passed (six plus two); Windows-target test code
compiled but was not executed. Core focused witness/bridge/prepare checks passed
(34 passed, two Windows-only skips), plus typecheck/lint. Independent source review
found no blocker within the stated trusted-launcher assumptions. Helper checksum
verification is a preflight pathname check, not atomic executable identity.
Protected installation and actual Windows tests are activation prerequisites.
The adapter remains absent from CLI/ACP/UI activation; no provider was called.

### Retained Windows executable guards

The next host slice retains read-sharing-only handles for the canonical
host/Core/writer executable leaves and their ancestors. Hashes come from the held
leaf handles; guards outlive child teardown. This addresses the launcher's prior
hash-to-spawn replacement window without changing ACLs or adding a configuration
switch. POSIX retains its prior preflight semantics. The separate WSL supervisor
script check is unchanged; direct native-reader factory callers are not protected
unless they use this guarded host lifecycle.

Portable guard tests (two) and existing executor tests (four) passed with Rust
1.99.1, offline/locked dependencies. Windows test targets compiled. Three native
Windows sharing/rename/junction/process tests and WSL/direct runtime acceptance
were NOT RUN. Independent source review found no blocker; native verification
remains required before saved-account activation.

### Private saved-account host envelope

The host now has a closed-schema preparation/envelope seam that reuses the existing
saved-key broker callback and retains protected Core/host images. Catalog storage
must be canonically disjoint from the task root; capsule and witness paths cannot
enter the task-visible tree. The ACP envelope excludes the provider key, secret
reference and catalog path, but contains the revision key: it remains private
secret-bearing material and must never enter transcripts or public diagnostics.

Six portable metadata/layout regressions passed; Windows test code compiled,
including a synthetic DPAPI preparation case that has NOT RUN. Independent review
caught and verified the catalog-root separation fix. This slice does not implement
or claim a bounded native subprocess pump. Pipe cancellation, descendant-held
handles and teardown still require a verified transport before launcher activation.

### Internal ACP startup integration

The Core adapter now reuses the existing ACP agent loop with private bootstrap
framing, one canonical root/session/primary prompt and the complete context-manifest
digest. Coalesced ACP bytes after the initial line are preserved. Revocation waits
for in-flight session construction and closes late runtimes; a review regression
also covers whitespace-prefixed command denial. The existing public CLI still
rejects native metadata and does not dispatch this staged adapter.

On final code, `bun test test/acp*.test.ts test/saved-account*.test.ts` passed
136 tests with two Windows-only skips across 16 files; typecheck/lint, documentation,
whitespace and redacted candidate secret checks passed. Independent source review
cleared both reported fixes. This is internal-flow verification, not provider or
Windows acceptance. Host durable admitted/prompt-ready acknowledgments, production
private-pipe transport and native gates remain necessary for product activation.

### Job-contained private broker transport

The internal Windows broker transport now launches the pinned Core image suspended,
whitelists inherited stdio handles and assigns a private kill-on-close Job before
resuming it or sending input. Owned scoped workers bound private input and receipt
output. Failure revokes authority before cleanup; cleanup waits for process exit,
zero active job processes and worker completion before releasing guarded images.
The two-second limit rejects late acceptance; it is not a promise of a hard OS
teardown deadline. Embedding callbacks must be trusted, nonblocking and nonpanicking.
The single trusted launch contract excludes unrelated concurrent inherit-all spawns
inside the same host process. No new dependency or public activation was added.

Eight portable saved-account host tests passed; Windows test targets compiled.
Five new native process/pipe/cancellation scenarios were NOT RUN. Two independent
source reviews cleared fixes for early-failure revocation ordering and final
acceptance rechecks. Native lifecycle/handle inheritance evidence is still needed.

### Typed Go-to-Rust launch assembly

The next paired slice reuses the current catalog selection and authoritative ledger
Work to assemble a bounded private envelope. It keeps the opaque reference and
per-attempt revision key out of public serialization, rechecks account/lease
currency around a single-use transport, and retires authority on cancellation or
failure. Rust validates the typed envelope against the trusted actual executor
descriptor, full manifest digest, root/layout and task identity before composing
the existing protected preparation path. Account and executor fences remain
separate values; no new account store or picker was introduced.

Affected Go connections/runtime race tests and vet passed. Five Rust parser tests
consume the actual Go-generated synthetic 4499-byte fixture, including different
account/executor fences; eight saved-account library tests and Windows test-target
compilation passed. Independent paired source review and candidate secret scans
passed. Manager.Run/main.go/public Core CLI selection remain unchanged: this proves
internal assembly, not native end-to-end activation or real provider operation.

### Cloud UI interruption fixes — 2026-10-17

These checks ran on Linux with Node v24.19.0 against the actual UI handlers in
isolated VMs. DOM, network and receipt boundaries are synthetic; they are not
browser, physical-phone, provider or Windows acceptance.

- `c94fc83559dbc129e69d8015c790b6ef10e3c87a`: create-bot command/view ownership
  preserves edited drafts, retry nonces and newer navigation under delayed replies.
  `node app/scripts/ui_bot_creation_races.mjs`: 10 PASS; the prior source had
  six failing schedules. Both UI asset syntax checks and `git diff --check` passed.
- `33a5aed24a907dd5a20cc9fead8eb0373505ed9c`: discovery loading belongs to its
  request rather than account-editor navigation; old responses cannot clear or
  overwrite newer discovery. `node app/scripts/ui_discovery_loading_races.mjs`:
  9 PASS. Existing saved-account status checks: 6 PASS; create-bot regression
  remained 10 PASS.
- `1a8b250fbf450e056d763692ae6df4ecbcbc85af`: reselected upload preparation
  rechecks session, dialog generation and original command after receipt lookup.
  Closing/reopening or choosing another file cannot dispatch the stale retry.
  `node app/scripts/ui_upload_retry_ownership.mjs`: 27 PASS; unchanged source
  had 20 FAIL and 7 PASS. Asset syntax and whitespace checks passed.

The attempted Chromium interaction launch was BLOCKED by socket permissions,
including escalation; it was not retried through another route. The existing
`ui_durable_upload_regression.mjs --journal-boundary true` harness failed all
19 cases with `window is not defined` during VM setup, identically on the unchanged
source and upload-fix delta. That initial gate was recorded as failed; the fixture-only correction below
subsequently restored it without weakening assertions.

Candidate secret scans passed. History scanning identified one reviewed synthetic
fixture opaque catalog locator in `saved-account-launch-v1.json`; its exact
fingerprint was excluded for the subsequent scan, with no broad rule suppression.
These local commits do not prove GitHub publication. The private backup patch is
incremental from `8748743844f82b919c6c1c0c74453464a7ba840a`, not an installer.

- `d9776f154a6443ed6da085fe0e566fae9d0fa449`: confirmed group creation returns
  independently of subsequent snapshot refresh. Refresh failure uses the existing
  reconnect path rather than making an acknowledged group retryable as a new one.
  `node app/scripts/ui_group_confirmed_refresh.mjs`: 7 PASS; previous source had
  five failing schedules. Checks include failed/hanging refresh, duplicate Send,
  original retry nonce and newer-draft/navigation ownership. The older tag harness
  had 14 baseline fixture setup failures (`sessionState`/`window` missing); its
  failures were not attributed to this one-expression product fix.

- `9c501a3c6d21607351de559d05063ab63c0354e6`: restored missing VM setup in
  the two existing gates, using extracted production session/cache/validation
  helpers, a local-owner fixture and minimal window/render boundaries. No product
  logic or behavioral assertions changed. On final source:
  - `node app/scripts/ui_durable_upload_regression.mjs --journal-boundary true --output /tmp/neko-final-journal.json`: 19 PASS, 0 FAIL.
  - `node app/scripts/ui_tag_collaboration_local.mjs`: 23 PASS, 0 FAIL.
  - Both harness syntax checks and `git diff --check`: PASS.
  This resolves the earlier missing-global failures for these VM gates only;
  rendered browser/native/provider acceptance remains unverified.

- `75d39a732d0e86483adf8602cace4b0943fa732c`: confirmed task controls no longer
  become orphaned retries when optional refresh fails or hangs. Storage-retirement
  failure retains its blocked reconciliation warning. On Linux / Node v24.19.0,
  `node app/scripts/ui_task_confirmed_refresh.mjs` passed 13 scenarios (all six
  actions, invalid acknowledgements, nonce/duplicate handling and dialog ownership).
  The portable journal-boundary gate passed 19 scenarios; neighboring group/upload
  VM regressions passed 7/27 respectively. Native host/provider/browser gates were
  not run. Official Go 1.27.2 was restored from the vendor archive and verified
  against its published SHA-256; that alone is not a new Go acceptance result.
