# Independent local permission review — final scoped result

Reviewed code commit: **33efa44902737f4db885a522dce099c664a62700**. Source was exported from this immutable Git object; all original exported files were checked against the source archive. Only audit-owned additional fixture tests were edited. No root source, UI, Git metadata, E drive inputs, browser/native process or other worker job was changed.

Environment: Windows amd64, Go 1.27.2; `GOPROXY=off`, `GOSUMDB=off`, audit-owned caches/state. The API uses the actual Go handler through `httptest.ResponseRecorder`, with no TCP listener. Receipt PIDs are synthetic ledger fixture values; no native child, Core or provider was executed by this review.

## Findings resolved

| Finding | Reproduced checkpoint | Fix checkpoint | Final result |
|---|---|---|---|
| P2: revoke before resumed admission rejects the matching new child stop receipt, leaving work suspending | 8dc8eb0 | f5afe64 | PASS: trusted current-attempt binding is recorded separately; matching proof survives three restarts and enables explicit refresh; saved binding remains intact |
| P2: refresh can commit an encoded runtime prompt above the intake/IPC bound | 8dc8eb0 | f5afe64 | PASS: ErrInvalid before commit, with work and sequence unchanged |
| P1: stronger unversioned refresh replay rule rejects a journal the previous code accepted | f5afe64 | 60c1811 | PASS: both real prior-code fixtures open; historical zero-version refresh is readable, new version retains the stronger bound |
| P2: historical refreshed work with a retained attempt and native stopped proof remains waiting_host despite a permanently oversized prompt | 60c1811 | 33efa44 | PASS: that task becomes suspended/context_prompt_oversized; attempt, receipt, pin, archive and obligation identities remain intact |

The old journals were produced using actual 8dc8eb0 code, then successfully reopened by that same code. Journal records and digests were not edited. Final recovery uses new owned copies of the frozen originals. Unlaunched and previously native-stopped cases both preserve their original context/archives across three reopen/recovery cycles and leave unrelated Davin intake usable.

Additional independent guards pass: failed/unverified resume without a binding cannot prove the old Core stopped; seven mismatches of session/task/root/activation/authority/runtime/attempt are rejected; cancellation can retain the shutdown receipt without readmission or resurrection; a new claim clears old proof; old-attempt receipts are rejected; transitive revocation still fences a consumer whose root source has left the direct eight-source pin. Existing ledger tests also pass, including old v1 pins, old completion records and effect reconciliation.

## Exact final validation

From `snapshot-33efa44/native`, using the environment above:

- `go test -count=1 -json ./internal/ledger` with `NEKO_AUDIT_LEGACY_STATE=<owned queued-legacy copy>` — **PASS**, `ledger-33efa44-with-probes.jsonl`.
- `go test -count=1 -json -run '^TestIndependentLegacyRefreshRecoversOnlyAffectedObligation$' ./internal/ledger` with `NEKO_AUDIT_LEGACY_STATE=<owned native-stopped legacy copy>` — **PASS**, `legacy-active-open-33efa44.jsonl`.
- `go test -count=1 -json -run '^TestIndependentPermissionHTTPRecorderNoListener$' ./internal/api` — **PASS**, `api-recorder-33efa44.jsonl`.

API checks cover missing permission choice/revision, cookie/origin gates, unknown/duplicate JSON keys, fractional revision, nonce retry, own-denied refresh, regrant, same obligation/message identity and stale refresh epoch.

`validation-final-33efa44.json` contains exact environment, source-file identity, executed commands, test counts, log hashes and frozen input hashes. `audit-8dc8eb0.md` and `audit-f5afe64.md` retain the failing baseline findings and are not rewritten as final success reports.

## Limits and remaining gates

This scoped review found no remaining reproduced P1/P2 defect after 33efa44. It does not execute or accept the native Rust/Core process boundary, UI interaction or model behavior. Integrator-owned native suites are separate evidence. Unknown process attempts remain fail-closed; they cannot refresh on the strength of a new unrelated child PID. Account runtime leases, channel publication/read ACL, real provider, cloud synchronization, physical phone/keyboard/assistive technology and user visual acceptance remain outside this review.
