# Spreadsheet artifact checks and stale proof revocation

Code commit: `bf07fbd0c24388a2e092b92dfae6cdbbf9b51f87`, based on `421a7585799fb685483ca03bfc7fa2ff32e12503`. Local pstack Feature / Prove It Works checkpoint; one implementer, no subagents.

The artifact checklist previously counted separators in CSV lines and occurrences of `<row` in XLSX XML. It could reject correctly quoted multiline CSV, accept a malformed row among otherwise consistent rows, or accept a workbook with missing relationships, malformed XML, empty rows or row-like comments. CSV now parses the entire bounded UTF-8/BOM file using Go's CSV reader, supporting comma/semicolon/tab separators and requiring consistent records, at least two columns and two nonempty records. Short reads and files above 2 MiB are not certified.

XLSX now shares the existing `documents` package's archive, XML, relationship, namespace and cell validation. `InspectXLSX` exposes counts from actual parsed cell coordinates without changing `ExtractXLSX`'s public result. At least one linked sheet needs two distinct populated rows; empty rows and line breaks inside cell text cannot inflate that count. Every linked sheet is validated before any summary is returned. Formulas are not evaluated: the verdict identifies formula cells and absent cached values. Existing minimal test workbooks were replaced with valid package relationships and cell addresses.

An integration regression also found that `verify.txt` kept a previous successful result after the artifact changed. The current checklist now writes the revoked result even when no item is complete and even after its in-memory cache is reset. The report is an editable file in the bot home, not an authoritative or tamper-proof receipt; the service derives checklist state from its current checks.

## Validation

Windows amd64, PowerShell 7.6, Go 1.27.1, provider/account variables excluded. Fixtures use in-memory Office ZIPs and actual temporary computer-home files; no spreadsheet application or provider account is invoked.

| Command in `app/native` | Result / local log |
|---|---|
| `go test -v -count=1 -timeout=120s -run 'Test(CheckSheet\|Spreadsheet\|OfficeXLSX\|ChecklistNeedsRealArtifacts)' ./internal/aurora ./internal/documents` before parser fix | FAIL: 14 new subcases demonstrate incorrect acceptance/rejection; `sheets-163436.log` |
| Same command with persistent-proof regression | FAIL: stale success in `verify.txt`; `sheets-163728.log` |
| Same command after clearing in-memory proof cache | FAIL: persisted success survived lost cache; `sheets-164142.log` |
| Focused command on final code | PASS; `sheets-164204.log` |
| `go test -json -p=1 -count=1 -timeout=300s ./...` | PASS: 1,635 test/subtest events, 37 existing skips, 0 failures, 25 packages; `go-serial-163834.log` |
| `go test -json -count=1 ./internal/documents ./internal/documentworker` | PASS: 113 events, 1 existing skip, 0 failures; `documents-163750.log` |
| `go test -json -count=1 -timeout=180s ./internal/aurora` on final code | PASS: 213 events, 24 existing skips, 0 failures; `aurora-164210.log` |
| `go vet ./...` on final code | PASS; `vet-164226.log` |
| `go build -o <audit>/neko-aurora.exe ./cmd/neko-aurora` on final code | PASS; `build-164229.log` |

Full Go/docs source manifest: `028d767f4c2572fa3d4a8e0e3de857698adf465c75f341b5d30c8cfd5af0e48e`. Final focused/Aurora/vet/build manifest: **`202aa984c796c244ee65e5805a9aa254410e7f8de994d9c16b8f30e27cb7e187`**. Only `artifacts.go` and its new spreadsheet regression differ: the final change extends proof revocation to a lost in-memory cache. No source changed during these successful runs; code bytes were matched to the commit. Exact commands/times/manifests are in `.audit/2026-10-28-spreadsheet-validation/validation.jsonl` and `code-checkpoint.json`.

Candidate secret scan: six files and 205 newly reachable history blobs, zero recognizable matches, no values printed. This checkpoint changes no UI; browser/Core/Rust suites were not rerun. The existing Office policy limits remain: 1 MiB package, 8 MiB expanded, 2 MiB XML and 512 KiB extracted text, with external/active content rejected. Files beyond that supported subset remain unverified, rather than being silently accepted from a prefix. Structural checks are not formula correctness, visual render, business accuracy, or a general Excel compatibility claim. No new changelog entry is credited for this hardening; full Pages/Library editing, versioning and export remain in the backlog.
