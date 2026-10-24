# pstack adoption study (2026-10-23)

Source: https://github.com/cursor/plugins/tree/main/pstack, commit 8b3326d (2026-10-23 10:15 UTC+7). Local copy: /workspace/pstack-ref/pstack.
Status: research only. Nothing implemented or deployed.

## 1. What pstack is

pstack is a Cursor/Claude Code plugin by Lauren Tan (poteto). It has 57 skills, 23 playbooks and 2 agents (poteto-agent, comment-sicko). It is **prose for a coding agent, not a runtime**: Markdown SKILL.md files, plus a few Bun/Node helper scripts (orch, watch-pr, check-plan, statechart/TLA+).

How it is structured:
- **Router:** `poteto-mode/SKILL.md`. It lists non-negotiable triggers (e.g. "nontrivial change → how", "about to ask a which-approach question → prototype it instead", "long or unattended work → show-me-your-work"), then a short index of principles, then playbooks. The agent opens a todo list whose first items are the chosen playbook's steps, copied verbatim; a skipped step stays in the list marked `skip: <reason>`.
- **Principles (22 × `principle-*`):** each is a short leaf with "Apply when…", Why, and Pattern. The router tells the agent to read a leaf in full before applying it and to name the principles it applied in the reply.
- **Workflow skills:** architect (ground → sketch → agree → implement → scrap), arena (N candidates, cross-judge, graft), swarm (frame → fan out → aggregate → report, with PASS/ISSUES/BLOCKED), interrogate (reviewers on different models, then the lead sorts findings into act on / consider / noted / dismissed, never auto-applied), tdd (failing test first, but only when a cheap test exists), show-me-your-work (a TSV decision log: ts, phase, decision, why, evidence, result), reflect, unslop, how/why.
- **Triggering:** 56 of 57 skills set `disable-model-invocation: true`. They run only when the user types a slash command or when poteto-mode's router sends the agent to them. So skills load **on demand** and the context stays small.
- **Subagents:** fresh agents by default, each with the consolidated scope. The parent owns the result and reviews the real diff instead of trusting an agent's summary. Models are assigned per role (a fast code model for code, a strong model for judgment), and a second opinion means the same prompt on a different model.

## 2. License and attribution

MIT, © 2026 Lauren Tan (pstack/LICENSE). We may copy, adapt and use it commercially. If we copy or substantially adapt text (prompts translated into Vietnamese count as adaptation), we must keep the copyright notice and the MIT text: add a pstack entry to THIRD_PARTY_NOTICES next to neko-core. If we only take ideas and rewrite them in our own words, no notice is legally required, but we credit it anyway. MIT is compatible with Aurora; it does not carry neko-core's AGPL concerns.

## 3. What Aurora already has (feat/multiuser-railway, e8c19bd)

`app/native/internal/aurora/`:
- turn.go `runTurn`:
  - `PLAN` directive, or user-numbered steps, run as a server-driven plan with step status and milestone notifications;
  - `planNudge` for multi-part requests;
  - `promiseOnly` → `actNudge`;
  - narration strip (`directivesOnly`);
  - `cutNudge`;
  - `continueNudge` for open checklist items;
  - `steer` (the user changes course mid-answer);
  - `verifyFinal`, which checks claimed files against the real file list.
- budget.go: `checkpoint` (4 min budget + 4 min slack, up to `maxResumes`=6 hidden continuation turns), `unfinished`, `pyPrecheck`, chunked writes and edits.
- validate.go: `validatePPTX` (hidden shapes, contrast, notes length, diacritics, overlap) and the request checklist (`mergeChecklist`/`openItems`).
- review.go: a second model pass over research facts before a user-facing file is written.
- harness.go: `compactHistory`, `budgetBlocks`/`truncateResult`, memory.

Aurora already covers about 60% of pstack's spirit with **deterministic Go gates**, and that is the right way to do it for GLM. What's missing: an explicit per-task-type playbook, a decision trail the user can see, failure-pattern memory inside a request (attack the premise), independent review, and on-demand skill text.

