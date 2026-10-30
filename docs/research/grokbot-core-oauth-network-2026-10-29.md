# Core MCP OAuth public endpoint transport — 2026-10-29

Code commit `0260fb0c781315f06cc970b197db0087b7f72079`, based on `d3445f6097798cd0343c6c18947106f1bde4dede`. Solo pstack Feature / Prove It Works / Show Me Your Work and self-review. Continues [process exclusion](grokbot-core-oauth-exclusion-2026-10-29.md) and [Windows credential storage](grokbot-core-oauth-storage-2026-10-29.md).

## Behavior

Production OAuth discovery, registration and token exchanges now use a narrow HTTPS adapter. The existing public-IP classifier rejects local and special-purpose literals and DNS answers containing any non-public or inconsistent address. The transport selects one checked address and supplies it through the HTTPS lookup callback, retaining the original URL host and TLS verification. It makes one connection attempt; redirects and alternate-address retries are refused. Configured resource RPC/SSE retains its existing streaming transport.

Only GET and POST with string/URLSearchParams bodies are supported. Requests cap at 64 KiB, headers at 32 KiB, successful GET responses at 1 MiB, and POST/error responses at 64 KiB. Declared and streamed response lengths are checked. A thirty-second request budget combines with caller cancellation, including DNS resolution. Responses must use identity encoding; compressed responses fail closed. Supplied cookies, proxy authorization, Host and transport-framing overrides are removed; normal explicit OAuth Authorization remains supported. Each request uses a fresh agent-free connection.

Existing consuming-token claims precede dispatch, so ambiguous failures still require explicit reconnect. Error output does not include endpoint/body/abort details. The shared public HTTP GET adapter retains its original behavior; only its resolver is exposed for reuse.

## Validation

Windows amd64, PowerShell 7.6, Bun 1.4.0, Node 25.9.0 and Python 3.13.7. Allowlisted environment, isolated HOME/USERPROFILE/XDG/npmrc, synthetic records, no real account/grant/provider request, push or deployment.

- Production-routing regression FAILS on the old default fetch path: discovered loopback metadata reached the transport. It passes after routing non-resource requests through the public endpoint adapter.
- Final network/public-HTTP suite: **29 PASS / 0 FAIL**. Boundary cases cover alternate loopback forms, mixed DNS, malformed endpoints, unsupported/oversized bodies, canceled DNS, a changed resolver answer, header handling, one POST, lost response, redirects, declared/streamed limits, compressed responses, pre-header/mid-body cancellation, deadline and exact-size/status preservation.
- Loopback HTTP exercises actual request/response socket lifecycle through the injected transport seam. A separate native HTTPS probe confirms this Bun runtime invokes the custom lookup for an otherwise unresolvable hostname; its TLS handshake is intentionally rejected. These checks do not prove live provider certificate-chain/SNI interoperability.
- The initial abort test stalled because Bun's rejection matcher waited before the fixture could issue abort. A trace confirmed it had not reached the ready/abort step. Reordering the fixture fixes it with the original native cancellation path; no deadline was increased, and the temporary extra abort wrapper was removed.
- SDK OAuth/response suite: **50 PASS / 0 FAIL**, including large RPC/SSE, shared endpoint URLs, PKCE, refresh rotation, error fences and no mutation replay.
- Full Core: **2262 PASS / 42 SKIP / 0 FAIL across 227 files**; sequential shard PASS/SKIP totals: 622/8, 735/2, 484/26, 421/6. Typecheck, lint, doctor, policy and production build PASS. Compiled ConPTY/ACP/startup checks are included in build; environment warnings remain explicit.

Final source manifest `cfae1608e591bd5150ce5e1ae09f15d7013928f67dd3592fc14c38d6184bb311`. Exact commands, base, environment, unchanged manifests, logs and decisions.tsv are in `.audit/2026-10-29-oauth-network/validation.jsonl`. Commands from core: `bun --no-env-file test test/mcp-oauth-network.test.ts test/public-http.test.ts`, the existing OAuth/response suite, `bun --no-env-file run typecheck`, `bun --no-env-file run lint`, `bun --no-env-file test --shard=N/4` for N=1..4, doctor, policy and `bun --no-env-file run build`. Doctor/policy run the absolute source launcher from an isolated workspace. All 1860 source files were verified against the code commit with existing LF normalization accounted for. The recognizable-secret scan checked four candidates and 692 newly reachable history blobs with zero matches; it is not exhaustive. The unrelated adoption-document edit remains excluded.

## Compatibility and remaining scope

Private-network authorization servers, proxy-required OAuth endpoints, compressed responses and redirect-dependent endpoints are intentionally unsupported by this adapter. No connection fallback occurs after a POST. Browser authorization navigation and the owner-configured MCP resource stream are separate transports and are not claimed to be DNS-pinned here. An injectable fetch override remains a trusted test seam. Live provider consent, registration, TLS, revocation and device acceptance remain unvalidated. No extra changelog bullet is marked complete.

Implementation references: [Node HTTPS request](https://nodejs.org/api/https.html#httpsrequesturl-options-callback), [Node HTTP request](https://nodejs.org/api/http.html#httprequesturl-options-callback), and [MCP security guidance](https://modelcontextprotocol.io/docs/2025-11-25/tutorials/security/security_best_practices). The adapter keeps normal TLS identity verification while pinning DNS results, consistent with the cited connection options and SSRF guidance.
