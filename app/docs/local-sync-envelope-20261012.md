# Bounded local sync envelope, 2026-10-12

`native/internal/syncstream` provides a standard-library-only version 1 wire envelope and a reference in-memory client reconciler. It does not open sockets, issue commands, persist state, authenticate users, register clients, mint tokens or grants, or implement cloud synchronization. The two clients in its tests are independent local state machines consuming synthetic public projections, including Vietnamese text. Actual journal/API integration belongs to the parent change and requires separate evidence.

## Public interface and payload

```go
NewSnapshot(instanceID, scopeID string, sequence uint64, objects []Object) (Envelope, error)
NewIncrement(instanceID, scopeID string, after, highWater uint64, frames []Frame) (Envelope, error)
Seal(Envelope) (Envelope, error)
Encode(Envelope) ([]byte, error)
Decode([]byte) (Envelope, error)
DecodeReader(io.Reader) (Envelope, error)
BindingDigest(instanceID, scopeID string) (string, error)

Cursor{Version, InstanceID, ScopeID, Sequence}.Encode() (string, error)
ParseCursor(string) (Cursor, error)
Cursor.Check(instanceID, scopeID string, highWater uint64) error

NewClient(instanceID, scopeID string) (*Client, error)
Client.Apply(Envelope) (Outcome, error) // Applied or Ignored
Client.Cursor() Cursor
Client.Objects() []Object
Client.HighWater() uint64
Client.NeedsSnapshot() bool
Client.ResetInstance(newInstanceID string) error
```

Every envelope has exactly these required top-level fields: `version`, `instanceId`, `scopeId`, `mode`, `after`, `to`, `highWater`, `scopeDigest`, `dataDigest`, `snapshot`, `frames`. Both arrays are present, including empty arrays. A frame contains `sequence` and `updates`. Each object requires `kind`, `id`, `revision`, `deleted` and the matching typed payload for a live object. A deleted object has no payload. Objects and each frame's updates have canonical ascending `kind:id` order and no duplicate identity. Constructors clone and sort their input; direct wire decoding requires that order.

The exclusive object union is:

| Kind | Public fields |
| --- | --- |
| bot | `name`, optional `alias`, `description`, `color` |
| message | `botId`, `channelId`, `role`, `text`, `status` |
| file | `name`, `mime`, `size`, `sha256`, `botId`, `channelId`, `projectId`, `status`, `version`, `accessRevision` |

Messages accept only `user` and `assistant` roles. `description` is public display text, never bot instructions. File `size` describes the byte count; there is no file content, excerpt, preview, path, or byte field. The union has no untyped raw extension. Unknown nested fields, duplicate JSON keys, differently cased field aliases, missing required fields, null scalar fields, trailing JSON, invalid UTF-8 and excessive nesting reject. Thus full ledger Work/Event/Snapshot types cannot be passed through as envelopes: the integration must explicitly construct this narrower projection. It must never place context pins, runtime context, Core bindings, admission/lease proofs, provider credentials, bot instructions, or file bytes into these DTOs. Public message bodies remain caller-supplied content; field validation is not a semantic secret detector.

File `version` is positive; `accessRevision` permits the canonical initial value zero through `2^53-1`. The adapter preserves that ledger value without an offset. Object `revision` is a separate positive projection revision.

## Identity, counters, integrity and replay

`instanceId` identifies one durable journal generation. It must survive restart of that journal, and change when a different/reset journal replaces it. The module does not create or persist this identity. A cursor is canonical unpadded base64url of the exact version 1 cursor JSON; it is not a bearer credential. Every sequence, revision, byte count and file counter is bounded by `2^53-1` so a JavaScript consumer can represent it exactly.

`scopeId` is immutable for the client's lifetime. The integration derives it from the authorized local owner, exact bot/channel/project selection, public projection version and visibility/ACL/resource revision fence. A change to any visibility fence requires a different scope and an explicitly created client for that selection. The client never adopts a scope from an incoming packet. Scope identifiers and cursor tokens supply no permission; the existing local-owner authorization and resource/publication access checks must run on every read.

`scopeDigest` is SHA-256 of canonical JSON `{version,instanceId,scopeId}`. `dataDigest` is SHA-256 of canonical typed envelope JSON with `dataDigest` replaced by the empty string. SHA-256 here detects inconsistent data and binding; it is integrity only, with no authentication, signing key or grant. An authorized or unauthenticated source could compute these hashes itself, so hashes never replace the API boundary.

A complete `snapshot` has `after=0`, `to=highWater`, `frames=[]`, and the entire selected public object set at exactly that captured boundary. It can replace materialized state only after validation; it cannot regress a previously observed entity revision. An incomplete or truncated snapshot must never be marked complete. Current deletions can be represented by tombstone objects. Omission in a later complete snapshot removes a materialized object while preserving the client's bounded revision floor against stale resurrection.

