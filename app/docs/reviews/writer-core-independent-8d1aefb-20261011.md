# Independent bounded writer-v2 contract audit

Source: Core `8d1aefb91068b07ad395865602b0c359a0d016bc`. Status: **PASS 44/44 synthetic gates, 0 FAIL**. This is source/fixture evidence, not native kernel, compiled Core, ACP, or crash recovery acceptance.

No reproducible P1/P2 defect was found within the reviewed trusted-helper/cooperative-writer scope. The owner-reported full Bun/build/native results and binary hashes are not independent evidence in this report.

## Checked contract

- Exact `task-writer-lease.ts` in a Node VM: absent descriptor behavior, incomplete descriptor, absolute path, regular file, one link, lowercase hash, receipt identity/shape, stdout/stderr bounds, helper exit/error, release token/idempotency, isolated helper environment and private token outside argv. **29 PASS**.
- Exact `TaskSessionCoordinator.load` and `close` method bodies in a Node VM with in-memory storage/runtime/lease fixtures: current/prior receipt matching, task/root/authority denial before runtime creation, uncertain factory/cleanup retaining ownership, and close ordering. **15 PASS**. Internal storage serialization, scope filesystem resolution and runtime assertions are fixture boundaries, not newly revalidated implementations.
- Read native helper Rust source: actual parent PID lookup, retained process handle, creation FILETIME, parent executable hash, exclusive share-mode-zero lock, reparse/link rejection, strict v2 record shape, released reuse, held-owner liveness proof, bounded command/token and release fsync-before-ACK. Native Win32 calls were not executed.
- Read import publication/source docs: same-directory no-replace `MoveFileExW(flags=0)`, KnownDLL `kernel32.dll`, and post-publication inode/link identity check. Kernel behavior was not rerun.

## Integration gates

**Lost resume ACK requires a bounded recovery contract.** Exact Core load body accepted client receipt epoch 1 against saved Core epoch 2 and returned fresh epoch 3 with `recoveredFromPriorReceipt=true`; ACP exposes `neko.taskRecovery` kind `prior_receipt_retry`. The previously audited App 375 epoch+1-only checks would deny this case. The new App writer-v2 checkpoint was not available during this bounded audit. Treat this as a consumer regression gate, not a confirmed defect on unreviewed integration code. Permit no arbitrary epoch jump: bind the exact task/root/session/authority and pinned runtime, require the authenticated prior-receipt recovery marker, and keep unknown effects fenced.

The App must pass both pinned helper fields on every create/resume after its environment filter, independently pin Core/helper, and never fallback to a new task/session or legacy lock path after a v2 ownership denial. No v1/unknown/corrupt/live lock migration was attempted.

## Synthetic robustness observations

The wrapper accepts a changed canonical-looking `parentCreated` in a release receipt and an unsolicited held+released pair. Reviewed pinned native helper source cannot emit either sequence: it retains one `Record` and only releases after matching release command plus durable update/handle close. These observations are optional parser/state-machine hardening; no exploit or P-level finding is claimed under the stated trust model. A synchronous held receipt followed by helper nonzero exit was denied.

## Evidence and limits

Source snapshot `C:\Users\Admin\Documents\Codex\2026-10-11\task\checkpoint-verify-20261011\.build\permission-review\core-writer-8d1aefb` contains 528 original exported files, each byte-equal to the exact archive and committed Git blob. Scripts only created owned audit evidence; no production/index/UI/E/reference edits, GUI, helper/Core launch, provider or grant calls.

- `writer-core-probes.cjs` → `writer-core-probes-8d1aefb.json`
- `writer-session-probes.cjs` → `writer-session-probes-8d1aefb.json`
- `source-manifest-core-writer-8d1aefb.json`
- `validation-core-writer-8d1aefb.json`

Commands/environment are recorded in validation JSON. Remaining gate: immutable App integration review and owner-operated actual native crash/lock recovery. Full suite, paid provider, cloud and phone are outside this audit.
