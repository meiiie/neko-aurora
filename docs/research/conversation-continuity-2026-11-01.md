# Conversation continuity: follow-up messages without replacing answers

Date: 2026-11-01. Repository: Neko Aurora, branch `codex/openmaus-interface`.
Base commit: `fe49941ff65eaa00c57ce4f05ac3ebbdf9cb12d9`.

## Report and reproduced failure

The owner asked for a joke, then added a request to draft a professional email while Meiiie was streaming. The original milk/six joke stopped at “Anh về”; a second answer replaced it with a bread/ten joke. The previous patch preserved the truncated prefix but retained the interruption mechanism. Its live acceptance case explicitly asked to stop a story, so it did not establish additive-follow-up continuity.

`TestFollowupDoesNotCancelOrRewriteVisibleAnswer` was added before changing production code. On the base implementation it failed: the first call logged `context canceled`, then the next call produced the replacement. The baseline log is in `.audit/2026-11-01-conversation-flow/baseline.log` (ignored local evidence).

There were two cancellation/replacement paths: `ask` polled the inbox every 250 ms and cancelled an active stream; `runTurn` could regenerate a finished answer when late input was pending. Both consumed a new request as grounds to discard the current answer. Saving a partial bubble did not correct this lifecycle.

## Source research

These are source-level observations at pinned commits, not a claim of running OpenMuse or adopting its whole architecture. OpenMausBot's separate live comparison from the preceding investigation remains available. The original reference repositories were kept intact; no reference implementation or assets were copied into Aurora.

