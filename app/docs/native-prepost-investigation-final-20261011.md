# Native pre-POST investigation: final bounded receipt (2026-10-11)

The bounded investigation is finished; no product patch is proposed. Three actual original-order trials used the historical Go/Host/Core/desktop binaries. Trial 3 proves an asynchronous viewport setup race before the original visible-hit assertion. The exact historical ba1 first-run post-hit 30-second timeout was not reproduced, and the original service179 driver is still missing. Later successful POSTs do not close either historical attribution.

## Source and preserved ordering

Owned source: `E:/CodexScratch/native-prepost-investigation-84ebe74`, branch `analysis-native-prepost-84ebe74`. Application snapshot is immutable Root `84ebe7425a42b3cf1a18b9c4977592e61b7197be`; the borrowed Go was tested by Root at `ba1c23869917e8106a0abd6049992841ee3f6da2`, with empty native/host/desktop/contracts diff to84. Capture analysis commits are `eb93d14de667915d6bca8ead6bdd53a5993546c2` (r1) and `e0fa8fb5dd5bfe5c505b7a87faef720dca5efa49` (r2/r3). Product files were never edited. This final document is additive; prior packets/results remain immutable.

The original group runner was recovered byte-exact from ba1 `scripts/native_group_recovery_ui.mjs`: SHA256 `05ee0b86562e921bc7314585a980d89b4714d8ab283dc6bcebad150f77299794`. The clearly labelled derivative is SHA256 `4c771ac2120178f6ac45376bc323bdd4fe064defb11530aeecf1185744ea8073`. All16 complete original assertions, 30-second polling, Emulation/data wait/modal/hit/screenshot/focus/keyboard Enter order remain unchanged. There is no added settling, click fallback, timeout extension or failure suppression. Passive instrumentation adds overhead and can affect scheduling; it is not an exact uninstrumented historical rerun.

The initial capture module SHA256 `65be33d21dafcea008c63439dfc2975ee856f81b434c07f6ba2c01efcec77427` remains beside the old derivative. r2/r3 companion SHA256 `6a60501a55edb8be58188f8fac2d06bb01954b04cd0d202fff6156afa4e3debb` adds same-object matchMedia observation at creation. When mobile MQL already existed before installation (r3), the diagnostic observer was registered after the app listener; its media-change record is emitted after the app callback. Function entry/return and real focus/resize events establish the intervening ordered mutations. No callback stack was captured.

## Actual native trials (all overall FAIL, retained)

Each command used `C:/Python313/python.exe .build/run-original-native-prepost.py <analysisCommit> <captureDriver> -rN`, cwd the owned E source, with its own E state/profile and loopback synthetic streaming provider. Each outer wrapper passed genuine retained Core HANDLE wait, exact generation/outbox receipt, and restart without automatic group resume, then retained the original driver's failure. Exact command/commit and binary hashes are in each result JSON.

| Trial | Original native checks | Observed ordering | Original failure |
| --- | --- | --- | --- |
| r1 / eb93 | 9 PASS,1 FAIL | app/MQL initialized after Emulation; trusted Enter clicked group-recover, map created before retention, durable record then POST1; all3 actual stages completed | later `same_original_group_goal_pins_deadline` |
| r2 / e0 | 9 PASS,1 FAIL | mobile MQL created at136.9ms already matching390px; modal opened324.6ms; trusted Enter525ms clicked group-recover525.6ms, map then durable retention/POST1; all3 actual stages completed | same later canonical assertion |
| r3 / e0 | 2 PASS,1 FAIL | app/MQL already initialized at1220px; modal open226.5ms, Close focused234.2ms; real resize235.9ms; setWorkPanel(false)237.7ms, work-open focused239.0ms; trusted media-change239.3ms; helper still allows original recovery | `native_mobile_group_recover_44px_visible_hit`, before Enter/POST |

r3 has zero keyDown, zero recoverGroup entry, zero command/record, zero POST; original group remains suspended with one original stage. Immutable app.js:147 mobile change callback invokes `closeNavigation();setWorkPanel(false)`. The actual captured resize/callback closes a just-opened modal before the hit check. This is a real original-driver viewport setup race, consistent with Root's earlier pre-hit r2 signature. It does not prove that the historical ba1-r1 callback ran after its already-passed hit/screenshot. There was no native post-hit Enter-to-work-open mismatch in these three trials.

