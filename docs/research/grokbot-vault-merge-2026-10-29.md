# Vault item merge and deletion integrity

Code commit `13e09e265e2ae36ed973c083ca8f19a30f9adb86`, based on `b0c3d6b58a047cf493f9a0523e82a0255ebb3627`. Solo pstack Feature / Prove It Works / Show Me Your Work. Reliability prerequisite for continued Vault adoption.

Previously, SaveItem read metadata, read fields and wrote the merged item in separate lock acquisitions. Two acknowledged edits could each merge against an older copy, losing one another's fields. An update paused before writing could also resurrect an item after a successful deletion. SaveItem now keeps one Vault mutex across reading, merging, validation, sealing and persistence. Typed field reads and deletion inspect metadata and fields in one lock scope. Shared private set/delete helpers retain the existing persistence-failure rollback.

Malformed or JSON-null stored field maps now stop an edit with ErrCorrupt instead of being silently replaced. Fields returns false for those maps; typed deletion reports corruption rather than treating it as a missing record. Empty field inputs still preserve existing values, and the existing 512-byte field / 8192-byte serialized aggregate limits remain. No new model or external-provider action is introduced.

## Validation

Windows amd64, Go 1.27.1, Python 3.13.7/Playwright, Chrome 156.0.8078.49, Node 25.9.0. Synthetic credentials, temporary stores and loopback fixtures only.

- RED `item-merge-021835.log`: lost acknowledged field, resurrection after acknowledged deletion, accepted null field map and silent replacement of malformed stored values.
- **11 focused test/subtest events PASS**, 48 Vault events, 43 Aurora/Vault events and 123 MCP events PASS. The concurrency fixture pauses the first write at AEAD NonceSize; it lets a concurrent request finish or releases the pause after 250 ms, then checks both live and reopened data. This exercises the observed old interleaving and completion of the serialized implementation, not exhaustive scheduling proof.
- Save failures at temporary-file creation and destination replacement retain live fields, metadata, original disk bytes, reopened values and redaction. The aggregate-size test uses escaped control characters to verify the serialized-size limit remains enforced.
- **Full Go: 1,972 PASS, 38 SKIP, zero FAIL across 25 packages**; vet and native build PASS. **44 browser groups PASS** across Vault lifecycle, Vault chat cards and general adoption. Go race detector was not run: this configured Windows environment has no working C race toolchain; synchronized interleaving tests are separate evidence.

| Exact command | Result / log |
|---|---|
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -v -count=1 -timeout=120s -run TestItemMerge ./internal/vault` | PASS `item-merge-021850.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -v -count=1 -timeout=180s -run TestVault|TestItem ./internal/aurora ./internal/vault` | PASS `vault-all-021853.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -v -count=1 ./internal/vault` | PASS `vault-021934.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -v -count=1 -timeout=180s -run TestMCP ./internal/aurora` | PASS `mcp-021937.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -json -p=1 -count=1 -timeout=300s ./...` | PASS `go-serial-021944.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe vet ./...` | PASS `vet-022258.log` |
| `python -u app/tests/aurora/vault-lifecycle-browser.py` | PASS `vault-browser-022301.log` |
| `python -u app/tests/aurora/vault-cards-browser.py` | PASS `vault-cards-022308.log` |
| `python -u app/tests/aurora/adoption-browser.py` | PASS `browser-022319.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe build -o E:\Sach\Sua\neko-aurora\.audit\2026-10-29-vault-merge\neko-aurora.exe ./cmd/neko-aurora` | PASS `build-022343.log` |

Go commands run in `app/native`; browser commands run at repository root. Manifest `b91b847b529fb0789f75025c5646f16811cdfe754f5621cc9fed40e2cf43b171`; 1,817 source files verified against code commit with existing Git LF normalization. Local validation.jsonl records cwd, UTC time, command and unchanged before/after source. 3 candidates and 522 newly reachable history blobs had no recognizable secret matches before code checkpoint; detection is not exhaustive.

## Limits

Serialization covers operations through one Vault instance in one process. Separate handles/processes can still overwrite the same file; there is no cross-process lock, version/CAS contract or durable request receipt. Concurrent writes to the same field remain last-writer-wins. Labels use the caller's supplied label as before. Item mutation and audit-journal append remain separate operations. Existing file-sync/rename behavior is retained; no new crash/power-loss or filesystem durability guarantee is claimed. No live fill/payment/login, model evaluation, Tauri or physical-device acceptance. Browser assets, Core and Rust were unchanged from the preceding checkpoint; their full gates were not repeated. Changelog coverage remains **21 partial, one excluded and 798 without feature-specific evidence**.
