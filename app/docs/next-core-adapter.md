# Core process adapter

Reference source is now available and hash-verified. Compare against reference/extracted/app-c9dbbd7/package/app/packages/adapters/src/neko-runtime.ts, verifyInitialize, and neko-protocol.ts; archived Core is reference/extracted/core-3d5725f/package/core (1.8.0, snapshot commit 3d5725f).

The host/src/lib.rs CoreHello/Binding are proposed native envelope fixtures. They are not the raw ACP initialize/session admission schema and are not proof of interoperability with Core. Keep this distinction when adding the adapter.

Observed raw initialize contract: protocolVersion 1 with _meta entries neko.taskProtocol (version 1, fixed-active-task), neko.executionProtocol, neko.operationProtocol and neko.hostEffectProtocol (version 1), plus neko.hostProfile (id nekobot, version 1, isolation exclusive, transport acp). Its exact tools are mcp__nekobot__capabilities and mcp__nekobot__execute_tool.

Observed task receipt: version 1, mode fixed-active-task, 32-character lowercase hexadecimal id and activationId, trusted exact root and positive activationEpoch. Execution receipt binds taskId/root/activationEpoch/activationId with authorityId/policyId/bashTarget and an exact allowed tool ceiling, computer unavailable. Rust must preserve these checks before accepting a native envelope; Go must never manufacture Core identity or reinterpret native epoch as Core activationEpoch.

The pinned runtime is now restored by scripts/restore_core_runtime.py with both part, archive and executable SHA checks. host/src/acp.rs implements the raw contract above; host/src/main.rs owns ACP process transport. Go persists admission before acknowledging the first prompt. The native envelope fixtures remain separate from raw ACP. Core owns the complete model/tool loop.

The fixture supports the archived Linux runtime in WSL and native Windows Core compiled from all 772 verified source files with frozen dependencies and lifecycle scripts disabled. The Windows PE carries AGPL metadata with adjacent LICENSE/NOTICE. Binary hashes are pinned in native/internal/protocol/core-runtimes-v1.json shared by Go/Rust. Each Core has an isolated per-work home, sanitized provider environment and loopback provider. Windows follows Core's lowercase physical-root convention and verifies the same filesystem root before admission. This private build is not a signed installer or OS confinement.

Foreground/background use distinct Core sessions and slots. Streamed replies are committed before delivery. artifact.write_text syncs immutable bytes, records canonical operation/result and serves authenticated checksum-verified downloads. Duplicates return stored results; conflicts fail. Started effects are reconciled from exact file hashes; unresolved outcomes block new mutations.

On transport loss, Rust sends receipt-bound cancel/close, answers MCP disconnect during shutdown and denies new tool calls before terminating its process. Go preserves incomplete work as recovery_required. Resume retains Core session/task IDs and rotates activation; failed resume never substitutes a fresh session. A forced Core kill leaving a stale writer lock remains fail-closed under the pinned v1 contract. Locks are never auto-deleted. Safe typed error codes identify this gate without exposing raw provider diagnostics.

scripts/e2e_core.py has passed process, foreground overlap, artifact, duplicate inbound, hard-killed Go with supervised close/resume and cancel fencing in Linux and native Windows paths. The forced-Core test records safe refusal and preserved lock separately from the blocked resume capability. Fixture latency is not provider performance/model quality. See connections-local.md for the implemented DPAPI account cards, disabled plugin manifests and browser outbox. Owned Tauri WebView2 tests now cover UI replies, files and secure-form races; record exact commit and hashes on each checkpoint run.

Remaining gates: forced Core crash recovery, permission suspension beyond immutable files, account-bound runtime leases, signed distribution/installer, cloud/mobile/device execution and real provider/phone acceptance.
