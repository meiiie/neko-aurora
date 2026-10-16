# Local source and protocol validation

Runtime source checkpoint: `bdf1738f022d2f915bb2ea48055262a8c455146d`.
Latest source/harness checkpoint: `44c09481fd1d468572556ecb5109bba51f0b1908`.
The seven Wiii application/protocol/unit-test files remain byte-identical to
the runtime checkpoint; two owned test controllers add the larger-input gate.
Actual large-input Go/Rust checkpoint: `6e3d025c36b99924bd4a0f4df63fa002e8c2d5a1`.
The earlier small-message gate remains separate at client `c93a122` and Go
`dd86f3f`; the byte-budgeted Go recorder reproduction is also pinned to DD.
Environment: Windows DESKTOP-PLLO4HG, Bun 1.4.0, Node 25.9.0,
TypeScript 7.0.2, Python 3.13, Go 1.27.2, offline Rust toolchain.

| Gate | Result | Evidence scope |
| --- | --- | --- |
| Restoration | PASS: 3,205 archive-manifest files | Exact byte identity before editing; historical HEAD plus captured working tree, not a release claim |
| Original source preservation | PASS: 3,203 unchanged files | Only the owned NekoChillApp and ProjectHome entry files changed |
| `bun test src/__tests__/neko-chill/nekobot-local-bridge.test.ts` | PASS: 54 tests, 140 assertions | Direct source client, protocol, original strict storage, original handoff and answer-block projector |
| Focused `tsc --strict --noUnusedLocals --noUnusedParameters --noEmit` | PASS | New protocol and durable client |
| Wiii entry `bun build --target browser --packages external` | PASS | 145 source modules; dependencies external, not a packaged app build |
| Injected IPC source bundle and Python syntax | PASS | Owned test transport/controller syntax |
| Owned authenticated Go/Rust/Core protocol | PASS: 12 assertions | Actual local compiled service and pinned Core with controlled fixture provider; registration, tagged routing, scoped artifact SHA, private-history exclusion, lost ACK reconciliation, restart and audience revision rejection |
| Larger native Wiii input | PASS: 14 assertions | Actual 9,275 UTF-8 bytes, original nonce/text retained through completed Core artifact, same 5-second HTTP and 30-second reply deadlines |
| Unchanged independent large-page reproduction | PASS: Go 2 tests, Bun 2 tests | Writer repeated the reviewer's exact recorder/client probes on a fresh DD native archive; no assertion edits, new client limit or timeout |
| Full Wiii frontend production build | PASS | Archived lockfile unchanged; pinned 844 packages materialized in a separate build copy, install scripts disabled; actual TypeScript 5.9.3 and Vite 8.0.10 `npm run build` exit 0 |
| Wiii native Tauri compile check | BLOCKED | Offline locked Cargo check found existing MSVC after initial PATH failure, then exited 101 on disk exhaustion (os error 112 / LLVM no space / PDB LNK1318); no app/source failure asserted |
| Wiii native installer/package | NOT ACCEPTED | No bundler or installer run; frontend production build is not native packaging acceptance |
| Wiii native caller/pairing, packaged GUI, AT/phone/cloud/provider | NOT ACCEPTED | Injected caller seam is present; this work supplies no OS authentication/bootstrap or real provider |

Exact commands, exit codes, raw source hashes and log hashes are in
`.build/evidence-e0cf7bb/validation.json`. The latest actual process gate is
`.build/evidence-44c0948/native-large-input-6e3d025.json`; its retained owned journal and
stderr paths are recorded there. The controller only bootstraps its own local
fixture owner. The Wiii module receives relative authenticated request results
over injected IPC and never receives cookies or credentials.

Negative history is retained: the first sandbox run was blocked before Wiii
transport by loopback WinError 10013. Actual registration then found the earlier
`{adapterSession,duplicate}` fixture mismatch. The strict decoder now accepts
the actual `{session,duplicate}` key and rejects the obsolete alias; an
independent real-Go wire test confirmed the corrected checkpoint.

The independent large-stream negative on Go 984 remains preserved: its
1,155,596-byte cumulative page exceeds the client's 1 MiB hostile-response
bound, so the client rejects before committing a cursor. Go DD coalesces the
latest cumulative update per reply and caps pages at 900,000 encoded JSON
bytes. A fresh export of the same 65,536-byte task stream passes the unchanged
client probe and advances the cursor. The client bound remains unchanged.
The writer's probe rerun is separate from the reviewer's original audit and
from the actual Core surface gate.

The reviewer's sealed DD/bdf wire review additionally passes escaped replay
across two pages and consumer restarts. Its original report and manifest live
under the integrator's `.build/product-gap-review/`; final handoff links their
exact hashes without treating that recorder evidence as a native Core job.

The first larger-input controller run preserved a completed Core artifact but
failed the original small-history assertion. The 9,275-byte original request
does not fit the unchanged 8,000-byte source-pin budget. The corrected larger
variant explicitly requires empty sources and `omitted=1`, while the original
small variant still requires its exact primary source. The prior failure and
journal remain retained. This is a recorded context-availability limit, not a
claim that the large source was supplied to the model.

The initial npm offline attempt was blocked by sandbox EPERM while creating
scoped node_modules. A narrow approved public-registry `npm ci --ignore-scripts`
then materialized exact lockfile versions/integrity in `../full-wiii-build/`,
with empty npm configs and an OS-only environment. Plugin store 2.4.2 has its
original MIT OR Apache-2.0 license. The prior missing-dependency typecheck is
preserved; the fresh full frontend build resolves it without feature changes
or a lockfile rewrite. No provider, secrets, grant, system install or app
launch occurred. Build command, environment scope and raw logs are recorded
in that directory's `evidence/`.

The bounded native command was `cargo check --offline --locked`, using Cargo
and rustc 1.97.1 against the unchanged archived Cargo.lock (SHA-256
`5102d5408ce1d4e32eff21bb2649cdb7bc25a324038059eaa0d15a395649e7fc`).
The first sanitized environment lacked `link.exe`; the second initialized
only the existing MSVC 14.43 toolchain's build variables and reached offline
dependency compilation. That attempt stopped on disk exhaustion, not on a
missing dependency or a Wiii source assertion. Both raw exit-101 receipts
remain in `../full-wiii-build/evidence/`. Their raw status is FAIL; the scoped
acceptance classification is environment BLOCKED, recorded separately.
Only this task's newly generated `full-wiii-build/native-target` was removed
after checking its resolved absolute path and absence of a directory link.
Source, lockfiles, frontend dist, logs and journals were retained. No further
native build, dependency download, installer or application launch was tried.

The re-exported native source manifest, original probe hashes, Go recorder
output, Bun result and real wire JSON are in the sibling
`../large-page-dd86f3f/` directory. Old test titles retain historical checkpoint
names because the probes are unchanged; the manifest records their actual
server and client source identities. Final handoff records exact commands and
all supporting artifact hashes. No scratch journal is included in a Git
checkpoint or selective patch.

Licenses and original archived handoff flow remain intact. No default Wiii
startup transport is installed. The optional ProjectHome panel only appears
for an explicitly supplied trusted surface and matching Project; it does not
export the Wiii transcript, local workspace, profile or Core session identity.
Artifact receipts are shown as metadata; native artifact opening/downloading
and trusted bootstrap remain separate integration work.