An `increment` has `snapshot=[]` and exactly one frame for every global journal sequence in `(after,to]`, in ascending order, with `after <= to <= highWater`. It may cover only the first bounded page before `highWater`. Events outside the selected scope still produce an empty `updates` frame: filtering does not renumber the global sequence or hide a gap. An empty increment is valid only when `after=to=highWater`. File unavailability and filtered prior messages do not reduce `highWater`.

Entity revisions must be stable for the same projected data at a given cursor and strictly increase when that data changes. A useful adapter chooses the last global event sequence that changed the entity's public projection, with a stable positive initial revision for built-in objects at sequence zero. Assigning the current snapshot sequence to every object breaks equality between a snapshot and an increment at the same cursor. Multiple updates to an entity at one revision must carry exactly equal public data.

New clients require a complete snapshot. An increment cannot establish initial state. After a gap, invalid envelope, mismatch, conflict or exceeded budget, `NeedsSnapshot()` becomes true and advancing increments are rejected until a validated complete snapshot restores consistency. Failed batches leave the accepted object set, cursor and high-water counter unchanged. Matching previously observed older/duplicate frames can be ignored; an overlap can advance only after all repeated frames match. Repeated same-cursor altered frame or snapshot data is a conflict. Unknown or evicted old history requires a snapshot instead of being silently ignored. A consistent current-cursor complete snapshot can clear the resync flag. Ignored packets may increase the observed high-water counter but never regress it.

Instance mismatches return `sync_instance_reset_required`; no packet resets a client. The caller must first confirm journal replacement through its existing trusted local boundary, then explicitly call `ResetInstance(newID)` and obtain a complete snapshot. This method preserves the scope, clears the old materialized state/history and rejects a reset to the same instance. Scope changes require a new client. None of these operations POSTs, retries a command, changes a draft, creates a grant or invokes Core.

## Fixed budgets

| Resource | Hard maximum |
| --- | --- |
| Encoded envelope / reader input | 262,144 bytes |
| Increment frames | 128 |
| Snapshot objects / total updates in one increment | 512 |
| Encoded object | 16,384 bytes |
| Message text | 12,288 UTF-8 bytes, also subject to encoded-object bound |
| Materialized client JSON | 2,097,152 bytes |
| Materialized object identities / lifetime revision floors | 512 each |
| Remembered frame hashes / accepted cursor state hashes | 256 each |
| Encoded cursor token | 512 bytes |
| JSON nesting depth | 8 |

String limits, valid union shape and safe counters are checked before hashing. Text containing escaped characters can reach the encoded-object bound before the text byte bound. `DecodeReader` reads at most the envelope maximum plus one byte. Client access is mutex-protected and returned objects are cloned. There is no durable client storage format in this slice: recreating a client starts with a fresh complete snapshot. Exceeding the lifetime identity budget requires an explicit fresh client/snapshot; retaining unlimited historical revision floors is outside this bounded implementation.

## Proposed additive read-only integration

The package adds no route itself. A concrete additive adapter can expose only:

```text
GET /v1/local-sync?botId=<id>&channelId=<id>&projectId=<id>
GET /v1/local-sync?botId=<id>&channelId=<id>&projectId=<id>&cursor=<canonical-v1-token>
```

The first form returns one complete snapshot. The second checks the cursor's instance, immutable derived scope and `sequence <= captured highWater`, then returns the next bounded contiguous increment. All selection parameters are singleton, authorized by the current existing local-owner API, and subject to the existing bot/channel/project/file resource/publication access checks. Reject unknown/duplicate query keys and invalid selection; do not interpret any caller-supplied ID as authorization. Existing owner header/session, loopback/Host/Origin policy and request deadline stay in force. Use `application/json; charset=utf-8`, `Cache-Control: no-store`; do not change `/v1/events` payloads or accept the legacy numeric SSE cursor as this typed cursor.

The success body is the envelope only. An optional `X-Neko-Sync-Cursor` header is produced with `Cursor{Version:1,InstanceID:e.InstanceID,ScopeID:e.ScopeID,Sequence:e.To}.Encode()`. Clients can derive the same token from the body. Snapshot set, frame projections, immutable visibility fence and high-water mark must come from one consistent captured journal boundary. If separate reads are used, bound replay to that boundary and recheck the visibility fence before sending; never combine newer materialized objects with older envelope counters. Select the largest prefix of at most 128 frames that fits every encoded/update bound. If the first frame cannot fit, fail explicitly instead of advancing past it. If the complete selected scope cannot fit, return a limit error instead of truncating or claiming completion.

Suggested public errors are `400 sync_invalid` for malformed input, `409 sync_cursor_ahead`, `409 sync_instance_reset_required`, `409 sync_scope_mismatch` and `409 sync_snapshot_required` for cursor/reconciliation failures, and `413 sync_limit` for a projection that exceeds the fixed envelope budgets. Access denial follows the existing API's owner/resource behavior. Return no private object identifiers or matched secret values in errors. Optional future SSE wrapping must preserve this exact envelope/cursor and emit complete bounded frames; the current SSE endpoint remains a separate contract.

