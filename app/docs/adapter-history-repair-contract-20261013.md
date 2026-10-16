# Adapter authoritative history repair v1

Status: FROZEN shared contract before production edits. Root, the Neko/Wiii implementation owners and independent QA reviewed the exact encoding/cursor/authority and atomic-publication rules; no schema blocker remained. Neko baseline `eaeb54efe55ec0450a6b85a43b853d6032abfed4`; available Wiii source `43dd366c22f2b2af51e9dbf1f99f4fbd7bd5138f`. Installed/running Wiii identity remains unknown. This local repair rebuilds presentation history for an existing binding; it does not recreate Core memory, change an accepted input/nonce, register a session, grant authority or start execution.

## Capability and requests

`GET /v1/adapters/capabilities` remains version 1 and adds `local_adapter_history_repair_v1` to its existing capabilities list. An explicit owner action uses `GET /v1/adapters/sessions/{id}/history` with no query/body. Continuation uses only `?cursor={token}`. Unknown/duplicate/empty parameters, query on the initial request, noncanonical escaped path, body and non-GET methods reject. Existing registration/message/delta paths and limits remain unchanged.

The public ledger method is proposed as `AdapterHistoryPage(id string, cursor string, signingKey [32]byte, now time.Time) (AdapterHistoryPage, error)`. Root owns API signing-key creation and route/query/error hooks; the key is an independent cryptographically random 32-byte value per Server lifetime, never the auth cookie or grant. The ledger worker owns a new `adapter_history.go` and tests. No Store field/index, startup mutation, existing delta change or new execution IPC is required.

Limits: 128 rows and 900,000 encoded JSON bytes per response; at most 8,192 rows and 67,108,864 canonical payload bytes for one repair; five-minute absolute lifetime. No limit silently drops or truncates a row. The 64 MiB budget measures the canonical history digest input defined below, excluding its fixed scope/header. This counts exact UTF-8 content/metadata once, independent of JSON escaping. Response size separately measures the actual encoded JSON. Native storage may enforce an additional explicit storage-failure bound; it must retain prior receipt/intent metadata and keep presentation held.

## Closed page and row schema

Every 200 page has exactly these fields:

```json
{
  "version": 1,
  "adapterSessionId": "adapter-id",
  "sourceApp": "wiii",
  "sourceSessionId": "original-source-session",
  "sourceThreadId": "original-source-thread",
  "channelId": "original-channel",
  "channelRevision": 1,
  "ownerUserId": "local-owner",
  "spaceId": "local-space",
  "returnRoute": "/v1/adapters/sessions/adapter-id/replies",
  "snapshotId": "h1_64-lowercase-hex",
  "snapshotCursor": 42,
  "authorityDigest": "64-lowercase-hex",
  "historyDigest": "64-lowercase-hex",
  "offset": 0,
  "totalRows": 1,
  "totalBytes": 300,
  "nextCursor": null,
  "confirmationCursor": "signed-final-confirmation-token",
  "complete": true,
  "rows": [{
    "replyId": "reply-request-id",
    "requestId": "request-id",
    "sourceMessageId": "original-source-message",
    "botId": "bot-meiiie",
    "role": "assistant",
    "text": "owned reply",
    "status": "completed",
    "sequence": 40,
    "artifacts": [],
    "availability": "available"
  }]
}
```

`sourceApp` is the existing session's `wiii` or `core-tui`; Wiii accepts only its own `wiii` binding. All binding fields must equal the original session, including its old returnRoute. IDs follow the existing bounded ASCII ID rules; numbers are nonnegative safe integers and channelRevision is positive. Digests are 64 lowercase hex digits. `snapshotId` is an opaque `h1_` plus 64 lowercase hex. Empty history is valid: totalRows/totalBytes/offset zero, rows empty, complete true, nextCursor null.

