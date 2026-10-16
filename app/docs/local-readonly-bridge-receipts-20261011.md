# Local bridge catalog and command receipts

These routes use the existing local HttpOnly session cookie, exact Host and Origin checks, and `Cache-Control: no-store`. They do not admit work, refresh authority, reconcile tasks, advance groups, or commit journal events.

## Adapter routing catalog

`GET /v1/adapters/catalog` takes no query parameters and returns exactly:

```json
{"version":1,"ownerUserId":"local-owner","spaceId":"local-space","channels":[{"id":"channel-…","name":"Team","revision":1,"botIds":["bot-davin","bot-meiiie"]}],"bots":[{"id":"bot-davin","name":"Davin"},{"id":"bot-meiiie","name":"Meiiie"}]}
```

Channels are the explicitly stored shared channels, sorted by ID. Bots are sorted by ID. Private conversations, messages, contexts, accounts, files, projects, secrets and adapter sessions are absent. Registration still checks the selected channel's current revision and bot membership at message intake.

## Original command receipt lookup

Snapshot capability: `local_command_receipts_v1`.

`GET /v1/command-receipts?kind=task_control|collaboration_create|collaboration_cancel&clientNonce=…`.

For `collaboration_cancel`, include exactly one `collaborationId=collab-…`. Other kinds reject that parameter. Missing, empty, repeated, malformed or extra parameters return 400. A nonce already committed for cancellation of a different group returns 409. A closed or poisoned store returns 503, never an absent receipt.

The response is `{version:1,kind,found,clientNonce,input?,work?,related?,collaboration?}`. `found:false` means no command of that kind with that nonce is in this local journal. It does not prove that a different server has not accepted it. The endpoint never creates a receipt or retries a command.

When found, `input` is the immutable typed original input. Task control uses `TaskControlInput`; group create uses `CollaborationInput`; group cancel uses `CollaborationCancelInput`, with the original expected revision reconstructed from its committed event. Group create participant and file arrays are sorted as at intake. Optional zero/empty account, project, file and text fields follow the existing Go `omitempty` serialization. A UI comparing its frozen input must normalize those arrays and omitted fields before canonical JSON/hash comparison. No server-supplied hash is necessary; compare the original input, kind, nonce and cancel route identity independently.

`work`, `related` and `collaboration` identify the same objects returned by an idempotent repeat of the original POST, at their current state. They may be cancelled, suspended, expired, completed or have advanced revisions. Their IDs remain fixed. Context pins and archived contexts are omitted, and pinned file excerpts are removed from this lookup response. Consumers resnapshot when they need their full authorized current view and must retain existing epoch/revision fences. Later authority changes do not erase the original command proof.

This supports bounded UI pending intent storage. Persist an authorized input before its first POST. After reload, perform lookup and resnapshot only. Unknown, expired, stale or revoked intents must not generate an automatic POST, new nonce, child or group; explicit retry keeps the original body. A found receipt can reconcile an expired local pending intent. Account secrets, full Work/context payloads and raw uploaded files do not belong in pending intent storage.

Validation receipts live under `.build/readonly-controls-*` and record exact code/binary source, commands and result. Unit probes cover authority changes, three restarts, immutable typed input, related task identity, current cancelled group state, invalid queries, unavailable stores, mutation isolation and authentication. Browser/native integration and durable UI validation remain separate gates.
