# Selected reply Markdown copy — 2026-10-29

Code commit: `b2868e5b4d9bab2c21a6844a2f3b92ee66aa895e` on `codex/grokbot-adoption`. This continues the local adaptation of [Grok Bot 0.78.1](https://x.ai/changelog/bot#v0.78.1), Fixed-1, after [formatted whole-reply copying](grokbot-reply-copy-2026-10-29.md). One agent performed implementation and review under the user's no-subagent instruction and pstack Feature / Prove It Works / Show Me Your Work. No credentials, provider calls, grants, OS clipboard writes, push or deployment were used.

## Behavior

Native Copy on a selection contained in one active, completed bot reply supplies Markdown as text/plain. It keeps selected heading/inline formatting, links, nested and task lists, the ordinal of a selected numbered item, quotes, fenced and inline code, and complete tables. Partial table selections contain only selected cells as text; they do not invent headers or include unselected cell contents. Controls and executable elements are excluded. Literal punctuation and HTML-looking text remain literal. Code backticks, indentation and blank lines are preserved. Service-worker cache: 83.

No new toolbar or clipboard permission prompt is required. Composer, owner-message, cross-message, header, empty, editing and unavailable/stale reply selections retain normal browser copying. The handler is scoped by connected reply DOM, current chat and workspace. It performs no network requests, mutations, draft edits or clipboard reads. Typing-animation content is not intercepted until stable.

A related renderer defect was corrected: matching multi-backtick code delimiters now preserve interior backticks. Heading nodes record their original Markdown level while retaining the existing visual heading size. The existing Markdown regression script now accepts AURORA_TEST_CHROME and has a Windows Chrome default.

## Evidence and limits

`selection-copy-125948.log` is the RED baseline: the selected bold fragment produced no Markdown payload. `selection-copy-130101.log` exposed the old renderer's multi-backtick failure. `selection-copy-130151.log` exposed accidental blank-line cleanup inside code; serialization now joins block boundaries without globally rewriting code text. Later escaping failures and test-only selector/class/loader corrections are retained in decisions.tsv and validation.jsonl rather than removed.

The committed browser fixture has 11 scenario groups. It dispatches real DOM Range and ClipboardEvent objects, uses a real mouse double-click and trusted Ctrl+C, and intercepts trusted fixture events before any OS clipboard write. API responses are synthetic loopback data. Mobile and English desktop screenshots were inspected. Clipboard payload validation is not paste acceptance in Slack, Gmail, Docs or Notion; no physical phone or Tauri WebView clipboard acceptance is claimed. Tables use the app's Markdown extension; this is not a full CommonMark conformance claim.

The implementation follows the browser [copy-event contract](https://developer.mozilla.org/en-US/docs/Web/API/Element/copy_event) for supplying text and canceling the default action, and the [CommonMark specification](https://spec.commonmark.org/0.31.2/) for delimiter/escape behavior. No reference-project code or assets were copied.

## Validation

Final committed source manifest: `299464983b8d290b78ed0ab094eb9066d6257fa85159418032c17bd84bcc73b2`, 1833 source files checked against Git with existing LF normalization accounted for. Windows amd64, PowerShell 7.6, Python 3.13.7, Chrome 156.0.8078.49, Node 25.9.0 and Go 1.27.1. Test subprocesses use an allowlisted environment excluding live credentials.

152 browser scenario groups, 12 existing/new Markdown assertions in Chrome, and 8 Node composer/outbox assertions pass. Syntax, embedded-asset validation and native build pass. After the browser gates, only md-test.mjs changed; validation-source-equivalence.json proves that all production sources and the other test files match the final state. The changed Markdown test was run again. Full Go evidence remains 2023 PASS / 38 SKIP at `1788bc2`; subsequent changes are confined to browser assets and fixtures, verified across the manifests. Full Go/Core/Rust suites were not rerun for this browser-only unit.

| Gate | Result | Seconds | Log under .audit/2026-10-29-selection-copy |
|---|---|---:|---|
| selection-copy | PASS | 5.92 | selection-copy-130452.log |
| selection-browser | PASS | 12.01 | selection-browser-130458.log |
| copy-browser | PASS | 6.15 | copy-browser-130510.log |
| identity-browser | PASS | 8.31 | identity-browser-130517.log |
| composer | PASS | 37.47 | composer-130525.log |
| table-viewer | PASS | 8.51 | table-viewer-130604.log |
| table-scroll | PASS | 8.27 | table-scroll-130614.log |
| content-browser | PASS | 7.8 | content-browser-130622.log |
| chat-loading | PASS | 11.21 | chat-loading-130630.log |
| read-state | PASS | 22.78 | read-state-130641.log |
| list-focus | PASS | 7.45 | list-focus-130704.log |
| browser | PASS | 25.88 | browser-130712.log |
| startup-browser | PASS | 24.24 | startup-browser-130738.log |
| composer-unit | PASS | 1.28 | composer-unit-130603.log |
| md-unit | PASS | 3.02 | md-unit-130857.log |
| syntax | PASS | 2.68 | syntax-130803.log |
| web-assets | PASS | 26.01 | web-assets-130805.log |
| build | PASS | 9.51 | build-130832.log |

Exact commands, execution times and source snapshots are in validation.jsonl. The Markdown test's local runner resolves playwright-core to the already-installed Python Playwright Node driver with a temporary loader; its log records the exact Node command, browser path and module resolution. No package was installed. Recognizable secret-pattern scans before the code checkpoint covered 5 candidate files and 606 newly reachable history blobs, with zero matches; this is not exhaustive secret detection. The unrelated pre-existing adoption-document edit remains outside the checkpoint.

## Backlog status

The [incremental adoption index](grokbot-adoption-index-2026-10-29.json) retains Fixed-1 as partial local validation: both copy paths now have local evidence, while target-application paste and native-platform acceptance remain open. Stored source-bullet totals remain 26 partial, one excluded and 893 unvalidated across 920 bullets; they are not independent feature counts. The broader Grok Bot backlog is not complete. [Reload and chat-identity evidence](grokbot-chat-identity-reload-2026-10-29.md) remains separate, including the boundary between the demonstrated fixture transport defect and the older untraced reload failure.
