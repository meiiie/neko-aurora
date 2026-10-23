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

## Full checklist (final, 2026-10-23, every row done)
Ideas only; no code copied (Apache-2.0, credited in THIRD_PARTY_NOTICES). Railway is a test deployment; the product is the downloadable app.

| Idea | Status |
|---|---|
| Steward checkpoints, verbatim owner msgs, model-decided compaction | done (3698dc0) |
| Memory journal + mobile memory panel (view/edit/delete) | done (0a29783, 3548d58) |
| Owner-scoped team memory shared across bots; group memory | done (0a29783, e4144fe) |
| Memory review | done (705f0b8) |
| Separate memory ask / summarize call | done: panel "Hỏi bộ nhớ" + "Gộp trùng lặp" (proposal, owner applies; journaled "summarize") (36cd11c, 59da315) |
| recall tool | done: memory_recall over own + team + group memory (36cd11c) |
| Budget-hit decision call | done (bec50ee) |
| Group chats, @mention, main-bot router, HANDOFF, [PASS] | done (d7d3e38) |
| Per-bot permission presets | done (d7d3e38, 801c2bb); the separate "Hỏi trước khi chạy lệnh" toggle was removed (e671923): cards only for the read-only preset or outward commands with network |
| Usage page | done (3698dc0) |
| Chat search + chat_search tool, reference a chat | done (0a29783, e4144fe) |
| Schedules | done (e4144fe) |
| Secret masking before model calls (also MCP results, thinking text) | done (e4144fe, 36cd11c) |
| Resume after restart, time context | done (705f0b8) |
| Model menu + providers | done: GLM default; OpenAI-compatible providers with the owner's key written only to the vault, never returned or logged; per-bot model; hosted mode dials via a private-network-blocking client (36cd11c) |
| Thinking display | done: per-bot toggle, reasoning stored per task, folded "Suy nghĩ của bot" under the reply, global show/hide (36cd11c, 59da315) |
| Vision / image fallback | done: photo upload from the + menu (downscaled on the phone), vision model (default glm-4.5v, configurable) describes it for the text model (image_describe) |
| Working folders | done: per-bot folder inside the bot computer; commands cd into it, relative paths resolve there; bots can set it via bot_update |
| Skills / capabilities | done: built-in + owner skills (name/desc in prompt, skill_load for the body), per-bot skill choice, capability switches web/computer/browser/mcp/bots/skills |
| MCP | done: remote Streamable HTTP and legacy SSE servers per bot, header auth from a vault secret; like DS Bot, remote calls run directly (stdio servers not offered in hosted mode) |
| create_bot / update_bot by bots | done, DS Bot behaviour: Main Bot creates bots and edits others, any bot edits itself (name, role, model, thinking, folder); applied directly |
| ask_user non-blocking | done (CHOICES) |
| show_view cards incl. forms | done: FORM directive → form card (text/number/date/select/textarea), answers come back as a message |
| Bot browser | done: headless Chrome/Chromium per call (fresh profile, read-only, public URLs only); hosts without a browser binary (the slim Railway image) fall back to the page reader and say so |
| Secret cards, web search, voice, themes/pin/avatars, home-lock, mid-turn steer, XML tool calls | existing |
| UX: fold long commands, Vietnamese file names, stacked bubbles, sandbox bold font | done (3548d58) |

Known limit: the Railway image ships no Chrome, so live "browser" runs use the fallback reader there; the desktop app and the box use real headless Chrome.
