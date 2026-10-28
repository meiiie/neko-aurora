# AI usage reading recovery

Code commit `9fe8014718e5bb06704ccb2b8a70fd87849ad5e3`, based on `f50841f27d61c88ebd64791058bf8b70e0a090f8`. Solo pstack Feature / Prove It Works / Show Me Your Work and webapp-testing. Adapts the reliability portion of [Grok Bot 0.77.0](https://x.ai/changelog/bot#v0.77.0) to Aurora's existing estimated AI usage page. This is not a billing implementation.

The page retains its last successful reading while loading or after an error, displays the time of that reading, and offers an explicit GET-only retry. Each workspace and period (1/7/30/365 days) has a separate cache; a period with no reading shows unavailable data rather than another period's numbers or fabricated zero. Valid readings survive a browser reload through localStorage. Storage failure retains the reading in this window and reports the loss of persistence.

Every new usage request includes its workspace. The server checks it before reading data, returns workspace/range/estimated markers, and sets no-store; malformed, duplicate or empty query fields are rejected. The browser validates those markers and finite nonnegative integer counts before replacing the prior reading. Latest-view ownership prevents slow period changes, closed sheets or older workspaces from redrawing the page or persisting stale responses. Closing clears the hidden panel. Bot names inside the usage breakdown bypass translation. SW cache v74.

## Validation

Windows amd64; Go 1.27.1, Node 25.9.0, Python 3.13.7/Playwright, Chrome 156.0.8078.49. Temporary stores and loopback HTTP/browser fixtures; no provider/account/billing request. Mobile viewport 390x844 and desktop 1280x900 screenshots were visually inspected.

- RED `usage-scope-020114.log` reproduced unscoped responses, missing no-store and accepted malformed/mismatched queries.
- RED `usage-browser-020151.log` reproduced a failed read removing the previous numbers and leaving no retry.
- **12 Go scope test/subtest events and 12 new browser groups PASS**: read failure, keyboard retry, reload persistence/time labeling, out-of-order period data, no cross-period fallback, malformed server data/cache, close/reopen, workspace change, storage failure, read-only requests and user-defined names.
- **Full Go: 1,961 PASS, 38 SKIP, zero FAIL across 25 packages**; vet/build PASS. **81 targeted browser groups PASS**: 12 usage, 21 general adoption, 27 memory, 12 chat loading and 9 read-state. Syntax and embedded assets PASS on the same final manifest. This gate is scoped to the changed usage/view lifecycle; the preceding memory checkpoint separately records 204 broader browser groups and 8 Node tests. Core/Rust were unchanged and their full gates were not repeated.

| Exact command | Result / log |
|---|---|
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -v -count=1 -run TestUsageWorkspaceScope ./internal/aurora` | PASS `usage-scope-020341.log` |
| `python -u app/tests/aurora/usage-recovery-browser.py` | PASS `usage-browser-020347.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -json -p=1 -count=1 -timeout=300s ./...` | PASS `go-serial-020400.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe vet ./...` | PASS `vet-020758.log` |
| `python -u app/tests/aurora/adoption-browser.py` | PASS `browser-020801.log` |
| `python -u app/tests/aurora/memory-lifecycle-browser.py` | PASS `memory-browser-020828.log` |
| `python -u app/tests/aurora/chat-loading-browser.py` | PASS `chat-loading-020848.log` |
| `python -u app/tests/aurora/read-state-browser.py` | PASS `read-state-020858.log` |
| `python .audit/2026-10-28-grok/syntax.py` | PASS `syntax-020917.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -count=1 ./internal/web` | PASS `web-assets-020919.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe build -o E:\Sach\Sua\neko-aurora\.audit\2026-10-29-usage-recovery\neko-aurora.exe ./cmd/neko-aurora` | PASS `build-020922.log` |

Go commands run in `app/native`; Python/syntax commands run at repository root. Source manifest `e56d7706d3f5419ff515bccfa9bf06b4d5d9cae562c69228b9506b53b0f06525`; 1,815 source files verified against the code commit with existing Git LF normalization. `.audit/2026-10-29-usage-recovery/validation.jsonl` records cwd, UTC timestamps, environment and unchanged before/after source. 6 candidate files and 505 newly reachable history blobs had no recognizable secret matches before the code checkpoint; detection is not exhaustive.

## Cache and compatibility limits

Cache is device-local and contains estimated totals, breakdowns and bot names; it is not encrypted cloud sync or billing authority. There is one persistent key per workspace/period and an in-window map capped at 32 entries. Cached JSON reads/writes are capped at 131,072 UTF-16 code units; arrays/names at 512 entries, keys at 200 code units and names at 300. Cached readings have no expiry; their read time is visible and each opening attempts a refresh. Unavailable storage means persistence across a relaunch is not guaranteed. Closing clears the visible/hidden view but keeps its scoped cache; this is not heap zeroization or deletion from device storage.

Legacy clients may omit workspaceId and retain the old unfenced read behavior. Omitted range still defaults to seven days; explicitly invalid ranges now return 400. Token counts remain the existing character-based estimate. No invoice, payment, billing-provider reconciliation, native relaunch, physical-device or multi-device acceptance is claimed. Existing chart aggregation behavior remains. Review also observed a separate legacy translation issue on bot names in the main conversation list; this checkpoint protects names in the usage breakdown only.

The changelog entry `0.77.0-Fixed-34` (source line 125, SHA256 `68d3fed3d5a0497679f8c7b2a5fa501e146646032a38e40307f7dc9bc2410871`) gains partial evidence for reading recovery. Coverage becomes **21 partial, one excluded and 798 without feature-specific evidence**. No complete upstream parity claim.
