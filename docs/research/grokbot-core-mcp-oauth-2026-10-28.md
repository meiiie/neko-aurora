# Core MCP OAuth callback and credential binding

Code commit: `ec5ee166e052a2e2f3191e2cb02295cf09adfc8a`, based on `136ae7f8f6365107cd5483e1dc0d735056cae413`. Solo pstack Feature / Prove It Works / Show Me Your Work. This repairs the existing Core SDK adapter; the Go/Aurora authorization preview remains a separate surface.

The callback previously accepted a code without validating state, endpoint or issuer, and its listener was not restricted to loopback. It now listens on 127.0.0.1, accepts the exact localhost callback host/path and GET method, checks a fresh random state and response issuer, refuses duplicate or mixed code/error fields, limits URL/code size and accepts one response. Issuer is mandatory when advertised by the authorization server. Responses are no-store and never echo authorization codes or provider error details. A received callback is not presented as completed token authorization.

Credential loading now requires resource metadata matching the exact configured resource, an advertised issuer matching authorization-server metadata, HTTPS endpoints and S256 support. New v2 directories are hashed from server name, resource, issuer and redirect URI, preventing sanitized-name collisions and cross-service reuse. Legacy files remain intact and are not silently imported. Client/token records are schema-checked, bounded to 64 KiB and atomically replaced; PKCE verifier is kept only in the current attempt's memory. POSIX mode requests are not a Windows DACL or encryption claim.

Only a genuine SDK authorization redirect enters the browser-wait path. Other connection errors return immediately; timeout, cancellation and errors close the listener/client/transport. Hub shutdown aborts and waits for pending OAuth setup and tracks the resulting transport. Network redirects are rejected and ambient cookies omitted. The existing browser opener replaces the previous cmd/start invocation; this checkpoint does not establish inbox executable identity for that shared opener.

The contract follows the identity, state, PKCE and resource-binding requirements described in the [MCP authorization specification](https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization) and its [security considerations](https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization/security-considerations). Installed MCP SDK version is 1.29.0; no dependency update or proprietary Grok implementation was used.

## Validation

Windows amd64, PowerShell 7.6, Bun 1.4.0 and Node 25.9.0. Runner allowlists environment variables and isolates USERPROFILE, HOME-sensitive test stores, XDG config, npmrc and Bun cache; no real provider credentials, browser or account grant. Actual loopback HTTP callbacks and the installed MCP SDK run against an injected OAuth/MCP service. The tests cover forged/multibyte state, issuer, host/path/method, duplicate parameters, denial, timeout, abort, token error, port release, credential isolation and corrupt/oversized records.

All final checks share source manifest `f0ac1e20b4b0f03234bb4241b17121ded2e7407b86716c2400c848a4302ba86d`; 1,775 source files were checked against the code commit, allowing existing Git LF normalization. No source changed during checks. Exact executable paths, commands, environment, manifests and results are in `.audit/2026-10-28-core-mcp-oauth/validation.jsonl`.

| Command (core unless stated) | Result |
|---|---|
| `bun --no-env-file run typecheck`; `bun --no-env-file run lint` | PASS `typecheck-191233.log`, `lint-191236.log` |
| `bun --no-env-file test test/mcp-oauth.test.ts test/mcp-lazy.test.ts test/mcp-call-safety.test.ts test/mcp-launch-safety.test.ts test/mcp-compose.test.ts` | 35 pass, 0 fail; `mcp-surface-191238.log`, including 11 OAuth tests |
| `bun --no-env-file test --shard=N/4`, N=1..4 sequentially | 2,183 pass, 42 skip, 0 fail across 221 files; logs `shard-1-191310`, `shard-2-191433`, `shard-3-191611`, `shard-4-191810` |
| `node <core>/bin/neko-source.cjs doctor`; `policy`, from isolated audit workspace | Exit 0, explicit WARN for missing model/key/browser/TTY and unconfined default profile; not evidence of a healthy OS sandbox |
| `bun --no-env-file run build` | PASS `core-build-191941.log`: compiled UI probe, real ConPTY keyboard input, ACP smoke, startup/exit ordering and four startup-input cases |

Initial typecheck found the old test constructor/token shape and an SDK metadata union field; a later typecheck/lint caught a nullable assertion and boundary typing. These were fixed before the final gate. No baseline RED is claimed for the callback rewrite. Candidate scan covered four files and 301 newly reachable history blobs with zero recognizable secret matches; matched values were never printed.

## Remaining scope

Strict resource/issuer/S256 requirements can reject older servers. Default callback port remains 41789; a collision fails without taking over another listener. Legacy unbound credentials require a fresh login. Core files are not yet encrypted or DACL-sealed; this is not shared Aurora token persistence, durable refresh-rotation recovery, remote revocation, provider-specific acceptance, physical-device acceptance or power-loss testing. Public-address DNS pinning and bounded OAuth network bodies remain follow-up work. SDK-internal refresh/replay on post-connect 401/403 is a separately identified follow-up; the existing hub-level no-replay behavior alone does not establish that property. Grok MCP backlog entries remain partially validated.
