# Composer reload and startup recovery — 2026-10-29

Code: `1788bc2a030eed47e912453d31796baa2ace3f82`. Base: `c120c7236463a685694b57aa42dcc31913c8e35a`. Branch: `codex/grokbot-adoption`. Work stayed in Neko Aurora; no agents, provider calls, grants, push or deployment.

## What changed

The composer browser fixture now waits for each language navigation before inspecting the new document. Failure diagnostics include JavaScript errors, recent response/failure URLs and whether the app initialized. The five-second assertion was not relaxed.

Startup now has a persistent loading/error panel and a keyboard-accessible Reload control. An early independent script handles a missing main script; a failed initial state read shows a durable error. Reload preserves the URL fragment and the existing device-local draft. Expired sessions stay in loading until authentication configuration resolves, then show sign-in. Vietnamese and English text are covered; service-worker cache version is 80.

## Evidence and limits

The original empty-shell failure remains at `.audit/2026-10-29-vault-editor/composer-031038.log`. Its diagnostics did not capture request or script errors, so its exact cause **has not been established**. Forty synchronized and 200 original-style language transitions on unchanged baseline source passed under tracing; these runs cannot close that historical cause. The initial diagnostic stress loop itself raced a second language change with navigation; its exception capture also encountered a null body. Those diagnostic mistakes are recorded, not counted as product evidence.

Deterministic RED: `startup-browser-120651.log` reproduces an unrecoverable shell after an initial-state HTTP 500. The committed regression also blocks app.js, holds state for 5.5 seconds, tests retry with an existing draft, holds the initial state response while a dependency fails, repeats 20 language reloads, exercises English desktop keyboard activation, and delays authentication configuration before showing sign-in. A second RED (startup-browser-121803.log) proved that a late state response could hide a fatal dependency failure. Startup now keeps a blocked initialization state and rejects that response before applying workspace data. No message POST occurs in those recovery scenarios. This proves the specified recovery behavior, not the root cause of the untraced historical flake.

The early startup script must itself load. No physical-phone, native installer, offline service-worker cache, real microphone or real provider acceptance is claimed. A previously issued request is not canceled by this change. Device-local draft persistence is not cloud sync.

## Validation

Final source manifest: `93918379082a88ca7b779937049b733a1161f35716be842b1ed47184590d333b` across 1828 files, checked against the code commit including Git LF normalization. Windows amd64, PowerShell 7.6, Python 3.13.7, Chrome 156.0.8078.49 headless, Node 25.9.0 and Go 1.27.1; allowlisted environment excludes live credentials. Real local UI, synthetic loopback APIs. Screenshots `startup-failed-mobile.png` and `startup-failed-desktop-en.png` were inspected.

127 browser scenario groups and 8 Node assertions pass. JavaScript syntax, Go embedded-asset tests and native binary build pass. The first embedded-asset gate failed because startup.js was not registered. The Go public-asset registry was corrected and the full Go suite rerun: 2023 PASS, 38 SKIP, 0 FAIL, plus go vet. Rust/Core execution was not repeated; their sources are unchanged. All reported gates use the final frozen source manifest, after correcting the registry and the fatal-initialization state race.

One composer run exited zero but changed source during execution (`composer-120754.log`); the harness marked it STALE. It was discarded and rerun on frozen source. Logs, exact commands, environment and before/after manifests live in `.audit/2026-10-29-composer-reload/validation.jsonl`.

| Gate | Result | Seconds | Log in audit directory |
|---|---|---:|---|
| startup-browser | PASS | 19.23 | startup-browser-121836.log |
| composer | PASS | 37.13 | composer-121855.log |
| chat-loading | PASS | 12.39 | chat-loading-121933.log |
| read-state | PASS | 19.29 | read-state-121945.log |
| schedule-test-browser | PASS | 16.75 | schedule-test-browser-122005.log |
| timezone-browser | PASS | 9.1 | timezone-browser-122021.log |
| content-browser | PASS | 7.59 | content-browser-122031.log |
| memory-browser | PASS | 20.52 | memory-browser-122039.log |
| editor-browser | PASS | 15.81 | editor-browser-122059.log |
| payment-browser | PASS | 10.27 | payment-browser-122115.log |
| composer-unit | PASS | 1.11 | composer-unit-122126.log |
| syntax | PASS | 2.92 | syntax-122127.log |
| web-assets | PASS | 5.21 | web-assets-122130.log |
| go-serial | PASS | 262.95 | go-serial-122135.log |
| vet | PASS | 2.53 | vet-122559.log |
| build | PASS | 3.64 | build-122601.log |

Before the code checkpoint, recognizable secret patterns were checked in 9 candidate files and 570 newly reachable history blobs; no matches. This is not exhaustive secret detection. The pre-existing two-space edit in `docs/grokbot-adoption-2026-10-28.md` was preserved outside the checkpoint.

## Updated reference inventory

The [official changelog](https://x.ai/changelog/bot) now includes 0.78.1 (2026-10-27, bundling 0.78.0). The [incremental index](grokbot-adoption-index-2026-10-29.json) records 100 additional bullets: 13 New, 27 Improved, 60 Fixed. It preserves source hashes without redistributing upstream prose. The prior 820-entry index remains a historical snapshot, with 24 narrower local implementations and one excluded item; this recovery work adds no upstream parity claim.

New work covers shopping/subscriptions, contact editing, atomic Page edits and chat/connector reliability. The next UI unit is formatted reply copying; integrations still require their own contracts and authorized acceptance. Completion of this local recovery unit is not completion of the entire Grok Bot backlog.

Workflow: pstack Feature, Prove It Works and Show Me Your Work, with direct self-review to honor the user's no-subagent instruction. The local decisions.tsv records investigation, RED, rejected STALE evidence and final gates.
