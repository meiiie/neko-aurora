# Local principal/device and Core discovery checkpoint

Tested application: `900d17d435ece36ad790eaf4e3d6cd101dbf0671`. Core requester adapter: `867966dd7f057dd1c03547014c81df93470d28e7`. Previous Main: `79a3c018656fc4bea61a06bcfac9488f9c3596ea`. Identity authority remains an explicit, default-off loopback fixture; ordinary local-owner startup remains available.

This revision binds accepted requests to a verified actor/device/session, preserves original nonce and receipt identity across same-actor renewal, fences late work after revocation, and scopes shared reads and browser caches. Manifest-v2 makes the real requester the primary sender while keeping the workspace owner separate. Historical owner receipts remain historical and do not gain new execution rights. The browser does not replay fixture intents automatically after reload.

Owner-only executable discovery now recognizes the installed, verified Core1.7.0 separately from a supported bundled Core. It does not read profile/configuration/keys, launch a discovered executable, or grant provider use. The installed1.7 copy was independently probed in an isolated home and refuses `--host-profile nekobot`; its ordinary ACP support does not satisfy Neko Bot's context/writer boundary. The connections panel shows detection before optional manual configuration, explains the unsupported use action, and progressively discloses endpoint/model fields. Optional OpenAI/Anthropic endpoint presets make no model or credential-readiness claims.

## Evidence

| Gate | Result | Source and limit |
| --- | --- | --- |
| Full Go tests/vet/build | PASS: 464 top-level + 479 subtests, 0 failure, 2 optional tests skipped | Clean application900d17d; DOCX reference/browser-seed opt-in tests skipped |
| Rust host fmt/tests/build | PASS: 28 tests | Clean6706f87; host Git tree and Core pin are byte-identical at900d17d |
| Core tests/typecheck/lint/compile | PASS: 78 tests, 1 skip | Clean867966d; 13 selected files, 1,168 assertions; not an entire Core-suite claim |
| Actual principal/device native e2e | PASS: 30 gates | Clean900d17d Go + exact Rust host/Core; loopback model fixture, retained genuine Core HANDLE stop |
| Actor/session browser | PASS: 37 gates | Exact900d17d assets in Chrome154; authored endpoint fixtures |
| Report seam checks | PASS: 6 gates | Exact extracted JS functions in isolated Node VM |
| Actual owner UI/discovery | PASS: 16 gates | Go900d17d embedded assets, installed executable metadata, legacy startup/outbox, presets and 390×260 form |
| Independent correction review | PASS: 12 JS checks + 3 Go tests/2 subtests | Three original P2 findings closed; finite review, not full-product acceptance |
| Tauri release build | PASS | Clean900d17d offline build; no new native-window UI acceptance claim |
| Protected inputs | PASS: 1,386 files, 17 ZIPs, 0 hash mismatch | Previous packets/binaries/media and raw user edits retained; user credential state was not inspected |

The release packet is `.checkpoints/local-principal-device-release`; its manifest binds each safe artifact and five revision-named binaries in `.build/local-principal-device-900d17d`. The launch manifest is explicit: no existing release manifest, installed Core, user state, key catalog, or old binary is replaced. The old live preview PIDs were absent at recheck; Root did not terminate or relaunch that window.

Failure history is retained. Native r1 failed because the test omitted the expected executable argument to the Core HANDLE checker; r2 stopped on an unclassified Windows connection reset, without a failed business assertion. Final r3 passed the full native flow, but does not establish a root cause for that one reset. Owner UI r1/r2 did not pass Chrome startup under an over-restricted Windows environment; r3 used the Windows browser variables with its own profile and sanitized provider environment. The three independent P2 negatives, browser preparation negatives, legacy-nonce negative, and host requester negative remain available rather than being relabeled as successes.

## Remaining gates

Real profile reuse is still a code boundary: add Core-owned public profile metadata and a unified credential-source/task-state-home resolver, then bind a selected profile to actor/device/runtime and task consent. Existing keys need not be copied or re-entered. No provider/account readiness was checked, no existing credentials consumed, no grants/OAuth/model calls, deployments or public pushes occurred.

Production human login/device pairing, remote/cloud sync, genuine model quality, physical phone/keyboard/safe-area/AT speech, and user review of the new connection controls remain unaccepted. Tauri build is not a new GUI acceptance. Formal Go `-race` was not run with this offline CGO-disabled toolchain. The fixture authority and synthetic model receipts are not evidence for those production gates.
