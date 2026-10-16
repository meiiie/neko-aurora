# Local conversation history export — 2026-10-12

`native/internal/historyexport` is a pure Go encoder with no ledger, runtime, file-system, network, provider, grant, or UI dependency. It accepts only an explicit public DTO; it does not inspect or reflect a `Work` record. Standard JSON serialization operates on these DTO fields only.

## Contract

`Encode(history History, format string) ([]byte, error)` accepts exactly `json` or `text`. `History` contains `Version`, `ConversationID`, `Title`, `BotID`, `Archived`, `AtSequence`, and `Messages`. Each `Message` contains `RequestID`, `BotID`, `CreatedAt`, `Text`, `Reply`, `Status`, and `Files`. Each `File` contains `ID`, `Name`, `SHA256`, `Bytes`, and `DownloadPath`. JSON keys use the corresponding lower-camel spelling.

`Version` must be 1. JSON is deterministic, indented UTF-8 with a terminal newline; nil message/file collections become `[]` without changing caller data. Text is a literal plain text transcript. Script tags, HTML-like strings, quotes, backslashes, Markdown fences, and backticks remain data; the encoder adds no executable wrapper. It performs no template evaluation. Root must serve the returned bytes with the matching JSON/plain-text MIME type, prevent content sniffing, and use a safe fixed-prefix download filename rather than interpolating the title.

Errors return **nil bytes**, never a truncated or partial export. `errors.As` can inspect `*historyexport.Error`, whose fixed `Kind` and `Field` identifiers never include source values. Kinds are `invalid_history`, `unsupported_format`, and `limit_exceeded`.

## Validation and bounds

| Field/check | Bound |
| --- | ---: |
| Messages | 500 |
| Encoded output, including JSON escaping and formatting | 1,048,576 bytes |
| Aggregate supplied string bytes before encoding | 1,048,576 bytes |
| Files per message | 64 |
| Each ID | 128 ASCII bytes |
| Title and each file name | 256 UTF-8 bytes |
| Each message text or reply | 65,536 UTF-8 bytes |
| Created timestamp and status | 64 bytes each |
| Download path | 512 bytes |
| File byte-size metadata | 0 through 1,073,741,824 |

IDs start with an ASCII letter/digit and contain only ASCII letters/digits, `-`, `_`, or `.`. Request IDs are unique across messages; file IDs are unique across all exported messages. Repeated bot IDs are valid. Titles and text may be empty; file names must be nonempty. All strings must be valid UTF-8. Text/reply permit literal tab, CR, and LF; other Unicode control characters are rejected. Other metadata rejects all control characters. Status starts with a lowercase ASCII letter and contains lowercase letters/underscores. Timestamps use a bounded RFC3339Nano parse with two-digit hours, a period for fractional seconds, and timezone hour/minute bounds; year zero is rejected.

SHA-256 values must be exactly 64 lowercase hexadecimal characters. An empty `DownloadPath` is valid metadata-only/unavailable information. A nonempty path must equal `/v1/channels/` + the exact exported `ConversationID` + `/artifacts/` + the exact file `ID`. Cross-conversation/file pointers, external URLs/hosts, query strings, fragments, percent escapes, backslashes, and traversal forms therefore fail validation.

The byte-size/hash/name/path tuple is metadata, not file bytes, availability, readiness, or permission. A user must make a **separate authenticated download** through the existing local route. The text export states this explicitly. Exporting metadata creates no grant and changes no access policy.

Root owns publication/access filtering and DTO projection. It must select only original published owner text, public reply/status, and currently authorized file metadata; omit revoked/uncertain files or leave their path empty. Internal collaboration-stage instructions, private context, account/profile information, nonces, leases, authority/runtime records, and other private fields must never be copied into this DTO. The encoder cannot identify private material incorrectly placed into the caller-supplied `Text` or `Reply`; it does not redact or silently alter those values. The DTO-field test establishes this narrow serialization boundary, not the ledger's publication/access behavior.

## Validation record

