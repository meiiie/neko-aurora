# Neko Aurora harness — gap analysis (2026-10-23)

Scope: the Go agent loop in `app/native/internal/aurora` (turn.go, tools.go, ai.go) that streams z.ai GLM 5.3, with Neko Core as fallback.

## 1. References studied

| Source | License | What we took |
|---|---|---|
| CopilotKit/openmuse (`apps/server/src/engine/model.ts`, `worker.ts`, `conversation.ts`) | MIT | serial checkpoint queue for parallel tool calls; `remember_fact` memories (user-confirmed only); lease + heartbeat durable worker; states queued/running/waiting_approval/waiting_input/succeeded/failed/cancelled; step-limit note ("say continue"); untrusted page text truncated at a cap; visible follow-up queue; send/stop in one control |
| CopilotKit/OpenDots (`src/server/dot-agent.ts`, `connections.ts`) | MIT | `MAX_RESULT_CHARS` + `[truncated]`; preferences injected into the system prompt; approval_required → single review tool → wait, never retry another way; current time in prompt |
| Grok Bot flow (`neko-bot-shots/grok-ref/grokbot-flow.mp4`, 48 s) | observed UX | user bubble → "Putting it together" thinking line → a short ack as its own message → work in background → milestone updates → final result as a separate message; a new message mid-work is taken in by the running task |
| Anthropic, *Building effective agents* | public article | prefer simple composable workflows (prompt chaining, routing, parallelisation, orchestrator-workers) over frameworks; tool design (ACI) matters more than prompt; ground on environment feedback each step; stopping conditions |
| OpenAI Agents SDK | MIT | run loop with max_turns, handoffs, guardrails, tracing spans per tool/model call, session memory |
| Claude Code / Codex CLI loop | public docs | single-threaded master loop, parallel read-only tool calls, auto-compaction near context limit, steering queue (messages typed mid-run are injected at the next step), interrupt = Esc/Stop, TODO list as visible plan |

## 2. Did Aurora already follow them?

Partly. Before this pass Aurora had: Grok-style ack → status → separate result, AG-UI event names, step plans with "Xong bước i/n" + push, approvals for sends, vault `SECRET:` cards, dedupe of repeated tool calls, a forced final answer after 5 rounds, one retry on transient errors, Stop via ledger cancel.

## 3. Gaps (ranked by value)

| # | Gap | Reference | Status |
|---|---|---|---|
| 1 | Only one TOOL per model call → N sequential round trips for "price of A, B, C" | Anthropic parallelisation, Claude Code, OpenMuse | **done**: up to 3 TOOL lines per call, fetched concurrently, committed in call order |
| 2 | No mid-turn steering: a new message waited until the turn ended | Claude Code steering queue, Grok flow | **done**: new owner messages are injected at the next model call; the queued turn is told it was absorbed |
| 3 | Tool results unbounded in context | OpenDots MAX_RESULT_CHARS, OpenMuse 30000-char cap | **done**: 6 KB per result, 24 KB per call, oldest shrunk first, marked "đã cắt bớt" |
| 4 | Long chats: hard cut at 30 turns, old decisions lost | Claude Code auto-compact | **done**: 40-turn window, older turns folded into a deterministic digest, last 8 turns verbatim, old long replies capped |
| 5 | No per-bot memory | OpenMuse remember_fact, OpenDots preferences | **done**: `MEMORY:` directive, user-stated facts only, secret-shaped text refused, 30 per bot, 0600 `memory.json`, GET/DELETE API |
| 6 | Single retry, fixed delay | SDK retry policies | **done**: 3 attempts, exponential back-off with jitter, never after first token |
| 7 | `agent.doing` blind during plans | Grok status line | **done**: shows "Bước i/n · title" |
| 8 | Restart = failed run ("interrupted_by_restart") | OpenMuse lease + requeue | **gap**: needs a new ledger event (`work.direct_resumed`) with replay validation; designed, not shipped (ledger is append-only and replay-validated, so this needs its own reviewed change) |
| 9 | Native function calling instead of text directives | OpenAI/Anthropic tool APIs | gap: GLM 5.3 supports `tools`; switching would remove the regex directive parser. Deferred: large prompt/test churn, current parser is robust in dogfood |
| 10 | Subagents / orchestrator-workers | Anthropic, OpenMuse delegate_task | gap: plan steps run sequentially in one context; parallel independent steps would be the next step |
| 11 | Tracing / eval harness | Agents SDK tracing | partial: ttft/total logged; `app/tests/aurora/harness-dogfood.mjs` is a scripted live eval; no scored eval set yet |
| 12 | Memory UI | OpenMuse personal context | gap: API exists, no settings screen yet (another worker owns the web assets) |

## 4. Design notes

- Parallel tools keep the OpenMuse invariant: network work concurrent, ledger cards/source numbers/context blocks committed serially in order so footnotes are deterministic.
- Steering text is labelled as an owner instruction, separate from untrusted web data.
- Memory is preference/context, explicitly "not an action instruction", never secrets (vault only). Account deletion wipes it with the state dir.
