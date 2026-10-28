# MCP HTTP connection lifetime

Code commit: `b8975cc255e8530e51bb648b1c66671f6f0d342e`, based on `8c41f8ffe8d6ff02c44edb76b32039cdb9615575`. Solo pstack continuation of scoped MCP configuration, before OAuth disconnect integration.

Previously, closing a connection canceled only its legacy SSE stream. Streamable HTTP calls continued after removal or service shutdown, old handles could still send tools/notifications, and stale configuration readers could recreate a removed server. Every connection now has a service-bound lifetime; HTTP calls, notifications and response-body reads also obey that lifetime while retaining the individual caller's cancellation. Dropping a connection cancels pending requests and prevents later dispatch through its old handle. An explicit reconnect creates a new handle. Under the connection gate, current bot configuration is rechecked before any credential lookup or network handshake.

Notification handling also checks body-read errors and cancellation instead of returning success after a truncated/canceled response. Caller cancellation of a single request does not itself poison a live shared handle. Existing failure/cooldown and no-tool-replay behavior remains covered by the MCP regression suite.

## Validation

Windows amd64, PowerShell 7.6 and Go 1.27.1. Allowlisted environment excludes live/provider credentials. Tests use actual local httptest HTTP requests and bounded cancellation waits, plus a response-body read probe to make the header/body boundary deterministic. Exact commands, manifests, timestamps and logs: `.audit/2026-10-28-mcp-lifetime/validation.jsonl`.

| Command (app/native) | Result |
|---|---|
| `go test -v -count=1 -timeout=90s -run TestMCPConnectionLifetime ./internal/aurora` | RED `mcp-lifetime-201033.log`: drop/shutdown did not cancel HTTP, old handles sent requests, stale config reconnected. GREEN `mcp-lifetime-201138.log` |
| Expanded lifetime tests | Initial body tests passed but did not deterministically reach body reading. A read probe then reproduced notification false success in `mcp-lifetime-201347.log`; final MCP suite covers the repair |
| `go test -v -count=1 -timeout=90s -run TestMCP ./internal/aurora` | 80 tests/subtests pass, `mcp-201443.log`; includes JSON/SSE/notification body cancellation, caller isolation, closed handle, explicit reconnect and legacy SSE regressions |
| `go test -json -count=1 -timeout=180s ./...` | FAIL `go-201458.log`: computer and ledger packages reached the 3-minute package limit; no failed test assertion. Timeout stacks show a local Railway fixture operation and ledger file work. This is recorded as a failed check, not a pass |
| `go test -json -p=1 -count=1 -timeout=300s ./...` | 1,799 pass, 37 skip, 0 fail across 25 packages; `go-serial-201947.log`, 336.06 seconds. Same source, assertions unchanged; packages run sequentially with a 5-minute per-package limit |
| `go vet ./...`; `go build -o <audit>/neko-aurora.exe ./cmd/neko-aurora` | PASS `vet-202523.log`, `build-202529.log` |

Final MCP/full-Go/vet/build checks share manifest `15e8ca4dca431e1e87ecabec1871332342f686a48fcdbbdca6af22685aa0caa4`; no source changed during checks. 1,781 source files verified against the code commit, allowing existing Git LF normalization. Candidate scan: two files and 334 newly reachable history blobs, zero recognizable secret matches; values not printed.

Cancellation cannot undo a remote effect already dispatched. A canceled tool may have succeeded remotely and requires result reconciliation; it is not automatically replayed. These tests do not prove remote revocation, provider-account acceptance, physical-device behavior or cross-process cancellation. No frontend changes, token grants, deployment or push in this unit.
