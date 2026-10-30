# Scoped, durable MCP configuration

Code commit: `a105fb8560467226f94ae2fb9d5032657b839c2c`, based on `91eb341cda289993846b0af5be19f29a26b13db4`. Solo pstack prerequisite for binding OAuth credentials to the current configuration.

Bot settings now return persistence errors and roll back memory when writing or replacing the settings file fails. Reads and writes copy capability maps and MCP/skill slices. Owner and model-side settings mutations share a per-bot gate, so changing an unrelated setting cannot overwrite a concurrently edited MCP list. MCP updates validate the entire list, including trimmed case-insensitive duplicate names, before touching live connections; existing connection gates are acquired, settings are persisted, and only then are caches closed and removed.

MCP writes and reconnects require the workspace ID and current MCP revision. The revision survives reopening and advances on every MCP-list replacement, including remove/re-add. GET settings exposes both fields. Older clients that omit scope on these operations receive 409 and must reload/update. Unrelated legacy settings requests remain compatible; a supplied mismatching workspace is rejected.

The settings panel captures its workspace and view generation before loading. Closing it, changing workspace, or opening a different settings view invalidates late reads and save/reconnect responses. Writes carry the captured identity, duplicate submissions are disabled while pending, and failed or conflicting saves preserve input with an explicit reload action. Errors receive focus and are scrolled into view. A lost acknowledgement can be checked by GET without resubmitting. The service-worker asset version is v59.

## Validation

Windows amd64, PowerShell 7.6, Go 1.27.1, Node 25.9.0, Python 3.13.7, Chrome 156.0.8078.48 headless. Allowlisted environment excludes live/provider credentials. HTTP/storage fixtures and local browser routes only; no account grant, provider call, deployment or push. Commands, source manifests and exact environment are retained in `.audit/2026-10-28-mcp-config/validation.jsonl`.

| Command | Result |
|---|---|
| `go test -v -count=1 -timeout=90s -run TestMCPConfig ./internal/aurora` | RED `mcp-config-194713.log`: failed save changed memory; returned capability map aliased live state. GREEN `mcp-config-195558.log`: storage failure/reopen, copied input/output, workspace/revision, remove/re-add, duplicate validation, connection preservation and concurrent saves |
| `go test -v -count=1 -timeout=90s -run TestMCP ./internal/aurora` | 70 tests/subtests pass, `mcp-200142.log`, including final case-insensitive duplicate check |
| `go test -json -count=1 -timeout=180s ./...` | 1,789 pass, 37 skip, 0 fail across 25 packages, `go-200157.log`, 171.89 seconds |
| `go vet ./...` | PASS `vet-200449.log` |
| `python -u app/tests/aurora/mcp-discovery-browser.py`; `adoption-browser.py`; `composer-browser.py` | 7, 21 and 23 scenarios respectively; `mcp-browser-200510.log`, `browser-200523.log`, `composer-200552.log` |
| `python -u app/tests/aurora/mcp-config-browser.py` | 10 scenarios pass, final `mcp-config-browser-200725.log`; scoped add/remove/reconnect, duplicate submit, save error/conflict, lost acknowledgement, late reads/saves/reconnect, workspace A→B→A and desktop/mobile viewport |
| `node --check app/native/internal/web/assets/app/app.js`; `go test -count=1 ./internal/web`; `go build -o <audit>/neko-aurora.exe ./cmd/neko-aurora` | Final PASS `app-syntax-200724.log`, `web-assets-200751.log`, `build-200757.log` |

Backend and broader browser checks use manifest `67360e88d14f18b1aaad4d4a3a7f31b23ef96f2bb7e6bb6b90b5ae20d1a652eb`. Final manifest `11ed5d1c3c7885a00e01981c7516d84a1ec9db38f7290c1de871c9f2d0a6a639` changes only the settings error focus/scroll behavior and its browser assertion. All Go sources are unchanged. 1,780 final source files were verified against the code commit, allowing existing Git LF normalization; no source changed during checks.

Initial browser fixture runs failed because navigation reused the same document and attempted input during the existing 400 ms sheet gesture guard; those harness errors were repaired without weakening product assertions. Screenshot review then exposed the error message outside the viewport. A new visibility assertion reproduced this in `mcp-config-browser-200651.log`; the final run verifies both focus and visibility. Mobile and desktop screenshots were inspected locally.

Candidate scan: 12 files and 319 newly reachable history blobs, zero recognizable secret matches; values not printed. This change does not make every extStore writer transactional, coordinate separate processes, prove power-loss durability, stop already dispatched remote side effects or complete OAuth exchange/refresh/disconnect. HTTP connection lifetime cancellation remains a separate follow-up; local configuration consistency is not remote revocation.
