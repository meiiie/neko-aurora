# Local executor registry v1

This package is the bounded registry and wire layer for S3, based on app931c4b76eb82bc92b1ca2c80a89ab04112c0d5db. Root owns Store/API/manager and Rust native integration. Existing adapter catalog v1 fields remain unchanged. This does not pair a remote device, call a provider, renew a lease or certify cloud/phone execution.

## Trusted configuration and authority

`executors.Open(absRegistryDirectory, Descriptor)` binds a single persistent executor ID to exact configured host/core/state-root/capability SHA256 values. `rootSha256` is supplied by the caller from the exact configured state root; it is never the per-Core request directory. Pins, lease IDs and proofs contain public identity metadata and are not bearer capabilities. Task scope contains server-created request/attempt IDs, work epoch and a server-derived hash of task/account/context/audience/resource authority. No renderer-supplied endpoint, key, path or argv is accepted.

`InstallPin`, `AttemptLease`, `RuntimeProof` use version1, lower-case hex digests/IDs and positive integers no larger than9007199254740991. Parse helpers reject unknown, duplicate, differently cased and omitted keys, invalid UTF8, fractional/exponent/negative-zero integers and JSON after the object. Wire payloads are limited to1024bytes. `Valid*` methods validate typed values for historical replay without querying today's registry or clock. `ProofFor` and `MatchRuntimeProof` perform static descriptor comparison, which is not live admission authority.

## Registry lifecycle

The registry holds an OS-exclusive writer lock. Its lock file retains a sync-written stable identity marker; deleting/corrupting the identity data does not silently regenerate an installation. Identity files must be regular, non-symlink, single-link files. Registry data is sync-written to an exclusive temporary file and atomically replaced (Windows write-through rename, Unix directory sync). A fresh boot ID is persisted on every Open, all previous leases are retired, and readiness starts false. Corrupt/mismatched state returns `ErrUnavailable` without overwriting the evidence. Root must keep the separate application journal readable and disable dispatch task-locally.

`Snapshot()` exposes only pin, boot, readiness, reason, revision and heartbeat expiry. `MarkReady(pin,boot)` establishes a fifteen-second trusted heartbeat window. `Heartbeat(pin,boot)` extends a still-live window and cannot reconnect an expired host. `MarkUnavailable(pin,boot,reason)` accepts only host_disconnected, host_unavailable or shutdown and retires active leases. Reconnect uses a new trusted MarkReady call; it never revives a retired attempt.

`Issue(pin, TaskScope)` returns `(lease, duplicate, error)`. The same request/attempt/scope returns the exact retained lease while it is live; changed scope conflicts, and retired attempts remain denied. Each new lease has a monotonically persisted fence and at most a ten-minute lifetime. An orphan lease after a failed ledger claim is not sufficient to launch; Root must match it to the exact current Work attempt and epoch.

`WithLease(pin,lease,callback)` verifies exact pin, boot, retained lease, current availability, expiry and identity-file continuity, then holds the executor mutex through the short callback. Root lock order is ledger -> connections -> executors. Callbacks must not re-enter this registry or write blocking native frames. Registry mutation methods never callback into ledger; Root reconciles affected work and closes owned processes after unlocking.

`Revoke(pin,nonce)` and `Configure(pin,newDescriptor,nonce)` use pin CAS and durable exact operation receipts. Both increment generation, retire old leases and leave readiness false; duplicate nonce returns the historical metadata receipt, not current readiness or authority. Reusing a nonce for another operation or descriptor conflicts. Re-enable requires a new generation and trusted readiness, never old-pin fallback.

Observed lease/heartbeat expiry and clock rewind are persisted before denial. Write failure or live identity tamper poisons the instance; readiness, issue and callbacks stay denied after clock rewind. Restart also rotates the boot and retires persisted leases, so an uncertain expiry write cannot reactivate an old attempt. Expiry does not prove a native process stopped.

## Parent integration invariants

New intake pins the configured installation, claim issues the attempt lease, and dispatch/admission/output/effect/completion recheck it alongside current account/context/audience permission. Rust validates the strict lease and installation digests before any task directory/Core launch and echoes an exact companion RuntimeProof. Parent admission/stop must match that proof to the saved current lease plus the existing native PID/session/current-attempt binding. A public proof alone never authorizes a renderer or cross-process receipt.

Historical replay validates descriptor/transition consistency only. It must not query live registry generation, boot, readiness or time. Legacy launched work (Core!=nil, AttemptID set, staged binding set or operations present) never acquires a new executor pin implicitly. The only approved compatibility branch is provably never-launched work: Core=nil, AttemptID empty, staged binding=nil and operations empty, pinned at a new versioned claim. Parent owns that branch and recovery policy.

Unavailable unclaimed work preserves its queued obligation and gains no lease. After restart a previously launched task requires an explicit fresh attempt, exact old stop/effect proof, same installation/root/runtime pin and current permissions; old binding/proof is not rewritten. Revoked or replaced installation pins cannot resume.

## Bounded limits and remaining gates

The registry retains up to4096 lease receipts and1024 mutation nonces in at most4MiB. Capacity denies new issuance/mutations without evicting old idempotency receipts. Trusted compaction/retention and long-running renewal are future work; this slice has no automatic ten-minute renewal. Root must stop/suspend an expired native attempt and record actual process stop separately.

Package tests cover parser/proof scope, persistence/fresh boots, availability, exact lease identity, revoke/configure CAS, corruption/deletion/hardlinks, write-fault poisoning, clock rewind, duplicate issuance and callback/revoke serialization. They do not certify integrated Work replay, native PID/session wiring, actual Core, provider, UI, cloud or phone behavior. Root owns those integration gates.
