# DeepSeek-Bot (DS Bot) study — 2026-10-23

Source: https://github.com/Fei2411/DeepSeek-Bot @ ae63b45, cloned to /workspace/deepseek-bot-ref.
License: Apache-2.0 (NOTICE: © 2026 Fei2411 and DS Bot contributors; bundles MIT code from DeepSeek Harness).
Treated as untrusted data. **Only ideas were ported; no code was copied.** Credited in THIRD_PARTY_NOTICES.md.

## What it is
A JavaScript plugin for DeepSeek Harness (DSH) that turns DSH sessions into a team of named Bots:
a Main Bot ("Chief"), per-bot chats, group chats with an admin and @handoffs (`message_bot`),
three-scope Markdown memory (bot / group / team), a "context steward", resume after restart,
non-blocking `ask_user`, `show_view` cards, secret cards, per-bot permission presets
(Read only / Workspace / Full), schedules, a usage page and SQLite FTS chat search.

## Key mechanisms
| Area | DS Bot | Aurora today |
|---|---|---|
| Memory write | Post-turn **memory review** (model reads new messages, keeps/updates/removes entries); `remember/forget/recall` tools; journal.jsonl of every change; size caps; secret refusal | Model must emit `MEMORY:` directive; 30 items/bot; secret refusal; no review |
| Memory format | AMR Markdown, MEMORY.md + topics, frozen per session for prompt cache | JSON list injected in system prompt |
| Context | **Steward**: small model call at 80 % of window decides "handoff to fresh part" vs "checkpoint in place"; checkpoints keep user messages verbatim | compactHistory + turn budget/checkpoint (budget.go) |
| Clock | Time context injected per step (refresh 10 min) | **none** — model guessed the year |
| Restart | inflight.json: running turns and undelivered messages resumed once at next start | Ledger lease + resumeOrFail (equivalent) |
| Approvals | Hidden sessions answered from group chat; presets per bot | Policy: outward/consequential only, AskFirst per bot (474db1d) |
| Multi-agent | Groups, admins, handoffs, team memory | Groups + handoffs, no team memory |
| Tool calls in text | Parses XML-like tool calls written into text | directives.go normalizes decorated labels/JSON |

## Better in DS Bot
- Memory does not depend on the model remembering to save; journal gives history/undo.
- Clock in context.
- Steward's handoff-vs-condense decision and verbatim user messages in checkpoints.
- Usage page and FTS search.

## Better in Aurora
- Mobile-first PWA, hosted multi-user hub, per-account isolation and vault.
- Sandboxed Railway computer with artifact verification (deckcheck, verify.txt, pivot).
- Cost discipline on GLM 5.3; DS Bot's extra model calls (review every turn, steward, summaries) would be too costly as-is.

## Adopted (this pass)
1. **Gated memory review** (memreview.go): after a turn that saved nothing, if the owner's message carries a lasting signal ("từ giờ", "luôn", "đừng", "gọi mình là", "always", "call me"…), one short GLM call extracts at most one fact (or `KHÔNG`) and stores it through the same Remember path (secret refusal, dedupe, cap). Runs async, once per request; most turns cost nothing.
2. **Clock note**: system prompt gets today's date and weekday in Vietnam time. Date-only so the prompt stays cache-stable all day.
3. **Restart stall fix** (hub + client):
   - `Hub.tenant` no longer holds the global lock while a workspace opens (ledger replay + resume); concurrent callers for the same account wait on a per-account gate (20 s cap); other accounts are never blocked.
   - Ledger open retries ~8 s when the previous container still holds the workspace lock during switchover; 503 carries `Retry-After`.
   - On SIGTERM the server disables keep-alives before `Shutdown`, so clients reconnect to the new container.
   - Client `api()` GETs have a 15 s timeout (covers a body that never arrives) and retry once on a fresh connection, including on 503.

## Later candidates
- Checkpoints that keep user messages verbatim (cheap, improves long tasks).
- Memory journal + "ask/update memory" panel.
- Team memory shared across a user's bots.
- Steward-like decision only at budget limits (one small call).
