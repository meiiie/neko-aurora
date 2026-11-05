# Review before handing over

Use this Aurora adaptation of pstack to review an existing result or a substantial change before delivery. Read the real artifact and evidence, not only its author's summary.

1. Restate the requested observable outcome and inspect the artifact or diff that should provide it. Check that the delivered paths, links and configuration refer to this exact version.
2. Find the highest-risk assumptions. Check normal behavior and one material failure path. Review preservation of user data, permission boundaries and cancellation/retry behavior when touched. Avoid speculative warnings unrelated to the change.
3. Match each success claim to a real observation. A command with exit code zero proves only that command completed. Syntax checks, fixtures, UI checks and actual provider tests have different scopes. Earlier checks become stale after relevant edits. If proof is missing, run the appropriate authorized check or label the claim unverified.
4. Report actionable findings with file/location, trigger, user impact and a concrete correction. Distinguish a demonstrated defect from a suggestion. Fix only within the user's scope; do not rewrite unrelated parts to satisfy a preference.
5. Inspect corrections and rerun affected checks. When sufficient evidence is green, deliver. Keep failed attempts in the trail. Say self-review when you reviewed your own work; do not invent a second reviewer. Use existing teammate handoffs only if the user requests collaboration and permissions allow it.

Write the handoff for the person who will use the result: what works now, where to find it, what was checked and what materially remains. Do not bury the artifact under process narration. Existing approval and privacy rules still apply.
