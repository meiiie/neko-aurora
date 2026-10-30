# Memory persistence acknowledgement and recovery

Code commit `7cd0cfe417336fb13c42ef37c736d987d248b83a`, based on `0bb9118b3057a0979a3c06264aadcba7be9b6a5e`. Solo pstack Feature / Prove It Works / Show Me Your Work. This supports the memory/reliability backlog; no additional changelog item is claimed complete.

Add, edit, delete and reviewed-summary replacement now build an independent next list, persist it, and only then append best-effort history. Failed persistence restores RAM, leaves the original facts intact and returns a generic storage error through the owner API. Import uses the same commit helper. Duplicate adds retain their existing no-op behavior. Summary apply validates the entire supplied list, including capacity, duplicates and unsafe facts, without silently dropping entries. Owner mutation APIs reject read-only ledgers.

Unreadable, malformed or JSON-null memory files no longer appear as a healthy empty store or get overwritten by a later write. Owner reads, mutations, ask/summarize and import report an unavailable store. Recovery requires restoring a trusted valid file and reopening the store; no automatic repair is implemented. The agent prompt path still reads an empty list after this load failure, so the owner-facing diagnostic is not claimed universal.

## Validation

Windows amd64; Go 1.27.1, Python 3.13.7/Playwright, Chrome 156.0.8078.49. Temporary ledgers/files, local HTTP fixtures and stub providers only. No real provider request.

- RED `memory-persistence-011850.log` reproduced RAM/history drift, successful add/summary acknowledgements despite storage failure, misleading delete 404 and writable owner APIs over a read-only ledger.
- Review RED `memory-persistence-012138.log` reproduced JSON-null reads returning 200 followed by a nil-map write panic. The store-load guard fixes this failure.
- **33 new focused test/subtest events PASS**, covering write/rename failures, private/team eviction, unchanged original/reopened data and history, API errors, read-only behavior, invalid summary lists, successful retry, concurrent writes on one store, and corrupt-store refusal. **41 memory/utility tests PASS**.
- **Full Go: 1,922 PASS, 38 SKIP, zero FAIL across 25 packages**; vet and build PASS. **35 browser groups PASS** (14 memory lifecycle plus 21 general adoption). Core and Rust sources are unchanged; their full gates were not repeated.

| Exact command | Result / log |
|---|---|
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -v -count=1 -timeout=120s -run TestMemoryPersistence ./internal/aurora` | PASS `memory-persistence-012321.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -v -count=1 -timeout=180s -run Test(Memory|Mem|TeamMemory|UtilityQuota) ./internal/aurora ./internal/hub` | PASS `memory-all-012332.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -json -p=1 -count=1 -timeout=300s ./...` | PASS `go-serial-012340.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe vet ./...` | PASS `vet-012719.log` |
| `python -u app/tests/aurora/memory-lifecycle-browser.py` | PASS `memory-browser-012722.log` |
| `python -u app/tests/aurora/adoption-browser.py` | PASS `browser-012737.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -count=1 ./internal/web` | PASS `web-assets-012802.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe build -o E:\Sach\Sua\neko-aurora\.audit\2026-10-29-memory-persistence\neko-aurora.exe ./cmd/neko-aurora` | PASS `build-012807.log` |

Go commands run in `app/native`; Python browser commands run at the repository root. Source manifest `6a5bcaf2c49bc9cf075571528b2c2416a38f879d2786ea88ae68942bb89270ef`; `code-checkpoint.json` verifies 1,811 source files against the code commit with existing Git LF normalization. `.audit/2026-10-29-memory-persistence/validation.jsonl` records exact cwd, UTC timestamps, environment and unchanged before/after manifests. Four candidate files and 478 newly reachable history blobs had no recognizable secret matches before the code checkpoint; detection is not exhaustive.

## Limits and next work

Facts and history remain separate files; history is best-effort, not an atomic pair. No fsync/power-loss, cross-process locking, durable mutation receipt, race-detector or general exactly-once claim. Concurrent-write coverage uses the same store mutex. A duplicate add retry does not establish general request idempotency. Read-only checks cover the owner mutation routes, not all internal bot methods.

Positional CRUD and summary apply still need revision checks, and already-sent requests need a server workspace fence; these are the next unit. UI error text remains generic. No cloud sync, model-quality, provider, Tauri or physical-device acceptance is claimed. Changelog coverage remains 19 partially validated entries, one excluded behavior and 800 entries without feature-specific evidence.
