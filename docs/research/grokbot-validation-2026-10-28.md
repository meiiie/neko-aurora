# Validation — Grok Bot adoption, 2026-10-28

Base: `66376d68691d8bce16b6565c6268c4331802ef57`, branch `codex/grokbot-adoption`. The [application manifest](grokbot-application-manifest-2026-10-28.json) binds changed application/toolchain sources to SHA256. This is local development evidence, not a deployed release or complete Grok Bot parity.

Windows amd64; PowerShell 7.6; Go 1.27.1; Rust/Cargo 1.97.1; MSVC 14.43.34618; Windows SDK 10.0.22621.0; Bun 1.4.0; Node 25.9.0; Python 3.13.7; Chrome 156.0.8078.48 headless. The runner allowlists OS/runtime environment variables, excluding live provider/account variables. Tests use synthetic credentials and loopback servers.

Exact executable paths, argv, cwd, base commit, source manifests, diff hashes, duration and exit code remain in ignored `.audit/2026-10-28-windows-backlog/validation.jsonl`. Final runs snapshot sources before execution and compare after; changed sources produce STALE, not PASS. Older exploratory runs captured the manifest after execution and are diagnostic history. Source manifests include untracked code. Go commands below run from `app/native`; other commands run from the repository root. Setup details are in [getting started](../getting-started.md).

## Verified results

| Check | Exact command apart from executable/output directory shorthand | Result |
|---|---|---|
| Six Windows regressions | `go test -count=1 -timeout=180s -run 'Test(CoreStreamStripsUsage\|HybridFallsBackToCoreOnlyBeforeText\|RailwayBackend\|RailwayRelaySync\|VaultSealsAndPersists\|SnapshotRestoreRotateAndKeyNeverArchived)' ./internal/aurora ./internal/computer ./internal/vault ./internal/auroraback` | PASS; all six, no removed assertion or new skip. Also included in the full suite. |
| Full Go suite | `go test -json -count=1 -timeout=180s ./...` | PASS: 25 packages; 1,523 passed and 37 skipped test/subtest events; zero failures. Log `go-121738.log`; source manifest SHA256 `352f08f2d930f14a408218e71e502cc5852e228771cec90517ca4c5514de2219`; no source changed during execution. |
| Go static checks | `go vet ./...` | PASS, same source manifest as the full Go suite. |
| Go executable | `go build -o <ignored-audit-output>/neko-aurora.exe ./cmd/neko-aurora` | PASS on final UI sources, `build-122736.log`. |
| File privacy | `go test -v -count=1 ./internal/privatefs ./internal/vault ./internal/auroraback` | PASS; real Windows DACL, creation before first write, replacement, repair of old ACL, rejection of an Everyone-read ACE added by icacls and of hardlinked paths. |
| Rust host | `pwsh -NoProfile -File tools/cargo-windows.ps1 test --locked --manifest-path app/host/Cargo.toml` | PASS 43 tests, zero ignored/failed. Explicit `FIXTURE_BUN_EXE` points to Bun 1.4.0; `FIXTURE_SAVED_CORE_MODULE` points to authored `core/src/adapters/saved-account-bridge.ts`. Includes DPAPI-to-private-stdin-to-Core encrypted capsule fixture. Host/Core sources and helper are unchanged since this run. |
| Tauri compilation | `pwsh -NoProfile -File tools/cargo-windows.ps1 check --locked --manifest-path app/desktop/Cargo.toml` | PASS. |
| Tauri executable linking | `pwsh -NoProfile -File tools/cargo-windows.ps1 build --locked --manifest-path app/desktop/Cargo.toml` | PASS; `rust-desktop-build-122717.log`, final source manifest `ed36ac5863567269cfb03d51655b4ed3f2f531fb7e05ef3e435429b0db0aa62d`, unchanged during run. The earlier successful build during a UI edit was marked STALE and superseded by this run. |
| Browser interactions | `python -u app/tests/aurora/adoption-browser.py` | PASS 21 assertions plus suite completion; desktop 1280×900 and 390×844 viewport. Final `browser-122726.log`, same final source manifest as Tauri build. |
| Durable queue | `node --test app/tests/aurora/outbox-test.mjs` | PASS 4/4 in initial adoption gate; storage implementation unchanged. |
| Cloud browser worker | `node --test tests/*.test.mjs` in `app/cloud-browser`, expanded to eight file arguments | PASS 75/75 in initial adoption gate; worker sources unchanged. |
| Embedded assets | `go test -count=1 ./internal/web` | PASS, also included in full Go suite. |
| JavaScript syntax | `node --check <file>` for each UI script, exact paths in `syntax-123010.log` | PASS. |
| Documentation | `python tools/check_docs.py` | PASS; 15 documents and 112 local links. |
| Candidate secret patterns | Local scan of changed/new files and newly reachable history, values suppressed | No matches across 64 candidate files; no newly reachable history before this checkpoint. Limited to recognizable key/token formats. |