Rows use the existing stable assistant identity and actual durable sequence, with required artifacts array. Availability is exactly `available` or `source_unavailable`. A denied/unknown current context yields empty text and artifacts plus source_unavailable; identity/status remains. No input body, original nonce, selected credential/resource material or full Work enters this response. Available rows preserve every complete reply and artifact receipt within the stated budget. Accepted but unanswered work remains a row with empty text, status and identity. Every returned sequence is an actual latest full Work event at or before snapshotCursor; snapshot masking is explicitly a current projection, so its body may differ at that same sequence. The new snapshot is replaced atomically; these rows must not be passed to old incremental equal-sequence upsert logic.

Order is ascending original Work.CreatedSequence, then RequestID ASCII bytes. Pages require exact contiguous offset; next offset equals offset+rows.length. No duplicate request/reply/source-message identity is permitted. `complete` is true exactly at totalRows; otherwise nextCursor is a nonempty signed cursor and confirmationCursor is null. A complete page has nextCursor null and confirmationCursor is the signed cursor at offset=totalRows. The client verifies total rows, total bytes and the final history digest before publication.

## Frozen projection and live authority

The first request captures the journal head as snapshotCursor, reconstructs each matching adapter Work's latest full state at that head and sorts it as above. Checkpoint events containing only requestId never replace full Work state. Only the exact requested session's Work enters the projection. Later work.text events do not replace frozen text or status.

For this bounded first version, the authority epoch is the sequence of the last journal event whose type is not work.text. This is deliberately conservative: unrelated typed metadata/commands can restart repair, while ongoing text streaming alone does not. The live predicate for each frozen row is contextAllowed(current stored Work) AND contextAllowed(frozen Work). A missing current Work is unavailable. Every page recomputes the authority digest over that epoch and all frozen rows' current predicates. This catches revoke/regrant ABA even if the final grant Boolean returns true, and catches an unknown in-memory denial with no event. Channel revision/current binding is checked separately before any page. Changed epoch/predicate rejects the whole continuation, never mixes generations or restores old cache text.

The final page checks the same live authority while holding the ledger mutex. The client stages all pages privately, computes the full digest, persists only a versioned candidate manifest, then revalidates the final cursor before making that manifest visible. A final cursor may be requested again: it returns a complete zero-row confirmation at offset=totalRows with the same witnesses, after live authority validation. The client does not append that confirmation to digest input. Visibility also requires unchanged native lease/generation/source and UI navigation ownership. After a reload, cached bodies stay authority-held until live validation. This defines a live verification boundary; it does not promise instant physical deletion or retract data already delivered before a later revocation.

The Wiii implementation owner owns explicit candidate-versus-active manifest publication state: a crash or failed storage write between private staging and final confirmation cannot activate that candidate on reload. The native errors connection_changed, unknown_link and disconnected hold presentation while keeping original intent/accepted nonce metadata. A refused untrusted delta/repair frame, including a frame that contains a clear before an invalid unknown row, also holds the prior visible cache; it cannot leave the old revoked body visible merely because atomic parsing rejected the rest of the frame. Root/QA exercise these guards against current-source modules; they do not claim OS dialog or phone acceptance.

## Portable canonical digest bytes

All integers below are unsigned big-endian. `S(x)` is u32 UTF-8 byte length followed by exact UTF-8 bytes; lengths are bytes, not UTF-16 characters. No Unicode normalization, JSON serialization, HTML escaping or key order is involved. Unpaired surrogates/invalid UTF-8 reject. `U(x)` is u64; `B(x)` is one byte 0/1. SHA values inserted as `H(x)` are their decoded 32 bytes. Domains are exact UTF-8 strings including the final NUL.

`Scope` is S(adapterSessionId), S(sourceApp), S(sourceSessionId), S(sourceThreadId), S(channelId), S(ownerUserId), S(spaceId), S(returnRoute), U(channelRevision), in that order.

`authorityDigest = SHA256("neko-adapter-authority-v1\0" || Scope || U(authorityEpoch) || U(totalRows) || for each frozen row: S(requestId) || B(livePredicate))`.

