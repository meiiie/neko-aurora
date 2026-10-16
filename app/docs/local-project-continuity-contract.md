# Local owner-authored project continuity

This slice implements the durable metadata part of direction §§2.6/2.7, REQ-PB-007 and SCN-PB-012. It does not implement a second semantic memory engine, compaction, model-generated memory acceptance or temporal reasoning. Neko Core retains the agent loop and semantic memory responsibility. A local owner explicitly writes instructions, confirmed or contested decisions, and dated source metadata.

Continuity is keyed by the exact `(projectId, channelId)`, outside the `Project` descriptor. Project membership is not blanket access to every channel's records. The authenticated local owner must still have the project attached to the requested channel. The local cookie is the existing owner principal; this is not a multi-owner connector ACL implementation.

## API

Capabilities: `local_project_continuity_v1`, `local_continuity_intake_cas_v1`.

`GET /v1/projects/{projectId}/continuity?channelId={exactChannelId}` returns `{continuity, usable}`. Missing or removed scope returns 404; absent metadata in an allowed scope returns revision 0. `usable` describes current recorded reference authority, not a provider check or proof that physical upload bytes remain available. Revoked or changed file references remain visible to the owner for repair, with `usable: false`.

`POST /v1/projects/{projectId}/continuity` replaces the bounded record. The path binds the project; a JSON `projectId`, owner, author, timestamp, grant or unknown field is rejected. All three expected counters are mandatory, including zero for the first continuity revision:

```json
{
  "channelId": "channel-<32 lowercase hex>",
  "expectedProjectRevision": 1,
  "expectedChannelRevision": 1,
  "expectedRevision": 0,
  "clientNonce": "owner-continuity-nonce",
  "instructions": "Compare dated sources; retain identity distinctions.",
  "decisions": [
    {
      "id": "price-choice",
      "text": "The price remains disputed.",
      "status": "contested",
      "effectiveDate": "2026-10-11",
      "entityId": "vendor-a",
      "referenceIds": ["quote-old", "quote-new"]
    }
  ],
  "references": [
    {
      "id": "quote-old",
      "title": "Alex quote",
      "sourceDate": "2026-10-01",
      "entityId": "person-a",
      "version": "1",
      "url": "https://example.invalid/quote-old"
    },
    {
      "id": "quote-new",
      "title": "Alex quote",
      "sourceDate": "2026-10-10",
      "entityId": "person-b",
      "version": "2",
      "file": {
        "fileId": "file-<32 lowercase hex>",
        "sha256": "<64 lowercase hex>",
        "accessRevision": 0
      }
    }
  ]
}
```

The response is `{continuity, duplicate}`. Server provenance includes `version: 1`, exact project/channel IDs and admission revisions, monotonic continuity `revision`, `ownerUserId`, `spaceId`, `authorType: "owner"`, `recordedAt` and SHA256 of the canonical Go JSON record with its SHA256 field blank. An AI cannot mint authority through an author field. An owner's `confirmed` label is an explicit owner assertion, not a model evaluation or independent verification.

Identical nonce and payload returns the original receipt, including after a newer revision, while the original exact scope remains owner-readable. A changed nonce payload is 409. A retry after a removed scope is denied; it does not recreate the operation. All records and nonce receipts are journaled. Replay validates the typed exclusive event, scope, provenance, reference descriptors, digest, revision and size. A metadata commit preceding suspension is safe: runtime/publication guards check current authority immediately, then recovery suspends dependent tasks.

Clear metadata by explicitly posting empty instructions and empty arrays at the current revision. This still creates a new durable revision and fences older pins. Project renames or channel revision changes do not rewrite metadata or silently discard its recorded provenance. A subsequent new request validates current project/channel authority.

## Bounded records and references

