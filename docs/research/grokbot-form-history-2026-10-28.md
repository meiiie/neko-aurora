# Durable form definitions

Code commit: `d77718fc559f5f782a4e5cc8689355db815ead1b`, based on `58e0cd5e0f73bd0a7d3306a2a8ad40e554d2a90d`. Solo pstack Feature / Prove It Works / Show Me Your Work. Extends the existing partial form evidence associated with [Grok Bot 0.77.0](https://x.ai/changelog/bot); no reference implementation or assets copied.

## Behavior

An unanswered form previously disappeared when the shared 300-entry thoughts/forms cache evicted it. New form definitions now live in the completed turn's ledger event. They survive eviction and journal reopen, with fields/options copied independently for inputs, returned values, snapshots and display. Legacy side-cache forms remain readable and now also receive deep copies; evicted legacy definitions cannot be reconstructed.

Definitions are bounded to ten fields, eight select options, 80-character titles/labels, 40-character unique field names and 60-character options. Supported types are text, textarea, number, date and select. The parser supplies deterministic names for unnamed fields, removes duplicate names/options and irrelevant options, and ignores model-supplied completion flags. The journal validates definitions and refuses forms on started, updated or failed turns. Successful completion is the sole admission point; approval decisions cannot rewrite the saved form.

Forms pause initial or in-progress plans and skip premature artifact verification. A failed final journal write emits an error instead of RUN_FINISHED, approval side effects or a success notification. New form completion requires a genuine owner receipt; a matching legacy nonce alone is insufficient. Group answers now target the form's author, fixing the previous general-routing behavior that could select another bot. Existing answer receipt replay stays compatible.

## Validation

Windows amd64, PowerShell 7.6, Go 1.27.1, Node 25.9.0, Python 3.13.7, Chrome 156.0.8078.48. Provider/account environment variables excluded. Model responses are in-memory or loopback fixtures. Exact commands, source snapshots and results: `.audit/2026-10-28-form-history/validation.jsonl`.

| Command (Go commands in `app/native`) | Result |
|---|---|
| `go test -v -count=1 -timeout=120s -run 'Test(FormHistory\|DirectForm\|FormCompletion\|FormDefinition)' ./internal/aurora ./internal/ledger` | RED on base: eviction loses form, both plan cases make extra calls; `form-history-174848.log`. Expanded group test exposed wrong recipient in `form-history-175211.log`. Final PASS: `form-history-175237.log` |
| `go test -v -count=1 -timeout=120s -run 'Test(FormReceipt\|FormReplay\|ProviderModelThinkingAndForm)' ./internal/aurora` | PASS: legacy compatibility; `form-replay-175114.log`; also included in final full run |
| `go test -json -p=1 -count=1 -timeout=300s ./...` | PASS: 1,680 test/subtest events, 37 existing skips, 0 failures, 25 packages; 220.15 s; `go-serial-175244.log` |
| `go vet ./...`; `go build -o <audit>/neko-aurora.exe ./cmd/neko-aurora` | PASS: `vet-175630.log`, `build-175633.log` |
| `python -u app/tests/aurora/adoption-browser.py` | PASS: 21 existing scenarios, desktop 1280 × 900 and mobile viewport 390 × 844; `browser-175637.log` |

Final focused, full-Go, vet, build and browser source manifest: `5fb7b811e3136313a8dd3de2600d23b1fa13c5b0ba15e59f5f0a9ceb81ef8a6b`. No source changed during these checks. Candidate scan: 10 files and 247 newly reachable history blobs, zero recognizable secret matches, no values printed. No frontend source changed in this unit.

## Limits

Journal reopen and simulated closed-store failure are not OS-power-loss, multi-device sync or model evaluation. Browser fixtures are not native/physical-device acceptance. No Go race run with the installed C toolchain. A failed final write leaves the work unfinished for the existing restart/lease recovery mechanism; partial text may already be present. Legacy forms retain their historical completion fallback. Answers still use the existing text payload; authoritative per-field validation and unsent form drafts remain separate work. The existing partial changelog entry stays partial.
