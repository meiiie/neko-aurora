# Composer drafts and attachment continuity

Base: `0b988fe7678bd110507eb7a61722cd8dd4532ec9`. This continues the pstack Feature / Prove It Works workflow with one implementer and no delegation.

The composer now saves text and the selected chat reference independently for each workspace and chat. Switching chats restores that chat's draft; returning after a reload restores the saved text. Enqueuing a message clears only that composer draft after the outbox has accepted it. Submitting a question/form answer does not discard an unrelated composer reference.

An upload captures its initial chat and workspace. Its eventual path is added to that draft, including when another chat is open. A workspace switch during image preparation prevents dispatch; the server also rejects an explicit workspace mismatch before writing. An upload already in progress may finish for its original workspace, but its result never enters the new workspace's draft. Each new upload receives a distinct path so choosing another image with the same filename does not overwrite the earlier attachment.

Storage errors preserve draft edits in the current window and report that they were not durably saved. A successful upload does not mask that warning. Late speech-recognition callbacks are ignored after stopping or switching conversations. This is callback isolation, not realtime voice or physical-microphone acceptance.

## Evidence and scope

The [composer browser fixture](../../app/tests/aurora/composer-browser.py) exercises the real UI with loopback API responses. It covers text/reference preservation, reload, delayed and failed uploads, workspace isolation, late speech callbacks, quota failures, successful enqueue and the other chat's untouched draft. The first attempt clicked Back within the existing sheet-close ghost-click guard. The fixture now waits for that documented 400 ms guard, and still uses normal actionable clicks and state assertions.

The [draft-store tests](../../app/tests/aurora/composer-drafts-test.mjs) cover reload, storage failures, workspace/chat key collisions and mutation isolation. The [upload test](../../app/native/internal/aurora/upload_draft_test.go) writes and reads actual temporary files through the computer manager without executing a sandbox command or calling a model. PWA assets include the new script in both the server registry and cache v49.

Browser runs passed on Windows with Chrome 156, Playwright, desktop 1280×900 and a 390×844 viewport. These are not physical-phone, service-worker update, cloud-sync or native Tauri acceptance. Drafts live in this browser's storage. They are not encrypted, synchronized across devices or a blob upload-retry queue; an unfinished file upload must still be selected again after a process crash. Editing the same draft in two tabs uses the most recent successful local-storage write.

Source entry `0.71.2-Fixed-6` receives partial local evidence for preserving a draft and explaining an upload failure. Other source bullets are not marked complete merely because a similar component exists.

Detailed command, base SHA, source manifest and PASS/FAIL records are in `.audit/2026-10-28-composer/validation.jsonl`. The final source manifest SHA256 is `3d6699da208ab629a4547bab977dbe099009b0cd6f34747ba1bd945377df2f0c`; no source changed during these runs:

| Command | Result |
|---|---|
| `go test -json -count=1 -timeout=180s ./...` in `app/native` | 25 packages PASS; 1,540 passing test/subtest events; 37 existing skips; zero failures; 107.19 s (`go-130832.log`) |
| `python -u app/tests/aurora/composer-browser.py` | 9 scenario assertions PASS (`composer-130748.log`) |
| `python -u app/tests/aurora/adoption-browser.py` | 21 scenario assertions PASS (`browser-130807.log`) |
| `node --test app/tests/aurora/composer-drafts-test.mjs app/tests/aurora/outbox-test.mjs` | 7 tests PASS; storage module unchanged since this check (`composer-unit-130609.log`) |
| `go vet ./...` and `go build -o <audit-dir>/neko-aurora.exe ./cmd/neko-aurora` in `app/native` | PASS on the final source manifest (`vet-131039.log`, `build-131042.log`) |

Environment: Windows amd64, PowerShell 7.6, Go 1.27.1, Node 25.9.0, Python 3.13.7. Live/provider environment variables were excluded. The local `checkpoint.json` records the exact code commit and confirms it matches the validated source manifest.
