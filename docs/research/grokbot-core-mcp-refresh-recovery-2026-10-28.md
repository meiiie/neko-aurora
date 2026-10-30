# Core MCP token recovery before connection

Code commit: `da2c00ca3cb8786e5133ad9a61d53b052d64abb3`, based on `de200026ff42a0c1ebc1a69710e4009cd93ade2c`. Solo pstack continuation after the Aurora OAuth lifecycle unit.

The installed MCP SDK 1.29.0 caught an unknown refresh failure and opened a new browser authorization inside the same connection attempt. Its typed invalid-client/invalid-grant recovery could also invalidate credentials and retry token exchange. The existing post-connect 401/403 guard did not cover these paths.

Before a token POST, Core now atomically replaces the token file with a credential-free consuming record. Success atomically writes ready tokens. Failure before persistence sends nothing and preserves the previous file; after the consuming record, lost response, malformed response or result-save failure cannot reuse the prior code/refresh token on the next provider instance. One connection may attempt only one token exchange, including a second initialization 401 after a successful refresh. State/verifier/redirect and SDK invalidation hooks block same-attempt fallback so the SDK cannot erase the consuming record or silently open another login. A later connection can begin fresh browser authorization with a new callback state/verifier. Existing post-connect tool replay protections remain in place.

The token endpoint comparison uses URL canonicalization on both sides, including hostname case and default port, matching the SDK's URL handling. Issuer checks remain exact. Cancellation already known before token dispatch leaves the previous cache intact and blocks the SDK fallback. File-system errors exposed to callers are static and do not disclose paths or provider details.

The v2 directory remains bound to server/resource/issuer/redirect, and client registration storage is unchanged. Legacy bare token JSON remains readable; the next write migrates to the versioned token envelope. Unsupported or malformed envelopes fail closed. Older binaries cannot read that envelope: rollback requires a compatible reader or a separately reviewed credential reset/migration. This does not add encryption or Windows DACLs to Core's credential files.

## Validation

Windows amd64, Bun 1.4.0, Node 25.9.0; live/provider credential variables excluded, child USERPROFILE/XDG config/npmrc isolated. All OAuth traffic uses an injected fake service with the real installed SDK and actual loopback callback listener. No live grants, provider accounts, deployment or push.

| Command (Core unless stated) | Result / local log |
|---|---|
| `bun.exe --no-env-file test test/mcp-oauth.test.ts test/mcp-lazy.test.ts test/mcp-call-safety.test.ts test/mcp-launch-safety.test.ts test/mcp-compose.test.ts` | PASS: 52 pass, 0 skip, 0 fail; `mcp-surface-212705.log` |
| `bun.exe --no-env-file run typecheck` | PASS: `typecheck-212745.log` |
| `bun.exe --no-env-file run lint` | PASS: `lint-212750.log` |
| `bun.exe --no-env-file test --shard=1/4` | PASS: 474 pass, 19 skip, 0 fail; `shard-1-212755.log` |
| `bun.exe --no-env-file test --shard=2/4` | PASS: 562 pass, 7 skip, 0 fail; `shard-2-213018.log` |
| `bun.exe --no-env-file test --shard=3/4` | PASS: 618 pass, 2 skip, 0 fail; `shard-3-213836.log` |
| `bun.exe --no-env-file test --shard=4/4` | PASS: 546 pass, 14 skip, 0 fail; `shard-4-214216.log` |
| `node <core>/bin/neko-source.cjs doctor (isolated audit workspace)` | PASS: `doctor-214447.log` |
| `node <core>/bin/neko-source.cjs policy (isolated audit workspace)` | PASS: `policy-214450.log` |
| `bun.exe --no-env-file run build` | PASS: `core-build-214453.log` |

Full Core aggregate: 2200 pass, 42 skip, 0 fail across 221 files. Doctor/policy exit 0 retains missing-configuration/default-profile/TTY warnings; build includes Windows ConPTY, ACP and startup/exit checks.

Initial RED `mcp-oauth-211817.log` observed one unexpected browser opening after a lost refresh response; GREEN `211917` covers the repair and a later fresh authorization without reusing that refresh token. Added storage/typed-error/second-401 cases passed in `212105`. Review identified URL normalization bypass; RED `212651` reproduced fallback for an uppercase hostname/default-443 token endpoint. The final focused suite includes this regression.

The earlier full-gate attempt is preserved: shard 1 passed in `212337`; shard 2 (`212557`) was deliberately stopped to add the canonical-endpoint regression, recorded FAIL/exit 4294967295, not passed or counted. The complete gate was restarted on the corrected source.

Final passing checks share source manifest `8714c8ec51d0725765b1325b8974d0260e4086574d75d062bab6c312a9a48071`; 1794 files verified against the code commit with existing Git LF normalization. Exact commands/environment/logs: `.audit/2026-10-28-core-mcp-refresh/validation.jsonl`. Candidate scan: 2 files and 367 newly reachable history blobs, zero recognizable secret matches (not an exhaustive secret-detection claim).

## Limits

The consuming/ready transition is single-file atomic replacement and tested with explicit write faults and reopening. It does not establish power-loss durability, a multi-process lock, provider-side exactly-once behavior, remote revocation or encrypted Windows credential storage. A new connection after an unknown result may require browser sign-in; it never restores the consumed token file. A pre-dispatch write failure is distinguishable locally and leaves the old token available to a later attempt.

This unit does not change SDK client-registration side effects, refresh-token response semantics, DNS/private-network policy, response body limits or the shared system browser opener. Provider compliance, live/phone acceptance and these remaining security/compatibility items remain separate backlog. Grok coverage stays partial.

A subsequent shard 3 run (`shard-3-213234.log`) had 618 pass/2 skip/0 fail, but the harness recorded STALE because Windows PowerShell wrote its generated `ModuleAnalysisCache` into the skill scripts directory. That untracked runtime file was moved into the ignored audit directory. Rerunning shard 3 and the remaining checks with `PSModuleAnalysisCachePath=NUL` passed without source changes; earlier checks used the same source manifest. This harness setting follows [Microsoft’s documented cache switch](https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_windows_powershell_5.1#module-analysis-cache); it does not claim a production helper environment repair.
