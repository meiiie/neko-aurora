# Bounded Core auth bridge follow-up

Owner: Root integration; Core worker owns canonical resolver/broker/provider policy; backend worker owns the existing profile consent/pin ledger. UI stays in the existing Connections flow. No new model loop, provider daemon, external framework, account login or cloud work is part of this slice.

Baseline: app `1d02a7c042be6266ab71593c41a5915ac234b83c`, Core `0aa0771ba47e6803018297a01d51427c9e61283f`; Wiii restored public delivery `d1880b234a3e037a6946274a6fe60b678fa64b89`, attributed production source `2ab2eabee5a1602f2efd1fa177ee3ffa40ce3bd0`. Wiii's installed Core lock `16860b0`/1.7 and its four-field discovery parser do not satisfy the Neko Aurora credential-revision contract. Its native process containment and distinct owner transport consent are reusable concepts, not provider authorization.

The parent requested actual local auth code and fixture evidence, while preserving live8765, candidate8767, archives, installed binaries, real credentials and prior edits. Technical choices are delegated; this design applies that authority without requesting an additional login or grant. No actual user environment/profile/key is read during development.

## Env source

An explicitly configured native source enables `environmentBroker`. Discovery still reads no credential environment. After inspection consent, a pre-agent Core broker command inherits the source process environment, reads only names configured by Core profiles, and accepts at most 16KiB private stdin describing source ID, trusted credential/state homes, capsule path, revision key and bounded expiry. It starts no tools, provider, login or updater.

Core writes an authenticated AES-GCM capsule and a current snapshot marker in private state; keys never appear in Go/Rust DTOs, task environment, model context, browser storage, public logs or receipts. Native processes carry only the opaque path, keyed revision, snapshot ID, deadline and already existing scope. Task Core decrypts internally and rechecks current capsule/marker before fetch and output/effects. Ciphertext and marker aliases, tampering, expired snapshots and mismatched identity deny.

Only explicit metadata inspection creates a snapshot. Intake and watchdog Verify use a cleared environment to read the current capsule; they cannot silently recapture or renew. A new inspection replaces the snapshot and invalidates old pins. Env-enabled native source revisions change on service restart: original Work/ACK remains durable history, while execution requires a fresh inspection and consent. Environment changes outside a captured process cannot be observed as live updates; new inspection/restart is the supported boundary.

## Cached OAuth

The bounded OAuth implementation is Kimi's existing cached access token with `cached_token_no_refresh`. Core uses direct transport and the existing required identity headers. Effective expiry is stored token expiry minus 300 seconds, further bounded by snapshot/consent/request expiry. Missing or expired cache, HTTP401, rotation and revocation fail; no refresh helper, login, source-token write or automatic regrant executes.

Access and refresh token literals are both protected at public ACP and tool-dispatch sinks, including split output. Full OAuth refresh for Kimi or other providers remains missing code: it requires refresh-origin/budget authority, token rotation CAS, cancellation/redirect and revision transitions. The UI will name the cached-only limit rather than advertise a login button.

## Wire and compatibility

Core catalog v2 adds closed `credentialSource` and `authPolicy` values, optional opaque `credentialSnapshotId` and `credentialExpiresAtUnixMs`, with readiness still `not_checked`. Broker catalogs include a public snapshot descriptor. New revisions use keyed `pr2_`; v1 `pr1_` metadata and pins retain their original replay shape.

Work pin v2 binds auth kind/source/policy/snapshot/credential expiry in addition to existing runtime, actor/device, destination/audience/project, context/resource, nonce and limits. Consent expiry is the minimum of the requested TTL and credential/snapshot deadline. Fresh runtime selection requires the newly built pinned Core; previous corrected and historical fingerprints remain recognizable only for their declared historical receipts.

## Finish criteria and exclusions

Keep RED receipts, then validate typed denial/replay tests, actual compiled Core env/cached-token journeys on owned loopback, budget/401/no-refresh, literal/split secret shielding, rotation/revoke/expiry and genuine process-stop evidence. Verify exact binaries and source commits, local checkpoints, public evidence hashes and preserved files. Integrate once after the meaningful gates pass; do not repeat broad audits without a new failure.

Native Tauri interaction is currently blocked: approved tools expose no native control surface, Desktop hardcodes8765, and its launcher rejects another port. Browser/native-host evidence does not replace Tauri/Wiii owner dialog or physical phone/keyboard/safe-area/AT. Candidate8767 stays unstarted. Real profiles/providers/model quality and device/cloud remain separately authorized acceptance; the finite prior product backlog is unchanged.

## Inspection and historical receipts

One inspection consent creates at most one metadata capture even when concurrent GET requests arrive. Other intake remains available while Core captures outside the store mutex. A new explicit inspection creates a new snapshot; it cannot rewrite prior consent, input or nonce. A read-only execution receipt checks the current private binding without recapturing environment. It preserves the original durable receipt while reporting retired snapshots as inactive. The browser also blocks a chosen tuple when fresh metadata for the same source has changed, preserving its original selection rather than silently rebinding it.

The concurrent-capture and retired-receipt regressions retain separate failing and passing receipts. The protocol fixture accepts either supported bridge version; historical v1 payloads keep their original shape, while the fresh runtime fingerprint advertises v2.

Review found a second wire mismatch: absent credential expiry on file-key/no-key v2 pins became the string zero in host environment, which Core correctly rejected. The host now omits it; a Rust RED/GREEN regression covers both tuples and the compiled journey includes each. HTTP401 follows the existing abnormal ACP contract: `recovery_required` with `core_rpc_rejected` and a genuine stopped process, rather than a successful completion or automatic replay. The first Kimi journey retained its timeout from expecting `failed`; the corrected driver checks the original non-success state, single request and no refresh/source write.
