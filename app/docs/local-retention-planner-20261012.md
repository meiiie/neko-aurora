# Local retention planner, 2026-10-12

`native/internal/retentionplan` computes a bounded metadata-only preview. It never deletes, archives, restores, exports, changes preferences, schedules work, reads raw text/files, issues transport calls, or executes Core. Every successful `Preview.Enabled` is `false`. The parent owns conversation lifecycle/export and durable preferences, plus the historical pin/effect/group protection facts passed to this planner.

## Interface

```go
type Item struct {
    ID string
    CreatedAt string
    Status string
    ProtectedReasons []string
    TextBytes int
}
type Candidate struct { ID, CreatedAt string; Bytes int }
type Protection struct { ID string; Reasons []string }
type Preview struct {
    Version int
    Days int
    AsOf string
    Enabled bool
    Candidates []Candidate
    Protected []Protection
    TotalBytes int
}
func Plan(items []Item, days int, now time.Time) (Preview, error)
```

Preview fields use lower-camel JSON tags (`version`, `days`, `asOf`, `enabled`, `candidates`, `protected`, `totalBytes`). Candidate/protection fields have the corresponding lower-camel tags. Empty successful candidate/protection collections are non-nil empty arrays. Errors return exactly `Preview{}` and a `*ValidationError{Code ErrorCode}`; no partially computed preview is returned. `errors.As` identifies the typed code, and `errors.Is(err, ErrInvalidInput)` matches every input validation error. `Error()` exposes only `retention_<code>` without IDs, timestamps, reasons or text values.

## Selection and protection

`days=0` keeps every item and supplies `retention_disabled`; `1..3650` uses a cutoff of `UTC(now) - days*24 hours`. Age is measured from the **creation timestamp**, not completion, archive time, last access, or message activity. An item becomes a candidate only when its creation instant is strictly before that cutoff, its exact state is `completed`, `failed`, or `cancelled`, and it has no caller-provided or derived protection reason. An item exactly at the cutoff is protected. Candidate `CreatedAt` and preview `AsOf` are normalized to UTC RFC3339Nano. `TotalBytes` is the sum of candidate `TextBytes`; it is an estimate of candidate text volume, not reclaimed disk bytes or journal compaction size.

All other nonempty valid states are protected with `nonterminal_status`, including queued, active, suspended, paused, cancelling, archived, and unknown states. Future creation timestamps are conservatively protected with `future_created_at`; they are never candidates. Recent items receive `within_retention_window` when retention days are nonzero. Explicit caller reasons are retained. All applicable reasons are deduplicated and sorted, and both candidate/protection lists are sorted by stable case-sensitive ASCII ID. Inputs and returned reason arrays have independent backing storage.

Missing, malformed, out-of-range or unrepresentable timestamps reject the entire input. Accepted timestamps have uppercase `T`, `Z` or an explicit `+/-HH:MM` offset, a real calendar date, and zero through nine fractional second digits. Offset hours/minutes must be within RFC3339 ranges. Timestamp strings are bounded to 64 bytes. A trusted caller supplies `now`; zero or UTC years outside `1..9999` reject. The planner has no wall-clock call and does not infer a trustworthy clock from the browser.

The parent must pass actual durable protection facts for historical context pins, unresolved tool/effect receipts, collaboration/group obligations and other preservation requirements. Omitting a protection reason is not evidence that no dependency exists. Preview generation does not grant authority for deletion, and saved preferences must remain explicitly disabled for this preview-only slice. If the input scope is larger than the fixed limits, show an explicit blocked preview instead of silently truncating it or representing a partial list as complete.

## Input bounds and errors

| Input | Bound |
| --- | --- |
| Items | At most 500 |
| Total `TextBytes` across all items | At most 1,048,576; each is nonnegative |
| ID | 1..128 ASCII bytes from `[-a-zA-Z0-9_:]`, unique within the input |
| Status | 1..64 ASCII bytes from the same token alphabet |
| Caller protection reasons | At most 32 per item |
| Each caller reason | 1..128 valid UTF-8 bytes, no control characters or leading/trailing whitespace |
| Retention days | 0..3650 |

Input byte accounting checks the remaining budget before addition, so even `int` maximum input cannot overflow. The byte bound includes protected items, not only candidates. Reason bounds apply before deduplication, preventing huge duplicate lists from defeating the input budget. Derived system reasons can add at most three distinct reasons per item. No item contains text or file bytes.

Typed codes are `invalid_days`, `invalid_clock`, `item_limit`, `invalid_id`, `duplicate_id`, `invalid_status`, `invalid_created_at`, `invalid_text_bytes`, `text_bytes_limit`, `invalid_protection_reason`, and `protection_reason_limit`. Error choice follows deterministic input validation order; it does not reveal the offending value. No cleanup, scheduler or deletion API is implemented.

## Working-tree validation

Baseline commit: `caaa9f7de0adb25075460e82441082787041cd27`, branch `local-conversations-retention-caaa9f7`, shared owned clone `C:/Users/Admin/Documents/Codex/2026-10-11/task/checkpoint-verify-20261011`. The worker writes only the new `plan.go`, `plan_test.go`, and this document. It does not stage, commit, modify shared ledger/API/UI files, or claim that these results belong to a future exact parent checkpoint.

Environment: Go `go1.27.2 windows/amd64` from `C:/Users/Admin/Documents/Codex/2026-10-11/task/.tooling/go/bin/go.exe`, working directory `<clone>/native`. Provider/credential-shaped environment variables are removed without printing values. `GOPROXY=off`, `GOSUMDB=off`, `GOTOOLCHAIN=local`, `GOENV=off`, `CGO_ENABLED=0`; cache/module/temp/output paths are exclusively under `E:/CodexScratch/conversations-retention-20261012/retention-worker/{cache,mod,tmp}` and its owned logs.

| Command / stage | Result | Evidence |
| --- | --- | --- |
| `go test ./internal/retentionplan -count=1 -v`, permissive scaffold | Expected feature failures, plus an assertion indexing an empty scaffold result; this fixture issue is preserved separately | `red.log` |
| Same command after guarding the assertion, still permissive scaffold | FAIL as intended: selection/protection/bounds/typed errors absent, six top-level tests and three fuzz seeds fail | `red-corrected.log` |
| Same command with implementation | PASS: six top-level unit tests, 24 typed-invalid-input subcases, three fuzz seeds | `green.log` |
| `go test ./internal/retentionplan -count=1 -v`, final code | PASS, exit 0 | `final-test.log`, `final-exits.json` |
| `go vet ./internal/retentionplan` | PASS, exit 0 | `final-vet.log`, `final-exits.json` |
| `go test ./internal/retentionplan -run '^$' -fuzz '^FuzzPlan$' -fuzztime=10s -parallel=2` | PASS, 399,462 executions, exit 0 | `final-fuzz.log`, `final-exits.json` |

These are synthetic deterministic planner fixtures, with no deletion or journal mutation. They are not proof of durable preferences, historical dependency completeness, journal/API/UI integration, export, disk-space recovery, model evaluation, cloud synchronization or phone acceptance. The parent must verify its integration at the exact final code commit. A whole native test-suite run is left to that integration because this delegated worker is constrained to no network/transport; existing native fixtures may start local HTTP servers. Formal race detection is not claimed with CGO disabled.

Source/log hashes, current HEAD, exact commands, environment, exit statuses and a candidate credential-pattern scan that never prints matched values are frozen in the owned scratch `verification-receipt.json`. All these are working-tree results pending the parent's final checkpoint and integration verification.
