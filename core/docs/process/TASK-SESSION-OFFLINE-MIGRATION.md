# Explicit offline task-session migration

This private extension derives from writer-v2 checkpoint
`8d1aefb91068b07ad395865602b0c359a0d016bc` and the verified Core source snapshot
`3d5725f222815e15b12a212e372cf17057a459c0`. It preserves the AGPL-3.0-only license.
It is an operator tool, not an agent/model tool, automatic startup migration, or
permission grant. No provider, runtime or GUI is started.

## Invocation and explicit modes

```text
neko task-session migrate-v1 <absolute-manifest-path>
```

The strict version-1 manifest has contract
`neko.task-session.explicit-v1-migration`, one operationId (32 lowercase hex), and:

| Field | Bound and purpose |
|---|---|
| mode | `schema-v1-label-import` or `writer-v1-metadata-rehome` |
| source.home | Fully qualified Windows trusted private home; no aliases; root/path <=4096 UTF-8 bytes |
| source.sessionId | 32 lowercase hex; file is derived from the trusted task store |
| source.sha256 / source.bytes | Exact original bytes, 1..64 MiB, before any backup/target |
| source.coreSha256 | Historical lowercase SHA-256 pin supplied by the operator |
| canonicalRoot | Exact existing canonical physical task root |
| authorityId / configId | Original host identity and nonsecret configuration digest |
| targetCoreSha256 | Must match the actual running executable |

The manifest path grammar is Windows on every runner: drive-qualified or UNC paths;
paths that depend on the current drive are rejected. The same Unicode digest vectors
can be checked on other operating systems; execution remains Windows-only.

Manifest files are regular one-link files, at most 16 KiB. Unknown fields, malformed
JSON, invalid UTF-8, unsafe identifiers, source/config/root mismatches and executable
pin mismatches are refused. SHA-256 pins are integrity checks, not signatures.
`source.coreSha256` is explicitly reported as an operator attestation: an arbitrary
transcript does not cryptographically prove which executable created it. The
launcher must check its trusted historical pin/receipt policy independently.

`schema-v1-label-import` accepts only task JSON schema 1 and extends the existing
explicit labels/ancestry import with a backup and restart journal.
`writer-v1-metadata-rehome` accepts only task JSON schema 2, including old App fixed
task metadata. These are different modes: task JSON schema 2 does not imply a
writer-v2 lock. Structured schema 3 rehome is unsupported and fails closed.

Both modes create a **different session and different task IDs**, preserve labels
and exact ancestry, and initialize empty messages/source events. Host effects,
execution authority, display history, context and old task activations remain
quarantined in the untouched source and backup. No task/session receipt is forged.

## Native ownership and protection

Migration requires the pinned native Windows writer-v2 descriptor documented in
[TASK-WRITER-WINDOWS.md](TASK-WRITER-WINDOWS.md). Both trusted helper environment
fields must be supplied. The helper's file shape/hash and actual parent identity
are verified by the existing writer implementation. The source, journal and target
are held under independent native writer leases, acquired in that order.

Any existing legacy v1 lock, empty/partial/corrupt/unknown record, live owner,
permission-denied liveness, reparse lock or hardlink lock remains refused. An old
lock's PID and age cannot prove its creation identity. **A legacy stale lock is
therefore still BLOCKED even for explicit migration.** This tool does not delete,
rewrite, age out, rename away or automatically migrate that lock. Owned scratch
sources without a conflicting owner, or valid writer-v2 ownership with exact dead
process proof, can proceed. No real user state was migrated during development.

Source JSON bytes are never rewritten. Native source ownership creates/reuses a
persistent v2 lock marker; the old v1 writer consequently cannot resume that source
after the explicit operation. This deliberately freezes cooperative legacy writes
without pretending the old session continued. Rollback is not an automatic removal
of that marker. The original JSON and byte-identical backup remain readable history.

