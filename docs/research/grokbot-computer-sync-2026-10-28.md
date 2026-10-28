# Computer file synchronization results

Code commit: `f48c78f625739af3544e224de95099d6acda8857`, based on `e1406548e630f73e501a8cf3fb18081e9417822a`. Local checkpoint, one implementer, pstack Feature / Prove It Works. No Railway account or paid sandbox was contacted.

## Problem and change

The chunk upload path treated a successful HTTP/GraphQL response as a completed remote command. A nonzero exit, absent exit, timeout or truncated result could therefore be followed by the owner's command against an incomplete home directory. The download path could also accept unconfirmed chunk results and replace the local home. A short archive could pass extraction when only its final gzip bytes were missing.

File-transfer commands now require HTTP success, exit code zero and no timeout/truncation. This includes relay push/pull, chunk setup, each uploaded chunk, extraction, archive sizing and each downloaded chunk. A failed push stops before the owner command. Archive sizes must be positive whole integer strings within the existing bound and divisible by four; downloaded chunks must contain exactly the expected number of bytes. Unconfirmed downloads never reach local-home replacement. Decoding and extraction are sequential commands joined by `&&`, so a successful tar process cannot hide a failed base64 decoder. Relay fallback logs no longer include remote stderr, which may contain private file content or URLs.

The [Railway sandbox documentation](https://docs.railway.com/sandboxes) confirms that command results expose exit status separately from transport success, and that `execHttp` reports timeout/truncation. This implementation continues using the existing HTTP transport; it does not claim WebSocket cancellation or session reattachment.

## Evidence

Windows amd64, PowerShell 7.6, Go 1.27.1, Git Bash and Python 3.13.7 for the pre-existing shell fixture. Commands ran with an allowlisted environment excluding provider/account credentials. Logs and exact commands are in `.audit/2026-10-28-railway-sync/validation.jsonl` and `code-checkpoint.json`.

Final source manifest SHA-256: **`81fe9cac9d3e6119aa83bad73d0fdbbe354c76cfe7779f6471a2f76e6581ae7d`**; no source changed during checks or before the checkpoint.

| Command in `app/native` | Result |
|---|---|
| `go test -v -count=1 -timeout=120s -run 'TestRailway(SyncPush\|SyncPull)' ./internal/computer` on pre-fix source with new tests | FAIL as expected: 22 failing child cases; one existing negative-size rejection passed; `sync-faults-152935.log` |
| Same focused command after the initial fix | PASS; `sync-faults-153008.log` |
| `go test -json -count=1 -timeout=180s ./internal/computer` on final source | PASS: 45 test/subtest events, 9 existing skips, 0 failures; 39.47 s; `computer-153114.log` |
| `go vet ./...` | PASS; `vet-153247.log` |
| `go build -o <audit>/neko-aurora.exe ./cmd/neko-aurora` | PASS; `build-153251.log` |

Final Computer tests include the HTTP-failure/success-body case and relay timeout/truncation fallback, plus existing native shell push/pull, idle VM recreation, restart reattachment, sync-cache and relay scenarios. Fault cases assert that no owner command or subsequent sync step runs after a failed push; a failed pull leaves the original local bytes intact. A fixture-only marker in remote stderr verifies log redaction. The GraphQL fault tests never execute shell commands; the existing Railway fixture executes commands only inside its own temporary local directories.

The previous upload checkpoint's full Go suite passed 1,571 events / 37 skips across 25 packages. That full suite was not rerun for these two Computer-only files; the entire changed package, native vet and native build were run instead. No Go race test or production Railway acceptance is claimed. Candidate scan: two files and 165 newly reachable history blobs, zero recognizable secret matches; values were not printed.

## Remaining work

This closes the file-transfer result-validation defect, not the whole remote Computer lifecycle. A dropped owner-command connection, lost change marker, failed pull after a successful command, VM destruction and local edits made while recovery is pending need a separate durable recovery protocol. The current local-home swap is not an OS-crash transaction. HTTP cancellation does not prove that a remote process stopped. These limitations remain in the adoption backlog; no additional changelog entry is marked complete from this patch alone.
