# Private Windows task writer v2

This local integration patch is derived from the verified Core source snapshot
`3d5725f222815e15b12a212e372cf17057a459c0`; its fresh local Git baseline is
`80b1394952bfd4594bd3cf9ff850b101ea762e12`. It does not recreate upstream Git history
or assert a published Core release. All 772 source files were verified against
`core-source-manifest.json` SHA-256
`e2720458d4b2a42951ca3df30568f8409326745912500762242c82ae7429000e`.

The private native helper and Core shell remain AGPL-3.0-only. The Windows build
copyright string now agrees with the existing package license and LICENSE/NOTICE.
No provider, agent loop, tool ceiling, model context or checkpoint schema is replaced.

## Trusted launcher descriptor

The integration is opt-in on Windows. A trusted launcher supplies both
`NEKO_TASK_WRITER_HELPER` (absolute executable path) and
`NEKO_TASK_WRITER_HELPER_SHA256` (lowercase SHA-256). Core verifies a regular file,
one link and exact digest before spawning. An absent descriptor retains the original
v1 writer behavior. An incomplete, invalid or unavailable descriptor fails closed;
it never falls back to a weaker writer after opt-in.

The launcher must protect the helper executable and task store from untrusted
writes, pin the Core and helper binaries, clear credential-bearing environments,
and keep both binaries with their source/license provenance. This is cooperative
ownership among trusted writers under one Windows user; it does not defend against
a same-user administrator rewriting a released lock or executable.

Build the helper locally with:

```powershell
cargo build --offline --locked --release --manifest-path native/writer-lease/Cargo.toml
```

Deploy a byte-verified copy of the output, with one filesystem link. Cargo may
hardlink incremental build output to its dependency directory; the trusted loader
correctly rejects that build-cache path. The native fixture uses an owned copy.

The Core helper child receives only `SystemRoot`, and no model/account secret.
Arguments are `--directory <physical task store> --session <32 lowercase hex>`
`--parent <Core PID> --parent-sha256 <current Core executable digest>`.
The helper checks its actual parent via the Windows process snapshot, retains a
kernel handle to that parent, checks executable SHA-256, and records its creation
FILETIME as a canonical decimal string, preserving all 64 bits. Noncanonical
representations (such as leading zeros or a plus sign) are rejected before
process identity comparison.

## Private IPC and persistent ownership

Input is bounded JSONL (4096 bytes), one acquire and one release command:

```json
{"version":2,"action":"acquire","token":"<private 32hex nonce>"}
{"version":2,"action":"release","token":"<same nonce>"}
```

The helper emits at most two bounded owner receipts with fields `version`,
`contract` (`neko.windows.writer-lease.v2`), `directory`, `sessionId`, `parentPid`,
`parentCreated`, `parentSha256`, `token`, `state` (`held`/`released`). The token is
private pipe/store metadata and never enters arguments, stderr, ACP receipts or
model prompts. Core validates the exact receipt shape and its own descriptor.

The helper keeps `<sessionId>.lock` open with Windows `share_mode(0)` throughout
the lease. It opens the reparse point itself and rejects reparse files and hardlinks.
It never unlinks the lock. A new file is written/fsynced as held before readiness.
Existing empty, partial, corrupt, legacy v1 or unknown records are preserved and
rejected. A v2 released record can be reused while the old Core process lives only
after explicit quiescent close. A v2 held record can be reused only while the helper
owns the exclusive file handle and proves the exact old PID/creation identity dead,
or proves that the live PID belongs to a different creation identity. Access denial
or unknown liveness blocks the claim. No timestamp age or PID-only heuristic is used.

The helper fsyncs released state and closes kernel file ownership before its release
ACK. Core awaits release only after the runtime is quiescent and closed. Factory or
cleanup failures with uncertain native runtime lifetime retain held ownership.
Stdin EOF, invalid release, helper crash and parent crash never mark released. When
Core dies, its retained process handle becomes signaled and the helper exits,
closing kernel file ownership. A future Core then resumes the same durable session
with the existing task activation protocol; it does not substitute a new session.

A v1 Core cannot load a v2 writer session because the persistent `.lock` still
exists, including after clean release. Legacy locks require an explicit separately
verified migration and are not automatically deleted or imported by this patch.

## Validation scope

New-session publication on Windows uses a same-directory `MoveFileExW` with flags
zero: no replace-existing and no cross-volume copy. The complete private temporary
file is fsynced before publication, then its identity and one-link target are checked.
The preloaded Windows KnownDLL `kernel32.dll` supplies this primitive; project/PATH
DLL shadows cannot select a replacement. A real non-executable shadow fixture verified
the loaded module remained the protected system Kernel32.

This avoids the Windows hardlink-publication/temporary-alias-deletion sequence that
reproduced later checkpoint `EPERM` on the original baseline, Bun and an independent
native Rust process. Direct publication and native no-replace publication remained
checkpoint-replaceable. The specific NTFS internal cause is not asserted. No rename
retry, destination deletion, lock recovery heuristic or weakened permission guard is
introduced. Other platforms retain the portable hardlink backend and its cleanup
fault hook; Windows publication has no temporary alias left to clean up. Existing
snapshot-replacement code remains unchanged. Tests cover repeated immediate checkpoint
replacement, kernel EEXIST, and two simultaneous real-process publishers.

`test/task-writer-native.test.ts` exercises actual Windows handles/processes. Set
`NEKO_TEST_NATIVE_WRITER_REQUIRED=1` to require the compiled release helper rather
than silently count an unavailable primitive. It covers simultaneous writers,
helper death while parent remains live, explicit release, same-session/task crash
recovery with durable messages and activation +1, old-Core rejection, legacy/empty/
corrupt/unknown/hardlinked records, parent executable mismatch and synthetic PID
reuse metadata. The PID reuse fixture does not claim real Windows PID reuse.

Persistence/lifecycle changes still require the complete local gate in
[TESTING.md](TESTING.md). A coordinator fixture is not model evaluation, and this
private source patch needs separate pinned App ACP integration validation before
the App may claim end-to-end forced-Core crash recovery.
