# Dictation preserves the current composer

Code commit: `bf165921460f9a044363b0c6866998dfad71971d`, based on `bcfdcdb346a7a662bf039b3f88dfc3d801206463`. Solo pstack Feature / Prove It Works / Show Me Your Work and webapp-testing. Source entry: [Grok Bot 0.77.0](https://x.ai/changelog/bot#v0.77.0), Improved 12.

The existing Web Speech API path already rendered interim results in the composer. Its callback retained the starting draft, so later recognition could overwrite manual edits or a completed attachment upload. Recognition now owns a specific workspace, chat, session and last written text. Manual input ends the session. A programmatic text change is preserved when the next result detects that ownership no longer matches. Stop revokes ownership before calling the recognizer, so even a synchronous final/ended callback cannot alter the message being submitted. Old result/end/error callbacks cannot affect a new recording.

Visible interim/final text still updates the existing draft store without automatic sending. Existing whitespace is retained. The microphone exposes aria-pressed and a start/stop label; recognizer language follows Vietnamese or English UI preference (vi-VN/en-US). Constructor, start and recognition errors clear the active state and keep text, allowing an explicit retry. Unsupported browsers retain text input. The service-worker cache is v66.

## Validation

Windows amd64; Python 3.13.7/Playwright, Chrome 156.0.8078.49, Go 1.27.1 and Node 25.9.0. Real Chrome DOM and local API fixtures, with injected SpeechRecognition events. No microphone permission, audio capture, remote speech service, provider account, grant, paid request or deployment. Screenshots at 1280×900 and 390×844 were visually inspected.

The RED run `dictation-233559` first passes existing interim/final behavior, then reproduces a late callback replacing the owner's manually edited text. Initial repair passes all 11 groups in `233629`. The final run adds Vietnamese-language, constructor-failure and keyboard-retry assertions within those groups. It covers final/interim replacement, manual edit, uploaded attachment, synchronous stop on submit, session restart, errors/retry, chat/workspace changes, whitespace/draft reload, English labels, viewport layouts and unavailable recognition.

**117 browser scenario groups pass**: 11 dictation + 23 composer + 21 general adoption + 12 chat loading + 9 read-state + 12 list focus + 10 selected excerpts + 11 table viewer + 8 table scrolling. Eight Node draft/outbox tests, JS syntax, embedded assets and Go build also pass on the same source manifest.

| Command | Result / log |
|---|---|
| `python -u app/tests/aurora/dictation-browser.py` | PASS `dictation-233752.log` |
| `python -u app/tests/aurora/composer-browser.py` | PASS `composer-233758.log` |
| `node --test app/tests/aurora/composer-drafts-test.mjs app/tests/aurora/outbox-test.mjs` | PASS `composer-unit-233832.log` |
| `python -u app/tests/aurora/adoption-browser.py` | PASS `browser-233833.log` |
| `python -u app/tests/aurora/chat-loading-browser.py` | PASS `chat-loading-233857.log` |
| `python -u app/tests/aurora/read-state-browser.py` | PASS `read-state-233907.log` |
| `python -u app/tests/aurora/list-focus-browser.py` | PASS `list-focus-233926.log` |
| `python -u app/tests/aurora/selection-browser.py` | PASS `selection-browser-233930.log` |
| `python -u app/tests/aurora/table-viewer-browser.py` | PASS `table-viewer-233942.log` |
| `python -u app/tests/aurora/table-scroll-browser.py` | PASS `table-scroll-233948.log` |
| `python .audit/2026-10-28-grok/syntax.py` | PASS `syntax-233954.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -count=1 ./internal/web` | PASS `web-assets-233957.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe build -o E:\Sach\Sua\neko-aurora\.audit\2026-10-28-dictation\neko-aurora.exe ./cmd/neko-aurora` | PASS `build-234003.log` |

Exact cwd, environment, timestamps, base and exit status: `.audit/2026-10-28-dictation/validation.jsonl`. Source manifest SHA256: `7d23fc96902b5eff3bb0285983c8c2a6755d8e1d7143c1863e1be579fce9b835`. `code-checkpoint.json` verifies 1803 source files against `bf165921460f9a044363b0c6866998dfad71971d` with existing Git LF normalization accounted for. Redacted checkpoint scan: 4 candidate files and 426 newly reachable history blobs; zero recognizable secret matches, not an exhaustive secret-detection guarantee.

## Limits

These tests validate recognition event handling, not transcription accuracy, real microphone/browser permission behavior, mobile keyboards, screen readers, physical phones, native Tauri voice or realtime conversational voice. Recognition may be supplied by the browser's own service in normal use; this test does not exercise that service. A programmatic composer change stops recognition on the next callback, while manual input stops it immediately. English/Vietnamese control labels do not imply complete application localization. Core/Rust and the full Go suite were not rerun for this frontend-only change. The source entry is recorded as partially validated; no complete Grok feature parity is claimed.
