# Remaining Aurora localization — 2026-11-01

This change completes the remaining reviewed `/app/` localization surfaces in
Vietnamese, English, Simplified Chinese and Traditional Chinese. Work followed
the repository's pstack adoption workflow with one implementer, as requested.
No subagents, model-provider calls, deployment or GitHub push were used.

## Behavior

- Cloud computer: controls, sign-in/control states, recovery and uncertain-outcome
  notices, vault approval, observation fallbacks, screen controls and accessible
  labels. Changing language preserves bot/element/vault selection, typed content
  and the existing one-use confirmation; it does not issue a browser action.
- Assistant communication: directory/session states, delivery versus task status,
  approval titles, counts, dates, byte limits, file validation and send recovery.
  Language changes preserve the draft, recipient, expanded approval and request
  nonce. Message bodies, names, file names and approval contents stay literal.
- Forms, questions, memory import, MCP authorization, scheduling feedback,
  generated conversation metadata and standalone sign-in/onboarding/callback/demo
  pages share the same catalogues. Bound messages update without replacing inputs.
- Privacy and terms have one complete set of sections per language, rather than
  Vietnamese text followed by a shorter English summary. The existing revision
  date and policy meaning are retained. The deletion confirmation documents both
  already-supported literal tokens, `XOÁ` and `DELETE`; tokens are not translated.
- Server-generated agent snippets carry a localization key and parameters.
  Reply excerpts do not, including replies literally saying “Đã xong”.
- Localized image attachment markers remain recognized by the server. Retrying
  an upload after changing language preserves the existing marker and draft.
- Mailbox language changes work independently of the selected date/number region;
  file-limit errors and accessible entry labels use the same catalogue.

Static legacy call sites map through `app/locales/source-keys.json`; new dynamic
messages use semantic keys and named interpolation. `Intl` handles numbers,
dates, lists and plural forms. Traditional Chinese is independently maintained
with Taiwan terminology (帳戶、訊息、權限、核准、設定檔、安全保管庫). OpenCC was an
offline drafting aid, followed by terminology review; no external native-speaker
or legal certification is claimed.

## Validation and reproducibility

Base commit: `782837a7faa3b64d1f603b4ce59a76f3eb05b658`, branch
`codex/openmaus-interface`. Exact candidate hashes, commands, environment,
results and checkpoint mapping are recorded in the local audit directory
`.audit/2026-11-01-localization-complete/`.

Environment: Windows 11, PowerShell 7.6, Node 25.9.0, Python 3.13, Go 1.27.1.
Validation uses an allowlisted environment and fixture providers. Commands:

```text
python -X utf8 tools/build_locales.py --check
node --test app/tests/aurora/localization.test.cjs app/tests/aurora/composer-drafts-test.mjs app/tests/aurora/interrupted-messages-test.mjs app/tests/aurora/outbox-test.mjs app/tests/aurora/working-status-test.mjs app/tests/aurora/ux-test.mjs
go test -p 1 -count=1 ./...             (app/native)
go vet ./...                          (app/native)
go build -o <audit>/neko-aurora.exe ./cmd/neko-aurora
python -X utf8 tools/check_docs.py
```

The browser fixture is isolated on port 63632. Its cloud and mailbox endpoints
cannot reach a provider, real cloud worker or delivery channel. The fixture uses
an ordinary text input for **synthetic** cloud input so its exact preservation can
be checked; the production cloud input remains masked. Four-language UI checks
exercise mailbox recovery/draft preservation, cloud selection/confirmation
preservation, full legal pages, form/question drafts, memory selection and MCP
expiry formatting. `app/tests/aurora/localization-fixture.html` and `.js` are the
reviewable interaction fixtures. These checks are not live cloud/model tests or
physical-phone acceptance.

An initial broad Node glob also selected older Playwright scripts that lack their
local dependency; those did not run. Browser validation uses CUA instead. An
initial Go run inherited the shell environment and failed the default-provider
expectation; a concurrent account-deletion timing check also failed. Both passed
when rerun in the allowlisted environment. Final full-suite receipts are recorded
separately; earlier failures remain in the audit logs.

Final checks passed: 642 semantic messages and 969 transitional messages in each
catalogue; syntax checks for 32 JavaScript files; 32 Node tests; the full Go suite;
focused image/upload/snippet contract tests; embedded web-asset tests; and the
documentation check. Sixty CUA UI checks cover the four languages, including
390-pixel layouts and a fixed `en-GB` format region while changing UI language.
Email field contents are redacted by the browser, so exact email-draft retention
is not claimed; form, question, onboarding, cloud and mailbox draft preservation
were directly verified with synthetic text.

The full Go run records its starting source manifest. Final image-marker changes
were then covered by the focused contract run; final frontend changes were
covered by the Node, web-asset and browser checks. The final candidate manifest
and checkpoint receipt identify the delivered source without treating an earlier
candidate test as a test of unreviewed later edits.

## Requested Grok UI/UX reference

Cloned [xingyingyuzhui/grok-bot-ui-ux-kit](https://github.com/xingyingyuzhui/grok-bot-ui-ux-kit)
to `reference/grok-bot-ui-ux-kit`, pinned at
`5bb4379e9ec1158b9ec29c3c5f1e2cc6604be5af`. It is ignored by the parent repository.
The kit describes a reconstruction of Grok Bot 0.68.1, with website observations
and extracted static application evidence kept distinct. It is not the official
application source or a working backend.

Its `NOTICE.md` preserves the original owners' rights and grants no new license
to the extracted assets/code. No reference assets or source were copied into
Aurora. The user-requested local preview runs separately at
`http://127.0.0.1:8766/`, using the shipped static pages and local sample data.
Typing and sending a simulated message and opening the computer preview were
verified. The preview explicitly has no model, account, payment or remote host.

Windows Git initially converted line endings. Tracked files were restored from
the exact Git archive with `core.autocrlf=false`, preserving the upstream bytes.
`python -X utf8 scripts/verify.py` then passed the original source/asset hashes,
136 token anchors, 808 icons, avatar evidence, links and JavaScript syntax.
The npm dependency-based suite was not claimed as run.

Useful interaction references include explicit simulation boundaries, consistent
composer behavior, a separate task context, and computer controls grouped with
their current state. The kit does not supply a multilingual architecture; Aurora
keeps its own catalogues, trusted-host boundaries and approval semantics.