The encoded UTF-8 JSON record, including server provenance and digest, is at most 6144 bytes. Instructions are at most 2000 UTF-8 bytes; decision text at most 1000 bytes. Up to 8 decisions and 8 references are accepted. IDs are unique safe tokens of 1–64 characters. Titles are at most 160 bytes, explicit entity IDs and source version strings at most 80 bytes. Instructions and decision bodies allow ordinary line breaks and tabs; other control characters are rejected. Raw decoded fields and the encoded record are scanned for recognized credential patterns before commit; this is pattern detection, not a guarantee against arbitrary secrets in prose.

Each reference is exactly one existing uploaded file descriptor or one descriptive HTTPS URL. File references require exact project/channel, ready text status, hash, granted access and current access revision. They do not expand the selected `Resources.Files` read boundary or expose file content. A URL is never fetched by this endpoint; user info, query strings and fragments are rejected. Dates, when supplied, are valid `YYYY-MM-DD` strings. Missing dates remain missing. Effective dates, source dates, source versions and entity IDs are retained literally; upload order does not choose an effective quote or merge people with the same name. Decision reference IDs must name existing record references, with no duplicates. An empty reference list is an owner statement with server provenance.

The entire bounded record from the exact channel is captured as an optional `ResourcePin.Continuity` with revision, SHA256 and private metadata. No model ranking, memory derivation or recursive source expansion occurs here. Existing bounded source/history selection remains in place, and final intake uses the shared 64000-byte encoded runtime-frame bound before commit.

Messages and collaborations accept optional `expectedContinuityRevision`, advertised by the intake CAS capability. An explicit pointer must match current revision, with 0 for absent metadata. Omitting it captures the current authorized record. Duplicate message nonces compare the original pin before current-state checks; they retain the original task, resource selection and receipt. Collaboration input freezes the optional expected counter, and all stages retain the original resource pin.

## Fences and projections

Creating, clearing or changing continuity invalidates earlier project task pins in that channel, including historical nil pins once a current record exists. Dispatch, text, effects, completion, channel artifact publication and unscoped artifact download reject stale continuity. Completed stale results cannot enter new pinned context. Active work enters suspending; proof of actual process exit remains the existing native runtime contract. This slice tests typed store bindings and does not claim new native process evidence.

Revoking a referenced file fences tasks even if that file was not in the separately selected read list. Regrant does not restore a stale reference access revision. The owner must write a new continuity record and create a new request. `RefreshContext` rejects stale continuity; it does not recapture or replace resource selection. A collaboration stops with its original group/stage resources intact; it cannot auto rebind or advance after the fence.

Internal `Snapshot` and `Get` remain full copies for runtime/recovery. `PublicSnapshot`, published `Events`, and posted work/control/group responses strip continuity metadata while retaining revision/hash descriptors. Only the exact-channel GET and explicit metadata write receipt return the full record. Public context hashes still refer to the original private pin; clients must not recompute a private pin digest from its redacted projection. Integrator command receipts must also strip `Continuity.Metadata` from receipt resources, retain the typed input including expected continuity revision, and preserve internal full pins.

Legacy journals have no new field; historical replay remains valid. Introducing current metadata fences their old project pins at dispatch/recovery without treating legitimate metadata changes as global corruption. Malformed mixed event payloads still fail journal validation. No frontend, Core, provider, connector grant, external fetch, deployment, public push or production state is changed by this slice.

## Validation scope

Owned tests exercise owner API authentication, explicit CAS, stale intake, original nonce identity, server provenance, dates and homonymous entities, current exact-channel file authority, copy isolation, concurrent CAS, metadata-only redaction in work/control/group replies, encoded bounds, raw secret rejection, empty record replay, legacy nil pins, commit-before-suspension restart three times, frozen collaboration stop, late output/effect/completion fences, both artifact routes and completed-history exclusion.

Exact checkpoint commands, environment and pass/fail/blocked receipts are frozen separately under the owned `.build` evidence directory. Earlier RED failures are retained. These tests are local Go journal/API fixtures. Actual Core semantic interpretation, native process stop behavior with this new slice, independent review, UI wiring, physical phone, multi-owner ACL, cloud sync and real-provider memory quality remain separate acceptance gates. Formal Go race detection remains blocked by absent GCC; bounded concurrent schedules are not a substitute.
