# Development workflow

Deliver one useful, working Neko Bot outcome at a time. Prefer less code and a small amount of decisive evidence over more agents, abstractions, or tests.

## Own the outcome

- One integrator owns scope, cross-component decisions, the final diff and the user-facing result. Delegate independent slices with explicit writable paths, acceptance criteria and the relevant check. Keep one writer per slice; resolve overlapping work before editing.
- Start with the observable outcome: for example, a user sends an intent, sees its actual status, and can cancel safely. Identify the existing path before adding another one.
- Reuse Neko Core for the semantic agent loop. Go owns control-plane/journal orchestration; Rust owns trusted admission; the UI presents intent and state. Do not rebuild the engine in Go or the UI. Keep Wiii separate and verify its base before integration. See [architecture](architecture.md).
- Prefer the smallest coherent fix, existing adapters and authoritative types. Remove redundant paths when their callers migrate. Do not expand the work into unrelated cleanup, a generic framework, or extra process documents.
- The integrator reviews the combined result as it becomes ready. A worker report is evidence to inspect, not automatic acceptance. Add independent review for consequential boundary changes, not a second worker solely to repeat a cheap command.

## Verify what could break

Choose checks from the changed behavior and its blast radius. Preserve the applicable requirements in [CONTRIBUTING](../CONTRIBUTING.md) and [Core testing](../core/docs/process/TESTING.md); this workflow avoids redundant work, not meaningful safety coverage.

- Bug fix: reproduce the visible failure. Add a failing-before/passing-after regression when there is a cheap existing test path. Otherwise use the closest useful executable or manual reproduction and state its limits. Do not build a large mock harness just to claim TDD.
- Assert observable output, persisted state or external effects, including failure/cancel/retry where relevant. Avoid tests of private helper order, restated constants or prose. Never weaken a meaningful assertion to obtain a pass.
- Docs only: check links, commands and consistency, run `python tools/check_docs.py` and `git diff --check`. Keep new pages in the docs checker’s file list; check any unlisted page separately. Review changed presentation at desktop/mobile sizes when layout or assets are affected; do not run application suites for prose edits.
- Component code: run the affected subsystem checks and applicable component lint/typecheck/build from the contribution guide. UI syntax checks need the relevant interaction check when behavior changes.
- Shared agent loop, authority, persistence, transport or lifecycle: run the broader local gate required by the component contract. A release needs its release gates on the exact candidate. Small diff size alone does not make a boundary change low-risk.
- After a clean checkpoint, reuse its evidence while the tested inputs, dependencies and environment remain unchanged. Do not repeatedly run the full suite after an unrelated documentation edit. Relevant edits, integration conflicts, failures or unresolved risks invalidate the affected evidence; rerun that scope. Keep expensive checks sequential.

Stop testing when the required checks and the specific user outcome have sufficient evidence, not when a test-count target is reached. Do not silently drop an unresolved failure.

## Checkpoint and report

Use a short existing task/PR report, not a new reporting system:

1. Record the source commit and any uncommitted delta, commands, environment, results and the behavior actually observed. Earlier evidence remains tied to its original source; explain why it still applies rather than relabeling it as a fresh run.
2. Before each authorized checkpoint or push, scan candidate source files and newly reachable history for secrets. Exclude private state, account stores, chat, credentials, caches and binaries. Never print matched secret values. Preserve component licenses and provenance.
3. Push only with applicable authorization. After a push, read the actual remote branch SHA and compare it to the intended local commit; only then report publication. Local commits and stale remote-tracking refs do not prove remote delivery.
4. Distinguish PASS, FAIL, BLOCKED and NOT RUN. Explicitly name unavailable Windows/ConPTY/SRT, native-device or real-provider checks. Linux fixtures, compile success and local replay do not establish Windows acceptance, provider quality, cloud sync or production readiness. State the next required environment/action without implying a pass.

## pstack reference and adaptation

Reviewed upstream: [cursor/plugins pstack README](https://github.com/cursor/plugins/blob/2f8377461fd533f5af782e1fd643783b1067b847/pstack/README.md), pinned revision `2f8377461fd533f5af782e1fd643783b1067b847`.

This is a Neko-specific adaptation of pstack's small-diff, experience-first, observable-behavior and direct-verification principles. Its [TDD skill](https://github.com/cursor/plugins/blob/2f8377461fd533f5af782e1fd643783b1067b847/pstack/skills/tdd/SKILL.md) explicitly permits practical alternatives to costly regression harnesses. Its [orchestration playbook](https://github.com/cursor/plugins/blob/2f8377461fd533f5af782e1fd643783b1067b847/pstack/skills/poteto-mode/playbooks/orchestrate.md) scales ceremony down for cheap units and work that fits one session. We adopt those principles, not its Cursor-specific model routing, automatic publishing assumptions, orchestration database or plugin installation.

Upstream pstack is [MIT licensed, copyright 2026 Lauren Tan](https://github.com/cursor/plugins/blob/2f8377461fd533f5af782e1fd643783b1067b847/pstack/LICENSE). No upstream implementation or substantial documentation bundle is vendored here. This reference does not change Neko Bot's component licensing or grant permission to push, deploy, use credentials or spend provider quota.
