# Formatted reply copying — 2026-10-29

Code: `e3dfacde724fab906b1866340b154eb23f94d506` on `codex/grokbot-adoption`, after startup recovery `1788bc2a030eed47e912453d31796baa2ace3f82`. No provider, grant, deployment, push or subagent was used. The user's existing two-space edit in the main adoption document remains untouched and outside these checkpoints.

## Behavior

Completed normal agent replies now have Copy reply. One clipboard item carries exact original Markdown as text/plain and an HTML representation built from Aurora's existing markdown renderer. Semantic headings, lists, numbered starts, links, quotes, code and tables are retained. A small DOM allowlist strips UI controls, executable markup, styling and arbitrary attributes; links only retain HTTP, HTTPS or mailto destinations. Copying adds no message POST and leaves the composer draft unchanged.

Browsers without the rich clipboard API receive Markdown through writeText. Permission denial produces an error and does not issue a second write. A pending operation is single-flight, exposes aria-disabled/aria-busy, and retains keyboard focus. A completion after leaving the chat, switching workspace or removing the original button does not notify the new context. A clipboard write already requested by the user's click is not canceled by those transitions. Service-worker cache version: 81.

The implementation follows the documented [Clipboard.write representations](https://developer.mozilla.org/en-US/docs/Web/API/Clipboard/write). No third-party code or assets were copied.

## Evidence

The committed fixture is `app/tests/aurora/reply-copy-browser.py`; run `python -u app/tests/aurora/reply-copy-browser.py` from the repository. It uses the real local UI and real ClipboardItem/Blob construction, with a synthetic clipboard sink so it does not touch the owner's OS clipboard. API responses, denial and delayed completion are fixtures. Eight scenarios cover exact Unicode Markdown and semantic HTML, executable-markup/UI stripping, keyboard/mobile focus, text fallback, denied permission, single-flight and cross-chat completion, workspace completion, and English desktop behavior.

RED `copy-browser-122627.log`: the control was absent. RED `copy-browser-122715.log`: native disabled state dropped focus; using an explicit pending state with ARIA kept focus and still blocked repeat activation. `copy-browser-122757.log` exposed a test-only Playwright evaluate mistake when restoring a native constructor; a block-bodied callback fixes that fixture. Final copy check: `copy-browser-122832.log`.

Final source manifest `05b6e3123fe9450ceb6d5b915122d07158a43a4090aff16b1423a4db09ff347a`, 1829 files, verified against the code commit including existing Git LF normalization. Windows amd64, PowerShell 7.6, Python 3.13.7, Chrome 156.0.8078.49 headless, Node 25.9.0, Go 1.27.1. An allowlisted environment excludes live credentials. Screenshots `reply-copy-mobile.png` and `reply-copy-desktop-en.png` were inspected.

134 browser scenario groups and 8 Node assertions pass, plus syntax, Go embedded-asset validation and native build. `browser-only-delta.json` compares the entire source manifest with startup recovery's final full Go gate: only five browser assets and this browser fixture differ. Native Go, Core, Rust and tools remain byte-identical; the prior 2023 PASS / 38 SKIP Go result is retained as prior evidence, not a newly executed full-suite result. Full Rust/Core gates were not repeated.

| Check | Result | Seconds | Log in .audit/2026-10-29-reply-copy |
|---|---|---:|---|
| copy-browser | PASS | 5.27 | copy-browser-122832.log |
| composer | PASS | 37.89 | composer-122849.log |
| selection-browser | PASS | 14.45 | selection-browser-122927.log |
| table-viewer | PASS | 8.89 | table-viewer-122942.log |
| table-scroll | PASS | 8.55 | table-scroll-122951.log |
| content-browser | PASS | 8.2 | content-browser-122959.log |
| browser | PASS | 26.37 | browser-123008.log |
| chat-loading | PASS | 11.84 | chat-loading-123034.log |
| read-state | PASS | 19.64 | read-state-123046.log |
| startup-browser | PASS | 16.95 | startup-browser-123106.log |
| editor-browser | PASS | 15.3 | editor-browser-123123.log |
| composer-unit | PASS | 1.14 | composer-unit-123139.log |
| syntax | PASS | 2.16 | syntax-123140.log |
| web-assets | PASS | 3.82 | web-assets-123142.log |
| build | PASS | 8.96 | build-123146.log |

Exact commands, environment, timestamps and before/after manifests are recorded in that directory's validation.jsonl. All gates above use the same frozen source. Before the code checkpoint, recognizable secret patterns were checked in 6 candidate files and 582 newly reachable blobs: no matches; this is not exhaustive secret detection.

## Backlog scope

This is a partial local implementation of [Grok Bot 0.78.1](https://x.ai/changelog/bot#v0.78.1), Fixed-1. The [incremental index](grokbot-adoption-index-2026-10-29.json) records that narrow scope. Selection-to-Markdown, native OS clipboard behavior and paste fidelity inside Slack, Gmail, Docs or Notion are not validated by this fixture. Existing selection-to-composer behavior is retained and regression-tested.

Across the two stored inventories, 25 of 920 source bullets have narrower local evidence, one is excluded, and 894 remain unvalidated. Bullets can bundle multiple behaviors; none of these counts establishes complete upstream parity. See [reload investigation](grokbot-composer-reload-2026-10-29.md) for the still-unproven exact cause of the old untraced composer failure and the deterministic recovery cases now covered.

Workflow: pstack Feature / Prove It Works / Show Me Your Work, direct self-review under the user's no-subagent rule. The decision trail and rejected evidence remain local in the audit directory.
