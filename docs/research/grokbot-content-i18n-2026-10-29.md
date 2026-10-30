# Literal user content in localized conversation UI

Code commit `b0f8acc90ab38e7f7c7705bf061956eb0bacfd5c`, based on `513c9027dc6519103be26dc352154aab6a3dd144`. Solo pstack Feature / Prove It Works / Show Me Your Work and webapp-testing. Follow-up to the English screenshot review in [usage recovery](grokbot-usage-recovery-2026-10-29.md).

An English UI changed the user-defined bot name `Phiên bản của tôi` to `Version của tôi`. The dictionary and pattern matcher treated conversation content as interface copy. New content boundaries protect bot/group names, pinned accessible names, custom section headings and their context-menu choices, idle snippets/descriptions, thread titles, empty-state names/descriptions, profile hero content, group members in the chat header, speaker labels, picker descriptions and selected member chips. Working/waiting statuses and action labels still translate. Pinned unread labels translate their suffix before combining it with the literal name. SW cache v75.

The translation observer now respects protected ancestors for newly added elements and attribute changes, matching its text-node behavior. This prevents a later DOM update from translating an attribute beneath `data-noi18n` or a Markdown/user-message content region. Existing table toolbar code already translates its own UI strings explicitly.

## Evidence

Windows amd64; Python 3.13.7/Playwright, Chrome 156.0.8078.49, Node 25.9.0 and Go 1.27.1. Loopback state/message fixtures, injected SSE events, read-only requests, no live model/account calls. Mobile 390x844 and desktop 1280x900 screenshots visually inspected.

- RED `content-browser-021240.log`: original list name changed to `Version…`.
- `content-browser-021315.log`: uncovered a missing Pin label translation, added alongside unread suffix.
- `content-browser-021408.log` and `content-browser-021426.log`: test expected the initial description after a simulated agent event had changed it; corrected the expected value to the current fixture description. These were test expectation failures, not evidence of a product regression.
- **157 browser scenario groups PASS**, including 10 new content/localization scenarios; **8 Node tests PASS**, JS syntax, embedded assets and native build PASS. Gate includes list keyboard focus, general adoption, usage recovery, memory lifecycle, chat loading, read-state, dictation, composer and table viewer/scroll regression suites.
- `.audit/2026-10-29-content-i18n/browser-only-delta.json` confirms only four browser assets and one browser test differ from usage code `9fe8014718e5bb06704ccb2b8a70fd87849ad5e3`. Its full Go **1,961 PASS / 38 SKIP** remains prior-checkpoint evidence; no repeated full Go/Core/Rust gate is claimed here.

| Exact command | Result / log |
|---|---|
| `python -u app/tests/aurora/content-i18n-browser.py` | PASS `content-browser-021443.log` |
| `python -u app/tests/aurora/list-focus-browser.py` | PASS `list-focus-021451.log` |
| `python -u app/tests/aurora/adoption-browser.py` | PASS `browser-021455.log` |
| `python -u app/tests/aurora/usage-recovery-browser.py` | PASS `usage-browser-021519.log` |
| `python -u app/tests/aurora/memory-lifecycle-browser.py` | PASS `memory-browser-021531.log` |
| `python -u app/tests/aurora/chat-loading-browser.py` | PASS `chat-loading-021550.log` |
| `python -u app/tests/aurora/read-state-browser.py` | PASS `read-state-021600.log` |
| `python -u app/tests/aurora/dictation-browser.py` | PASS `dictation-021618.log` |
| `python -u app/tests/aurora/composer-browser.py` | PASS `composer-021624.log` |
| `node --test app/tests/aurora/composer-drafts-test.mjs app/tests/aurora/outbox-test.mjs` | PASS `composer-unit-021659.log` |
| `python -u app/tests/aurora/table-viewer-browser.py` | PASS `table-viewer-021700.log` |
| `python -u app/tests/aurora/table-scroll-browser.py` | PASS `table-scroll-021706.log` |
| `python .audit/2026-10-28-grok/syntax.py` | PASS `syntax-021712.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -count=1 ./internal/web` | PASS `web-assets-021714.log` |
| `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe build -o E:\Sach\Sua\neko-aurora\.audit\2026-10-29-content-i18n\neko-aurora.exe ./cmd/neko-aurora` | PASS `build-021717.log` |

Go commands run in `app/native`; other commands run at repository root. Manifest `b01c21ab5dff7fad53fd06db9bec39086b4a931ecf78e0a71fea1298ed409dba`; 1,816 files verified against code commit with existing Git LF normalization. Local validation.jsonl records exact cwd, timestamp, source hashes and unchanged before/after source. 5 candidates and 515 newly reachable history blobs had no recognizable secret matches before checkpoint; detection is not exhaustive.

## Limits

This is a content-boundary fix for the listed conversation surfaces, not a complete localization rewrite or full app translation audit. The dictionary still lacks some UI strings; screenshots retain some Vietnamese UI labels in English mode. Other mixed user/system strings outside these covered surfaces need separate review. No server data migration, provider quality, hosted synchronization, screen-reader application, Tauri runtime or physical-phone acceptance is claimed. Changelog coverage stays **21 partial, one excluded and 798 without feature-specific evidence**; no new upstream bullet is marked validated for this Aurora-specific defect.