## 4. Mapping pstack patterns to the Aurora runtime

| pstack pattern | Aurora mapping | Fit |
|---|---|---|
| Router + playbooks (poteto-mode) | A Go classifier (regex/keywords + the checklist, with no extra LLM call) picks a **task playbook**: research-answer, deck, video, script/data, email draft. Its steps become the `PLAN` and the checklist, instead of the model inventing them. | High |
| Plan before act (architect "Ground/Sketch", copy steps verbatim) | Already partly there (`PLAN`, `planNudge`). Add: for computer tasks, a short **artifact spec** first (files, slide count, length, palette) stored in pcState. Validators check against the spec, not a guess. | High |
| Prove it works | `verifyFinal` + `validatePPTX` already do this. Extend to every deliverable type: mp4 (ffprobe duration, audio stream, size > 0), xlsx/csv (opens, rows), py (exit 0 plus an output file), and "done" only once all checks pass. Keep the check output as an artifact (a `verify.txt` chip). | High (highest value) |
| Show-me-your-work (decision log) | A per-request `decisions` list in the ledger (phase, decision, why, evidence = file/exit code, result), written **by the server** from real events, not by the model. In the UI it's a folded "Cách mình làm" section under Các bước. | High, cheap |
| Attack the premise after 2 failed fixes | Track a failure signature (same command/validator fail, normalized stderr) per request. On the 2nd identical failure, inject a nudge: "write the premise down in one sentence, test it with one command, then change approach (e.g. a different font or library)". On the 3rd, stop and give an honest status plus a CHOICES card. This targets the run 9/10 video-font loop directly. | High |
| Fix root causes / laziness | Prompt lines in the computer note: smallest change, `computer_edit_file` instead of rewriting the file. Already nudged; keep it short. | Medium |
| Guard the context window | Already `compactHistory` and `budgetBlocks`. Add: the tool output the model sees is a tail plus an error excerpt; the full log goes to a file. Skill text loads only when needed (below). | High |
| Skill loading on demand | Store the playbook and guidance texts (pptx rules, Vietnamese/font rules, video pipeline) as embedded files and inject only the ones the classifier picked, instead of growing `systemPrompt` and the computer note. This lowers input tokens on every turn. | High |
| Never block on the human (prototype instead of asking) | For computer tasks, settle a fact by running it (e.g. "is the font installed": `fc-list`) instead of asking with CHOICES. Already partly in the step prompt ("Đừng tự tạo bước chờ"). | Medium |
| Make operations idempotent | Already done (message IDs, resume from checkpoint). Keep. | Done |
| Interrogate / arena (multi-model adversarial review) | Only a light version: one extra non-streamed GLM pass as a "checker" for the final deck/script (review.go already does this for facts). No multi-model fan-out. | Low/Medium |
| Swarm / subagent fan-out | Parallel **lookups** (up to 3) exist. Parallel computer workers, e.g. rendering slide images while narration is generated, could run as server jobs, not as LLM agents. | Low |
| TDD | Not for end users. Use it only in our own development of the harness (regression tests for validators already exist). | Dev-only |
| Babysit/shipping/orchestrate/worktree/statechart/TLA+ | Developer workflow for us, not for Aurora's bot. | No |
| Name principles in the reply, todo list shown verbatim | For a mobile user this is noise. Keep it internal (decision log); the reply stays short in Vietnamese. | No (in UI) |

## 5. What does NOT fit a mobile end-user chat bot, and why

