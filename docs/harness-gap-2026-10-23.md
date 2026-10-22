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
| 8 | Restart = failed run ("interrupted_by_restart") | OpenMuse lease + requeue | **done**: `work.direct_resumed` ledger event (replay-validated, max 2 resumes then failed); 90 s lease renewed every 30 s; sweeper requeues lapsed unowned runs. Resume restarts the turn from scratch (finished steps are redone) |
| 9 | Native function calling instead of text directives | OpenAI/Anthropic tool APIs | **measured, rejected**: z.ai GLM 5.3 supports `tools` (incl. parallel). Bench `app/tests/aurora/glm-fc-bench.py`, 6 tasks: both 6/6 correct tool choice; directives 12.8 s total vs native 28.3 s (≈2.2× slower). Kept directives |
| 10 | Subagents / orchestrator-workers | Anthropic, OpenMuse delegate_task | gap: plan steps run sequentially in one context; parallel independent steps would be the next step |
| 11 | Tracing / eval harness | Agents SDK tracing | **done**: 14 scored Vietnamese tasks `app/tests/aurora/eval/` (12 checks each: answered, Vietnamese only, no internal names, addresses “bạn”, tools, citations, must-contain, approval, secret card, plan, memory, steering order). Live: 14/14, mean 7.6 s |
| 12 | Memory UI | OpenMuse personal context | gap: API exists, no settings screen yet (another worker owns the web assets) |

## 4. Design notes

- Parallel tools keep the OpenMuse invariant: network work concurrent, ledger cards/source numbers/context blocks committed serially in order so footnotes are deterministic.
- Steering text is labelled as an owner instruction, separate from untrusted web data.
- Memory is preference/context, explicitly "not an action instruction", never secrets (vault only). Account deletion wipes it with the state dir.

## 5. Follow-up (same day)

- Steering message is now journaled on the absorbing run (`DirectRun.Steered`) and rendered before the rewritten answer; the absorbed queued turn finishes silently.
- Prompt: bot says “mình” and calls the owner “bạn”; eval check `addressesBan` passes on all 14 tasks.
- SJC price: sjc.com.vn sits behind a Cloudflare challenge from the server; `gold_price` falls back to PNJ’s public price feed (lists SJC bars), cited as its own source.
- Live restart test: 4-step plan, server killed during step 3, restarted → `resumed … (attempt 1)` → all 4 steps + final result delivered.
