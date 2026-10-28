# Durable owner-message admission

Base: `f76feacc0ff62c4bc2ee94253a9b011e9a68c5eb`, branch `codex/grokbot-adoption`. Local implementation and fixtures only; no provider calls, deployment, push or grants.

## Outcome and design

An Aurora send now records the original text, reference selection and all admitted task IDs together. Group admission writes one bounded, checksummed journal frame and syncs it before exposing any recipient. Repeating the same message after a lost HTTP response or restart returns the existing tasks even if the referenced chat or group membership changed. Reusing that nonce with different text or a different reference returns a conflict.

Group recipient selection uses the owner's text before attaching referenced history. Explicit group recipients bypass private-chat alias rerouting, which previously rejected `@all` or could collapse several recipients into the first mentioned bot. The existing publication checks still suspend tasks whose audience is no longer authorized after membership changes. A receipt does not restore those rights.

The new version-2 storage frame contains ordinary, individually sequenced `work.accepted` events. History, runtime and sync readers continue consuming those events. Replay checks each event's existing identity, publication, context and runtime invariants, plus the outer receipt. Version-1 frames remain readable. Older binaries do not understand version-2 frames and will reject journals written by this implementation; reverting code alone is not a data downgrade.

This applies to Aurora's local-owner message route. It is not a multi-tenant admission API, a distributed transaction or an exactly-once guarantee for external tool effects. A pre-upgrade partial group send is rejected rather than silently completed with new membership. References are pinned when the server first admits the message, not when an offline user initially selects a chat.

## pstack checkpoint

Workflow: [Feature](https://github.com/cursor/plugins/blob/main/pstack/skills/poteto-mode/playbooks/feature.md) and [Prove It Works](https://github.com/cursor/plugins/blob/main/pstack/skills/principle-prove-it-works/SKILL.md).

- Blocking work: trace journal admission, replay, nonce identity and all event consumers before choosing a storage change.
- Independent work: inspect UI draft preservation and source inventory while the Go gates run; code remains unchanged during validation.
- Shared state: the ledger lock covers preparation, receipt lookup and one durable commit. Pending siblings do not become completed history sources.
- Smallest safe unit: one owner implements ledger, route and recovery tests together because their invariants are coupled. Delegation and other-chat handoffs are skipped under the user's explicit prohibition. No PR/push is authorized.

## Validation

Commands, environment, source manifests and outcomes are recorded in `.audit/2026-10-28-message-batch/validation.jsonl`. The runner removes live/provider environment variables. Tracked fixtures are [ledger transaction tests](../../app/native/internal/ledger/owner_messages_test.go) and [HTTP receipt tests](../../app/native/internal/aurora/message_receipt_test.go).

The tests cover invalid/stale recipients, concurrent replay, restart, membership changes, immutable references, changed-payload conflicts, failed writes, three incomplete-frame cuts, rehashed receipt/context/authority/sequence tampering, legacy partial admission, `@all` and multiple named recipients. The first run exposed an existing `@all` routing issue and an incorrect test assumption that task execution state should remain unchanged after membership revocation. The implementation preserves revocation; the test now compares receipts with current task state while separately checking immutable identity and text.

Windows amd64, PowerShell 7.6, Go 1.27.1. The final full run used `go test -json -count=1 -timeout=180s ./...` from `app/native`: **25 packages PASS, 1,538 passing test/subtest events, 37 skips, zero failures**, in 133.30 seconds. Code did not change during the run. Log: `go-125608.log`; source manifest SHA256: `e868b32511b61c9c8970282338b37bd00ec213fff0ab6f2f9b328e6bcfb4cc90`. The existing skip reasons remain visible in the log; they are not counted as passes. These are local tests, not model-quality or native-device acceptance.

The same source manifest passed `go vet ./...` and `go build -o <audit-dir>/neko-aurora.exe ./cmd/neko-aurora` (`vet-125849.log`, `build-125900.log`). Candidate files and newly reachable history were scanned for recognizable credential formats without printing matched values. The exact resulting code commit is linked to the checked source manifest in the local `checkpoint.json` evidence.
