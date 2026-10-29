# Local versioned Pages — 2026-10-29

Backend commit `ee349210bab41ff862b8e54764ac5b68c0a8e109`, based on `7b9d31fb1b266dea484a1fad3053e9cda0cea150`; UI commit `57609b145e3828fe0b4f86fd6b437ed433e2e9bf`. Solo pstack Feature / Prove It Works / Show Me Your Work and webapp-testing, with self-review. No independent reviewer, provider call, credentials, grant, push or deployment.

The owner now has a Markdown library with immutable versions, device drafts, conflict recovery and exact-version downloads. This is a bounded foundation for the Pages direction in [Grok Bot's changelog](https://x.ai/changelog/bot#v0.75.2), not evidence of Bot-authored dashboards or shared/cloud documents. See the [owner guide](../pages.md).

## Persistence and API

[Page records](../../app/native/internal/ledger/pages.go) reuse the existing journal mutex, process lock, durable-before-apply commit and poisoned-store rules. Each update requires the current base revision and a nonce. Repeating the identical nonce/input returns its original immutable result; a changed payload or stale base conflicts. New documents have random IDs. Archive and restore append versions, preserving earlier content. Journal replay checks identities, revision continuity, nonce uniqueness, title/text boundaries, timestamp and SHA-256. Returned event copies cannot mutate the stored Page projection.

The limit is 64 pages including archived pages, 64 KiB UTF-8 text per version and 512 bytes per trimmed title. Empty bodies are allowed; control characters in titles and NUL/invalid UTF-8 in bodies are rejected. History returns at most 50 summaries with an exclusive revision cursor. List rows contain metadata rather than document bodies.

[HTTP routes](../../app/native/internal/aurora/pages.go) require explicit workspace identity. Reads reject ambiguous/unknown query parameters; saves reject extra JSON fields, trailing JSON, invalid UTF-8 and oversized bodies. Exact-version export requires a revision and returns workspace/page/revision/digest headers. The UI verifies those headers, size and exact text before producing a Markdown download. All resource responses use no-store. These owner endpoints do not grant a Bot or teammate access.

## UI and recovery

[Pages UI](../../app/native/internal/web/assets/app/pages.js) opens from New or Settings. It includes search, archived filtering, editable title/Markdown, safe Markdown preview, history pagination, restoration into a draft and downloads. Existing Markdown rendering is reused; document titles and contents remain literal under English localization. Responsive light/dark styles and a native dialog provide keyboard operation, explicit Tab cycling, Escape and return focus. Static assets are registered with the server and service-worker cache v88.

Drafts are isolated by workspace and page. Before sending a save, the browser persists the exact payload and 128-bit random nonce. Storage failure blocks dispatch and leaves a draft download. Pending saves freeze edits. Reopening checks the receipt with GET; retry is explicit and reuses identical bytes/nonce. There is no automatic mutation replay. A recovered receipt can be older than the server's latest version, so the editor checks again. Conflicts keep the draft and require explicit rebase followed by Save. Closing or changing workspace revokes the view and aborts its requests; a late acknowledgement cannot populate another workspace.

## Validation

Windows amd64, PowerShell 7.6, Go 1.27.1, Python 3.13.7, Node 25.9.0 and Chrome 156.0.8078.49. Tests use an allowlisted environment excluding live/provider credentials. Go uses an isolated USERPROFILE/XDG directory. Browser checks preserve OS USERPROFILE for Chrome but use fresh temporary Playwright profiles and loopback fixtures.

- Initial API regression: HTTP 404. The first route implementation exposed a Go ServeMux pattern collision, corrected by moving receipts to `/app/api/page-receipts/{nonce}`. Focused ledger/API tests then passed.
- Full backend: **2091 PASS / 38 SKIP / 0 FAIL across 25 packages**; `go vet` and production build PASS. Tests cover restart, eight competing writers, duplicate/stale nonce, archive/restore, exact limits, history pagination, failed persistence, read-only, forged replay, workspace fencing, malformed input and exact export.
- Initial browser regression: New had no Page menu entry. A later keyboard check showed native Tab traversal leaving the document; explicit cycling fixed it. A nonce-portability check failed when `crypto.randomUUID` was unavailable; the implementation now uses `getRandomValues` and refuses dispatch if secure random generation fails.
- Final [Pages browser fixture](../../app/tests/aurora/pages-browser.py): **16 scenario groups PASS**, all with randomUUID disabled. Covers create, safe preview, reload drafts, conflict, restore, archive, exact export and wrong-scope refusal, lost ACK on update and creation, identical explicit retry, storage failure, size/IME, load errors, 55-version history, late workspace response, keyboard focus and English literal content. Mobile 390×844 and desktop 1280×900 screenshots were inspected, including dark English UI.
- Existing adoption and composer browser suites PASS. Composer/outbox Node tests: **8 PASS**. Final Pages fixture, embedded web asset suite and native production build PASS after the nonce fix. Existing chat/composer sources and fixtures are unchanged by that final fix; their preceding results remain applicable. Their output locations now accept an audit-directory environment variable so old evidence is preserved.

Exact commands and source receipts are in `.audit/2026-10-29-pages/validation.jsonl`; framing and self-review decisions are in `decisions.tsv`. Backend commands from app/native: `go test -v -count=1 -timeout=120s -run TestPage ./internal/ledger ./internal/aurora`, `go test -json -p=1 -count=1 -timeout=300s ./...`, `go vet ./...`, and `go build -o <audit>/neko-aurora.exe ./cmd/neko-aurora`. UI commands from the root: `python -u app/tests/aurora/pages-browser.py`, `python -u app/tests/aurora/adoption-browser.py`, `python -u app/tests/aurora/composer-browser.py`, and `node --test app/tests/aurora/composer-drafts-test.mjs app/tests/aurora/outbox-test.mjs`; embedded assets use `go test -count=1 ./internal/web`.

Backend manifest: `80914c3bb90f11b06e590e4872401549860a2d3f08e34ad9f967cb59736c46f4`, 1864 files verified against its commit. Final UI manifest: `ec22c0523cc867c7100cc1013497ba9829c01d82f0552774cde9d60aa03aaf54`, 1867 files verified. Existing LF normalization is accounted for. No source changed during a gate. Recognizable-secret scans checked seven backend candidates plus 698 newly reachable history blobs, then ten UI candidates plus 705 history blobs, with zero matches; this is not exhaustive. The unrelated adoption-document edit remains excluded.

## Remaining scope and compatibility

History accumulates in the local journal; there is no pruning, aggregate disk quota, permanent Page deletion, cloud synchronization, sharing, collaborative rich-text editing, Bot Page tool or arbitrary executable dashboard. Device drafts use origin-local browser storage and are not encrypted by this feature. Clearing that storage removes draft recovery. Multiple tabs can share that storage; server CAS still protects committed versions, but there is no cross-tab draft merge protocol.

Older binaries that do not know `aurora.page` cannot read a journal after Page writes. Use a compatible binary and preserve the journal when upgrading; no older-format rollback or conversion is supplied. Existing user state was not migrated or opened by these tests. Chrome fixture results are not native WebView, physical-phone, screen-reader or cloud acceptance.

No source bullet is newly marked complete or partially adopted solely because this foundation exists. The stored indexes still contain 34 partial-local entries, one safeguard-preservation exclusion and 885 unvalidated source bullets, 920 total. They are historical bullets, not 920 independent features; the wider backlog remains open.
