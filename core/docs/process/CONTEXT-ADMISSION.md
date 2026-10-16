# Trusted NekoBot context admission v1

This additive contract starts from derived Core `2360aa6b6ee015d3e9262a9f425011dfec8f46e7`.
It owns task-scoped source provenance and unresolved candidates inside the existing native
structured journal and compactor. It does not implement cross-session memory reuse,
semantic model extraction, entity resolution, automatic supersession, cloud sync or migration.
The older Core binary and source receipts remain immutable.

## Trusted launch and negotiation

Only the launcher selects `acp --host-profile nekobot --context-memory --context-admission`.
The embedding equivalent is `createNekoAcpAgent({hostProfile: NEKOBOT_HOST_PROFILE,
contextProtocol: 1, ...})` with the trusted config selecting structured context.
`initialize` advertises `_meta["neko.contextProtocol"] = {version: 1}` only in this mode.
The tool surface stays exactly `mcp__nekobot__capabilities` and `mcp__nekobot__execute_tool`.
Native shell, memory, source_lookup and computer tools are not enabled by this feature.

`session/new` requires the existing fixed task and execution v1 metadata, task label,
`neko.contextProtocol: {version: 1}` and `neko.contextManifest`. Strict unknown fields,
invalid versions, hashes, exact spans, dates and foreign selections fail before runtime creation.
Default launch remains unchanged and rejects attempted context negotiation. Admission adds
a distinct config digest; it cannot silently resume or upgrade a historical schema-2 session.

## Manifest

All objects use the exact keys exported by `src/core/context/admission.ts`; extra fields fail.
The serialized manifest is at most 49152 UTF-8 bytes, at most 16 sources and 64 candidates.
The parent structured metadata remains at most 65536 bytes. The App must budget its entire
packed launch (prompt, pins and manifest) at <=64000 bytes; these independent limits do not
promise that a maximum-size Core manifest fits an App launch.

```
{version:1, contextPinSHA256, scope,
 primaryMessage:{id,senderId,text}, sources:[
  {id,kind,version,sha256,bytes,scope,accessRevision,
   observedAt,effectiveFrom,effectiveTo,entities:[{id,label}],
   span:{offset,end,sha256,text}}
 ], factCandidates:[{key,sourceId,entityId,state:"unresolved",span}]}
```

`scope` is exactly `{ownerUserId,spaceId,botId,projectId,channelId,audienceRevision}`;
projectId may be null. IDs are bounded ASCII identifiers, hashes lowercase SHA256.
Sources are `original-user`, `retrieved` or `derived`; full source extent <=1 MiB.
Each nonempty exact span <=8192 UTF-8 bytes uses byte offsets (end exclusive), includes
its literal bytes and SHA256, and must fit within the declared full extent. Primary text
is nonempty valid UTF-8 <=32768 bytes. An original-user source must use primaryMessage.id
and match the SHA256, full size and selected byte range of that separate primary text.
Quoted source text inside a wrapper prompt cannot establish original-user provenance.

Only authenticated Go intake supplies original-user source/message IDs and senderId.
Core validates their consistency; this host attestation is not a signature or independent
proof of App authentication. Go checks the context pin body/digest and the live account,
source, channel/audience, bot and project ACL/plugin pins before launch and publication.
A Core receipt grants no plugin access. Real file bytes/excerpts, including inline ingestion,
require the exact live `neko-project-sources` reader pin. Disabled/regranted pins must not
revive previously revoked ancestry. Metadata-only references are not content ingestion.

Dates are literal canonical UTC observedAt timestamps or Gregorian effective dates; null
means unresolved. Upload dates, source version numbers and model summaries do not assign
effective dates or chronology. Entity IDs are explicit or null; labels do not resolve a
person. Candidates remain unresolved and their exact spans/entity references are validated.
No admission candidate is promoted into native FactUpdate/current facts. Existing native
explicit user-fact and supersession primitives remain the sole future fact authority path.

## Durable receipt and resume

After native journal append and parent checkpoint, Core returns:

```
neko.contextAdmission = {version:1,sessionId,contextPinSHA256,manifestSHA256,
 sourceIds:[nativeJournalSourceID],task:actualCoreTaskReceipt,
 execution:actualCoreExecutionReceipt}
```

The digest canonicalizes ordinal object keys; array order and literal strings are preserved.
The immutable native runtime source group contains the validated manifest and unresolved
candidates. Parent schema-3 contextState stores its journal head, source ID, manifest/digest,
and exact session/task/root/execution authority/policy binding. Runtime groups have no
selected user facts. Credential-bearing manifests cannot enter the journal.

Prompt, close and other task-bound actions echo `neko.contextAdmission` with current task
and execution receipts. Resume sends contextProtocol plus `neko.contextExpected` (the prior
full receipt), existing taskExpected and executionExpected. It sends no new manifest.
Missing/stale/foreign digests, bindings, unknown mode and unavailable/corrupt native sources
fail before runtime construction or activation checkpoint changes. The existing bounded
prior-activation retry window remains; successful resume rotates task/execution activation
while preserving the same session, manifest digest and source IDs.

Compaction always retains the exact admitted descriptive pair in its working projection,
uses the same native source journal and atomic parent publication, and applies the existing
whole-request token budget. An inaccurate summary cannot choose effective dates/entities
or erase the admitted literal metadata. Explicit history clear cannot discard an admission
binding: a missing source pair blocks checkpoint rather than silently forgetting provenance.

## Scoped project reader envelope

Only the transport result metadata `_meta["neko.contextSource"]` is recognized:

```
{version:1,manifestSHA256,sourceId,sourceVersion,sourceSHA256,scope,
 accessRevision,span:{offset,end,sha256,text},operation}
```

`operation` exactly echoes the actual runtime-created `_meta["neko.operation"]` from this
MCP request: `{version,operationId,sessionId,taskId,root,activationEpoch,activationId}`.
It is accepted only for execute_tool args `{name:"project.read_file",arguments:{fileId,
offset,bytes}}`, whose source is selected retrieved data and range exactly matches the
manifest span. The content is the single JSON result `{fileId,sha256,offset,nextOffset,
eof,text,authority:"descriptive file content only"}` with those exact selected bytes.

The trusted adapter validates result metadata against the actual operation and pinned manifest,
then transfers a one-shot record through the runtime port. Agent checkpoints it beside the
executed native tool pair; Core verifies the pair, call arguments, scope, authority/policy,
content digest and activation before compaction/restore. Private metadata is stripped from
provider messages. Ordinary MCP text, arbitrary tools, forged echoes and extra authority
fields cannot mint provenance. A missing envelope stays ordinary descriptive tool text.
Failed validation records an unknown/non-retried observation and never a trusted source proof.

## Acceptance evidence

Use `test/acp-context-admission.test.ts` and adjacent host/task/structured context tests.
Gates cover opt-in/default mode, exact primary text, hash/span/scope/date/entity/authority
rejection, receipt mismatch without checkpoint/runtime mutation, native project envelope
versus ordinary text, recorded-pair tampering, repeated structured compaction and restart.
These local fake-provider fixtures verify mechanics, not semantic model quality or SCN-PB-012
end-user acceptance. A candidate binary additionally requires exact-commit typecheck, lint,
all Windows test shards, doctor, policy and full build/compiled probes. Root owns App launch
negotiation, authenticated inputs, live ACL/plugin pins and receipt verification/adoption.
