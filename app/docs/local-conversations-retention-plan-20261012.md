# Private conversations, export and retention preview

The authorized follow-up starts from deliverycaaa9f7/application8ceb759. Scope: new private conversation per stable bot, archive/restore, public history export, and a saved retention preview window. Existing light reference, background work, immutable context and file bindings must survive navigation. Cloud/device pairing, OAuth/provider and native owner consent stay separate. No permanent data removal or deletion policy is authorized.

## Boundaries

- `Conversation` is durable metadata over an existing publication lane. New IDs are conversation-32hex; legacy conversation-botID remains the initial lane. Bot ID/alias and config revision do not change. The virtual single-bot audience revision stays1 independently of the archive metadata revision, so accepted work keeps its original authority and can finish.
- Archive blocks only fresh intake and hides the lane from normal browsing. Existing request receipts, task controls, pinned context, original downloads and replay are preserved. Restore changes metadata via CAS/nonce. New lanes do not import the old private history or project/file ACL. Returning to the old lane restores its drafts/history/tasks/files.
- Lifecycle events and retention preferences use strict typed union validation and detached projections. Original nonce/input survives retry; changed intent or stale CAS conflicts. No conversation deletion route exists.
- Export projection owns the scope under the journal lock and contains only original published owner text, reply/status and authorized artifact metadata. Formatter is a separate pure bounded package. It cannot reflect Work/context/account/runtime structs. JSON and plain text are inert downloads; metadata/pointer does not supply bytes or a grant. Explicit limits fail rather than truncate.
- Retention setting is a preview window, default0/keep all. The pure planner uses trusted server time and creation age in UTC days. It protects nonterminal work, unresolved effects, unproven native stop, context/task links, collaboration stages, source pins and result-file bindings. Preview reports IDs/estimated text bytes/reasons, never raw context or file content. Enabled staysfalse; no scheduler/delete/filesystem mutation is connected.

## Implementation and validation

Root owns ledger/publication routing, event replay, snapshots and API. Independent workers own pure history formatter, pure retention planner, and frontend/harness. They use nonoverlapping paths and E scratch/cache/temp. Root freezes a combined commit before fresh full Go/vet/build and actual HTTP/browser/native checks; an independent reviewer reads immutable Git objects and runs private probes.

Tests cover original lane history versus empty fresh lane; archive while work is alive; original duplicate ACK versus fresh archived409; CAS/nonce/type strictness; caller-detached metadata; repeated restart; artifact physical bytes/hash/mtime and scope preserved; no private execution fields in exports; unauthorized/extra-query/deletion/grant rejection; preview read-only and disabled; age/byte/time bounds; exact lost/late ACK behavior, modal focus, narrow/short viewport, drafts/reload and new-session file separation. Native processes use the pinned Rust host/Core and controlled loopback model only. Phone/AT/user review/native-window acceptance is not inferred from headless geometry.

Preserve earlier packets, ZIPs, previews, source changes and production state. New source/binaries/evidence are saved in a separate revision packet. Update the complete product gap matrix with implemented, missing-code and unaccepted scopes after integration; do not relabel old media/tests or claim complete product acceptance.