The final UI change after the full Go suite only re-enables the MCP retry button after cooldown. Final browser and Go/Tauri builds cover that change; no Go source changed afterward. Syntax, docs and secret-scan records are in the same audit directory. Manifests hash working-tree bytes; Git text attributes may normalize line endings in a checkpoint.

## Repairs and feature boundaries

The original full Go run had 1,506 pass, six failures and 37 skips. Two Core fixtures wrote Unix shell executables that Windows could not launch. They now invoke the Go test executable as a controlled child process, preserving argument, output and fallback assertions. Railway fixtures now use Git Bash and Python on Windows, stream long commands through stdin and force GNU tar to treat drive-letter paths as local archives. One-pass path mapping also avoids recursively rewriting Linux temporary paths. These are local HTTP/shell fixtures, not a Railway cloud deployment.

Vault and backup previously used POSIX mode checks as Windows privacy assertions. `privatefs` now creates files with a protected DACL granting full access only to the current user's SID before writing sensitive bytes; checks owner, ACE and inheritance through Windows APIs; refuses reparse/hardlinked paths; and repairs existing owned files. POSIX still uses 0600. This does not claim protection from administrators, malware running as the same user, or private parent-directory ACLs. Only Windows execution was performed here.

Rust stable metadata was inconsistent with its installed binaries. The project now pins functional Rust 1.97.1. The Windows helper discovers installed x64 MSVC and SDK libraries and scopes PATH/LIB/INCLUDE to its Cargo process. No lockfile or global toolchain was replaced to obtain a pass. A linked executable does not establish startup, native UI, installer/update or physical-device acceptance.

Form completion is derived from an accepted ledger message with a form-specific nonce. Tests cover no premature completion, retry identity, conflicting answers, failed admission and reload. Browser tests preserve form values through offline reload and lost responses. Existing completion flags from older versions are retained for compatibility.

Remote MCP has service/workspace-scoped caches, serialized initialization, credential-rotation invalidation, typed 401/403/429 handling, bounded Retry-After, explicit reconnect, session synchronization and caller-bounded SSE handshake. Synthetic HTTP tests verify separate workspace credentials/tools, concurrent initialization, rotation, blocked retries, no automatic tool replay and no credential forwarding through HTTP redirects or cross-origin SSE endpoints. Behavior was checked against primary [MCP transport](https://modelcontextprotocol.io/specification/2025-06-18/basic/transports) and [authorization](https://modelcontextprotocol.io/specification/2025-06-18/basic/authorization) documentation. This is not an OAuth discovery/refresh implementation or live provider acceptance.

Two browser attempts failed at initial page loading while checks were concurrent. An isolated diagnostic rendered the same fixture without a JavaScript exception; subsequent full runs passed. The exact cause was not established. Timeout assertions were not relaxed; load-failure diagnostics were added. This remains a test-startup observation, not proof that every timing condition is resolved.

## Review and remaining work

Work and review were performed solo under the user's no-subagent instruction, using pstack Feature and Prove It Works. The initial red probes for tool capacity and utility quota remain in `.audit/2026-10-28-grok/`. New backend behavior, failure paths and browser interactions have executable tests in source. No live accounts, grants, paid models, cloud deployment or push were used.

The complete [820-entry source index](grokbot-adoption-index-2026-10-28.json) remains conservative: eight entries have partial local evidence, one behavior is not adopted, 811 entries are not validated. See the [adoption report](../grokbot-adoption-2026-10-28.md) for remaining atomic group admission, pinned chat references, multi-question cards, provider-specific OAuth, routines/webhooks, document editing, realtime voice, enterprise roles and platform acceptance. Full Core Bun/release gates, Wiii integration, native rendering, APNs/payment/OAuth live acceptance and physical devices were not exercised.
