# Recoverable image uploads

Code commit: `868bff92fb327dfbb7d7d59e4d2287a1557031e3`, based on `0a49946092cef2661f61dd1dcfcc803dc03da536`. Local checkpoint only. One implementer following pstack Feature / Prove It Works; no subagents or live provider calls.

## Behavior

Compressed images enter a bounded browser queue before dispatch. The storage promise resolves on transaction completion, not the individual write request. A reload or closed window restores pending images for explicit retry; it never sends a chat message automatically. The queue is scoped by workspace and bot, limits each image to 1 MiB and the browser queue to 12 images / 8 MiB decoded bytes. Ready receipts are stored before inserting the path into a draft. If draft storage fails, the ready image remains available for insertion after reload. Retrying insertion checks for the existing reference. Storage failures are visible, without claiming that an aborted write saved the image.

The HTTP route accepts opt-in `deduplicate:true` with mandatory matching workspace identity. A full SHA-256 of the bytes and a bounded, stable sanitized name determine the path. Existing identical regular files return the same path without charging quota twice. A differing file, directory or symlink is a conflict. File publication uses a random exclusive staging file, sync/close and exclusive hard link; competing writers cannot overwrite a prior path. Legacy callers retain unique-path behavior. Both paths remain compatible with the image-reading tool.

Upload POSTs have a 30-second response deadline. A timeout is an unknown receipt, not proof that the server rejected the bytes; retry resubmits the same body to the content-addressed route. A late result remains tied to the original workspace/chat. UI controls allow retry, insertion and removal from the local queue. PWA cache v52 and the Go public asset registry include the queue module.

The transaction boundary follows the [IndexedDB complete event](https://developer.mozilla.org/en-US/docs/Web/API/IDBTransaction/complete_event). This is browser storage, not cloud synchronization or encrypted storage. Browser eviction/clearing, power-loss durability, multi-tab atomicity across IndexedDB and localStorage, hard-link support on every filesystem and native/physical-phone acceptance are not proven. Tests close/reopen a page and abort transactions; they do not kill the OS or simulate power failure. A server crash can leave a staging file consuming quota. Bot processes may subsequently modify uploaded files; a retry detects changed bytes rather than treating the path as immutable. Removing a queue item does not delete an already uploaded server file.

## Validation

Windows amd64; PowerShell 7.6; Go 1.27.1; Node 25.9.0; Python 3.13.7; Chrome 156.0.8078.48 via Playwright. Runner uses an allowlisted environment without live/provider credentials. Local logs: `.audit/2026-10-28-uploads/validation.jsonl` and `code-checkpoint.json`. Final source manifest SHA-256: **`e3da1d989e4feb401fac2640f02b743a0290a399366eecf5fd7db484bc0dbef0`**. Source stayed unchanged during every final run and matches the committed Git tree after its existing CRLF normalization (90 files).

| Command | Result / local log |
|---|---|
| `go test -json -p=1 -count=1 -timeout=300s ./...` in `app/native` | PASS: 1,571 test/subtest events, 37 existing skips, 0 failures; all 25 packages; 181.88 s; `go-serial-152051.log` |
| `go vet ./...` in `app/native` | PASS; `vet-152353.log` |
| `go build -o <audit>/neko-aurora.exe ./cmd/neko-aurora` in `app/native` | PASS; `build-152357.log` |
| `python -u app/tests/aurora/upload-browser.py` | PASS: 11 scenarios; `upload-browser-152007.log` |
| `python -u app/tests/aurora/composer-browser.py` | PASS: 23 scenarios; `composer-152401.log` |
| `python -u app/tests/aurora/adoption-browser.py` | PASS: 21 scenarios; `browser-152438.log` |
| `node --test app/tests/aurora/composer-drafts-test.mjs app/tests/aurora/outbox-test.mjs` | PASS: 8 tests; `composer-unit-152502.log` |
| `python tools/check_docs.py` | PASS before checkpoint; repeated for this report |

The new restart/retry HTTP regression failed on the previous implementation (`upload-storage-151148.log`, status 400 after duplicate quota accounting), then passed after the fix. New filesystem tests cover same-body concurrency, two managers competing for a path, original-byte preservation, quota, path escape, symlink refusal and normal temporary-file cleanup. These tests passed with real temporary files; no sandbox command was executed by the upload-specific fixtures.

Browser cases cover lost responses, page close while uploading, keyboard retry, account switch, failed draft persistence, interrupted queue deletion, aborted image transaction, byte/item caps, invalid server paths, aborted ready-receipt persistence, and timeout after fixture server acceptance. API responses are loopback fixtures; no model quality or production-service acceptance claim. Screenshot inspection at 390×844 confirms no horizontal overflow; it is not phone acceptance. Go race tests were not run (no C toolchain selected).

The recognizable-secret scan found zero matches in 11 candidate files and 150 newly reachable history blobs before the code checkpoint. No credentials or matched values were printed. The initial post-commit byte-for-byte check detected Git line-ending normalization; a subsequent Git blob identity check verified every file, without altering source or rerunning already valid tests.

## Source coverage

This extends partial evidence for `0.71.2-Fixed-6` (draft preservation during upload failure). It does not turn every attachment-related changelog bullet into a completed feature. The existing limits remain: no general 200 MB file uploader, remote multi-device upload queue or all file-type previews. Coverage totals stay 10 partial, 1 intentionally not adopted, 809 not validated.
