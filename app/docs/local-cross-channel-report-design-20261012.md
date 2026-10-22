# Local cross-channel report delivery

The owner opens a shared Neko channel and asks `@Meiiie gửi báo cáo đã làm ở cuộc trò chuyện riêng vào kênh này`. Neko Aurora finds the permitted completed result and posts it in that original channel. A clear request does not need a file picker or another consent dialog. An ambiguous report needs only a short question about the missing subject. This implementation uses local channels and authored fixtures, with no external transmission or provider grant.

## Existing boundary and selected design

The baseline is delivery `16cbdf5a575309ee7bcfbb2e27a68f0c1f948c14`, application `f8fff28b5d21f8250f5b6501e92834dba93477b0`. Go owns journal, read authority and channel publication. Rust is the trusted host and Neko Core owns agent execution. The existing `publicationAllowed` and context-source checks keep a Work's raw context and reply in its own conversation. They remain intact.

| Candidate | Ownership and recovery | Decision |
| --- | --- | --- |
| A. Separate ReportDelivery | One typed journal event stores the original owner command, safe completed output, immutable source provenance and destination pin. A durable receipt owns the result across retry/restart. | Selected. Delivering a completed report needs no new model run. |
| B. Destination ReportWork | A new runtime task receives an output-only source capability, its own attempt and delivery phase. It requires new execution/context and recovery transitions. | Deferred. It adds runtime work without improving this requested transfer. |

The independent design review is `E:/CodexScratch/cross-channel-report-20261012/review/design-review.md`. No copied Work impersonates a new model execution. No same-conversation guard is relaxed to enable export.

## Authority and payload

The authenticated local HTTP session represents `local-owner` in `local-space`. These are current single-owner identities, not a multi-human login or connector ACL. The request cannot supply a different actor. Host, Origin, cookie, strict JSON and read-only restore guards remain in force. Source document text, a bot response or another agent cannot authorize a publication command.

A new command binds the stable called bot ID, exact human text, destination channel ID and audience revision, and client nonce. The server resolves only that bot's eligible completed private results. It chooses a unique report or an explicitly requested latest result; it asks a minimal question when none or multiple remain. The newest128 candidate window and latest selection use successful completion events at the current task epoch, rather than intake time or later access edits. Deterministic text matching is bounded retrieval, not a claim of semantic model quality.

The disclosure boundary is the completed Reply and authorized generated result artifacts. Source Text, raw context, conversation history, resource excerpts, uploaded input files, operations, credentials and runtime configuration are excluded. Result instructions remain literal content. They never trigger follow-on tools or expand the file selection. Provenance records source request/conversation, completion/epoch/access identity, exact result hash and output file hashes. It does not copy a private conversation title or prompt into the shared channel.

Source access and resource/context fences are separate from destination membership/publication. The called bot must be active and present in the destination. An archived private conversation can supply an existing permitted result; delivery does not reopen it. Its archive state alone is not a grant or access revocation. Authority is checked again at the atomic delivery commit. Current source access and destination fences also govern projection, export and destination-scoped download. A later revocation can hide current local content and links; it does not erase bytes already legitimately read or published externally.

## Journal and UI

The implementation contract uses `local_report_delivery_v1`, `POST /v1/report-deliveries` and a strict read-only receipt lookup. Initial input is `{botId,destinationChannelId,expectedDestinationRevision,clientNonce,text}`. The response is a separate delivery aggregate, never a full source Work. One fsynced event atomically owns the command and its delivered result or safe clarification. Replay validates historical authority at that event's sequence; later revocation must not turn a valid older event into global journal corruption.

The owner can answer an ambiguous report question with only its missing subject. A follow-up adds `clarificationOf` referencing the active original clarification, keeps its exact bot/destination/audience, uses a new nonce and stores the exact short human answer. The already durable original human request authorizes the requested result transfer. The bot's question is not an authority source. The server verifies the reference and scope before resolving the missing subject; an unrelated, revoked or cross-channel reference cannot grant publication. The UI never fabricates a full command and labels it as the owner's original text. A moved user, new draft or ordinary unrelated request cannot be silently captured by an old clarification.

