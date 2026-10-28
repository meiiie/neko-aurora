# Windows system browser opener for OAuth and support links

Code commit: `fd822355b0fe60fa4363d695b9b7b43955942079`, based on `2b79ad21d27cc16ccb9c602b00b534389167ad39`. Solo pstack continuation of the Core MCP/OAuth backlog.

The shared opener used a bare `rundll32` and relative `url.dll`, so its first executable/library resolution depended on the environment and current directory. The process also inherited the full ambient environment. The regression intercepts process creation (no browser is launched), places inert shims in cwd/PATH, and observes the previous bare executable name.

The opener now accepts absolute HTTP(S) links, rejects control/whitespace, embedded user/password credentials and other protocols, and preserves encoded OAuth parameters through URL parsing. On Windows it resolves both files under System32, passes the absolute DLL plus `FileProtocolHandler`, launches with System32 as cwd without a shell, and uses the existing minimal system environment plus USERPROFILE/APPDATA/LOCALAPPDATA needed by a default browser. Missing files and a comma in the DLL path decline the launch instead of searching elsewhere. Detached/hidden behavior and the best-effort void API are retained; asynchronous launch errors do not start another login attempt. macOS/Linux keep their existing opener command selection, with the same URL validation.

## Validation

Windows amd64; Bun 1.4.0, Node 25.9.0, PowerShell 7.6. Test homes/config/npmrc isolated, provider credential environment excluded, PowerShell module cache disabled in the harness. Six focused tests cover cwd/PATH shadowing, exact URL arguments, minimal environment, missing executable/library, ambiguous DLL syntax, invalid/non-web URLs, and asynchronous child failure. OS executable and DLL presence were checked, but spawn is mocked. No real browser, provider account, live grant or external deployment/push.

| Command | Result / audit log |
|---|---|
| `C:/Users/Admin/AppData/Roaming/npm/node_modules/bun/bin/bun.exe --no-env-file test test/browser-opener.test.ts` | PASS `opener-221912.log` |
| `C:/Users/Admin/AppData/Roaming/npm/node_modules/bun/bin/bun.exe --no-env-file run typecheck` | PASS `typecheck-221913.log` |
| `C:/Users/Admin/AppData/Roaming/npm/node_modules/bun/bin/bun.exe --no-env-file run lint` | PASS `lint-221916.log` |
| `C:/Users/Admin/AppData/Roaming/npm/node_modules/bun/bin/bun.exe --no-env-file test test/browser-opener.test.ts test/chatgpt-auth.test.ts test/cline-auth.test.ts test/grok-auth.test.ts test/kimi-auth.test.ts test/opencode-auth.test.ts test/mcp-oauth.test.ts test/windows-system.test.ts test/browser-bridge.test.ts` | PASS `auth-221919.log` |
| `C:/Users/Admin/AppData/Roaming/npm/node_modules/bun/bin/bun.exe --no-env-file test --shard=1/4` | PASS `shard-1-221924.log` |
| `C:/Users/Admin/AppData/Roaming/npm/node_modules/bun/bin/bun.exe --no-env-file test --shard=2/4` | PASS `shard-2-222106.log` |
| `C:/Users/Admin/AppData/Roaming/npm/node_modules/bun/bin/bun.exe --no-env-file test --shard=3/4` | PASS `shard-3-222240.log` |
| `C:/Users/Admin/AppData/Roaming/npm/node_modules/bun/bin/bun.exe --no-env-file test --shard=4/4` | PASS `shard-4-222419.log` |
| `node E:\Sach\Sua\neko-aurora\core\bin\neko-source.cjs doctor` | PASS `doctor-222627.log` |
| `node E:\Sach\Sua\neko-aurora\core\bin\neko-source.cjs policy` | PASS `policy-222628.log` |
| `C:/Users/Admin/AppData/Roaming/npm/node_modules/bun/bin/bun.exe --no-env-file run build` | PASS `core-build-222629.log` |

The final Core suite passed **2,206 tests, 42 skipped, zero failed across 222 files**. The focused auth set passed 76 tests in 9 files. Build includes real Windows ConPTY input, ACP and startup lifecycle/input probes. All final gates share source manifest `89160b1dd32e6d0e7d2e90d84f91d3ba20fc202bdde857f07ef3251c14189a84`; 1,797 files were verified against the code commit with existing Git LF normalization. Exact cwd/environment/commands and retained failures are in `.audit/2026-10-28-browser-opener/validation.jsonl`. Candidate scan covered 2 files and 387 newly reachable history blobs, with zero recognizable secret-pattern matches. Go/Rust gates were not rerun for this Core-only change.

RED `opener-221658.log`: all five initial cases failed against the prior implementation (bare executable, unsupported URL dispatch, and no refusal of missing/ambiguous system files). Initial GREEN `221750` passed those cases; typecheck `221751` failed on the test's nullable expected path and was fixed with a runtime presence check. Adding valid encoded/loopback links brought the total to six. Lint `221841` rejected a conditional empty-object spread; explicit process-option construction passed in the final gate. These failures are retained in the audit log.

## Compatibility and limits

`openBrowser` now declines non-HTTP(S), relative, control/whitespace-bearing and credential-bearing URLs. Existing auth/support call sites use web links; Chromium extension internal pages use the separate direct-browser path. The opener still returns void and cannot prove that the default browser displayed a page or completed sign-in. This is not actual browser-association acceptance on Windows, macOS or Linux.

This boundary relies on the host's SystemRoot/Windows installation and configured default browser; it does not add signature verification, an OS sandbox, or control over the browser's registry association and stored profile. Core token files remain unsealed on Windows, and OAuth network public-address pinning/body limits/provider contracts remain separate backlog. No new upstream changelog item is marked complete.
