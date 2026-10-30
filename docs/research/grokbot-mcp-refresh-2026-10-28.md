# MCP cancellation and catalog refresh

Base: `359741b741704bc0e89d1f6c6207ee05e7e22e4c`. This is a local continuation of the connector backlog using pstack Feature / Prove It Works. The implementer reproduced failures, made the change and reviewed it without delegation.

Concurrent connection and catalog requests now share a per-server gate that honors each caller's context. Cancelling a caller no longer imposes a shared failure cooldown on the next caller. Explicit reconnect and configuration updates also honor request cancellation while waiting for that gate. URL parsing failures in legacy SSE return a bounded error instead of dereferencing a nil request.

Catalog loading follows cursors, serializes concurrent refreshes, returns independent copies and detects repeated cursors, duplicate names and malformed schemas. The existing ceiling of 40 exposed tools remains; pagination is bounded at 16 pages and 2,048 bytes per cursor. This is intentionally a bounded catalog, not support for every tool in a larger remote service. Input schemas must be JSON objects, but this change is not a general JSON Schema validator.

`notifications/tools/list_changed` increments the connection's catalog revision. Received notifications invalidate the cache and prompt listing; refresh results that crossed a revision change are rejected. Legacy SSE transport now detects EOF before an endpoint or result arrives. A tool request whose stream disappears returns an unknown-outcome error and is not replayed. Remote cancellation of an already dispatched tool is not guaranteed by closing its transport.

Protocol references: MCP's [tool listing and change notifications](https://modelcontextprotocol.io/specification/2025-06-18/server/tools) and [HTTP/SSE transport semantics](https://modelcontextprotocol.io/specification/2025-06-18/basic/transports). These are the protocol version already requested by Aurora. Notifications are handled on streams the client receives; this change does not add a persistent Streamable HTTP GET subscription, resumable SSE event replay, OAuth discovery/refresh or provider-specific acceptance. TTL expiry and explicit reconnect remain available when a server does not deliver notifications.

## Evidence

The [new loopback tests](../../app/native/internal/aurora/mcp_refresh_test.go) exercise queued cancellation, cancelled-owner recovery, malformed saved URLs, early EOF, live catalog change, interrupted tool calls, eight concurrent readers, deep-copy isolation, pagination cycles and malformed responses. Existing [lifecycle tests](../../app/native/internal/aurora/mcp_lifecycle_test.go) retain credential rotation, workspace isolation, cooldowns, redirects and no side-effect replay.

The red run `mcp-134231.log` reproduced held-gate cancellation, malformed URL panic and incomplete pagination. SSE cancellation after headers already passed on the base and is retained as regression coverage, not claimed as a new fix. The subsequent targeted suite passed. No real connector, model, account or credential was used.

The first whole-Go run reached the ledger package's three-minute aggregate timeout after 1,350 passing test/subtest events, with no failed assertion. The timeout stack was in Windows `CreateFile` while creating a fixture executor record; the currently running test had consumed less than a second. That run remains FAIL in the evidence. A sequential package run uses `-p=1 -timeout=300s` to reduce concurrent filesystem pressure and allow the entire package to finish; it does not change functional assertions or individual caller deadlines.

Exact code manifests, command lines, environment and PASS/FAIL are retained in `.audit/2026-10-28-mcp-refresh/validation.jsonl`; `checkpoint.json` binds the result to the committed code. Environment: Windows amd64, Go 1.27.1, PowerShell 7.6, with live/provider variables excluded. Go's race detector was not run: GCC/Clang was not available through this toolchain's PATH. Concurrent fixture assertions are not a substitute for a race-detector result.

Final source manifest: `3388c923da3d03deb13a42a70afc50540b73a503de173f35cf779a64fafab392`, unchanged during the checks.

| Command in `app/native` | Result |
|---|---|
| `go test -v -count=1 -timeout=90s -run TestMCP ./internal/aurora` | PASS (`mcp-135051.log`) |
| `go test -json -p=1 -count=1 -timeout=300s ./...` | 25 packages PASS; 1,559 passing test/subtest events; 37 existing skips; zero failures; 413.73 s (`go-serial-135109.log`) |
| `go vet ./...` | PASS (`vet-135804.log`) |
| `go build -o <audit-dir>/neko-aurora.exe ./cmd/neko-aurora` | PASS (`build-135813.log`) |

No frontend or Rust source changed in this checkpoint, so their previously recorded gates were not repeated.
