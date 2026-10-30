# Selected message excerpts

Code commit: `1c8f558324c83dad9982249ce7c9816d54a23428`, based on `25898d7553ad5a6a5d609f435a73685f3cb8886e`. Local-only pstack Feature / Prove It Works checkpoint, one implementer, no subagents.

Selecting text inside one rendered message offers **Thêm vào chat / Add to chat**. Activation appends the selected plain text and its visible author to the current draft. It preserves the existing draft, chat reference and structured reply quote; it never sends a message. The result uses the existing workspace/chat draft store and its storage-failure warning. It is a plain-text excerpt authored by the user, not a new server-verified quotation or a cross-chat reference.

A captured selection is bound to its workspace, chat, source DOM range and exact text. Changing chats/accounts, replacing the source, selecting across messages or selecting controls invalidates insertion. Text outside message bodies is excluded. A selection over 2,000 Unicode code points or an insertion that would exceed the 10,000-code-point draft limit is refused without truncation. The control supports keyboard activation, Escape with focus return, translated labels and viewport bounds. It hides when the source scrolls outside the visible chat area.

Message rerenders preserve a selection only when its original nodes, text and context remain valid. Automatic scrolling checks for a live selection at the time the scroll would run, including previously scheduled animation frames. This fixed a browser-test race in which a pending render scroll moved the selected text off-screen. Source changes still invalidate the selection. Streaming text itself is not promised to keep a selection while its DOM text is replaced.

PWA cache v53 and the Go public asset registry include the module. This extends partial local evidence for source entry `0.70.2-Improved-8`, which also has the earlier structured quoted-reply work. Coverage totals remain 10 partial, 1 intentionally not adopted and 809 not validated.

## Evidence

Windows amd64; PowerShell 7.6; Go 1.27.1; Node 25.9.0; Python 3.13.7; Chrome 156.0.8078.48 via Playwright. API/model responses use loopback fixtures; no provider, real account or paid service is contacted. The new selection fixture uses actual DOM ranges, normal button clicks and keyboard events. A 390×844 screenshot was visually inspected. This does not prove touch handles, physical phones, other browser engines, Tauri acceptance, real model behavior or service-worker update behavior.

Final source manifest SHA-256: **`2e8fba5b2724a72967901025e38875e27362d9c1f6150ff0dddd37bb71692830`**. No source changed during the final checks or before the code checkpoint. Full command/environment records are in `.audit/2026-10-28-selection/validation.jsonl` and `code-checkpoint.json`.

| Command | Result / local log |
|---|---|
| `python -u app/tests/aurora/selection-browser.py` | PASS: 10 scenarios; `selection-browser-154605.log` |
| `python -u app/tests/aurora/composer-browser.py` | PASS: 23 scenarios; `composer-154614.log` |
| `python -u app/tests/aurora/upload-browser.py` | PASS: 11 scenarios; `upload-browser-154647.log` |
| `python -u app/tests/aurora/adoption-browser.py` | PASS: 21 scenarios; `browser-154656.log` |
| `node --test app/tests/aurora/composer-drafts-test.mjs app/tests/aurora/outbox-test.mjs` | PASS: 8 tests; `composer-unit-154720.log` |
| `go test -count=1 ./internal/web` in `app/native` | PASS; `web-assets-154721.log` |
| `go build -o <audit>/neko-aurora.exe ./cmd/neko-aurora` in `app/native` | PASS; `build-154725.log` |
| `python tools/check_docs.py` | PASS; repeated for this report |

The retained initial failure `selection-browser-154017.log` came from separating scroll and selection operations across a detached DOM node; the fixture now takes a range synchronously on the live node. Failures `154201` and `154351` exposed startup/render/scroll timing. The implementation now preserves stable ranges and checks selection inside the scheduled scroll callback; the language-reload fixture also waits for initial network idle. No timeout was raised and no failed scenario was skipped.

The recognizable-secret scan found no matches in 8 candidate files or 169 newly reachable history blobs before the code checkpoint. The full Go suite and Core checks from earlier checkpoints were not rerun for this UI/asset-only change; the affected asset package, build and all relevant browser/draft suites were run.
