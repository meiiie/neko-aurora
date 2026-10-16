# Account authority in the local runtime

Selected account metadata is pinned by the server at intake. Credential generation advances only on a submitted credential; account revision advances on each edit. Used account nonces survive edits/removal/restart and are bounded to 512 receipts with no eviction. Stale retries reject rather than create another identity.

Each claim writes a new ten-minute lease descriptor to the task journal, bound to request, bot, owner, space, source conversation, attempt, epoch and a monotonically increasing fence. This descriptor contains no secret, secret reference or bearer token. It is excluded from the model prompt. Only explicit `local-controlled-fixture` execution is supported; saved cards remain `stored_unverified` and their keys are not consumed by Core.

The single lock order is ledger then connections. Intake, dispatch (host start plus initial frame), admission, text publication, artifact write and completion hold the live account guard across their commit. Removal or revision/generation change commits in the catalog before its API ACK; reconciliation then fences affected tasks. An effect already holding the guard finishes before revocation can commit. No output/effect admitted after revocation may use that pin. Recovery receipts remain writable after revocation; historical journal replay validates descriptors without consulting current accounts.

Missing authority or lease expiry suspends only the affected task. Restart does not rebind to another account or revive old authority. A new task must explicitly select the current account revision. Lease renewal, real provider authentication/key consumption, stable credential broker descriptors for crash resume, OAuth, third-party grants and cloud/multi-owner authority remain unimplemented gates.

Observed expiry commits AccountExpiredAt before denial. Requeue, claim and context refresh cannot renew that expired obligation, even after clock rewind and restart. If the journal cannot persist that observation, it is poisoned: all bound and unbound runtime permission checks and Dispatch deny until a fresh store is opened. No callback may launch from a closed or poisoned journal.

Validation commands and exact commits are kept in `.build/authority-*/validation.json`; independent audit follows each immutable runtime checkpoint. Synthetic ledger receipts are not native process proof. Native fixture runs, when reported, use only the owned loopback provider and placeholder keys.
