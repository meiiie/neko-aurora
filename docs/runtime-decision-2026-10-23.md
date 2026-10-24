# Runtime decision: neko-core vs the Aurora Go harness (2026-10-23)

**Decision: keep the Go harness as Aurora's runtime; port neko-core's handling patterns (re-implemented, attributed). neko-core stays available as the optional local fallback (`Hybrid`).**

| Criterion | neko-core (github.com/meiiie/neko-core) | Aurora Go harness |
|---|---|---|
| Stack | TypeScript on Bun, terminal-first agent (Ink TUI, CLI `neko run`), ~2.3k-line `src/core/agent.ts` | Go, same process as the HTTP/SSE server, ledger and vault |
| Multi-tenant hosting | one process per session/CWD, per-user home on disk; hosting N users = N Bun subprocesses + process supervision | per-user workspaces already in one Go binary (hub, ledger per user, quotas) |
| Sandbox | its own tool sandbox for a developer machine (shell, files, browser) | bubblewrap + prlimit bot computers, approval cards, no network by default |
| Latency | measured earlier through the Core fallback: ~23 s to first word (process start + CLI turn) | direct streaming to GLM 5.3: ~2–4 s to first word |
| Licensing | AGPL-3.0-only outside `sdk/` (same owner, so usable, but any linked hosted use carries AGPL obligations; there is no Apache SDK yet) | Aurora's own code; MIT/Apache borrowings attributed |
| Maintenance | two languages and two state models (Core sessions + Aurora ledger) to keep in sync | one language, one journal, `go test ./...` covers it |
| What neko-core does better | mature coding-agent loop: checkpointed continuation after a broken stream, sealing tool calls left dangling by an interrupt, compaction tuned to token budgets, closed-loop review, a non-blocking composer whose input queues and drains in order | — |

## Ported patterns (re-implemented in Go, no code copied)

- **Never-blocking composer with an ordered queue** (neko-core `src/ui/chat.tsx` queue/drain): the web/iOS composer has an outbox; every message gets a client nonce and is delivered in order; retries across reconnect reuse the nonce (server dedupes).
- **Interrupt-and-merge at step boundaries** (neko-core abort-signal turns + Claude Code/Codex steering): the per-bot single runner polls its inbox while a model call streams; a new message cancels the stream, the draft is discarded and the message folded in (bounded, 3 interrupts). During a tool call messages are absorbed at the next step boundary.
- **Checkpointed continuation after interruption** (neko-core `runWithContinuation`): `work.direct_resumed` + lease/heartbeat (also OpenMuse `worker.ts`).
- **One visible reply per turn**: status lives in the status line, not a preamble bubble.

Attribution: THIRD_PARTY_NOTICES.md (neko-core section).
