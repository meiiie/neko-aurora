# MCP authorization discovery and preview

Code commit: `383d94b0b00826395e6321154a4215a79462a1eb`, based on `b405992e5e4c6080a3cdc6a4e970626825f839e4`. Solo pstack Feature / Prove It Works / Show Me Your Work. This is the discovery/preview unit of OAuth adoption, not a completed account connector.

## Reference and behavior

Checked the current official [MCP authorization specification](https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization) on 2026-10-28. [Discovery requirements](https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization/authorization-server-discovery) require protected-resource metadata, OAuth/OIDC metadata discovery and exact issuer binding. [Client registration](https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization/client-registration) now prioritizes existing registration or client-ID metadata documents; dynamic registration is deprecated. The [security requirements](https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization/security-considerations) require verification of PKCE capability before authorization. No reference code/assets copied.

MCP 401/403 failures retain a bounded parsed Bearer challenge internally, without exposing its raw contents through failure JSON. Multiple header lines, quoted commas/escapes, case-insensitive schemes and empty list members are handled; duplicate parameters, ambiguous Bearer challenges and malformed scopes fail closed. Advertised metadata takes precedence; otherwise discovery tries endpoint-path then root well-known resource metadata. Scope from the challenge takes precedence over resource defaults, including an explicit empty scope.

Protected-resource metadata must match the configured resource exactly. Each advertised issuer is validated; multiple issuers require owner selection. OAuth metadata, OIDC path insertion and OIDC path appending are supported; the returned issuer must match exactly. The preview reports S256 and client-ID metadata capabilities. It does not register clients, create a grant, exchange tokens, alter existing credentials, reconnect a session or retry tools.

Every metadata fetch uses a separate public-HTTPS-only client, with no Vault credential or cookie jar. The dialer blocks private and special-purpose IP ranges after DNS resolution. Redirects, non-200 responses other than missing-endpoint 404/405, documents over 64 KiB, duplicate top-level fields and invalid UTF-8/JSON are rejected. At most 16 issuers and 64 scopes of 128 bytes are accepted; scope challenges are bounded to 16 headers / 8 KiB. A 12-second overall discovery context bounds sequential probes. Errors are static and do not echo provider bodies.

The owner-triggered preview endpoint requires exact workspace and configured server URL, serializes discovery for that configuration and rechecks the configuration after fetching metadata. The UI shows service, requested scopes and selected issuer, with an explicit action for each issuer choice. Closed panels and changed workspaces ignore late responses; failures clear stale results and allow explicit retry. It states that no account permission has been granted. PWA cache v58 includes the preview module.

## Validation

Windows amd64, PowerShell 7.6, Go 1.27.1, Node 25.9.0, Python 3.13.7, Chrome 156.0.8078.48. Allowlisted environment excludes provider/account credentials. Discovery uses an in-memory fake HTTP transport; a TLS loopback fixture verifies private-address dial rejection. No live provider requests. Exact records: `.audit/2026-10-28-mcp-oauth/validation.jsonl`.

| Command (Go commands in app/native) | Result |
|---|---|
| `go test -v -count=1 -timeout=90s -run 'TestMCP(Authorization\|Challenge\|Discovery)' ./internal/aurora` | Integration RED before failure capture/route wiring: `mcp-discovery-183951.log`; GREEN `mcp-discovery-184012.log`. Expanded config-race and scope checks PASS `mcp-discovery-184330.log`. RED empty header list member lost scope: `mcp-discovery-184806.log` |
| `go test -v -count=1 -timeout=90s -run TestMCP ./internal/aurora` | Final PASS: 60 tests/subtests, `mcp-184837.log`; includes parser fix, root/path metadata variants, issuer/resource checks, invalid metadata, HTTP failure policy, credentials/cookies, cancellation, private-address denial, workspace/config changes |
| `go test -json -p=1 -count=1 -timeout=300s ./...` | PASS: 1,743 test/subtest events, 37 existing skips, 0 failures, 25 packages; 191.62 s; `go-serial-184427.log` |
| `python -u app/tests/aurora/mcp-discovery-browser.py` | PASS: 7 scenarios, `mcp-browser-184341.log` |
| `python -u app/tests/aurora/adoption-browser.py` | PASS: 21 existing scenarios, `browser-184352.log` |
| `go test -count=1 ./internal/web` | PASS `web-assets-184417.log` |
| `go vet ./...`; `go build -o <audit>/neko-aurora.exe ./cmd/neko-aurora` | Final PASS: `vet-184843.log`, `build-184847.log` |

Full-Go, 28 browser scenarios, asset and initial vet/build manifest: `03b36032911b9e28e4a3b55db40b2d68783b1ddd0d9fba744d2680e739cfefe9`. The final parser guard and extra tests use manifest `ae5ea42b3010e0102edc42d9105b721b970662e94e913e49eeb19c7072b5f40b`, validated by all MCP tests, vet and build; no frontend changed after the browser run. No source changed during any check. Reviewed 390 × 844 screenshot. Candidate scan: 11 candidate files and 283 newly reachable history blobs; zero recognizable secret matches, no values printed.

## Limits and next unit

Discovery is owner-initiated and currently restricted to public HTTPS even in desktop mode; existing manually configured local MCP transport remains separate. Resource and issuer URLs with query/fragment are outside this implementation's discovery contract; metadata/authorization/token endpoint queries are supported. No fallback after an advertised metadata failure, authorization/transient status or invalid document. Exact resource/issuer strings are used without normalization. Metadata is not a verified identity attestation or a trustworthy claim about requested privileges. No token, callback, PKCE exchange, refresh rotation, encrypted OAuth recovery, scope upgrade or revocation is implemented by this checkpoint. The MCP transport protocol negotiation remains at its existing version.

In-memory transports, loopback rejection and browser fixtures are not provider/account, TLS deployment, native app or physical-device acceptance. No Go race run with the installed C toolchain. No grants, provider registration, deployment or push. Existing partial MCP index entries remain partial.