Recorded at `2026-10-12T02:58:20Z`. Exact HEAD: `caaa9f7de0adb25075460e82441082787041cd27`, branch `local-conversations-retention-caaa9f7`. Exporter additions were **working tree**, not a new code commit. No index/staging/checkpoint/push was performed by this worker. Root must scan candidate files for secrets before its checkpoint and record/revalidate the exact integrated code commit.

Environment: Windows/amd64, PowerShell, explicit `go1.27.2 windows/amd64` at `C:\Users\Admin\Documents\Codex\2026-10-11\task\.tooling\go\bin\go.exe`. Go working directory: `C:\Users\Admin\Documents\Codex\2026-10-11\task\checkpoint-verify-20261011\native`. All caches, temporary files, and logs used `E:\CodexScratch\conversations-retention-20261012\export-worker`; no third-party dependency was added. `GOPROXY=off`, `GOSUMDB=off`, `GOTOOLCHAIN=local`, and `GOFLAGS=''`; `GOCACHE`, `GOTMPDIR`, `TEMP`, and `TMP` pointed to this scratch directory's cache/tmp children. Reference and manifest output fixture environment variables were unset. Provider credential environment variables were removed by name without printing values using the case-insensitive pattern `KEY|TOKEN|SECRET|PASSWORD|CREDENTIAL|OPENAI|ANTHROPIC|GROQ|GEMINI|XAI|AZURE|AWS_|GOOGLE_|HF_|HUGGINGFACE|COHERE|MISTRAL|DEEPSEEK|NEBIUS|TOGETHER`.

| Result | Exact command arguments on the explicit Go executable | Evidence under scratch `output/` |
| --- | --- | --- |
| FAIL, expected TDD red | `test ./internal/historyexport -count=1 -v` against a typed-error placeholder | `tdd-red.txt`; JSON/text success, collection normalization, supported quota, escaped-output quota, and format expectations failed before implementation |
| FAIL, expected added timestamp red | `test ./internal/historyexport -count=1 -run TestEncodeRejectsNonRFC3339TimestampsAndEmptyStatusIdentifiers -v` | `strict-time-red.txt`; permissive Go timestamp parsing exposed before strict guards |
| PASS | `test ./internal/historyexport -count=1 -v` | `package-final-strict.txt`; 14 top-level tests passed, including Unicode, literal scripts/fences, controls, exact public DTO keys, nil normalization/no input mutation, duplicate IDs, unavailable/cross-scope pointers, 500-message quota, UTF-8 byte bounds, JSON expansion, and exact 1 MiB output with one-byte excess rejection |
| PASS | `vet ./internal/historyexport` | `vet-final.txt`; exit 0, no diagnostics |
| FAIL, integration pending | `test ./... -count=1` | `native-suite.txt`; ledger compilation failed because in-progress `conversation_history_test.go` referenced missing `ExportConversation`, `SetRetentionPreference`, `PreviewRetention`, and `RetentionInput`. Nine other test-bearing packages passed; two had no tests. This run preceded the final strict timestamp guard and is not integrated acceptance. |

Outputs were captured with `2>&1 | Tee-Object -FilePath "$scratch\output\<file>"`; each Go exit code was propagated through `exit $LASTEXITCODE`. Formatting used the matching `gofmt.exe -w internal/historyexport/export.go internal/historyexport/export_test.go`.

Final raw-file SHA-256:

| File | SHA-256 |
| --- | --- |
| `native/internal/historyexport/export.go` | `35f13d41457295bccd722ecedce65c7c73684176f33350362092df8f431dd078` |
| `native/internal/historyexport/export_test.go` | `d533aea3180e6e16d183334227e04cb2818225dbc001d53b7311828d684e11b7` |

No exporter implementation blocker remains. Integrated ledger/API/UI acceptance and validation on the final code commit remain pending with Root. These checks establish local encoder behavior, not cloud sync, model evaluation, actual download readiness, new authorization, browser/phone acceptance, or provider execution.
