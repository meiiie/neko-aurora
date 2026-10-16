# NekoBot exclusive ACP host

Status: local implementation candidate; not a published release or a claim about Wiii's bundled binary.

A trusted launcher starts `neko acp --host-profile nekobot`. The profile is immutable,
launch-selected and exclusive. ACP metadata, model text and tool arguments cannot
select it or widen it. It exposes only `mcp__nekobot__capabilities` (safe discovery)
and `mcp__nekobot__execute_tool` (gated host invocation). Native shell/filesystem,
computer, global MCP, procedural tools and subagents are absent. Canonical agent
trajectory, compaction and task checkpoints remain in Neko Core; this profile does
not expose the ordinary native memory tool.

NekoBot owns channel routing, its work-item and delivery records, host capability
validation and effect authorization. It must not run a second model/tool loop.
A host configured to own effect approvals may deliberately launch Core in `auto`
mode; that skips Core's MCP prompt but never grants host authorization. In default
mode an execute_tool request also passes Core's ordinary approval gate.

## Admission

- Require ACP v1 and initialize metadata `neko.taskProtocol` v1,
  `neko.executionProtocol` v1 and `neko.operationProtocol` v1.
- Require `neko.hostProfile` identity `nekobot`, version 1, exclusive ACP transport,
  the exact two-tool ceiling and operationProtocol 1.
- New/load/resume must opt into both task and execution v1. Missing opt-in fails
  before any host MCP connection. Use the task and execution receipt/echo rules
  from [task scope](ACP-TASK-PROTOCOL-V1.md) and
  [execution binding](ACP-EXECUTION-BINDING-V1.md).
- Supply exactly one in-band server:
  `{type:"acp",name:"nekobot",serverId:"<opaque host server ID>"}`.
  No client command, URL, other server or native tool surface is accepted.
- Validate the returned execution ceiling, independently authorized machine/root,
  process generation and task receipt, then persist mapping before first prompt.
  A receipt is correlation/policy information, not OS permission or confinement.

## Host operation v1

Core sends the existing `mcp/message` request with `method:"tools/call"` and:

```json
{
  "name": "execute_tool",
  "arguments": {"name":"host_tool_name","arguments":{}},
  "_meta": {
    "neko.operation": {
      "version":1,
      "operationId":"<32 lowercase hex characters>",
      "sessionId":"<Core coordinator ID>",
      "taskId":"<Core task ID>",
      "root":"<canonical physical root>",
      "activationEpoch":1,
      "activationId":"<current activation ID>"
    }
  }
}
```

The exact host tool schema is supplied by the host's tools/list. The profile only
sets the names/authority ceiling; host code must validate each invocation against
its own bounded declared schema and currently permitted capability set.

Operation IDs are cryptographically random, minted by the Agent for each new
canonical invocation and saved in its local `_neko_tool_operations` message
metadata before dispatch. They are independent of provider tool IDs, JSON-RPC
request IDs and model arguments. Hosted safe tools do not use eager execution:
their identity also passes the pre-effect checkpoint. Failed checkpoints dispatch
nothing. Plain JSON copies of an operation are rejected by the in-process boundary.
Private operation metadata is omitted from provider history. Never recover an
operation's authority by trusting arguments named `_meta` or `operationId`.

The host must validate sessionId/taskId/root/activation against its admitted
receipt and generation before every dispatch. Persist an operation ledger keyed
by sessionId + taskId + operationId, with immutable tool/argument fingerprint and
status. Repeated identity with different payload is a protocol error. Repeating a
completed operation returns its recorded result; an operation whose side effect
may have started without a result remains unknown until reconciliation. This Core
extension supplies canonical identity; it does not itself implement the host's
transaction/idempotency ledger.

A new model call after resume is a new invocation with a new ID even if the model
reuses its toolCallId or the same arguments. Semantically repeating an unknown
mutation is not made safe by this new ID: the host must still apply its pending /
unknown-effect policy. A restored unanswered Core tool call is sealed as unknown
and never automatically dispatched again. Resume rotates activation, preserving
old operation evidence. A revoked task scope cannot issue host calls.

## Results and cancellation

Return ordinary MCP content. `isError:true` is preserved as an Error-prefixed Core
observation, including denied/failed host results. Do not turn a pending approval,
takeover or denied effect into a success string. Approval pauses use the negotiated host-effect protocol below. Do not cancel before
Core consumes a typed pending receipt: cancellation can turn that call into an unknown
outcome and cannot be upgraded to a suspension later. Other interruption/takeover
paths may still cancel, preserving their distinct unknown/non-replay semantics.
Cancellation and transport failure are not proof of rollback. AcpHostMcp never
retries a tools/call after an uncertain result.

## Verification

`test/nekobot-host.test.ts` covers the exact profile, fixed-task/execution admission,
forged and duplicate/missing provider IDs, private-metadata stripping, durable
pre-effect identity, failed checkpoint denial, native tool exclusion, receipt
rotation, denied result persistence, cancellation and non-replay on resume. Existing
ACP/task/execution/profile tests protect compatibility with ordinary ACP and NekoCut.

Production acceptance additionally requires a real Core child process, the NekoBot
host ledger and channel outbox, duplicate inbound delivery, approval/cancel races,
crash after effect before ACK, and the actual Wiii/native or cloud host. An initialize
smoke or in-memory SDK test alone does not establish that end-to-end journey.


## Host-effect protocol v1: suspended approval

Only the restricted `nekobot` launch profile advertises
`_meta["neko.hostEffectProtocol"]={version:1}` during initialize. New clients require
it and opt in with that same metadata on `session/new` and `session/resume`/`load`,
alongside task-v1 and execution-v1. Opt-in is persisted; omission, addition or
version changes on resume reject. Existing sessions without it retain their
original contract and cannot promote cancelled/unknown calls to suspended work.

