# How Aurora applies pstack

Aurora uses pstack's working principles in bot conversations, not only in the
development process. Substantive tasks should lead to an observable result:
understand the relevant context, choose a focused change, check the real output,
and deliver it with honest evidence. Simple conversation should stay simple.

## Runtime behavior

The existing skill catalogue now includes four bundled Aurora playbooks:

| Skill ID | When to use it |
| --- | --- |
| `pstack-build` | Create or improve an artifact, verify it, and hand it over |
| `pstack-debug` | Reproduce a defect, test its cause, fix it, and repeat the check |
| `pstack-research` | Investigate a question using primary sources and explicit uncertainty |
| `pstack-review` | Inspect the actual result, evidence and important failure paths |

When at least one of these skills is enabled, the bot receives a short working
agreement and an index of available skills. The model selects a matching playbook
through the existing `skill_load` tool. This selection is model behavior, not a
deterministic guarantee that every request will use the right skill. Greetings,
brief explanations, simple drafts and small follow-ups should not load one.

The runtime records a skill load as a normal tool step. Successfully loaded
bundled playbooks stay intact in that run's context, outside the budget that trims
old tool output. They are loaded once per run. Only the exact bundled bodies are
eligible for this policy slot. Custom skills and fetched content remain tool
results; a name, prefix, or claimed role cannot make them bundled policy. No state
or guidance is shared with another bot's run.

Existing bot skill selections and the skills-off switch still apply. An explicit
selection limits both the advertised catalogue and what `skill_load` accepts.
The default empty selection continues to mean all available skills. User-facing
names and descriptions are localized in Vietnamese, English, Simplified Chinese
and Traditional Chinese. Owners manage them in the existing AI/tools settings.

### Knowing when to hand over

Tool receipts now continue the current request in the model conversation, after
the request that caused them. Previously they were placed in system context while
the same owner request was sent last on every call. With a stateless completion
provider this could restart an already successful check until the round budget
forced a final answer.

Aurora's text directive protocol transports these observations as a labeled,
JSON-escaped data envelope in a user-role message. A separate system rule marks
that role as a transport convention, never a new owner request or approval. Tool
content cannot become system policy. The same ordering applies to plan steps and
the legacy Core prompt. Bundled skill loads also produce a completion receipt.

The model should match the requested outcome to the evidence, preserve failures
and missing coverage, and deliver once the relevant checks are sufficient. An
edit, failure, coverage gap or new owner request can justify another check. There
is no command cache that silently skips revalidation, and exit code zero alone
does not mark the whole task complete. This fixes the lost continuation boundary;
it is not a guarantee of perfect judgment for every model and task.

## Principles and boundaries

- **Prove It Works:** inspect the actual artifact and distinguish syntax checks,
  fixtures, browser checks, real-provider calls and physical-device acceptance.
  A successful command alone does not establish every outcome.
- **Attack the Premise:** repeated failures should trigger a test of the shared
  assumption and a changed approach. Existing bounded failure fingerprints in
  `pivot.go` remain in place.
- **Model the Domain:** use the existing skill catalogue and run-local context
  rather than a second agent engine or a new settings system.
- **Encode Lessons in Structure:** loaded bundled instructions cannot disappear
  merely because a later tool returns too much text. Disabled/unselected skills
  cannot be loaded by guessing their names.
- **Show Me Your Work:** substantial work should keep concise decisions and
  evidence, including failed attempts and limitations. This is a task record,
  not private reasoning. Existing tool receipts remain the execution record.

Playbooks cannot grant access, enable tools, widen network/workspace boundaries,
read secrets, or approve external actions. They do not install software or invoke
Cursor tools. They do not automatically create teammates or start multi-model
fan-out. Existing teammate handoffs remain available when the user requests
collaboration and the configured permissions allow it. Self-review is not labeled
independent review. Push, deployment and publication remain separately authorized.

## Source and scope

Studied [pstack](https://github.com/cursor/plugins/tree/ae5650febf4320793844c12c4adaa937f417f8da/pstack)
by Lauren Tan, MIT, at commit
`ae5650febf4320793844c12c4adaa937f417f8da`. Relevant leaves include
`poteto-mode`, `how`, `architect`, `show-me-your-work`, `principle-prove-it-works`,
`principle-attack-the-premise`, `principle-model-the-domain`, and
`principle-encode-lessons-in-structure`.

The bundled guides are Aurora adaptations, written for Aurora's actual tools and
permissions. This is not the full Cursor plugin and does not claim parity with
every pstack playbook or Grok Bot's private implementation. See
[third-party notices](../THIRD_PARTY_NOTICES.md). The earlier
[adoption study](pstack-adoption-2026-10-23.md) is historical, not the current status.

## Verification

`workstyle_test.go` checks real tool dispatch, durable load receipts, selection and
disable boundaries, failed/peer-denied loads, deduplication, bounded bundled
content, and preservation of loaded guidance under large tool results. It also
checks that custom skill content does not gain bundled authority or leak across
runs. These fixtures establish plumbing and boundaries, not model quality.

`tool_continuation_test.go` checks observation ordering, fresh-request isolation,
failure/edit evidence, JSON envelope escaping, and handoff through both ordinary
turns and plan steps. The repetition investigation and real ChatGPT acceptance
records are in `.audit/2026-11-01-completion-evidence/`.

Local acceptance records live under `.audit/2026-11-01-pstack-runtime/`, including
the exact base commit, source hashes, commands, environment, results and any live
provider evaluation. These records are local and are not a release declaration.