The trusted launcher must protect the home/journal/binaries with appropriate Windows
ACLs. POSIX creation modes alone are not a Windows ACL guarantee. Backup contents
can be sensitive; they remain in the private host store and never enter logs, model
context, ACP events or public artifact uploads. Tests use only synthetic scratch data.

## Durable mapping and restart

The source session ID selects an immutable journal in
`<home>/.neko-core/task-session-migrations/`. One manifest/operation per source ID
prevents a second retry target. Files are published without replacement:

1. `<source>.v1.backup.json`: exact source bytes, file fsync and byte verification.
2. `<source>.intent.json`: manifest digest, exact planned target snapshot, its hash.
3. Target JSON: the new IDs selected by the durable intent, with native writer-v2 lock.
4. `<source>.report.json`: exact mapping/provenance and initial target snapshot hash.

The intent is durable before the target is published. Restart after backup can
prepare the first plan; restart after intent/target reuses that exact plan. A lost
return acknowledgement after report publication replays the same report. Corrupt,
missing, changed, hardlinked or aliased required artifacts are preserved and refused.
Prepared snapshots, ancestry and mapping entries use exact metadata-only fields;
self-consistent modified intents with extra context, reused old IDs or changed
active/order mappings are refused before target publication.
No retry can overwrite an existing target. After the target has legitimately been
activated or changed, rerunning this offline transaction requires reconciliation;
it does not reset live progress to the initial snapshot.

Manifest digest is SHA-256 of Core's recursively sorted canonical JSON, encoded as
UTF-8 without whitespace. Strings use JSON.stringify escaping; bounded integers are
decimal. No Unicode normalization or path spelling rewrite is performed. Shared
Unicode/property-order vectors are in
`test/fixtures/task-session-v1-migration-digests.json`; Go/Rust adopters must use those
bytes, not assume their default JSON encoder yields the same digest.

The report contract is `neko.task-session.v1-migration`, version 1, and includes the
mode, operationId, manifestDigest, source/backup pins and sizes, source revision and
activeTaskId, target session/activeTask IDs, exact ordered taskMap, root/authority/
config, target executable/snapshot hash and helper hash. `sourceCorePin` is
`operator-attestation`; `semantics` is `new-empty-session-with-labels-and-ancestry`.
Pending/unknown historical host effects are counted and flagged for reconciliation;
they are not replayed or marked completed by migration.

## App adoption and acceptance limits

`target.taskProtocol` is reported as `legacy-metadata`. This tool does not mint a
fixed-task activation or carry old execution grants. The current App fixed-task ACP
load contract cannot directly adopt such a target; an explicit Core-owned protocol
initialization contract or a reconciled new-request flow is still required. Go must
keep the old obligation/unknown effects and this typed mapping separate. It must not
replace an old receipt with a guessed activation or label this as same-session resume.

Regression coverage includes failing-first missing-backup/duplicate-target evidence,
strict pins and bounds, byte preservation, idempotent replay, interrupted stages,
actual source process hardkill after intent/target, live-owner denial, legacy-lock
denial, artifact corruption/hardlinks and schema2 context/effect quarantine. Native
tests account for three leases per transaction, each with existing 10s acquire and
release receipt bounds and a bounded 10s helper exit. Release now waits for actual
helper exit after its validated ACK, because Windows still owns the executable
image before that exit. A five-case native control and failing-first adapter
regression establish this difference; no deletion retry is added. Receipt deadlines
are unchanged. The initial five-second
test-runner clipping and malformed synthetic activation fixture failures are retained.
The inherited native regression harness now bounds each process exit at 5s and
uses the sum of its existing 10s receipt and 5s exit ceilings for each multi-process
case. Its assertions are unchanged. Byte-identical baseline tests and a controlled
350ms fixture-delay experiment distinguish aggregate runner clipping from an
unavailable owner or blocked helper exit. Original full-gate failures are retained.

Persistence changes require the exact-commit full gate in [TESTING.md](TESTING.md).
Local synthetic migration is not cloud restore, live provider evaluation, real user
data migration, App ACP adoption or mobile acceptance.
