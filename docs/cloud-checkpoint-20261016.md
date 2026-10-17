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
