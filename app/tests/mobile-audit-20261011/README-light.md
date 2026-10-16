# Light UI independent audit

Use the existing installed Chrome and Node with built-in WebSocket; no package installation is required. Keep `context-cases.mjs` and `light-cases.mjs` beside the runner.

```powershell
node tests/mobile-audit-20261011/light-audit.mjs --source <UI-worktree> --output <owned-evidence-directory> --commit <exact-commit> --committed true --suite regression
node tests/mobile-audit-20261011/light-audit.mjs --source <UI-worktree> --output <different-owned-evidence-directory> --commit <exact-commit> --committed true --suite context
node tests/mobile-audit-20261011/light-audit.mjs --source <UI-worktree> --output <different-owned-evidence-directory> --commit <exact-commit> --committed true --suite features
```

The runner serves immutable UI, font and notice Git objects using a new headless Chrome profile and owned loopback ports. It never attaches to a visible/native/team window. In-memory fixture routes include messages, SSE, context and connection inventory; no actual context API/grants, runtime/Core, provider or credentials are used. It records exact command, environment, source/harness/cases hashes and per-check pass/fail/blocked. Exit 0 passes, 1 indicates failed assertions, 2 indicates execution blockers.

`regression` reuses the 28-gate baseline suite; `context` reuses the 28-gate context/binding/focus suite. `features` adds exact fixture fonts/CDP custom-font evidence, empty-result drawer focus, search preservation/target size, lane keyboard/geometry, mention selection/escape/composition/repeat/reverse-tab/scroll visibility/short viewport, pending status AX exposure, mobile reply actions and reduced motion. Every mode has overlapping startup/source checks; do not add counts as unique scenarios.

Fixture fonts have `font-src 'self'` and `font/woff2` headers explicitly supplied by Node. Their success does not establish actual Go routing/CSP or Tauri typography. Physical phone, real IME/keyboard/safe areas, AT speech, OS back/lifecycle, native stop proof, cloud/multi-host sync and user visual approval remain separate gates. Historical f6 report and evidence are preserved; details are in `docs/reviews/mobile-accessibility-light-20261011.md`.