1. **Multi-model fan-out (arena, interrogate, swarm, architect runners).** We have one model (GLM 5.3) and a small Railway budget. Every extra runner adds a full call (about 2–4 s to first token, plus thousands of input tokens). On a phone, people expect 10–60 s, not minutes. Model diversity, which is pstack's core signal, doesn't exist with one model.
2. **Subagent spawning per step.** Aurora's sandbox is 1 GB per bot. Parallel LLM agents would multiply sandbox and token cost and complicate continuous messaging (one runner per bot, with an inbox).
3. **Asking for nothing / full autonomy.** pstack pushes "don't ask the human". Aurora must still stop at the approval card for external actions, secrets and paid actions. That policy stays above any playbook.
4. **Verbose rituals.** Citing principles, copying playbooks verbatim into the visible reply, and long reports go against the short-Vietnamese-reply rule and Grok-iOS-style UX.
5. **Coding-repo playbooks** (PRs, CI, worktrees, Bugbot, comment-sicko, TLA+): Aurora's users are not editing a git repo.
6. **Trusting the model to self-route by reading long Markdown.** GLM follows long English meta-instructions less reliably than frontier models (seen in runs 3–9: narration, promise-only). Routing and gates must live in Go, with the model given only short, concrete Vietnamese instructions.

## 6. Prioritized implementation plan (not started)

Assumptions about GLM 5.3 cost: about 2–4 s TTFT per call; the system prompt + computer note is about 3–5k tokens per call today; a deck run makes about 20–40 calls.

| P | Change | Where | Latency impact | Token/cost impact |
|---|---|---|---|---|
| P0 | **Attack-the-premise breaker**: failure signature per request; 2nd repeat → premise nudge; 3rd → stop with an honest status and CHOICES | budget.go/turn.go, pcState | Saves minutes on stuck loops (run 9/10 video font) | Negative (fewer wasted rounds and resumes) |
| P0 | **Prove-it-works for all deliverables**: validators for mp4 (ffprobe), xlsx/csv, images, py output; the checklist item closes only when its validator passes; `verify.txt` artifact | validate.go | +1–3 s per final (local checks, no LLM) | ~0 |
| P1 | **Server-written decision log** (show-me-your-work) shown folded in the UI and kept across resumes | ledger + view.go/UI | 0 | 0 (not sent to the model; optionally a 5-row tail in the resume prompt, ~200 tok) |
| P1 | **Playbook router + on-demand guidance**: classify the task in Go and inject only the relevant playbook/guidance; trim the global prompt | turn.go, new playbooks/*.md (embed) | Slightly faster (smaller prompt) | −20–40% input tokens on non-computer chats |
| P2 | **Artifact spec before build** (plan-before-act): one short spec call for computer tasks; validators read it | build.go | +1 call (~3–6 s) once per task | +~1–2k tokens per task; offset by fewer redo loops |
| P2 | **Context hygiene for tool output**: the model sees an error excerpt plus a tail; full logs go to a file | harness.go | slight gain | −10–30% on build turns |
| P3 | **Single checker pass** (light interrogate) on final decks/scripts, only when validators pass but the task is large | review.go | +5–15 s, once | +3–8k tokens per deliverable; make it optional or plan-gated |
| — | Arena, swarm, multi-model, TDD for users | — | skip | — |

Gates for each step: unit tests in `aurora/*_test.go`; a replay of the run 9 transcript; one live run on the mercury-to-gold deck **after run 10**, staying within the credit floor (stop below $1.2).

## 7. Risks

- **Over-prompting GLM.** Adding pstack prose to the system prompt raises tokens and dilutes instructions. Mitigation: enforce in Go and inject only short text.
- **Latency creep.** Every extra verification or checker call adds seconds on mobile. Mitigation: deterministic local checks first; the LLM checker only when gated.
- **Cost on the Railway Hobby trial** (~$1.74 left) and z.ai spend if fan-out sneaks in. Mitigation: no multi-agent work; cap resumes.
- **False "done" or false "failed"** if validators are wrong. Mitigation: unit tests on real artifacts from runs 8–10.
- **Breaker too aggressive.** It could stop work that would have succeeded. Mitigation: normalized signatures, with the threshold at 2 identical failures, not 2 failures of any kind.
- **UX noise.** A decision log must stay folded; the reply stays short.
- **License.** Fine (MIT), but copied or translated text needs the notice in THIRD_PARTY_NOTICES.
- **Conflict with run 10 work.** Run 10 touches the same files (turn.go, budget.go, validate.go), so start only after run 10 lands, to avoid merge conflicts.