| Reference | Observation | Aurora decision |
| --- | --- | --- |
| [OpenMausBot admission.ts](https://github.com/milind-soni/OpenMausBot/blob/88d54eb17a3655b968470cbbf3b5097de225a8b5/server/admission.ts) | Busy direct chat steers only when the engine declares that capability. Unsupported steering or images are queued. | Current Core/HTTP stream adapters cannot steer an active call. Finish the active response; retain arrivals in the durable inbox. |
| [OpenMausBot Codex driver](https://github.com/milind-soni/OpenMausBot/blob/88d54eb17a3655b968470cbbf3b5097de225a8b5/server/drivers/codex.ts) | Native `turn/steer` identifies the active turn. Delta events and completed assistant items have distinct handling. | Do not imitate native steering by cancelling and replaying a prompt. Store the complete answer before ending its draft. |
| [OpenMausBot bot-message routes](https://github.com/milind-soni/OpenMausBot/blob/88d54eb17a3655b968470cbbf3b5097de225a8b5/server/routes/bot-messages.ts) | Queue admission distinguishes acknowledged steering, refusal and indeterminate execution. Indeterminate sends must not be replayed blindly. | Keep the existing request nonce/journal and one runner per bot. Do not create an extra model call just because input arrived during streaming. |
| [OpenMuse conversation queue](https://github.com/CopilotKit/openmuse/blob/a4944f4c471d3dc93b8bfc060b5da309b5216ab2/apps/mobile/src/conversation-queue.ts) | One run at a time while composing remains available. Failed in-flight messages are not silently resent; refusal before execution can restore a pending item. | Keep the composer usable and show an explicit received/waiting state. This patch changes follow-up admission, not Aurora's existing transient-provider recovery policy. |
| [OpenMuse chat](https://github.com/CopilotKit/openmuse/blob/a4944f4c471d3dc93b8bfc060b5da309b5216ab2/apps/mobile/src/chat.tsx), [run errors](https://github.com/CopilotKit/openmuse/blob/a4944f4c471d3dc93b8bfc060b5da309b5216ab2/apps/mobile/src/conversation-run.ts) | Run locks, queue state, explicit stop and save errors are separate. A resolved run promise alone is not success evidence. | Assert journal content, SSE lifecycle and reloaded UI, not only a successful HTTP response. Keep explicit Stop distinct from an ordinary new message. |
| [OpenMuse conversation engine](https://github.com/CopilotKit/openmuse/blob/a4944f4c471d3dc93b8bfc060b5da309b5216ab2/apps/server/src/engine/conversation.ts), [worker](https://github.com/CopilotKit/openmuse/blob/a4944f4c471d3dc93b8bfc060b5da309b5216ab2/apps/server/src/engine/worker.ts) | Conversation and durable background tasks have separate lifecycles. Claims depend on tool/action receipts; task workers use leases and checkpoints. | Preserve Aurora's trusted execution/approval boundaries. A draft email should be useful chat content with explicit placeholders, not an invented recipient or a send action. No claim of adding concurrent background workers here. |

OpenMausBot is Apache-2.0; OpenMuse repository code is MIT with separate dependencies. The newly cloned `reference/openmuse` is pinned to the commit above, not installed or executed.

## Implemented behavior

1. A follow-up arriving during text streaming does not cancel the stream or trigger a replacement answer. Once the original answer is saved, the queued request runs with that complete answer in history.
2. During tool work, pending owner input is incorporated at the next model-call boundary. Multiple pending messages can share that next call. Their exact text is reconstructed as user-role messages from the journal, including after the transient tool context is gone. It is no longer truncated to 1,500 characters and disguised as a tool result.
3. Completed replies carry an ordering anchor to the following owner message. Live SSE insertion, updates, late delivery and journal reload agree on order. A waiting input gets a visible received/waiting note, and the current draft remains above that waiting input.
4. Tool metadata and unchanged prior response updates cannot erase an active draft. Legacy interrupted records remain readable without migration; the obsolete interruption/restart helper is removed.
5. Prompt guidance preserves already-completed content. Draft-only email requests produce an editable template when details are missing, then a short clarification. The existing review boundary still applies to actually sending anything.

The current tradeoff is deliberate: ordinary follow-ups can wait for the current model response. Instant native steering would require an acknowledged runtime protocol; cancelling a stream is not an equivalent implementation. Explicit Stop remains the way to interrupt immediately. This does not implement concurrent answers from the same bot or claim that all provider failure/retry paths are redesigned.

## Validation

Validation uses Windows, Go 1.27.1, Node 25.9.0 and the existing local Bun/Core installation. Commands and source-file hashes are retained in `.audit/2026-11-01-conversation-flow/validation.jsonl` and `candidate-manifest.json`. Isolated real-provider checks use the already-authorized ChatGPT connection, synthetic prompts and an access-only temporary lease; production chat and provider selections are preserved.

| Check | Command / evidence | Result |
| --- | --- | --- |
| Baseline reproduction | `go test ./internal/aurora -run TestFollowupDoesNotCancelOrRewriteVisibleAnswer -count=1` on base implementation plus new regression | Expected FAIL; active stream cancelled and original joke not retained complete. |
| Focused regression | `go test ./internal/aurora -run 'Test(Followup|Burst|RepeatedInterjections|Steer|ResendSameNonce)' -count=1` | PASS: additive follow-up, five-message burst, tool boundary, reconstructed history, legacy records, duplicate nonce and explicit Stop. |
| Affected Go packages | `go test -p 1 -json ./internal/aurora ./internal/tools ./internal/ledger ./internal/web -count=1` | 1,449 passed, 11 skipped, zero failed across completed package runs. The initial shell terminated with exit 1073807364 during ledger; aurora/tools had passed. Resumed ledger/web using `go test -p 1 -json ./internal/ledger ./internal/web -count=1`, without repeating completed packages. |
| Static checks and build | `go vet ./internal/aurora ./internal/tools ./internal/ledger ./internal/web`; `go build -o <audit>/neko-aurora.exe ./cmd/neko-aurora` | PASS. Candidate binary SHA-256: `2db005a23bda909e2a3ee4beed79957b20e7c0f732c4a4ace2ad151c1ed8c964`. |
| Frontend | `node --test app/tests/aurora/interrupted-messages-test.mjs app/tests/aurora/ux-test.mjs`; `node --check` on app.js, ux.js, i18n.js and sw.js | PASS: four message-lifecycle tests and 19 existing UX assertions; syntax checks pass. |
| Documentation | `python tools/check_docs.py` | PASS: 15 indexed documents / 168 local links. This is not an external-link or provider test. |
| Real ChatGPT | Exact owner prompts, isolated state, `core-chatgpt:gpt-6.1-sol`, native Core transport; `live.json`, `live-check.json` | PASS: follow-up accepted as queued before first response ended; 22 monotonic first-response deltas; stored original response equals final stream; two separate complete answers; no interrupted flag or send approval. |
| Browser reload | CUA at `127.0.0.1:63631`; `browser-check.json`, `joke-and-email.png`, `after-reload.png` | PASS: four message items, identical text and order before/after reload, zero interrupted notes. |

The real first answer retained the milk/six joke through its punchline. The next answer contained a separate email template with `[Người nhận]`, `[Chủ đề]` and a concise question, without retelling the joke. These observations verify this regression with one real model connection, not universal provider quality.

The live harness initially waited for `idle`, although a successfully completed bot is `done`. After the capture finished, `check_live.py` verified the same saved SSE trace and messages against the correct terminal states. No model request was replayed to obtain a pass. The provider/authentication path was unchanged by the product patch.

The local activation receipt records the final Git commit and preserves the existing original state directory. Core and Bun paths are passed explicitly when launching the local preview so its existing ChatGPT connection is available after the tool-session restart. Reference servers and production preview are separate from the isolated test state. No cloud deployment or GitHub push is part of this change.
