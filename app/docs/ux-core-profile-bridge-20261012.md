# Core profile bridge UI, 2026-10-12

This slice extends the existing light Connections card. Neko Bot uses a compatible bundled Core for the bridge; the installed Core and its profile remain intact. A discovered installation alone does not establish bridge compatibility or working credentials.

The source-only UI checkout starts from application `16e9daddd568450379877b75f94a954384beca8d`. Its independent baseline is `ccdc4f99659b634ef9f5a9fcf7bff1a65e81532f`, with the identical tracked tree `f6c9bfb8999619f1f1d6cd4e99e8d9bb9ef349fe`. No dirty backend files, state, binaries, profiles or ignored archives were copied. Root owns integration and the final application/Core code identity.

## Interaction

`Dùng profile có sẵn` is enabled only for the default local owner with `local_core_profiles_v1`. Fixture principal sessions cannot inspect or choose these profiles. The action first discovers safe source descriptors and explains inspection. It does not read profiles or create consent until the human chooses `Cho phép xem profile`.

Inspection consent lasts 300 seconds. The subsequent bounded GET returns only profile ID, provider, model, authentication kind, public origin, configuration status, profile revision and transport support. `configured` describes the configuration shape. `credentialReadiness: not_checked` remains explicit; the UI makes no credential or provider readiness claim. Unsupported/incomplete profiles cannot create execution consent and do not fall back to another provider. The existing manual configuration form remains available.

Execution approval is a separate action. It shows the current channel or server-returned private conversation, bot, project if selected, provider origin, data categories and editable limits. Default limits are 64,000 bytes for the **whole provider input envelope**, 1,024 output tokens, one request, one step, 120 seconds per human Work, and a 300-second consent lifetime. The output note states that UTF-8 text, JSON and tool output reservations may stop execution early. These are bounded execution permissions; metadata inspection alone cannot start work.

The normal composer pins only `coreProfileSelection: {consentId, expectedConsentRevision}` plus the exact channel/audience/project scope. It cannot mix this pin with a manual account or bot defaults. A known default private conversation uses the server-returned `conversation-bot-id` and audience revision 1, independently of the conversation metadata revision. An absent private scope is not invented or created automatically. Two-bot collaboration does not silently discard the profile selection.

## Persistence and races

The UI keeps at most eight safe pending operation records under the already session-namespaced `neko.core-profile-operations.v1`. Records contain typed original input, operation kind, creation time and SHA-256. They contain no key, profile configuration, bearer, raw session ID or actor/device object. Loading and asynchronous hashing capture the actor/session namespace before awaits, then recheck it before importing, persisting or posting.

Reload and reconnect never create inspection/execution consent. Reloaded records require read-only receipt reconciliation and an explicit use/retry action. A missing receipt allows an explicit retry of the same nonce, body and limits only while the original scope still matches. Duplicate server creation returns the original pin; it does not renew approval. Malformed or credential-bearing origins and mixed account/profile pins are rejected before cache or network use.

Profile-selected message outbox entries preserve their original pin. Reload does not automatically POST them. A Work ACK must match both the original selection and public consent pin before retiring an entry. A changed selection cannot claim ownership of an old retry's draft.

Late consent ACKs cannot select another channel or clear its draft. Session rotation clears all Core profile memory and prevents a deferred digest from writing into the new actor namespace. Revoke/expiry/suspension SSE events synchronously mask the selection before the original receipt GET. Reconnect masks the selection and restores only the same still-active receipt/scope; it does not inspect credentials or mint a new grant. Fresh submit checks catch idle expiry before building the normal message. Report delivery remains on its separate non-provider route.

## Validation

Commands run from the owned checkout with Node `v25.9.0`, Windows, Chrome `154.0.8037.98`. Every browser profile and test output is isolated under owned E scratch. The harness metadata records exact source commit, working asset hashes, harness hash, command and scope. Source commits do not contain scratch profiles or generated media.

```
node --check native/internal/web/assets/app.js
node --check native/internal/web/assets/connections.js
node scripts/ui_core_profile_bridge_local.mjs --output E:/CodexScratch/core-profile-bridge-20261012/ui/<run>
node scripts/ui_connections_secure_local.mjs
node scripts/ui_principal_device_local.mjs --output E:/CodexScratch/principal-device-boundary-20261012/ui/core-profile-regression16e9dad
python scripts/secret_scan.py <owned-checkout>
git diff --check
```

Core UI RED evidence preserves disabled/missing flow and dropped pin, silent invalid-pin fallback, automatic restored outbox POST, SSE selection masking, malformed origin, mobile select overflow/hit area, private audience revision and idle consent expiry failures. Harness setup failures are also retained: initial navigation polling was corrected; Chrome can automatically retry a disconnected HTTP transport, so the authored pre/postcommit uncertain-ACK fixture uses explicit 503 responses and correctly deduplicates exact nonces. Those setup failures are not product pass claims.

The bounded profile browser suite has 37 checks, including consent ordering, scope/caps, unsupported transport, lost/uncertain ACK reconciliation, exact missing-receipt retry, reload, late navigation ACK, async digest actor rotation, revocation, idle expiry, truthful private scope and mobile controls. The existing manual-account VM suite has 13 checks and the principal/device browser suite has 37 checks.

These are actual UI assets in headless Chrome against authored HTTP/session/profile fixtures. They do **not** establish Go/Core bridge interoperability, real identity verification, credential usability, paid model quality, native-window or phone acceptance. Actual Go/Core fixture integration and any future user-home/provider test remain Root-owned and require their own exact evidence and authorization. No user Core configuration, stored key, real provider, live state/process, port 8765, native window, Library upload or production grant was accessed by this slice.

## Catalog interoperability correction

Root's actual native catalog surfaced a mismatch in the original authored fixtures. Core emits `openai_compat`/`api_key`, a bounded catalog up to 64 entries, unsupported HTTPS presets and unsupported localhost/loopback origins, plus incomplete entries with empty provider/model/origin. The original UI rejected some descriptive entries and capped the list at 32. Catalog RED evidence records five failures including the rejected 64-entry list.

The corrected metadata validator accepts these **descriptions** only with safe origin-only syntax, bounded ASCII host/port and the server's unsupported/incomplete status. It still rejects credentials, paths, queries and fragments. Empty provider/model/origin is allowed only for incomplete unsupported entries. The public list limit is 64 and the fixture's eligible profile now uses the actual provider/authentication enums.

Executable eligibility remains separate: configured + supported, `api_key` or `none`, an allowed bridge provider, and the original strict source origin policy. Fixture execution still requires explicit loopback-IP HTTP with a port; owner-home execution still requires HTTPS. Unsupported preset origins and localhost descriptions cannot create or validate an execution consent. The corrected browser suite has 50 checks, retaining all previous scope/retry/session/race assertions. Prior 30ca751 evidence is preserved as historical UI fixture evidence; the follow-up checkpoint identifies the catalog correction separately.
