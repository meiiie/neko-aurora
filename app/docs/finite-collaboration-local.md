# Finite local collaboration

REQ-PB-006 / SCN-PB-006 implements an owner-authorized, bounded coordination record using existing Neko Core tasks. Go advances three durable stages: selected coordinator proposes, the other selected bot reviews, then the same coordinator concludes. Go does not run a second agent loop or interpret agreement as verified model quality.

## Contract

`POST /v1/collaborations` requires the existing authenticated local-owner cookie and same-origin boundary. It accepts:

```json
{
  "channelId": "channel-<32 hex>",
  "expectedChannelRevision": 1,
  "participants": ["bot-meiiie", "bot-davin"],
  "coordinatorBotId": "bot-meiiie",
  "goal": "Compare options and return a final report",
  "maxStages": 3,
  "clientNonce": "owner-collaboration-request",
  "accountId": "optional existing public account ID",
  "accountRevision": 1,
  "projectId": "optional existing project ID",
  "projectRevision": 1,
  "fileIds": []
}
```

Optional account/project fields must be omitted together when absent. The two participants must be distinct stable bot IDs and current members of the exact shared channel; the coordinator must be one of them. Participants/file selections are sorted canonically. Intake freezes the server-generated account pin and selected project/source bytes/revisions. The API never accepts actor, scope, credential, runtime binding or collaboration-stage authority from a client.

New intake returns 201 `{collaboration, duplicate:false}`. A repeated nonce with identical canonical input returns the current original group receipt, even after cancellation or authority removal, with 200 and `duplicate:true`. A changed input conflicts. Stale channel membership/revision or account selection returns 409 before intake commit. Invalid structural input returns 400. Collection limits are 128 groups and exactly three stages; goal is at most 2,000 UTF-8 bytes.

`GET /v1/collaborations` returns `{collaborations}`; `/v1/snapshot` additionally contains `collaborations` and capability `local_collaboration_v1`. SSE uses `event.collaboration` for `collaboration.created`, `collaboration.stage`, `collaboration.stopped`, `collaboration.cancelling`, and `collaboration.cancelled`. Group `revision` is a separate monotonic CAS counter from each task's epoch.

`POST /v1/collaborations/{id}/cancel` accepts `{expectedRevision, clientNonce}`. It commits cancellation intent before cancelling every task bound to that group, including an accepted task whose mapping commit was lost. It preserves completed results and unresolved effects; cancellation is not rollback. A repeated action nonce returns the original action's current group without rebuilding authority. The action cannot cancel a task in another group or an independent owner task.

## Durable advancement and effects

Each stage uses a stable nonce `collab-<id>-stage-N`, keeps the origin channel, and receives `work.collaboration:{id,stage}`. Work acceptance precedes the group-to-request mapping commit. A crash between these commits reuses the accepted request. An unmapped accepted task cannot be claimed or publish. Legacy Work/Event/Snapshot records omit additive fields and retain existing replay semantics.

The resource pin stays byte-identical across stages, including when another relevant file is uploaded later. Each stage captures current authorized shared-channel v3 history, including the exact preceding stage results. It excludes target private lanes and other channels. If the normal bounded context cannot contain all required prior stage results, the group stops `needs_owner` with `stage_context_unavailable`; it does not proceed using an incomplete coordination history.

Stage 1 and stage 2 discovery omit `artifact.write_text`, and the host denies attempts before recording any effect. The same guard rejects replayed early-stage effect starts. Stage 3 alone may use the existing immutable artifact action, subject to the task's original authority, current channel/project/file permissions and account lease. External effects remain unavailable in all stages. The final file is retrieved through the existing exact-channel artifact boundary.

There is a five-minute server ceiling (`createdAt`/`deadlineAt`) in addition to three stages. Expiry or backward clock observation stops `needs_owner` with `stage_budget_expired`. The process watcher observes the group fence on the next manager tick. There is no automatic renewal, regrant, retry, goal replacement, participant addition, or extension of the stage budget.

Statuses are:

| Status | Evidence and behavior |
| --- | --- |
| `running` | Accepted intent, at most one outstanding stage; existing foreground tasks still schedule independently. |
| `completed` | Exactly three completed, nonempty, resolved results; final coordinator conclusion is available. This does not prove semantic accuracy. |
| `suspended` | Channel/account/project/source authority changed, or a task was suspended; no further stages or publication. Task stop receipts are inspected separately. |
| `needs_owner` | Transport/effect outcome unresolved, empty result, required context unavailable or time budget exhausted; no implicit task requeue. |
| `failed` | Stage intake conflict or individual task cancellation; no further stage. |
| `cancelling` | Owner cancel intent persisted, all bound tasks fenced. Missing runtime-stop receipt keeps this state. |
| `cancelled` | Every launched cancelled/suspended task has a retained stop receipt, or never had an attempt; completed results remain available. |

## Integration ownership

New files are `ledger/collaborations.go`, ledger/API tests, `scripts/collaboration_fixture.py`, `scripts/e2e_collaborations.py`, and this document. Existing hooks:

- `store.go`: additive Work/Event/Snapshot fields and maps; collaboration event replay/apply/deep copy; accepted-work frozen-resource validation; extracted `cancelLocked` for atomic scoped cancellation.
- `runtime.go`: Work binding deep copy and pre-effect/replay single-writer guard.
- `permissions.go`: group authority included in context/publication fences; completed authorized results remain reusable.
- `actions.go`: stage-aware discovery.
- `manager.go`: call `AdvanceCollaborations()` after account reconciliation and before work scheduling.
- `api/server.go`: the three collection/intake/cancel routes above.

No frontend, Core, production state, E: checkout or original archive is changed by this branch. The shared dispatch timeout discovered with large prompts is a separate integration dependency. The approved transport candidate from Root `dd86f3f6cf5cdbb428f8c95c5da89d2517c80fcb` is imported only in `manager.go`, `frames.go`, and `frames_test.go`: process start remains fenced, initial frames are written outside the store mutex in bounded chunks with the watcher armed first. The approved Root `6e3d025c36b99924bd4a0f4df63fa002e8c2d5a1` change in `host/src/main.rs` starts bounded input reading before expensive binary hashes; pin checks still precede task directory creation and Core launch. Actual-Core acceptance must be rerun against this combined checkpoint.

## Validation scope

The initial endpoint test failed at the immutable baseline with 404, then passed with the new route. Tests cover three-stage identity/context/writer boundaries, canonical nonce/CAS, task-before-mapping crash/restart3, orphan cancellation, independent-task isolation, retained stop receipt, frozen sources, authority changes, unresolved result, time ceiling, completed-result availability and forged replay controls. Formal Go race detector is blocked by missing GCC; finite schedule probes do not replace it.

Exact checkpoint commands/results and hashes are kept in the branch's `.build/evidence-*` handoff manifest. Actual-Core tests use own temporary datasets and a synthetic loopback provider, not cloud sync, connector grants, provider credentials, UI or physical-phone acceptance. The initial actual-Core run reached the finite stages and final file but failed at an API snapshot timeout; its whole suite is not a PASS. Semantic conflict resolution and real model quality remain unaccepted.

The first transport-combined run completed foreground work, but its eight-second fixture delay ended before slow machine startup completed the foreground assertion. That failure journal is retained. The follow-up fixture holds the slow stage on an explicit teardown gate with a sixty-second ceiling, while the original twenty-five-second foreground and thirty-five-second group assertions remain unchanged. Cancellation must still prove the owned Core handle stopped, with no later effects or next stage.