For **execute_tool only**, a trusted MCP response may contain:

```json
{"content":[{"type":"text","text":"Awaiting approval"}],"_meta":{"neko.hostEffect":{
  "version":1,"status":"awaiting_approval","operationId":"<original 32hex>",
  "effectId":"<durable host effect ID>","requestDigest":"<64hex>",
  "bindingDigest":"<64hex>","effectStarted":false
}}}
```

`requestDigest` is SHA256 of UTF-8 canonical JSON of the exact gateway invocation:
`{name:"execute_tool",arguments:<gateway arguments>}`. Canonical JSON recursively
sorts object keys lexicographically, preserves array order, and uses JSON primitive
encoding. Core computes it independently. `bindingDigest` is the host's immutable
actor/tenant/bot/thread/run/resource/approval binding digest. Core binds it by exact
equality; authenticating actors, resource revisions, run leases, and effect CAS is
the host's responsibility. The host inserts that association atomically before
returning pending. Text or argument lookalikes have no suspension authority.

Core keeps the original full `neko.operation` receipt (including its activation),
a typed pending tool slot, and a separate append-only journal in the same
revision-checked task snapshot. It checkpoints before acknowledging suspension,
stops before another provider step or tool, and aborts managed-provider continuation.
Queued and late managed callbacks cannot dispatch. Unexecuted later calls in the
same response batch receive explicit not-executed observations.

New/resume/prompt responses include:

```json
{"_meta":{"neko.hostEffects":{"version":1,"pending":[
  {"version":1,"status":"awaiting_approval","operationId":"...","effectId":"...",
   "requestDigest":"...","bindingDigest":"...","effectStarted":false,
   "operation":{"version":1,"operationId":"...","sessionId":"...","taskId":"...",
                "root":"...","activationEpoch":1,"activationId":"..."}}
],"unknown":[]}}}
```

ACP's standard prompt `stopReason` is `end_turn`; a nonempty typed pending list means
**suspended, not completed**. A host may dispatch the approved effect only after it
has correlated a Core-listed pending descriptor with its exact durable approved
record under the current lease and actor/resource authorization. A missing pending
ACK is not permission to infer suspension; reopen with the saved activation receipt
and inspect authoritative state. Prompt and compaction are blocked while any journal
entry is pending or terminal-unknown.

After the host's existing executor/CAS has durably recorded the outcome, send:

```json
{"method":"session/resolve_host_effect","params":{
  "sessionId":"...","_meta":{"neko.task":{},"neko.execution":{}},
  "resolution":{"version":1,"operationId":"<original>","effectId":"...",
    "requestDigest":"...","bindingDigest":"...","status":"completed",
    "result":"<actual durable tool observation>","resultDigest":"<64hex>"}
}}
```

Echo **current** task/execution receipts; the journal retains the original operation
activation. Terminal status is `completed`, `denied`, or `unknown`. `resultDigest`
is SHA256 of the exact UTF-8 result string (maximum 256 KiB). No replacement arguments
are accepted. This RPC never invokes a tool. It appends a terminal event and settles
only the exact proven-pending canonical tool slot, checkpointing before returning
`_meta["neko.hostEffectResolution"]` with the exact resolution and current receipts.
It cannot rewrite old unknown/cancelled observations. Pending audit evidence remains
unchanged and survives compaction separately from model context.

An equal duplicate returns the durable outcome without effect dispatch, including
after restart or lost ACK. A conflicting outcome/digest/binding, stale activation,
missing pending operation or protocol downgrade rejects. Terminal unknown remains
blocked and immutable in v1, requiring explicit operator recovery outside this
protocol. A failed checkpoint poisons the live runtime; it must not continue from
uncommitted memory. Reopen under the coordinator's recovery rules and use the host
ledger's persisted result. Never repeat an effect whose outcome is unknown.

The Core host tests cover ordinary and managed providers, no second call after
suspension, pending/reconciliation checkpoint failures, restart/lost ACK, duplicate
and conflicting terminal results, denied/unknown outcomes, stale receipts, wrong
operation/request/binding digests, opt-in downgrade, and spoofed metadata in text.
Host integration separately verifies authenticated actor/resource/FIFO/run-fence,
physical once-only effects, and durable result/side-effect crash windows.

## Credential-bearing model endpoint redirects

Core's Anthropic Messages, OpenAI-compatible Chat Completions, Responses and shared
keyed model-catalog requests reject all HTTP 3xx responses. Direct ChatGPT completion
uses the same protection. Both cross-origin redirects and same-origin path changes
are denied before another request; neither offline retry nor configured-model-list
fallback hides the denial. The error does not include redirect Location, response
body, request URL or credential values. This is an endpoint-containment rule, not a
claim of a complete audit of separate subscription account/catalog transports.

Anthropic Z.AI requests retain the explicit `https://api.z.ai/api/anthropic` route.
OpenAI protocol paths are rejected before key resolution; there is no automatic
switch to the paid API or another protocol.

The current generic Anthropic compatibility path maps GLM `low` effort to
`thinking:{type:"enabled",budget_tokens:2048}`. It also raises `max_tokens:4096`
to `10240` to leave its existing 8192-token response allowance. Therefore that
configuration is **not a 4096-token hard ceiling**, nor verified native GLM
`reasoning_effort:"low"` translation. Z.AI's current GLM-5.3 documentation requires
reasoning enabled and lists `low/high/max`; its 4096/top-level-effort example uses
Chat Completions. Do not silently substitute that route or claim live Anthropic
budget compatibility from a synthetic payload test. See the
[official GLM-5.3 contract](https://docs.z.ai/guides/llm/glm-5.3).