An identical retry returns the original receipt and source pin, even when a newer report appears. Changed input with the same nonce conflicts. Ordinary intake and other command nonces cannot consume a reserved delivery intent. Reload uses read-only receipt/snapshot reconciliation. An unknown receipt exposes an explicit retry of the exact original command. It never republishes automatically or binds to the channel that happens to be visible later.

Journal selection policy is versioned separately from the public delivery DTO. Older candidate events without a policy field keep their historical intake-based selector for replay. New events use policy2 for completion order and strict command tokens. Current projections and downloads apply strict current intent even to older records; malformed historical commands retain a metadata receipt without restoring disclosure. Historical validity does not grant a new publication. This prevents a selection fix from corrupting an otherwise valid older journal or silently changing its original result.

The UI retains the approved light design and the name Neko Aurora. A delivered report has one owner command and one bot result in the destination. Late ACKs cannot change navigation, clear a new draft or steal focus. Current revoked content is not restored from an old ACK. Valid append-only text frames from an already known running task do not mask reports or fetch a snapshot per token. Unknown fields, changed authority/status, revocations and cursor gaps still trigger a synchronous visibility fence and read-only resnapshot. Same-channel two-bot collaboration and ordinary single tags remain distinct commands.

## Throughput checkpoint and verification

- Blocking first steps are baseline/source inspection, the two design candidates and the source/destination disclosure contract.
- Independent workstreams are Go/backend implementation, frontend/browser demonstration and independent authority/replay review.
- Shared mutable state has one writer per product layer. Root alone stages, commits and updates Main. Fixture state, profiles and logs have separate E scratch roots.
- The smallest safe decomposition is one ReportDelivery aggregate and matching boundary methods. Core, Rust host, account broker and remote transports are outside this slice.

Tests must include source/private canaries, ambiguity, bot identity, authenticated HTTP, stale audience/source revision, result-file download, duplicate and changed nonce, reciprocal ordinary-intake nonce collisions, crash before/after durability, restart and malicious replay. Snapshot, SSE, public export and UI projection need checks in addition to the DOM. Real local browser screenshots and video must show the requested journey against the compiled backend. Fixture results do not establish phone, Tauri, multi-owner, cloud or provider acceptance.

## Sources used

- [Development direction](../reference/handoff/neko-development-direction.md), DIR08/09, REQ-PB-004/005/012 and section 2.4, requires reply at the original destination and scoped private context.
- [Verified Grok client analysis](reviews/grok-reference-20261012/report.md), discusses distinct agent/room commands, nonce reconciliation and missing server implementation. The read-only bundle is a design reference; no code/assets are executed or reused.
- Official [pstack poteto-mode](https://raw.githubusercontent.com/cursor/plugins/main/pstack/skills/poteto-mode/SKILL.md), [Feature](https://raw.githubusercontent.com/cursor/plugins/main/pstack/skills/poteto-mode/playbooks/feature.md), [How](https://raw.githubusercontent.com/cursor/plugins/main/pstack/skills/how/SKILL.md), [Architect](https://raw.githubusercontent.com/cursor/plugins/main/pstack/skills/architect/SKILL.md), [Model the Domain](https://raw.githubusercontent.com/cursor/plugins/main/pstack/skills/principle-model-the-domain/SKILL.md), [Make Operations Idempotent](https://raw.githubusercontent.com/cursor/plugins/main/pstack/skills/principle-make-operations-idempotent/SKILL.md), [Separate Shared State](https://raw.githubusercontent.com/cursor/plugins/main/pstack/skills/principle-separate-before-serializing-shared-state/SKILL.md) and [Prove It Works](https://raw.githubusercontent.com/cursor/plugins/main/pstack/skills/principle-prove-it-works/SKILL.md) were read for this run. The available native agents adapt their ownership/review workflow. Prescribed Grok/Claude arena models are unavailable; no multi-model arena is claimed. User authorization covers implementation and technical choices. No PR/push/deployment is opened.

Capacity at start was C 986,607,616 bytes free and E 41,197,608,960 bytes free. New caches, profiles, test state and media use E. These are a point-in-time observation, not reserved capacity. Existing ZIPs, raw edits, state and older release packets remain protected.
