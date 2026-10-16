# Native stop outbox and explicit service-crash recovery

Base: immutable Root `3dc27c64eb3ec119695afd62e38bb852fe9c949b`; Core `9f2303f` / `97ca4c8a…` remains unchanged. This slice handles individual context-v1 tasks. Group stage recovery is deliberately unsupported by `ControlTask`; old context-v0 or generation-less launches remain blocked.

## Native proof

The trusted Rust host obtains `nativeProcessGeneration` as a canonical positive decimal u64 Windows creation FILETIME **from the live spawned Child handle**, paired with that child's PID. This is companion metadata outside the strict context receipt, manifest, Core RPC and model prompt. The admitted binding retains the actual context receipt (hashes, source IDs, task and execution metadata), not context/source contents.

After actual child wait succeeds, Rust writes a version-1 record containing exactly `version`, `requestId`, `attemptId`, `installPin`, `attemptLease`, `binding`, and `receipt`. The stopped receipt is version 1 plus the same generation and saved executor companion. No runtime manifest, selected source text, credentials or writer-lock token enters this file. Core close/cancel requests, writer `released` markers and PID absence are never stop proof.

The trusted Go manager supplies the configured STATE ROOT, not a renderer path. The destination is `<STATE ROOT>/runtime-stops/req-<32 lowercase hex>/attempt-<32 lowercase hex>.json`. Rust canonicalizes the state root, rejects reparse directories and uses an exclusive pending file, flush+sync, then Windows `MoveFileExW(WRITE_THROUGH)` **without** replacement. An existing regular single-link file is accepted only when bytes match; differing, partial, pending or oversized files are retained and denied, never overwritten. Bound: 16 KiB; no deadline/cap increase.

Go uses a read handle that denies concurrent write/delete, rejects reparse/dir/hardlink files, verifies the final physical handle path, reads at most 16385 bytes and rejects unknown/duplicate/missing keys or trailing JSON. It reconciles only a saved prior-boot CURRENT attempt whose original install, lease (including original epoch), binding, context receipt, PID, generation and session all match. Replay validates typed saved history without today's clock/registry. Recording a retired stop receipt never grants authority or changes intent.

Startup keeps the existing `RecoverInterrupted`/executor retirement order. A missing/bad file leaves only that task blocked, never corrupts the journal globally. Once-per-second bounded reconciliation discovers authentic receipts delivered after startup. Diagnostics are task-scoped codes; no path, raw context or credentials are exposed through them.

## Explicit recovery intent

`POST /v1/task-controls` uses the existing local session boundary. New input: `{version:2,action:"recover",requestId,expectedEpoch,expectedRevision,clientNonce}`. Version 2 accepts only this action; version 1 cannot express it. No caller-supplied PID, proof, generation or outbox path is accepted.

Recover requires `suspended`/desired `suspended` with `executor_lease_unavailable`, an actual saved context-v1 Core and current attempt binding, exact valid stopped receipt/generation, completed effects (no started/unknown effects), original context ancestry/plugin/account/audience/project rights and exact current install. The commit is guarded under ledger → account → executor locks. It changes versioned intent to waiting_host, increments epoch/task revision and keeps original Core/context/output/effects/installation. A subsequent claim obtains a distinct lease/attempt/fence; old lease is never renewed/revived. Existing bounded prior-receipt retry activation rules remain unchanged. Repeated exact nonce input returns the current command receipt; changed input or stale CAS is rejected.

There is no automatic resume, implicit repin, context recapture, group restart, provider selection or grant. Availability loss with an unchanged install is distinct from installation authority; new lease still requires readiness. An old missing generation or unprovable stop remains BLOCKED.

## Verification

Focused ledger/parser/filesystem/API negatives and Rust immutable publication tests exercise current binding, cancellation, unknown effects, lost ACK, replay and path/bound invariants. The owned `scripts/e2e_service_crash.py` runs compiled Go/Rust/Core against a loopback fixture, retains a live OS process handle, kills only its owned Go process and inspects the authentic post-wait receipt. Separate fresh-state cases exercise explicit resume, delayed delivery, missing/corrupt/mismatched receipt and cancellation. The delayed fixture moves authentic bytes after native wait; it does not fabricate proof. Exact clean checkpoint receipts/logs are published separately after running; narrow fixtures are not phone/cloud/model-quality or product-wide acceptance.
