# MCP OAuth must not replay a tool after authentication failure

Code commit: `dbda8cf18bed46a739d7ace20de398911ae27c34`, based on `a0133bf2042124d797582f28a6807ae72bf7380d`. Solo pstack continuation of the [Core OAuth callback repair](grokbot-core-mcp-oauth-2026-10-28.md).

The installed MCP SDK 1.29.0 refreshes authentication and resends the original RPC after 401 or 403 insufficient_scope. The hub's outer no-retry guard did not prevent this inner replay. A real-SDK fixture that records a mutation before returning either status reproduced two tool calls from one application call.

After initialization, the OAuth transport now intercepts 401/403 before the SDK's authentication handler, cancels the response body and returns a static reconnect-required error with unknown request outcome. It does not expose the provider error body. Initial login and initialization retries remain available. The regression verifies that explicit reconnect can refresh credentials, does not replay the old tool call, and permits a subsequent explicitly issued tool call. A held SSE response also verifies that a post-connect authentication failure cannot start a hidden refresh.

## Validation

Windows amd64, PowerShell 7.6, Bun 1.4.0, Node 25.9.0. Isolated test home/config/cache and allowlisted environment; actual MCP SDK and loopback callback with injected service responses, no provider account or browser opened. Final source manifest `a5b43b60f935b620d266f97a46185edaf99f251be28076fd0e9594c963f3c2fd` covers all ten gate checks, with no source changes during execution. 1,775 source files verified against the code commit, allowing existing Git LF normalization. Exact commands/environment/logs: `.audit/2026-10-28-core-mcp-replay/validation.jsonl`.

| Command (core unless stated) | Result |
|---|---|
| `bun --no-env-file test test/mcp-oauth.test.ts` | RED `mcp-oauth-192158.log`: both 401 and 403 performed two mutations. GREEN `mcp-oauth-192235.log`; expanded reconnect/SSE tests PASS `mcp-oauth-192320.log`, 14 tests |
| `bun --no-env-file run typecheck`; `bun --no-env-file run lint` | PASS `typecheck-192329.log`, `lint-192331.log` |
| `bun --no-env-file test test/mcp-oauth.test.ts test/mcp-lazy.test.ts test/mcp-call-safety.test.ts test/mcp-launch-safety.test.ts test/mcp-compose.test.ts` | 38 pass, 0 fail; `mcp-surface-192334.log` |
| `bun --no-env-file test --shard=N/4`, N=1..4 sequentially | 2,186 pass, 42 skip, 0 fail across 221 files |
| `node <core>/bin/neko-source.cjs doctor`; `policy`, from isolated audit workspace | Exit 0, explicit default-profile/missing-configuration/TTY warnings; no OS sandbox health claim |
| `bun --no-env-file run build` | PASS `core-build-193024.log`, including real ConPTY keyboard, ACP and startup/exit/input checks |

Candidate scan: two files and 308 newly reachable history blobs, zero recognizable secret matches. No values printed, deployment or push.

This resolves post-connect SDK authentication replay, not exactly-once behavior at a remote service. The first operation may already have succeeded; its outcome must be reconciled before deciding on another tool call. Pre-connect refresh rotation uncertainty, encrypted Aurora token persistence, Core Windows credential sealing, public-address OAuth transport pinning, shared browser-opener executable identity and real provider acceptance remain separate work. The source changelog's MCP entries remain partially validated.
