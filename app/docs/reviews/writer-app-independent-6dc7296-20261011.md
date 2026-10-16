# Independent App writer-v2 audit

Exact final App `6dc72962560492974c35a4e68ccac152ed159976`; guard fix `7ee2cc8b5a89a4b9e818a027a0c13167ff124776`. **Scoped PASS**: Go ledger/protocol 51 top-level + 48 subtests, Rust 9/9, vet; separate 40 finite concurrent schedules PASS. No independent native Core/helper, API network or GUI acceptance is claimed.

## Closed P2: unbound recovery proof bypassed admission and replay

RED `5b872fe87a3f36605e136f3e12ce0b9380ff826e`: `Store.Admit` and `work.admitted` replay only compared receipt to staged binding when a staged binding existed. Unbound first activation could carry `prior_receipt_retry`; unbound resume could change session or use a historical runtime not opted into recovery. These bindings were rejected by `validAttemptBinding`, but accepted by `Admit`. The replay validator accepted first-activation proof, and an owned hash-valid forged journal was opened successfully. Production native manager normally stages a binding; no exploit or claim that pinned Rust produces these invalid states is made.

Final guard rejects any nonnil recovery proof without matching staged attempt binding in admission and replay. The original RED overlay is byte-identical, SHA256 `82accf48c20de989287dbb77f9b86859d9bebb592b31e4e89cba424b3a79c4fb`. Its three admission negatives plus replay-validator negative pass unchanged on final. The additional full journal negative now rejects `journal_corrupt`.

## Independent verification

- Staged negative matrix: nil/wrong proof, PID/session/authority/pin mismatch, historical runtime, repeated activation, step +1 or +3 rejected without changing saved Core.
- Recovery proof copies: caller mutation, Get, Snapshot and Events cannot alter durable proof. Three restarts retain proof; next normal activation +1 does not inherit old recovery proof.
- Permission/cancel race: after bound +2, stale admission denied; old PID stopped receipt denied; new PID receipt retained with fence through three restarts.
- 40 concurrent schedules: bind +2 races cancel or source revoke, then stale admission, exact stop receipt and restart. These are finite schedules, not a formal data-race detector or exhaustive concurrency proof.
- Exact frozen e0 proof-free journal replay survives three restarts with unchanged historical work and independent new intake.
- Original Go ledger/protocol tests and 4 Rust library tests pass. Five independent Rust tests cover exact proof shape and +2 opt-in/rotation/bounds, pending/unknown effects denial, duplicate JSON denial, current Windows pin, missing/tampered/invalid/regular helper descriptor and `env_clear` ordering. Helper descriptor function is an exact extraction from committed `main.rs`; only owned synthetic files are used.

## Source review and limits

The helper descriptor is set after environment clearing. Core/helper hashes come from embedded pins; helper discovery is beside the canonical pinned Core, not model/project configuration. Core itself additionally enforces one link. Prior recovery marker is strict in Rust, opted in only for pinned current Windows Core, bounded to +2 with rotated activation, and combined with task/root/authority checks before effects-clear and admission. New Go typed proof is copied and replayed. Proof-free historical events remain supported. No v1 migration or new-session fallback was added.

Source manifests verify 185 final original exported files byte-equal to exact commit archives, and 183 RED originals. Every file matches its committed Git blob directly or after reversing Windows Git CRLF export; canonical Git identity and actual exported bytes are both recorded. Audit overlays are separate. Initial protocol runs lacked the shared canonical contract fixture in my bounded export; logs are preserved, exact supplementary contract archive added, and final scoped command passes. This was an audit-export issue, not a product failure.

No remaining reproducible P1/P2 found within this scoped review. Owner-reported actual native ACK-loss/kernel gates are excluded from independent evidence. Remaining acceptance: owner-operated actual pinned Go/Rust/Core/helper crash recovery, live/legacy/unknown lock gates, and broader UI/cloud/phone/provider work.

Evidence in `C:\Users\Admin\Documents\Codex\2026-10-11\task\checkpoint-verify-20261011\.build\permission-review`:

- `writer-ledger-5b872fe.jsonl`, `writer-replay-negative-5b872fe.jsonl` (RED preserved)
- `writer-go-complete-6dc7296.jsonl`, `writer-go-concurrent-6dc7296.jsonl`, `writer-go-vet-6dc7296.txt`
- `writer-host-scoped-6dc7296.txt`
- `validation-app-writer-6dc7296.json`, `source-manifest-app-writer-6dc7296.json`, `source-manifest-app-writer-5b872fe.json`
- Immutable source ZIPs and owned overlay tests listed with hashes in validation JSON.

No production/index/UI/E/reference edits, deploy, provider, grants or secrets were performed. All commands, environment, pass/fail/not-run scopes are recorded in validation JSON.