`Row` is S(replyId), S(requestId), S(sourceMessageId), S(botId), S(role), S(status), S(availability), U(sequence), S(text), u32(artifacts.length), followed by each artifact in original durable order: S(id), S(filename), S(sha256), U(bytes). Artifact SHA remains its exact 64 lowercase-hex string. `totalBytes` is the sum of Row byte lengths.

`historyDigest = SHA256("neko-adapter-history-v1\0" || Scope || U(snapshotCursor) || H(authorityDigest) || U(totalRows) || Row[0] || ... || Row[n-1])`.

`snapshotId = "h1_" + hex(SHA256("neko-adapter-snapshot-v1\0" || Scope || U(snapshotCursor) || H(authorityDigest) || H(historyDigest)))`.

Authored shared vectors will be saved in `scripts/adapter_history_repair_vectors_20261013.json`, including HTML/control/NUL/U+2028/U+2029/non-BMP Unicode and masked rows. Go and current Wiii must match these bytes/hashes before integration.

## Signed cursor

The raw cursor is version byte 1, SHA256(Scope) 32 bytes, U(snapshotCursor), U(offset), U(issuedAtUnixMs), U(expiresAtUnixMs), H(authorityDigest), H(historyDigest), then HMAC-SHA256(signingKey, all preceding bytes). Base64url without padding yields exactly 215 ASCII characters; max cursor accepted is 256. The original adapter ID plus this token fits the existing 512-byte native path bound. The server checks length, encoding, MAC with constant-time comparison, scope hash, bounds, deadline and witness before materializing a page. Cursor expiry is exactly issuedAt+300000 ms; now before issuedAt or after expiry rejects. A restarted server has a new independent signing key; old cursors safely fail validation. No signer value appears in public DTO/log/evidence.

## Error distinctions

The new route returns only `{ "error": "agreed_code" }` for its typed errors; native forwarding admits this closed enum only, never arbitrary error bodies:

| HTTP | Code | Client behavior |
| --- | --- | --- |
| 400 | adapter_history_cursor_invalid | Keep held cache/receipts; start a fresh explicit repair; includes tamper or retired server signer |
| 403 | adapter_history_binding_mismatch | Hold; do not select/register another scope |
| 409 | adapter_history_scope_changed | Hold; original scope needs explicit owner resolution |
| 409 | adapter_history_authority_changed | Discard provisional pages; keep original intent/receipt metadata held |
| 410 | adapter_history_snapshot_expired | Discard provisional pages; offer fresh explicit repair |
| 413 | adapter_history_budget_exceeded | Explain row/total bound; no silent truncation or deletion |
| 404 | not_found | Preserve original binding and receipts; no new session |
| 401/403 | existing auth/principal error | Hold; use existing owner connection flow |
| 503 | store_unavailable | Preserve held metadata and offer retry |

Capability absence/404 on an older peer means unsupported repair. The existing generic delta409 does not prove a limit; UI names the stopped history and explicit repair opportunity, then displays the new route's precise error. Unknown/denied rows remain masked in a successful snapshot. Old v1 delta handling remains fail closed; current Wiii must choose a compatible refresh policy for the repaired authoritative cache rather than exposing it after an untrusted failed delta.

## Current legal row and available client gates

Go AppendText/replay limits legitimate replies to 65,536 UTF-8 bytes. Runtime operations/artifacts are capped at eight; produced filenames at 80 bytes. Worst valid text JSON escaping expands at most sixfold, so a legitimate row fits 900,000 bytes even with all bounded metadata/artifacts. A maximal legitimate control-character fixture proves this; three such rows require split pages. A malformed/future oversized row is explicit 413, never an invented successful truncated result.

Current Wiii43dd saved outbox and replies both cap at 100, and every reply depends on an accepted outbox entry. The repair therefore requires versioned receipt/history shards, each bounded to 100 entries, with the pending queue still capped at 100. Known accepted original inputs/nonces remain immutable; unknown authoritative rows are observed identity records, not fabricated accepted inputs or resend authority. Atomic manifest completion and native route/capability/error forwarding require actual current-source fixtures. Historical C9 snippets are not sufficient; native UI/CUA/Chrome/phone/installed-binary gates remain blocked.
