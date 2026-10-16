# Profile auth bridge v2

Extends bridge v1 with a Core-owned bounded encrypted environment snapshot and cached Kimi OAuth. Existing v1 catalog/pin behavior is preserved. This is fixture-tested local transport support; credentialReadiness remains not_checked. Full OAuth renewal and real account/provider acceptance are outside this change.

## Public schema

profiles --json --profile-v2 --credential-home <source> returns {version:2,profiles:[...]}. Each row has the ten v1 fields plus credentialSource (file_api_key|process_env|oauth_cache|local_none|unavailable) and authPolicy (static_key|cached_token_no_refresh|unavailable). Revision is pr2_<64-lowercase-hex>.

credentialSnapshotId and credentialExpiresAtUnixMs are omitted when absent. File API-key/local-none static rows carry neither. Process-env rows require both. Cached Kimi always carries effective expiry and can be file-only without a snapshot. Capsule-bound Kimi carries the matching snapshot ID and expiry is capped by snapshot expiry. Rows with snapshots match report sourceSnapshot:{id,createdAtUnixMs,expiresAtUnixMs}. Snapshot IDs use cs1_<32-lowercase-hex>. No raw key/token/env names/home/account IDs/endpoint paths enter metadata.

File/local revisions ignore global env snapshots. Re-capture invalidates process-env and capsule-bound OAuth rows; static file/local rows retain source-file/HMAC semantics. Missing/unsupported material has unavailable policy, not an execution grant.

## Pre-agent broker and Verify

profiles --json --broker-stdin accepts one UTF-8 JSON value, at most 16 KiB, with exactly:

{version:1,sourceId,credentialHome,stateHome,capsulePath,revisionKey,expiresAtUnixMs}

All roots are absolute real local directories, source/state disjoint. Capsule parent exists outside both roots. revisionKey is the existing private parent-owned 64-hex key. Expiry is future and at most 300 seconds from capture. Broker reads bounded source profile/auth files and configured key_env/fallback names; it never enumerates env values, creates a provider/agent/tool, refreshes/logs in, imports project/MCP/trust, installs or updates. Native may inherit owner environment only for this pre-agent process after inspection consent. Development uses explicitly constructed fake environments.

Keys never enter task environment. Core writes a bounded AES-256-GCM capsule with purpose-separated HMAC-derived encryption/connection/marker keys. AAD authenticates source ID, canonical source home, snapshot/times and configured-env-only policy. Encrypted records bind exact env names/connection revision. Capsule and .current marker are fsync'd and atomically replaced separately; every pair mismatch fails closed, including interruption between replacements. Alias parents, symlinks, hardlinks, nonregular/oversized files and detected read-time file-identity changes are refused.

Verify never runs the broker. It invokes v2 profiles with cleared env plus capsule path and current accepted snapshot ID. Tasks validate the pair against pinned ID. Replaying a valid old pair cannot pass another current expected ID. Native registry/ledger owns snapshot high-water, restart service-generation invalidation, consent/source revisions and stop reconciliation; Core cannot recover high-water after whole-private-state rollback. No automatic repair/re-capture/rollback.

## Task private parameters

Alongside existing revision key/revision/origin/limits and credential-home:

- NEKO_PROFILE_CAPSULE_PATH and NEKO_PROFILE_SNAPSHOT_ID only for snapshot-bound rows; marker is capsulePath + .current.
- NEKO_PROFILE_AUTH_POLICY / NEKO_PROFILE_CREDENTIAL_SOURCE match the selected row.
- NEKO_PROFILE_CREDENTIAL_EXPIRES_AT_UNIX_MS only when row has expiry.
- NEKO_PROFILE_DEADLINE_UNIX_MS is mandatory, within credential and native consent/request lifetime.

Core rechecks source/revision/policy/model/origin/snapshot/expiry before construction, completion, attempts, output and in-band effects. Redirect/input/request/output/time budgets remain v1. Wire cancellation uses the earliest absolute deadline even for silent/hung providers. Clock-backward/monotonic guards remain. Private fields are excluded from printable config/model-driven children; public read/search shields capsule/marker/temp variants and credential source.

## Cached Kimi only

Uses existing bounded .neko-core/kimi-auth.json snake/camel schema; expires_at/expiresAt is a safe integer in seconds. Effective expiry is seconds*1000-300000, optionally capped by snapshot expiry. Access token goes directly to Chat Completions with existing Kimi identity headers. Pseudonymous device ID is task-local; device/OS headers belong to this transport.

No validKimiAccessToken, Kimi 401-repair wrapper, refresh endpoint, login or source write. HTTP401 is terminal. Expiry/rotation changes original revision and requires new inspection/consent; no billing-route fallback. Access and refresh literals are both protected in Core: split/mixed-channel streaming, notification/tool/error/history/context surfaces are redacted or refused. Arbitrary encoded/inferred exfiltration is not claimed solved.

Other OAuth remains unsupported. Renewal needs explicit origins/refresh authority, budget/cancellation/redirect guard, rotation CAS/revision receipts and concurrency policy. Installed legacy Core is unchanged.
