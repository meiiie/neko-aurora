# Core credential-source UI, 2026-10-12

UI baseline: application `1d02a7c042be6266ab71593c41a5915ac234b83c`, isolated Root branch `core-auth-bridge-1d02a7c`. Root is the sole Git/index/checkpoint writer. This UI leaf changes only `connections.js`, the existing profile support note in `index.html`, this document and the new authored fixture driver `scripts/ui_core_auth_bridge_local.mjs`. The approved light palette, three-step journey, scope controls, explicit inspection/execution actions, original nonce recovery and navigation/focus guards remain in place.

## Public metadata and eligibility

Legacy `pr1_` metadata is readable only when all four new credential fields are absent. New `pr2_` metadata uses closed `credentialSource` and `authPolicy` values. Snapshot IDs use `cs1_` plus 32 lowercase hexadecimal characters; expiry is a positive safe integer in Unix milliseconds. These fields describe authority and policy, not a credential value or successful login.

Supported source/policy combinations are file API key/static key, local no-key/static key, process environment/static key with a bounded snapshot and expiry, and cached Kimi OAuth/cached-token-no-refresh with a future expiry. A cached Kimi profile may omit the snapshot ID when no broker is involved. Other OAuth sources remain unavailable/unsupported. Unsupported profiles remain descriptive entries; they cannot create execution consent or silently switch provider/source.

Metadata reports accept the exact v1/v2 envelope. v2 source snapshots contain only ID, creation time and expiry, bounded to 300 seconds. Report capture must occur within that window. Every profile snapshot ID must match the envelope and cannot expire after it. A v2 catalog cannot disguise new policy fields as pr1 metadata. v1 catalogs continue to pass the unchanged legacy flow.

Environment eligibility depends on a source snapshot supplied by the configured native host broker. The browser cannot select environment variable names, home directories, credential files or key values. The support note distinguishes a catalog with a supported environment source from one without a ready broker source. The selected profile explains its source/policy and expiry. Cached Kimi copy says `Kimi token đã lưu · không tự gia hạn`; no login or refresh button is added. Configuration and source metadata never establish credential readiness.

## Consent lifetime and immutable intent

Execution consent expires at `min(createdAt + requestedTTL, profileCredentialExpiry)` when a credential expiry is present. ACK/receipt validation requires that exact deadline, rejecting both an overlong grant and an arbitrary shorter response. Validation of an original expired receipt uses the original creation time, so the record remains readable without becoming executable again.

The future request form retains its human-selected TTL. The active budget reads the **original request's TTL**, and separately displays the server's actual clipped expiry. It does not derive a new requested lifetime from a fractional credential deadline. Current execution eligibility checks expiry again at approval time. Existing grants retain their original consent ID/revision/profile revision and expiry; reading replacement source snapshots does not activate, refresh or renew them. A new execution grant still requires the existing explicit approval action.

The input/nonce DTO and message selection format are unchanged. Safe pending operation records retain `pr1_` or `pr2_` revision pins and never persist credential values, OAuth cache contents or environment names. Uncertain ACK recovery reads the original receipt only. A late cached-token consent ACK cannot select a new destination or clear its draft.

## Validation evidence and limits

Commands ran in the isolated Root checkout using Node `v25.9.0`, Windows and headless Chrome `154.0.8037.98`. Output/profile caches are isolated E scratch directories. The new driver's receipt records baseline commit, exact asset hashes, harness hash, command, environment, named PASS/FAIL results and scope. Root's final checkpoint will identify the integrated Go/Core/UI source; these dirty-checkout UI receipts do not claim that later immutable identity.

```
node --check scripts/ui_core_auth_bridge_local.mjs
node --check native/internal/web/assets/connections.js
node scripts/ui_core_auth_bridge_local.mjs --output E:/CodexScratch/core-auth-bridge-20261012/ui/green-r4
node scripts/ui_core_profile_bridge_local.mjs --output E:/CodexScratch/core-profile-bridge-20261012/ui/auth-legacy-58-regression-r1
node scripts/ui_core_profile_steps_local.mjs --output E:/CodexScratch/core-profile-bridge-20261012/ui/auth-legacy-58-steps-regression-r1
```

New auth gates: **39 PASS / 0 FAIL**. Unchanged legacy bridge gates: **50 PASS / 0 FAIL**. Unchanged three-step gates: **58 PASS / 0 FAIL**. The latter is the requested original 58-case suite; neither old driver nor its assertions was edited. Existing historical receipts were not overwritten.

RED/setup evidence is retained separately. The initial v2/policy/clipped-expiry run recorded 14 failed assertions and rejected the catalog. An initial driver syntax error was corrected before that run. A fixture snapshot window accidentally spanned 301 seconds; the UI correctly rejected it, and the fixture was corrected to the contract's 300-second bound. A later support-copy assertion used lowercase `broker` while the UI displayed `Broker`; its failure is preserved and only that case-sensitive harness expectation was corrected.

These checks exercise actual UI functions and rendered UI against authored public metadata and HTTP fixtures. They do not inspect a real environment key, OAuth cache, user Core configuration or provider; they do not launch actual Core, verify model quality, prove identity, test a phone/native window or mutate the live application/state/port 8765. Root owns compiled Go/Core fixture interoperability and final integrated checkpointing. No Git mutation, actual credential access, provider request, login, OAuth refresh, production grant or external upload occurred in this UI leaf.

## Explicit snapshot replacement follow-up

Baseline `0905cf3a976aebcd533997f237588c295a3b4c43`. One additional fixture approves environment snapshot A, then explicitly reads fresh metadata for snapshot B with the same source/profile ID and a different pr2 revision/snapshot. The original A input, nonce and consent remain unchanged. No execution grant, message POST or model call is created by the replacement read.

On a valid same-source catalog read, the UI compares the chosen profile's revision, credential source/policy, snapshot ID and expiry, plus the source revision/runtime hash. A changed or removed tuple synchronously marks the chosen selection and its old execution record inactive. The original chosen object and pending/history record are retained; B is only a future form choice requiring explicit execution approval. Root separately verifies current profile authority for receipt activity, so a stale original snapshot cannot reactivate merely through receipt resync.

The new case recorded **39 PASS / 1 FAIL** before the fix, then **40 PASS / 0 FAIL** after the fix. Receipts: `E:/CodexScratch/core-auth-bridge-20261012/ui/snapshot-red-r1/result.json` and `snapshot-green-r1/result.json`. The other 39 assertions were unchanged. The old 58-case runner was not rerun for this narrow fix; its earlier passing receipt remains historical evidence. Root retains sole Git/checkpoint ownership.
