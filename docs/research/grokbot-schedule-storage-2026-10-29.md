# Routine store preservation — 2026-10-29

Code commit `3a7868f2d25534611361217ec5206f9e848b2e21`, based on `e47a25253fd0846e64734a993566ba95797303b3`. Solo pstack Feature / Prove It Works / Show Me Your Work, with self-review.

## Behavior

A missing initial schedules.json still means no routines. An unreadable, oversized, nonregular, malformed, partially decoded, unknown-field or invalid-timing store now returns HTTP 503 and refuses mutations/new dispatch. Whole-file validation rejects duplicate IDs and invalid repeat/time/date values before publishing any records to memory. Broken bytes are preserved. Legacy null/empty arrays remain accepted; unknown timezones remain editable through the existing repair path.

Routine publication now reuses privatefs.WriteFile: random exclusive private temporary file, write/sync/close and rename. Failed removal, occurrence progress and completed-legacy cleanup restore their original in-memory list. Read-only ledgers cannot advance routine progress. Work already admitted to the durable ledger is still awakened when saving routine progress fails; the existing occurrence nonce/receipt prevents repeated admission on retry.

The schedule list reports storage unavailability through the existing load-error UI. A failed save retains the editor draft. A corrupt store remains unavailable until its file is repaired and the service restarts; the UI reload button does not repair disk corruption. There is no new automatic recovery/reset command.

## Validation

Windows amd64, Go 1.27.1, Python 3.13.7, Node 25.9.0 and Chrome 156.0.8078.49. Go uses an allowlisted environment and isolated HOME/USERPROFILE/XDG/npmrc. Browser tests preserve the OS USERPROFILE only for Chrome startup and use a new temporary Playwright browser profile. Synthetic temporary stores and loopback HTTP only, without provider credentials, grants, push or deployment.

The initial storage suite failed against the previous implementation. Separate regressions then exposed completed-legacy cleanup and five invalid-timing cases; each failed before its corresponding fix. The first browser attempt failed before execution because Chrome could not start debugging under the fake USERPROFILE; the corrected harness is recorded, not counted as a product pass.

Final source manifest `8b2c1e7fd1536798bca30803ae6a2b6f0c6c73e077a92be1e7b64cd69e3607d9`:

- Focused storage and schedule/API/network suites PASS.
- Browser loading: 14 scenario groups PASS; editor: nine scenarios PASS. Screenshots inspected at 390×844 and 1280×900.
- Full Go: **2083 PASS / 38 SKIP / 0 FAIL**, 25 passing packages. Go vet and executable build PASS.

Exact commands from app/native: `go test -v -count=1 -timeout=120s -run TestScheduleStorage ./internal/aurora`, `go test -count=1 -timeout=120s -run "Test(Schedule|AdoptionAPI|Network)" ./internal/aurora`, `go test -json -p=1 -count=1 -timeout=300s ./...`, `go vet ./...`, and `go build -o <audit>/neko-aurora.exe ./cmd/neko-aurora`. Browser commands from root: `python -u app/tests/aurora/schedule-loading-browser.py` and `python -u app/tests/aurora/schedule-browser.py`, with AURORA_BROWSER_AUDIT_DIR pointing to separate evidence folders.

Receipts, manifests, RED/GREEN logs, decisions.tsv and screenshots are under `.audit/2026-10-29-schedule-storage`. All 1858 source files were verified against the code commit, accounting for existing LF normalization. The recognizable-secret scan checked nine candidates and 681 newly reachable history blobs with zero matches; it is not exhaustive. The unrelated adoption-document edit remains excluded.

## Limits

These are local store, ledger and Chrome fixtures. They do not prove distributed exactly-once side effects, cloud synchronization, webhook delivery, model quality, native phone/installer acceptance or arbitrary power-loss atomicity. No cross-process schedule writer is added: the application retains its existing ledger writer lock and in-process schedule mutex. External changes during a running process are not automatically reconciled. No additional changelog bullet is marked complete for this hardening.
