# Localization foundation and Settings — 2026-11-01

Starting code commit: `bcb789a1ae2b1ca90328b60a193585e0de4041cd` on
`codex/openmaus-interface`. Work was performed solo using the repository's
pstack-adoption evidence workflow, without subagents or model/provider calls.

The previous language control cycled through auto/Vietnamese/English and reloaded
the page. English relied on a Vietnamese-source DOM dictionary with 659 entries;
many newer controls had no entry. Date and time formatting was mostly hardcoded
to `vi-VN`. This pass adds explicit language and formatting-region selectors,
four local catalogues (including both Chinese scripts requested by the user),
in-place reversible switching and offline i18next 26.6.0.

The catalogues contain 969 legacy messages and 51 semantic messages per language.
There is complete parity **within those catalogues**, not a claim of complete
translation of all repository strings. New Settings messages use semantic keys;
older views have a compatibility adapter with protected user-content boundaries.
Main chat timestamps, relative times, number formatting, sorting and speech
locale use the selected locale. Saved conversation content, model selection,
streaming bodies, drafts and schedule time zones are not rewritten.

The design follows BCP 47 language/script/region separation, ordered device
preferences, CLDR/Intl formatting, full-message interpolation and explicit
plural forms. See [localization guide](../localization.md) for primary sources,
the vocabulary policy, translator workflow and remaining migration scope.

## Validation evidence

Environment: Windows 11, PowerShell 7.6, Node 25.9.0, Python 3.13, Go 1.27.1.
No provider credentials are needed for the test fixture or build.

| Check | Command or method | Result |
| --- | --- | --- |
| Catalogues | `python tools/build_locales.py --check` | PASS: four locales, key parity, nonempty values, placeholders and generated bundle |
| Runtime and chat regressions | `node --test app/tests/aurora/localization.test.cjs app/tests/aurora/composer-drafts-test.mjs app/tests/aurora/interrupted-messages-test.mjs` | PASS: 14 tests |
| Embedded scripts | `go test ./internal/web -count=1` from `app/native` | PASS: local asset serving/content types and cache revalidation |
| Native control | `go build -o <audit>/neko-aurora.exe ./cmd/neko-aurora` | PASS: frontend embedded in Go executable |
| Documentation | `python tools/check_docs.py` | PASS within the existing checker scope |
| Desktop browser | CUA, local fixture at `127.0.0.1:63632`, normal 1280×720 viewport | PASS: Settings, navigation and conversation controls |
| Narrow browser | CUA at 390×844, four successive language selections | PASS: controls fit, no horizontal page overflow, original draft and messages unchanged |
| Persistence | Select Simplified Chinese with UK formats, reload, reopen Settings | PASS: both preferences and the composer draft survive |
| Content protection | Literal bot name `Phiên bản của tôi`, owner name `Cài đặt`, mixed-script user/assistant text | PASS: names/content stay literal while action labels change |
| Locale edge cases | Unit tests for `zh-TW`, `zh-HK`, script-over-region tags, unsupported scripts, ordered fallback, storage failure and cross-tab events | PASS |

One format bug was caught and fixed: an unnecessarily maximized `en-Latn-GB`
identifier fell back to US-style dates in the tested Intl implementation. The
resolved formatting tag now keeps the language and region without an unnecessary
script subtag. Traditional Chinese wording received explicit adjustments such
as 設定、儲存、帳戶、訊息、排程、登出、觸覺回饋.

Exact command receipts, the source SHA-256 manifest and browser observations are
in the ignored local audit directory `.audit/2026-11-01-localization/`:
`validation.jsonl`, `candidate-manifest.json`, `browser-check.json` and screenshots.
The final local launch receipt records the exact committed source, binary hashes,
unchanged state directory and per-bot model selections. Candidate files and newly
reachable history are scanned before the local checkpoint; matched secret values
are never printed. No GitHub push or deployment is part of this change.

This is UI/locale validation, not model evaluation, cloud sync, external linguistic
certification or testing on a physical phone. Legal text and portions of the
cloud/network and older operational views remain a separate migration batch.