r1/r2's later canonical failure is separately reproduced by original functions: groupRecoveryIdentity includes explicit undefined keys, CDP returnByValue drops them, and canonicalJSON of the live object differs from the roundtripped object. Original canonical38 evidence is retained in `E:/CodexScratch/ui-group-recovery-b655f79/.build/keyboard-investigation-ba1c/`. The original failed assertion was intentionally not corrected here; later ACK/reload assertions were not reached and cannot be claimed PASS from these trials.

## Hypothesis evidence versus historical attribution

Pure original-function tests show recoverGroup inserts the command map synchronously before awaiting durable retention, and a retention failure keeps that map. The historical empty map is not positive evidence of a journal failure; the old trace lacks handler-entry/guard/key/callback measurements, so those alternatives cannot be resolved retrospectively.

A separately labelled synthetic headless replay invokes the exact original mobile callback after a successful hit: it closes the modal, hidden recovery focus is ignored, and real CDP Enter clicks work-open, reopens the modal and leaves Close focused with map0/record0/POST0. All26 synthetic replay checks PASS. This discriminates a possible mechanism matching the old terminal signature; it is not an actual historical native cause. No synthetic event is labelled trusted native media evidence.

Instrument checks: original-function pureVM17 PASS; capture headless29 PASS; synthetic late-media26 PASS; e0 pureVM17 rechecked after native trials; syntax and diff checks PASS. The headless checks use real borrowed Go/embedded assets but synthetic typed stop/group authority and a before-server503 fixture, separate unchanged DPAPI sealer. They are not native process/Core/model/phone acceptance. Existing13ea110/service61/durable51 and canonical38 histories are untouched.

## Service179 original-source blocker and search coverage

Wanted original SHA256 `70d7f97b93b7323a8d866fe0f52067e346343f03adf49ba64ff07b24e805ac73` was not recovered. Bounded coverage:138 MJS <=512KiB under Root `.build/.checkpoints` and primary checkpoint/product output routes;50 additional bounded historical text artifacts at the exact ui-service-recovery investigation packet, native-service179[-r2], and product-service651[-source] routes; no tracked ignored-driver Git history; two manifest/hash/head-verified source bundles (179 and651) inspected via owned Git objects. Source bundles track related UI fixtures, not the ignored original native driver. Exact routes, limits and hashes are in `.build/provenance/*search.json`.

Current service runner SHA `f4e0866...3ba2e8` and minimal catch-only reconstruction SHA `5413605...5ec4d7c` do not match wanted70d and are explicitly derivatives, never relabelled original. Original179/ba1 error reports, diagnostics and launcher outputs were copied byte-exact; original journal bytes remain untouched in place with hashes. No raw session logs or browser profiles/cookies were searched/copied. Missing original179 source plus missing historical event trace is a specific evidence blocker;179-r2 PASS is not rootcause closure.

## Cleanup and deliverables

Native slot is released. All9 native PID/creation-FILETIME identities found in the three journals/outboxes are closed (OpenProcess ERROR_INVALID_PARAMETER); no exact owned Go/host/desktop/profile WebView2 process remains; no8765 LISTEN socket remains. Retained Core handles were closed and owned Go processes killed/waited in wrapper finally. No foreign process, production state or primary archive was modified. Profiles/journals/failure artifacts remain as owned evidence.

Final evidence is under `.build/final-e0fa8fb/`: `trial-summary.json`, `process-closure.json`, `prior-packet-verification.json`, capture-unit confirmation and final manifest. Complete traces/results and journals remain in `.build/native/native-group-original-order-{eb93d14-r1,e0fa8fb-r2,e0fa8fb-r3}`. Prior eb93 packet verifies23/23 artifacts unchanged. The final manifest binds source, commands, environment, binary hashes, failed assertions and unchanged original drivers. No product fix, historical-timeout resolution, real-provider quality, cloud or physical-device acceptance is claimed.
