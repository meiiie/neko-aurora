# Quoted replies with pinned source context

Base: `be8e1408ccb8d675b0bc9da3d3ec51f66fe2e580`. The pstack Feature / Prove It Works loop reproduced the missing server validation, added the behavior and reviewed local evidence without delegation.

An owner can quote a user message or a completed assistant reply in the same private/group conversation. The server resolves the source from its own history; client-provided display text is not authoritative. Cross-thread, peer, collaboration and unfinished-assistant sources are rejected. The quote contributes data context to the model, and does not change group mention routing or grant a tool permission.

The existing atomic owner-message frame now holds the source ID, task, role, author and a bounded text snapshot. Work IDs index that receipt for history and SSE projection. The owner's message stays separate from the model's appended quote/reference context. Lost-response retries keep the first receipt; a different quote using the same nonce conflicts. Replay validates the snapshot against the batch and source conversation. Read/caller mutation cannot modify stored receipts. Older version-2 frames without quote fields remain readable; older binaries are not promised forward compatibility with the new fields.

The PWA offers a reply action, quote preview and removal while preserving typed text. Drafts remain scoped to browser, workspace and chat. An offline send retains the selection in the outbox and history renders the accepted server snapshot. Model/source content is rendered as text and excluded from automatic UI translation. Long snapshots can be expanded. A source button expands the rendered window and focuses the original message when it is in loaded history. If it is outside the loaded history, the app says so and retains the snapshot. PWA asset cache is v51.

Source entry `0.70.2-Improved-8` receives partial local evidence. Selection-based Add to chat, complete-history navigation, real devices and native/client synchronization are not covered. The current snapshot is limited to 2,000 Unicode code points and the combined model input retains the existing ledger byte ceiling. A rejected send stays in the outbox; it is not silently truncated or retried as a new message.

## Evidence

The [HTTP tests](../../app/native/internal/aurora/quoted_reply_test.go) cover source validation, user/assistant/group behavior, context injection, restart, history and nonce conflicts. The [ledger tests](../../app/native/internal/ledger/quoted_reply_test.go) exercise immutable receipts, their recovered work index and rehashed tampering. The [browser fixture](../../app/tests/aurora/composer-browser.py) covers switching conversations/workspaces, reload, offline send, lost responses, HTML as text, expansion and removal. The [draft-store test](../../app/tests/aurora/composer-drafts-test.mjs) covers quote-only drafts and mutation isolation.

The first red HTTP run (`quotes-140213.log`) showed the old handler accepting a request that selected a source from another conversation. It ignored the unsupported `replyTo` field; it did not expose that source's contents. The new request contract rejects the source instead. All tests use temporary local journals, loopback APIs and a headless browser; no real provider or account is involved.

A navigation test adds 65 intervening messages so the source is outside the initial render window. It caught focus being dropped when the top-scroll handler loaded older items. Thread rendering now restores focus to the same message identity; the regression retains the focus assertion.

Exact commands, code manifests, Windows environment and PASS/FAIL records are retained in `.audit/2026-10-28-quotes/validation.jsonl`. The final `checkpoint.json` binds the validated source to its code commit. Browser viewport evidence is not physical-phone or cloud-sync acceptance.

| Command | Result |
|---|---|
| `go test -json -p=1 -count=1 -timeout=300s ./...` in `app/native` | 25 packages PASS; 1,566 passing test/subtest events; 37 existing skips; zero failures; 200.87 s (`go-serial-141239.log`) |
| `go vet ./...` in `app/native` | PASS (`vet-141600.log`) |
| `python -u app/tests/aurora/composer-browser.py` | 23 scenario assertions PASS (`composer-141956.log`) |
| `python -u app/tests/aurora/adoption-browser.py` | 21 scenario assertions PASS (`browser-142029.log`) |
| `node --test app/tests/aurora/composer-drafts-test.mjs app/tests/aurora/outbox-test.mjs` | 8 tests PASS (`composer-unit-141238.log`) |

The final frontend manifest is `0aebe96a15688b0e6db02594c1852278d53720bcd31407fd6a9bb4bff504712d`. After the whole-Go run, only frontend navigation/focus and browser fixture code changed; Go source stayed unchanged. The Go web-asset suite and embedded executable build were rerun for that final frontend. Environment: Windows amd64, Go 1.27.1, Node 25.9.0, Python 3.13.7 and headless Chrome 156, with provider/account environment variables excluded.
