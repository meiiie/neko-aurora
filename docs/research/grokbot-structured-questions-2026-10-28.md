# Structured question cards and durable answers

Base: `4f82d204490171f16d9fc4957ee2dc266e9dc0e0`. This follows pstack Feature / Prove It Works with one implementer, review and local fixtures. It extends source entry `0.77.0-New-3`, which already had partial evidence for single questions; it does not increase the number of covered entries.

Aurora parses one to four questions with stable ordinal IDs, two to eight options per question and optional multiple selection. The model can emit compact or multiline JSON. The journal stores validated question structure, preserves it across restart and deep-copies options on reads. Existing `CHOICES` records remain readable. A structured card takes precedence when both protocols appear.

The card offers next/back navigation, custom answers, a review screen and a workspace/chat/question-scoped draft. Storage failure keeps the current edits in memory and explains that they were not saved. Question text and options are model content: changing the UI language does not translate or alter them. Native radio/checkbox controls and focus restoration support keyboard interaction.

The server resolves IDs and selected indexes against the journaled source. It rejects missing, duplicate, out-of-range or contradictory answers, wrong threads and submissions without a completed source. Canonical text comes from source labels, not client labels. In a group, an answer goes to the bot that asked; an option containing `@all` does not fan out to other bots. A fixed question nonce preserves the same receipt after lost responses and restart. A different subsequent answer conflicts. Normal follow-up messages leave the question open.

The UI only shows completion after an accepted message appears in history. Offline submission and a lost HTTP response retain the full structured payload in the existing outbox. Question drafts and pending messages remain browser-local, unencrypted and unsynchronized. Native devices, assistive technology and real model quality have not been accepted by these tests.

## Validation

The [Go contract tests](../../app/native/internal/aurora/questions_test.go) exercise parsing, a scripted loopback model, HTTP validation, group routing and receipt recovery. The [ledger tests](../../app/native/internal/ledger/direct_questions_test.go) cover invalid updates, mutation isolation and restart. The [browser fixture](../../app/tests/aurora/composer-browser.py) exercises selections, navigation, custom answers, reload, offline send, lost-response replay, language isolation and viewport fit.

Initial failures exposed a mistaken test expectation for the ledger's existing `ErrConflict` update contract and automatic translation of model option text. Both were corrected; failed runs are retained in local evidence. A final review also restored clearing of obsolete question fields when a resumed run completes without another card.

Exact commands, Windows environment, source manifests and PASS/FAIL results are retained in `.audit/2026-10-28-questions/validation.jsonl`. Live/provider environment variables were excluded. The local `checkpoint.json` binds the validated source manifest to the exact code commit. These fixtures do not call a real provider, deploy the application or establish cloud/device parity.

| Command | Final result |
|---|---|
| `go test -json -count=1 -timeout=180s ./...` in `app/native` | 25 packages PASS; 1,546 passing test/subtest events; 37 existing skips; zero failures (`go-133811.log`, 95.62 s) |
| `python -u app/tests/aurora/composer-browser.py` | 17 scenario assertions PASS, including editable question layout at 390×844 |
| `python -u app/tests/aurora/adoption-browser.py` | 21 scenario assertions PASS (`browser-133746.log`) |
| `go vet ./...`; `go build -o <audit-dir>/neko-aurora.exe ./cmd/neko-aurora` | PASS (`vet-133238.log`, `build-133257.log`); production Go source unchanged afterward |
| `python tools/check_docs.py` | PASS |

The full Go source manifest is `5ece6f89e5827f84c9edbb59bf309bd6c2639d66906e55db27ce8a7f14b8648a`. A later browser-only adjustment waits for the finite chat transition before checking geometry and capturing the mobile image. A pre-existing steering fixture now waits for the model connection instead of sleeping, and accepts only a queued message or one demonstrably absorbed in the running turn's journal. One earlier full run caught that timing-sensitive assumption. Two earlier navigation checks also failed and did not recur in subsequent browser runs; failure diagnostics remain enabled rather than treating them as conclusively fixed.
