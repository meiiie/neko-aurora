# Core MCP OAuth response bounds and cancellation

Code commit: bbc1a4553c593556be7a672773157effff6d4580, based on `54d44378084685ee214e8d1f5ebd4453b9c00c68`. Solo pstack Feature / Prove It Works / Show Me Your Work continuation. The installed MCP SDK 1.29.0 calls `Response.json()`/`text()` for discovery, registration, token and resource-error responses without a local body cap. The existing connection deadline aborted fetch, but did not interrupt an injected body reader that ignored that signal.

OAuth metadata GET responses now allow at most 1 MiB; registration/token POST responses and unsuccessful HTTP responses allow at most 64 KiB. A fixed-size buffer counts actual decoded bytes, independently of absent, invalid or understated Content-Length. An oversized declared length declines before reading. Cancellation and the connection deadline interrupt a pending read; rejection cancels/releases the original reader without waiting for an untrusted cancellation promise to settle. Bounded-read error text is fixed and excludes upstream bodies and abort reasons. A bounded-read failure aborts the connection so SDK metadata fallback cannot continue making requests. Status, headers and response URL are preserved for the SDK.

Successful MCP resource JSON and SSE responses retain their existing streaming behavior and limits. The adapter distinguishes SDK resource requests by the configured canonical URL and the SDK's SSE Accept header. OAuth requests use JSON/form request headers, including when endpoints share the resource URL. A regression also exposed registration being mistaken for a token attempt when registration and token URLs coincide; the provider now recognizes the SDK JSON registration request. A real-SDK fixture uses one URL for resource/registration/token, receives a tool result larger than 1 MiB and a notification after more than 1 MiB of SSE data, and still performs exactly one token POST.

Token responses that fail after dispatch leave the existing `consuming` record. They neither reuse the refresh token/code nor open a fallback login. Pre-dispatch cancellation keeps the existing behavior. This change does not add a retry or another authentication loop.

## Validation

Windows amd64; Bun 1.4.0; Node 25.9.0; PowerShell 7.6. Child USERPROFILE/XDG/npmrc isolated, allowlisted environment excludes provider credentials, PSModuleAnalysisCachePath=NUL. No external provider/account, browser sign-in, grant or paid request was used. Tests use injected responses and a real local callback listener with the installed SDK.

- Initial pending-body probe (`mcp-oauth-232040`) did not finish after its 2-second connection deadline. Its own Bun child was stopped at about 60 seconds; the FAIL record and unchanged source manifest are retained. The finite-body RED run (`232141`) reports all five new over-limit stages failing on the old code: metadata/issuer/registration/token proceeded successfully; resource challenge consumed the oversized body instead of canceling at the bound.
- First repair (`232222`) passes those five stages. Expanded tests (`232343`) find the shared registration/token endpoint bug; fixed in `232500`. Final buffer allocation and bounded SSE-fixture wait changes are covered by the final manifest below.
- Final OAuth suite: 50 PASS across two files (39 SDK/callback/cache cases plus 11 reader boundary cases). This adds 22 tests to the previous Core suite; the separate broader MCP run includes the lazy/call/launch/compose suites and reports 63 PASS across five files.
- Full Core suite: **2228 PASS, 42 SKIP, 0 FAIL across 223 files**. Shards: 611/8, 732/2, 470/26, 415/6 (PASS/SKIP). Typecheck/lint, doctor/policy and production build including real ConPTY keyboard input, ACP smoke, startup lifecycle and four startup/input scenarios all PASS. Doctor/policy retain explicit warnings for the isolated profile, missing credentials/TTY and unconfined auto mode; exit 0 is not proof of a sandbox.

| Check | Exact command | Result / log |
|---|---|---|
| mcp-oauth | `C:/Users/Admin/AppData/Roaming/npm/node_modules/bun/bin/bun.exe --no-env-file test test/mcp-oauth.test.ts test/mcp-oauth-http.test.ts` | PASS — `mcp-oauth-232604.log` |
| mcp-surface | `C:/Users/Admin/AppData/Roaming/npm/node_modules/bun/bin/bun.exe --no-env-file test test/mcp-oauth.test.ts test/mcp-lazy.test.ts test/mcp-call-safety.test.ts test/mcp-launch-safety.test.ts test/mcp-compose.test.ts` | PASS — `mcp-surface-232606.log` |
| typecheck | `C:/Users/Admin/AppData/Roaming/npm/node_modules/bun/bin/bun.exe --no-env-file run typecheck` | PASS — `typecheck-232641.log` |
| lint | `C:/Users/Admin/AppData/Roaming/npm/node_modules/bun/bin/bun.exe --no-env-file run lint` | PASS — `lint-232644.log` |
| shard-1 | `C:/Users/Admin/AppData/Roaming/npm/node_modules/bun/bin/bun.exe --no-env-file test --shard=1/4` | PASS — `shard-1-232646.log` |
| shard-2 | `C:/Users/Admin/AppData/Roaming/npm/node_modules/bun/bin/bun.exe --no-env-file test --shard=2/4` | PASS — `shard-2-232920.log` |
| shard-3 | `C:/Users/Admin/AppData/Roaming/npm/node_modules/bun/bin/bun.exe --no-env-file test --shard=3/4` | PASS — `shard-3-233123.log` |
| shard-4 | `C:/Users/Admin/AppData/Roaming/npm/node_modules/bun/bin/bun.exe --no-env-file test --shard=4/4` | PASS — `shard-4-233303.log` |
| doctor | `node E:\Sach\Sua\neko-aurora\core\bin\neko-source.cjs doctor` | PASS — `doctor-233354.log` |
| policy | `node E:\Sach\Sua\neko-aurora\core\bin\neko-source.cjs policy` | PASS — `policy-233355.log` |
| core-build | `C:/Users/Admin/AppData/Roaming/npm/node_modules/bun/bin/bun.exe --no-env-file run build` | PASS — `core-build-233356.log` |


Exact commands, cwd, start time, base, exit code and source hashes: `.audit/2026-10-28-oauth-body/validation.jsonl`. Source manifest: `b1adb9c9b110bb4b8f2b9bf6c8518adf7adbf01a10dd76d347a3e567f80ed968`. `code-checkpoint.json` verifies all 1802 source files against commit `bbc1a4553c593556be7a672773157effff6d4580` (existing Git LF normalization accounted for). Before checkpoint, a redacted scan covered 4 candidate files and 418 newly reachable history blobs with zero recognizable secret matches; this is not exhaustive secret detection.

## Scope and remaining work

The bounds apply to bytes delivered by Fetch (after runtime decompression), not all network/TLS/header allocations. One supplied stream chunk can already exist before the adapter rejects it. Successful resource data/SSE keep their separate pre-existing behavior; this is not a global MCP result limit. SDK classification is tested against the pinned 1.29.0 request shapes and should be revisited on SDK upgrades. Registration still uses the SDK's existing dynamic-client flow; this is not proof of general DCR/CIMD or live-provider compatibility. Limits are deliberately finite and can reject unusual provider metadata or tokens larger than the stated budgets.

Core still needs public-DNS/dial pinning for OAuth, Windows credential sealing/ACL hardening, cross-process token exclusion and provider-specific acceptance. This unit does not claim SSRF protection, remote revocation, power-loss durability or model evaluation. No Rust/Go/frontend code changed, so their full gates were not repeated. The two existing MCP changelog entries remain partially validated; no additional source bullet is credited merely for this hardening.