The reference client follows GET-only recovery: after a gap/conflict it requests the first form for the same verified instance/scope, applies the validated snapshot, then resumes from its own cursor. It cannot pair/register devices, manufacture auth, replay commands or broaden a resource selection. Neko Core continues to own the agent loop; neither projection nor reconciliation executes work.

## Working-tree validation evidence

Baseline code commit: `fef7758f963e54bdb4f3356b9053564523ed6e3f`, branch `local-bot-file-ux-fef7758`. These package changes were not staged or committed by this worker. All results below are working-tree evidence, not a claim about an exact final checkpoint. The parent must record the final commit and rerun appropriate integration checks after its code is composed.

Environment: Windows amd64, Go `go1.27.2` at `C:/Users/Admin/Documents/Codex/2026-10-11/task/.tooling/go/bin/go.exe`, working directory `C:/Users/Admin/Documents/Codex/2026-10-11/task/checkpoint-verify-20261011/native`. `GOPROXY=off`, `GOSUMDB=off`, `GOTOOLCHAIN=local`, `GOENV=off`, `CGO_ENABLED=0`; provider/credential-shaped environment variables were removed without printing their values. `GOCACHE`, `GOMODCACHE`, `GOTMPDIR`, `TEMP`, `TMP` and validation logs use only `E:/CodexScratch/local-ux-fef7758/sync-owned/{cache,mod,tmp}` and its owned log files.

| Command / stage | Result | Owned evidence |
| --- | --- | --- |
| `go test ./internal/syncstream -count=1`, permissive envelope scaffold | FAIL as intended: unsafe/cursor/version/count and ambiguous/private/oversize wire accepted | `red-envelope.log` |
| `go test ./internal/syncstream -count=1`, permissive client scaffold | FAIL as intended: no initial snapshot requirement, gap accepted, two fixture projections incorrect | `red-client.log` |
| `go test ./internal/syncstream -count=1 -v`, first implementation | PASS | `green-initial.log` |
| `go test ./internal/syncstream -count=1 -v`, expanded boundary/replay/budget suite | PASS | `green-expanded.log` |
| `go test ./internal/syncstream -count=1 -v`, concurrent duplicate schedules and empty-backlog rejection | PASS, 14 top-level unit tests plus 3 fuzz seed cases | `green-final.log` |
| `go vet ./internal/syncstream`, before zero-access-revision correction | PASS, exit 0 | `vet-final.log`, `final-exits.json` |
| `go test ./internal/syncstream -run '^$' -fuzz '^FuzzDecodeEnvelope$' -fuzztime=15s -parallel=2`, before zero-access-revision correction | PASS, 293,258 executions, exit 0 | `fuzz-final.log`, `final-exits.json` |
| `go test ./internal/syncstream -run '^TestFileInitialZeroAccessRevisionPreserved$' -count=1 -v` | FAIL as intended: initial access revision zero rejected; verified against canonical ledger `file.accepted` validation | `red-zero-access-revision.log` |
| `go test ./internal/syncstream -count=1 -v`, final corrected zero-access-revision files | PASS, 15 top-level unit tests plus 3 source fuzz seed cases | `green-zero-access-final.log` |
| `go vet ./internal/syncstream`, final corrected files | PASS, exit 0 | `vet-zero-access-final.log`, `zero-access-final-exits.json` |
| `go test ./internal/syncstream -run '^$' -fuzz '^FuzzDecodeEnvelope$' -fuzztime=15s -parallel=2`, final corrected files | PASS, 200,254 executions with 68 cached/source seeds, exit 0 | `fuzz-zero-access-final.log`, `zero-access-final-exits.json` |

The test fixtures `testdata/client-a.json` and `testdata/client-b.json` specify independent duplicate/replay and out-of-order/reconnect schedules and verify equal final narrow state. Concurrent duplicate schedules are bounded goroutine fixtures. Unit fixtures are not actual journal/API transport, cloud sync, multi-owner isolation, browser acceptance, physical phone acceptance, model evaluation or native Rust/Tauri execution. Formal race detection is not claimed with CGO disabled. No provider, credentials, grants, deploy, push, native window or network is used by this package's tests.

Source, fixture and log hashes, current HEAD, exact final commands, environment, exit statuses and the value-free candidate scan outcome are frozen in the owned scratch `verification-receipt-final.json`. The earlier `verification-receipt.json` remains evidence for the pre-correction working tree. The initial sandbox-only attempt to create the assigned E scratch directory was denied before a valid test run/log was produced; the explicitly authorized scratch path was then used through reviewed escalation. This infrastructure attempt is not counted as a RED test result.
