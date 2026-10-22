# Independent mobile/accessibility audit

Run from the repository root, with an installed Chrome and Node supporting built-in WebSocket (validated with Node 25.9.0). No npm dependencies or software installation are required.

```powershell
node tests/mobile-audit-20261011/audit.mjs --source <repo-or-UI-worktree> --output <owned-evidence-directory> --commit <exact-UI-code-commit> --committed true
```

The runner freezes `index.html`, `style.css`, `app.js`, and `connections.js` before serving them and compares each file with the declared Git commit. `--committed true` serves immutable Git object bytes, so unrelated changes in the team's checkout cannot alter the examined snapshot. Omit it to examine working files; only Windows CRLF/LF is normalized when matching commit text, with raw hashes retained. JSON records exact command, source hashes, harness hash frozen at startup, browser version, environment and each pass/fail/blocked. Exit 0 means its checks pass, 1 means observed defects, and 2 means execution/source access is blocked. A harness interaction error is blocked, never counted as a product defect. No existing UI, runtime, test/config or Git index is modified.

Chrome runs headless with a new temporary profile, an owned dynamic debugger port and an in-memory loopback API fixture. It does not attach to the team's browser or native window, alter network settings, launch Core/Rust/Go, use provider accounts, import plugins, create grants or execute external tools. Every fixture response and submission is local. The outbox tests are UI-contract tests; mock acceptance is not evidence of backend durability.

Coverage: 320/360/390/414/700/701/768/1000/1440 px widths, landscape 844×390 including an explicit coarse pointer/5 touch points, a reduced-height layout viewport, long unbroken chat/artifact content, mobile AX toolbar names, 44×44 product targets, navigation/work-panel focus scope, Tab/Escape/return focus, native dialog labels/tab arrows/dismissal/password clearing, form reflow/text sizing, compact completion announcements, computed reduced motion, synthetic composition/Shift+Enter guard, streaming reading/focus/selection preservation, accessible connection state, duplicate/gapped SSE handling, same-nonce retry, outbox reconnect, lost ACK reconciliation, stale ACK/bot-switch race, initial snapshot failure and denied storage.

44×44 is the Neko product goal. A measured failure against that goal does not itself establish a WCAG 2.2 AA violation: SC 2.5.8 has a 24×24 minimum with spacing/equivalent/inline/user-agent/essential exceptions. See the review for manual criteria and primary sources.

For actual local-journal behavior, build the control binary from the stated code commit into an owned directory and run:

```powershell
python tests/mobile-audit-20261011/journal-check.py --binary <owned-neko-aurora.exe> --commit <exact-build-source-commit> --output <owned-results.json>
```

This runner uses two independent local HTTP cookie jars, an ephemeral IPv4 listener and its own temporary journal. It tests concurrent nonce deduplication, payload conflicts, independent requests/bots, snapshot equality, replay cursor, scoped cancellation, hard-kill/restart and ahead-cursor rejection. It records binary SHA-256 and does not inherit provider credentials or configure a host. The commit argument identifies the source supplied by the builder; the accompanying build-validation evidence must establish that mapping. It does not prove authenticated cloud sync or tool-effect exactly-once delivery.

Actual phone safe areas, keyboard/IME, VoiceOver/TalkBack, OS back behavior, suspension and resume, real network handover, authenticated separate devices, cloud sync and native mobile packaging remain manual or separately authorized gates. Desktop viewport emulation is never phone acceptance.

## Independent context supplement

```powershell
node tests/mobile-audit-20261011/context-audit.mjs --source <UI-worktree> --output <owned-evidence-directory> --commit <exact-UI-code-commit> --committed true
```

Keep `context-cases.mjs` beside the runner. Both hashes are recorded at startup. This separate suite reuses the owned headless/CDP setup and freezes Git object bytes, while its context scenarios live in the independently authored cases file. All context routes are the suite's own in-memory loopback fixture: no actual context API/grants, host/provider or credentials are used.

Coverage: capability absent/default metadata, context toggle AX semantics/44 px sizing/short-height scrolling, coarse-pointer landscape, keyboard Tab/Escape/return, PID/session stop-proof guard matrix, pending duplication/no optimistic grant, identical uncertain-retry nonce/body, retry visible/AX label consistency, conflict snapshot reconciliation, bot/draft/focus preservation after late ACK, equal-counter and mixed-counter newer-SSE cases, explicit refresh payload/guard, focus after refresh hides, uncertain refresh without auto replay and capability removal. Mixed-counter cases deliberately challenge full-work ACK monotonicity; they do not prove the backend generates those responses. See `docs/reviews/mobile-accessibility-context-20261011.md` for exact code/results and remaining physical/native/backend gates.

The follow-up cases also check `runtimeAttemptBinding` with no Core object, PID/session mismatch, missing/non-string identity, current binding precedence over old Core data and rejection of the never-launched fallback when a binding exists. Focus fences are checked after the user moves to another dialog control or closes/reopens the work panel. The exact d138 cases are retained at `evidence/context-d138-final/context-cases.mjs`; their historical hash remains in that result JSON.
